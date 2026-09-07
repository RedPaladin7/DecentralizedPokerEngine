# How to Present This Project in an Interview

A structured report, in the same simple language as `THE_FULLFLOW.md`. Each claim was checked against the live code, not only against the design docs.

There is already a more technical coaching note in `SYSTEMS_DESIGN_INTERVIEW.md`. This file is the one to speak from: what the project is, why it exists, how it works, what it still cannot do, and what a good interviewer will poke at.

---

## How to use this file

Talk in this order:

1. What it is
2. The one goal
3. How that goal is achieved
4. The hard problems, and the choices that fix them
5. What is still unfinished
6. Data structures and patterns, if they ask
7. Questions they may ask

Do not start with libp2p. Do not start with Go packages. Start with the problem a poker site creates, then say what this program does instead.

If they only have ten minutes, stop after section 4. If they have forty-five, walk sections 1–5, then let them pick from section 7.

---

## 1. What this project is

This is a peer-to-peer Texas Hold’em engine. There is no dealer and no game server. Every laptop runs the same program.

Cards are shuffled and dealt with mental poker: layered encryption so that no single player is the dealer, and no one can peek at another player’s hole cards.

Each laptop keeps its own copy of the table. When something happens, it does not take another machine’s word for the new state. It checks the message, then updates its own copy.

`poker host` is not a server. Host only means “I started first, so the others have a place to connect.” After that, Alice, Bob, Carol, and Dave are equal peers.

There are two ways to play:

- **Live multiplayer.** `poker host` / `poker join`. Default path uses real crypto dealing. Needs 3–9 seats.
- **Local vs bots.** One process, no network, a normal deck. Useful for testing the poker rules.

`--no-crypto` is a debug switch. Every laptop shuffles the same public seed and can see every card. It is not the product. Mixed tables (some crypto, some not) are refused.

Chips in the demo are numbers in memory. A Solidity escrow contract exists for later money settlement. The live game never talks to Ethereum.

---

## 2. The main goal

A normal online poker site is a trusted house. You have to believe:

- they shuffled fairly,
- they did not look at hole cards,
- they will not rewrite the pot,
- the table stays up only if their server stays up.

The goal of this project is a table with **no house**.

Write this sentence on the board if they let you:

> If everybody starts from the same state, and applies the same actions in the same order, everybody ends in the same state.

That is the public half.

The private half is:

> Cards are produced by a joint lock-and-mix protocol. Only the owner of a hole card can take the last lock off. Community cards are unlocked by everyone.

Those two together are the product: same pots and winners on every honest laptop, without a dealer who can see or choose the deck.

What this is **not**:

- Not a 10,000-player tournament. A poker table is 3–9 people. Scale-out would be many independent tables, not one bigger mesh.
- Not a blockchain game. The chain is only for money later. The hand itself never waits for a block.
- Not Byzantine agreement on every fold. Honest copies stay in lockstep. A liar who sends two stories can be detected after the fact. The program does not run a vote on every action.

---

## 3. How we achieve that

Think of four jobs on every laptop. They are separate on purpose.

| Job | What it does | What it does not do |
|---|---|---|
| **Network** | Find friends, sign messages, shout into rooms, open one-to-one pipes | Does not decide the winner |
| **Crypto** | Lock, mix, and peel cards | Does not know about bets |
| **Game machine** | Hold’em rules: turns, pots, showdown | Does not open sockets and does not know about SRA |
| **Fault** | Heartbeats, timeout votes, rebuild a missing key | Does not shuffle |

The punchline:

> The poker rules have no sockets. Networking only delivers signed bytes. Mixing those two is how a central server happens by accident.

### Finding people

On a LAN, mDNS is “I run poker. Is anyone else here?” It finds machines, not tables. The table name `--table friday` is a gossip room. Until there is a TCP pipe, that room is empty.

A friend on another network pastes an address with `--peer`. Relays are off. NAT is weak. This is a LAN demo first.

Each person has an identity key (Ed25519). From it, libp2p derives a Peer ID. That is the address on the network. Display names are just labels on the screen. Every public shout is signed with the private part. Anyone can check it with the public part.

Each person also has a second key pair, SRA: `(e, d)`. Identity signs messages. SRA locks and unlocks cards. `e` goes out in the join shout. `d` never goes out.

