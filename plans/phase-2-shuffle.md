# Phase 2 — Distributed shuffle FSM (implementation spec)

Parent: [`CRYPTO_DEAL_PLAN.md`](../CRYPTO_DEAL_PLAN.md). Consumes Phase 1 `Keyring`. Fixes the **library** half of issues 2–3 for shuffle: each peer runs **one** encrypt-then-permute step with its own `d`, publishes **output deck + commitment only**, and a turn-taking sequencer applies `SHUFFLE_STEP`s in seat order even if GossipSub delivers them out of order.

**This phase does not change live dealing.** `poker host` / `poker join` still shuffle from the shared seed. Phase 5 is when this FSM is driven by `OnShuffleStep` / `BroadcastShuffleStep`.

**Do not implement later phases from this doc.** After this lands and tests pass, stop.

---

## Why this phase exists

Today:

| Fact | Where |
|---|---|
| `ExecuteStep` encrypts + secret-permutes + commits | `internal/crypto/shuffle.go` |
| `RunFullShuffle` runs **every** seat’s step in one process with every `*SRAKey` | same file; used by `CryptoGame.RunShuffle` |
| `ShuffleStep.Permutation` lives on the in-process struct | never on the proto (good) |
| `BroadcastShuffleStep` already sends deck + hash + nonce, **not** the permutation | `internal/network/node.go` |
| `OnShuffleStep` exists but is unset in `runP2PMode` | `cmd/poker/main.go` |
| GossipSub does not preserve send order | same reason `actionSequencer` exists for `PlayerAction` |
| Phase 1 `Keyring` can encrypt with any seat’s `e` and decrypt only with local `d` | `internal/crypto/keyring.go` |

`RunFullShuffle` is the right **oracle** (one machine, all keys). It is the wrong **protocol**. On the network:

1. Only the player whose turn it is may produce a step, and only with `Keyring.LocalKey()`.
2. Everyone else verifies the commitment and adopts `OutputDeck` as the next input.
3. A later seat’s gossip message can arrive **before** an earlier seat’s. Apply-by-arrival-order would desync replicas. Sequence by **seat index**, buffer the rest.

Phase 2 is that FSM plus an in-process fake bus. It does **not** deal cards and does **not** touch `main.go`.

Keep `RunFullShuffle` / `CryptoGame` as the in-process oracle. Existing shuffle tests stay valid.

---

## Goal / done when

1. A `ShuffleSession` per replica: same `Keyring` seat order, same session id, same hand number, same initial plaintext deck.
2. If it is our turn: `ExecuteStep` with **local** key; the object that leaves the session has **output deck + commitment only** (no permutation, no input deck).
3. If it is not our turn: accept only the expected seat’s message, `VerifyStep`, adopt output as next input. Buffer future seats; reject wrong player / wrong hand / bad commitment / conflicting duplicate.
4. Fake bus (Go channels, **no libp2p**): 2- and 3-player replicas all finish with **identical** ciphertexts.
5. A replica that only has public `e`s (plus its own `d`) cannot turn the final deck into ranks/suits.
6. `RunFullShuffle` tests still pass. `go test ./...` is green.
7. Live multiplayer is still the shared-seed path. No `OnShuffleStep` wiring.

You should be able to explain: why N sequential shuffles beat a single shuffler; why the commitment does not prove the permutation is random (it only binds the published deck); why gossip needs a turn index.

---

## Crypto reminder (do not re-derive)

SRA is commutative: after every seat encrypts, card `m` is `m^{e_1 e_2 … e_n} mod P`. Each seat also applies a **secret** permutation. Nobody knows the final order unless they collude with every shuffler.

`VerifyStep` today only checks `Commitment.VerifyDeck(OutputDeck)`. That is intentional:

- Verifiers **cannot** check the permutation (it never leaves the shuffling peer).
- Verifiers **cannot** check that the shuffler used their real `e` without a shuffle argument (out of scope; we are not replacing SRA).
- The commitment **does** stop a shuffler from later claiming a different output deck (equivocation on the published ciphertext).

`SHUFFLE_COMMIT` in the proto stays unused. `SHUFFLE_STEP` already carries `commitment_hash` + `commitment_nonce`. Do not add proto fields.

`P` is not sent. Honest nodes use `Keyring.Modulus()` (= `SharedPrime()` in production).

Deck on the wire is `[][]byte` of `big.Int.Bytes()` (see `DeckToWire`). Phase 2 talks in `[]*big.Int`; Phase 5 maps through the existing codec.

