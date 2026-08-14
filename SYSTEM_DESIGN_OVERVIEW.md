# System Design Overview — Decentralized Poker Engine

This document is the living architecture brief for the P2P Texas Hold'em engine in this repository (`github.com/RedPaladin7/DecentralizedPokerEngine`). It is written for systems-design interview prep: what problem the design solves, how the pieces compose, where concurrency lives, and how fairness is enforced without a trusted dealer.

**Status snapshot (as implemented today):** default `poker host` / `poker join` is LAN mental-poker Hold'em. Each peer holds only its own SRA private exponent `d`, jointly shuffles over `SHUFFLE_STEP`, deals with `PARTIAL_DECRYPT` + ZK peels, and keeps opponent hole cards empty until showdown. After the shuffle, a silent peer is timeout-folded; survivors reconstruct `d` from Shamir shares and finish peels. Mid-shuffle disconnect aborts the hand. `--no-crypto` is the old shared-seed plaintext path (all cards visible). Blockchain RPC in `internal/chain` is still stubbed; `PokerEscrow.sol` is real and Hardhat-tested. P2P tables require **3–9 seats**. Local vs bots is 2–9 and does not use SRA.

---

## 1. System Goals & Requirements

### Problem a centralized poker server creates

A classic online poker stack is a trusted server:

- The house shuffles the deck, deals hole cards, and is the sole source of truth for stacks, pots, and whose turn it is.
- Players must trust that the operator does not peek at cards, stack the deck, delay actions, or rewrite history.
- Availability is a single point of failure: if the server dies, the table dies.
- Settlement (chips / money) is whatever the operator says it is.

That trust model is fine for a casino with legal and reputational constraints. It is a poor model for a trustless table among strangers who may want real-money settlement without a house.

### What this architecture is trying to solve

| Goal | How the design addresses it |
|---|---|
| **No trusted dealer** | Every seated peer runs an identical replica of `game.Machine`. There is no authoritative game server. |
| **Identical outcomes** | Peers apply the same sequenced action log to the same initial state. Winners and payouts are computed locally, not announced by a host. |
| **Fair shuffle** | Mental-poker SRA: each player encrypts and permutes in seat order; no single player knows the permutation or can see another player's hole cards. `--no-crypto` is debug only. |
| **Authenticated, replay-safe messaging** | Gossip envelopes are Ed25519-signed; per-sender sequence numbers drop duplicates and replays. |
| **Liveness without a referee** | Heartbeats + 2/3 timeout votes force-fold a silent player. After shuffle, survivors reconstruct that peer's `d` and peel on their behalf. Mid-shuffle drop aborts (permutation is gone). |
| **Accountable cheating** | Append-only `Gamelog`, equivocation detection (same seq, two payloads), ZK proof checks, slash records that can be submitted on-chain. |
| **Optional real-money settlement** | `PokerEscrow.sol` holds buy-ins, pays out on a multi-signed outcome, and slashes on dispute evidence. |

### Functional requirements

- Texas Hold'em: blinds, hole cards, flop/turn/river, check/call/raise/fold/all-in, main + side pots, showdown, multi-hand sessions. Local 2–9 seats; **P2P 3–9** (Shamir reconstruction after a drop needs `n ≥ 3`).
- Two run modes: **local** (human vs bots on one process, plaintext deck) and **P2P** (`poker host` / `poker join`, SRA by default).
- Table identity is a string (`table_id`); seats fill until `max_seats`, then play starts.
- LAN discovery via mDNS; WAN via explicit libp2p multiaddrs (`--peer`).

### Non-functional / constraints (honest)

- **Consistency model:** replicated state machine, not BFT consensus. Safety assumes honest majority of *messages applied in order*, not a PBFT/Raft quorum on every action.
- **Ordering:** GossipSub is unordered; an application-level `actionSequencer` restores total order via `PlayerAction.Seq`.
- **Network:** TCP + Noise; NAT hole-punching is `NATPortMap` only. Relays are disabled. Best on LAN.
- **Crypto dealing in the live path:** default. Each peer's Keyring stores only local `d`. ShuffleSession / DealSession / `CryptoHand` run over gossip (shuffle) and gossip+streams (peels). `--no-crypto` is the shared-seed Fisher–Yates path — every node sees every card.
- **Reconnection:** a disconnected player cannot catch up mid-hand (`GAME_STATE_SYNC` unused). After a timeout they cannot rejoin; restart the table.
- **Chain client:** deploy/join/report/dispute return synthetic receipts; the Solidity contract is real, the Go RPC glue is not live.

