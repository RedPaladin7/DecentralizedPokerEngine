# Phase 1 — Keyring (implementation spec)

Parent: [`CRYPTO_DEAL_PLAN.md`](../CRYPTO_DEAL_PLAN.md). Fixes the **library** half of issues 1–2: each peer must be able to hold **only its own** private exponent `d`, while storing everyone else’s public `e`. Also fixes the `--no-crypto` join panic (issue 1 / risk note 5).

**This phase does not change live dealing.** `poker host` / `poker join` still shuffle from the shared seed. Phase 5 is when Keyring is used in `runP2PMode`.

**Do not implement later phases from this doc.** After this lands and tests pass, stop.

---

## Why this phase exists

Today:

| Fact | Where |
|---|---|
| `NewCryptoGame` generates **every** `(e, d)` in one process | `internal/crypto/crypto_game.go` |
| `JOIN_TABLE` already carries `sra_pub_key_e`; lobby already stores it | proto + `Lobby.HandleJoin` |
| `BroadcastJoin` does `n.sraKey.PublicKey().Bytes()` with no nil check | `internal/network/node.go` |
| `--no-crypto` sets `sraKey = nil` then still calls `BroadcastJoin` | `cmd/poker/main.go` → panic |

Later phases (shuffle FSM, peels, `StartHandCrypto`) all need the same invariant:

> This node can encrypt with any seated player’s `e`. It can decrypt only with **our** `d`. No other `d` is in memory.

Phase 1 builds that object and the lobby helpers that feed it. It does **not** run a shuffle.

Keep `NewCryptoGame` / `RunShuffle` / `DealToEngine` as the **in-process oracle**. Tests that already hold every key on one machine stay valid.

---

## Goal / done when

1. A public-only `SRAKey` (built from `(p, e)`, `d == nil`) encrypts; decrypt returns an error (no panic).
2. `Keyring` holds the local full key plus a map of **public-only** keys. There is no API that returns another peer’s `d`. Asking for the local peer’s *public* view also returns `d == nil`.
3. Lobby can list public exponents in canonical seat order and answer “does every seat have a non-empty `e`?”
4. `BroadcastJoin` with `sraKey == nil` publishes empty `sra_pub_key_e` and does not panic. `--no-crypto` join works.
5. `go test ./...` is green. Existing `internal/crypto` tests still pass, including `TestCryptoGame_FullProtocol`.
6. Live multiplayer is still the shared-seed path. No shuffle/deal wiring in `cmd/poker`.

You should be able to explain: why `e` can be public and `d` cannot; why the local simulation is still allowed in tests.

---

## Crypto reminder (do not re-derive)

SRA on the RFC 3526 2048-bit MODP prime `P` (`SharedPrime()`):

- `c = m^e mod P`, `m = c^d mod P`
- `e · d ≡ 1 (mod P-1)`
- Encryption is commutative: `E_A(E_B(m)) = E_B(E_A(m))`
- Publishing `e` lets others encrypt **to** you and check well-formedness later. Publishing `d` lets them decrypt **your** layer and recover cards. `d` never goes on the wire (Shamir shares of `d` are issue 6 / out of scope).

`JOIN_TABLE.sra_pub_key_e` encoding is already `big.Int.Bytes()` (unsigned big-endian, no length prefix). Reconstruct with `new(big.Int).SetBytes(b)`. Empty slice is **not** a valid exponent (that is the `--no-crypto` signal).

`P` is not sent; every honest node uses `SharedPrime()`.

---

## Scope

### Touch

| File | Change |
|---|---|
| `internal/crypto/sra.go` | `PublicSRAKey`, `IsPrivate`, `PublicView`; nil-`d` guards on decrypt / verify |
| `internal/crypto/keyring.go` | **new** — `Keyring` type |
| `internal/crypto/keyring_test.go` | **new** — keyring + public-key tests (or split: public-key tests may live in `crypto_test.go`) |
| `internal/crypto/crypto_test.go` | public-only `SRAKey` tests if not in `keyring_test.go` |
| `internal/network/lobby.go` | `PublicExponents`, `AllSeatsHavePublicE`; `KeyringFromLobby` |
| `internal/network/node.go` | `BroadcastJoin` nil `sraKey` |
| `internal/network/network_test.go` | lobby `e` assertion, `KeyringFromLobby`, nil-join |

### Do not touch