---

## Scope

### Touch

| File | Change |
|---|---|
| `internal/crypto/shuffle_session.go` | **new** — `ShuffleMessage`, `ShuffleSession` |
| `internal/crypto/shuffle_session_test.go` | **new** — FSM, sequencer, fake-net, privacy |
| `internal/crypto/shuffle.go` | small guards only (private key on `ExecuteStep`; nil-safe `VerifyStep`; copy decks in the step). **Do not** rewrite `RunFullShuffle`. |
| `internal/crypto/keyring.go` | tiny helper `SeatIndex(peerID) (int, bool)` |

### Do not touch

- `cmd/poker/main.go` — no `OnShuffleStep`, no `BroadcastShuffleStep`, no `StartHand` change
- `internal/crypto/deal.go`, `crypto_game.go`, `zkp.go`, `commit.go`, `params.go`, `sra.go`
- `internal/game/*`
- `internal/network/*` including `node.go`, `coordinator.go`, `codec.go`, proto / `messages.proto` / `messages.pb.go`
- `BroadcastShuffleStep` callers (there are still none — keep it that way)
- Ethereum, Shamir, fault manager, TUI, README / CLI help

`crypto` must **not** import `network`. The session speaks `ShuffleMessage`, not the proto type. Phase 5 maps:

```
proto ShuffleStep  ←→  crypto.ShuffleMessage  (DeckToWire / DeckFromWire already exist)
```

Fixing `BroadcastJoin` was Phase 1. Do not reopen `node.go`.

---

## Current code to reuse (do not rewrite)

```go
type ShuffleStep struct {
    PlayerID     string
    InputDeck    []*big.Int
    OutputDeck   []*big.Int
    Permutation  []int          // LOCAL ONLY — never copy into ShuffleMessage
    Commitment   *Commitment
}

func NewShuffleProtocol(p *big.Int, sessionID []byte) *ShuffleProtocol  // NumCards = 52
func (sp *ShuffleProtocol) ExecuteStep(playerID string, deck []*big.Int, key *SRAKey) (*ShuffleStep, error)
func (sp *ShuffleProtocol) VerifyStep(step *ShuffleStep) error
func (sp *ShuffleProtocol) RunFullShuffle(...) ([]*big.Int, []*ShuffleStep, error) // oracle; keep

func BuildPlaintextDeck(p *big.Int) []*big.Int          // 52 cards, CardToField
func NewEncryptedDeck(cards []*big.Int, p *big.Int, sessionID []byte) (*EncryptedDeck, error)
func SessionID(playerIDs []string, nonce []byte) []byte

func (kr *Keyring) LocalKey() *SRAKey                   // only private accessor
func (kr *Keyring) Public(peerID string) (*SRAKey, bool) // always D == nil
func (kr *Keyring) SeatOrder() []string
func (kr *Keyring) Modulus() *big.Int
```

`BroadcastShuffleStep` already omits `Permutation` and `InputDeck`. The session’s wire type must match that subset.

`node.dispatch` ignores self-echo (`env.SenderId == n.Host.PeerID`). The fake bus must do the same: the producer applies its own step locally; it does **not** need to receive it back. If an echo does arrive, treat it as a matching duplicate (ignore), not as a new step.

Canonical seat order is `Keyring.SeatOrder()`, which Phase 1 already aligned with `Lobby.PlayerIDs()`. Shuffle turn `k` is `SeatOrder()[k]`. Do not invent a second order.

---

## Design

### 1. `ShuffleMessage` (the thing that may leave the process)

Package `crypto`. This is the library stand-in for proto `ShuffleStep` **without** pulling protobuf into crypto.

```go
// ShuffleMessage is the published shuffle step: output deck + commitment.
// It must not contain a permutation or the input deck.
type ShuffleMessage struct {
    HandNum    int64
    PlayerID   string
    OutputDeck []*big.Int
    Commitment *Commitment
}

// ShuffleMessageFromStep copies output deck + commitment. Permutation and
// InputDeck are dropped. handNum is the session's hand number.
func ShuffleMessageFromStep(handNum int64, step *ShuffleStep) (*ShuffleMessage, error)
```

**Rules**

- `OutputDeck` is a **deep copy** (`new(big.Int).Set` each card). Callers cannot mutate the session by editing the message.
- `Commitment` is a copy of `Hash` and `Nonce` slices.
- No `Permutation` field. If you feel the urge to add one “for tests,” put it on the session privately (`localPerm []int`), never on this struct.
- `ShuffleMessageFromStep` errors if `step`, `step.OutputDeck`, or `step.Commitment` is nil.

