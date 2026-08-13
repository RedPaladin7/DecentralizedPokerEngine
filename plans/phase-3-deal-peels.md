# Phase 3 — Distributed peels (implementation spec)

Parent: [`CRYPTO_DEAL_PLAN.md`](../CRYPTO_DEAL_PLAN.md). Consumes Phase 1 `Keyring` and Phase 2’s agreed ciphertext deck. Fixes the **library** half of issue 3 for dealing: each peer peels with **only its own** `d`, hole cards are finished locally by the recipient, community and showdown peels are public, and a turn-taking sequencer applies `PARTIAL_DECRYPT`s in peel order even if GossipSub delivers them out of order.

**This phase does not change live dealing.** `poker host` / `poker join` still shuffle from the shared seed. Phase 5 is when this FSM is driven by `OnPartialDecrypt` / `SendDirectPartialDecrypt` / `BroadcastPartialDecrypt`.

**Do not implement later phases from this doc.** After this lands and tests pass, stop.

---

## Why this phase exists

Today:

| Fact | Where |
|---|---|
| `DealProtocol.RevealToPlayer` / `RevealCommunity` peel using **every** `*SRAKey` in a slice | `internal/crypto/deal.go` |
| `applyPartialDecryption` calls `key.Decrypt` + `ProveDecryption` in one process | same file |
| Recipient’s last decrypt never becomes a `PartialDecryption` (good) | `RevealToPlayer` |
| `SubstitutePartialDecryption` + ZK already catch a wrong result | `deal.go` / `zkp.go` / `TestDeal_MaliciousDecryption_Detected` |
| `BroadcastPartialDecrypt` / `SendDirectPartialDecrypt` exist; **no caller** in `cmd/poker` | `internal/network/node.go` |
| `OnPartialDecrypt` exists but is unset in `runP2PMode` | `cmd/poker/main.go` |
| Phase 1 `Keyring` can decrypt only with local `d` | `internal/crypto/keyring.go` |
| Phase 2 `ShuffleSession.EncryptedDeck()` is the agreed ciphertext | `internal/crypto/shuffle_session.go` |

`DealProtocol` is the right **oracle** (one machine, all keys). It is the wrong **protocol**. On the network:

1. A peer may peel only with `Keyring.LocalKey()`.
2. Hole cards: everyone **except** the recipient publishes a peel; the recipient peels last **locally**. Other seats’ hole cards stay unknown on this node until showdown.
3. Flop / turn / river: **everyone** peels; the last result is plaintext on every replica.
4. Showdown: public-peel the remaining players’ hole-card **indexes** (same as a community peel of private indexes). Without this, other nodes cannot evaluate.
5. Peels of one card are ordered. A later peeler’s gossip can arrive before an earlier peeler’s. Sequence by **peel-turn index**, buffer the rest.

Phase 3 is `Peel` / `VerifyAndApply` plus that FSM plus an in-process fake bus. It does **not** change `machine.go` streets and does **not** touch `main.go`.

Keep `DealProtocol` / `CryptoGame` as the in-process oracle. Existing deal tests stay valid.

---

## Goal / done when

1. `Peel(localKey, …)` produces a `PartialDecryption` with ZK. Public-only keys are refused. `VerifyAndApply` checks ZK + ciphertext chaining and returns the next ciphertext.
2. A `DealSession` per replica: same `Keyring` seat order, same session id, same hand number, same encrypted deck, same dealer index.
3. Hole deal: after both of a seat’s cards are finished, **only that seat’s replica** can `FieldToCard` them. Other replicas still have ciphertext (or nothing).
4. Community peels: all replicas finish with the **same** public cards. Burns are skipped, never peeled.
5. Showdown peels: public peels of a seat’s two hole indexes fill that seat on **every** replica; `EvaluateBest7` agrees.
6. Fake bus (Go channels, **no libp2p**): 2- and 3-player replicas. Wrong-seat / wrong-hand / bad ZK / conflicting duplicate rejected.
7. `DealHoleCards` / `DealCommunityCards` / `TestCryptoGame_FullProtocol` still pass. `go test ./...` is green.
8. Live multiplayer is still the shared-seed path. No `OnPartialDecrypt` wiring.

You should be able to explain: commutative peeling; why hole peels are unicast-ish and community peels are broadcast; why showdown is just “community peel of private indexes.”

---

## Crypto reminder (do not re-derive)

After the shuffle, card `m` at a deck index is `c = m^{e_1 e_2 … e_n} mod P`. Peeling with `d_i` removes one exponent because encryption commutes:

`c^{d_i} = m^{e_1 … e_{i-1} e_{i+1} … e_n}`.

