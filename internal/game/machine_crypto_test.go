package game

import (
	"testing"
)

func newCryptoTestGame(n int, stack int64) (*Machine, []*Player) {
	m, players := newTestGame(n, stack)
	m.State.Deck = nil
	return m, players
}

func playUntilWait(t *testing.T, m *Machine) {
	t.Helper()
	for i := 0; i < 30; i++ {
		ph := m.State.Phase
		if ph == PhaseAwaitingStreet || ph == PhaseShowdown || ph == PhaseSettled {
			return
		}
		current := m.State.CurrentPlayer()
		if current == nil {
			t.Fatalf("no current player in phase %s", ph)
		}
		toCall := m.State.CurrentBet - current.CurrentBet
		var a Action
		if toCall > 0 {
			a = Action{PlayerID: current.ID, Type: ActionCall}
		} else {
			a = Action{PlayerID: current.ID, Type: ActionCheck}
		}
		if err := m.ApplyAction(a); err != nil {
			t.Fatalf("ApplyAction: %v (phase %s)", err, m.State.Phase)
		}
	}
	t.Fatalf("stuck in phase %s", m.State.Phase)
}

func playCryptoCheckDown(t *testing.T, m *Machine, flop []Card, turn, river Card) {
	t.Helper()
	playUntilWait(t, m)
	if err := m.ApplyStreet(flop); err != nil {
		t.Fatalf("flop: %v", err)
	}
	playUntilWait(t, m)
	if err := m.ApplyStreet([]Card{turn}); err != nil {
		t.Fatalf("turn: %v", err)
	}
	playUntilWait(t, m)
	if err := m.ApplyStreet([]Card{river}); err != nil {
		t.Fatalf("river: %v", err)
	}
	playUntilWait(t, m)
}

func TestPhaseAwaitingStreet_String(t *testing.T) {
	s := PhaseAwaitingStreet.String()
	if s == "" {
		t.Fatal("PhaseAwaitingStreet.String() is empty")
	}
	if s != "Awaiting Street" {
		t.Errorf("got %q, want %q", s, "Awaiting Street")
	}
}

func TestStartHandCrypto_OnlySeat0Holes_Preflop(t *testing.T) {
	m, players := newCryptoTestGame(2, 200)
	players[0].HoleCards = [2]Card{{Ace, Hearts}, {King, Hearts}}
	if err := m.StartHandCrypto(); err != nil {
		t.Fatalf("StartHandCrypto: %v", err)
	}
	if m.State.Phase != PhasePreFlop {
		t.Errorf("expected PreFlop, got %s", m.State.Phase)
	}
	if players[1].HoleCards[0] != (Card{}) || players[1].HoleCards[1] != (Card{}) {
		t.Errorf("seat 1 holes should still be empty, got %v", players[1].HoleCards)
	}
}

func TestStartHand_NilDeckRejected(t *testing.T) {
	m, _ := newTestGame(2, 200)
	m.State.Deck = nil
	if err := m.StartHand(); err == nil {
		t.Fatal("expected error from StartHand with nil deck")
	}
}

func TestDealFlop_NilDeck_NoPanic(t *testing.T) {
	m, _ := newTestGame(2, 200)
	m.State.Deck = nil
	if err := m.dealFlop(); err == nil {
		t.Fatal("expected error from dealFlop with nil deck")
	}
	if _, err := (*Deck)(nil).Deal(); err == nil {
		t.Fatal("expected error from nil Deck.Deal")
	}
}

func TestCrypto_PreflopEnd_WaitsForStreet(t *testing.T) {
	m, _ := newCryptoTestGame(2, 200)
	if err := m.StartHandCrypto(); err != nil {
		t.Fatal(err)
	}
	playUntilWait(t, m)
	if m.State.Phase != PhaseAwaitingStreet {
		t.Fatalf("expected AwaitingStreet, got %s", m.State.Phase)
	}
	if !m.NeedsStreet() {
		t.Error("NeedsStreet() should be true")
	}
	if m.PendingStreetCount() != 3 {
		t.Errorf("PendingStreetCount=%d, want 3", m.PendingStreetCount())
	}
	if len(m.State.CommunityCards) != 0 {
		t.Errorf("community should be empty, got %d", len(m.State.CommunityCards))
	}
	current := m.State.CurrentPlayer()
	pid := m.State.Players[0].ID
	if current != nil {
		pid = current.ID
	}
	if err := m.ApplyAction(Action{PlayerID: pid, Type: ActionCheck}); err == nil {
		t.Fatal("expected ApplyAction to reject PhaseAwaitingStreet")
	}
}

