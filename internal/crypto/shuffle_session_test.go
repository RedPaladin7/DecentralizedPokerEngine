package crypto

import (
	"context"
	"math/big"
	"sync"
	"testing"
	"time"
)

const testHandNum int64 = 1

func tinyDeck() []*big.Int {
	return []*big.Int{
		big.NewInt(2),
		big.NewInt(3),
		big.NewInt(4),
		big.NewInt(6),
	}
}

func testSessionID(ids []string) []byte {
	return SessionID(ids, []byte("shuffle-session-test"))
}

func nPlayerKeyrings(t *testing.T, p *big.Int, ids []string) []*Keyring {
	t.Helper()
	keys := make([]*SRAKey, len(ids))
	publicE := make(map[string][]byte, len(ids))
	for i, id := range ids {
		k, err := GenerateSRAKey(p)
		if err != nil {
			t.Fatalf("GenerateSRAKey %s: %v", id, err)
		}
		keys[i] = k
		publicE[id] = k.PublicKey().Bytes()
	}
	rings := make([]*Keyring, len(ids))
	for i, id := range ids {
		kr, err := NewKeyring(id, keys[i], publicE, ids)
		if err != nil {
			t.Fatalf("NewKeyring %s: %v", id, err)
		}
		rings[i] = kr
	}
	return rings
}

func newTinySession(t *testing.T, kr *Keyring) *ShuffleSession {
	t.Helper()
	s, err := newShuffleSessionN(kr, testSessionID(kr.SeatOrder()), testHandNum, tinyDeck())
	if err != nil {
		t.Fatalf("newShuffleSessionN: %v", err)
	}
	return s
}

func dummyShuffleMsg(handNum int64, player string, deck []*big.Int) *ShuffleMessage {
	c, err := NewDeckCommitment(deck)
	if err != nil {
		panic(err)
	}
	return &ShuffleMessage{
		HandNum:    handNum,
		PlayerID:   player,
		OutputDeck: copyDeck(deck),
		Commitment: c,
	}
}

func decksEqual(a, b []*big.Int) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] == nil || b[i] == nil || a[i].Cmp(b[i]) != 0 {
			return false
		}
	}
	return true
}

func mustStart(t *testing.T, s *ShuffleSession) *ShuffleMessage {
	t.Helper()
	msg, err := s.Start()
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	return msg
}

func mustHandle(t *testing.T, s *ShuffleSession, msg *ShuffleMessage) *ShuffleMessage {
	t.Helper()
	out, err := s.HandleMessage(msg)
	if err != nil {
		t.Fatalf("HandleMessage: %v", err)
	}
	return out
}

// ── A. Wire shape ─────────────────────────────────────────────────────────────

func TestShuffleMessageFromStep_OmitsPermutation(t *testing.T) {
	rings := nPlayerKeyrings(t, smallPrime, []string{"alice", "bob"})
	sp := NewShuffleProtocol(smallPrime, testSessionID([]string{"alice", "bob"}))
	sp.NumCards = 4
	step, err := sp.ExecuteStep("alice", tinyDeck(), rings[0].LocalKey())
	if err != nil {
		t.Fatalf("ExecuteStep: %v", err)
	}
	if len(step.Permutation) == 0 {
		t.Fatal("step is missing permutation (needed to prove it was dropped)")
	}

	msg, err := ShuffleMessageFromStep(testHandNum, step)
	if err != nil {
		t.Fatalf("ShuffleMessageFromStep: %v", err)
	}
	if msg.PlayerID != "alice" {
		t.Errorf("PlayerID: got %q", msg.PlayerID)
	}
	if msg.HandNum != testHandNum {
		t.Errorf("HandNum: got %d", msg.HandNum)
	}
	if len(msg.OutputDeck) != 4 {
		t.Fatalf("OutputDeck len: got %d", len(msg.OutputDeck))
	}
	if msg.Commitment == nil || len(msg.Commitment.Hash) == 0 {
		t.Fatal("commitment missing")
	}
	if !decksEqual(msg.OutputDeck, step.OutputDeck) {
		t.Error("message deck does not match step output")
	}

	msg.OutputDeck[0].Add(msg.OutputDeck[0], big.NewInt(1))
	if msg.OutputDeck[0].Cmp(step.OutputDeck[0]) == 0 {
		t.Error("mutating message deck changed step.OutputDeck")
	}
}

func TestExecuteStep_RejectsPublicOnlyKey(t *testing.T) {
	rings := nPlayerKeyrings(t, smallPrime, []string{"alice", "bob"})
	sp := NewShuffleProtocol(smallPrime, testSessionID([]string{"alice", "bob"}))
	sp.NumCards = 4
	pub, ok := rings[0].Public("alice")
	if !ok {
		t.Fatal("Public(alice) missing")
	}
	if _, err := sp.ExecuteStep("alice", tinyDeck(), pub); err == nil {
		t.Error("expected error for public-only key")
	}
}

