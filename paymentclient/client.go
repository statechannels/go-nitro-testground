package paymentclient

import (
	"math/big"
	"math/rand"
	"os"

	"github.com/ethereum/go-ethereum/crypto"
	m "github.com/statechannels/go-nitro-testground/messaging"
	"github.com/statechannels/go-nitro-testground/peer"

	"github.com/statechannels/go-nitro/channel/state/outcome"
	nitroclient "github.com/statechannels/go-nitro/client"
	"github.com/statechannels/go-nitro/client/engine"
	"github.com/statechannels/go-nitro/client/engine/chainservice"
	"github.com/statechannels/go-nitro/client/engine/store"
	"github.com/statechannels/go-nitro/protocols"
	"github.com/statechannels/go-nitro/protocols/directfund"
	"github.com/statechannels/go-nitro/protocols/virtualfund"
	"github.com/statechannels/go-nitro/types"
	"github.com/testground/sdk-go/runtime"
)

// PaymentClient is a basic client that wraps a go-nitro client.
// It it uses so to simulate making payments.
type PaymentClient struct {
	client *nitroclient.Client
	ms     *m.P2PMessageService
	me     peer.MyInfo
	peers  []peer.PeerInfo
	cm     *completionMonitor
}

// NewClient creates a new payment client. It will create a new nitro client as well as a message service to communicate with the peers.
func NewClient(me peer.MyInfo, peers []peer.PeerInfo, chain *chainservice.MockChain, metrics *runtime.MetricsApi) *PaymentClient {

	store := store.NewMemStore(crypto.FromECDSA(&me.PrivateKey))

	ms := m.NewP2PMessageService(me, peers, metrics)

	// The outputs folder will be copied when results are collected.
	logDestination, _ := os.OpenFile("./outputs/nitro-client.log", os.O_CREATE|os.O_WRONLY, 0666)

	client := nitroclient.New(ms, chain, store, logDestination, &engine.PermissivePolicy{}, metrics)
	cm := newCompletionMonitor(&client)

	return &PaymentClient{client: &client, ms: ms, me: me, cm: cm, peers: peers}

}

// ConnectToPeers establishes a connection to all our peers using the lib p2p2 message service
// This should be called once all clients have been created.
func (c *PaymentClient) ConnectToPeers() {
	c.ms.DialPeers()
}

// CreateLedgerChannels creates a directly funded ledger channel with each hub in hubs.
// The funding for each channel will be set to amount for both participants.
// this function blocks until all ledger channels have successfully been created.
func (c *PaymentClient) CreateLedgerChannels(amount uint) {
	ids := []protocols.ObjectiveId{}
	for _, p := range c.peers {
		if p.Role != peer.Hub {
			continue
		}
		outcome := outcome.Exit{outcome.SingleAssetExit{
			Allocations: outcome.Allocations{
				outcome.Allocation{
					Destination: types.AddressToDestination(c.me.Address),
					Amount:      big.NewInt(int64(amount)),
				},
				outcome.Allocation{
					Destination: types.AddressToDestination(p.Address),
					Amount:      big.NewInt(int64(amount)),
				},
			},
		}}

		request := directfund.ObjectiveRequest{
			CounterParty:      p.Address,
			Outcome:           outcome,
			AppDefinition:     types.Address{},
			AppData:           types.Bytes{},
			ChallengeDuration: big.NewInt(0),
			Nonce:             rand.Int63(),
		}
		r := c.client.CreateDirectChannel(request)
		ids = append(ids, r.Id)
	}
	c.cm.waitForObjectivesToComplete(ids)

}

// CreatePaymentChannel establishes a payment channel with the given payee using the given hub.
// It returns a payment channel that can be used to pay the payee.
// this function blocks until the virtual funding objective is complete and the channel is open.
func (c *PaymentClient) CreatePaymentChannel(hub types.Address, payee types.Address, amount uint) *PaymentChannel {
	outcome := outcome.Exit{outcome.SingleAssetExit{
		Allocations: outcome.Allocations{
			outcome.Allocation{
				Destination: types.AddressToDestination(c.me.Address),
				Amount:      big.NewInt(int64(amount)),
			},
			outcome.Allocation{
				Destination: types.AddressToDestination(payee),
				Amount:      big.NewInt(0),
			},
		},
	}}

	request := virtualfund.ObjectiveRequest{
		CounterParty:      payee,
		Intermediary:      hub,
		Outcome:           outcome,
		AppDefinition:     types.Address{},
		AppData:           types.Bytes{},
		ChallengeDuration: big.NewInt(0),
		Nonce:             rand.Int63(),
	}
	r := c.client.CreateVirtualChannel(request)
	c.cm.waitForObjectivesToComplete([]protocols.ObjectiveId{r.Id})
	pc := PaymentChannel{id: r.ChannelId, Total: 0, maxAmount: amount, nitroclient: c.client, cm: c.cm, payee: payee}
	return &pc

}

// Close closes the payment client
func (pc *PaymentClient) Close() {
	pc.ms.Close()
	pc.cm.close()

}

// PaymentChannel is a payment channel that can be used to pay a payee.
type PaymentChannel struct {
	id          types.Destination
	payee       types.Address
	Total       uint
	maxAmount   uint
	nitroclient *nitroclient.Client
	cm          *completionMonitor
	done        bool
}

// Id returns the channel id of the payment channel
func (pc *PaymentChannel) Id() types.Destination {
	return pc.id
}

// Pay pays the payee the paymentAmount by adding it to the total that will be paid when the channel is settled.
func (pc *PaymentChannel) Pay(paymentAmount uint) {
	if pc.done {
		panic("Payment channel is already closed")
	}
	if pc.Total+paymentAmount > pc.maxAmount {
		panic("Payment channel is full")
	}

	pc.Total += paymentAmount

}

// Settle closes the payment channel and pays out the Total amount to the payee.
func (pc *PaymentChannel) Settle() {
	if pc.done {
		panic("Payment channel is already closed")
	}
	pc.done = true
	r := pc.nitroclient.CloseVirtualChannel(pc.id, big.NewInt(int64(pc.Total)))
	pc.cm.waitForObjectivesToComplete([]protocols.ObjectiveId{r})
}
