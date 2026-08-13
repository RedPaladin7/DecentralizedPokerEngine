package network

import (
	"context"
	"math/big"
	"sync"
	"testing"
	"time"

	pokercrypto "github.com/RedPaladin7/DecentralizedPokerEngine/internal/crypto"
	"github.com/RedPaladin7/DecentralizedPokerEngine/internal/game"
)

func TestShuffleMessageWire_RoundTrip(t *testing.T) {
	msg := &pokercrypto.ShuffleMessage{
		HandNum:  2,
		PlayerID: "alice",
		OutputDeck: []*big.Int{
			big.NewInt(2),
			big.NewInt(3),
			big.NewInt(4),
		},
		Commitment: &pokercrypto.Commitment{
			Hash:  []byte{0x01, 0x02, 0x03},
			Nonce: []byte{0x0a, 0x0b},
		},
	}
	pb := ShuffleMessageToWire("table-1", msg)
	if pb == nil {
		t.Fatal("ToWire returned nil")
	}
	if pb.TableId != "table-1" || pb.HandNum != 2 || pb.PlayerId != "alice" {
		t.Fatalf("proto header mismatch: %+v", pb)
	}
	back := ShuffleMessageFromWire(pb)
	if back.HandNum != msg.HandNum || back.PlayerID != msg.PlayerID {
		t.Fatalf("header round-trip mismatch: %+v", back)
	}
	if len(back.OutputDeck) != 3 || back.OutputDeck[1].Cmp(big.NewInt(3)) != 0 {
		t.Fatalf("deck round-trip mismatch: %v", back.OutputDeck)
	}
	if back.Commitment == nil || string(back.Commitment.Hash) != string(msg.Commitment.Hash) {
		t.Fatalf("commitment hash mismatch: %v", back.Commitment)
	}
	if string(back.Commitment.Nonce) != string(msg.Commitment.Nonce) {
		t.Fatalf("commitment nonce mismatch: %v", back.Commitment)
	}
}

func TestPeelMessageWire_RoundTrip(t *testing.T) {
	msg := &pokercrypto.PeelMessage{
		HandNum:    3,
		PlayerID:   "bob",
		CardIndex:  7,
		Ciphertext: big.NewInt(11),
		Result:     big.NewInt(13),
		Proof: &pokercrypto.ZKProof{
			A: big.NewInt(2),
			B: big.NewInt(3),
			S: big.NewInt(4),
			H: big.NewInt(5),
		},
	}
	pd := PeelMessageToPD(msg)
	pb := PartialDecryptToWire("t1", msg.HandNum, pd)
	back := PeelMessageFromWire(pb)
	if back.HandNum != 3 {
		t.Fatalf("HandNum dropped: got %d", back.HandNum)
	}
	if back.PlayerID != "bob" || back.CardIndex != 7 {
		t.Fatalf("peel header mismatch: %+v", back)
	}
	if back.Ciphertext.Cmp(msg.Ciphertext) != 0 || back.Result.Cmp(msg.Result) != 0 {
		t.Fatal("ciphertext/result mismatch")
	}
	if back.Proof == nil || back.Proof.A.Cmp(msg.Proof.A) != 0 || back.Proof.H.Cmp(msg.Proof.H) != 0 {
		t.Fatalf("proof mismatch: %+v", back.Proof)
	}
}

func twoPlayerKeyrings(t *testing.T) []*pokercrypto.Keyring {
	t.Helper()
	p := pokercrypto.SharedPrime()
	ids := []string{"alice", "bob"}
	keys := make([]*pokercrypto.SRAKey, 2)
	pubs := make(map[string][]byte, 2)
	for i, id := range ids {
		k, err := pokercrypto.GenerateSRAKey(p)
		if err != nil {
			t.Fatalf("GenerateSRAKey %s: %v", id, err)
		}
		keys[i] = k
		pubs[id] = k.PublicKey().Bytes()
	}
	rings := make([]*pokercrypto.Keyring, 2)
	for i, id := range ids {
		kr, err := pokercrypto.NewKeyring(id, keys[i], pubs, ids)
		if err != nil {
			t.Fatalf("NewKeyring %s: %v", id, err)
		}
		rings[i] = kr
	}
	return rings
}

