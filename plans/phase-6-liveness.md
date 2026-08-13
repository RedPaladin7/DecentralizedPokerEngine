# Phase 6 — Liveness (timeout fold + Shamir peels)

Parent: [`ISSUES_AND_RECOMMENDATIONS.md`](../ISSUES_AND_RECOMMENDATIONS.md) issue 6. Live crypto dealing is Phase 5 ([`phase-5-wire-p2p.md`](./phase-5-wire-p2p.md), [`CRYPTO_DEAL_PLAN.md`](../CRYPTO_DEAL_PLAN.md)).

This phase is **not** required to call the project decentralized poker. It is the liveness story: a silent peer is folded, cannot rejoin, and if two or more players remain, they reconstruct that peer’s `d` and finish peels so the hand does not halt.

**Do not implement later work from this doc** (reconnect, mid-shuffle recovery, ETH, DHT). After this lands and tests pass, stop.

---

## Why this phase exists

Today:

| Fact | Where |
|---|---|
| Table accepts `--seats 2` | `config.Validate` (2–9), `printHelp` |
| Heartbeats are **sent** | `HeartbeatSender.Run` in `runP2PMode` |
| Heartbeats are **recorded** | `OnHeartbeat` → `fm.RecordHeartbeat` |
| Heartbeat **monitor** is never started | `FaultManager.Run` has no caller in `cmd/poker` |
| Timeout votes exist on the wire | `BroadcastTimeoutVote` / `OnTimeoutVote` — **unset** |
| `forceFold` applies a local fold only | `p2pGameModel.forceFold` — no `BroadcastAction` |
| Shamir split/reconstruct is unit-tested | `internal/crypto/commit.go`, `internal/fault/shamir.go` |
| Shares are never sent | no `KEY_SHARE` proto; `SplitAndDistribute` only in tests |
| `DealSession` can only peel with `LocalKey()` | `executeLocalLocked` |
| `kickCryptoAdvance` holds `machineMu` across `WaitStreet` | deadlock if timeout fold needs that mutex |
| Mid-shuffle / mid-peel wait | 2-minute `Wait*` then `ErrorMsg`; hand never settles |

Folding a silent player is **not** enough in crypto mode once the shuffle has finished. Every card still has that player’s encryption layer. Two remaining players cannot peel the flop without their `d`. Timeout-fold and Shamir are one product.

**Heads-up math:** Shamir threshold is ≥ 2. After one drop from a 2-seat table, one share remains. Reconstruction is impossible. This phase therefore requires **n ≥ 3** on the live P2P table. At n = 3, t = 2, one drop leaves two shares — reconstruction can work.

Keep `SplitSecret` / `KeyShareStore` / `TimeoutManager` as the in-process implementations. This phase **schedules** them from `runP2PMode` and teaches `DealSession` to publish a peel *for* a reconstructed key.

---

## Goal / done when

1. Live P2P (`host` / `join`) rejects `MaxSeats < 3`. Local vs bots is unchanged (still 2–9).
2. `FaultManager.Run` starts next to `HeartbeatSender`. Silence → `TIMEOUT_VOTE` → 2/3 of `n-1` → fold.
3. That fold is applied **and** broadcast as `PLAYER_ACTION` (same sequencer as a real fold). Local-only fold is a bug.
4. After lobby fill (crypto mode): each peer `SplitAndDistribute`s its `d` and **direct-sends** one share to each other seat. Never gossip live shares.
5. After a confirmed timeout, remaining peers publish their share of the missing `d` (gossip is OK **now**). Each reconstructs `SRAKey`. Do **not** put that `d` in `Keyring`.
6. When `DealSession` expects a peel from the missing player, a **designated survivor** (first remaining id in `SeatOrder()`) calls `PeelOnBehalf` and broadcasts the `PeelMessage` with `PlayerID = missing`. Others `HandlePeel` as usual.
7. Mid-shuffle disconnect: **unchanged** — `WaitShuffle` times out, `ErrorMsg`, hand aborts. Reconstructing `d` does not recover the secret permutation. No rejoin.
8. `--no-crypto`: timeout fold only (plaintext deck needs no `d`). No share distribution.
9. `machineMu` is **not** held across `WaitShuffle` / `WaitHoles` / `WaitStreet` / `WaitReveal`.
10. `go test ./...` green. Fake-net 3-player test: after holes, one replica stops; remaining two fold them, reconstruct, flop peels complete, boards equal.

