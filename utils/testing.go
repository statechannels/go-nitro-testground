package utils

import (
	"context"
	"errors"
	"fmt"

	s "sync"
	"sync/atomic"
	"time"

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

func Abbreviate(s fmt.Stringer) string {
	return s.String()[0:8] + ".."
}
