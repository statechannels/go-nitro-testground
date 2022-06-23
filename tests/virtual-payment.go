package tests

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"math/rand"
	"time"

	"github.com/statechannels/go-nitro-testground/chain"
	"github.com/statechannels/go-nitro-testground/utils"
	"github.com/statechannels/go-nitro-testground/utils/monitor"

	"github.com/statechannels/go-nitro/protocols"
	"github.com/statechannels/go-nitro/protocols/virtualfund"
	"github.com/testground/sdk-go/network"
	"github.com/testground/sdk-go/runtime"
	"github.com/testground/sdk-go/sync"
)

func CreateVirtualPaymentTest(runEnv *runtime.RunEnv) error {
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

	numOfHubs := int64(runEnv.IntParam("numOfHubs"))

	ip, err := net.GetDataNetworkIP()
	if err != nil {
		panic(err)
	}
	me := utils.GenerateMe(seq, seq <= numOfHubs, ip.String())

	runEnv.RecordMessage("I am %+v", me)
	// We wait until everyone has chosen an address.
	client.MustSignalAndWait(ctx, "peerInfoGenerated", runEnv.TestInstanceCount)

	// Broadcasts our info and get peer info from all other instances.
	peers := utils.GetPeers(me.PeerInfo, ctx, client, runEnv.TestInstanceCount)

	chainSyncer := chain.NewChainSyncer(me, client, ctx)
	defer chainSyncer.Close()

	nitroClient, ms := utils.CreateNitroClient(me, peers, chainSyncer.MockChain(), runEnv.D())
	defer ms.Close()
	runEnv.RecordMessage("nitro client created")

	// We wait until every instance has successfully created their client
	client.MustSignalAndWait(ctx, "clientReady", runEnv.TestInstanceCount)

	ms.DialPeers()
	client.MustSignalAndWait(ctx, "msDialed", runEnv.TestInstanceCount)

	cm := monitor.NewCompletionMonitor(nitroClient, runEnv)
	defer cm.Close()

	if me.IsHub {
		client.MustSignalAndWait(ctx, sync.State("ledgerDone"), runEnv.TestInstanceCount)
	} else {

		// Create ledger channels with all the hubs
		ledgerIds := []protocols.ObjectiveId{}
		hubs := utils.FilterPeersByHub(peers, true)
		for _, h := range hubs {
			r := utils.CreateLedgerChannel(me.Address, h.Address, nitroClient)
			runEnv.RecordMessage("Creating ledger channel %s with hub %s", utils.Abbreviate(r.ChannelId), utils.Abbreviate((h.Address)))
			ledgerIds = append(ledgerIds, r.Id)
		}
		cm.WaitForObjectivesToComplete(ledgerIds)

		client.MustSignalAndWait(ctx, sync.State("ledgerDone"), runEnv.TestInstanceCount)

		testDuration := time.Duration(runEnv.IntParam("paymentTestDuration")) * time.Second
		jobCount := int64(runEnv.IntParam("concurrentPaymentJobs"))

		createVirtualPaymentsJob := func() {
			randomHub := utils.SelectRandomPeer(utils.FilterPeersByHub(peers, true))
			randomPayee := utils.SelectRandomPeer(utils.FilterPeersByHub(peers, false))
			var r virtualfund.ObjectiveResponse

			runDetails := fmt.Sprintf("me=%s,amHub=%v,hubs=%d,clients=%d,duration=%s,concurrentJobs=%d,jitter=%d,latency=%d",
				me.Address, me.IsHub, numOfHubs, runEnv.TestInstanceCount-int(numOfHubs), testDuration, jobCount, networkJitterMS, networkLatencyMS)

			runEnv.D().Timer("time_to_first_payment," + runDetails).Time(func() {
				r = utils.CreateVirtualChannel(me.Address, randomHub, randomPayee, nitroClient)
				cm.WaitForObjectivesToComplete([]protocols.ObjectiveId{r.Id})
			})
			runEnv.RecordMessage("Opened virtual channel %s with %s using hub %s", utils.Abbreviate(r.ChannelId), utils.Abbreviate(randomPayee), utils.Abbreviate(randomHub))
			// We always want to wait a little bit to avoid https://github.com/statechannels/go-nitro/issues/744
			minSleep := 1 * time.Second
			sleepDuration := time.Duration(rand.Int63n(int64(time.Second*1))) + minSleep
			runEnv.RecordMessage("Sleeping %v to simulate payment exchanges for %s", sleepDuration, utils.Abbreviate(r.ChannelId))
			time.Sleep(sleepDuration)

			totalPaymentSize := big.NewInt(rand.Int63n(10))
			id := nitroClient.CloseVirtualChannel(r.ChannelId, totalPaymentSize)
			runEnv.RecordMessage("Closing %s with payment of %d to %s", utils.Abbreviate(r.ChannelId), totalPaymentSize, utils.Abbreviate(randomPayee))
			cm.WaitForObjectivesToComplete([]protocols.ObjectiveId{id})

		}

		utils.RunJob(createVirtualPaymentsJob, testDuration, jobCount)

	}

	client.MustSignalAndWait(ctx, "done", runEnv.TestInstanceCount)

	return nil

}
