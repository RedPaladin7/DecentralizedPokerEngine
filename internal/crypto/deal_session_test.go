package crypto

import (
	"context"
	"math/big"
	"sync"
	"testing"
	"time"

	"github.com/RedPaladin7/DecentralizedPokerEngine/internal/game"
)

func dealSessionID(ids []string) []byte {
	return SessionID(ids, []byte("deal-session-test"))
}

func tinyDealDeck(n int) []*big.Int {
	out := make([]*big.Int, n)
	for i := 0; i < n; i++ {
		out[i] = big.NewInt(int64(i + 2))
	}
	return out
}

func shuffleTinyDeck(t *testing.T, rings []*Keyring, sid []byte, initial []*big.Int) []*big.Int {
	t.Helper()
	ids := rings[0].SeatOrder()
	keys := make([]*SRAKey, len(rings))
	for i := range rings {
		keys[i] = rings[i].LocalKey()
	}
	sp := NewShuffleProtocol(rings[0].Modulus(), sid)
	sp.NumCards = len(initial)
	final, _, err := sp.RunFullShuffle(ids, keys, initial)
	if err != nil {
		t.Fatalf("RunFullShuffle: %v", err)
	}
	return final
}

func setupTinyDeals(t *testing.T, ids []string, nCards, dealer int) ([]*Keyring, []*DealSession, []*big.Int, []byte) {
	t.Helper()
	rings := nPlayerKeyrings(t, smallPrime, ids)
	sid := dealSessionID(ids)
	final := shuffleTinyDeck(t, rings, sid, tinyDealDeck(nCards))
	sessions := make([]*DealSession, len(ids))
	for i, kr := range rings {
		s, err := newDealSessionN(kr, final, sid, testHandNum, dealer)
		if err != nil {
			t.Fatalf("newDealSessionN %s: %v", ids[i], err)
		}
		sessions[i] = s
	}
	return rings, sessions, final, sid
}

func stubPeel(handNum int64, player string, cardIndex int) *PeelMessage {
	return &PeelMessage{
		HandNum:    handNum,
		PlayerID:   player,
		CardIndex:  cardIndex,
		Ciphertext: big.NewInt(2),
		Result:     big.NewInt(3),
		Proof:      &ZKProof{A: big.NewInt(1), B: big.NewInt(1), S: big.NewInt(1), H: big.NewInt(1)},
	}
}

func collectPeels(s *DealSession, first *PeelMessage) []*PeelMessage {
	var out []*PeelMessage
	if first != nil {
		out = append(out, first)
	}
	out = append(out, s.Outbound()...)
	return out
}

func mustBeginHoles(t *testing.T, s *DealSession) []*PeelMessage {
	t.Helper()
	msg, err := s.BeginHoles()
	if err != nil {
		t.Fatalf("BeginHoles: %v", err)
	}
	return collectPeels(s, msg)
}

func mustBeginStreet(t *testing.T, s *DealSession, street Street) []*PeelMessage {
	t.Helper()
	msg, err := s.BeginStreet(street)
	if err != nil {
		t.Fatalf("BeginStreet: %v", err)
	}
	return collectPeels(s, msg)
}

func mustBeginReveal(t *testing.T, s *DealSession, playerID string) []*PeelMessage {
	t.Helper()
	msg, err := s.BeginReveal(playerID)
	if err != nil {
		t.Fatalf("BeginReveal: %v", err)
	}
	return collectPeels(s, msg)
}

func mustHandlePeel(t *testing.T, s *DealSession, msg *PeelMessage) []*PeelMessage {
	t.Helper()
	out, err := s.HandlePeel(msg)
	if err != nil {
		t.Fatalf("HandlePeel: %v", err)
	}
	return collectPeels(s, out)
}

func driveSequential(t *testing.T, sessions []*DealSession, begin func(*DealSession) (*PeelMessage, error), done func(*DealSession) bool) {
	t.Helper()
	n := len(sessions)
	inbox := make([][]*PeelMessage, n)
	for i, s := range sessions {
		msg, err := begin(s)
		if err != nil {
			t.Fatalf("begin replica %d: %v", i, err)
		}
		for _, m := range collectPeels(s, msg) {
			for j := range sessions {
				if j != i {
					inbox[j] = append(inbox[j], copyPeelMessage(m))
				}
			}
		}
	}
	for step := 0; step < 10000; step++ {
		allDone := true
		progress := false
		for i, s := range sessions {
			if !done(s) {
				allDone = false
			}
			if len(inbox[i]) == 0 {
				continue
			}
			msg := inbox[i][0]
			inbox[i] = inbox[i][1:]
			out, err := s.HandlePeel(msg)
			if err != nil {
				t.Fatalf("HandlePeel replica %d: %v", i, err)
			}
			progress = true
			for _, m := range collectPeels(s, out) {
				for j := range sessions {
					if j != i {
						inbox[j] = append(inbox[j], copyPeelMessage(m))
					}
				}
			}
		}
		if allDone {
			return
		}
		if !progress {
			t.Fatal("deadlock in sequential peel driver")
		}
	}
	t.Fatal("sequential peel driver exceeded step limit")
}

