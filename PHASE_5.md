# Phase 5 — Liveness, settlement, tests, honest gaps

This is the fifth onboarding chapter. After it you should be able to **say which faults the table survives (post-shuffle silence) and which it does not (mid-shuffle drop, lost gossip, mid-hand reconnect), walk a timeout-fold plus Shamir peel hop by hop, and say the Solidity contract is real while the Go Ethereum client is not wired.**

The reading list this chapter expands is in [`READ_GUIDE.md`](./READ_GUIDE.md). The teaching narrative it sits next to is [`HOW_IT_WORKS.md`](./HOW_IT_WORKS.md) §§17–19, 22, and 24. Phase 4 taught you how cards appear without a dealer. This chapter is what happens when a replica goes silent *after* those cards are locked, plus the money layer that was designed and then left off the hot path.

**You are here to learn:** after the shuffle, a silent peer can be timeout-folded and their private exponent `d` reconstructed from Shamir shares so remaining peels finish. Mid-shuffle disconnect aborts. Slash records are evidence, not a live ETH burn. Integration tests and the original issues list tell you what is real versus designed.

**Do this with your hands before you finish the chapter:**

```bash
go test ./internal/fault ./internal/network ./internal/chain ./internal/integration -count=1
```

Optionally, if you have Node 18+, `cd contracts && npx hardhat test`. Optionally, three-player crypto table: kill one peer *after* `Shuffling…` finishes and watch the other two timeout-fold and keep peeling. Mid-shuffle kill is the other lab: the hand should error, not recover.

**Do not treat as current status:** [`ISSUES_AND_RECOMMENDATIONS.md`](./ISSUES_AND_RECOMMENDATIONS.md) issues 1–4 and 6, or [`CRYPTO_DEAL_PLAN.md`](./CRYPTO_DEAL_PLAN.md) as a “what `poker host` runs” document. Those were written before live SRA dealing and timeout recovery were wired. Prefer [`README.md`](./README.md) and this chapter for “what runs today.”

**Architectural rule to keep in your head the whole time:** `internal/game` never imports `internal/network`. `internal/crypto` never imports `internal/fault`. Reconstructed keys go on `CryptoHand.gone`, **not** on the Keyring. Mixing those layers is how “survivors accidentally become a dealer who holds every `d`” happens.

---

## Table of contents

