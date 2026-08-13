package network

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"

	pokercrypto "github.com/RedPaladin7/DecentralizedPokerEngine/internal/crypto"
	"github.com/RedPaladin7/DecentralizedPokerEngine/internal/game"
)

const earlyBufCap = 64

// CryptoHand runs one replica's shuffle + peels for a single hand.
// It produces cards; callers feed them into game.Machine.
type CryptoHand struct {
	mu         sync.Mutex
	kr         *pokercrypto.Keyring
	handNum    int64
	dealerIdx  int
	sessionID  []byte
	shuffle    *pokercrypto.ShuffleSession
	deal       *pokercrypto.DealSession
	shuffleGate *waitGate
	holesGate   *waitGate
	streetGate  *waitGate
	revealGate  *waitGate
	earlyShuffle []*pokercrypto.ShuffleMessage
	earlyPeels   []*pokercrypto.PeelMessage
}

func NewCryptoHand(kr *pokercrypto.Keyring, lobbyNonce []byte, handNum int64, dealerIdx int) (*CryptoHand, error) {
	if kr == nil {
		return nil, errors.New("NewCryptoHand: keyring is nil")
	}
	if handNum < 1 {
		return nil, errors.New("NewCryptoHand: handNum must be >= 1")
	}
	if dealerIdx < 0 || dealerIdx >= kr.Len() {
		return nil, fmt.Errorf("NewCryptoHand: dealerIdx %d out of range", dealerIdx)
	}
	if len(lobbyNonce) == 0 {
		return nil, errors.New("NewCryptoHand: lobby nonce is empty")
	}
	nonce := append(append([]byte{}, lobbyNonce...), byte(handNum>>8), byte(handNum))
	sid := pokercrypto.SessionID(kr.SeatOrder(), nonce)
	sess, err := pokercrypto.NewShuffleSession(kr, sid, handNum)
	if err != nil {
		return nil, fmt.Errorf("NewCryptoHand: %w", err)
	}
	return &CryptoHand{
		kr:          kr,
		handNum:     handNum,
		dealerIdx:   dealerIdx,
		sessionID:   sid,
		shuffle:     sess,
		shuffleGate: newWaitGate(),
		holesGate:   newWaitGate(),
		streetGate:  newWaitGate(),
		revealGate:  newWaitGate(),
	}, nil
}

