package messaging

import (
	"crypto/ecdsa"

	p2pcrypto "github.com/libp2p/go-libp2p-core/crypto"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/statechannels/go-nitro/types"
)

type Role = uint

const (
	Hub Role = iota
	Payer
	Payee
	PayerPayee
)

type PeerInfo struct {
	Port      int64
	Id        peer.ID
	Address   types.Address
	Role      Role
	IpAddress string
}

func (p PeerInfo) IsPayer() bool {
	return p.Role == Payer || p.Role == PayerPayee
}

func (p PeerInfo) IsPayee() bool {
	return p.Role == Payee || p.Role == PayerPayee
}

type MyInfo struct {
	PeerInfo
	PrivateKey ecdsa.PrivateKey
	MessageKey p2pcrypto.PrivKey
}
