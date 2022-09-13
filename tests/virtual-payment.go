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
	m "github.com/statechannels/go-nitro-testground/messaging"
	"github.com/statechannels/go-nitro-testground/peer"
	"github.com/statechannels/go-nitro-testground/utils"
	"github.com/statechannels/go-nitro/channel/state/outcome"
	nitro "github.com/statechannels/go-nitro/client"
	"github.com/statechannels/go-nitro/client/engine"
	"github.com/statechannels/go-nitro/client/engine/store"
	"github.com/statechannels/go-nitro/protocols"

	"github.com/statechannels/go-nitro/protocols/virtualfund"
	"github.com/statechannels/go-nitro/types"
	"github.com/testground/sdk-go/run"
	"github.com/testground/sdk-go/runtime"
	"github.com/testground/sdk-go/sync"
)

const (
	FINNEY_IN_WEI = 1000000000000000
	GWEI_IN_WEI   = 1000000000
	KWEI_IN_WEI   = 1000
)

func CreateVirtualPaymentTest(runEnv *runtime.RunEnv, init *run.InitContext) error {
	runEnv.D().SetFrequency(1 * time.Second)
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

	store := store.NewMemStore(crypto.FromECDSA(&me.PrivateKey))

	ms := m.NewP2PMessageService(me, peers, runEnv.D())

	// The outputs folder will be copied when results are collected.
	logDestination, _ := os.OpenFile("./outputs/nitro-client.log", os.O_CREATE|os.O_WRONLY, 0666)

	nClient := nitro.New(ms, chain.NewChainService(seq, logDestination), store, logDestination, &engine.PermissivePolicy{}, runEnv.D())
	cm := utils.NewCompletionMonitor(&nClient, runEnv.RecordMessage)
	defer cm.Close()
	runEnv.RecordMessage("payment client created")
	// We wait until everyone has chosen an address.
	client.MustSignalAndWait(ctx, "client created", runEnv.TestInstanceCount)
	ms.DialPeers()
	client.MustSignalAndWait(ctx, "client connected", runEnv.TestInstanceCount)

	if me.Role == peer.Hub {
		client.MustSignalAndWait(ctx, sync.State("ledgerDone"), runEnv.TestInstanceCount)
	} else {
		// Create ledger channels with all the hubs
		utils.CreateLedgerChannels(nClient, cm, FINNEY_IN_WEI, me.PeerInfo, peers)
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

			var channelId types.Destination
			runEnv.D().Timer("time_to_first_payment," + runDetails).Time(func() {

				outcome := outcome.Exit{outcome.SingleAssetExit{
					Allocations: outcome.Allocations{
						outcome.Allocation{
							Destination: types.AddressToDestination(me.Address),
							Amount:      big.NewInt(int64(10 * GWEI_IN_WEI)),
						},
						outcome.Allocation{
							Destination: types.AddressToDestination(randomPayee.Address),
							Amount:      big.NewInt(0),
						},
					},
				}}

				request := virtualfund.ObjectiveRequest{
					CounterParty:      randomPayee.Address,
					Intermediary:      randomHub.Address,
					Outcome:           outcome,
					AppDefinition:     types.Address{},
					AppData:           types.Bytes{},
					ChallengeDuration: big.NewInt(0),
					Nonce:             rand.Int63(),
				}

				r := nClient.CreateVirtualChannel(request)
				channelId = r.ChannelId
				cm.WaitForObjectivesToComplete([]protocols.ObjectiveId{r.Id})

				runEnv.RecordMessage("Opened virtual channel %s with %s using hub %s", utils.Abbreviate(channelId), utils.Abbreviate(randomPayee.Address), utils.Abbreviate(randomHub.Address))

				paymentAmount := big.NewInt(KWEI_IN_WEI)
				nClient.Pay(r.ChannelId, paymentAmount)
				runEnv.RecordMessage("Sent payment of %d  wei to %s using channel %s", paymentAmount.Int64(), utils.Abbreviate(randomPayee.Address), utils.Abbreviate(channelId))

				// TODO: Should we wait for receipt of this payment before stopping the time_to_first_payment timer?
			})

			// Perform between 1 and 5 payments additional payments
			amountOfPayments := 1 + rand.Intn(4)
			for i := 0; i < amountOfPayments; i++ {
				// pay between 1 and 2 kwei
				paymentAmount := big.NewInt(KWEI_IN_WEI + (rand.Int63n(KWEI_IN_WEI)))
				nClient.Pay(channelId, paymentAmount)

				runEnv.RecordMessage("Sent payment of %d wei to %s using channel %s", paymentAmount.Int64(), utils.Abbreviate(randomPayee.Address), utils.Abbreviate(channelId))

			}

			// TODO: If we attempt to close a virtual channel too fast we can cause other clients to fail.
			// See https://github.com/statechannels/go-nitro/issues/744
			time.Sleep(time.Duration(250 * time.Millisecond))

			// TODO: get payment balance and output it to the log
			runEnv.RecordMessage("Closing %s with payment to %s", utils.Abbreviate(channelId), utils.Abbreviate(randomPayee.Address))
			nClient.CloseVirtualChannel(channelId)

		}

		// Run the job(s)
		utils.RunJobs(createVirtualPaymentsJob, config.PaymentTestDuration, int64(config.ConcurrentPaymentJobs))
	}

	client.MustSignalAndWait(ctx, "done", runEnv.TestInstanceCount)

	return nil

}
