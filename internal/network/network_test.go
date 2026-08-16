package network

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"sync"
	"testing"
	"time"

	pokercrypto "github.com/RedPaladin7/DecentralizedPokerEngine/internal/crypto"
	"github.com/RedPaladin7/DecentralizedPokerEngine/internal/game"
	"google.golang.org/protobuf/proto"
)

// ── test helpers ──────────────────────────────────────────────────────────────

func generateTestEd25519() (ed25519.PublicKey, ed25519.PrivateKey) {
	pub, priv, err := ed25519.GenerateKey(nil)
	if err != nil {
		panic(err)
	}
	return pub, priv
}

func makeTestNode(t *testing.T, tableID, name string) *Node {
	t.Helper()
	ctx := context.Background()
	p := pokercrypto.SharedPrime()
	key, err := pokercrypto.GenerateSRAKey(p)
	if err != nil {
		t.Fatalf("SRAKey for %s: %v", name, err)
	}
	n, err := NewNode(ctx, tableID, name, 1000, 6, key, "/ip4/127.0.0.1/tcp/0", nil)
	if err != nil {
		t.Fatalf("NewNode %s: %v", name, err)
	}
	if err := n.Start(ctx); err != nil {
		t.Fatalf("Start %s: %v", name, err)
	}
	t.Cleanup(func() { n.Close() })
	return n
}

func connectNodes(t *testing.T, a, b *Node) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	addrs := b.Host.Addrs()
	if len(addrs) == 0 {
		t.Fatal("connectNodes: node B has no addresses")
	}
	if err := a.Host.Connect(ctx, addrs[0]); err != nil {
		t.Fatalf("connectNodes: %v", err)
	}
	time.Sleep(200 * time.Millisecond) // GossipSub mesh formation
}

// ── Codec tests ───────────────────────────────────────────────────────────────

func TestEncodeDecodeEnvelope_RoundTrip(t *testing.T) {
	pub, priv := generateTestEd25519()

	env := NewEnvelope(MsgType_PLAYER_ACTION, "peer-abc", 1, []byte("test payload"))
	frame, err := EncodeEnvelope(env, priv)
	if err != nil {
		t.Fatalf("EncodeEnvelope: %v", err)
	}

	decoded, err := DecodeEnvelope(frame, func(_ string) (ed25519.PublicKey, error) {
		return pub, nil
	})
	if err != nil {
		t.Fatalf("DecodeEnvelope: %v", err)
	}
	if decoded.SenderId != "peer-abc" {
		t.Errorf("sender mismatch: got %q", decoded.SenderId)
	}
	if string(decoded.Payload) != "test payload" {
		t.Errorf("payload mismatch: got %q", decoded.Payload)
	}
	if decoded.Type != MsgType_PLAYER_ACTION {
		t.Errorf("type mismatch: got %v", decoded.Type)
	}
}

func TestEncodeDecodeEnvelope_WrongKey_Rejected(t *testing.T) {
	_, priv := generateTestEd25519()
	pub2, _ := generateTestEd25519() // different key

	env := NewEnvelope(MsgType_PLAYER_ACTION, "peer1", 1, []byte("data"))
	frame, _ := EncodeEnvelope(env, priv)

	_, err := DecodeEnvelope(frame, func(_ string) (ed25519.PublicKey, error) {
		return pub2, nil
	})
	if err == nil {
		t.Error("expected signature verification failure with wrong key")
	}
}

func TestEncodeDecodeEnvelope_FrameTooShort(t *testing.T) {
	_, err := DecodeEnvelope([]byte{0x00, 0x01}, nil)
	if err == nil {
		t.Error("expected error for too-short frame")
	}
}

func TestEncodeDecodeEnvelope_NoVerification(t *testing.T) {
	// nil pubKeyFn = skip verification (used when Noise guarantees auth)
	_, priv := generateTestEd25519()
	env := NewEnvelope(MsgType_HEARTBEAT, "peer2", 5, []byte("hb"))
	frame, _ := EncodeEnvelope(env, priv)

	decoded, err := DecodeEnvelope(frame, nil)
	if err != nil {
		t.Fatalf("DecodeEnvelope (no verify): %v", err)
	}
	if decoded.Seq != 5 {
		t.Errorf("seq mismatch: got %d", decoded.Seq)
	}
}

