# Read Guide — Decentralized Poker Engine

A new-joiner map of **what to read, in what order**, so you can go from “I just cloned this” to “I can explain a four-player crypto hand hop by hop.”

[`HOW_IT_WORKS.md`](./HOW_IT_WORKS.md) is the teaching narrative. This file is the **reading order** for the rest of the repository. Do not try to absorb every source file in one sitting. Follow the numbered list once if you want a complete pass; otherwise work **one phase at a time**.

A detailed walkthrough for each phase lives in its own file:

| Phase | File | One-line goal |
|---|---|---|
| 1 | [`PHASE_1.md`](./PHASE_1.md) | What this is, how to run it, where the process starts |
| 2 | [`PHASE_2.md`](./PHASE_2.md) | Pure Hold’em engine + local TUI (no network) |
| 3 | [`PHASE_3.md`](./PHASE_3.md) | Identity, mesh, lobby, sequenced public actions |
| 4 | [`PHASE_4.md`](./PHASE_4.md) | Mental poker: SRA shuffle, peels, `CryptoHand` |
| 5 | [`PHASE_5.md`](./PHASE_5.md) | Disconnects, escrow, tests, honest gaps |

Those five files are **onboarding chapters**. They are not the historical implementation specs in `plans/` (those were written while crypto and liveness were being wired).

---

## How to use this guide

