# Phase 5 — Wire into `runP2PMode` (implementation spec)

Parent: [`CRYPTO_DEAL_PLAN.md`](../CRYPTO_DEAL_PLAN.md). Consumes Phase 1 `Keyring`, Phase 2 `ShuffleSession`, Phase 3 `DealSession`, Phase 4 `StartHandCrypto` / `ApplyStreet` / `ApplyHoleReveal`. This is issues 1–3 as the examiner sees them: **the live `poker host` / `poker join` path actually runs SRA shuffle + peels**, and `--no-crypto` becomes the old shared-seed debug path.

**This is the product phase.** After this lands and tests pass, the one-sentence claim in [`ISSUES_AND_RECOMMENDATIONS.md`](../ISSUES_AND_RECOMMENDATIONS.md) is true.

**Do not implement later work from this doc** (Shamir, chain, DHT). After this lands, stop.

---

## Why this phase exists

Phases 1–4 built libraries. Live multiplayer still does this after the lobby fills:

```
nonce  = Lobby.SessionNonce()
seed   = XOR(nonce) then LCG mix
machine.StartHand()   // every replica shuffles the same plaintext deck
```

That seed is **public**. Every honest node can reconstruct every hole card. `--no-crypto` only skips generating an SRA key; dealing is shared-seed either way. `OnShuffleStep` / `OnPartialDecrypt` exist on `Node` and are **unset**. `BroadcastShuffleStep` / `BroadcastPartialDecrypt` / `SendDirectPartialDecrypt` have **no caller** in `cmd/poker`.

| Fact | Where |
|---|---|
| Shared-seed `StartHand` after lobby fill | `cmd/poker/main.go` `runP2PMode` / `Init` / `startNextHand` |
| SRA key generated when `!noCrypto`, then ignored | same file |
| `OnShuffleStep` / `OnPartialDecrypt` unset | `runP2PMode` |
| Phase 1 `KeyringFromLobby` unused by the live loop | `internal/network/lobby.go` |
| Phase 2 `ShuffleSession` unused by the live loop | `internal/crypto/shuffle_session.go` |
| Phase 3 `DealSession` unused by the live loop | `internal/crypto/deal_session.go` |
| Phase 4 crypto machine APIs unused by the live loop | `internal/game/machine.go` |
| TUI already hides non-local holes | `internal/tui/player_panel.go` (`IsLocalPlayer \|\| IsWinner`) |
| README still describes crypto as optional / default as plaintext | `README.md` |

Phase 5 is **orchestration**, not new math. The risk is distributed hangs (waiting forever for a shuffle step), not the exponentiation.

Keep `NewCryptoGame` / `HandCoordinator.RunHand` as the **in-process oracle**. Do not route live P2P through them (they generate every `(e, d)` on one machine).

---

## Goal / done when

1. Default `poker host` / `poker join` (no `--no-crypto`): Keyring from lobby `e`s → shuffle over real `OnShuffleStep` / `BroadcastShuffleStep` → hole peels → **local holes only** → `StartHandCrypto`.
2. During play: when the machine `NeedsStreet()`, community peels then `ApplyStreet` with the **new** cards only. All-in runout peels remaining streets without waiting for more betting.
3. At showdown: public-peel remaining seats in **canonical seat order** (not “whoever is missing on this replica”), `ApplyHoleReveal`, existing settle / next hand.
4. Next hand: **new** shuffle (do not reuse the encrypted deck). Mix `handNum` into the crypto session id.
5. `--no-crypto`: today’s shared-seed `StartHand`, labeled as debug. Mixed tables (one peer crypto, one `--no-crypto`) **error**, do not silently fall back.
6. `OnShuffleStep` and `OnPartialDecrypt` are assigned **before** `node.Start()`.
7. README + CLI help: crypto dealing is default; `--no-crypto` = all cards visible, sync testing only. One honest status line at process start.
8. `go test ./...` green. New fake-bus integration test (no libp2p, no two binaries) plays shuffle → holes → one betting round → flop peel → fold or showdown.
9. Manual 2-player LAN demo: opponent holes hidden during the hand; community appears together; wire log shows big-integer ciphertexts, not ranks.

You should be able to explain: how `main.go` sequences lobby → shuffle → deal → machine → streets → showdown; where `--no-crypto` branches; what a hang looks like (waiting forever for a shuffle step); why reveal order cannot be `MissingRevealIDs()`.

---

## Protocol reminder (do not re-derive)

