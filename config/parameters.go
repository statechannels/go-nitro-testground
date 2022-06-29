package config

type param string

// The names of various test parameters we expect in runEnv.
const (
	numHubsParam               param = "numOfHubs"
	numPayeeParam              param = "numOfPayees"
	numPayersParam             param = "numOfPayers"
	NumPayeePayersParam        param = "numOfPayeePayers"
	networkJitterParam         param = "networkJitter"
	networkLatencyParam        param = "networkLatency"
	concurrentPaymentJobsParam param = "concurrentPaymentJobs"
	paymentTestDurationParam   param = "paymentTestDuration"
)