You should be able to explain: why n ≥ 3; why fold without Shamir still hangs on the next street; why mid-shuffle is aborted; why initial shares are unicast.

---

## Protocol reminder (do not re-derive)

```
lobby full (n ≥ 3)
  → (crypto) SplitAndDistribute local d; unicast share[i] to SeatOrder()[i]
  → existing Phase 5 shuffle + hole peels + StartHandCrypto
  → HeartbeatSender + FaultManager.Run

silence > heartbeat_timeout
  → BroadcastTimeoutVote
  → 2/3 of (n−1) yes
  → forceFold + BroadcastAction(fold)     // betting liveness
  → (crypto, shuffle already Done)
        survivors gossip KEY_SHARE of missing d
        ReconstructSRAKey
        CryptoHand.MarkGone(id, reconstructed)
        designated survivor PeelOnBehalf whenever that id is next peeler
  → AdvanceCrypto continues (flop / turn / river / reveal)

WaitShuffle timeout → abort hand (out of scope to recover)
disconnect → no rejoin (GAME_STATE_SYNC still unused)
```

Timeout vote threshold is already `TimeoutVoteThreshold = 2/3` of `n-1` (`timeout.go`). For n = 3 that is 2 voters, `int(2*(2/3)+0.5) = 1` — **one** other player’s vote confirms. Accept that for v1; do not invent a second threshold. For n = 4 it is 2 of 3.

`SplitAndDistribute` already uses `t = max(2, (n+1)/2)`:

| n | t | After 1 drop, remaining shares of missing d | Reconstruct? |
|---|---|---|---|
| 2 | 2 | 1 | **No** — why min 3 |
| 3 | 2 | 2 | Yes |
| 4 | 2 | 3 | Yes |
| 5 | 3 | 4 | Yes |

Share `i` (proto `index` = Shamir `x` = `i+1`) is issued to `SeatOrder()[i]`. The owner keeps their own share locally (no network). When they disconnect, that share is gone; survivors still meet `t` for n ≥ 3.

Delegated `PeelMessage.PlayerID` is the **missing** player, not the publisher. ZK is a proof of the reconstructed `d`. `HandlePeel` already accepts a peel whose `PlayerID != LocalID`.

---

## Scope

### Touch

| File | Change |
|---|---|
| `internal/network/messages.proto` | `KEY_SHARE = 13` + `KeyShare` message; regenerate `messages.pb.go` |
| `internal/network/codec.go` | `KeyShare` ↔ `pokercrypto.ShamirShare` helpers |
| `internal/network/node.go` | `SendDirectKeyShare`, `BroadcastKeyShare`; stream handler dispatches `KEY_SHARE` as well as `PARTIAL_DECRYPT`; `OnKeyShare` callback |
| `internal/crypto/deal_session.go` | `PeelOnBehalf(playerID, key)`, `ExpectedPeeler() string` |
| `internal/crypto/deal_session_test.go` | delegated peel + “gone peeler unblocks” |
| `internal/fault/manager.go` | `SetHandNum`; keep existing vote/share APIs |
| `internal/network/crypto_hand.go` | `MarkGone`, `TryDelegatedPeels`; `AdvanceCrypto` / waits must work with gone peelers |
| `internal/network/crypto_hand_test.go` | 3-player fake-net: kill one after holes, reconstruct, flop |
| `internal/network/liveness.go` | **new** (optional name) — share distribute + recover driver, **no** TUI |
| `cmd/poker/main.go` | min seats 3; `fm.Run`; `OnTimeoutVote`; broadcast fold; share distribute; reconstruct; **unlock `machineMu` during Wait\*** |
| `cmd/poker/main.go` `printHelp` | seats 3–9 |
| `README.md` | known limitation: mid-shuffle abort; disconnect = fold + Shamir peels, no rejoin |

If `liveness.go` stays small, the driver may live in `crypto_hand.go` instead. Do not put share maps in `package crypto`.

### Do not touch

- `internal/crypto/shuffle_session.go` — no abort-and-reshuffle API
- `internal/crypto/keyring.go` — still never stores another peer’s `d` (keep `Len() >= 2` for library tests)
- `internal/crypto/sra.go`, `zkp.go`, `commit.go` Shamir math, `params.go`
- `internal/crypto/crypto_game.go`, `coordinator.go` (oracle)
- `internal/game/machine.go` pot/eval (fold already exists)
- `internal/tui/*` except maybe a one-line log if `NetworkMsg` is already there
- ETH, DHT, relays, `GAME_STATE_SYNC`, mid-hand reconnect
- Replacing SRA; changing timeout from 2/3; lowering Shamir t below 2

