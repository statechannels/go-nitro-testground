package main

import (
	"context"
	"time"

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

	// signal entry in the 'init' state, and obtain a sequence number.
	seq := client.MustSignalEntry(ctx, sync.State("init"))
	numOfHubs := int64(runenv.IntParam("numOfHubs"))

	me, myKey := generateMe(seq, netclient, numOfHubs)
	runenv.RecordMessage("I am %+v", me)
	// We wait until everyone has chosen an address and broadcasted it.
	client.MustSignalEntry(ctx, "readyForPeerInfo")
	<-client.MustBarrier(ctx, sync.State("readyForPeerInfo"), runenv.TestInstanceCount).C

	// Read all our peers from the sync.Topic
	peers := getPeers(me, ctx, client, runenv)

	chain := setupChain(me, runenv, ctx, client)

	nitroClient, ms := createNitroClient(me, myKey, peers, chain)
	runenv.RecordMessage("nitro client created")

	client.MustSignalEntry(ctx, "clientReady")
	<-client.MustBarrier(ctx, sync.State("clientReady"), runenv.TestInstanceCount).C

	if !me.IsHub {
		ledgerCm := NewCompletionMonitor(nitroClient, *runenv)
		for _, p := range peers {
			if p.Address != me.Address && p.IsHub {
				id := createLedgerChannel(runenv, me.Address, p.Address, nitroClient)

				ledgerCm.WatchObjective(id)

			}
		}
		ledgerCm.WaitForObjectivesToComplete()
	}
	client.MustSignalEntry(ctx, sync.State("ledgerDone"))
	<-client.MustBarrier(ctx, sync.State("ledgerDone"), runenv.TestInstanceCount).C
	runenv.RecordMessage("All ledger channel objectives completed")
	cm := NewCompletionMonitor(nitroClient, *runenv)
	if !me.IsHub {
		numOfChannels := runenv.IntParam("numOfChannels")

		for i := 0; i < numOfChannels; i++ {

			hubToUse := selectRandomPeer(peers, me.Address, true)
			peer := selectRandomPeer(peers, me.Address, false)
			id := createVirtualChannel(runenv, me.Address, hubToUse, peer, nitroClient)
			cm.WatchObjective(id)
		}

	}
	cm.WaitForObjectivesToComplete()
	runenv.RecordMessage("All virtual channel objectives completed")
	client.MustSignalEntry(ctx, sync.State("done"))
	<-client.MustBarrier(ctx, sync.State("done"), runenv.TestInstanceCount).C

	// TODO: We sleep a second to make sure messages are flushed
	// There's probably a more elegant solution
	time.Sleep(time.Second)
	ms.Close()

	return nil
}