// ── Big.Int and ZKProof wire encoding tests ───────────────────────────────────

func TestBigIntWire_RoundTrip(t *testing.T) {
	p := pokercrypto.SharedPrime()
	original := pokercrypto.CardToField(27, p)
	recovered := BytesToBigInt(BigIntToBytes(original))
	if original.Cmp(recovered) != 0 {
		t.Errorf("big.Int round-trip failed")
	}
}

func TestBigIntWire_Nil(t *testing.T) {
	if BigIntToBytes(nil) != nil {
		t.Error("BigIntToBytes(nil) should return nil")
	}
	if BytesToBigInt(nil) != nil {
		t.Error("BytesToBigInt(nil) should return nil")
	}
}

func TestZKProofWire_RoundTrip(t *testing.T) {
	p := pokercrypto.SharedPrime()
	key, _ := pokercrypto.GenerateSRAKey(p)
	sid := pokercrypto.SessionID([]string{"test"}, []byte("nonce"))

	ct := pokercrypto.CardToField(5, p)
	result, _ := key.Decrypt(ct)
	proof, err := pokercrypto.ProveDecryption(key, ct, result, sid)
	if err != nil {
		t.Fatalf("ProveDecryption: %v", err)
	}

	wire := ZKProofToWire(proof)
	recovered := ZKProofFromWire(wire)

	if proof.A.Cmp(recovered.A) != 0 {
		t.Error("ZKProof.A mismatch after wire round-trip")
	}
	if proof.B.Cmp(recovered.B) != 0 {
		t.Error("ZKProof.B mismatch after wire round-trip")
	}
	if proof.S.Cmp(recovered.S) != 0 {
		t.Error("ZKProof.S mismatch after wire round-trip")
	}
	if proof.H.Cmp(recovered.H) != 0 {
		t.Error("ZKProof.H mismatch after wire round-trip")
	}

	// The recovered proof must still verify.
	if err := pokercrypto.VerifyDecryption(recovered, ct, result, p, sid); err != nil {
		t.Errorf("recovered ZKProof failed verification: %v", err)
	}
}

func TestDeckWire_RoundTrip(t *testing.T) {
	p := pokercrypto.SharedPrime()
	deck := pokercrypto.BuildPlaintextDeck(p)

	wire := DeckToWire(deck)
	if len(wire) != 52 {
		t.Fatalf("expected 52 wire entries, got %d", len(wire))
	}

	recovered := DeckFromWire(wire)
	for i, v := range deck {
		if v.Cmp(recovered[i]) != 0 {
			t.Errorf("deck[%d] mismatch after wire round-trip", i)
		}
	}
}

// ── GameLog tests ─────────────────────────────────────────────────────────────

func TestGameLog_AppendAndLen(t *testing.T) {
	gl := NewGameLog("t1", 1)
	for i := 1; i <= 5; i++ {
		e := &Envelope{Type: MsgType_PLAYER_ACTION, SenderId: "alice", Seq: int64(i)}
		if err := gl.Append(e); err != nil {
			t.Fatalf("Append %d: %v", i, err)
		}
	}
	if gl.Len() != 5 {
		t.Errorf("expected 5 entries, got %d", gl.Len())
	}
}

func TestGameLog_DuplicateRejected(t *testing.T) {
	gl := NewGameLog("t1", 1)
	e := &Envelope{SenderId: "alice", Seq: 1}
	if err := gl.Append(e); err != nil {
		t.Fatal(err)
	}
	if err := gl.Append(e); err == nil {
		t.Error("expected duplicate error")
	}
}

func TestGameLog_StateRootChanges(t *testing.T) {
	gl := NewGameLog("t1", 1)
	gl.Append(&Envelope{SenderId: "alice", Seq: 1, Payload: []byte("fold")})
	root1 := gl.StateRootHex()

	gl.Append(&Envelope{SenderId: "bob", Seq: 1, Payload: []byte("call")})
	root2 := gl.StateRootHex()

	if root1 == root2 {
		t.Error("state root did not change after appending a new entry")
	}
}