func (h *CryptoHand) StartShuffle() ([]*pokercrypto.ShuffleMessage, error) {
	if h == nil {
		return nil, errors.New("CryptoHand.StartShuffle: hand is nil")
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	out, err := h.shuffle.Start()
	if err != nil {
		return nil, err
	}
	extra, err := h.drainShuffleLocked()
	if err != nil {
		return nil, err
	}
	if err := h.afterShuffleLocked(); err != nil {
		return nil, err
	}
	return appendShuffle(out, extra), nil
}

func (h *CryptoHand) HandleShuffle(msg *pokercrypto.ShuffleMessage) ([]*pokercrypto.ShuffleMessage, error) {
	if h == nil {
		return nil, errors.New("CryptoHand.HandleShuffle: hand is nil")
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	out, err := h.shuffle.HandleMessage(msg)
	if err != nil {
		if isPrematureShuffle(err) {
			h.bufferShuffleLocked(msg)
			return nil, nil
		}
		return nil, err
	}
	extra, err := h.drainShuffleLocked()
	if err != nil {
		return nil, err
	}
	if err := h.afterShuffleLocked(); err != nil {
		return nil, err
	}
	return appendShuffle(out, extra), nil
}

func (h *CryptoHand) ShuffleDone() bool {
	if h == nil || h.shuffle == nil {
		return false
	}
	return h.shuffle.Done()
}

func (h *CryptoHand) WaitShuffle(ctx context.Context) error {
	if h == nil {
		return errors.New("CryptoHand.WaitShuffle: hand is nil")
	}
	if h.ShuffleDone() {
		h.mu.Lock()
		err := h.ensureDealLocked()
		h.mu.Unlock()
		if err != nil {
			return err
		}
		h.shuffleGate.signal()
		return nil
	}
	return h.shuffleGate.wait(ctx, "CryptoHand.WaitShuffle")
}

func (h *CryptoHand) StartHoles() ([]*pokercrypto.PeelMessage, error) {
	if h == nil {
		return nil, errors.New("CryptoHand.StartHoles: hand is nil")
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if err := h.ensureDealLocked(); err != nil {
		return nil, err
	}
	h.holesGate.reset()
	first, err := h.deal.BeginHoles()
	if err != nil {
		return nil, err
	}
	out := collectPeels(h.deal, first)
	extra, err := h.drainPeelsLocked()
	if err != nil {
		return nil, err
	}
	out = append(out, extra...)
	h.signalPeelProgressLocked()
	return out, nil
}

func (h *CryptoHand) HandlePeel(msg *pokercrypto.PeelMessage) ([]*pokercrypto.PeelMessage, error) {
	if h == nil {
		return nil, errors.New("CryptoHand.HandlePeel: hand is nil")
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.deal == nil {
		h.bufferPeelLocked(msg)
		return nil, nil
	}
	first, err := h.deal.HandlePeel(msg)
	if err != nil {
		if isPrematurePeel(err) {
			h.bufferPeelLocked(msg)
			return nil, nil
		}
		return nil, err
	}
	out := collectPeels(h.deal, first)
	extra, err := h.drainPeelsLocked()
	if err != nil {
		return nil, err
	}
	out = append(out, extra...)
	h.signalPeelProgressLocked()
	return out, nil
}

func (h *CryptoHand) HolesDone() bool {
	if h == nil {
		return false
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.deal != nil && h.deal.HolesDone()
}

func (h *CryptoHand) WaitHoles(ctx context.Context) error {
	if h == nil {
		return errors.New("CryptoHand.WaitHoles: hand is nil")
	}
	if h.HolesDone() {
		h.holesGate.signal()
		return nil
	}
	return h.holesGate.wait(ctx, "CryptoHand.WaitHoles")
}

func (h *CryptoHand) LocalHoles() ([2]game.Card, error) {
	var z [2]game.Card
	if h == nil {
		return z, errors.New("CryptoHand.LocalHoles: hand is nil")
	}
	h.mu.Lock()
	deal := h.deal
	h.mu.Unlock()
	if deal == nil {
		return z, errors.New("CryptoHand.LocalHoles: deal has not started")
	}
	return deal.LocalHoles()
}

func (h *CryptoHand) StartStreet(street pokercrypto.Street) ([]*pokercrypto.PeelMessage, error) {
	if h == nil {
		return nil, errors.New("CryptoHand.StartStreet: hand is nil")
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if err := h.ensureDealLocked(); err != nil {
		return nil, err
	}
	h.streetGate.reset()
	first, err := h.deal.BeginStreet(street)
	if err != nil {
		return nil, err
	}
	out := collectPeels(h.deal, first)
	extra, err := h.drainPeelsLocked()
	if err != nil {
		return nil, err
	}
	out = append(out, extra...)
	h.signalPeelProgressLocked()
	return out, nil
}

func (h *CryptoHand) WaitStreet(ctx context.Context) error {
	if h == nil {
		return errors.New("CryptoHand.WaitStreet: hand is nil")
	}
	h.mu.Lock()
	done := h.deal != nil && h.deal.StreetDone()
	h.mu.Unlock()
	if done {
		h.streetGate.signal()
		return nil
	}
	return h.streetGate.wait(ctx, "CryptoHand.WaitStreet")
}

func (h *CryptoHand) NewCommunityCards(already int) ([]game.Card, error) {
	if h == nil {
		return nil, errors.New("CryptoHand.NewCommunityCards: hand is nil")
	}
	h.mu.Lock()
	deal := h.deal
	h.mu.Unlock()
	if deal == nil {
		return nil, errors.New("CryptoHand.NewCommunityCards: deal has not started")
	}
	all, err := deal.CommunityCards()
	if err != nil {
		return nil, err
	}
	if already < 0 || already > len(all) {
		return nil, fmt.Errorf("CryptoHand.NewCommunityCards: already=%d len=%d", already, len(all))
	}
	out := make([]game.Card, len(all)-already)
	copy(out, all[already:])
	return out, nil
}

func (h *CryptoHand) StartReveal(playerID string) ([]*pokercrypto.PeelMessage, error) {
	if h == nil {
		return nil, errors.New("CryptoHand.StartReveal: hand is nil")
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if err := h.ensureDealLocked(); err != nil {
		return nil, err
	}
	h.revealGate.reset()
	first, err := h.deal.BeginReveal(playerID)
	if err != nil {
		return nil, err
	}
	out := collectPeels(h.deal, first)
	extra, err := h.drainPeelsLocked()
	if err != nil {
		return nil, err
	}
	out = append(out, extra...)
	h.signalPeelProgressLocked()
	return out, nil
}

func (h *CryptoHand) WaitReveal(ctx context.Context) error {
	if h == nil {
		return errors.New("CryptoHand.WaitReveal: hand is nil")
	}
	h.mu.Lock()
	done := h.deal != nil && h.deal.RevealDone()
	h.mu.Unlock()
	if done {
		h.revealGate.signal()
		return nil
	}
	return h.revealGate.wait(ctx, "CryptoHand.WaitReveal")
}

func (h *CryptoHand) RevealedHoles(playerID string) ([2]game.Card, error) {
	var z [2]game.Card
	if h == nil {
		return z, errors.New("CryptoHand.RevealedHoles: hand is nil")
	}
	h.mu.Lock()
	deal := h.deal
	h.mu.Unlock()
	if deal == nil {
		return z, errors.New("CryptoHand.RevealedHoles: deal has not started")
	}
	return deal.RevealedHoles(playerID)
}

// RemainingShowdownIDs is remaining Active/All-In seats in table order.
// Use this for BeginReveal, not Machine.MissingRevealIDs (replica-local).
func RemainingShowdownIDs(gs *game.GameState) []string {
	if gs == nil {
		return nil
	}
	var ids []string
	for _, p := range gs.Players {
		if p.Status == game.StatusActive || p.Status == game.StatusAllIn {
			ids = append(ids, p.ID)
		}
	}
	return ids
}

// StreetFromPending maps NeedsStreet + board length onto a DealSession street.
func StreetFromPending(m *game.Machine) (pokercrypto.Street, error) {
	if m == nil || !m.NeedsStreet() {
		return pokercrypto.StreetNone, errors.New("StreetFromPending: no street pending")
	}
	n := m.PendingStreetCount()
	already := 0
	if m.State != nil {
		already = len(m.State.CommunityCards)
	}
	switch {
	case n == 3 && already == 0:
		return pokercrypto.StreetFlop, nil
	case n == 1 && already == 3:
		return pokercrypto.StreetTurn, nil
	case n == 1 && already == 4:
		return pokercrypto.StreetRiver, nil
	default:
		return pokercrypto.StreetNone, fmt.Errorf("StreetFromPending: unexpected board size %d pending %d", already, n)
	}
}

// AdvanceCrypto peels pending streets and showdown reveals into the machine.
// send is called with every locally produced peel (including Outbound).
func AdvanceCrypto(ctx context.Context, h *CryptoHand, m *game.Machine, send func([]*pokercrypto.PeelMessage) error) error {
	if h == nil || m == nil {
		return errors.New("AdvanceCrypto: nil hand or machine")
	}
	if send == nil {
		send = func([]*pokercrypto.PeelMessage) error { return nil }
	}
	for m.NeedsStreet() {
		street, err := StreetFromPending(m)
		if err != nil {
			return err
		}
		already := len(m.State.CommunityCards)
		msgs, err := h.StartStreet(street)
		if err != nil {
			return err
		}
		if err := send(msgs); err != nil {
			return err
		}
		if err := h.WaitStreet(ctx); err != nil {
			return err
		}
		cards, err := h.NewCommunityCards(already)
		if err != nil {
			return err
		}
		if err := m.ApplyStreet(cards); err != nil {
			return err
		}
	}
	if m.NeedsReveal() {
		// Snapshot once. Each replica must peel every remaining seat in the
		// same order (local holes are already filled on one replica, so
		// applying the last missing seat can settle before the loop ends).
		ids := RemainingShowdownIDs(m.State)
		for _, pid := range ids {
			msgs, err := h.StartReveal(pid)
			if err != nil {
				return err
			}
			if err := send(msgs); err != nil {
				return err
			}
			if err := h.WaitReveal(ctx); err != nil {
				return err
			}
			pair, err := h.RevealedHoles(pid)
			if err != nil {
				return err
			}
			if m.State.Phase != game.PhaseShowdown {
				continue
			}
			if err := m.ApplyHoleReveal(pid, pair); err != nil {
				return err
			}
		}
	}
	return nil
}

func (h *CryptoHand) afterShuffleLocked() error {
	if !h.shuffle.Done() {
		return nil
	}
	if err := h.ensureDealLocked(); err != nil {
		return err
	}
	h.shuffleGate.signal()
	return nil
}

func (h *CryptoHand) ensureDealLocked() error {
	if h.deal != nil {
		return nil
	}
	if !h.shuffle.Done() {
		return errors.New("CryptoHand: shuffle is not complete")
	}
	ed, err := h.shuffle.EncryptedDeck()
	if err != nil {
		return err
	}
	deal, err := pokercrypto.NewDealSession(h.kr, ed, h.handNum, h.dealerIdx)
	if err != nil {
		return err
	}
	h.deal = deal
	return nil
}

func (h *CryptoHand) drainShuffleLocked() ([]*pokercrypto.ShuffleMessage, error) {
	var produced []*pokercrypto.ShuffleMessage
	pending := h.earlyShuffle
	h.earlyShuffle = nil
	for _, msg := range pending {
		out, err := h.shuffle.HandleMessage(msg)
		if err != nil {
			if isPrematureShuffle(err) {
				h.bufferShuffleLocked(msg)
				continue
			}
			return nil, err
		}
		if out != nil {
			produced = append(produced, out)
		}
	}
	return produced, nil
}

func appendShuffle(first *pokercrypto.ShuffleMessage, extra []*pokercrypto.ShuffleMessage) []*pokercrypto.ShuffleMessage {
	var out []*pokercrypto.ShuffleMessage
	if first != nil {
		out = append(out, first)
	}
	return append(out, extra...)
}

func (h *CryptoHand) drainPeelsLocked() ([]*pokercrypto.PeelMessage, error) {
	if h.deal == nil {
		return nil, nil
	}
	var produced []*pokercrypto.PeelMessage
	pending := h.earlyPeels
	h.earlyPeels = nil
	for _, msg := range pending {
		first, err := h.deal.HandlePeel(msg)
		if err != nil {
			if isPrematurePeel(err) {
				h.bufferPeelLocked(msg)
				continue
			}
			return nil, err
		}
		produced = append(produced, collectPeels(h.deal, first)...)
	}
	return produced, nil
}

func (h *CryptoHand) bufferShuffleLocked(msg *pokercrypto.ShuffleMessage) {
	if msg == nil || len(h.earlyShuffle) >= earlyBufCap {
		return
	}
	h.earlyShuffle = append(h.earlyShuffle, msg)
}

func (h *CryptoHand) bufferPeelLocked(msg *pokercrypto.PeelMessage) {
	if msg == nil || len(h.earlyPeels) >= earlyBufCap {
		return
	}
	h.earlyPeels = append(h.earlyPeels, msg)
}

func (h *CryptoHand) signalPeelProgressLocked() {
	if h.deal == nil {
		return
	}
	if h.deal.HolesDone() {
		h.holesGate.signal()
	}
	if h.deal.StreetDone() {
		h.streetGate.signal()
	}
	if h.deal.RevealDone() {
		h.revealGate.signal()
	}
}

func collectPeels(deal *pokercrypto.DealSession, first *pokercrypto.PeelMessage) []*pokercrypto.PeelMessage {
	var out []*pokercrypto.PeelMessage
	if first != nil {
		out = append(out, first)
	}
	if deal != nil {
		out = append(out, deal.Outbound()...)
	}
	return out
}

func isPrematureShuffle(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(err.Error(), "not started")
}

func isPrematurePeel(err error) bool {
	if err == nil {
		return false
	}
	s := err.Error()
	return strings.Contains(s, "no job in progress") ||
		strings.Contains(s, "wrong card index")
}

type waitGate struct {
	mu   sync.Mutex
	ch   chan struct{}
	done bool
}

func newWaitGate() *waitGate {
	return &waitGate{ch: make(chan struct{})}
}

func (g *waitGate) reset() {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.ch = make(chan struct{})
	g.done = false
}

func (g *waitGate) signal() {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.done {
		return
	}
	g.done = true
	close(g.ch)
}

func (g *waitGate) wait(ctx context.Context, who string) error {
	g.mu.Lock()
	ch := g.ch
	done := g.done
	g.mu.Unlock()
	if done {
		return nil
	}
	select {
	case <-ctx.Done():
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return fmt.Errorf("%s: timed out waiting", who)
		}
		return fmt.Errorf("%s: %w", who, ctx.Err())
	case <-ch:
		return nil
	}
}
