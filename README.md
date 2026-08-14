# Decentralized Poker Engine

Peer-to-peer Texas Hold'em with **no game server**. Every seated peer runs the same Hold'em state machine. Cards are dealt with a joint SRA shuffle and partial decrypts (mental poker), so opponents' hole cards stay hidden until showdown.

**Status:** LAN mental-poker Hold'em is the default (`poker host` / `poker join`). `--no-crypto` is debug only: shared-seed plaintext, all cards visible. On-chain ETH escrow is specified and tested in Solidity; the Go RPC client is **not** wired into the live loop.

Companion docs: [system design](./SYSTEM_DESIGN_OVERVIEW.md) · [interview walkthrough](./SYSTEMS_DESIGN_INTERVIEW.md)

---

## What works today

- Complete Texas Hold'em: blinds, check/call/raise/fold/all-in, side pots, 7-card eval, multi-hand sessions
- Equal peers over libp2p (GossipSub + Noise + signed envelopes)
- Sequenced actions so honest nodes compute the same pots and winners
- Default cryptographic dealing: each peer keeps only its own private exponent `d`; shuffle steps and peels go on the wire as ciphertexts
- Timeout fold + Shamir reconstruction so a disconnect **after** the shuffle does not deadlock remaining peels
- Local mode: human vs bots on one process
- TUI (Bubble Tea)

Multiplayer needs **3–9 seats** (Shamir recovery needs `n ≥ 3`). Local vs bots still allows 2–9.

---

## Prerequisites

- **Go 1.25+** (see `go.mod`)
- Git
- Optional: Node.js 18+ only if you want to run Hardhat tests in `contracts/`

---

## Build

```bash
cd DecentralizedPokerEngine
go build -o poker ./cmd/poker
```

On Windows that produces `poker.exe`. Commands below use `./poker`; on PowerShell use `.\poker.exe`.

```bash
./poker version    # currently v0.7.0
./poker help
```

---

## Run a multiplayer table (default: crypto dealing)

You need **three processes**. On one LAN, mDNS can find the host; otherwise copy the multiaddr the host prints.

**Terminal 1 — host**

```bash
./poker host --seats 3 --name Alice --table friday
```

You should see something like:

```
=== P2P Poker  ·  Alice ===
Peer ID  : 12D3KooXxx...

Share one of these addresses with the other player:
  /ip4/192.168.1.100/tcp/9000/p2p/12D3KooXxx...

Table : friday   Seats : 3   Buy-in : 1000 chips

Waiting for players… (Ctrl-C to quit)
```

**Terminals 2 and 3 — joiners (same LAN, mDNS)**

```bash
./poker join --name Bob --table friday
./poker join --name Carol --table friday
```

**Joiners on another network** — pass the host multiaddr:

```bash
./poker join --name Bob --table friday --peer "/ip4/192.168.1.100/tcp/9000/p2p/12D3KooXxx..."
```

When all seats are filled the table prints `Cryptographic dealing · SRA 2048-bit · opponent holes stay hidden`, then `Shuffling…`. 2048-bit encrypt-then-permute of 52 cards takes **several seconds**. That is expected, not a hang.

If two processes share one machine, give joiners a free listen port:

```bash
./poker join --name Bob --table friday --listen /ip4/0.0.0.0/tcp/9001
./poker join --name Carol --table friday --listen /ip4/0.0.0.0/tcp/9002
```

A mixed table (one peer `--no-crypto`, others default) **exits** rather than silently falling back to plaintext.

### Debug: all cards visible

```bash
./poker host --seats 3 --name Alice --no-crypto
./poker join --name Bob --no-crypto
```

Every peer must pass `--no-crypto`. This is the old shared-seed shuffle for sync testing only.

---

## Local game (bots)

```bash
./poker init          # writes config.yaml if missing
./poker               # reads config.yaml
./poker -c custom.yaml
```

Bots live in the same process. No libp2p, no SRA shuffle.

---

## Configuration

`./poker init` writes `config.yaml`. Defaults match `config.DefaultYAML()`:

```yaml
player_name: "Player"
data_dir: "~/.poker"

network:
  listen_addr: "/ip4/0.0.0.0/tcp/9000"   # libp2p multiaddr, not host:port
  enable_mdns: true
  max_peers: 20

game:
  table_id: "my-table"
  max_seats: 6          # local 2–9; P2P host/join requires 3–9
  small_blind: 5
  big_blind: 10
  buy_in: 1000

fault:
  heartbeat_interval: 5s
  heartbeat_timeout: 15s
  vote_expiry: 30s

chain:
  enabled: false        # not used by host/join yet
  rpc_url: "http://127.0.0.1:8545"
```

Identity is a libp2p Ed25519 seed under `data_dir` (`~/.poker/identity.key`). `./poker keygen` prints an Ethereum key for a future chain mode; it is not required to play.

---

## How a crypto hand runs

1. Lobby fills; each `JOIN_TABLE` carries that peer's public exponent `e` (`d` never leaves the node).
2. Peers split Shamir shares of `d` over **direct streams** (not gossip).
3. Seat-order SRA shuffle: each peer encrypts-then-permutes and publishes `SHUFFLE_STEP` (output deck + commitment, **no permutation**).
4. Hole cards: everyone except the recipient peels one layer (`PARTIAL_DECRYPT` + ZK proof). The recipient peels last **locally**. Other seats' holes stay empty on this replica.
5. Betting uses the existing sequenced `PLAYER_ACTION` log. The engine does not deal streets from a local deck.
6. Flop / turn / river: public peels, then `ApplyStreet` with the new cards only.
7. Showdown: public peel of remaining hole indexes in **seat order** (same on every replica). Fold-to-winner skips reveals.
8. Next hand: new shuffle (session id mixes `handNum`). Keys do not rotate.

