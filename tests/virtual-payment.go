package tests

import (
	"context"
	"fmt"
	"math/rand"
	"time"

	"github.com/statechannels/go-nitro-testground/chain"
	c "github.com/statechannels/go-nitro-testground/config"
	"github.com/statechannels/go-nitro-testground/paymentclient"
	"github.com/statechannels/go-nitro-testground/peer"
	"github.com/statechannels/go-nitro-testground/utils"
	"github.com/testground/sdk-go/runtime"
	"github.com/testground/sdk-go/sync"
)

func CreateVirtualPaymentTest(runEnv *runtime.RunEnv) error {
	runEnv.D().SetFrequency(1 * time.Second)
	ctx := context.Background()
	// instantiate a sync service client, binding it to the RunEnv.
	client := sync.MustBoundClient(ctx, runEnv)
	defer client.Close()
	networkJitterMS, networkLatencyMS := runEnv.IntParam("networkJitter"), runEnv.IntParam("networkLatency")
	// instantiate a network client amd wait for it to be ready.
	net, err := utils.ConfigureNetworkClient(ctx, client, runEnv, networkJitterMS, networkLatencyMS)
	if err != nil {
		panic(err)
	}

	// This generates a unique sequence number for this test instance.
	// We use seq to determine the role we play and the port for our message service.
	seq := client.MustSignalAndWait(ctx, sync.State("get seq"), runEnv.TestInstanceCount)

	ip, err := net.GetDataNetworkIP()
	if err != nil {
		panic(err)
	}
	config, err := c.GetRunConfig(runEnv)
	if err != nil {
		panic(err)
	}
	me := peer.GenerateMe(seq, config, ip.String())

	runEnv.RecordMessage("I am %+v", me)
	// We wait until everyone has chosen an address.
	client.MustSignalAndWait(ctx, "peerInfoGenerated", runEnv.TestInstanceCount)

	// Broadcasts our info and get peer info from all other instances.
	peers := utils.SharePeerInfo(me.PeerInfo, ctx, client, runEnv.TestInstanceCount)

	chainSyncer := chain.NewChainSyncer(me, client, ctx)
	defer chainSyncer.Close()

	pc := paymentclient.NewClient(me, peers, chainSyncer.ChainService(), runEnv.D())
	defer pc.Close()

	runEnv.RecordMessage("payment client created")
	// We wait until everyone has chosen an address.
	client.MustSignalAndWait(ctx, "client created", runEnv.TestInstanceCount)
	pc.ConnectToPeers()
	client.MustSignalAndWait(ctx, "client connected", runEnv.TestInstanceCount)

	if me.Role == peer.Hub {
		client.MustSignalAndWait(ctx, sync.State("ledgerDone"), runEnv.TestInstanceCount)
	} else {
		// Create ledger channels with all the hubs
		pc.CreateLedgerChannels(1_000_000_000_000)
		client.MustSignalAndWait(ctx, sync.State("ledgerDone"), runEnv.TestInstanceCount)
	}

	if me.IsPayer() {

		hubs := peer.FilterByRole(peers, peer.Hub)
		payees := peer.FilterByRole(peers, peer.Payee)
		payees = append(payees, peer.FilterByRole(peers, peer.PayerPayee)...)

		createVirtualPaymentsJob := func() {
			randomHub := utils.SelectRandom(hubs)
			randomPayee := utils.SelectRandom(payees)

			runDetails := fmt.Sprintf("me=%s,role=%v,hubs=%d,payers=%d,payees=%d,payeePayers=%d,duration=%s,concurrentJobs=%d,jitter=%d,latency=%d",
				me.Address, me.Role, config.NumHubs, config.NumPayers, config.NumPayees, config.NumPayeePayers, config.PaymentTestDuration, config.ConcurrentPaymentJobs, config.NetworkJitter.Milliseconds(), config.NetworkLatency.Milliseconds())

			var paymentChan *paymentclient.PaymentChannel
			runEnv.D().Timer("time_to_first_payment," + runDetails).Time(func() {
				paymentChan = pc.CreatePaymentChannel(randomHub.Address, randomPayee.Address, 100)
				paymentChan.Pay(1)
			})

			runEnv.RecordMessage("Opened virtual channel %s with %s using hub %s", utils.Abbreviate(paymentChan.Id()), utils.Abbreviate(randomPayee.Address), utils.Abbreviate(randomHub.Address))

			for i := 0; i < int(rand.Int63n(5))+5; i++ {
				minSleep := time.Duration(50 * time.Millisecond)
				time.Sleep(minSleep + time.Duration(rand.Int63n(int64(time.Millisecond*100))))
				paymentChan.Pay(uint(rand.Int63n(5)))
			}

			runEnv.RecordMessage("Closing %s with payment of %d to %s", utils.Abbreviate(paymentChan.Id()), paymentChan.Total, utils.Abbreviate(randomPayee.Address))
			paymentChan.Settle()

		}

		// Run the job(s)
		utils.RunJobs(createVirtualPaymentsJob, config.PaymentTestDuration, int64(config.ConcurrentPaymentJobs))
	}

	client.MustSignalAndWait(ctx, "done", runEnv.TestInstanceCount)

	return nil

}
