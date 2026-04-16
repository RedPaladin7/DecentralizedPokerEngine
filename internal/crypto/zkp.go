package crypto

import (
	"crypto/rand"
	"crypto/sha256"
	"errors"
	"fmt"
	"math/big"
)

type ZKProof struct {
	A *big.Int
	B *big.Int
	S *big.Int
	H *big.Int
}

var g = big.NewInt(2)

func ProveDecryption(key *SRAKey, ciphertext, result *big.Int, sessionID []byte) (*ZKProof, error) {
	P := key.P
	phi := new(big.Int).Sub(P, big.NewInt(1))
	h := new(big.Int).Exp(g, key.D, P)

	r, err := rand.Int(rand.Reader, phi)
	if err != nil {
		return nil, fmt.Errorf("")
	}
	if r.Sign() == 0{
		r.SetInt64(1)
	}

	A := new(big.Int).Exp(g, r, P)
	B := new(big.Int).Exp(ciphertext, r, P)
	c := computeChallenge(P, h, ciphertext, result, A, B, sessionID)

	s := new(big.Int).Mul(c, key.D)
	s.Add(s, r)
	s.Mod(s, phi)

	return &ZKProof{A: A, B: B, S: s, H: h}, nil
}

func VerifyDecryption(proof *ZKProof, ciphertext, result *big.Int, P *big.Int, sessionID []byte) error {
	if proof == nil {
		return errors.New("VerifyDecryption: proof is nil")
	}
	if proof.A == nil || proof.B == nil || proof.S == nil || proof.H == nil {
		return errors.New("VerifyDecryption: proof has nil fields")
	}

	c := computeChallenge(P, proof.H, ciphertext, result, proof.A, proof.B, sessionID)

	lhs1 := new(big.Int).Exp(g, proof.S, P)
	hc := new(big.Int).Exp(proof.H, c, P)
	rhs1 := new(big.Int).Mul(hc, proof.A)
	rhs1.Mod(rhs1, P)
	if lhs1.Cmp(rhs1) != 0 {
		return fmt.Errorf("")
	}

	lhs2 := new(big.Int).Exp(ciphertext, proof.S, P)
	resultC := new(big.Int).Exp(result, c, P)
	rhs2 := new(big.Int).Mul(resultC, proof.B)
	rhs2.Mod(rhs2, P)
	if lhs2.Cmp(rhs2) != 0 {
		return fmt.Errorf("")
	}
	return nil
}

func computeChallenge(P, h, ciphertext, result, A, B *big.Int, sessionID []byte) *big.Int {
	hash := sha256.New()
	for _, v := range []*big.Int{P, h, ciphertext, result, A, B} {
		b := v.Bytes()
		length := make([]byte, 4)
		length[0] = byte(len(b)>>24)
		length[1] = byte(len(b)>>16)
		length[2] = byte(len(b)>>8)
		length[3] = byte(len(b))
		hash.Write(length)
		hash.Write(b)
	}
	hash.Write(sessionID)
	digest := hash.Sum(nil)

	c := new(big.Int).SetBytes(digest)
	phi := new(big.Int).Sub(P, big.NewInt(1))
	c.Mod(c, phi)
	return c
}

type PartialDecryption struct {
	PlayerID string 
	CardIndex int 
	Ciphertext *big.Int
	Result *big.Int
	Proof *ZKProof
}

func (pd *PartialDecryption) Verify(P *big.Int, sessionID []byte) error {
	if err := VerifyDecryption(pd.Proof, pd.Ciphertext, pd.Result, P, sessionID); err != nil {
		return fmt.Errorf("")
	}
	return nil
}