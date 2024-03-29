package chain

import (
	"fmt"

	hdwallet "github.com/miguelmota/go-ethereum-hdwallet"
)

func GetHyperspaceFundedPrivateKey(seq uint) string {
	// This is an arbitrary mnemonic
	const HYPERSPACE_MNEMONIC = "army forest resource shop tray cluster teach cause spice judge link oppose"
	// This is the amount of funded accounts we can expect
	const NUM_FUNDED = 25
	// This is the HD path the glif wallet uses
	const HD_PATH = "m/44'/1'/0'/0"

	return getPrivateKey(seq, HYPERSPACE_MNEMONIC, HD_PATH, NUM_FUNDED)
}

// GetFundedPrivateKey returns a funded private key for a given sequence number
// It will always return the same private key for a given sequence number
func GetHardhatFundedPrivateKey(seq uint) string {
	// See https://hardhat.org/hardhat-network/docs/reference#accounts for defaults
	// This is the default mnemonic used by hardhat
	const HARDHAT_MNEMONIC = "test test test test test test test test test test test junk"
	// We manually set the amount of funded accounts in our hardhat config
	// If that value changes, this value must change as well
	const NUM_FUNDED = 1000
	// This is the default hd wallet path used by hardhat
	const HD_PATH = "m/44'/60'/0'/0"
	return getPrivateKey(seq, HARDHAT_MNEMONIC, HD_PATH, NUM_FUNDED)
}

func getPrivateKey(seq uint, mnemonic string, path string, numFunded uint) string {

	ourIndex := seq - 1 // seq starts at 1

	if numFunded < seq {
		panic(fmt.Errorf("only the first %d accounts are funded", numFunded))
	}

	wallet, err := hdwallet.NewFromMnemonic(mnemonic)

	ourPath := fmt.Sprintf("%s/%d", path, ourIndex)
	if err != nil {
		panic(err)
	}

	a, err := wallet.Derive(hdwallet.MustParseDerivationPath(ourPath), false)
	if err != nil {
		panic(err)
	}

	pk, err := wallet.PrivateKeyHex(a)
	if err != nil {
		panic(err)
	}
	return pk
}