### 2. `SeatIndex` on `Keyring`

```go
func (kr *Keyring) SeatIndex(peerID string) (int, bool)
```

Linear scan of `kr.order`. Returns `false` for unknown ids / nil keyring. Session uses this instead of duplicating the loop. Do **not** add a method that returns private keys in seat order.

### 3. `ShuffleSession`

```go
type ShuffleSession struct {
    // unexported fields; sketch for implementers:
    mu         sync.Mutex
    kr         *Keyring
    proto      *ShuffleProtocol
    handNum    int64
    nextIndex  int                 // next seat that must be applied (0..n)
    current    []*big.Int          // current input deck (plaintext, then ciphertexts)
    pending    map[int]*ShuffleMessage // future seats, at most one per index
    localPerm  []int               // set when we ExecuteStep; never published
    done       bool
}

func NewShuffleSession(kr *Keyring, sessionID []byte, handNum int64) (*ShuffleSession, error)

// Start initializes the plaintext deck. If we are seat 0, execute and return
// our ShuffleMessage. Otherwise return (nil, nil) and wait for HandleMessage.
func (s *ShuffleSession) Start() (*ShuffleMessage, error)

// HandleMessage is the sequencer. See apply rules below.
// If after applying (and draining pending) it is our turn, execute and return
// our ShuffleMessage; otherwise (nil, nil).
func (s *ShuffleSession) HandleMessage(msg *ShuffleMessage) (*ShuffleMessage, error)

func (s *ShuffleSession) Done() bool
func (s *ShuffleSession) ExpectedPlayer() string        // SeatOrder()[nextIndex]; "" if Done
func (s *ShuffleSession) NextIndex() int
func (s *ShuffleSession) EncryptedDeck() (*EncryptedDeck, error) // only when Done; 52 cards
```

**Constructor invariants (reject with error)**

1. `kr != nil` and `kr.Len() >= 2`.
2. `len(sessionID) > 0`.
3. `handNum >= 1` (engine starts hands at 1).
4. `kr.Modulus()` non-nil.
5. Session stores a **copy** of `sessionID`.
6. Production path: `proto.NumCards == 52`, initial deck = `BuildPlaintextDeck(kr.Modulus())` (copied). `Start` is what installs the deck so `NewShuffleSession` can stay side-effect light — either place is fine as long as every replica uses the same plaintext encoding.

**Do not** take `[]*SRAKey` or a mixed private/public slice. Always `kr.LocalKey()` vs expected `PlayerID` string.

### 4. Turn-taking + sequencer

`nextIndex` is the seat that must be applied next. Gossip can deliver seat 1 before seat 0.

**`HandleMessage` rules** (hold `s.mu` for the whole call)

| Incoming | Action |
|---|---|
| `msg == nil` / empty `PlayerID` / `len(OutputDeck) != proto.NumCards` / nil commitment | error |
| `msg.HandNum != s.handNum` | error (`ShuffleSession.HandleMessage: wrong hand`) |
| `PlayerID` not in `SeatOrder()` | error |
| `seat < nextIndex` and decks+commitment equal the already-applied step | **ignore** — return `(nil, nil)` (self-echo / gossip retry) |
| `seat < nextIndex` and payload **differs** | error (equivocation) |
| `seat == nextIndex` but `PlayerID != ExpectedPlayer()` | error (should be unreachable if index maps to id; still check) |
| `seat == nextIndex` | `VerifyStep` on a reconstructed `ShuffleStep{PlayerID, OutputDeck, Commitment}` (InputDeck/Permutation unused by verify). On success, adopt a copy of `OutputDeck` as `current`, `nextIndex++`. Then **drain** `pending`. |
| `seat > nextIndex` | buffer **one** message per seat index. If that index is already buffered and equal, ignore. If buffered and different, error. Do not verify yet (the input deck is not yet known / not required for commit verify, but applying early would advance the turn). |

After a successful apply (including drain): if `!Done()` and `ExpectedPlayer() == kr.LocalID()`, call `executeLocalLocked()` and return that message.

**`executeLocalLocked`**