1. [How to use this chapter](#1-how-to-use-this-chapter)
2. [The one idea: folding is not enough](#2-the-one-idea-folding-is-not-enough)
3. [What the table survives, and what it does not](#3-what-the-table-survives-and-what-it-does-not)
4. [Four objects you must never confuse](#4-four-objects-you-must-never-confuse)
5. [Package map](#5-package-map)
6. [Wire vocabulary for this phase](#6-wire-vocabulary-for-this-phase)
7. [Heartbeats: last-seen, not a health protocol](#7-heartbeats-last-seen-not-a-health-protocol)
8. [Timeout votes: 2/3 of the others](#8-timeout-votes-23-of-the-others)
9. [Shamir shares of `d`](#9-shamir-shares-of-d)
10. [Designated survivor and `PeelOnBehalf`](#10-designated-survivor-and-peelonbehalf)
11. [Equivocation and slash records](#11-equivocation-and-slash-records)
12. [`FaultManager`: the composition](#12-faultmanager-the-composition)
13. [Network glue: `liveness.go` and the adaptor](#13-network-glue-livenessgo-and-the-adaptor)
14. [Call graph from `runP2PMode`](#14-call-graph-from-runp2pmode)
15. [Worked example: Dave unplugs after the flop](#15-worked-example-dave-unplugs-after-the-flop)
16. [Worked example: Carol unplugs during shuffle](#16-worked-example-carol-unplugs-during-shuffle)
17. [On-chain escrow (designed, not live)](#17-on-chain-escrow-designed-not-live)
18. [The Go chain client is a stub](#18-the-go-chain-client-is-a-stub)
19. [Tests in this phase](#19-tests-in-this-phase)
20. [Historical docs (read last)](#20-historical-docs-read-last)
21. [Honest gaps that remain](#21-honest-gaps-that-remain)
22. [Common mistakes](#22-common-mistakes)
23. [Exit check](#23-exit-check)
24. [Phase 5 glossary](#24-phase-5-glossary)

---

## 1. How to use this chapter

Read top to bottom once. When a code excerpt appears, open that file in the editor and match the excerpt to the live source. Line numbers here were accurate when this chapter was written; if they drift, trust the file.

This chapter is **not** a rewrite of [`HOW_IT_WORKS.md`](./HOW_IT_WORKS.md) §17 or §22. Those sections are the short policy and the cheat/disconnect variants. This file is the types, the call graph, two worked hops, the escrow split, and the mistakes people make the week they first edit `manager.go` or `forceFold`.

Suggested time: half a day after Phase 4, including `go test` of the packages above. Stop when the [exit check](#23-exit-check) is true.

File order matches the read guide. Do not skip to `PokerEscrow.sol` before timeout-fold makes sense — money is a *judge of evidence*, not the liveness mechanism. Do not skip to `ISSUES_AND_RECOMMENDATIONS.md` before you can check each row against the code; several “must fix” items are already done.

Two splits keep coming back. Learn them now:

| Split | In-process (library / tests) | Live replica (gossip / streams) |
|---|---|---|
| Silence → fold | `TimeoutManager` + `ApplyTimeoutFold` | `forceFold` + `BroadcastAction` |
| Recover `d` | `KeyShareStore.ReconstructSRAKey` | unicast distribute, gossip contribute, `MarkGone` |
| Peel for the missing seat | `DealSession.PeelOnBehalf` | designated survivor in `TryDelegatedPeels` |
| Settlement | `BuildOutcome` / Hardhat tests | **not called** from `host` / `join` |

If you grep `EscrowManager.SubmitOutcome` and think that is how a winner is paid, you are looking at a helper that `main` never reaches.

---

## 2. The one idea: folding is not enough

A normal poker server handles a disconnect by folding that seat. The dealer still holds the deck. The flop can still come out.

This table has **no dealer**. After the joint shuffle, every remaining ciphertext still has every seated player’s encryption layer on it — including the player who just closed the laptop. Folding them in the Hold’em machine stops them from acting. It does **not** remove their `e` from the flop. Two remaining players cannot peel the turn without that missing `d`.

So post-shuffle liveness is one product with two halves:

1. **Betting liveness.** A 2/3 timeout vote produces a `PLAYER_ACTION` fold that every replica applies through the same sequencer as a real fold. A local-only fold desyncs stacks.
2. **Crypto liveness.** Survivors reconstruct the missing `d` from Shamir shares that were unicasted at table start, then a **designated survivor** peels *as* that player so `DealSession` can advance.

If the timeout happens **during** the shuffle, reconstruction is useless. The secret permutation π lives only in that player’s RAM. `AbortShuffle`. The hand errors. Restart the processes.

If you remember only one sentence from this section: **folding a silent player without reconstructing `d` hangs the next street.**

---

## 3. What the table survives, and what it does not

Keep this table in your head while you read the files. It is the exit check in compact form.

| Event | What honest replicas do | Survives? |
|---|---|---|
| Peer silent **after** shuffle (holes done, or mid-betting, or before a street peel) | Timeout vote → broadcast fold → gossip shares → reconstruct `d` → `MarkGone` → designated `PeelOnBehalf` | **Yes**, if enough shares arrive |
| Peer silent **during** their shuffle step | `AbortShuffle`; 2-minute wait then error | **No** — permutation is gone |
| Junk `PARTIAL_DECRYPT` (bad ZK) | Peel rejected; slash record in the detector | Hand stalls on honest replicas; no cards leaked |
| Same `(sender, seq)` two payloads | `Gamelog.DetectEquivocation` finds the pair | Detected after the fact, not prevented |
| Mixed `--no-crypto` at one seat | Table exits at keyring build | Intentional hard fail |
| Lost gossip `PLAYER_ACTION` | Sequencer waits forever | **No** retry in v1 |
| Mid-hand reconnect | `GAME_STATE_SYNC` unused | **No** — restart the table |
| NAT / WAN without a reachable `--peer` | No DHT, no relays | **No** — LAN or port-forward |
| Want ETH paid to the winner | Solidity exists; Go RPC is stubbed; `main` never calls it | **Not live** |

`--no-crypto` tables still timeout-fold. They skip share distribution and recovery: the deck was never hidden.

P2P refuses fewer than **3** seats. After one drop you still need Shamir threshold `t ≥ 2` leftover shares. Heads-up is local-vs-bots only.

```1388:1395:cmd/poker/main.go
const minP2PSeats = 3

func requireP2PSeats(n int) error {
	if n < minP2PSeats {
		return fmt.Errorf("runP2PMode: need at least 3 seats for timeout recovery (got %d)", n)
	}
	return nil
}
```

`config.Validate` still allows 2–9 because local mode is heads-up legal. The 3-seat gate is `runP2PMode` only.

---

## 4. Four objects you must never confuse

| Object | Holds another peer’s `d`? | Job |
|---|---|---|
| `Keyring` | **Never.** Public `e` only for others | Encrypt, and peel *yourself* |
| `KeyShareStore` | Shares of `d`, not `d` itself until reconstruct | Distribute / pool / reconstruct |
| `CryptoHand.gone` | **Yes**, after a confirmed timeout | `PeelOnBehalf` for a missing peeler |
| `EscrowManager` | No keys | Would submit payouts; **not on the live path** |

Phase 4’s invariant still holds: no Keyring API returns another peer’s `d`. Timeout recovery does not punch a hole in that API. It stores the reconstructed key on the hand:

```353:372:internal/network/crypto_hand.go
// MarkGone records a reconstructed key for a timed-out peer. The key is never
// stored on the Keyring. Idempotent for the same id.
func (h *CryptoHand) MarkGone(id string, key *pokercrypto.SRAKey) error {
	if h == nil {
		return errors.New("CryptoHand.MarkGone: hand is nil")
	}
	if id == "" {
		return errors.New("CryptoHand.MarkGone: empty player id")
	}
	if key == nil || !key.IsPrivate() {
		return errors.New("CryptoHand.MarkGone: reconstructed key is missing d")
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.gone == nil {
		h.gone = make(map[string]*pokercrypto.SRAKey)
	}
	h.gone[id] = key
	return nil
}
```

If you `kr.Put(missingID, reconstructed)` to “make peels easier,” you have turned every survivor into a house.

---

## 5. Package map

```
internal/fault          policy objects (no GossipSub, no TUI)
  types.go              LogEntry + EquivocationChecker
  heartbeat.go          last-seen + HeartbeatSender
  timeout.go            2/3 votes → OnConfirmed
  shamir.go             share store + SplitAndDistribute
  slash.go              records (equivocation, bad ZK, …)
  manager.go            composes the above; ApplyTimeoutFold
  fault_test.go         unit coverage for each object

internal/network        live wiring
  liveness.go           TableShareHandNum, DesignatedSurvivor, unicast distribute
  fault_adaptor.go      Gamelog → EquivocationChecker
  crypto_hand.go        MarkGone, TryDelegatedPeels, AbortShuffle
  node.go               HEARTBEAT / TIMEOUT_VOTE / KEY_SHARE send + dispatch
  liveness_test.go      fake-net kill-after-holes

cmd/poker/main.go       composition root: fm.Run, forceFold, beginRecovery

contracts/              money (off the hot path)
  PokerEscrow.sol       buy-in, 2/3 settlement, challenge, slash
  test/PokerEscrow.test.js

internal/chain          Go helpers that would call the contract
  client.go             **synthetic** receipts
  escrow.go             BuildOutcome, SignOutcome, BuildDisputeFromSlash

internal/integration    happy path + adversarial in-process tests
```

`internal/fault` is allowed to import `internal/crypto` (Shamir math, `SRAKey`) and `internal/game` (`ApplyTimeoutFold` builds an `Action`). It must not import `internal/network`. The live loop in `main` is the glue.

---

## 6. Wire vocabulary for this phase

You already read `messages.proto` in Phase 3. These four types are the ones this chapter owns.

```14:19:internal/network/messages.proto
    HEARTBEAT = 8;
    TIMEOUT_VOTE = 9;
    HAND_RESULT = 10;
    LEAVE_TABLE = 11;
    EQUIVOCATION_EVIDENCE = 12;
    KEY_SHARE = 13;
```

```97:108:internal/network/messages.proto
message Heartbeat {
    string table_id = 1;
    int64 hand_num = 2;
    int64 seq = 3;
}

message TimeoutVote {
    string table_id = 1;
    int64 hand_num = 2;
    string voting_player_id = 3;
    string timeout_player_id = 4;
}
```

```130:136:internal/network/messages.proto
message KeyShare {
    string table_id = 1;
    int64  hand_num = 2;
    string owner_id = 3;   // whose d this share is of
    int32  index = 4;      // Shamir x (1..n)
    bytes  value = 5;      // big.Int.Bytes() of the share
}
```

Facts that surprise people:

- **`TimeoutVote` has no yes/no bit.** Publishing the message *is* a yes. `HandleTimeoutVote(..., true)`.
- **`KeyShare.owner_id` is whose `d` this is a piece of**, not who is sending it. During distribution, Alice unicasts shares of *Alice’s* `d`. During recovery, Alice gossips the share of *Carol’s* `d` that she was holding.
- **`hand_num` on live `KEY_SHARE` is `TableShareHandNum = 0`.** Keys do not rotate between hands. Re-splitting every hand would desync who holds what. `OnKeyShare` also accepts `hand_num == 1` as a compatibility window.
- **`GAME_STATE_SYNC` (type 7) is unused.** Disconnect is terminal for that player. There is no catch-up.

Two transports, still:

| When | Transport | Why |
|---|---|---|
| Table start, distribute shares | Direct stream `/poker/1.0.0` | Live shares must not sit on the shuffle/peel gossip bus |
| After timeout, contribute shares | Gossip (`KEY_SHARE` on the table topic) | The owner is gone; the table now *needs* the layer |
| Heartbeats (intended) | Gossip topic `poker/heartbeat/<table>` | A large shuffle frame should not look like death |
| Timeout votes, timeout folds | Gossip table topic | Same authenticated log as play |

`SendDirectKeyShare` vs `BroadcastKeyShare`:

```538:558:internal/network/node.go
func (n *Node) BroadcastKeyShare(ctx context.Context, handNum int64, ownerID string, share pokercrypto.ShamirShare) error {
	msg := KeyShareToWire(n.tableID, handNum, ownerID, share)
	b, err := MarshalKeyShare(msg)
	if err != nil {
		return fmt.Errorf("marshal KeyShare: %w", err)
	}
	return n.publish(ctx, MsgType_KEY_SHARE, b)
}

func (n *Node) SendDirectKeyShare(ctx context.Context, toPeerID peer.ID, handNum int64, ownerID string, share pokercrypto.ShamirShare) error {
	msg := KeyShareToWire(n.tableID, handNum, ownerID, share)
	// ...
	return n.streamPool.Send(ctx, toPeerID, frame)
}
```

Dispatch passes `viaGossip`: stream handler calls `OnKeyShare(msg, false)`; table-topic dispatch calls `OnKeyShare(msg, true)`. That boolean is how `main` decides *store* vs *pool for reconstruct*.

---

## 7. Heartbeats: last-seen, not a health protocol

Defaults live in two places that should match: `config.Fault` (`5s` interval, `15s` timeout) and `fault.DefaultHeartbeatInterval` / `DefaultHeartbeatTimeout`.

```13:39:internal/fault/heartbeat.go
const DefaultHeartbeatInterval = 5 * time.Second
const DefaultHeartbeatTimeout = 15 * time.Second

type PeerStatus uint8

const (
	PeerAlive PeerStatus = iota
	PeerSuspect
	PeerTimedOut
	PeerDisconnected
)

type HeartbeatMonitor struct {
	mu sync.RWMutex
	peers map[string]*PeerLiveness
	timeout time.Duration

	OnTimeout func(peerID string)
}
```

`RegisterPeer` stamps `LastSeen = now` as Alive. `RecordHeartbeat` resets to Alive. `CheckTimeouts` walks everyone who is not already `PeerDisconnected`:

- elapsed ≥ timeout → `PeerTimedOut`, and if this is the *transition* into timeout, fire `OnTimeout` once in a goroutine
- elapsed ≥ interval but still under timeout → `PeerSuspect`

```77:104:internal/fault/heartbeat.go
func (hm *HeartbeatMonitor) CheckTimeouts() []string {
	hm.mu.Lock()
	defer hm.mu.Unlock()

	var timedOut []string
	now := time.Now()

	for _, pl := range hm.peers {
		if pl.Status == PeerDisconnected {
			continue
		}
		elapsed := now.Sub(pl.LastSeen)
		if elapsed >= hm.timeout {
			wasAlive := pl.Status != PeerTimedOut
			pl.Status = PeerTimedOut
			pl.MissedBeats = int(elapsed / DefaultHeartbeatInterval)
			timedOut = append(timedOut, pl.PeerID)
			if wasAlive && hm.OnTimeout != nil {
				peerID := pl.PeerID
				go hm.OnTimeout(peerID)
			}
		} else if elapsed >= DefaultHeartbeatInterval {
			pl.Status = PeerSuspect
			pl.MissedBeats++
		}
	}
	return timedOut
}
```

`FaultManager.Run` is just `heartbeat.Run` — a ticker that calls `CheckTimeouts`. Confirmed votes later call `MarkDisconnected` so the monitor stops nagging that id.

`HeartbeatSender` is the other half: every interval, bump a payload seq and call the `send` closure. In `runP2PMode` that closure is `BroadcastHeartbeat` with the **live** `handNum` (`atomic.Int64`), not a closed-over `1`.

A separate GossipSub topic is the *design*: `poker/heartbeat/<table>` vs `poker/table/<table>`. `PublishHeartbeat` writes the heartbeat topic. `receiveLoop` today reads **only** the table topic (`NewTableMessage`). `GossipManager.NewHeartbeatMessage` exists and has **no caller**. `dispatch` still has a `MsgType_HEARTBEAT` case, which would fire if a heartbeat envelope ever arrived on the table topic.

That mismatch is an honest gap, not a doc error. The timeout *policy* (monitor, votes, fold, Shamir) is wired. The intended “shuffle bytes must not look like death” receive path for the heartbeat topic is not started. When you edit liveness, grep `NewHeartbeatMessage` before you assume last-seen is being refreshed from the wire. Silence can still be detected from `LastSeen` aging after `RegisterPeer`; whether *live* peers stay Alive depends on that receive path actually running.

Do not turn heartbeats into Raft. They answer one question: “has this Peer ID published recently enough that we should start a vote?”

---

## 8. Timeout votes: 2/3 of the others

The accused does not vote. `TotalVoters = n - 1`. Threshold is float `2/3` rounded (`int(x + 0.5)`), not a second consensus module.

```9:54:internal/fault/timeout.go
const TimeoutVoteThreshold = 2.0 / 3.0

func (tv *TimeoutVote) AddVote(voterID string, yes bool) VoteStatus {
	if tv.Status != VotePending {
		return tv.Status
	}
	tv.Votes[voterID] = yes

	yesCount := 0
	for _, v := range tv.Votes {
		if v {
			yesCount++
		}
	}

	threshold := int(float64(tv.TotalVoters)*TimeoutVoteThreshold + 0.5)
	if threshold < 1 {
		threshold = 1
	}

	if yesCount >= threshold {
		tv.Status = VoteConfirmed
		tv.ConfirmedAt = time.Now()
	} else if len(tv.Votes) == tv.TotalVoters {
		tv.Status = VoteRejected
	}
	return tv.Status
}
```

Worked numbers:

| n | Eligible (`n-1`) | Threshold in code | Meaning |
|---|---|---|---|
| 3 | 2 | `int(2·2/3 + 0.5) = 1` | **One** other yes confirms. Harsh; accepted for v1 |
| 4 | 3 | `int(2.5) = 2` | 2 of 3 |
| 5 | 4 | `int(3.166) = 3` | 3 of 4 |
| 6 | 5 | `int(3.833) = 3` | 3 of 5 (this is *not* `⌈5·2/3⌉ = 4`; comments in tests sometimes say ceil — trust the `int(x+0.5)` line) |

`StartVote` creates a pending vote, casts the caller’s yes, and if that already meets threshold (heads-up math; live P2P forbids n = 2) fires `OnConfirmed`. `RecordVote` can create the vote if gossip arrives before local `StartVote` — replicas do not share one `TimeoutManager`; each has its own, and 2/3 on *this* replica is what fires *this* replica’s `forceFold`.

`ExpireStaleVotes` exists (default 30s). `FaultManager.Run` does **not** call it. Stale votes sit until the process dies. Do not invent a second expiry loop in `slash.go`; if you need it, drive it from `Run`.

On confirm, `RegisterPlayers` wired:

```103:108:internal/fault/manager.go
	fm.timeouts.OnConfirmed = func(targetPeerID string) {
		fm.heartbeat.MarkDisconnected(targetPeerID)
		if fm.OnPlayerFolded != nil {
			fm.OnPlayerFolded(targetPeerID)
		}
	}
```

That callback is `p2pGameModel.forceFold`. Every survivor who recorded 2/3 both folds and starts recovery. Recovery is idempotent (`recovering` map, `MarkGone` overwrite).

A single malicious yes must not fold a four-player table. `TestAdversarial_TimeoutAbuse_SingleVoteNotEnough` is the sentence in test form.

---

## 9. Shamir shares of `d`

**Shamir secret sharing:** split a secret into `n` shares such that any `t` reconstruct it and `t-1` reveal nothing. The math lives in Phase 4’s `commit.go`. This chapter is *when* shares move.

```83:117:internal/crypto/commit.go
func SplitSecret(secret *big.Int, t, n int, p *big.Int) ([]ShamirShare, error) {
	if t < 2 {
		return nil, errors.New("SplitSecret: threshold must be >= 2")
	}
	if n < t {
		return nil, fmt.Errorf("SplitSecret: n=%d must be >= t=%d", n, t)
	}
	// polynomial of degree t-1; a0 = secret; evaluate at x = 1..n
	// ...
	shares[x-1] = ShamirShare{Index: x, Value: y}
	return shares, nil
}
```

`fault.SplitAndDistribute` chooses `t`:

```87:99:internal/fault/shamir.go
func SplitAndDistribute(ownerKey *pokercrypto.SRAKey, numPlayers int) ([]pokercrypto.ShamirShare, int, error) {
	if numPlayers < 2 {
		return nil, 0, fmt.Errorf("")
	}
	threshold := (numPlayers + 1) / 2
	if threshold < 2 {
		threshold = 2
	}
	share, err := pokercrypto.SplitSecret(ownerKey.D, threshold, numPlayers, ownerKey.P)
	// ...
	return share, threshold, nil
}
```

| n | t | After 1 drop, leftover shares of the missing `d` | Reconstruct? |
|---|---|---|---|
| 2 | 2 | 1 | **No** — why live P2P min 3 |
| 3 | 2 | 2 | Yes |
| 4 | 2 | 3 | Yes |
| 5 | 3 | 4 | Yes |

Share `i` (Shamir `x = i+1`) is issued to `SeatOrder()[i]`. The owner keeps their own share locally. When they disconnect, that share is gone; for n ≥ 3 the survivors still meet `t`.

`KeyShareStore` is two maps, not one:

| Map | API | Meaning |
|---|---|---|
| `sharesReceived` | `StoreMyShare` / `ContributeShare` | Shares **this replica was given** at table start (unicast). “I hold a piece of Carol’s `d`.” |
| `sharesHeld` | `AddReconstructionShare` / `Reconstruct` | Pool **after** a timeout. Gossiped contributions plus our own contribution. |

`ReconstructSRAKey` rebuilds `e` as `d⁻¹ mod (p-1)` so the result is a full private `SRAKey`. `TryReconstructKey` is the live wrapper: if `CanReconstruct`, return the key; never put it on the Keyring.

**Collusion caveat at n = 4, t = 2:** two live players who *also* steal extra shares could reconstruct a third `d` without a timeout. Honest software does not gossip live shares. That is a liveness/security trade, not a feature. [`HOW_IT_WORKS.md`](./HOW_IT_WORKS.md) §24 states it; do not “fix” it by raising `t` without re-checking the one-drop leftover-share table.

Distribution is once per table, from `DistributeLocalShares`, after `KeyringFromLobby`, **before** shuffle. Six retries per seat, 400 ms apart, best-effort. A failed unicast means reconstruction of *that owner* may later fail; v1 logs and continues.

```37:50:internal/network/liveness.go
// DistributeLocalShares splits local d and unicasts one share to each other seat.
// The owner keeps their own share locally. Never put shares on the shuffle/peel bus.
// Call once after KeyringFromLobby; do not re-split on later hands.
func DistributeLocalShares(ctx context.Context, node *Node, fm *fault.FaultManager, kr *pokercrypto.Keyring, sraKey *pokercrypto.SRAKey) error {
	// ...
	shares, thresh, err := fault.SplitAndDistribute(sraKey, n)
	fm.SetShamirThreshold(thresh)
```

`startNextHand` must not call this again. Same keys, new `CryptoHand`, new shuffle, session id mixes `handNum`.

---

## 10. Designated survivor and `PeelOnBehalf`

After reconstruct, someone must publish peels whose `PlayerID` is the **missing** seat, using the reconstructed key. If every survivor peeled, the deal sequencer would see duplicates. If the publisher used `LocalID`, `HandlePeel` would reject (“local peel must be produced locally”) or apply the wrong identity.

**Designated survivor** = first id in `SeatOrder()` that is not in `gone`. Every replica computes the same value. Only that replica peels; others `HandlePeel` as usual.

```16:26:internal/network/liveness.go
// DesignatedSurvivor is the first remaining SeatOrder id (not in gone).
func DesignatedSurvivor(seatOrder []string, gone map[string]*pokercrypto.SRAKey) string {
	for _, id := range seatOrder {
		if _, ok := gone[id]; !ok {
			return id
		}
	}
	return ""
}
```

Folded-but-alive still counts as remaining. If Alice folded on the flop but her process is up, and Dave is gone, Alice is still first in seat order → Alice peels as Dave. “Survivor” means process liveness, not chip status.

`DealSession.PeelOnBehalf`:

```338:365:internal/crypto/deal_session.go
// PeelOnBehalf publishes a peel as playerID using key (a reconstructed d).
// PlayerID on the PeelMessage is playerID, not LocalID.
func (s *DealSession) PeelOnBehalf(playerID string, key *SRAKey) (*PeelMessage, error) {
	// ...
	if playerID == s.kr.LocalID() {
		return nil, errors.New("PeelOnBehalf: playerID is local; use the normal local path")
	}
	expected := s.expectedPeelerLocked()
	if expected != playerID {
		return nil, fmt.Errorf("PeelOnBehalf: expected peeler %q, got %q", expected, playerID)
	}
	if key == nil || !key.IsPrivate() {
		return nil, errors.New("PeelOnBehalf: reconstructed key is missing d")
	}
	pd, err := Peel(key, s.current, s.cardIndex, playerID, s.sessionID)
```

ZK is a proof of the reconstructed `d`. Honest replicas verify it the same way they verify a live peel. `CryptoHand.TryDelegatedPeels` loops while `ExpectedPeeler()` is in `gone` and we are designated:

```385:410:internal/network/crypto_hand.go
func (h *CryptoHand) tryDelegatedPeelsLocked() ([]*pokercrypto.PeelMessage, error) {
	if h.deal == nil || len(h.gone) == 0 {
		return nil, nil
	}
	if DesignatedSurvivor(h.kr.SeatOrder(), h.gone) != h.kr.LocalID() {
		return nil, nil
	}
	var produced []*pokercrypto.PeelMessage
	for {
		peeler := h.deal.ExpectedPeeler()
		if peeler == "" {
			break
		}
		key, ok := h.gone[peeler]
		if !ok {
			break
		}
		msg, err := h.deal.PeelOnBehalf(peeler, key)
		// ...
	}
	return produced, nil
}
```

If the missing player was the **recipient** of a hole job, others still publish n−1 peels; nobody `FinishHole`s for them. They will be folded. Empty opponent holes are already legal (`StartHandCrypto`).

If the fold leaves a single player in the hand, `resolveSingleWinner` fires and **no further community peels** are required. Recovery still ran; it may be unused. That is fine.

---

## 11. Equivocation and slash records

**Equivocation:** same sender, same envelope `seq`, two different payloads, both signed. Court-admissible. Not prevented. A modified client can still split the table if replicas apply different actions before the scan fires.

```3:11:internal/fault/types.go
type LogEntry struct {
	SenderID string
	Seq int64
	Payload []byte
	Signature []byte
}

type EquivocationChecker interface {
	DetectEquivocation() (senderID string, a, b *LogEntry)
}
```

`Gamelog.DetectEquivocation` walks the append-only log. Duplicate identical payloads are *not* a violation (gossip echo). Different payloads at the same `(sender, seq)` are.

```84:111:internal/network/gamelog.go
func (gl *Gamelog) DetectEquivocation() (string, *Envelope, *Envelope, error) {
	// ...
	if prev, exists := seen[k]; exists {
		if string(prev.Payload) != string(env.Payload) {
			return env.SenderId, prev, env, nil
		}
	}
	return "", nil, nil, nil
}
```

`GameLogFaultAdaptor` converts those envelopes to `fault.LogEntry` so `SlashDetector` does not import protobuf.

`Node.equivocationScanLoop` ticks every 5s, runs the adaptor, and calls `OnEquivocation` if set. **`runP2PMode` never sets `OnEquivocation`.** Detection can print nothing on the live path. The scanner and the slash types exist; the callback into `FaultManager.CheckEquivocation` is not installed. Treat slash-on-equivocation as designed and unit-tested, not as a live ETH event and not as a guaranteed TUI line.

Slash reasons:

```12:22:internal/fault/slash.go
const (
	SlashEquivocation SlashReason = iota
	SlashBadZKProof
	SlashInvalidAction
	SlashKeyWithholding
)
```

`CheckPartialDecryption` is the one with teeth: it calls `pd.Verify`. A junk peel does not become a card. `CheckInvalidAction` / `CheckKeyWithholding` are *recorders* — they do not themselves reject the action or wait for a peel; the engine and the deal FSM already refused. Without the chain client, all four are logs. With the contract, 20% of the accused buy-in would burn (`SLASH_BURN_BPS = 2000`).

Live `HandlePeel` in `main` logs verify errors from `CryptoHand`; it does not call `fm.CheckZKProof`. The adversarial tests do. If you wire slash into the loop, call the detector on the verify failure you already have — do not re-implement ZK in `slash.go`.

---

## 12. `FaultManager`: the composition

```14:29:internal/fault/manager.go
type FaultManager struct {
	mu            sync.RWMutex
	cfg           FaultConfig
	handNum       int64
	localPeerID   string
	playerIDs     []string
	heartbeat     *HeartbeatMonitor
	timeouts      *TimeoutManager
	keyShares     *KeyShareStore
	slashDetector *SlashDetector

	OnPlayerFolded      func(peerID string)
	OnKeyShareNeeded    func(ownerID string, share pokercrypto.ShamirShare)
	OnSlash             func(record *SlashRecord)
	OnTimeoutVoteNeeded func(targetPeerID string)
}
```

Construction order that actually matters:

1. `NewFaultManager` — monitor exists; `timeouts` is still nil
2. **Set `OnTimeoutVote` / `OnKeyShare` on `Node` before `Start()`** so early messages are not dropped into nil handlers
3. After lobby fill: `RegisterPlayers` — registers *other* ids on the monitor, constructs `TimeoutManager`, sets Shamir `t` if unset
4. Point callbacks at the game model (`OnPlayerFolded = forceFold`, …)
5. `go fm.Run(ctx)` and `HeartbeatSender` **before** shuffle

`RegisterPlayers` must run before votes. `HandleTimeoutVote` errors if `timeouts == nil`.

`ApplyTimeoutFold` is a pure helper: look up the seat, refuse if already folded / sitting out, return `Action{Type: Fold}`. It does **not** call `ApplyAction`. The live path applies and broadcasts itself so the sequencer stays the source of order.

```219:229:internal/fault/manager.go
func ApplyTimeoutFold(gs *game.GameState, peerID string) (game.Action, error) {
	idx := gs.SeatIndex(peerID)
	if idx == -1 {
		return game.Action{}, fmt.Errorf("")
	}
	p := gs.Players[idx]
	if p.Status == game.StatusFolded || p.Status == game.StatusSittingOut {
		return game.Action{}, fmt.Errorf("")
	}
	return game.Action{PlayerID: peerID, Type: game.ActionFold}, nil
}
```

Empty `fmt.Errorf("")` leftovers in this package are historical. Do not “clean them up” in the same PR as a liveness bug unless you are already touching the line; the plans asked implementers not to churn that.

---

## 13. Network glue: `liveness.go` and the adaptor

`liveness.go` is intentionally small: table-level share hand number, designated survivor, distribute helper. No TUI. No `forceFold`. Policy stays in `fault`; orchestration stays in `main`.

`fault_adaptor.go` is the other small file: `Gamelog` → `EquivocationChecker`. `crypto` must not import `network`. `fault` must not import protobuf envelopes. The adaptor is the seam.

`OnKeyShare` in `runP2PMode` is the live seam for shares:

```429:444:cmd/poker/main.go
	node.OnKeyShare = func(msg *network.KeyShare, viaGossip bool) {
		if fm == nil || msg == nil {
			return
		}
		if msg.HandNum != network.TableShareHandNum && msg.HandNum != 1 {
			return
		}
		share := network.KeyShareFromWire(msg)
		if viaGossip {
			fm.AddReconstructionShare(msg.OwnerId, share)
			return
		}
		if msg.OwnerId != node.Host.PeerID {
			fm.StoreKeyShare(msg.OwnerId, share)
		}
	}
```

- Direct stream, `owner ≠ me` → `StoreKeyShare` (I now hold a piece of their `d`)
- Gossip → `AddReconstructionShare` (contribute phase)
- Ignore other `hand_num` values so a confused peer cannot mix a future scheme into this table

`OnKeyShareNeeded` (local contribute) adds *our* held share to the reconstruction pool **and** broadcasts it, so we do not wait to receive our own gossip echo (self-echo is dropped).

---

## 14. Call graph from `runP2PMode`

```
runP2PMode
  ├─ requireP2PSeats (≥ 3)
  ├─ Node callbacks BEFORE Start()
  │    OnTimeoutVote → fm.HandleTimeoutVote(..., true)
  │    OnKeyShare    → store vs reconstruct pool
  │    OnHeartbeat   → placeholder, then RecordHeartbeat
  ├─ Start() → receiveLoop, equivocationScanLoop
  ├─ lobby fill, ready, 2s pause
  ├─ NewFaultManager + RegisterPlayers
  ├─ fm.OnPlayerFolded = forceFold
  ├─ fm.OnTimeoutVoteNeeded → BroadcastTimeoutVote(liveHandNum)
  ├─ fm.OnKeyShareNeeded → AddReconstructionShare + BroadcastKeyShare
  ├─ go fm.Run                    // HeartbeatMonitor ticker
  ├─ go HeartbeatSender           // BroadcastHeartbeat
  ├─ crypto:
  │    KeyringFromLobby
  │    DistributeLocalShares      // unicast, once
  │    dealCryptoHand             // shuffle, holes, StartHandCrypto
  └─ TUI / sequencer / AdvanceCryptoLocked
```

Timeout path:

```
HeartbeatMonitor.OnTimeout
  → OnTimeoutVoteNeeded → gossip TIMEOUT_VOTE
  → TimeoutManager.StartVote(local yes)
       ↘ other replicas: OnTimeoutVote → RecordVote
  → 2/3 → OnConfirmed → forceFold
```

`forceFold` (the function you will break if you hold the wrong mutex):

```963:1024:cmd/poker/main.go
func (m *p2pGameModel) forceFold(peerID string) {
	m.noteGone(peerID)

	// 1. Mid-shuffle: abort. Do not MarkGone.
	if h != nil && !h.ShuffleDone() {
		err := fmt.Errorf("shuffle aborted: peer %s timed out", peerID)
		h.AbortShuffle(err)
		return
	}

	// 2. Machine may still be nil (holes in flight) → recovery only.
	// 3. Else ApplyAction(fold) under machineMu, bump sequencer, unlock,
	//    BroadcastAction, kickCryptoAdvance, beginRecovery.
}
```

`beginRecovery`:

```1077:1140:cmd/poker/main.go
func (m *p2pGameModel) beginRecovery(missingID string) {
	// idempotent: if already recovering, just TryDelegatedPeels again
	m.fm.BroadcastMyShareFor(missingID)
	// poll TryReconstructKey for up to 30s
	h.MarkGone(missingID, key)
	m.sendDelegatedPeels(h)  // TryDelegatedPeels → SendPeel
}
```

**Lock rule, restated from Phase 4 because it is now load-bearing:** `AdvanceCryptoLocked` must **not** hold `machineMu` across `WaitStreet` / `WaitHoles` / `WaitReveal`. `forceFold` needs that mutex to apply the fold. Holding it for two minutes is a deadlock, not “careful synchronization.”

```1159:1187:cmd/poker/main.go
func (m *p2pGameModel) kickCryptoAdvance() {
	go func() {
		m.foldGoneIfCurrent()
		// ...
		err := network.AdvanceCryptoLocked(ctx, h, machine, m.machineMu, func(msgs []*pokercrypto.PeelMessage) error {
			for _, msg := range msgs {
				if err := m.node.SendPeel(m.ctx, msg); err != nil {
					return err
				}
			}
			return nil
		})
```

`fm.Run` starts **before** `dealCryptoHand` so a death during hole peels can still recover. A death during shuffle still aborts — recovery is skipped in `forceFold` when `!ShuffleDone()`.

`--no-crypto`: same votes and `forceFold`; skip `DistributeLocalShares` and `beginRecovery`.

---

## 15. Worked example: Dave unplugs after the flop

Same four people as [`HOW_IT_WORKS.md`](./HOW_IT_WORKS.md) §21. Seat order Alice, Bob, Carol, Dave. Shuffle done. Holes done. Flop is on the board. Dave yanks the cable before the turn peel.

**t ≈ 0.** Dave’s process is gone. His last heartbeat, if any, is already on the wire or not. Alice, Bob, Carol keep publishing.

**t ≈ 15s.** Each survivor’s `HeartbeatMonitor` sees Dave’s `LastSeen` age past `heartbeat_timeout`. `OnTimeout` fires once per replica. Each:

1. `BroadcastTimeoutVote` for Dave (`TimeoutPlayerId = Dave`, `VotingPlayerId = self`)
2. `StartVote` locally (self yes)

**Votes.** n = 4, eligible = 3, need 2 yes. Alice’s vote plus Bob’s vote confirms on every replica that recorded both. Carol’s vote is surplus. `OnConfirmed` → `forceFold("dave")` on each of Alice, Bob, Carol.

**Fold (betting liveness).** Each `forceFold`:

1. `noteGone("dave")`
2. Shuffle is done → do not abort
3. Under `machineMu`: `ApplyAction({Dave, Fold})` if not already folded; bump `nextSeq`
4. Unlock
5. `BroadcastAction` that fold — **this is the sequencer input**. Remote replicas who had not applied yet will apply via `OnPlayerAction`. Applying locally *and* broadcasting is how you avoid “folded on Alice, Dave still to act on Bob”
6. `kickCryptoAdvance()` — if Dave was to act, the machine can move; if the street is waiting, peels can start
7. `go beginRecovery("dave")`

If two replicas both broadcast the fold, the sequencer sees the same player-fold; the second apply should no-op or be ignored as already folded. `already` in `forceFold` exists so a replica that applied once does not keep bumping seq.

**Shares (crypto liveness).** Each survivor `BroadcastMyShareFor("dave")`: look up the unicast share of Dave’s `d` received at table start, add it to the reconstruction pool, gossip `KEY_SHARE` with `owner_id = Dave`. Alice’s own share of *Alice’s* `d` is irrelevant. Dave’s own share of Dave’s `d` is gone with his laptop. t = 2; any two of Alice/Bob/Carol’s shares suffice.

**Reconstruct.** Within 30s, `TryReconstructKey("dave")` returns a private `SRAKey`. `MarkGone("dave", key)` on this replica’s `CryptoHand`. Keyring still has `Public("dave")` with `D == nil`.

**Designated survivor.** `SeatOrder = [Alice, Bob, Carol, Dave]`, `gone = {Dave}` → Alice. Only Alice’s `TryDelegatedPeels` produces messages. When `DealSession` expects Dave on the turn (and later the river, and any leftover hole peel), Alice `PeelOnBehalf("dave", key)`. Wire `PARTIAL_DECRYPT.player_id = Dave`. Bob and Carol `HandlePeel` as if Dave had spoken, ZK included.

**If Alice had already folded** (status bit) but her process is alive, she is still designated. If Alice’s process is *also* gone, she would be in `gone` too and Bob would be designated — that is a second timeout, same machinery.

**If Dave’s fold leaves only Alice in the hand** (Bob and Carol already folded), `resolveSingleWinner` and no turn peel is required. Reconstruction still happened; unused peels are not a bug.

**Dave cannot reconnect.** There is no `GAME_STATE_SYNC` consumer. Restart all remaining processes for a new session if you want him back.

**The next hand still lists Dave.** `startNextHand` increments `handNum`, rotates the dealer, `ResetForNewHand` on the same `m.players` slice, and builds a new `CryptoHand` from the same Keyring. It does **not** drop `gone` seats. The next shuffle will wait for Dave’s step and then abort (or sit on `WaitShuffle` until the 2-minute timeout). README’s line is literal: *restart the table for the next session.* Timeout recovery finishes **this** hand’s peels. It is not a seating protocol.

Hop table for the recovery itself (order of magnitude, n = 4, two yes votes):

| Hop | Who | What |
|---|---|---|
| 1–3 | Alice, Bob, Carol | `TIMEOUT_VOTE` for Dave (gossip, table topic) |
| 4 | each survivor | local `ApplyAction(fold)` + `PLAYER_ACTION` (gossip) |
| 5–7 | each survivor | gossip `KEY_SHARE` of Dave’s `d` |
| 8 | each survivor | `TryReconstructKey` once t shares sit in `sharesHeld` |
| 9 | Alice (designated) | `PARTIAL_DECRYPT` with `player_id = Dave` for the turn |
| 10 | Bob, Carol | `HandlePeel` as usual |
| 11+ | same | river / leftover hole peels if the machine still needs them |

This is `TestLiveness_FakeNet_3Players_KillAfterHoles_FlopCompletes` with four names instead of three: shuffle + holes on a fake bus, kill replica 2, reconstruct from test shares, `MarkGone`, remaining boards equal.

---

## 16. Worked example: Carol unplugs during shuffle

Seat order Alice → Bob → Carol → Dave. Alice has published `SHUFFLE_STEP`. Bob has published. Carol is permuting 52 ciphertexts and her process dies.

Her permutation π_C exists only in her RAM. Reconstructing `d_C` lets you decrypt *her encryption layer*. It does not tell you the order she would have published. Survivors cannot agree on a deck.

`forceFold` sees `!h.ShuffleDone()`, calls `AbortShuffle`, sends `ErrorMsg` to the TUI, **returns without `beginRecovery`**. `WaitShuffle` unblocks with `shuffle aborted: peer … timed out` (or the 2-minute wait if the vote never confirms). `MarkGone` is not called. There is no delegated shuffle step API — `plans/phase-6-liveness.md` forbade touching `shuffle_session.go` for recovery.

Harsh, documented, correct. Restart all four processes. Do not add “skip Carol and continue with Alice’s deck”; that reintroduces a dealer (whoever’s permutation last landed).

`TestLiveness_ShuffleTimeoutAborts` is the cheap version: `AbortShuffle` → `WaitShuffle` errors → `ShuffleDone() == false`.

---

## 17. On-chain escrow (designed, not live)

Poker at two seconds per action cannot wait for block time. The intended split: **Ethereum is the judge of money and evidence, not the dealer.** The contract never sees a card.

`contracts/PokerEscrow.sol` is real Solidity, 0.8.20, Hardhat-tested. `internal/chain.Client` returns synthetic receipts. `cmd/poker` does not import `internal/chain`. Demo chips are honor-system counters on `Player.Stack`.

```4:18:contracts/PokerEscrow.sol
    uint256 public constant SETTLEMENT_DEADLINE = 1000;
    uint256 public constant CHALLENGE_WINDOW = 50;
    uint256 public constant SIG_THRESHOLD_NUM = 2;
    uint256 public constant SIG_THRESHOLD_DEN = 3;

    uint256 public constant SLASH_BURN_BPS = 2000;

    enum TableState {
        Open,
        Playing,
        Settled,
        Disputed,
        Abandoned
    }
```

Intended life cycle:

1. **Open.** Players `joinTable{value}` with a `peerID` string. Duplicate address rejected. Zero buy-in rejected. Seat map uses `seatOf[addr] = seat+1` so `0` means “not seated.”
2. **Playing.** Last join fills `maxSeats`; `gameStartBlock` is set. Off-chain game runs exactly as Phases 2–4.
3. **Settled.** A seated player `reportOutcome(payoutDeltas, stateRoot, signatures, handNum)`. Deltas must **sum to 0** (chip conservation). `stateRoot` is meant to be a hash of the ordered gamelog. Signatures: at least `⌈n·2/3⌉` (`_requiredSigs`). Payout is `buyIn + delta` per seat.
4. **Disputed.** For 50 blocks after settlement, a seated player `submitDispute` with evidence and an accuser signature. 20% of the accused buy-in is the burn amount; the rest redistributes to non-slashed, non-withdrawn others. State returns to Settled.
5. **Abandoned.** If still Playing after 1000 blocks, anyone `markAbandoned`; seated players `refund` their buy-in.

```99:119:contracts/PokerEscrow.sol
    function reportOutcome(
        int256[] calldata payoutDeltas,
        bytes32 _stateRoot,
        bytes[] calldata signatures,
        uint256 handNum
    ) external inState(TableState.Playing) onlySeated {
        require(payoutDeltas.length == players.length, "PokerEscrow: invalid payout deltas");
        require(_verifyChipConservation(payoutDeltas), "PokerEscrow: invalid chip conservation");
        require(signatures.length >= _requiredSigs(), "PokerEscrow: not enough signatures");

        bytes32 digest = _outcomeDigest(handNum, payoutDeltas, _stateRoot);
        _verifySignatures(digest, signatures);
        // ... pay ...
    }
```

Read `_verifySignatures` with a reviewer’s eyes: it recovers signers, counts unique seated addresses into `validCount`, and **does not `require(validCount >= _requiredSigs())`**. The outer check is `signatures.length`, which is cheaper to satisfy with junk bytes than “2/3 distinct seated keys.” That is a real contract gap. When you wire ETH, close it in Solidity; do not paper over it in Go.

`_executeSlash` computes `burnAmount` and leaves it in the contract (no `address(0)` transfer). Redistribution uses integer division; dust stays in the contract. Fine for a capstone design; say so if an examiner asks “where does the 20% go?”

`receive()` reverts: deposits must go through `joinTable`.

Hardhat: `contracts/hardhat.config.js` (solc 0.8.20, chain id 31337). Tests in `contracts/test/PokerEscrow.test.js` cover deploy, join, conservation, settlement shape, dispute. If a JS revert string disagrees with the Solidity `"PokerEscrow: …"` message, **trust the contract**. `package.json` is Hardhat-only; the Go binary does not use Node.

---

## 18. The Go chain client is a stub

```89:99:internal/chain/client.go
func (c *Client) Deploy(ctx context.Context, tableID string, maxSeats uint8) (Address, *TxReceipt, error) {
	// ...
	var addr Address
	copy(addr[:], []byte(tableID)[:min(20, len(tableID))])
	return addr, &TxReceipt{Status: 1, GasUsed: 800_000, BlockNumber: big.NewInt(1)}, nil
}
```

`JoinTable`, `ReportOutcome`, `SubmitDispute`, `Refund`, `MarkAbandoned` all return `Status: 1` without dialing `RPCURL`. `TableState` always returns Playing. `WaitForSettlement` will spin until `ConfirmTimeout` and then error, because state never becomes Settled.

That is deliberate scaffolding: the **API shape** matches the contract so a future `go-ethereum` ethclient can fill the bodies. `NewClient` still requires a non-empty RPC URL and contract address so you cannot accidentally construct a silent client — and then every method ignores the socket.

`EscrowManager` is the higher helper:

- `HostTable` / `Join` — would deploy + deposit
- `BuildOutcome(gs, handNum, logRoot, playerOrder)` — requires `PhaseSettled`, builds deltas from `gs.Payouts`, SHA-256 of the log if the root is not already 32 bytes
- `SignOutcome` — Ethereum personal-sign of a digest
- `BuildDisputeFromSlash` — maps `fault.SlashReason` to `"equivocation"` / `"bad_zk_proof"` / `"invalid_action"` / `"key_withholding"`

`config.Chain.Enabled` defaults **false**. `Validate` requires RPC URL + contract address only when enabled. Even then, `main` never constructs a `Client`. Enabling the YAML flag does not pay anyone.

`internal/chain/abi/PokerEscrow.go` is generated bindings. Do not study it. Read the `.sol`.

---

## 19. Tests in this phase

Run:

```bash
go test ./internal/fault ./internal/network ./internal/chain ./internal/integration -count=1
```

Read a test when you want a worked example, not before the types make sense.

### `internal/fault/fault_test.go`

| Test | What it proves |
|---|---|
| `TestHeartbeat_TimeoutDetected` | No beats → `PeerTimedOut` |
| `TestHeartbeat_OnTimeoutCallback` | Transition fires once |
| `TestHeartbeat_MarkDisconnected` | Disconnected ids leave `AlivePeers` |
| `TestTimeoutVote_MajorityConfirms` | n = 4, 2 of 3 yes |
| `TestTimeoutVote_InsufficientVotes` | Two yes on n = 6 stay pending |
| `TestTimeoutVote_HeadsUp` | n = 2 confirms on one vote (library; live P2P forbids this) |
| `TestKeyShares_FullRoundTrip` | Split 4 / t = 2; reconstructed `d` decrypts |
| `TestKeyShares_DeduplicateShares` | Same index twice does not count as two |
| `TestSlash_BadZKProof` | Tampered result → `SlashBadZKProof` |
| `TestSlash_ValidProofNotSlashed` | Honest peel is quiet |
| `TestFaultManager_TimeoutVoteFlow` | Votes → `OnPlayerFolded` |
| `TestFaultManager_KeyShareFlow` | Store + pool → `TryReconstructKey` |
| `TestSplitAndDistribute_ThresholdIsHalfN` | `t = max(2, (n+1)/2)` for n = 2..6 |

### `internal/network/liveness_test.go`

| Test | What it proves |
|---|---|
| `TestLiveness_ShareUnicastNotOnShuffleBus` | Shares travel a different channel than shuffle/peel |
| `TestLiveness_ShuffleTimeoutAborts` | `AbortShuffle` fails `WaitShuffle`; shuffle not Done |
| `TestLiveness_FakeNet_3Players_TimeoutFold_NoCryptoStyle` | Two machines apply a fold for the silent seat; neither stuck on that actor |
| `TestLiveness_FakeNet_3Players_KillAfterHoles_FlopCompletes` | **The phase test.** Production-size keys, 3 replicas, kill after holes, reconstruct, `MarkGone`, remaining flops **equal**. Timeout up to 4 minutes |

That last test also asserts reconstructed Carol `d` is not Alice’s Keyring `d`. If you “helpfully” merge keys, it fails.

### `internal/chain/chain_test.go`

Validates the stub’s validation: empty RPC, zero buy-in, chip conservation on `ReportOutcome`, slash-reason mapping, `BuildOutcome` refuses non-settled state. These tests passing does **not** mean a chain was contacted.

### `internal/integration`

The package comment mentions GossipSub. The tests as written are **in-process**: `game.Machine` + `FaultManager` + `CryptoGame` oracle. They do not start libp2p. That is still useful.

| Test | What it proves |
|---|---|
| `TestE2E_HeadsUpHand_ChipConservation` | Local reducer: stacks sum to buy-ins |
| `TestE2E_SidePot_ThreePlayers` | Short stack → side pot, still conserved |
| `TestE2E_100Hands_ChipConservation` | Long soak of the engine |
| `TestE2E_FaultManager_TimeoutVote_FoldsPlayer` | 4-player vote confirms `player-3` |
| `TestE2E_CryptoGame_ShuffleAndDeal` | Oracle path (every `d` on one machine) — not privacy |
| `TestAdversarial_BadZKProof_DetectedAndSlashed` | Tamper is caught |
| `TestAdversarial_TimeoutAbuse_SingleVoteNotEnough` | One yes does not fold n = 4 |
| `TestAdversarial_ForcedFold_ChipsConserved` | Timeout fold does not print chips |
| `TestAdversarial_HonestPlayer_NeverSlashed` | Valid peels stay clean |

`CryptoGame` in e2e is the Phase 4 oracle. Do not cite it as “multi-node mental poker.” Fake-net `CryptoHand` tests in `liveness_test.go` / `crypto_hand_test.go` are the replica-shaped ones.

### Hardhat

Optional. Proves the Solidity state machine, not that `poker join` pays ETH.

---

## 20. Historical docs (read last)

The `plans/` files and the two root markdown notes were written **while** crypto and liveness were being wired. Read them as archaeology after the code.

| File | What it was specifying | What you should take from it now |
|---|---|---|
| [`plans/phase-6-liveness.md`](./plans/phase-6-liveness.md) | Timeout fold + Shamir peels + unlock `machineMu` | Almost the spec this chapter walks. Checkboxes at the end are the review list. Implementation-plan “Phase 6” = onboarding Phase 5 |
| [`CRYPTO_DEAL_PLAN.md`](./CRYPTO_DEAL_PLAN.md) | Index of SRA wiring phases 1–5 | Crypto dealing **is** the default now. Do not quote its “acceptance demo: 2 players” as live policy — P2P min is 3 |
| [`ISSUES_AND_RECOMMENDATIONS.md`](./ISSUES_AND_RECOMMENDATIONS.md) | Gap analysis vs the thesis | Issues **1–4** (plaintext default, unwired shuffle) and **6** (timeout not started) are **done**. Issue **5** (no live ETH) is still true. Issues **8–12** (clocks, not BFT, no reconnect, LAN, one table) remain acceptable limitations |

Those plan numbers are **not** this onboarding series. Onboarding Phase 4 covered `plans/phase-1-keyring.md` … `phase-5-wire-p2p.md`. Onboarding Phase 5 (this file) covers `plans/phase-6-liveness.md` plus `contracts/` plus the honest leftover of the issues list.

If you paste issue 1 into a report today (“shared-seed is the default P2P path”), you will be wrong. Check [`README.md`](./README.md) first.

---

## 21. Honest gaps that remain

Saying these out loud is part of understanding the project. None of them undo the thesis if you state them. The thesis is not “production PokerStars.” It is: **poker as a replicated state machine whose private inputs come from commutative encryption rather than a dealer.**

1. **Heartbeat topic receive loop.** `PublishHeartbeat` writes `poker/heartbeat/<id>`. `receiveLoop` reads the table topic only. `NewHeartbeatMessage` has no caller. Last-seen refresh from the intended topic is not started. Fix is a second loop into `dispatch`, with a hard think about `CheckAndUpdateSeq` sharing one counter across topics.

2. **Slash callbacks not on the live path.** `OnEquivocation` unset. `CheckZKProof` not called from `HandlePeel` in `main`. Records exist; ETH slash does not; even in-process slash may not fire during `host`/`join`.

3. **No live ETH.** Solidity is tested; Go RPC is stubbed; `main` never talks to a node. `Chain.Enabled` does not change that.

4. **Contract signature check is length, not unique seated signers.** `_verifySignatures` never requires `validCount`. Close this before mainnet fantasies.

5. **Mid-shuffle disconnect aborts.** Permutation cannot be recovered. Not a TODO; a theorem of this shuffle.

6. **Disconnect is terminal** for that player. `GAME_STATE_SYNC` unused. No mid-hand catch-up.

7. **Lost gossip stalls.** No application-level NAK/retransmit. TCP reliability is per hop; GossipSub is not TCP for `PLAYER_ACTION`.

8. **Not BFT.** Authenticated total order among honest nodes; equivocation detected after the fact. One modified client can still split replicas if they apply different payloads.

9. **LAN / port-forward.** mDNS or a reachable `--peer`. No DHT, no relays. NAT is UPnP only.

10. **P2P needs 3–9 seats.** Shamir after one drop. Heads-up is local bots.

11. **Seat order uses join timestamps.** Clock skew can theoretically reorder seats.

12. **`t = 2` at `n = 4`.** Two colluding live players plus extra shares is a real caveat. Honest software does not gossip live shares.

13. **`ExpireStaleVotes` unused** in `FaultManager.Run`.

14. **Integration tests are in-process.** They do not prove a three-laptop GossipSub mesh. Fake-net crypto tests are the replica proof; `go test ./internal/network` has some real libp2p cases (Windows mDNS can flake).

15. **SRA is slow and old.** Fine for a LAN capstone. A modern shuffle argument is future work; wiring SRA for real beat an unwired fancier paper.

16. **Gone seats are not removed for the next shuffle.** Recovery unblocks *this* hand. `startNextHand` still deals from the original player list. Restart the processes.

Hang / deadlock diagnosis (from [`plans/phase-6-liveness.md`](./plans/phase-6-liveness.md), still accurate):

| Symptom | Likely cause |
|---|---|
| Timeout never folds | `fm.Run` not started, or `OnTimeoutVote` nil at `Start`, or last-seen never ages (heartbeat receive path — §7) |
| Folded on one replica, other still waiting for their action | `forceFold` did not `BroadcastAction` |
| Folded but flop never comes | Reconstruct failed, or `PeelOnBehalf` never ran, or designated id differs per replica |
| `HandlePeel: local peel must be produced locally` | Delegated message used `LocalID` instead of missing id |
| Deadlock, 100% idle | `machineMu` still held in `WaitStreet` while `forceFold` waits on it |
| Shuffle hangs then error | Expected — mid-shuffle abort |
| `--seats 2` still starts | min check after `NewNode` / only in help text |
| Next hand hangs on shuffle after a timeout | Expected until you restart — gone seat is still in `SeatOrder` |

[`HOW_IT_WORKS.md`](./HOW_IT_WORKS.md) §25 is the same list in teaching form. [`README.md`](./README.md) “Known limitations” is what you quote externally.

---

## 22. Common mistakes

1. **Putting reconstructed `d` on the Keyring.** `MarkGone` exists so `Public(id)` stays `D == nil`. Survivors are not a house.

2. **Local-only timeout fold.** `ApplyAction` without `BroadcastAction` desyncs stacks. The sequencer is the log.

3. **Holding `machineMu` across `WaitStreet`.** `forceFold` deadlocks. `AdvanceCryptoLocked` releases during waits.

4. **Gossiping shares before the owner is voted out.** Live shares are unicast. Gossip is the contribute phase.

5. **Re-splitting Shamir on every hand.** `d` does not rotate. `TableShareHandNum = 0`. `startNextHand` must not call `DistributeLocalShares` again.

6. **Delegated peel with `PlayerID = LocalID`.** `HandlePeel` will fight you. Missing id on the message; reconstructed key in the math.

7. **Assuming every replica is designated.** First remaining `SeatOrder` id only. The others wait for `HandlePeel`.

8. **Trying to recover a mid-shuffle drop.** `AbortShuffle`. No `PeelOnBehalf` of a permutation.

9. **`--seats 2` on P2P.** Reconstruction after one drop is impossible. Local mode is the heads-up lab.

10. **Claiming ETH payouts.** The contract is real; the client is a stub; `main` does not call it. Chips on the TUI are counters.

11. **Quoting `ISSUES_AND_RECOMMENDATIONS.md` issue 1 as current.** Default `host`/`join` is SRA. Check README.

12. **Wiring `HandCoordinator.RunHand` into recovery.** That oracle holds every `d` already. Live path is `CryptoHand` + `gone`.

13. **Treating `TimeoutVote` as a yes/no proto.** Publishing is yes.

14. **Using `MissingRevealIDs` after a timeout fold.** Replica-local. `RemainingShowdownIDs` is table order of Active/All-In (Phase 4; still true when seats are gone).

15. **Starting a heartbeat receive loop that shares `CheckAndUpdateSeq` with the table topic without thinking.** Envelope seq is one per sender across `nextSeq()`. Two loops can drop the lower seq as a “replay.”

16. **Calling `ExpireStaleVotes` from a random TUI tick.** If you need expiry, drive it from `FaultManager.Run` next to `CheckTimeouts`.

17. **Raising Shamir `t` to “be more secure” without the leftover-share table.** At n = 3, t = 3 makes one drop unrecoverable — you have reinvented “folding is not enough” as a hang.

18. **Adding a server in `cmd/poker` to “handle disconnects.”** That fights the whole design. The host is not a game server in Phase 1 and it is not a liveness server now.

---

## 23. Exit check

You can explain, **without notes**:

1. **Which faults the table survives.** Post-shuffle silence: vote, broadcast fold, reconstruct `d`, designated `PeelOnBehalf`. Junk peels are rejected. Mixed `--no-crypto` exits.
2. **Which it does not.** Mid-shuffle drop (permutation gone). Lost gossip with no retransmit. Mid-hand reconnect. WAN without a reachable address. Live ETH.
3. **Why n ≥ 3.** After one drop Shamir still needs `t ≥ 2` leftover shares. At n = 2, t = 2, one share remains.
4. **Why the fold must be broadcast.** Same sequencer as a real fold. Local-only fold desyncs honest replicas.
5. **Why reconstructed keys are not on the Keyring.** `CryptoHand.gone` + `PeelOnBehalf`. The Keyring’s job is still “I only hold my `d`.”
6. **Solidity vs Go client.** `PokerEscrow.sol` is a real contract with Hardhat tests. `internal/chain.Client` returns synthetic receipts. `main` never dials Ethereum.

You have **run** `go test ./internal/fault ./internal/network ./internal/chain ./internal/integration -count=1`. Optionally you have killed one of three crypto peers *after* shuffle and watched recovery, or killed one *during* shuffle and watched abort.

When the six bullets are true, you are done with the five onboarding chapters. Re-read [`HOW_IT_WORKS.md`](./HOW_IT_WORKS.md) §21 once more, then §22; they should now name files you have actually opened. You are ready to change code — without “just adding a server,” without putting `d` on the Keyring, and without claiming the stub client pays ETH.

---

## 24. Phase 5 glossary

A subset of [`HOW_IT_WORKS.md`](./HOW_IT_WORKS.md) §26, limited to words this chapter used.

| Term | Meaning in this project |
|---|---|
| **Heartbeat** | Periodic signed “I am here.” Intended topic `poker/heartbeat/<table>` |
| **Suspect / TimedOut / Disconnected** | Monitor states. Disconnected is after a confirmed vote |
| **Timeout vote** | Yes-by-publication among `n-1` peers; ~2/3 confirms |
| **Force-fold** | Sequenced `PLAYER_ACTION` fold for a silent seat |
| **Shamir sharing** | Split `d` so any `t` shares reconstruct it |
| **Unicast distribute** | Direct `KEY_SHARE` at table start; owner still alive |
| **Gossip contribute** | `KEY_SHARE` after timeout; owner is gone |
| **`TableShareHandNum`** | `0`. Shares are per table, not per hand |
| **`gone` map** | Reconstructed keys for timed-out peers. Not on the Keyring |
| **Designated survivor** | First remaining `SeatOrder` id; peels on behalf of `gone` |
| **`PeelOnBehalf`** | Publish a peel as the missing `PlayerID` using reconstructed `d` |
| **`AbortShuffle`** | Fail `WaitShuffle`. Mid-shuffle recovery is out of scope |
| **Equivocation** | Same `(sender, seq)`, two payloads, both signed |
| **Slash record** | Evidence object. Not a live ETH burn today |
| **Escrow** | On-chain pot of ETH; contract real, Go client stubbed |
| **Chip conservation** | Payout deltas sum to 0 |
| **`stateRoot`** | Hash binding the off-chain log for settlement |
| **Challenge window** | 50 blocks after `reportOutcome` to `submitDispute` |
| **`--no-crypto` liveness** | Timeout fold only; no shares, no `MarkGone` |

---

## Companion reading (this phase only)

| File | Why |
|---|---|
| [`HOW_IT_WORKS.md`](./HOW_IT_WORKS.md) §17 | Heartbeats, votes, Shamir, slash in one sitting |
| [`HOW_IT_WORKS.md`](./HOW_IT_WORKS.md) §19 | Escrow split: judge of money, not dealer |
| [`HOW_IT_WORKS.md`](./HOW_IT_WORKS.md) §§20–22 | Wiring diagram; full hand; then cheat/disconnect variants |
| [`HOW_IT_WORKS.md`](./HOW_IT_WORKS.md) §24–25 | Misconceptions and limitations |
| [`PHASE_4.md`](./PHASE_4.md) §9, §14 | Keyring invariant; `CryptoHand` — recovery plugs in here |
| [`internal/fault/manager.go`](./internal/fault/manager.go) | Composition root of policy |
| [`internal/network/liveness.go`](./internal/network/liveness.go) | Distribute + designated survivor |
| [`cmd/poker/main.go`](./cmd/poker/main.go) `forceFold`, `beginRecovery` | Live orchestration |
| [`contracts/PokerEscrow.sol`](./contracts/PokerEscrow.sol) | Settlement design |
| [`internal/chain/client.go`](./internal/chain/client.go) | Stub — do not claim RPC |
| [`plans/phase-6-liveness.md`](./plans/phase-6-liveness.md) | Historical spec, after the code |
| [`README.md`](./README.md) Known limitations | What you quote externally |

You have finished the five onboarding chapters. The architectural rule has not changed: networking produces authenticated, ordered inputs; the engine reduces them; crypto supplies private card inputs; fault recovery reconstructs a missing layer without turning survivors into a dealer; money, if it ever lands, stays off the hot path.