func TestCrypto_ApplyStreet_FlopTurnRiver(t *testing.T) {
	m, players := newCryptoTestGame(2, 200)
	players[0].HoleCards = [2]Card{{Ace, Spades}, {Ace, Hearts}}
	if err := m.StartHandCrypto(); err != nil {
		t.Fatal(err)
	}

	playUntilWait(t, m)
	flop := []Card{{Two, Spades}, {Seven, Hearts}, {Nine, Diamonds}}
	if err := m.ApplyStreet(flop); err != nil {
		t.Fatalf("flop: %v", err)
	}
	if m.State.Phase != PhaseFlop {
		t.Errorf("expected Flop, got %s", m.State.Phase)
	}
	if len(m.State.CommunityCards) != 3 {
		t.Errorf("expected 3 community, got %d", len(m.State.CommunityCards))
	}

	playUntilWait(t, m)
	if m.State.Phase != PhaseAwaitingStreet {
		t.Fatalf("expected AwaitingStreet after flop betting, got %s", m.State.Phase)
	}
	if m.PendingStreetCount() != 1 {
		t.Errorf("PendingStreetCount=%d, want 1", m.PendingStreetCount())
	}
	if err := m.ApplyStreet([]Card{{Three, Clubs}}); err != nil {
		t.Fatalf("turn: %v", err)
	}
	if m.State.Phase != PhaseTurn {
		t.Errorf("expected Turn, got %s", m.State.Phase)
	}

	playUntilWait(t, m)
	if err := m.ApplyStreet([]Card{{Four, Spades}}); err != nil {
		t.Fatalf("river: %v", err)
	}
	if m.State.Phase != PhaseRiver {
		t.Errorf("expected River, got %s", m.State.Phase)
	}

	playUntilWait(t, m)
	if m.State.Phase != PhaseShowdown {
		t.Fatalf("expected Showdown (seat 1 holes empty), got %s", m.State.Phase)
	}
	if !m.NeedsReveal() {
		t.Error("NeedsReveal() should be true")
	}
	if players[1].HoleCards[0] != (Card{}) {
		t.Error("seat 1 holes should still be empty")
	}
}

func TestCrypto_ApplyStreet_WrongLengthRejected(t *testing.T) {
	m, _ := newCryptoTestGame(2, 200)
	if err := m.StartHandCrypto(); err != nil {
		t.Fatal(err)
	}
	playUntilWait(t, m)
	if err := m.ApplyStreet([]Card{{Two, Spades}}); err == nil {
		t.Fatal("expected error applying 1 card as flop")
	}
	if m.State.Phase != PhaseAwaitingStreet {
		t.Errorf("should still be AwaitingStreet, got %s", m.State.Phase)
	}
	if len(m.State.CommunityCards) != 0 {
		t.Errorf("community should still be empty, got %d", len(m.State.CommunityCards))
	}
}

func TestCrypto_ApplyStreet_NotAwaitingRejected(t *testing.T) {
	m, _ := newCryptoTestGame(2, 200)
	if err := m.StartHandCrypto(); err != nil {
		t.Fatal(err)
	}
	flop := []Card{{Two, Spades}, {Seven, Hearts}, {Nine, Diamonds}}
	if err := m.ApplyStreet(flop); err == nil {
		t.Fatal("expected error applying street during preflop")
	}
}

func TestCrypto_ApplyStreet_PlaintextRejected(t *testing.T) {
	m, _ := newTestGame(2, 200)
	if err := m.StartHand(); err != nil {
		t.Fatal(err)
	}
	flop := []Card{{Two, Spades}, {Seven, Hearts}, {Nine, Diamonds}}
	if err := m.ApplyStreet(flop); err == nil {
		t.Fatal("expected error applying street in plaintext mode")
	}
}

