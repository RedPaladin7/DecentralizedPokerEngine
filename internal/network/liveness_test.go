package network

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	pokercrypto "github.com/RedPaladin7/DecentralizedPokerEngine/internal/crypto"
	"github.com/RedPaladin7/DecentralizedPokerEngine/internal/fault"
	"github.com/RedPaladin7/DecentralizedPokerEngine/internal/game"
)

func nPlayerNetworkKeyrings(t *testing.T, ids []string) []*pokercrypto.Keyring {
	t.Helper()
	p := pokercrypto.SharedPrime()
	keys := make([]*pokercrypto.SRAKey, len(ids))
	pubs := make(map[string][]byte, len(ids))
	for i, id := range ids {
		k, err := pokercrypto.GenerateSRAKey(p)
		if err != nil {
			t.Fatalf("GenerateSRAKey %s: %v", id, err)
		}
		keys[i] = k
		pubs[id] = k.PublicKey().Bytes()
	}
	rings := make([]*pokercrypto.Keyring, len(ids))
	for i, id := range ids {
		kr, err := pokercrypto.NewKeyring(id, keys[i], pubs, ids)
		if err != nil {
			t.Fatalf("NewKeyring %s: %v", id, err)
		}
		rings[i] = kr
	}
	return rings
}

func TestLiveness_ShareUnicastNotOnShuffleBus(t *testing.T) {
	ids := []string{"alice", "bob", "carol"}
	p := pokercrypto.SharedPrime()
	key, err := pokercrypto.GenerateSRAKey(p)
	if err != nil {
		t.Fatalf("GenerateSRAKey: %v", err)
	}
	bus := newFakeCryptoBus(ids)

	shares, _, err := fault.SplitAndDistribute(key, 3)
	if err != nil {
		t.Fatalf("SplitAndDistribute: %v", err)
	}
	shuffleBefore := len(bus.shuffle["bob"])
	peelBefore := len(bus.peels["bob"])
	for i, id := range ids {
		if id == "alice" {
			continue
		}
		bus.shares[id] <- shares[i]
	}
	if len(bus.shuffle["bob"]) != shuffleBefore || len(bus.peels["bob"]) != peelBefore {
		t.Fatal("Shamir shares must not go on the shuffle/peel bus")
	}
	if len(bus.shares["bob"]) != 1 || len(bus.shares["carol"]) != 1 {
		t.Fatalf("expected unicast shares on the share channel, got bob=%d carol=%d",
			len(bus.shares["bob"]), len(bus.shares["carol"]))
	}
}

func TestLiveness_ShuffleTimeoutAborts(t *testing.T) {
	rings := twoPlayerKeyrings(t)
	h, err := NewCryptoHand(rings[0], []byte("abort-nonce"), 1, 0)
	if err != nil {
		t.Fatal(err)
	}
	h.AbortShuffle(fmt.Errorf("shuffle aborted: peer bob timed out"))
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	err = h.WaitShuffle(ctx)
	if err == nil || !strings.Contains(err.Error(), "bob") {
		t.Fatalf("WaitShuffle error = %v, want shuffle aborted: peer bob", err)
	}
	if h.ShuffleDone() {
		t.Fatal("mid-shuffle abort must not complete the shuffle")
	}
}

func TestLiveness_FakeNet_3Players_TimeoutFold_NoCryptoStyle(t *testing.T) {
	ids := []string{"alice", "bob", "carol"}
	players := []*game.Player{
		game.NewPlayer("alice", "Alice", 1000),
		game.NewPlayer("bob", "Bob", 1000),
		game.NewPlayer("carol", "Carol", 1000),
	}
	makeMachine := func() *game.Machine {
		ps := make([]*game.Player, 3)
		for i, p := range players {
			cp := *p
			ps[i] = &cp
		}
		gs := game.NewGameState("plain", 1, ps, 0, 10, 20)
		gs.Deck = game.NewDeck()
		m := game.NewMachine(gs, nil)
		if err := m.StartHand(); err != nil {
			t.Fatalf("StartHand: %v", err)
		}
		return m
	}
	aliceM, bobM := makeMachine(), makeMachine()
	_ = ids

	applyBoth := func(a game.Action) {
		t.Helper()
		if err := aliceM.ApplyAction(a); err != nil {
			t.Fatalf("alice ApplyAction: %v", err)
		}
		if err := bobM.ApplyAction(a); err != nil {
			t.Fatalf("bob ApplyAction: %v", err)
		}
	}

	for n := 0; n < 40; n++ {
		ph := aliceM.State.Phase
		if ph == game.PhaseFlop || ph == game.PhaseAwaitingStreet || ph == game.PhaseSettled {
			break
		}
		cur := aliceM.State.CurrentPlayer()
		if cur == nil {
			t.Fatalf("no current player in phase %s", ph)
		}
		if cur.ID == "carol" {
			applyBoth(game.Action{PlayerID: "carol", Type: game.ActionFold})
			continue
		}
		toCall := aliceM.State.CurrentBet - cur.CurrentBet
		a := game.Action{PlayerID: cur.ID, Type: game.ActionCheck}
		if toCall > 0 {
			a.Type = game.ActionCall
		}
		applyBoth(a)
	}
	if aliceM.State.CurrentPlayer() != nil && aliceM.State.CurrentPlayer().ID == "carol" {
		t.Fatal("phase stuck on timed-out carol")
	}
	if bobM.State.CurrentPlayer() != nil && bobM.State.CurrentPlayer().ID == "carol" {
		t.Fatal("bob replica stuck on timed-out carol")
	}
}

