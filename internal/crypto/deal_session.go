package crypto

import (
	"errors"
	"fmt"
	"math/big"
	"sync"

	"github.com/RedPaladin7/DecentralizedPokerEngine/internal/game"
)

// Street is a community-card street peeled by DealSession.
type Street int

const (
	StreetNone Street = iota
	StreetFlop
	StreetTurn
	StreetRiver
)

type jobKind int

const (
	jobIdle jobKind = iota
	jobHole
	jobPublic
)

type seqKind int

const (
	seqIdle seqKind = iota
	seqHoles
	seqStreet
	seqReveal
)

type peelJob struct {
	kind      jobKind
	cardIndex int
	recipient string
	round     int
}

// PeelMessage is a published partial decrypt. It must not contain d or a card rank.
type PeelMessage struct {
	HandNum    int64
	PlayerID   string
	CardIndex  int
	Ciphertext *big.Int
	Result     *big.Int
	Proof      *ZKProof
}

// PeelMessageFromPD copies the partial decryption into a wire message.
func PeelMessageFromPD(handNum int64, pd *PartialDecryption) (*PeelMessage, error) {
	if pd == nil {
		return nil, errors.New("PeelMessageFromPD: partial decryption is nil")
	}
	if pd.Ciphertext == nil {
		return nil, errors.New("PeelMessageFromPD: ciphertext is nil")
	}
	if pd.Result == nil {
		return nil, errors.New("PeelMessageFromPD: result is nil")
	}
	if pd.Proof == nil {
		return nil, errors.New("PeelMessageFromPD: proof is nil")
	}
	return &PeelMessage{
		HandNum:    handNum,
		PlayerID:   pd.PlayerID,
		CardIndex:  pd.CardIndex,
		Ciphertext: copyBig(pd.Ciphertext),
		Result:     copyBig(pd.Result),
		Proof:      copyProof(pd.Proof),
	}, nil
}

// DealSession is a per-replica turn-taking FSM for distributed peels.
type DealSession struct {
	mu        sync.Mutex
	kr        *Keyring
	handNum   int64
	sessionID []byte
	p         *big.Int
	dealerIdx int
	cards     []*big.Int

	seq       seqKind
	seqJobs   []peelJob
	seqJobIdx int
	street    Street
	revealID  string

	jobKind    jobKind
	cardIndex  int
	recipient  string
	peelers    []string
	nextPeel   int
	current    *big.Int
	pending    map[int]*PeelMessage
	applied    map[int]*PeelMessage
	currentJob peelJob

	localHoles   [2]*big.Int
	revealed     map[string][2]*big.Int
	community    []*big.Int
	decoded      map[int]*big.Int
	holesDone    bool
	streetDone   bool
	revealDone   bool
	lastStreet   Street
	holesStarted bool
	outbound     []*PeelMessage
}

// NewDealSession builds a production session from a 52-card encrypted deck.
func NewDealSession(kr *Keyring, deck *EncryptedDeck, handNum int64, dealerIdx int) (*DealSession, error) {
	if deck == nil {
		return nil, errors.New("NewDealSession: deck is nil")
	}
	if len(deck.Cards) != 52 {
		return nil, fmt.Errorf("NewDealSession: expected 52 cards, got %d", len(deck.Cards))
	}
	return newDealSession(kr, copyDeck(deck.Cards), deck.SessionID, deck.P, handNum, dealerIdx)
}

// newDealSessionN is a test-only constructor for smaller decks.
func newDealSessionN(kr *Keyring, cards []*big.Int, sessionID []byte, handNum int64, dealerIdx int) (*DealSession, error) {
	if kr == nil {
		return nil, errors.New("NewDealSession: keyring is nil")
	}
	return newDealSession(kr, copyDeck(cards), sessionID, kr.Modulus(), handNum, dealerIdx)
}