// ── A. Primitives ─────────────────────────────────────────────────────────────

func TestPeel_RejectsPublicOnlyKey(t *testing.T) {
	rings := nPlayerKeyrings(t, smallPrime, []string{"alice", "bob"})
	pub, ok := rings[0].Public("alice")
	if !ok {
		t.Fatal("Public(alice) missing")
	}
	if _, err := Peel(pub, big.NewInt(2), 0, "alice", []byte("sid")); err == nil {
		t.Error("expected Peel to reject public-only key")
	}
	if _, err := ProveDecryption(pub, big.NewInt(2), big.NewInt(3), []byte("sid")); err == nil {
		t.Error("expected ProveDecryption to reject public-only key")
	}
}

func TestPeel_ThenVerifyAndApply(t *testing.T) {
	rings := nPlayerKeyrings(t, smallPrime, []string{"alice", "bob"})
	alice := rings[0].LocalKey()
	bob := rings[1].LocalKey()
	sid := []byte("peel-roundtrip")
	m := big.NewInt(2)

	cA, err := alice.Encrypt(m)
	if err != nil {
		t.Fatalf("alice encrypt: %v", err)
	}
	c, err := bob.Encrypt(cA)
	if err != nil {
		t.Fatalf("bob encrypt: %v", err)
	}

	pd, err := Peel(alice, c, 0, "alice", sid)
	if err != nil {
		t.Fatalf("Peel: %v", err)
	}
	if pd.PlayerID != "alice" || pd.CardIndex != 0 {
		t.Errorf("unexpected pd identity: %+v", pd)
	}

	next, err := VerifyAndApply(c, pd, smallPrime, sid)
	if err != nil {
		t.Fatalf("VerifyAndApply: %v", err)
	}
	plain, err := bob.Decrypt(next)
	if err != nil {
		t.Fatalf("bob decrypt: %v", err)
	}
	if plain.Cmp(m) != 0 {
		t.Fatalf("recovered %s, want %s", plain, m)
	}
}

func TestVerifyAndApply_CiphertextMismatch(t *testing.T) {
	rings := nPlayerKeyrings(t, smallPrime, []string{"alice", "bob"})
	sid := []byte("mismatch")
	c, err := rings[0].LocalKey().Encrypt(big.NewInt(2))
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	pd, err := Peel(rings[0].LocalKey(), c, 0, "alice", sid)
	if err != nil {
		t.Fatalf("Peel: %v", err)
	}
	wrong := new(big.Int).Add(c, big.NewInt(1))
	if _, err := VerifyAndApply(wrong, pd, smallPrime, sid); err == nil {
		t.Fatal("expected ciphertext mismatch")
	}
}

func TestVerifyAndApply_TamperedResult(t *testing.T) {
	rings := nPlayerKeyrings(t, smallPrime, []string{"alice", "bob"})
	sid := []byte("tamper")
	c, err := rings[0].LocalKey().Encrypt(big.NewInt(2))
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	pd, err := Peel(rings[0].LocalKey(), c, 0, "alice", sid)
	if err != nil {
		t.Fatalf("Peel: %v", err)
	}
	wrong := new(big.Int).Add(pd.Result, big.NewInt(1))
	tampered := SubstitutePartialDecryption(pd, wrong)
	if _, err := VerifyAndApply(c, tampered, smallPrime, sid); err == nil {
		t.Fatal("expected tampered result to fail ZK")
	}
}

func TestFinishPublic_NotACard(t *testing.T) {
	if _, err := FinishPublic(big.NewInt(1), SharedPrime()); err == nil {
		t.Fatal("expected FinishPublic to reject non-card")
	}
}

func TestHoleCardIndex_MatchesDealHoleCards(t *testing.T) {
	for _, n := range []int{2, 3, 4} {
		for dealer := 0; dealer < n; dealer++ {
			deckPos := 0
			for round := 0; round < 2; round++ {
				for i := 0; i < n; i++ {
					playerIdx := (dealer + 1 + i) % n
					got, err := HoleCardIndex(n, dealer, playerIdx, round)
					if err != nil {
						t.Fatalf("n=%d dealer=%d: %v", n, dealer, err)
					}
					if got != deckPos {
						t.Errorf("n=%d dealer=%d player=%d round=%d: got %d want %d", n, dealer, playerIdx, round, got, deckPos)
					}
					deckPos++
				}
			}
		}
	}
}

