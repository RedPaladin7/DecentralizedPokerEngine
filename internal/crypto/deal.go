package crypto

import (
	"errors"
	"fmt"
	"math/big"

	"github.com/RedPaladin7/DecentralizedPokerEngine/internal/game"
)

type DealProtocol struct {
	Deck      *EncryptedDeck
	Players   []string
	Keys      []*SRAKey
	SessionID []byte
}

func NewDealProtocol(deck *EncryptedDeck, players []string, keys []*SRAKey) *DealProtocol {
	return &DealProtocol{
		Deck:      deck,
		Players:   players,
		Keys:      keys,
		SessionID: deck.SessionID,
	}
}

// Peel decrypts one layer with the local private key and attaches a ZK proof.
// Ciphertext / Result on the returned value are copies.
func Peel(key *SRAKey, ciphertext *big.Int, cardIndex int, playerID string, sessionID []byte) (*PartialDecryption, error) {
	if key == nil || !key.IsPrivate() {
		return nil, errors.New("Peel: private exponent d is not present")
	}
	if playerID == "" {
		return nil, errors.New("Peel: playerID is empty")
	}
	if cardIndex < 0 {
		return nil, fmt.Errorf("Peel: cardIndex %d is negative", cardIndex)
	}
	if ciphertext == nil {
		return nil, errors.New("Peel: ciphertext is nil")
	}
	if len(sessionID) == 0 {
		return nil, errors.New("Peel: sessionID is empty")
	}

	result, err := key.Decrypt(ciphertext)
	if err != nil {
		return nil, fmt.Errorf("Peel: decrypt: %w", err)
	}
	proof, err := ProveDecryption(key, ciphertext, result, sessionID)
	if err != nil {
		return nil, fmt.Errorf("Peel: prove: %w", err)
	}
	return &PartialDecryption{
		PlayerID:   playerID,
		CardIndex:  cardIndex,
		Ciphertext: copyBig(ciphertext),
		Result:     copyBig(result),
		Proof:      proof,
	}, nil
}

// VerifyAndApply checks pd.Verify and that pd.Ciphertext equals current.
// Returns a copy of pd.Result as the next ciphertext.
func VerifyAndApply(current *big.Int, pd *PartialDecryption, p *big.Int, sessionID []byte) (*big.Int, error) {
	if current == nil {
		return nil, errors.New("VerifyAndApply: current ciphertext is nil")
	}
	if pd == nil {
		return nil, errors.New("VerifyAndApply: partial decryption is nil")
	}
	if pd.Ciphertext == nil || pd.Result == nil || pd.Proof == nil {
		return nil, errors.New("VerifyAndApply: partial decryption has nil fields")
	}
	if current.Cmp(pd.Ciphertext) != 0 {
		return nil, errors.New("VerifyAndApply: ciphertext mismatch")
	}
	if err := pd.Verify(p, sessionID); err != nil {
		return nil, fmt.Errorf("VerifyAndApply: %w", err)
	}
	return copyBig(pd.Result), nil
}

// FinishHole is the recipient's last local decrypt. Not a PartialDecryption.
func FinishHole(key *SRAKey, remaining *big.Int, p *big.Int) (game.Card, error) {
	if key == nil || !key.IsPrivate() {
		return game.Card{}, errors.New("FinishHole: private exponent d is not present")
	}
	plain, err := key.Decrypt(remaining)
	if err != nil {
		return game.Card{}, fmt.Errorf("FinishHole: decrypt: %w", err)
	}
	return FinishPublic(plain, p)
}

// FinishPublic maps a fully peeled value to a card (community / showdown).
func FinishPublic(value *big.Int, p *big.Int) (game.Card, error) {
	if value == nil {
		return game.Card{}, errors.New("FinishPublic: value is nil")
	}
	if p == nil {
		return game.Card{}, errors.New("FinishPublic: modulus is nil")
	}
	cardID := FieldToCard(value, p)
	if cardID == -1 {
		return game.Card{}, errors.New("FinishPublic: result is not a card")
	}
	return game.CardFromID(cardID), nil
}

// HoleCardIndex is the deck index of round (0 or 1) for playerIdx.
// playerIdx is an index into canonical seat order. Same walk as DealHoleCards:
//
//	playerIdx := (dealerIdx + 1 + i) % n  at  deckPos := round*n + i
func HoleCardIndex(nPlayers, dealerIdx, playerIdx, round int) (int, error) {
	if nPlayers < 2 {
		return 0, fmt.Errorf("HoleCardIndex: need at least 2 players, got %d", nPlayers)
	}
	if dealerIdx < 0 || dealerIdx >= nPlayers {
		return 0, fmt.Errorf("HoleCardIndex: dealerIdx %d out of range", dealerIdx)
	}
	if playerIdx < 0 || playerIdx >= nPlayers {
		return 0, fmt.Errorf("HoleCardIndex: playerIdx %d out of range", playerIdx)
	}
	if round != 0 && round != 1 {
		return 0, fmt.Errorf("HoleCardIndex: round must be 0 or 1, got %d", round)
	}
	i := (playerIdx - dealerIdx - 1 + nPlayers) % nPlayers
	return round*nPlayers + i, nil
}

