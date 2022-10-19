package utils

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"math/rand"
	"runtime/debug"
	"sort"

	s "sync"
	"sync/atomic"
	"time"

	"github.com/statechannels/go-nitro-testground/config"
	"github.com/statechannels/go-nitro-testground/peer"
	"github.com/statechannels/go-nitro/channel/state/outcome"
	nitro "github.com/statechannels/go-nitro/client"
	"github.com/statechannels/go-nitro/protocols"
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
			Network:       "default",
			Enable:        true,
			RoutingPolicy: network.AllowAll,

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

// AbbreviateSlice returns a string with abbreviated elements of the given slice.
func AbbreviateSlice[U ~[]T, T fmt.Stringer](col U) string {
	abbreviated := ""
	for i, s := range col {
		if i > 0 {
			abbreviated += ", "
		}
		abbreviated += s.String()[0:8]
	}

	return abbreviated
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

// SelectRandomHubs selects numHub hubs randomly from hubs
func SelectRandomHubs(hubs []peer.PeerInfo, numHubs int) []types.Address {
	// Copy and shuffle the slice of hubs
	shuffled := make([]peer.PeerInfo, len(hubs))
	copy(shuffled, hubs)
	rand.Shuffle(len(shuffled),
		func(i, j int) { shuffled[i], shuffled[j] = shuffled[j], shuffled[i] })

	// Select the amount of hubs we want
	selected := make([]peer.PeerInfo, numHubs)
	for i := 0; i < numHubs; i++ {
		selected[i] = shuffled[i]
	}

	// TODO: Virtual defunding seems to fail if intermediaries are not in "order".
	// The order seems to be determined by the initiator of the ledger channel.
	// Since we use the sequence number to determine the initiator, we can just sort on sequence number.
	sort.Slice(selected, func(i, j int) bool {
		return selected[i].Seq < selected[j].Seq
	})

	// Convert to addresses for the callers convenience
	selectedAddresses := make([]types.Address, numHubs)
	for i, hub := range selected {
		selectedAddresses[i] = hub.Address
	}
	return selectedAddresses
}

// CreateLedgerChannels creates a directly funded ledger channel with each hub in hubs.
// The funding for each channel will be set to amount for both participants.
// This function blocks until all ledger channels have successfully been created.
// If the participant is a hub we use the participant's Seq to determine the initiator of the ledger channel.
func CreateLedgerChannels(client nitro.Client, cm *CompletionMonitor, amount uint, me peer.PeerInfo, peers []peer.PeerInfo) []types.Destination {
	ids := []protocols.ObjectiveId{}
	cIds := []types.Destination{}
	hubs := peer.FilterByRole(peers, peer.Hub)
	for _, p := range hubs {

		// To co-ordinate creating ledger channels between hubs a hub will
		// only create a channel with another hub if it has a greater sequence number.
		if me.Role == peer.Hub && p.Seq <= me.Seq {
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
		r := client.CreateLedgerChannel(p.Address, 0, outcome)
		cIds = append(cIds, r.ChannelId)
		ids = append(ids, r.Id)
	}

	cm.WaitForObjectivesToComplete(ids)
	return cIds
}

func GetVersion(path string) string {
	info, _ := debug.ReadBuildInfo()
	for _, dep := range info.Deps {
		switch dep.Path {
		case path:
			return dep.Version

		}
	}
	panic("could not find version for " + path)
}

// RecordRunInfo records a single point using the metrics API tagged with the various parameters and versions for the run.
func RecordRunInfo(me peer.MyInfo, config config.RunConfig, metrics *runtime.MetricsApi) {
	info, _ := debug.ReadBuildInfo()
	testVersion := info.Main.Version
	nitroVersion := GetVersion("github.com/statechannels/go-nitro")
	tgVersion := GetVersion("github.com/testground/testground")
	tgSDKVersion := GetVersion("github.com/testground/sdk-go")

	runDetails := fmt.Sprintf("nitroVersion=%s,testVersion=%s,tgVersion=%s,tgSdkVersion=%s,me=%s,role=%v,hubs=%d,payers=%d,payees=%d,payeePayers=%d,duration=%s,concurrentJobs=%d,jitter=%d,latency=%d",
		nitroVersion, testVersion, tgVersion, tgSDKVersion,
		me.Address, me.Role, config.NumHubs, config.NumPayers,
		config.NumPayees, config.NumPayeePayers,
		config.PaymentTestDuration, config.ConcurrentPaymentJobs,
		config.NetworkJitter.Milliseconds(), config.NetworkLatency.Milliseconds())

	metrics.RecordPoint("run_info,"+runDetails, 1)

}