1. `step, err := s.proto.ExecuteStep(kr.LocalID(), copyDeck(s.current), kr.LocalKey())`
2. Store `s.localPerm = copyInts(step.Permutation)` (optional; useful for debug tests; **do not** expose via a public getter that Phase 5 could accidentally log).
3. Build `ShuffleMessageFromStep`, apply our own output locally (`nextIndex++`, `current = output`), then drain pending.
4. Return the message for the caller to broadcast.

**`Start`**

1. If already started / `nextIndex != 0`, error (`ShuffleSession.Start: already started`).
2. Set `current = BuildPlaintextDeck(...)` (or the test deck).
3. If we are seat 0, `executeLocalLocked` and return the message.
4. Else return `(nil, nil)`.

**`Done`** iff `nextIndex == kr.Len()`.

**`EncryptedDeck`** errors if `!Done()`. Uses `NewEncryptedDeck(current, modulus, sessionID)`. Production `NumCards` is 52, which `NewEncryptedDeck` already requires.

**Concurrency.** Fake-net tests use one goroutine per replica. `Start` / `HandleMessage` / `Done` / `EncryptedDeck` may be called from that goroutine while the test waits on `Done` from the parent — put a mutex on the session. Keyring stays immutable (Phase 1).

### 5. Small `shuffle.go` guards (in scope)

| Change | Why |
|---|---|
| `ExecuteStep`: if `!key.IsPrivate()` → error | Session must never shuffle with `Public(...)`. |
| `ExecuteStep`: copy `InputDeck` / `OutputDeck` into the returned step | Prevent aliasing with the caller’s slice. |
| `VerifyStep`: nil `step` / nil `Commitment` → error, no panic | Session feeds reconstructed steps. |

Do **not** change `RunFullShuffle` control flow. Do **not** “improve” `VerifyStep` to check permutations (impossible without the perm) or to re-encrypt the input (would require knowing the perm).

Empty `fmt.Errorf("")` leftovers in `VerifyStep` / `RunFullShuffle` stay unless you are already touching that line for a nil guard. Match Phase 1: new errors use a function prefix.

### 6. Test-only smaller decks (keep production at 52)

`P = 23` cannot hold 52 distinct `CardToField` values. FSM reject/sequencer tests should stay **fast**.

Unexported constructor used only from `shuffle_session_test.go` (same package):

```go
func newShuffleSessionN(kr *Keyring, sessionID []byte, handNum int64, initial []*big.Int) (*ShuffleSession, error)
```

Sets `proto.NumCards = len(initial)`. `EncryptedDeck()` is **not** required for these tests (it demands 52). Assert on `current` via an unexported test helper, e.g. `func (s *ShuffleSession) testDeck() []*big.Int`, or compare by running until `Done()` and reading a test-only getter.

Production `NewShuffleSession` must **not** expose `NumCards`. Do not add a public `WithNumCards` option.

Tiny-deck encoding on `smallPrime`: use values that pass `validateMessage` (`1 ≤ m ≤ P-2`), e.g. `2, 3, 4, 6` (four cards). Do **not** use `BuildPlaintextDeck(smallPrime)`.

### 7. Fake bus (tests only — not a production type)

In `shuffle_session_test.go`:

```go
type fakeShuffleBus struct {
    chans map[string]chan *ShuffleMessage // buffer cap >= 16
}

func (b *fakeShuffleBus) Broadcast(from string, msg *ShuffleMessage)
```

- Do **not** deliver to `from` (mirrors `node.dispatch` self-ignore).
- Deep-copy the message per recipient so replicas do not share `*big.Int`.
- Each replica goroutine: `Start` → maybe broadcast; then loop `HandleMessage` until `Done` or context timeout.
- Use `context.WithTimeout` (e.g. 30s for tiny decks, 2 minutes for 2048-bit 52-card). A hung sequencer must fail the test, not `go test` forever.

Do **not** put `fakeShuffleBus` in `shuffle_session.go`.

---

## Tests

Tiny-deck + `smallPrime` unless a row says `SharedPrime()`.

Helper: build N full keys; each replica’s `NewKeyring(localID, localFull, publicEBytes, order)` — **only** `E.Bytes()` in the map (same as Phase 1).

### A. Wire shape — `shuffle_session_test.go`

| Test | Assert |
|---|---|
| `TestShuffleMessageFromStep_OmitsPermutation` | After `ExecuteStep`, `ShuffleMessageFromStep` has matching player / 52-or-N deck / commitment; there is no way to read `Permutation` off the message. Mutating the message deck does not change `step.OutputDeck`. |
| `TestExecuteStep_RejectsPublicOnlyKey` | `ExecuteStep(..., kr.Public(localID))` errors. |