- `cmd/poker/main.go` — no shuffle/deal wiring, no `StartHand` change, no `OnShuffleStep` / `OnPartialDecrypt`
- `internal/crypto/shuffle.go`, `deal.go`, `crypto_game.go`, `zkp.go`, `commit.go`, `params.go`
- `internal/game/*` (`StartHand`, `StartHandCrypto`, street dealing)
- `internal/network/coordinator.go`, proto / `messages.proto` / `messages.pb.go`
- `BroadcastShuffleStep`, `SendDirectPartialDecrypt`, new message types
- Ethereum, Shamir, fault manager, TUI
- README / CLI help (Phase 5)

Fixing `BroadcastJoin` in `node.go` is enough for “`--no-crypto` no longer crashes on join.” `main.go` already passes `sraKey == nil`; it does not need a Phase 1 edit.

---

## Current code to reuse (do not rewrite)

```go
type SRAKey struct {
    E *big.Int
    D *big.Int
    P *big.Int
}

func GenerateSRAKey(p *big.Int) (*SRAKey, error)  // full pair; keep as-is
func (k *SRAKey) Encrypt(m *big.Int) (*big.Int, error)
func (k *SRAKey) Decrypt(c *big.Int) (*big.Int, error)
func (k *SRAKey) PublicKey() *big.Int             // copy of E
func (k *SRAKey) VerifyKeyPair() bool             // e*d ≡ 1 mod (P-1)
```

Lobby already copies `msg.SraPubKeyE` into `SeatInfo.SRAKeyE`. `Seats()` / `PlayerIDs()` already sort by `JoinedAtUnixMs` then `PlayerID`. Canonical order for exponents **must** be that same order (dealer / shuffle seat index will use it in Phase 2).

`NewNode(..., sraKey *SRAKey, ...)` already accepts nil. Only `BroadcastJoin` assumes non-nil.

---

## Design

### 1. Public-only `SRAKey`

Add constructors / predicates on the existing type. Do **not** add a second key type.

```go
// PublicSRAKey builds a key that can encrypt but not decrypt.
// D is nil. p and e are copied.
func PublicSRAKey(p, e *big.Int) (*SRAKey, error)

// IsPrivate reports whether D is present (local full key).
func (k *SRAKey) IsPrivate() bool

// PublicView returns a copy with D == nil. Safe to hand to other packages.
func (k *SRAKey) PublicView() *SRAKey
```

**`PublicSRAKey` validation**

- `p` and `e` non-nil
- `e > 0` and `e < p` (same range spirit as a public exponent; do not require `gcd(e, p-1) == 1` here — that is checked at generation time, and we must accept whatever `e` arrived on the wire)
- Copy `p` and `e` with `new(big.Int).Set(...)` so callers cannot mutate the key through the originals

**Nil-`d` guards** (prevent panics in `math/big`):

| Method | Public-only behavior |
|---|---|
| `Encrypt` / `EncryptAll` | unchanged (uses `E`) |
| `Decrypt` / `DecryptAll` | `error` if `k == nil` or `k.D == nil` — **never** call `Exp` with nil `D` |
| `VerifyKeyPair` | `false` if `k`, `E`, `D`, or `P` is nil (today it panics) |
| `PublicKey` | still copy of `E`; if `k` or `E` is nil, return nil (or panic-free error — prefer returning nil) |

Suggested decrypt error: `"SRAKey.Decrypt: private exponent d is not present"`.

`Encrypt` should also fail clearly if `k` or `E` or `P` is nil, for the same reason.

Do **not** change `GenerateSRAKey`. Do **not** change `ProveDecryption` in this phase.

### 2. `Keyring` (`internal/crypto/keyring.go`)

Package `crypto` must **not** import `network`. Keyring is a pure crypto object. The lobby adapter lives in `network`.

```go
type Keyring struct {
    localID string
    local   *SRAKey            // full key, D != nil
    pubs    map[string]*SRAKey // peerID → public-only (D == nil), includes local
    order   []string           // canonical seat order
}

// NewKeyring binds a local full key to everyone else's public exponents.
// publicE values are big.Int.Bytes() encodings (same as JOIN_TABLE).
// seatOrder is canonical player order (same as Lobby.PlayerIDs()).
func NewKeyring(localID string, local *SRAKey, publicE map[string][]byte, seatOrder []string) (*Keyring, error)

func (kr *Keyring) LocalID() string
func (kr *Keyring) LocalKey() *SRAKey                 // full key; D present
func (kr *Keyring) SeatOrder() []string               // copy
func (kr *Keyring) Public(peerID string) (*SRAKey, bool) // always D == nil
func (kr *Keyring) PublicExponents() []*big.Int       // copies, seat order
func (kr *Keyring) Len() int
```

