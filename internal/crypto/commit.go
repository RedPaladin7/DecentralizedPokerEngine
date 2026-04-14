package crypto

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"math/big"
)

type Commitment struct {
	Hash []byte 
	Nonce []byte 
}

func NewCommitment(data []byte) (*Commitment, error) {
	nonce := make([]byte, 20)
	if _, err := rand.Read(nonce); err != nil {
		return nil, fmt.Errorf("")
	}
	hash := computeCommitmentHash(data, nonce)
	return &Commitment{Hash: hash, Nonce: nonce}, nil
}

func (c *Commitment) Verify(data []byte) error {
	expected := computeCommitmentHash(data, c.Nonce)
	if len(expected) != len(c.Hash) {
		return errors.New("")
	}
	var diff byte 
	for i := range expected {
		diff |= expected[i] ^ c.Hash[i]
	}
	if diff != 0 {
		return errors.New("")
	}
	return nil
}

func (c *Commitment) HashHex() string {
	return hex.EncodeToString(c.Hash)
}

func computeCommitmentHash(data, nonce []byte) []byte {
	h := sha256.New()

	lenData := make([]byte, 4)
	lenData[0] = byte(len(data)>>24)
	lenData[1] = byte(len(data)>>16)
	lenData[2] = byte(len(data)>>8)
	lenData[3] = byte(len(data))
	h.Write(lenData)
	h.Write(data)
	h.Write(nonce)
	return h.Sum(nil)
}

func NewDeckCommitment(deck []*big.Int) (*Commitment, error) {
	return NewCommitment(serialiseDeck(deck))
}

func (c *Commitment) VerifyDeck(deck []*big.Int) error {
	return c.Verify(serialiseDeck(deck))
}

func serialiseDeck(deck []*big.Int) []byte {
	const fieldWidth = 256 
	out := make([]byte, len(deck)*fieldWidth)
	for i, v := range deck {
		b := v.Bytes()
		offset := i * fieldWidth
		copy(out[offset+fieldWidth-len(b):offset+fieldWidth], b)
	}
	return out
}

type ShamirShare struct {
	Index int 
	Value *big.Int
}

func SplitSecret(secret *big.Int, t, n int, p *big.Int) ([]ShamirShare, error) {
	if t < 2 {
		return nil, errors.New("")
	}
	if n < t {
		return nil, fmt.Errorf("")
	}

	coeffs := make([]*big.Int, t)
	coeffs[0] = new(big.Int).Set(secret)
	for i := 1; i < t; i++ {
		r, err := rand.Int(rand.Reader, p)
		if err != nil {
			return nil, fmt.Errorf("")
		}
		coeffs[i] = r
	}

	shares := make([]ShamirShare, n)
	for x := 1; x <= n; x++ {
		xBig := big.NewInt(int64(x))
		y := new(big.Int).Set(coeffs[0])
		xPow := new(big.Int).Set(xBig)
		for i := 1; i < t; i++ {
			term := new(big.Int).Mul(coeffs[i], xPow)
			term.Mod(term, p)
			y.Add(y, term)
			y.Mod(y, p)
			xPow.Mul(xPow, xBig)
			xPow.Mod(xPow, p)
		}
		shares[x-1] = ShamirShare{Index: x, Value: y}
	}
	return shares, nil
}

func ReconstructSecret(shares []ShamirShare, p *big.Int) (*big.Int, error) {
	if len(shares) == 0 {
		return nil, errors.New("")
	}
	secret := big.NewInt(0)
	for i, si := range shares {
		xi := big.NewInt(int64(si.Index))
		num := big.NewInt(1)
		den := big.NewInt(1)
		for j, sj := range shares {
			if i == j {
				continue 
			}
			xj := big.NewInt(int64(sj.Index))
			negXj := new(big.Int).Sub(p, xj)
			num.Mul(num, negXj)
			num.Mod(num, p)

			diff := new(big.Int).Sub(xi, xj)
			diff.Mod(diff, p)
			den.Mul(den, diff)
			den.Mod(den, p)
		}

		denInv := new(big.Int).ModInverse(den, p)
		if denInv == nil {
			return nil, fmt.Errorf("")
		}
		lagrange := new(big.Int).Mul(si.Value, num)
		lagrange.Mod(lagrange, p)
		lagrange.Mul(lagrange, denInv)
		lagrange.Mod(lagrange, p)

		secret.Add(secret, lagrange)
		secret.Mod(secret, p)
	}
	return secret, nil
}