1. Skim the [skip list](#files-you-should-not-study) so you do not waste time on generated or lock files.
2. Read **Phase 1** in full ([`PHASE_1.md`](./PHASE_1.md)) before opening packages. You should be able to build and run a local game after it.
3. Then do Phases 2 → 5 in order. Each phase lists files, a matching slice of `HOW_IT_WORKS.md`, and an exit check.
4. Tests sit **after** the code they cover in the same phase. Read a test when you want a worked example, not before the types make sense.
5. The [master reading order](#master-reading-order) is the same files flattened into one numbered list. Use it if you prefer a single checklist.

**Architectural rule to keep in your head the whole time:** `internal/game` never imports `internal/network`. Networking produces authenticated, ordered inputs. The engine reduces them. Mixing those layers is how “the host accidentally becomes the server” happens.

**Stale-doc warning:** [`ISSUES_AND_RECOMMENDATIONS.md`](./ISSUES_AND_RECOMMENDATIONS.md) and parts of [`CRYPTO_DEAL_PLAN.md`](./CRYPTO_DEAL_PLAN.md) describe the world *before* live SRA dealing was wired. Prefer [`README.md`](./README.md), [`HOW_IT_WORKS.md`](./HOW_IT_WORKS.md), and [`SYSTEM_DESIGN_OVERVIEW.md`](./SYSTEM_DESIGN_OVERVIEW.md) for “what runs today.” Read the older notes in Phase 5 as history, not as current status.

---

## Files you should not study

| File | Why skip |
|---|---|
| `go.sum` | Module checksums. Not architecture. |
| `package-lock.json` | Hardhat lockfile. |
| `internal/network/messages.pb.go` | Generated from `messages.proto`. Read the `.proto`, not this. |
| `internal/chain/abi/PokerEscrow.go` | Generated Go bindings. Read `contracts/PokerEscrow.sol` instead. |
| `extra.txt` | One `protoc` command. Useful only if you change the proto. |
| `.gitignore` | Repo hygiene, not the engine. |

---

## Master reading order

Read top to bottom. Phase boundaries are marked. Companion `HOW_IT_WORKS.md` sections are in the [five phases](#five-phases) below.

### Phase 1 — Orientation

1. [`README.md`](./README.md) — product claim, how to build, host/join vs local, what works today
2. [`HOW_IT_WORKS.md`](./HOW_IT_WORKS.md) §§1–9, 23, 25–26 — networking primer, what “decentralized” means here, repo map, glossary
3. [`SYSTEM_DESIGN_OVERVIEW.md`](./SYSTEM_DESIGN_OVERVIEW.md) §§1–2 — goals, non-goals, mesh diagram, how a table forms
4. [`go.mod`](./go.mod) — Go 1.25, libp2p, GossipSub, Bubble Tea, protobuf (do not read `go.sum`)
5. [`config/config.go`](./config/config.go) — YAML shape: network, game, fault, chain; identity seed vs Ethereum keygen
6. [`config/loader.go`](./config/loader.go) — load/default/validate; P2P seats 3–9 vs local 2–9
7. [`cmd/poker/main.go`](./cmd/poker/main.go) — CLI (`init`, `keygen`, `host`, `join`, default local); `runLocalMode` first, then skim `runP2PMode` as a map of later phases
8. [`cmd/poker/main_test.go`](./cmd/poker/main_test.go) — CLI and mode-flag coverage

### Phase 2 — Pure Hold’em engine

9. [`internal/game/deck.go`](./internal/game/deck.go) — 52 cards, id `0..51`, Fisher–Yates (plaintext only)
10. [`internal/game/player.go`](./internal/game/player.go) — stack, holes, status, `PlaceBet`
11. [`internal/game/state.go`](./internal/game/state.go) — phases, action types, `GameState`, whose turn
12. [`internal/game/pot.go`](./internal/game/pot.go) — main + side pots from contribution layers
13. [`internal/game/hand_eval.go`](./internal/game/hand_eval.go) — best 5 of 7, rank then kickers
14. [`internal/game/machine.go`](./internal/game/machine.go) — `StartHand` / `StartHandCrypto`, `ApplyAction`, `ApplyStreet`, `ApplyHoleReveal`
15. [`internal/game/game_test.go`](./internal/game/game_test.go) — plaintext rules: blinds, side pots, showdown
16. [`internal/game/machine_crypto_test.go`](./internal/game/machine_crypto_test.go) — engine with `Deck == nil`; streets and reveals are inputs
17. [`internal/tui/styles.go`](./internal/tui/styles.go) — colors / layout constants
18. [`internal/tui/card_view.go`](./internal/tui/card_view.go) — how a card is drawn
19. [`internal/tui/player_panel.go`](./internal/tui/player_panel.go) — **hides opponent holes** unless local or winner
20. [`internal/tui/table_view.go`](./internal/tui/table_view.go) — board, pot, seats
21. [`internal/tui/log_view.go`](./internal/tui/log_view.go) — action / network log
22. [`internal/tui/bet_input.go`](./internal/tui/bet_input.go) — check / call / raise / fold / all-in widget
23. [`internal/tui/model.go`](./internal/tui/model.go) — Bubble Tea model; actions leave via a callback, they do not mutate the engine here
24. [`internal/tui/tui_test.go`](./internal/tui/tui_test.go) — UI rendering and input

### Phase 3 — Mesh, lobby, sequenced actions

25. [`internal/network/messages.proto`](./internal/network/messages.proto) — every `MsgType`, `Envelope`, payload shapes (skip `messages.pb.go`)
26. [`internal/network/codec.go`](./internal/network/codec.go) — protobuf marshal/unmarshal, Ed25519 sign/verify of envelopes
27. [`internal/network/host.go`](./internal/network/host.go) — libp2p host: identity, listen multiaddr, Noise, TCP, no relays
28. [`internal/network/discovery.go`](./internal/network/discovery.go) — mDNS `p2p-poker-v1`
29. [`internal/network/gossip.go`](./internal/network/gossip.go) — GossipSub topics `poker/table/<id>` and `poker/heartbeat/<id>`
30. [`internal/network/protocol.go`](./internal/network/protocol.go) — direct streams `/poker/1.0.0`, length-prefixed frames
31. [`internal/network/lobby.go`](./internal/network/lobby.go) — seats, canonical order, ready barrier, session nonce, `KeyringFromLobby`
32. [`internal/network/gamelog.go`](./internal/network/gamelog.go) — append-only envelopes; evidence, not consensus
33. [`internal/network/node.go`](./internal/network/node.go) — the player process: host + gossip + lobby + log + callbacks + broadcast helpers
34. [`internal/network/coordinator.go`](./internal/network/coordinator.go) — in-process crypto oracle helper (tests / local simulation, not the live loop)
35. [`internal/network/network_test.go`](./internal/network/network_test.go) — lobby, codec, gossip, node wiring
36. [`SYSTEMS_DESIGN_INTERVIEW.md`](./SYSTEMS_DESIGN_INTERVIEW.md) — same architecture, interview-shaped; read after the mesh files, not before

### Phase 4 — Mental poker

37. [`internal/crypto/params.go`](./internal/crypto/params.go) — shared MODP prime, card ↔ field-element encoding
38. [`internal/crypto/sra.go`](./internal/crypto/sra.go) — commutative encrypt/decrypt; public-only keys cannot decrypt
39. [`internal/crypto/commit.go`](./internal/crypto/commit.go) — shuffle commitments + Shamir split/reconstruct math
40. [`internal/crypto/zkp.go`](./internal/crypto/zkp.go) — proof that a peel used the claimed `d`
41. [`internal/crypto/keyring.go`](./internal/crypto/keyring.go) — this node’s private `d` plus everyone else’s public `e` only
42. [`internal/crypto/shuffle.go`](./internal/crypto/shuffle.go) — encrypt-then-permute **in one process** (library + oracle)
43. [`internal/crypto/shuffle_session.go`](./internal/crypto/shuffle_session.go) — distributed shuffle FSM (one step per seat over gossip)
44. [`internal/crypto/deal.go`](./internal/crypto/deal.go) — partial decrypt / peel **in one process**
45. [`internal/crypto/deal_session.go`](./internal/crypto/deal_session.go) — distributed peel FSM (holes, streets, showdown)
46. [`internal/crypto/crypto_game.go`](./internal/crypto/crypto_game.go) — full-hand oracle with every `d` in one process; keep as a test reference
47. [`internal/crypto/keyring_test.go`](./internal/crypto/keyring_test.go)
48. [`internal/crypto/crypto_test.go`](./internal/crypto/crypto_test.go) — SRA, shuffle, deal, ZKP, `CryptoGame`
49. [`internal/crypto/shuffle_session_test.go`](./internal/crypto/shuffle_session_test.go)
50. [`internal/crypto/deal_session_test.go`](./internal/crypto/deal_session_test.go)
51. [`internal/network/crypto_hand.go`](./internal/network/crypto_hand.go) — one replica’s shuffle + peels for one hand; feeds `game.Machine`
52. [`internal/network/crypto_hand_test.go`](./internal/network/crypto_hand_test.go)
53. [`cmd/poker/main.go`](./cmd/poker/main.go) again — now read `runP2PMode` / `startNextHand` in detail (Keyring, Shamir unicast, `CryptoHand`, `--no-crypto` debug path)
54. [`plans/phase-1-keyring.md`](./plans/phase-1-keyring.md) — historical spec: Keyring invariant
55. [`plans/phase-2-shuffle.md`](./plans/phase-2-shuffle.md) — historical spec: `ShuffleSession`
56. [`plans/phase-3-deal-peels.md`](./plans/phase-3-deal-peels.md) — historical spec: `DealSession`
57. [`plans/phase-4-machine-streets.md`](./plans/phase-4-machine-streets.md) — historical spec: `StartHandCrypto` / `ApplyStreet` / `ApplyHoleReveal`
58. [`plans/phase-5-wire-p2p.md`](./plans/phase-5-wire-p2p.md) — historical spec: live `runP2PMode` wiring

### Phase 5 — Liveness, settlement, tests, gaps

59. [`internal/fault/types.go`](./internal/fault/types.go) — votes, slash records, shared types
60. [`internal/fault/heartbeat.go`](./internal/fault/heartbeat.go) — who last spoke
61. [`internal/fault/timeout.go`](./internal/fault/timeout.go) — 2/3 timeout votes → force-fold
62. [`internal/fault/shamir.go`](./internal/fault/shamir.go) — store and reconstruct shares of `d`
63. [`internal/fault/slash.go`](./internal/fault/slash.go) — equivocation / bad-peel records
64. [`internal/fault/manager.go`](./internal/fault/manager.go) — composes the above; callbacks into the network loop
65. [`internal/fault/fault_test.go`](./internal/fault/fault_test.go)
66. [`internal/network/liveness.go`](./internal/network/liveness.go) — designated survivor, table-level share hand number
67. [`internal/network/fault_adaptor.go`](./internal/network/fault_adaptor.go) — glue from `FaultManager` to gossip / streams / `CryptoHand`
68. [`internal/network/liveness_test.go`](./internal/network/liveness_test.go)
69. [`plans/phase-6-liveness.md`](./plans/phase-6-liveness.md) — historical spec: timeout fold + Shamir peels
70. [`contracts/PokerEscrow.sol`](./contracts/PokerEscrow.sol) — buy-in, 2/3 settlement, challenge, slash (designed, real Solidity)
71. [`contracts/hardhat.config.js`](./contracts/hardhat.config.js)
72. [`contracts/test/PokerEscrow.test.js`](./contracts/test/PokerEscrow.test.js)
73. [`package.json`](./package.json) — Hardhat scripts only; the Go binary does not use Node
74. [`internal/chain/client.go`](./internal/chain/client.go) — RPC client **stub** (synthetic receipts)
75. [`internal/chain/escrow.go`](./internal/chain/escrow.go) — Go helpers that would call the contract; **not** on the live `host`/`join` path
76. [`internal/chain/chain_test.go`](./internal/chain/chain_test.go)
77. [`internal/integration/e2e_test.go`](./internal/integration/e2e_test.go) — multi-node happy path
78. [`internal/integration/adversarial_test.go`](./internal/integration/adversarial_test.go) — cheats, silence, mixed `--no-crypto`
79. [`CRYPTO_DEAL_PLAN.md`](./CRYPTO_DEAL_PLAN.md) — index of how crypto was planned; treat as history
80. [`ISSUES_AND_RECOMMENDATIONS.md`](./ISSUES_AND_RECOMMENDATIONS.md) — original gap analysis; several “must fix” items are now done — check against README before quoting
81. [`HOW_IT_WORKS.md`](./HOW_IT_WORKS.md) §§20–22, 24 — wiring diagram, full four-player hand, cheat/disconnect cases, misconceptions

Then you are done. Re-read §21 of `HOW_IT_WORKS.md` once more; it should now name files you have actually opened.

---

## Five phases

Each phase below is one onboarding chapter. [`PHASE_1.md`](./PHASE_1.md) expands the Phase 1 outline (types, call graph, a local-hand walkthrough). [`PHASE_2.md`](./PHASE_2.md) expands the engine and TUI. [`PHASE_3.md`](./PHASE_3.md) expands the mesh, lobby, and sequencer. [`PHASE_4.md`](./PHASE_4.md) expands mental poker (SRA, Keyring, shuffle/peel FSMs, `CryptoHand`). [`PHASE_5.md`](./PHASE_5.md) expands liveness, escrow, tests, and honest gaps.

---

### Phase 1 — Orientation

**Chapter:** [`PHASE_1.md`](./PHASE_1.md)

**You are here to learn:** what problem this repo solves, what it deliberately does *not* solve, how to build and run it, and which package is which.

**Read with:** `HOW_IT_WORKS.md` §§1–9, 23, 25–26. `SYSTEM_DESIGN_OVERVIEW.md` §§1–2.

**Files (in order)**

| File | Why this file, now |
|---|---|
| `README.md` | Ground truth for commands, seats, crypto vs `--no-crypto`, known limits |
| `HOW_IT_WORKS.md` (early + glossary) | Vocabulary: LAN, multiaddr, gossip vs streams, replica |
| `SYSTEM_DESIGN_OVERVIEW.md` §§1–2 | Compact architecture before any Go |
| `go.mod` | What the binary actually depends on |
| `config/config.go`, `config/loader.go` | The knobs `main` will pass into every package |
| `cmd/poker/main.go` | Process entry. Study `main`, `runLocalMode`, and only the *shape* of `runP2PMode` |
| `cmd/poker/main_test.go` | How modes and flags are supposed to behave |

**Do this with your hands:** `go build -o poker ./cmd/poker`, then `./poker` (or `.\poker.exe`) for a local vs-bots hand. Optionally `./poker host --seats 3 --name Alice` in a second terminal just to see a multiaddr printed. You do not need three peers yet.

**Do not read yet:** `internal/crypto`, `internal/fault`, `contracts/`, `plans/`.

**Exit check.** You can explain, without notes: host is not a game server; P2P needs 3–9 seats; local mode has no SRA; chips in the demo are local counters; identity lives in `~/.poker/identity.key`.

---

### Phase 2 — Pure Hold’em engine (and the local UI)

**Chapter:** [`PHASE_2.md`](./PHASE_2.md)

**You are here to learn:** the engine is a pure reducer. Same seats + same ordered actions + same card inputs → same pots and winners on every honest machine. Cards in crypto mode are *inputs*, not something `Machine` samples.

**Read with:** `HOW_IT_WORKS.md` §§7 and 10.

**Files (in order)**

| File | Why this file, now |
|---|---|
| `internal/game/deck.go` | Card ids are the integers cryptography will encrypt later |
| `internal/game/player.go` | Stack and status changes that `ApplyAction` will call |
| `internal/game/state.go` | Phases, including `PhaseAwaitingStreet` for crypto |
| `internal/game/pot.go` | Side pots are math, not a special betting case |
| `internal/game/hand_eval.go` | Showdown does not trust a `HAND_RESULT` on the wire |
| `internal/game/machine.go` | The only betting mutation; plaintext vs crypto start paths |
| `internal/game/game_test.go` | Worked plaintext hands |
| `internal/game/machine_crypto_test.go` | How the engine behaves with `Deck == nil` |
| `internal/tui/*.go` (styles → card → panel → table → log → bet → model) | How a human sees `GameState`. Note hole-card hiding in `player_panel.go` |
| `internal/tui/tui_test.go` | What the UI is tested to show and hide |

**Do this with your hands:** play one local hand and, while you fold/call/raise, keep `machine.go` open. Every keystroke becomes an `Action` callback into `ApplyAction`. The TUI does not contain the rules.

**Do not read yet:** `internal/network` except curiosity. The engine must make sense with zero sockets.

**Exit check.** You can trace `ApplyAction` for fold / check / call / raise / all-in, explain when a betting round ends, and say what `StartHandCrypto` refuses to do (it will not shuffle or deal from a local deck).

---

### Phase 3 — Mesh, lobby, sequenced actions

**Chapter:** [`PHASE_3.md`](./PHASE_3.md)

**You are here to learn:** how four laptops find each other, how bytes move (gossip vs direct streams), how joins become a seat order, and how unordered GossipSub becomes a total order of `PLAYER_ACTION`s. Public game state is a replicated log, not a broadcast of `GameState`.

**Read with:** `HOW_IT_WORKS.md` §§4–6 and 11–15. Then `SYSTEMS_DESIGN_INTERVIEW.md`.

**Files (in order)**

| File | Why this file, now |
|---|---|
| `internal/network/messages.proto` | The wire vocabulary. Dual counters: envelope `seq` vs `PlayerAction.Seq` |
| `internal/network/codec.go` | Signatures over gossip; why Noise on the hop is not enough |
| `internal/network/host.go` | Peer ID, listen, Noise, TCP, peerstore |
| `internal/network/discovery.go` | LAN mDNS; what does *not* exist (DHT, relays) |
| `internal/network/gossip.go` | Two topics; self-echo dropped; apply-local-then-broadcast |
| `internal/network/protocol.go` | Unicast Shamir shares and best-effort hole peels |
| `internal/network/lobby.go` | Seat map, timestamps, ready barrier, session nonce |
| `internal/network/gamelog.go` | Paper trail for disputes, not a consensus log |
| `internal/network/node.go` | Composition root of a peer; wire callbacks *before* `Start()` |
| `internal/network/coordinator.go` | Local/oracle helper — do not confuse with live `CryptoHand` |
| `internal/network/network_test.go` | Concrete join / ready / sign cases |
| `SYSTEMS_DESIGN_INTERVIEW.md` | Recap in a form you could say out loud |

**Do this with your hands:** three terminals, same machine, different `--listen` ports, `--no-crypto` is fine for this phase. Watch joins, ready, then a betting round. You are studying *order of actions*, not ciphertexts.

**Do not deep-read yet:** `crypto_hand.go`, `internal/crypto`, `internal/fault`. Know that `Node` has `OnShuffleStep` / `OnPartialDecrypt` callbacks; Phase 4 fills them.

**Exit check.** You can explain why gossip is unordered, why there are two sequence spaces, why the acting player applies locally first, and why `HAND_RESULT` cannot move chips on an honest replica.

---

### Phase 4 — Mental poker (hidden cards, no dealer)

**Chapter:** [`PHASE_4.md`](./PHASE_4.md)

**You are here to learn:** commutative encryption lets everyone lock the deck; selective peels let only the rightful player see a hole card; ZK proofs stop garbage decrypts; `CryptoHand` is the live replica protocol, while `CryptoGame` is an in-process oracle with every key.

**Read with:** `HOW_IT_WORKS.md` §§16, 20–21.

**Files (in order)**

| File | Why this file, now |
|---|---|
| `internal/crypto/params.go` | Shared `p`; cards as numbers |
| `internal/crypto/sra.go` | `(e, d)` per peer; public-only `SRAKey` |
| `internal/crypto/commit.go` | Bind a shuffle step without publishing the permutation |
| `internal/crypto/zkp.go` | Peel correctness |
| `internal/crypto/keyring.go` | **Invariant:** no API returns another peer’s `d` |
| `internal/crypto/shuffle.go` then `shuffle_session.go` | Library vs distributed FSM |
| `internal/crypto/deal.go` then `deal_session.go` | Same split for peels |
| `internal/crypto/crypto_game.go` | Oracle: all keys on one machine — tests only |
| crypto `*_test.go` files | Smallest working examples of shuffle + peel |
| `internal/network/crypto_hand.go` (+ test) | Live path: gossip shuffle, gossip+stream peels, gates |
| `cmd/poker/main.go` (`runP2PMode`) | Where Keyring, shares, and `CryptoHand` are actually started |
| `plans/phase-1-keyring.md` … `plans/phase-5-wire-p2p.md` | How this was built, after you already understand the code |

**Do this with your hands:** three-player **crypto** table (default, no `--no-crypto`). Expect several seconds of “Shuffling…”. Confirm your TUI does not show opponents’ holes. Optional: one debug table with `--no-crypto` on **every** peer to contrast a public deck.

**Exit check.** You can walk a hole-card deal: who peels on the wire, who peels last locally, why opponent holes stay empty, and why a mixed table (one peer `--no-crypto`) must exit.

---

### Phase 5 — Liveness, settlement, tests, honest gaps

**Chapter:** [`PHASE_5.md`](./PHASE_5.md)

**You are here to learn:** after the shuffle, a silent peer can be timeout-folded and their `d` reconstructed from Shamir shares so remaining peels finish. Mid-shuffle disconnect aborts. Money, if it ever ships, sits in escrow off the hot path. Integration tests and the original issues list tell you what is real vs designed.

**Read with:** `HOW_IT_WORKS.md` §§17–19, 22, 24.

**Files (in order)**

| File | Why this file, now |
|---|---|
| `internal/fault/*` | Heartbeats → 2/3 votes → fold → reconstruct `d` → slash records |
| `internal/network/liveness.go`, `fault_adaptor.go` | How the fault package is driven from the live loop |
| `plans/phase-6-liveness.md` | Historical spec for the above |
| `contracts/PokerEscrow.sol` + Hardhat test | Settlement design; 2/3 signatures, challenge window, slash |
| `internal/chain/client.go`, `escrow.go` | **Not live.** Stub RPC. Do not claim ETH payouts |
| `internal/integration/e2e_test.go` | Happy multi-node path |
| `internal/integration/adversarial_test.go` | Silence, junk peels, equivocation, mixed crypto flags |
| `CRYPTO_DEAL_PLAN.md`, `ISSUES_AND_RECOMMENDATIONS.md` | History and remaining honesty (reconnect, NAT, BFT, chain glue) |
| `HOW_IT_WORKS.md` §§20–22, 24 | Payoff: one full hand, then cheat/disconnect variants |

**Do this with your hands:** `go test ./...`. Optionally Hardhat tests in `contracts/` if you have Node 18+. Kill one of three crypto peers *after* shuffling and watch timeout-fold + recovery if you want the liveness story in the flesh.

**Exit check.** You can say which faults the table survives (post-shuffle silence) and which it does not (mid-shuffle drop, lost gossip with no retransmit, mid-hand reconnect). You can say the Solidity contract is real and the Go Ethereum client is not wired.

---

## Suggested calendar

| Pace | How to use the phases |
|---|---|
| One afternoon | Phase 1 + Phase 2. Run local mode. Stop. |
| Two days | Add Phase 3. Run `--no-crypto` host/join. Stop. |
| Three to four days | Add Phase 4. Run a real crypto table. Read §21 of `HOW_IT_WORKS.md`. |
| Extra half day | Phase 5. Tests + limitations. You are ready to change code. |

Do not start a feature in `internal/crypto` until Phase 4’s exit check is true. Do not “just add a server” in `cmd/poker` — that fights the whole design.

---

## After the five phases

Phase 1’s detailed chapter is [`PHASE_1.md`](./PHASE_1.md). Phase 2’s is [`PHASE_2.md`](./PHASE_2.md). Phase 3’s is [`PHASE_3.md`](./PHASE_3.md). Phase 4’s is [`PHASE_4.md`](./PHASE_4.md). Phase 5’s is [`PHASE_5.md`](./PHASE_5.md).

After the five chapters, re-read §21 of `HOW_IT_WORKS.md`; it should now name files you have actually opened.