// ── B. Session FSM ────────────────────────────────────────────────────────────

func TestShuffleSession_Seat0Starts(t *testing.T) {
	rings := nPlayerKeyrings(t, smallPrime, []string{"alice", "bob"})
	alice := newTinySession(t, rings[0])
	bob := newTinySession(t, rings[1])

	msg := mustStart(t, alice)
	if msg == nil {
		t.Fatal("alice Start() should return a message")
	}
	if alice.NextIndex() != 1 {
		t.Errorf("alice NextIndex: got %d, want 1", alice.NextIndex())
	}
	if msg.PlayerID != "alice" {
		t.Errorf("message player: got %q", msg.PlayerID)
	}

	bobMsg := mustStart(t, bob)
	if bobMsg != nil {
		t.Fatal("bob Start() should return nil")
	}
	if bob.NextIndex() != 0 {
		t.Errorf("bob NextIndex: got %d, want 0", bob.NextIndex())
	}
}

func TestShuffleSession_TwoPlayers_Sequential(t *testing.T) {
	rings := nPlayerKeyrings(t, smallPrime, []string{"alice", "bob"})
	alice := newTinySession(t, rings[0])
	bob := newTinySession(t, rings[1])

	aliceStep := mustStart(t, alice)
	mustStart(t, bob)

	bobStep := mustHandle(t, bob, aliceStep)
	if bobStep == nil {
		t.Fatal("bob should produce a step after receiving alice")
	}
	if !bob.Done() {
		t.Fatal("bob should be done after producing the last step")
	}

	if out := mustHandle(t, alice, bobStep); out != nil {
		t.Fatal("alice should not produce another step")
	}
	if !alice.Done() || !bob.Done() {
		t.Fatal("both replicas should be done")
	}
	if !decksEqual(alice.testDeck(), bob.testDeck()) {
		t.Fatal("final decks differ")
	}
}

func TestShuffleSession_WrongSeatRejected(t *testing.T) {
	rings := nPlayerKeyrings(t, smallPrime, []string{"alice", "bob"})
	bob := newTinySession(t, rings[1])
	mustStart(t, bob)
	if bob.ExpectedPlayer() != "alice" {
		t.Fatalf("expected alice, got %q", bob.ExpectedPlayer())
	}
	before := bob.NextIndex()
	_, err := bob.HandleMessage(dummyShuffleMsg(testHandNum, "bob", tinyDeck()))
	if err == nil {
		t.Fatal("expected error when bob tries to shuffle out of turn")
	}
	if bob.NextIndex() != before {
		t.Errorf("nextIndex changed: got %d, want %d", bob.NextIndex(), before)
	}
}

func TestShuffleSession_WrongHandRejected(t *testing.T) {
	rings := nPlayerKeyrings(t, smallPrime, []string{"alice", "bob"})
	bob := newTinySession(t, rings[1])
	mustStart(t, bob)
	before := bob.NextIndex()
	_, err := bob.HandleMessage(dummyShuffleMsg(2, "alice", tinyDeck()))
	if err == nil {
		t.Fatal("expected error for wrong hand")
	}
	if bob.NextIndex() != before {
		t.Errorf("nextIndex changed: got %d, want %d", bob.NextIndex(), before)
	}
}

func TestShuffleSession_TamperedCommitmentRejected(t *testing.T) {
	rings := nPlayerKeyrings(t, smallPrime, []string{"alice", "bob"})
	alice := newTinySession(t, rings[0])
	bob := newTinySession(t, rings[1])
	aliceStep := mustStart(t, alice)
	mustStart(t, bob)

	tampered := copyShuffleMessage(aliceStep)
	tampered.Commitment.Hash[0] ^= 0x01
	before := bob.NextIndex()
	if _, err := bob.HandleMessage(tampered); err == nil {
		t.Fatal("expected error for tampered commitment")
	}
	if bob.NextIndex() != before {
		t.Errorf("nextIndex changed: got %d, want %d", bob.NextIndex(), before)
	}
}

func TestShuffleSession_DuplicateIgnored(t *testing.T) {
	rings := nPlayerKeyrings(t, smallPrime, []string{"alice", "bob"})
	alice := newTinySession(t, rings[0])
	bob := newTinySession(t, rings[1])
	aliceStep := mustStart(t, alice)
	mustStart(t, bob)

	bobStep := mustHandle(t, bob, aliceStep)
	if bobStep == nil {
		t.Fatal("bob should produce a step")
	}
	if bob.NextIndex() != 2 {
		t.Fatalf("bob NextIndex after first apply: got %d, want 2", bob.NextIndex())
	}
	out, err := bob.HandleMessage(aliceStep)
	if err != nil {
		t.Fatalf("duplicate should be ignored, got error: %v", err)
	}
	if out != nil {
		t.Fatal("duplicate should not produce a message")
	}
	if bob.NextIndex() != 2 {
		t.Errorf("nextIndex changed after duplicate: got %d", bob.NextIndex())
	}
}