type fakeCryptoBus struct {
	shuffle map[string]chan *pokercrypto.ShuffleMessage
	peels   map[string]chan *pokercrypto.PeelMessage
}

func newFakeCryptoBus(ids []string) *fakeCryptoBus {
	b := &fakeCryptoBus{
		shuffle: make(map[string]chan *pokercrypto.ShuffleMessage, len(ids)),
		peels:   make(map[string]chan *pokercrypto.PeelMessage, len(ids)),
	}
	for _, id := range ids {
		b.shuffle[id] = make(chan *pokercrypto.ShuffleMessage, 32)
		b.peels[id] = make(chan *pokercrypto.PeelMessage, 64)
	}
	return b
}

func (b *fakeCryptoBus) broadcastShuffle(from string, msgs []*pokercrypto.ShuffleMessage) {
	for _, msg := range msgs {
		if msg == nil {
			continue
		}
		for id, ch := range b.shuffle {
			if id == from {
				continue
			}
			ch <- ShuffleMessageFromWire(ShuffleMessageToWire("t", msg))
		}
	}
}

func (b *fakeCryptoBus) broadcastPeels(from string, msgs []*pokercrypto.PeelMessage) {
	for _, msg := range msgs {
		if msg == nil {
			continue
		}
		pd := PeelMessageToPD(msg)
		pb := PartialDecryptToWire("t", msg.HandNum, pd)
		for id, ch := range b.peels {
			if id == from {
				continue
			}
			ch <- PeelMessageFromWire(pb)
		}
	}
}

func runCryptoReceiver(ctx context.Context, id string, h *CryptoHand, bus *fakeCryptoBus, errCh chan error) {
	for {
		select {
		case <-ctx.Done():
			return
		case msg := <-bus.shuffle[id]:
			out, err := h.HandleShuffle(msg)
			if err != nil {
				select {
				case errCh <- err:
				default:
				}
				return
			}
			bus.broadcastShuffle(id, out)
		case msg := <-bus.peels[id]:
			out, err := h.HandlePeel(msg)
			if err != nil {
				select {
				case errCh <- err:
				default:
				}
				return
			}
			bus.broadcastPeels(id, out)
		}
	}
}

func setupTwoCryptoHands(t *testing.T) (ids []string, rings []*pokercrypto.Keyring, hands []*CryptoHand, bus *fakeCryptoBus, ctx context.Context, cancel context.CancelFunc, errCh chan error) {
	t.Helper()
	ids = []string{"alice", "bob"}
	rings = twoPlayerKeyrings(t)
	nonce := []byte("lobby-nonce-phase5")
	hands = make([]*CryptoHand, 2)
	for i := range ids {
		h, err := NewCryptoHand(rings[i], nonce, 1, 0)
		if err != nil {
			t.Fatalf("NewCryptoHand %s: %v", ids[i], err)
		}
		hands[i] = h
	}
	bus = newFakeCryptoBus(ids)
	ctx, cancel = context.WithTimeout(context.Background(), 3*time.Minute)
	errCh = make(chan error, 4)
	for i, id := range ids {
		go runCryptoReceiver(ctx, id, hands[i], bus, errCh)
	}
	return ids, rings, hands, bus, ctx, cancel, errCh
}

func checkReceiverErr(t *testing.T, errCh chan error) {
	t.Helper()
	select {
	case err := <-errCh:
		t.Fatalf("receiver: %v", err)
	default:
	}
}