- **Hole card for recipient R:** peel order is canonical seat order **minus R**. After N−1 verified peels, only R’s layer remains. R calls `Decrypt` locally. That last decrypt **must not** become a `PeelMessage` (it is the card).
- **Community / showdown:** peel order is the full seat order. After N peels, `FieldToCard` succeeds on every replica.
- **Showdown does not resume the hole-peel chain.** Non-recipients never received R’s last decrypt, and should not have stored the leftover ciphertext. Showdown starts again from `EncryptedDeck.CardAt(index)` with a **public** peel of all N layers.

`PartialDecryption.Verify` is the existing Schnorr-style check (`VerifyDecryption`). It does **not** bind `proof.H` to the peeler’s public `e`. Do **not** “improve” the ZK relation in this phase. Binding the peel to the expected `PlayerID` / `CardIndex` / current ciphertext is the session’s job.

`P` is not sent. Honest nodes use `Keyring.Modulus()` / `EncryptedDeck.P`.

Partial decrypt on the wire is proto `PartialDecrypt` (`player_id`, `card_index`, `ciphertext`, `result`, `proof`, `hand_num`). Phase 3 talks in `PeelMessage`; Phase 5 maps through `PartialDecryptToWire` / `PartialDecryptFromWire` (already exist). Do not add proto fields.

---

## Scope

### Touch

| File | Change |
|---|---|
| `internal/crypto/deal.go` | `Peel`, `VerifyAndApply`, `FinishHole`, `FinishPublic`, index helpers; `DealProtocol.applyPartialDecryption` / reveal paths call them. **Do not** rewrite `DealHoleCards` / `DealCommunityCards` control flow. |
| `internal/crypto/deal_session.go` | **new** — `PeelMessage`, `DealSession` |
| `internal/crypto/deal_session_test.go` | **new** — primitives, FSM, sequencer, fake-net, privacy, showdown |
| `internal/crypto/zkp.go` | small guards only (`ProveDecryption` refuses public-only keys; `PartialDecryption.Verify` nil-safe). **Do not** change the ZK math. |

### Do not touch

- `cmd/poker/main.go` — no `OnPartialDecrypt`, no `BroadcastPartialDecrypt` / `SendDirectPartialDecrypt`, no `StartHand` change
- `internal/crypto/shuffle.go`, `shuffle_session.go`, `crypto_game.go`, `commit.go`, `params.go`, `sra.go`, `keyring.go`
- `internal/game/*` (`StartHandCrypto`, `dealFlop`, showdown) — Phase 4
- `internal/network/*` including `node.go`, `codec.go`, proto / `messages.proto` / `messages.pb.go`
- Ethereum, Shamir, fault manager, TUI, README / CLI help

`crypto` must **not** import `network`. The session speaks `PeelMessage`, not the proto type. Phase 5 maps:

```
proto PartialDecrypt  ←→  crypto.PeelMessage  (PartialDecryptToWire / FromWire already exist)
```

`DealProtocol` keeps taking `[]*SRAKey` (the oracle). The session takes a `Keyring`. Do not add `KeysInSeatOrder()` on Keyring.

---

## Current code to reuse (do not rewrite)

```go
func ProveDecryption(key *SRAKey, ciphertext, result *big.Int, sessionID []byte) (*ZKProof, error)
func (pd *PartialDecryption) Verify(P *big.Int, sessionID []byte) error
func SubstitutePartialDecryption(pd *PartialDecryption, wrongResult *big.Int) *PartialDecryption

func (k *SRAKey) Decrypt(c *big.Int) (*big.Int, error) // already errors if D == nil
func FieldToCard(val *big.Int, p *big.Int) int         // -1 if not a card encoding
func game.CardFromID(id int) game.Card
func game.EvaluateBest7(cards [7]game.Card) game.EvaluatedHand

func (ed *EncryptedDeck) CardAt(index int) (*big.Int, error) // already copies
func NewEncryptedDeck(cards []*big.Int, p *big.Int, sessionID []byte) (*EncryptedDeck, error) // 52 cards

func (dp *DealProtocol) DealHoleCards(dealerIdx int) ([][2]game.Card, error)
func (dp *DealProtocol) DealCommunityCards(startPos int, batches []int) ([][]game.Card, error)
func (cg *CryptoGame) HolecardStartPos() int // == len(Players)*2

func (kr *Keyring) LocalKey() *SRAKey
func (kr *Keyring) LocalID() string
func (kr *Keyring) SeatOrder() []string
func (kr *Keyring) SeatIndex(peerID string) (int, bool)
func (kr *Keyring) Modulus() *big.Int
```

`RevealToPlayer` already skips the recipient in the loop and decrypts locally. The session must preserve that split.

Canonical seat order is `Keyring.SeatOrder()` (Phase 1 = `Lobby.PlayerIDs()`). Dealer index is an index **into that slice**, same as `DealHoleCards` / `gs.DealerIdx`.

Deck indexes must match the engine and the oracle:

```
holes:  left of dealer, two rounds          — DealHoleCards / machine.dealHoleCards
flop:   burn 1, deal 3                      — DealCommunityCards / dealFlop
turn:   burn 1, deal 1
river:  burn 1, deal 1
```

