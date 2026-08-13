# Cryptographic dealing — overview plan (issues 1–3)

This is the **index**, not the implementation spec. Issues 1–3 are one product change: stop dealing from a public shared seed in `runP2PMode`, and actually run SRA shuffle + partial decrypt across peers.

We will **not** implement from this file. For each phase we will later add a short plan doc, implement it, run tests, then move on.

Companion: [`ISSUES_AND_RECOMMENDATIONS.md`](./ISSUES_AND_RECOMMENDATIONS.md). Design: [`SYSTEM_DESIGN_OVERVIEW.md`](./SYSTEM_DESIGN_OVERVIEW.md).

---

## Goal (when all phases are done)

Default `poker host` / `poker join` (without `--no-crypto`):

1. Each peer holds **only its own** private exponent `d`. `e` is public in `JOIN_TABLE`.
2. Peers shuffle in seat order over gossip (`SHUFFLE_STEP`). Permutations never leave the shuffling peer.
3. Hole cards are dealt with `PARTIAL_DECRYPT`: everyone except the recipient peels; the recipient peels last **locally**. Other seats’ hole cards stay empty on this node until showdown.
4. Flop / turn / river are public peels (everyone peels; plaintext appears on every replica).
5. Showdown reveals remaining hole cards the same way (public peels of those deck indexes).
6. `--no-crypto` is the **old** shared-seed path, labeled as debug.

Acceptance demo: 2 players on LAN. A debug log shows big-integer ciphertexts on the wire, not ranks/suits. Player A’s TUI does not show player B’s hole cards during the hand.

---

## How we will work

| Step | What |
|---|---|
| 0 | This overview (you are here). |
| For each phase | Write `plans/phase-N-<name>.md` with files, APIs, tests, and a “do not touch” list. |
| Then | Implement **only that phase**. |
| Then | Run that phase’s tests **plus** `go test ./...` so nothing old breaks. |
| Then | Stop. Next phase is a new conversation / new plan doc. |

**Rule:** a phase is done when its tests pass and the live P2P path still behaves as that phase promises (usually: unchanged until Phase 5). Do not “just also wire main.go” in Phases 1–4.

Per-phase docs will live in `plans/` (created starting with Phase 1). This file stays the map.

---

## Time (AI vs you)

| Who | Realistic |
|---|---|
| AI implementing all 5 phases, sequentially, with tests | About **one long day** if Phase 5’s 2-player path is cooperative. The risk is distributed hangs in Phase 5, not the math. |
| You reading each phase after it lands | **3–4 days** is right: ~half a day per phase, extra on Phase 5. Do not try to understand all five at once. |

If a phase’s plan looks bigger than half a day of AI work, we split it before coding — we do not grow Phase 5 into a second project.

---

## What already exists — reuse, do not rewrite

| Piece | Where | Status |
|---|---|---|
| SRA encrypt/decrypt, 2048-bit MODP | `internal/crypto/sra.go` | Real |
| Encrypt-then-permute + commitment | `internal/crypto/shuffle.go` | Real, **in-process** (`RunFullShuffle`) |
| Partial decrypt + ZK | `internal/crypto/deal.go`, `zkp.go` | Real, **needs every `d` in one process** |
| Local full-hand helper | `internal/crypto/crypto_game.go`, `network/coordinator.go` | Local simulation; keep as **test oracle** |
| `JOIN_TABLE.sra_pub_key_e` | proto + `Lobby.SeatInfo.SRAKeyE` | Already stored |
| `BroadcastShuffleStep` / `BroadcastPartialDecrypt` / `SendDirectPartialDecrypt` | `internal/network/node.go` | Senders exist; **no caller in `cmd/poker`** |
| `OnShuffleStep` / `OnPartialDecrypt` | `node.go` | Callbacks exist; **unset in `runP2PMode`** |
| Direct streams `/poker/1.0.0` | `internal/network/protocol.go` | Real |
| `StartHandCrypto` | `internal/game/machine.go` | Skips local shuffle; **currently requires every seat to already have hole cards** |
| TUI hides non-local hole cards | `internal/tui/player_panel.go` | Already `IsLocalPlayer \|\| IsWinner` |
| Shared-seed deal | `cmd/poker/main.go` | **Live default today** |

