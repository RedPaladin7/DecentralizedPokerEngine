package crypto

import (
	"fmt"
	"math/big"

	"github.com/RedPaladin7/DecentralizedPokerEngine/internal/game"
)

type CryptoGame struct {
	P *big.Int
	SessionID []byte 
	Players []string 
	Keys []*SRAKey
	Deck *EncryptedDeck
	ShuffleLog []*ShuffleStep
	DealLog []PartialDecryption
}

func NewCryptoGame(playersIDs []string, nonce []byte) (*CryptoGame, error) {
	if len(playersIDs) < 2 {
		return nil, fmt.Errorf("")
	}
	p := SharedPrime()
	sid := SessionID(playersIDs, nonce)

	keys := make([]*SRAKey, len(playersIDs))
	for i, pid := range playersIDs {
		k, err := GenerateSRAKey(p)
		if err != nil {
			return nil, fmt.Errorf("NewCryptoGame: key gen for %s: %w", pid, err)
		}
		keys[i] = k 
	}

	return &CryptoGame{
		P: p,
		SessionID: sid,
		Players: playersIDs,
		Keys: keys,
	}, nil
}

func (cg *CryptoGame) RunShuffle() error {
	sp := NewShuffleProtocol(cg.P, cg.SessionID)
	initialDeck := BuildPlaintextDeck(cg.P)
	
	finalDeck, steps, err := sp.RunFullShuffle(cg.Players, cg.Keys, initialDeck)
	if err != nil {
		return fmt.Errorf("")
	}
	cg.ShuffleLog = steps 
	cg.Deck, err = NewEncryptedDeck(finalDeck, cg.P, cg.SessionID)
	if err != nil {
		return fmt.Errorf("")
	}
	return nil
}

func (cg *CryptoGame) DealToEngine(gs *game.GameState) error {
	if cg.Deck == nil {
		return fmt.Errorf("")
	}
	dp := NewDealProtocol(cg.Deck, cg.Players, cg.Keys)

	holeCards, err := dp.DealHoleCards(gs.DealerIdx)
	if err != nil {
		return fmt.Errorf("")
	}

	for _, p := range gs.Players {
		pIdx := cg.playerIndex(p.ID)
		if pIdx == -1 {
			return fmt.Errorf("")
		}
		p.HoleCards = holeCards[pIdx]
	}

	gs.Deck = nil 
	return nil
}

func (cg *CryptoGame) DealFlop(startPos int) ([]game.Card, error) {
	return cg.dealBatch(startPos, 3)
}

func (cg *CryptoGame) DealTurn(startPos int) ([]game.Card, error) {
	return cg.dealBatch(startPos, 1)
}

func (cg *CryptoGame) DealRiver(startPos int) ([]game.Card, error) {
	return cg.dealBatch(startPos, 1)
}

func (cg *CryptoGame) playerIndex(playerID string) int {
	for i, pid := range cg.Players {
		if pid == playerID {
			return i
		}
	}
	return -1
}

func (cg *CryptoGame) dealBatch(startPos, count int) ([]game.Card, error) {
	dp := NewDealProtocol(cg.Deck, cg.Players, cg.Keys)
	pos := startPos + 1 

	cards := make([]game.Card, count)
	for i := 0; i < count; i++ {
		card, partials, err := dp.RevealCommunity(pos)
		if err != nil {
			return nil, fmt.Errorf("")
		}
		cg.DealLog = append(cg.DealLog, partials...)
		cards[i] = card 
		pos++ 
	}
	return cards, nil
}

func (cg *CryptoGame) HolecardStartPos() int {
	return len(cg.Players) * 2 
}

func (cg *CryptoGame) VerifyFullLog() error {
	return VerifyAllProofs(cg.DealLog, cg.P, cg.SessionID)
}