### What "decentralized" means here vs. what it does not

- **Does:** no central game server; equal peers; local computation of winners; hidden hole cards on the default P2P path; timeout fold + Shamir peels after shuffle; signed evidence trail; optional on-chain escrow (contract only).
- **Does not:** Byzantine-fault-tolerant agreement on every action; working ETH payouts; internet-scale NAT traversal; mid-hand reconnect; multi-table tournaments; 2-player P2P.

---

## 2. High-Level Architecture

### Topology

The network is a **libp2p mesh**, not a star.

```
                    ┌─────────────┐
                    │  Peer Alice │
                    │  (host CLI) │
                    └──────┬──────┘
                           │  GossipSub topics
                           │    poker/table/<tableID>
                           │    poker/heartbeat/<tableID>
                           │  Direct streams /poker/1.0.0
              ┌────────────┼────────────┐
              │            │            │
       ┌──────▼──────┐ ┌───▼───┐ ┌──────▼──────┐
       │  Peer Bob   │ │ Carol │ │    Dave     │
       └─────────────┘ └───────┘ └─────────────┘
```

- **`poker host` is not a server.** It is the first peer to listen and print multiaddrs. Joiners bootstrap to that multiaddr. After the mesh forms, every node is both publisher and subscriber.
- GossipSub builds a sparse overlay: a message is received from a neighbor (`ReceivedFrom`), not necessarily from the original signer. That is why **gossip payloads must be signed**; Noise only authenticates the hop.
- Direct libp2p streams (`/poker/1.0.0`) carry point-to-point traffic: hole peels (best-effort; gossip is authoritative) and **unicast Shamir shares** of `d` at table start. Noise authenticates the stream. Live shares are never gossiped until a timeout vote (then survivors contribute shares of the missing `d`).

### Identity and transport

Each process is a `PokerHost` (`internal/network/host.go`):

1. Ed25519 keypair, persisted under `~/.poker/identity.key` (64-byte seed) or generated fresh.
2. libp2p Peer ID is derived from the public key — the stable network address.
3. Listen multiaddr, default `/ip4/0.0.0.0/tcp/9000`.
4. **Noise** for connection encryption/authentication.
5. **TCP** transport; relays disabled; UPnP/NAT-PMP attempted via `NATPortMap()`.

### Peer discovery

Two mechanisms, composed in `Node.Start()`:

1. **mDNS** (`internal/network/discovery.go`)
   - Service tag: `p2p-poker-v1`.
   - On `HandlePeerFound`, the peer is added to the peerstore and `Connect` is issued on a new goroutine.
   - Same LAN only.

2. **Bootstrap peers**
   - Joiners pass `--peer /ip4/.../tcp/.../p2p/<PeerID>`.
   - Host stores that in `cfg.Network.BootstrapPeers` and `PokerHost.Connect` parses the multiaddr into `peer.AddrInfo`, writes the peerstore, then dials.
   - Connection failure is non-fatal; the mesh can still form once both sides are up (join is retried).

There is no DHT / rendezvous server in this repo.

### How a table forms

