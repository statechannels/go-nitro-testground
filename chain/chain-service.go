package chain

import (
	"context"
	"encoding/hex"
	"io"
	"log"
	"math/big"

	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/statechannels/go-nitro/client/engine/chainservice"
	NitroAdjudicator "github.com/statechannels/go-nitro/client/engine/chainservice/adjudicator"
	Create2Deployer "github.com/statechannels/go-nitro/client/engine/chainservice/create2deployer"
	"github.com/testground/sdk-go/runtime"
	"github.com/testground/sdk-go/sync"
)

func NewChainService(ctx context.Context, syncClient sync.Client, runEnv *runtime.RunEnv, seq int64, logDestination io.Writer) chainservice.ChainService {
	client, err := ethclient.Dial("ws://hardhat:8545/")
	if err != nil {
		log.Fatal(err)
	}

	txSubmitter, err := bind.NewKeyedTransactorWithChainID(getFundedPrivateKey(uint(seq)), big.NewInt(1337))
	if err != nil {
		log.Fatal(err)
	}
	txSubmitter.GasLimit = uint64(30_000_000) // in units

	gasPrice, err := client.SuggestGasPrice(context.Background())
	if err != nil {
		log.Fatal(err)
	}
	txSubmitter.GasPrice = gasPrice

	deployer, err := Create2Deployer.NewCreate2Deployer(common.HexToAddress("0x5fbdb2315678afecb367f032d93f642f64180aa3"), client)
	if err != nil {
		log.Fatal(err)
	}

	hexBytecode, err := hex.DecodeString(NitroAdjudicator.NitroAdjudicatorMetaData.Bin[2:])
	if err != nil {
		log.Fatal(err)
	}

	naAddress, err := deployer.ComputeAddress(&bind.CallOpts{}, [32]byte{}, crypto.Keccak256Hash(hexBytecode))
	if err != nil {
		log.Fatal(err)
	}

	// One testground instance attempts to deploy NitroAdjudicator
	if seq == 1 {
		bytecode, err := client.CodeAt(ctx, naAddress, nil) // nil is latest block
		if err != nil {
			log.Fatal(err)
		}

		// Has NitroAdjudicator been deployed? If not, deploy it.
		if len(bytecode) == 0 {
			_, err = deployer.Deploy(txSubmitter, big.NewInt(0), [32]byte{}, hexBytecode)
			if err != nil {
				log.Fatal(err)
			}
		}
	}

	// All instances wait for until the NitroAdjudicator has been deployed.
	contractSetup := sync.State("contractSetup")
	syncClient.MustSignalEntry(ctx, contractSetup)
	syncClient.MustBarrier(ctx, contractSetup, runEnv.TestInstanceCount)

	na, err := NitroAdjudicator.NewNitroAdjudicator(naAddress, client)
	if err != nil {
		log.Fatal(err)
	}

	cs, err := chainservice.NewEthChainService(client, na, naAddress, common.Address{}, common.Address{}, txSubmitter, logDestination)
	if err != nil {
		log.Fatal(err)
	}
	return cs
}