### Sitting down

Joins are signed shouts on `poker/table/friday`. Gossip is not a history book. Old shouts disappear, so each laptop repeats its join every 2 seconds with the **same** first timestamp until the table is full.

Each Lobby sorts seats by that timestamp, then by Peer ID if two times match. Same shouts, same order. Nobody sends a copy of “the whole waiting room.”

When the seat count hits the configured number, every laptop shouts `PLAYER_READY`. Alice is not in charge of that.

### Cards without a dealer

The starting deck is fifty-two known numbers, the same on every laptop. Anyone can rebuild that list. What must stay secret is the mixing.

Seat 0 locks every card with their `e`, secretly rearranges, fingerprints the result, and publishes `SHUFFLE_STEP`. The next seat does the same to that pile, and so on. After the last step, all four laptops hold the same fifty-two numbers, each locked by all four `e`s, in an order nobody is supposed to map back.

Encryption commutes: locks can come off in any order. That is why peels work even though the mix order was different.

To deal Bob a hole card: Alice, Carol, and Dave each take their lock off in public, with a small proof that the peel was real. Bob takes his last lock off only on his laptop. He does not publish that last number. The others still see a locked leftover. They cannot turn it into a face.

Board cards are different. Nobody is skipped. After the last peel, every laptop can read the face.

Burns are skipped indexes. They are never peeled. That keeps the crypto slots lined up with real Hold’em dealing.

### Playing the hand

Each laptop then builds its own `GameState`: stacks, blinds, pot, whose turn. It copies only its own two faces. Opponent holes stay blank.

Blinds are posted locally by the same rule. Nobody publishes a blind shout.

A click becomes a signed `PLAYER_ACTION` on the table room: who, what, how much, and a sequence number. Gossip can arrive out of order, so each laptop parks early actions and applies them only when the numbers line up.

Nobody publishes “now it is Alice’s turn.” After Dave’s action is applied, the pointer steps by itself.

When a betting round ends, the machine waits for the next street. It does not deal from a pile in RAM. The peels happen, then `ApplyStreet` writes the faces. Showdown is the same idea for remaining hole cards.

Winners are not announced. They are computed. Same start state, same cards, same rule — same payouts.

### If someone goes quiet

Heartbeats live on a second room, `poker/heartbeat/friday`, so “I am alive” does not fight with “I raise.” After a short silence the others vote that the seat is gone and fold it for the current hand.

If the shuffle already finished, they can rebuild that player’s `d` from Shamir shares that were sent one-to-one at table start. One surviving seat then peels on their behalf.

If they vanish in the middle of a shuffle, the hand dies. The mix was only in their RAM. Shares can rebuild a key. They cannot rebuild a mix that was never published.

The dead seat is not dropped. Next hand still wants that lock. Restart the table for a new group.

---

## 4. Problems, and the design choices that fix them

This is the section that sells the project. Pair each problem with the choice, then say what you still cannot claim.

### Problem 1: Gossip is a shout, not a log

**What goes wrong.** GossipSub can deliver “raise, then call” as “call, then raise.” Those are not the same pot.

**Choice.** Two sequence numbers.

- Envelope sequence: per sender. Drop simple replays.
- Action sequence: table-wide. Park out-of-order actions in a map. Apply only a contiguous prefix.

The current actor assigns the next action number from their own copy. Under the honest assumption that everyone already applied the earlier ones, nobody needs a leader to pick the number.

**Still true.** A missing number waits forever. There is no “please resend action 7.” Two different actions with the same number overwrite the parked slot. This is an optimistic baton, not Raft.

### Problem 2: One dealer would see and choose every card

**What goes wrong.** If Alice shuffles a plaintext deck, she knows every hole card and can stack the order.

**Choice.** Every seat encrypts the whole deck with SRA and privately permutes it. One honest mix is enough to randomize, as long as that mix is real. Hole cards keep the owner’s last lock on until they peel it at home.

**Still true.** The fingerprint only binds the bytes that were published. It does not prove “this pile is a re-encryption and permutation of the last pile.” A proper verifiable shuffle is not in this program. Four honest copies stay in lockstep. That is the demo, not a proof against a malicious shuffler.

### Problem 3: A peel is just a number. Garbage would brick the hand

