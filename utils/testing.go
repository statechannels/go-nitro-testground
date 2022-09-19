package utils

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"math/rand"

	s "sync"
	"sync/atomic"
	"time"

	"github.com/statechannels/go-nitro-testground/peer"
	"github.com/statechannels/go-nitro/channel/state/outcome"
	nitro "github.com/statechannels/go-nitro/client"
	"github.com/statechannels/go-nitro/protocols"
	"github.com/statechannels/go-nitro/protocols/directfund"
	"github.com/statechannels/go-nitro/protocols/virtualfund"
	"github.com/statechannels/go-nitro/types"
	"github.com/testground/sdk-go/network"
	"github.com/testground/sdk-go/runtime"
	"github.com/testground/sdk-go/sync"
)

const (
	FINNEY_IN_WEI = 1000000000000000
	GWEI_IN_WEI   = 1000000000
	KWEI_IN_WEI   = 1000
)

// ConfigureNetworkClient configures a network client with the given jitter and latency settings.
func ConfigureNetworkClient(ctx context.Context, netClient *network.Client, syncClient sync.Client, runEnv *runtime.RunEnv, networkJitterMS, networkLatencyMS int) error {

	runEnv.RecordMessage("waiting for network initialization")
	netClient.MustWaitNetworkInitialized(ctx)

	if !runEnv.TestSidecar && (networkJitterMS > 0 || networkLatencyMS > 0) {
		err := errors.New("can only apply network jitter/latency when running with docker")
		return err

	} else if runEnv.TestSidecar {

		config := network.Config{
			// Control the "default" network. At the moment, this is the only network.
			Network: "default",
			Enable:  true,

			// Set the traffic shaping characteristics.
			Default: network.LinkShape{
				Latency: time.Duration(networkLatencyMS) * time.Millisecond,
				Jitter:  time.Duration(networkJitterMS) * time.Millisecond,
			},

			// Set what state the sidecar should signal back to you when it's done.
			CallbackState: "network-configured",
		}
		netClient.MustConfigureNetwork(ctx, &config)

	}

	runEnv.RecordMessage("network configured")
	return nil
}

// RunJobs constantly runs some job function for the given duration.
// It will spawn a new goroutine calling the job function until the amount
// of concurrently running jobs is concurrencyTarget.
func RunJobs(job func(), duration time.Duration, concurrencyTarget int64) {
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

// Abbreviate shortens a string to 8 characters and adds an ellipsis.
func Abbreviate(s fmt.Stringer) string {
	return s.String()[0:8] + ".."
}

// SharePeerInfo will broadcast our peer info to other instances and listen for broadcasts from other instances.
// It returns a slice that contains a PeerInfo for all other instances.
// The slice will not contain a PeerInfo for the current instance.
func SharePeerInfo(me peer.PeerInfo, ctx context.Context, client sync.Client, instances int) []peer.PeerInfo {

	peerTopic := sync.NewTopic("peer-info", peer.PeerInfo{})

	peers := []peer.PeerInfo{}
	peerChannel := make(chan *peer.PeerInfo)

	_, _ = client.MustPublishSubscribe(ctx, peerTopic, me, peerChannel)

	for i := 0; i <= instances-1; i++ {
		t := <-peerChannel
		// We only add the peer info if it's not ours
		if t.Address != me.Address {
			peers = append(peers, *t)
		}
	}
	return peers
}

// SelectRandom selects a random element from a slice.
func SelectRandom[U ~[]T, T any](collection U) T {
	randomIndex := rand.Intn(len(collection))
	return collection[randomIndex]
}

// CreateLedgerChannels creates a directly funded ledger channel with each hub in hubs.
// The funding for each channel will be set to amount for both participants.
// This function blocks until all ledger channels have successfully been created.
func CreateLedgerChannels(client nitro.Client, cm *CompletionMonitor, amount uint, me peer.PeerInfo, peers []peer.PeerInfo) []types.Destination {
	ids := []protocols.ObjectiveId{}
	cIds := []types.Destination{}
	for _, p := range peers {
		if p.Role != peer.Hub {
			continue
		}
		outcome := outcome.Exit{outcome.SingleAssetExit{
			Allocations: outcome.Allocations{
				outcome.Allocation{
					Destination: types.AddressToDestination(me.Address),
					Amount:      big.NewInt(int64(amount)),
				},
				outcome.Allocation{
					Destination: types.AddressToDestination(p.Address),
					Amount:      big.NewInt(int64(amount)),
				},
			},
		}}

		request := directfund.ObjectiveRequestForConsensusApp{
			CounterParty: p.Address,
			Outcome:      outcome,

			ChallengeDuration: 0,
			Nonce:             rand.Uint64(),
		}
		r := client.CreateLedgerChannel(request)
		cIds = append(cIds, r.ChannelId)
		ids = append(ids, r.Id)
	}

	cm.WaitForObjectivesToComplete(ids)
	return cIds
}

// GenerateVirtualFundObjectiveRequest generates a virtual channel request  with 10 gwei in funding
func GenerateVirtualFundObjectiveRequest(me, payee, hub types.Address) virtualfund.ObjectiveRequest {
	outcome := outcome.Exit{outcome.SingleAssetExit{
		Allocations: outcome.Allocations{
			outcome.Allocation{
				Destination: types.AddressToDestination(me),
				Amount:      big.NewInt(int64(10 * GWEI_IN_WEI)),
			},
			outcome.Allocation{
				Destination: types.AddressToDestination(payee),
				Amount:      big.NewInt(0),
			},
		},
	}}

	return virtualfund.ObjectiveRequest{
		CounterParty:      payee,
		Intermediary:      hub,
		Outcome:           outcome,
		AppDefinition:     types.Address{},
		AppData:           types.Bytes{},
		ChallengeDuration: 0,
		Nonce:             rand.Uint64(),
	}
}