func TestCommunityIndexes_MatchDealCommunityCards(t *testing.T) {
	for _, n := range []int{2, 3, 4} {
		start := CommunityStartPos(n)
		pos := start
		wantFlop := [3]int{}
		var wantTurn, wantRiver int
		for batch, count := range []int{3, 1, 1} {
			pos++ // burn
			for j := 0; j < count; j++ {
				switch {
				case batch == 0:
					wantFlop[j] = pos
				case batch == 1:
					wantTurn = pos
				default:
					wantRiver = pos
				}
				pos++
			}
		}
		flop, err := FlopIndexes(n)
		if err != nil {
			t.Fatalf("FlopIndexes n=%d: %v", n, err)
		}
		if flop != wantFlop {
			t.Errorf("n=%d flop: got %v want %v", n, flop, wantFlop)
		}
		turn, err := TurnIndex(n)
		if err != nil {
			t.Fatalf("TurnIndex n=%d: %v", n, err)
		}
		if turn != wantTurn {
			t.Errorf("n=%d turn: got %d want %d", n, turn, wantTurn)
		}
		river, err := RiverIndex(n)
		if err != nil {
			t.Fatalf("RiverIndex n=%d: %v", n, err)
		}
		if river != wantRiver {
			t.Errorf("n=%d river: got %d want %d", n, river, wantRiver)
		}
	}
}

// ── B. Session FSM ────────────────────────────────────────────────────────────

func TestDealSession_BeginHoles_FirstPeelerProduces(t *testing.T) {
	ids := []string{"alice", "bob"}
	_, sessions, _, _ := setupTinyDeals(t, ids, 8, 0)
	alice, bob := sessions[0], sessions[1]

	aMsgs := mustBeginHoles(t, alice)
	if len(aMsgs) == 0 {
		t.Fatal("alice (first peeler) produced no message")
	}
	if aMsgs[0].CardIndex != 0 {
		t.Errorf("alice card index: got %d want 0", aMsgs[0].CardIndex)
	}
	if aMsgs[0].PlayerID != "alice" {
		t.Errorf("alice player: got %q", aMsgs[0].PlayerID)
	}

	bMsgs := mustBeginHoles(t, bob)
	if len(bMsgs) != 0 {
		t.Fatalf("bob Start produced %d messages, want 0", len(bMsgs))
	}
}

func TestDealSession_TwoPlayers_HolesSequential(t *testing.T) {
	ids := []string{"alice", "bob"}
	_, sessions, _, _ := setupTinyDeals(t, ids, 8, 0)
	driveSequential(t, sessions, (*DealSession).BeginHoles, (*DealSession).HolesDone)

	aliceIdx := map[int]bool{1: true, 3: true} // dealer 0: seat 0 gets i=1 in each round → indexes 1, 3
	bobIdx := map[int]bool{0: true, 2: true}

	alice, bob := sessions[0], sessions[1]
	seen := make(map[string]bool)
	for i := 0; i < 4; i++ {
		ad := alice.testDecoded(i)
		bd := bob.testDecoded(i)
		if aliceIdx[i] {
			if ad == nil {
				t.Errorf("alice missing decode at %d", i)
			} else {
				seen[ad.String()] = true
			}
			if bd != nil {
				t.Errorf("bob decoded alice's hole at %d", i)
			}
		}
		if bobIdx[i] {
			if bd == nil {
				t.Errorf("bob missing decode at %d", i)
			} else {
				seen[bd.String()] = true
			}
			if ad != nil {
				t.Errorf("alice decoded bob's hole at %d", i)
			}
		}
	}
	if len(seen) != 4 {
		t.Errorf("expected 4 unique decoded values, got %d", len(seen))
	}
}

func TestDealSession_WrongPeelerRejected(t *testing.T) {
	ids := []string{"alice", "bob", "carol"}
	_, sessions, _, _ := setupTinyDeals(t, ids, 8, 0)
	alice, bob := sessions[0], sessions[1]
	mustBeginHoles(t, alice)
	mustBeginHoles(t, bob)
	before := bob.testNextPeel()
	// bob is the recipient of card 0, not a peeler
	if _, err := bob.HandlePeel(stubPeel(testHandNum, "bob", 0)); err == nil {
		t.Fatal("expected error for recipient peeling")
	}
	if bob.testNextPeel() != before {
		t.Errorf("nextPeel changed: got %d want %d", bob.testNextPeel(), before)
	}
}