**What goes wrong.** Dave can publish junk instead of taking his lock off. The next person has nothing honest to peel.

**Choice.** Each peel carries a Fiat–Shamir proof: the same hidden exponent links a public base to `H`, and the old number to the new number. Junk that is not that lock coming off gets caught. The proof is not the rank and not `d`.

**Still true.** `H` is supplied by the prover. The verifier does not check that `d` is the inverse of the `e` from the lobby. The proof says “I used one exponent consistently,” not “I used this seat’s advertised key.”

### Problem 4: Late friends never hear the first join

**What goes wrong.** Gossip is not a membership database. Bob connects after Alice’s first shout. That shout is already gone.

**Choice.** Repeat `JOIN_TABLE` every 2 seconds with a frozen timestamp, until the table is full. Duplicates are ignored. Seat order does not jump.

**Still true.** This only helps the waiting room. After a hand starts, a late or restarted laptop cannot catch up. `GameStateSync` exists in the protobuf and is not wired.

### Problem 5: Heartbeats would steal sequence numbers from the table

**What goes wrong.** One outbound counter is shared. If heartbeat 11 arrives before table message 10, and both rooms share one “last seen” number, 10 is dropped as old.

**Choice.** Two rooms, two receive loops, two watermarks. Heartbeats stay out of the evidence notebook.

### Problem 6: One silent player can freeze every later card

**What goes wrong.** Every card still has that player’s lock. Folding them is not enough if the flop still needs their `d`.

**Choice.** At setup, each player splits `d` into shares and sends them one-to-one. After a timeout vote, survivors pool shares, rebuild `d`, and a designated seat peels for them.

**Still true.** About half the table can collude and rebuild a key even before anyone leaves. Shares are table-level, so a rebuilt key is burned for later hands. Mid-shuffle dropout still aborts.

### Problem 7: The bytes you heard may have hopped through Carol

**What goes wrong.** Noise only proves the last hop. Carol could forward a fake Alice shout.

**Choice.** Sign the envelope: type, sender, sequence, time, inner bytes. Check the signature with the public key pulled out of the claimed Peer ID. The hop does not matter. The signature still says Alice.

### Problem 8: Private replicas cannot be identical before showdown

**What goes wrong.** Alice’s copy has Alice’s hole cards. Bob’s copy has Bob’s. If the engine required a full identical `GameState`, crypto dealing would be impossible.

**Choice.** `StartHandCrypto` allows empty opponent holes. The machine pauses at “waiting for street,” takes public cards as inputs, and pauses at showdown until remaining holes are publicly peeled.

The public projection should match: phase, board, bets, stacks, whose turn. Whole-state compare before showdown would be the wrong test.

### Problem 9: Waiting for peels must not freeze a timeout fold

**What goes wrong.** A worker waiting two minutes for the flop used to hold the same lock the timeout path needs to fold a silent player.

**Choice.** Start the peel job, drop the machine lock, wait, then take the lock again before writing cards.

**Still true.** Two of those workers can overlap. There is no “only one advance at a time” guard yet.

### Problem 10: A map has no seat order

**What goes wrong.** Go maps iterate in a random-looking order. Dealer, shuffle turns, and card slots would disagree.

**Choice.** Canonical slice: join time, then Peer ID. Every later recipe (who shuffles first, who peels, which slot is Bob’s first card) is derived from that slice. Shuffle always starts at seat 0. Dealer rotates for blinds and hole slots only.

---

## 5. The story you tell in sixty seconds

Use this almost verbatim.

> “A poker site is a trusted dealer. I built a table where there is no house. Every laptop is a full replica of Texas Hold’em. Public events go over a signed gossip mesh. Cards are jointly locked and mixed with commutative encryption, so I never hand anyone a plaintext deck. Each node applies the same ordered actions to the same rules, so honest players compute the same pot. If someone disconnects after the shuffle, we vote them folded and can rebuild their card key from shares. If they vanish during the mix, we abort, because a secret shuffle cannot be rebuilt. It is a LAN prototype, not a casino and not PBFT. The interesting work is the replica plus the dealing protocol, kept as two separate layers.”

Then stop. Let them pick a layer.

---

## 6. What is still unfinished (say this before they find it)

Honesty here is how you sound senior. Do not claim the chain, catch-up, or a malicious-proof shuffle.

