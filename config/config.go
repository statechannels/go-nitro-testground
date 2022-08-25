package config

import (
	"fmt"
	"time"

	"github.com/testground/sdk-go/runtime"
)

// RunConfig is the configuration for a test run.
type RunConfig struct {
	NumHubs               uint
	NumPayees             uint
	NumPayers             uint
	NumPayeePayers        uint
	ConcurrentPaymentJobs uint
	NetworkJitter         time.Duration
	NetworkLatency        time.Duration
	PaymentTestDuration   time.Duration
}

//  Validate validates the config values. It uses instanceCount to check that it has the correct amount of roles.
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
	config.NetworkJitter = time.Duration(runEnv.IntParam(string(networkJitterParam))) * time.Millisecond
	config.NetworkLatency = time.Duration(runEnv.IntParam(string(networkLatencyParam))) * time.Millisecond
	config.PaymentTestDuration = time.Duration(runEnv.IntParam(string(paymentTestDurationParam))) * time.Second
	config.ConcurrentPaymentJobs = uint(runEnv.IntParam(string(concurrentPaymentJobsParam)))
	err := config.Validate(uint(runEnv.TestInstanceCount))

	return config, err
}