func dealHolesBoth(t *testing.T, ctx context.Context, ids []string, hands []*CryptoHand, bus *fakeCryptoBus, errCh chan error) {
	t.Helper()
	for i, id := range ids {
		out, err := hands[i].StartShuffle()
		if err != nil {
			t.Fatalf("StartShuffle %s: %v", id, err)
		}
		bus.broadcastShuffle(id, out)
	}
	for i, id := range ids {
		if err := hands[i].WaitShuffle(ctx); err != nil {
			checkReceiverErr(t, errCh)
			t.Fatalf("WaitShuffle %s: %v", id, err)
		}
	}
	for i, id := range ids {
		out, err := hands[i].StartHoles()
		if err != nil {
			t.Fatalf("StartHoles %s: %v", id, err)
		}
		bus.broadcastPeels(id, out)
	}
	for i, id := range ids {
		if err := hands[i].WaitHoles(ctx); err != nil {
			checkReceiverErr(t, errCh)
			t.Fatalf("WaitHoles %s: %v", id, err)
		}
	}
}

func startMachines(t *testing.T, ids []string, hands []*CryptoHand) []*game.Machine {
	t.Helper()
	machines := make([]*game.Machine, 2)
	for i, id := range ids {
		holes, err := hands[i].LocalHoles()
		if err != nil {
			t.Fatalf("LocalHoles %s: %v", id, err)
		}
		players := []*game.Player{
			game.NewPlayer("alice", "Alice", 1000),
			game.NewPlayer("bob", "Bob", 1000),
		}
		gs := game.NewGameState("crypto-table", 1, players, 0, 10, 20)
		gs.Players[i].HoleCards = holes
		m := game.NewMachine(gs, nil)
		if err := m.StartHandCrypto(); err != nil {
			t.Fatalf("StartHandCrypto %s: %v", id, err)
		}
		machines[i] = m
	}
	a0, _ := hands[0].LocalHoles()
	a1, _ := hands[1].LocalHoles()
	if a0[0] == a1[0] && a0[1] == a1[1] {
		t.Fatal("expected different local hole pairs")
	}
	for i, m := range machines {
		other := 1 - i
		if machines[i].State.Players[other].HoleCards[0] != (game.Card{}) ||
			machines[i].State.Players[other].HoleCards[1] != (game.Card{}) {
			t.Fatalf("%s can see opponent holes before showdown", ids[i])
		}
		_ = m
	}
	return machines
}

func playUntilWaitBoth(t *testing.T, machines []*game.Machine) {
	t.Helper()
	for n := 0; n < 40; n++ {
		ph := machines[0].State.Phase
		if ph == game.PhaseAwaitingStreet || ph == game.PhaseShowdown || ph == game.PhaseSettled {
			return
		}
		current := machines[0].State.CurrentPlayer()
		if current == nil {
			t.Fatalf("no current player in phase %s", ph)
		}
		toCall := machines[0].State.CurrentBet - current.CurrentBet
		var a game.Action
		if toCall > 0 {
			a = game.Action{PlayerID: current.ID, Type: game.ActionCall}
		} else {
			a = game.Action{PlayerID: current.ID, Type: game.ActionCheck}
		}
		for _, m := range machines {
			if err := m.ApplyAction(a); err != nil {
				t.Fatalf("ApplyAction: %v (phase %s)", err, m.State.Phase)
			}
		}
	}
	t.Fatalf("stuck in phase %s", machines[0].State.Phase)
}

func advanceBoth(t *testing.T, ctx context.Context, ids []string, hands []*CryptoHand, machines []*game.Machine, bus *fakeCryptoBus, errCh chan error) {
	t.Helper()
	var wg sync.WaitGroup
	advErr := make(chan error, 2)
	for i := range hands {
		wg.Add(1)
		i := i
		go func() {
			defer wg.Done()
			err := AdvanceCrypto(ctx, hands[i], machines[i], func(msgs []*pokercrypto.PeelMessage) error {
				bus.broadcastPeels(ids[i], msgs)
				return nil
			})
			if err != nil {
				advErr <- err
			}
		}()
	}
	wg.Wait()
	select {
	case err := <-advErr:
		checkReceiverErr(t, errCh)
		t.Fatalf("AdvanceCrypto: %v", err)
	default:
	}
	checkReceiverErr(t, errCh)
}