`crypto` must **not** import `network` or `fault`. `PeelOnBehalf` takes a `*SRAKey` the caller already reconstructed.

---

## Current code to reuse (do not rewrite)

```go
func SplitAndDistribute(ownerKey *SRAKey, numPlayers int) ([]ShamirShare, int, error)
func (fm *FaultManager) Run(ctx context.Context)           // starts HeartbeatMonitor
func (fm *FaultManager) StartTimeoutVote(target string)
func (fm *FaultManager) HandleTimeoutVote(target, voter string, yes bool) (VoteStatus, error)
func (fm *FaultManager) StoreKeyShare(ownerID string, share ShamirShare)
func (fm *FaultManager) BroadcastMyShareFor(ownerID string) // → OnKeyShareNeeded
func (fm *FaultManager) AddReconstructionShare(ownerID string, share ShamirShare)
func (fm *FaultManager) TryReconstructKey(ownerID string) (*SRAKey, bool)

func (n *Node) BroadcastTimeoutVote(ctx, handNum, timeoutPeerID) error
func (n *Node) BroadcastAction(ctx, handNum, action, seq) error
func SendDirect(ctx, host, peer.ID, frame) error
func PeerIDFromString(s string) (peer.ID, error)

func Peel(key *SRAKey, ciphertext *big.Int, cardIndex int, playerID string, sessionID []byte) (*PartialDecryption, error)
func (s *DealSession) HandlePeel(msg *PeelMessage) (*PeelMessage, error)
func RemainingShowdownIDs(gs *game.GameState) []string
func ApplyTimeoutFold(gs *game.GameState, peerID string) (game.Action, error)
```

`OnPlayerFolded` is already set to `forceFold`. `TimeoutVote` proto has no yes/no bit — publishing the message **is** a yes. `HandleTimeoutVote(..., true)`.

`RegisterProtocolHandler` today only unmarshals `PARTIAL_DECRYPT`. Extend it. Gossip `KEY_SHARE` (contribute phase) goes through `dispatch` like other table messages — add a `case MsgType_KEY_SHARE`.

---

## Design

### 1. Minimum seats (live P2P only)

```go
const minP2PSeats = 3
```

In `runHost` / `runJoin` / `runP2PMode` **before** `NewNode`: if `cfg.Game.MaxSeats < 3` → `fmt.Errorf("runP2PMode: need at least 3 seats for timeout recovery (got %d)", ...)`.

Do **not** change `Config.Validate` (local mode may still be 2). Help text: `Number of seats (3-9, ...)`. README examples: 3-player host, not 2.

`--seats 2` must fail fast, not start a lobby.

### 2. Proto: `KEY_SHARE`

```protobuf
enum MsgType {
    // ... existing 0–12 unchanged ...
    KEY_SHARE = 13;
}

message KeyShare {
    string table_id = 1;
    int64  hand_num = 2;
    string owner_id = 3;   // whose d this share is of
    int32  index = 4;      // Shamir x (1..n)
    bytes  value = 5;      // big.Int.Bytes() of the share
}
```

No `holder_id` required if `index` maps to `SeatOrder()[index-1]`. Regenerating `messages.pb.go` is in scope (same `protoc` command the repo already used). Do not renumber existing fields.

### 3. Share distribution (crypto, once per table)

After `KeyringFromLobby` succeeds, **before** `StartShuffle`:

```
shares, thresh, err := fault.SplitAndDistribute(sraKey, kr.Len())
fm.cfg.ShamirThreshold = thresh   // or SetThreshold
for i, id := range kr.SeatOrder() {
    if id == localID {
        fm.StoreKeyShare(localID, shares[i]) // own share of own d; unused after we die
        continue
    }
    SendDirectKeyShare(ctx, id, KeyShare{OwnerId: localID, Index: shares[i].Index, Value: ...})
}
```

Receivers: `OnKeyShare` if `owner_id != local` and this is **distribution** (owner still alive) → `StoreKeyShare(owner_id, share)`.

Cap: ignore shares with wrong `hand_num` after we mix table-level shares (v1: `hand_num = 0` or `1` for the table; **d does not rotate** between hands, so distribute **once**, not every hand). Next-hand `startNextHand` must **not** re-split (would desync who holds what). Document that.

If any seat never receives a share, reconstruction of that owner later fails. v1: best-effort send with a few retries (same style as `BroadcastJoin`); do not block the shuffle for a full 2 minutes. Log `[crypto] key share to %s: %v`.

