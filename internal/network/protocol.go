package network

// "internal/network/protocol.go"
// sending direct messages

import (
	"bufio"
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"sync"

	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/core/protocol"
)

// id so the receiver knows which handler to dispatch to
const PokerProtocolID = protocol.ID("/poker/1.0.0")

type StreamHandler func(env *Envelope, from peer.ID)

type StreamPool struct {
	mu sync.Mutex
	h host.Host
	streams map[peer.ID]network.Stream
}

func NewStreamPool(h host.Host) *StreamPool {
	return &StreamPool{
		h: h, streams: make(map[peer.ID]network.Stream),
	}
}

// receiving direct messages
// whenever a peer opens a /poker/1.0.0 stream to me, spawn a new go routine per incoming stream
// generic receiver, what do with the msg depends on the type and thus the handler will differ
func RegisterProtocolHandler(h host.Host, handler StreamHandler) {
	h.SetStreamHandler(PokerProtocolID, func(s network.Stream){
		defer s.Close()
		remotePeer := s.Conn().RemotePeer()
		// reading a batch of bytes at once
		reader := bufio.NewReaderSize(s, 4096)

		for {
			// tcp ensures bytes arrive in order, dropped ones are retransmitted
			lenBuf := make([]byte, 4) // first four bytes for the length
			if _, err := io.ReadFull(reader, lenBuf); err != nil {
				return 
			}
			msgLen := binary.BigEndian.Uint32(lenBuf)
			if msgLen == 0 || int(msgLen) > MaxMessageSize {
				return 
			}
			msgBuf := make([]byte, msgLen) // actual message
			if _, err := io.ReadFull(reader, msgBuf); err != nil {
				return 
			}

			// assembling the final frame
			frame := make([]byte, 4+msgLen)
			copy(frame[:4], lenBuf)
			copy(frame[4:], msgBuf)

			// decoding the envelope
			env, err := DecodeEnvelope(frame, nil)
			if err != nil {
				continue 
			}
			// handler function: what to actually do with the received message
			handler(env, remotePeer)
		}
	})
}

// new stream opened and closed for every message
// but streams are multiplexed over one tcp connection so no new tcp handshake happens
func SendDirect(ctx context.Context, h host.Host, peerID peer.ID, frame []byte) error {
	s, err := h.NewStream(ctx, peerID, PokerProtocolID)
	if err != nil {
		return fmt.Errorf("")
	}
	defer s.Close()

	if _, err := s.Write(frame); err != nil {
		return fmt.Errorf("")
	}
	return nil
}

func (sp *StreamPool) Send(ctx context.Context, peerID peer.ID, frame []byte) error {
	sp.mu.Lock()
	s, ok := sp.streams[peerID]
	sp.mu.Unlock()

	if !ok {
		var err error 
		s, err = sp.h.NewStream(ctx, peerID, PokerProtocolID)
		if err != nil {
			return fmt.Errorf("")
		}
		sp.mu.Lock()
		sp.streams[peerID] = s
		sp.mu.Unlock()
	}

	if _, err := s.Write(frame); err != nil {
		sp.mu.Lock()
		delete(sp.streams, peerID)
		sp.mu.Unlock()
		return fmt.Errorf("")
	}
	return nil
}

func (sp *StreamPool) CloseAll() {
	sp.mu.Lock()
	defer sp.mu.Unlock()
	for _, s := range sp.streams {
		s.Close()
	}
	sp.streams = make(map[peer.ID]network.Stream)
}