```
lobby full
  → KeyringFromLobby (local d + everyone else's e)
  → ShuffleSession: N sequential encrypt-then-permute steps, sequenced by seat index
  → EncryptedDeck (52 ciphertexts, identical on every replica)
  → DealSession.BeginHoles: N−1 peels per hole card; recipient FinishHole locally
  → fill gs.Players[local].HoleCards only; StartHandCrypto
  → betting (existing ApplyAction + actionSequencer)
  → NeedsStreet → BeginStreet → ApplyStreet (flop 3 / turn 1 / river 1)
  → fold-to-winner: settle, no reveals
  → showdown: BeginReveal each remaining seat in seat order → ApplyHoleReveal
  → next hand: new ShuffleSession + new DealSession
```

Shuffle steps and peels are already sequenced **inside** the sessions (Phase 2/3). GossipSub unordered delivery is their problem, not a second sequencer in `main.go`. The action sequencer stays for `PlayerAction` only.

`SHUFFLE_COMMIT` stays unused. No new proto fields.

2048-bit modexp × 52 cards × 2 players is **seconds**. Fine. Do not optimize SRA. Print a status line so the demo does not look hung.

---

## Scope

### Touch

| File | Change |
|---|---|
| `internal/network/codec.go` | wire adapters: proto `ShuffleStep` ↔ `ShuffleMessage`; proto `PartialDecrypt` ↔ `PeelMessage` |
| `internal/network/node.go` | `BroadcastShuffleMessage` (or adapt `BroadcastShuffleStep` to take `ShuffleMessage` so a permutation can never be passed in); peel send helper |
| `internal/network/crypto_hand.go` | **new** — `CryptoHand` driver (shuffle + peels + card outputs). **No** TUI, **no** libp2p types in the FSM |
| `internal/network/crypto_hand_test.go` | **new** — fake-bus 2-player full loop + adapter unit tests |
| `cmd/poker/main.go` | `runP2PMode` branch; callbacks; `--no-crypto` flag parse; status line; next-hand reshuffle |
| `cmd/poker/main.go` `printHelp` | crypto is default; `--no-crypto` is debug |
| `README.md` | honest status line + `--no-crypto` wording (issue 4) |

Tiny helpers in `codec.go` / `node.go` are in scope because Phase 5 is the first caller of those senders.

### Do not touch

- `internal/crypto/shuffle_session.go`, `deal_session.go`, `keyring.go`, `sra.go`, `zkp.go`, `commit.go`, `params.go` — call them; do not “improve” them
- `internal/crypto/crypto_game.go`, `internal/network/coordinator.go` — oracle stays
- `internal/game/machine.go` pot / eval / plaintext `StartHand` — Phase 4 is done. Driver-side helpers live in `network`, not a new machine phase
- proto / `messages.proto` / `messages.pb.go`
- `internal/tui/*` — hiding opponent holes already works on empty `Card{}`
- Ethereum, Shamir, `FaultManager.Run`, DHT, reconnect
- Replacing SRA; 9-player WAN; deleting the local simulation

`crypto` must **not** import `network`. `cmd/poker` may import both. `CryptoHand` lives in `network` so `cmd/poker` stays a thin wire-up and tests do not import `package main`.

---

## Current code to reuse (do not rewrite)

