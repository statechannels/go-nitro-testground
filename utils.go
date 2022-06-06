package main

import (
	"context"
	"fmt"
	"math/big"
	"math/rand"
	"os"
	"path/filepath"
	"time"

	"github.com/ethereum/go-ethereum/crypto"
	p2pcrypto "github.com/libp2p/go-libp2p-core/crypto"
	"github.com/libp2p/go-libp2p-core/peer"
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
	"github.com/testground/sdk-go/sync"
)

// getPeers will broadcast our peer info to other instances and listen for broadcasts from other instances.
// It returns a map that contains a PeerInfo for all other instances.
// The map will not contain a PeerInfo for the current instance.
func getPeers(me PeerInfo, ctx context.Context, client sync.Client, instances int) map[types.Address]PeerInfo {

	peerTopic := sync.NewTopic("peer-info", &PeerInfo{})

	// Publish my entry to the topic
	client.Publish(ctx, peerTopic, me)

	peers := map[types.Address]PeerInfo{}
	peerChannel := make(chan *PeerInfo)
	// Ready all my peers entries from the topic
	client.Subscribe(ctx, peerTopic, peerChannel)

	for i := 0; i <= instances-1; i++ {
		t := <-peerChannel
		// We only add the peer info if it's not ours
		if t.Address != me.Address {
			peers[t.Address] = *t
		}
	}
	return peers
}

func shareTransactions(listener chan protocols.ChainTransaction, ctx context.Context, client *sync.DefaultClient, topic *sync.Topic, chain *chainservice.MockChain, myAddress types.Address) {
	// TODO: Close this gracefully?
	go func() {
		for trans := range listener {

			_, err := client.Publish(ctx, topic, &PeerTransaction{From: myAddress, Transaction: trans})
			if err != nil {
				// TODO: This gofunc should get aborted instead of just swallowing an error
				return
			}
		}
	}()
}

// replayTransactions listens for transactions that occured on other client's chains and replays them on ours
func replayTransactions(ctx context.Context, client *sync.DefaultClient, topic *sync.Topic, chain *chainservice.MockChain, myAddress types.Address) {
	// TODO: Close this gracefully?
	go func() {
		peerTransactions := make(chan *PeerTransaction)
		client.Subscribe(ctx, topic, peerTransactions)

		for t := range peerTransactions {
			if t.From != myAddress {

				chain.SendTransaction(t.Transaction)

			}
		}
	}()
}

// setupChain creates a mock chain instance and will share transactions with other MockChains using a sync.Topic
func setupChain(me PeerInfo, ctx context.Context, client *sync.DefaultClient) *chainservice.MockChain {

	transListener := make(chan protocols.ChainTransaction, 1000)

	chain := chainservice.NewMockChainWithTransactionListener(transListener)
	transTopic := sync.NewTopic("chain-transaction", &PeerTransaction{})
	go shareTransactions(transListener, ctx, client, transTopic, chain, me.Address)
	go replayTransactions(ctx, client, transTopic, chain, me.Address)

	return chain
}

// createNitroClient starts a nitro client using the given unique sequence number and private key.
func createNitroClient(me MyInfo, peers map[types.Address]PeerInfo, chain *chainservice.MockChain, metrics *runtime.MetricsApi) (*nitroclient.Client, *P2PMessageService) {

	store := store.NewMemStore(crypto.FromECDSA(&me.PrivateKey))

	ms := NewP2PMessageService(me, peers, metrics)

	// TODO: Figure out good place to log this
	filename := filepath.Join("../artifacts", fmt.Sprintf("testground-%s.log", me.Address))
	logDestination, _ := os.OpenFile(filename, os.O_CREATE|os.O_WRONLY, 0666)

	client := nitroclient.New(ms, chain, store, logDestination, &engine.PermissivePolicy{}, metrics)
	return &client, ms

}

func createLedgerChannel(myAddress types.Address, counterparty types.Address, nitroClient *nitroclient.Client) directfund.ObjectiveResponse {
	outcome := outcome.Exit{outcome.SingleAssetExit{
		Allocations: outcome.Allocations{
			outcome.Allocation{
				Destination: types.AddressToDestination(myAddress),
				Amount:      big.NewInt(1000),
			},
			outcome.Allocation{
				Destination: types.AddressToDestination(counterparty),
				Amount:      big.NewInt(1000),
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

func selectRandomPeer(peers []PeerInfo) types.Address {

	randomIndex := rand.Intn(len(peers))

	return peers[randomIndex].Address

}

// filterPeersByHub returns peers that where p.IsHub == shouldBeHub
func filterPeersByHub(peers map[types.Address]PeerInfo, shouldBeHub bool) []PeerInfo {
	filteredPeers := make([]PeerInfo, 0)
	for _, p := range peers {
		if p.IsHub == shouldBeHub {
			filteredPeers = append(filteredPeers, p)
		}
	}
	return filteredPeers
}

func createVirtualChannel(myAddress types.Address, intermediary types.Address, counterparty types.Address, nitroClient *nitroclient.Client) virtualfund.ObjectiveResponse {
	outcome := outcome.Exit{outcome.SingleAssetExit{
		Allocations: outcome.Allocations{
			outcome.Allocation{
				Destination: types.AddressToDestination(myAddress),
				Amount:      big.NewInt(1),
			},
			outcome.Allocation{
				Destination: types.AddressToDestination(counterparty),
				Amount:      big.NewInt(1),
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

// GeneratePeerInfo generates a random  message key/ peer id and returns a PeerInfo
func generateMe(seq int64, isHub bool, ipAddress string) MyInfo {

	messageKey, _, err := p2pcrypto.GenerateECDSAKeyPair(rand.New(rand.NewSource(time.Now().UnixNano())))
	if err != nil {
		panic(err)
	}

	id, err := peer.IDFromPrivateKey(messageKey)
	if err != nil {
		panic(err)
	}
	privateKey, err := crypto.GenerateKey()
	if err != nil {
		panic(err)
	}
	address := crypto.PubkeyToAddress(privateKey.PublicKey)
	port := int64(PORT_START + seq)
	myPeerInfo := PeerInfo{Id: id, Address: address, IsHub: isHub, Port: port, IpAddress: ipAddress}
	return MyInfo{myPeerInfo, *privateKey, messageKey}
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
