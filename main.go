package main

import (
	"github.com/statechannels/go-nitro-testground/tests"

	"github.com/testground/sdk-go/run"
)

func main() {
	run.InvokeMap(map[string]interface{}{
		"virtual-payment":      run.InitializedTestCaseFn(tests.CreateVirtualPaymentTest),
		"fevm-virtual-payment": run.InitializedTestCaseFn(tests.CreateFEVMVirtualFundTest),
	})
}