```go
func KeyringFromLobby(localID string, local *SRAKey, lobby *Lobby) (*Keyring, error)
func (l *Lobby) AllSeatsHavePublicE() bool
func (l *Lobby) SessionNonce() []byte
func (l *Lobby) Seats() []SeatInfo          // canonical order = PlayerIDs()

func SessionID(playerIDs []string, nonce []byte) []byte
func NewShuffleSession(kr *Keyring, sessionID []byte, handNum int64) (*ShuffleSession, error)
func (s *ShuffleSession) Start() (*ShuffleMessage, error)
func (s *ShuffleSession) HandleMessage(msg *ShuffleMessage) (*ShuffleMessage, error)
func (s *ShuffleSession) Done() bool
func (s *ShuffleSession) EncryptedDeck() (*EncryptedDeck, error)

func NewDealSession(kr *Keyring, deck *EncryptedDeck, handNum int64, dealerIdx int) (*DealSession, error)
func (s *DealSession) BeginHoles() (*PeelMessage, error)
func (s *DealSession) BeginStreet(street Street) (*PeelMessage, error)
func (s *DealSession) BeginReveal(playerID string) (*PeelMessage, error)
func (s *DealSession) HandlePeel(msg *PeelMessage) (*PeelMessage, error)
func (s *DealSession) Outbound() []*PeelMessage   // MUST drain after every Begin*/HandlePeel
func (s *DealSession) HolesDone() bool
func (s *DealSession) StreetDone() bool
func (s *DealSession) RevealDone() bool
func (s *DealSession) LocalHoles() ([2]game.Card, error)
func (s *DealSession) CommunityCards() ([]game.Card, error) // accumulated flop+turn+river
func (s *DealSession) RevealedHoles(playerID string) ([2]game.Card, error)

func (m *Machine) StartHandCrypto() error
func (m *Machine) NeedsStreet() bool
func (m *Machine) PendingStreetCount() int // 3, 1, or 0
func (m *Machine) NeedsReveal() bool
func (m *Machine) ApplyStreet(cards []Card) error
func (m *Machine) ApplyHoleReveal(playerID string, cards [2]Card) error
func (m *Machine) ApplyAction(a Action) error

func DeckToWire / DeckFromWire
func PartialDecryptToWire / PartialDecryptFromWire
func (n *Node) BroadcastShuffleStep(ctx, handNum, step *ShuffleStep) error  // uses OutputDeck+Commitment only
func (n *Node) BroadcastPartialDecrypt(ctx, handNum, pd *PartialDecryption) error
func (n *Node) SendDirectPartialDecrypt(ctx, to peer.ID, handNum, pd) error
```

`node.dispatch` already ignores self-echo. Fake bus and live gossip must keep that: the producer applies locally; it does not need the echo. Matching duplicates are ignored by the sessions.

`DealSession.Outbound()` is a real footgun. `executeLocalLocked` can produce a **follow-up** peel (next hole card / next flop card) in addition to the returned `*PeelMessage`. Phase 3 tests use `collectPeels`. Phase 5 must do the same or the table hangs on the unsent extra peel.

---

## Design

### 1. Wire adapters (`codec.go`)

Keep protobuf out of `crypto`. All mapping in `network`.

```go
func ShuffleMessageFromWire(pb *ShuffleStep) *pokercrypto.ShuffleMessage
func ShuffleMessageToWire(tableID string, msg *pokercrypto.ShuffleMessage) *ShuffleStep

func PeelMessageFromWire(pb *PartialDecrypt) *pokercrypto.PeelMessage
func PeelMessageToPD(msg *pokercrypto.PeelMessage) *pokercrypto.PartialDecryption
```

**Rules**

- `ShuffleMessageFromWire` copies deck limbs and commitment slices. `PlayerID` comes from `pb.PlayerId` (the shuffling seat), not from the envelope sender — they should match; the session still checks seat order.
- There is **no** permutation field to copy. Do not reconstruct a `crypto.ShuffleStep` just to call the old helper if that struct could carry a perm. Prefer `BroadcastShuffleMessage(ctx, msg *ShuffleMessage)` that marshals `ShuffleMessageToWire` only.
- `PeelMessageFromWire` sets `HandNum` from the proto (`PartialDecryptFromWire` drops it because `PartialDecryption` has no hand field).
- Nil proto → nil / error, no panic.

Keep `BroadcastShuffleStep(*ShuffleStep)` working for existing tests; live path must not pass a step that still has `Permutation` populated. New helper is the live caller.

### 2. Peel send policy (v1)

Cryptographic privacy does **not** depend on unicast. A hang **does** depend on a flaky direct stream.

```go
func (n *Node) SendPeel(ctx context.Context, msg *pokercrypto.PeelMessage) error
```

1. Always `BroadcastPartialDecrypt` (gossip). This is enough for the 2-player demo.
2. Optionally also `SendDirectPartialDecrypt` to the hole-card **recipient** when `msg` is a hole peel and `PeerIDFromString` succeeds. Direct failure is **non-fatal** (log once). Session treats a matching duplicate as ignore.

Do **not** make correctness depend on streams. If direct is flaky on a cold mesh, gossip still completes the job.

Log (debug, not ranks):

```
[crypto] peel hand=%d player=%s card=%d result_bytes=%d
[crypto] shuffle hand=%d player=%s deck_cards=%d limb_bytes=%d
```

`limb_bytes` = `len(OutputDeck[0].Bytes())` (or first peel result). Never `FieldToCard` on the wire path.

### 3. `CryptoHand` (`internal/network/crypto_hand.go`)

One object per replica per hand. Wraps one `ShuffleSession` then one `DealSession`. Mutex for callback vs wait. No `Machine` inside it — it **produces cards**; `main.go` (and the fake-bus test) feeds the machine.

