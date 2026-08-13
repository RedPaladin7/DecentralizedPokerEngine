package crypto

import (
	"bytes"
	"errors"
	"fmt"
	"math/big"
	"sync"
)

// ShuffleMessage is the published shuffle step: output deck + commitment.
// It must not contain a permutation or the input deck.
type ShuffleMessage struct {
	HandNum    int64
	PlayerID   string
	OutputDeck []*big.Int
	Commitment *Commitment
}

// ShuffleMessageFromStep copies output deck + commitment. Permutation and
// InputDeck are dropped. handNum is the session's hand number.
func ShuffleMessageFromStep(handNum int64, step *ShuffleStep) (*ShuffleMessage, error) {
	if step == nil {
		return nil, errors.New("ShuffleMessageFromStep: step is nil")
	}
	if step.OutputDeck == nil {
		return nil, errors.New("ShuffleMessageFromStep: output deck is nil")
	}
	if step.Commitment == nil {
		return nil, errors.New("ShuffleMessageFromStep: commitment is nil")
	}
	return &ShuffleMessage{
		HandNum:    handNum,
		PlayerID:   step.PlayerID,
		OutputDeck: copyDeck(step.OutputDeck),
		Commitment: copyCommitment(step.Commitment),
	}, nil
}

// ShuffleSession is a per-replica turn-taking FSM for the distributed SRA shuffle.
type ShuffleSession struct {
	mu        sync.Mutex
	kr        *Keyring
	proto     *ShuffleProtocol
	handNum   int64
	sessionID []byte
	initial   []*big.Int
	started   bool
	nextIndex int
	current   []*big.Int
	pending   map[int]*ShuffleMessage
	applied   map[int]*ShuffleMessage
	localPerm []int
}

// NewShuffleSession builds a production session (52-card plaintext deck).
func NewShuffleSession(kr *Keyring, sessionID []byte, handNum int64) (*ShuffleSession, error) {
	if kr == nil {
		return nil, errors.New("NewShuffleSession: keyring is nil")
	}
	mod := kr.Modulus()
	if mod == nil {
		return nil, errors.New("NewShuffleSession: keyring modulus is nil")
	}
	return newShuffleSession(kr, sessionID, handNum, BuildPlaintextDeck(mod))
}

// newShuffleSessionN is a test-only constructor for smaller decks.
func newShuffleSessionN(kr *Keyring, sessionID []byte, handNum int64, initial []*big.Int) (*ShuffleSession, error) {
	return newShuffleSession(kr, sessionID, handNum, initial)
}

func newShuffleSession(kr *Keyring, sessionID []byte, handNum int64, initial []*big.Int) (*ShuffleSession, error) {
	if kr == nil {
		return nil, errors.New("NewShuffleSession: keyring is nil")
	}
	if kr.Len() < 2 {
		return nil, fmt.Errorf("NewShuffleSession: need at least 2 seats, got %d", kr.Len())
	}
	if len(sessionID) == 0 {
		return nil, errors.New("NewShuffleSession: sessionID is empty")
	}
	if handNum < 1 {
		return nil, errors.New("NewShuffleSession: handNum must be >= 1")
	}
	if kr.Modulus() == nil {
		return nil, errors.New("NewShuffleSession: keyring modulus is nil")
	}
	if len(initial) == 0 {
		return nil, errors.New("NewShuffleSession: initial deck is empty")
	}

	sid := make([]byte, len(sessionID))
	copy(sid, sessionID)
	sp := NewShuffleProtocol(kr.Modulus(), sid)
	sp.NumCards = len(initial)

	return &ShuffleSession{
		kr:        kr,
		proto:     sp,
		handNum:   handNum,
		sessionID: sid,
		initial:   copyDeck(initial),
		pending:   make(map[int]*ShuffleMessage),
		applied:   make(map[int]*ShuffleMessage),
	}, nil
}

