# Phase 4 — Game machine: cards as inputs (implementation spec)

Parent: [`CRYPTO_DEAL_PLAN.md`](../CRYPTO_DEAL_PLAN.md). Consumes Phase 3’s peeled cards (`LocalHoles` / `CommunityCards` / `RevealedHoles`). Fixes the **engine** half of issues 1–3: in crypto mode the machine must not sample a local deck, must not require every seat’s hole cards at the start of the hand, and must take flop / turn / river / showdown cards as **external inputs**.

**This phase does not change live dealing.** `poker host` / `poker join` still shuffle from the shared seed. Phase 5 is when `runP2PMode` calls `StartHandCrypto` / `ApplyStreet` / `ApplyHoleReveal`.

**Do not implement later phases from this doc.** After this lands and tests pass, stop.

---

## Why this phase exists

Today:

| Fact | Where |
|---|---|
| `StartHand` shuffles `gs.Deck` and deals holes + later streets from it | `internal/game/machine.go` |
| `StartHandCrypto` skips the local shuffle **but requires every seat to already have hole cards** | same file |
| `DealToEngine` fills **all** seats then sets `gs.Deck = nil` | `internal/crypto/crypto_game.go` |
| `ApplyAction` → `endBettingRound` → `dealFlop` / `dealTurn` / `dealRiver` always calls `gs.Deck.Deal()` | `machine.go` |
| `Deck.Deal()` on a nil receiver **panics** (`len(d.Cards)`) | `internal/game/deck.go` |
| `startShowdown` → `distributePots` → `EvaluateBest7` on whatever is in `Player.HoleCards` | `machine.go` |
| TUI already hides non-local holes (`IsLocalPlayer \|\| IsWinner`) | `internal/tui/player_panel.go` |
| Phase 3 `DealSession` can decode **only local** holes until `BeginReveal` | `internal/crypto/deal_session.go` |

Even with perfect peels, wiring Phase 3 into the live loop would still fail:

1. `StartHandCrypto` refuses to start unless opponents’ cards are already on this replica — that is the leak (risk note 2 in the parent plan).
2. The first check/call that ends preflop still pulls the flop from `gs.Deck`. With `Deck == nil` that panics; with a leftover plaintext deck it desyncs from the peeled cards.
3. Showdown evaluates empty `Card{}` holes on every replica that did not see the opponent’s cards.

The machine is a **reducer**. Betting actions are inputs (already). In crypto mode, cards are inputs too. Shared-seed `StartHand` was cheating by sampling the same RNG on every node.

Keep `StartHand` as the plaintext path. Do not change pot / eval math.

---

## Goal / done when

1. Crypto mode = `gs.Deck == nil`. `StartHandCrypto` **sets** that (and does not deal from a deck).
2. `StartHandCrypto` does **not** require opponent (or any) hole cards. Post blinds, go to preflop. Document: other seats’ holes are filled at showdown via `ApplyHoleReveal`.
3. When `Deck == nil`, `endBettingRound` does **not** call `dealFlop` / `dealTurn` / `dealRiver`. It enters `PhaseAwaitingStreet`. `ApplyAction` rejects that phase (the last action already committed — do **not** return a need-street error from `ApplyAction`).
4. `ApplyStreet(cards []Card)`: flop expects 3, turn/river expect 1; then `startNewBettingRound`. All replicas will call this with the same peeled cards (Phase 5).
5. Showdown: if remaining players still have empty holes, stay in `PhaseShowdown` and wait for `ApplyHoleReveal`. When every remaining seat is filled, existing `distributePots`.
6. `dealFlop` / `Deal()` on a nil deck **must not panic**.
7. Plaintext `StartHand` + existing `game_test.go` / integration hands still pass. `go test ./...` is green.
8. Live multiplayer is still the shared-seed path. No `runP2PMode` change.

You should be able to explain: the machine is a reducer; cards are inputs, not something it samples; why `StartHandCrypto` must not demand every seat’s holes; why `ApplyAction` must not return “need street” as an error (the fold/check already happened).

---

