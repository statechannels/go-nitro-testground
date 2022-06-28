package utils

import (
	"context"
	"math/rand"
	"time"

	"github.com/ethereum/go-ethereum/crypto"
	p2pcrypto "github.com/libp2p/go-libp2p-core/crypto"
	"github.com/libp2p/go-libp2p-core/peer"
	m "github.com/statechannels/go-nitro-testground/messaging"
	"github.com/statechannels/go-nitro/types"
	"github.com/testground/sdk-go/sync"
)

// The first TCP port for the range of ports used by clients' messaging services.
const PORT_START = 7000

// getPeers will broadcast our peer info to other instances and listen for broadcasts from other instances.
// It returns a map that contains a PeerInfo for all other instances.
// The map will not contain a PeerInfo for the current instance.
func GetPeers(me m.PeerInfo, ctx context.Context, client sync.Client, instances int) map[types.Address]m.PeerInfo {

	peerTopic := sync.NewTopic("peer-info", &m.PeerInfo{})

	// Publish my entry to the topic
	_, _ = client.Publish(ctx, peerTopic, me)

	peers := map[types.Address]m.PeerInfo{}
	peerChannel := make(chan *m.PeerInfo)
	// Ready all my peers entries from the topic
	_, _ = client.Subscribe(ctx, peerTopic, peerChannel)

	for i := 0; i <= instances-1; i++ {
		t := <-peerChannel
		// We only add the peer info if it's not ours
		if t.Address != me.Address {
			peers[t.Address] = *t
		}
	}
	return peers
}

// FilterPeersByHub returns peers that where p.IsHub == shouldBeHub
func FilterPeersByHub(peers map[types.Address]m.PeerInfo, shouldBeHub bool) []m.PeerInfo {
	filteredPeers := make([]m.PeerInfo, 0)
	for _, p := range peers {
		if p.IsHub == shouldBeHub {
			filteredPeers = append(filteredPeers, p)
		}
	}
	return filteredPeers
}

func SelectRandomPeer(peers []m.PeerInfo) types.Address {

	randomIndex := rand.Intn(len(peers))

	return peers[randomIndex].Address

}

// GenerateMe generates a random  message key/ peer id and returns a PeerInfo
func GenerateMe(seq int64, isHub bool, ipAddress string) m.MyInfo {

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
	port := int64(PORT_START + seq)
	myPeerInfo := m.PeerInfo{Id: id, Address: address, IsHub: isHub, Port: port, IpAddress: ipAddress}
	return m.MyInfo{PeerInfo: myPeerInfo, PrivateKey: *privateKey, MessageKey: messageKey}
}
