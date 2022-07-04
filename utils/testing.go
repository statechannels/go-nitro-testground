package utils

import (
	"context"
	"errors"
	"fmt"
	"math/rand"

	s "sync"
	"sync/atomic"
	"time"

	"github.com/statechannels/go-nitro-testground/peer"
	"github.com/testground/sdk-go/network"
	"github.com/testground/sdk-go/runtime"
	"github.com/testground/sdk-go/sync"
)

// ConfigureNetworkClient sets up the network client for a test run.
// It will check for the network jitter and latency parameters and adjust the network accordingly.
func ConfigureNetworkClient(ctx context.Context, client sync.Client, runEnv *runtime.RunEnv, networkJitterMS, networkLatencyMS int) (*network.Client, error) {
	// instantiate a network client amd wait for it to be ready.
	net := network.NewClient(client, runEnv)

	runEnv.RecordMessage("waiting for network initialization")
	net.MustWaitNetworkInitialized(ctx)

	if !runEnv.TestSidecar && (networkJitterMS > 0 || networkLatencyMS > 0) {
		err := errors.New("can only apply network jitter/latency when running with docker")
		return nil, err

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
		net.MustConfigureNetwork(ctx, &config)

	}

	runEnv.RecordMessage("network configured")
	return net, nil
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