func TestGameLog_DifferentLogsProduceDifferentRoots(t *testing.T) {
	gl1 := NewGameLog("t1", 1)
	gl2 := NewGameLog("t1", 1)

	gl1.Append(&Envelope{SenderId: "alice", Seq: 1, Payload: []byte("fold")})
	gl2.Append(&Envelope{SenderId: "alice", Seq: 1, Payload: []byte("call")}) // different payload

	if gl1.StateRootHex() == gl2.StateRootHex() {
		t.Error("different logs produced the same state root")
	}
}

func TestGameLog_EquivocationDetected(t *testing.T) {
	gl := NewGameLog("t1", 1)
	// Manually insert two entries with same (sender, seq) but different payloads
	// — simulates what would happen if a malicious peer signs conflicting messages.
	gl.entries = append(gl.entries,
		&Envelope{SenderId: "alice", Seq: 1, Payload: []byte("fold")},
		&Envelope{SenderId: "alice", Seq: 1, Payload: []byte("raise 100")},
	)

	senderID, envA, envB, _ := gl.DetectEquivocation()
	if senderID != "alice" {
		t.Errorf("expected equivocation by alice, got %q", senderID)
	}
	if envA == nil || envB == nil {
		t.Error("expected both conflicting envelopes to be returned")
	}
}

func TestGameLog_NoEquivocation(t *testing.T) {
	gl := NewGameLog("t1", 1)
	gl.Append(&Envelope{SenderId: "alice", Seq: 1, Payload: []byte("fold")})
	gl.Append(&Envelope{SenderId: "alice", Seq: 2, Payload: []byte("call")})
	gl.Append(&Envelope{SenderId: "bob", Seq: 1, Payload: []byte("raise")})

	senderID, _, _, _ := gl.DetectEquivocation()
	if senderID != "" {
		t.Errorf("unexpected equivocation detected for %q", senderID)
	}
}

func TestGameLog_ValidateSequences(t *testing.T) {
	gl := NewGameLog("t1", 1)
	gl.Append(&Envelope{SenderId: "alice", Seq: 1})
	gl.Append(&Envelope{SenderId: "alice", Seq: 2})
	gl.Append(&Envelope{SenderId: "alice", Seq: 3})

	if err := gl.ValidateSequences([]string{"alice"}); err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	// Insert a gap.
	gl.Append(&Envelope{SenderId: "alice", Seq: 5}) // skipped 4
	if err := gl.ValidateSequences([]string{"alice"}); err == nil {
		t.Error("expected gap error, got nil")
	}
}

// ── Lobby tests ───────────────────────────────────────────────────────────────

func TestLobby_JoinAndReady_ThreePlayers(t *testing.T) {
	l := NewLobby("t1", 3)

	for _, pid := range []string{"p1", "p2", "p3"} {
		msg := &JoinTable{TableId: "t1", PlayerName: pid, BuyIn: 500}
		if err := l.HandleJoin(msg, pid); err != nil {
			t.Fatalf("join %s: %v", pid, err)
		}
	}
	if l.Count() != 3 {
		t.Errorf("expected 3 seated, got %d", l.Count())
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		if err := l.WaitReady(ctx); err != nil {
			t.Errorf("WaitReady: %v", err)
		}
	}()

	for _, pid := range []string{"p1", "p2", "p3"} {
		if err := l.HandleReady(&PlayerReady{TableId: "t1"}, pid); err != nil {
			t.Errorf("ready %s: %v", pid, err)
		}
	}
	wg.Wait()

	if l.State() != LobbyReady {
		t.Errorf("expected LobbyReady, got %d", l.State())
	}
}

func TestLobby_TableFull(t *testing.T) {
	l := NewLobby("t1", 2)
	l.HandleJoin(&JoinTable{PlayerName: "a", BuyIn: 100}, "p1")
	l.HandleJoin(&JoinTable{PlayerName: "b", BuyIn: 100}, "p2")
	if err := l.HandleJoin(&JoinTable{PlayerName: "c", BuyIn: 100}, "p3"); err == nil {
		t.Error("expected full table error")
	}
}