```go
type CryptoHand struct { /* unexported: kr, handNum, dealerIdx, sessionID, shuffle, deal, mu, shuffleCh, peelCh */ }

func NewCryptoHand(kr *pokercrypto.Keyring, lobbyNonce []byte, handNum int64, dealerIdx int) (*CryptoHand, error)

func (h *CryptoHand) HandleShuffle(msg *pokercrypto.ShuffleMessage) (*pokercrypto.ShuffleMessage, error)
func (h *CryptoHand) HandlePeel(msg *pokercrypto.PeelMessage) []*pokercrypto.PeelMessage // returned + Outbound()

func (h *CryptoHand) StartShuffle() (*pokercrypto.ShuffleMessage, error)
func (h *CryptoHand) ShuffleDone() bool
func (h *CryptoHand) WaitShuffle(ctx context.Context) error

func (h *CryptoHand) StartHoles() []*pokercrypto.PeelMessage // BeginHoles + Outbound
func (h *CryptoHand) HolesDone() bool
func (h *CryptoHand) WaitHoles(ctx context.Context) error
func (h *CryptoHand) LocalHoles() ([2]game.Card, error)

func (h *CryptoHand) StartStreet(street pokercrypto.Street) []*pokercrypto.PeelMessage
func (h *CryptoHand) WaitStreet(ctx context.Context) error
func (h *CryptoHand) NewCommunityCards(already int) ([]game.Card, error) // CommunityCards()[already:]

func (h *CryptoHand) StartReveal(playerID string) []*pokercrypto.PeelMessage
func (h *CryptoHand) WaitReveal(ctx context.Context) error
func (h *CryptoHand) RevealedHoles(playerID string) ([2]game.Card, error)
```

**Session id**

```go
nonce := append(append([]byte{}, lobbyNonce...), byte(handNum>>8), byte(handNum))
sid := pokercrypto.SessionID(kr.SeatOrder(), nonce)
```

`handNum` is also on every `ShuffleMessage` / `PeelMessage`. Mixing it into `SessionID` domain-separates ZK / commitments across hands. Do not reuse `sid` from hand 1 on hand 2.

**Constructor**

1. `kr != nil`, `handNum >= 1`, `dealerIdx` in range, `len(lobbyNonce) > 0` (or allow empty only if tests need it — production lobby nonce is concatenation of peer IDs, non-empty).
2. `NewShuffleSession(kr, sid, handNum)`. Do **not** create `DealSession` until shuffle `Done` (needs `EncryptedDeck`).
3. Wait channels / cond: signal on `HandleShuffle` when `Done()`, on `HandlePeel` when `HolesDone` / `StreetDone` / `RevealDone`.

**`HandlePeel` return**

Always `append(nonNil(out), deal.Outbound()...)`. Callers send **every** element. Never drop `Outbound()`.

**`NewCommunityCards(already)`**

`ApplyStreet` wants the **new** street only. After flop, `CommunityCards()` has length 3; `already==0` → 3 cards. After turn, length 4; `already==3` → 1 card. Do not pass the full slice into `ApplyStreet`.

**Timeouts**

`Wait*` uses `ctx`. Production: `context.WithTimeout(parent, 2*time.Minute)` around shuffle, `2*time.Minute` around each hole/street/reveal wait. On timeout: return a prefixed error (`CryptoHand.WaitShuffle: timed out waiting for shuffle`). **Abort the hand** (surface as `tui.ErrorMsg` / process error). Do **not** build Shamir. Do not wait forever — that is the hang the parent plan warned about.

**Do not** put `ApplyStreet` / `StartHandCrypto` inside `CryptoHand`. Tests and `main.go` both need to see the cards before applying them (privacy assertions).

### 4. Callback wiring **before** `node.Start()`

Same rule as `OnPlayerAction`. Sessions do not exist yet (lobby is empty). Callbacks must tolerate nil:

```go
var cryptoMu sync.Mutex
var liveHand *network.CryptoHand
var earlyShuffle []*network.ShuffleStep
var earlyPeels []*network.PartialDecrypt

node.OnShuffleStep = func(pb *network.ShuffleStep) {
    cryptoMu.Lock()
    h := liveHand
    if h == nil {
        earlyShuffle = append(earlyShuffle, pb) // cap e.g. 16
        cryptoMu.Unlock()
        return
    }
    cryptoMu.Unlock()
    out, err := h.HandleShuffle(network.ShuffleMessageFromWire(pb))
    if err != nil { log and return }
    if out != nil { _ = node.BroadcastShuffleMessage(ctx, out) }
}
node.OnPartialDecrypt = func(pb *network.PartialDecrypt) {
    // same pattern: buffer if liveHand == nil, else HandlePeel and SendPeel each
}
```