## Engine reminder (do not re-derive)

Street order and deck indexes are already fixed by the engine and by Phase 3. This phase never peels and never looks at an encrypted deck. It only **accepts** the plaintext cards Phase 3 produced:

| Street | `ApplyStreet` length | `len(CommunityCards)` before | `Phase` after success |
|---|---|---|---|
| flop | 3 | 0 | `PhaseFlop` |
| turn | 1 | 3 | `PhaseTurn` |
| river | 1 | 4 | `PhaseRiver` |

Burns are **not** machine inputs. Phase 3 skipped them; the reducer only sees the public cards.

Showdown is “community peel of private indexes” on the crypto side. On the engine side it is `ApplyHoleReveal(playerID, [2]Card)` until every **remaining** seat (`StatusActive` or `StatusAllIn`) has both hole cards. Folded / sitting-out seats are not required.

Fold-to-one-winner (`resolveSingleWinner`) does **not** need cards or reveals. Unchanged.

The machine has **no concept of a local player**. Requiring “our” holes would mean passing a peer id into `StartHandCrypto` and would make two honest replicas disagree on whether the hand can start. Each replica fills whatever holes it already knows (Phase 5: local only) and leaves the rest empty.

---

## Scope

### Touch

| File | Change |
|---|---|
| `internal/game/state.go` | `PhaseAwaitingStreet` **appended** to the iota; `String()` |
| `internal/game/machine.go` | `StartHandCrypto`; crypto branch in `endBettingRound` / `startNewBettingRound`; `ApplyStreet`; `ApplyHoleReveal`; query helpers; nil-deck guards; `ApplyAction` rejects awaiting-street |
| `internal/game/deck.go` | nil-receiver guard on `Deal` / `Remaining` / `Shuffle` (no panic) |
| `internal/game/game_test.go` | replace the “all holes required” test; new crypto-street / reveal tests |

### Do not touch

- `cmd/poker/main.go` — no `StartHandCrypto`, no `OnShuffleStep` / `OnPartialDecrypt`
- `internal/crypto/*` — including `DealToEngine` (oracle still fills every seat; that is the local simulation)
- `internal/network/*` including `coordinator.go` (`HandCoordinator.RunHand` stays the in-process oracle bootstrap)
- proto / `messages.proto` / `messages.pb.go`
- `internal/game/pot.go`, `hand_eval.go`, `player.go` (except you may add a tiny `holeCardsDealt` helper in `player.go` or keep it unexported in `machine.go`)
- Ethereum, Shamir, fault manager, TUI source, README / CLI help

`game` must **not** import `crypto` or `network`. Tests inject `[]Card` / `[2]Card` by hand. Phase 5 maps:

```
DealSession.LocalHoles()        →  Players[local].HoleCards; StartHandCrypto()
DealSession.CommunityCards()    →  ApplyStreet
DealSession.RevealedHoles(pid)  →  ApplyHoleReveal
```

Do not add proto fields for streets. Cards arrive from peels, not from a new gossip type, in this phase.

---

## Current code to reuse (do not rewrite)

```go
func (m *Machine) StartHand() error           // plaintext; keep
func (m *Machine) StartHandCrypto() error     // rewrite the hole-card requirement + nil deck
func (m *Machine) ApplyAction(a Action) error
func (m *Machine) endBettingRound() error     // plaintext branch unchanged
func (m *Machine) dealFlop() error            // plaintext only; nil-deck must error, not panic
func (m *Machine) startNewBettingRound() error
func (m *Machine) startShowdown() error
func (m *Machine) distributePots() error      // pot math unchanged
func (m *Machine) resolveSingleWinner() error // unchanged; no cards needed

func NewGameState(...) *GameState             // still allocates Deck: NewDeck()
func (d *Deck) Deal() (Card, error)

func EvaluateBest7(cards [7]Card) EvaluatedHand
func CalculatePots(players []*Player) []PotSlice
```

`NewGameState` always creates a plaintext deck. Crypto callers (tests now; Phase 5 later) must end up with `Deck == nil`. `StartHandCrypto` is what locks that in.

