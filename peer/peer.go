package peer

import (
	"context"
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
	"github.com/testground/sdk-go/sync"
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

func getRole(seq int64, c config.RolesConfig) Role {
	switch {
	case seq <= int64(c.AmountHubs):
		return Hub

	case seq <= int64(c.AmountHubs+c.Payers):
		return Payee

	case seq <= int64(c.AmountHubs+c.Payers+c.Payees):
		return Payer

	case seq <= int64(c.AmountHubs+c.Payers+c.Payees+c.PayeePayeers):
		return PayerPayee

	default:
		panic("sequence number is larger than the amount of roles we expect")
	}
}

// GenerateMe generates a random  message key/ peer id and returns a PeerInfo
func GenerateMe(seq int64, c config.RolesConfig, ipAddress string) MyInfo {
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
func SelectRandomPeer(peers []PeerInfo) PeerInfo {

	randomIndex := rand.Intn(len(peers))

	return peers[randomIndex]

}

// getPeers will broadcast our peer info to other instances and listen for broadcasts from other instances.
// It returns a map that contains a PeerInfo for all other instances.
// The map will not contain a PeerInfo for the current instance.
func GetPeers(me PeerInfo, ctx context.Context, client sync.Client, instances int) []PeerInfo {

	peerTopic := sync.NewTopic("peer-info", PeerInfo{})

	// Publish my entry to the topic
	_, _ = client.Publish(ctx, peerTopic, me)

	peers := []PeerInfo{}
	peerChannel := make(chan *PeerInfo)
	// Ready all my peers entries from the topic
	_, _ = client.Subscribe(ctx, peerTopic, peerChannel)

	for i := 0; i <= instances-1; i++ {
		t := <-peerChannel
		// We only add the peer info if it's not ours
		if t.Address != me.Address {
			peers = append(peers, *t)
		}
	}
	return peers
}