// Start initializes the plaintext deck. If we are seat 0, execute and return
// our ShuffleMessage. Otherwise return (nil, nil) and wait for HandleMessage.
func (s *ShuffleSession) Start() (*ShuffleMessage, error) {
	if s == nil {
		return nil, errors.New("ShuffleSession.Start: session is nil")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.started || s.nextIndex != 0 {
		return nil, errors.New("ShuffleSession.Start: already started")
	}
	s.started = true
	s.current = copyDeck(s.initial)
	if s.expectedPlayerLocked() == s.kr.LocalID() {
		return s.executeLocalLocked()
	}
	return nil, nil
}

// HandleMessage is the sequencer. If after applying (and draining pending) it
// is our turn, execute and return our ShuffleMessage; otherwise (nil, nil).
func (s *ShuffleSession) HandleMessage(msg *ShuffleMessage) (*ShuffleMessage, error) {
	if s == nil {
		return nil, errors.New("ShuffleSession.HandleMessage: session is nil")
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.started {
		return nil, errors.New("ShuffleSession.HandleMessage: not started")
	}
	if err := s.validateIncomingLocked(msg); err != nil {
		return nil, err
	}

	seat, ok := s.kr.SeatIndex(msg.PlayerID)
	if !ok {
		return nil, fmt.Errorf("ShuffleSession.HandleMessage: unknown player %q", msg.PlayerID)
	}

	if seat < s.nextIndex {
		prev, exists := s.applied[seat]
		if exists && shuffleMessagesEqual(prev, msg) {
			return nil, nil
		}
		return nil, errors.New("ShuffleSession.HandleMessage: conflicting duplicate step")
	}

	if seat == s.nextIndex {
		if msg.PlayerID != s.expectedPlayerLocked() {
			return nil, fmt.Errorf("ShuffleSession.HandleMessage: wrong player %q, expected %q", msg.PlayerID, s.expectedPlayerLocked())
		}
		if msg.PlayerID == s.kr.LocalID() {
			return nil, errors.New("ShuffleSession.HandleMessage: local step must be produced locally")
		}
		if err := s.verifyAndAdoptLocked(msg); err != nil {
			return nil, err
		}
		return s.afterApplyLocked()
	}

	// seat > nextIndex: buffer future steps from other players.
	if msg.PlayerID == s.kr.LocalID() {
		return nil, errors.New("ShuffleSession.HandleMessage: wrong seat")
	}
	if prev, exists := s.pending[seat]; exists {
		if shuffleMessagesEqual(prev, msg) {
			return nil, nil
		}
		return nil, errors.New("ShuffleSession.HandleMessage: conflicting buffered step")
	}
	s.pending[seat] = copyShuffleMessage(msg)
	return nil, nil
}

func (s *ShuffleSession) Done() bool {
	if s == nil {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.doneLocked()
}

func (s *ShuffleSession) ExpectedPlayer() string {
	if s == nil {
		return ""
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.expectedPlayerLocked()
}

func (s *ShuffleSession) NextIndex() int {
	if s == nil {
		return 0
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.nextIndex
}

func (s *ShuffleSession) EncryptedDeck() (*EncryptedDeck, error) {
	if s == nil {
		return nil, errors.New("ShuffleSession.EncryptedDeck: session is nil")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.doneLocked() {
		return nil, errors.New("ShuffleSession.EncryptedDeck: shuffle is not complete")
	}
	return NewEncryptedDeck(s.current, s.kr.Modulus(), s.sessionID)
}

func (s *ShuffleSession) testDeck() []*big.Int {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return copyDeck(s.current)
}

func (s *ShuffleSession) validateIncomingLocked(msg *ShuffleMessage) error {
	if msg == nil {
		return errors.New("ShuffleSession.HandleMessage: message is nil")
	}
	if msg.PlayerID == "" {
		return errors.New("ShuffleSession.HandleMessage: empty player ID")
	}
	if s.proto == nil || len(msg.OutputDeck) != s.proto.NumCards {
		want := 0
		if s.proto != nil {
			want = s.proto.NumCards
		}
		return fmt.Errorf("ShuffleSession.HandleMessage: expected %d cards, got %d", want, len(msg.OutputDeck))
	}
	for i, c := range msg.OutputDeck {
		if c == nil {
			return fmt.Errorf("ShuffleSession.HandleMessage: nil card at index %d", i)
		}
	}
	if msg.Commitment == nil {
		return errors.New("ShuffleSession.HandleMessage: commitment is nil")
	}
	if msg.HandNum != s.handNum {
		return errors.New("ShuffleSession.HandleMessage: wrong hand")
	}
	return nil
}

func (s *ShuffleSession) executeLocalLocked() (*ShuffleMessage, error) {
	if s.expectedPlayerLocked() != s.kr.LocalID() {
		return nil, errors.New("ShuffleSession: not our turn")
	}
	step, err := s.proto.ExecuteStep(s.kr.LocalID(), copyDeck(s.current), s.kr.LocalKey())
	if err != nil {
		return nil, err
	}
	s.localPerm = copyInts(step.Permutation)
	msg, err := ShuffleMessageFromStep(s.handNum, step)
	if err != nil {
		return nil, err
	}
	if err := s.verifyAndAdoptLocked(msg); err != nil {
		return nil, err
	}
	if err := s.drainPendingLocked(); err != nil {
		return nil, err
	}
	return msg, nil
}

func (s *ShuffleSession) afterApplyLocked() (*ShuffleMessage, error) {
	if err := s.drainPendingLocked(); err != nil {
		return nil, err
	}
	if !s.doneLocked() && s.expectedPlayerLocked() == s.kr.LocalID() {
		return s.executeLocalLocked()
	}
	return nil, nil
}

func (s *ShuffleSession) drainPendingLocked() error {
	for {
		if s.doneLocked() {
			return nil
		}
		if s.expectedPlayerLocked() == s.kr.LocalID() {
			return nil
		}
		msg, ok := s.pending[s.nextIndex]
		if !ok {
			return nil
		}
		delete(s.pending, s.nextIndex)
		if err := s.verifyAndAdoptLocked(msg); err != nil {
			return err
		}
	}
}

func (s *ShuffleSession) verifyAndAdoptLocked(msg *ShuffleMessage) error {
	step := &ShuffleStep{
		PlayerID:   msg.PlayerID,
		OutputDeck: msg.OutputDeck,
		Commitment: msg.Commitment,
	}
	if err := s.proto.VerifyStep(step); err != nil {
		return fmt.Errorf("ShuffleSession.HandleMessage: %w", err)
	}
	copied := copyShuffleMessage(msg)
	s.applied[s.nextIndex] = copied
	s.current = copyDeck(copied.OutputDeck)
	s.nextIndex++
	return nil
}

func (s *ShuffleSession) doneLocked() bool {
	return s.kr != nil && s.nextIndex >= s.kr.Len()
}

func (s *ShuffleSession) expectedPlayerLocked() string {
	if s.kr == nil || s.nextIndex >= s.kr.Len() {
		return ""
	}
	order := s.kr.SeatOrder()
	if s.nextIndex < 0 || s.nextIndex >= len(order) {
		return ""
	}
	return order[s.nextIndex]
}

func copyCommitment(c *Commitment) *Commitment {
	if c == nil {
		return nil
	}
	h := make([]byte, len(c.Hash))
	copy(h, c.Hash)
	n := make([]byte, len(c.Nonce))
	copy(n, c.Nonce)
	return &Commitment{Hash: h, Nonce: n}
}

func copyShuffleMessage(msg *ShuffleMessage) *ShuffleMessage {
	if msg == nil {
		return nil
	}
	return &ShuffleMessage{
		HandNum:    msg.HandNum,
		PlayerID:   msg.PlayerID,
		OutputDeck: copyDeck(msg.OutputDeck),
		Commitment: copyCommitment(msg.Commitment),
	}
}

func shuffleMessagesEqual(a, b *ShuffleMessage) bool {
	if a == nil || b == nil {
		return a == b
	}
	if a.HandNum != b.HandNum || a.PlayerID != b.PlayerID {
		return false
	}
	if len(a.OutputDeck) != len(b.OutputDeck) {
		return false
	}
	for i := range a.OutputDeck {
		if a.OutputDeck[i] == nil || b.OutputDeck[i] == nil {
			if a.OutputDeck[i] != b.OutputDeck[i] {
				return false
			}
			continue
		}
		if a.OutputDeck[i].Cmp(b.OutputDeck[i]) != 0 {
			return false
		}
	}
	return commitmentsEqual(a.Commitment, b.Commitment)
}

func commitmentsEqual(a, b *Commitment) bool {
	if a == nil || b == nil {
		return a == b
	}
	return bytes.Equal(a.Hash, b.Hash) && bytes.Equal(a.Nonce, b.Nonce)
}