Empty hole cards are `Card{}` (`Rank == 0`). Rank `Two` is `2`, so `Two of Spades` is **not** empty. Keep using `c == (Card{})` (or a tiny helper). Do not treat rank 0 as a dealt card.

---

## Design

### 1. `PhaseAwaitingStreet` (append, do not insert)

```go
const (
    PhaseWaiting Phase = iota
    PhasePreFlop
    PhaseFlop
    PhaseTurn
    PhaseRiver
    PhaseShowdown
    PhaseSettled
    PhaseAwaitingStreet // NEW — must stay last
)
```

**Append.** Inserting in the middle would shift `PhaseShowdown` / `PhaseSettled` integer values (`GameStateSync.phase` is an `int32`; TUI tests assign the named constants, but Phase 5 will send the number). Update `Phase.String()` with `"Awaiting Street"` as the last slot. If the slice is short, `String()` panics — that is the one TUI-visible change, and it does **not** require editing `internal/tui`.

Do **not** add `PhaseAwaitingReveal`. Incomplete showdown stays `PhaseShowdown`, which `ApplyAction` already rejects.

### 2. Crypto mode

```go
func (m *Machine) cryptoMode() bool { return m.State != nil && m.State.Deck == nil }
```

Unexported. Phase 5 should not need it; it uses `NeedsStreet` / `NeedsReveal`.

### 3. `StartHandCrypto`

Replace the “every seat has holes” loop. Suggested body:

1. `Phase == PhaseWaiting`; `len(Players) >= 2`. Real errors with prefix `StartHandCrypto:` (today both are `fmt.Errorf("")` — fix those two while you are here).
2. `len(CommunityCards) == 0` else error (`StartHandCrypto: community cards already dealt`).
3. **`gs.Deck = nil`** — even if `NewGameState` allocated one. This is what makes `cryptoMode()` true for the rest of the hand.
4. Do **not** inspect hole cards. Do **not** call `dealHoleCards`.
5. `postBlinds()`; set `ActionIdx` / `LastRaiserIdx` / `RoundActionCount` the same as today; `Phase = PhasePreFlop`.

Callers **should** fill local holes first (Phase 5 will). The engine does not enforce it, so two replicas with different known holes can still start the same betting round.

`StartHand` plaintext path: add a nil-deck guard (`StartHand: no deck; use StartHandCrypto`) so `Deck.Shuffle` cannot panic. Do not change shuffle / deal / blinds otherwise.

Existing `TestStartHandCrypto_RequiresHoleCards` **must change**: empty holes are now allowed. Replace it (see tests). `TestStartHandCrypto_WithHoleCards` stays valid (all seats filled is still a legal start — that is the oracle / control).

### 4. Do not return “need street” from `ApplyAction`

After a successful fold/check/call, `advanceAction` may call `endBettingRound`. If that returns an error, `ApplyAction` surfaces it and callers treat the action as **rejected** — but the player is already folded and the log already has the action. There is no rollback.

Therefore:

- Crypto `endBettingRound` returns **`nil`** after setting `PhaseAwaitingStreet`.
- `ApplyAction` then returns `nil`.
- Callers (tests now; Phase 5 later) inspect `NeedsStreet()` / `Phase`.

Do **not** introduce `ErrNeedStreet` as an `ApplyAction` return. A sentinel used by `ApplyStreet` when called at the wrong time is optional; a prefixed `fmt.Errorf` is enough.

`ApplyAction` reject list becomes:

```go
if gs.Phase == PhaseShowdown || gs.Phase == PhaseSettled ||
    gs.Phase == PhaseWaiting || gs.Phase == PhaseAwaitingStreet {
    return fmt.Errorf("ApplyAction: no actions allowed in phase %s", gs.Phase)
}
```

You are already on the `ApplyAcction` typo line — fix the string. Do not sweep other typos in the file.

### 5. `endBettingRound` crypto branch