After creating `liveHand`, **drain** `earlyShuffle` / `earlyPeels` into `Handle*` before `StartShuffle()`. Seat 0 can otherwise broadcast before seat 1 has a session, and GossipSub will drop on a nil callback… here the callback exists but the session does not — buffering is mandatory.

Direct-stream peels (`RegisterProtocolHandler`) already call `OnPartialDecrypt`. Same handler covers gossip + unicast.

### 5. Live sequence in `runP2PMode`

Keep lobby wait + `BroadcastReady` + the existing ~2s mesh pause. Then branch.

**`--no-crypto` (debug)**

Today’s path: shared seed, `StartHand`, TUI `Init` can stay as-is **or** start the hand before TUI (either is fine; do not change betting). Print:

```
DEBUG  ·  --no-crypto  ·  shared-seed plaintext  ·  all cards visible
```

If `noCrypto` but `AllSeatsHavePublicE()` is true, still use plaintext — the flag is the source of truth for **this** process. The dangerous case is the inverse.

**Default (crypto)**

1. If `!node.Lobby.AllSeatsHavePublicE()` → error and exit: `runP2PMode: crypto dealing requires every seat to publish e; a peer joined with --no-crypto`. Do not start a shared-seed hand.
2. `kr, err := network.KeyringFromLobby(localPeerID, sraKey, node.Lobby)`.
3. `dealerIdx := 0` on hand 1 (same as today: first seat in lobby order). `handNum := 1`.
4. `liveHand = NewCryptoHand(kr, lobby.SessionNonce(), 1, dealerIdx)`; drain early buffers.
5. Print `Cryptographic dealing  ·  SRA 2048-bit  ·  opponent holes stay hidden` and `Shuffling…`.
6. `StartShuffle` → broadcast if non-nil → `WaitShuffle(ctx)`.
7. `deal` is created inside the hand on shuffle done; `StartHoles` → send peels → `WaitHoles`.
8. `holes := LocalHoles()`; find local player in `players`; `p.HoleCards = holes`. **Do not** write opponent holes.
9. `gs := NewGameState(...)`; `machine := NewMachine(gs, nil)` (rng unused in crypto mode); `StartHandCrypto()`.
10. Then build TUI. `Init` must **not** call `StartHand` / `StartHandCrypto` again (phase is already preflop). `Init` only pushes `GameStateMsg`.

`--no-crypto` may keep `StartHand` in `Init`. Crypto must not, or the second start errors (`expected PhaseWaiting`).

**Status while shuffling:** this happens **before** alt-screen, so `fmt.Printf` is visible. Next-hand shuffle happens **inside** the TUI — set `ui.LobbyStatus` / send `tui.NetworkMsg` if it exists; otherwise blocking on the Bubble Tea tick is acceptable for a 2-player demo (winner screen already waits 3s). Do not add a new TUI widget.

### 6. Advancing streets and reveals after every action

After **every** successful `ApplyAction` (local TUI, remote `OnPlayerAction`, `forceFold`):

```go
func (m *p2pGameModel) maybeAdvanceCrypto() {
    if m.noCrypto || liveHand == nil { return }
    machine := *m.machinePtr
    for machine.NeedsStreet() {
        street := streetFromPending(machine) // 3 → Flop; 1 + len==3 → Turn; 1 + len==4 → River
        already := len(machine.State.CommunityCards)
        send(liveHand.StartStreet(street))
        if err := liveHand.WaitStreet(m.ctx); err != nil { /* ErrorMsg */ return }
        cards, err := liveHand.NewCommunityCards(already)
        _ = machine.ApplyStreet(cards) // may immediately NeedsStreet again (all-in runout)
    }
    if machine.NeedsReveal() {
        for _, pid := range remainingShowdownIDs(machine.State) {
            send(liveHand.StartReveal(pid))
            if err := liveHand.WaitReveal(m.ctx); err != nil { return }
            pair, err := liveHand.RevealedHoles(pid)
            _ = machine.ApplyHoleReveal(pid, pair) // idempotent if this replica already has them
        }
    }
}
```

**`remainingShowdownIDs`** (put on `CryptoHand` or a tiny helper in `crypto_hand.go`, **not** `MissingRevealIDs()`):

