package chain

import (
	"crypto/ecdsa"
	"fmt"

	hdwallet "github.com/miguelmota/go-ethereum-hdwallet"
)

// getFundedPrivateKey returns a funded private key for a given sequence number
// It will always return the same private key for a given sequence number
func getFundedPrivateKey(seq uint) *ecdsa.PrivateKey {
	// See https://hardhat.org/hardhat-network/docs/reference#accounts for defaults
	// This is the default mnemonic used by hardhat
	const HARDHAT_MNEMONIC = "test test test test test test test test test test test junk"
	// We manually set the amount of funded accounts in our hardhat config
	// If that value changes, this value must change as well
	const NUM_FUNDED = 1000
	// This is the default hd wallet path used by hardhat
	const HD_PATH = "m/44'/60'/0'/0"

	ourIndex := seq - 1 // seq starts at 1

	if NUM_FUNDED < seq {
		panic(fmt.Errorf("only the first %d accounts are funded", NUM_FUNDED))
	}

	wallet, err := hdwallet.NewFromMnemonic(HARDHAT_MNEMONIC)

	ourPath := fmt.Sprintf("%s/%d", HD_PATH, ourIndex)
	if err != nil {
		panic(err)
	}

	a, err := wallet.Derive(hdwallet.MustParseDerivationPath(ourPath), false)
	if err != nil {
		panic(err)
	}
	pk, err := wallet.PrivateKey(a)
	if err != nil {
		panic(err)
	}
	return pk

}