```go
func (m *Machine) endBettingRound() error {
    gs := m.State
    gs.Pots = CalculatePots(gs.Players)

    if m.cryptoMode() {
        switch gs.Phase {
        case PhasePreFlop, PhaseFlop, PhaseTurn:
            gs.Phase = PhaseAwaitingStreet
            return nil
        case PhaseRiver:
            return m.startShowdown()
        default:
            return fmt.Errorf("endBettingRound: unexpected phase %s", gs.Phase)
        }
    }

    // plaintext: existing switch → dealFlop / dealTurn / dealRiver / startShowdown
}
```

Which street is pending is derived from `len(CommunityCards)` (0 → flop, 3 → turn, 4 → river). Do **not** add a `pendingStreet` field unless the length check becomes ambiguous (it should not).

### 6. All-in runout (`startNewBettingRound`)

Today, if nobody `CanAct()` (`first == -1`), the function jumps to `startShowdown` **without** dealing remaining streets. That is wrong Hold’em (all-in preflop should still run flop/turn/river) but plaintext tests may depend on it.

**Crypto only:** if `first == -1`, call `endBettingRound()` instead of `startShowdown()`. That waits for the next street (or showdown after river) via the branch in §5.

```go
first := gs.nextActiveIndex(gs.DealerIdx)
if first == -1 {
    if m.cryptoMode() {
        return m.endBettingRound()
    }
    return m.startShowdown()
}
```

Do **not** change the plaintext `first == -1` path. The `countCanAct() <= 1` line already calls `endBettingRound` and is correct for “one player can act, others all-in.”

### 7. `ApplyStreet`

```go
func (m *Machine) ApplyStreet(cards []Card) error
```

**Rules** (prefix `ApplyStreet:`)

1. `m.cryptoMode()` else error (`not crypto mode`).
2. `Phase == PhaseAwaitingStreet` else error.
3. `want := pendingStreetCount(len(gs.CommunityCards))` — `3` if 0 community, `1` if 3, `1` if 4; otherwise error (`board already complete` / `unexpected board size`).
4. `len(cards) == want`.
5. Every card `IsDealt()` (rank in `Two..Ace`, suit in `Spades..Clubs`). Reject `Card{}`.
6. No duplicates inside `cards`, and none that already appear in `CommunityCards` or in any **already-dealt** hole pair. (Catches test mistakes; Phase 5 peels should already be unique.)
7. Append a copy of `cards` to `CommunityCards`.
8. Set `Phase` to `PhaseFlop` / `PhaseTurn` / `PhaseRiver` from the new length (3 / 4 / 5).
9. `return m.startNewBettingRound()`.

`startNewBettingRound` may immediately come back to `PhaseAwaitingStreet` (all-in runout). That is success: `ApplyStreet` returns nil and `NeedsStreet()` is true again.

### 8. Showdown wait + `ApplyHoleReveal`

```go
func (m *Machine) startShowdown() error {
    gs := m.State
    gs.Phase = PhaseShowdown
    gs.Pots = CalculatePots(gs.Players)
    if m.cryptoMode() && m.remainingHolesIncomplete() {
        return nil
    }
    return m.distributePots()
}

func (m *Machine) ApplyHoleReveal(playerID string, cards [2]Card) error
```

`remainingHolesIncomplete`: some player with `StatusActive` or `StatusAllIn` has a `Card{}` hole. Folded / sitting-out do not count.

**`ApplyHoleReveal` rules** (prefix `ApplyHoleReveal:`)

1. `m.cryptoMode()` else error.
2. `Phase == PhaseShowdown` else error (not Settled, not a betting street).
3. `playerID` seated; status is Active or All-In (revealing a folded seat → error).
4. Both cards `IsDealt()`; not duplicates of each other; not duplicates of community or other **already-dealt** holes.
5. If this seat already has both holes:
   - equal to `cards` → **ignore** (idempotent; local replica already filled them at deal time);
   - different → error (equivocation).
6. If empty, assign `p.HoleCards = cards`.
7. If `!remainingHolesIncomplete()`, `return m.distributePots()`.
8. Else return nil; stay in `PhaseShowdown`.

Reveal order is free. Phase 5 will `BeginReveal` remaining seats one at a time; the engine does not care.