For `n` players, `start = 2*n`:

| Street | Burn index | Card indexes |
|---|---|---|
| holes | — | `0 .. 2n-1` |
| flop | `2n` | `2n+1, 2n+2, 2n+3` |
| turn | `2n+4` | `2n+5` |
| river | `2n+6` | `2n+7` |

Example: 3 players, dealer 0 → holes `0..5`, flop `7,8,9`, turn `11`, river `13`. Burns `6,10,12` are never peeled.

---

## Design

### 1. Primitives (`deal.go`)

```go
// Peel decrypts one layer with the local private key and attaches a ZK proof.
// Ciphertext / Result on the returned value are copies.
func Peel(key *SRAKey, ciphertext *big.Int, cardIndex int, playerID string, sessionID []byte) (*PartialDecryption, error)

// VerifyAndApply checks pd.Verify and that pd.Ciphertext equals current.
// Returns a copy of pd.Result as the next ciphertext.
func VerifyAndApply(current *big.Int, pd *PartialDecryption, p *big.Int, sessionID []byte) (*big.Int, error)

// FinishHole is the recipient's last local decrypt. Not a PartialDecryption.
func FinishHole(key *SRAKey, remaining *big.Int, p *big.Int) (game.Card, error)

// FinishPublic maps a fully peeled value to a card (community / showdown).
func FinishPublic(value *big.Int, p *big.Int) (game.Card, error)
```

**`Peel` rules**

1. `key.IsPrivate()`; else error (`Peel: private exponent d is not present`).
2. `playerID` non-empty; `cardIndex >= 0`; `ciphertext` non-nil; `sessionID` non-empty.
3. `result, err := key.Decrypt(ciphertext)`.
4. `proof, err := ProveDecryption(key, ciphertext, result, sessionID)`.
5. Return `&PartialDecryption{PlayerID, CardIndex, Ciphertext: copy(ciphertext), Result: copy(result), Proof}`.

**`VerifyAndApply` rules**

1. `current` and `pd` non-nil; `pd.Ciphertext` / `pd.Result` / `pd.Proof` non-nil.
2. `current.Cmp(pd.Ciphertext) == 0` else error (`VerifyAndApply: ciphertext mismatch`). This is the chain bind.
3. `pd.Verify(p, sessionID)`.
4. Return `new(big.Int).Set(pd.Result)`.

Do **not** take a `Keyring` here. Expected player / card index belong on the session.

**`FinishHole` / `FinishPublic`**

- `FinishHole`: `Decrypt` then `FieldToCard`; `-1` → error (`FinishHole: result is not a card`). Requires private key.
- `FinishPublic`: `FieldToCard` only (no decrypt). `-1` → error.

**Oracle refactor (in scope, behavior unchanged)**

`applyPartialDecryption` becomes a call to `Peel`. `RevealToPlayer` / `RevealCommunity` use `VerifyAndApply` instead of `partial.Verify` + `current = partial.Result`. Recipient branch uses `FinishHole`. Community last step uses `FinishPublic`.

Do **not** change `DealHoleCards` index arithmetic. Empty `fmt.Errorf("")` leftovers in the oracle stay unless you are already on that line for the refactor; new helpers use function-prefix errors.

### 2. Deck index helpers (`deal.go`)

Exported so Phase 4/5 and tests share one formula. Must match `DealHoleCards` / `DealCommunityCards` exactly.

```go
// HoleCardIndex is the deck index of `round` (0 or 1) for playerIdx.
// playerIdx is an index into canonical seat order. Same walk as DealHoleCards:
//   playerIdx := (dealerIdx + 1 + i) % n  at  deckPos := round*n + i
func HoleCardIndex(nPlayers, dealerIdx, playerIdx, round int) (int, error)

func CommunityStartPos(nPlayers int) int // 2*nPlayers; first burn before flop

func FlopIndexes(nPlayers int) ([3]int, error)  // start+1, start+2, start+3
func TurnIndex(nPlayers int) (int, error)       // start+5
func RiverIndex(nPlayers int) (int, error)      // start+7
```

Reject `nPlayers < 2`, out-of-range indexes, `round` not in `{0,1}`.

```go
// PeelOrder is canonical seat order, skipping recipient when recipient != "".
// Public peels pass recipient == "".
func PeelOrder(seatOrder []string, recipient string) []string
```

Copy the slice. If `recipient` is non-empty and not in `seatOrder`, error (or return error from the session that calls this). For a hole card, `len(PeelOrder) == n-1`. For 2 players that is 1 peeler.

### 3. `PeelMessage` (the thing that may leave the process)

Package `crypto`. Library stand-in for proto `PartialDecrypt` **without** pulling protobuf into crypto.

