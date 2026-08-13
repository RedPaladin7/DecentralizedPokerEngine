package crypto

import (
	"crypto/rand"
	"errors"
	"fmt"
	"math/big"
	mathrand "math/rand"
)

type ShuffleStep struct {
	PlayerID    string
	InputDeck   []*big.Int
	OutputDeck  []*big.Int
	Permutation []int
	Commitment  *Commitment
}

type ShuffleProtocol struct {
	P         *big.Int
	SessionID []byte
	NumCards  int
}

func NewShuffleProtocol(p *big.Int, sessionID []byte) *ShuffleProtocol {
	return &ShuffleProtocol{P: p, SessionID: sessionID, NumCards: 52}
}

func (sp *ShuffleProtocol) ExecuteStep(playerID string, deck []*big.Int, key *SRAKey) (*ShuffleStep, error) {
	if key == nil || !key.IsPrivate() {
		return nil, errors.New("ExecuteStep: private exponent d is not present")
	}
	if len(deck) != sp.NumCards {
		return nil, fmt.Errorf("ExecuteStep %s: expected %d cards, got %d", playerID, sp.NumCards, len(deck))
	}

	encrypted, err := key.EncryptAll(deck)
	if err != nil {
		return nil, fmt.Errorf("ExecuteStep %s: encrypt: %w", playerID, err)
	}

	perm, err := randomPermutation(sp.NumCards)
	if err != nil {
		return nil, fmt.Errorf("ExecuteStep %s: permutation: %w", playerID, err)
	}
	permuted := make([]*big.Int, sp.NumCards)
	for i, srcIndex := range perm {
		permuted[i] = encrypted[srcIndex]
	}

	commitment, err := NewDeckCommitment(permuted)
	if err != nil {
		return nil, fmt.Errorf("ExecuteStep %s: commitment: %w", playerID, err)
	}

	return &ShuffleStep{
		PlayerID:    playerID,
		InputDeck:   copyDeck(deck),
		OutputDeck:  copyDeck(permuted),
		Permutation: copyInts(perm),
		Commitment:  commitment,
	}, nil
}

func (sp *ShuffleProtocol) VerifyStep(step *ShuffleStep) error {
	if step == nil {
		return errors.New("VerifyStep: step is nil")
	}
	if step.Commitment == nil {
		return errors.New("VerifyStep: commitment is nil")
	}
	if err := step.Commitment.VerifyDeck(step.OutputDeck); err != nil {
		return fmt.Errorf("VerifyStep: commitment: %w", err)
	}
	return nil
}

func (sp *ShuffleProtocol) RunFullShuffle(players []string, keys []*SRAKey, initialDeck []*big.Int) ([]*big.Int, []*ShuffleStep, error) {
	if len(players) != len(keys) {
		return nil, nil, fmt.Errorf("")
	}

	steps := make([]*ShuffleStep, len(players))
	current := initialDeck

	for i, pid := range players {
		step, err := sp.ExecuteStep(pid, current, keys[i])
		if err != nil {
			return nil, nil, err
		}
		if err := sp.VerifyStep(step); err != nil {
			return nil, nil, err
		}
		steps[i] = step
		current = step.OutputDeck
	}
	return current, steps, nil
}

func randomPermutation(n int) ([]int, error) {
	seedBytes := make([]byte, 8)
	if _, err := rand.Read(seedBytes); err != nil {
		return nil, err
	}
	var seed int64
	for _, b := range seedBytes {
		seed = seed<<8 | int64(b)
	}

	rng := mathrand.New(mathrand.NewSource(seed))
	perm := make([]int, n)
	for i := range perm {
		perm[i] = i
	}
	rng.Shuffle(n, func(i, j int) {
		perm[i], perm[j] = perm[j], perm[i]
	})
	return perm, nil
}

type EncryptedDeck struct {
	Cards     []*big.Int
	P         *big.Int
	SessionID []byte
}

func NewEncryptedDeck(cards []*big.Int, p *big.Int, sessionID []byte) (*EncryptedDeck, error) {
	if len(cards) != 52 {
		return nil, fmt.Errorf("NewEncryptedDeck: expected 52 cards, got %d", len(cards))
	}
	c := make([]*big.Int, 52)
	for i, v := range cards {
		c[i] = new(big.Int).Set(v)
	}
	sid := make([]byte, len(sessionID))
	copy(sid, sessionID)
	return &EncryptedDeck{Cards: c, P: p, SessionID: sid}, nil
}

func (ed *EncryptedDeck) CardAt(index int) (*big.Int, error) {
	if index < 0 || index >= len(ed.Cards) {
		return nil, fmt.Errorf("")
	}
	return new(big.Int).Set(ed.Cards[index]), nil
}

func copyDeck(deck []*big.Int) []*big.Int {
	if deck == nil {
		return nil
	}
	out := make([]*big.Int, len(deck))
	for i, v := range deck {
		if v != nil {
			out[i] = new(big.Int).Set(v)
		}
	}
	return out
}

func copyInts(a []int) []int {
	if a == nil {
		return nil
	}
	out := make([]int, len(a))
	copy(out, a)
	return out
}
