package network

import (
	"crypto/ed25519"
	"encoding/binary"
	"fmt"
	"math/big"
	"time"

	pokercrypto "github.com/RedPaladin7/DecentralizedPokerEngine/internal/crypto"
	"github.com/libp2p/go-libp2p/core/peer"
	"google.golang.org/protobuf/proto"
)

const MaxMessageSize = 4 * 1024 * 1024

func EncodeEnvelope(env *Envelope, privKey ed25519.PrivateKey) ([]byte, error) {
	sigData := envelopeSignBytes(env)
	env.Signature = ed25519.Sign(privKey, sigData)

	b, err := proto.Marshal(env)
	if err != nil {
		return nil, fmt.Errorf("")
	}
	if len(b) > MaxMessageSize {
		return nil, fmt.Errorf("")
	}

	frame := make([]byte, 4+len(b))
	binary.BigEndian.PutUint32(frame[:4], uint32(len(b)))
	copy(frame[4:], b)
	return frame,  nil
}

func DecodeEnvelope(frame []byte, pubKeyFn func(peerID string) (ed25519.PublicKey, error)) (*Envelope, error) {
	if len(frame) < 4 {
		return nil, fmt.Errorf("")
	}
	msgLen := binary.BigEndian.Uint32(frame[:4])
	if int(msgLen) > MaxMessageSize {
		return nil, fmt.Errorf("")
	}
	if int(msgLen) > len(frame)-4 {
		return nil, fmt.Errorf("")
	}
	env := &Envelope{}
	if err := proto.Unmarshal(frame[4:4+msgLen], env); err != nil {
		return nil, fmt.Errorf("")
	}

	if pubKeyFn != nil && len(env.Signature) > 0 {
		pub, err := pubKeyFn(env.SenderId)
		if err != nil {
			return nil, fmt.Errorf("")
		}
		if pub != nil {
			sigData := envelopeSignBytes(env)
			if !ed25519.Verify(pub, sigData, env.Signature) {
				return nil, fmt.Errorf("")
			}
		}
	}
	return env, nil
}

func envelopeSignBytes(env *Envelope) []byte {
	buf := make([]byte, 0, 1+len(env.SenderId)+1+8+8+len(env.Payload))
	buf = append(buf, byte(env.Type)) 
	buf = append(buf, []byte(env.SenderId)...)
	buf = append(buf, 0x00)
	var seq [8]byte 
	binary.BigEndian.PutUint64(seq[:], uint64(env.Seq))
	buf = append(buf, seq[:]...)
	var ts [8]byte 
	binary.BigEndian.PutUint64(ts[:], uint64(env.Timestamp))
	buf = append(buf, ts[:]...)
	buf = append(buf, env.Payload...)
	return buf
}

func NewEnvelope(msgType MsgType, senderID string, seq int64, payload []byte) *Envelope {
	return &Envelope{
		Type:      msgType,
		SenderId:  senderID,
		Seq:       seq,
		Timestamp: time.Now().UnixMilli(),
		Payload:   payload,
	}
}

func BigIntToBytes(n *big.Int) []byte {
	if n == nil {
		return nil
	}
	return n.Bytes()
}

func BytesToBigInt(b []byte) *big.Int {
	if b == nil {
		return nil
	}
	return new(big.Int).SetBytes(b)
}

func ZKProofToWire(p *pokercrypto.ZKProof) *ZKProofWire {
	if p == nil {
		return nil
	}
	return &ZKProofWire{
		A: BigIntToBytes(p.A),
		B: BigIntToBytes(p.B),
		S: BigIntToBytes(p.S),
		H: BigIntToBytes(p.H),
	}
}

func ZKProofFromWire(p *ZKProofWire) *pokercrypto.ZKProof {
	if p == nil {
		return nil
	}
	return &pokercrypto.ZKProof{
		A: BytesToBigInt(p.A),
		B: BytesToBigInt(p.B),
		S: BytesToBigInt(p.S),
		H: BytesToBigInt(p.H),
	}
}

func DeckToWire(deck []*big.Int) [][]byte {
	out := make([][]byte, len(deck))
	for i, v := range deck {
		out[i] = BigIntToBytes(v)
	}
	return out
}

func DeckFromWire(wire [][]byte) []*big.Int {
	out := make([]*big.Int, len(wire))
	for i, b := range wire {
		out[i] = BytesToBigInt(b)
	}
	return out
}

func PeerIDFromString(s string) (peer.ID, error) {
	pid, err := peer.Decode(s)
	if err != nil {
		return "", fmt.Errorf("")
	}
	return pid, nil
} 

func ExtractEd25519PubKey(pid peer.ID) (ed25519.PublicKey, error) {
	pubKey, err := pid.ExtractPublicKey()
	if err != nil {
		return nil, fmt.Errorf("")
	}
	raw, err := pubKey.Raw()
	if err != nil {
		return nil, fmt.Errorf("")
	}
	return ed25519.PublicKey(raw), nil
}

func PartialDecryptToWire(tableID string, handNum int64, pd *pokercrypto.PartialDecryption) *PartialDecrypt {
	return &PartialDecrypt{
		TableId:     tableID,
		HandNum:     handNum,
		PlayerId:    pd.PlayerID,
		CardIndex: int32(pd.CardIndex),
		Ciphertext: BigIntToBytes(pd.Ciphertext),
		Result: BigIntToBytes(pd.Result),
		Proof: ZKProofToWire(pd.Proof),
	}
}

func PartialDecryptFromWire(w *PartialDecrypt) *pokercrypto.PartialDecryption {
	return &pokercrypto.PartialDecryption{
		PlayerID: w.PlayerId,
		CardIndex: int(w.CardIndex),
		Ciphertext: BytesToBigInt(w.Ciphertext),
		Result: BytesToBigInt(w.Result),
		Proof: ZKProofFromWire(w.Proof),
	}
}

func MarshalPayload(m proto.Message) ([]byte, error) {
	b, error := proto.Marshal(m)
	if error != nil {
		return nil, fmt.Errorf("")
	}
	return b, nil
}