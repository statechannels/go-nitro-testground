package main

import (
	"context"

	nitroclient "github.com/statechannels/go-nitro/client"
	"github.com/statechannels/go-nitro/types"
	"github.com/testground/sdk-go/network"
	"github.com/testground/sdk-go/runtime"
	"github.com/testground/sdk-go/sync"
)

func createLedgerTest(runEnv *runtime.RunEnv) error {
	ctx := context.Background()
	// instantiate a sync service client, binding it to the RunEnv.
	client := sync.MustBoundClient(ctx, runEnv)
	defer client.Close()

	// instantiate a network client; see 'Traffic shaping' in the docs.
	netclient := network.NewClient(client, runEnv)
	runEnv.RecordMessage("waiting for network initialization")

	// wait for the network to initialize; this should be pretty fast.
	netclient.MustWaitNetworkInitialized(ctx)

	// signal entry in the 'init' state, and obtain a sequence number.
	seq := client.MustSignalEntry(ctx, sync.State("init"))

	ip, err := netclient.GetDataNetworkIP()
	if err != nil {
		panic(err)
	}
	me := generateMe(seq, false, ip.String())
	runEnv.RecordMessage("I am %+v", me)

	client.MustSignalEntry(ctx, "readyForPeerInfo")
	<-client.MustBarrier(ctx, sync.State("readyForPeerInfo"), runEnv.TestInstanceCount).C

	peers := getPeers(me.PeerInfo, ctx, client, runEnv.TestInstanceCount)

	chain := setupChain(me.PeerInfo, ctx, client)

	nitroClient, ms := createNitroClient(me, peers, chain, runEnv.D())
	runEnv.RecordMessage("nitro client created")

	defer ms.Close()

	client.MustSignalEntry(ctx, "clientReady")
	<-client.MustBarrier(ctx, sync.State("clientReady"), runEnv.TestInstanceCount).C
	ms.DialPeers()
	client.MustSignalEntry(ctx, "msDialed")
	<-client.MustBarrier(ctx, sync.State("msDialed"), runEnv.TestInstanceCount).C
	// We can only have one direct channel with a peer, so we only allow one client to create channels
	isChannelCreator := seq == 1
	if isChannelCreator {
		createLedgerChannels(me.PeerInfo, runEnv, nitroClient, filterPeersByHub(peers, false))
	}

	client.MustSignalEntry(ctx, sync.State("done"))
	<-client.MustBarrier(ctx, sync.State("done"), runEnv.TestInstanceCount).C
	return nil
}

// createLedgerChannels creates a ledger channel between me and every peer in filtered peers
func createLedgerChannels(me PeerInfo, runenv *runtime.RunEnv, nc *nitroclient.Client, filteredPeers []PeerInfo) []types.Destination {

	cm := NewCompletionMonitor(nc, *runenv)
	ledgerIds := []types.Destination{}
	for _, p := range filteredPeers {

		r := createLedgerChannel(me.Address, p.Address, nc)
		cm.WatchObjective(r.Id)
		ledgerIds = append(ledgerIds, r.ChannelId)

	}

	cm.WaitForObjectivesToComplete()

	return ledgerIds
}
