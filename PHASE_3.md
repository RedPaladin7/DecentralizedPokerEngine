# Phase 3 — Mesh, lobby, sequenced actions

This is the third onboarding chapter. After it you should be able to **explain how four laptops find each other, how bytes move (gossip vs direct streams), how joins become a seat order, and how unordered GossipSub becomes a total order of `PLAYER_ACTION`s.** You still do not need to understand SRA peels or escrow.

The reading list this chapter expands is in [`READ_GUIDE.md`](./READ_GUIDE.md). The teaching narrative it sits next to is [`HOW_IT_WORKS.md`](./HOW_IT_WORKS.md) §§4–6 and 11–15. After the mesh files, read [`SYSTEMS_DESIGN_INTERVIEW.md`](./SYSTEMS_DESIGN_INTERVIEW.md). Phase 2’s reducer is the function this log is for.

**You are here to learn:** public game state is a replicated log, not a broadcast of `GameState`. Networking produces authenticated, ordered inputs. The engine from Phase 2 reduces them. The host is a bootstrap peer, not a game server.

**Do this with your hands before you finish the chapter:** three terminals, same machine, different `--listen` ports, **`--no-crypto` on every peer**. Watch joins, ready, then a betting round. You are studying *order of actions*, not ciphertexts. Then run:

```bash
go test ./internal/network -count=1
```

**Do not deep-read yet:** `crypto_hand.go`, `internal/crypto`, `internal/fault`. Know that `Node` has `OnShuffleStep` / `OnPartialDecrypt` callbacks; Phase 4 fills them. Heartbeats, timeout votes, and Shamir shares exist on the wire in this chapter so you can recognize the types; their *policy* is Phase 5.

**Architectural rule to keep in your head the whole time:** `internal/game` never imports `internal/network`. Mixing those layers is how “the host accidentally becomes the server” happens.

---

## Table of contents