Do **not** add new proto fields until this loop actually runs. `SHUFFLE_COMMIT` can stay unused; `SHUFFLE_STEP` already carries the commitment.

---

## Out of scope (all phases)

- Timeout votes, Shamir reconstruct, `FaultManager.Run` (issue 6) — unless a later Phase 5 bug **deadlocks** a 2-player demo, in which case we add a tiny “abort hand on shuffle timeout,” not full Shamir.
- `ApplyAction` mutex (issue 7) — optional one-line fix only if we are already in `main.go` in Phase 5.
- Ethereum / Hardhat (issue 5).
- DHT, reconnect, mid-hand sync, 9-player WAN.
- Replacing SRA with a modern shuffle argument.
- Deleting the local `CryptoGame` simulation; tests still need it.

---

## Phase map

```
Phase 1  Keyring          (library)     live path unchanged
    │
Phase 2  Shuffle FSM      (library)     live path unchanged
    │
Phase 3  Deal peels       (library)     live path unchanged
    │
Phase 4  Machine streets  (game pkg)    plaintext P2P still default
    │
Phase 5  Wire runP2PMode  (product)     crypto becomes default
```

Phases 1–3 are crypto/protocol libraries with in-process tests (fake message bus, no libp2p). Phase 4 is the engine API so streets and showdown do not read a local deck. Phase 5 is the only change to what `poker host` / `poker join` do.

---

## Phase 1 — Keyring (public `e`, private `d` stays here)

**Why first.** Every later phase needs “I have my key; I have everyone else’s `e` only.” Today `NewCryptoGame` generates **all** `(e, d)` on one machine. `BroadcastJoin` also nil-derefs `sraKey` when `--no-crypto` is set.

**Changes (expected)**

- `SRAKey` helper: build a **public-only** key from `(p, e)` with `d == nil`. Encrypt works; decrypt must fail.
- A small `Keyring` (name TBD): local full key + map `peerID → public e`, built from lobby seats + our key. Never put another peer’s `d` in it.
- Lobby helper: `PublicExponents()` in canonical seat order; optional “all seats have non-empty `e`” check for crypto mode.
- `BroadcastJoin`: if `sraKey == nil`, send empty `sra_pub_key_e` instead of panicking.
- Keep `NewCryptoGame` / `RunShuffle` as the **local oracle** for existing tests.

**Do not**

- Change how `StartHand` deals.
- Call shuffle/deal from `runP2PMode`.
- Invent new messages.

**Tests**

- Public-only key encrypts; decrypt errors.
- Keyring refuses to expose any `d` except local.
- Lobby stores `e` from `JoinTable` (already does — add an assertion test).
- Existing `internal/crypto` tests still pass.

**Done when.** Unit tests green. Live multiplayer is still shared-seed. `--no-crypto` no longer crashes on join.

**You should understand.** Why `e` can be public and `d` cannot; why the local simulation is still allowed in tests.

---

## Phase 2 — Distributed shuffle (in-process, not live)

**Why.** `ExecuteStep` + `VerifyStep` already exist. `RunFullShuffle` runs every step in one process. We need a **turn-taking FSM**: player `k` encrypts+permutes, publishes output deck + commitment, others verify and wait.

**Changes (expected)**

- New type, e.g. `internal/crypto/shuffle_session.go` or `internal/network/shuffle_coord.go`:
  - Input: seat order, keyring, session id, plaintext deck.
  - If it is our turn: `ExecuteStep` with **local** key, broadcast **output deck + commitment only** (permutation stays in the step object locally, never on the wire — `BroadcastShuffleStep` already omits it).
  - If not our turn: accept only the current seat’s `SHUFFLE_STEP`, `VerifyStep`, adopt `OutputDeck` as next input.
  - Reject: wrong player, wrong hand, bad commitment, duplicate step.
- A **fake bus** in tests (Go channels), not libp2p. Three goroutines, three keyrings, same final encrypted deck on all replicas.
- Sequencer: shuffle steps are ordered by **seat index**, not GossipSub arrival. Same idea as `actionSequencer`.

