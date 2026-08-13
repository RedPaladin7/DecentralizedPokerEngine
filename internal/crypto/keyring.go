package crypto

import (
	"errors"
	"fmt"
	"math/big"
)

// Keyring holds this node's full SRA key and public-only keys for every seat.
// There is no API that returns another peer's private exponent d.
type Keyring struct {
	localID string
	local   *SRAKey            // full key, D != nil
	pubs    map[string]*SRAKey // peerID → public-only (D == nil), includes local
	order   []string           // canonical seat order
}

// NewKeyring binds a local full key to everyone else's public exponents.
// publicE values are big.Int.Bytes() encodings (same as JOIN_TABLE).
// seatOrder is canonical player order (same as Lobby.PlayerIDs()).
func NewKeyring(localID string, local *SRAKey, publicE map[string][]byte, seatOrder []string) (*Keyring, error) {
	if localID == "" {
		return nil, errors.New("NewKeyring: localID is empty")
	}
	if local == nil || !local.IsPrivate() {
		return nil, errors.New("NewKeyring: local key must be private")
	}
	if local.P == nil || local.E == nil {
		return nil, errors.New("NewKeyring: local key missing P or E")
	}
	if len(seatOrder) < 2 {
		return nil, fmt.Errorf("NewKeyring: need at least 2 seats, got %d", len(seatOrder))
	}

	seen := make(map[string]struct{}, len(seatOrder))
	localFound := false
	for _, id := range seatOrder {
		if id == "" {
			return nil, errors.New("NewKeyring: empty seat ID")
		}
		if _, dup := seen[id]; dup {
			return nil, fmt.Errorf("NewKeyring: duplicate seat %q", id)
		}
		seen[id] = struct{}{}
		if id == localID {
			localFound = true
		}
	}
	if !localFound {
		return nil, fmt.Errorf("NewKeyring: localID %q not in seatOrder", localID)
	}
	if len(publicE) != len(seatOrder) {
		return nil, fmt.Errorf("NewKeyring: publicE has %d entries, seatOrder has %d", len(publicE), len(seatOrder))
	}
	for id := range publicE {
		if _, ok := seen[id]; !ok {
			return nil, fmt.Errorf("NewKeyring: extra publicE key %q not in seatOrder", id)
		}
	}

	localEFromLobby := new(big.Int).SetBytes(publicE[localID])
	if localEFromLobby.Cmp(local.E) != 0 {
		return nil, errors.New("NewKeyring: local public e does not match local key")
	}

	pubs := make(map[string]*SRAKey, len(seatOrder))
	for _, id := range seatOrder {
		raw, ok := publicE[id]
		if !ok || len(raw) == 0 {
			return nil, fmt.Errorf("NewKeyring: empty or missing public e for %q", id)
		}
		e := new(big.Int).SetBytes(raw)
		pub, err := PublicSRAKey(local.P, e)
		if err != nil {
			return nil, fmt.Errorf("NewKeyring: public key for %q: %w", id, err)
		}
		pubs[id] = pub
	}

	order := make([]string, len(seatOrder))
	copy(order, seatOrder)

	return &Keyring{
		localID: localID,
		local:   cloneSRAKey(local),
		pubs:    pubs,
		order:   order,
	}, nil
}

func (kr *Keyring) LocalID() string {
	if kr == nil {
		return ""
	}
	return kr.localID
}

// LocalKey returns a copy of the local full key (D present).
func (kr *Keyring) LocalKey() *SRAKey {
	if kr == nil {
		return nil
	}
	return cloneSRAKey(kr.local)
}

// SeatOrder returns a copy of canonical seat order.
func (kr *Keyring) SeatOrder() []string {
	if kr == nil {
		return nil
	}
	out := make([]string, len(kr.order))
	copy(out, kr.order)
	return out
}

// Public always returns a public-only key (D == nil), including for localID.
func (kr *Keyring) Public(peerID string) (*SRAKey, bool) {
	if kr == nil {
		return nil, false
	}
	pub, ok := kr.pubs[peerID]
	if !ok {
		return nil, false
	}
	return cloneSRAKeyPublic(pub), true
}

// PublicExponents returns copies of each seat's e in canonical seat order.
func (kr *Keyring) PublicExponents() []*big.Int {
	if kr == nil {
		return nil
	}
	out := make([]*big.Int, len(kr.order))
	for i, id := range kr.order {
		out[i] = new(big.Int).Set(kr.pubs[id].E)
	}
	return out
}

func (kr *Keyring) Len() int {
	if kr == nil {
		return 0
	}
	return len(kr.order)
}

// SeatIndex returns the canonical seat index of peerID.
func (kr *Keyring) SeatIndex(peerID string) (int, bool) {
	if kr == nil {
		return 0, false
	}
	for i, id := range kr.order {
		if id == peerID {
			return i, true
		}
	}
	return 0, false
}

// Modulus returns a copy of the shared SRA modulus P.
func (kr *Keyring) Modulus() *big.Int {
	if kr == nil || kr.local == nil || kr.local.P == nil {
		return nil
	}
	return new(big.Int).Set(kr.local.P)
}
