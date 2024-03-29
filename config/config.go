package config

import (
	"fmt"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/statechannels/go-nitro/types"
	"github.com/testground/sdk-go/runtime"
)

// RunConfig is the configuration for a test run.
type RunConfig struct {
	NumHubs                      uint
	NumPayees                    uint
	NumPayers                    uint
	NumPayeePayers               uint
	NumIntermediaries            uint
	ConcurrentPaymentJobs        uint
	NetworkJitter                time.Duration
	NetworkLatency               time.Duration
	PaymentTestDuration          time.Duration
	UseHyperspace                bool
	HyperspaceAdjudicatorAddress types.Address
	StoreSyncFrequency           uint
}

// Validate validates the config values. It uses instanceCount to check that it has the correct amount of roles.
func (c *RunConfig) Validate(instanceCount uint) error {
	total := c.NumHubs + c.NumPayeePayers + c.NumPayees + c.NumPayers
	if total != instanceCount {
		return fmt.Errorf("total number of roles (%d) does not match instance count (%d)", total, instanceCount)
	}

	return nil
}

// GetRunConfig generates a RunConfig by reading the parameters from runEnv
func GetRunConfig(runEnv *runtime.RunEnv) (RunConfig, error) {
	config := RunConfig{}

	config.NumHubs = uint(runEnv.IntParam(string(numHubsParam)))
	config.NumPayees = uint(runEnv.IntParam(string(numPayeeParam)))
	config.NumPayers = uint(runEnv.IntParam(string(numPayersParam)))
	config.NumPayeePayers = uint(runEnv.IntParam(string(NumPayeePayersParam)))
	config.NumIntermediaries = uint(runEnv.IntParam(string(NumIntermediaries)))
	config.NetworkJitter = time.Duration(runEnv.IntParam(string(networkJitterParam))) * time.Millisecond
	config.NetworkLatency = time.Duration(runEnv.IntParam(string(networkLatencyParam))) * time.Millisecond
	config.PaymentTestDuration = time.Duration(runEnv.IntParam(string(paymentTestDurationParam))) * time.Second
	config.ConcurrentPaymentJobs = uint(runEnv.IntParam(string(concurrentPaymentJobsParam)))
	config.UseHyperspace = (runEnv.BooleanParam(string(useHyperspace)))
	config.HyperspaceAdjudicatorAddress = common.HexToAddress(runEnv.StringParam(string(hyperspaceAdjudicatorAddress)))
	config.StoreSyncFrequency = uint(runEnv.IntParam(string(storeSyncFrequency)))
	err := config.Validate(uint(runEnv.TestInstanceCount))

	return config, err
}

// GetSleepDuration calculates the duration to sleep before and after the payment test.
func (c *RunConfig) GetSleepDuration() time.Duration {

	// The duration we wait is based on the payment test duration and the amount of concurrent jobs.
	toSleep := (c.PaymentTestDuration * time.Duration(c.ConcurrentPaymentJobs)) / 10
	// Restrict the sleep duration to be between 1 and 20 seconds
	if toSleep > 20*time.Second {
		toSleep = 20 * time.Second
	}
	if toSleep < 1*time.Second {
		toSleep = 1 * time.Second
	}
	return toSleep
}