Do **not** change `distributePots` / `potWinners` arithmetic. Optional safety (in scope, one `if`): if `cryptoMode()` and still incomplete, `distributePots` returns an error instead of evaluating `Card{}`. `startShowdown` / `ApplyHoleReveal` should already prevent that.

### 9. Query helpers (exported for Phase 5)

```go
func (m *Machine) NeedsStreet() bool
func (m *Machine) PendingStreetCount() int // 3, 1, or 0; 0 if !NeedsStreet
func (m *Machine) NeedsReveal() bool       // PhaseShowdown && remaining holes incomplete
func (m *Machine) MissingRevealIDs() []string // remaining seats with empty holes; stable seat order
```

No mutex on `Machine` today — do not add one. Phase 5 continues to apply actions on one goroutine per replica (same as `ApplyAction`).

### 10. Nil-deck guards

| Method | Nil / crypto behavior |
|---|---|
| `Deck.Deal` | `d == nil` → error, **no panic** (`Deck.Deal: deck is nil`) |
| `Deck.Shuffle` / `Remaining` | same; `Remaining` may return 0 |
| `dealFlop` / `dealTurn` / `dealRiver` | if `gs.Deck == nil` → error (`dealFlop: no local deck; use ApplyStreet`); do not call `Deal` |
| `StartHand` | `gs.Deck == nil` → error |

Crypto `endBettingRound` must not call `dealFlop` at all; the guards are belt-and-suspenders for same-package tests and future mistakes.

### 11. Tiny card helper (optional, same package)

```go
func (c Card) dealt() bool {
    return c.Rank >= Two && c.Rank <= Ace && c.Suit <= Clubs
}

func holeCardsDealt(p *Player) bool {
    return p != nil && p.HoleCards[0].dealt() && p.HoleCards[1].dealt()
}
```

Unexported is fine. Do not add a second `Card` type.

---

## Tests

All in `internal/game` unless noted. Plaintext tests keep using `newTestGame` + `StartHand`. Crypto tests:

```go
func newCryptoTestGame(n int, stack int64) (*Machine, []*Player) {
    m, players := newTestGame(n, stack)
    m.State.Deck = nil // StartHandCrypto also nils it; setting here documents intent
    return m, players
}
```

Do **not** import `crypto`. Inject cards as literals.

### A. `StartHandCrypto` — replace / extend existing

| Test | Assert |
|---|---|
| `TestStartHandCrypto_EmptyHolesOK` | **Replaces** `TestStartHandCrypto_RequiresHoleCards`. No holes filled; `StartHandCrypto` succeeds; `PhasePreFlop`; `Deck == nil`; blinds posted (`CurrentBet == BigBlind`). |
| `TestStartHandCrypto_OnlySeat0Holes_Preflop` | Seat 0 has `Ah Kh`; seat 1 empty. Preflop. Seat 1 still empty. |
| `TestStartHandCrypto_WithHoleCards` | Keep: both seats filled still works (oracle / control). Also assert `Deck == nil` after the call. |
| `TestStartHand_NilDeckRejected` | `newTestGame` then `Deck = nil` then `StartHand` → error; no panic. |

### B. Streets (crypto, 2 players, check it down)

Drive with `ActionCheck` / `ActionCall` until the betting round ends (same style as existing game tests). After the action that completes preflop:

| Test | Assert |
|---|---|
| `TestCrypto_PreflopEnd_WaitsForStreet` | `ApplyAction` returns **nil**; `Phase == PhaseAwaitingStreet`; `NeedsStreet()`; `PendingStreetCount()==3`; `len(CommunityCards)==0`. A further `ApplyAction` errors. |
| `TestCrypto_ApplyStreet_FlopTurnRiver` | `ApplyStreet` 3 cards → `PhaseFlop`, 3 community, new betting round. Check through flop → awaiting, `PendingStreetCount()==1`. `ApplyStreet` 1 → turn. Same for river. After river betting, `PhaseShowdown` or `PhaseSettled` depending on whether opponent holes were filled (this test should leave seat 1 empty → showdown wait). |
| `TestCrypto_ApplyStreet_WrongLengthRejected` | Awaiting flop; `ApplyStreet` 1 card → error; still awaiting; community still empty. |
| `TestCrypto_ApplyStreet_NotAwaitingRejected` | Call `ApplyStreet` during preflop → error. |
| `TestCrypto_ApplyStreet_PlaintextRejected` | After `StartHand` (deck present), `ApplyStreet` errors. |
| `TestDealFlop_NilDeck_NoPanic` | Same-package: `Deck = nil`; call `dealFlop()`; error, no panic. Also `(*Deck)(nil).Deal()`. |

