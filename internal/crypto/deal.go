package crypto

import (
	"fmt"
	"math/big"

	"github.com/RedPaladin7/DecentralizedPokerEngine/internal/game"
)

type DealProtocol struct {
	Deck *EncryptedDeck
	Players []string 
	Keys []*SRAKey
	SessionID []byte
}

func NewDealProtocol(deck *EncryptedDeck, players []string, keys []*SRAKey) *DealProtocol {
	return &DealProtocol{
		Deck: deck,
		Players: players,
		Keys: keys,
		SessionID: deck.SessionID,
	}
}

func (dp *DealProtocol) RevealToPlayer(deckIndex, recipentIdx int) (game.Card, []PartialDecryption, error) {
	ciphertext, err := dp.Deck.CardAt(deckIndex)
	if err != nil {
		return game.Card{}, nil, err
	}

	current := new(big.Int).Set(ciphertext)
	var partials []PartialDecryption

	for i, pid := range dp.Players {
		if i == recipentIdx {
			continue 
		}
		partial, err := dp.applyPartialDecryption(pid, i, deckIndex, current)
		if err != nil {
			return game.Card{}, nil, err
		}
		if err := partial.Verify(dp.Deck.P, dp.SessionID); err != nil {
			return game.Card{}, nil, fmt.Errorf("")
		}
		partials = append(partials, *partial)
		current = partial.Result
	}

	recipentKey := dp.Keys[recipentIdx]
	plaintext, err := recipentKey.Decrypt(current)
	if err != nil {
		return game.Card{}, nil, fmt.Errorf("")
	}

	cardID := FieldToCard(plaintext, dp.Deck.P)
	if cardID == -1 {
		return game.Card{}, nil, fmt.Errorf("")
	}
	return game.CardFromID(cardID), partials, nil
}

func (dp *DealProtocol) RevealCommunity(deckIndex int) (game.Card, []PartialDecryption, error) {
	ciphertext, err := dp.Deck.CardAt(deckIndex)
	if err != nil {
		return game.Card{}, nil, err
	}

	current := new(big.Int).Set(ciphertext)
	var partials []PartialDecryption

	for i, pid := range dp.Players {
		partial, err := dp.applyPartialDecryption(pid, i, deckIndex, current)
		if err != nil {
			return game.Card{}, nil, err
		}
		if err := partial.Verify(dp.Deck.P, dp.SessionID); err != nil {
			return game.Card{}, nil, fmt.Errorf("")
		}
		partials = append(partials, *partial)
		current = partial.Result
	}
	cardID := FieldToCard(current, dp.Deck.P)
	if cardID == -1 {
		return game.Card{}, nil, fmt.Errorf("")
	}
	return game.CardFromID(cardID), partials, nil
}

func (dp *DealProtocol) applyPartialDecryption(playerID string, keyIdx, cardIdx int, input *big.Int) (*PartialDecryption, error) {
	key := dp.Keys[keyIdx]
	result, err := key.Decrypt(input)
	if err != nil {
		return nil, fmt.Errorf("")
	}
	proof, err := ProveDecryption(key, input, result, dp.SessionID)
	if err != nil {
		return nil, fmt.Errorf("")
	}
	return &PartialDecryption{
		PlayerID: playerID,
		CardIndex: cardIdx,
		Ciphertext: input,
		Result: result,
		Proof: proof,
	}, nil
}

func (dp *DealProtocol) DealHoleCards(dealerIdx int) ([][2]game.Card, error) {
	n := len(dp.Players)
	result := make([][2]game.Card, n)

	deckPos := 0 
	for round := 0; round < 2; round++ {
		for i := 0; i < n; i++ {
			playerIdx := (dealerIdx + 1 + i) % n 
			card, _ , err := dp.RevealToPlayer(deckPos, playerIdx)
			if err != nil {
				return nil, fmt.Errorf("")
			}
			result[playerIdx][round] = card 
			deckPos++ 
		}
	}
	return result, nil
}

func (dp *DealProtocol) DealCommunityCards(startPos int, batches []int) ([][]game.Card, error) {
	pos := startPos 
	result := make([][]game.Card, len(batches))

	for batch, count := range batches {
		pos++ 
		cards := make([]game.Card, count)
		for j := 0; j < count; j++ {
			card, _, err := dp.RevealCommunity(pos)
			if err != nil {
				return nil, fmt.Errorf("")
			}
			cards[j] = card 
			pos++ 
		}
		result[batch] = cards
	}
	return result, nil
}

func VerifyAllProofs(partials []PartialDecryption, P *big.Int, sessionID []byte) error {
	for i, pd := range partials {
		if err := pd.Verify(P, sessionID); err != nil {
			return fmt.Errorf("VerifyAllProofs[%d]: %w", i, err)
		}
	}
	return nil
}

func SubstitutePartialDecryption(pd *PartialDecryption, wrongResult *big.Int) *PartialDecryption {
	tampered := *pd 
	tampered.Result = wrongResult
	return &tampered
}