func TestLobby_DuplicateJoin_Rejected(t *testing.T) {
	l := NewLobby("t1", 4)
	l.HandleJoin(&JoinTable{PlayerName: "a", BuyIn: 100}, "p1")
	if err := l.HandleJoin(&JoinTable{PlayerName: "a", BuyIn: 100}, "p1"); err == nil {
		t.Error("expected duplicate join error")
	}
}

func TestLobby_InvalidBuyIn_Rejected(t *testing.T) {
	l := NewLobby("t1", 4)
	if err := l.HandleJoin(&JoinTable{PlayerName: "a", BuyIn: 0}, "p1"); err == nil {
		t.Error("expected invalid buy-in error")
	}
}

func TestLobby_PlayerIDs_InJoinOrder(t *testing.T) {
	l := NewLobby("t1", 3)
	// Use explicit timestamps — no sleep needed, no time.Now() nondeterminism
	l.HandleJoin(&JoinTable{PlayerName: "first", BuyIn: 100}, "p1", 1000)
	l.HandleJoin(&JoinTable{PlayerName: "second", BuyIn: 100}, "p2", 2000)
	l.HandleJoin(&JoinTable{PlayerName: "third", BuyIn: 100}, "p3", 3000)

	ids := l.PlayerIDs()
	if ids[0] != "p1" || ids[1] != "p2" || ids[2] != "p3" {
		t.Errorf("unexpected order: %v", ids)
	}
}

func TestLobby_SameTimestamp_PeerIDTiebreaker(t *testing.T) {
	l := NewLobby("t1", 3)
	// Same timestamp — PeerID decides order
	l.HandleJoin(&JoinTable{PlayerName: "z-player", BuyIn: 100}, "zzz-peer", 1000)
	l.HandleJoin(&JoinTable{PlayerName: "a-player", BuyIn: 100}, "aaa-peer", 1000)
	l.HandleJoin(&JoinTable{PlayerName: "m-player", BuyIn: 100}, "mmm-peer", 1000)

	ids := l.PlayerIDs()
	if ids[0] != "aaa-peer" || ids[1] != "mmm-peer" || ids[2] != "zzz-peer" {
		t.Errorf("expected lexicographic PeerID order, got: %v", ids)
	}
}

func TestLobby_StoresSRAPubKeyE(t *testing.T) {
	l := NewLobby("t1", 2)
	raw := []byte{0x01, 0x02, 0x03}
	msg := &JoinTable{PlayerName: "a", BuyIn: 100, SraPubKeyE: raw}
	if err := l.HandleJoin(msg, "p1"); err != nil {
		t.Fatalf("HandleJoin: %v", err)
	}
	raw[0] = 0xff
	got := l.Seats()[0].SRAKeyE
	if !bytes.Equal(got, []byte{0x01, 0x02, 0x03}) {
		t.Errorf("stored SRAKeyE mutated: got %v", got)
	}
}

func TestLobby_PublicExponents_CanonicalOrder(t *testing.T) {
	l := NewLobby("t1", 3)
	l.HandleJoin(&JoinTable{PlayerName: "first", BuyIn: 100, SraPubKeyE: []byte{0x01}}, "p1", 1000)
	l.HandleJoin(&JoinTable{PlayerName: "second", BuyIn: 100, SraPubKeyE: []byte{0x02}}, "p2", 2000)
	l.HandleJoin(&JoinTable{PlayerName: "third", BuyIn: 100, SraPubKeyE: []byte{0x03}}, "p3", 3000)

	ids := l.PlayerIDs()
	exps := l.PublicExponents()
	if len(exps) != len(ids) {
		t.Fatalf("PublicExponents len %d != PlayerIDs len %d", len(exps), len(ids))
	}
	want := [][]byte{{0x01}, {0x02}, {0x03}}
	for i := range ids {
		if ids[i] != []string{"p1", "p2", "p3"}[i] {
			t.Fatalf("PlayerIDs order: %v", ids)
		}
		if !bytes.Equal(exps[i], want[i]) {
			t.Errorf("PublicExponents[%d]=%v, want %v", i, exps[i], want[i])
		}
	}
}

