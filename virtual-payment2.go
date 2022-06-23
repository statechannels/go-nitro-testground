package main

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"math/rand"
	"time"

	"github.com/statechannels/go-nitro/protocols"
	"github.com/statechannels/go-nitro/protocols/virtualfund"
	"github.com/testground/sdk-go/network"
	"github.com/testground/sdk-go/runtime"
	"github.com/testground/sdk-go/sync"
)

// This script describes the "busy provider" topology. 1 hub (seq=0), 1 provider (seq=1), (i-2) clients (seq>1) connecting through the hub to the provider.
// Note i must be greater than 2
// e.g.
// testground run s -p=go-nitro-test-plan -t=virtual-payment2 -b=exec:go -r=local:exec -tp=numOfChannels=4 -i=10
func createVirtualPayment2Test(runEnv *runtime.RunEnv) error {
	runEnv.D().SetFrequency(1 * time.Second)
	ctx := context.Background()
	// instantiate a sync service client, binding it to the RunEnv.
	client := sync.MustBoundClient(ctx, runEnv)
	defer client.Close()

	// instantiate a network client amd wait for it to be ready.
	net := network.NewClient(client, runEnv)

	runEnv.RecordMessage("waiting for network initialization")
	net.MustWaitNetworkInitialized(ctx)
	networkJitterMS, networkLatencyMS := runEnv.IntParam("networkJitter"), runEnv.IntParam("networkLatency")
	if !runEnv.TestSidecar && (networkJitterMS > 0 || networkLatencyMS > 0) {
		err := errors.New("can only apply network jitter/latency when running with docker")
		return err

	} else if runEnv.TestSidecar {

		config := network.Config{
			// Control the "default" network. At the moment, this is the only network.
			Network: "default",
			Enable:  true,

			// Set the traffic shaping characteristics.
			Default: network.LinkShape{
				Latency: time.Duration(networkLatencyMS) * time.Millisecond,
				Jitter:  time.Duration(networkJitterMS) * time.Millisecond,
			},

			// Set what state the sidecar should signal back to you when it's done.
			CallbackState: "network-configured",
		}
		net.MustConfigureNetwork(ctx, &config)

	}

	runEnv.RecordMessage("network configured")
	// This generates a unqiue sequence number for this test instance.
	// We use seq to determine the role we play and the port for our message service.
	seq := client.MustSignalAndWait(ctx, sync.State("network configured"), runEnv.TestInstanceCount)

	numOfHubs := 1 // hardcoded single hub

	ip, err := net.GetDataNetworkIP()
	if err != nil {
		panic(err)
	}

	// set Role
	var role Role
	switch seq {
	case 1:
		role = Hub
	case 2:
		role = Payee
	default:
		role = Payer
	}

	me := generateMe(seq, role, ip.String())

	runEnv.RecordMessage("I am %+v", me)
	// We wait until everyone has chosen an address.
	client.MustSignalAndWait(ctx, "peerInfoGenerated", runEnv.TestInstanceCount)

	// Broadcasts our info and get peer info from all other instances.
	peers := getPeers(me.PeerInfo, ctx, client, runEnv.TestInstanceCount)

	chainSyncer := NewChainSyncer(me, client, ctx)
	defer chainSyncer.Close()

	nitroClient, ms := createNitroClient(me, peers, chainSyncer.MockChain(), runEnv.D())
	defer ms.Close()
	runEnv.RecordMessage("nitro client created")

	// We wait until every instance has successfully created their client
	client.MustSignalAndWait(ctx, "clientReady", runEnv.TestInstanceCount)

	ms.DialPeers()
	client.MustSignalAndWait(ctx, "msDialed", runEnv.TestInstanceCount)

	cm := NewCompletionMonitor(nitroClient, runEnv)
	defer cm.Close()

	if me.isHub() {
		client.MustSignalAndWait(ctx, sync.State("ledgerDone"), runEnv.TestInstanceCount)
	} else {
		hub := filterPeersByHub(peers, true)[0]
		// Create a ledger channel with the hub
		ledgerIds := []protocols.ObjectiveId{}

		r := createLedgerChannel(me.Address, hub.Address, nitroClient)
		runEnv.RecordMessage("Creating ledger channel %s with hub %s", abbreviate(r.ChannelId), abbreviate((hub.Address)))
		ledgerIds = append(ledgerIds, r.Id)

		cm.WaitForObjectivesToComplete(ledgerIds)

		client.MustSignalAndWait(ctx, sync.State("ledgerDone"), runEnv.TestInstanceCount)

		testDuration := time.Duration(runEnv.IntParam("paymentTestDuration")) * time.Second
		jobCount := int64(runEnv.IntParam("concurrentPaymentJobs"))

		if me.isPayer() {
			createVirtualPaymentsJob := func() {
				payee := selectRandomPeer(filterPeersByPayee(peers, false))
				var r virtualfund.ObjectiveResponse

				runDetails := fmt.Sprintf("me=%s,amHub=%v,hubs=%d,clients=%d,duration=%s,concurrentJobs=%d,jitter=%d,latency=%d",
					me.Address, me.isHub(), numOfHubs, runEnv.TestInstanceCount-int(numOfHubs), testDuration, jobCount, networkJitterMS, networkLatencyMS)

				runEnv.D().Timer("time_to_first_payment," + runDetails).Time(func() {
					r = createVirtualChannel(me.Address, hub.Address, payee, nitroClient)
					cm.WaitForObjectivesToComplete([]protocols.ObjectiveId{r.Id})
				})
				runEnv.RecordMessage("Opened virtual channel %s with %s using hub %s", abbreviate(r.ChannelId), abbreviate(payee), abbreviate(hub.Address))
				// We always want to wait a little bit to avoid https://github.com/statechannels/go-nitro/issues/744
				minSleep := 1 * time.Second
				sleepDuration := time.Duration(rand.Int63n(int64(time.Second*1))) + minSleep
				runEnv.RecordMessage("Sleeping %v to simulate payment exchanges for %s", sleepDuration, abbreviate(r.ChannelId))
				time.Sleep(sleepDuration)

				totalPaymentSize := big.NewInt(rand.Int63n(10))
				id := nitroClient.CloseVirtualChannel(r.ChannelId, totalPaymentSize)
				runEnv.RecordMessage("Closing %s with payment of %d to %s", abbreviate(r.ChannelId), totalPaymentSize, abbreviate(payee))
				cm.WaitForObjectivesToComplete([]protocols.ObjectiveId{id})

			}

			RunJob(createVirtualPaymentsJob, testDuration, jobCount)
		}

	}

	client.MustSignalAndWait(ctx, "done", runEnv.TestInstanceCount)

	return nil

}