func TestLiveness_FakeNet_3Players_KillAfterHoles_FlopCompletes(t *testing.T) {
	ids := []string{"alice", "bob", "carol"}
	rings := nPlayerNetworkKeyrings(t, ids)
	nonce := []byte("liveness-3p")
	hands := make([]*CryptoHand, 3)
	for i := range ids {
		h, err := NewCryptoHand(rings[i], nonce, 1, 0)
		if err != nil {
			t.Fatalf("NewCryptoHand %s: %v", ids[i], err)
		}
		hands[i] = h
	}
	bus := newFakeCryptoBus(ids)
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	defer cancel()
	errCh := make(chan error, 6)
	recvCtx, recvCancel := context.WithCancel(ctx)
	defer recvCancel()
	for i, id := range ids {
		go runCryptoReceiver(recvCtx, id, hands[i], bus, errCh)
	}

	carolShares, thresh, err := fault.SplitAndDistribute(rings[2].LocalKey(), 3)
	if err != nil {
		t.Fatalf("SplitAndDistribute carol: %v", err)
	}

	dealHolesN(t, ctx, ids, hands, bus, errCh)

	bus.dead["carol"] = true
	recvCancel()
	time.Sleep(20 * time.Millisecond)

	store := fault.NewKeyShareStore(pokercrypto.SharedPrime())
	store.AddReconstructionShare("carol", carolShares[0])
	store.AddReconstructionShare("carol", carolShares[1])
	recon, err := store.ReconstructSRAKey("carol", thresh)
	if err != nil {
		t.Fatalf("ReconstructSRAKey: %v", err)
	}
	if rings[0].LocalKey().D.Cmp(recon.D) == 0 {
		t.Fatal("reconstructed carol d must not replace alice's keyring d")
	}
	for _, idx := range []int{0, 1} {
		if err := hands[idx].MarkGone("carol", recon); err != nil {
			t.Fatalf("MarkGone %s: %v", ids[idx], err)
		}
	}

	liveIDs := []string{"alice", "bob"}
	liveHands := []*CryptoHand{hands[0], hands[1]}
	machines := startMachines3(t, liveIDs, liveHands)

	recvCtx2, recvCancel2 := context.WithCancel(ctx)
	defer recvCancel2()
	errCh2 := make(chan error, 4)
	for i, id := range liveIDs {
		go runCryptoReceiver(recvCtx2, id, liveHands[i], bus, errCh2)
	}

	playUntilWait3(t, machines)
	for machines[0].State.CurrentPlayer() != nil && machines[0].State.CurrentPlayer().ID == "carol" {
		fold := game.Action{PlayerID: "carol", Type: game.ActionFold}
		for _, m := range machines {
			if err := m.ApplyAction(fold); err != nil {
				t.Fatalf("fold carol: %v", err)
			}
		}
		playUntilWait3(t, machines)
	}

	advanceLive(t, ctx, liveIDs, liveHands, machines, bus, errCh2)

	if len(machines[0].State.CommunityCards) != 3 || len(machines[1].State.CommunityCards) != 3 {
		t.Fatalf("flop lengths %d %d", len(machines[0].State.CommunityCards), len(machines[1].State.CommunityCards))
	}
	for i := 0; i < 3; i++ {
		if machines[0].State.CommunityCards[i] != machines[1].State.CommunityCards[i] {
			t.Fatalf("flop mismatch at %d: %v vs %v", i, machines[0].State.CommunityCards[i], machines[1].State.CommunityCards[i])
		}
	}
}

func dealHolesN(t *testing.T, ctx context.Context, ids []string, hands []*CryptoHand, bus *fakeCryptoBus, errCh chan error) {
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

func startMachines3(t *testing.T, ids []string, hands []*CryptoHand) []*game.Machine {
	t.Helper()
	machines := make([]*game.Machine, len(ids))
	for i, id := range ids {
		holes, err := hands[i].LocalHoles()
		if err != nil {
			t.Fatalf("LocalHoles %s: %v", id, err)
		}
		players := []*game.Player{
			game.NewPlayer("alice", "Alice", 1000),
			game.NewPlayer("bob", "Bob", 1000),
			game.NewPlayer("carol", "Carol", 1000),
		}
		gs := game.NewGameState("crypto-table", 1, players, 0, 10, 20)
		gs.Players[i].HoleCards = holes
		m := game.NewMachine(gs, nil)
		if err := m.StartHandCrypto(); err != nil {
			t.Fatalf("StartHandCrypto %s: %v", id, err)
		}
		machines[i] = m
	}
	return machines
}

func playUntilWait3(t *testing.T, machines []*game.Machine) {
	t.Helper()
	for n := 0; n < 60; n++ {
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
		if current.ID == "carol" {
			a = game.Action{PlayerID: "carol", Type: game.ActionFold}
		} else if toCall > 0 {
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

func advanceLive(t *testing.T, ctx context.Context, ids []string, hands []*CryptoHand, machines []*game.Machine, bus *fakeCryptoBus, errCh chan error) {
	t.Helper()
	var wg sync.WaitGroup
	advErr := make(chan error, len(hands))
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