func newDealSession(kr *Keyring, cards []*big.Int, sessionID []byte, p *big.Int, handNum int64, dealerIdx int) (*DealSession, error) {
	if kr == nil {
		return nil, errors.New("NewDealSession: keyring is nil")
	}
	if kr.Len() < 2 {
		return nil, fmt.Errorf("NewDealSession: need at least 2 seats, got %d", kr.Len())
	}
	if len(sessionID) == 0 {
		return nil, errors.New("NewDealSession: sessionID is empty")
	}
	if handNum < 1 {
		return nil, errors.New("NewDealSession: handNum must be >= 1")
	}
	if dealerIdx < 0 || dealerIdx >= kr.Len() {
		return nil, fmt.Errorf("NewDealSession: dealerIdx %d out of range", dealerIdx)
	}
	if p == nil {
		return nil, errors.New("NewDealSession: modulus is nil")
	}
	mod := kr.Modulus()
	if mod == nil {
		return nil, errors.New("NewDealSession: keyring modulus is nil")
	}
	if p.Cmp(mod) != 0 {
		return nil, errors.New("NewDealSession: deck modulus does not match keyring")
	}
	if len(cards) == 0 {
		return nil, errors.New("NewDealSession: deck is empty")
	}

	sid := make([]byte, len(sessionID))
	copy(sid, sessionID)
	return &DealSession{
		kr:        kr,
		handNum:   handNum,
		sessionID: sid,
		p:         new(big.Int).Set(p),
		dealerIdx: dealerIdx,
		cards:     copyDeck(cards),
		pending:   make(map[int]*PeelMessage),
		applied:   make(map[int]*PeelMessage),
		revealed:  make(map[string][2]*big.Int),
		decoded:   make(map[int]*big.Int),
	}, nil
}

