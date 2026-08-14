# How the Decentralized Poker Engine Works

A long-form walkthrough of this repository, written for a reader who already knows computer science (programs, data structures, cryptography at a high level, how a poker hand is supposed to go) but is **new to computer networking**. It starts with “what is a LAN?” and ends with a second-by-second account of four people playing one Texas Hold’em hand on this engine.

You do not need to have read the other docs first. If you later want the compact architecture brief, see [`SYSTEM_DESIGN_OVERVIEW.md`](./SYSTEM_DESIGN_OVERVIEW.md). If you want the interview-shaped version, see [`SYSTEMS_DESIGN_INTERVIEW.md`](./SYSTEMS_DESIGN_INTERVIEW.md). This file is the teaching version.

**What this project is, in one sentence.** Four (or three to nine) laptops on a local network run the same Texas Hold’em rules, gossip signed actions to each other, jointly shuffle an encrypted deck so nobody is the dealer, and each player sees only their own hole cards until showdown.

---

## Table of contents

1. [Who this is for, and how to read it](#1-who-this-is-for-and-how-to-read-it)
2. [What the project is trying to do](#2-what-the-project-is-trying-to-do)
3. [What you learn by studying this codebase](#3-what-you-learn-by-studying-this-codebase)
4. [Networking from zero](#4-networking-from-zero)
5. [Client–server vs peer-to-peer](#5-clientserver-vs-peer-to-peer)
6. [What “decentralized” means here](#6-what-decentralized-means-here)
7. [Texas Hold’em, as the engine models it](#7-texas-holdem-as-the-engine-models-it)
8. [Why a normal online poker site is a trusted dealer](#8-why-a-normal-online-poker-site-is-a-trusted-dealer)
9. [Map of the repository](#9-map-of-the-repository)
10. [The game engine](#10-the-game-engine)
11. [Identity, addresses, and encrypted connections](#11-identity-addresses-and-encrypted-connections)
12. [Finding other players (discovery)](#12-finding-other-players-discovery)
13. [Two ways messages travel: gossip and direct streams](#13-two-ways-messages-travel-gossip-and-direct-streams)
14. [The lobby](#14-the-lobby)
15. [Replicated state machines and action order](#15-replicated-state-machines-and-action-order)
16. [Mental poker: hiding cards without a dealer](#16-mental-poker-hiding-cards-without-a-dealer)
17. [Faults: silence, folds, and reconstructing a missing key](#17-faults-silence-folds-and-reconstructing-a-missing-key)
18. [The terminal UI](#18-the-terminal-ui)
19. [On-chain escrow (designed, not live)](#19-on-chain-escrow-designed-not-live)
20. [How the pieces snap together](#20-how-the-pieces-snap-together)
21. [A full four-player hand, step by step](#21-a-full-four-player-hand-step-by-step)
22. [What if someone cheats or disconnects during that hand](#22-what-if-someone-cheats-or-disconnects-during-that-hand)
23. [Local mode (one process, no network)](#23-local-mode-one-process-no-network)
24. [Common misconceptions](#24-common-misconceptions)
25. [Honest limitations](#25-honest-limitations)
26. [Glossary](#26-glossary)

---

## 1. Who this is for, and how to read it

Assume you can write a program, you know what a hash function and a public/private key pair are, and you know how Texas Hold’em is played. Do **not** assume you know what an IP address is for, why a port number exists, what TCP guarantees, or why “just send the game state to everyone” is a bad idea on a mesh of peers.

Sections 4–6 are the networking primer. Sections 10–19 explain each subsystem in isolation. Section 20 is the wiring diagram. Section 21 is the payoff: Alice, Bob, Carol, and Dave sitting at one table. Sections 23–24 cover local-vs-bots mode and mistakes this design is easy to make.

Whenever a term is used in a networking sense, it is defined on first use. A glossary sits at the end.

---

## 2. What the project is trying to do

A classic online poker site is a **server**. You open a browser, the site shuffles, deals, tells you whose turn it is, and pays the winner. You have to trust that operator:

- that they shuffled fairly,
- that they did not peek at hole cards,
- that they will not rewrite the pot,
- that the site staying up is the only way the table stays up.

This repository’s goal is a **table with no house**. Every seated program is a full replica of the same Hold’em engine. There is no process whose job is “be the dealer.” Cards are produced by a cryptographic protocol called **mental poker** (here: Shamir–Rivest–Adleman commutative encryption, abbreviated **SRA**). Public facts — whose turn it is, how big the pot is, who won — are computed independently on every laptop from the same ordered list of actions.

Concrete product goals:

| Goal | What that means in this repo |
|---|---|
| Play real Hold’em | Blinds, hole cards, flop / turn / river, check / call / raise / fold / all-in, side pots, 7-card evaluation, multiple hands |
| No game server | `poker host` is only the first listener. After that, every peer publishes and subscribes |
| Same outcome on every honest machine | Identical initial state + identical ordered actions → identical `GameState` |
| Hidden hole cards | You decrypt your own last encryption layer locally. Opponents’ holes stay empty on your replica until showdown |
| Unbiased shuffle | Every player encrypts **and** secretly permutes. One honest permuter is enough to randomize the order |
| Authenticated talk | Every gossip message is Ed25519-signed. You cannot impersonate another seat |
| The table can survive one crash **after** the shuffle | Timeout-fold the silent player, reconstruct their private exponent from Shamir shares, finish the remaining decrypts |
| Optional money later | A Solidity escrow contract exists. The Go client does **not** talk to Ethereum yet. Chips in the demo are local counters |

Non-goals (on purpose):

- Not a 10,000-player tournament. A poker table is 3–9 people. Scale-out would be **many independent tables**, not a bigger mesh.
- Not Byzantine agreement (PBFT / HotStuff) on every fold. Honest nodes apply a total order of signed actions; a double-talker is **detected** after the fact, not prevented by a quorum on every action.
- Not a public-internet product. Discovery is LAN multicast or a copied address. Relays are off. NAT traversal is UPnP only.
- Not 2-player P2P. Shamir recovery after a disconnect needs at least three seats. Heads-up is local-versus-bots only.

Two run modes exist:

1. **P2P (the interesting one):** `poker host` / `poker join`. Default is cryptographic dealing. `--no-crypto` is a debug mode where every node shuffles the same public seed and **all cards are visible**.
2. **Local:** one process, you versus bots. No network, no SRA. Useful for testing the rules engine and the UI.

---

## 3. What you learn by studying this codebase

If you only remember five ideas from this project, make them these:

1. **A LAN is a neighborhood of machines that can talk without going through the public internet.** This table is designed for that neighborhood. The same ideas extend to the internet, but the hard part then becomes *finding* each other and *getting through firewalls*, not the poker rules.

2. **Peer-to-peer is not “no networking.”** It is a different shape: every program is both a sender and a receiver. Someone still has to go first so the others have an address to dial. That first process is not a server in the game-logic sense.

3. **Gossip delivers bytes; it does not deliver a total order.** If you apply “raise” before “call” on one machine and the other way around on another, the pots diverge and the hand is ruined. The fix is an application-level **sequencer**.

4. **Public game state can be a pure function.** The Hold’em engine has no sockets. Cards in crypto mode are *inputs* (`ApplyStreet`, `ApplyHoleReveal`), not something the engine samples from a local deck. That is what makes every replica compute the same winner.

5. **Hiding cards without a dealer is a protocol, not a role.** Commutative encryption lets everyone lock the deck; selective unlocking lets only the rightful player see a hole card; zero-knowledge proofs stop a cheater from publishing garbage as a “decrypt.”

Along the way you also see: signed envelopes vs hop encryption, mDNS, libp2p multiaddrs, Shamir secret sharing as a liveness tool, and why money (if you ever add it) should sit off the hot path in an escrow contract.

---

## 4. Networking from zero

This section is the vocabulary you need before the rest of the file will make sense. None of it is poker-specific.

### 4.1 What a network even is

A **network** is a set of computers that can exchange messages. A message here is just a bag of bytes with a destination. Your laptop already does this constantly (web pages, Discord, game updates). This project uses the same physical wires and Wi-Fi; it just refuses to put a poker company in the middle.

### 4.2 LAN vs WAN vs the internet

**LAN** means **Local Area Network**. Think: the Wi-Fi in your apartment, the ethernet in a lab, two laptops sharing a hotspot. Machines on a LAN can usually reach each other directly. They share a private address range (classic examples: `192.168.x.x` or `10.x.x.x`). A packet from Alice’s laptop to Bob’s laptop on the same Wi-Fi often never leaves the building. It hits the home router (or the switch) and comes back out to Bob.

**WAN** means **Wide Area Network** — sites that are far apart, typically talking across the public internet.

This engine’s happy path is a LAN:

- Peers can **discover** each other automatically (a protocol called mDNS, explained in §12).
- Latency is milliseconds, not hundreds of milliseconds.
- Nobody has to punch through a carrier-grade NAT for the demo to work.

You *can* join from another network if you copy the host’s address and that address is reachable (port forwarded). That is a WAN use of the same code, not a different architecture. There is no DHT (distributed hash table) and no relay servers in this repo, so “my friend is behind a strict dorm NAT” is a real operational problem, not a solved one.

### 4.3 IP addresses: names for machines on a network

An **IP address** is a number that identifies a machine *on a particular network*. IPv4 looks like `192.168.1.100`. It is not a person’s name and it is not permanent. Your laptop may get a new one when you reconnect to Wi-Fi.

On a LAN, the router hands out these addresses (DHCP). From Alice’s point of view, Bob is “whatever IP the router gave Bob.” From the public internet’s point of view, your whole apartment often looks like **one** address — the router’s public IP. That is why WAN is harder: Bob cannot dial `192.168.1.100` from another city. That number is not globally unique.

This project mostly uses IPv4. You will see it inside **multiaddrs** (self-describing addresses) such as:

```
/ip4/192.168.1.100/tcp/9000/p2p/12D3KooW...
```

Read that left to right: “use IPv4, this host, TCP, this port, and the peer you should find there is this ID.”

### 4.4 Ports: names for programs on a machine

One machine runs many programs. A **port** is a 16-bit number that says *which program* should receive an incoming connection. Web servers traditionally use 80 or 443. This poker process, by default, listens on **TCP port 9000**.

If Alice and Bob run two copies on the **same** laptop (a common way to demo), they cannot both bind 9000. The second process must pick another port (`--listen /ip4/0.0.0.0/tcp/9001`). `/ip4/0.0.0.0` means “accept connections on every local IPv4 interface,” not “the internet at large.”

### 4.5 TCP: a reliable pipe of bytes

**TCP** (Transmission Control Protocol) is the workhorse transport of the internet. If Alice opens a TCP connection to Bob:

- Bytes arrive **in order**.
- Lost packets are **retransmitted**.
- You get a stream, not “messages.” If Alice writes 100 bytes then 50 bytes, Bob might read 150 at once, or 20 then 130. The application must **frame** messages (this project prefixes each message with a 4-byte length).

TCP does **not** encrypt. Anyone on the path can read the bytes. It also does not tell Bob, by itself, that the far end is really Alice rather than an impostor.

A picture of one poker message on that pipe:

```
[ 4 bytes: length N, big-endian ] [ N bytes: protobuf Envelope ]
```

The Envelope contains a type tag, Alice’s peer id, a sequence number, a timestamp, a payload (itself protobuf: a fold, a shuffle step, …), and an Ed25519 signature. Maximum `N` is 4 MiB so a hostile peer cannot ask you to allocate a gigabyte. Shuffle steps are the large ones: 52 × 256-byte integers, plus a 32-byte hash and nonce — tens of kilobytes, not megabytes.

TCP’s reliability is **per connection** (Alice↔Bob). If Alice gossips a fold and the overlay forwards it Alice→Carol→Dave, there are two TCP hops. Each hop is reliable. The overlay as a whole is still allowed to drop or reorder *application* messages. That is why this project does not treat GossipSub as “TCP for four people.”

### 4.6 Encryption on the connection: Noise

Once a TCP pipe exists, this project runs the **Noise** protocol on top of it (via libp2p). Noise does two jobs for each *direct* connection:

1. **Confidentiality** — an eavesdropper on the Wi-Fi sees ciphertext.
2. **Authentication of the hop** — the TCP session is bound to the remote peer’s key.

That is **hop security**. It is enough for a direct Alice↔Bob stream. It is **not** enough for gossip (see §13), because a gossip message may be forwarded by Carol. Carol’s Noise session only proves “this hop is Carol,” not “the original author was Alice.” So gossip payloads are **also signed**.

### 4.7 Packets, hops, and why “the host” is not magic

A **packet** is a chunk of data with addressing information. On a LAN it typically goes: Alice’s Wi-Fi radio → access point / router → Bob’s Wi-Fi radio. Each device in between is a **hop**.

Nothing in that path understands poker. The router does not shuffle cards. `poker host` is not sitting in the router. The host is just another laptop that happened to start listening first so joiners have something to dial.

### 4.8 Unicast vs broadcast vs multicast

- **Unicast:** one sender, one receiver. “Alice sends Bob her Shamir share of her private exponent.” Direct libp2p streams are unicast.
- **Broadcast:** one sender, everyone on the local network. Too noisy; this project does not use Ethernet broadcast for game messages.
- **Multicast:** one sender, a *group* of interested receivers. **mDNS** uses multicast on the LAN so peers can find each other without a directory server.
- **Publish/subscribe (GossipSub):** an overlay on top of unicast TCP. You publish to a **topic**; interested peers eventually receive a copy. Delivery is best-effort and **unordered**. That is the main game channel.

### 4.9 NAT, in one paragraph

**NAT** (Network Address Translation) is why your laptop’s `192.168.1.100` is not reachable from the internet. The home router rewrites addresses. Incoming connections to “the apartment” hit the router, which does not know which laptop wanted them unless you **port-forward** or the laptop asks the router (UPnP / NAT-PMP) to open a mapping.

This project calls `NATPortMap()` and disables relays. On a LAN you do not care. Across the internet, if UPnP fails, someone must copy a reachable multiaddr or the join never happens. That is a networking limitation, not a poker-rules limitation.

### 4.10 What “latency” and “bandwidth” mean for a card game

**Latency** is how long a message takes to get there. On a LAN, think 1–10 ms. Humans take hundreds of milliseconds to click Fold. So mesh latency is not the bottleneck.

**Bandwidth** is how many bytes per second you can push. A shuffle step carries 52 big integers (2048-bit), plus a commitment. That is tens of kilobytes, not megabytes. Fine on Wi-Fi.

The slow part of this demo is **CPU**: 2048-bit modular exponentiation, 52 cards, once per player, in seat order. Several seconds of “Shuffling…” is expected. That is cryptography, not a hung network.

---

## 5. Client–server vs peer-to-peer

### 5.1 The shape you already know (client–server)

```
   Alice ──┐
   Bob   ──┼──►  Poker.com server  ──►  shuffles, deals, stores pots
   Carol ──┘
```

Every client talks only to the server. The server is:

- the **rendezvous** (players find the table by connecting to a well-known host),
- the **dealer**,
- the **source of truth** for chips,
- a **single point of failure** and a **single point of trust**.

TLS (the lock in the browser) only proves you are talking to Poker.com. It does not prove Poker.com shuffled fairly.

### 5.2 The shape this project uses (peer-to-peer mesh)

```
        Alice
       /     \
     Bob     Carol
       \     /
        Dave
```

Every process is a **peer**: it listens, it dials, it publishes, it subscribes. After the mesh exists, there is no distinguished “server socket” in the game protocol. Alice’s laptop runs the **same binary** as Dave’s.

libp2p (the library underneath) maintains a **mesh**, not necessarily a full clique. With four players you will often be fully connected anyway. The important mental model is: a message Alice publishes may arrive at Dave **via Carol**. Dave must still be able to prove the payload came from Alice. That is why signatures exist.

### 5.3 Why there is still a command called `host`

Someone has to listen first, or there is nobody to connect to. `poker host --seats 4 --name Alice --table friday` does three unglamorous things:

1. Binds a TCP port and prints multiaddrs.
2. Advertises itself on the LAN via mDNS (service tag `p2p-poker-v1`).
3. Creates the lobby for table id `friday` with four seats.

After Bob, Carol, and Dave join, Alice is not the dealer, not the sequencer, and not the judge of the pot. If you draw a star with Alice in the middle *as a game server*, you have misunderstood the design. She is the **bootstrap peer**.

---

## 6. What “decentralized” means here

“Decentralized” is a marketing word. Here it has a precise meaning and a precise non-meaning.

### 6.1 What it does mean

- **No central game process.** Every seated peer runs `game.Machine`. Winners are computed, not announced.
- **Equal protocol roles.** After join, every node uses the same message types. Seat 0 shuffles first because of canonical seat order, not because they are privileged.
- **Private inputs stay private.** Default P2P dealing never puts hole-card ranks on the wire. Ciphertexts and proofs travel; your private exponent `d` does not (except as Shamir shares, unicast, for crash recovery).
- **Evidence instead of a house.** Signed envelopes, an append-only log, equivocation detection, slash records that *could* go on-chain.

### 6.2 What it does not mean

- **Not “no computers in the middle.”** Packets still traverse a router. Decentralization is about **who you trust for the game**, not about physics.
- **Not blockchain consensus for every action.** Ethereum, if wired, would hold **money**, not cards. Putting every fold on-chain would make the game unplayable (block time, gas, reorgs).
- **Not Byzantine-fault-tolerant agreement.** A partition can stall the sequencer. A player who sends fold to Alice and raise to Bob is caught later when both signed envelopes exist, not blocked at send time.
- **Not internet-scale.** No DHT, no relays, no matchmaking service, no tournaments.
- **Not live ETH.** `contracts/PokerEscrow.sol` is real. `internal/chain` is a stub. `cmd/poker` never calls it.

A fair one-line claim:

> A LAN mental-poker Hold’em table: peers jointly shuffle under commutative encryption, deal with partial decrypts and ZK checks, and agree on pots locally; on-chain escrow is specified but not integrated.

---

## 7. Texas Hold’em, as the engine models it

Skip this if you already play. It exists so the later “four people” story uses the same words as `internal/game`.

- **Seats** 2–9 locally, **3–9** on P2P. Each seat has a stack of chips (default buy-in 1000).
- A **dealer button** rotates. **Small blind** and **big blind** post forced bets (defaults 5 and 10). With four players: dealer, SB, BB, then **under the gun** (first to act pre-flop).
- Each player gets **two hole cards**, dealt starting left of the dealer, two rounds (as in a real casino: one card each, then one card each again).
- Betting rounds: **pre-flop**, **flop** (three community cards, after a burn), **turn** (one card, after a burn), **river** (one card, after a burn).
- Legal actions: fold, check, call, raise, all-in. A raise must meet `MinRaise`.
- If everyone but one folds, that player wins the pot — **no cards needed**.
- Otherwise **showdown**: best five-card hand from seven (two hole + five community). Rank from high card up to royal flush. Ties split; odd chips go to the winner closest left of the dealer.
- **Side pots** appear when someone is all-in for less than others put in. Eligibility is “who contributed to this layer.”

The engine’s phases (`internal/game/state.go`):

```
Waiting → PreFlop → Flop → Turn → River → Showdown → Settled
```

Crypto mode inserts **Awaiting Street** after a betting round ends, until the network peels the next public cards and calls `ApplyStreet`. That phase is how the engine avoids dealing from a local deck it does not have.

---

## 8. Why a normal online poker site is a trusted dealer

Even a perfectly honest site is a **trust concentration**:

| Power the site has | Abuse |
|---|---|
| It shuffles | Stack the deck for a whale, or for the house |
| It sees every hole card | Collude with a player, or sell “tells” |
| It stores stacks | Rewrite history, delay a raise, drop a disconnect conveniently |
| It holds the money | Slow payouts, insolvency, freeze accounts |
| It is one process | If it dies, the table dies |

You can audit the client. You cannot audit the shuffle unless the site publishes a protocol that does not need the site. **Mental poker** is that class of protocol: the deck is a cryptographic object the players build together.

This project splits the old server into three jobs and gives each job to a different mechanism:

| Old server job | Replacement |
|---|---|
| Shuffle and deal | SRA shuffle + partial decrypts (`internal/crypto`, `network.CryptoHand`) |
| Track whose turn / pots | Replicated `game.Machine` + sequenced `PLAYER_ACTION` |
| Hold the money | Optional `PokerEscrow.sol` (not wired into the live loop) |

---

## 9. Map of the repository

Module: `github.com/RedPaladin7/DecentralizedPokerEngine`. Language: Go 1.25. Binary: `cmd/poker`.

```
cmd/poker/              CLI, local vs-bots loop, P2P loop (lobby → shuffle → peels → TUI)
config/                 YAML config, libp2p identity seed, unused Ethereum keygen
internal/game/          Pure Hold’em. No sockets. Same inputs → same outputs
internal/network/       libp2p host, GossipSub, lobby, protobuf codec, CryptoHand, liveness
internal/crypto/        SRA, Keyring, ShuffleSession, DealSession, ZK peels, commitments, Shamir math
internal/fault/         Heartbeats, 2/3 timeout votes, key-share store, slash records
internal/tui/           Bubble Tea table UI
internal/chain/         Escrow helpers + stub RPC (not called from main)
internal/integration/   End-to-end and adversarial tests
contracts/              PokerEscrow.sol + Hardhat tests
plans/                  Phase notes for how crypto and liveness were wired
```

The architectural rule: **the game engine never imports the network package.** Networking produces authenticated, ordered inputs. The engine reduces them. Mixing those layers is how “the host accidentally becomes the server” happens.

---

## 10. The game engine

Package: `internal/game`. This is the “consensus object,” even though there is no consensus protocol. If every honest replica starts from the same seats/blinds/dealer and applies the same actions (and, in crypto mode, the same peeled cards), they cannot disagree on the pot.

### 10.1 Cards and the deck

A card is a rank (2–Ace) and a suit (spades, hearts, diamonds, clubs). Internally it also has a **card id** `0..51`:

```
id = suit * 13 + (rank - 2)
```

That integer is what cryptography will encode as a field element. `NewDeck()` builds the 52 cards in order. `Shuffle` is Fisher–Yates, used only in **plaintext** mode (local bots, or `--no-crypto`). `Deal` pops from the front.

In **crypto** mode the machine’s `Deck` pointer is **nil**. The engine is not allowed to sample cards. Streets arrive through `ApplyStreet`. Opponent holes arrive through `ApplyHoleReveal`.

### 10.2 Players

Each `Player` has an id (the libp2p peer id string), a display name, a stack, two hole cards, a status (Active / Folded / All-In / SittingOut), and bet counters for the current round and the hand.

`PlaceBet` moves chips from stack into the bet. If the amount would empty the stack, status becomes All-In.

### 10.3 The reducer: `ApplyAction`

`Machine.ApplyAction` is the only betting mutation:

1. Reject if it is not this player’s turn, or if the phase does not allow actions (Waiting, Awaiting Street, Showdown, Settled).
2. Apply fold / check / call / raise / all-in with the usual legality checks.
3. Append to `Log`.
4. If only one player remains, award the pot (`resolveSingleWinner`) — no showdown.
5. Else advance the action index, or end the betting round.

Ending a betting round in **plaintext** mode deals the next street from the local deck (burn then deal, matching casino procedure). Ending a betting round in **crypto** mode sets `PhaseAwaitingStreet` (or starts showdown after the river). The network layer then peels and calls `ApplyStreet` with three cards (flop) or one (turn/river).

### 10.4 Pots (including side pots)

`CalculatePots` sorts players by total chips put in this hand, then builds layers: everyone who put in at least this much is eligible for this slice. Identical eligibility sets merge. That is how side pots fall out of the math instead of being a special case in the betting code.

A four-player example, because this confuses people more than the networking does.

Alice, Bob, Carol, Dave start a street with stacks 30, 100, 100, 100. Alice goes all-in 30. Bob raises to 100 (all-in). Carol calls 100. Dave folds (having put in 0 this hand, ignore blinds for a moment). Contributions: Alice 30, Bob 100, Carol 100, Dave 0.

- Layer 1: the first 30 that *everyone who put money in* matched. Amount = 30×3 = 90. Eligible: Alice, Bob, Carol.
- Layer 2: the extra 70 that only Bob and Carol put in. Amount = 70×2 = 140. Eligible: Bob, Carol.

Alice can only win the 90. The 140 is a **side pot**. At showdown the engine evaluates Alice against Bob and Carol for pot 1, and only Bob against Carol for pot 2. If Alice has the nuts she takes 90 and Bob/Carol contest 140. Crypto mode does not change this math: once hole cards are revealed, `distributePots` is ordinary Hold’em.

### 10.5 Showdown

`EvaluateBest7` tries every way to drop two of seven cards and keeps the best five-card hand. Comparison is rank, then kickers. Each pot is awarded independently among eligible, non-folded players.

In crypto mode, showdown **waits** until every remaining seat has hole cards filled (`ApplyHoleReveal`). Fold-to-winner never needs that.

### 10.6 Why this purity matters

Suppose Bob’s client broadcast “I won with a full house.” Alice’s replica would ignore that. She has the action log and (at showdown) the revealed cards. She computes the winner. `HAND_RESULT` on the wire is informational. A lying result does not move chips on an honest replica.

---

## 11. Identity, addresses, and encrypted connections

Package: `internal/network/host.go`. Library: [libp2p](https://libp2p.io/).

### 11.1 Why not “Alice on 192.168.1.100”?

IP addresses change. Display names collide. A cheater can claim to be Alice. This project’s stable identity is a **key**.

Each process has an **Ed25519** keypair, stored as a 64-byte seed under `~/.poker/identity.key` (or generated fresh). Two things are derived from it:

1. The **libp2p Peer ID** — a hash of the public key. This is the string you see printed at startup (`12D3KooW…`). It is the player’s id inside the lobby and the game.
2. The key used to **sign gossip envelopes**.

If you copy the identity file to another laptop, you *are* the same peer. If you delete it, you are a new person as far as the table is concerned.

This identity is **not** an Ethereum key. `poker keygen` prints a separate secp256k1 key for a future chain mode. Live play does not use it.

### 11.2 Listening

`NewPokerHost` builds a libp2p host with:

- the Ed25519 identity,
- a listen multiaddr (default `/ip4/0.0.0.0/tcp/9000`),
- **Noise** for connection security,
- **TCP** as the transport,
- **relays disabled**,
- **NATPortMap** attempted.

A **multiaddr** is a self-describing address. When Alice prints:

```
/ip4/192.168.1.100/tcp/9000/p2p/12D3KooWAlice...
```

Bob’s joiner parses that into “dial this IP and port, and the peer key I expect is Alice’s.” If someone else answers, the cryptographic handshake fails. That is stronger than a hostname.

### 11.3 Connecting

`PokerHost.Connect` parses a multiaddr, writes the address into the **peerstore** (a local address book: peer id → addresses), then dials TCP and completes Noise.

On a LAN, Bob may never type that string: mDNS can find Alice and call `Connect` for him. On another network, `--peer` is how Bob learns where to dial.

### 11.4 Multiplexing

Once Alice and Bob have one TCP connection, libp2p can open many **streams** on it (Yamux). Opening a `/poker/1.0.0` stream to send a Shamir share does **not** mean a new TCP handshake. Think of streams as logical channels over one pipe.

---

## 12. Finding other players (discovery)

Before you can play, you must answer: *who else is running this program, and how do I dial them?*

There is no company directory. There are two mechanisms, composed in `Node.Start()`.

### 12.1 mDNS on the LAN

**mDNS** (multicast DNS) is how printers and Chromecasts show up on your Wi-Fi without you typing an IP. A device multicasts “I offer service X” and others multicast queries.

This project’s service tag is `p2p-poker-v1` (`internal/network/discovery.go`). When a peer is found, `HandlePeerFound` adds them to the peerstore and `Connect`s on a new goroutine.

Limitations you should expect:

- Same LAN (same multicast domain). A phone hotspot and a dorm VLAN may or may not share multicast.
- Windows multicast is flaky in tests; a real four-terminal demo on one Wi-Fi usually works.
- mDNS finds *poker processes*, not a specific table. The **table id** (`--table friday`) is an application-level filter after you are connected.

### 12.2 Bootstrap multiaddr

Joiners can pass `--peer "/ip4/.../tcp/.../p2p/..."`. That is stored as a bootstrap peer and dialed. Connection failure is non-fatal; join is retried a few times while the mesh forms.

This is how you play across networks, *if* the address is reachable.

### 12.3 What does not exist

No Kademlia DHT. No bootstrap server operated by the project. No circuit relay. If discovery fails, the lobby stays at “Waiting for players” forever. That is the first troubleshooting item in the README, and it is a **network** failure, not a crypto failure.

---

## 13. Two ways messages travel: gossip and direct streams

Once peers are connected, the game still needs a way to say things. This project uses **two** transports on purpose.

### 13.0 Why two transports at all?

A beginner instinct is “open a TCP connection to everyone and send the same bytes three times.” With four players that works. The project still uses pub/sub because:

- membership changes (a join, a timeout);
- heartbeats should not sit behind a 52-card shuffle in one queue;
- libp2p already maintains a mesh.

The cost is unordered, best-effort delivery, which is fixed **above** GossipSub by sequencers. Direct streams exist for the few messages that should not be flooded: live Shamir shares of `d`.

### 13.1 GossipSub (publish / subscribe)

`GossipManager` (`internal/network/gossip.go`) joins two topics:

| Topic | Purpose |
|---|---|
| `poker/table/<tableID>` | Joins, ready, shuffle steps, peels, actions, votes, results |
| `poker/heartbeat/<tableID>` | Liveness pings, **separate** so a slow table mesh does not look like a dead player |

You **publish** a byte frame. Every subscriber eventually gets a copy, typically from a **neighbor**, not from the original author. `ReceivedFrom` is the neighbor. The original author is inside the signed envelope (`sender_id`).

Properties of GossipSub that drive the rest of the design:

- **Unordered.** Bob can see action 3 before action 2.
- **Best-effort.** A message can be dropped. The sequencer will then wait forever (a known gap: no NAK/retransmit in v1).
- **Echo.** You receive your own message back. `dispatch` drops envelopes whose `sender_id` is you. That is why the acting player **applies locally first**, then broadcasts — otherwise their UI would wait for a round trip to themselves.
- **Forwarding.** Signatures are mandatory. Noise only authenticated the last hop.

### 13.2 Direct streams (`/poker/1.0.0`)

`protocol.go` registers a stream handler. Frames are **length-prefixed** (4-byte big-endian length, max 4 MiB). Direct streams carry:

- **Unicast Shamir shares** of `d` at table start (never gossiped while the player is alive — a gossiped live share would let too many people collect reconstruction material casually).
- **Hole peels**, best-effort. Gossip is still the authoritative peel path so a missed direct message does not brick the hand.

On a direct stream, the remote Peer ID is bound by Noise. Envelope signature verify is skipped for that reason.

### 13.3 The signed envelope

Every gossip payload is wrapped (`messages.proto`):

```
Envelope {
  type, sender_id, seq, timestamp, payload, signature
}
```

The signature is Ed25519 over `type ‖ sender ‖ 0x00 ‖ seq ‖ timestamp ‖ payload`. Verification uses the public key extracted from the Peer ID.

**Envelope `seq`** is per-sender and strictly increasing. It is **replay protection**, not game order. If Alice’s envelope 7 arrives twice, the second is dropped. If Alice’s *action* 7 arrives before action 6, that is a different counter (`PlayerAction.Seq`) handled by the action sequencer.

This dual-counter design is one of the most important systems details in the repo. Mixing them up would either reject legal reordering or accept replays.

### 13.4 Message types (what actually goes on the wire)

| `MsgType` | When |
|---|---|
| `JOIN_TABLE` | Seat claim: name, buy-in, public SRA exponent `e`, session nonce |
| `PLAYER_READY` | Barrier: I am willing to start |
| `SHUFFLE_STEP` | My encrypt-then-permute output + commitment (no permutation) |
| `PARTIAL_DECRYPT` | I peeled one layer of one card + ZK proof |
| `PLAYER_ACTION` | Fold/check/call/raise/all-in + **table-wide** action seq |
| `HEARTBEAT` | I am alive |
| `TIMEOUT_VOTE` | I vote that X has gone silent |
| `KEY_SHARE` | A Shamir share of someone’s `d` (unicast live; gossip only after a timeout) |
| `HAND_RESULT` | Informational pots / state root |
| `EQUIVOCATION_EVIDENCE` | Two conflicting signed envelopes |
| `GAME_STATE_SYNC` | Defined for catch-up; **unused** in the live loop |
| `SHUFFLE_COMMIT` | Defined; **unused** (`SHUFFLE_STEP` already carries the commitment) |

Protobuf is just a structured binary encoding. The interesting part is which fields exist: shuffle steps have a deck of **bytes** (big integers), not ranks. A packet capture of a honest crypto table should not show `A♠`.

---

## 14. The lobby

Package: `internal/network/lobby.go`. This is everything that happens **before** the first shuffle.

### 14.1 Seats

A `Lobby` is a map of peer id → `SeatInfo` (name, buy-in, public `e`, nonce, ready flag, join timestamp). Max seats comes from config / `--seats`. P2P requires 3–9.

`HandleJoin` rejects joins after the lobby is no longer waiting, rejects a full table, rejects a duplicate peer, rejects a non-positive buy-in.

### 14.2 Canonical seat order

Every honest node that saw the same joins must agree who is seat 0. Order is:

1. `JoinedAtUnixMs` ascending (taken from the **sender’s** envelope timestamp),
2. then `PlayerID` string compare.

That order is used for: dealer rotation, who shuffles first, peel order, designated survivor after a crash, and (in `--no-crypto`) the shared seed.

**Footgun:** join timestamps are sender-stamped. Clock skew can theoretically reorder seats, which would desynchronize shuffle turns. The assumption is honest, roughly synchronized LAN clocks. A production system would order by a hash of peer ids or a commit-reveal, not wall clocks.

### 14.3 Ready barrier

When `Count() >= maxSeats`, nodes broadcast `PLAYER_READY`. `WaitReady` is event-driven: a channel is closed once (via `sync.Once`) when every seat is ready. No busy loop on the ready path itself (the fill wait in `main` is a 250 ms poll on count).

### 14.4 Session binding

Each join carries a **session nonce** (in practice the peer’s own id bytes). Concatenated in seat order, those bytes bind this table:

- Crypto: mixed into `SessionID = SHA256(playerIDs ‖ nonce ‖ handNum)` so ZK proofs and commitments cannot be replayed across tables or hands.
- `--no-crypto`: mixed into a public RNG seed so every replica Fisher–Yates-shuffles the same plaintext deck.

A mixed table (one peer omitted `e` because they passed `--no-crypto`, others did not) **errors out**. Silent fallback to plaintext would be a privacy disaster.

---

## 15. Replicated state machines and action order

This is the public-state half of the thesis. Write it on a whiteboard:

```
identical initial state  +  identical ordered actions  =  identical GameState
```

### 15.1 Why not broadcast `GameState` every tick?

If Alice broadcasts a blob “pot = 80, Bob’s stack = 920,” then Alice is a server. Bob has to trust her snapshot. Replicas would also fight over whose snapshot is newer.

Instead Alice broadcasts “I raise 40” with a sequence number. Everyone applies that function to their local state.

`GAME_STATE_SYNC` exists in the proto for a future reconnect path (send log + state root, replay, accept only if the root matches). The live loop does not use it. Disconnect is terminal for that player.

### 15.2 Gossip is not a log

If Bob applies eagerly:

- Alice’s replica: call, then raise → pot 30
- Dave’s replica: raise, then call → illegal or different pot

The hand is over, and not in a fun way.

**Fix:** `actionSequencer` in `cmd/poker/main.go`. It holds `nextSeq` and a map of pending actions. When a `PLAYER_ACTION` arrives, it is stored. Whenever `pending[nextSeq]` exists, it is applied and `nextSeq` increments. Gaps wait.

The acting player assigns the next seq, applies locally under `machineMu`, then broadcasts. Remotes only apply through the sequencer.

### 15.3 A fold, hop by hop

Suppose it is Alice’s turn and she presses `f`.

1. **UI thread (Alice).** Bubble Tea delivers a key event. `p2pGameModel.applyAndBroadcast` runs on that thread.
2. **Reducer (Alice).** Under `machineMu`, `actionSequencer` hands out the next table-wide seq (say 2). `ApplyAction({Alice, Fold, 0})` sets her status to Folded, appends the log, advances the actor to Bob. Unlock.
3. **Sign (Alice).** `BroadcastAction` builds a `PlayerAction` protobuf, wraps it in an `Envelope` with a fresh *envelope* seq (a different counter), signs with Ed25519, length-prefixes the bytes.
4. **Publish (Alice).** GossipSub puts the frame on `poker/table/friday`. libp2p sends it over Alice’s existing Noise/TCP sessions to her mesh neighbors (maybe Bob and Carol, maybe not Dave yet).
5. **Forward (Carol).** Carol’s GossipSub may send a copy to Dave. Carol’s Noise session proves “this hop is Carol.” Dave must still verify Alice’s signature on the envelope.
6. **Receive (Bob).** `receiveLoop` reads the frame. `dispatch`: not from me, envelope seq > last Alice seq, signature ok, append Gamelog, unmarshal `PLAYER_ACTION`.
7. **Sequencer (Bob).** If he already has seq 1 applied and this is seq 2, apply now. If seq 3 arrived earlier and is sitting in `pending`, applying 2 may drain 3 immediately after. Same `ApplyAction` code as Alice used.
8. **UI (Bob).** `notifyCh` or the 250 ms tick pushes `GameStateMsg`. Bob’s panel for Alice goes muted/folded. Highlight moves to whoever `CurrentPlayer()` is.

Alice also receives her own gossip echo. `dispatch` drops it (`sender_id == me`). If she had *not* applied locally in step 2, she would wait to see her own fold come back — extra latency and a window where her UI still thinks she can act.

### 15.4 Two sequence spaces, again

| Counter | Scope | Job |
|---|---|---|
| Envelope `seq` | Per sender | Drop duplicates and replays |
| `PlayerAction.Seq` | Whole table | Total order of `ApplyAction` |

Shuffle steps and peels have **their own** sequencers inside `ShuffleSession` / `DealSession`, keyed by **seat index**, not by this table-wide action counter. Mixing those would be wrong: a peel is not a betting action.

### 15.5 Locks

`game.Machine` is not internally synchronized. The live process treats it as a **single-threaded reducer**. `machineMu` is held around every `ApplyAction` / `ApplyStreet` / `ApplyHoleReveal`. Crypto **wait** loops (`WaitShuffle`, `WaitStreet`, …) **release** that lock so a timeout fold can run while peels are in flight. Holding the lock across a two-minute shuffle wait would deadlock liveness.

### 15.6 What this is not (CAP, Raft, BFT)

During a partition, a minority that misses action 7 stalls on the sequencer. There is no committed index, no leader, no view change. The timeout path force-folds **silence**; it does not merge two conflicting histories.

That is a conscious trade: N ≤ 9, human-speed actions, LAN. If you needed adversarial WAN, you would replace **only the log transport** with Raft or BFT and keep `ApplyAction` unchanged. That is why the engine has no sockets.

---

## 16. Mental poker: hiding cards without a dealer

Package: `internal/crypto`. Orchestration: `internal/network/crypto_hand.go`, called from `dealCryptoHand` in `cmd/poker`.

If you remember one math fact, remember **commutativity**:

```
((m ^ e_A) ^ e_B)  ≡  ((m ^ e_B) ^ e_A)   (mod p)
```

Encrypting with Alice then Bob is the same ciphertext as Bob then Alice. Therefore the group can lock the whole deck under **all** public exponents, permute in between, and later peel layers in **any order**. Nobody needs a distinguished dealer who saw the plaintext order.

### 16.1 Shared prime and card encoding

All players use the same 2048-bit prime `p` from RFC 3526 (the MODP group in `params.go`). It is a public parameter, like “we all agreed to use this deck of 52 physical cards.”

A card id `i` becomes a field element:

```
m_i = 2^(i+1)  mod p
```

That puts plaintext in the multiplicative group so exponentiation is a valid lock. After all layers are peeled, `FieldToCard` brute-forces which of the 52 encodings matches (52 modular exponentiations — cheap next to the shuffle).

### 16.2 Keys: public `e`, private `d`

Each peer generates `(e, d)` with `gcd(e, p-1) = 1` and `d = e⁻¹ mod (p-1)`. Then:

```
Encrypt:  c = m^e  mod p
Decrypt:  m = c^d  mod p
```

`e` is published in `JOIN_TABLE`. `d` never leaves the node except as Shamir shares.

`Keyring` is the data structure that makes this hard to get wrong: local full key plus a map of **public-only** keys for every seat. `Public(id)` always returns a key with `d == nil`, including for yourself. There is no API that returns another peer’s `d`.

`CryptoGame` / `HandCoordinator` still generate **every** `(e, d)` on one machine. They are the **test oracle**, not the live path. If you grep those types and think that is how `poker host` works, you are looking at the simulator.

### 16.3 Shuffle (distributed, seat order)

`ShuffleSession` is a turn-taking state machine. Every replica starts from the same plaintext encoding of cards 0–51 (not a shuffled deck — shuffling is the permutations).

On player k’s turn:

1. Encrypt every current ciphertext with **their** `e` (actually `EncryptAll` using the local private key’s `e`).
2. Choose a **secret random permutation** (never sent).
3. Apply it to the 52 ciphertexts.
4. Compute a **SHA-256 commitment** `H(len ‖ serialized_deck ‖ nonce)` over 256-byte big-endian limbs.
5. Publish `SHUFFLE_STEP`: output deck + hash + nonce. **No permutation. No input deck.**

Everyone else:

- Rejects a step from the wrong seat or wrong hand.
- Verifies the commitment opens to the published deck (so the shuffler cannot later claim a different output).
- Adopts that output as the next input.

Gossip can deliver Dave’s step before Bob’s. The session **buffers** by seat index (`pending` map), same idea as the action sequencer.

After four players, the deck is encrypted under `e_A e_B e_C e_D` and permuted by four secret permutations. One honest random permutation randomizes the order. Collusion of **all four** still breaks this — do not oversell.

**Why a commitment is not a fairness proof.** It binds the published bytes. It does not prove the permutation was random. Fairness comes from “I do not know your permutation, and you encrypted first.”

**Why mid-shuffle disconnect aborts.** The permutation lives only in that player’s RAM. Reconstructing `d` does not reconstruct the permutation. Survivors cannot finish a consistent encrypted deck. The hand dies; restart the table.

### 16.4 Dealing hole cards (selective peel)

Deck **indexes** match the engine’s deal order so crypto and rules cannot drift.

For 4 players, dealer index 0:

| What | Indexes |
|---|---|
| Hole round 1 (SB, BB, UTG, dealer) | 0, 1, 2, 3 |
| Hole round 2 | 4, 5, 6, 7 |
| Burn before flop | 8 |
| Flop | 9, 10, 11 |
| Burn before turn | 12 |
| Turn | 13 |
| Burn before river | 14 |
| River | 15 |

`HoleCardIndex` implements the same walk as `dealHoleCards`.

To deal Bob’s first hole card at index `i`:

1. Ciphertext `C` at that index is public (everyone has the same encrypted deck).
2. **Everyone except Bob** publishes a `PARTIAL_DECRYPT`: input ciphertext, output after peeling **their** `d`, plus a ZK proof.
3. Peel order is canonical seats minus Bob. Out-of-order peels buffer.
4. After three verified peels, only Bob’s layer remains. Bob runs `FinishHole` **locally**. That last decrypt is **not published**.
5. Alice’s replica stores Bob’s hole slots as empty. Alice still sees a ciphertext she cannot peel (missing `d_Bob`).

Direct streams try to send hole peels to the recipient; gossip is authoritative.

### 16.5 Community cards (public peel)

Everyone peels. After four peels the value is plaintext. `FinishPublic` maps it to a `Card`. Burns are **skipped indexes**, not peeled. All replicas call `ApplyStreet` with the same three (or one) cards.

### 16.6 Showdown reveals

Remaining players’ hole **indexes** are peeled publicly, in **seat order on every replica**. You must not peel “whoever is missing on this replica” (`MissingRevealIDs()`), because Alice already has her own holes and would choose a different order than Bob, and then `ApplyHoleReveal` would diverge.

Fold-to-winner skips reveals. Next hand starts a **new** shuffle. Keys do not rotate. Session id mixes `handNum` so proofs do not replay.

### 16.7 Zero-knowledge proofs on peels

A partial decrypt is a pair of big integers. Without a proof, Dave can publish garbage and freeze the hand, or try to steer a community card.

Each peel carries a Schnorr-style proof of correct exponentiation, Fiat–Shamir hashed with the session id (`zkp.go`):

- Prover knows `d`, publishes `h = g^d`, `A = g^r`, `B = ciphertext^r`.
- Challenge `c = SHA256(p, h, ciphertext, result, A, B, sessionID) mod (p-1)`.
- Response `s = r + c·d`.
- Verifier checks `g^s = h^c · A` and `ciphertext^s = result^c · B`.

A failed proof is a slashable event (`SlashBadZKProof`). In the live loop it is logged; it is not yet submitted on-chain.

You do not need to memorize the equations. You need the sentence: **anyone can check that this peel used the claimed `d` without learning `d`.**

### 16.8 Debug path: `--no-crypto`

Every node:

```
seed = mix(sessionNonce)
deck = NewDeck(); rng = rand.New(seed); deck.Shuffle(rng)
StartHand()  // local deal, all holes filled on every replica
```

Identical, fast, and **zero privacy**. Any peer can print the entire deck. Use it to test that the sequencer and pots stay in lockstep without waiting seconds for SRA.

---

## 17. Faults: silence, folds, and reconstructing a missing key

Package: `internal/fault`, wired in `runP2PMode` and `network/liveness.go`.

Mental poker creates a new failure mode a normal poker server does not have: **if someone holds an encryption layer the table still needs, folding them is not enough.** The flop still has their `e` on it. Survivors must obtain `d` or the hand deadlocks.

### 17.1 Heartbeats

Every few seconds (default 5s) each peer publishes `HEARTBEAT` on the heartbeat topic. `HeartbeatMonitor` records last-seen. After `heartbeat_timeout` (default 15s) the peer is TimedOut and a vote starts.

A separate topic matters: a large shuffle step on the table topic should not look like death.

### 17.2 Timeout votes

The other `n-1` players vote. Threshold is `⌈(n-1)·2/3⌉` (implemented as float 2/3 rounded). For **n = 4**, that is **2 of 3** yes votes.

On confirm:

1. `forceFold` applies a fold on the local machine **and** `BroadcastAction`s that fold so every replica applies it through the sequencer. A local-only fold would desync stacks.
2. In crypto mode, if the shuffle already finished, start Shamir reconstruction (next subsection).
3. The silent player cannot rejoin. Restart the table for a new session.

If the timeout happens **during shuffle**, reconstruction is useless. `AbortShuffle`; the hand errors out after the wait timeout (2 minutes).

### 17.3 Shamir shares of `d`

**Shamir secret sharing:** split a secret into `n` shares such that any `t` reconstruct it and `t-1` reveal nothing.

At table start (once per table, not per hand — keys do not rotate), each peer:

```
t = max(2, (n+1)/2)
shares = SplitSecret(d, t, n, p)
```

For **n = 4**, `t = 2`. Each other seat receives **one** share over a **direct stream**. The owner keeps their own share. Live shares are not gossiped.

After a confirmed timeout of Carol:

- Survivors **gossip** their share of Carol’s `d` (now it is appropriate: she is gone, the table needs the layer).
- `TryReconstructKey` builds an `SRAKey` for Carol.
- That key is stored on `CryptoHand.MarkGone`, **not** inserted into the Keyring (the Keyring’s job is “I only hold my `d`”).
- **Designated survivor** = first remaining id in seat order. That peer calls `PeelOnBehalf` whenever the deal session expects Carol’s peel, publishing a `PARTIAL_DECRYPT` with `PlayerID = Carol`.

Why P2P minimum 3 seats: after one drop you still need `t` shares. At n = 2, t = 2, one share remains, reconstruction is impossible.

### 17.4 Equivocation

`Gamelog` appends every authenticated envelope. Every 5 seconds it looks for the same `(sender, seq)` with two different payloads. Both signatures are court-admissible evidence. Detection is not prevention.

### 17.5 Slash records

Reasons exist: bad ZK, invalid action, key withholding, equivocation. Without the chain client, these are logs. With the contract, 20% of the accused buy-in would burn.

---

## 18. The terminal UI

Package: `internal/tui`. Charm **Bubble Tea** (Elm-style: `Model` / `Update` / `View`) plus Lip Gloss styles.

The view is a pure function of `*game.GameState` plus “who am I.” Opponent hole cards render face-down unless `IsLocalPlayer || IsWinner` (`player_panel.go`). That hide is **defense in depth**. The real privacy is that the replica’s `Player.HoleCards` for opponents are empty until reveal. The TUI would have nothing to show even if that check were removed — unless you are on `--no-crypto`.

Keyboard: `f` fold, `c` check/call, `r` raise, `a` all-in, arrows to move, Enter to confirm, `q` to quit.

In P2P mode, a keypress calls `applyAndBroadcast`: lock, bump action seq, `ApplyAction`, unlock, gossip, then kick crypto advance (street/reveal waits **without** the machine lock). A notify channel (buffer 16) plus a 250 ms tick keep the UI fresh if a notify is dropped.

Local mode never starts libp2p. Bots check/call after 600 ms on the same Bubble Tea program.

---

## 19. On-chain escrow (designed, not live)

`contracts/PokerEscrow.sol` is a real Solidity contract with Hardhat tests. `internal/chain.Client` returns **synthetic** receipts. `main.go` never talks to an Ethereum node. Demo chips are honor-system counters.

The intended split is still worth understanding, because it is the correct way to add money without putting poker on the hot path of a blockchain:

1. Players `joinTable{value}` while the contract is `Open`. When full, state becomes `Playing`.
2. The off-chain game runs as in this document. At the end, payout **deltas** must sum to 0 (chip conservation). A `stateRoot` is SHA-256 over the ordered gamelog. ≥2/3 of seated players sign the outcome digest.
3. `reportOutcome` pays. A 50-block **challenge window** allows `submitDispute` with slash evidence. 20% of the accused buy-in burns (`SLASH_BURN_BPS = 2000`); the rest redistributes.
4. If nobody settles by 1000 blocks, `markAbandoned` and refund.

The contract never sees cards. That is the point. Poker at two seconds per action cannot wait for block time. Ethereum is the judge of **money and evidence**, not the dealer.

---

## 20. How the pieces snap together

Read this once after the component chapters. It is the wiring, not new mechanism.

```
 Discovery          Overlay                 Inside one process                 Money (future)
 ---------          -------                 ------------------                 --------------
 mDNS (LAN)    →    GossipSub          →   signed envelope
 bootstrap          table topic            → sequencer (actions)
 multiaddr          heartbeat topic        → game.Machine (identical replica)
                    TCP + Noise            → append-only Gamelog
                    streams /poker/1.0.0         ↑
                                           CryptoHand (SRA shuffle + peels)
                                           FaultManager (timeout + Shamir)
                                           TUI (subscriber)
                                                                              → PokerEscrow
                                                                                (not called)
```

**Start-up order that actually matters** (from `runP2PMode`):

1. Load identity seed; generate SRA `(e, d)` unless `--no-crypto`.
2. Construct `Node`. **Assign every callback before `Start()`** so the receive loop cannot drop early shuffle/peel/join messages into nil handlers. Early shuffle/peels buffer until `CryptoHand` exists.
3. `Start()`: stream handler, mDNS, bootstrap dials, receive loop, equivocation scan.
4. `BroadcastJoin` (retried). Wait until lobby count hits `max_seats`. `BroadcastReady`. Sleep 2s for propagation.
5. Canonical seats. Start `FaultManager.Run` and `HeartbeatSender` **before** the shuffle (a death during shuffle should still be observed, even if the hand then aborts).
6. Crypto: `KeyringFromLobby` → unicast Shamir shares → `dealCryptoHand` (shuffle, holes, `StartHandCrypto` with **local holes only**) → TUI.
7. During play: sequencer + `ApplyAction`. When the machine needs a street or reveal, `AdvanceCryptoLocked` peels then applies cards. Waits run unlocked.
8. Next hand: new `CryptoHand`, new shuffle, same keys, session id mixes `handNum`.

**Who talks to whom:**

| Producer | Consumer |
|---|---|
| TUI keypress | `ApplyAction` + `BroadcastAction` |
| Gossip `PLAYER_ACTION` | sequencer → `ApplyAction` |
| Gossip `SHUFFLE_STEP` | `CryptoHand.HandleShuffle` → maybe broadcast my step |
| Gossip `PARTIAL_DECRYPT` | `CryptoHand.HandlePeel` → maybe send my peel |
| Direct `KEY_SHARE` (live) | `FaultManager.StoreKeyShare` |
| Gossip `KEY_SHARE` (after timeout) | reconstruction buffer |
| Heartbeat | `RecordHeartbeat` |
| Timeout confirm | `forceFold` + share gossip + `PeelOnBehalf` |
| Peeled community / holes | `ApplyStreet` / `ApplyHoleReveal` |
| `GameState` | TUI render |

Nothing in `internal/game` knows that GossipSub exists. Nothing in `internal/crypto` knows that Bubble Tea exists. `cmd/poker` is the composition root. That is deliberate.

---

## 21. A full four-player hand, step by step

This is the section the rest of the file exists to support. Four people, one LAN, default crypto dealing, nobody cheats, nobody disconnects. Then §22 breaks it.

### 21.1 The people and the machines

| Person | Role in the story | Process | Listen port |
|---|---|---|---|
| Alice | First to start; seat 0; **dealer** this hand | `poker host --seats 4 --name Alice --table friday` | TCP 9000 |
| Bob | Seat 1; **small blind** | `poker join --name Bob --table friday` | 9000 on his laptop, or 9001 if sharing Alice’s |
| Carol | Seat 2; **big blind** | same | |
| Dave | Seat 3; **under the gun** (first to act pre-flop) | same | |

They are on the same Wi-Fi. mDNS is on. Each has a persistent `identity.key`, so Peer IDs are stable. Each generates a fresh SRA `(e, d)` for this process start. Buy-in 1000, blinds 5/10.

Assume clocks are close enough that join order is Alice, then Bob, then Carol, then Dave. Canonical seats:

```
Seat 0 Alice (dealer)
Seat 1 Bob   (SB)
Seat 2 Carol (BB)
Seat 3 Dave  (UTG)
```

If Dave had joined before Carol, Carol would not be BB. The rest of the protocol would still work; the story would just rotate. We freeze this order so the indexes stay readable.

### 21.2 Phase 0 — four programs exist, nobody has friends yet

Alice’s process:

1. Reads or creates `~/.poker/identity.key`.
2. Generates SRA key on the shared 2048-bit prime.
3. Builds a libp2p host, listens on `0.0.0.0:9000`, enables Noise, starts mDNS with tag `p2p-poker-v1`.
4. Subscribes to `poker/table/friday` and `poker/heartbeat/friday`.
5. Prints her Peer ID and multiaddrs. One of them looks like `/ip4/192.168.1.42/tcp/9000/p2p/12D3KooWAlice`.
6. Applies her own `JOIN_TABLE` to her lobby (GossipSub will echo it; self-messages are ignored on receive, so she must apply locally).
7. Prints `Waiting for players…`.

Bob’s process does the same on his laptop. mDNS hears Alice. `HandlePeerFound` dials Alice’s multiaddr. TCP three-way handshake, then Noise. Now there is an encrypted multiplexed pipe. Carol and Dave likewise form a mesh. With four nodes, GossipSub will build a sparse overlay; in practice on a LAN everyone can reach everyone.

**What has not happened:** no cards, no chips moved, no dealer ceremony. They have only achieved “these four programs can send authenticated bytes.”

If Bob were on another network, he would skip mDNS success and pass `--peer` with Alice’s multiaddr. From this point on the story is identical.

### 21.3 Phase 1 — lobby fill

Each of the four publishes `JOIN_TABLE`:

- `table_id = "friday"`
- display name
- `buy_in = 1000`
- `sra_pub_key_e` = bytes of their `e` (**not** `d`)
- `session_nonce`

Every node’s `Lobby.HandleJoin` stores a `SeatInfo`. Alice’s terminal prints `[lobby] Bob joined (2 / 4 seats)` and so on.

When count hits 4, each node broadcasts `PLAYER_READY`. When all four ready bits are set, `readyCh` closes. They sleep two seconds so a late ready still lands. Seat order is sorted. Session nonce bytes are concatenated in that order.

**Identical initial public facts now exist on all four laptops:** who sits where, blinds, buy-ins, everyone’s `e`, the nonce that will bind proofs.

Each node builds a `Keyring`: my `d`, four public `e`s. Alice’s Keyring cannot decrypt with Bob’s `d` because she does not have it.

### 21.4 Phase 2 — Shamir backup, before anyone shuffles

Still before the deck moves. Each player splits their `d` into **4 shares, threshold 2**.

Alice’s share 0 she keeps. Share 1 she unicasts to Bob on `/poker/1.0.0`. Share 2 to Carol. Share 3 to Dave. Bob, Carol, Dave do the same with their own `d`.

After this:

- Bob holds: his full `d`, plus one share of Alice’s `d`, one of Carol’s, one of Dave’s.
- Nobody can reconstruct Alice’s `d` unless **two** of those Alice-shares are later pooled. One share is useless.

Heartbeats start. `FaultManager.Run` starts. If Dave’s laptop dies **now**, before shuffle completes, the hand will abort rather than reconstruct — the permutation would be missing. The shares are for **after** a successful shuffle.

### 21.5 Phase 3 — the joint shuffle (this is the slow part)

`dealCryptoHand` constructs a `CryptoHand` for hand 1. Session id = SHA-256(seat ids ‖ lobby nonce ‖ hand number). All four replicas start a `ShuffleSession` from the **same** plaintext encoding of cards 0–51.

**Turn 0 — Alice.** It is her seat index. She:

1. Encrypts all 52 values with `e_Alice`. Each card is now `m^{e_A}`.
2. Draws a secret permutation π_A (a random ordering of 0..51). Applies it.
3. Commits to the 52 output integers.
4. Publishes `SHUFFLE_STEP` on the table topic.

Bob, Carol, and Dave receive that gossip (possibly in different orders relative to other messages, but there is no other shuffle yet). They verify the commitment. They do **not** learn π_A. They set their current deck to Alice’s output.

**Turn 1 — Bob.** He encrypts Alice’s output with `e_Bob`, secret-permutes π_B, commits, publishes. Cards are now `m^{e_A e_B}` in an order neither Alice nor Bob can invert alone (Alice does not know π_B; Bob does not know π_A, and both layers are on every card).

**Turn 2 — Carol.** Same with `e_Carol`, π_C.

**Turn 3 — Dave.** Same with `e_Dave`, π_D.

If Dave’s step arrives at Alice **before** Carol’s (GossipSub), Alice’s `ShuffleSession` parks Dave’s message in `pending[3]` and refuses to apply it until `nextIndex == 3`. When Carol’s step lands, Alice applies it, then drains Dave’s pending step. All four finish with **byte-identical** 52 ciphertexts.

Wall-clock time: **several seconds**. 52 × 2048-bit modexp × 4 players, sequential. The TUI/status line says `Shuffling…` so nobody thinks the program froze.

**What each person knows after the shuffle:**

- Everyone: the same 52 big integers, and that they are a jointly encrypted, jointly shuffled deck.
- Nobody: which ciphertext is the Ace of spades.
- Nobody: anyone else’s permutation.
- Nobody: anyone else’s `d`.

### 21.6 Phase 4 — hole cards (eight selective deals)

Holes are dealt left of the dealer, two rounds. Recipient order per round: Bob, Carol, Dave, Alice.

Take **Bob’s first card**, deck index 0.

Peel order = seats minus Bob = Alice, Carol, Dave.

1. Alice peels her layer of ciphertext[0], attaches a ZK proof, publishes `PARTIAL_DECRYPT` (and tries a direct stream to Bob). Every replica verifies the proof and replaces the current value with Alice’s result.
2. Carol peels, proves, publishes. Verify-and-apply.
3. Dave peels, proves, publishes. Verify-and-apply.
4. **Only Bob** decrypts the last layer locally. He gets a `Card`, say `K♣`. He puts it in `HoleCards[0]`. He does **not** broadcast the rank.

On Alice’s machine, Bob’s hole slot is still empty. Alice has a leftover ciphertext. She cannot map it to a card. The TUI draws a face-down card for Bob.

Repeat for indexes 1–7 with the appropriate recipient. After `WaitHoles`:

| Replica | Alice’s holes | Bob’s | Carol’s | Dave’s |
|---|---|---|---|---|
| Alice’s laptop | known | empty | empty | empty |
| Bob’s laptop | empty | known | empty | empty |
| Carol’s laptop | empty | empty | known | empty |
| Dave’s laptop | empty | empty | empty | known |

Each person also posted blinds **after** `StartHandCrypto`: Bob 5, Carol 10. Stacks: Alice 1000, Bob 995, Carol 990, Dave 1000. Phase = PreFlop. Action index = Dave (left of BB). `Deck == nil` on every replica.

**Important:** `StartHandCrypto` does **not** require opponent holes. Requiring them would force a leak.

### 21.7 Phase 5 — pre-flop betting (the replicated log)

Dave sees his two cards and the blinds. He presses `c` (call 10).

On Dave’s process:

1. `machineMu` lock.
2. Sequencer says next action seq is 1. Assign seq 1.
3. `ApplyAction({Dave, Call, 10})`. His stack 990, current bet 10.
4. Unlock. Broadcast `PLAYER_ACTION` with seq 1. Kick crypto advance (nothing to peel yet). TUI redraws.

Gossip carries that envelope. Alice, Bob, Carol:

1. Verify Ed25519, check envelope seq watermark, append Gamelog, drop if sender is self (N/A).
2. `OnPlayerAction` → lock → sequencer. If this is seq 1, apply; if somehow seq 2 arrived first, buffer.
3. Unlock, notify TUI.

All four machines now have Dave called 10. Next actor is Alice (dealer acts after UTG in this seating).

Alice folds (`f`). Seq 2. Applied everywhere. She is out; her holes never need to be revealed if she stays folded (and they will not be at a fold-to-winner; if the hand goes to showdown among others, her cards stay private).

Bob calls (seq 3): adds 5 more to match 10.

Carol checks (seq 4): already 10 in as BB.

Betting round complete. Pots calculated: 40 chips, all four contributed? Alice folded after posting 0 pre-flop wait — she posted nothing yet (dealer is not a blind with four players). Pot = Dave 10 + Bob 10 + Carol 10 = 30, Alice 0. (Alice folded without putting chips in.) Eligible: Bob, Carol, Dave.

In **plaintext** mode the engine would now burn and deal the flop from a local deck. In **crypto** mode `endBettingRound` sets `PhaseAwaitingStreet` and returns. No replica samples a card.

### 21.8 Phase 6 — the flop appears together

`kickCryptoAdvance` / `AdvanceCryptoLocked` sees `NeedsStreet`. `BeginStreet(Flop)` starts public peels of indexes 9, 10, 11 (index 8 is the burn, skipped).

For flop card 1 (index 9), peel order is **all four** seats: Alice, Bob, Carol, Dave. Alice still peels even though she folded — her **key layer** is still on the ciphertext. Folding is a game status, not a key deletion.

Each peel is verified. After four peels, plaintext. Repeat for indexes 10 and 11. All four replicas obtain the same three cards, e.g. `A♠ 7♦ 2♣`. They call `ApplyStreet` with those three. Phase = Flop. New betting round. First to act is first active left of dealer: Bob.

Alice’s TUI shows the flop. She still does not see Bob’s holes. That is the product working.

### 21.9 Phase 7 — flop, turn, river betting

Same mechanism as pre-flop. Suppose:

- Bob checks (seq 5).
- Carol checks (seq 6).
- Dave bets 20 (seq 7, raise).
- Bob calls 20 (seq 8).
- Carol folds (seq 9).

Round ends. `PhaseAwaitingStreet`. Public peel of turn index 13 (burn 12 skipped). `ApplyStreet` one card. Phase = Turn.

Bob and Dave check it down (seqs 10–11). Peel river index 15. `ApplyStreet`. Phase = River. They check again (seqs 12–13). Betting complete after river → `startShowdown`. Remaining players Bob and Dave still have empty opponent holes on each other’s machines (and on Alice/Carol). `remainingHolesIncomplete` is true, so the engine waits.

### 21.10 Phase 8 — showdown reveals, then math

Reveal order is **canonical seats**, not “who is missing here.” Remaining in-hand: Bob (seat 1), Dave (seat 3). Alice and Carol are folded; their hole indexes are not publicly peeled.

For Bob’s two hole indexes, **everyone including Bob** peels publicly (this is now a community-style peel of private indexes). After four layers, both cards are plaintext on **every** replica. `ApplyHoleReveal(Bob, {K♣, 9♣})`. Repeat for Dave’s indexes. `ApplyHoleReveal(Dave, …)`.

Now every honest machine has:

- the same five community cards,
- Bob’s two holes,
- Dave’s two holes,
- Alice and Carol still unrevealed.

`EvaluateBest7` for Bob and for Dave. Suppose Bob’s flush beats Dave. `distributePots`: Bob’s stack increases by the pot. `PhaseSettled`. `HAND_RESULT` is gossiped for logs. TUI shows winners with a golden border and face-up cards for the people who showed.

Alice, looking at her screen, sees Bob’s cards **now**, not earlier. That is showdown, not a leak.

### 21.11 Phase 9 — next hand

Dealer index advances. A **new** `CryptoHand` is built. Session id mixes `handNum = 2`. New shuffle from plaintext encodings again — you do **not** reuse the old encrypted deck (that would reuse permutations/ciphertexts unsafely). Keys `(e, d)` stay. Shamir shares are **not** redistributed (same `d`). Heartbeats continue. Play again until they quit.

### 21.12 What Alice sees vs what Bob sees vs what is on the wire

This is the privacy story in one place. Pick the moment **after holes are dealt, before any betting**.

**Alice’s terminal**

- Her two hole cards face-up.
- Bob, Carol, Dave: two face-down cards each (TUI), and in memory those `HoleCards` are the zero value, not secret ranks she is politely hiding.
- Community: empty.
- Pot: 15 (SB 5 + BB 10).
- Highlight on Dave: his turn.

**Bob’s terminal**

- His two hole cards face-up (different ranks than Alice’s).
- Alice, Carol, Dave face-down / empty in memory.
- Same pot, same “Dave to act.” Public state matches. Private state does not.

**On the Wi-Fi (if you could decrypt Noise, which a random laptop cannot)**

You would still not see `K♣`. You would see gossip frames whose payloads are 256-byte integers, commitment hashes, ZK tuples `(A, B, S, H)`, and later tiny `PLAYER_ACTION` messages (`action=1` for fold, an amount, a seq). Ranks exist only after a *public* peel completes, i.e. flop/turn/river/showdown.

**If Alice dumps her process memory**

She has: her `d`, four public `e`s, the 52 joint ciphertexts, her two plaintext holes, empty opponent holes, Shamir *shares* of the others’ `d` (not enough alone to reconstruct). She cannot decrypt Bob’s remaining layer. That is the definition of “hidden hole cards” this project actually implements.

### 21.13 Counting messages in that hand (order of magnitude)

A networking beginner often asks “how chatty is this?” Rough counts for our four-player example, crypto mode, no disconnects:

| Phase | What is flooded | Ballpark |
|---|---|---|
| Lobby | 4 joins + 4 readies | ~8 small messages |
| Shamir | 4 players × 3 unicasts | 12 direct stream frames (not gossip) |
| Shuffle | 4 `SHUFFLE_STEP`s | 4 **large** messages |
| Hole peels | 8 cards × 3 published peels | 24 `PARTIAL_DECRYPT`s (plus optional direct copies) |
| Pre-flop actions | 4 actions in the story | 4 small |
| Flop peels | 3 cards × 4 peels | 12 |
| Flop/turn/river actions | ~9 in the story | 9 small |
| Turn + river peels | 1+1 cards × 4 | 8 |
| Showdown | 4 hole cards × 4 peels | 16 |
| Heartbeats | every 5s × 4 peers | background noise on the other topic |

Betting is cheap. **Dealing is the bandwidth and CPU.** That is why “Shuffling…” is slow and why a lost shuffle step is more painful than a lost heartbeat.

### 21.14 What each layer did in that hand (one table)

| Layer | What it contributed |
|---|---|
| LAN / TCP / Noise | Bytes arrived privately on each hop |
| mDNS | They found each other without a website |
| GossipSub | Everyone saw the same *set* of messages |
| Envelope signatures | Dave’s call could not be forged by Carol |
| Envelope seq | Replays dropped |
| Action sequencer | Call before raise, even if gossip swapped them |
| `game.Machine` × 4 | Same pots and winner from the same inputs |
| SRA shuffle | Nobody picked the order alone |
| Selective peels + ZK | Hole privacy + no garbage decrypts |
| TUI | Humans could act; opponents stayed face-down |
| Shamir / timeouts | Idle this hand (nobody died) |
| Escrow contract | Unused; chips were local integers |

That is the whole system, exercised once.

---

## 22. What if someone cheats or disconnects during that hand

Same four people. Different failures.

### 22.1 Dave unplugs after the flop, before the turn peel

Heartbeats from Dave stop. After ~15s he is TimedOut. Alice, Bob, Carol vote. 2 of 3 yes confirms.

Each survivor:

- Applies and broadcasts a **fold** for Dave (so the machine’s action log matches).
- Gossips their Shamir share of Dave’s `d`.
- Reconstructs Dave’s `(e, d)` once two shares exist.
- Marks Dave gone on `CryptoHand`.

Bob is the designated survivor if Alice folded and seat order says the first remaining id… actually designated survivor is first remaining in **SeatOrder** who is not gone: Alice is still seated (folded, but her process is alive). Alice’s id is first. Alice `PeelOnBehalf` for Dave’s layer on the turn and river (and on any remaining hole peels if needed).

If Dave’s fold leaves only one player in the hand, `resolveSingleWinner` fires and **no further peels** are required for showdown. If two remain, peels continue with Alice acting as Dave’s decryptor.

Dave cannot reconnect mid-hand. `GAME_STATE_SYNC` is unused.

### 22.2 Carol unplugs **during** her shuffle step

Her permutation is gone. Reconstructing `d` does not help. `WaitShuffle` hits the 2-minute timeout (or the abort path). The hand errors. Restart all four processes. Harsh, documented, correct.

### 22.3 Bob publishes a junk partial decrypt

ZK verify fails. Slash record `SlashBadZKProof`. The peel is rejected. The deal session will not advance on a honest replica. Bob has not learned anyone’s cards; he has stalled or incriminated himself. With live escrow, that evidence could burn 20% of his buy-in. Today it is a log line.

### 22.4 Alice sends “fold” to Bob and “raise” to Carol, same action seq

Both envelopes are signed. `Gamelog.DetectEquivocation` finds the pair. The table now has cryptographic proof of double-talk. Honest nodes may have already diverged if they applied different payloads — this is the “detect, don’t prevent” trade. On a LAN with honest software it does not happen. Against a modified client, you need either slash-and-abort or a BFT log.

### 22.5 Eve on the same Wi-Fi, not in the lobby

She can see Noise ciphertext on the radio, not plaintext poker. She is not subscribed to the GossipSub topic as a seated peer with a matching table id and signed joins. She cannot inject a raise as Alice without Alice’s Ed25519 key. mDNS tells her *that poker processes exist*; it does not give her a seat after the lobby is full.

### 22.6 Someone runs `--no-crypto` while the others do not

`KeyringFromLobby` / the crypto dealing check sees an empty `e`. The table **exits**. It does not quietly show all cards to the people who thought they had privacy.

### 22.7 Gossip drops action seq 7

Every replica waits forever on the sequencer. There is no retransmission in v1. Humans Ctrl-C and restart. This is a real limitation; the design note is “add NAK or state-sync later,” not “pretend GossipSub is TCP for application messages.” (TCP reliability is per hop. Gossip is a multi-hop overlay.)

---

## 23. Local mode (one process, no network)

`./poker` with no `host`/`join` is a different product that shares the **rules engine and the TUI** and almost nothing else.

- One `game.Machine`. `StartHand` shuffles a real `Deck` with a local RNG. Every bot “seat” lives in the same process, so of course the process knows every hole card. Privacy is not the point.
- Bots are Bubble Tea timers (~600 ms) that check or call. No libp2p, no GossipSub, no SRA, no Shamir.
- Seats 2–9 are allowed. This is how you test heads-up and side pots without three laptops.
- Next-hand delay is ~1.5 s. Ctrl-C ends it.

If you are learning networking, local mode is useful as a **control experiment**: the Hold’em math is the same, the distributed parts are gone. If pots disagree between local and P2P `--no-crypto`, the bug is in the engine or the sequencer, not in SRA. If `--no-crypto` P2P matches local but default crypto disagrees on a showdown, the bug is in peels or `ApplyStreet` order.

## 24. Common misconceptions

**“Alice is the server because we ran `poker host`.”**  
She printed an address. After join, she publishes and subscribes like everyone else. Seat 0 shuffles first because of join order, not because she is privileged.

**“The router is dealing the cards.”**  
The router forwards IP packets. It does not run this binary. Decentralized means “no trusted poker operator,” not “no electronics between laptops.”

**“GossipSub is like a group chat with guaranteed order.”**  
It is a group chat without order or delivery guarantees. Order is restored by sequence numbers in this application.

**“Noise encryption means we do not need signatures.”**  
Noise authenticates a *hop*. Gossip is forwarded. Signatures authenticate the *author*.

**“If I fold, my key is gone and I can close the laptop.”**  
Folding is a status bit. Your `e` is still multiplied into every remaining ciphertext. If you vanish after the shuffle, survivors reconstruct `d` and peel for you. If you vanish during the shuffle, they cannot.

**“Shamir means any two players can look at my hole cards whenever they want.”**  
They would need two shares of **your** `d` **and** to peel cards that still have only your layer (or all other layers). Live shares are unicast, not gossiped. Pooling shares is supposed to happen **after a timeout vote**, when you are already folded and gone. Two colluding live players who also steal extra shares is a real caveat of `t = 2` at `n = 4`; it is a liveness/security trade, not a feature. Honest software does not gossip live shares.

**“The blockchain shuffles.”**  
It does not, and it should not. The contract, if wired, would see payout deltas and a hash of the signed log.

**“`--no-crypto` is ‘the fast production mode.’”**  
It is a debug mode where every replica holds every card. Fast and identical. Zero privacy.

**“We run PBFT / Raft / ‘blockchain consensus’ on every fold.”**  
We do not. We authenticate, sequence, and detect equivocation. Upgrading the log transport would not require rewriting Hold’em.

## 25. Honest limitations

Saying these out loud is part of understanding the project.

1. **P2P needs 3–9 seats.** Shamir after one drop needs leftover shares. Heads-up is local bots only.
2. **Disconnect is terminal** for that player. No mid-hand catch-up.
3. **Mid-shuffle disconnect aborts** the hand.
4. **No live ETH.** Solidity is tested; Go RPC is stubbed; `main` never calls it.
5. **LAN / port-forward.** mDNS or a reachable `--peer`. No DHT, no relays. NAT is UPnP only.
6. **One table.** No tournaments, no matchmaking.
7. **Not BFT.** Authenticated total order among honest nodes; equivocation detected after the fact.
8. **Seat order uses join timestamps.** Clock skew can reorder seats in theory.
9. **SRA is slow and old.** Fine for a LAN capstone. Collusion of all players still wins. A modern shuffle argument (Bayer–Groth, etc.) is future work; wiring SRA for real beats an unwired fancier paper.
10. **Lost gossip messages stall.** No application-level retry.
11. **`--no-crypto` has no card privacy.** It is a sync test tool.

None of these undo the thesis if you state them. The thesis is not “production PokerStars.” It is: **poker as a replicated state machine whose private inputs come from commutative encryption rather than a dealer.**

---

## 26. Glossary

| Term | Meaning in this project |
|---|---|
| **LAN** | Local network (home Wi-Fi, lab). Machines can usually dial each other by private IP |
| **WAN / internet** | Everything beyond that router. Needs a reachable address; this repo does not provide relays |
| **IP address** | Number identifying a machine on a network (`192.168.1.100`) |
| **Port** | Number identifying a program on that machine (default 9000) |
| **TCP** | Reliable, ordered byte pipe. No encryption by itself |
| **Noise** | Encryption + authentication of a **direct** libp2p connection |
| **Peer** | One running `poker` process. Both sender and receiver |
| **Peer ID** | libp2p identity derived from an Ed25519 public key |
| **Multiaddr** | Self-describing address: IP + TCP port + peer id |
| **mDNS** | LAN multicast discovery (“is anyone offering `p2p-poker-v1`?”) |
| **GossipSub** | Publish/subscribe overlay. Unordered, best-effort, forwarded |
| **Unicast / stream** | Direct message on `/poker/1.0.0` over the existing TCP connection |
| **Envelope** | Signed wrapper around a protobuf payload |
| **Envelope seq** | Per-sender counter for replay protection |
| **Action seq** | Table-wide counter for `ApplyAction` order |
| **Sequencer** | Buffer that applies messages in seq order despite gossip reordering |
| **Replica / RSM** | Every peer runs the same state machine on the same log |
| **Host (CLI)** | First listener / bootstrap peer. **Not** a game server |
| **SRA** | Commutative modular exponentiation used as a lock on cards |
| **`e` / `d`** | Public / private SRA exponents. `e` on the wire; `d` stays local |
| **Keyring** | Local `d` + everyone else’s public `e` only |
| **Shuffle step** | Encrypt all 52, secret-permute, publish output + commitment |
| **Peel / partial decrypt** | Remove one player’s encryption layer from one card |
| **ZK proof** | Checkable evidence a peel used the right `d` without revealing `d` |
| **Commitment** | Hash binding a published shuffled deck so it cannot be swapped later |
| **Shamir sharing** | Split `d` so any `t` shares reconstruct it |
| **Designated survivor** | First remaining seat, peels on behalf of a reconstructed key |
| **Mental poker** | Cryptographic dealing among mutually distrusting players |
| **`--no-crypto`** | Debug: public shared seed, all cards visible |
| **Escrow** | On-chain pot of ETH; not wired into live play |

---

## How to keep going

- Run three or four terminals as in [`README.md`](./README.md). Watch `Shuffling…` take seconds. Confirm opponent holes stay hidden until showdown.
- Read `internal/game/machine.go` as a reducer with no I/O.
- Read `internal/crypto/shuffle_session.go` and `deal_session.go` as sequencers keyed by seat, not as “the dealer.”
- Read `cmd/poker/main.go` `runP2PMode` as the composition root: callbacks before `Start`, shares before shuffle, waits without `machineMu`.
- For interview framing, [`SYSTEMS_DESIGN_INTERVIEW.md`](./SYSTEMS_DESIGN_INTERVIEW.md).
- For file-level architecture, [`SYSTEM_DESIGN_OVERVIEW.md`](./SYSTEM_DESIGN_OVERVIEW.md).

The shortest true summary remains:

> There is no game server. Public state is a deterministic engine fed by a totally ordered signed log. Private cards are a commutative encryption protocol. Money, if it ever lands, is an escrow contract that never sees a card.
