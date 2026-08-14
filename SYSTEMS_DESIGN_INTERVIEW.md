# Systems Design Interview Guide — Decentralized Poker Engine

How to present this project in a **45-minute systems design interview**. Treat it as a **replicated state machine with a mental-poker dealing layer**, not “P2P poker on libp2p.” That framing is what a staff interviewer wants.

Companion document: [`SYSTEM_DESIGN_OVERVIEW.md`](./SYSTEM_DESIGN_OVERVIEW.md) (architecture source of truth).

**Suggested clock**

| Minutes | Phase |
|---|---|
| 0–7 | Requirements + scope |
| 7–18 | High-level design (draw this) |
| 18–35 | Deep dive: consistency + cheating |
| 35–45 | Trade-offs, scale, what you’d ship next |

If they interrupt, drop the drawing polish and go to the deep dive. That is where you get hired.

---

## 1. Requirements Gathering (first ~7 minutes)

Do **not** start with libp2p. Start with the problem, then lock scope, then NFRs.

### Open with the problem (30 seconds)

> “A centralized poker server is a trusted dealer. Players have to believe the house shuffled fairly, didn’t look at hole cards, and won’t rewrite the pot. I want a table among mutually distrusting peers with **no house**, where every node independently computes the same outcome, cards can be hidden, and money can settle on-chain.”

Then pin the product:

> “Texas Hold’em, local 2–9 / P2P 3–9 seats, one table, multi-hand. Not a 10k-player tournament, not a global matchmaking service.”

That last sentence is important. Poker’s natural shard size is a table. You are not designing GossipSub for a million nodes.

### Functional requirements — say these out loud

Write them on the left of the board:

1. **Join / seat / start** — table fills to N, then a hand starts. Canonical seat order.
2. **Full Hold’em loop** — blinds, hole cards, pre-flop → flop → turn → river → showdown, check/call/raise/fold/all-in, main + side pots, next hand.
3. **Identical outcomes** — every honest peer computes the same stacks, pots, and winner. No node is “the server.”
4. **Hidden hole cards** — only the recipient learns their two cards; community cards become public at the right street.
5. **Unbiased shuffle** — no single player chooses the deck order.
6. **Authenticated actions** — you cannot impersonate another seat.
7. **Liveness** — a silent player is force-folded; after shuffle, survivors reconstruct `d` and finish peels. Mid-shuffle abort.
8. **Accountability** — if someone double-speaks or fakes a decrypt, we have signed evidence.
9. **Optional settlement** — buy-in escrow, multi-sig payout, dispute/slash window.

If they ask “what did you actually ship?” be precise:

> “Live path today: replicated Hold’em over gossip, SRA shuffle + ZK peels so hole cards stay hidden, timeout fold plus Shamir reconstruct if someone dies after the shuffle. `--no-crypto` is a debug shared-seed mode. Ethereum escrow is a real Solidity contract; the Go RPC client is still stubbed — chips in the demo are local counters.”

Do not claim live ETH payouts or 2-player P2P. Claiming the chain is wired when it isn’t will get you caught.

### Non-functional requirements — put numbers on them

Interviewers punish vague NFRs. Use these:

| NFR | Target you should state | Why |
|---|---|---|
| **Fairness** | No player learns another’s hole cards; shuffle unbiased unless *all* collude | Core product |
| **Consistency** | Strong: all honest nodes apply the same total order of actions | Poker cannot fork mid-hand |
| **Latency** | 100–500 ms mesh latency is OK; action RTT < ~1s | Human-speed game, not HFT |
| **Availability** | Table survives 1 crash **after shuffle** via timeout fold + Shamir peels; mid-shuffle abort. Not “five nines” | 9-player table, not S3 |
| **Fault model** | Crash-stop + equivocation + invalid decrypt. Not full BFT on every action | Critical distinction |
| **Security** | Authenticated gossip, replay-safe, evidence for slash | Trustless table |
| **Scale** | Local 2–9; P2P 3–9 per table; many tables are independent | Don’t over-design the mesh |
| **Durability of money** | On-chain escrow is source of truth for ETH; chips off-chain | Separate game from cash |