```go
// PeelMessage is a published partial decrypt. It must not contain d or a card rank.
type PeelMessage struct {
    HandNum    int64
    PlayerID   string
    CardIndex  int
    Ciphertext *big.Int
    Result     *big.Int
    Proof      *ZKProof
}

func PeelMessageFromPD(handNum int64, pd *PartialDecryption) (*PeelMessage, error)
```

**Rules**

- Deep-copy `Ciphertext`, `Result`, and the four `ZKProof` limbs (`A,B,S,H`).
- No plaintext card field. No `d`. If you want a test-only decoded value, keep it on the session (`decoded map[int]*big.Int`), never on this struct.
- Errors if `pd` / ciphertext / result / proof is nil.
- Do **not** add `HandNum` to `PartialDecryption` (that type is used by the oracle, fault, and codec).

### 4. `DealSession`

```go
type Street int

const (
    StreetNone Street = iota
    StreetFlop
    StreetTurn
    StreetRiver
)

type DealSession struct {
    // unexported fields; sketch for implementers:
    mu          sync.Mutex
    kr          *Keyring
    handNum     int64
    sessionID   []byte
    p           *big.Int
    dealerIdx   int
    cards       []*big.Int          // encrypted deck copy (52 production, N in tests)
    // current job
    jobKind     jobKind             // idle / hole / public
    cardIndex   int
    recipient   string              // hole only; "" for public
    peelers     []string            // PeelOrder for this job
    nextPeel    int                 // next peeler index into peelers
    current     *big.Int            // remaining ciphertext for this card
    pending     map[int]*PeelMessage
    applied     map[int]*PeelMessage // this card only
    // outputs
    localHoles  [2]*big.Int         // set when we are the recipient; nil otherwise
    revealed    map[string][2]*big.Int
    community   []*big.Int          // public cards in flop/turn/river order as they finish
    holesDone   bool
}

func NewDealSession(kr *Keyring, deck *EncryptedDeck, handNum int64, dealerIdx int) (*DealSession, error)

// BeginHoles starts the hole-card sequence (left of dealer, two rounds).
// If we are the first peeler of the first card, return our PeelMessage.
func (s *DealSession) BeginHoles() (*PeelMessage, error)

// BeginStreet starts public peels for flop (3), turn (1), or river (1).
// Holes must be done. Streets must be called in order. Burns are skipped.
func (s *DealSession) BeginStreet(street Street) (*PeelMessage, error)

// BeginReveal starts a public peel of playerID's two hole-card indexes (round 0 then 1).
// Holes must be done. Streets are NOT required (fold-to-showdown is Phase 4/5).
func (s *DealSession) BeginReveal(playerID string) (*PeelMessage, error)

func (s *DealSession) HandlePeel(msg *PeelMessage) (*PeelMessage, error)

func (s *DealSession) HolesDone() bool
func (s *DealSession) StreetDone() bool           // current street's cards all public
func (s *DealSession) RevealDone() bool           // current BeginReveal's two cards public

func (s *DealSession) LocalHoles() ([2]game.Card, error)          // only after we decoded both; FieldToCard
func (s *DealSession) CommunityCards() ([]game.Card, error)       // accumulated public cards that FieldToCard
func (s *DealSession) RevealedHoles(playerID string) ([2]game.Card, error)
```

**Constructor invariants (reject with error)**

1. `kr != nil`, `kr.Len() >= 2`, `deck != nil`, `len(deck.Cards) == 52` (production).
2. `len(deck.SessionID) > 0`; store a copy.
3. `handNum >= 1`.
4. `dealerIdx` in `[0, kr.Len())`.
5. `deck.P` equals `kr.Modulus()` (`Cmp == 0`); do not “fix it up.”
6. Deep-copy every card into `s.cards`. The original deck must remain available for showdown peels (always peel from `s.cards[index]`, never from a leftover hole chain).

**Do not** take `[]*SRAKey`. Always `kr.LocalKey()` vs expected `PlayerID`.

**Test-only smaller decks** (same package):

```go
func newDealSessionN(kr *Keyring, cards []*big.Int, sessionID []byte, handNum int64, dealerIdx int) (*DealSession, error)
```

`P = 23` cannot hold 52 distinct `CardToField` values. FSM tests stay fast with tiny field elements (`2,3,4,6,…` in `1 ≤ m ≤ P-2`). `LocalHoles()` / `CommunityCards()` / `RevealedHoles()` will fail `FieldToCard` on those values — that is fine. Tiny tests assert via an unexported helper:

```go
func (s *DealSession) testDecoded(cardIndex int) *big.Int // nil if this replica has not decoded that index
```

Set `testDecoded` for (a) hole indexes we received as recipient after `FinishHole`’s decrypt, **before** requiring a card encoding, and (b) public indexes after the last peel, storing the fully peeled `*big.Int`. Production `LocalHoles` still requires `FieldToCard != -1`.

Production `NewDealSession` must **not** expose `NumCards`.

### 5. Job sequencer

One **job** = one `cardIndex` + a peel order.

