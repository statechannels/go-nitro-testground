package main

import (
	"context"

	"github.com/statechannels/go-nitro/protocols"
	"github.com/testground/sdk-go/network"
	"github.com/testground/sdk-go/runtime"
	"github.com/testground/sdk-go/sync"
)

func createVirtualTest(runenv *runtime.RunEnv) error {
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

	numOfHubs := int64(runenv.IntParam("numOfHubs"))
	isHub := seq <= numOfHubs

	// Publish my entry
	client.Publish(ctx, peerInfoTopic, &PeerEntry{myAddress, myUrl, isHub})

	client.MustSignalEntry(ctx, "readyForPeerInfo")
	<-client.MustBarrier(ctx, sync.State("readyForPeerInfo"), runenv.TestInstanceCount).C

	peers := getPeers(ctx, client, peerInfoTopic, runenv)
	runenv.RecordMessage("I am %+v", peers[myAddress])

	transListener := make(chan protocols.ChainTransaction, 10)
	nitroClient, ms, chain := setupClient(seq, myKey, myUrl, peers, transListener)
	runenv.RecordMessage("nitro client created")

	shareTransactions(transListener, runenv, ctx, client, transTopic, chain, myAddress)
	handleTransactions(runenv, ctx, client, transTopic, chain, myAddress)

	defer ms.Close()

	client.MustSignalEntry(ctx, "clientReady")
	<-client.MustBarrier(ctx, sync.State("clientReady"), runenv.TestInstanceCount).C

	if !isHub {
		for _, p := range peers {
			if p.Address != myAddress && p.IsHub {
				createLedgerChannel(runenv, myAddress, p.Address, nitroClient)
				runenv.RecordMessage("created channel with hub")
			}
		}

	}

	// The channel creator will have channels with every peer
	// The other peers will have one channel with the channel creator
	expectedCompleted := 1
	if isHub {
		expectedCompleted = runenv.TestInstanceCount - int(numOfHubs)
	}

	for i := 0; i < expectedCompleted; i++ {
		// TODO: Make sure the objective ids are correct
		c := <-nitroClient.CompletedObjectives()
		runenv.RecordMessage("ledger objective completed %v", c)
	}

	client.MustSignalEntry(ctx, sync.State("ledgerDone"))
	<-client.MustBarrier(ctx, sync.State("ledgerDone"), runenv.TestInstanceCount).C

	numOfChannels := runenv.IntParam("numOfChannels")
	if !isHub {
		for i := 0; i < numOfChannels; i++ {
			hubToUse := selectRandomPeer(peers, myAddress, true)
			peer := selectRandomPeer(peers, myAddress, false)
			createVirtualChannel(runenv, myAddress, hubToUse, peer, nitroClient)
		}

		for i := 0; i < numOfChannels; i++ {
			// TODO: Make sure the objective ids are correct
			c := <-nitroClient.CompletedObjectives()
			runenv.RecordMessage("virtual objective completed %v", c)
		}
	}

	client.MustSignalEntry(ctx, sync.State("done"))
	<-client.MustBarrier(ctx, sync.State("done"), runenv.TestInstanceCount).C

	return nil
}