### Clarify with the interviewer (ask 3 questions, then stop)

1. “Is the threat model **honest-but-curious** peers, or **Byzantine** peers who equivocate?” → You designed for both, but **did not run PBFT per action**.
2. “Is real-money settlement in scope?” → Yes as a layer; game continues if chain is down.
3. “LAN only or internet?” → LAN via mDNS; WAN via bootstrap multiaddr. Relays off. Call NAT a known gap.

Then lock:

> “I’m designing **one table, P2P N=3..9, replicated state machine, mental-poker dealing, gossip for broadcast, chain only for money.** Out of scope: matchmaking, tournaments, mobile, reconnection/catch-up — I’ll mention those as follow-ups.”

---

## 2. High-Level Design (minutes 7–18)

### The sentence that should sit above the diagram

> “There is no game server. Every peer is a full replica. Public game state is a **deterministic state machine** fed by a **totally ordered action log**. Private cards are a **commutative encryption protocol**. Money is an **optional escrow contract** that only sees a signed outcome hash.”

### What to draw (keep it four boxes + a mesh)

Draw this, left to right:

```
  Discovery          Overlay              Per-peer process                 Settlement
  ---------          -------              ----------------                 ----------
  mDNS (LAN)    →    GossipSub mesh  →   ┌─────────────────────┐
  bootstrap          table topic         │  Signed envelope    │
  multiaddr          heartbeat topic     │         ↓           │     →  PokerEscrow
                     direct streams      │  Sequencer          │        (buy-in,
                     /poker/1.0.0        │         ↓           │         2/3 sigs,
                                         │  Game machine       │         dispute)
                                         │  (identical replica)│
                                         │         ↓           │
                                         │  Append-only log    │
                                         └─────────────────────┘
                                              ↑
                                         Mental poker
                                         (SRA shuffle +
                                          partial decrypt)
```

Say while you draw:

**Host is not a server.** `poker host` is the first listener that prints a multiaddr. After join, every node publishes and subscribes. If you draw a star with “Host” in the middle, you have already failed the design.

**Two topics.** Game messages on `poker/table/<id>`. Heartbeats on a separate topic so a slow action mesh does not look like a dead player.

**Two transports.** Gossip for “everyone must see this” (joins, actions, shuffle steps, public peels). Direct streams for hole peels (best-effort) and **unicast Shamir shares of `d`**. Gossip is forwarded, so **Noise is not enough** — you sign the payload. Direct streams are Noise-bound to a Peer ID, so hop auth is enough. Live shares are not gossiped until a timeout vote.

### Walk the happy path in 90 seconds (do this on the board as numbered arrows)

1. **Identity** — Ed25519 key → libp2p Peer ID. Persistent identity, not a username.
2. **Discover / dial** — mDNS on LAN, or `--peer` multiaddr.
3. **Lobby** — `JOIN_TABLE` (name, buy-in, SRA public exponent, nonce). Seats ordered by join time, then Peer ID. That order is the canonical permutation of the table.
4. **Barrier** — `PLAYER_READY`. When N seats are ready, the hand exists.
5. **Shared session binding** — concatenate nonces in seat order → session id / seed. Every honest node that saw the same joins gets the same bytes.
6. **Deal** — default: SRA shuffle then selective decrypt (local holes only). `--no-crypto`: shared-seed shuffle, all cards visible.
7. **Play** — current player signs an action; everyone applies it in sequence number order. Crypto: streets and showdown cards come from peels (`ApplyStreet` / `ApplyHoleReveal`), not from a local deck.
8. **Settle** — optional: payout deltas + log root, ≥2/3 signatures → escrow contract.

### Four components per node (the “box inside a peer”)