| Job | `recipient` | `peelers` | After last peel |
|---|---|---|---|
| hole card | that seat’s player ID | `PeelOrder(seats, recipient)` | recipient: decrypt locally, store; others: store **nothing** |
| public card | `""` | full `SeatOrder()` | `FinishPublic` / store decoded value on **every** replica |

`BeginHoles` installs hole jobs in engine order: for `round := 0..1`, for `i := 0..n-1`, recipient = `SeatOrder()[(dealerIdx+1+i)%n]`, index = `HoleCardIndex(...)`. Auto-advance to the next hole card when a job finishes. `HolesDone` when both rounds are done.

`BeginStreet(StreetFlop)` installs three public jobs (`FlopIndexes`). Turn/river install one. Auto-advance within the street. Do not peel burn indexes.

`BeginReveal(id)` installs two public jobs for `HoleCardIndex(..., id, 0)` and `..., 1`. Auto-advance. After both, `RevealedHoles(id)` works on every replica (including the owner — they still **peel** so others can finish the chain).

**`HandlePeel` rules** (hold `s.mu` for the whole call)

Incoming validation errors (prefix `DealSession.HandlePeel:`):

- nil message / empty `PlayerID` / nil ciphertext, result, or proof
- `HandNum != s.handNum`
- no job in progress (`Begin*` not called or previous sequence fully idle)
- `CardIndex !=` current job index — **error**, do not buffer across cards (Phase 5 starts the next street only after the previous sequence is done; keep this phase small)
- `PlayerID` not in current `peelers`

Then, `seat` = index of `PlayerID` in `peelers` (not `Keyring.SeatIndex` — hole jobs skip the recipient):

| Incoming | Action |
|---|---|
| `seat < nextPeel` and payload equals `applied[seat]` | **ignore** — `(nil, nil)` (self-echo / retry) |
| `seat < nextPeel` and payload **differs** | error (equivocation) |
| `seat == nextPeel` but `PlayerID != peelers[nextPeel]` | error |
| `seat == nextPeel` and `PlayerID == kr.LocalID()` | error (`local peel must be produced locally`) |
| `seat == nextPeel` | `VerifyAndApply(s.current, pd, p, sessionID)`; on success adopt result, `nextPeel++`, store applied copy, **drain** pending |
| `seat > nextPeel` | buffer **one** message per peeler index. Equal duplicate ignore; different → error. Do not verify yet |

After a successful apply (including drain): if the job still has peelers and `peelers[nextPeel] == kr.LocalID()`, `executeLocalLocked` and return that message. If the job is complete (`nextPeel == len(peelers)`), `finishJobLocked` then auto-start the next job in the sequence (next hole / next flop card / second reveal card). If that next job’s first peeler is us, return **that** message (same as shuffle draining into our turn).

**`executeLocalLocked`**

1. `pd, err := Peel(kr.LocalKey(), s.current, s.cardIndex, kr.LocalID(), s.sessionID)`
2. `PeelMessageFromPD`; `VerifyAndApply` on ourselves; `nextPeel++`; drain; maybe finish/auto-advance.
3. Return the message for the caller to send. Never return the recipient’s `FinishHole` as a message.

**`finishJobLocked`**

- Hole + we are recipient: `plain, err := kr.LocalKey().Decrypt(s.current)` (same as `FinishHole` but keep the `*big.Int` for tiny tests). Store in `localHoles[round]` and `decoded[cardIndex]`. Then, if `FieldToCard` works, that is what `LocalHoles()` returns.
- Hole + we are not recipient: do **not** decrypt. `decoded[cardIndex]` stays nil.
- Public: store `s.current` as decoded on every replica. Append to `community` if this was a street card; fill `revealed[playerID][round]` if this was a reveal card.

**`BeginHoles` / `BeginStreet` / `BeginReveal`**

1. Error if a job sequence is already in progress and not finished.
2. `BeginStreet` / `BeginReveal` require `HolesDone()`.
3. `BeginStreet` requires flop before turn before river (once flop started, turn cannot precede it). Calling flop twice → error.
4. Install first job: `current = copy(s.cards[cardIndex])`, `nextPeel = 0`, clear pending/applied for this card.
5. If `peelers[0] == kr.LocalID()`, `executeLocalLocked`. Else `(nil, nil)`.

**Concurrency.** Fake-net = one goroutine per replica. Mutex on the session. Keyring stays immutable.

**Self-echo.** Fake bus must not deliver to `from` (mirrors `node.dispatch`). If an echo arrives, matching duplicate → ignore.

### 6. Small `zkp.go` guards (in scope)

| Change | Why |
|---|---|
| `ProveDecryption`: if `key == nil \|\| !key.IsPrivate()` → error | `Peel` must never prove with `Public(...)`. Today `Exp(g, key.D, P)` panics on nil `D`. |
| `PartialDecryption.Verify`: nil `pd` → error | Session / `VerifyAndApply` should not panic. |