func TestDealSession_WrongHandRejected(t *testing.T) {
	ids := []string{"alice", "bob"}
	_, sessions, _, _ := setupTinyDeals(t, ids, 8, 0)
	bob := sessions[1]
	mustBeginHoles(t, bob)
	if _, err := bob.HandlePeel(stubPeel(2, "alice", 0)); err == nil {
		t.Fatal("expected wrong-hand error")
	}
}

func TestDealSession_WrongCardIndexRejected(t *testing.T) {
	ids := []string{"alice", "bob"}
	_, sessions, _, _ := setupTinyDeals(t, ids, 8, 0)
	bob := sessions[1]
	mustBeginHoles(t, bob)
	if _, err := bob.HandlePeel(stubPeel(testHandNum, "alice", 3)); err == nil {
		t.Fatal("expected wrong-card-index error")
	}
}

func TestDealSession_TamperedZKRejected(t *testing.T) {
	ids := []string{"alice", "bob", "carol"}
	_, sessions, _, _ := setupTinyDeals(t, ids, 8, 0)
	alice, bob := sessions[0], sessions[1]
	aMsgs := mustBeginHoles(t, alice)
	if len(aMsgs) == 0 {
		t.Fatal("alice produced no peel")
	}
	mustBeginHoles(t, bob)
	before := bob.testNextPeel()
	tampered := copyPeelMessage(aMsgs[0])
	tampered.Result = new(big.Int).Add(tampered.Result, big.NewInt(1))
	if _, err := bob.HandlePeel(tampered); err == nil {
		t.Fatal("expected tampered ZK to be rejected")
	}
	if bob.testNextPeel() != before {
		t.Errorf("nextPeel changed: got %d want %d", bob.testNextPeel(), before)
	}
}

func TestDealSession_DuplicateIgnored(t *testing.T) {
	ids := []string{"alice", "bob", "carol"}
	_, sessions, _, _ := setupTinyDeals(t, ids, 8, 0)
	alice, bob := sessions[0], sessions[1]
	aMsgs := mustBeginHoles(t, alice)
	if len(aMsgs) == 0 {
		t.Fatal("alice produced no peel")
	}
	mustBeginHoles(t, bob)
	mustHandlePeel(t, bob, aMsgs[0])
	after := bob.testNextPeel()
	out, err := bob.HandlePeel(aMsgs[0])
	if err != nil {
		t.Fatalf("duplicate should be ignored, got %v", err)
	}
	if out != nil {
		t.Fatal("duplicate produced a message")
	}
	if bob.testNextPeel() != after {
		t.Errorf("nextPeel changed on duplicate: got %d want %d", bob.testNextPeel(), after)
	}
}

func TestDealSession_ConflictingDuplicateRejected(t *testing.T) {
	ids := []string{"alice", "bob", "carol"}
	_, sessions, _, _ := setupTinyDeals(t, ids, 8, 0)
	alice, bob := sessions[0], sessions[1]
	aMsgs := mustBeginHoles(t, alice)
	if len(aMsgs) == 0 {
		t.Fatal("alice produced no peel")
	}
	mustBeginHoles(t, bob)
	mustHandlePeel(t, bob, aMsgs[0])
	conflict := copyPeelMessage(aMsgs[0])
	conflict.Result = new(big.Int).Add(conflict.Result, big.NewInt(1))
	if _, err := bob.HandlePeel(conflict); err == nil {
		t.Fatal("expected conflicting duplicate error")
	}
}