func TestCryptoHand_FakeNet_2Players_FoldAfterFlop(t *testing.T) {
	ids, _, hands, bus, ctx, cancel, errCh := setupTwoCryptoHands(t)
	defer cancel()

	dealHolesBoth(t, ctx, ids, hands, bus, errCh)
	machines := startMachines(t, ids, hands)

	playUntilWaitBoth(t, machines)
	if !machines[0].NeedsStreet() || machines[0].PendingStreetCount() != 3 {
		t.Fatalf("expected flop wait, got phase %s pending %d", machines[0].State.Phase, machines[0].PendingStreetCount())
	}

	advanceBoth(t, ctx, ids, hands, machines, bus, errCh)

	if len(machines[0].State.CommunityCards) != 3 || len(machines[1].State.CommunityCards) != 3 {
		t.Fatalf("flop lengths %d %d", len(machines[0].State.CommunityCards), len(machines[1].State.CommunityCards))
	}
	for i := 0; i < 3; i++ {
		if machines[0].State.CommunityCards[i] != machines[1].State.CommunityCards[i] {
			t.Fatalf("flop mismatch at %d: %v vs %v", i, machines[0].State.CommunityCards[i], machines[1].State.CommunityCards[i])
		}
	}
	for i := range machines {
		other := 1 - i
		if machines[i].State.Players[other].HoleCards[0] != (game.Card{}) {
			t.Fatalf("%s leaked opponent holes after flop", ids[i])
		}
	}

	folder := machines[0].State.CurrentPlayer()
	if folder == nil {
		t.Fatal("no actor to fold after flop")
	}
	fold := game.Action{PlayerID: folder.ID, Type: game.ActionFold}
	for _, m := range machines {
		if err := m.ApplyAction(fold); err != nil {
			t.Fatalf("fold: %v", err)
		}
	}
	if machines[0].State.Phase != game.PhaseSettled || machines[1].State.Phase != game.PhaseSettled {
		t.Fatalf("expected settled after fold, got %s / %s", machines[0].State.Phase, machines[1].State.Phase)
	}
	if machines[0].NeedsReveal() || machines[1].NeedsReveal() {
		t.Fatal("fold-to-winner should not need reveal")
	}
	var total int64
	for _, p := range machines[0].State.Players {
		total += p.Stack
	}
	if total != 2000 {
		t.Fatalf("chip conservation: got %d want 2000", total)
	}
}