Do **not** change `computeChallenge` or the two `Exp` checks. Do **not** bind `proof.H` to `e`.

Empty `fmt.Errorf("")` leftovers in `VerifyDecryption` stay.

### 7. Fake bus (tests only — not a production type)

In `deal_session_test.go`:

```go
type fakePeelBus struct {
    chans map[string]chan *PeelMessage // buffer cap >= 32
}

func (b *fakePeelBus) Broadcast(from string, msg *PeelMessage)
```

- Do **not** deliver to `from`.
- Deep-copy the message per recipient (`*big.Int` and proof limbs).
- Broadcast **all** peels to all other replicas, including hole peels. Cryptographic privacy does not depend on unicast. Phase 5 may prefer `SendDirectPartialDecrypt` to the recipient + remaining peelers; this phase does not.
- Each replica goroutine: `Begin*` → maybe broadcast; loop `HandlePeel` until the sequence is done or context timeout.
- `context.WithTimeout`: 30s tiny, 3 minutes for 2048-bit 52-card. A hung sequencer must fail the test, not hang `go test`.

Do **not** put `fakePeelBus` in `deal_session.go`.

Reuse `nPlayerKeyrings` from `shuffle_session_test.go` (same package) or a thin wrapper in the new test file. Do not duplicate key generation logic if a shared test helper is already there — calling `nPlayerKeyrings` is fine.

---

## Tests

Tiny-deck + `smallPrime` unless a row says `SharedPrime()`.

Helper: N full keys; each replica `NewKeyring(localID, localFull, publicEBytes, order)` — **only** `E.Bytes()` in the map.

Tiny encrypted deck: `newShuffleSessionN` / `RunFullShuffle` with `NumCards = len(tiny)` **or** encrypt the tiny values with every key in the test harness (`EncryptAll` in seat order, no permutation) when a test only cares about peels. Prefer a shuffled tiny deck for session tests so indexes are not identity. Agreement is replica-vs-replica.

A 3-player hole deal needs 6 cards; flop+turn+river add 5 cards + 3 burns → **14** slots. Tiny community tests should use at least 14 field elements in `1 ≤ m ≤ 21` (e.g. `2..15`). Tiny hole-only tests can use 6–8 cards.

Do **not** call `NewEncryptedDeck` for tiny decks (it demands 52).

### A. Primitives — `deal_session_test.go` or `crypto_test.go`

| Test | Assert |
|---|---|
| `TestPeel_RejectsPublicOnlyKey` | `Peel(kr.Public(local), …)` errors; `ProveDecryption` on a public-only key errors (no panic) |
| `TestPeel_ThenVerifyAndApply` | 2 keys on `smallPrime`; `c = Encrypt_B(Encrypt_A(m))`; Alice peels; `VerifyAndApply` returns Bob-layer ciphertext; Bob `FinishHole` recovers `m` (raw int, not a card) |
| `TestVerifyAndApply_CiphertextMismatch` | `pd.Ciphertext` ≠ `current` → error |
| `TestVerifyAndApply_TamperedResult` | `SubstitutePartialDecryption` → `VerifyAndApply` error (wraps existing ZK fail) |
| `TestFinishPublic_NotACard` | `FinishPublic(big.NewInt(1), SharedPrime())` errors (`FieldToCard == -1`) |
| `TestHoleCardIndex_MatchesDealHoleCards` | For `n=2,3,4` and each `dealerIdx`, `HoleCardIndex` equals the `deckPos` `DealHoleCards` would use. Can assert against a tiny simulation of the nested loops; do not require a 52-card shuffle. |
| `TestCommunityIndexes_MatchDealCommunityCards` | `FlopIndexes` / `TurnIndex` / `RiverIndex` match `DealCommunityCards(CommunityStartPos(n), []int{3,1,1})` positions (burns skipped). |

Existing `TestDeal_HoleCards_AllValid`, `TestDeal_CommunityCards`, `TestDeal_MaliciousDecryption_Detected`, `TestDeal_NoCardDuplicate_FullHand` must still pass (oracle).

### B. Session FSM (tiny deck, 2–3 players, **single goroutine**, inject messages by hand)

Build one encrypted tiny deck in the harness; give each replica `newDealSessionN` + its Keyring.