1. Every node constructs a `Node` (host + GossipSub + lobby + gamelog) and **wires callbacks before `Start()`**, so the receive loop never drops early messages into nil handlers.
2. `Start()` registers the stream handler, starts mDNS, dials bootstraps, then launches `receiveLoop` and `equivocationScanLoop`.
3. Each node `BroadcastJoin` (`JOIN_TABLE`): name, buy-in, SRA public exponent `e` (empty when `--no-crypto`), session nonce (= own Peer ID). The sender also applies the join to its own lobby (GossipSub will echo it back, and self-messages are ignored on receive).
4. Join is retried a few times while the mesh forms.
5. Nodes poll `Lobby.Count() >= maxSeats`.
6. `BroadcastReady` (`PLAYER_READY`). A 2s sleep lets ready messages propagate.
7. Seats are ordered by join timestamp, then Peer ID — **canonical seat order**, identical on every honest node that saw the same joins.
8. **Crypto (default):** `KeyringFromLobby` (local `d` + everyone else's `e`). Unicast Shamir shares of `d`. `CryptoHand` runs ShuffleSession then DealSession. Each replica fills **only local** hole cards and calls `StartHandCrypto` (`Deck == nil`). Mixed tables (any empty `e`) error out.
9. **`--no-crypto`:** `Lobby.SessionNonce()` concatenates each seat's nonce in that order. That byte string is mixed into an `int64` (XOR + LCG) → **shared RNG seed**. Every node `StartHand`s the same plaintext deck. All cards are visible.

`OnShuffleStep`, `OnPartialDecrypt`, `OnTimeoutVote`, and `OnKeyShare` are assigned **before** `node.Start()`. Early shuffle/peel messages are buffered until `CryptoHand` exists.

### How game state is shared

State is **not** pushed as a blob every tick (though `GAME_STATE_SYNC` exists in the proto for catch-up). The live design is a **replicated state machine**:

```
identical initial state  +  identical ordered actions  →  identical GameState
```

- Initial state: table ID, blinds, buy-ins, seat order, dealer index. Crypto: jointly shuffled encrypted deck (identical ciphertexts on honest replicas). `--no-crypto`: shared shuffle seed.
- Actions: `PLAYER_ACTION` messages with a global `Seq`. Local player applies immediately, then broadcasts. Remote actions go through `actionSequencer` then `Machine.ApplyAction`.
- Streets and showdown cards in crypto mode are **inputs** (`ApplyStreet`, `ApplyHoleReveal`) from public peels — the machine does not sample a local deck (`PhaseAwaitingStreet` while peels run).
- On settle, `HAND_RESULT` is gossiped (pots + winners). It is informational; settlement already happened locally.
- `Gamelog` records every authenticated envelope. `StateRoot()` is a SHA-256 over the ordered log — intended as the on-chain fingerprint.

### Message types (protobuf)

Defined in `internal/network/messages.proto`, wrapped in a signed `Envelope` (`type`, `sender_id`, `seq`, `timestamp`, `payload`, `signature`):

| MsgType | Role |
|---|---|
| `JOIN_TABLE` | Seat claim + SRA pubkey + nonce |
| `PLAYER_READY` | Hand-start barrier |
| `SHUFFLE_STEP` | Mental-poker shuffle round (output deck + commitment). `SHUFFLE_COMMIT` exists but is unused |
| `PARTIAL_DECRYPT` | Peel one encryption layer off a card + ZK proof |
| `PLAYER_ACTION` | Fold/check/call/raise/all-in + action seq |
| `GAME_STATE_SYNC` | Snapshot + action log (catch-up; unused in live loop) |
| `HEARTBEAT` | Liveness, separate topic |
| `TIMEOUT_VOTE` | Vote that a peer has gone silent |
| `HAND_RESULT` | Settled pots / state root |
| `EQUIVOCATION_EVIDENCE` | Two conflicting envelopes |
| `KEY_SHARE` | Shamir share of an SRA `d` (unicast at table start; gossip only after a timeout vote) |

Framing: 4-byte big-endian length prefix, max 4 MiB (`codec.go`).

---

## 3. Core Components

Module: `github.com/RedPaladin7/DecentralizedPokerEngine` (Go 1.25). Binary: `cmd/poker`.

```
cmd/poker/main.go          CLI, local loop, P2P loop, action sequencer, TUI glue
config/                    YAML config, identity key, Ethereum keygen
internal/game/             Pure Hold'em engine (no network)
internal/network/          libp2p host, gossip, lobby, codec, CryptoHand, liveness
internal/crypto/           SRA, keyring, shuffle session, deal session, ZKP, Shamir math
internal/fault/            Heartbeat, timeout votes, slash, Shamir store
internal/chain/            Escrow manager + stubbed RPC client
internal/tui/              Bubble Tea table UI
internal/integration/      E2E and adversarial tests
contracts/PokerEscrow.sol  On-chain escrow / dispute / slash
plans/                     Phase specs for the crypto + liveness wiring
```

### 3.1 CLI / process orchestration — `cmd/poker`

Commands: default local game, `host`, `join`, `init`, `keygen`, `version`.

- **Local mode:** one `game.Machine`, bots that check/call after 600ms, Bubble Tea program. No libp2p. Plaintext `StartHand`.
- **P2P mode (`runP2PMode`):** rejects `MaxSeats < 3`. Constructs SRA key (unless `--no-crypto`), `network.Node`, lobby wait. Crypto: Keyring → share distribute → `dealCryptoHand` (shuffle + holes) → `StartHandCrypto` with local holes only. Debug: shared seed + `StartHand`. Starts `FaultManager.Run` and `HeartbeatSender` **before** shuffle. TUI `p2pGameModel` advances streets/reveals via `AdvanceCryptoLocked`.
- **`actionSequencer`:** lives here, not in `internal/network`. Buffers `map[int64]*PlayerAction` until `nextSeq` is present, then drains in order.
- **Machine pointer indirection:** `liveMachine` / `liveGS` behind `machineMu`. That mutex is held around **every** `ApplyAction` (local, remote, timeout fold). Street/reveal **waits** (`WaitStreet` / `WaitReveal`) run **unlocked** so a timeout fold can take the lock.

### 3.2 Game engine — `internal/game`

Pure, deterministic, no I/O. Same inputs → same outputs. This is the "consensus object."

| File | Responsibility |
|---|---|
| `state.go` | `Phase` (Waiting → PreFlop → Flop → Turn → River → Showdown → Settled, plus `PhaseAwaitingStreet` appended last), `Action`, `GameState` |
| `machine.go` | `StartHand` (plaintext), `StartHandCrypto` (no local deck; opponent holes may be empty), `ApplyAction`, `ApplyStreet`, `ApplyHoleReveal`, betting-round completion, showdown |
| `deck.go` | 52-card deck, Fisher–Yates `Shuffle`, `Deal` |
| `player.go` | Stack, hole cards, status (Active/Folded/AllIn/SittingOut), `PlaceBet` |
| `pot.go` | Side pots from contribution levels, merge identical eligibility |
| `hand_eval.go` | Best 5-card hand from 7 (`EvaluateBest7`), rank + kickers |

`ApplyAction` is the single mutation API for betting: validate actor is `CurrentPlayer()`, apply fold/check/call/raise/all-in, append to `Log`, then `advanceAction` or `endBettingRound`. In **crypto mode** (`Deck == nil`), end of a betting round sets `PhaseAwaitingStreet` instead of dealing from a deck. Callers peel the street, then `ApplyStreet` (flop 3 / turn 1 / river 1). Showdown waits for `ApplyHoleReveal` until every remaining seat has holes. Fold-to-one-winner needs no cards.

Plaintext `StartHand` still burns then deals from the local deck (flop 3, turn 1, river 1). Showdown uses `EvaluateBest7` per eligible player per pot; odd chips go to the winner closest left of the dealer.

`StartHandCrypto()` does **not** require opponent hole cards (that would leak). `CryptoGame.DealToEngine` / `HandCoordinator` still fill every seat — that is the in-process **oracle**, not the live P2P path.

### 3.3 Networking — `internal/network`

| Type | Role |
|---|---|
| `PokerHost` | libp2p host, Ed25519, listen/dial, multiaddrs |
| `GossipManager` | Two topics (table + heartbeat), publish, subscribe, per-sender seq watermark |
| `MDNSDiscovery` | LAN find + connect |
| `Lobby` | Seats, ready barrier (`readyCh`), canonical order, session nonce |
| `Node` | Facade: dispatch, broadcast helpers, stream pool, equivocation scan |
| `CryptoHand` | Live shuffle + peels per replica per hand (no Machine inside; produces cards) |
| `StreamPool` | Reused `/poker/1.0.0` streams for direct peels and key shares |
| `Gamelog` | Append-only envelopes, state root, equivocation, sequence-gap validation |
| `HandCoordinator` | In-process oracle bootstrap (`CryptoGame`); **not** the live `host`/`join` path |
| `codec.go` | Sign/verify envelopes, ShuffleMessage / PeelMessage / KeyShare wire adapters |

`Node.dispatch` is the network state machine: decode → drop self → seq check → log append → unmarshal payload → callback.

### 3.4 Cryptography — `internal/crypto`

Mental poker on a shared 2048-bit prime (RFC 3526 MODP group, `params.go`).

| Piece | Role |
|---|---|
| `SRAKey` / `PublicSRAKey` | Commutative modular exponentiation. Public-only keys encrypt; decrypt requires local `d` |
| `Keyring` | Local full key + map of public `e`s. `Public(id)` never returns another peer's `d` |
| `ShuffleProtocol` | Encrypt-all → secret permute → SHA-256 commitment (`RunFullShuffle` is the in-process oracle) |
| `ShuffleSession` | Distributed turn-taking FSM; publishes `ShuffleMessage` (deck + commitment, no permutation) |
| `DealProtocol` | Oracle peels using every `d` in one process |
| `DealSession` | Distributed peels with Keyring; hole jobs skip the recipient; `PeelOnBehalf` for reconstructed keys |
| `ZKProof` | Schnorr-style proof that a partial decrypt used the claimed `d` (Fiat–Shamir via SHA-256) |
| `Commitment` | `SHA256(len ‖ data ‖ nonce)` over serialized deck |
| `CardToField` | Card `id` → `2^(id+1) mod p` so plaintext sits in the multiplicative group |
| `CryptoGame` | Local simulation (all keys on one machine). Test oracle only |
| Shamir (`commit.go`) | Split SRA private `d`; live distribute/reconstruct is `fault` + `network/liveness.go` |

**Commutativity** is the whole trick: encrypting with A then B equals B then A, so the joint ciphertext can be decrypted in any order.

### 3.5 Fault tolerance — `internal/fault`

| Piece | Role |
|---|---|
| `HeartbeatMonitor` | Alive → Suspect → TimedOut; `OnTimeout` callback |
| `HeartbeatSender` | Periodic `BroadcastHeartbeat` |
| `TimeoutManager` | Per-target vote; confirm at ⌈(n-1)·2/3⌉ yes votes |
| `SlashDetector` | Equivocation, bad ZK proof, invalid action, key withholding |
| `KeyShareStore` | Hold/contribute/reconstruct Shamir shares → `SRAKey` |
| `FaultManager` | Facade wiring the above; `OnPlayerFolded` → `forceFold` in the TUI model |

Live P2P starts `FaultManager.Run` next to `HeartbeatSender` **before** shuffle. Timeout votes go on the wire; a confirmed vote `forceFold`s **and** `BroadcastAction`s that fold. In crypto mode, survivors gossip their share of the missing `d`, reconstruct an `SRAKey` (never stored on the Keyring), and the designated survivor (`first remaining SeatOrder` id) calls `PeelOnBehalf`. Mid-shuffle timeout calls `AbortShuffle` — no delegated shuffle step.

### 3.6 Blockchain — `internal/chain` + `contracts/`

`PokerEscrow.sol`:

- States: Open → Playing → Settled / Disputed / Abandoned.
- `joinTable{value}` seats players; table starts when full.
- `reportOutcome(payoutDeltas, stateRoot, signatures, handNum)`: chip conservation (deltas sum to 0), ≥2/3 signatures, then payout.
- `submitDispute` in a 50-block challenge window; slash burns 20% (`SLASH_BURN_BPS = 2000`).
- `markAbandoned` after 1000 blocks without settlement; `refund`.

Go `Client` is a **stub** (hardcoded receipts, no real ethclient). `EscrowManager` knows how to build outcome payloads from `GameState.Payouts` + log root, sign Ethereum digests, and map slash reasons to on-chain strings. Not called from `main.go`.

### 3.7 TUI — `internal/tui`

Charm Bubble Tea + Lip Gloss. `Model` is a view over `*game.GameState`:

- Modes: Lobby, Spectate, Betting, Showdown, Error.
- `OnAction` callback: local mode applies to the machine; P2P mode `applyAndBroadcast`.
- Keyboard: `f` fold, `c` check/call, `r` raise, `a` all-in.

The P2P/local wrappers (`p2pGameModel`, `localGameModel`) implement `tea.Model` and forward `GameStateMsg` / `WinnerMsg` into this view.

### 3.8 Config — `config/`

`config.yaml`: player name, listen addr, bootstrap peers, table/seats/blinds/buy-in, heartbeat timings, optional chain RPC. Identity key isolated from the Ethereum key (`poker keygen`).

---

## 4. Data Flow & Concurrency

### End-to-end action path (P2P)

```
[TUI goroutine]  keypress
        │
        ▼
  tui.OnAction → p2pGameModel.applyAndBroadcast
        │  lock machineMu, bump sequencer nextSeq,
        │  Machine.ApplyAction locally, unlock,
        │  Node.BroadcastAction (signed envelope → GossipSub)
        │  kickCryptoAdvance (streets / reveals; Wait* without machineMu)
        │  prog.Send(GameStateMsg)
        ▼
[libp2p / GossipSub mesh]
        │
        ▼
[receiveLoop goroutine]  Gossip.NewTableMessage
        │  dispatch: verify sig, seq watermark, Gamelog.Append
        │  OnPlayerAction
        │     lock machineMu, sequencer.push
        │     ApplyAction for each in-order msg
        │     notifyCh <- struct{}{}  (non-blocking, cap 16)
        ▼
[TUI waitForUpdate cmd]  select notifyCh / 250ms tick / ctx.Done
        │  prog.Send(GameStateMsg)
        ▼
  Bubble Tea redraw
```

Self-echoes are dropped in `dispatch` (`env.SenderId == local PeerID`), which is why the acting node must apply locally before broadcast.

### Goroutines (P2P process)

| Goroutine | Lifetime | Work |
|---|---|---|
| Bubble Tea main loop | Until quit | TUI `Update`/`View`; runs `applyAndBroadcast` on the UI thread |
| `Node.receiveLoop` | `Start` → ctx cancel | Blocking `tableSub.Next`; `dispatch` |
| `Node.equivocationScanLoop` | 5s ticker | `Gamelog.DetectEquivocation` via adaptor |
| mDNS `HandlePeerFound` | Per discovery | `Connect` to new peer |
| libp2p stream handler | Per inbound `/poker/1.0.0` stream | Length-prefixed read loop |
| `HeartbeatSender.Run` | Until ctx done | Publish heartbeat every interval (`handNum` follows the live hand) |
| `FaultManager.Run` | Until ctx done | Heartbeat monitor loop |
| `HeartbeatMonitor.OnTimeout` | On timeout | Spawned `go`; starts timeout vote |
| `TimeoutManager.OnConfirmed` | On 2/3 votes | `go` → `OnPlayerFolded` → `forceFold` + share recovery |
| Slash `OnSlash` | On detection | `go` callback |

Local mode is simpler: only the Bubble Tea loop plus `tea.Tick` cmds for bot actions (600ms) and next-hand (1.5s).

### Channels

| Channel | Purpose |
|---|---|
| `Lobby.readyCh` | Closed once (`sync.Once`) when seats full and all ready. `WaitReady` is a `select` on this vs `ctx.Done` — event-driven barrier, no polling. |
| `p2pGameModel.notifyCh` | `chan struct{}`, buffer 16. Network thread pokes TUI without blocking; overflow is dropped (UI also polls every 250ms). |
| `ctx` from `signal.NotifyContext` | SIGINT/SIGTERM cancels receive loop, heartbeats, TUI. |
| GossipSub subscriptions | Internal to libp2p; exposed as blocking `Next(ctx)`. |
| `tea.Program.Send` | Thread-safe injection of `GameStateMsg` into the TUI event loop. |

### Locks and shared mutable state

| Guard | Protects |
|---|---|
| `machineMu` | `liveMachine` / `liveGS` **and** every `ApplyAction` / `ApplyStreet` / `ApplyHoleReveal`. Released during `WaitShuffle` / `WaitHoles` / `WaitStreet` / `WaitReveal` |
| `cryptoMu` | `liveHand` pointer + early shuffle/peel buffers |
| `actionSequencer.mu` | `nextSeq` + `pending` map |
| `Lobby.mu` | Seats and ready state |
| `GossipManager.mu` | Per-sender last-seq map (replay protection) |
| `Gamelog.mu` | Envelope list |
| `Node.mu` | Started flag + cached peer pubkeys |
| `HeartbeatMonitor.mu`, `TimeoutManager.mu`, `SlashDetector.mu`, `KeyShareStore.mu` | Fault-subsystem maps |

`ApplyAction` itself is **not** internally synchronized. The live loop holds `machineMu` around every apply (local TUI, remote sequencer drain, timeout fold). Street peels wait **without** that lock so recovery can fold a silent peer. Interview talking point: **the game machine is a single-threaded reducer; the network must feed it a total order.**

### Ordering and determinism tricks

1. **Gossip is not FIFO.** `PlayerAction.Seq` is a table-wide counter. Sequencer buffers holes (`got 3 before 2`) until the gap fills.
2. **Envelope `seq`** (per sender, atomic on `Node`) is a different counter — replay/dup protection, not action order.
3. **Shared seed:** `XOR(session nonce bytes)` then LCG mix `seed = seed*6364136223846793005 + 1442695040888963407`. Next hand: `sharedSeed XOR (handNum * 2654435761)`.
4. **Seat order:** `JoinedAtUnixMs` then `PlayerID` — join timestamps are taken from the sender's envelope timestamp, so clock skew can theoretically reorder seats. Honest same-network clocks are the implicit assumption.
5. **Callbacks before Start:** avoids a race where the receive goroutine runs with nil `OnPlayerAction`.

### Local vs P2P concurrency comparison

Local mode never leaves one OS thread of game logic: bots are `tea.Tick` callbacks on the same program. P2P splits **producer** (network) and **consumer** (TUI/machine) across goroutines, with channels for wakeups and mutexes for pointer swaps — a classic Go "don't communicate by sharing memory" hybrid: events travel on channels, the reducer is still shared memory.

---

## 5. Security & Cryptography

### Threat model

Players are mutually distrusting. A cheater may try to:

- See another player's hole cards.
- Stack or un-shuffle the deck.
- Equivocate (send action A to Bob, action B to Carol).
- Replay old messages.
- Impersonate a peer.
- Stall (go silent on their turn or withhold a decrypt).
- Submit a fake showdown / steal the pot.

The house is not in the trust set. Ethereum (when wired) is the settlement root of trust for money, not for card secrecy.

### Transport and authenticity

- **Noise** encrypts and authenticates each TCP session (hop security).
- **Ed25519** signs every gossip envelope over `type ‖ sender ‖ 0x00 ‖ seq ‖ timestamp ‖ payload`. Verification uses the public key extracted from the Peer ID (or a cached map).
- **Replay:** `GossipManager.CheckAndUpdateSeq` requires strictly increasing per-sender envelope seq.
- **Self-filter:** ignore own gossip echoes.
- **Max message size** 4 MiB to bound memory.
- Direct streams skip signature verify because Noise binds the stream to the remote Peer ID.

### Two dealing modes

#### A. Live default: SRA mental poker

Orchestrated by `network.CryptoHand` from `runP2PMode` (not `HandCoordinator` / `CryptoGame` — those still generate every `(e, d)` on one machine and are the test oracle).

**Setup**

1. All players share prime `p` (RFC 3526 2048-bit MODP).
2. Each player generates `(e, d)` with `gcd(e, p-1) = 1`, `d = e⁻¹ mod (p-1)`.
3. `e` is published in `JOIN_TABLE.SraPubKeyE`. `d` never leaves the node except as Shamir shares (unicast at table start).
4. `SessionID = SHA256(playerIDs in canonical order ‖ lobby nonce ‖ handNum bytes)` binds proofs and commitments to this table/hand.

**Shuffle (each player, in seat order)**

1. Start from plaintext encoding: card `i` → `2^(i+1) mod p`.
2. Player `k` encrypts every card: `c' = c^{e_k} mod p`, secret-permutes, publishes **output deck + commitment only** (`SHUFFLE_STEP`). The permutation never goes on the wire.
3. Others verify the commitment and adopt the output as the next input. Gossip is unordered; ShuffleSession sequences by **seat index**.
4. After `n` steps the deck is encrypted under all keys and permuted by all permutations.

Because SRA is commutative, the joint ciphertext is `m^{e_1 e_2 … e_n} mod p`.

**Deal hole card to player R at deck index i**

1. Ciphertext `C` is public (agreed encrypted deck).
2. Every player **except R** publishes a **partial decrypt** plus a ZK proof (`PARTIAL_DECRYPT`). Peel order is canonical seats minus R; out-of-order peels are buffered.
3. R decrypts the last layer privately (`FinishHole`). That last decrypt is **not** published. Others still see a ciphertext they cannot peel (missing `d_R`).

**Community cards**

Everyone peels. After `n` peels the plaintext is public. Burns are skipped by index (`FlopIndexes` / `TurnIndex` / `RiverIndex`), matching the engine.

**Showdown**

Public peel of remaining players' hole-card **indexes** (same as a community peel of private indexes), in **seat order on every replica** — not `MissingRevealIDs()`, which would differ per node. Then `ApplyHoleReveal`. Fold-to-winner skips this.

**Zero-knowledge proofs (`zkp.go`)**

Schnorr-like proof of correct exponentiation, Fiat–Shamir hashed with session ID:

- Prover knows `d`, publishes `h = g^d`, `A = g^r`, `B = ciphertext^r`.
- Challenge `c = SHA256(p, h, ciphertext, result, A, B, sessionID) mod (p-1)`.
- Response `s = r + c·d`.
- Verifier checks `g^s = h^c · A` and `ciphertext^s = result^c · B`.

A failed proof becomes `SlashBadZKProof`.

#### B. Debug: `--no-crypto` shared-seed plaintext shuffle

After lobby fill, every node:

```
deck = NewDeck()
rng = rand.New(rand.NewSource(sharedSeed))
deck.Shuffle(rng)
deal hole cards in seat order
```

**Properties:** deterministic, simple, all nodes stay in lockstep.  
**Security:** **none against curious peers.** Every replica holds every hole card. Use only for sync testing. Mixed tables (one peer crypto, one `--no-crypto`) are rejected.

### Commitments

Deck commitments prevent a shuffler from equivocating on what deck they output. Opening is the nonce + serialization (256-byte big-endian limbs per card).

### Key withholding and crashes

If player R disconnects **after** the shuffle, before remaining peels finish (community / showdown / leftover hole peels):

- Each player already unicast Shamir shares of their `d` at table start (`SplitAndDistribute`, threshold `max(2, (n+1)/2)`).
- Remaining peers gossip their share of R's `d`; `TryReconstructKey` recovers `(e, d)` — stored on `CryptoHand.MarkGone`, **not** on the Keyring.
- The designated survivor publishes `PeelOnBehalf` with `PlayerID = R`.
- R is folded and cannot rejoin.

If R disconnects **during shuffle**, the secret permutation is gone. The hand aborts. No delegated shuffle step.

Refusing to decrypt when required is still `SlashKeyWithholding` (logged; not submitted on-chain).

### Equivocation

`Gamelog.DetectEquivocation`: same `(sender, seq)`, different payload. Evidence is two signed envelopes — suitable for on-chain `submitDispute`. Scanned every 5s.

### Timeout / liveness

- Heartbeats on a **separate topic** so table-message backlog does not look like death.
- After `heartbeat_timeout` (default 15s), peer is TimedOut.
- Timeout vote among the other `n-1` players; 2/3 yes → local fold **and** broadcast `PLAYER_ACTION` fold (local-only fold would desync replicas).
- Crypto: also reconstruct `d` and finish peels (above). `--no-crypto`: fold only.

### On-chain economic security (intended)

Off-chain game produces:

- Payout deltas (must sum to 0 — chip conservation).
- `stateRoot` from `Gamelog`.
- ≥2/3 player signatures on the outcome digest.

Contract pays. During the challenge window, a seated player can submit slash evidence (equivocation, bad ZK, invalid action, key withholding). 20% of the accused buy-in is burned; remainder redistributed. If nobody settles before `SETTLEMENT_DEADLINE` blocks, table is Abandoned and buy-ins refund.

**Gap:** Go `Client` does not talk to an Ethereum node yet, so this loop is design-complete, integration-incomplete.

### Honest summary for interviews

| Property | Default live P2P (crypto) | `--no-crypto` debug |
|---|---|---|
| Agreement on actions | Sequenced gossip + local machine | Same |
| Hidden hole cards | Yes (recipient last decrypt is local) | No |
| Unbiased shuffle | Yes if at least one honest permuter; collusion of all still wins | No (shared seed; any peer can compute the deck) |
| Proof of honest decrypt | ZK proofs on peels | N/A |
| Crash of a key holder | Timeout fold + Shamir reconstruct (after shuffle); mid-shuffle abort | Force fold only |
| Money | Local chip counters | Same |
| Impersonation | Ed25519 / Peer ID | Same |
| Conflicting histories | Equivocation scan | Same |

The architectural thesis: **treat poker as a replicated state machine whose inputs are authenticated, totally ordered actions, and whose private inputs (cards) are supplied by a commutative encryption protocol rather than a dealer.** Both halves now run in `poker host` / `poker join`. Escrow remains specified beside the live loop.

---

## Appendix — Interview map of "where to look"

| Question they might ask | Primary files |
|---|---|
| How do peers find each other? | `network/discovery.go`, `network/host.go`, `Node.Start` |
| How is state replicated? | `cmd/poker/main.go` (`runP2PMode`, `actionSequencer`), `game/machine.go` |
| Why not just send GameState? | Section 2; proto still has `GAME_STATE_SYNC` for future catch-up |
| How would you hide cards? | `crypto/keyring.go`, `shuffle_session.go`, `deal_session.go`, `network/crypto_hand.go`, `cmd/poker/main.go` (`dealCryptoHand`) |
| What if GossipSub reorders? | `actionSequencer` in `main.go`; shuffle/peel sequencers inside the sessions |
| What if a player disconnects? | `fault/heartbeat.go`, `timeout.go`, `shamir.go`, `network/liveness.go`, `DealSession.PeelOnBehalf` |
| How do you pay winners without a house? | `contracts/PokerEscrow.sol`, `chain/escrow.go` |
| How do you detect a double-talker? | `network/gamelog.go` `DetectEquivocation` |
| Local vs distributed? | `runLocalMode` vs `runP2PMode` in `main.go` |

---

*Generated from a full-repo scan. Live dealing is SRA on `host`/`join`; chain RPC is still unwired.*
