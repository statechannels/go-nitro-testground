package messaging

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"time"

	libp2p "github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/network"
	p2ppeer "github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/core/peerstore"
	"github.com/statechannels/go-nitro-testground/peer"
	"github.com/statechannels/go-nitro/client/engine/store/safesync"
	"github.com/statechannels/go-nitro/protocols"
	"github.com/testground/sdk-go/runtime"
)

const (
	MESSAGE_ADDRESS      = "/messages/1.0.0"
	DELIMITER            = '\n'
	BUFFER_SIZE          = 1_000_000
	NUM_CONNECT_ATTEMPTS = 20
	RETRY_SLEEP_DURATION = 5 * time.Second
)

// P2PMessageService is a rudimentary message service that uses TCP to send and receive messages
type P2PMessageService struct {
	out chan protocols.Message // for sending message to engine

	peers *safesync.Map[peer.PeerInfo]

	quit chan struct{} // quit is used to signal the goroutine to stop

	me      peer.MyInfo
	p2pHost host.Host

	metrics *runtime.MetricsApi
}

// NewTestMessageService returns a running SimpleTcpMessageService listening on the given url
func NewP2PMessageService(me peer.MyInfo, peers []peer.PeerInfo, metrics *runtime.MetricsApi) *P2PMessageService {

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

	safePeers := safesync.Map[peer.PeerInfo]{}
	for _, p := range peers {
		safePeers.Store(p.Address.String(), p)
	}
	h := &P2PMessageService{
		out:     make(chan protocols.Message, BUFFER_SIZE),
		peers:   &safePeers,
		p2pHost: host,
		quit:    make(chan struct{}),
		me:      me,
		metrics: metrics,
	}

	for _, p := range peers {
		if p.Address == h.me.Address {
			continue
		}
		// Extract the peer ID from the multiaddr.
		info, err := p2ppeer.AddrInfoFromP2pAddr(p.MultiAddress())
		h.checkError(err)

		// Add the destination's peer multiaddress in the peerstore.
		// This will be used during connection and stream creation by libp2p.
		h.p2pHost.Peerstore().AddAddrs(info.ID, info.Addrs, peerstore.PermanentAddrTTL)

	}

	h.p2pHost.SetStreamHandler(MESSAGE_ADDRESS, func(stream network.Stream) {

		select {
		case <-h.quit:
			stream.Close()
			return
		default:

			reader := bufio.NewReader(stream)
			// Create a buffer stream for non blocking read and write.
			raw, err := reader.ReadString(DELIMITER)

			// An EOF means the stream has been closed by the other side.
			if errors.Is(err, io.EOF) {
				stream.Close()
				return
			}
			h.checkError(err)
			m, err := protocols.DeserializeMessage(raw)

			h.checkError(err)
			h.out <- m
			stream.Close()
		}

	})

	return h

}

// Send sends messages to other participants
func (ms *P2PMessageService) Send(msg protocols.Message) {
	start := time.Now()
	raw, err := msg.Serialize()
	ms.checkError(err)

	peer, ok := ms.peers.Load(msg.To.String())
	if !ok {
		panic(fmt.Errorf("could not load peer %s", msg.To.String()))
	}

	for i := 0; i < NUM_CONNECT_ATTEMPTS; i++ {
		s, err := ms.p2pHost.NewStream(context.Background(), peer.Id, MESSAGE_ADDRESS)
		if err == nil {
			writer := bufio.NewWriter(s)
			_, err = writer.WriteString(raw + string(DELIMITER))
			ms.checkError(err)
			ms.recordOutgoingMessageMetrics(msg, []byte(raw))

			writer.Flush()
			s.Close()

			return
		} else {
			fmt.Printf("attempt %d: could not open stream to %s, retrying in %s\n", i, peer.Address.String(), RETRY_SLEEP_DURATION.String())
			time.Sleep(RETRY_SLEEP_DURATION)
		}
	}
	ms.metrics.Timer(fmt.Sprintf("msg_send,sender=%s", ms.me.Address)).Update(time.Since(start))

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

// recordOutgoingMessageMetrics records various metrics about an outgoing message using the metrics API
func (h *P2PMessageService) recordOutgoingMessageMetrics(msg protocols.Message, raw []byte) {
	h.metrics.Gauge(fmt.Sprintf("msg_proposal_count,sender=%s,receiver=%s", h.me.Address, msg.To)).Update(float64(len(msg.LedgerProposals)))
	h.metrics.Gauge(fmt.Sprintf("msg_payment_count,sender=%s,receiver=%s", h.me.Address, msg.To)).Update(float64(len(msg.Payments)))
	h.metrics.Gauge(fmt.Sprintf("msg_payload_count,sender=%s,receiver=%s", h.me.Address, msg.To)).Update(float64(len(msg.ObjectivePayloads)))

	totalPayloadsSize := 0
	for _, p := range msg.ObjectivePayloads {
		totalPayloadsSize += len(p.PayloadData)
	}
	h.metrics.Gauge(fmt.Sprintf("msg_payload_size,sender=%s,receiver=%s", h.me.Address, msg.To)).Update(float64(totalPayloadsSize))

	h.metrics.Gauge(fmt.Sprintf("msg_size,sender=%s,receiver=%s", h.me.Address, msg.To)).Update(float64(len(raw)))
}