Players with `StatusActive` or `StatusAllIn`, in `gs.Players` order (already canonical seat order). Include seats whose holes this replica **already** knows.

Why: after hole deal, Alice’s machine has Alice’s holes filled and Bob’s empty; Bob’s machine is the opposite. `MissingRevealIDs()` is therefore **Alice=[Bob], Bob=[Alice]**. If each replica `BeginReveal`s only its missing seat, they start different jobs (different card indexes) and `HandlePeel` fails / hangs.

Fold-to-winner: `NeedsReveal()==false`, loop skipped. Unchanged pot path.

`BeginStreet` / `BeginReveal` twice → session error. Guard: only call `StartStreet` when `NeedsStreet()` and the deal session is idle for that sequence. The `for NeedsStreet` loop plus `WaitStreet` completing before the next `StartStreet` is the guard. Do not fire `StartStreet` from both “I just applied” and a background poll without that wait.

### 7. `machineMu` covers ApplyAction (issue 7, now required)

Today the mutex only protects pointer swaps; `ApplyAction` runs unlocked on the TUI goroutine **and** the gossip goroutine. Street peels plus actions will race.

Hold `machineMu` for: `ApplyAction`, `maybeAdvanceCrypto`, `ApplyStreet`, `ApplyHoleReveal`, `StartHandCrypto`, pointer swap in `startNextHand`. Callbacks that only **read** `liveHand` use `cryptoMu` (sessions already have their own mutex).

Do not add a mutex inside `game.Machine`.

### 8. Next hand

`startNextHand` already increments `handNum` and rotates `dealerIdx`. Crypto branch:

1. Do **not** reuse `liveHand` / the encrypted deck.
2. `liveHand = NewCryptoHand(kr, nonce, int64(handNum), dealerIdx)` (same `kr` — keys do not rotate).
3. Drain any stale buffers for the **old** hand (`HandNum` mismatch → session already errors; still drop old-hand early buffers).
4. Shuffle + holes + fill **local** holes only + `StartHandCrypto` on the new machine.
5. `seq.reset()` as today.

`Lobby.Reset()` only resets ready-state, **not** seats / `e`s. Keep it. Do not require a second join.

Plaintext `--no-crypto` next-hand path unchanged (mix `handNum` into shared seed).

### 9. `--no-crypto` flag parse

Current host/join loops are `for i := 0; i < len(args)-1; i++`, so `poker host --seats 2 --no-crypto` **never sees the flag** (it is the last token). Fix: iterate all args; flags that take a value still look at `i+1`. Same for join.

`--no-crypto` on only one peer is the mixed-table error in §5.

### 10. README + help

Top of README (replace the current status line):

```
**Status**: LAN mental-poker Hold'em is the default (`poker host` / `poker join`). `--no-crypto` is debug: shared-seed plaintext, all cards visible.
```

Features bullet: cryptographic dealing is default SRA shuffle + partial decrypt, not “optional.”

Known limitations: **remove** “default mode has no cryptographic card dealing.” Keep LAN-only, no reconnect, chain unwired, 2-player milestone.

CLI help `--no-crypto` line: `Debug: shared-seed plaintext dealing (all cards visible; sync testing only)`.

Do not claim 9-player WAN, Shamir, or live Ethereum.

---

## Tests

Tiny / `smallPrime` is **not** usable for a machine-integrated test (`FieldToCard` fails). Adapter tests are cheap. The loop test uses `SharedPrime()` + 52 cards (same class as `TestShuffleSession_FakeNet_2Players_ProductionDeck` / `TestDealSession_FakeNet_2Players_Production`).

Fake bus: Go channels, **no libp2p**, do not deliver to `from`, deep-copy messages. Reuse the Phase 2/3 pattern; do not start a `Node`.

Helper: two `NewKeyring`s with only `E.Bytes()` in the public map (never `d`).

### A. Adapters — `internal/network/crypto_hand_test.go` or `network_test.go`

| Test | Assert |
|---|---|
| `TestShuffleMessageWire_RoundTrip` | Build a `ShuffleMessage` (tiny deck ok); `ToWire` / `FromWire`; decks and commitment equal; proto has no extra fields you could have stuffed a perm into |
| `TestPeelMessageWire_RoundTrip` | `PeelMessage` → proto → back; `HandNum` preserved (this is why we do not use `PartialDecryptFromWire` alone) |

### B. Fake-net full loop — `internal/network/crypto_hand_test.go`

Drive **two replicas**, each with: `CryptoHand` + `game.Machine` + its Keyring.