// CommunityStartPos is 2*nPlayers: the first burn before the flop.
func CommunityStartPos(nPlayers int) int {
	return 2 * nPlayers
}

// FlopIndexes returns the three flop deck indexes (after one burn).
func FlopIndexes(nPlayers int) ([3]int, error) {
	var z [3]int
	if nPlayers < 2 {
		return z, fmt.Errorf("FlopIndexes: need at least 2 players, got %d", nPlayers)
	}
	start := CommunityStartPos(nPlayers)
	return [3]int{start + 1, start + 2, start + 3}, nil
}

// TurnIndex is the turn deck index (after the flop burn).
func TurnIndex(nPlayers int) (int, error) {
	if nPlayers < 2 {
		return 0, fmt.Errorf("TurnIndex: need at least 2 players, got %d", nPlayers)
	}
	return CommunityStartPos(nPlayers) + 5, nil
}

// RiverIndex is the river deck index (after the turn burn).
func RiverIndex(nPlayers int) (int, error) {
	if nPlayers < 2 {
		return 0, fmt.Errorf("RiverIndex: need at least 2 players, got %d", nPlayers)
	}
	return CommunityStartPos(nPlayers) + 7, nil
}

// PeelOrder is canonical seat order, skipping recipient when recipient != "".
// Public peels pass recipient == "".
func PeelOrder(seatOrder []string, recipient string) ([]string, error) {
	if recipient == "" {
		out := make([]string, len(seatOrder))
		copy(out, seatOrder)
		return out, nil
	}
	found := false
	out := make([]string, 0, len(seatOrder))
	for _, id := range seatOrder {
		if id == recipient {
			found = true
			continue
		}
		out = append(out, id)
	}
	if !found {
		return nil, fmt.Errorf("PeelOrder: recipient %q not in seat order", recipient)
	}
	return out, nil
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
		partial, err := Peel(dp.Keys[i], current, deckIndex, pid, dp.SessionID)
		if err != nil {
			return game.Card{}, nil, err
		}
		next, err := VerifyAndApply(current, partial, dp.Deck.P, dp.SessionID)
		if err != nil {
			return game.Card{}, nil, err
		}
		partials = append(partials, *partial)
		current = next
	}

	card, err := FinishHole(dp.Keys[recipentIdx], current, dp.Deck.P)
	if err != nil {
		return game.Card{}, nil, err
	}
	return card, partials, nil
}

func (dp *DealProtocol) RevealCommunity(deckIndex int) (game.Card, []PartialDecryption, error) {
	ciphertext, err := dp.Deck.CardAt(deckIndex)
	if err != nil {
		return game.Card{}, nil, err
	}

	current := new(big.Int).Set(ciphertext)
	var partials []PartialDecryption

	for i, pid := range dp.Players {
		partial, err := Peel(dp.Keys[i], current, deckIndex, pid, dp.SessionID)
		if err != nil {
			return game.Card{}, nil, err
		}
		next, err := VerifyAndApply(current, partial, dp.Deck.P, dp.SessionID)
		if err != nil {
			return game.Card{}, nil, err
		}
		partials = append(partials, *partial)
		current = next
	}
	card, err := FinishPublic(current, dp.Deck.P)
	if err != nil {
		return game.Card{}, nil, err
	}
	return card, partials, nil
}

func (dp *DealProtocol) applyPartialDecryption(playerID string, keyIdx, cardIdx int, input *big.Int) (*PartialDecryption, error) {
	return Peel(dp.Keys[keyIdx], input, cardIdx, playerID, dp.SessionID)
}

func (dp *DealProtocol) DealHoleCards(dealerIdx int) ([][2]game.Card, error) {
	n := len(dp.Players)
	result := make([][2]game.Card, n)

	deckPos := 0
	for round := 0; round < 2; round++ {
		for i := 0; i < n; i++ {
			playerIdx := (dealerIdx + 1 + i) % n
			card, _, err := dp.RevealToPlayer(deckPos, playerIdx)
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

func copyBig(n *big.Int) *big.Int {
	if n == nil {
		return nil
	}
	return new(big.Int).Set(n)
}