// BeginHoles starts the hole-card sequence (left of dealer, two rounds).
// If we are the first peeler of the first card, return our PeelMessage.
func (s *DealSession) BeginHoles() (*PeelMessage, error) {
	if s == nil {
		return nil, errors.New("DealSession.BeginHoles: session is nil")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.seq != seqIdle {
		return nil, errors.New("DealSession.BeginHoles: sequence already in progress")
	}
	if s.holesStarted || s.holesDone {
		return nil, errors.New("DealSession.BeginHoles: holes already started")
	}

	n := s.kr.Len()
	order := s.kr.SeatOrder()
	jobs := make([]peelJob, 0, 2*n)
	for round := 0; round < 2; round++ {
		for i := 0; i < n; i++ {
			playerIdx := (s.dealerIdx + 1 + i) % n
			idx, err := HoleCardIndex(n, s.dealerIdx, playerIdx, round)
			if err != nil {
				return nil, err
			}
			jobs = append(jobs, peelJob{
				kind:      jobHole,
				cardIndex: idx,
				recipient: order[playerIdx],
				round:     round,
			})
		}
	}
	s.holesStarted = true
	return s.startSequenceLocked(seqHoles, jobs, StreetNone, "")
}

// BeginStreet starts public peels for flop (3), turn (1), or river (1).
func (s *DealSession) BeginStreet(street Street) (*PeelMessage, error) {
	if s == nil {
		return nil, errors.New("DealSession.BeginStreet: session is nil")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.seq != seqIdle {
		return nil, errors.New("DealSession.BeginStreet: sequence already in progress")
	}
	if !s.holesDone {
		return nil, errors.New("DealSession.BeginStreet: holes are not done")
	}
	if street != s.lastStreet+1 {
		return nil, fmt.Errorf("DealSession.BeginStreet: expected street %d, got %d", s.lastStreet+1, street)
	}

	n := s.kr.Len()
	var jobs []peelJob
	switch street {
	case StreetFlop:
		idxs, err := FlopIndexes(n)
		if err != nil {
			return nil, err
		}
		for _, idx := range idxs {
			jobs = append(jobs, peelJob{kind: jobPublic, cardIndex: idx})
		}
	case StreetTurn:
		idx, err := TurnIndex(n)
		if err != nil {
			return nil, err
		}
		jobs = append(jobs, peelJob{kind: jobPublic, cardIndex: idx})
	case StreetRiver:
		idx, err := RiverIndex(n)
		if err != nil {
			return nil, err
		}
		jobs = append(jobs, peelJob{kind: jobPublic, cardIndex: idx})
	default:
		return nil, fmt.Errorf("DealSession.BeginStreet: invalid street %d", street)
	}
	s.streetDone = false
	return s.startSequenceLocked(seqStreet, jobs, street, "")
}

// BeginReveal starts a public peel of playerID's two hole-card indexes.
func (s *DealSession) BeginReveal(playerID string) (*PeelMessage, error) {
	if s == nil {
		return nil, errors.New("DealSession.BeginReveal: session is nil")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.seq != seqIdle {
		return nil, errors.New("DealSession.BeginReveal: sequence already in progress")
	}
	if !s.holesDone {
		return nil, errors.New("DealSession.BeginReveal: holes are not done")
	}
	playerIdx, ok := s.kr.SeatIndex(playerID)
	if !ok {
		return nil, fmt.Errorf("DealSession.BeginReveal: unknown player %q", playerID)
	}
	if pair, ok := s.revealed[playerID]; ok && pair[0] != nil && pair[1] != nil {
		return nil, fmt.Errorf("DealSession.BeginReveal: %q already revealed", playerID)
	}

	n := s.kr.Len()
	jobs := make([]peelJob, 2)
	for round := 0; round < 2; round++ {
		idx, err := HoleCardIndex(n, s.dealerIdx, playerIdx, round)
		if err != nil {
			return nil, err
		}
		jobs[round] = peelJob{kind: jobPublic, cardIndex: idx, round: round}
	}
	s.revealDone = false
	return s.startSequenceLocked(seqReveal, jobs, StreetNone, playerID)
}

func (s *DealSession) startSequenceLocked(kind seqKind, jobs []peelJob, street Street, revealID string) (*PeelMessage, error) {
	if len(jobs) == 0 {
		return nil, errors.New("DealSession: empty job sequence")
	}
	s.seq = kind
	s.seqJobs = jobs
	s.seqJobIdx = 0
	s.street = street
	s.revealID = revealID
	if err := s.installJobLocked(jobs[0]); err != nil {
		s.seq = seqIdle
		return nil, err
	}
	return s.startCurrentJobLocked()
}

// HandlePeel is the sequencer for the current peel job.
// ExpectedPeeler returns the player ID who must peel next, or "" if idle/done.
func (s *DealSession) ExpectedPeeler() string {
	if s == nil {
		return ""
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.expectedPeelerLocked()
}

func (s *DealSession) expectedPeelerLocked() string {
	if s.seq == seqIdle || s.jobKind == jobIdle {
		return ""
	}
	if s.nextPeel < 0 || s.nextPeel >= len(s.peelers) {
		return ""
	}
	return s.peelers[s.nextPeel]
}

// PeelOnBehalf publishes a peel as playerID using key (a reconstructed d).
// PlayerID on the PeelMessage is playerID, not LocalID.
func (s *DealSession) PeelOnBehalf(playerID string, key *SRAKey) (*PeelMessage, error) {
	if s == nil {
		return nil, errors.New("PeelOnBehalf: session is nil")
	}
	if playerID == "" {
		return nil, errors.New("PeelOnBehalf: empty player ID")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if playerID == s.kr.LocalID() {
		return nil, errors.New("PeelOnBehalf: playerID is local; use the normal local path")
	}
	if s.seq == seqIdle || s.jobKind == jobIdle {
		return nil, errors.New("PeelOnBehalf: no job in progress")
	}
	expected := s.expectedPeelerLocked()
	if expected != playerID {
		return nil, fmt.Errorf("PeelOnBehalf: expected peeler %q, got %q", expected, playerID)
	}
	if key == nil || !key.IsPrivate() {
		return nil, errors.New("PeelOnBehalf: reconstructed key is missing d")
	}
	pd, err := Peel(key, s.current, s.cardIndex, playerID, s.sessionID)
	if err != nil {
		return nil, fmt.Errorf("PeelOnBehalf: %w", err)
	}
	msg, err := PeelMessageFromPD(s.handNum, pd)
	if err != nil {
		return nil, fmt.Errorf("PeelOnBehalf: %w", err)
	}
	if err := s.applyIncomingLocked(msg); err != nil {
		return nil, err
	}
	extra, err := s.afterApplyLocked()
	if err != nil {
		return nil, err
	}
	if extra != nil {
		s.outbound = append(s.outbound, extra)
	}
	return msg, nil
}

func (s *DealSession) HandlePeel(msg *PeelMessage) (*PeelMessage, error) {
	if s == nil {
		return nil, errors.New("DealSession.HandlePeel: session is nil")
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.seq == seqIdle || s.jobKind == jobIdle {
		return nil, errors.New("DealSession.HandlePeel: no job in progress")
	}
	if err := s.validateIncomingLocked(msg); err != nil {
		return nil, err
	}

	seat := peelerIndex(s.peelers, msg.PlayerID)
	if seat < 0 {
		return nil, fmt.Errorf("DealSession.HandlePeel: unknown player %q", msg.PlayerID)
	}

	if seat < s.nextPeel {
		prev, exists := s.applied[seat]
		if exists && peelMessagesEqual(prev, msg) {
			return nil, nil
		}
		return nil, errors.New("DealSession.HandlePeel: conflicting duplicate peel")
	}

	if seat == s.nextPeel {
		if msg.PlayerID != s.peelers[s.nextPeel] {
			return nil, fmt.Errorf("DealSession.HandlePeel: wrong player %q, expected %q", msg.PlayerID, s.peelers[s.nextPeel])
		}
		if msg.PlayerID == s.kr.LocalID() {
			return nil, errors.New("DealSession.HandlePeel: local peel must be produced locally")
		}
		if err := s.applyIncomingLocked(msg); err != nil {
			return nil, err
		}
		return s.afterApplyLocked()
	}

	if msg.PlayerID == s.kr.LocalID() {
		return nil, errors.New("DealSession.HandlePeel: wrong seat")
	}
	if prev, exists := s.pending[seat]; exists {
		if peelMessagesEqual(prev, msg) {
			return nil, nil
		}
		return nil, errors.New("DealSession.HandlePeel: conflicting buffered peel")
	}
	s.pending[seat] = copyPeelMessage(msg)
	return nil, nil
}

func (s *DealSession) HolesDone() bool {
	if s == nil {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.holesDone
}

func (s *DealSession) StreetDone() bool {
	if s == nil {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.streetDone
}

func (s *DealSession) RevealDone() bool {
	if s == nil {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.revealDone
}

// Outbound returns additional locally-produced peels that must be sent after
// the value returned by BeginHoles / BeginStreet / BeginReveal / HandlePeel.
func (s *DealSession) Outbound() []*PeelMessage {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	out := s.outbound
	s.outbound = nil
	return out
}

func (s *DealSession) LocalHoles() ([2]game.Card, error) {
	var z [2]game.Card
	if s == nil {
		return z, errors.New("DealSession.LocalHoles: session is nil")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.localHoles[0] == nil || s.localHoles[1] == nil {
		return z, errors.New("DealSession.LocalHoles: local holes not decoded")
	}
	c0, err := FinishPublic(s.localHoles[0], s.p)
	if err != nil {
		return z, fmt.Errorf("DealSession.LocalHoles: card 0: %w", err)
	}
	c1, err := FinishPublic(s.localHoles[1], s.p)
	if err != nil {
		return z, fmt.Errorf("DealSession.LocalHoles: card 1: %w", err)
	}
	return [2]game.Card{c0, c1}, nil
}

func (s *DealSession) CommunityCards() ([]game.Card, error) {
	if s == nil {
		return nil, errors.New("DealSession.CommunityCards: session is nil")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]game.Card, len(s.community))
	for i, v := range s.community {
		c, err := FinishPublic(v, s.p)
		if err != nil {
			return nil, fmt.Errorf("DealSession.CommunityCards[%d]: %w", i, err)
		}
		out[i] = c
	}
	return out, nil
}

func (s *DealSession) RevealedHoles(playerID string) ([2]game.Card, error) {
	var z [2]game.Card
	if s == nil {
		return z, errors.New("DealSession.RevealedHoles: session is nil")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	pair, ok := s.revealed[playerID]
	if !ok || pair[0] == nil || pair[1] == nil {
		return z, fmt.Errorf("DealSession.RevealedHoles: %q not revealed", playerID)
	}
	c0, err := FinishPublic(pair[0], s.p)
	if err != nil {
		return z, fmt.Errorf("DealSession.RevealedHoles: card 0: %w", err)
	}
	c1, err := FinishPublic(pair[1], s.p)
	if err != nil {
		return z, fmt.Errorf("DealSession.RevealedHoles: card 1: %w", err)
	}
	return [2]game.Card{c0, c1}, nil
}

func (s *DealSession) testDecoded(cardIndex int) *big.Int {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return copyBig(s.decoded[cardIndex])
}

func (s *DealSession) testNextPeel() int {
	if s == nil {
		return 0
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.nextPeel
}

func (s *DealSession) testCardIndex() int {
	if s == nil {
		return -1
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.cardIndex
}

func (s *DealSession) validateIncomingLocked(msg *PeelMessage) error {
	if msg == nil {
		return errors.New("DealSession.HandlePeel: message is nil")
	}
	if msg.PlayerID == "" {
		return errors.New("DealSession.HandlePeel: empty player ID")
	}
	if msg.Ciphertext == nil || msg.Result == nil || msg.Proof == nil {
		return errors.New("DealSession.HandlePeel: nil ciphertext, result, or proof")
	}
	if msg.HandNum != s.handNum {
		return errors.New("DealSession.HandlePeel: wrong hand")
	}
	if msg.CardIndex != s.cardIndex {
		return errors.New("DealSession.HandlePeel: wrong card index")
	}
	return nil
}

func (s *DealSession) installJobLocked(j peelJob) error {
	if j.cardIndex < 0 || j.cardIndex >= len(s.cards) {
		return fmt.Errorf("DealSession: card index %d out of range (deck has %d)", j.cardIndex, len(s.cards))
	}
	peelers, err := PeelOrder(s.kr.SeatOrder(), j.recipient)
	if err != nil {
		return err
	}
	if len(peelers) == 0 {
		return errors.New("DealSession: no peelers")
	}
	s.jobKind = j.kind
	s.cardIndex = j.cardIndex
	s.recipient = j.recipient
	s.peelers = peelers
	s.nextPeel = 0
	s.current = copyBig(s.cards[j.cardIndex])
	s.pending = make(map[int]*PeelMessage)
	s.applied = make(map[int]*PeelMessage)
	s.currentJob = j
	return nil
}

func (s *DealSession) startCurrentJobLocked() (*PeelMessage, error) {
	if s.peelers[0] == s.kr.LocalID() {
		return s.executeLocalLocked()
	}
	return nil, nil
}

func (s *DealSession) executeLocalLocked() (*PeelMessage, error) {
	if s.nextPeel >= len(s.peelers) || s.peelers[s.nextPeel] != s.kr.LocalID() {
		return nil, errors.New("DealSession: not our turn")
	}
	pd, err := Peel(s.kr.LocalKey(), s.current, s.cardIndex, s.kr.LocalID(), s.sessionID)
	if err != nil {
		return nil, err
	}
	msg, err := PeelMessageFromPD(s.handNum, pd)
	if err != nil {
		return nil, err
	}
	if err := s.applyIncomingLocked(msg); err != nil {
		return nil, err
	}
	extra, err := s.afterApplyLocked()
	if err != nil {
		return nil, err
	}
	if extra != nil {
		s.outbound = append(s.outbound, extra)
	}
	return msg, nil
}

func (s *DealSession) afterApplyLocked() (*PeelMessage, error) {
	if err := s.drainPendingLocked(); err != nil {
		return nil, err
	}
	if s.jobCompleteLocked() {
		if err := s.finishJobLocked(); err != nil {
			return nil, err
		}
		return s.advanceSequenceLocked()
	}
	if s.nextPeel < len(s.peelers) && s.peelers[s.nextPeel] == s.kr.LocalID() {
		return s.executeLocalLocked()
	}
	return nil, nil
}

func (s *DealSession) drainPendingLocked() error {
	for {
		if s.jobCompleteLocked() {
			return nil
		}
		if s.peelers[s.nextPeel] == s.kr.LocalID() {
			return nil
		}
		msg, ok := s.pending[s.nextPeel]
		if !ok {
			return nil
		}
		delete(s.pending, s.nextPeel)
		if err := s.applyIncomingLocked(msg); err != nil {
			return err
		}
	}
}

func (s *DealSession) applyIncomingLocked(msg *PeelMessage) error {
	pd := peelMessageToPD(msg)
	next, err := VerifyAndApply(s.current, pd, s.p, s.sessionID)
	if err != nil {
		return fmt.Errorf("DealSession.HandlePeel: %w", err)
	}
	seat := peelerIndex(s.peelers, msg.PlayerID)
	if seat < 0 {
		return fmt.Errorf("DealSession.HandlePeel: unknown player %q", msg.PlayerID)
	}
	s.applied[seat] = copyPeelMessage(msg)
	s.current = next
	s.nextPeel++
	return nil
}

func (s *DealSession) jobCompleteLocked() bool {
	return s.nextPeel >= len(s.peelers)
}

func (s *DealSession) finishJobLocked() error {
	j := s.currentJob
	if j.kind == jobHole {
		if s.recipient == s.kr.LocalID() {
			plain, err := s.kr.LocalKey().Decrypt(s.current)
			if err != nil {
				return fmt.Errorf("DealSession: recipient decrypt: %w", err)
			}
			s.localHoles[j.round] = copyBig(plain)
			s.decoded[j.cardIndex] = copyBig(plain)
		}
		return nil
	}
	val := copyBig(s.current)
	s.decoded[j.cardIndex] = val
	if s.seq == seqStreet {
		s.community = append(s.community, copyBig(val))
	}
	if s.seq == seqReveal {
		pair := s.revealed[s.revealID]
		pair[j.round] = copyBig(val)
		s.revealed[s.revealID] = pair
	}
	return nil
}

func (s *DealSession) advanceSequenceLocked() (*PeelMessage, error) {
	s.seqJobIdx++
	if s.seqJobIdx >= len(s.seqJobs) {
		switch s.seq {
		case seqHoles:
			s.holesDone = true
		case seqStreet:
			s.streetDone = true
			s.lastStreet = s.street
		case seqReveal:
			s.revealDone = true
		}
		s.seq = seqIdle
		s.jobKind = jobIdle
		return nil, nil
	}
	if err := s.installJobLocked(s.seqJobs[s.seqJobIdx]); err != nil {
		return nil, err
	}
	return s.startCurrentJobLocked()
}

func peelerIndex(peelers []string, id string) int {
	for i, p := range peelers {
		if p == id {
			return i
		}
	}
	return -1
}

func peelMessageToPD(msg *PeelMessage) *PartialDecryption {
	if msg == nil {
		return nil
	}
	return &PartialDecryption{
		PlayerID:   msg.PlayerID,
		CardIndex:  msg.CardIndex,
		Ciphertext: msg.Ciphertext,
		Result:     msg.Result,
		Proof:      msg.Proof,
	}
}

func copyProof(p *ZKProof) *ZKProof {
	if p == nil {
		return nil
	}
	return &ZKProof{
		A: copyBig(p.A),
		B: copyBig(p.B),
		S: copyBig(p.S),
		H: copyBig(p.H),
	}
}

func copyPeelMessage(msg *PeelMessage) *PeelMessage {
	if msg == nil {
		return nil
	}
	return &PeelMessage{
		HandNum:    msg.HandNum,
		PlayerID:   msg.PlayerID,
		CardIndex:  msg.CardIndex,
		Ciphertext: copyBig(msg.Ciphertext),
		Result:     copyBig(msg.Result),
		Proof:      copyProof(msg.Proof),
	}
}

func peelMessagesEqual(a, b *PeelMessage) bool {
	if a == nil || b == nil {
		return a == b
	}
	if a.HandNum != b.HandNum || a.PlayerID != b.PlayerID || a.CardIndex != b.CardIndex {
		return false
	}
	if a.Ciphertext == nil || b.Ciphertext == nil || a.Result == nil || b.Result == nil {
		return a.Ciphertext == b.Ciphertext && a.Result == b.Result
	}
	if a.Ciphertext.Cmp(b.Ciphertext) != 0 || a.Result.Cmp(b.Result) != 0 {
		return false
	}
	return proofsEqual(a.Proof, b.Proof)
}

func proofsEqual(a, b *ZKProof) bool {
	if a == nil || b == nil {
		return a == b
	}
	if a.A == nil || b.A == nil || a.B == nil || b.B == nil || a.S == nil || b.S == nil || a.H == nil || b.H == nil {
		return false
	}
	return a.A.Cmp(b.A) == 0 && a.B.Cmp(b.B) == 0 && a.S.Cmp(b.S) == 0 && a.H.Cmp(b.H) == 0
}