func TestLobby_AllSeatsHavePublicE(t *testing.T) {
	l := NewLobby("t1", 2)
	l.HandleJoin(&JoinTable{PlayerName: "a", BuyIn: 100, SraPubKeyE: []byte{0x01}}, "p1", 1000)
	l.HandleJoin(&JoinTable{PlayerName: "b", BuyIn: 100, SraPubKeyE: []byte{0x02}}, "p2", 2000)
	if !l.AllSeatsHavePublicE() {
		t.Error("expected true when every seat has e")
	}

	l2 := NewLobby("t1", 2)
	l2.HandleJoin(&JoinTable{PlayerName: "a", BuyIn: 100, SraPubKeyE: []byte{0x01}}, "p1", 1000)
	l2.HandleJoin(&JoinTable{PlayerName: "b", BuyIn: 100}, "p2", 2000)
	if l2.AllSeatsHavePublicE() {
		t.Error("expected false when one seat has empty e")
	}

	l3 := NewLobby("t1", 2)
	l3.HandleJoin(&JoinTable{PlayerName: "a", BuyIn: 100}, "p1", 1000)
	l3.HandleJoin(&JoinTable{PlayerName: "b", BuyIn: 100}, "p2", 2000)
	if l3.AllSeatsHavePublicE() {
		t.Error("expected false when all e are empty")
	}
}

func TestKeyringFromLobby_OK(t *testing.T) {
	p := pokercrypto.SharedPrime()
	alice, err := pokercrypto.GenerateSRAKey(p)
	if err != nil {
		t.Fatalf("alice key: %v", err)
	}
	bob, err := pokercrypto.GenerateSRAKey(p)
	if err != nil {
		t.Fatalf("bob key: %v", err)
	}

	l := NewLobby("t1", 2)
	if err := l.HandleJoin(&JoinTable{PlayerName: "alice", BuyIn: 100, SraPubKeyE: alice.PublicKey().Bytes()}, "alice", 1000); err != nil {
		t.Fatalf("join alice: %v", err)
	}
	if err := l.HandleJoin(&JoinTable{PlayerName: "bob", BuyIn: 100, SraPubKeyE: bob.PublicKey().Bytes()}, "bob", 2000); err != nil {
		t.Fatalf("join bob: %v", err)
	}

	kr, err := KeyringFromLobby("alice", alice, l)
	if err != nil {
		t.Fatalf("KeyringFromLobby: %v", err)
	}
	pub, ok := kr.Public("bob")
	if !ok {
		t.Fatal("Public(bob) missing")
	}
	if pub.D != nil {
		t.Error("Public(peer).D is not nil")
	}
	localPub, ok := kr.Public("alice")
	if !ok {
		t.Fatal("Public(alice) missing")
	}
	if localPub.D != nil {
		t.Error("Public(local).D is not nil")
	}
}

func TestKeyringFromLobby_MissingE(t *testing.T) {
	p := pokercrypto.SharedPrime()
	alice, err := pokercrypto.GenerateSRAKey(p)
	if err != nil {
		t.Fatalf("alice key: %v", err)
	}
	l := NewLobby("t1", 2)
	l.HandleJoin(&JoinTable{PlayerName: "alice", BuyIn: 100, SraPubKeyE: alice.PublicKey().Bytes()}, "alice", 1000)
	l.HandleJoin(&JoinTable{PlayerName: "bob", BuyIn: 100}, "bob", 2000) // empty e, --no-crypto style
	if _, err := KeyringFromLobby("alice", alice, l); err == nil {
		t.Error("expected error when a seat has empty e")
	}
}

// ── Replay protection tests ───────────────────────────────────────────────────

func TestReplayProtection_StrictlyIncreasing(t *testing.T) {
	gm := &GossipManager{seqNums: make(map[string]int64)}

	if err := gm.CheckAndUpdateSeq("alice", 1); err != nil {
		t.Fatalf("seq 1 should be accepted: %v", err)
	}
	if err := gm.CheckAndUpdateSeq("alice", 2); err != nil {
		t.Fatalf("seq 2 should be accepted: %v", err)
	}
	if err := gm.CheckAndUpdateSeq("alice", 10); err != nil {
		t.Fatalf("seq 10 should be accepted (gaps allowed): %v", err)
	}
}