| Test | Assert |
|---|---|
| `TestCryptoHand_FakeNet_2Players_FoldAfterFlop` | Production 52-card. Both shuffle `Done`; `LocalHoles` valid and **different**; opponent `HoleCards` stay `Card{}` on each machine after `StartHandCrypto`. SB/BB check/call through preflop (`ApplyAction` nil, `NeedsStreet`). Flop peels; `ApplyStreet` 3 cards; boards **equal**. One player folds; `PhaseSettled`; `NeedsReveal()==false`. Chip conservation (stacks sum unchanged aside from the pot moving). Timeout ~3 minutes. |
| `TestCryptoHand_FakeNet_2Players_Showdown` | Same setup; check down to river (three `NeedsStreet` cycles); both remaining. Each replica `StartReveal` in **the same seat order** (both remaining ids). `ApplyHoleReveal`; `PhaseSettled`; `Payouts` equal across replicas; each replica now has both seats’ holes; `EvaluateBest7` winner identity agrees. |
| `TestCryptoHand_RevealOrderIndependentOfLocalHoles` | After holes, replica 0’s `MissingRevealIDs` (if you query the machine) is the opponent, replica 1’s is the other way. The driver still reveals `remainingShowdownIDs` in one shared order. If this test builds the id list from `MissingRevealIDs` it **must fail** — that is the point. Assert the helper returns both remaining ids in seat order on **both** replicas. |

Do **not** require distributed cards to equal a `CryptoGame` oracle on a **second** shuffle. For a shared `EncryptedDeck` (both replicas used the same shuffle messages), an oracle `DealProtocol` **with all keys in the test harness** may check `LocalHoles` per seat — optional, already proven in Phase 3.

Privacy: in the fold-after-flop test, before the fold, `machine.State.Players[other].HoleCards` is empty (`Card{}`) on each replica. After fold, still no need to fill them.

### C. Flag / mixed table (cheap, no 2048-bit)

| Test | Assert |
|---|---|
| Host/join arg parse | If you extract `parseHostArgs(args) (cfg, noCrypto, err)`, `[]string{"--seats", "2", "--no-crypto"}` sets `noCrypto==true`. If you do not extract, skip this unit test and verify manually — but **the parse bug must still be fixed**. |
| Mixed table | `KeyringFromLobby` already errors on empty `e`. A small `runP2PMode` helper is hard to unit-test; asserting `AllSeatsHavePublicE==false` when one join has empty `e` already exists from Phase 1. Live path must call that check. No new lobby test required unless you change `HandleJoin`. |

### D. Regression

```
go test ./internal/network -count=1
go test ./internal/crypto ./internal/game ./internal/network
go test ./...
```

Must still pass: all Phase 1–4 tests, `TestCryptoGame_FullProtocol`, plaintext `internal/integration` hands, `TestStartHandCrypto_EmptyHolesOK`.

No new proto. Manual 2-binary demo is **acceptance**, not a `go test`.

---

## Implementation order (do this, then stop)

1. `codec.go` adapters + round-trip tests (A).
2. `BroadcastShuffleMessage` + `Node.SendPeel` (gossip always; direct best-effort).
3. `CryptoHand` shuffle + wait + fake-bus 2-player **shuffle only** (can assert `EncryptedDeck` equal). Then holes. Then stop and plug the machine for test B’s fold-after-flop.
4. `maybeAdvanceCrypto` + `remainingShowdownIDs` used by the fake-net tests (B). Do not open `main.go` until B is green — the driver must work without libp2p first.
5. `runP2PMode`: callbacks before `Start`; buffer; crypto vs `--no-crypto` branch; start hand before TUI in crypto mode; `Init` skip second start; `machineMu` around actions; next-hand new `CryptoHand`.
6. Fix `--no-crypto` arg parse; help text; README status line.
7. `go test ./...`
8. Manual: two terminals, 2 seats, one hand. Confirm opponent holes hidden; community appears together; optional ciphertext log.
9. Stop. Do not start Shamir, chain RPC, or TUI redesign.

If a step’s tests fail, fix that step. If the live path hangs, add logging around `WaitShuffle` / `WaitHoles` / `ExpectedPlayer` / current peel index — do not add timeout-vote recovery.

---

## Error style

New code: `fmt.Errorf` / `errors.New` with a function prefix (`NewCryptoHand: ...`, `CryptoHand.WaitShuffle: ...`, `runP2PMode: crypto dealing requires ...`). Match Phases 1–4. Do not drive-by rewrite empty `fmt.Errorf("")` leftovers in `coordinator.go` / `codec.go`.

