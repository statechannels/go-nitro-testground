package messaging

import (
	"crypto/ecdsa"

	p2pcrypto "github.com/libp2p/go-libp2p-core/crypto"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/statechannels/go-nitro/types"
)

type PeerInfo struct {
	Port      int64
	Id        peer.ID
	Address   types.Address
	IsHub     bool
	IpAddress string
}

type MyInfo struct {
	PeerInfo
	PrivateKey ecdsa.PrivateKey
	MessageKey p2pcrypto.PrivKey
}