Name them; don’t dump packages:

| Layer | Job |
|---|---|
| **Network** | Mesh, signed envelopes, lobby |
| **Sequencer** | Turn unordered gossip into a total order |
| **Game machine** | Pure Hold’em reducer: same inputs → same `GameState` |
| **Crypto / fault / chain** | Hidden cards, timeouts, money |

Punchline:

> “The game engine has no sockets. That’s deliberate. Consensus is ‘apply the log.’ Networking is ‘deliver authenticated bytes.’ Mixing them is how centralized servers happen by accident.”

### What *not* to draw

- A Raft leader. You don’t have one.
- A “dealer node.” Dealing is a protocol, not a role.
- A global DHT. Discovery is mDNS + bootstrap.

If they ask “why GossipSub not a fully connected TCP mesh?”:

> “N=9, fully connected is fine. I still use pub/sub because membership changes, heartbeats shouldn’t head-of-line-block actions, and libp2p already gives mesh maintenance. The cost is **unordered, best-effort delivery**, which I fix at the application layer.”

That last clause is your bridge into the deep dive.

---

## 3. Deep Dive (minutes 18–35)

This is the interview. Two problems only: **(A) how all nodes stay identical**, **(B) how you prevent cheating without a dealer**. Spend ~8 minutes on A, ~8 on B.

### A. State consistency without a leader

**Name the pattern:** replicated state machine (same idea as Raft’s log application, **without** Raft’s leader election or committed-index).

**Invariant you write on the board:**

```
identical initial state  +  identical ordered actions  =  identical GameState
```

If that holds, you never broadcast “the pot is $80.” You broadcast “Alice raises 40.” Everyone computes the pot.

#### Initial state (how you get the same starting point)

Three things must match:

1. **Seat vector** — join messages, ordered by timestamp then Peer ID.
2. **Config** — blinds, buy-in, table id, dealer index.
3. **Deck permutation** — default: jointly shuffled ciphertext (ShuffleSession; replicas agree without sharing `d`). `--no-crypto`: `seed = mix(sessionNonce)`; every node Fisher–Yates with that seed.

Call out the footgun: join timestamps are sender-stamped. Clock skew can reorder seats. Senior move: “I’d replace wall-clock order with a hash of Peer IDs plus a VRF/commit-reveal for dealer, or a single observed receive-order only if we had a leader — which we don’t. For LAN v1, synchronized clocks are the assumption.”

#### The hard part: GossipSub is not a log

Say this cleanly:

> “Gossip gives me *dissemination*, not *order*. Peer A can see action 3 before action 2. If I apply eagerly, replicas diverge and the hand is over.”

**Fix: two sequence spaces** (draw them; interviewers love this):

| Counter | Scope | Purpose |
|---|---|---|
| Envelope `seq` | Per sender | Replay / dup protection. Drop if `seq ≤ last` |
| Action `Seq` | Table-wide | Total order of game mutations |

Local actor: apply immediately, assign next action seq, broadcast.  
Remote: **buffer until `nextSeq` arrives**, then drain. Classic gap-buffer / sequencer.

Self-echo: GossipSub will bounce your own message back. Drop if `sender == me`. That’s why you apply locally first (optimistic UI) — otherwise you wait for your own round-trip.

#### Deterministic reducer

`ApplyAction` is the only betting mutation. It validates “is it your turn?”, updates stacks, maybe ends the betting round. In crypto mode the next street is **not** dealt from a local deck — the replica waits (`PhaseAwaitingStreet`), peels publicly, then `ApplyStreet`. Showdown fills opponent holes via `ApplyHoleReveal`. No I/O in the engine.

Consequences you should say:

- Winners are not announced; they are computed. A lying `HAND_RESULT` is ignored if it disagrees with local state.
- Next hand starts a **new** shuffle (session id mixes `handNum`). `--no-crypto` mixes `handNum` into the seed instead.
- When the machine object is replaced for a new hand, the network thread must not apply to the stale one — pointer swap under a lock.
- `machineMu` is held around every apply; crypto wait loops release it so a timeout fold can run.

