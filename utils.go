package main

import (
	"context"
	"crypto/ecdsa"
	"fmt"
	"log"
	"math/big"
	"math/rand"
	"os"
	"path/filepath"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/statechannels/go-nitro/channel/state/outcome"
	nitroclient "github.com/statechannels/go-nitro/client"
	"github.com/statechannels/go-nitro/client/engine/chainservice"
	simpletcp "github.com/statechannels/go-nitro/client/engine/messageservice/simple-tcp"
	"github.com/statechannels/go-nitro/client/engine/store"
	"github.com/statechannels/go-nitro/protocols"
	"github.com/statechannels/go-nitro/protocols/directfund"
	"github.com/statechannels/go-nitro/protocols/virtualfund"
	"github.com/statechannels/go-nitro/types"
	"github.com/testground/sdk-go/network"
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

				chain.In() <- t.Transaction

			}
		}
	}()
}

// setupChain creates a mock chain instance and will share transactions with other MockChains using a sync.Topic
func setupChain(me PeerInfo, ctx context.Context, client *sync.DefaultClient) *chainservice.MockChain {

	transListener := make(chan protocols.ChainTransaction, 1000)

	chain := chainservice.NewMockChainWithTransactionListener(transListener)
	transTopic := sync.NewTopic("chain-transaction", &PeerTransaction{})
	go shareTransactions(transListener, ctx, client, transTopic, &chain, me.Address)
	go replayTransactions(ctx, client, transTopic, &chain, me.Address)

	return &chain
}

// createNitroClient starts a nitro client using the given unique sequence number and private key.
func createNitroClient(me PeerInfo, myKey *ecdsa.PrivateKey, peers map[types.Address]PeerInfo, chain *chainservice.MockChain) (*nitroclient.Client, *simpletcp.SimpleTCPMessageService) {

	store := store.NewMemStore(crypto.FromECDSA(myKey))

	peerUrlMap := make(map[types.Address]string)
	for _, p := range peers {
		peerUrlMap[p.Address] = p.Url
	}

	ms := simpletcp.NewSimpleTCPMessageService(me.Url, peerUrlMap)

	chainservice := chainservice.NewSimpleChainService(chain, me.Address)
	// TODO: Figure out good place to log this
	filename := filepath.Join("../artifacts", fmt.Sprintf("testground-%s.log", me.Address))
	logDestination, _ := os.OpenFile(filename, os.O_CREATE|os.O_WRONLY, 0666)

	client := nitroclient.New(ms, chainservice, store, logDestination)
	return &client, ms

}

func generateMyUrl(n *network.Client, seq int64) string {
	host, err := n.GetDataNetworkIP()
	if err != nil {
		panic(err)
	}
	return fmt.Sprintf("%s:%d", host, PORT_START+seq)
}

func createLedgerChannel(myAddress types.Address, counterparty types.Address, nitroClient *nitroclient.Client) protocols.ObjectiveId {
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

	return r.Id

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

func createVirtualChannel(myAddress types.Address, intermediary types.Address, counterparty types.Address, nitroClient *nitroclient.Client) protocols.ObjectiveId {
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
	return r.Id

}

// generateMe generates peer info for the instance.
// It relies on being provided a unique sequence number.
func generateMe(seq int64, c *network.Client, numOfHubs int64) (PeerInfo, *ecdsa.PrivateKey) {
	url := generateMyUrl(c, seq)
	myKey, err := crypto.GenerateKey()
	if err != nil {
		panic(err)
	}
	publicKey := myKey.Public()
	publicKeyECDSA, ok := publicKey.(*ecdsa.PublicKey)
	if !ok {
		log.Fatal("error casting public key to ECDSA")
	}
	address := crypto.PubkeyToAddress(*publicKeyECDSA)

	isHub := seq <= numOfHubs

	return PeerInfo{Url: url, Address: address, IsHub: isHub}, myKey
}