**Constructor invariants (reject with error)**

1. `localID` non-empty; `local != nil` and `local.IsPrivate()`.
2. `local.P` and `local.E` non-nil.
3. `len(seatOrder) >= 2` (a table needs at least two players).
4. `seatOrder` has no duplicates; `localID` appears exactly once.
5. `len(publicE) == len(seatOrder)`; every ID in `seatOrder` has a **non-empty** `publicE` entry. Extra map keys not in `seatOrder` → error (prevents silently ignoring a leaked extra `d` blob).
6. For every peer, `PublicSRAKey(local.P, eFromBytes)` succeeds.
7. `SetBytes(publicE[localID]).Cmp(local.E) == 0`. If the lobby copy of *our* `e` does not match the key we generated, refuse. Do not “fix it up.”
8. Store **only** `PublicSRAKey` in `pubs`, including for `localID`. Even if a confused caller later exists, `Public(localID).D` is nil.

**API rules (the whole point of this type)**

- `Public` **always** returns a public-only key (`D == nil`), including for `localID`.
- `LocalKey` is the **only** method that returns a key with `D` set, and only the local one.
- Do **not** add `KeysInSeatOrder() []*SRAKey` that mixes private + public. Phase 2/3 must call `LocalKey()` vs `Public(id)` explicitly. A mixed slice would make it too easy to feed Keyring into `RunFullShuffle` and accidentally treat every key as private.
- Do **not** take `map[string]*SRAKey` as input. Bytes/`*big.Int` cannot carry `d`.
- Immutable after `NewKeyring`. No mutex. Callers snapshot lobby first.
- `LocalKey` / `Public` / `SeatOrder` / `PublicExponents` return copies (copy `*big.Int` and the seat slice) so a caller cannot mutate the ring or set `D` on a public view.

**Optional helper (include; tiny, used by Phase 5):**

```go
func (kr *Keyring) SharedPrime() *big.Int // copy of local.P
```

Name it `Modulus()` if `SharedPrime` is too easy to confuse with the package-level function.

### 3. Lobby helpers (`internal/network/lobby.go`)

```go
// PublicExponents returns each seat's SRA e in canonical order (same as Seats()).
// Empty SRAKeyE becomes a 0-length slice in the result, not an error.
func (l *Lobby) PublicExponents() [][]byte

// AllSeatsHavePublicE is true iff every seated player has len(SRAKeyE) > 0.
func (l *Lobby) AllSeatsHavePublicE() bool

// KeyringFromLobby snapshots seats and builds a crypto.Keyring.
// Fails if AllSeatsHavePublicE is false, or if local is not seated / not private.
func KeyringFromLobby(localID string, local *pokercrypto.SRAKey, lobby *Lobby) (*pokercrypto.Keyring, error)
```

`PublicExponents` returns `[][]byte` (proto-faithful). `Keyring.PublicExponents` returns `[]*big.Int` (crypto-faithful). Do not collapse them into one type.

`KeyringFromLobby` implementation sketch:

```
seats := lobby.Seats()           // already copied + sorted
order := PlayerIDs from seats
pubs  := map[playerID]SRAKeyE
return pokercrypto.NewKeyring(localID, local, pubs, order)
```

`lobby.go` already imports protobuf only. Adding `internal/crypto` import is fine (`node.go` already imports it). Do not import `crypto` under a cycle — `crypto` must not import `network`.

### 4. `BroadcastJoin` nil guard (`node.go`)

Replace the unconditional `n.sraKey.PublicKey().Bytes()`:

```go
var eBytes []byte
if n.sraKey != nil {
    eBytes = n.sraKey.PublicKey().Bytes()
}
msg := &JoinTable{
    TableId:      n.tableID,
    PlayerName:   n.playerName,
    BuyIn:        n.buyIn,
    SraPubKeyE:   eBytes, // nil/empty when --no-crypto
    SessionNonce: []byte(n.Host.PeerID),
}
```

Protobuf `bytes` treats nil and empty the same on the wire. Local `HandleJoin` stores whatever we put here. No other `BroadcastJoin` behavior changes (self-apply to lobby, timestamp, publish).