---

## Explicit non-goals (push back if asked mid-phase)

- Timeout votes, Shamir reconstruct, `FaultManager.Run` (issue 6). Abort-hand-on-wait-timeout is the only stall handling.
- Ethereum / Hardhat (issue 5).
- DHT, relays, 9-player, mid-hand reconnect, `GAME_STATE_SYNC`.
- New proto fields; `SHUFFLE_COMMIT`.
- Putting `Permutation` or ranks on the wire “for debug.”
- Rewriting `ShuffleSession` / `DealSession` / `StartHandCrypto`.
- Routing live play through `HandCoordinator.RunHand` / `NewCryptoGame`.
- Making hole peels **require** direct streams.
- Using `MissingRevealIDs()` as the reveal sequence.
- Optimizing 2048-bit SRA.
- TUI changes beyond a status string / existing `NetworkMsg`.
- `go test` that starts two real `Node`s / two binaries (manual only).

---

## Hang diagnosis (read before coding)

| Symptom | Likely cause |
|---|---|
| Both sides sit on “Shuffling…” forever | `OnShuffleStep` nil at `node.Start`, or early step dropped before `liveHand` exists (missing buffer), or `Outbound`/self-echo confusion, or `WaitShuffle` without `HandleShuffle` signaling `Done` |
| Holes never finish | Forgot `DealSession.Outbound()`; or mixed reveal/hole jobs; or `SendPeel` only via direct and the stream failed |
| `HandlePeel: wrong card index` at showdown | Each replica `BeginReveal`’d a different player (`MissingRevealIDs` trap) |
| `StartHandCrypto: expected PhaseWaiting` | `Init` still calls `StartHand` after the pre-TUI crypto start |
| Opponent holes visible in TUI mid-hand | Filled every seat before `StartHandCrypto` (Phase 4 leak, or `DealToEngine` / oracle used by mistake) |
| Community boards differ | Shuffle sessions diverged (applied gossip order instead of seat index — should be impossible if Phase 2 is used); or `ApplyStreet` got the full accumulated slice twice |
| `--no-crypto` still crypto / still shared-seed unexpectedly | Flag parse `len(args)-1` bug |

---

## What the examiner should see (this is the last crypto-deal phase)

Default LAN table:

> Peers jointly shuffle under commutative encryption, deal with partial decrypts and ZK checks, and agree on pots locally. Each TUI shows only that player’s hole cards until showdown.

`--no-crypto` is an honest debug switch, not the product.

---

## Review checklist (before calling Phase 5 done)

- [ ] `OnShuffleStep` / `OnPartialDecrypt` set before `node.Start()`
- [ ] Early shuffle/peel messages buffered until `CryptoHand` exists, then drained
- [ ] Default path: Keyring → shuffle → local holes only → `StartHandCrypto`
- [ ] `--no-crypto`: shared-seed `StartHand`; status line says debug / all cards visible
- [ ] Mixed table (empty `e` on any seat) errors out in crypto mode
- [ ] `--no-crypto` parsed when it is the last CLI arg
- [ ] Every `Begin*` / `HandlePeel` drains `Outbound()` and sends those peels
- [ ] Live shuffle send uses `ShuffleMessage` (no permutation on the wire)
- [ ] Streets: `ApplyStreet` gets only the new cards; all-in runout loops `NeedsStreet`
- [ ] Showdown reveal order = remaining seats in seat order on **every** replica
- [ ] Fold-to-winner does not reveal
- [ ] Next hand creates a new `CryptoHand` / new session id (handNum mixed in)
- [ ] `machineMu` held across `ApplyAction` + crypto advance
- [ ] README + help match reality
- [ ] Fake-net 2-player fold-after-flop + showdown tests pass
- [ ] `TestCryptoGame_FullProtocol` / Phase 1–4 tests still pass
- [ ] `go test ./...` green
- [ ] No new proto fields; `SHUFFLE_COMMIT` still unused
- [ ] `internal/crypto` session files unchanged (or only if a bug blocker — prefer not)
- [ ] Manual 2-player: opponent holes hidden; community appears together

---

## Time

This is the slowest phase: 2048-bit shuffle + peels in tests, plus live callback races. Still **one sequential implementation pass** if this spec is followed — do not grow it into Shamir or a second package rewrite.

If the fake-bus loop (step 4) is not green, **do not** wire `main.go`. A hanging demo with no test is worse than shared-seed still being default for one more conversation.
