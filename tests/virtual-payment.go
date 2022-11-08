package tests

import (
	"context"
	"fmt"
	"math/big"
	"math/rand"
	"os"
	"time"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/statechannels/go-nitro-testground/chain"
	c "github.com/statechannels/go-nitro-testground/config"
	"github.com/statechannels/go-nitro-testground/peer"
	"github.com/statechannels/go-nitro-testground/utils"
	"github.com/statechannels/go-nitro/channel/state/outcome"
	nitro "github.com/statechannels/go-nitro/client"
	"github.com/statechannels/go-nitro/client/engine"
	p2pms "github.com/statechannels/go-nitro/client/engine/messageservice/p2p-message-service"
	"github.com/statechannels/go-nitro/client/engine/store"
	"github.com/statechannels/go-nitro/protocols"
	"github.com/statechannels/go-nitro/types"
	"github.com/testground/sdk-go/run"
	"github.com/testground/sdk-go/runtime"
	"github.com/testground/sdk-go/sync"
)

// START_PORT is the start of the port range we'll use to issue unique ports.
const START_PORT = 49000

func CreateVirtualPaymentTest(runEnv *runtime.RunEnv, init *run.InitContext) error {

	ctx := context.Background()

	client := init.SyncClient
	net := init.NetClient

	networkJitterMS, networkLatencyMS := runEnv.IntParam("networkJitter"), runEnv.IntParam("networkLatency")
	// instantiate a network client amd wait for it to be ready.
	err := utils.ConfigureNetworkClient(ctx, net, client, runEnv, networkJitterMS, networkLatencyMS)
	if err != nil {
		panic(err)
	}

	seq := init.GlobalSeq
	ip := net.MustGetDataNetworkIP()

	runConfig, err := c.GetRunConfig(runEnv)
	if err != nil {
		panic(err)
	}
	// Test
	role := peer.GetRole(seq, runConfig)
	// We use the sequence in the random source so we generate a unique key even if another client is running at the same time
	privateKey, err := crypto.GenerateKey()
	if err != nil {
		panic(err)
	}

	pk := crypto.FromECDSA(privateKey)
	address := crypto.PubkeyToAddress(privateKey.PublicKey)
	port := (START_PORT) + int(seq)
	ipAddress := ip.String()

	// Create the ms using the given key
	ms := p2pms.NewMessageService(ipAddress, port, pk)
	client.MustSignalAndWait(ctx, "msStarted", runEnv.TestInstanceCount)

	mePeerInfo := peer.PeerInfo{PeerInfo: p2pms.PeerInfo{Address: address, IpAddress: ipAddress, Port: port, Id: ms.Id()}, Role: role, Seq: seq}
	me := peer.MyInfo{PeerInfo: mePeerInfo, PrivateKey: *privateKey}

	runEnv.RecordMessage("I am address:%s role:%d seq:%d", me.Address, me.Role, me.Seq)

	utils.RecordRunInfo(me, runConfig, runEnv.R())

	// Broadcasts our info and get peer info from all other instances.
	peers := utils.SharePeerInfo(me.PeerInfo, ctx, client, runEnv.TestInstanceCount)

	// Register our peers with the message service
	ms.AddPeers(peer.GetMessageServicePeers(peers))
	client.MustSignalAndWait(ctx, "peersAdded", runEnv.TestInstanceCount)

	store := store.NewMemStore(crypto.FromECDSA(&me.PrivateKey))

	// We skip the 0x prefix by slicing from index 2
	shortAddress := me.Address.String()[2:8]
	logPath := fmt.Sprintf("./outputs/nitro-client-%s-role-%d.log", shortAddress, me.Role)
	// The outputs folder will be copied when results are collected.
	logDestination, _ := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY, 0666)

	// All instances wait until the NitroAdjudicator has been deployed (seq = 1 instance is responsible)
	cs := chain.NewChainService(ctx, seq, logDestination)
	contractSetup := sync.State("contractSetup")
	client.MustSignalEntry(ctx, contractSetup)
	client.MustBarrier(ctx, contractSetup, runEnv.TestInstanceCount)

	nClient := nitro.New(ms, cs, store, logDestination, &engine.PermissivePolicy{}, runEnv.R())

	cm := utils.NewCompletionMonitor(&nClient, runEnv.RecordMessage)
	defer cm.Close()

	// We wait until everyone has chosen an address.
	client.MustSignalAndWait(ctx, "client created", runEnv.TestInstanceCount)

	client.MustSignalAndWait(ctx, "message service connected", runEnv.TestInstanceCount)

	// Create ledger channels with all the hubs
	ledgerIds := utils.CreateLedgerChannels(nClient, cm, utils.FINNEY_IN_WEI, me.PeerInfo, peers)

	client.MustSignalAndWait(ctx, sync.State("ledgerDone"), runEnv.TestInstanceCount)
	toSleep := runConfig.GetSleepDuration()
	runEnv.RecordMessage("Waiting %s before starting payments", toSleep)
	time.Sleep(toSleep)

	if me.IsPayer() {

		hubs := peer.FilterByRole(peers, peer.Hub)
		payees := peer.FilterByRole(peers, peer.Payee)
		payees = append(payees, peer.FilterByRole(peers, peer.PayerPayee)...)

		createVirtualPaymentsJob := func() {
			selectedHubs := utils.SelectRandomHubs(hubs, int(runConfig.NumIntermediaries))
			randomPayee := utils.SelectRandom(payees)

			var channelId types.Destination
			ttfpStart := time.Now()
			outcome := outcome.Exit{outcome.SingleAssetExit{
				Allocations: outcome.Allocations{
					outcome.Allocation{
						Destination: types.AddressToDestination(me.Address),
						Amount:      big.NewInt(int64(10 * utils.GWEI_IN_WEI)),
					},
					outcome.Allocation{
						Destination: types.AddressToDestination(randomPayee.Address),
						Amount:      big.NewInt(0),
					},
				},
			}}

			r := nClient.CreateVirtualPaymentChannel(selectedHubs, randomPayee.Address, 0, outcome)

			channelId = r.ChannelId
			cm.WaitForObjectivesToComplete([]protocols.ObjectiveId{r.Id})

			runEnv.RecordMessage("Opened virtual channel %s with %s using hubs %s", utils.Abbreviate(channelId), utils.Abbreviate(randomPayee.Address), utils.AbbreviateSlice(selectedHubs))

			paymentAmount := big.NewInt(utils.KWEI_IN_WEI)
			nClient.Pay(r.ChannelId, paymentAmount)
			runEnv.RecordMessage("Sent payment of %d  wei to %s using channel %s", paymentAmount.Int64(), utils.Abbreviate(randomPayee.Address), utils.Abbreviate(channelId))

			// TODO: Should we wait for receipt of this payment before stopping the time_to_first_payment timer?
			runEnv.R().Timer(fmt.Sprintf("time_to_first_payment,me=%s", me.Address)).UpdateSince(ttfpStart)
			// Perform between 1 and 5 payments additional payments
			amountOfPayments := 1 + rand.Intn(4)
			for i := 0; i < amountOfPayments; i++ {
				// pay between 1 and 2 kwei
				paymentAmount := big.NewInt(utils.KWEI_IN_WEI + (rand.Int63n(utils.KWEI_IN_WEI)))
				nClient.Pay(channelId, paymentAmount)

				runEnv.RecordMessage("Sent payment of %d wei to %s using channel %s", paymentAmount.Int64(), utils.Abbreviate(randomPayee.Address), utils.Abbreviate(channelId))

			}

			// TODO: If we attempt to close a virtual channel too fast we can cause other clients to fail.
			// See https://github.com/statechannels/go-nitro/issues/744
			time.Sleep(time.Duration(250 * time.Millisecond))

			// TODO: get payment balance and output it to the log
			runEnv.RecordMessage("Closing %s with payment to %s", utils.Abbreviate(channelId), utils.Abbreviate(randomPayee.Address))
			closeId := nClient.CloseVirtualChannel(channelId)
			cm.WaitForObjectivesToComplete([]protocols.ObjectiveId{closeId})

		}

		// Run the job(s)
		utils.RunJobs(createVirtualPaymentsJob, runConfig.PaymentTestDuration, int64(runConfig.ConcurrentPaymentJobs))
		runEnv.R().Timer(fmt.Sprintf("time_to_first_payment,me=%s", me.Address)).Stop()
		toSleep := runConfig.GetSleepDuration()
		runEnv.RecordMessage("Waiting %s before closing ledger channels", toSleep)
		time.Sleep(toSleep)
	}
	client.MustSignalAndWait(ctx, "paymentsDone", runEnv.TestInstanceCount)

	// TODO: Closing as a hub seems to fail: https://github.com/statechannels/go-nitro-testground/issues/134
	// For now we avoid closing as a hub
	if len(ledgerIds) > 0 && me.Role != peer.Hub {
		// Close all the ledger channels with the hub
		oIds := []protocols.ObjectiveId{}
		for _, ledgerId := range ledgerIds {

			oId := nClient.CloseLedgerChannel(ledgerId)
			oIds = append(oIds, oId)
		}
		cm.WaitForObjectivesToComplete(oIds)
		runEnv.RecordMessage("All ledger channels closed")
	}

	// Record the mean time to first payment to nightly/ci metrics if applicable
	// This allows us to track performance over time
	mean := runEnv.R().Timer(fmt.Sprintf("time_to_first_payment,me=%s", me.Address)).Mean()
	if runEnv.BooleanParam("isNightly") {
		runEnv.R().RecordPoint(
			fmt.Sprintf("nightly_mean_time_to_first_payment,me=%s,nitroVersion=%s,runCompletedAt=%s",
				me.Address,
				utils.GetVersion("github.com/statechannels/go-nitro"),
				time.Now().Format("2006-01-02 15:04:05"),
			), float64(mean))
	}

	if runEnv.BooleanParam("isCI") {
		runEnv.R().RecordPoint(
			fmt.Sprintf("ci_mean_time_to_first_payment,me=%s,nitroVersion=%s,runCompletedAt=%s",
				me.Address,
				utils.GetVersion("github.com/statechannels/go-nitro"),
				time.Now().Format("2006-01-02 15:04:05"),
			), float64(mean))
	}
	client.MustSignalAndWait(ctx, "done", runEnv.TestInstanceCount)

	return nil

}
