package tests

import (
	"context"
	"fmt"
	"math/rand"
	"time"

	"github.com/statechannels/go-nitro-testground/chain"
	"github.com/statechannels/go-nitro-testground/messaging"
	"github.com/statechannels/go-nitro-testground/paymentclient"
	"github.com/statechannels/go-nitro-testground/utils"
	"github.com/statechannels/go-nitro/protocols/virtualfund"
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
	config := utils.RolesConfig{}
	config.AmountHubs = uint(runEnv.IntParam("numOfHubs"))
	config.Payees = uint(runEnv.IntParam("numOfPayees"))
	config.Payers = uint(runEnv.IntParam("numOfPayers"))
	config.PayeePayeers = 0

	if err := config.Validate(uint(runEnv.TestInstanceCount)); err != nil {
		return err
	}
	me := utils.GenerateMe(seq, config, ip.String())

	runEnv.RecordMessage("I am %+v", me)
	// We wait until everyone has chosen an address.
	client.MustSignalAndWait(ctx, "peerInfoGenerated", runEnv.TestInstanceCount)

	// Broadcasts our info and get peer info from all other instances.
	peers := utils.GetPeers(me.PeerInfo, ctx, client, runEnv.TestInstanceCount)

	chainSyncer := chain.NewChainSyncer(me, client, ctx)
	defer chainSyncer.Close()

	pc := paymentclient.NewClient(me, peers, chainSyncer.MockChain(), runEnv.D())
	defer pc.Close()

	runEnv.RecordMessage("payment client created")
	// We wait until everyone has chosen an address.
	client.MustSignalAndWait(ctx, "client created", runEnv.TestInstanceCount)
	pc.ConnectToPeers()
	client.MustSignalAndWait(ctx, "client connected", runEnv.TestInstanceCount)
	testDuration := time.Duration(runEnv.IntParam("paymentTestDuration")) * time.Second
	jobCount := int64(runEnv.IntParam("concurrentPaymentJobs"))
	if me.Role == messaging.Hub {
		client.MustSignalAndWait(ctx, sync.State("ledgerDone"), runEnv.TestInstanceCount)
	} else {
		// Create ledger channels with all the hubs
		pc.CreateLedgerChannels(1_000_000_000_000)
		client.MustSignalAndWait(ctx, sync.State("ledgerDone"), runEnv.TestInstanceCount)
	}

	if me.IsPayer() {

		hubs := utils.FilterByRole(peers, messaging.Hub)
		payees := utils.FilterByRole(peers, messaging.Payee)
		payees = append(payees, utils.FilterByRole(peers, messaging.PayerPayee)...)

		createVirtualPaymentsJob := func() {
			randomHub := utils.SelectRandomPeer(hubs)
			randomPayee := utils.SelectRandomPeer(payees)
			var r virtualfund.ObjectiveResponse

			runDetails := fmt.Sprintf("me=%s,role=%v,hubs=%d,payers=%d,payees=%d,duration=%s,concurrentJobs=%d,jitter=%d,latency=%d",
				me.Address, me.Role, config.AmountHubs, config.Payers, config.Payees, testDuration, jobCount, networkJitterMS, networkLatencyMS)

			var paymentChan *paymentclient.PaymentChannel
			runEnv.D().Timer("time_to_first_payment," + runDetails).Time(func() {
				paymentChan = pc.CreatePaymentChannel(randomHub, randomPayee, 100)
				paymentChan.Pay(1)
			})
			runEnv.RecordMessage("Opened virtual channel %s with %s using hub %s", utils.Abbreviate(r.ChannelId), utils.Abbreviate(randomPayee), utils.Abbreviate(randomHub))
			for i := 0; i < int(rand.Int63n(5))+5; i++ {
				minSleep := time.Duration(50 * time.Millisecond)
				time.Sleep(minSleep + time.Duration(rand.Int63n(int64(time.Millisecond*100))))
				paymentChan.Pay(uint(rand.Int63n(5)))
			}

			runEnv.RecordMessage("Closing %s with payment of %d to %s", utils.Abbreviate(r.ChannelId), paymentChan.Total, utils.Abbreviate(randomPayee))
			paymentChan.Settle()

		}
		utils.RunJob(createVirtualPaymentsJob, testDuration, jobCount)
	}

	client.MustSignalAndWait(ctx, "done", runEnv.TestInstanceCount)

	return nil

}