Use distinct dealt cards, e.g. flop `2s 7h 9d`, turn `3c`, river `4s`, so duplicate checks are meaningful.

### C. Showdown reveals + payout control

| Test | Assert |
|---|---|
| `TestCrypto_Showdown_WaitsForReveal` | Seat 0 holes filled at start; seat 1 empty. Play check-check through all streets with injected board. After river: `PhaseShowdown`, `NeedsReveal()`, `MissingRevealIDs()` is seat 1 only, `Payouts` still empty / stacks unchanged aside from blinds already in the pot. |
| `TestCrypto_ApplyHoleReveal_ThenPayoutsMatchControl` | **Machine A:** only seat 0 holes at start; streets injected; `ApplyHoleReveal` seat 1. **Machine B (control):** both seats’ holes filled at `StartHandCrypto`; same `ApplyStreet` cards; no reveal needed (or reveal is idempotent). After settle: same `Payouts`, same stacks, both `PhaseSettled`. Use a board where seat 0 clearly wins (e.g. `As Ah` vs `Ks Kh`, dry board) so the assertion is obvious. |
| `TestCrypto_ApplyHoleReveal_IdempotentLocal` | Seat 0 already has holes; `ApplyHoleReveal` with the **same** two cards succeeds; different two cards → error. |
| `TestCrypto_FoldToWinner_NoReveal` | Crypto, seat 1 empty holes; seat 1 folds preflop. `PhaseSettled`; pot to seat 0; `NeedsReveal()==false`. |
| `TestCrypto_AllInRunout_WaitsEachStreet` | Both players all-in preflop (or SB/BB consume stacks). After the all-in action: awaiting flop (not showdown, not settled). Three `ApplyStreet` calls in a row may be needed if `startNewBettingRound` immediately re-awaits. End in showdown wait if opponent holes empty. |

### D. Regression

```
go test ./internal/game -count=1
go test ./...
```

Must still pass: `TestStartHand`, `TestFoldWinsHand`, community-card / side-pot / multi-hand tests in `game_test.go`, `internal/integration` plaintext hands, `TestCryptoGame_FullProtocol`, all Phase 1–3 tests.

`HandCoordinator.RunHand` still fills every seat via `DealToEngine` then `StartHandCrypto` — that remains legal. Nobody plays streets on that object in tests today. Do not “fix” it to inject streets.

No new proto. No live 2-player crypto demo.

---

## Implementation order (do this, then stop)

1. `PhaseAwaitingStreet` + `String()`; a one-line test that `PhaseAwaitingStreet.String()` is non-empty and does not panic.
2. `Deck` nil guards + `TestDealFlop_NilDeck_NoPanic`.
3. `StartHandCrypto` rewrite + `StartHand` nil-deck guard. Tests in group A. Run `go test ./internal/game -count=1`.
4. Crypto `endBettingRound` + `ApplyAction` reject + `NeedsStreet` / `PendingStreetCount`. Group B waiting test.
5. `ApplyStreet` + flop/turn/river test + all-in runout crypto branch in `startNewBettingRound`.
6. `startShowdown` wait + `ApplyHoleReveal` + group C (control payouts, fold, idempotent).
7. `go test ./...`
8. Stop. Do not open `main.go`. Do not wire `OnPartialDecrypt`.

If a step’s tests fail, fix that step. Do not start Phase 5 files.

---

## Error style

