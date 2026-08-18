# Heartbeat topic receive loop

Parent: [`PHASE_5.md`](../PHASE_5.md) honest gap 1 / pitfall 15. Liveness policy is already [`plans/phase-6-liveness.md`](./phase-6-liveness.md) (votes, fold, Shamir). This plan is **only** the last mile: read the GossipSub topic we already publish to, without letting heartbeat envelope seqs eat table messages.

**Do not** add reconnect, NAK/retransmit, a third topic, proto changes, or ETH slash. After this lands and tests pass, stop.

---

## Why this exists

Heartbeats are **sent**. The monitor is **ticking**. The intended pipe is **not read**.

| Fact | Where |
|---|---|
| Two topics exist | `GossipManager`: `poker/table/<id>` and `poker/heartbeat/<id>` |
| Beats are published on the heartbeat topic | `Node.BroadcastHeartbeat` → `PublishHeartbeat` |
| Sender ticker is running | `runP2PMode`: `HeartbeatSender.Run` |
| Callback records last-seen | `OnHeartbeat` → `fm.RecordHeartbeat` |
| Timeout ticker is running | `go fm.Run(ctx)` → `HeartbeatMonitor.CheckTimeouts` |
| Receive loop reads **only** the table topic | `Node.receiveLoop` → `NewTableMessage` |
| Heartbeat subscription has **no caller** | `GossipManager.NewHeartbeatMessage` |
| `dispatch` already has `MsgType_HEARTBEAT` | Would fire only if a beat arrived on the **table** topic |

`LastSeen` is stamped at `RegisterPeer` (lobby fill) and then ages. Live peers keep shouting on a topic nobody drains. After `heartbeat_timeout` (~15s) the monitor can treat a healthy table as dead and start timeout votes. Isolation of shuffle bytes from liveness was the *reason* for two topics; without a second loop, isolation is a publish-only fiction.

