package utils

import (
	"math/big"
	"math/rand"
	"os"

	"github.com/ethereum/go-ethereum/crypto"
	m "github.com/statechannels/go-nitro-testground/messaging"
	"github.com/statechannels/go-nitro/channel/state/outcome"
	nitroclient "github.com/statechannels/go-nitro/client"
	"github.com/statechannels/go-nitro/client/engine"
	"github.com/statechannels/go-nitro/client/engine/chainservice"
	"github.com/statechannels/go-nitro/client/engine/store"
	"github.com/statechannels/go-nitro/protocols/directfund"
	"github.com/statechannels/go-nitro/protocols/virtualfund"
	"github.com/statechannels/go-nitro/types"
	"github.com/testground/sdk-go/runtime"
)

// CreateNitroClient starts a nitro client using the given unique sequence number and private key.
func CreateNitroClient(me m.MyInfo, peers map[types.Address]m.PeerInfo, chain *chainservice.MockChain, metrics *runtime.MetricsApi) (*nitroclient.Client, *m.P2PMessageService) {

	store := store.NewMemStore(crypto.FromECDSA(&me.PrivateKey))

	ms := m.NewP2PMessageService(me, peers, metrics)

	// The outputs folder will be copied when results are collected.
	logDestination, _ := os.OpenFile("./outputs/nitro-engine.log", os.O_CREATE|os.O_WRONLY, 0666)

	client := nitroclient.New(ms, chain, store, logDestination, &engine.PermissivePolicy{}, metrics)
	return &client, ms

}

// CreateLedgerChannel creates a ledger channel with the given counterparty with a large amount of funds (1_000_000_000_000_000_000 for each party)
func CreateLedgerChannel(myAddress types.Address, counterparty types.Address, nitroClient *nitroclient.Client) directfund.ObjectiveResponse {
	outcome := outcome.Exit{outcome.SingleAssetExit{
		Allocations: outcome.Allocations{
			outcome.Allocation{
				Destination: types.AddressToDestination(myAddress),
				Amount:      big.NewInt(1_000_000_000_000_000_000),
			},
			outcome.Allocation{
				Destination: types.AddressToDestination(counterparty),
				Amount:      big.NewInt(1_000_000_000_000_000_000),
			},
		},
	}}

	request := directfund.ObjectiveRequest{
		CounterParty:      counterparty,
		Outcome:           outcome,
		AppDefinition:     types.Address{},
		AppData:           types.Bytes{},
		ChallengeDuration: big.NewInt(0),
		Nonce:             rand.Int63(),
	}
	r := nitroClient.CreateDirectChannel(request)

	return r

}

// CreateVirtualChannel creates a virtual channel with the given counterparty using the intermediary as a hub
func CreateVirtualChannel(myAddress types.Address, intermediary types.Address, counterparty types.Address, nitroClient *nitroclient.Client) virtualfund.ObjectiveResponse {
	outcome := outcome.Exit{outcome.SingleAssetExit{
		Allocations: outcome.Allocations{
			outcome.Allocation{
				Destination: types.AddressToDestination(myAddress),
				Amount:      big.NewInt(100),
			},
			outcome.Allocation{
				Destination: types.AddressToDestination(counterparty),
				Amount:      big.NewInt(100),
			},
		},
	}}

	request := virtualfund.ObjectiveRequest{
		CounterParty:      counterparty,
		Intermediary:      intermediary,
		Outcome:           outcome,
		AppDefinition:     types.Address{},
		AppData:           types.Bytes{},
		ChallengeDuration: big.NewInt(0),
		Nonce:             rand.Int63(),
	}
	r := nitroClient.CreateVirtualChannel(request)
	return r

}