New code: `fmt.Errorf` / `errors.New` with a function prefix (`StartHandCrypto: ...`, `ApplyStreet: ...`, `ApplyHoleReveal: ...`, `Deck.Deal: deck is nil`). Match Phases 1–3. Fix the two empty `fmt.Errorf("")` in `StartHandCrypto` because you are rewriting that function. Do not drive-by rewrite other empty errors in `machine.go`.

---

## Explicit non-goals (push back if asked mid-phase)

- Wiring `StartHandCrypto` / `ApplyStreet` in `runP2PMode` — Phase 5.
- Importing `DealSession` into `game` or calling peels from the machine.
- Changing `DealToEngine` to fill only one seat (oracle stays “all keys, all holes”).
- Changing plaintext `StartHand` street dealing, pot splits, or hand eval.
- Fixing plaintext `first == -1` skip-the-board (crypto-only runout is enough).
- New proto messages for street cards.
- Mutex on `Machine` / `ApplyAction` (issue 7) unless you are already on a line that panics; even then, skip — Phase 5 optional one-liner.
- TUI changes (hiding opponent holes already works; empty `Card{}` already renders as hidden/empty).
- Timeouts / abort-hand.
- libp2p. If a test starts a `Node`, it is out of scope.

---

## How Phase 5 will consume this (do not implement)

```go
holes, err := deal.LocalHoles()
gs.Players[localIdx].HoleCards = holes
if err := machine.StartHandCrypto(); err != nil { ... }

// after each successful ApplyAction:
if machine.NeedsStreet() {
    out, err := deal.BeginStreet(streetFromCount(machine.PendingStreetCount()))
    // ... HandlePeel until StreetDone ...
    cards, err := deal.CommunityCards() // or the suffix for this street
    _ = machine.ApplyStreet(justDealt)
}
if machine.NeedsReveal() {
    for _, pid := range machine.MissingRevealIDs() {
        out, err := deal.BeginReveal(pid)
        // ... HandlePeel until RevealDone ...
        pair, err := deal.RevealedHoles(pid)
        _ = machine.ApplyHoleReveal(pid, pair)
    }
}
```

If Phase 4 still requires every hole at `StartHandCrypto`, Phase 5 will “wire crypto” and still show every card. If Phase 4 still deals the flop from `gs.Deck`, replicas will disagree with peeled community cards. If `ApplyAction` returns a need-street **error**, Phase 5 will treat a legal check as a failed action.

---

## Review checklist (before calling Phase 4 done)

- [ ] `StartHandCrypto` succeeds with empty opponent holes; `Deck == nil`
- [ ] `StartHandCrypto` does not require any particular seat to have cards
- [ ] Plaintext `StartHand` still shuffles and deals; existing game tests pass
- [ ] Crypto end-of-preflop: `ApplyAction` nil, `PhaseAwaitingStreet`, further actions rejected
- [ ] `ApplyStreet` 3 / 1 / 1 moves flop → turn → river; wrong length rejected
- [ ] `dealFlop` / `Deck.Deal` on nil does not panic
- [ ] Showdown waits when remaining holes are empty; `ApplyHoleReveal` then payouts match a control with cards known from the start
- [ ] Fold-to-winner in crypto settles without reveals
- [ ] All-in crypto runout waits for each street instead of showing down a short board
- [ ] `ApplyHoleReveal` idempotent on matching local holes; equivocation rejected
- [ ] `distributePots` / `EvaluateBest7` math unchanged
- [ ] `go test ./...` green
- [ ] `cmd/poker/main.go` diff is empty
- [ ] `internal/crypto` diff is empty
- [ ] `internal/network` diff is empty
- [ ] No new proto fields
- [ ] `PhaseAwaitingStreet` is **appended** (existing phase integers unchanged)

---

## Time

Half a day of AI implementation including tests, if this spec is followed without wiring `main.go`. There is no 2048-bit work in this phase; tests should be milliseconds.

If the work grows into `OnPartialDecrypt`, `runP2PMode`, or libp2p, stop and split — do not pull Phase 5 into this conversation.
