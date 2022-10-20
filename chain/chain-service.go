package chain

import (
	"context"
	"io"
	"log"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/statechannels/go-nitro/client/engine/chainservice"
)

func NewChainService(ctx context.Context, seq int64, logDestination io.Writer) chainservice.ChainService {
	pk := common.Bytes2Hex(crypto.FromECDSA(getFundedPrivateKey(uint(seq))))
	fcs, err := chainservice.NewFevmChainService("https://wallaby.node.glif.io/rpc/v0", pk, log.Default().Writer())
	if err != nil {
		panic(err)
	}
	// One testground instance attempts to deploy NitroAdjudicator
	if seq == 1 {
		err = fcs.DeployAdjudicator()
		if err != nil {
			panic(err)
		}
	}

	return fcs

}