func TestShuffleSession_ConflictingDuplicateRejected(t *testing.T) {
	rings := nPlayerKeyrings(t, smallPrime, []string{"alice", "bob"})
	alice := newTinySession(t, rings[0])
	bob := newTinySession(t, rings[1])
	aliceStep := mustStart(t, alice)
	mustStart(t, bob)
	mustHandle(t, bob, aliceStep)

	conflict := dummyShuffleMsg(testHandNum, "alice", tinyDeck())
	before := bob.NextIndex()
	if _, err := bob.HandleMessage(conflict); err == nil {
		t.Fatal("expected error for conflicting duplicate")
	}
	if bob.NextIndex() != before {
		t.Errorf("nextIndex changed: got %d, want %d", bob.NextIndex(), before)
	}
}

func TestShuffleSession_BuffersOutOfOrder(t *testing.T) {
	ids := []string{"alice", "bob", "carol"}
	rings := nPlayerKeyrings(t, smallPrime, ids)
	alice := newTinySession(t, rings[0])
	bob := newTinySession(t, rings[1])
	carol := newTinySession(t, rings[2])

	aliceStep := mustStart(t, alice)
	mustStart(t, bob)
	mustStart(t, carol)

	bobStep := mustHandle(t, bob, aliceStep)
	if bobStep == nil {
		t.Fatal("bob should produce a step")
	}

	// Carol receives Bob before Alice.
	if out := mustHandle(t, carol, bobStep); out != nil {
		t.Fatal("carol should buffer bob, not produce yet")
	}
	if carol.Done() || carol.NextIndex() != 0 {
		t.Fatalf("carol should still be waiting for alice: done=%v next=%d", carol.Done(), carol.NextIndex())
	}

	carolStep := mustHandle(t, carol, aliceStep)
	if carolStep == nil {
		t.Fatal("carol should drain bob and produce her step")
	}
	if !carol.Done() {
		t.Fatal("carol should be done")
	}

	mustHandle(t, alice, bobStep)
	mustHandle(t, alice, carolStep)
	mustHandle(t, bob, carolStep)

	if !alice.Done() || !bob.Done() || !carol.Done() {
		t.Fatal("all replicas should be done")
	}
	if !decksEqual(alice.testDeck(), bob.testDeck()) || !decksEqual(alice.testDeck(), carol.testDeck()) {
		t.Fatal("final decks differ")
	}
}

func TestShuffleSession_UnknownPlayerRejected(t *testing.T) {
	rings := nPlayerKeyrings(t, smallPrime, []string{"alice", "bob"})
	bob := newTinySession(t, rings[1])
	mustStart(t, bob)
	before := bob.NextIndex()
	if _, err := bob.HandleMessage(dummyShuffleMsg(testHandNum, "mallory", tinyDeck())); err == nil {
		t.Fatal("expected error for unknown player")
	}
	if bob.NextIndex() != before {
		t.Errorf("nextIndex changed: got %d, want %d", bob.NextIndex(), before)
	}
}

// ── C. Fake-net ───────────────────────────────────────────────────────────────

type fakeShuffleBus struct {
	chans map[string]chan *ShuffleMessage
}

func newFakeShuffleBus(ids []string) *fakeShuffleBus {
	b := &fakeShuffleBus{chans: make(map[string]chan *ShuffleMessage, len(ids))}
	for _, id := range ids {
		b.chans[id] = make(chan *ShuffleMessage, 16)
	}
	return b
}

func (b *fakeShuffleBus) Broadcast(from string, msg *ShuffleMessage) {
	for id, ch := range b.chans {
		if id == from {
			continue
		}
		ch <- copyShuffleMessage(msg)
	}
}

func runReplica(ctx context.Context, id string, sess *ShuffleSession, bus *fakeShuffleBus) error {
	out, err := sess.Start()
	if err != nil {
		return err
	}
	if out != nil {
		bus.Broadcast(id, out)
	}
	for {
		if sess.Done() {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case msg := <-bus.chans[id]:
			out, err := sess.HandleMessage(msg)
			if err != nil {
				return err
			}
			if out != nil {
				bus.Broadcast(id, out)
			}
		}
	}
}