### Agreement

There is no quorum certificate for seats, actions, or winners. `HAND_RESULT` is logged. Nobody compares pot results. A network split can produce two different local stories. When the network heals, there is no merge.

### Identity bugs

Unsigned envelopes can be accepted. If a Peer ID parses but its key cannot be pulled out, verification can fail open. A payload can name a different player than the envelope signer. Direct streams know the Noise peer, but do not always check that against the envelope. The on-disk identity bytes are 64 random bytes, not a proper Ed25519 seed-plus-public construction.

### Crypto gaps

Shuffle commitment is “hash matches these bytes,” not “this is a valid re-encrypt-and-permute of the last deck.” Peel proofs are not bound to lobby `e`. Shamir shares are not verifiable. Mixed with the half-table threshold, collusion can steal `d`.

### Recovery gaps

Timeout math rounds instead of taking a ceiling. At three seats, one vote can confirm. A live but uncooperative peer can keep heartbeats going and refuse to act. `ActionTimeout` is loaded and not used. The dead seat stays on the next shuffle.

### Loss and reorder

If envelope 12 arrives before 11, 11 is dropped as old. The action sequencer never asks for a missing number. Early crypto buffers are capped; overflow is silent. Equivocation scanning usually cannot see both conflicting shouts, because the first one already used that sequence slot. The scan callback is not installed on the live path.

### Money

Chips vanish when the process exits. The Go chain client returns fake receipts. The Solidity contract is a prototype: signature threshold is not actually required, payouts happen before the challenge window, and slash evidence is not judged. Do not put real funds in it.

### Product limits

No reconnect. No leave protocol. No dropping a seat for the next hand. No DHT, no relays, weak NAT. Several config flags are ignored (`enable_mdns`, `max_peers`, `action_timeout`). Some betting edge cases (all-in raise, unmatched folded chips, busted players coming back) are still wrong in the rule engine. Tests are strong on units and in-memory buses. They do not launch several real `runP2PMode` processes over GossipSub.

### What you would ship next (pick a short list)

If they ask “what would you do before production?” say these in this order:

1. Fix identity generation and bind every payload to the authenticated sender.
2. Add a real verifiable shuffle, and bind peel proofs to lobby public keys.
3. Replace the optimistic action baton with a certified log plus catch-up / retransmission.
4. Repair timeout membership and thresholds; add a progress timeout for a live stall.
5. Rotate keys per hand; shrink the roster after a confirmed leave.
6. Either drop the escrow claim, or build a real chain client and a contract that waits and checks evidence.

---

## 7. Data structures (and why each exists)

You do not need to recite this. Have it ready when they ask “what lives in memory?”

### Network and membership

| Thing | Shape | Why |
|---|---|---|
| `Node` | One struct of host, gossip, lobby, log, streams, callbacks | One process = one player. Startup and shutdown have a single boundary |
| libp2p peerstore | Peer ID → addresses | Phone book. Not the seat list |
| `Lobby.seats` | `map[peerID]*SeatInfo` | Dedup joins, update ready flags |
| Canonical seats | sorted `[]*SeatInfo` | Maps have no order. Dealer, shuffle, and peels need one |
| `StreamPool` | `map[peer.ID]Stream` | Reuse one `/poker/1.0.0` stream per friend. No second TCP handshake |
| Public-key cache | `map[string]ed25519.PublicKey` | Avoid extracting the key from a Peer ID on every shout |

### Ordering and evidence

| Thing | Shape | Why |
|---|---|---|
| Envelope watermark | two `map[sender]lastSeq` | Constant-space replay check. One map for the table room, one for heartbeats |
| `actionSequencer` | `nextSeq` + `map[seq]*PlayerAction` | Gossip is unordered. Release only a contiguous prefix |
| `Gamelog.entries` | `[]*Envelope` | Arrival-order notebook for hashing |
| `Gamelog.byKey` | `map["sender:seq"]struct{}` | Duplicate set. Empty struct uses no extra value storage |
| Envelope | protobuf: sender, seq, time, payload, signature | The hop is not the author. The signature is |

### Game