The naive fix (`go` a second loop into the same `dispatch`) is how you drop a shuffle step. See [The footgun](#the-footgun).

---

## Goal / done when

1. `Node.Start` launches a heartbeat receive loop next to `receiveLoop`. It calls `NewHeartbeatMessage` and feeds verified `HEARTBEAT` envelopes to `OnHeartbeat`.
2. Table-topic replay protection and heartbeat-topic replay protection are **separate watermarks**. Accepting heartbeat envelope seq 11 must not cause table envelope seq 10 to be dropped as a replay.
3. Heartbeat envelopes are **not** appended to `Gamelog`. `StateRoot` stays a hash of the hand trail, not of who missed a 5s ping.
4. Duplicate / old heartbeat envelope seqs on the heartbeat topic are still rejected (replay of a captured beat must not keep a dead player Alive forever).
5. `OnHeartbeat` in `cmd/poker` is unchanged. No proto change. No new message type.
6. `go test ./internal/network` (and `go test ./...`) stays green. New tests below pass. `-short` still skips libp2p integration.

You should be able to explain: why two loops, why two watermarks, why heartbeats stay out of `Gamelog`.

---

## The footgun

Every outbound envelope, including heartbeats, shares one `Node.nextSeq()`. `dispatch` then:

```go
if err := n.Gossip.CheckAndUpdateSeq(env.SenderId, env.Seq); err != nil {
    return
}
```

`CheckAndUpdateSeq` is “this sender’s seq must be **greater than** the last one I accepted” (`seqNums` map). Gaps are allowed. Going backwards is not.

Alice publishes, one counter:

```
table    SHUFFLE_STEP     envelope seq 10
heartbeat HEARTBEAT       envelope seq 11
table    PARTIAL_DECRYPT  envelope seq 12
```

One receive loop on the table topic never sees 11, so last=10 then last=12. Fine.

Two loops racing on **one** map:

1. Heartbeat loop accepts seq 11 → `seqNums[Alice]=11`
2. Table loop sees seq 10 → `10 <= 11` → **dropped**

That is PHASE_5 pitfall 15. The receive loop is the easy line. The watermark split is the actual design.

`RecordHeartbeat` stamps **local `time.Now()`**, not the envelope timestamp. An attacker who can replay Alice’s last signed beat would refresh `LastSeen` forever if we skip seq checks. Keep a heartbeat watermark. Just do not share it with the table.

---

## Design

### Second loop

Mirror `receiveLoop`. Same `ctx`, same cancel on `Start`’s context.

```go
func (n *Node) Start(ctx context.Context) error {
    // ... existing handler, mDNS, bootstraps ...
    go n.receiveLoop(ctx)
    go n.heartbeatReceiveLoop(ctx)
    go n.equivocationScanLoop(ctx)
    return nil
}

func (n *Node) heartbeatReceiveLoop(ctx context.Context) {
    for {
        data, _, err := n.Gossip.NewHeartbeatMessage(ctx)
        if err != nil {
            if ctx.Err() != nil {
                return
            }
            continue
        }
        n.dispatchHeartbeat(data)
    }
}
```

Keep `receiveLoop` calling table `dispatch` (or rename today’s `dispatch` to `dispatchTable` and share a private decode helper). Do not funnel both topics through one function that still calls `CheckAndUpdateSeq`.

Shared work (both paths):

- `DecodeEnvelope` + pubkey lookup
- ignore self-echo (`env.SenderId == n.Host.PeerID`)
- verify type is what that topic expects (heartbeat loop: drop non-`HEARTBEAT`; table loop: drop `HEARTBEAT` if it ever appears)

Heartbeat path then: heartbeat watermark → `OnHeartbeat(msg, env.SenderId)`. No `Log.Append`.

Table path then: table watermark → `Log.Append` → existing `switch`.

### Two watermarks

On `GossipManager`:

```go
seqNums   map[string]int64 // table topic (keep CheckAndUpdateSeq)
hbSeqNums map[string]int64 // heartbeat topic
```

Init `hbSeqNums` in `NewGossipManager` (and in tests that construct `GossipManager` by hand).

Add `CheckAndUpdateHeartbeatSeq` with the same `seq <= last` rule on `hbSeqNums`. Do **not** change the semantics of `CheckAndUpdateSeq`. Existing `TestReplayProtection_*` stay valid.

Envelope seq remains one global `nextSeq()` per sender. That is fine: uniqueness for “this exact envelope” still holds. The two maps answer “have I already accepted this seq **on this topic**?” Table will see 10 then 12 (gap 11). Heartbeat will see 11 then 14 (gaps are table traffic). Both are allowed.

### Gamelog

Today `dispatch` appends **before** the type switch, so a beat that reached `dispatch` would become an evidence row. Heartbeats are not hand evidence. Missed beats would make `StateRoot()` diverge across honest replicas.

Skip `Log.Append` on the heartbeat path. Equivocation scanning stays about shuffle / peels / actions.

Do not start appending beats “for completeness.” If you want a liveness audit trail later, that is a different object.

### `cmd/poker`

No change. `OnHeartbeat` is already `fm.RecordHeartbeat(sender)`. `HeartbeatSender` already calls `BroadcastHeartbeat`. Wiring the receive path makes that callback fire from the wire.

---

## Scope

### Touch

| File | Change |
|---|---|
| `internal/network/gossip.go` | `hbSeqNums`; init in `NewGossipManager`; `CheckAndUpdateHeartbeatSeq` |
| `internal/network/node.go` | `heartbeatReceiveLoop`; start it in `Start`; split dispatch so heartbeats use the heartbeat watermark and skip `Gamelog` |
| `internal/network/network_test.go` | Watermark isolation + heartbeat receive (see tests) |

### Do not touch

- `internal/fault/*` (`HeartbeatMonitor`, `HeartbeatSender`, `FaultManager.Run`)
- `cmd/poker/main.go` callbacks / sender ticker
- `messages.proto` / `messages.pb.go`
- `BroadcastHeartbeat` topic choice (`PublishHeartbeat` stays)
- `internal/crypto/*`, `internal/game/*`, `internal/tui/*`
- Timeout votes, `forceFold`, Shamir, `StreamPool`
- GossipSub constructor options / `HistoryLength`
- `Node.SetHandNum` / per-hand `Gamelog` replacement (separate gap)

---

## Implementation notes

### Drop wrong types per topic

A beat published only on the heartbeat topic should never hit `receiveLoop`. Still, if it did, today’s table `dispatch` would `CheckAndUpdateSeq` it and pollute the table watermark **and** `Gamelog`. After the split, table dispatch should ignore `MsgType_HEARTBEAT` (no watermark update, no append). Heartbeat dispatch should ignore everything except `HEARTBEAT`.

### Self-echo

GossipSub echoes the publisher. Existing `dispatch` already returns before seq check when `SenderId == PeerID`. Keep that on both paths so your own beats do not need to be “accepted” as if they were remote.

### Signature

Same `DecodeEnvelope` as the table path. Heartbeats are forwarded; Noise on the last hop is not authorship.

### Empty `OnHeartbeat`

Same rule as other callbacks: if nil, drop after verify. `Start` still requires callbacks to be set first. Live path already sets this before `Start`.

### Close

`GossipManager.Close` already cancels `heartbeatSub`. The new loop should exit on `ctx.Err()` the same way `receiveLoop` does. No extra close API.

---

## Tests

Add next to the existing replay-protection and libp2p node tests in `internal/network/network_test.go`.

### Unit — watermark isolation

Construct a `GossipManager` with both maps (no libp2p).

1. `CheckAndUpdateHeartbeatSeq("alice", 11)` succeeds.
2. `CheckAndUpdateSeq("alice", 10)` **succeeds** (would fail today on a shared map).
3. `CheckAndUpdateSeq("alice", 12)` succeeds.
4. `CheckAndUpdateHeartbeatSeq("alice", 11)` fails (duplicate beat).
5. `CheckAndUpdateHeartbeatSeq("alice", 5)` fails (old beat).
6. Existing `TestReplayProtection_*` still pass unchanged against `CheckAndUpdateSeq`.

### Integration — beat is received (`testing.Short` skip)

Mirror `TestNode_BroadcastAndReceiveAction`: two `makeTestNode`s, `connectNodes`, set `OnHeartbeat` on B, `BroadcastHeartbeat` from A, wait for the callback with sender = A’s Peer ID. Timeout ~10s like the action test.

Optional extra: after a successful beat, B’s `Log.Len()` is unchanged (or `EntriesBySender` has no `HEARTBEAT`). That locks the Gamelog skip.

Do **not** require a 15s timeout-fold e2e here. That is phase-6 policy and already has fault-package tests. This plan proves the pipe.

---

## Pitfalls

1. **One `CheckAndUpdateSeq` for both loops.** Drops shuffle / peels / actions. Two maps, or you have not done the work.
2. **Giving heartbeats their own `nextSeq` while keeping one watermark.** Then table seq 1 and heartbeat seq 1 collide in the same map. Leave `nextSeq()` global; split the **receive** watermarks.
3. **Appending beats to `Gamelog`.** Honest `StateRoot` divergence.
4. **Skipping heartbeat seq checks entirely.** Replay of the last signed beat keeps a disconnected player Alive.
5. **Publishing heartbeats on the table topic “so the existing loop sees them.”** That undoes topic isolation (a large shuffle should not look like death, and a beat should not sit behind a 52-ciphertext frame). Read the heartbeat topic.
6. **Starting the heartbeat loop without setting `OnHeartbeat` first.** Same silent-drop as other callbacks. Live path is already ordered; tests must set the callback before `Start`.
7. **Using payload `Heartbeat.Seq` as the only watermark and ignoring envelope seq.** Harmless as a *second* check; not a substitute for signed-envelope replay protection unless you also unique-index it. Prefer envelope seq on `hbSeqNums` (matches table).
8. **Touching `cmd/poker` or `internal/fault` “to be safe.”** If `OnHeartbeat` does not fire after the loop exists, the bug is still in `network`, not in the monitor.

---

## Out of scope (do not sneak in)

- Wiring `Node.SetHandNum` so `Gamelog` is actually per-hand
- `ExpireStaleVotes` on `FaultManager.Run`
- `OnEquivocation` live callback
- Heartbeat receive as a health protocol / Raft
- Application-level retransmit of lost `PLAYER_ACTION`

---

## Docs after the code (optional, same PR or follow-up)

Once tests pass, the “honest gap” sentences are stale:

- `PHASE_5.md` §7 and gap 1 / pitfall 15
- `THE_STORY.md` §10.4, §23, second-pass heartbeat paragraph

Do not rewrite those until the loop exists. The plan is the code; the story is the footnote.
