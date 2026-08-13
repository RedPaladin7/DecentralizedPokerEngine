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

func GenerateSRAKey(p *big.Int) (*SRAKey, error) {
	if p == nil || !p.ProbablyPrime(20) {
		return nil, errors.New("GenerateSRAKey: p must be a prime")
	}
	phi := new(big.Int).Sub(p, big.NewInt(1))

	one := big.NewInt(1)
	var e *big.Int

	for {
		candidate, err := rand.Int(rand.Reader, phi)
		if err != nil {
			return nil, fmt.Errorf("GenerateSRAKey: rand %w", err)
		}
		candidate.Add(candidate, big.NewInt(2))
		if candidate.Cmp(phi) >= 0 {
			continue
		}
		gcd := new(big.Int).GCD(nil, nil, candidate, phi)
		if gcd.Cmp(one) == 0 {
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

// PublicSRAKey builds a key that can encrypt but not decrypt.
// D is nil. p and e are copied.
func PublicSRAKey(p, e *big.Int) (*SRAKey, error) {
	if p == nil {
		return nil, errors.New("PublicSRAKey: p is nil")
	}
	if e == nil {
		return nil, errors.New("PublicSRAKey: e is nil")
	}
	if e.Sign() <= 0 || e.Cmp(p) >= 0 {
		return nil, errors.New("PublicSRAKey: e must be > 0 and < p")
	}
	return &SRAKey{
		E: new(big.Int).Set(e),
		D: nil,
		P: new(big.Int).Set(p),
	}, nil
}

// IsPrivate reports whether D is present (local full key).
func (k *SRAKey) IsPrivate() bool {
	return k != nil && k.D != nil
}

// PublicView returns a copy with D == nil. Safe to hand to other packages.
func (k *SRAKey) PublicView() *SRAKey {
	if k == nil {
		return nil
	}
	return cloneSRAKeyPublic(k)
}

func (k *SRAKey) Encrypt(m *big.Int) (*big.Int, error) {
	if k == nil || k.E == nil || k.P == nil {
		return nil, errors.New("SRAKey.Encrypt: key, e, or p is not present")
	}
	if err := k.validateMessage(m); err != nil {
		return nil, fmt.Errorf("")
	}
	return new(big.Int).Exp(m, k.E, k.P), nil
}

func (k *SRAKey) Decrypt(c *big.Int) (*big.Int, error) {
	if k == nil || k.D == nil {
		return nil, errors.New("SRAKey.Decrypt: private exponent d is not present")
	}
	if k.P == nil {
		return nil, errors.New("SRAKey.Decrypt: modulus p is not present")
	}
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
	if k == nil || k.E == nil {
		return nil
	}
	return new(big.Int).Set(k.E)
}

func (k *SRAKey) VerifyKeyPair() bool {
	if k == nil || k.E == nil || k.D == nil || k.P == nil {
		return false
	}
	phi := new(big.Int).Sub(k.P, big.NewInt(1))
	product := new(big.Int).Mul(k.E, k.D)
	product.Mod(product, phi)
	return product.Cmp(big.NewInt(1)) == 0
}

func cloneSRAKey(k *SRAKey) *SRAKey {
	if k == nil {
		return nil
	}
	out := &SRAKey{}
	if k.E != nil {
		out.E = new(big.Int).Set(k.E)
	}
	if k.D != nil {
		out.D = new(big.Int).Set(k.D)
	}
	if k.P != nil {
		out.P = new(big.Int).Set(k.P)
	}
	return out
}

func cloneSRAKeyPublic(k *SRAKey) *SRAKey {
	if k == nil {
		return nil
	}
	out := &SRAKey{}
	if k.E != nil {
		out.E = new(big.Int).Set(k.E)
	}
	if k.P != nil {
		out.P = new(big.Int).Set(k.P)
	}
	return out
}