func TestReplayProtection_DuplicateRejected(t *testing.T) {
	gm := &GossipManager{seqNums: make(map[string]int64)}
	gm.CheckAndUpdateSeq("bob", 5)

	if err := gm.CheckAndUpdateSeq("bob", 5); err == nil {
		t.Error("duplicate seq 5 should be rejected")
	}
}

func TestReplayProtection_OldSeqRejected(t *testing.T) {
	gm := &GossipManager{seqNums: make(map[string]int64)}
	gm.CheckAndUpdateSeq("carol", 10)

	if err := gm.CheckAndUpdateSeq("carol", 3); err == nil {
		t.Error("old seq 3 should be rejected after seq 10 was seen")
	}
}

func TestReplayProtection_IndependentPerPeer(t *testing.T) {
	gm := &GossipManager{seqNums: make(map[string]int64)}
	gm.CheckAndUpdateSeq("alice", 5)

	// bob's seq is independent — seq 1 from bob should be fine after seq 5 from alice.
	if err := gm.CheckAndUpdateSeq("bob", 1); err != nil {
		t.Errorf("bob seq 1 should be accepted: %v", err)
	}
}

// ── Proto round-trip tests ────────────────────────────────────────────────────

func TestProto_ShuffleStep_RoundTrip(t *testing.T) {
	p := pokercrypto.SharedPrime()
	deck := pokercrypto.BuildPlaintextDeck(p)

	original := &ShuffleStep{
		TableId:  "table1",
		HandNum:  3,
		PlayerId: "alice",
		Deck:     DeckToWire(deck),
	}
	b, err := proto.Marshal(original)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	recovered := &ShuffleStep{}
	if err := proto.Unmarshal(b, recovered); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if recovered.HandNum != 3 || recovered.PlayerId != "alice" {
		t.Errorf("fields mismatch: %+v", recovered)
	}
	recoveredDeck := DeckFromWire(recovered.Deck)
	for i, v := range deck {
		if v.Cmp(recoveredDeck[i]) != 0 {
			t.Errorf("deck[%d] mismatch after proto round-trip", i)
		}
	}
}

func TestProto_PartialDecrypt_RoundTrip(t *testing.T) {
	p := pokercrypto.SharedPrime()
	key, _ := pokercrypto.GenerateSRAKey(p)
	sid := pokercrypto.SessionID([]string{"a", "b"}, []byte("n"))

	ct := pokercrypto.CardToField(10, p)
	result, _ := key.Decrypt(ct)
	proof, _ := pokercrypto.ProveDecryption(key, ct, result, sid)

	pd := &pokercrypto.PartialDecryption{
		PlayerID:   "alice",
		CardIndex:  10,
		Ciphertext: ct,
		Result:     result,
		Proof:      proof,
	}

	wire := PartialDecryptToWire("t1", 1, pd)
	b, _ := proto.Marshal(wire)
	recovered := &PartialDecrypt{}
	proto.Unmarshal(b, recovered)

	recoveredProof := ZKProofFromWire(recovered.Proof)
	if err := pokercrypto.VerifyDecryption(recoveredProof, ct, result, p, sid); err != nil {
		t.Errorf("ZK proof failed verification after proto round-trip: %v", err)
	}
}

func TestProto_HandResult_RoundTrip(t *testing.T) {
	original := &HandResult{
		TableId: "t1",
		HandNum: 7,
		Pots: []*PotResult{
			{Amount: 500, WinnerIds: []string{"alice"}},
			{Amount: 200, WinnerIds: []string{"bob", "carol"}},
		},
		StateRoot: []byte("abc123"),
	}
	b, _ := proto.Marshal(original)
	recovered := &HandResult{}
	proto.Unmarshal(b, recovered)

	if recovered.HandNum != 7 {
		t.Errorf("HandNum: got %d", recovered.HandNum)
	}
	if len(recovered.Pots) != 2 {
		t.Fatalf("expected 2 pots, got %d", len(recovered.Pots))
	}
	if recovered.Pots[0].Amount != 500 {
		t.Errorf("Pot[0] amount: got %d", recovered.Pots[0].Amount)
	}
	if len(recovered.Pots[1].WinnerIds) != 2 {
		t.Errorf("Pot[1] winners: got %v", recovered.Pots[1].WinnerIds)
	}
}

