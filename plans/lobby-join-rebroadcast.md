# Lobby join rebroadcast (shortcut)

Fixes late joiners missing `JOIN_TABLE` because GossipSub is not a log. This is the **shortcut**, not the proper lobby (signed join catch-up + roster-hash ready + clockless seat order). Do not implement those here.

**Do not** bump GossipSub `HistoryLength`. **Do not** add `GAME_STATE_SYNC`. **Do not** change `Seats()` sort keys.

---

## Why this exists

Each node publishes `JOIN_TABLE` once (six 400 ms retries only if `Publish` errors). GossipSub’s default message cache is ~5 seconds. A peer who subscribes later never receives the original envelope. Their lobby never contains the early sitters, so `Count() >= maxSeats` never fires on that laptop.

The demo assumption was “everyone starts the binary in the same few seconds.” We want people to be able to show up ~1 minute apart while the lobby is still open.

---

## Will a 1-minute window actually work?

**On a LAN, if the late process can form a mesh, yes — and not only for 1 minute.** The implementation must rebroadcast **for the whole lobby wait**, not for 60 seconds and then stop. Stopping at 60 s recreates the bug at t=61 s. A 1-minute *goal* is “Carol can be a minute late.” The mechanism is “keep shouting join until the table is full.” That also covers 2 minutes, 5 minutes, etc., until someone Ctrl-C’s or the lobby fills.

What must be true for Carol at t=60 s:

1. She used the same `--table` id (topic `poker/table/<id>`).
2. She found at least one already-seated peer (mDNS on the LAN, or `--peer` multiaddr).
3. GossipSub GRAFT finished (usually 1–2 heartbeats, ~1–2 s after she is connected).
4. Then she hears **someone’s next rebroadcast** (interval below: 2 s). Worst case she waits one extra interval after mesh join, not one extra minute.

What this does **not** guarantee:

| Case | Result |
|---|---|
| Carol never connects (NAT, wrong table id, no `--peer` off-LAN) | Unchanged. Rebroadcast cannot reach her. |
| Table already full / hand started | Unchanged. `HandleJoin` rejects. No mid-hand splice. |
| A single gossip packet is dropped | Next repeat (~2 s later) should land. LAN loss of *every* repeat for a minute is not realistic; it is also not proven. |
| Two partitions each filling 4 seats | Unchanged. Ready still does not bind a roster hash. Out of scope. |
| Clock skew reordering seats | Unchanged. Still `JoinedAtUnixMs` then Peer ID. Frozen timestamp only stops *rebroadcast* from changing the stamp. |

So: **not a proof, not WAN-safe, not catch-up after ready.** Honest claim for a write-up: *while the lobby is waiting, each peer rebroadcasts its original signed join every 2 s so a late subscriber can observe it as a live message.* For four laptops on one Wi-Fi, a 1-minute stagger should work. If mesh formation fails, it will still hang on “Waiting for players…” — same as today.

---

## Goal / done when

1. `BroadcastJoin` stamps `JoinedAtUnixMs` **once** per process and reuses that value on every later publish (same payload, new envelope `seq`).
2. `runP2PMode` rebroadcasts join every 2 s until `Lobby.Count() >= maxSeats`.
3. Duplicate joins (same peer already seated) do not print another `[lobby] … joined` line.
4. A second `BroadcastJoin` from the same node does not change that node’s stored `JoinedAtUnixMs`, and a remote who only saw the second publish stores **that same** timestamp (the frozen one on the envelope).
5. `go test ./internal/network ./cmd/poker` (or `go test ./...`) stays green. Existing `TestNode_BroadcastJoin_LobbyUpdated` still passes.
6. No proto change. No seat-order change. No ready-barrier change.

---

## Design

### Frozen timestamp

Today every `BroadcastJoin` does `time.Now().UnixMilli()`. If Alice sits at t=0 and republishes at t=45, Bob (who saw the first envelope) has Alice at 0 and Carol (who only saw the repeat) has Alice at 45000. `Seats()` then disagrees.

Store `joinTimestamp` on `Node`. First successful local `HandleJoin` sets it. Every publish, including repeats, puts that value on `Envelope.Timestamp`.

Envelope `seq` **must** keep increasing (`nextSeq()`). Late joiners have no watermark yet, so they accept the first seq they see. Peers who already seated you: `CheckAndUpdateSeq` allows the new seq, then `HandleJoin` rejects duplicate peer. Original `SeatInfo` (and original timestamp) stays. That is correct.

### Rebroadcast until full, not a 60 s timer

Keep the existing 6×400 ms loop for “`Publish` returned an error while the mesh is coming up.”

Then in `waitLoop`, every 2 s call `BroadcastJoin` again while `Count() < maxSeats`. Leave the 250 ms poll for the count check. Do not add a lobby timeout. Do not stop rebroadcasting after 60 s.

2 s is long enough to be cheap (~30 extra gossip messages per player per minute) and short enough that after GRAFT, Carol waits at most one interval.

### Duplicate callback

`dispatch` currently:

```
_ = n.Lobby.HandleJoin(...)
n.OnJoinTable(...)   // always
```

`HandleJoin` already errors on duplicate peer. Only fire `OnJoinTable` when `HandleJoin` returns nil. Same idea is optional for `HandleReady` but not required for this fix.

