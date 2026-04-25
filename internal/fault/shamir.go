package fault

import (
	"fmt"
	"math/big"
	"sync"

	pokercrypto "github.com/RedPaladin7/DecentralizedPokerEngine/internal/crypto"
)

type KeyShareStore struct {
	mu sync.RWMutex 
	sharesReceived map[string]pokercrypto.ShamirShare
	sharesHeld map[string][]pokercrypto.ShamirShare
	prime *big.Int
}

func NewKeyShareSTore(prime *big.Int) *KeyShareStore {
	return &KeyShareStore{
		prime: prime,
		sharesReceived: make(map[string]pokercrypto.ShamirShare),
		sharesHeld: make(map[string][]pokercrypto.ShamirShare),
	}
}

func (ks *KeyShareStore) StoreMyShare(ownerID string, share pokercrypto.ShamirShare) {
	ks.mu.RLock()
	defer ks.mu.RUnlock()
	ks.sharesReceived[ownerID] = share
}

func (ks *KeyShareStore) ContributeShare(ownerID string) (pokercrypto.ShamirShare, bool) {
	ks.mu.RLock()
	defer ks.mu.RUnlock()
	share, ok := ks.sharesReceived[ownerID]
	return share, ok
}

func (ks *KeyShareStore) AddReconstructShare(ownerID string, share pokercrypto.ShamirShare) {
	ks.mu.Lock()
	defer ks.mu.Unlock()
	for _, existing := range ks.sharesHeld[ownerID] {
		if existing.Index == share.Index {
			return 
		}
	}
	ks.sharesHeld[ownerID] = append(ks.sharesHeld[ownerID], share)
}

func (ks *KeyShareStore) CanReconstruct(ownerID string, threshold int) bool {
	ks.mu.RLock()
	defer ks.mu.RUnlock()
	return len(ks.sharesHeld[ownerID]) >= threshold
}

func (ks *KeyShareStore) Reconstruct(ownerID string, threshold int) (*big.Int, error) {
	ks.mu.RLock()
	shares := make([]pokercrypto.ShamirShare, len(ks.sharesHeld[ownerID]))
	copy(shares, ks.sharesHeld[ownerID])
	ks.mu.RUnlock()
	
	if len(shares) < threshold {
		return nil, fmt.Errorf("")
	}

	key, err := pokercrypto.ReconstructSecret(shares[:threshold], ks.prime)
	if err != nil {
		return nil, fmt.Errorf("")
	}
	return key, nil
}

func (ks *KeyShareStore) ReconstrusctSRAKey(ownerID string, threshold int) (*pokercrypto.SRAKey, error) {
	d, err := ks.Reconstruct(ownerID, threshold)
	if err != nil {
		return nil, err
	}

	phi := new(big.Int).Sub(ks.prime, big.NewInt(1))
	e := new(big.Int).ModInverse(d, phi)
	if e == nil {
		return nil, fmt.Errorf("")
	}
	return &pokercrypto.SRAKey{E: e, D: d, P: ks.prime}, nil
}

func SplitAndContribute(ownerKey *pokercrypto.SRAKey, numPlayers int) ([]pokercrypto.ShamirShare, int, error) {
	if numPlayers < 2 {
		return nil, 0, fmt.Errorf("")
	}
	threshold := (numPlayers + 1) / 2 
	if threshold < 2 {
		threshold = 2
	}
	share, err := pokercrypto.SplitSecret(ownerKey.D, threshold, numPlayers, ownerKey.P)
	if err != nil {
		return nil, 0, fmt.Errorf("")
	}
	return share, threshold, nil
}