func TestCryptoHand_FakeNet_2Players_Showdown(t *testing.T) {
	ids, _, hands, bus, ctx, cancel, errCh := setupTwoCryptoHands(t)
	defer cancel()

	dealHolesBoth(t, ctx, ids, hands, bus, errCh)
	machines := startMachines(t, ids, hands)

	for street := 0; street < 3; street++ {
		playUntilWaitBoth(t, machines)
		if machines[0].State.Phase == game.PhaseShowdown || machines[0].State.Phase == game.PhaseSettled {
			break
		}
		if !machines[0].NeedsStreet() {
			t.Fatalf("expected street wait, got %s", machines[0].State.Phase)
		}
		advanceBoth(t, ctx, ids, hands, machines, bus, errCh)
	}
	playUntilWaitBoth(t, machines)
	if machines[0].NeedsStreet() {
		advanceBoth(t, ctx, ids, hands, machines, bus, errCh)
		playUntilWaitBoth(t, machines)
	}
	if !machines[0].NeedsReveal() {
		t.Fatalf("expected reveal wait, got phase %s", machines[0].State.Phase)
	}

	wantOrder := RemainingShowdownIDs(machines[0].State)
	if len(wantOrder) != 2 || wantOrder[0] != "alice" || wantOrder[1] != "bob" {
		t.Fatalf("reveal order %v", wantOrder)
	}
	if got := RemainingShowdownIDs(machines[1].State); len(got) != 2 || got[0] != wantOrder[0] || got[1] != wantOrder[1] {
		t.Fatalf("replicas disagree on reveal order: %v vs %v", wantOrder, got)
	}

	advanceBoth(t, ctx, ids, hands, machines, bus, errCh)

	if machines[0].State.Phase != game.PhaseSettled || machines[1].State.Phase != game.PhaseSettled {
		t.Fatalf("expected settled, got %s / %s", machines[0].State.Phase, machines[1].State.Phase)
	}
	for _, id := range ids {
		h0, err := hands[0].RevealedHoles(id)
		if err != nil {
			t.Fatalf("replica0 RevealedHoles %s: %v", id, err)
		}
		h1, err := hands[1].RevealedHoles(id)
		if err != nil {
			t.Fatalf("replica1 RevealedHoles %s: %v", id, err)
		}
		if h0 != h1 {
			t.Fatalf("revealed holes for %s differ: %v vs %v", id, h0, h1)
		}
	}
	for k, v := range machines[0].State.Payouts {
		if machines[1].State.Payouts[k] != v {
			t.Fatalf("payout mismatch for %s: %d vs %d", k, v, machines[1].State.Payouts[k])
		}
	}

	comm := machines[0].State.CommunityCards
	if len(comm) != 5 {
		t.Fatalf("expected 5 community, got %d", len(comm))
	}
	var winners [2]game.EvaluatedHand
	for i, id := range ids {
		pair, err := hands[0].RevealedHoles(id)
		if err != nil {
			t.Fatal(err)
		}
		var seven [7]game.Card
		seven[0], seven[1] = pair[0], pair[1]
		copy(seven[2:], comm)
		winners[i] = game.EvaluateBest7(seven)
	}
	aliceWins := winners[0].Compare(winners[1])
	var sevenBob [7]game.Card
	pair1, _ := hands[1].RevealedHoles("alice")
	pair2, _ := hands[1].RevealedHoles("bob")
	sevenBob[0], sevenBob[1] = pair1[0], pair1[1]
	copy(sevenBob[2:], machines[1].State.CommunityCards)
	wAlice := game.EvaluateBest7(sevenBob)
	sevenBob[0], sevenBob[1] = pair2[0], pair2[1]
	wBob := game.EvaluateBest7(sevenBob)
	if wAlice.Compare(winners[0]) != 0 || wBob.Compare(winners[1]) != 0 {
		t.Fatal("EvaluateBest7 disagrees across replicas")
	}
	_ = aliceWins
}

func TestCryptoHand_RevealOrderIndependentOfLocalHoles(t *testing.T) {
	ids, _, hands, bus, ctx, cancel, errCh := setupTwoCryptoHands(t)
	defer cancel()

	dealHolesBoth(t, ctx, ids, hands, bus, errCh)
	machines := startMachines(t, ids, hands)

	order0 := RemainingShowdownIDs(machines[0].State)
	order1 := RemainingShowdownIDs(machines[1].State)
	if len(order0) != 2 || order0[0] != "alice" || order0[1] != "bob" {
		t.Fatalf("RemainingShowdownIDs replica0 = %v", order0)
	}
	if order1[0] != order0[0] || order1[1] != order0[1] {
		t.Fatalf("RemainingShowdownIDs disagree: %v vs %v", order0, order1)
	}
	miss0 := machines[0].MissingRevealIDs()
	miss1 := machines[1].MissingRevealIDs()
	if len(miss0) != 1 || miss0[0] != "bob" {
		t.Fatalf("alice replica MissingRevealIDs=%v want [bob]", miss0)
	}
	if len(miss1) != 1 || miss1[0] != "alice" {
		t.Fatalf("bob replica MissingRevealIDs=%v want [alice]", miss1)
	}
	if len(order0) == len(miss0) && order0[0] == miss0[0] {
		t.Fatal("RemainingShowdownIDs must not collapse to MissingRevealIDs")
	}
}

func TestRemainingShowdownIDs_SkipsFolded(t *testing.T) {
	players := []*game.Player{
		game.NewPlayer("alice", "Alice", 1000),
		game.NewPlayer("bob", "Bob", 1000),
	}
	gs := game.NewGameState("t", 1, players, 0, 10, 20)
	gs.Players[1].Status = game.StatusFolded
	got := RemainingShowdownIDs(gs)
	if len(got) != 1 || got[0] != "alice" {
		t.Fatalf("got %v", got)
	}
}