`BroadcastJoin` already ignores the local duplicate `HandleJoin` error on repeats (`_ =`). Keep that. Do not reset timestamp if `HandleJoin` says duplicate.

### Gamelog

Remote `dispatch` appends every accepted envelope seq, so repeats show up as extra `JOIN_TABLE` rows with rising `seq`. That is not equivocation (equivocation is same sender+seq, two payloads). Leave the log as-is. Do not add a “skip log on duplicate join” branch in this change unless a test fails.

---

## Scope

### Touch

| File | Change |
|---|---|
| `internal/network/node.go` | `joinTimestamp int64` on `Node`; freeze in `BroadcastJoin`; `dispatch` JOIN only calls `OnJoinTable` if `HandleJoin` succeeded |
| `cmd/poker/main.go` | In `waitLoop`, rebroadcast join every 2 s until lobby full |
| `internal/network/network_test.go` | Frozen timestamp + duplicate `OnJoinTable` not fired |

### Do not touch

- `internal/network/lobby.go` sort (`JoinedAtUnixMs` then `PlayerID`)
- `messages.proto` / `messages.pb.go`
- `internal/crypto/*`, `internal/game/*`, `internal/fault/*`
- Ready path, 2 s sleep after ready, shuffle, peels, sequencer
- GossipSub constructor options
- `GAME_STATE_SYNC`, stream catch-up, roster hash on `PLAYER_READY`

---

## Implementation notes

### `BroadcastJoin` (`node.go`)

```go
if n.joinTimestamp == 0 {
    n.joinTimestamp = time.Now().UnixMilli()
}
selfTimestamp := n.joinTimestamp
_ = n.Lobby.HandleJoin(msg, n.Host.PeerID, selfTimestamp)
// envelope Timestamp = selfTimestamp; seq = n.nextSeq() as today
```

First call seats us. Later calls: `HandleJoin` duplicate error, ignored; publish still happens with the **original** timestamp and a **new** seq.

If you take `n.mu`, do not hold it across `Publish`. A single `int64` written once from the `runP2PMode` goroutine is enough; do not invent a new lock unless a test races.

### `waitLoop` (`cmd/poker/main.go`)

Keep the 250 ms ticker for count. Add a 2 s rebroadcast ticker (or a `lastJoin` timestamp and fire from the same poll). On fire: `node.BroadcastJoin(ctx, 1)`; log publish errors, do not abort the wait.

Pseudo-structure:

```
poll 250ms, joinRepeat 2s
loop:
  ctx done → return
  poll → if Count() >= maxSeats → break
  joinRepeat → BroadcastJoin (ignore error / print it)
```

Do not `BroadcastJoin` on every 250 ms tick.

The initial 6-attempt loop can stay as a fast path for the first publish. After it, the 2 s ticker is what saves Carol at t=60 s.

### `dispatch`

```go
if err := n.Lobby.HandleJoin(msg, env.SenderId, env.Timestamp); err != nil {
    return
}
if n.OnJoinTable != nil {
    n.OnJoinTable(msg, env.SenderId)
}
```

Empty `fmt.Errorf("")` from `HandleJoin` is still `err != nil`. Duplicates stop here. Late Carol’s first copy of Alice still succeeds.

---

## Tests

Add in `internal/network/network_test.go` (unit, no mesh required where possible):

1. **`TestBroadcastJoin_RebroadcastKeepsTimestamp`**  
   Call `BroadcastJoin` twice on one node (can use the existing `makeTestNode` / short skip if it needs libp2p). Local `Seats()[0].JoinedAtUnixMs` equal after both calls. Envelope timestamp is not required if you only assert lobby state; if you intercept publish, also assert the second envelope’s `Timestamp` equals the first.

2. **`TestDispatch_DuplicateJoin_DoesNotCallback`**  
   Seat peer `p1` via `HandleJoin` (or first dispatch). Dispatch a second `JOIN_TABLE` from `p1` with a **higher** envelope seq and a **different** timestamp. `OnJoinTable` count stays 1. `Seats()` timestamp still the first one. (You need a valid signed envelope — follow `TestNode_BroadcastJoin_LobbyUpdated` / whatever helper already builds frames. If signing is painful, test `HandleJoin` duplicate + a tiny exported helper, but the callback gate lives in `dispatch`, so prefer a dispatch test.)

3. Keep **`TestNode_BroadcastJoin_LobbyUpdated`**: first join still reaches a second node.

Optional live check (not automated): host Alice, wait ~45–60 s, start Bob with `--peer`. Bob’s lobby should show Alice after at most a few seconds, then both wait for remaining seats. If you only ever start all processes at once, you have not tested this fix.

---

## Out of scope (proper lobby, later)

- Unicast the original signed join envelopes to a new peer
- `PLAYER_READY` carrying a roster hash
- Sort by Peer ID instead of clocks
- Mid-hand rejoin

Those are a multi-day lobby protocol. This file is the evening patch that makes a 1-minute stagger work on a LAN.

---

## Checklist

- [ ] `Node.joinTimestamp` set once; `BroadcastJoin` reuses it
- [ ] `waitLoop` rebroadcasts every 2 s until full
- [ ] `OnJoinTable` only on successful `HandleJoin`
- [ ] Timestamp test on double `BroadcastJoin`
- [ ] Duplicate-join does not reprint lobby line
- [ ] `go test ./internal/network` green
- [ ] Manual: start host, wait ~1 min, join — joiner sees host