#### What this is *not*

> “This is **not** BFT consensus. I am not running a quorum of `Prepare/Commit` on every fold. I am assuming: messages are authenticated, the sequencer eventually delivers a single total order to honest nodes, and a player who sends two different payloads for the same seq is **detectable** via the log, not prevented by PBFT.”

If they push “so a partition can split the table?”:

> “Yes. Unlike Raft, I don’t have a committed majority. A minority partition that doesn’t see actions will stall on the sequencer gap. The timeout path force-folds silence; it does not merge forks. For N≤9 on a LAN that’s acceptable. For adversarial WAN I’d add either a leader-based log (Raft) or BFT (HotStuff) **just for the action log**, and keep the game machine unchanged. That’s the point of keeping the reducer pure.”

That answer is staff-level: you know when RSM-over-gossip is enough and how you’d upgrade the log without rewriting Hold’em.

#### Concurrency in one sentence

> “Network goroutine produces signed actions; sequencer serializes them; the game machine is a single-threaded reducer; the TUI is a subscriber. Heartbeats run on their own loop so liveness checks don’t share a queue with raises.”

### B. Cheating without a central server

Split the threat model on the board. Don’t mush “fairness” into one word.

| Attack | Defense |
|---|---|
| Impersonate Alice | Peer ID = pubkey; Ed25519 over envelope |
| Replay an old raise | Per-sender envelope seq watermark |
| See Bob’s hole cards | SRA: Bob’s layer stays on the ciphertext |
| Stack the deck | Every player encrypts **and** permutes |
| Send fold to Alice, raise to Bob | Equivocation: same `(sender, seq)`, two payloads, both signed |
| Fake a decrypt | ZK proof of correct exponentiation |
| Stall / withhold decrypt | Timeout fold + Shamir reconstruction of `d` |
| Steal the pot on-chain | Chip-conservation deltas + 2/3 signatures + challenge window |

Now walk **mental poker** like a protocol designer, not a crypto paper.

#### Why SRA (commutative encryption)

Each player has `(e, d)` on a shared 2048-bit prime: `c = m^e mod p`. Encryption commutes: order of keys doesn’t matter. Final ciphertext is `m^{e1 e2 … en}`. Anyone can peel **their** layer; nobody can peel someone else’s without `d`.

**Shuffle (n sequential steps):**

1. Encode cards as field elements (`2^(id+1) mod p`) so they live in the multiplicative group.
2. Player k encrypts all 52, secret-permutes, publishes the deck + SHA-256 commitment.
3. Next player takes that as input.

After n steps: jointly encrypted, jointly shuffled. One honest shuffler is enough to randomize (assuming they actually permute randomly). Collusion of **all** players still breaks this — say that; don’t oversell.

**Deal hole card to R:**

Everyone except R publishes a partial decrypt + ZK proof. R peels last, privately. Others still see a ciphertext.

**Community:** everyone peels. Burns are index skips, matching the engine’s burn-then-deal so crypto and rules stay aligned.

#### ZK proofs (say what they prove, not the algebra)

> “A partial decrypt is just a number. Without a proof I can publish garbage and brick the hand. Each peel is a Schnorr-style proof of correct exponentiation, Fiat–Shamir’d with the session id so proofs don’t replay across tables.”

Failed proof → slash record. Don’t write the equations unless asked.

#### Commitments

The shuffle output is committed. You cannot later claim you published a different deck. Cheap, necessary, not sufficient alone (need the encrypt+permute).

#### Equivocation vs. prevention

This is a distinction that separates you:

> “I **detect** double-talk; I don’t **prevent** it at broadcast time. Gossip has no atomic broadcast. Both conflicting envelopes are signed, so I have court-admissible evidence. Prevention would mean a BFT broadcast layer. I chose detect-and-slash because the table is small and money can sit in escrow until the challenge window.”

