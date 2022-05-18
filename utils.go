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
	"github.com/testground/sdk-go/runtime"
	"github.com/testground/sdk-go/sync"
)

func getPeers(ctx context.Context, client sync.Client, peerTopic *sync.Topic, runenv *runtime.RunEnv) map[types.Address]PeerEntry {
	peers := map[types.Address]PeerEntry{}
	peerChannel := make(chan *PeerEntry)
	client.Subscribe(ctx, peerTopic, peerChannel)

	for i := 0; i <= runenv.TestInstanceCount-1; i++ {
		t := <-peerChannel
		peers[t.Address] = *t
	}
	return peers
}

func generateRandomAddress() (types.Address, *ecdsa.PrivateKey) {
	sk, err := crypto.GenerateKey()
	if err != nil {
		panic(err)
	}
	myAddress := getAddressFromSecretKey(*sk)
	return myAddress, sk
}

func shareTransactions(listener chan protocols.ChainTransaction, runenv *runtime.RunEnv, ctx context.Context, client *sync.DefaultClient, topic *sync.Topic, chain *chainservice.MockChain, myAddress types.Address) {
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

func handleTransactions(runenv *runtime.RunEnv, ctx context.Context, client *sync.DefaultClient, topic *sync.Topic, chain *chainservice.MockChain, myAddress types.Address) {
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

func setupClient(seq int64, myKey *ecdsa.PrivateKey, myUrl string, peers map[types.Address]PeerEntry, transListener chan protocols.ChainTransaction) (*nitroclient.Client, *simpletcp.SimpleTCPMessageService, *chainservice.MockChain) {

	store := store.NewMemStore(crypto.FromECDSA(myKey))
	myAddress := getAddressFromSecretKey(*myKey)
	peerUrlMap := make(map[types.Address]string)
	for _, p := range peers {
		peerUrlMap[p.Address] = p.Url
	}

	ms := simpletcp.NewSimpleTCPMessageService(myUrl, peerUrlMap)
	chain := chainservice.NewMockChainWithTransactionListener(transListener)
	chain.Subscribe(myAddress)
	chainservice := chainservice.NewSimpleChainService(&chain, myAddress)
	// TODO: Figure out good place to log this
	filename := filepath.Join("../artifacts", fmt.Sprintf("testground-%d.log", seq))
	logDestination, _ := os.OpenFile(filename, os.O_CREATE|os.O_WRONLY, 0666)

	client := nitroclient.New(ms, chainservice, store, logDestination)
	return &client, ms, &chain

}

func generateMyUrl(n *network.Client, seq int64) string {
	host, err := n.GetDataNetworkIP()
	if err != nil {
		panic(err)
	}
	return fmt.Sprintf("%s:%d", host, PORT_START+seq)
}

func getAddressFromSecretKey(secretKey ecdsa.PrivateKey) types.Address {
	publicKey := secretKey.Public()
	publicKeyECDSA, ok := publicKey.(*ecdsa.PublicKey)
	if !ok {
		log.Fatal("error casting public key to ECDSA")
	}
	return crypto.PubkeyToAddress(*publicKeyECDSA)
}

func createLedgerChannel(runenv *runtime.RunEnv, myAddress types.Address, counterparty types.Address, nitroClient *nitroclient.Client) {
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
	runenv.RecordMessage("channel %s created", r.ChannelId)

}

func selectRandomPeer(peers map[types.Address]PeerEntry, myAddress types.Address, shouldBeHub bool) types.Address {

	peersWithoutMe := make([]types.Address, 0)
	for _, p := range peers {
		if myAddress != p.Address && p.IsHub == shouldBeHub {
			peersWithoutMe = append(peersWithoutMe, p.Address)
		}
	}

	randomIndex := rand.Intn(len(peersWithoutMe))

	return peersWithoutMe[randomIndex]

}

func createVirtualChannel(runenv *runtime.RunEnv, myAddress types.Address, intermediary types.Address, counterparty types.Address, nitroClient *nitroclient.Client) {
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
	runenv.RecordMessage("virtual channel creation started %s", r.ChannelId)

}
