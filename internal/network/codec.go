package network

// "/internal/network/codec.go"
// Translator layer between go data structures and raw bytes over network

import (
	"crypto/ed25519"
	"encoding/binary"
	"fmt"
	"math/big"
	"time"

	pokercrypto "github.com/RedPaladin7/DecentralizedPokerEngine/internal/crypto"
	"github.com/libp2p/go-libp2p/core/peer"
	"google.golang.org/protobuf/encoding/protowire"
	"google.golang.org/protobuf/proto"
)

const MaxMessageSize = 4 * 1024 * 1024 // prevent memory exhaustion attack
// framing scheme => first 4 bytes tell the length of the message (bigEndian)

func EncodeEnvelope(env *Envelope, privKey ed25519.PrivateKey) ([]byte, error) {
	sigData := envelopeSignBytes(env)
	env.Signature = ed25519.Sign(privKey, sigData) // signing contents of data with private key

	b, err := proto.Marshal(env) // struct to bytes
	if err != nil {
		return nil, fmt.Errorf("")
	}
	if len(b) > MaxMessageSize {
		return nil, fmt.Errorf("")
	}

	frame := make([]byte, 4+len(b)) // adding length of message as prefix
	binary.BigEndian.PutUint32(frame[:4], uint32(len(b)))
	copy(frame[4:], b)
	return frame, nil
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
	// reading uptill message length
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
	// signature type => type + sender + 0x00 + seq + timestamp + payload
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
		TableId:    tableID,
		HandNum:    handNum,
		PlayerId:   pd.PlayerID,
		CardIndex:  int32(pd.CardIndex),
		Ciphertext: BigIntToBytes(pd.Ciphertext),
		Result:     BigIntToBytes(pd.Result),
		Proof:      ZKProofToWire(pd.Proof),
	}
}

func PartialDecryptFromWire(w *PartialDecrypt) *pokercrypto.PartialDecryption {
	if w == nil {
		return nil
	}
	return &pokercrypto.PartialDecryption{
		PlayerID:   w.PlayerId,
		CardIndex:  int(w.CardIndex),
		Ciphertext: BytesToBigInt(w.Ciphertext),
		Result:     BytesToBigInt(w.Result),
		Proof:      ZKProofFromWire(w.Proof),
	}
}

// ShuffleMessageFromWire maps a proto ShuffleStep onto the crypto library type.
// There is no permutation field on the wire.
func ShuffleMessageFromWire(pb *ShuffleStep) *pokercrypto.ShuffleMessage {
	if pb == nil {
		return nil
	}
	var commit *pokercrypto.Commitment
	if len(pb.CommitmentHash) > 0 || len(pb.CommitmentNonce) > 0 {
		commit = &pokercrypto.Commitment{
			Hash:  append([]byte(nil), pb.CommitmentHash...),
			Nonce: append([]byte(nil), pb.CommitmentNonce...),
		}
	}
	return &pokercrypto.ShuffleMessage{
		HandNum:    pb.HandNum,
		PlayerID:   pb.PlayerId,
		OutputDeck: DeckFromWire(pb.Deck),
		Commitment: commit,
	}
}

// ShuffleMessageToWire maps a crypto ShuffleMessage onto proto ShuffleStep.
func ShuffleMessageToWire(tableID string, msg *pokercrypto.ShuffleMessage) *ShuffleStep {
	if msg == nil {
		return nil
	}
	var hash, nonce []byte
	if msg.Commitment != nil {
		hash = append([]byte(nil), msg.Commitment.Hash...)
		nonce = append([]byte(nil), msg.Commitment.Nonce...)
	}
	return &ShuffleStep{
		TableId:         tableID,
		HandNum:         msg.HandNum,
		PlayerId:        msg.PlayerID,
		Deck:            DeckToWire(msg.OutputDeck),
		CommitmentHash:  hash,
		CommitmentNonce: nonce,
	}
}

// PeelMessageFromWire preserves HandNum (PartialDecryptFromWire drops it).
func PeelMessageFromWire(pb *PartialDecrypt) *pokercrypto.PeelMessage {
	if pb == nil {
		return nil
	}
	return &pokercrypto.PeelMessage{
		HandNum:    pb.HandNum,
		PlayerID:   pb.PlayerId,
		CardIndex:  int(pb.CardIndex),
		Ciphertext: BytesToBigInt(pb.Ciphertext),
		Result:     BytesToBigInt(pb.Result),
		Proof:      ZKProofFromWire(pb.Proof),
	}
}

