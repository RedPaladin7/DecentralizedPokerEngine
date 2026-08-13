# Gaps, Issues, and What to Fix

This note is a gap analysis of **Decentralized Poker Engine** against its own design (`DecentralizedPokerEngine/SYSTEM_DESIGN_OVERVIEW.md`). It is written for a **4th-year college engineering project**: what is already enough to defend, what looks unfinished, and what actually **defeats the thesis**.

The thesis, as stated in the design doc, is:

> Treat poker as a replicated state machine whose inputs are authenticated, totally ordered actions, and whose private inputs (cards) are supplied by a commutative encryption protocol rather than a dealer.

The first half (P2P replicated Hold'em) works. The second half (hidden cards, unbiased shuffle, on-chain money) is implemented as libraries and a Solidity contract, but **is not what `poker host` / `poker join` actually run**.

---

## How to read the verdicts

| Verdict | Meaning for a student project |
|---|---|
| **Must fix** | An examiner who reads the README / title will call this a hole in the claim. Fixing it (or honestly shrinking the claim) is the difference between “decentralized poker” and “LAN multiplayer poker.” |
| **Should fix if you claim the feature** | Fine to leave unfinished *if* you present it as future work. Not fine if the demo or report says it already works. |
| **Acceptable** | Real limitation, common at this scope. Document it. Do not spend the rest of the semester here unless everything above is done. |
| **Nice to have** | Production / product polish. Not required to pass a capstone. |

Priority for remaining work, if time is limited:

1. Wire **real multi-party SRA shuffle + deal** into `runP2PMode` (or stop claiming cryptographic dealing).
2. Make `--no-crypto` and the README match reality.
3. Optionally: drive timeout votes; optionally: talk to a local Hardhat node.
4. Everything else: write it up as known limitations.

---

## Snapshot

| # | Issue | Live today? | Verdict |
|---|---|---|---|
| 1 | Shared-seed plaintext shuffle: every peer sees every card | Default P2P path | **Must fix** |
| 2 | SRA / shuffle / deal exist, but only as a **local simulation** with all keys on one machine | Packages + tests | **Must fix** (same root cause) |
| 3 | Network messages for shuffle/decrypt exist but are never sent from the game loop | Proto + `Node` helpers only | **Must fix** if you keep the crypto claim |
| 4 | `--no-crypto` / README imply crypto dealing is the default or a working flag | Misleading | **Should fix** (docs + flag) |
| 5 | Ethereum escrow: Solidity is real, Go RPC is stubbed, `main.go` never calls it | Design-complete | **Should fix if you claim live payouts**; otherwise acceptable as a designed settlement layer |
| 6 | Timeout votes + Shamir recovery not started from `runP2PMode` | Partial (heartbeats only) | **Should fix** once crypto dealing is live; acceptable as-is for plaintext play |
| 7 | `ApplyAction` can race across TUI vs network goroutines | Known tightness | **Should fix** (small lock); not thesis-breaking |
| 8 | Seat order from sender clocks | Theoretical reordering | **Acceptable** |
| 9 | Not BFT; GossipSub + sequencer, honest-majority assumption | By design | **Acceptable** |
| 10 | No mid-hand reconnect (`GAME_STATE_SYNC` unused) | Missing | **Acceptable** |
| 11 | LAN-only: no DHT, no relays, NAT is UPnP only | Missing | **Acceptable** |
| 12 | No multi-table / tournaments | Out of scope | **Nice to have** |

---

## 1. Shared-seed plaintext shuffle (the one that defeats the purpose)

**What happens today.** After the lobby fills, every node does:

```
nonce  = concat(peer IDs in join order)
seed   = XOR(nonce bytes) then LCG mix
deck   = Fisher–Yates shuffle with math/rand seeded by that value
deal hole cards in seat order
```

That seed is **public and identical on every honest node**. Any peer can reconstruct the full deck, including every other player's hole cards. `--no-crypto` only skips generating an SRA key; `runP2PMode` still deals this way either way.

**Why it matters.** The problem statement is “no trusted dealer, hidden cards, fair shuffle.” A shared RNG is a **correctness trick** so replicas stay in lockstep. It is the opposite of mental poker. A curious (or cheating) player does not even need to hack anything — they just print the deck.

For a 4th-year project this is **not** “a small TODO.” If the live demo still works this way, a reasonable examiner conclusion is: *the interesting cryptography was written but the product does not use it, so the system does not actually provide the property it is named for.*

**What is fine to keep.** Using a shared seed for **local mode vs bots**, or as a `--no-crypto` debug/dev mode, is completely reasonable. Deterministic shuffle is a good teaching tool.

**Recommended fix.**

- Default `host`/`join` should run the SRA path (see issues 2–3).
- Keep plaintext shuffle behind `--no-crypto` and say so in the README: “debug mode: all cards visible, for testing sync only.”
- Until that is wired, the report/README should say **plaintext P2P poker with a crypto library beside it**, not “optional cryptographic dealing in multiplayer.”

This is the highest-ROI remaining piece of work. The algorithm packages already exist; the missing work is **protocol orchestration in the live loop**.

---

## 2. Crypto packages are a local simulation, not a distributed protocol

**What exists.** `internal/crypto` has real pieces: SRA on a 2048-bit MODP prime, encrypt-then-permute shuffle, commitments, partial decrypt, Schnorr-style ZK proofs, Shamir split of `d`. Tests and `HandCoordinator.RunHand` can run a hand with `StartHandCrypto`.

**What is wrong.** `CryptoGame.NewCryptoGame` generates **every player's** `(e, d)` on the calling node. `RunShuffle` then runs every shuffle step locally with those keys. `DealToEngine` peels layers using the same in-memory key list.

That means:

- One process holds every private exponent.
- There is no “I encrypt and permute, you cannot see my permutation.”
- Hole-card secrecy is vacuously true on a single machine and **false** if you imagined this running across peers.

`HandCoordinator` is a bootstrap helper around that local object. It is **not** “Alice shuffles, then Bob, then Carol, over the mesh.”

**Verdict.** Implementing the math in-process is an **excellent** student milestone (you can unit-test commutativity, proofs, and deal order). Leaving it as the only crypto path is **not** enough if the project claims P2P mental poker. A real dealer who “runs the protocol for everyone” is exactly the trust model the design rejects.

**Recommended fix.**

1. Each peer generates **only its own** `SRAKey`. Publish `e` in `JOIN_TABLE` (already in the proto). Never send `d`.
2. Shuffle in seat order over gossip:
   - Player `k` takes the previous committed deck, `EncryptAll` + secret permute, publishes `SHUFFLE_STEP` (output deck + commitment).
   - Others verify the commitment opening; they cannot check the permutation (that is intended) but they can check well-formedness and that the player is the one whose turn it is.
3. Deal hole cards with `PARTIAL_DECRYPT` on **direct streams** (`/poker/1.0.0` is already there for this): every peer except the recipient peels and attaches a ZK proof; the recipient peels last locally.
4. Community cards: everyone peels; plaintext becomes public.
5. `game.Machine.StartHandCrypto` stays as the engine entry once hole cards are filled.

You do **not** need a novel cryptosystem. Wiring the existing types onto the existing message types is the project-sized remaining task.

**How much is “enough” for a capstone.** A 2–3 player LAN demo where:

- `tcpdump` / a debug flag shows ciphertexts on the wire, not ranks/suits;
- a modified client that skips its last decrypt cannot see another player's hole cards;
- community cards appear only after all peels;

…is a complete story. You do not need 9-player WAN mental poker.

---

## 3. Shuffle / decrypt network path is unused

`messages.proto` already has `SHUFFLE_STEP`, `SHUFFLE_COMMIT`, `PARTIAL_DECRYPT`. `Node` has `BroadcastShuffleStep`, `BroadcastPartialDecrypt`, and callbacks. Grep shows **no caller** from `cmd/poker` or `HandCoordinator`.

**Verdict.** Dead protocol surface. Fine while building, not fine in a final demo that lists those message types as how the game works.

**Recommended fix.** Same as issue 2: `runP2PMode` (or a real `HandCoordinator`) must be the caller. Do not add more proto fields until this loop actually runs.

---

## 4. Docs and flags oversell the live system

Examples:

- README feature list: “Plaintext **or** cryptographic dealing.”
- README known limitations: “Default mode has no cryptographic card dealing (use `--no-crypto` to opt in to this)” — the flag meaning is inverted vs `main.go`, and crypto dealing is not the non-default live path either.
- CLI help: `--no-crypto` “Disable cryptographic shuffling (plaintext cards)” — shuffling is already plaintext.
- README highlights: “Fairness Built-In — Optional SRA shuffling protocol.”

**Verdict.** For a student project, **honest README > extra features.** Examiners notice this immediately.

**Recommended fix.** One status line at the top:

> Multiplayer over libp2p works. Cards in the live path are **not** hidden (shared-seed shuffle). SRA / ZK / escrow are implemented as modules and are not the `host`/`join` dealing path yet.

Then make the flag real once issue 1 is fixed.

---

## 5. Blockchain: contract yes, live money no

**What exists.** `PokerEscrow.sol` is a real contract: join with ETH, 2/3-signed `reportOutcome`, chip conservation, dispute window, slash burn, abandon/refund. Hardhat tests exist. `EscrowManager` can build payloads from `GameState.Payouts` + gamelog root.

**What is missing.** `internal/chain.Client` returns **synthetic receipts** (no `ethclient`, no JSON-RPC). `main.go` never calls the chain package.

**Verdict.**

- If the report says “optional on-chain settlement, contract designed and tested, Go client not live” → **acceptable** and still impressive.
- If the demo or abstract says players get paid in ETH → **must not claim that.**

Live mainnet / real-money poker is **out of scope** for a college project (and legally messy). A local Hardhat loop is the right ceiling.

**Recommended fix (only if you have time after crypto dealing).**

1. Replace the stub `Client` with `go-ethereum` `ethclient` against `http://127.0.0.1:8545`.
2. `poker host` deploys or attaches; joiners `joinTable{value}`.
3. On settle, collect ≥2/3 signatures on `(payouts, stateRoot, handNum)` and `reportOutcome`.
4. Keep it on a local chain. Do not chase Infura/mainnet.

If time is short: keep the stub, show the Solidity tests, and say the economic layer is specified but not integrated. That does **not** defeat the card-secrecy thesis the way issue 1 does.

---

## 6. Fault path is half-wired

**What works.** Heartbeats are published; `OnHeartbeat` calls `RecordHeartbeat`; `OnPlayerFolded` can `forceFold`. Equivocation is scanned every 5s on the gamelog.

**What does not run in `runP2PMode`.**

- `FaultManager.Run` (heartbeat **monitor** loop) is never started — only the sender is.
- Timeout **votes** (2/3 of `n-1`) are implemented and tested, not driven from the live handlers.
- Shamir share distribute / reconstruct is not scheduled. That only becomes necessary once a player’s `d` is required to finish a decrypt.

**Verdict.** For plaintext shared-seed poker, force-fold on silence is enough for a demo. For mental poker, a disconnect during a peel **deadlocks the hand** unless you reconstruct or fold-and-skip. So: acceptable now; **should fix as part of issue 2**.

**Recommended fix.**

1. Start `fm.Run(ctx)` next to `HeartbeatSender`.
2. On timeout: broadcast `TIMEOUT_VOTE`; on 2/3, `forceFold` **and** broadcast that fold as a `PLAYER_ACTION` so every replica applies it (local-only fold desyncs the machine).
3. After SRA is live: `SplitAndDistribute` at hand start; on withholding, reconstruct and finish community peels; slash if you have the chain path.

Do not build a full BFT view-change. 2/3 timeout + fold is the right student-sized liveness story.

---

## 7. Game machine is not single-threaded in the live process

`ApplyAction` has no internal lock. The TUI applies locally then broadcasts; the receive goroutine applies remote actions after unlocking `machineMu` (the mutex only protects the **pointer**, not the machine). A delayed or hostile message can interleave with a local apply.

**Verdict.** Real bug, small fix. It does not defeat “decentralized poker,” but it is a good systems talking point and a cheap win.

**Recommended fix.** Hold one mutex around **every** `ApplyAction` (local and remote), including sequencer drain. Treat the machine as a single-threaded reducer. Document that GossipSub reordering is handled by `actionSequencer`, not by the engine.

---

## 8. Seat order depends on sender timestamps

Canonical seats: `JoinedAtUnixMs` then `PlayerID`. The timestamp comes from the **sender's** envelope, so clock skew can reorder seats and thus the shared seed / dealer button.

**Verdict.** Acceptable. Honest LAN clocks are a fair assumption. Mention it in the report.

**Recommended fix (if you touch lobby code anyway).** Order by `PlayerID` only, or by a hash of `(table_id ‖ peer_id ‖ join_seq)` assigned by first-seen signed join. Do not build a clock-sync protocol.

---

## 9. Not Byzantine consensus

The design is explicit: replicated state machine, not PBFT/Raft. Safety assumes the same ordered action log. GossipSub is unordered; `PlayerAction.Seq` restores order. A partition or a player who shows different seq streams to different peers is an equivocation problem (detected after the fact), not prevented by quorum.

**Verdict.** **Acceptable and correct to leave.** Full BFT on every fold/check would dominate the semester and is not how mental-poker papers usually specify the game loop. Signed log + equivocation evidence is the right scope.

**Recommended write-up.** “We provide authenticated, totally ordered actions among honest nodes, plus after-the-fact proofs of double-talk. We do not provide agreement in the presence of a fully partitioned mesh.”

---

## 10. No mid-hand reconnection

`GAME_STATE_SYNC` exists in the proto; the live loop does not use it. A disconnect is terminal for that player.

**Verdict.** Acceptable. Catch-up is a product feature. For the demo, restart the table.

**Recommended fix (later).** Snapshot + `Gamelog` replay is the obvious design (the proto already sketches it). Only do this after crypto dealing works — replaying encrypted cards is harder than replaying plaintext actions.

---

## 11. Network is a LAN mesh, not the public internet

- Discovery: mDNS (LAN) + explicit `--peer` multiaddrs.
- No DHT / rendezvous.
- Relays disabled; NAT is `NATPortMap` only.
- `poker host` is the first listener, not a server, but joiners still need a reachable multiaddr.

**Verdict.** Acceptable. libp2p + GossipSub + Noise + signed envelopes is already a strong networking chapter. Public-internet NAT traversal is a different project.

**Recommended write-up.** “Best-effort LAN / port-forwarded WAN. Not a production overlay.”

---

## 12. Scope that should stay out of the remaining time

- Multi-table tournaments, late registration, sit-and-go.
- Mobile UI.
- Replacing SRA with a modern MPC / shuffle argument (e.g. Bayer–Groth). SRA is old and has known caveats (collusion of all-but-one, need for a safe prime / group setup you already took from RFC 3526). For a capstone, **SRA done for real on the wire** beats **a fancier paper protocol that still is not wired**.
- Mainnet money, matchmaking servers, app stores.

---

## Honest claim you can defend today

**Safe to claim**

- Complete Texas Hold'em engine (pots, side pots, 7-card eval, multi-hand).
- Equal peers over libp2p; no central game server.
- Signed gossip, replay protection, sequenced actions → identical state on honest nodes.
- Local vs P2P modes and a TUI.
- Crypto *library*: SRA, commitments, partial decrypt, ZK proofs, Shamir (unit-tested).
- Solidity escrow with dispute/slash semantics (tested in Hardhat).

**Not safe to claim until issues 1–3 are done**

- Hidden hole cards in multiplayer.
- Unbiased shuffle that a single peer cannot invert.
- “Optional SRA dealing” as a working `host`/`join` feature.
- Live ETH payouts.

**One-sentence examiner summary if you ship as-is**

> A working P2P Hold'em replica with solid networking and a substantial unused mental-poker and escrow stack; the live table still deals from a public seed, so card secrecy — the original reason to avoid a dealer — is not provided.

**One-sentence summary if you wire SRA into `runP2PMode` and keep chain stubbed**

> A LAN mental-poker Hold'em table: peers jointly shuffle under commutative encryption, deal with partial decrypts and ZK checks, and agree on pots locally; on-chain escrow is specified but not integrated.

That second sentence is a complete, defensible 4th-year systems/crypto project. The first one is a good engine with a hole in the middle.

---

## Suggested remaining work (smallest path that saves the thesis)

1. **Orchestrate shuffle+deal over the existing proto** in `runP2PMode` (issues 1–3). This is the only item that is not optional.
2. **Lock `ApplyAction`** (issue 7) while you are in `main.go`.
3. **Rewrite README / `--no-crypto`** (issue 4) so the demo script cannot be accused of advertising vapor.
4. Start `FaultManager.Run` and broadcast timeout folds (issue 6) so a killed peer does not freeze the table.
5. Stop. Put chain RPC, reconnect, DHT, and tournaments in “future work.”

If you only have time for one coding milestone, do **(1)**. Everything else is either documentation or polish. A shared-seed live path with a beautiful unused `internal/crypto` package will not survive a design review of a project whose point is untrusted dealing.
