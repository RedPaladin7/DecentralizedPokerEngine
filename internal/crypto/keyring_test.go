package crypto

import (
	"math/big"
	"testing"
)

func twoPlayerPubs(t *testing.T) (alice, bob *SRAKey, publicE map[string][]byte, order []string) {
	t.Helper()
	var err error
	alice, err = GenerateSRAKey(smallPrime)
	if err != nil {
		t.Fatalf("alice key: %v", err)
	}
	bob, err = GenerateSRAKey(smallPrime)
	if err != nil {
		t.Fatalf("bob key: %v", err)
	}
	order = []string{"alice", "bob"}
	publicE = map[string][]byte{
		"alice": alice.PublicKey().Bytes(),
		"bob":   bob.PublicKey().Bytes(),
	}
	return alice, bob, publicE, order
}

func TestNewKeyring_OK(t *testing.T) {
	alice, bob, publicE, order := twoPlayerPubs(t)
	kr, err := NewKeyring("alice", alice, publicE, order)
	if err != nil {
		t.Fatalf("NewKeyring: %v", err)
	}
	if kr.Len() != 2 {
		t.Errorf("Len: got %d, want 2", kr.Len())
	}
	if !kr.LocalKey().IsPrivate() {
		t.Error("LocalKey is not private")
	}
	localPub, ok := kr.Public("alice")
	if !ok {
		t.Fatal("Public(alice) missing")
	}
	if localPub.D != nil {
		t.Error("Public(local) D is not nil")
	}
	other, ok := kr.Public("bob")
	if !ok {
		t.Fatal("Public(bob) missing")
	}
	if other.D != nil {
		t.Error("Public(other) D is not nil")
	}
	if other.E.Cmp(bob.E) != 0 {
		t.Error("Public(bob).E does not match bob's E")
	}
}

func TestKeyring_Public_NeverReturnsD(t *testing.T) {
	alice, _, publicE, order := twoPlayerPubs(t)
	kr, err := NewKeyring("alice", alice, publicE, order)
	if err != nil {
		t.Fatalf("NewKeyring: %v", err)
	}
	for _, id := range kr.SeatOrder() {
		pub, ok := kr.Public(id)
		if !ok {
			t.Fatalf("Public(%s) missing", id)
		}
		if pub.D != nil {
			t.Errorf("Public(%s) returned non-nil D", id)
		}
	}
	if kr.LocalKey().D == nil {
		t.Error("LocalKey().D is nil")
	}
}

func TestKeyring_RejectsEmptyPeerE(t *testing.T) {
	alice, _, publicE, order := twoPlayerPubs(t)
	publicE["bob"] = []byte{}
	if _, err := NewKeyring("alice", alice, publicE, order); err == nil {
		t.Error("expected error for empty peer e")
	}
}

func TestKeyring_RejectsLocalEMismatch(t *testing.T) {
	alice, bob, publicE, order := twoPlayerPubs(t)
	publicE["alice"] = bob.PublicKey().Bytes()
	if _, err := NewKeyring("alice", alice, publicE, order); err == nil {
		t.Error("expected error for local e mismatch")
	}
}

func TestKeyring_RejectsPublicOnlyLocal(t *testing.T) {
	alice, _, publicE, order := twoPlayerPubs(t)
	pub, err := PublicSRAKey(smallPrime, alice.E)
	if err != nil {
		t.Fatalf("PublicSRAKey: %v", err)
	}
	if _, err := NewKeyring("alice", pub, publicE, order); err == nil {
		t.Error("expected error for public-only local key")
	}
}

func TestKeyring_RejectsUnknownLocalID(t *testing.T) {
	alice, _, publicE, order := twoPlayerPubs(t)
	if _, err := NewKeyring("carol", alice, publicE, order); err == nil {
		t.Error("expected error for localID not in seatOrder")
	}
}

func TestKeyring_RejectsDuplicateSeats(t *testing.T) {
	alice, _, publicE, _ := twoPlayerPubs(t)
	if _, err := NewKeyring("alice", alice, publicE, []string{"alice", "alice"}); err == nil {
		t.Error("expected error for duplicate seats")
	}
}