| Test | Assert |
|---|---|
| `TestDealSession_BeginHoles_FirstPeelerProduces` | Dealer 0, 2 players: first hole recipient is seat 1; first peeler is seat 0. Seat 0 `BeginHoles` returns a message (`CardIndex==0`); seat 1 returns nil. |
| `TestDealSession_TwoPlayers_HolesSequential` | Drive both hole cards by hand. Both `HolesDone()`. Seat 0 `testDecoded` non-nil only on seat 0’s two indexes; seat 1 only on seat 1’s. The six-or-four decoded values across replicas are unique. |
| `TestDealSession_WrongPeelerRejected` | Feed Carol’s peel while Alice is expected → error; `nextPeel` unchanged. |
| `TestDealSession_WrongHandRejected` | `HandNum` 2 into a hand-1 session → error. |
| `TestDealSession_WrongCardIndexRejected` | Message for index 3 while job is index 0 → error (no cross-card buffer). |
| `TestDealSession_TamperedZKRejected` | `SubstitutePartialDecryption` (or flip `Result` without a new proof) → `HandlePeel` error; job not advanced. |
| `TestDealSession_DuplicateIgnored` | Same valid peel twice → second `(nil, nil)`. |
| `TestDealSession_ConflictingDuplicateRejected` | Same peeler already applied, different `Result` → error. |
| `TestDealSession_BuffersOutOfOrderPeelers` | 3 players, public (or hole with 2 peelers): replica receives peeler 1 **before** peeler 0. First call buffers. Second (peeler 0) applies, drains, maybe produces. Final decoded value matches a sequentially driven replica. |
| `TestDealSession_UnknownPlayerRejected` | `PlayerID: "mallory"` → error. |
| `TestDealSession_RecipientDoesNotPublishLastDecrypt` | After a hole job, the recipient replica has `testDecoded` set and **no** `PeelMessage` was produced with that replica as `PlayerID` for the last layer. Count of messages for that card is `n-1`. |

### C. Fake-net (goroutines + channels)

| Test | Assert |
|---|---|
| `TestDealSession_FakeNet_3Players_Holes` | Tiny deck, `smallPrime`, 3 replicas, dealer 0. All `HolesDone()`. Each replica’s `testDecoded` is non-nil for **exactly two** indexes (its holes) and nil for the other four. Union of decoded values has 6 unique field elements. |
| `TestDealSession_FakeNet_3Players_Community` | After holes, `BeginStreet(Flop)` then turn then river (or flop only if the tiny deck is 14+). All three replicas’ `testDecoded` for flop indexes are **equal and non-nil**. Non-recipients still have nil on opponent hole indexes. |
| `TestDealSession_FakeNet_2Players_Production` | **`SharedPrime()` + 52-card `RunFullShuffle` (or ShuffleSession) + `NewDealSession`.** 2 replicas. `HolesDone()`; `LocalHoles()` succeeds; `FieldToCard` valid; the two replicas’ local holes are **different** pairs; opponent `RevealedHoles` errors until reveal. Then `BeginStreet(Flop)`: both get the same 3 `game.Card`s. Then `BeginReveal` of the opponent: both replicas’ `RevealedHoles` match the opponent’s `LocalHoles()`. This is the slow test. |

Do **not** require the distributed hole cards to equal a `DealProtocol.DealHoleCards` run on a second shuffle — permutations are random. For production tests that **share** one shuffled `EncryptedDeck` across replicas, an oracle `DealProtocol` **with all keys in the test harness** may be used as a control: each replica’s `LocalHoles()` must equal the oracle pair for that seat. That is a valid assertion because the deck is the same object.

### D. Privacy + showdown eval

| Test | Assert |
|---|---|
| `TestDealSession_HolePrivacy_PublicCannotFinish` | Production 2- or 3-player result. For a non-recipient replica: `LocalKey().Decrypt(CardAt(opponentIndex))` still has `FieldToCard == -1` (two layers remain on the **original** ciphertext). `Public(opponent).Decrypt` errors. `LocalHoles()` does not return the opponent’s cards. |
| `TestDealSession_Showdown_EvaluateBest7Agrees` | Production 2-player: holes + flop + turn + river + `BeginReveal` both seats (or reveal the opponent; local already known). Each replica builds `[7]game.Card` from that seat’s holes + five community cards; `EvaluateBest7` equal across replicas. Repeat for the other seat. Winner identity agrees. |

Do **not** run `FieldToCard` privacy assertions on `smallPrime`.

### E. Regression

```
go test ./internal/crypto -count=1
go test ./...
```

Must still pass: `TestDeal_HoleCards_AllValid`, `TestDeal_CommunityCards`, `TestDeal_MaliciousDecryption_Detected`, `TestDeal_NoCardDuplicate_FullHand`, `TestCryptoGame_FullProtocol`, all Phase 1 keyring tests, all Phase 2 shuffle-session tests.

No new proto. No live 2-player crypto demo.

---

## Implementation order (do this, then stop)

1. `zkp.go` guards (`ProveDecryption` private, `PartialDecryption.Verify` nil-safe) + a tiny unit test.
2. `Peel` / `VerifyAndApply` / `FinishHole` / `FinishPublic` + primitive tests (A).
3. Index helpers + `PeelOrder`; `TestHoleCardIndex_MatchesDealHoleCards`.
4. Refactor `DealProtocol` to call the primitives. Re-run existing deal tests.
5. `PeelMessage` + `DealSession` production constructor + `BeginHoles` / `HandlePeel` sequencer. Drive with single-goroutine tests (B).
6. `BeginStreet` / `BeginReveal` + tiny community / reveal tests.
7. Fake-net 3-player tiny holes + community (C).
8. Production 52-card 2-player fake-net + privacy + `EvaluateBest7` (C+D).
9. `go test ./...`
10. Stop. Do not open `main.go`. Do not change `StartHandCrypto` / `dealFlop` (Phase 4).

