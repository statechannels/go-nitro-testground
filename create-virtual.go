package main

import (
	"context"

	"github.com/testground/sdk-go/network"
	"github.com/testground/sdk-go/runtime"
	"github.com/testground/sdk-go/sync"
)

func createVirtualTest(runenv *runtime.RunEnv) error {
	ctx := context.Background()
	// instantiate a sync service client, binding it to the RunEnv.
	client := sync.MustBoundClient(ctx, runenv)
	defer client.Close()

	// instantiate a network client amd wait for it to be ready.
	net := network.NewClient(client, runenv)
	runenv.RecordMessage("waiting for network initialization")
	net.MustWaitNetworkInitialized(ctx)

	// This generates a unqiue sequence number for this test instance.
	// We use seq to determine the role we play and the port for our message service.
	seq := client.MustSignalEntry(ctx, sync.State("init"))
	numOfHubs := int64(runenv.IntParam("numOfHubs"))

	me, myKey := generateMe(seq, net, numOfHubs)

	runenv.RecordMessage("I am %+v", me)
	// We wait until everyone has chosen an address.
	client.MustSignalEntry(ctx, "peerInfoGenerated")
	<-client.MustBarrier(ctx, sync.State("peerInfoGenerated"), runenv.TestInstanceCount).C

	// Broadcasts our info and get peer info from all other instances.
	peers := getPeers(me, ctx, client, runenv.TestInstanceCount)
	// Set up our mock chain that communicates with our instances using a sync.Topic
	chain := setupChain(me, runenv, ctx, client)

	nitroClient, ms := createNitroClient(me, myKey, peers, chain)
	defer ms.Close()
	runenv.RecordMessage("nitro client created")

	// We wait until every instance has successfully created their client
	client.MustSignalEntry(ctx, "clientReady")
	<-client.MustBarrier(ctx, sync.State("clientReady"), runenv.TestInstanceCount).C

	if !me.IsHub {
		// Create ledger channels between me and any hubs.
		createLedgerChannels(me, runenv, nitroClient, filterPeersByHub(peers, true))
	}
	runenv.RecordMessage("All ledger channel objectives completed")

	// We wait until every instance has finished up with ledger channel creation
	client.MustSignalEntry(ctx, sync.State("ledgerDone"))
	<-client.MustBarrier(ctx, sync.State("ledgerDone"), runenv.TestInstanceCount).C

	// If we're not the hub we create numOfChannels with a random peer/hub.
	numOfChannels := runenv.IntParam("numOfChannels")
	cm := NewCompletionMonitor(nitroClient, *runenv)
	if !me.IsHub {
		for i := 0; i < numOfChannels; i++ {

			hubToUse := selectRandomPeer(filterPeersByHub(peers, true))
			peer := selectRandomPeer(filterPeersByHub(peers, false))
			id := createVirtualChannel(runenv, me.Address, hubToUse, peer, nitroClient)
			cm.WatchObjective(id)
		}

	}
	cm.WaitForObjectivesToComplete()
	runenv.RecordMessage("All virtual channel objectives completed")

	client.MustSignalEntry(ctx, sync.State("done"))
	<-client.MustBarrier(ctx, sync.State("done"), runenv.TestInstanceCount).C

	return nil
}