#### Liveness vs. safety of cards

Timeout: heartbeats, 15s, then a **2/3 vote of the other n−1** to force-fold **and broadcast that fold**. That is liveness of the *betting* machine.

Key withholding: if the missing player holds a decrypt layer the table needs (showdown / community), fold is not enough. **Shamir-share `d` at table start** (unicast; threshold ~n/2, min 2 — hence P2P `n ≥ 3`). Survivors reconstruct and a designated peer peels **on behalf** of the missing id. Mid-shuffle the permutation is gone: abort the hand. Different failure mode, different tool. Say that explicitly.

#### Money is not the game

> “Chips in the engine are a counter. ETH is the contract. The contract never sees cards. It sees payout deltas that sum to zero, a state root of the signed log, and enough signatures. Disputes in a short window slash with evidence. If nobody settles, abandon and refund.”

That’s how you keep the chain off the hot path. Poker at 2s/action cannot wait for block time.

#### Honesty about what is not live

> “SRA dealing and Shamir peels are on the default `host`/`join` path. `--no-crypto` is shared-seed — great for lockstep, **zero card privacy**. Ethereum escrow is designed and tested in Solidity; the Go client does not talk to a node, so demo chips are honor-system counters.”

If they only want shipped reality, stay on sequencer + replica + SRA. If they want the remaining gap, go chain RPC and reconnect. Read the room.

---

## 4. Trade-offs (minutes 35–45)

Don’t list random cons. Pair each with **why you accepted it** and **what you’d change**.

### 1. Gossip + sequencer vs. consensus per action

| | Gossip RSM | Raft / BFT log |
|---|---|---|
| Latency | One flood, ~100–500 ms | Extra RTTs (2–3 for Raft, more for BFT) |
| Complexity | Sequencer + detect equivocation | Leader, terms, views |
| Fork / partition | Stalls or splits; no commit index | Majority commit |
| Throughput | Fine for ~1 action / few seconds | Overkill |

**Call:** human-speed table, N≤9 → gossip. **Upgrade path:** same `ApplyAction`, replace the log transport. Don’t put Raft inside the poker rules.

### 2. Latency vs. fairness (mental poker)

SRA shuffle is **n sequential** encrypt-and-permute rounds, 52 modular exponentiations per player per round, 2048-bit. Deal is O(n) peels per card with ZK proofs.

That’s seconds, not milliseconds, before pre-flop. Fine for a LAN demo / home game; death for “instant play.” Do not pretend you optimized SRA.

`--no-crypto` shared seed is **fast and identical and totally transparent**. Never mix modes at one table (the live loop errors if any seat is missing `e`).

### 3. Mesh scalability

GossipSub is for **many** topics/peers. Your bottleneck is **not** the mesh. It’s:

- Mental-poker crypto: O(n²) peels as n grows.
- Sequencer: one total order; n=9 is trivial.
- UI / humans: 9-max is a **product** constraint.

**Do not** say “we’ll shard GossipSub.” Say: **scale-out is many independent tables**, not a bigger mesh. Tournaments = a coordinator that assigns tables; each table is this system. That’s the correct scale story.

WAN: no relays, weak NAT. Mesh over the public internet is the real ops bottleneck, not algorithmics. Circuit relay or a small bootstrap/signaling service is the pragmatic fix — and it’s a **discovery** concession, not a game server, if it never sees actions or cards.

### 4. Strong consistency vs. reconnection

No catch-up in v1. A reconnecting player has no snapshot.

`GAME_STATE_SYNC` exists in the protocol for a reason. Staff answer:

> “I’d send state root + action log from any peer. Verify signatures, replay into a fresh machine, accept only if root matches. That’s log replication, not ‘ask the host for the truth.’”

Until then, disconnect = timeout fold. Harsh, simple, consistent.