### B. Session FSM (tiny deck, 2–3 players, **single goroutine**, inject messages by hand)

| Test | Assert |
|---|---|
| `TestShuffleSession_Seat0Starts` | Alice is seat 0; `Start()` returns a message; `NextIndex()==1`; Bob `Start()` returns nil. |
| `TestShuffleSession_TwoPlayers_Sequential` | Alice start → Bob `HandleMessage` → Bob returns his step → Alice `HandleMessage` → both `Done()`; decks equal. |
| `TestShuffleSession_WrongSeatRejected` | Bob’s message fed to Alice while `ExpectedPlayer` is Alice (or while waiting for Alice on Carol) → error; `nextIndex` unchanged. |
| `TestShuffleSession_WrongHandRejected` | `HandNum` 2 into a hand-1 session → error. |
| `TestShuffleSession_TamperedCommitmentRejected` | Flip one bit of `Commitment.Hash` (or one limb of a card without updating the hash) → `HandleMessage` error; session not advanced. |
| `TestShuffleSession_DuplicateIgnored` | Deliver the same valid Alice step twice to Bob → second call `(nil, nil)`, still `nextIndex==1` then after first apply `nextIndex==2` and second is ignore. |
| `TestShuffleSession_ConflictingDuplicateRejected` | Same player/seat already applied, **different** deck → error. |
| `TestShuffleSession_BuffersOutOfOrder` | Carol receives Bob’s step **before** Alice’s. First call buffers (`Done` still false, `nextIndex==0`). Second call (Alice) applies Alice, drains Bob, and if Carol is next she produces. Final decks match a sequentially driven replica. |
| `TestShuffleSession_UnknownPlayerRejected` | `PlayerID: "mallory"` → error. |

### C. Fake-net (goroutines + channels)

| Test | Assert |
|---|---|
| `TestShuffleSession_FakeNet_2Players` | Tiny deck, `smallPrime`. Two goroutines, two keyrings. Both `Done`; decks identical; `len==NumCards`; not equal to the initial plaintext (shuffle had an effect — retry once on the ~1/n! fluke if you want, or just check ciphertext ≠ plaintext values). |
| `TestShuffleSession_FakeNet_3Players` | Same, three replicas. All three decks identical. |
| `TestShuffleSession_FakeNet_2Players_ProductionDeck` | **`SharedPrime()` + 52 cards + `NewShuffleSession` (not `newShuffleSessionN`)**. Two replicas. Both `Done`; `EncryptedDeck()` succeeds; `Cards` equal across replicas. This is the slow test; it is the one that matches Phase 5. |

Existing `TestShuffle_FourPlayers` already proves `RunFullShuffle` on 2048-bit. The production fake-net test proves **replicas agree** without sharing `d`.

### D. Privacy

| Test | Assert |
|---|---|
| `TestShuffleSession_PublicCannotRecoverPlaintext` | Use the production 2-player result (or run a dedicated `SharedPrime` 2-player session). For a replica: `FieldToCard(final[i], P) == -1` for every i (ciphertexts are not plaintext encodings). `LocalKey().DecryptAll(final)` still has `FieldToCard == -1` (one layer remains). `Public(other).Decrypt` errors (`d` absent). |

Do **not** run this assertion on `smallPrime`: accidental landing on a card encoding is possible.

Do **not** require the distributed final deck to equal a `RunFullShuffle` oracle run — permutations are random per `ExecuteStep`. Agreement is replica-vs-replica, not replica-vs-oracle.

### E. Regression

```
go test ./internal/crypto -count=1
go test ./...
```

Must still pass: `TestShuffle_FourPlayers`, `TestShuffle_CommitmentsVerify`, `TestCryptoGame_FullProtocol`, all Phase 1 keyring tests.

No new proto. No live 2-player crypto demo.

---

## Implementation order (do this, then stop)

1. `keyring.go`: `SeatIndex` + a one-liner test in `keyring_test.go` (`TestKeyring_SeatIndex`).
2. `shuffle.go` guards (`IsPrivate`, copies, nil `VerifyStep`).
3. `ShuffleMessage` + `ShuffleMessageFromStep` + omit-perm test.
4. `ShuffleSession` production constructor + `Start` / `HandleMessage` / sequencer. Drive with the single-goroutine tests (B).
5. Fake-net 2- and 3-player tiny-deck tests (C).
6. Production-deck 2-player fake-net + privacy test (C+D).
7. `go test ./...`
8. Stop. Do not open `main.go`. Do not start `deal` peel APIs (`Peel` / `VerifyAndApply` are Phase 3).

