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
	run.Invoke(twoPeerTest)
}

func twoPeerTest(runenv *runtime.RunEnv) error {
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
	<-client.MustBarrier(ctx, sync.State("readyForPeerInfo"), runenv.TestInstanceCount-1).C
	peers := getPeers(ctx, client, peerInfoTopic, runenv)

	transListener := make(chan protocols.ChainTransaction, 10)
	nitroClient, ms, chain := setupClient(seq, myKey, myUrl, peers, transListener)
	runenv.RecordMessage("nitro client created")

	shareTransactions(transListener, runenv, ctx, client, transTopic, chain, myAddress)
	handleTransactions(runenv, ctx, client, transTopic, chain, myAddress)

	defer ms.Close()

	client.MustSignalEntry(ctx, "clientReady")
	<-client.MustBarrier(ctx, sync.State("clientReady"), runenv.TestInstanceCount-1).C

	// We only want one participant to create the channel for now
	if seq == 1 {
		counterparty := selectAPeer(peers, myAddress)
		createDirectChannel(runenv, myAddress, counterparty, nitroClient)

	}

	// TODO: Make sure the objective ids are correct
	comp := <-nitroClient.CompletedObjectives()
	runenv.RecordMessage("Completed objective %s", comp)

	client.MustSignalEntry(ctx, sync.State("done"))
	<-client.MustBarrier(ctx, sync.State("done"), runenv.TestInstanceCount-1).C
	return nil
}