func TestCrypto_Showdown_WaitsForReveal(t *testing.T) {
	m, players := newCryptoTestGame(2, 200)
	players[0].HoleCards = [2]Card{{Ace, Spades}, {Ace, Hearts}}
	start0, start1 := players[0].Stack, players[1].Stack
	if err := m.StartHandCrypto(); err != nil {
		t.Fatal(err)
	}
	playCryptoCheckDown(t, m,
		[]Card{{Two, Spades}, {Seven, Hearts}, {Nine, Diamonds}},
		Card{Three, Clubs},
		Card{Four, Spades},
	)
	if m.State.Phase != PhaseShowdown {
		t.Fatalf("expected Showdown, got %s", m.State.Phase)
	}
	if !m.NeedsReveal() {
		t.Fatal("NeedsReveal() should be true")
	}
	missing := m.MissingRevealIDs()
	if len(missing) != 1 || missing[0] != players[1].ID {
		t.Errorf("MissingRevealIDs=%v, want [%s]", missing, players[1].ID)
	}
	if len(m.State.Payouts) != 0 {
		t.Errorf("payouts should be empty before reveal, got %v", m.State.Payouts)
	}
	// Stacks dropped by the blinds/call; those chips sit in the pot until reveal.
	if players[0].Stack+players[1].Stack+TotalPot(m.State.Pots) != start0+start1 {
		t.Errorf("chip conservation failed before reveal: stacks %d+%d pot %d start %d+%d",
			players[0].Stack, players[1].Stack, TotalPot(m.State.Pots), start0, start1)
	}
	if players[0].Stack >= start0 {
		t.Error("seat 0 should have posted/called into the pot")
	}
}

func TestCrypto_ApplyHoleReveal_ThenPayoutsMatchControl(t *testing.T) {
	holes0 := [2]Card{{Ace, Spades}, {Ace, Hearts}}
	holes1 := [2]Card{{King, Spades}, {King, Hearts}}
	flop := []Card{{Two, Spades}, {Seven, Hearts}, {Nine, Diamonds}}
	turn := Card{Three, Clubs}
	river := Card{Four, Spades}

	a, pa := newCryptoTestGame(2, 200)
	pa[0].HoleCards = holes0
	if err := a.StartHandCrypto(); err != nil {
		t.Fatal(err)
	}
	playCryptoCheckDown(t, a, flop, turn, river)
	if err := a.ApplyHoleReveal(pa[1].ID, holes1); err != nil {
		t.Fatalf("ApplyHoleReveal: %v", err)
	}

	b, pb := newCryptoTestGame(2, 200)
	pb[0].HoleCards = holes0
	pb[1].HoleCards = holes1
	if err := b.StartHandCrypto(); err != nil {
		t.Fatal(err)
	}
	playCryptoCheckDown(t, b, flop, turn, river)

	if a.State.Phase != PhaseSettled || b.State.Phase != PhaseSettled {
		t.Fatalf("expected both settled, got A=%s B=%s", a.State.Phase, b.State.Phase)
	}
	for i := range pa {
		if pa[i].Stack != pb[i].Stack {
			t.Errorf("stack[%d]: A=%d B=%d", i, pa[i].Stack, pb[i].Stack)
		}
		if a.State.Payouts[pa[i].ID] != b.State.Payouts[pb[i].ID] {
			t.Errorf("payouts[%s]: A=%d B=%d", pa[i].ID, a.State.Payouts[pa[i].ID], b.State.Payouts[pb[i].ID])
		}
	}
	if a.State.Payouts[pa[0].ID] == 0 {
		t.Error("seat 0 (pocket aces) should have won the pot")
	}
}

func TestCrypto_ApplyHoleReveal_IdempotentLocal(t *testing.T) {
	m, players := newCryptoTestGame(2, 200)
	local := [2]Card{{Ace, Spades}, {Ace, Hearts}}
	players[0].HoleCards = local
	if err := m.StartHandCrypto(); err != nil {
		t.Fatal(err)
	}
	playCryptoCheckDown(t, m,
		[]Card{{Two, Spades}, {Seven, Hearts}, {Nine, Diamonds}},
		Card{Three, Clubs},
		Card{Four, Spades},
	)
	if err := m.ApplyHoleReveal(players[0].ID, local); err != nil {
		t.Fatalf("idempotent reveal: %v", err)
	}
	if m.State.Phase != PhaseShowdown {
		t.Errorf("still waiting for seat 1, got %s", m.State.Phase)
	}
	wrong := [2]Card{{Queen, Spades}, {Queen, Hearts}}
	if err := m.ApplyHoleReveal(players[0].ID, wrong); err == nil {
		t.Fatal("expected equivocation error")
	}
}