| Thing | Shape | Why |
|---|---|---|
| `GameState` | one struct | One transition updates a coherent table |
| `[]*Player` | ordered slice | Seat order matters. A map would not |
| `[2]Card` / evaluator `[7]Card` | fixed arrays | Hole cards and 7-card eval have known size |
| `[]PotSlice` | ordered levels | Main pot then side pots, each with an eligible set |
| `map[string]int64` payouts | sparse by player id | Not every seat wins |

### Crypto

| Thing | Shape | Why |
|---|---|---|
| `*big.Int` | arbitrary integers | 2048-bit modular math does not fit in `int64` |
| `SRAKey{E,D,P}` | three integers | Public lock, private unlock, shared prime |
| `Keyring` | local full key + public-only map + seat slice | No API returns another player’s `d` |
| `ShuffleSession` | `nextIndex`, current deck, pending/applied maps | One shuffler at a time. Park early steps. Reject a second different step |
| `DealSession` | list of `peelJob`, expected peeler index | Turns a long protocol into small turns |
| `Commitment` | hash + nonce | Bind the published pile |
| `ZKProof{A,B,S,H}` | four integers | Prove a peel without publishing `d` |

### Fault

| Thing | Shape | Why |
|---|---|---|
| liveness map | `map[peer]*PeerLiveness` | Last seen, by id |
| timeout vote | per accused peer, inner `map[voter]bool` | Dedup voters |
| Shamir store | my share per owner; reconstruction slice per missing owner | Pool shares until the threshold |

### Concurrency

| Thing | Shape | Why |
|---|---|---|
| `sync.Mutex` / `RWMutex` | locks around maps and FSMs | Short mutations. Shared memory behind locks |
| `waitGate` | close-only channel plus stored error | Sleep until shuffle/peels finish, or fail |
| `readyCh` + `sync.Once` | one close | “Everyone is ready” is a one-time event |
| `notifyCh` capacity 16 | lossy wakeup | UI rereads current state. A dropped ping is fine |
| `atomic.Int64` | hand number | Heartbeat loop needs a cheap read |

---

## 8. Design patterns (in plain words)

These are the names interviewers like. Map each back to a real object so you do not sound like a vocabulary list.

**Replicated state machine.** Every laptop is a full copy of the Hold’em reducer. You broadcast inputs (“Dave raises 40”), not outputs (“the pot is 80”). Same start + same ordered inputs → same public table.

**Finite state machines.** `game.Machine` walks phases (waiting, pre-flop, awaiting street, showdown). `ShuffleSession` walks seat 0..n-1. `DealSession` walks peel jobs. Each only accepts the next legal turn.

**Layered architecture.** `internal/game` has no networking and no SRA. `internal/crypto` does not decide bets. `cmd/poker` is the glue. That is why you can unit-test pots without libp2p.

**Coordinator.** `CryptoHand` sits above shuffle and deal sessions. It is the per-hand conductor. `FaultManager` is the same idea for liveness, votes, shares, and slash records.

**Facade.** `network.Node` is the one object the rest of the program talks to. Inside it: host, gossip, lobby, log, streams.

**Callback / observer.** `OnShuffleStep`, `OnPlayerAction`, `OnHeartbeat`, and the rest. The node does not import the TUI or the machine. The top-level loop installs handlers before `Start()`, so early messages are not dropped.

**Sequencer / gap buffer.** `actionSequencer` is the classic “buffer until the next number arrives.” Same idea as a TCP receive window, but only for poker actions, and with no retransmission.

**Object pool.** `StreamPool` keeps one logical stream per peer.

**Barrier.** Lobby ready channel, and crypto `waitGate`. Close the channel, every waiter wakes.

**Capability-limited key store.** `Keyring` can encrypt for anyone and decrypt only for you.

**Idempotent dual delivery.** A peel is gossiped and also sent one-to-one. Duplicates are ignored. Gossip is what counts. The stream is a shortcut.

**Detect, do not prevent (equivocation).** Two signed stories with the same sequence are evidence. Gossip cannot make the second story unsendable. Prevention would mean a BFT broadcast layer.

**Pure function for scoring.** Hand ranking and pot slicing are deterministic functions of the cards and contributions they already have. Nobody sends “you won.”

**Adapter.** The TUI turns keys into `game.Action` and draws a `GameState` it does not own. `HandCoordinator` / `CryptoGame` is an in-process adapter for tests: all keys on one machine. That is not the live P2P path. Do not confuse the two.

---

