package chain

import (
	"context"
	"fmt"
	"log"
	"math/big"

	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/statechannels/go-nitro/client/engine/chainservice"
	NitroAdjudicator "github.com/statechannels/go-nitro/client/engine/chainservice/adjudicator"
)

type logger struct{}

func (l logger) Write(p []byte) (n int, err error) {
	return fmt.Printf("%s", p)
}

func NewChainService(seq int64) chainservice.ChainService {
	client, err := ethclient.Dial("ws://hardhat:8545/")
	if err != nil {
		log.Fatal(err)
	}

	// TODO: do not hardcode the NitroAdjudicator address. Instead, find address on chain
	naAddress := common.HexToAddress("0x5fbdb2315678afecb367f032d93f642f64180aa3")
	na, err := NitroAdjudicator.NewNitroAdjudicator(naAddress, client)
	if err != nil {
		log.Fatal(err)
	}

	// todo: check that index is within range
	pk, err := crypto.HexToECDSA(pks[seq])
	if err != nil {
		log.Fatal(err)
	}
	txSubmitter, err := bind.NewKeyedTransactorWithChainID(pk, big.NewInt(1337))
	if err != nil {
		log.Fatal(err)
	}
	txSubmitter.GasLimit = uint64(300000) // in units

	gasPrice, err := client.SuggestGasPrice(context.Background())
	if err != nil {
		log.Fatal(err)
	}
	txSubmitter.GasPrice = gasPrice

	cs, err := chainservice.NewEthChainService(client, na, naAddress, common.Address{}, txSubmitter, logger{})
	if err != nil {
		log.Fatal(err)
	}
	return cs
}