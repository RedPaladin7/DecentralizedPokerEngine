package crypto

import (
	"crypto/rand"
	"errors"
	"fmt"
	"math/big"
)

type SRAKey struct {
	E *big.Int
	D *big.Int
	P *big.Int
}

func GenerateSRAKey(p *big.Int) (*SRAKey, error){
	if p == nil || !p.ProbablyPrime(20) {
		return nil, errors.New("GenerateSRAKey: p must be a prime")
	}
	phi := new(big.Int).Sub(p, big.NewInt(1))

	one := big.NewInt(1)
	var e *big.Int

	for {
		candidate, err := rand.Int(rand.Reader, phi)
		if err != nil{
			return nil, fmt.Errorf("GenerateSRAKey: rand %w", err)
		}
		candidate.Add(candidate, big.NewInt(2))
		if candidate.Cmp(phi) >= 0 {
			continue
		}
		gcd := new(big.Int).GCD(nil, nil, candidate, phi)
		if gcd.Cmp(one) == 0{
			e = candidate
			break 
		}
	}

	d := new(big.Int).ModInverse(e, phi)
	if d == nil {
		return nil, errors.New("")
	}
	return &SRAKey{E: e, D: d, P: p}, nil
}

func (k *SRAKey) Encrypt(m *big.Int) (*big.Int, error) {
	if err := k.validateMessage(m); err != nil {
		return nil, fmt.Errorf("")
	}
	return new(big.Int).Exp(m, k.E, k.P), nil
}

func (k *SRAKey) Decrypt(c *big.Int) (*big.Int, error) {
	if err := k.validateMessage(c); err != nil {
		return nil, fmt.Errorf("")
	}
	return new(big.Int).Exp(c, k.D, k.P), nil
}

func (k *SRAKey) validateMessage(m *big.Int) error {
	if m == nil {
		return errors.New("validateMessage: m cannot be nil")
	}
	one := big.NewInt(1)
	pMinus2 := new(big.Int).Sub(k.P, big.NewInt(2))
	if m.Cmp(one) < 0 || m.Cmp(pMinus2) > 0 {
		return fmt.Errorf("")
	}
	return nil
}

func (k *SRAKey) EncryptAll(cards []*big.Int) ([]*big.Int, error) {
	out := make([]*big.Int, len(cards))
	for i, c := range cards {
		enc, err := k.Encrypt(c)
		if err != nil {
			return nil, fmt.Errorf("EncryptAll[%d]: %w", i, err)
		}
		out[i] = enc
	}
	return out, nil
}

func (k *SRAKey) DecryptAll(cards []*big.Int) ([]*big.Int, error) {
	out := make([]*big.Int, len(cards))
	for i, c := range cards {
		dec, err := k.Decrypt(c)
		if err != nil {
			return nil, fmt.Errorf("DecryptAll[%d]: %w", i, err)
		}
		out[i] = dec
	}
	return out, nil
}

func (k *SRAKey) PublicKey() *big.Int {
	return new(big.Int).Set(k.E)
}

func (k *SRAKey) VerifyKeyPair() bool {
	phi := new(big.Int).Sub(k.P, big.NewInt(1))
	product := new(big.Int).Mul(k.E, k.D)
	product.Mod(product, phi)
	return product.Cmp(big.NewInt(1)) == 0
}