`--no-crypto` skips steps 2–4 and 6–7: every node Fisher–Yates-shuffles the same public seed and sees every card.

---

## Commands

```bash
./poker                         # local vs bots
./poker init                    # create config.yaml
./poker keygen                  # print an Ethereum private key (unused in live play)
./poker version
./poker help

./poker host [flags]
  --seats N                     # 3–9 (required for Shamir recovery)
  --name NAME
  --table ID
  --listen MULTIADDR            # default /ip4/0.0.0.0/tcp/9000
  --no-crypto                   # debug: shared-seed plaintext

./poker join [flags]
  --peer MULTIADDR              # host address; optional on the same LAN (mDNS)
  --name NAME
  --table ID                    # must match the host
  --listen MULTIADDR
  --no-crypto
```

### Keyboard (TUI)

| Key | Action |
|---|---|
| `f` | Fold |
| `c` | Check / call |
| `r` | Raise (amount input) |
| `a` | All-in |
| `←` `→` / `h` `l` | Move between actions |
| Enter | Confirm |
| `↑` `↓` / `k` `j` | Scroll log |
| `q` | Quit |

---

## Architecture

| Package | Role |
|---|---|
| `cmd/poker` | CLI, local loop, `runP2PMode` (lobby → shuffle → peels → TUI) |
| `internal/game` | Pure Hold'em reducer. Crypto mode takes cards as inputs (`StartHandCrypto`, `ApplyStreet`, `ApplyHoleReveal`) |
| `internal/crypto` | SRA, Keyring (own `d` only), ShuffleSession, DealSession, ZK peels, Shamir math |
| `internal/network` | libp2p, GossipSub, lobby, `CryptoHand`, share unicast, protobuf codec |
| `internal/fault` | Heartbeats, 2/3 timeout votes, key-share store, slash records |
| `internal/tui` | Bubble Tea table; opponent holes hidden unless local or winner |
| `internal/chain` | Escrow helper + **stub** RPC client (not called from `main`) |
| `contracts/` | `PokerEscrow.sol` + Hardhat tests |

There is no central dealer process. `poker host` is only the first listener.

Design notes: [`SYSTEM_DESIGN_OVERVIEW.md`](./SYSTEM_DESIGN_OVERVIEW.md). How the crypto path was wired: [`CRYPTO_DEAL_PLAN.md`](./CRYPTO_DEAL_PLAN.md) and `plans/phase-1-keyring.md` … `plans/phase-6-liveness.md`.

---

## Testing

```bash
go test ./...                          # full suite (a few 2048-bit tests; tens of seconds)
go test ./internal/game -count=1
go test ./internal/crypto -count=1
go test ./internal/network -count=1    # includes fake-net crypto hands (no two binaries)
```

Protocol tests use an in-process fake bus. They do **not** start two `poker` processes. A 3-terminal LAN run is still the manual acceptance check.

Optional contract tests:

```bash
cd contracts && npm test
```

---

## Known limitations

1. **P2P minimum 3 seats** — after one drop, Shamir still needs enough shares. Heads-up is local-vs-bots only.
2. **Disconnect = fold, no rejoin** — timeout vote folds the seat; survivors reconstruct `d` and finish **peels**. Restart the table for the next session. `GAME_STATE_SYNC` is unused.
3. **Disconnect during shuffle aborts the hand** — the secret permutation cannot be recovered.
4. **No live ETH** — Solidity escrow exists; `internal/chain` is stubbed; `main.go` never talks to a node.
5. **LAN / port-forward** — mDNS on the LAN, or `--peer` with a reachable multiaddr. No DHT, no relays. NAT is UPnP/`NATPortMap` only.
6. **One table** — no tournaments, no mid-hand catch-up.
7. **Not BFT** — authenticated total order among honest nodes; equivocation is detected after the fact, not prevented by a quorum on every fold.
8. **Seat order uses join timestamps** — clock skew can theoretically reorder seats. Honest LAN clocks are the assumption.

---

## Troubleshooting

**Waiting for players forever**  
Joiners must use the same `--table` and a reachable address. Copy the host multiaddr exactly. On one PC, give each joiner a different `--listen` port.

**`--seats 2` errors**  
P2P requires 3+. Use local mode for heads-up vs bots.

**`crypto dealing requires every seat to publish e`**  
One peer joined with `--no-crypto`. Either all crypto or all `--no-crypto`.

**Stuck on `Shuffling…` for minutes**  
A shuffle step was lost or a peer died mid-shuffle. Mid-shuffle disconnect **aborts** (2-minute wait, then error). Restart all three processes. Several seconds of shuffle is normal.

**Opponent hole cards visible mid-hand**  
You are on `--no-crypto`, or looking at showdown/winners. Default crypto TUI hides non-local holes until reveal.

**`address already in use`**  
Default listen is TCP 9000. Use `--listen /ip4/0.0.0.0/tcp/0` or another port.

**Actions not syncing**  
Same table id, same network (or `--peer`). Firewalls must allow the TCP port in the multiaddr.

**Windows mDNS warnings in tests**  
`TestNode_ThreePeerMesh_AllReceive` can flake on Windows multicast. Retry; crypto fake-net tests do not use libp2p.

---

## Roadmap

**Done:** Hold'em engine, P2P mesh, TUI, SRA dealing on `host`/`join`, timeout fold + Shamir peels.

**Not started in the live loop:** Ethereum RPC, mid-hand reconnect, DHT/relays, tournaments.

---

## License

MIT
