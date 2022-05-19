package main

import (
	"context"
	"time"

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

	// We wait until everyone has chosen an address and broadcasted it.
	client.MustSignalEntry(ctx, "readyForPeerInfo")
	<-client.MustBarrier(ctx, sync.State("readyForPeerInfo"), runenv.TestInstanceCount).C

	// Read all our peers from the sync.Topic
	peers := getPeers(ctx, client, peerInfoTopic, runenv)

	runenv.RecordMessage("I am %+v", peers[myAddress])

	transListener := make(chan protocols.ChainTransaction, 1000)
	// Create the nitro client with our key
	nitroClient, ms, chain := setupClient(seq, myKey, myUrl, peers, transListener)
	runenv.RecordMessage("nitro client created")

	shareTransactions(transListener, runenv, ctx, client, transTopic, chain, myAddress)
	handleTransactions(runenv, ctx, client, transTopic, chain, myAddress)

	client.MustSignalEntry(ctx, "clientReady")
	<-client.MustBarrier(ctx, sync.State("clientReady"), runenv.TestInstanceCount).C

	if !isHub {
		ledgerCm := NewCompletionMonitor(nitroClient, *runenv)
		for _, p := range peers {
			if p.Address != myAddress && p.IsHub {
				id := createLedgerChannel(runenv, myAddress, p.Address, nitroClient)

				ledgerCm.Add(id)

				runenv.RecordMessage("created channel with hub")
			}
		}
		if !ledgerCm.AllDone() {
			ledgerCm.Wait()
		}

		ledgerCm.Stop()
	}

	client.MustSignalEntry(ctx, sync.State("ledgerDone"))
	<-client.MustBarrier(ctx, sync.State("ledgerDone"), runenv.TestInstanceCount).C

	cm := NewCompletionMonitor(nitroClient, *runenv)
	if !isHub {
		numOfChannels := runenv.IntParam("numOfChannels")

		for i := 0; i < numOfChannels; i++ {

			hubToUse := selectRandomPeer(peers, myAddress, true)
			peer := selectRandomPeer(peers, myAddress, false)
			id := createVirtualChannel(runenv, myAddress, hubToUse, peer, nitroClient)
			cm.Add(id)
		}

	}
	if !cm.AllDone() {
		cm.Wait()
	}
	cm.Stop()
	client.MustSignalEntry(ctx, sync.State("done"))
	<-client.MustBarrier(ctx, sync.State("done"), runenv.TestInstanceCount).C
	// TODO: We sleep a second to make sure messages are flushed
	// There's probably a more elegant solution
	time.Sleep(time.Second)
	ms.Close()

	return nil
}