## 9. How the repo is split (if they open the laptop)

```
cmd/poker          orchestration, TUI wiring, action sequencer, live crypto loop
internal/network   libp2p host, gossip, lobby, envelopes, streams, CryptoHand
internal/crypto    SRA, keyring, shuffle FSM, deal FSM, commitments, ZK proofs
internal/game      Hold’em machine, pots, 7-card eval  (no sockets)
internal/fault     heartbeats, timeout votes, Shamir store, slash records
internal/tui       terminal view
internal/chain     prototype escrow client (not called by host/join)
contracts/         PokerEscrow.sol (prototype, not live)
```

Live path: `runP2PMode` in `cmd/poker`. Helper path: `HandCoordinator` holds every `d` in one process. Say that difference out loud if they look at `crypto_game.go`.

---

## 10. Questions an interviewer might ask

Answers are short on purpose. Expand only if they follow up.

### What is the core architecture?

Each player runs the same Go process. libp2p handles pipes, Noise, mDNS, streams, and GossipSub. A node decodes signed protobuf envelopes. A coordinator feeds ordered actions and peeled cards into a deterministic poker machine. The terminal only draws that local state. There is no central database.

### Why not a server with TLS?

TLS hides the pipe from outsiders. It does not stop the server from stacking the deck or peeking at hole cards. The whole point is that there is no such server.

### Why gossip instead of everyone-to-everyone TCP?

At nine seats, a full mesh is fine. Gossip still helps: a shout can hop, heartbeats can live on their own room, and membership can change. The cost is unordered, best-effort delivery. Sequencing and signatures sit on top because of that, not instead of it.

### How do peers agree on a raise?

The current actor applies it locally, stamps the next table-wide sequence, and publishes. Others park by sequence and apply only the next contiguous number to the same machine. It is an optimistic baton. Production would add acks, conflict certificates, and repair.

### How do you stop one player from seeing every card?

There is no dealer. Every seat locks and secretly mixes the pile. For a hole card, everyone except the owner peels in public. The owner peels last, at home, and does not publish that last number. Observers still see a locked leftover.

### What does the zero-knowledge proof actually prove?

That the same hidden exponent links `g` to `H` and the old ciphertext to the new number. It catches a swapped result on an otherwise honest proof. It does not, today, prove that exponent is the inverse of the lobby `e`. I would add that binding before claiming malicious security.

### Is the shuffle verifiable?

No. The hash only says “these are the bytes I claimed.” It does not prove a permutation of the previous pile. Honest programs stay in lockstep. A malicious shuffler can substitute an invalid deck. A production design needs a verifiable re-encryption shuffle.

### What happens when someone disconnects?

Heartbeats stop. After a vote, they are folded for this hand. If the shared pile already exists, survivors rebuild `d` and one seat peels for them. Mid-shuffle, abort. They are not removed from the next hand. Restart the table to change the group.

### What if they keep sending heartbeats but refuse to act?

Today the table can stall. Heartbeats are liveness of the process, not progress of the protocol. I would add an action/progress timeout separate from heartbeat timeout.

### How are duplicates and out-of-order messages handled?

Per-sender envelope watermark. Table-wide action map. Crypto sessions park future turns and reject conflicting duplicates. Weakness: a highest-seen watermark drops 11 if 12 arrived first. I would use a receive window or per-message ids plus anti-entropy.

### What is the concurrency model?

Dedicated receive loops for table and heartbeat, a periodic log scan, heartbeat send/monitor, Bubble Tea’s UI loop, plus libp2p internals. Shared table state sits behind mutexes. Channels are for “this phase finished” and UI wakeups. Mostly shared memory behind locks, not “one goroutine owns everything.”

### What happens in a network partition?

No safe merge. Each side may vote, stall, or continue from different facts. I would refuse to make progress without a quorum, then resume from a signed common prefix. Right now that is not built.

### How does this scale?

Action messages are small. Crypto is the cost: `n` full 2048-bit decks, about `2n(n-1)` hole peels, about `5n` board peels, plus proofs, often sent twice. Fine for a home table. Not a service for thousands of seats on one mesh. Scale-out is many independent tables.

### Is this CAP?

During a split there is no majority commit. The honest behavior is to stall on a gap rather than fork the pot. That is CP-shaped, without an actual quorum. I would not call it a CAP solution.

