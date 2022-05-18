package main

import (
	"github.com/statechannels/go-nitro/protocols"

	"github.com/statechannels/go-nitro/types"

	"github.com/testground/sdk-go/run"
)

type PeerEntry struct {
	Address types.Address
	Url     string
	IsHub   bool
}

type PeerTransaction struct {
	Transaction protocols.ChainTransaction
	From        types.Address
}

const PORT_START = 7000

func main() {
	run.InvokeMap(map[string]interface{}{
		"create-ledgers": createLedgerTest,
		"create-virtual": createVirtualTest,
	})
}