func runFakeNet(t *testing.T, timeout time.Duration, rings []*Keyring, sessions []*ShuffleSession) {
	t.Helper()
	ids := rings[0].SeatOrder()
	bus := newFakeShuffleBus(ids)
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	var wg sync.WaitGroup
	errCh := make(chan error, len(ids))
	for i, id := range ids {
		wg.Add(1)
		go func(id string, sess *ShuffleSession) {
			defer wg.Done()
			if err := runReplica(ctx, id, sess, bus); err != nil {
				errCh <- err
			}
		}(id, sessions[i])
	}
	wg.Wait()
	close(errCh)
	for err := range errCh {
		t.Fatalf("replica error: %v", err)
	}
	for i, s := range sessions {
		if !s.Done() {
			t.Fatalf("replica %s not done", ids[i])
		}
	}
}

func TestShuffleSession_FakeNet_2Players(t *testing.T) {
	ids := []string{"alice", "bob"}
	rings := nPlayerKeyrings(t, smallPrime, ids)
	sessions := []*ShuffleSession{newTinySession(t, rings[0]), newTinySession(t, rings[1])}
	runFakeNet(t, 30*time.Second, rings, sessions)

	if !decksEqual(sessions[0].testDeck(), sessions[1].testDeck()) {
		t.Fatal("final decks differ")
	}
	if len(sessions[0].testDeck()) != 4 {
		t.Fatalf("deck len: got %d", len(sessions[0].testDeck()))
	}
	if decksEqual(sessions[0].testDeck(), tinyDeck()) {
		t.Fatal("final deck equals plaintext — shuffle had no effect")
	}
}

func TestShuffleSession_FakeNet_3Players(t *testing.T) {
	ids := []string{"alice", "bob", "carol"}
	rings := nPlayerKeyrings(t, smallPrime, ids)
	sessions := []*ShuffleSession{
		newTinySession(t, rings[0]),
		newTinySession(t, rings[1]),
		newTinySession(t, rings[2]),
	}
	runFakeNet(t, 30*time.Second, rings, sessions)

	a, b, c := sessions[0].testDeck(), sessions[1].testDeck(), sessions[2].testDeck()
	if !decksEqual(a, b) || !decksEqual(a, c) {
		t.Fatal("final decks differ")
	}
}

func TestShuffleSession_FakeNet_2Players_ProductionDeck(t *testing.T) {
	ids := []string{"alice", "bob"}
	rings := nPlayerKeyrings(t, SharedPrime(), ids)
	sid := testSessionID(ids)
	sessions := make([]*ShuffleSession, 2)
	for i := range sessions {
		s, err := NewShuffleSession(rings[i], sid, testHandNum)
		if err != nil {
			t.Fatalf("NewShuffleSession: %v", err)
		}
		sessions[i] = s
	}
	runFakeNet(t, 2*time.Minute, rings, sessions)

	ed0, err := sessions[0].EncryptedDeck()
	if err != nil {
		t.Fatalf("alice EncryptedDeck: %v", err)
	}
	ed1, err := sessions[1].EncryptedDeck()
	if err != nil {
		t.Fatalf("bob EncryptedDeck: %v", err)
	}
	if !decksEqual(ed0.Cards, ed1.Cards) {
		t.Fatal("EncryptedDeck cards differ")
	}
	if len(ed0.Cards) != 52 {
		t.Fatalf("expected 52 cards, got %d", len(ed0.Cards))
	}
}

// ── D. Privacy ────────────────────────────────────────────────────────────────

func TestShuffleSession_PublicCannotRecoverPlaintext(t *testing.T) {
	ids := []string{"alice", "bob"}
	rings := nPlayerKeyrings(t, SharedPrime(), ids)
	sid := testSessionID(ids)
	sessions := make([]*ShuffleSession, 2)
	for i := range sessions {
		s, err := NewShuffleSession(rings[i], sid, testHandNum)
		if err != nil {
			t.Fatalf("NewShuffleSession: %v", err)
		}
		sessions[i] = s
	}
	runFakeNet(t, 2*time.Minute, rings, sessions)

	ed, err := sessions[0].EncryptedDeck()
	if err != nil {
		t.Fatalf("EncryptedDeck: %v", err)
	}
	p := rings[0].Modulus()
	for i, c := range ed.Cards {
		if FieldToCard(c, p) != -1 {
			t.Fatalf("final[%d] decoded as a plaintext card", i)
		}
	}

	peeled, err := rings[0].LocalKey().DecryptAll(ed.Cards)
	if err != nil {
		t.Fatalf("DecryptAll: %v", err)
	}
	for i, c := range peeled {
		if FieldToCard(c, p) != -1 {
			t.Fatalf("after local decrypt, card[%d] decoded as plaintext", i)
		}
	}

	other, ok := rings[0].Public("bob")
	if !ok {
		t.Fatal("Public(bob) missing")
	}
	if _, err := other.Decrypt(ed.Cards[0]); err == nil {
		t.Fatal("Public(bob).Decrypt should error (d absent)")
	}
}
