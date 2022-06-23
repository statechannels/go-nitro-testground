package main

import (
	"github.com/statechannels/go-nitro/protocols"

	"github.com/statechannels/go-nitro/types"

	"github.com/testground/sdk-go/run"
)

// PeerTransaction is a a transaction that also indicates which peer sent it.
// This is used to replay a transaction on our local chain instance
type PeerTransaction struct {
	Transaction protocols.ChainTransaction
	From        types.Address
}

// The first TCP port for the range of ports used by clients' messaging services.
const PORT_START = 7000

func main() {
	run.InvokeMap(map[string]interface{}{
		"virtual-payment":  createVirtualPaymentTest,
		"virtual-payment2": createVirtualPayment2Test,
	})
}