`--no-crypto`: skip this entire step (`sraKey == nil`).

### 4. Timeout votes + broadcast fold

Set **before** `node.Start()`:

```go
node.OnTimeoutVote = func(msg *network.TimeoutVote) {
    st, _ := fm.HandleTimeoutVote(msg.TimeoutPlayerId, msg.VotingPlayerId, true)
    _ = st
}
fm.OnTimeoutVoteNeeded = func(target string) {
    _ = node.BroadcastTimeoutVote(ctx, int64(currentHandNum), target)
}
```

`currentHandNum` must be a pointer / atomic the heartbeat sender also reads (today the sender closes over `handNum := 1` — **fix that** in this phase).

`go fm.Run(ctx)` next to `HeartbeatSender`.

**`forceFold`** (replace the body):

1. Lock `machineMu`.
2. If already folded / sitting out / nil machine → unlock, return.
3. `ApplyAction(fold)` locally; bump sequencer the same way as `applyAndBroadcast`.
4. Unlock.
5. `BroadcastAction` that fold.
6. `kickCryptoAdvance()`.
7. If crypto: `beginRecovery(targetID)` (step 5).

Remote replicas apply the fold via the existing `OnPlayerAction` sequencer. They **also** run `beginRecovery` when the vote confirms locally (`OnConfirmed` fires on every replica that recorded 2/3 — each replica’s `TimeoutManager` is independent). So every survivor both folds and starts contributing shares. Do not recover twice for the same id (idempotent `MarkGone`).

`--no-crypto`: steps 1–6 only.

### 5. Reconstruct + delegated peels

`beginRecovery(missingID)`:

1. `fm.BroadcastMyShareFor(missingID)` → `OnKeyShareNeeded` → `BroadcastKeyShare` (gossip).
2. Incoming contribute shares: `fm.AddReconstructionShare`.
3. Loop `TryReconstructKey` (or signal when `CanReconstruct`). Timeout 30s → log and give up (then street wait may still 2-minute abort — acceptable).
4. `liveHand.MarkGone(missingID, reconstructedKey)`.
5. `msgs := liveHand.TryDelegatedPeels()` → `SendPeel` each.

**Designated survivor:** `SeatOrder()` minus `gone`, first remaining id. Every replica computes the same. Only that replica `PeelOnBehalf`; others wait for `HandlePeel`.

```go
// DealSession.PeelOnBehalf publishes a peel as playerID using key (reconstructed d).
// Error if playerID == LocalID (use the normal local path).
// Error if ExpectedPeeler() != playerID.
// PlayerID on the PeelMessage is playerID, not LocalID.
func (s *DealSession) PeelOnBehalf(playerID string, key *SRAKey) (*PeelMessage, error)

func (s *DealSession) ExpectedPeeler() string // peelers[nextPeel], "" if idle/done
```

Implementation: same as `executeLocalLocked` but `Peel(key, ..., playerID, ...)` and **do not** hit the “local peel must be produced locally” path (that path is `HandlePeel` inbound). Apply the message locally via `applyIncomingLocked` like `executeLocalLocked` does.

`TryDelegatedPeels` on `CryptoHand`: while deal is in progress and `ExpectedPeeler()` is gone and we are designated, `PeelOnBehalf` + drain `Outbound()`. Call it from `HandlePeel` after inbound apply **and** from `beginRecovery` (the waiter may be blocked in `WaitStreet`; inbound Alice-peel from the designated peer unblocks it).

If **we** are designated and WaitStreet is already blocking: `beginRecovery` must `TryDelegatedPeels` + send **without** taking `machineMu` for the wait. Network `OnPartialDecrypt` already does not take `machineMu`. `beginRecovery` should take `cryptoMu` only.

### 6. Do not hold `machineMu` across Wait*

Today `kickCryptoAdvance` locks `machineMu` for the whole `AdvanceCrypto`, including `WaitStreet` (up to 2 minutes). `forceFold` needs that lock → **deadlock**.

Change `AdvanceCrypto` (or its caller) so that:

- `NeedsStreet` / `StartStreet` / `ApplyStreet` / `ApplyHoleReveal` run under `machineMu`
- `WaitStreet` / `WaitReveal` run **unlocked**
- After wait, re-lock to apply cards

