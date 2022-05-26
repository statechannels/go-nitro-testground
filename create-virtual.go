package main

import (
	"context"

	"github.com/testground/sdk-go/network"
	"github.com/testground/sdk-go/runtime"
	"github.com/testground/sdk-go/sync"
)

func createVirtualTest(runEnv *runtime.RunEnv) error {
	ctx := context.Background()
	// instantiate a sync service client, binding it to the RunEnv.
	client := sync.MustBoundClient(ctx, runEnv)
	defer client.Close()

	// instantiate a network client amd wait for it to be ready.
	net := network.NewClient(client, runEnv)
	runEnv.RecordMessage("waiting for network initialization")
	net.MustWaitNetworkInitialized(ctx)

	// This generates a unqiue sequence number for this test instance.
	// We use seq to determine the role we play and the port for our message service.
	seq := client.MustSignalEntry(ctx, sync.State("init"))
	numOfHubs := int64(runEnv.IntParam("numOfHubs"))

	me := generateMe(seq, seq <= numOfHubs)

	runEnv.RecordMessage("I am %+v", me)
	// We wait until everyone has chosen an address.
	client.MustSignalEntry(ctx, "peerInfoGenerated")
	<-client.MustBarrier(ctx, sync.State("peerInfoGenerated"), runEnv.TestInstanceCount).C

	// Broadcasts our info and get peer info from all other instances.
	peers := getPeers(me.PeerInfo, ctx, client, runEnv.TestInstanceCount)
	// Set up our mock chain that communicates with our instances using a sync.Topic
	chain := setupChain(me.PeerInfo, ctx, client)

	nitroClient, ms := createNitroClient(me, peers, chain, runEnv.D())
	defer ms.Close()
	runEnv.RecordMessage("nitro client created")

	// We wait until every instance has successfully created their client
	client.MustSignalEntry(ctx, "clientReady")
	<-client.MustBarrier(ctx, sync.State("clientReady"), runEnv.TestInstanceCount).C

	ms.DialPeers()
	client.MustSignalEntry(ctx, "msDialed")
	<-client.MustBarrier(ctx, sync.State("msDialed"), runEnv.TestInstanceCount).C

	if !me.IsHub {
		// Create ledger channels between me and any hubs.
		createLedgerChannels(me.PeerInfo, runEnv, nitroClient, filterPeersByHub(peers, true))
	}
	runEnv.RecordMessage("All ledger channel objectives completed")

	// We wait until every instance has finished up with ledger channel creation
	client.MustSignalEntry(ctx, sync.State("ledgerDone"))
	<-client.MustBarrier(ctx, sync.State("ledgerDone"), runEnv.TestInstanceCount).C

	// If we're not the hub we create numOfChannels with a random peer/hub.
	numOfChannels := runEnv.IntParam("numOfChannels")
	cm := NewCompletionMonitor(nitroClient, *runEnv)
	if !me.IsHub {
		for i := 0; i < numOfChannels; i++ {

			hubToUse := selectRandomPeer(filterPeersByHub(peers, true))
			peer := selectRandomPeer(filterPeersByHub(peers, false))
			id := createVirtualChannel(me.Address, hubToUse, peer, nitroClient)
			cm.WatchObjective(id)
		}

	}
	cm.WaitForObjectivesToComplete()
	runEnv.RecordMessage("All virtual channel objectives completed")

	client.MustSignalEntry(ctx, sync.State("done"))
	<-client.MustBarrier(ctx, sync.State("done"), runEnv.TestInstanceCount).C

	return nil
}