If a step’s tests fail, fix that step. Do not start Phase 4 files.

---

## Error style

New code: `fmt.Errorf` / `errors.New` with a function prefix (`Peel: ...`, `VerifyAndApply: ...`, `NewDealSession: ...`, `DealSession.HandlePeel: ...`). Match Phase 1–2 / `GenerateSRAKey`. Do not drive-by rewrite empty `fmt.Errorf("")` leftovers in `RevealToPlayer` except where the refactor already replaces that line.

---

## Explicit non-goals (push back if asked mid-phase)

- Wiring `OnPartialDecrypt` / streams in `runP2PMode` — Phase 5.
- `StartHandCrypto` accepting empty opponent holes; `ApplyStreet` / `ApplyHoleReveal` — Phase 4.
- Buffering peels for a **future card index** (wrong-card is an error).
- Unicast-only hole peels on the fake bus (broadcast is fine; Phase 5 may still prefer direct).
- Requiring Shamir / timeout abort.
- Binding ZK `H` to public `e`.
- Putting ranks/suits on `PeelMessage` “for debug.”
- libp2p in this phase. If a test starts a `Node`, it is out of scope.
- Replacing `DealProtocol` inside `CryptoGame` with `DealSession`.
- Changing `EncryptedDeck` to allow non-52 production decks.

---

## How Phase 4 / 5 will consume this (do not implement)

Phase 4 needs **cards as inputs** to the machine. This phase only **produces** those cards:

```go
holes, err := session.LocalHoles()          // fill gs.Players[local].HoleCards only
flop, err := session.CommunityCards()       // after BeginStreet(StreetFlop) completes
opp, err := session.RevealedHoles(pid)      // after BeginReveal(pid)
```

Phase 5:

```go
ed, err := shuffleSess.EncryptedDeck()
deal, err := pokercrypto.NewDealSession(kr, ed, int64(handNum), dealerIdx)

node.OnPartialDecrypt = func(pb *network.PartialDecrypt) {
    pd := network.PartialDecryptFromWire(pb)
    msg := &pokercrypto.PeelMessage{HandNum: pb.HandNum, /* fields from pd */}
    out, err := deal.HandlePeel(msg)
    if out != nil { sendPeel(out) } // direct to recipient for holes; gossip for streets
}

out, err := deal.BeginHoles()
// ... wait HolesDone → StartHandCrypto with local holes only
out, err = deal.BeginStreet(pokercrypto.StreetFlop)
// ... ApplyStreet (Phase 4 API)
out, err = deal.BeginReveal(opponentID)
// ... ApplyHoleReveal (Phase 4 API)
```

If Phase 3 accidentally published the recipient’s last decrypt, Phase 5 will leak hole cards on gossip. If Phase 3 applies peels in arrival order instead of peel-turn index, replicas will disagree on community cards and the machine will desync.

---

## Review checklist (before calling Phase 3 done)

- [ ] `Peel` refuses a public-only key; `ProveDecryption` does not panic on nil `d`
- [ ] `VerifyAndApply` rejects ciphertext mismatch and `SubstitutePartialDecryption`
- [ ] `PeelMessage` has no rank/suit / no `d`
- [ ] Hole job publishes exactly N−1 peels; recipient `FinishHole` is local
- [ ] Non-recipient `testDecoded` / `LocalHoles` does not contain opponent cards
- [ ] Community: all replicas same public cards; burns never peeled
- [ ] Showdown public peel fills opponent holes; `EvaluateBest7` agrees
- [ ] Wrong peeler / hand / card index / bad ZK rejected; turn index unchanged
- [ ] Out-of-order peeler for the **current** card is buffered, then applied
- [ ] Matching duplicate ignored; conflicting duplicate rejected
- [ ] Fake-net 3-player tiny holes + community
- [ ] Production 52-card 2-player fake-net
- [ ] `DealHoleCards` / `DealCommunityCards` / `TestCryptoGame_FullProtocol` still pass
- [ ] `go test ./...` green
- [ ] `cmd/poker/main.go` diff is empty
- [ ] `internal/game` diff is empty
- [ ] `internal/network` diff is empty
- [ ] No new proto fields

---

## Time

Half a day of AI implementation including tests, if this spec is followed without wiring `main.go` or `machine.go`. The 2048-bit 52-card fake-net (shuffle + peels) is the slow one; tiny-deck tests should be milliseconds.

If the work grows into `StartHandCrypto`, `ApplyStreet`, or libp2p, stop and split — do not pull Phase 4 into this conversation.
