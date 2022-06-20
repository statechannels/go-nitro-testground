package main

import (
	"bufio"
	"context"
	"crypto/ecdsa"
	"errors"
	"fmt"
	"io"

	"github.com/libp2p/go-libp2p"
	p2pcrypto "github.com/libp2p/go-libp2p-core/crypto"
	"github.com/libp2p/go-libp2p-core/host"
	"github.com/libp2p/go-libp2p-core/network"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/libp2p/go-libp2p-core/peerstore"
	"github.com/multiformats/go-multiaddr"
	"github.com/statechannels/go-nitro/client/engine/store/safesync"
	"github.com/statechannels/go-nitro/protocols"
	"github.com/statechannels/go-nitro/types"
	"github.com/testground/sdk-go/runtime"
)

const (
	MESSAGE_ADDRESS = "/messages/1.0.0"
	DELIMETER       = '\n'
	BUFFER_SIZE     = 1_000_000
)

// P2PMessageService is a rudimentary message service that uses TCP to send and receive messages
type P2PMessageService struct {
	out   chan protocols.Message // for sending message to engine
	in    chan protocols.Message // for receiving messages from engine
	peers safesync.Map[PeerInfo]

	quit chan struct{} // quit is used to signal the goroutine to stop

	me      MyInfo
	p2pHost host.Host

	metrics *runtime.MetricsApi
}

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

// MultiAddress returns the multiaddress of the peer based on their port and Id
func (p PeerInfo) MultiAddress() multiaddr.Multiaddr {

	a, err := multiaddr.NewMultiaddr(fmt.Sprintf("/ip4/%s/tcp/%d/p2p/%s", p.IpAddress, p.Port, p.Id))
	if err != nil {
		panic(err)
	}

	return a
}

// NewTestMessageService returns a running SimpleTcpMessageService listening on the given url
func NewP2PMessageService(me MyInfo, peers map[types.Address]PeerInfo, metrics *runtime.MetricsApi) *P2PMessageService {

	options := []libp2p.Option{libp2p.Identity(me.MessageKey),
		libp2p.ListenAddrStrings(fmt.Sprintf("/ip4/%s/tcp/%d", me.IpAddress, me.Port)),
		libp2p.DefaultTransports,
		libp2p.NoSecurity,
		libp2p.DefaultMuxers,
	}
	host, err := libp2p.New(options...)
	if err != nil {

		panic(err)
	}

	safePeers := safesync.Map[PeerInfo]{}
	for _, p := range peers {
		safePeers.Store(p.Address.String(), p)
	}
	h := &P2PMessageService{
		in:      make(chan protocols.Message, BUFFER_SIZE),
		out:     make(chan protocols.Message, BUFFER_SIZE),
		peers:   safePeers,
		p2pHost: host,
		quit:    make(chan struct{}),
		me:      me,
		metrics: metrics,
	}
	h.p2pHost.SetStreamHandler(MESSAGE_ADDRESS, func(stream network.Stream) {

		reader := bufio.NewReader(stream)
		for {
			select {
			case <-h.quit:
				stream.Close()
				return
			default:

				// Create a buffer stream for non blocking read and write.
				raw, err := reader.ReadString(DELIMETER)
				// TODO: If the stream has been closed we just bail for now
				// TODO: Properly check for and handle stream reset error
				if errors.Is(err, io.EOF) || fmt.Sprintf("%s", err) == "stream reset" {
					stream.Close()
					return
				}
				h.checkError(err)
				m, err := protocols.DeserializeMessage(raw)
				h.checkError(err)
				h.out <- m
			}
		}
	})

	return h

}

// DialPeers dials all peers in the peer list and establishs a connection with them.
// This should be called once all the message services are running.
// TODO: The message service should handle this internally
func (s *P2PMessageService) DialPeers() {
	go s.connectToPeers()
}

// connectToPeers establishes a stream with all our peers and uses that stream to send messages
func (s *P2PMessageService) connectToPeers() {
	// create a map with streams to all peers
	peerStreams := make(map[types.Address]network.Stream)
	s.peers.Range(func(key string, p PeerInfo) bool {

		if p.Address == s.me.Address {
			return false
		}
		// Extract the peer ID from the multiaddr.
		info, err := peer.AddrInfoFromP2pAddr(p.MultiAddress())
		s.checkError(err)

		// Add the destination's peer multiaddress in the peerstore.
		// This will be used during connection and stream creation by libp2p.
		s.p2pHost.Peerstore().AddAddrs(info.ID, info.Addrs, peerstore.PermanentAddrTTL)

		stream, err := s.p2pHost.NewStream(context.Background(), info.ID, MESSAGE_ADDRESS)
		s.checkError(err)
		peerStreams[p.Address] = stream
		return true
	})
	for {
		select {
		case <-s.quit:

			for _, writer := range peerStreams {
				writer.Close()
			}
			return
		case m := <-s.in:
			raw, err := m.Serialize()
			s.checkError(err)
			s.recordOutgoingStats(m)
			writer := bufio.NewWriter(peerStreams[m.To])
			_, err = writer.WriteString(raw)
			s.checkError(err)
			err = writer.WriteByte(DELIMETER)
			s.checkError(err)
			writer.Flush()

		}
	}

}

// Send dispatches messages
func (s *P2PMessageService) Send(msg protocols.Message) {
	// TODO: Now that the in chan has been deprecated from the API we should remove in from this message serviceß
	s.in <- msg
}

// checkError panics if the SimpleTCPMessageService is running, otherwise it just returns
func (s *P2PMessageService) checkError(err error) {
	if err == nil {
		return
	}
	select {

	case <-s.quit: // If we are quitting we can ignore the error
		return
	default:
		panic(err)
	}
}

func (s *P2PMessageService) Out() <-chan protocols.Message {
	return s.out
}

// Close closes the P2PMessageService
func (s *P2PMessageService) Close() {

	close(s.quit)
	s.p2pHost.Close()

}

func (s *P2PMessageService) recordOutgoingStats(m protocols.Message) {
	proposalCount := len(m.SignedProposals())
	if proposalCount > 0 {

		first := m.SignedProposals()[0]
		point := fmt.Sprintf("proposal-count,sender=%s,receiver=%s,ledger=%s", s.me.Address, m.To, first.Payload.Proposal.LedgerID)
		s.metrics.RecordPoint(point, float64(proposalCount))

	}
}