Do not generate a throwaway SRA key in `--no-crypto` mode. Empty `e` is the explicit debug signal.

---

## Tests

Use `smallPrime` (`P = 23`) for Keyring / public-key unit tests so they stay fast. Use `SharedPrime()` only where an existing test already does, or for one “bytes round-trip matches `PublicKey().Bytes()`” case.

### A. `SRAKey` public-only — `internal/crypto`

| Test | Assert |
|---|---|
| `TestPublicSRAKey_Encrypts` | `PublicSRAKey(p, full.E)` encrypts the same as `full.Encrypt` for a field element (e.g. `CardToField(7, p)` or `m = 2` on `smallPrime`) |
| `TestPublicSRAKey_DecryptErrors` | `Decrypt` and `DecryptAll` return error; `IsPrivate() == false` |
| `TestPublicSRAKey_VerifyKeyPairFalse` | `VerifyKeyPair() == false` (no panic) |
| `TestSRAKey_PublicView_HidesD` | `full.PublicView().D == nil`; original `full.D` still set; mutating the view’s `E` does not change `full.E` |
| `TestPublicSRAKey_RejectsNil` | nil `p` or nil `e` → error |
| `TestDecrypt_NilKey_NoPanic` | `(*SRAKey)(nil).Decrypt(...)` errors (optional but cheap) |

Existing tests must still pass: `TestGenerateSRAKey_ValidPair`, `TestSRA_EncryptDecryptRoundTrip`, `TestSRA_Commutativity`, `TestCryptoGame_FullProtocol`.

### B. `Keyring` — `internal/crypto/keyring_test.go`

Helper: generate two (or three) full keys on `smallPrime`; publish only `E.Bytes()`.

| Test | Assert |
|---|---|
| `TestNewKeyring_OK` | 2 players; `Len()==2`; `LocalKey().IsPrivate()`; `Public(local).D == nil`; `Public(other).D == nil`; `Public(other).E` matches the other key’s `E` |
| `TestKeyring_Public_NeverReturnsD` | for every ID in `SeatOrder()`, `Public(id)` has `D == nil`; only `LocalKey().D` is non-nil |
| `TestKeyring_RejectsEmptyPeerE` | empty `[]byte{}` for a peer → error |
| `TestKeyring_RejectsLocalEMismatch` | `publicE[localID]` is some other key’s `e` → error |
| `TestKeyring_RejectsPublicOnlyLocal` | `local = PublicSRAKey(...)` → error |
| `TestKeyring_RejectsUnknownLocalID` | `localID` not in `seatOrder` → error |
| `TestKeyring_RejectsDuplicateSeats` | duplicate in `seatOrder` → error |
| `TestKeyring_RejectsExtraMapKey` | `publicE` has an ID not in `seatOrder` → error |
| `TestKeyring_PublicExponents_SeatOrder` | exponents follow `seatOrder`, not map iteration |
| `TestKeyring_PublicMissingPeer` | `Public("nobody")` → `ok == false` |
| `TestKeyring_SinglePlayerRejected` | `len(seatOrder)==1` → error |

Do **not** add a test that reconstructs plaintext of another player’s card from Keyring alone — that is Phase 2/3. Phase 1 only proves `d` is not stored.

### C. Lobby — `internal/network/network_test.go`

`HandleJoin` already stores `SraPubKeyE`. Add an **assertion** test; do not change `HandleJoin`.

| Test | Assert |
|---|---|
| `TestLobby_StoresSRAPubKeyE` | join with `SraPubKeyE: []byte{0x01, 0x02, 0x03}`; `Seats()[0].SRAKeyE` equal (copy, not mutated if caller changes the slice afterward — if today’s code aliases the slice, **copy on store** in `HandleJoin`; that is a one-line hardening, in scope) |
| `TestLobby_PublicExponents_CanonicalOrder` | three joins with explicit timestamps (same style as `TestLobby_PlayerIDs_InJoinOrder`); `PublicExponents()` order matches `PlayerIDs()` |
| `TestLobby_AllSeatsHavePublicE` | two seats with non-empty `e` → true; one empty → false; all empty → false |
| `TestKeyringFromLobby_OK` | two joins with real `GenerateSRAKey(SharedPrime()).PublicKey().Bytes()` (or `smallPrime` — lobby does not care); `KeyringFromLobby` succeeds; `Public(peer).D == nil` |
| `TestKeyringFromLobby_MissingE` | `--no-crypto` style empty `e` → error from `KeyringFromLobby` |