func TestDealSession_BuffersOutOfOrderPeelers(t *testing.T) {
	ids := []string{"alice", "bob", "carol"}
	_, sessions, _, _ := setupTinyDeals(t, ids, 14, 0)
	driveSequential(t, sessions, (*DealSession).BeginHoles, (*DealSession).HolesDone)

	alice, bob, carol := sessions[0], sessions[1], sessions[2]
	aMsgs := mustBeginStreet(t, alice, StreetFlop)
	if len(aMsgs) == 0 {
		t.Fatal("alice produced no flop peel")
	}
	mustBeginStreet(t, bob, StreetFlop)
	mustBeginStreet(t, carol, StreetFlop)

	bMsgs := mustHandlePeel(t, bob, aMsgs[0])
	if len(bMsgs) == 0 {
		t.Fatal("bob produced no flop peel")
	}

	// Carol receives bob before alice — should buffer.
	out, err := carol.HandlePeel(bMsgs[0])
	if err != nil {
		t.Fatalf("buffering bob's peel: %v", err)
	}
	if out != nil {
		t.Fatal("buffered peel should not produce yet")
	}
	if carol.testNextPeel() != 0 {
		t.Fatalf("nextPeel after buffer: got %d want 0", carol.testNextPeel())
	}

	cMsgs := mustHandlePeel(t, carol, aMsgs[0])
	if len(cMsgs) == 0 {
		t.Fatal("carol should produce after drain")
	}
	flop, err := FlopIndexes(3)
	if err != nil {
		t.Fatal(err)
	}
	// Drive remaining so the first flop card completes on a sequential replica for comparison.
	mustHandlePeel(t, alice, bMsgs[0])
	mustHandlePeel(t, alice, cMsgs[0])
	mustHandlePeel(t, bob, cMsgs[0])

	if alice.testDecoded(flop[0]) == nil || carol.testDecoded(flop[0]) == nil {
		t.Fatal("first flop card not decoded")
	}
	if alice.testDecoded(flop[0]).Cmp(carol.testDecoded(flop[0])) != 0 {
		t.Fatal("carol's decoded flop card differs from alice")
	}
}

func TestDealSession_UnknownPlayerRejected(t *testing.T) {
	ids := []string{"alice", "bob"}
	_, sessions, _, _ := setupTinyDeals(t, ids, 8, 0)
	bob := sessions[1]
	mustBeginHoles(t, bob)
	if _, err := bob.HandlePeel(stubPeel(testHandNum, "mallory", 0)); err == nil {
		t.Fatal("expected unknown player error")
	}
}

func TestDealSession_RecipientDoesNotPublishLastDecrypt(t *testing.T) {
	ids := []string{"alice", "bob"}
	_, sessions, _, _ := setupTinyDeals(t, ids, 8, 0)
	alice, bob := sessions[0], sessions[1]

	published := mustBeginHoles(t, alice)
	mustBeginHoles(t, bob)

	inbox := []*PeelMessage{copyPeelMessage(published[0])}
	for len(inbox) > 0 {
		msg := inbox[0]
		inbox = inbox[1:]
		var outs []*PeelMessage
		if msg.PlayerID == "alice" {
			outs = mustHandlePeel(t, bob, msg)
		} else {
			outs = mustHandlePeel(t, alice, msg)
		}
		published = append(published, outs...)
		inbox = append(inbox, outs...)
		if alice.HolesDone() && bob.HolesDone() {
			break
		}
	}
	if !alice.HolesDone() || !bob.HolesDone() {
		t.Fatal("holes not done")
	}

	card0Count := 0
	for _, m := range published {
		if m.CardIndex != 0 {
			continue
		}
		card0Count++
		if m.PlayerID == "bob" {
			t.Fatal("recipient bob published a peel for card 0")
		}
	}
	if card0Count != 1 {
		t.Fatalf("card 0 peel count: got %d want 1 (n-1)", card0Count)
	}
	if bob.testDecoded(0) == nil {
		t.Fatal("bob (recipient) did not decode card 0")
	}
}

// ── C. Fake-net ───────────────────────────────────────────────────────────────

type fakePeelBus struct {
	chans map[string]chan *PeelMessage
}

func newFakePeelBus(ids []string) *fakePeelBus {
	b := &fakePeelBus{chans: make(map[string]chan *PeelMessage, len(ids))}
	for _, id := range ids {
		b.chans[id] = make(chan *PeelMessage, 32)
	}
	return b
}

func (b *fakePeelBus) Broadcast(from string, msg *PeelMessage) {
	for id, ch := range b.chans {
		if id == from {
			continue
		}
		ch <- copyPeelMessage(msg)
	}
}

func broadcastPeel(bus *fakePeelBus, from string, sess *DealSession, first *PeelMessage) {
	if first != nil {
		bus.Broadcast(from, first)
	}
	for _, extra := range sess.Outbound() {
		bus.Broadcast(from, extra)
	}
}

func runPeelReplica(ctx context.Context, id string, sess *DealSession, bus *fakePeelBus, begin func(*DealSession) (*PeelMessage, error), done func(*DealSession) bool) error {
	out, err := begin(sess)
	if err != nil {
		return err
	}
	broadcastPeel(bus, id, sess, out)
	for {
		if done(sess) {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case msg := <-bus.chans[id]:
			out, err := sess.HandlePeel(msg)
			if err != nil {
				return err
			}
			broadcastPeel(bus, id, sess, out)
		}
	}
}

