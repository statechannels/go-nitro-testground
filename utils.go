package main

import (
	"context"
	"fmt"
	"math/big"
	"math/rand"
	"os"
	s "sync"
	"sync/atomic"
	"time"

	"github.com/ethereum/go-ethereum/crypto"
	p2pcrypto "github.com/libp2p/go-libp2p-core/crypto"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/statechannels/go-nitro/channel/state/outcome"
	nitroclient "github.com/statechannels/go-nitro/client"
	"github.com/statechannels/go-nitro/client/engine"
	"github.com/statechannels/go-nitro/client/engine/chainservice"
	"github.com/statechannels/go-nitro/client/engine/store"
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
	_, _ = client.Publish(ctx, peerTopic, me)

	peers := map[types.Address]PeerInfo{}
	peerChannel := make(chan *PeerInfo)
	// Ready all my peers entries from the topic
	_, _ = client.Subscribe(ctx, peerTopic, peerChannel)

	for i := 0; i <= instances-1; i++ {
		t := <-peerChannel
		// We only add the peer info if it's not ours
		if t.Address != me.Address {
			peers[t.Address] = *t
		}
	}
	return peers
}

// createNitroClient starts a nitro client using the given unique sequence number and private key.
func createNitroClient(me MyInfo, peers map[types.Address]PeerInfo, chain *chainservice.MockChain, metrics *runtime.MetricsApi) (*nitroclient.Client, *P2PMessageService) {

	store := store.NewMemStore(crypto.FromECDSA(&me.PrivateKey))

	ms := NewP2PMessageService(me, peers, metrics)

	// The outputs folder will be copied when results are collected.
	logDestination, _ := os.OpenFile("./outputs/nitro-engine.log", os.O_CREATE|os.O_WRONLY, 0666)

	client := nitroclient.New(ms, chain, store, logDestination, &engine.PermissivePolicy{}, metrics)
	return &client, ms

}

func createLedgerChannel(myAddress types.Address, counterparty types.Address, nitroClient *nitroclient.Client) directfund.ObjectiveResponse {
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

// GeneratePeerInfo generates a random  message key/ peer id and returns a PeerInfo
func generateMe(seq int64, isHub bool, ipAddress string) MyInfo {

	// We use the sequence in the random source so we generate a unique key even if another client is running at the same time
	messageKey, _, err := p2pcrypto.GenerateECDSAKeyPair(rand.New(rand.NewSource(time.Now().UnixNano() + seq)))
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

// RunJob constantly runs some job function for the given duration.
// It will spawn a new goroutine calling the job function until the amount
// of concurrently running jobs is concurrencyTarget.
func RunJob(job func(), duration time.Duration, concurrencyTarget int64) {
	// This how long we wait between checking if we can start another job.
	waitDuration := time.Duration(50 * time.Millisecond)

	jobCount := int64(0)
	startTime := time.Now()
	wg := s.WaitGroup{}

	for time.Since(startTime) < duration {
		time.Sleep(waitDuration)

		// Check if we're below our concurrency target
		if jobCount < concurrencyTarget {
			jobCount++
			wg.Add(1)

			// Start the job in a go routine
			go func() {
				job()
				atomic.AddInt64(&jobCount, -1)
				wg.Done()

			}()

		}
	}
	wg.Wait()
}

func abbreviate(s fmt.Stringer) string {
	return s.String()[0:8] + ".."
}
