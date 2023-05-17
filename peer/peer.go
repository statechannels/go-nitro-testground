package peer

import (
	"crypto/ecdsa"
	"fmt"

	"github.com/multiformats/go-multiaddr"
	"github.com/statechannels/go-nitro-testground/config"
	p2pms "github.com/statechannels/go-nitro/client/engine/messageservice/p2p-message-service"
)

// START_PORT is the start of the port range we'll use to issue unique ports.
const START_PORT = 49000

type Role = uint

const (
	Hub Role = iota
	Payer
	Payee
	PayerPayee
)

// PeerInfo represents a peer testground instance.
// It contains information about the peers address and role that instance is playing.
type PeerInfo struct {
	p2pms.PeerInfo
	Role Role
	Seq  int64
}

// IsPayer returns true if the peer's role is a Payer or PayeePayer
func (p PeerInfo) IsPayer() bool {
	return p.Role == Payer || p.Role == PayerPayee
}

// IsPayee returns true if the peer's role is a Payee or PayeePayer
func (p PeerInfo) IsPayee() bool {
	return p.Role == Payee || p.Role == PayerPayee
}

// MultiAddress returns the multiaddress of the peer based on their port and Id
func (p PeerInfo) MultiAddress() multiaddr.Multiaddr {

	a, err := multiaddr.NewMultiaddr(fmt.Sprintf("/ip4/%s/tcp/%d/p2p/%s", p.IpAddress, p.Port, p.Id))
	if err != nil {
		panic(err)
	}

	return a
}

// MyInfo contains an instance's private information.
type MyInfo struct {
	PeerInfo
	PrivateKey ecdsa.PrivateKey
}

// GetRole determines the role an instance will play based on the run config.
func GetRole(seq int64, c config.RunConfig) Role {
	switch {
	case seq <= int64(c.NumHubs):
		return Hub

	case seq <= int64(c.NumHubs+c.NumPayers):
		return Payer

	case seq <= int64(c.NumHubs+c.NumPayers+c.NumPayees):
		return Payee

	case seq <= int64(c.NumHubs+c.NumPayers+c.NumPayees+c.NumPayeePayers):
		return PayerPayee

	default:
		panic("sequence number is larger than the amount of roles we expect")
	}
}

// FilterByRole filters a slice of PeerInfos by the given role.
// It returns a slice containing peers with the given role.
func FilterByRole(peers []PeerInfo, role Role) []PeerInfo {
	filtered := []PeerInfo{}
	for _, peer := range peers {
		if peer.Role == role {
			filtered = append(filtered, peer)
		}
	}
	return filtered
}

// GetMessageServicePeers takes in our PeerInfos and returns a slice of p2pms.PeerInfos
func GetMessageServicePeers(peers []PeerInfo) []p2pms.PeerInfo {
	peerInfos := []p2pms.PeerInfo{}
	for _, p := range peers {
		peerInfos = append(peerInfos, p.PeerInfo)
	}
	return peerInfos
}
