package chain

import (
	"context"
	"io"
	"log"

	"github.com/statechannels/go-nitro/client/engine/chainservice"
)

func NewChainService(ctx context.Context, seq int64, logDestination io.Writer) chainservice.ChainService {
	fcs, err := chainservice.NewFevmChainService("https://wallaby.node.glif.io/rpc/v0", "9182b5bf5b9c966e001934ebaf008f65516290cef6e3069d11e718cbd4336aae", log.Default().Writer())
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