### 5. Detect-and-slash vs. prevent

Slash needs **money at stake** and a **judge** (contract or social). Without chain, slash records are logs. With chain, 20% burn is the deterrent.

If stakes are high and adversaries will equivocate on a partition, detect-and-slash is late. Then you pay for atomic broadcast.

### 6. On-chain settlement vs. hot path

Every action on-chain = unplayable (12s+ blocks, gas, reorgs).  
Every action off-chain = you reintroduced a house if someone holds the chips.

**Call:** off-chain game, on-chain escrow + outcome. Challenge window is the security/latency knob (50 blocks in the contract — tune it).

### 7. Single-threaded reducer vs. concurrent applies

The machine is not internally locked. The live loop holds one mutex around every `ApplyAction`. Street peels wait **without** that lock so timeout recovery can fold.

**Call:** keep the reducer single-threaded; never apply from two goroutines. If you need it, a single apply goroutine owning the machine, everyone else sends on a channel. That’s the Go-shaped version of “the log consumer is one thread.”

### Bottlenecks to name if they ask “what breaks first?”

1. **SRA CPU / sequential shuffle** — hand start time, grows with n.
2. **NAT / connectivity** — table never forms on WAN.
3. **Sequencer stall** — lost action 7, everyone waits forever (need NAK/retransmit or state sync).
4. **Clock-ordered lobby** — seat permutation disagreement → different shuffle turns / seeds → instant desync.
5. **Chain stub / challenge game** — money path not live; without it, chips are honor system.

### Close (last 60 seconds)

> “I would draw this as three layers that don’t leak into each other: a **pure deterministic engine**, an **authenticated totally-ordered log** over a mesh, and a **mental-poker + escrow** layer for privacy and money. The shipped demo is lockstep play without a server **and** hidden cards on the LAN. Remaining work is plugging the log root into the contract, reconnect/catch-up, and WAN discovery — not rewriting the architecture.”

Stop talking. Let them probe.

---

## Coaching notes (how you sound senior)

**Do**

- Say “replicated state machine,” “total order,” “commutative encryption,” “detect vs prevent,” “chain off the hot path.”
- Separate **envelope seq** from **action seq** without being asked.
- Volunteer the remaining gaps before they find them: chain RPC stub, no reconnect, mid-shuffle abort, P2P min 3 seats, LAN-only.
- Scope to one table immediately.

**Don’t**

- Draw a host/server.
- Claim PBFT or “blockchain consensus” for folds.
- Dive into Bubble Tea, protobuf fields, or Go mutex names unless they ask about implementation.
- Pretend 2048-bit SRA is cheap.
- Scale stories about “millions of concurrent players on one GossipSub topic.”

**If they hijack into “design PokerStars”**

Matchmaking and wallets are a different system. Your table is a **stateful shard**. You can spend two minutes on: lobby service assigns `table_id` + bootstrap peers; each table is this design; escrow per table; that’s it.

**Likely probes and one-line answers**

| They ask | You say |
|---|---|
| Why not a trusted server + TLS? | Solves transport privacy, not dealer honesty. |
| Why not all-to-all TCP? | Fine at n=9; gossip still needs signatures and sequencing; I wanted topic isolation for heartbeats. |
| Why not run every action on Ethereum? | Human game vs block time; use escrow for money only. |
| How do you know the shuffle wasn’t stacked? | One honest permuter + all encrypt; commitments bind outputs; collusion of all still wins. |
| What if gossip drops a message? | Sequencer blocks; I need retransmission or snapshot sync — current gap. |
| Is this CAP? | During partition you don’t have a majority commit; you choose not to make progress rather than fork the pot. CP-shaped, without a quorum. |

Learn the invariant, the two sequence numbers, SRA deal vs `--no-crypto`, timeout-plus-Shamir vs mid-shuffle abort, and the gossip-vs-BFT trade. Everything else is supporting detail. You can walk this in 45 minutes without opening a file.