**Do not**

- Hook `OnShuffleStep` in `main.go`.
- Deal any cards yet.
- Send `Permutation` on the wire (it is not in the proto; do not add it).

**Tests**

- 2- and 3-player fake-net: all replicas finish with identical ciphertexts.
- A replica that only has public `e`s cannot recover plaintext order.
- Wrong-seat step rejected; tampered commitment rejected.
- Existing `RunFullShuffle` tests still pass (oracle).

**Done when.** Those tests pass. `poker host` still shared-seed.

**You should understand.** Why N sequential shuffles beat a single shuffler; why commitment does not prove the permutation is random (it only binds the published deck); why gossip needs a turn index.

---

## Phase 3 — Distributed peels (hole, community, showdown)

**Why.** `DealProtocol` peels using **every** `d` in a slice. On the network, each peer can only peel with **its** `d`. Hole cards go to one recipient (direct stream in production; fake bus in this phase). Community and showdown peels are public.

**Changes (expected)**

- Split “apply my decrypt + ZK” from “run the whole deal with all keys”:
  - `Peel(localKey, ciphertext, cardIndex, sessionID) → PartialDecryption`
  - `VerifyAndApply(current, pd) → next ciphertext`
  - Recipient: after N−1 verified peels, decrypt locally, `FieldToCard`.
  - Community / showdown: N peels, last result is plaintext (same as `RevealCommunity` today).
- Deal order must match the engine: hole cards left of dealer, two rounds; then burn + flop 3, burn + turn, burn + river (same indexes `DealHoleCards` / `DealCommunityCards` already use).
- Showdown: for each remaining player, publicly peel their two hole-card indexes so every replica can fill `Player.HoleCards` and evaluate. (The issues doc does not name this; without it hidden cards make showdown impossible on other nodes.)

**Do not**

- Change `machine.go` street dealing yet (that is Phase 4).
- Wire streams in `main.go`.
- Require Shamir.

**Tests**

- 3 in-process players: after hole deal, only the recipient’s keyring can decode that seat’s two cards; others still have ciphertext.
- Community peel: all three get the same public cards.
- Bad ZK / wrong result rejected (`SubstitutePartialDecryption` already exists).
- Showdown peel fills the other seats; then `EvaluateBest7` agrees on all replicas.
- Old `DealHoleCards` / `DealCommunityCards` tests still pass.

**Done when.** Privacy + community tests pass without libp2p. Live path still shared-seed.

**You should understand.** Commutative peeling; why hole peels are unicast-ish and community peels are broadcast; why showdown is just “community peel of private indexes.”

---

## Phase 4 — Game machine: no local deck in crypto mode

**Why.** Even with perfect peels, `ApplyAction` → `endBettingRound` → `dealFlop` still pulls from `gs.Deck`. `StartHandCrypto` also refuses to start unless **every** seat already has hole cards, which would force us to leak them.

**Changes (expected)**

- Crypto mode = `gs.Deck == nil` (already set by `DealToEngine`).
- `StartHandCrypto`: require **only the local player** to have hole cards, **or** (cleaner for a replicated machine) require nothing from other seats; do not error on empty opponent holes. Post blinds, go to preflop. Document: opponents’ cards are filled at showdown.
- When `Deck == nil`, `endBettingRound` must **not** call `dealFlop` / `dealTurn` / `dealRiver`. Instead enter a waiting phase or return a clear “need street” signal, e.g. `PhaseAwaitingStreet` or `ErrNeedStreet`.
- New API, e.g. `ApplyStreet(cards []Card)`: flop expects 3, turn/river expect 1; then `startNewBettingRound`. All replicas call this with the **same** peeled cards.
- Showdown: if remaining players still have empty holes, do not evaluate; wait for `ApplyHoleReveal(playerID, [2]Card)` (or equivalent) until all remaining seats are filled, then existing showdown logic.
- **Plaintext path unchanged:** `StartHand` still shuffles and deals from `rng`.

**Do not**

- Put networking inside `machine.go`.
- Change pot / eval math.
- Switch `runP2PMode` to `StartHandCrypto` yet.