// PeelMessageToPD copies a PeelMessage into a PartialDecryption for existing send helpers.
func PeelMessageToPD(msg *pokercrypto.PeelMessage) *pokercrypto.PartialDecryption {
	if msg == nil {
		return nil
	}
	var proof *pokercrypto.ZKProof
	if msg.Proof != nil {
		proof = &pokercrypto.ZKProof{
			A: copyBigInt(msg.Proof.A),
			B: copyBigInt(msg.Proof.B),
			S: copyBigInt(msg.Proof.S),
			H: copyBigInt(msg.Proof.H),
		}
	}
	return &pokercrypto.PartialDecryption{
		PlayerID:   msg.PlayerID,
		CardIndex:  msg.CardIndex,
		Ciphertext: copyBigInt(msg.Ciphertext),
		Result:     copyBigInt(msg.Result),
		Proof:      proof,
	}
}

// KeyShare is a Shamir share of a player's private exponent d.
// Marshaled with proto3 wire encoding (compatible with messages.proto).
type KeyShare struct {
	TableId string
	HandNum int64
	OwnerId string
	Index   int32
	Value   []byte
}

func KeyShareToWire(tableID string, handNum int64, ownerID string, share pokercrypto.ShamirShare) *KeyShare {
	return &KeyShare{
		TableId: tableID,
		HandNum: handNum,
		OwnerId: ownerID,
		Index:   int32(share.Index),
		Value:   BigIntToBytes(share.Value),
	}
}

func KeyShareFromWire(w *KeyShare) pokercrypto.ShamirShare {
	if w == nil {
		return pokercrypto.ShamirShare{}
	}
	return pokercrypto.ShamirShare{
		Index: int(w.Index),
		Value: BytesToBigInt(w.Value),
	}
}

func MarshalKeyShare(msg *KeyShare) ([]byte, error) {
	if msg == nil {
		return nil, fmt.Errorf("MarshalKeyShare: nil")
	}
	var b []byte
	if msg.TableId != "" {
		b = protowire.AppendTag(b, 1, protowire.BytesType)
		b = protowire.AppendString(b, msg.TableId)
	}
	if msg.HandNum != 0 {
		b = protowire.AppendTag(b, 2, protowire.VarintType)
		b = protowire.AppendVarint(b, uint64(msg.HandNum))
	}
	if msg.OwnerId != "" {
		b = protowire.AppendTag(b, 3, protowire.BytesType)
		b = protowire.AppendString(b, msg.OwnerId)
	}
	if msg.Index != 0 {
		b = protowire.AppendTag(b, 4, protowire.VarintType)
		b = protowire.AppendVarint(b, uint64(msg.Index))
	}
	if len(msg.Value) > 0 {
		b = protowire.AppendTag(b, 5, protowire.BytesType)
		b = protowire.AppendBytes(b, msg.Value)
	}
	return b, nil
}

func UnmarshalKeyShare(b []byte) (*KeyShare, error) {
	msg := &KeyShare{}
	for len(b) > 0 {
		num, typ, n := protowire.ConsumeTag(b)
		if n < 0 {
			return nil, fmt.Errorf("UnmarshalKeyShare: bad tag")
		}
		b = b[n:]
		switch {
		case num == 1 && typ == protowire.BytesType:
			s, n := protowire.ConsumeString(b)
			if n < 0 {
				return nil, fmt.Errorf("UnmarshalKeyShare: table_id")
			}
			msg.TableId = s
			b = b[n:]
		case num == 2 && typ == protowire.VarintType:
			v, n := protowire.ConsumeVarint(b)
			if n < 0 {
				return nil, fmt.Errorf("UnmarshalKeyShare: hand_num")
			}
			msg.HandNum = int64(v)
			b = b[n:]
		case num == 3 && typ == protowire.BytesType:
			s, n := protowire.ConsumeString(b)
			if n < 0 {
				return nil, fmt.Errorf("UnmarshalKeyShare: owner_id")
			}
			msg.OwnerId = s
			b = b[n:]
		case num == 4 && typ == protowire.VarintType:
			v, n := protowire.ConsumeVarint(b)
			if n < 0 {
				return nil, fmt.Errorf("UnmarshalKeyShare: index")
			}
			msg.Index = int32(v)
			b = b[n:]
		case num == 5 && typ == protowire.BytesType:
			v, n := protowire.ConsumeBytes(b)
			if n < 0 {
				return nil, fmt.Errorf("UnmarshalKeyShare: value")
			}
			msg.Value = append([]byte(nil), v...)
			b = b[n:]
		default:
			n := protowire.ConsumeFieldValue(num, typ, b)
			if n < 0 {
				return nil, fmt.Errorf("UnmarshalKeyShare: skip field")
			}
			b = b[n:]
		}
	}
	return msg, nil
}

func copyBigInt(n *big.Int) *big.Int {
	if n == nil {
		return nil
	}
	return new(big.Int).Set(n)
}

func MarshalPayload(m proto.Message) ([]byte, error) {
	b, error := proto.Marshal(m)
	if error != nil {
		return nil, fmt.Errorf("")
	}
	return b, nil
}