Need `bytes` import in `network_test.go` if not already there.

### D. `BroadcastJoin` nil — `internal/network/network_test.go`

| Test | Assert |
|---|---|
| `TestNode_BroadcastJoin_NilSRAKey_NoPanic` | `NewNode(..., sraKey=nil, listen=/ip4/127.0.0.1/tcp/0, ...)` then `Start` then `BroadcastJoin`. Must not panic. Local lobby seat for this peer has `len(SRAKeyE)==0`. |

This starts libp2p. Follow `TestNode_BroadcastJoin_LobbyUpdated`: skip with `testing.Short()`. Do **not** require a second node unless you also want to assert the empty `e` arrives remotely (nice, not required). Local lobby self-apply is enough to prove we did not crash and we published empty bytes.

Keep `TestNode_BroadcastJoin_LobbyUpdated` (non-nil key) passing.

### E. Regression

```
go test ./internal/crypto ./internal/network
go test ./...
```

No new proto. No live 2-player crypto demo in this phase.

---

## Implementation order (do this, then stop)

1. `sra.go`: `IsPrivate`, `PublicView`, `PublicSRAKey`, decrypt/verify nil guards.
2. Public-only unit tests; run `go test ./internal/crypto -count=1`.
3. `keyring.go` + `keyring_test.go`; run the same package tests.
4. Lobby helpers + `KeyringFromLobby` + tests.
5. `BroadcastJoin` nil guard + nil-join test.
6. `go test ./...`
7. Stop. Do not open `main.go` for shuffle wiring.

If a step’s tests fail, fix that step. Do not start Phase 2 files (`shuffle_session.go`, etc.).

---

## Error style

New code should use non-empty `fmt.Errorf` / `errors.New` with a function prefix (`NewKeyring: ...`, `PublicSRAKey: ...`). Match `GenerateSRAKey`, not the empty `fmt.Errorf("")` leftovers elsewhere. Do not drive-by rewrite those leftovers.

---

## Explicit non-goals (push back if asked mid-phase)

- Wiring Keyring into `runP2PMode` after lobby fill — Phase 5.
- Using Keyring inside `ShuffleProtocol` / `DealProtocol` — Phases 2–3.
- Validating `gcd(e, P-1)==1` on join (malicious `e` is a later concern; empty `e` is the only check now).
- Copying `P` onto the wire.
- Shamir split of `d` at join.
- Changing seat order (issue 8).

---

## How Phase 2 will consume this (do not implement)

```
kr, err := network.KeyringFromLobby(localPeerID, localSRAKey, node.Lobby)
// our turn:
step, err := shuffle.ExecuteStep(localID, deck, kr.LocalKey())
// not our turn:
pub, ok := kr.Public(claimedPlayerID) // encrypt not needed; VerifyStep uses commitment
```

Phase 3:

```
pd := Peel(kr.LocalKey(), ciphertext, ...)
next, err := VerifyAndApply(current, pd)
// recipient last:
plaintext, err := kr.LocalKey().Decrypt(current)
```

If Keyring accidentally stored every `d`, those phases cannot claim privacy even with a perfect FSM.

---

## Review checklist (before calling Phase 1 done)

- [ ] `PublicSRAKey` encrypts; decrypt errors; no `math/big` panic
- [ ] `Keyring.Public` never returns non-nil `D`
- [ ] `Keyring.LocalKey` is the only private key accessor
- [ ] Constructor rejects empty `e`, mismatched local `e`, public-only local, extra map keys
- [ ] `Lobby.PublicExponents` order == `PlayerIDs`
- [ ] `Lobby.AllSeatsHavePublicE` false when any `SRAKeyE` empty
- [ ] `BroadcastJoin` with nil key does not panic; stores empty `e`
- [ ] `NewCryptoGame` untouched; `TestCryptoGame_FullProtocol` still passes
- [ ] `go test ./...` green
- [ ] `cmd/poker/main.go` diff is empty
- [ ] No new proto fields

---

## Time

Half a day of AI implementation including tests, if this spec is followed without expanding into shuffle. If it grows past `sra.go` + `keyring.go` + lobby helpers + `BroadcastJoin`, stop and split — do not pull Phase 2 into this PR/conversation.
