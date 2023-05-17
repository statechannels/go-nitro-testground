package chain

import (
	"context"
	"io"
	"log"

	"github.com/ethereum/go-ethereum/common"
	"github.com/statechannels/go-nitro-testground/utils"
	"github.com/statechannels/go-nitro/client/engine/chainservice"
	NitroAdjudicator "github.com/statechannels/go-nitro/client/engine/chainservice/adjudicator"
	chainutils "github.com/statechannels/go-nitro/client/engine/chainservice/utils"
	"github.com/statechannels/go-nitro/types"
	"github.com/testground/sdk-go/sync"
)

// NewHyperspaceChainService creates a new chain service for the Hyperspace testnet
func NewHyperspaceChainService(ctx context.Context, seq int64, naAddress types.Address, logDestination io.Writer) chainservice.ChainService {
	// TODO: Eventually we should generate accounts using a HD wallet path used by the hyperspace burner wallet ( "m/44'/1'/0'/0")
	// However the addresses we generate from hd wallet seem to differ from the ones generated by the glif wallet
	// For now we just use the same HD wallet path as hardhat, and manually fund the addresses
	cs, err := chainservice.NewEthChainService(
		"https://api.hyperspace.node.glif.io/rpc/v0",
		GetHyperspaceFundedPrivateKey(uint(seq)),
		naAddress,
		common.Address{},
		common.Address{},
		logDestination)

	if err != nil {
		log.Fatal(err)
	}
	return cs
}

func NewChainService(ctx context.Context, seq int64, logDestination io.Writer, syncClient sync.Client, instances int) chainservice.ChainService {

	var naAddress types.Address
	// One testground instance attempts to deploy NitroAdjudicator
	if seq == 1 {

		naAddress, err := deployAdjudicator(ctx)
		if err != nil {
			log.Fatal(err)
		}
		utils.SendAdjudicatorAddress(context.Background(), syncClient, naAddress)
	} else {
		naAddress = utils.WaitForAdjudicatorAddress(context.Background(), syncClient, instances)
	}
	cs, err := chainservice.NewEthChainService(
		"ws://chain:8545/",
		GetHardhatFundedPrivateKey(uint(seq)),
		naAddress,
		common.Address{},
		common.Address{},
		logDestination)

	if err != nil {
		log.Fatal(err)
	}
	return cs
}

// deployAdjudicator deploys th  NitroAdjudicator contract.
func deployAdjudicator(ctx context.Context) (common.Address, error) {
	const FUNDED_TEST_PK = "59c6995e998f97a5a0044966f0945389dc9e86dae88c7a8412f4603b6b78690d"
	client, txSubmitter, err := chainutils.ConnectToChain(
		context.Background(),
		"ws://chain:8545",
		common.Hex2Bytes(FUNDED_TEST_PK))
	if err != nil {
		return types.Address{}, err
	}
	naAddress, _, _, err := NitroAdjudicator.DeployNitroAdjudicator(txSubmitter, client)
	if err != nil {
		return types.Address{}, err
	}
	return naAddress, nil
}
