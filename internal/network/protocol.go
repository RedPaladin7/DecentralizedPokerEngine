package network

import (
	"bufio"
	"context"
	"encoding/binary"
	"fmt"
	"io"

	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/core/protocol"
)

const PokerProtocolID = protocol.ID("/poker/1.0.0")

type StreamHandler func(env *Envelope, from peer.ID)

func RegisterProtocolHandler(h host.Host, handler StreamHandler) {
	h.SetStreamHandler(PokerProtocolID, func(s network.Stream){
		defer s.Close()
		remotePeer := s.Conn().RemotePeer()
		reader := bufio.NewReaderSize(s, 4096)

		for {
			lenBuf := make([]byte, 4)
			if _, err := io.ReadFull(reader, lenBuf); err != nil {
				return 
			}
			msgLen := binary.BigEndian.Uint32(lenBuf)
			if msgLen == 0 || int(msgLen) > MaxMessageSize {
				return 
			}
			msgBuf := make([]byte, msgLen)
			if _, err := io.ReadFull(reader, msgBuf); err != nil {
				return 
			}

			frame := make([]byte, 4+msgLen)
			copy(frame[:4], lenBuf)
			copy(frame[4:], msgBuf)

			env, err := DecodeEnvelope(frame, nil)
			if err != nil {
				continue 
			}
			handler(env, remotePeer)
		}
	})
}

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