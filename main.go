package main

import (
	"context"

	"github.com/statechannels/go-nitro/protocols"

	"github.com/statechannels/go-nitro/types"
	"github.com/testground/sdk-go/network"
	"github.com/testground/sdk-go/run"
	"github.com/testground/sdk-go/runtime"
	"github.com/testground/sdk-go/sync"
)

type PeerEntry struct {
	Address types.Address
	Url     string
}

type PeerTransaction struct {
	Transaction protocols.ChainTransaction
	From        types.Address
}

const PORT_START = 7000

func main() {
	run.Invoke(createLedgerTest)
}

func createLedgerTest(runenv *runtime.RunEnv) error {
	ctx := context.Background()
	// instantiate a sync service client, binding it to the RunEnv.
	client := sync.MustBoundClient(ctx, runenv)
	defer client.Close()

	// instantiate a network client; see 'Traffic shaping' in the docs.
	netclient := network.NewClient(client, runenv)
	runenv.RecordMessage("waiting for network initialization")

	// wait for the network to initialize; this should be pretty fast.
	netclient.MustWaitNetworkInitialized(ctx)

	peerInfoTopic := sync.NewTopic("peer-info", &PeerEntry{})
	transTopic := sync.NewTopic("chain-transaction", &PeerTransaction{})

	// signal entry in the 'init' state, and obtain a sequence number.
	seq := client.MustSignalEntry(ctx, sync.State("init"))

	myUrl := generateMyUrl(netclient, seq)
	myAddress, myKey := generateRandomAddress()

	// Publish my entry
	client.Publish(ctx, peerInfoTopic, &PeerEntry{myAddress, myUrl})

	client.MustSignalEntry(ctx, "readyForPeerInfo")
	peerCount := runenv.TestInstanceCount - 1
	<-client.MustBarrier(ctx, sync.State("readyForPeerInfo"), peerCount).C
	peers := getPeers(ctx, client, peerInfoTopic, runenv)

	transListener := make(chan protocols.ChainTransaction, 10)
	nitroClient, ms, chain := setupClient(seq, myKey, myUrl, peers, transListener)
	runenv.RecordMessage("nitro client created")

	shareTransactions(transListener, runenv, ctx, client, transTopic, chain, myAddress)
	handleTransactions(runenv, ctx, client, transTopic, chain, myAddress)

	defer ms.Close()

	client.MustSignalEntry(ctx, "clientReady")
	<-client.MustBarrier(ctx, sync.State("clientReady"), peerCount).C

	// We can only have one direct channel with a peer, so we only allow one client to create channels
	isChannelCreator := seq == 1

	if isChannelCreator {
		for p := range peers {
			if p != myAddress {
				createLedgerChannel(runenv, myAddress, p, nitroClient)
			}
		}

	}

	// The channel creator will have channels with every peer
	// The other peers will have one channel with the channel creator
	expectedCompleted := 1
	if isChannelCreator {
		expectedCompleted = peerCount
	}

	for i := 0; i < expectedCompleted; i++ {
		// TODO: Make sure the objective ids are correct
		c := <-nitroClient.CompletedObjectives()
		runenv.RecordMessage("objective completed %v", c)
	}

	client.MustSignalEntry(ctx, sync.State("done"))
	<-client.MustBarrier(ctx, sync.State("done"), runenv.TestInstanceCount-1).C
	return nil
}