// ── Network integration tests (require working libp2p) ───────────────────────

func TestNode_BroadcastAndReceiveAction(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping network integration test in -short mode")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	nodeA := makeTestNode(t, "net-test", "Alice")
	nodeB := makeTestNode(t, "net-test", "Bob")
	connectNodes(t, nodeA, nodeB)

	received := make(chan *PlayerAction, 1)
	nodeB.OnPlayerAction = func(msg *PlayerAction) {
		received <- msg
	}

	action := game.Action{PlayerID: nodeA.Host.PeerID, Type: game.ActionRaise, Amount: 100}
	if err := nodeA.BroadcastAction(ctx, 1, action, 1); err != nil {
		t.Fatalf("BroadcastAction: %v", err)
	}

	select {
	case msg := <-received:
		if msg.Action != int32(game.ActionRaise) {
			t.Errorf("expected Raise, got %d", msg.Action)
		}
		if msg.Amount != 100 {
			t.Errorf("expected amount 100, got %d", msg.Amount)
		}
	case <-time.After(10 * time.Second):
		t.Error("timeout: action not received within 10s")
	}
}

func TestNode_BroadcastJoin_NilSRAKey_NoPanic(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping network integration test in -short mode")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	n, err := NewNode(ctx, "nil-sra-test", "Alice", 1000, 6, nil, "/ip4/127.0.0.1/tcp/0", nil)
	if err != nil {
		t.Fatalf("NewNode: %v", err)
	}
	if err := n.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() { n.Close() })

	if err := n.BroadcastJoin(ctx, 1); err != nil {
		t.Fatalf("BroadcastJoin: %v", err)
	}
	seats := n.Lobby.Seats()
	if len(seats) != 1 {
		t.Fatalf("expected 1 local seat, got %d", len(seats))
	}
	if len(seats[0].SRAKeyE) != 0 {
		t.Errorf("expected empty SRAKeyE, got %v", seats[0].SRAKeyE)
	}
}

func TestNode_BroadcastJoin_LobbyUpdated(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping network integration test in -short mode")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	nodeA := makeTestNode(t, "lobby-net-test", "Alice")
	nodeB := makeTestNode(t, "lobby-net-test", "Bob")
	connectNodes(t, nodeA, nodeB)

	joined := make(chan string, 1)
	nodeB.OnJoinTable = func(msg *JoinTable, from string) {
		joined <- from
	}

	if err := nodeA.BroadcastJoin(ctx, 1); err != nil {
		t.Fatalf("BroadcastJoin: %v", err)
	}

	select {
	case from := <-joined:
		if from != nodeA.Host.PeerID {
			t.Errorf("join from unexpected peer: got %s, want %s", from, nodeA.Host.PeerID)
		}
		if nodeB.Lobby.Count() != 1 {
			t.Errorf("lobby should have 1 player, got %d", nodeB.Lobby.Count())
		}
	case <-time.After(10 * time.Second):
		t.Error("timeout: join message not received")
	}
}

func TestBroadcastJoin_RebroadcastKeepsTimestamp(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping network integration test in -short mode")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	n := makeTestNode(t, "join-ts-test", "Alice")
	if err := n.BroadcastJoin(ctx, 1); err != nil {
		t.Fatalf("first BroadcastJoin: %v", err)
	}
	seats := n.Lobby.Seats()
	if len(seats) != 1 {
		t.Fatalf("expected 1 seat after first join, got %d", len(seats))
	}
	firstTS := seats[0].JoinedAtUnixMs
	if firstTS == 0 {
		t.Fatal("expected non-zero join timestamp")
	}
	if n.joinTimestamp != firstTS {
		t.Fatalf("node joinTimestamp %d != lobby %d", n.joinTimestamp, firstTS)
	}

	time.Sleep(20 * time.Millisecond)
	if err := n.BroadcastJoin(ctx, 1); err != nil {
		t.Fatalf("second BroadcastJoin: %v", err)
	}
	seats = n.Lobby.Seats()
	if len(seats) != 1 {
		t.Fatalf("rebroadcast must not add a seat, got %d", len(seats))
	}
	if seats[0].JoinedAtUnixMs != firstTS {
		t.Errorf("rebroadcast changed JoinedAtUnixMs: first %d, then %d", firstTS, seats[0].JoinedAtUnixMs)
	}
	if n.joinTimestamp != firstTS {
		t.Errorf("joinTimestamp mutated on rebroadcast: got %d want %d", n.joinTimestamp, firstTS)
	}
}