Same for any wait inside `dealCryptoHand` (that path is pre-TUI; `forceFold` is not running yet). If someone dies during `WaitHoles`, `fm.Run` is already up (start it **before** `dealCryptoHand`, not after TUI). Move `fm.Run` + heartbeat sender to just after lobby fill / keyring, **before** shuffle.

Mid-shuffle: `WaitShuffle` still 2-minute abort. Do **not** `MarkGone` during shuffle. If a vote confirms mid-shuffle, still abort the hand (`ErrorMsg`: `shuffle aborted: peer X timed out`). Do not try to skip a shuffle seat.

### 7. Hole-deal disconnect (after shuffle, before StartHandCrypto)

In scope. Shuffle is `Done`; remaining hole peels are ordinary peels. `beginRecovery` + `PeelOnBehalf` unblocks `WaitHoles`. Then `StartHandCrypto` + `forceFold` so they never act. Empty opponent holes are already legal.

If the missing player was the **recipient** of a hole job, others still publish n−1 peels; nobody `FinishHole`s for them. That is fine — they will be folded.

### 8. README / help

Known limitations:

- Minimum 3 seats in multiplayer.
- Disconnect: no rejoin; folded after timeout vote; remaining reconstruct `d` and finish **peels**.
- Disconnect **during shuffle**: hand aborts.
- LAN only; no live ETH.

Remove “Timeout Detection” as a lie if it was still a ✅ without `fm.Run`; after this phase it can stay ✅ with the shuffle-abort caveat.

---

## Tests

Tiny / `smallPrime` for FSM. Production 2048-bit only where a row says so.

### A. `DealSession` — `deal_session_test.go`

| Test | Assert |
|---|---|
| `TestDealSession_PeelOnBehalf_ProducesMissingPlayerID` | 3-player tiny public job; Carol is next; Alice calls `PeelOnBehalf("carol", carolKey)` (test may use Carol’s real key as stand-in for reconstructed); message `PlayerID=="carol"`; Bob `HandlePeel` accepts; job advances |
| `TestDealSession_PeelOnBehalf_RejectsLocalID` | `PeelOnBehalf(localID, localKey)` errors |
| `TestDealSession_PeelOnBehalf_WrongExpected` | Expected is Alice; `PeelOnBehalf("bob", ...)` errors |

### B. Fake-net liveness — `crypto_hand_test.go` or `liveness_test.go`

Helper: 3 keyrings, 3 `CryptoHand`s, fake peel/share bus. **No libp2p.**

| Test | Assert |
|---|---|
| `TestLiveness_FakeNet_3Players_TimeoutFold_NoCryptoStyle` | Optional: three `game.Machine` plaintext; kill one on their turn; remaining apply fold+broadcast analogue; `Phase` not stuck on that actor. Can be a machine-only test without crypto. |
| `TestLiveness_FakeNet_3Players_KillAfterHoles_FlopCompletes` | **Production 52-card.** Shuffle + holes complete. Stop replica 2 (no more peels). Remaining: reconstruct from test harness shares (or the fake share bus); `MarkGone`; designated `PeelOnBehalf`; flop `ApplyStreet`; two boards **equal**; replica 2’s machine is unused. Timeout ~4 minutes. |
| `TestLiveness_ShareUnicastNotOnShuffleBus` | Distribution helper never puts `ShamirShare` on the shuffle/peel bus used for `SHUFFLE_STEP`. Cheap invariant test. |

Do **not** require a fake-net “kill during shuffle then recover” test. Add `TestLiveness_ShuffleTimeoutAborts` only if cheap (WaitShuffle ctx timeout → error, no MarkGone).

### C. CLI / config

| Test | Assert |
|---|---|
| Host `--seats 2` | `runHost` / extracted parse+validate returns error containing `3` |
| `--seats 3` | accepted |

If parse stays in `package main`, a tiny `minP2PSeats` check test may live next to other main tests **or** be manual. Prefer extracting `func requireP2PSeats(n int) error`.

### D. Regression

```
go test ./internal/crypto ./internal/fault ./internal/network ./internal/game -count=1
go test ./...
```

Phase 1–5 tests still pass, including 2-player `TestCryptoHand_FakeNet_*` and `TestKeyring` with 2 seats. Live P2P min 3 ≠ library min 2.

---

## Implementation order (do this, then stop)