func TestKeyring_RejectsExtraMapKey(t *testing.T) {
	alice, _, publicE, order := twoPlayerPubs(t)
	publicE["carol"] = []byte{0x02}
	if _, err := NewKeyring("alice", alice, publicE, order); err == nil {
		t.Error("expected error for extra publicE key")
	}
}

func TestKeyring_PublicExponents_SeatOrder(t *testing.T) {
	alice, bob, _, _ := twoPlayerPubs(t)
	order := []string{"bob", "alice"}
	publicE := map[string][]byte{
		"alice": alice.PublicKey().Bytes(),
		"bob":   bob.PublicKey().Bytes(),
	}
	kr, err := NewKeyring("alice", alice, publicE, order)
	if err != nil {
		t.Fatalf("NewKeyring: %v", err)
	}
	exps := kr.PublicExponents()
	if len(exps) != 2 {
		t.Fatalf("expected 2 exponents, got %d", len(exps))
	}
	if exps[0].Cmp(bob.E) != 0 || exps[1].Cmp(alice.E) != 0 {
		t.Errorf("exponents did not follow seatOrder: got [%s, %s]", exps[0], exps[1])
	}
}

func TestKeyring_PublicMissingPeer(t *testing.T) {
	alice, _, publicE, order := twoPlayerPubs(t)
	kr, err := NewKeyring("alice", alice, publicE, order)
	if err != nil {
		t.Fatalf("NewKeyring: %v", err)
	}
	if _, ok := kr.Public("nobody"); ok {
		t.Error("expected ok == false for unknown peer")
	}
}

func TestKeyring_SinglePlayerRejected(t *testing.T) {
	alice, err := GenerateSRAKey(smallPrime)
	if err != nil {
		t.Fatalf("GenerateSRAKey: %v", err)
	}
	publicE := map[string][]byte{"alice": alice.PublicKey().Bytes()}
	if _, err := NewKeyring("alice", alice, publicE, []string{"alice"}); err == nil {
		t.Error("expected error for single-player seatOrder")
	}
}

func TestKeyring_SeatIndex(t *testing.T) {
	alice, _, publicE, order := twoPlayerPubs(t)
	kr, err := NewKeyring("alice", alice, publicE, order)
	if err != nil {
		t.Fatalf("NewKeyring: %v", err)
	}
	idx, ok := kr.SeatIndex("alice")
	if !ok || idx != 0 {
		t.Errorf("SeatIndex(alice): got %d, %v; want 0, true", idx, ok)
	}
	idx, ok = kr.SeatIndex("bob")
	if !ok || idx != 1 {
		t.Errorf("SeatIndex(bob): got %d, %v; want 1, true", idx, ok)
	}
	if _, ok := kr.SeatIndex("nobody"); ok {
		t.Error("SeatIndex(nobody) should be false")
	}
	if _, ok := (*Keyring)(nil).SeatIndex("alice"); ok {
		t.Error("SeatIndex on nil keyring should be false")
	}
}

func TestKeyring_CopiesAreIndependent(t *testing.T) {
	alice, _, publicE, order := twoPlayerPubs(t)
	kr, err := NewKeyring("alice", alice, publicE, order)
	if err != nil {
		t.Fatalf("NewKeyring: %v", err)
	}
	seats := kr.SeatOrder()
	seats[0] = "mutated"
	if kr.SeatOrder()[0] != "alice" {
		t.Error("mutating SeatOrder copy changed the keyring")
	}
	local := kr.LocalKey()
	local.D.Add(local.D, big.NewInt(1))
	if kr.LocalKey().D.Cmp(alice.D) != 0 {
		t.Error("mutating LocalKey copy changed the stored key")
	}
	pub, _ := kr.Public("bob")
	pub.E.Add(pub.E, big.NewInt(1))
	again, _ := kr.Public("bob")
	if again.E.Cmp(pub.E) == 0 {
		t.Error("mutating Public copy changed the stored public key")
	}
}