### Why two sequence numbers?

Envelope seq is “have I already accepted this sender’s packet 7?” Action seq is “is this the next mutation of the table?” Mixing them would let a heartbeat or a join consume a number the raise needed, or the reverse.

### Why not put every action on Ethereum?

Human poker cannot wait for block time, gas, and reorgs. The contract should only see a signed outcome later. The demo does not even do that yet. Chips are local counters.

### Why SRA instead of a commit-reveal shuffle?

Commit-reveal can pick a seed. Everyone who knows the seed can reconstruct the whole deck, including hole cards. That is `--no-crypto`. SRA keeps a lock that only the owner can finish.

### Why Shamir instead of voiding the hand on any dropout?

After the pile exists, a dropout should not hold the flop hostage. Shares are the way to finish peels. The trade-off is collusion and the fact that a rebuilt key is burned for later hands.

### Why is the game package pure?

So pots and ranking can be tested with a fake RNG, and so crypto cannot accidentally publish opponent cards through the rule engine. The hard glue is the coordinator in `main.go`. That is also where most of the remaining races live.

### What tests exist?

Strong unit tests for cards, pots, SRA round trips, proof tampering, shuffle/deal FSMs, and basic gossip. Multi-peer crypto tests use an in-memory fake bus. There is no test that launches several real host/join processes, completes a crypto hand, kills one, reconstructs `d`, and compares public state.

### What would you change without rewriting Hold’em?

Replace the log transport: same `ApplyAction`, but a Raft or BFT log underneath. Keep the reducer. Keep SRA above it. That is the point of the layers.

### Host vs join — who is in charge?

Nobody. Host is the first listener. Dealer is index 0 of the sorted seat list, which may not be the person who typed `host`. Shuffle always starts at seat 0 even when the dealer button has moved.

### Can a fifth person sit down?

Not after the table is full. They can still get a TCP pipe. `HandleJoin` will not give them a seat. There is no spectator mode.

---

## 11. How to sound like you built it

**Do**

- Say “replicated state machine,” then immediately translate: same start, same ordered actions, same public table.
- Separate envelope sequence from action sequence without being asked.
- Separate “host” from “dealer” from “seat 0 shuffles.”
- Volunteer gaps: no verifiable shuffle, no catch-up, chain not live, mid-shuffle abort, identity bugs.
- Scope to one table immediately if they start talking about PokerStars.

**Do not**

- Draw a server in the middle.
- Claim PBFT, Raft, or “blockchain consensus” for folds.
- Claim live ETH payouts.
- Pretend 2048-bit SRA is fast. Several seconds of shuffle is expected.
- Scale-story a million players on one GossipSub topic.
- Dive into Bubble Tea widgets unless they ask about the UI.
- Confuse `CryptoGame` (all keys on one process) with the live P2P path.

**If they hijack into “design a whole poker company”**

Matchmaking and wallets are a different system. This table is one stateful shard. A lobby service would hand out a `table_id` and a bootstrap address. Each table is this design. Escrow would be per table. Then stop.

---

## 12. One-page cheat sheet

Keep this in your head. It is the whole project.

| Layer | Honest-path promise | Honest limit |
|---|---|---|
| Discovery | mDNS or a pasted address finds a pipe | No DHT, weak NAT, no relays |
| Gossip | Signed public events reach the table room | Unordered, not durable, not consensus |
| Lobby | Same seat list from the same joins | Sender timestamps; no catch-up later |
| SRA shuffle | No one starts with a plaintext order | Mix is not a verifiable shuffle |
| Hole peels | Only the owner finishes the last lock | Proof not bound to lobby `e` |
| Game machine | Same public rules, same pots | Some betting edges still wrong |
| Sequencer | Out-of-order gossip becomes one action order | Missing seq never repaired |
| Heartbeats + Shamir | Finish a hand after a post-shuffle drop | Mid-shuffle dies; seat stays forever |
| Escrow | Designed on paper | Not wired; contract not safe for funds |

Opening line:

> “I built a no-house Hold’em table: equal replicas, signed gossip, and mental-poker dealing. The demo is lockstep play with hidden hole cards on a LAN. The remaining work is making the shuffle and the log hold up against a liar, then plugging money in — not rewriting the poker rules.”
