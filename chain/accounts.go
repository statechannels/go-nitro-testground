package chain

import (
	"crypto/ecdsa"
	"fmt"

	hdwallet "github.com/miguelmota/go-ethereum-hdwallet"
)

// getFundedPrivateKeys returns a collection of private keys that are funded on the hardhat network
func getFundedPrivateKeys() []*ecdsa.PrivateKey {
	// See https://hardhat.org/hardhat-network/docs/reference#accounts for defaults
	// This is the default mnemonic used by hardhat
	const HARDHAT_MNEMONIC = "test test test test test test test test test test test junk"
	// We manually set the amount of funded accounts in our hardhat config
	// If that value changes, this value must change as well
	const NUM_FUNDED = 1000
	// This is the default hd wallet path used by hardhat
	const HD_PATH = "m/44'/60'/0'/0"

	wallet, err := hdwallet.NewFromMnemonic(HARDHAT_MNEMONIC)
	if err != nil {
		panic(err)
	}

	pks := make([]*ecdsa.PrivateKey, NUM_FUNDED)
	for i := 0; i < NUM_FUNDED; i++ {
		// Construct the full hd path for the account at index i
		p := fmt.Sprintf("%s/%d", HD_PATH, i)

		a, err := wallet.Derive(hdwallet.MustParseDerivationPath(p), true)
		if err != nil {
			panic(err)
		}

		pk, err := wallet.PrivateKey(a)
		if err != nil {
			panic(err)
		}

		pks[i] = pk

	}
	return pks
}