func runPeelPhase(t *testing.T, timeout time.Duration, rings []*Keyring, sessions []*DealSession, begin func(*DealSession) (*PeelMessage, error), done func(*DealSession) bool) {
	t.Helper()
	ids := rings[0].SeatOrder()
	bus := newFakePeelBus(ids)
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	var wg sync.WaitGroup
	errCh := make(chan error, len(ids))
	for i, id := range ids {
		wg.Add(1)
		go func(id string, sess *DealSession) {
			defer wg.Done()
			if err := runPeelReplica(ctx, id, sess, bus, begin, done); err != nil {
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
		if !done(s) {
			t.Fatalf("replica %s not done", ids[i])
		}
	}
}

func holeIndexesForSeat(n, dealer, player int) [2]int {
	i0, _ := HoleCardIndex(n, dealer, player, 0)
	i1, _ := HoleCardIndex(n, dealer, player, 1)
	return [2]int{i0, i1}
}

func TestDealSession_FakeNet_3Players_Holes(t *testing.T) {
	ids := []string{"alice", "bob", "carol"}
	rings, sessions, _, _ := setupTinyDeals(t, ids, 8, 0)
	runPeelPhase(t, 30*time.Second, rings, sessions, (*DealSession).BeginHoles, (*DealSession).HolesDone)

	union := make(map[string]bool)
	n, dealer := 3, 0
	for p, s := range sessions {
		own := holeIndexesForSeat(n, dealer, p)
		ownSet := map[int]bool{own[0]: true, own[1]: true}
		decoded := 0
		for i := 0; i < 6; i++ {
			v := s.testDecoded(i)
			if ownSet[i] {
				if v == nil {
					t.Errorf("%s missing own hole index %d", ids[p], i)
				} else {
					decoded++
					union[v.String()] = true
				}
			} else if v != nil {
				t.Errorf("%s decoded opponent hole index %d", ids[p], i)
			}
		}
		if decoded != 2 {
			t.Errorf("%s decoded %d hole cards, want 2", ids[p], decoded)
		}
	}
	if len(union) != 6 {
		t.Errorf("expected 6 unique hole values, got %d", len(union))
	}
}

func TestDealSession_FakeNet_3Players_Community(t *testing.T) {
	ids := []string{"alice", "bob", "carol"}
	rings, sessions, _, _ := setupTinyDeals(t, ids, 14, 0)
	runPeelPhase(t, 30*time.Second, rings, sessions, (*DealSession).BeginHoles, (*DealSession).HolesDone)
	runPeelPhase(t, 30*time.Second, rings, sessions, func(s *DealSession) (*PeelMessage, error) {
		return s.BeginStreet(StreetFlop)
	}, (*DealSession).StreetDone)
	runPeelPhase(t, 30*time.Second, rings, sessions, func(s *DealSession) (*PeelMessage, error) {
		return s.BeginStreet(StreetTurn)
	}, (*DealSession).StreetDone)
	runPeelPhase(t, 30*time.Second, rings, sessions, func(s *DealSession) (*PeelMessage, error) {
		return s.BeginStreet(StreetRiver)
	}, (*DealSession).StreetDone)

	flop, err := FlopIndexes(3)
	if err != nil {
		t.Fatal(err)
	}
	for _, idx := range flop {
		base := sessions[0].testDecoded(idx)
		if base == nil {
			t.Fatalf("alice missing flop index %d", idx)
		}
		for i := 1; i < 3; i++ {
			v := sessions[i].testDecoded(idx)
			if v == nil || v.Cmp(base) != 0 {
				t.Fatalf("replica %s flop[%d] mismatch", ids[i], idx)
			}
		}
	}
	// Opponent holes stay private.
	for p, s := range sessions {
		own := holeIndexesForSeat(3, 0, p)
		ownSet := map[int]bool{own[0]: true, own[1]: true}
		for i := 0; i < 6; i++ {
			if ownSet[i] {
				continue
			}
			if s.testDecoded(i) != nil {
				t.Errorf("%s decoded opponent hole %d after community", ids[p], i)
			}
		}
	}
}

func setupProduction2Player(t *testing.T) ([]*Keyring, []*DealSession, *EncryptedDeck, [][2]game.Card) {
	t.Helper()
	ids := []string{"alice", "bob"}
	rings := nPlayerKeyrings(t, SharedPrime(), ids)
	sid := dealSessionID(ids)
	keys := []*SRAKey{rings[0].LocalKey(), rings[1].LocalKey()}
	sp := NewShuffleProtocol(SharedPrime(), sid)
	final, _, err := sp.RunFullShuffle(ids, keys, BuildPlaintextDeck(SharedPrime()))
	if err != nil {
		t.Fatalf("RunFullShuffle: %v", err)
	}
	ed, err := NewEncryptedDeck(final, SharedPrime(), sid)
	if err != nil {
		t.Fatalf("NewEncryptedDeck: %v", err)
	}
	dp := NewDealProtocol(ed, ids, keys)
	oracle, err := dp.DealHoleCards(0)
	if err != nil {
		t.Fatalf("oracle DealHoleCards: %v", err)
	}
	sessions := make([]*DealSession, 2)
	for i, kr := range rings {
		s, err := NewDealSession(kr, ed, testHandNum, 0)
		if err != nil {
			t.Fatalf("NewDealSession: %v", err)
		}
		sessions[i] = s
	}
	return rings, sessions, ed, oracle
}

func TestDealSession_FakeNet_2Players_Production(t *testing.T) {
	rings, sessions, ed, oracle := setupProduction2Player(t)
	ids := []string{"alice", "bob"}
	timeout := 3 * time.Minute

	runPeelPhase(t, timeout, rings, sessions, (*DealSession).BeginHoles, (*DealSession).HolesDone)

	aliceHoles, err := sessions[0].LocalHoles()
	if err != nil {
		t.Fatalf("alice LocalHoles: %v", err)
	}
	bobHoles, err := sessions[1].LocalHoles()
	if err != nil {
		t.Fatalf("bob LocalHoles: %v", err)
	}
	if aliceHoles != oracle[0] {
		t.Fatalf("alice holes %v != oracle %v", aliceHoles, oracle[0])
	}
	if bobHoles != oracle[1] {
		t.Fatalf("bob holes %v != oracle %v", bobHoles, oracle[1])
	}
	if aliceHoles == bobHoles {
		t.Fatal("both players have the same hole cards")
	}
	if _, err := sessions[0].RevealedHoles("bob"); err == nil {
		t.Fatal("alice should not have bob's holes before reveal")
	}

	// Privacy: one local decrypt of the original ciphertext is not a card.
	bobIdx, err := HoleCardIndex(2, 0, 1, 0)
	if err != nil {
		t.Fatal(err)
	}
	ct, err := ed.CardAt(bobIdx)
	if err != nil {
		t.Fatal(err)
	}
	oneLayer, err := rings[0].LocalKey().Decrypt(ct)
	if err != nil {
		t.Fatalf("alice decrypt opponent ct: %v", err)
	}
	if FieldToCard(oneLayer, SharedPrime()) != -1 {
		t.Fatal("alice single-layer decrypt of bob's card decoded as plaintext")
	}
	pub, ok := rings[0].Public("bob")
	if !ok {
		t.Fatal("Public(bob) missing")
	}
	if _, err := pub.Decrypt(ct); err == nil {
		t.Fatal("Public(bob).Decrypt should error")
	}

	runPeelPhase(t, timeout, rings, sessions, func(s *DealSession) (*PeelMessage, error) {
		return s.BeginStreet(StreetFlop)
	}, (*DealSession).StreetDone)
	flop0, err := sessions[0].CommunityCards()
	if err != nil {
		t.Fatalf("alice community: %v", err)
	}
	flop1, err := sessions[1].CommunityCards()
	if err != nil {
		t.Fatalf("bob community: %v", err)
	}
	if len(flop0) != 3 || len(flop1) != 3 {
		t.Fatalf("flop len: alice %d bob %d", len(flop0), len(flop1))
	}
	for i := range flop0 {
		if flop0[i] != flop1[i] {
			t.Fatalf("flop[%d] mismatch: %v vs %v", i, flop0[i], flop1[i])
		}
	}

	runPeelPhase(t, timeout, rings, sessions, func(s *DealSession) (*PeelMessage, error) {
		return s.BeginStreet(StreetTurn)
	}, (*DealSession).StreetDone)
	runPeelPhase(t, timeout, rings, sessions, func(s *DealSession) (*PeelMessage, error) {
		return s.BeginStreet(StreetRiver)
	}, (*DealSession).StreetDone)

	runPeelPhase(t, timeout, rings, sessions, func(s *DealSession) (*PeelMessage, error) {
		return s.BeginReveal("bob")
	}, (*DealSession).RevealDone)
	revealed, err := sessions[0].RevealedHoles("bob")
	if err != nil {
		t.Fatalf("alice RevealedHoles(bob): %v", err)
	}
	if revealed != bobHoles {
		t.Fatalf("revealed bob holes %v != bob local %v", revealed, bobHoles)
	}

	runPeelPhase(t, timeout, rings, sessions, func(s *DealSession) (*PeelMessage, error) {
		return s.BeginReveal("alice")
	}, (*DealSession).RevealDone)

	comm, err := sessions[0].CommunityCards()
	if err != nil || len(comm) != 5 {
		t.Fatalf("community cards: %v len %d", err, len(comm))
	}
	for i, id := range ids {
		holes, err := sessions[0].RevealedHoles(id)
		if err != nil {
			t.Fatalf("replica0 RevealedHoles(%s): %v", id, err)
		}
		var seven [7]game.Card
		seven[0], seven[1] = holes[0], holes[1]
		copy(seven[2:], comm)
		h0 := game.EvaluateBest7(seven)
		holes1, err := sessions[1].RevealedHoles(id)
		if err != nil {
			t.Fatalf("replica1 RevealedHoles(%s): %v", id, err)
		}
		var seven1 [7]game.Card
		seven1[0], seven1[1] = holes1[0], holes1[1]
		comm1, err := sessions[1].CommunityCards()
		if err != nil {
			t.Fatal(err)
		}
		copy(seven1[2:], comm1)
		h1 := game.EvaluateBest7(seven1)
		if h0.Compare(h1) != 0 {
			t.Errorf("EvaluateBest7 mismatch for %s", id)
		}
		_ = i
	}
}

func TestDealSession_HolePrivacy_PublicCannotFinish(t *testing.T) {
	// Covered in the production fake-net test; keep a focused holes-only check.
	rings, sessions, ed, _ := setupProduction2Player(t)
	runPeelPhase(t, 3*time.Minute, rings, sessions, (*DealSession).BeginHoles, (*DealSession).HolesDone)

	bobIdx, _ := HoleCardIndex(2, 0, 1, 0)
	ct, _ := ed.CardAt(bobIdx)
	oneLayer, err := rings[0].LocalKey().Decrypt(ct)
	if err != nil {
		t.Fatal(err)
	}
	if FieldToCard(oneLayer, SharedPrime()) != -1 {
		t.Fatal("non-recipient decoded opponent hole from original ciphertext")
	}
	pub, _ := rings[0].Public("bob")
	if _, err := pub.Decrypt(ct); err == nil {
		t.Fatal("Public(bob).Decrypt should error")
	}
	aliceHoles, err := sessions[0].LocalHoles()
	if err != nil {
		t.Fatal(err)
	}
	bobHoles, err := sessions[1].LocalHoles()
	if err != nil {
		t.Fatal(err)
	}
	if aliceHoles == bobHoles {
		t.Fatal("LocalHoles leaked opponent cards")
	}
}

func TestDealSession_Showdown_EvaluateBest7Agrees(t *testing.T) {
	rings, sessions, _, _ := setupProduction2Player(t)
	timeout := 3 * time.Minute
	runPeelPhase(t, timeout, rings, sessions, (*DealSession).BeginHoles, (*DealSession).HolesDone)
	for _, st := range []Street{StreetFlop, StreetTurn, StreetRiver} {
		st := st
		runPeelPhase(t, timeout, rings, sessions, func(s *DealSession) (*PeelMessage, error) {
			return s.BeginStreet(st)
		}, (*DealSession).StreetDone)
	}
	for _, pid := range []string{"alice", "bob"} {
		pid := pid
		runPeelPhase(t, timeout, rings, sessions, func(s *DealSession) (*PeelMessage, error) {
			return s.BeginReveal(pid)
		}, (*DealSession).RevealDone)
	}

	comm0, err := sessions[0].CommunityCards()
	if err != nil || len(comm0) != 5 {
		t.Fatalf("community: %v", err)
	}
	var best0, best1 game.EvaluatedHand
	for i, pid := range []string{"alice", "bob"} {
		h0, err := sessions[0].RevealedHoles(pid)
		if err != nil {
			t.Fatal(err)
		}
		h1, err := sessions[1].RevealedHoles(pid)
		if err != nil {
			t.Fatal(err)
		}
		if h0 != h1 {
			t.Fatalf("revealed holes differ for %s", pid)
		}
		var seven0, seven1 [7]game.Card
		seven0[0], seven0[1] = h0[0], h0[1]
		seven1[0], seven1[1] = h1[0], h1[1]
		comm1, _ := sessions[1].CommunityCards()
		copy(seven0[2:], comm0)
		copy(seven1[2:], comm1)
		e0 := game.EvaluateBest7(seven0)
		e1 := game.EvaluateBest7(seven1)
		if e0.Compare(e1) != 0 {
			t.Fatalf("eval mismatch for %s", pid)
		}
		if i == 0 {
			best0 = e0
		} else {
			best1 = e0
		}
	}
	_ = best0
	_ = best1
}