1. Proto `KEY_SHARE` + regenerate pb.go + codec round-trip test.
2. `DealSession.ExpectedPeeler` / `PeelOnBehalf` + tests (A).
3. Unlock `machineMu` around `Wait*` in `kickCryptoAdvance` / `AdvanceCrypto`. **Do this before wiring votes** or you will deadlock on the first timeout during a street.
4. Min 3 seats in `runP2PMode` + help/README.
5. `fm.Run`, `OnTimeoutVote`, broadcast fold, live `handNum` for heartbeats. `--no-crypto` 3-player: kill a window on their turn, other two continue (manual). Fake-net fold test if easy.
6. Unicast distribute + gossip contribute + `MarkGone` + `TryDelegatedPeels`.
7. Fake-net 3-player kill-after-holes (B).
8. Start `fm.Run` before shuffle so hole-deal timeouts can recover.
9. Mid-shuffle: on confirmed timeout, fail `WaitShuffle` / surface `ErrorMsg` (do not MarkGone).
10. `go test ./...`. Manual 3-terminal: one `Ctrl-C` on a betting turn → fold → flop still appears for the other two.
11. Stop. No reconnect, no shuffle restart, no chain.

If step 7 is not green, **do not** call the README timeout line “done.” Fold-only (`--no-crypto`) can still ship as a subset, but crypto recovery is the phase.

---

## Error style

Prefix: `PeelOnBehalf: ...`, `runP2PMode: need at least 3 seats...`, `CryptoHand.MarkGone: ...`. Match Phases 1–5. Do not rewrite empty `fmt.Errorf("")` leftovers in `fault` except lines you already touch (`ApplyTimeoutFold`).

---

## Explicit non-goals (push back if asked mid-phase)

- Recovering a **mid-shuffle** drop (permutation is gone). Abort the hand.
- Rejoin / `GAME_STATE_SYNC`.
- 2-player P2P (Shamir cannot reconstruct).
- Putting reconstructed `d` on `Keyring`.
- Gossip of shares **before** the owner is voted out.
- Changing 2/3 vote math or Shamir t.
- ETH slash on withholding (record slash if `RecordKeyWithholding` is one line; do not submit on-chain).
- TUI redesign.
- Optimizing 2048-bit; 3-player demo **will** be slower. Status logs already exist (`[crypto] shuffle ...`).
- libp2p in unit tests.

---

## Hang / deadlock diagnosis

| Symptom | Likely cause |
|---|---|
| Timeout never folds | `fm.Run` not started, or `OnTimeoutVote` nil at `Start` |
| Folded on one replica, other still waiting for their action | `forceFold` did not `BroadcastAction` |
| Folded but flop never comes | Reconstruct failed, or `PeelOnBehalf` never ran, or designated id differs per replica |
| `HandlePeel: local peel must be produced locally` | Delegated message used `LocalID` instead of missing id |
| Deadlock, 100% idle | `machineMu` still held in `WaitStreet` while `forceFold` waits on it |
| Shuffle hangs then error | Expected — mid-shuffle abort |
| `--seats 2` still starts | min check after `NewNode` / only in help text |

---

## What the examiner should see

3-player LAN table:

> If a peer goes silent after the shuffle, the others vote them out, fold that seat, reconstruct `d` from Shamir shares, and finish community peels. The disconnected player cannot rejoin. A disconnect **during** the shuffle aborts the hand.

`--no-crypto`: fold only; cards were never hidden.

---

## Review checklist (before calling Phase 6 done)

- [ ] P2P `MaxSeats < 3` errors; help says 3–9
- [ ] `fm.Run` started before shuffle; heartbeat `handNum` follows the live hand
- [ ] `OnTimeoutVote` set before `node.Start()`
- [ ] Confirmed timeout → local fold **and** `BroadcastAction`
- [ ] Crypto: shares unicast at table start; contribute gossip only after vote
- [ ] Reconstructed key never stored on `Keyring`
- [ ] `PeelOnBehalf` uses missing `PlayerID`; designated survivor is first remaining `SeatOrder()` id
- [ ] `machineMu` not held across `Wait*`
- [ ] Mid-shuffle timeout aborts; no delegated shuffle step
- [ ] No rejoin
- [ ] Fake-net 3-player kill-after-holes: remaining boards match
- [ ] Phase 5 2-player crypto tests still pass
- [ ] `go test ./...` green
- [ ] README matches (including shuffle-abort caveat)

---

## Time

About **one sequential implementation pass** if this spec is followed: timeout-fold glue is hours; Shamir + `PeelOnBehalf` + lock fix is the rest of the week. The slow test is 3-player 2048-bit shuffle + holes + delegated flop.

If the work grows into shuffle restart, reconnect, or chain slashing, stop and split — those are not Phase 6.
