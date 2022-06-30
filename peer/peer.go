package peer

import (
	"crypto/ecdsa"
	"fmt"
	"math/rand"
	"time"

	"github.com/ethereum/go-ethereum/crypto"
	p2pcrypto "github.com/libp2p/go-libp2p-core/crypto"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/multiformats/go-multiaddr"
	"github.com/statechannels/go-nitro-testground/config"
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

// MultiAddress returns the multiaddress of the peer based on their port and Id
func (p PeerInfo) MultiAddress() multiaddr.Multiaddr {

	a, err := multiaddr.NewMultiaddr(fmt.Sprintf("/ip4/%s/tcp/%d/p2p/%s", p.IpAddress, p.Port, p.Id))
	if err != nil {
		panic(err)
	}

	return a
}

type MyInfo struct {
	PeerInfo
	PrivateKey ecdsa.PrivateKey
	MessageKey p2pcrypto.PrivKey
}

func getRole(seq int64, c config.RunConfig) Role {
	switch {
	case seq <= int64(c.NumHubs):
		return Hub

	case seq <= int64(c.NumHubs+c.NumPayers):
		return Payee

	case seq <= int64(c.NumHubs+c.NumPayers+c.NumPayees):
		return Payer

	case seq <= int64(c.NumHubs+c.NumPayers+c.NumPayees+c.NumPayeePayers):
		return PayerPayee

	default:
		panic("sequence number is larger than the amount of roles we expect")
	}
}

// GenerateMe generates a random  message key/ peer id and returns a PeerInfo
func GenerateMe(seq int64, c config.RunConfig, ipAddress string) MyInfo {
	role := getRole(seq, c)
	// We use the sequence in the random source so we generate a unique key even if another client is running at the same time
	messageKey, _, err := p2pcrypto.GenerateECDSAKeyPair(rand.New(rand.NewSource(time.Now().UnixNano() + seq)))
	if err != nil {
		panic(err)
	}

	id, err := peer.IDFromPrivateKey(messageKey)
	if err != nil {
		panic(err)
	}
	privateKey, err := crypto.GenerateKey()
	if err != nil {
		panic(err)
	}
	address := crypto.PubkeyToAddress(privateKey.PublicKey)
	port := int64(c.PortStart) + seq
	myPeerInfo := PeerInfo{Id: id, Address: address, Role: role, Port: port, IpAddress: ipAddress}
	return MyInfo{PeerInfo: myPeerInfo, PrivateKey: *privateKey, MessageKey: messageKey}
}

func FilterByRole(peers []PeerInfo, role Role) []PeerInfo {
	filtered := []PeerInfo{}
	for _, peer := range peers {
		if peer.Role == role {
			filtered = append(filtered, peer)
		}
	}
	return filtered
}