func TestDispatch_DuplicateJoin_DoesNotCallback(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping network integration test in -short mode")
	}
	n := makeTestNode(t, "dup-join-cb", "Alice")
	pub, priv := generateTestEd25519()
	n.RegisterPeer("p1", pub)

	payload, err := proto.Marshal(&JoinTable{
		TableId:      "dup-join-cb",
		PlayerName:   "Bob",
		BuyIn:        100,
		SessionNonce: []byte("p1"),
	})
	if err != nil {
		t.Fatalf("marshal JoinTable: %v", err)
	}

	calls := 0
	n.OnJoinTable = func(*JoinTable, string) { calls++ }

	env1 := NewEnvelope(MsgType_JOIN_TABLE, "p1", 1, payload)
	env1.Timestamp = 1000
	frame1, err := EncodeEnvelope(env1, priv)
	if err != nil {
		t.Fatalf("EncodeEnvelope first: %v", err)
	}
	n.dispatch(frame1)
	if calls != 1 {
		t.Fatalf("first join callback count: got %d want 1", calls)
	}
	seats := n.Lobby.Seats()
	if len(seats) != 1 || seats[0].JoinedAtUnixMs != 1000 {
		t.Fatalf("first join not stored with timestamp 1000: %+v", seats)
	}

	env2 := NewEnvelope(MsgType_JOIN_TABLE, "p1", 2, payload)
	env2.Timestamp = 99999
	frame2, err := EncodeEnvelope(env2, priv)
	if err != nil {
		t.Fatalf("EncodeEnvelope second: %v", err)
	}
	n.dispatch(frame2)
	if calls != 1 {
		t.Errorf("duplicate join must not callback: got %d want 1", calls)
	}
	seats = n.Lobby.Seats()
	if len(seats) != 1 {
		t.Fatalf("duplicate join added a seat: %d", len(seats))
	}
	if seats[0].JoinedAtUnixMs != 1000 {
		t.Errorf("duplicate join changed timestamp: got %d want 1000", seats[0].JoinedAtUnixMs)
	}
}

func TestNode_ThreePeerMesh_AllReceive(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping network integration test in -short mode")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	nodeA := makeTestNode(t, "mesh-test", "Alice")
	nodeB := makeTestNode(t, "mesh-test", "Bob")
	nodeC := makeTestNode(t, "mesh-test", "Carol")
	connectNodes(t, nodeA, nodeB)
	connectNodes(t, nodeB, nodeC)
	connectNodes(t, nodeA, nodeC)

	var mu sync.Mutex
	receivedBy := make(map[string]bool)

	for name, node := range map[string]*Node{"Bob": nodeB, "Carol": nodeC} {
		n := name
		nd := node
		nd.OnPlayerAction = func(msg *PlayerAction) {
			mu.Lock()
			receivedBy[n] = true
			mu.Unlock()
		}
	}

	action := game.Action{PlayerID: nodeA.Host.PeerID, Type: game.ActionFold}
	if err := nodeA.BroadcastAction(ctx, 1, action, 1); err != nil {
		t.Fatalf("BroadcastAction: %v", err)
	}

	deadline := time.After(15 * time.Second)
	for {
		time.Sleep(100 * time.Millisecond)
		mu.Lock()
		both := receivedBy["Bob"] && receivedBy["Carol"]
		mu.Unlock()
		if both {
			break
		}
		select {
		case <-deadline:
			mu.Lock()
			t.Errorf("not all peers received the action: %v", receivedBy)
			mu.Unlock()
			return
		default:
		}
	}
}
