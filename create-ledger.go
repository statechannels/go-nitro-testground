package main

import (
	"context"

	"github.com/statechannels/go-nitro/protocols"
	"github.com/testground/sdk-go/network"
	"github.com/testground/sdk-go/runtime"
	"github.com/testground/sdk-go/sync"
)

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
	client.Publish(ctx, peerInfoTopic, &PeerEntry{myAddress, myUrl, false})

	client.MustSignalEntry(ctx, "readyForPeerInfo")
	<-client.MustBarrier(ctx, sync.State("readyForPeerInfo"), runenv.TestInstanceCount).C

	peers := getPeers(ctx, client, peerInfoTopic, runenv)

	transListener := make(chan protocols.ChainTransaction, 10)
	nitroClient, ms, chain := setupClient(seq, myKey, myUrl, peers, transListener)
	runenv.RecordMessage("nitro client created")

	shareTransactions(transListener, runenv, ctx, client, transTopic, chain, myAddress)
	handleTransactions(runenv, ctx, client, transTopic, chain, myAddress)

	defer ms.Close()

	client.MustSignalEntry(ctx, "clientReady")
	<-client.MustBarrier(ctx, sync.State("clientReady"), runenv.TestInstanceCount).C

	// We can only have one direct channel with a peer, so we only allow one client to create channels
	isChannelCreator := seq == 1
	cm := NewCompletionMonitor(nitroClient, *runenv)
	if isChannelCreator {
		for p := range peers {

			id := createLedgerChannel(runenv, myAddress, p, nitroClient)
			cm.WatchObjective(id)

		}

	}

	cm.WaitForObjectivesToComplete()

	client.MustSignalEntry(ctx, sync.State("done"))
	<-client.MustBarrier(ctx, sync.State("done"), runenv.TestInstanceCount).C
	return nil
}