func TestCrypto_FoldToWinner_NoReveal(t *testing.T) {
	m, players := newCryptoTestGame(2, 200)
	players[0].HoleCards = [2]Card{{Ace, Spades}, {Ace, Hearts}}
	if err := m.StartHandCrypto(); err != nil {
		t.Fatal(err)
	}
	// Heads-up SB (seat 0) raises so BB (seat 1) can fold. Empty opponent holes
	// must not block settle — fold-out does not need a reveal.
	sb := m.State.CurrentPlayer()
	if sb == nil || sb.ID != players[0].ID {
		t.Fatalf("expected seat 0 to act first, got %v", sb)
	}
	if err := m.ApplyAction(Action{PlayerID: sb.ID, Type: ActionRaise, Amount: 20}); err != nil {
		t.Fatalf("raise: %v", err)
	}
	bb := m.State.CurrentPlayer()
	if bb == nil || bb.ID != players[1].ID {
		t.Fatalf("expected seat 1 to act after raise, got %v", bb)
	}
	if err := m.ApplyAction(Action{PlayerID: bb.ID, Type: ActionFold}); err != nil {
		t.Fatalf("fold: %v", err)
	}
	if m.State.Phase != PhaseSettled {
		t.Fatalf("expected Settled, got %s", m.State.Phase)
	}
	if m.NeedsReveal() {
		t.Error("NeedsReveal should be false after fold-out")
	}
	if m.State.Payouts[players[0].ID] == 0 {
		t.Error("seat 0 should have won the pot")
	}
}

func TestCrypto_FoldToWinner_ThreePlayers(t *testing.T) {
	m, players := newCryptoTestGame(3, 200)
	players[0].HoleCards = [2]Card{{Ace, Spades}, {Ace, Hearts}}
	if err := m.StartHandCrypto(); err != nil {
		t.Fatal(err)
	}
	// UTG (seat 0) raises so SB/BB must respond; they fold. Empty holes on
	// folded seats must not block settle.
	for i := 0; m.State.Phase != PhaseSettled; i++ {
		if i > 10 {
			t.Fatalf("did not settle, stuck at %s", m.State.Phase)
		}
		current := m.State.CurrentPlayer()
		if current == nil {
			t.Fatalf("no current player in phase %s", m.State.Phase)
		}
		if current.ID == players[0].ID {
			if err := m.ApplyAction(Action{PlayerID: current.ID, Type: ActionRaise, Amount: 20}); err != nil {
				t.Fatal(err)
			}
		} else {
			if err := m.ApplyAction(Action{PlayerID: current.ID, Type: ActionFold}); err != nil {
				t.Fatal(err)
			}
		}
	}
	if m.NeedsReveal() {
		t.Error("NeedsReveal should be false")
	}
	if m.State.Payouts[players[0].ID] == 0 {
		t.Error("seat 0 should have won the pot")
	}
}

func TestCrypto_AllInRunout_WaitsEachStreet(t *testing.T) {
	m, players := newCryptoTestGame(2, 10)
	players[0].HoleCards = [2]Card{{Ace, Spades}, {Ace, Hearts}}
	if err := m.StartHandCrypto(); err != nil {
		t.Fatal(err)
	}
	current := m.State.CurrentPlayer()
	if current == nil {
		t.Fatal("expected SB to act")
	}
	if err := m.ApplyAction(Action{PlayerID: current.ID, Type: ActionAllIn}); err != nil {
		t.Fatalf("all-in: %v", err)
	}
	if m.State.Phase != PhaseAwaitingStreet {
		t.Fatalf("expected AwaitingStreet after all-in, got %s", m.State.Phase)
	}
	if m.PendingStreetCount() != 3 {
		t.Errorf("PendingStreetCount=%d, want 3", m.PendingStreetCount())
	}

	if err := m.ApplyStreet([]Card{{Two, Spades}, {Seven, Hearts}, {Nine, Diamonds}}); err != nil {
		t.Fatalf("flop: %v", err)
	}
	if m.State.Phase != PhaseAwaitingStreet {
		t.Fatalf("expected immediate turn wait, got %s", m.State.Phase)
	}
	if m.PendingStreetCount() != 1 {
		t.Errorf("PendingStreetCount=%d, want 1", m.PendingStreetCount())
	}

	if err := m.ApplyStreet([]Card{{Three, Clubs}}); err != nil {
		t.Fatalf("turn: %v", err)
	}
	if m.State.Phase != PhaseAwaitingStreet {
		t.Fatalf("expected immediate river wait, got %s", m.State.Phase)
	}

	if err := m.ApplyStreet([]Card{{Four, Spades}}); err != nil {
		t.Fatalf("river: %v", err)
	}
	if m.State.Phase != PhaseShowdown {
		t.Fatalf("expected Showdown, got %s", m.State.Phase)
	}
	if !m.NeedsReveal() {
		t.Error("NeedsReveal should be true (seat 1 holes empty)")
	}
}