1. [How to use this chapter](#1-how-to-use-this-chapter)
2. [The one idea: a replicated log](#2-the-one-idea-a-replicated-log)
3. [What `poker host` actually is](#3-what-poker-host-actually-is)
4. [Package map](#4-package-map)
5. [Wire vocabulary (`messages.proto`)](#5-wire-vocabulary-messagesproto)
6. [Two sequence spaces](#6-two-sequence-spaces)
7. [Codec: framing, sign, verify](#7-codec-framing-sign-verify)
8. [Identity and the libp2p host](#8-identity-and-the-libp2p-host)
9. [Finding peers: mDNS and bootstrap](#9-finding-peers-mdns-and-bootstrap)
10. [GossipSub: two topics, echo, forwarding](#10-gossipsub-two-topics-echo-forwarding)
11. [Direct streams (`/poker/1.0.0`)](#11-direct-streams-poker100)
12. [The lobby](#12-the-lobby)
13. [Gamelog: evidence, not consensus](#13-gamelog-evidence-not-consensus)
14. [`Node`: composition root of a peer](#14-node-composition-root-of-a-peer)
15. [Call graph from `runP2PMode`](#15-call-graph-from-runp2pmode)
16. [The action sequencer](#16-the-action-sequencer)
17. [Worked example: three terminals fill a lobby](#17-worked-example-three-terminals-fill-a-lobby)
18. [Worked example: a fold, hop by hop](#18-worked-example-a-fold-hop-by-hop)
19. [`--no-crypto` as the Phase 3 lab](#19---no-crypto-as-the-phase-3-lab)
20. [`HandCoordinator`: the trap](#20-handcoordinator-the-trap)
21. [Tests in this phase](#21-tests-in-this-phase)
22. [Interview-shaped recap](#22-interview-shaped-recap)
23. [Common mistakes](#23-common-mistakes)
24. [Exit check](#24-exit-check)
25. [Phase 3 glossary](#25-phase-3-glossary)

---

## 1. How to use this chapter

Read top to bottom once. When a code excerpt appears, open that file in the editor and match the excerpt to the live source. Line numbers here were accurate when this chapter was written; if they drift, trust the file.

This chapter is **not** a rewrite of [`HOW_IT_WORKS.md`](./HOW_IT_WORKS.md) §§4–6 or 11–15. Those sections are the vocabulary and the hop-by-hop story. This file is the types, the call graph, two worked examples, and the mistakes people make the week they first edit `node.go` or `actionSequencer`.

Suggested time: one sitting after Phase 2, including a three-process `--no-crypto` table. Stop when the [exit check](#24-exit-check) is true.

File order matches the read guide. Do not skip to `node.go` before `messages.proto` / `codec.go` / `host.go` make sense — `Node` is a composition of those types, not a second engine.

---

## 2. The one idea: a replicated log

Phase 2 burned this in:

```
identical GameState  +  identical ordered Action list  +  identical card inputs
        →  identical pots, stacks, and PhaseSettled
```

This chapter is how the **ordered Action list** arrives from other laptops.

A beginner instinct is: Alice’s client computes the new pot and broadcasts `GameState`. That makes Alice a server. Bob has to trust her snapshot. Two honest replicas would fight over whose blob is newer.

Instead Alice broadcasts “I fold” (or “I raise 40”) with a **table-wide sequence number**. Every replica calls the same `ApplyAction` Phase 2 already taught you.

[`HOW_IT_WORKS.md`](./HOW_IT_WORKS.md) §15 puts it on a whiteboard:

```
identical initial state  +  identical ordered actions  =  identical GameState
```

GossipSub will **not** give you that order. It is unordered and best-effort. The application layer restores a total order with `actionSequencer` in `cmd/poker/main.go`. That type does **not** live in `internal/network`. Networking delivers authenticated bytes. `main` sequences them. `game.Machine` reduces them.

`HAND_RESULT` on the wire is informational. A lying result does not move chips on an honest replica. `GAME_STATE_SYNC` exists in the proto and is unused. Disconnect is terminal for that player in v1.

Two transports, on purpose:

| Transport | Job in this phase |
|---|---|
| GossipSub topic `poker/table/<id>` | Everyone must see this: joins, ready, actions |
| Direct stream `/poker/1.0.0` | Unicast: Shamir shares of `d` (Phase 5), best-effort hole peels (Phase 4) |

If you remember only one sentence: **public state is a log of signed actions, not a screenshot of the table.**

---

## 3. What `poker host` actually is

[`HOW_IT_WORKS.md`](./HOW_IT_WORKS.md) §5.3: someone has to listen first, or there is nobody to connect to.

`poker host --seats 3 --name Alice --table friday` does three unglamorous things:

1. Binds a TCP port and prints multiaddrs.
2. Advertises itself on the LAN via mDNS (service tag `p2p-poker-v1`).
3. Creates the lobby for table id `friday` with three seats.

After Bob and Carol join, Alice is not the dealer, not the sequencer, and not the judge of the pot. If you draw a star with Alice in the middle *as a game server*, you have misunderstood the design. She is the **bootstrap peer**.

`runHost` and `runJoin` both call the same function:

```269:290:cmd/poker/main.go
// runHost: poker host [--seats N] [--name NAME] [--table ID] [--listen ADDR] [--no-crypto]
func runHost(ctx context.Context, args []string) error {
	cfg := config.Default()
	cfg.Network.ListenAddr = "/ip4/0.0.0.0/tcp/9000"
	noCrypto := applyP2PFlags(cfg, args)
	fmt.Println("Starting as HOST — share the address below with other players.")
	return runP2PMode(ctx, cfg, noCrypto)
}

// runJoin: poker join --peer MULTIADDR [--name NAME] [--table ID] [--listen ADDR] [--no-crypto]
func runJoin(ctx context.Context, args []string) error {
	cfg := config.Default()
	cfg.Network.ListenAddr = "/ip4/0.0.0.0/tcp/9001"
	noCrypto := applyP2PFlags(cfg, args)
	fmt.Println("Starting as JOINER — connecting to the host…")
	return runP2PMode(ctx, cfg, noCrypto)
}
```

The only durable differences: default listen port (9000 vs 9001) and a print line. Joiners typically pass `--peer` so they can dial before mDNS fires. After `Node.Start()`, every process publishes and subscribes the same way.

`applyP2PFlags` mutates the same `config.Config` Phase 1 taught you: `--seats`, `--listen`, `--name`, `--table`, `--peer`, `--no-crypto`. Table id is an **application-level** filter. mDNS finds poker processes, not a specific Friday night table. If Bob joins `--table saturday` he will mesh at the libp2p layer and sit in a different lobby.

---

## 4. Package map

This phase lives in `internal/network` plus one type in `cmd/poker`. `internal/game` does not import any of it.

```
internal/network/
  messages.proto        Wire vocabulary. Dual counters live here. Skip messages.pb.go
  messages.pb.go        Generated. Do not study
  codec.go              Length-prefix, Ed25519 sign/verify, big.Int / KeyShare adapters
  host.go               libp2p host: identity, listen, Noise, TCP, no relays
  discovery.go          mDNS tag p2p-poker-v1
  gossip.go             Two GossipSub topics; per-sender envelope-seq watermark
  protocol.go           Direct streams /poker/1.0.0; StreamPool
  lobby.go              Seats, canonical order, ready barrier, session nonce
  gamelog.go            Append-only envelopes; evidence, not consensus
  node.go               Composition root: host + gossip + lobby + log + callbacks
  coordinator.go        In-process oracle helper — not the live loop
  network_test.go       Lobby, codec, replay, gossip, node wiring

cmd/poker/main.go
  actionSequencer       Table-wide total order of PLAYER_ACTION (not in network/)
  p2pGameModel          applyAndBroadcast, OnPlayerAction → sequencer → ApplyAction
```

`crypto_hand.go`, `liveness.go`, and `fault_adaptor.go` sit in the same package. Do not deep-read them this week. `Node` exposing `OnShuffleStep` is enough.

The composition rule: **wire every `OnXxx` callback before `Node.Start()`**. `Start` launches `receiveLoop` immediately. A nil callback silently drops the message.

---

## 5. Wire vocabulary (`messages.proto`)

Read [`internal/network/messages.proto`](./internal/network/messages.proto). Do not read `messages.pb.go`. Protobuf here is a structured binary encoding. The interesting part is which fields exist, and which messages the live loop actually handles.

### 5.1 Envelope

Every gossip payload is wrapped:

```22:29:internal/network/messages.proto
message Envelope {
    MsgType type = 1;
    string sender_id = 2;
    int64 seq = 3;
    int64 timestamp = 4;
    bytes payload = 5;
    bytes signature = 6;
}
```

- `sender_id` is the libp2p Peer ID string (`12D3KooW…`), not a display name.
- `seq` is **per sender**, strictly increasing, for replay protection. Not game order.
- `timestamp` is the sender’s Unix millis. Lobby seat order uses it. Clock skew can reorder seats (see §12.2).
- `payload` is itself protobuf: a `JoinTable`, a `PlayerAction`, …
- `signature` is Ed25519 over a canonical byte string (see §7). It is **not** Noise. Noise authenticated the last hop.

### 5.2 Message types

| `MsgType` | Live path in this phase | Later |
|---|---|---|
| `JOIN_TABLE` | Seat claim: name, buy-in, public SRA `e`, session nonce | `e` empty means `--no-crypto` |
| `PLAYER_READY` | Barrier: I am willing to start | |
| `PLAYER_ACTION` | Fold/check/call/raise/all-in + **table-wide** `Seq` | This chapter’s sequencer |
| `HAND_RESULT` | Informational pots / state root after settle | Does not move chips |
| `SHUFFLE_STEP` | Callback exists; early messages buffered in `main` | Phase 4 |
| `PARTIAL_DECRYPT` | Same: gossip + best-effort stream | Phase 4 |
| `HEARTBEAT` | Published on a **separate** topic | Phase 5 |
| `TIMEOUT_VOTE` | Wired to `FaultManager` | Phase 5 |
| `KEY_SHARE` | Unicast at table start; gossip after timeout | Phase 5 |
| `GAME_STATE_SYNC` | Defined | **Unused** (no mid-hand reconnect) |
| `SHUFFLE_COMMIT` | Defined | **Unused** (`SHUFFLE_STEP` already carries the commitment) |
| `LEAVE_TABLE` | Defined | **Unused** in `dispatch` |
| `EQUIVOCATION_EVIDENCE` | Defined | Detection exists; see §13 |

A packet capture of an honest **crypto** table should not show `A♠`. Shuffle steps carry bytes (big integers). `--no-crypto` still sends `PLAYER_ACTION` the same way; the cards are local, the actions are still the log.

### 5.3 Payloads you must know this week

**Join** — what a seat is, on the wire:

```31:37:internal/network/messages.proto
message JoinTable {
    string table_id = 1;
    string player_name = 2;
    int64 buy_in = 3;
    bytes sra_pub_key_e = 4;
    bytes session_nonce = 5;
}
```

`sra_pub_key_e` is empty when the process passed `--no-crypto`. A mixed table (one peer omitted `e`, others did not) **errors out** in `runP2PMode`. Silent fallback to plaintext would be a privacy disaster. Phase 4 will make that sentence loud; this week, treat empty `e` as “this peer is on the debug path.”

**Action** — two `seq` fields in one mental model, only one of them here:

```77:84:internal/network/messages.proto
message PlayerAction {
    string table_id = 1;
    int64 hand_num = 2;
    string player_id = 3;
    int32 action = 4;
    int64 amount = 5;
    int64 seq = 6;
}
```

`PlayerAction.seq` is the **table-wide** counter. `Envelope.seq` is a different number on the wrapper. Mixing them up is the most common systems bug in this repo.

**KeyShare** is in this proto as documentation of the shape. The generated `messages.pb.go` does **not** currently contain a `KeyShare` type. The live path uses a hand-rolled struct and `protowire` marshal in `codec.go`. If you re-run `protoc` (the command in `extra.txt`) you will collide with that type. Do not regenerate protobuf “to clean things up” this week.

---

## 6. Two sequence spaces

Write this on a sticky note and leave it on the monitor:

| Counter | Scope | Job | Gaps |
|---|---|---|---|
| Envelope `seq` | Per sender | Drop duplicates and replays | **Allowed.** Seq 1 then 10 is fine |
| `PlayerAction.Seq` | Whole table | Total order of `ApplyAction` | **Not allowed to apply past a gap.** Seq 3 waits for 2 |

Tests lock the envelope-seq rule:

```496:507:internal/network/network_test.go
func TestReplayProtection_StrictlyIncreasing(t *testing.T) {
	gm := &GossipManager{seqNums: make(map[string]int64)}

	if err := gm.CheckAndUpdateSeq("alice", 1); err != nil {
		t.Fatalf("seq 1 should be accepted: %v", err)
	}
	if err := gm.CheckAndUpdateSeq("alice", 2); err != nil {
		t.Fatalf("seq 2 should be accepted: %v", err)
	}
	if err := gm.CheckAndUpdateSeq("alice", 10); err != nil {
		t.Fatalf("seq 10 should be accepted (gaps allowed): %v", err)
	}
}
```

Duplicates and older envelope seqs are rejected. Independent senders have independent watermarks: Alice at 5 does not block Bob at 1.

If envelope seq *rejected* gaps, a lost heartbeat-sized message would permanently mute a player. If action seq *accepted* gaps, replicas would apply raise-before-call and diverge.

Shuffle steps and peels have **their own** sequencers inside `ShuffleSession` / `DealSession`, keyed by **seat index**, not by this table-wide action counter. Mixing those would be wrong: a peel is not a betting action. Phase 4.

---

## 7. Codec: framing, sign, verify

[`codec.go`](./internal/network/codec.go) is the translator between Go structs and bytes. Two jobs: **frame** a protobuf envelope for a byte stream, and **sign** it so a forwarded gossip payload still proves authorship.

### 7.1 Length prefix

TCP (and a libp2p stream) is a pipe of bytes, not messages. Alice can write 100 then 50; Bob might read 150 at once. The application prefixes each envelope:

```
[ 4 bytes: length N, big-endian ] [ N bytes: protobuf Envelope ]
```

`MaxMessageSize` is 4 MiB so a hostile peer cannot ask you to allocate a gigabyte. Shuffle steps are the large ones: 52 × ~256-byte integers — tens of kilobytes, not megabytes.

```19:37:internal/network/codec.go
const MaxMessageSize = 4 * 1024 * 1024 // prevent memory exhaustion attack
// framing scheme => first 4 bytes tell the length of the message (bigEndian)

func EncodeEnvelope(env *Envelope, privKey ed25519.PrivateKey) ([]byte, error) {
	sigData := envelopeSignBytes(env)
	env.Signature = ed25519.Sign(privKey, sigData) // signing contents of data with private key

	b, err := proto.Marshal(env) // struct to bytes
	if err != nil {
		return nil, fmt.Errorf("")
	}
	if len(b) > MaxMessageSize {
		return nil, fmt.Errorf("")
	}

	frame := make([]byte, 4+len(b)) // adding length of message as prefix
	binary.BigEndian.PutUint32(frame[:4], uint32(len(b)))
	copy(frame[4:], b)
	return frame, nil
}
```

`DecodeEnvelope` reads the length, bounds it, unmarshals, then optionally verifies.

### 7.2 What is signed

```72:86:internal/network/codec.go
func envelopeSignBytes(env *Envelope) []byte {
	// signature type => type + sender + 0x00 + seq + timestamp + payload
	buf := make([]byte, 0, 1+len(env.SenderId)+1+8+8+len(env.Payload))
	buf = append(buf, byte(env.Type))
	buf = append(buf, []byte(env.SenderId)...)
	buf = append(buf, 0x00)
	var seq [8]byte
	binary.BigEndian.PutUint64(seq[:], uint64(env.Seq))
	buf = append(buf, seq[:]...)
	var ts [8]byte
	binary.BigEndian.PutUint64(ts[:], uint64(env.Timestamp))
	buf = append(buf, ts[:]...)
	buf = append(buf, env.Payload...)
	return buf
}
```

The signature covers type, sender, envelope seq, timestamp, and payload. It does **not** cover the signature field itself (that would be circular). Verification uses the public key extracted from the Peer ID, or a cached copy in `Node.peers`.

`TestEncodeDecodeEnvelope_WrongKey_Rejected` is the contract: a frame signed by key A does not verify under key B.

### 7.3 When verification is skipped

`DecodeEnvelope(..., nil)` skips the signature check. Direct streams use that path:

```67:68:internal/network/protocol.go
			// decoding the envelope
			env, err := DecodeEnvelope(frame, nil)
```

The comment in `gossip.go` states the reason: Noise already binds a stream to the remote Peer ID. Gossip is forwarded by neighbors, so hop auth is not author auth. **Do not copy the `nil` pubKeyFn into the gossip receive path.**

### 7.4 Adapters you can ignore until Phase 4

`BigIntToBytes` / `DeckToWire` / `ZKProofToWire` / `ShuffleMessageToWire` / `PeelMessageFromWire` exist so crypto types can cross the protobuf boundary without putting ranks on the wire. Skim the names. Do not trace a peel this week.

`KeyShare` marshal is hand-rolled `protowire` (compatible with the proto field numbers). Live unicast shares go through `MarshalKeyShare` / `UnmarshalKeyShare`, not `proto.Marshal`.

---

## 8. Identity and the libp2p host

Package: [`host.go`](./internal/network/host.go). Library: [libp2p](https://libp2p.io/). Phase 1 already told you there are two keys; this is where the Ed25519 seed becomes a process on the mesh.

### 8.1 Why not “Alice on 192.168.1.100”?

IP addresses change. Display names collide. A cheater can claim to be Alice. Stable identity is a **key**.

`runP2PMode` loads `~/.poker/identity.key` (64 bytes) and passes it into `NewNode` → `NewPokerHost`. If the seed is 64 bytes, the host restores that Ed25519 key. Otherwise it generates a fresh pair (tests pass `nil` and listen on `/ip4/127.0.0.1/tcp/0`).

Two things are derived from the same key:

1. The **libp2p Peer ID** — a hash of the public key. Printed at startup. This is `Player.ID` in the lobby and the engine.
2. The key used to **sign gossip envelopes** (`PokerHost.Ed25519PK`).

If you copy `identity.key` to another laptop, you *are* the same peer. If you delete it, you are a new person as far as the table is concerned. This identity is **not** the Ethereum key from `poker keygen`.

### 8.2 Constructing the host

```59:75:internal/network/host.go
	h, err := libp2p.New(
		libp2p.Identity(libPrivKey),
		libp2p.ListenAddrs(maddr),
		libp2p.Security(noise.ID, noise.New), // noise protocol, encryption and authentication of connection between 2 peers
		libp2p.Transport(tcp.NewTCPTransport),
		libp2p.DisableRelay(), // direction comm not middle men
		libp2p.NATPortMap(), // asking home router to open hole in network firewall to let players from internet connect
	)
	if err != nil {
		return nil, fmt.Errorf("")
	}

	return &PokerHost{
		Host: h,
		Ed25519PK: rawEd,
		PeerID: h.ID().String(),
	}, nil
```

| Knob | Meaning |
|---|---|
| Listen multiaddr | Default `/ip4/0.0.0.0/tcp/9000`. Two processes on one laptop cannot both bind 9000 |
| Noise | Confidentiality + hop authentication on each TCP session |
| TCP | Reliable ordered bytes **per connection**. Overlay-wide gossip is still unordered |
| Relays off | No circuit-relay helper. NAT is UPnP (`NATPortMap`) only |
| Multiaddr suffix | `Addrs()` appends `/p2p/<PeerID>` so a joiner knows who should answer |

A multiaddr such as `/ip4/192.168.1.100/tcp/9000/p2p/12D3KooWAlice...` reads left to right: IPv4, this host, TCP, this port, and the peer key you expect. If someone else answers, the handshake fails. That is stronger than a hostname.

### 8.3 Connecting

`PokerHost.Connect` parses a multiaddr, writes the address into the **peerstore** (local address book: peer id → addresses, `PermanentAddrTTL`), then dials TCP and completes Noise.

Once Alice and Bob have one TCP connection, libp2p can open many **streams** on it (Yamux). Opening `/poker/1.0.0` to send a Shamir share does **not** mean a new TCP handshake.

---

## 9. Finding peers: mDNS and bootstrap

Before you can play you must answer: *who else is running this program, and how do I dial them?* There is no company directory. Two mechanisms, composed in `Node.Start()`.

### 9.1 mDNS on the LAN

**mDNS** (multicast DNS) is how printers show up on Wi-Fi without an IP. This project’s service tag is `p2p-poker-v1`.

```16:16:internal/network/discovery.go
const PokerServiceTag = "p2p-poker-v1"
```

When a peer is found, `HandlePeerFound` appends them to a local list and fires `onFound` on a **new goroutine**. `Node.Start` sets that callback to: add addresses to the peerstore (10-minute TTL), then `Connect`.

Limitations you should expect:

- Same LAN (same multicast domain). A phone hotspot and a dorm VLAN may not share multicast.
- Windows multicast is flaky in tests; a real three-terminal demo on one machine with explicit `--listen` / `--peer` is the reliable lab.
- mDNS finds *poker processes*, not a table id. `--table friday` is applied after you are connected.

`WaitForPeers` exists on `MDNSDiscovery` and is **not** what `runP2PMode` uses. Lobby fill polls `Lobby.Count()`, not mDNS peer count. You can be connected to six poker processes and still wait on a three-seat Friday table.

### 9.2 Bootstrap multiaddr

Joiners pass `--peer "/ip4/.../tcp/.../p2p/..."`. That lands in `cfg.Network.BootstrapPeers` and is dialed in `Start`. Connection failure is **non-fatal**; join is retried a few times while the mesh forms.

This is how you play across networks, *if* the address is reachable. Relays are off. If UPnP fails, someone must copy a reachable multiaddr or the join never happens. That is a networking limitation, not a poker-rules limitation.

### 9.3 What does not exist

No Kademlia DHT. No bootstrap server operated by the project. No circuit relay. If discovery fails, the lobby stays at “Waiting for players” forever. That is the first troubleshooting item in the README, and it is a **network** failure, not a crypto failure.

---

## 10. GossipSub: two topics, echo, forwarding

[`gossip.go`](./internal/network/gossip.go) joins two topics:

| Topic | Purpose |
|---|---|
| `poker/table/<tableID>` | Joins, ready, shuffle steps, peels, actions, votes, results |
| `poker/heartbeat/<tableID>` | Liveness pings, **separate** so a slow table mesh does not look like a dead player |

You **publish** a byte frame (already length-prefixed and signed). Every subscriber eventually gets a copy, typically from a **neighbor**, not from the original author. `NewTableMessage` returns `msg.Data` and `msg.ReceivedFrom`. `ReceivedFrom` is the neighbor. The original author is inside the signed envelope (`sender_id`).

Properties that drive the rest of the design:

- **Unordered.** Bob can see action 3 before action 2.
- **Best-effort.** A message can be dropped. The sequencer then waits forever (a known gap: no NAK/retransmit in v1).
- **Echo.** You receive your own message back. `dispatch` drops envelopes whose `sender_id` is you. That is why the acting player **applies locally first**, then broadcasts — otherwise their UI would wait for a round trip to themselves.
- **Forwarding.** Signatures are mandatory. Noise only authenticated the last hop.

Replay watermark:

```108:117:internal/network/gossip.go
func (gm *GossipManager) CheckAndUpdateSeq(senderID string, seq int64) error {
	gm.mu.Lock()
	defer gm.mu.Unlock()
	last, exists := gm.seqNums[senderID]
	if exists && seq <= last {
		return fmt.Errorf("")
	}
	// new message must have greater seq num than the last received
	gm.seqNums[senderID] = seq
	return nil
}
```

Honest note for later: `BroadcastHeartbeat` publishes on the heartbeat topic, and `NewHeartbeatMessage` exists, but `Node.receiveLoop` only reads the **table** topic. Heartbeat *policy* (who last spoke, timeout votes) is Phase 5. Do not assume a second receive loop this week; the two-topic split is the design, the table loop is what you will trace for actions.

---

## 11. Direct streams (`/poker/1.0.0`)

[`protocol.go`](./internal/network/protocol.go) registers a stream handler on protocol id `/poker/1.0.0`. Frames are the same 4-byte length prefix, max 4 MiB. Direct streams carry:

- **Unicast Shamir shares** of `d` at table start (never gossiped while the player is alive — a gossiped live share would let too many people collect reconstruction material casually).
- **Hole peels**, best-effort. Gossip is still the authoritative peel path so a missed direct message does not brick the hand.

```20:21:internal/network/protocol.go
// id so the receiver knows which handler to dispatch to
const PokerProtocolID = protocol.ID("/poker/1.0.0")
```

`SendDirect` opens a new stream per message and closes it. `StreamPool.Send` reuses a stream per peer and drops it on write error. `Node` uses the pool for peels and key shares.

`RegisterProtocolHandler` decodes with `pubKeyFn == nil` (Noise binds the remote Peer ID) and, on `Node.Start`, only unmarshals `PARTIAL_DECRYPT` and `KEY_SHARE`. Joins and actions do **not** travel this path.

`SendPeel` (Phase 4) always gossips first, then best-effort unicasts to every other seat. A stream failure must not fail the hand; duplicates are ignored.

---

## 12. The lobby

[`lobby.go`](./internal/network/lobby.go) is everything that happens **before** the first shuffle (or, this week, before the shared-seed `StartHand`).

### 12.1 Seats and states

```17:46:internal/network/lobby.go
type LobbyState int

const (
	LobbyWaiting LobbyState = iota
	LobbyReady
	LobbyPlaying
)

// info that we have on each player in lobby
type SeatInfo struct {
	PlayerID       string
	PlayerName     string
	BuyIn          int64
	SRAKeyE        []byte
	Nonce          []byte
	IsReady        bool
	JoinedAt       time.Time
	JoinedAtUnixMs int64
}
```

A `Lobby` is a map of peer id → `SeatInfo`. `maxSeats` comes from config / `--seats`. P2P requires 3–9 (`requireP2PSeats`). `NewNode` used to hardcode `maxSeats=9`; that is fixed. If you reintroduce the constant 9, a three-seat table never trips `Count() >= maxSeats`.

`HandleJoin` rejects: not `LobbyWaiting`, table full, duplicate peer, non-positive buy-in. It copies `SraPubKeyE` so a caller cannot mutate the seat by editing the join message after the fact (`TestLobby_StoresSRAPubKeyE`).

Join timestamp: if the caller passes `senderTimestamp` (the envelope’s timestamp), that value is stored. `Node.dispatch` does this. `BroadcastJoin` applies locally with the same millis it puts on the envelope, so the sender and the receivers agree **if they all saw that envelope**. `HandleJoin` without a timestamp falls back to `time.Now()` — that path is for tests that do not care about cross-replica order.

### 12.2 Canonical seat order

Every honest node that saw the same joins must agree who is seat 0. Order is:

1. `JoinedAtUnixMs` ascending (sender’s envelope timestamp),
2. then `PlayerID` string compare.

```147:152:internal/network/lobby.go
	sort.Slice(out, func(i, j int) bool {
		if out[i].JoinedAtUnixMs != out[j].JoinedAtUnixMs {
			return out[i].JoinedAtUnixMs < out[j].JoinedAtUnixMs
		}
		return out[i].PlayerID < out[j].PlayerID
	})
```

That order is used for: dealer rotation, who shuffles first (Phase 4), peel order, designated survivor after a crash (Phase 5), and (in `--no-crypto`) the shared seed.

`TestLobby_PlayerIDs_InJoinOrder` and `TestLobby_SameTimestamp_PeerIDTiebreaker` lock it. Tests pass explicit timestamps so `time.Now()` cannot flake.

**Footgun:** join timestamps are sender-stamped. Clock skew can theoretically reorder seats, which would desynchronize shuffle turns and `--no-crypto` decks. The assumption is honest, roughly synchronized LAN clocks. A production system would order by a hash of peer ids or a commit-reveal, not wall clocks. [`SYSTEMS_DESIGN_INTERVIEW.md`](./SYSTEMS_DESIGN_INTERVIEW.md) wants you to say that out loud.

### 12.3 Ready barrier

When `runP2PMode` sees `Lobby.Count() >= maxSeats`, it `BroadcastReady`. Fill wait is a 250 ms poll on count. Ready wait inside the lobby type is event-driven:

```112:135:internal/network/lobby.go
func (l *Lobby) checkAllReady() {
	if len(l.seats) < l.maxSeats {
		return
	}
	for _, s := range l.seats {
		if !s.IsReady {
			return
		}
	}
	if l.state == LobbyWaiting {
		l.state = LobbyReady
		l.once.Do(func() { close(l.readyCh) })
	}
}

func (l *Lobby) WaitReady(ctx context.Context) error {
	// event driven, no cpu usage while waiting
	select {
	case <-l.readyCh: // fired when channel closed
		return nil
	case <-ctx.Done(): // fired on time out
		return fmt.Errorf("")
	}
}
```

`sync.Once` so the channel closes exactly once. `Reset()` (next hand) allocates a new channel and a new `Once`. The live loop’s 2 s sleep after broadcasting ready is a LAN fudge so ready messages propagate; it is not consensus. `HandCoordinator.WaitReady` uses the channel; `runP2PMode` currently sleeps instead of calling `WaitReady`. Both are “everyone said ready,” one of them is a timer.

### 12.4 Session binding

Each join carries a **session nonce**. In `BroadcastJoin` that is the peer’s own id bytes. Concatenated in seat order:

```223:230:internal/network/lobby.go
func (l *Lobby) SessionNonce() []byte {
	seats := l.Seats()
	var combined []byte
	for _, s := range seats {
		combined = append(combined, s.Nonce...)
	}
	return combined
}
```

Those bytes bind this table:

- Crypto (Phase 4): mixed into `SessionID = SHA256(playerIDs ‖ nonce ‖ handNum)` so ZK proofs cannot be replayed across tables or hands.
- `--no-crypto` (this week): mixed into a public RNG seed so every replica Fisher–Yates-shuffles the same plaintext deck.

`AllSeatsHavePublicE` / `KeyringFromLobby` fail closed if any seat omitted `e`. Empty `e` is a 0-length slice, not an error **until** you ask for a Keyring.

`BroadcastJoin` applies the join to the **local** lobby before publishing, because GossipSub will echo it and `dispatch` drops self:

```327:352:internal/network/node.go
func (n *Node) BroadcastJoin(ctx context.Context, handNum int64) error {
	var eBytes []byte
	if n.sraKey != nil {
		eBytes = n.sraKey.PublicKey().Bytes()
	}
	msg := &JoinTable{
		TableId:      n.tableID,
		PlayerName:   n.playerName,
		BuyIn:        n.buyIn,
		SraPubKeyE:   eBytes, // nil/empty when --no-crypto
		SessionNonce: []byte(n.Host.PeerID),
	}
	// ...
	selfTimestamp := time.Now().UnixMilli()
	_ = n.Lobby.HandleJoin(msg, n.Host.PeerID, selfTimestamp)
	// ... EncodeEnvelope, Gossip.Publish
```

`BroadcastReady` does the same local `HandleReady` then `publish`. Apply-local-then-broadcast is a repeating pattern: joins, readies, and (in `main`) actions.

---

## 13. Gamelog: evidence, not consensus

[`gamelog.go`](./internal/network/gamelog.go) is an append-only list of envelopes plus a set keyed `senderId:seq`. It is a **paper trail** for disputes, not Raft, not a committed index, not the thing `ApplyAction` reads.

```30:41:internal/network/gamelog.go
func (gl *Gamelog) Append(env *Envelope) error {
	gl.mu.Lock()
	defer gl.mu.Unlock()
	
	key := fmt.Sprintf("%s:%d", env.SenderId, env.Seq)
	if _, exists := gl.byKey[key]; exists {
		return fmt.Errorf("")
	}
	gl.entries = append(gl.entries, env)
	gl.byKey[key] = struct{}{}
	return nil
}
```

`dispatch` appends after signature verify and envelope-seq check. Duplicate `(sender, envelope seq)` is rejected. That is why `DetectEquivocation` in tests **injects** a second entry into `gl.entries` directly: the live `Append` path cannot store two payloads for the same key. Equivocation as a *concept* is “same seq, different payload, both signed.” The scanner in `Node.equivocationScanLoop` runs every 5 s via `NewGameLogFaultAdaptor`. Treat slash records as Phase 5; this week, know the log exists and what `StateRoot` hashes.

`StateRoot()` is SHA-256 over every entry’s type, sender, seq, payload, **and signature**. It is intended as an on-chain fingerprint, not as something the engine consults.

`ValidateSequences` checks that each listed player’s envelope seqs are `1..n` with no gaps. Envelope seqs on the live gossip path **allow** gaps (heartbeats and dropped messages). Do not call `ValidateSequences` as if it were the action sequencer.

`SetHandNum` on `Node` replaces the whole log with a fresh `NewGameLog`. It does not replay.

---

## 14. `Node`: composition root of a peer

[`node.go`](./internal/network/node.go) is “a complete player”: host + gossip + lobby + log + discovery + stream pool + callbacks. It is both client and server. It is **not** the Hold’em engine and **not** the action sequencer.

```21:57:internal/network/node.go
// Node represents a complete player in the P2P network.
// It is both a client (sends messages) and a server (receives messages).
type Node struct {
	Host      *PokerHost     // libp2p host — manages connections, peerstore, mux
	Gossip    *GossipManager // GossipSub publisher/subscriber
	Lobby     *Lobby         // tracks who has joined and who is ready
	Log       *Gamelog       // append-only per-hand evidence log
	Discovery *MDNSDiscovery // LAN peer discovery
	// ...
	// Callbacks — set these BEFORE calling Start().
	OnJoinTable      func(*JoinTable, string)
	OnPlayerReady    func(*PlayerReady, string)
	OnShuffleStep    func(*ShuffleStep)
	OnPartialDecrypt func(*PartialDecrypt)
	OnPlayerAction   func(*PlayerAction)
	// ...
}
```

### 14.1 `NewNode` then `Start`

`NewNode` builds host + gossip + lobby (with the **configured** `maxSeats`) + empty log + stream pool. It does not listen for application messages yet.

`Start`:

1. Registers the `/poker/1.0.0` handler (peels and key shares only).
2. Starts mDNS; on found peer, peerstore + `Connect`.
3. Dials bootstrap peers (errors ignored).
4. `go receiveLoop` and `go equivocationScanLoop`.

If you set `OnPlayerAction` after `Start`, an action that arrives in the gap is dropped. `runP2PMode` wires callbacks first. The comment at the top of that function is load-bearing.

### 14.2 `dispatch`

```176:190:internal/network/node.go
func (n *Node) dispatch(data []byte) {
	env, err := DecodeEnvelope(data, n.lookupPubKey)
	if err != nil {
		return
	}
	// Ignore our own messages echoed back by GossipSub.
	if env.SenderId == n.Host.PeerID {
		return
	}
	// Replay protection: drop old or duplicate sequence numbers.
	if err := n.Gossip.CheckAndUpdateSeq(env.SenderId, env.Seq); err != nil {
		return
	}
	_ = n.Log.Append(env)
```

Then a `switch` on `env.Type`: unmarshal payload, maybe `Lobby.HandleJoin` / `HandleReady`, then the `OnXxx` callback if non-nil. Unknown or unused types (`LEAVE_TABLE`, `GAME_STATE_SYNC` unless a callback is set) fall through.

`lookupPubKey` checks `n.peers`, then extracts Ed25519 from the Peer ID string and caches it. Extract failure returns `(nil, nil)` so `DecodeEnvelope` skips verify — a hostile `sender_id` that is not a valid Peer ID can fail closed at extract, or skip verify if the API treats nil pub as “no check.” Do not weaken this without a test.

### 14.3 Broadcast helpers

`publish` allocates the next **envelope** seq (`atomic.AddInt64`), builds `NewEnvelope`, signs, publishes on the table topic.

`BroadcastAction` is the one you will watch in the lab:

```410:427:internal/network/node.go
func (n *Node) BroadcastAction(ctx context.Context, handNum int64, a game.Action, actionSeq int64) error {
	playerID := a.PlayerID
	if playerID == "" {
		playerID = n.Host.PeerID
	}
	msg := &PlayerAction{
		TableId:  n.tableID,
		HandNum:  handNum,
		PlayerId: playerID,
		Action:   int32(a.Type),
		Amount:   a.Amount,
		Seq:      actionSeq,
	}
	b, err := proto.Marshal(msg)
	if err != nil {
		return fmt.Errorf("marshal PlayerAction: %w", err)
	}
	return n.publish(ctx, MsgType_PLAYER_ACTION, b)
}
```

Notice `actionSeq` is an argument. `Node` does not own table-wide action order. `p2pGameModel.applyAndBroadcast` does.

`BroadcastHandResult` is called from the TUI when `PhaseSettled`. Recipients print a line. They do not call `distributePots`.

---

## 15. Call graph from `runP2PMode`

Phase 1 previewed this. This chapter traces it for real, stopping before `dealCryptoHand`.

```
runHost / runJoin
  └── applyP2PFlags → runP2PMode(ctx, cfg, noCrypto)
        ├── requireP2PSeats (≥ 3)
        ├── cfg.LoadIdentityKey()
        ├── maybe GenerateSRAKey          ← skip when --no-crypto
        ├── actionSequencer{nextSeq: 1}
        ├── network.NewNode(...)
        ├── wire OnJoinTable / OnPlayerReady / OnPlayerAction / OnShuffleStep / ...
        ├── node.Start()                  ← mDNS, bootstraps, receiveLoop
        ├── print Peer ID + multiaddrs
        ├── BroadcastJoin (retry × 6)
        ├── poll Lobby.Count() ≥ MaxSeats
        ├── BroadcastReady + sleep 2s
        ├── SessionNonce → sharedSeed     ← --no-crypto deck
        ├── players from Lobby.Seats()    ← canonical order
        ├── FaultManager + heartbeats     ← Phase 5; started before shuffle
        ├── either:
        │     --no-crypto: NewMachine(gs, rng from sharedSeed)
        │     default:     KeyringFromLobby → shares → dealCryptoHand  ← Phase 4
        ├── liveMachine / liveGS under machineMu
        └── tea.NewProgram(p2pGameModel)
              ├── Init → StartHand() if noCrypto
              ├── key → applyAndBroadcast → ApplyAction + BroadcastAction
              └── OnPlayerAction → seq.push → ApplyAction → GameStateMsg
```

Five comments at the top of `runP2PMode` are the historical bug list. They are still the map:

```297:321:cmd/poker/main.go
// runP2PMode — the full multiplayer implementation
//
// Design decisions / bug fixes vs the previous broken version:
//
//  1. ALL callbacks are wired BEFORE node.Start() so no messages are silently
//     dropped in the window between Start() launching the receive goroutine
//     and the caller setting the callbacks.
//
//  2. SHARED RNG SEED: every node derives the same int64 seed from
//     Lobby.SessionNonce() — a deterministic concatenation of all peer IDs in
//     join-time order that is identical on every node.  This makes every node
//     shuffle the deck in exactly the same order, so all players see the same
//     hole cards dealt to the correct seats.
//
//  3. ACTION SEQUENCER: GossipSub does not guarantee delivery order.
//     PlayerAction messages carry a Seq field.  The actionSequencer buffers
//     out-of-order messages and releases them in monotonically increasing
//     Seq order so game.Machine.ApplyAction is always called in the right
//     sequence on every node.
//
//  4. MACHINE POINTER INDIRECTION: when a new hand starts, a new
//     game.Machine is created.  The network callback goroutine must see the
//     updated pointer.  We use a *(*game.Machine) with a mutex so the callback
//     always operates on the current hand's machine.
//
//  5. LOBBY MAX SEATS: node.go was hardcoding NewLobby(tableID, 9) regardless
//     of the configured MaxSeats — fixed in node.go.
```

Point 2 describes the **`--no-crypto`** path. The default crypto path does **not** shuffle a local deck from that seed. Phase 1 already warned you the comment is slightly historical. For this chapter, point 2 is the lab.

Point 4: `OnPlayerAction` reads `liveMachine` under `machineMu`. `startNextHand` swaps the pointer. Without the mutex, the receive goroutine would apply seq 7 to a freed or stale machine.

`machineMu` is also why crypto wait loops in Phase 4 **release** the lock: holding it across a two-minute shuffle would deadlock timeout folds. This week, hold it around every `ApplyAction` and nothing else.

---

## 16. The action sequencer

This type lives in `cmd/poker/main.go`, not in `internal/network`. That is deliberate. The network package must not import a policy about Hold’em order.

```715:745:cmd/poker/main.go
type actionSequencer struct {
	mu      sync.Mutex
	nextSeq int64
	pending map[int64]*network.PlayerAction
}

// push adds msg and returns all messages that are now in-order.
func (s *actionSequencer) push(msg *network.PlayerAction) []*network.PlayerAction {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.pending[msg.Seq] = msg
	var out []*network.PlayerAction
	for {
		if m, ok := s.pending[s.nextSeq]; ok {
			out = append(out, m)
			delete(s.pending, s.nextSeq)
			s.nextSeq++
		} else {
			break
		}
	}
	return out
}

// reset clears state between hands.
func (s *actionSequencer) reset() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.nextSeq = 1
	s.pending = make(map[int64]*network.PlayerAction)
}
```

Classic gap buffer. `nextSeq` starts at 1. If seq 3 arrives first, it sits in `pending` until 1 and 2 have been applied. Draining is greedy: applying 2 may immediately release 3.

### 16.1 Local actor vs remote

The acting player does **not** call `push` on their own action. They assign the next seq, increment, apply, then broadcast:

```787:814:cmd/poker/main.go
func (m *p2pGameModel) applyAndBroadcast(a game.Action) {
	m.machineMu.Lock()
	machine := *m.machinePtr
	gs := *m.gsPtr
	if machine == nil {
		m.machineMu.Unlock()
		return
	}

	m.seq.mu.Lock()
	outSeq := m.seq.nextSeq
	m.seq.nextSeq++
	m.seq.mu.Unlock()

	_ = machine.ApplyAction(a)
	m.machineMu.Unlock()

	if err := m.node.BroadcastAction(m.ctx, int64(m.handNum), a, outSeq); err != nil {
		fmt.Printf("[error] broadcast action: %v\n", err)
	}
	// ...
}
```

Remotes only apply through `push`:

```382:403:cmd/poker/main.go
	node.OnPlayerAction = func(msg *network.PlayerAction) {
		machineMu.Lock()
		m := liveMachine
		gs := liveGS
		if m == nil {
			machineMu.Unlock()
			return
		}
		ready := seq.push(msg)
		for _, rm := range ready {
			a := game.Action{
				PlayerID: rm.PlayerId,
				Type:     game.ActionType(rm.Action),
				Amount:   rm.Amount,
			}
			_ = m.ApplyAction(a)
		}
		machineMu.Unlock()
		if prog != nil && len(ready) > 0 {
			prog.Send(tui.GameStateMsg{State: gs})
		}
	}
```

Each replica has its **own** `actionSequencer`. They stay aligned because:

1. Only the current actor assigns a seq (the engine rejects wrong-player actions).
2. That actor applies locally and broadcasts that seq.
3. Everyone else buffers until `nextSeq` is present.
4. Gossip echo of your own action is dropped in `dispatch`, so you do not double-apply.

If you also `push` the local action after incrementing `nextSeq`, you skip or stall. The split (local: increment without pending; remote: push) is load-bearing.

If `BroadcastAction` fails after a local apply, remotes stall on that seq forever. No retransmit in v1. That is an honest gap, not a TODO you should “just fix” by broadcasting `GameState`.

`startNextHand` calls `seq.reset()` so the next hand’s first action is seq 1 again. `HandNum` on the protobuf is informational for humans and for crypto sessions; the sequencer does not key on it.

### 16.2 Locks, again

`game.Machine` is not internally synchronized. The live process treats it as a **single-threaded reducer**. `machineMu` is held around every `ApplyAction` (local, remote, and later timeout fold). The TUI’s `OnAction` runs on the Bubble Tea thread; `OnPlayerAction` runs on the receive goroutine. The mutex is the meeting point.

This is **not** CAP, Raft, or BFT. During a partition, a minority that misses action 7 stalls on the sequencer. There is no leader and no view change. The timeout path (Phase 5) force-folds **silence**; it does not merge two conflicting histories. If you needed adversarial WAN, you would replace **only the log transport** and keep `ApplyAction` unchanged. That is why the engine has no sockets.

---

## 17. Worked example: three terminals fill a lobby

This is the Phase 3 lab, narrated against the code. `--no-crypto` on **every** peer. Same table id. Different listen ports if you are on one machine.

Windows (PowerShell), from the repo root after `go build -o poker.exe ./cmd/poker`:

```powershell
# Terminal 1
.\poker.exe host --seats 3 --name Alice --table friday --no-crypto --listen /ip4/127.0.0.1/tcp/9000

# Terminal 2 — copy Alice’s printed multiaddr into --peer
.\poker.exe join --name Bob --table friday --no-crypto --listen /ip4/127.0.0.1/tcp/9001 --peer /ip4/127.0.0.1/tcp/9000/p2p/12D3KooW...

# Terminal 3
.\poker.exe join --name Carol --table friday --no-crypto --listen /ip4/127.0.0.1/tcp/9002 --peer /ip4/127.0.0.1/tcp/9000/p2p/12D3KooW...
```

On three laptops on one Wi-Fi you can omit `--listen` / `--peer` and let mDNS work; still pass `--table friday` and `--no-crypto` everywhere.

### 17.1 Construction (Alice)

`runHost` sets default listen `9000`, `applyP2PFlags` sets seats 3, name, table, `noCrypto=true`. `runP2PMode` refuses 2 seats, loads `identity.key`, **skips** `GenerateSRAKey`, constructs `actionSequencer` and `Node` with `sraKey == nil`.

Callbacks are set. `OnJoinTable` prints `[lobby] Bob joined (2 / 3 seats)`. `OnPlayerAction` is already the sequencer, even though `liveMachine` is still nil — early actions would no-op, which is correct (no hand yet).

`Start()` prints Peer ID and multiaddrs. Share one address that contains `/p2p/`.

### 17.2 Joins

Each process `BroadcastJoin`s (up to 6 attempts, 400 ms apart). Alice’s lobby gets Alice immediately (local `HandleJoin`). Bob’s `JOIN_TABLE` arrives on Alice via gossip: `dispatch` verifies signature, checks envelope seq, appends gamelog, `Lobby.HandleJoin` with the envelope timestamp, prints the lobby line.

When `Count() == 3`, each process leaves the poll loop, broadcasts `PLAYER_READY`, sleeps 2 s. Canonical `Seats()` is now identical on honest replicas that saw the same three joins: timestamp then Peer ID, **not** “host is seat 0.” Alice might be seat 2. Dealer index starts at 0 in that vector, not “the person who typed host.”

### 17.3 Shared seed and plaintext machine

```577:654:cmd/poker/main.go
	nonce := node.Lobby.SessionNonce()
	sharedSeed := int64(0)
	for i, b := range nonce {
		sharedSeed ^= int64(b) << (uint(i%8) * 8)
	}
	sharedSeed = sharedSeed*6364136223846793005 + 1442695040888963407
	// ...
	if noCrypto {
		fmt.Println("DEBUG  ·  --no-crypto  ·  shared-seed plaintext  ·  all cards visible")
		gs = game.NewGameState(cfg.Game.TableID, handNum, players, dealerIdx,
			cfg.Game.SmallBlind, cfg.Game.BigBlind)
		machine = game.NewMachine(gs, rand.New(rand.NewSource(sharedSeed)))
```

`players` are `game.NewPlayer(s.PlayerID, s.PlayerName, s.BuyIn)` in canonical order — same types as local mode. The RNG seed is a public function of the concatenated nonces (peer id bytes). Every replica Fisher–Yates-shuffles the same deck. All hole cards are filled in memory. The TUI still hides opponents (Phase 2’s defense in depth).

`Init` on `p2pGameModel` calls `StartHand()` only when `noCrypto`. Crypto mode already started the hand inside `dealCryptoHand`. Mixing those would double-post blinds.

Then play. Every fold/call/raise is §18.

---

## 18. Worked example: a fold, hop by hop

Suppose canonical order is Alice, Bob, Carol. Dealer is Alice (index 0). Pre-flop first actor is left of the big blind — Carol if three-handed. Use whoever `CurrentPlayer()` highlights. Say it is Alice’s turn and she presses `f`.

This is [`HOW_IT_WORKS.md`](./HOW_IT_WORKS.md) §15.3 with file names.

1. **UI thread (Alice).** Bubble Tea delivers the key. `tui.Model.submitAction` builds `game.Action{PlayerID: AlicePeerID, Type: Fold}` and calls `OnAction`, which is `applyAndBroadcast`.

2. **Seq + reducer (Alice).** Under `machineMu`, `actionSequencer` hands out `outSeq` (say 2; blinds are not `PLAYER_ACTION`s). `nextSeq` becomes 3. `ApplyAction` sets Alice Folded, appends `GameState.Log`, advances the actor to Bob. Unlock. Alice’s replica is already consistent.

3. **Sign (Alice).** `BroadcastAction` builds `PlayerAction{Seq: 2, ...}`, wraps `Envelope` with a fresh **envelope** seq (a different counter, say Alice’s 14th gossip message this process), signs with Ed25519, length-prefixes.

4. **Publish (Alice).** GossipSub puts the frame on `poker/table/friday`. libp2p sends it over existing Noise/TCP sessions to mesh neighbors (maybe Bob, maybe not Carol yet).

5. **Forward (Bob, maybe).** Bob’s GossipSub may send a copy to Carol. Bob’s Noise session proves “this hop is Bob.” Carol must still verify Alice’s signature on the envelope.

6. **Receive (Carol).** `receiveLoop` → `dispatch`: not from me, envelope seq > last Alice seq, signature ok, `Gamelog.Append`, unmarshal `PLAYER_ACTION`, `OnPlayerAction`.

7. **Sequencer (Carol).** If she already applied seq 1 and this is 2, `push` returns it immediately. `ApplyAction` — same function Alice used. If seq 3 from a confused peer sat in `pending`, applying 2 may drain 3; the engine will reject a wrong-player 3.

8. **UI (Carol).** `notifyCh` or the 250 ms `waitForUpdate` tick pushes `GameStateMsg`. Alice’s panel goes folded. Highlight moves to `CurrentPlayer()`.

Alice also receives her own gossip echo. `dispatch` drops it (`sender_id == me`). If she had *not* applied locally in step 2, she would wait to see her own fold come back — extra latency and a window where her UI still thinks she can act.

Bob does the same as Carol. Three honest replicas, one ordered log, identical pots. Nobody broadcast a `GameState` blob. When the hand settles, each TUI may `BroadcastHandResult`; that is a log line, not a chip movement.

---

## 19. `--no-crypto` as the Phase 3 lab

Default `host` / `join` is mental poker. That is the product. It is the **wrong** lab for learning gossip order, because you will sit on “Shuffling…” for several seconds and then stare at peels.

`--no-crypto` on **every** peer:

- Omits SRA key generation.
- Publishes empty `sra_pub_key_e`.
- Derives `sharedSeed` from `SessionNonce()`.
- `NewMachine(gs, rng)` + `StartHand()` — Phase 2 plaintext path, now with three processes.
- Next hand: `nextSeed := sharedSeed XOR handNum*constant`, `seq.reset()`, rotate dealer.

Rules for the lab:

- All peers must pass `--no-crypto`. One crypto peer and one debug peer → `AllSeatsHavePublicE` fails and `runP2PMode` exits.
- You will see every hole card **in process memory**. The TUI still paints `??` for opponents until showdown chrome. Do not “fix” that hide this week.
- You are proving the **sequencer**, not fairness of the shuffle. A shared seed is a trusted public permutation. Fine for sync testing; disastrous as a product.

When Phase 4’s exit check is true, you will rerun **without** the flag and watch the same `PLAYER_ACTION` path on top of hidden cards.

---

## 20. `HandCoordinator`: the trap

[`coordinator.go`](./internal/network/coordinator.go) looks like “the thing that runs a hand.” It is not the live `host` / `join` path.

```27:58:internal/network/coordinator.go
func (hc *HandCoordinator) RunHand(ctx context.Context, dealerIdx int, sb, bb int64) (*game.GameState, *game.Machine, *pokercrypto.CryptoGame, error) {
	if err := hc.lobby.WaitReady(ctx); err != nil {
		return nil, nil, nil, fmt.Errorf("")
	}
	playerIDs := hc.lobby.CanonicalPlayerOrder()
	nonce := hc.lobby.SessionNonce()
	// ...
	cg, err := pokercrypto.NewCryptoGame(playerIDs, nonce)
	// ...
	if err := cg.RunShuffle(); err != nil {
		return nil, nil, nil, fmt.Errorf("")
	}
	// DealToEngine fills every seat — oracle, all keys on this machine
	m := game.NewMachine(gs, nil)
	if err := m.StartHandCrypto(); err != nil {
		return nil, nil, nil, fmt.Errorf("")
	}
```

`CryptoGame` holds **every** private `d` in one process. That is a test oracle (Phase 4). Live dealing is `CryptoHand` over gossip, one `d` per laptop. If you “simplify” `runP2PMode` by calling `HandCoordinator.RunHand`, you have reintroduced a dealer.

Read this file once so the name does not lure you. Then leave it alone.

---

## 21. Tests in this phase

Run from repo root:

```bash
go test ./internal/network -count=1
```

`-short` skips the libp2p mesh tests (`TestNode_BroadcastAndReceiveAction`, join, three-peer mesh). Run without `-short` at least once on a machine where UDP multicast / localhost TCP is allowed.

You do not need `go test ./...` yet (2048-bit crypto tests are slow and are Phase 4–5).

### 21.1 Codec and proto

| Test | What it locks |
|---|---|
| `TestEncodeDecodeEnvelope_RoundTrip` | Frame → struct, sender/payload/type |
| `WrongKey_Rejected` | Signature actually binds the key |
| `FrameTooShort` | Truncation fails closed |
| `NoVerification` | `pubKeyFn == nil` skips verify (stream path) |
| `TestBigIntWire_*` / `DeckWire` / `ZKProofWire` | Adapters; ZK still verifies after a round trip |
| `TestProto_ShuffleStep/PartialDecrypt/HandResult_RoundTrip` | Payload shapes survive marshal |

### 21.2 Gamelog

| Test | What it locks |
|---|---|
| `AppendAndLen` / `DuplicateRejected` | Set keyed sender:seq |
| `StateRootChanges` / `DifferentLogsProduceDifferentRoots` | Hash includes payload |
| `EquivocationDetected` | Same sender+seq, different payload — injected, not via `Append` |
| `NoEquivocation` | Distinct seqs are fine |
| `ValidateSequences` | Envelope seq 1..n; gap at 5 after 1,2,3 fails |

### 21.3 Lobby

| Test | What it locks |
|---|---|
| `JoinAndReady_ThreePlayers` | `WaitReady` unblocks; state `LobbyReady` |
| `TableFull` / `DuplicateJoin` / `InvalidBuyIn` | Gates |
| `PlayerIDs_InJoinOrder` | Explicit timestamps |
| `SameTimestamp_PeerIDTiebreaker` | Lexicographic Peer ID |
| `StoresSRAPubKeyE` | Defensive copy |
| `PublicExponents_CanonicalOrder` | `e` follows seat order |
| `AllSeatsHavePublicE` | Empty `e` is false |
| `KeyringFromLobby_OK` / `MissingE` | Fails closed; Phase 4 type, Phase 3 invariant |

### 21.4 Replay and node wiring

| Test | What it locks |
|---|---|
| `ReplayProtection_*` | Envelope seq: increase, dup, old, per-peer independence; **gaps allowed** |
| `BroadcastAndReceiveAction` | Two nodes, GossipSub, raise 100 arrives |
| `BroadcastJoin_NilSRAKey_NoPanic` | `--no-crypto` join does not crash; empty `e` |
| `BroadcastJoin_LobbyUpdated` | Remote lobby count 1 |
| `ThreePeerMesh_AllReceive` | A fold published by A is seen by B and C |

There is **no** unit test that `actionSequencer.push` drains 3 after 1,2. That type lives in `main.go`. `internal/integration/e2e_test.go` is Phase 5. If you edit the sequencer, write a small test next to it or you will only find divergence in a live three-terminal session.

---

## 22. Interview-shaped recap

Read [`SYSTEMS_DESIGN_INTERVIEW.md`](./SYSTEMS_DESIGN_INTERVIEW.md) **after** the mesh files, not before. The doc is the same architecture in the shape a staff interviewer wants. This chapter already taught the sentences you should say:

> There is no game server. Every peer is a full replica. Public game state is a deterministic state machine fed by a totally ordered action log.

> Gossip gives me dissemination, not order. I restore order with two sequence spaces: envelope seq per sender for replays, action seq table-wide for `ApplyAction`.

> Host is the first listener. After join, equal protocol roles. Seat 0 is canonical join order, not “the person who typed host.”

> Noise authenticates the hop. Gossip is forwarded, so I also sign the payload.

> I did not run PBFT per fold. A double-talker is detected after the fact when two signed envelopes exist. A partition stalls the sequencer.

> `GAME_STATE_SYNC` is defined and unused. I do not broadcast the pot. I broadcast the action.

If they ask why GossipSub instead of a fully connected TCP mesh at N=9: membership changes, heartbeats should not head-of-line-block a 52-card shuffle, libp2p already maintains a mesh. The cost is unordered best-effort delivery, fixed above GossipSub.

Do not claim live ETH, 2-player P2P, or mid-hand reconnect. Do not draw a Raft leader.

---

## 23. Common mistakes

These are the mistakes people make **the week they edit `internal/network` or `actionSequencer`.**

1. **Broadcasting `GameState` every tick.** That makes a server. Broadcast `PLAYER_ACTION`. `GAME_STATE_SYNC` is unused on purpose.

2. **Applying gossip eagerly.** Action 3 before action 2 diverges replicas. Buffer in `actionSequencer`. Envelope seq is not a substitute.

3. **Mixing the two sequence spaces.** Envelope seq: per sender, gaps allowed, replays dropped. `PlayerAction.Seq`: table-wide, gaps wait. Shuffle/peel sequencers are a third space (Phase 4).

4. **Waiting for your own echo.** `dispatch` drops `sender_id == me`. Apply locally first, then publish. Joins and readies do the same for the lobby.

5. **Setting callbacks after `Start()`.** The receive loop will drop joins into a nil handler. Wire first. Buffer early shuffle/peel messages if the session does not exist yet (`runP2PMode` already does).

6. **Hardcoding lobby `maxSeats` to 9.** Three-seat tables never fill. Use the configured value.

7. **Treating `poker host` as the dealer / sequencer / judge.** Same binary, same `dispatch`, same `ApplyAction`. Canonical seat 0 is join time then Peer ID.

8. **Ordering seats by receive time or by “who hosted.”** Use `JoinedAtUnixMs` then `PlayerID`. Pass the envelope timestamp into `HandleJoin`. Know the clock-skew footgun.

9. **Skipping gossip signatures because Noise exists.** Noise proves the last hop. Carol can forward Alice’s fold. Verify Ed25519 on gossip. Skipping verify is for Noise-bound **streams** only.

10. **Putting the sequencer inside `internal/network` or sockets inside `internal/game`.** Bytes vs order vs rules. Mixing them is how a host becomes a server.

11. **Awarding pots from `HAND_RESULT`.** Informational. Settlement already happened locally. A liar cannot move chips on an honest replica.

12. **Calling `HandCoordinator.RunHand` from the live loop.** That is an in-process oracle with every `d`. Live path is `CryptoHand` (Phase 4) or `--no-crypto` shared seed (this chapter).

13. **A mixed `--no-crypto` table.** Empty `e` plus real `e` must error. Do not “helpfully” fall back to plaintext.

14. **Two processes on one laptop, both on port 9000.** Change `--listen`. Joiners on the same machine also need `--peer` if mDNS is flaky (Windows).

15. **Regenerating `messages.pb.go` and expecting `KeyShare`.** The proto documents it; `codec.go` implements it by hand. `protoc` will collide. `LEAVE_TABLE` / `SHUFFLE_COMMIT` / `GAME_STATE_SYNC` being in the enum does not mean they run.

16. **Using `Gamelog` as the action log.** It is evidence. `ApplyAction` reads `game.Action` values from the sequencer. `Append` cannot store two payloads for the same `(sender, envelope seq)`, so live equivocation needs a different path than the unit test’s slice injection.

17. **Holding `machineMu` across a network wait.** Local apply + broadcast should unlock before you care about peels (Phase 4) or timeout votes (Phase 5).

18. **Starting a feature in `internal/crypto` this week.** Phase 3’s exit check is gossip order, not a peel.

---

## 24. Exit check

You can explain, **without notes**:

1. **Why gossip is unordered, and why that matters.** Neighbors forward. Bob can see action 3 before 2. Eager `ApplyAction` diverges pots. `actionSequencer` buffers until `nextSeq`.
2. **Why there are two sequence spaces.** Envelope `seq`: per sender, replay/dup protection, gaps allowed. `PlayerAction.Seq`: table-wide total order of betting mutations.
3. **Why the acting player applies locally first.** GossipSub echoes you. `dispatch` drops self. Apply-then-broadcast keeps the UI honest without a round trip.
4. **Why `HAND_RESULT` cannot move chips on an honest replica.** Same reason as Phase 2: winners are computed from the log and cards. The wire result is a log line.
5. **What `poker host` is.** First listener, mDNS advertiser, lobby creator. Not dealer, not sequencer, not judge.

You have **run** a three-process `--no-crypto` table and `go test ./internal/network -count=1`.

You have **not** yet walked a hole-card peel, a ZK proof, or a timeout-fold. That is Phases 4–5.

When the five bullets are true, open [`PHASE_4.md`](./PHASE_4.md) if it exists; otherwise follow Phase 4 in [`READ_GUIDE.md`](./READ_GUIDE.md), starting at `internal/crypto/params.go`.

---

## 25. Phase 3 glossary

A subset of [`HOW_IT_WORKS.md`](./HOW_IT_WORKS.md) §26, limited to words this chapter used.

| Term | Meaning in this project |
|---|---|
| **Peer** | A process that listens, dials, publishes, and subscribes. Every seat |
| **Bootstrap peer** | The first listener (`poker host`). Not a game server |
| **Peer ID** | libp2p id derived from Ed25519 public key. `Player.ID` |
| **Multiaddr** | Self-describing address: `/ip4/…/tcp/…/p2p/<PeerID>` |
| **Peerstore** | Local map peer id → addresses |
| **Noise** | Hop encryption + authentication on a TCP session |
| **mDNS** | LAN multicast discovery; tag `p2p-poker-v1` |
| **GossipSub** | Overlay pub/sub. Unordered, best-effort, forwarded |
| **Topic** | `poker/table/<id>` vs `poker/heartbeat/<id>` |
| **Envelope `seq`** | Per-sender replay watermark |
| **`PlayerAction.Seq`** | Table-wide `ApplyAction` order |
| **Apply-local-first** | Mutate, then publish; drop self-echo |
| **Canonical seat order** | Join timestamp, then Peer ID string |
| **Ready barrier** | `PLAYER_READY` until `Count == maxSeats` and all ready |
| **Session nonce** | Concatenated join nonces in seat order; binds seed / session id |
| **Gamelog** | Append-only signed envelopes. Evidence, not consensus |
| **State root** | SHA-256 over the gamelog; intended chain fingerprint |
| **Stream `/poker/1.0.0`** | Unicast frames; Noise-bound; peels + Shamir shares |
| **Replica** | Every peer running the same machine on the same ordered inputs |
| **`--no-crypto`** | Debug shared-seed plaintext. All peers or none |

---

## Companion reading (this phase only)

| File | Why |
|---|---|
| [`HOW_IT_WORKS.md`](./HOW_IT_WORKS.md) §§4–6 | Networking from zero; client–server vs mesh; what “decentralized” means here |
| [`HOW_IT_WORKS.md`](./HOW_IT_WORKS.md) §§11–15 | Identity, discovery, gossip vs streams, lobby, replicated log |
| [`SYSTEM_DESIGN_OVERVIEW.md`](./SYSTEM_DESIGN_OVERVIEW.md) §§1–2 | Compact mesh diagram; how a table forms |
| [`internal/network/messages.proto`](./internal/network/messages.proto) | Wire vocabulary. Dual counters. Skip `messages.pb.go` |
| [`internal/network/codec.go`](./internal/network/codec.go) | Signatures over gossip; framing |
| [`internal/network/host.go`](./internal/network/host.go) | Peer ID, listen, Noise, TCP, peerstore |
| [`internal/network/discovery.go`](./internal/network/discovery.go) | LAN mDNS; what does *not* exist |
| [`internal/network/gossip.go`](./internal/network/gossip.go) | Two topics; envelope-seq watermark |
| [`internal/network/protocol.go`](./internal/network/protocol.go) | Direct streams |
| [`internal/network/lobby.go`](./internal/network/lobby.go) | Seats, timestamps, ready, nonce, `KeyringFromLobby` |
| [`internal/network/gamelog.go`](./internal/network/gamelog.go) | Paper trail |
| [`internal/network/node.go`](./internal/network/node.go) | Composition root; wire callbacks before `Start()` |
| [`internal/network/coordinator.go`](./internal/network/coordinator.go) | Oracle helper — not live `CryptoHand` |
| [`internal/network/network_test.go`](./internal/network/network_test.go) | Join / ready / sign / mesh cases |
| [`cmd/poker/main.go`](./cmd/poker/main.go) `runP2PMode`, `actionSequencer`, `applyAndBroadcast` | Where unordered gossip becomes `ApplyAction` |
| [`SYSTEMS_DESIGN_INTERVIEW.md`](./SYSTEMS_DESIGN_INTERVIEW.md) | Same architecture, out loud |

Next: Phase 4 — mental poker. The log you just learned still carries `PLAYER_ACTION`. Cards stop being a shared RNG and become a joint ciphertext the engine consumes as inputs.