If a step’s tests fail, fix that step. Do not start Phase 3 files.

---

## Error style

New code: `fmt.Errorf` / `errors.New` with a function prefix (`NewShuffleSession: ...`, `ShuffleSession.HandleMessage: ...`, `ExecuteStep: private exponent d is not present`). Match Phase 1 / `GenerateSRAKey`. Do not drive-by rewrite empty `fmt.Errorf("")` leftovers in `RunFullShuffle`.

---

## Explicit non-goals (push back if asked mid-phase)

- Wiring `OnShuffleStep` / `BroadcastShuffleStep` in `runP2PMode` — Phase 5.
- Dealing hole / community / showdown peels — Phase 3.
- `StartHandCrypto` / nil-deck streets — Phase 4.
- Using `SHUFFLE_COMMIT` as a second message.
- Putting `Permutation` on the proto “just for debug.”
- Verifying that a shuffler used a uniform permutation (needs a real shuffle argument).
- Timeouts / abort-hand-on-stall (issue 6). Fake-net tests use a context timeout; the session itself has no timer.
- libp2p in this phase. If a test starts a `Node`, it is out of scope.
- Replacing `RunFullShuffle` with the session inside `CryptoGame`.

---

## How Phase 3 / 5 will consume this (do not implement)

Phase 3 (peels) needs the agreed ciphertext deck:

```go
ed, err := session.EncryptedDeck()
ct, err := ed.CardAt(index)
// Peel with kr.LocalKey() only
```

Phase 5:

```go
kr, err := network.KeyringFromLobby(localPeerID, localSRAKey, node.Lobby)
sid := pokercrypto.SessionID(kr.SeatOrder(), lobby.SessionNonce())
sess, err := pokercrypto.NewShuffleSession(kr, sid, int64(handNum))

node.OnShuffleStep = func(pb *network.ShuffleStep) {
    msg := &pokercrypto.ShuffleMessage{
        HandNum:    pb.HandNum,
        PlayerID:   pb.PlayerId,
        OutputDeck: network.DeckFromWire(pb.Deck),
        Commitment: &pokercrypto.Commitment{Hash: pb.CommitmentHash, Nonce: pb.CommitmentNonce},
    }
    out, err := sess.HandleMessage(msg)
    if out != nil {
        _ = node.BroadcastShuffleStep(ctx, out.HandNum, /* reconstruct crypto.ShuffleStep for the existing helper */)
    }
}

out, err := sess.Start()
if out != nil {
    _ = node.BroadcastShuffleStep(...)
}
```

If Phase 2 accidentally put `Permutation` on `ShuffleMessage`, Phase 5 will leak it the first time someone logs the struct. If Phase 2 applies gossip order instead of seat index, two honest nodes will disagree on the deck and Phase 5 will hang or desync.

---

## Review checklist (before calling Phase 2 done)

- [ ] `ShuffleMessage` has no permutation / input-deck fields
- [ ] `ExecuteStep` refuses a public-only key
- [ ] `Start` only produces a message when local seat is 0
- [ ] Wrong-seat, wrong-hand, tampered commitment rejected; turn index unchanged
- [ ] Out-of-order future step is buffered, then applied after the gap fills
- [ ] Matching duplicate ignored; conflicting duplicate rejected
- [ ] Fake-net 2- and 3-player: identical ciphertexts
- [ ] Production 52-card 2-player fake-net: `EncryptedDeck` agrees
- [ ] Privacy: public/`LocalKey` alone cannot `FieldToCard` the final deck
- [ ] `RunFullShuffle` / `TestCryptoGame_FullProtocol` still pass
- [ ] `go test ./...` green
- [ ] `cmd/poker/main.go` diff is empty
- [ ] `internal/network` diff is empty (except you must not touch it at all)
- [ ] No new proto fields; `SHUFFLE_COMMIT` still unused

---

## Time

Half a day of AI implementation including tests, if this spec is followed without wiring `main.go`. The 2048-bit 52-card fake-net test is the slow one (seconds, same class as `TestShuffle_FourPlayers`); tiny-deck tests should be milliseconds.

If the work grows into peels, `machine.go`, or libp2p, stop and split — do not pull Phase 3 into this conversation.