**Tests** (all in `internal/game`)

- Existing `game_test.go` / community-card tests still pass (plaintext).
- Crypto: `StartHandCrypto` with only seat 0’s holes filled → preflop.
- Check through to end of preflop → machine waits; `ApplyStreet` 3 cards → flop; same for turn/river.
- Showdown waits until hole reveals; then payouts match a control where cards were known from the start.
- Calling `dealFlop` internally on a nil deck must not panic.

**Done when.** Engine can play a full crypto hand **if a test injects cards**. No P2P yet.

**You should understand.** The machine is a reducer: cards are **inputs**, not something it samples. Shared-seed was cheating by sampling the same RNG on every node.

---

## Phase 5 — Wire into `runP2PMode` (the product)

**Why.** Everything above is unused until this phase. This is issues 1–3 as the examiner sees them.

**Changes (expected)**

- After lobby is full:
  - `--no-crypto`: keep today’s shared-seed `StartHand` (and document it as debug).
  - default: Keyring from lobby `e`s + local key → shuffle session over real `OnShuffleStep` / `BroadcastShuffleStep` → hole peels (`SendDirectPartialDecrypt` to recipient; gossip is acceptable for v1 if direct is flaky, but prefer direct as designed) → fill **local** holes only → `StartHandCrypto`.
- During play: when machine needs a street, run community peels, `ApplyStreet`.
- At showdown: public-peel remaining holes, `ApplyHoleReveal`, existing settle / next hand.
- Next hand: new shuffle (do not reuse the encrypted deck). Mix `handNum` into session id if needed.
- Wire `OnShuffleStep` and `OnPartialDecrypt` **before** `node.Start()` (same rule as other callbacks).
- README + CLI help: crypto dealing is default; `--no-crypto` = all cards visible, sync testing only. One honest status line at the top.
- Optional debug log: print ciphertext byte length / prefix on shuffle and peels (not ranks).

**Do not**

- Add DHT, timeout-vote Shamir, or chain RPC.
- Target 9 players. **2 is the milestone; 3 is nice.**
- New proto fields.

**Tests**

- `go test ./...` still green.
- New integration test with a **fake bus** driving two full `p2p`-style loops (shuffle → hole → one betting round → flop peel → fold/showdown) **without** requiring two real binaries. If we already have in-process node tests, extend those.
- Manual: two terminals, `host` + `join --peer …`, play one hand. Confirm opponent holes hidden; community appears together.

**Done when.** The one-sentence claim in the issues doc is true:

> A LAN mental-poker Hold’em table: peers jointly shuffle under commutative encryption, deal with partial decrypts and ZK checks, and agree on pots locally.

**You should understand.** How `main.go` sequences lobby → shuffle → deal → machine → streets → showdown; where `--no-crypto` branches; what a hang looks like (waiting forever for a shuffle step).

---

## Risk notes (read before Phase 5)

1. **Shuffle steps on GossipSub are unordered.** Phase 2’s sequencer is mandatory, not optional polish.
2. **`StartHandCrypto` today leaks by construction** (all holes required). Phase 4 must land before Phase 5 or we will “wire crypto” and still show every card.
3. **2048-bit modexp × 52 cards × N players** is slow (seconds). Fine for a 2-player demo; do not “optimize SRA” in this project.
4. **Disconnect mid-peel deadlocks.** Out of scope unless it blocks the demo; then abort the hand, do not build Shamir in Phase 5.
5. **`BroadcastJoin` nil `sraKey`.** Fix in Phase 1 so `--no-crypto` remains a valid fallback after Phase 5.

---

## Suggested later filenames

Created when we start that phase, not now:

```
plans/phase-1-keyring.md
plans/phase-2-shuffle.md
plans/phase-3-deal-peels.md
plans/phase-4-machine-streets.md
plans/phase-5-wire-p2p.md
```

Each of those should be short: files to touch, function signatures, test names, explicit “do not touch `cmd/poker`” (Phases 1–4).

---

## Order of next action

Next message should be: **write `plans/phase-1-keyring.md` only** — still no code. After you accept that phase plan, we implement Phase 1 and run tests.
