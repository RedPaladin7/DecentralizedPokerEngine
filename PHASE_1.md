# Phase 1 — Orientation

This is the first onboarding chapter. After it you should be able to **build the binary, play a local hand against bots, and explain what this repository is (and is not)** without opening `internal/crypto` or `internal/network`.

The reading list this chapter expands is in [`READ_GUIDE.md`](./READ_GUIDE.md). The teaching narrative it sits next to is [`HOW_IT_WORKS.md`](./HOW_IT_WORKS.md) §§1–9, 23, 25–26. The compact architecture brief is [`SYSTEM_DESIGN_OVERVIEW.md`](./SYSTEM_DESIGN_OVERVIEW.md) §§1–2.

**You are here to learn:** the product claim, the two run modes, how configuration and identity work, how `main` dispatches, and which package owns which job. You are **not** here to learn SRA, GossipSub sequencers, or escrow.

**Do this with your hands before you finish the chapter:**

```bash
go build -o poker ./cmd/poker
./poker            # Windows: .\poker.exe
```

Optionally, in a second terminal, start a host just to see a multiaddr printed. You do not need three peers yet:

```bash
./poker host --seats 3 --name Alice
```

**Do not read yet:** `internal/crypto`, `internal/fault`, `contracts/`, `plans/`. Peeking will not help; those chapters assume this one.

**Architectural rule to keep in your head the whole time:** `internal/game` never imports `internal/network`. Networking produces authenticated, ordered inputs. The engine reduces them. Mixing those layers is how “the host accidentally becomes the server” happens.

---

## Table of contents

1. [How to use this chapter](#1-how-to-use-this-chapter)
2. [What this repository is](#2-what-this-repository-is)
3. [What it deliberately does not solve](#3-what-it-deliberately-does-not-solve)
4. [Build, run, and the first multiaddr](#4-build-run-and-the-first-multiaddr)
5. [Two run modes](#5-two-run-modes)
6. [Networking vocabulary you need now](#6-networking-vocabulary-you-need-now)
7. [Map of the repository](#7-map-of-the-repository)
8. [What the binary actually depends on (`go.mod`)](#8-what-the-binary-actually-depends-on-gomod)
9. [Configuration: types and invariants](#9-configuration-types-and-invariants)
10. [Loading config](#10-loading-config)
11. [Identity: two keys, two jobs](#11-identity-two-keys-two-jobs)
12. [Process entry: `cmd/poker/main.go`](#12-process-entry-cmdpokermain.go)
13. [Call graph from `main`](#13-call-graph-from-main)
14. [Worked example: one local hand](#14-worked-example-one-local-hand)
15. [The shape of P2P mode (preview only)](#15-the-shape-of-p2p-mode-preview-only)
16. [Tests in this phase](#16-tests-in-this-phase)
17. [Common mistakes](#17-common-mistakes)
18. [Exit check](#18-exit-check)
19. [Phase 1 glossary](#19-phase-1-glossary)

---

## 1. How to use this chapter

Read top to bottom once. When a code excerpt appears, open that file in the editor and match the excerpt to the live source. Line numbers here were accurate when this chapter was written; if they drift, trust the file.

This chapter is **orientation**, not a rewrite of [`HOW_IT_WORKS.md`](./HOW_IT_WORKS.md). Where that file teaches networking from zero, this file teaches **how this repo is laid out so you can run it**. If a term is new (LAN, multiaddr, GossipSub), the glossary at the end is enough for Phase 1. Phase 3 is where those words become code.

Suggested time: one afternoon, including a local game. Stop when the [exit check](#18-exit-check) is true.

---

## 2. What this repository is

A classic online poker site is a **server**. You open a browser; the site shuffles, deals, tells you whose turn it is, and pays the winner. You have to trust that operator: that they shuffled fairly, that they did not peek at hole cards, that they will not rewrite the pot, and that the site staying up is the only way the table stays up.

This repository’s goal is a **table with no house**. Every seated program is a full replica of the same Texas Hold’em engine. There is no process whose job is “be the dealer.” Cards on the multiplayer path are produced by a cryptographic protocol called **mental poker** (here: Shamir–Rivest–Adleman commutative encryption, abbreviated **SRA**). Public facts — whose turn it is, how big the pot is, who won — are computed independently on every laptop from the same ordered list of actions.

[`README.md`](./README.md) states the product in one paragraph:

> Peer-to-peer Texas Hold'em with **no game server**. Every seated peer runs the same Hold'em state machine. Cards are dealt with a joint SRA shuffle and partial decrypts (mental poker), so opponents' hole cards stay hidden until showdown.

That is the claim you should be able to repeat. Everything else in the repo is either (a) the Hold’em reducer that makes “same inputs → same pots” true, (b) the mesh that delivers those inputs, (c) the crypto that hides cards without a dealer, or (d) liveness and settlement around the edges.

Concrete product goals, as implemented today:

| Goal | What that means in this repo |
|---|---|
| Play real Hold’em | Blinds, hole cards, flop / turn / river, check / call / raise / fold / all-in, side pots, 7-card evaluation, multiple hands |
| No game server | `poker host` is only the first listener. After that, every peer publishes and subscribes |
| Same outcome on every honest machine | Identical initial state + identical ordered actions → identical `GameState` |
| Hidden hole cards | You decrypt your own last encryption layer locally. Opponents’ holes stay empty on your replica until showdown |
| Unbiased shuffle | Every player encrypts **and** secretly permutes. One honest permuter is enough to randomize the order |
| Authenticated talk | Every gossip message is Ed25519-signed. You cannot impersonate another seat |
| Survive one crash **after** the shuffle | Timeout-fold the silent player, reconstruct their private exponent from Shamir shares, finish remaining decrypts |
| Optional money later | A Solidity escrow contract exists. The Go client does **not** talk to Ethereum yet. Chips in the demo are local counters |

**Status snapshot** (from the README; this is ground truth, not aspiration):

- LAN mental-poker Hold’em is the **default** (`poker host` / `poker join`).
- `--no-crypto` is **debug only**: shared-seed plaintext, all cards visible.
- On-chain ETH escrow is specified and tested in Solidity; the Go RPC client is **not** wired into the live loop.
- Multiplayer needs **3–9 seats**. Local vs bots still allows **2–9**.

If you remember only one sentence from this section: **chips on the screen are not money, and `poker host` is not a game server.**

---

## 3. What it deliberately does not solve

Non-goals are as important as goals. New joiners waste weeks “just adding a server” or “just wiring ETH” because they treat the README’s limitations as bugs.

From [`HOW_IT_WORKS.md`](./HOW_IT_WORKS.md) §2 and [`SYSTEM_DESIGN_OVERVIEW.md`](./SYSTEM_DESIGN_OVERVIEW.md) §1:

- **Not a 10,000-player tournament.** A poker table is 3–9 people. Scale-out would be **many independent tables**, not a bigger mesh.
- **Not Byzantine agreement** (PBFT / HotStuff) on every fold. Honest nodes apply a total order of signed actions; a double-talker is **detected** after the fact, not prevented by a quorum on every action.
- **Not a public-internet product.** Discovery is LAN multicast or a copied address. Relays are off. NAT traversal is UPnP only.
- **Not 2-player P2P.** Shamir recovery after a disconnect needs at least three seats. Heads-up is local-versus-bots only.
- **Not live ETH.** `contracts/PokerEscrow.sol` is real Solidity. `internal/chain` is a stub. `cmd/poker` never calls it.
- **Not mid-hand reconnect.** A disconnected player is folded. `GAME_STATE_SYNC` exists in the proto and is unused.

[`README.md`](./README.md) lists the same limits under “Known limitations.” Prefer that list (and this chapter) over [`ISSUES_AND_RECOMMENDATIONS.md`](./ISSUES_AND_RECOMMENDATIONS.md) when you want *current* status. The issues file was written before live SRA dealing shipped; several “must fix” items are now done.

A fair one-line claim, from [`HOW_IT_WORKS.md`](./HOW_IT_WORKS.md) §6:

> A LAN mental-poker Hold’em table: peers jointly shuffle under commutative encryption, deal with partial decrypts and ZK checks, and agree on pots locally; on-chain escrow is specified but not integrated.

---

## 4. Build, run, and the first multiaddr

### 4.1 Prerequisites

- **Go 1.25+** (see `go.mod`; currently `go 1.25.7`)
- Git
- Optional: Node.js 18+ **only** if you want Hardhat tests in `contracts/` (Phase 5). The Go binary does not use Node.

### 4.2 Build

From the repo root:

```bash
go build -o poker ./cmd/poker
```

On Windows that produces `poker.exe`. Commands below use `./poker`; in PowerShell use `.\poker.exe`.

```bash
./poker version    # currently v0.7.0
./poker help
```

The version string in code is slightly more specific than the README:

```1329:1336:cmd/poker/main.go
func printHelp() {
	fmt.Print(`P2P Texas Hold'em Poker Engine

USAGE:
  poker                    Local game vs bots (reads config.yaml)
  poker host [flags]       Host a multiplayer table
  poker join [flags]       Join an existing table
```

and:

```33:35:cmd/poker/main.go
		case "version":
			fmt.Println("p2p-poker v0.7.0 (phase 7 — integration)")
			return
```

“Phase 7” here is historical numbering from the crypto-wiring plans, not “Phase 1 of this onboarding track.” Do not confuse `plans/phase-1-keyring.md` with this chapter.

### 4.3 Local game (what you should run first)

```bash
./poker init          # writes config.yaml if missing
./poker               # reads config.yaml, or defaults if none
./poker -c custom.yaml
```

Bots live in the **same process**. No libp2p, no SRA shuffle. This is the control experiment: Hold’em rules + TUI, nothing distributed.

Keyboard (TUI):

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

Play one hand. Notice: you can see **every** hole card, including bots’. That is expected. Privacy is a P2P-crypto property, not a local-mode property.

### 4.4 Optional: print a host multiaddr

You do not need three peers in Phase 1. Starting a host is useful only so the words “Peer ID” and “multiaddr” stop being abstract:

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

Read that address left to right: IPv4, this host, TCP, this port, and the peer you should find there is this ID. That string is a **libp2p multiaddr**. Phase 3 explains how joiners use it. For now, Ctrl-C is fine.

If you already have a local game bound to TCP 9000, the host will fail with `address already in use`. Local mode does **not** listen on 9000 — only `host` / `join` do. Two hosts on one machine need different `--listen` ports.

### 4.5 What not to run yet

A three-terminal crypto table (`host` + two `join`s) is the Phase 4 lab. A three-terminal `--no-crypto` table is the Phase 3 lab. Phase 1’s lab is **local mode**. If you jump to a full mesh now, you will watch “Shuffling…” for several seconds and have no types to attach the wait to.

---

## 5. Two run modes

Two products share one binary:

```
./poker              →  local vs bots   (plaintext deck, one process)
./poker host | join  →  P2P table       (SRA by default; --no-crypto is debug)
```

They share `internal/game` (the rules) and `internal/tui` (the picture). They share almost nothing else.

### 5.1 Local

From [`HOW_IT_WORKS.md`](./HOW_IT_WORKS.md) §23:

- One `game.Machine`. `StartHand` shuffles a real `Deck` with a local RNG.
- Every bot “seat” lives in the same process, so of course the process knows every hole card.
- Bots are Bubble Tea timers (~600 ms) that check or call.
- Seats **2–9** are allowed. This is how you test heads-up without three laptops.
- Next-hand delay is ~1.5 s.

If pots later disagree between local mode and P2P `--no-crypto`, the bug is in the engine or the sequencer, not in SRA. That is why local mode exists as a control, not as a toy.

### 5.2 P2P (the interesting one — later)

`poker host` / `poker join`. Default is cryptographic dealing. `--no-crypto` is a debug mode where every node shuffles the same public seed and **all cards are visible**.

P2P **rejects fewer than 3 seats**. The reason is liveness, not Hold’em: after one player drops, Shamir reconstruction of their private exponent still needs leftover shares. Heads-up P2P would deadlock on a disconnect. That check lives in `main`, not in `config.Validate`:

```1388:1395:cmd/poker/main.go
const minP2PSeats = 3

func requireP2PSeats(n int) error {
	if n < minP2PSeats {
		return fmt.Errorf("runP2PMode: need at least 3 seats for timeout recovery (got %d)", n)
	}
	return nil
}
```

`config.Validate` still allows 2–9, because local mode is allowed to be heads-up. Two validators, two jobs. Mixing them is a common edit-time bug (see [§17](#17-common-mistakes)).

### 5.3 `--no-crypto` is not “fast production”

Every peer must pass `--no-crypto`. A mixed table (one peer with the flag, others default) **exits** rather than silently falling back to plaintext. That is intentional: silent fallback would look like a working crypto table with every card public.

Do not treat `--no-crypto` as the mode you ship. It exists so Phase 3 can study action order without waiting on 2048-bit exponentiation.

---

## 6. Networking vocabulary you need now

You do not need to read `internal/network` yet. You do need these words, because the README, the host banner, and `config.yaml` use them.

### 6.1 LAN, not “the cloud”

This engine’s happy path is a **LAN** (the Wi-Fi in an apartment, ethernet in a lab). Machines on a LAN can usually reach each other by private IP (`192.168.x.x`, `10.x.x.x`). Discovery via **mDNS** (the same idea as “the printer showed up without me typing an IP”) only works inside that neighborhood.

You *can* join from another network if you copy the host multiaddr and that address is reachable (port forwarded). There is no DHT and no relay in this repo. “My friend is behind a strict dorm NAT” is an operational problem, not a solved one.

### 6.2 IP, port, TCP, multiaddr

- An **IP address** names a machine on a network. It is not a person and it is not permanent.
- A **port** names a program on that machine. This process defaults to **TCP 9000**. Two copies on one laptop cannot both bind 9000; the second needs `--listen /ip4/0.0.0.0/tcp/9001`.
- `/ip4/0.0.0.0` means “accept connections on every local IPv4 interface,” not “the public internet.”
- **TCP** delivers an ordered, retransmitted **byte stream**. It does not encrypt, and it does not deliver application messages — the code prefixes each message with a 4-byte length. You will see that in Phase 3. For now: “TCP is the pipe.”
- A **multiaddr** is a self-describing address:

```
/ip4/192.168.1.100/tcp/9000/p2p/12D3KooW...
```

### 6.3 Host is a bootstrap peer

Someone has to listen first, or there is nobody to connect to. `poker host` does three unglamorous things:

1. Binds a TCP port and prints multiaddrs.
2. Advertises itself on the LAN via mDNS (service tag `p2p-poker-v1`).
3. Creates the lobby for a table id with N seats.

After joiners arrive, Alice is not the dealer, not the sequencer, and not the judge of the pot. She is the **bootstrap peer**. If you draw a star with Alice in the middle *as a game server*, you have misunderstood the design.

[`SYSTEM_DESIGN_OVERVIEW.md`](./SYSTEM_DESIGN_OVERVIEW.md) §2 draws the real topology: a **libp2p mesh**, not a star. GossipSub topics `poker/table/<id>` and `poker/heartbeat/<id>` plus direct streams `/poker/1.0.0`. Phase 3 fills that in. The Phase 1 takeaway is: **equal peers after join.**

### 6.4 “Decentralized” here

From [`HOW_IT_WORKS.md`](./HOW_IT_WORKS.md) §6:

- **Does mean:** no central game process; equal protocol roles; private inputs stay private on the default P2P path; evidence (signed envelopes) instead of a house.
- **Does not mean:** no computers in the middle (packets still hit a router); blockchain consensus for every fold; BFT; internet-scale NAT traversal; live ETH.

Decentralization is about **who you trust for the game**, not about physics.

### 6.5 Replica

Every seated peer runs the same `game.Machine`. Public state is a **replicated state machine**:

```
identical initial state  +  identical ordered actions  →  identical GameState
```

The engine has no sockets. That is why `internal/game` must not import `internal/network`. Phase 2 is the engine. Phase 1 only needs you to believe the rule exists.

---

## 7. Map of the repository

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

**Phase 1 lives in `README.md`, the early HOW_IT_WORKS / SYSTEM_DESIGN slices, `go.mod`, `config/`, and `cmd/poker/`.** Everything under `internal/` except a light touch of `game` + `tui` in the local-mode walkthrough is a later phase.

Package roles, from the README:

| Package | Role |
|---|---|
| `cmd/poker` | CLI, local loop, `runP2PMode` (lobby → shuffle → peels → TUI) |
| `internal/game` | Pure Hold’em reducer. Crypto mode takes cards as inputs |
| `internal/crypto` | SRA, Keyring (own `d` only), shuffle/deal sessions, ZK peels, Shamir math |
| `internal/network` | libp2p, GossipSub, lobby, `CryptoHand`, share unicast, protobuf codec |
| `internal/fault` | Heartbeats, 2/3 timeout votes, key-share store, slash records |
| `internal/tui` | Bubble Tea table; opponent holes hidden unless local or winner |
| `internal/chain` | Escrow helper + **stub** RPC client (not called from `main`) |
| `contracts/` | `PokerEscrow.sol` + Hardhat tests |

### Files you should not study

| File | Why skip |
|---|---|
| `go.sum` | Module checksums. Not architecture. |
| `package-lock.json` | Hardhat lockfile. |
| `internal/network/messages.pb.go` | Generated from `messages.proto`. |
| `internal/chain/abi/PokerEscrow.go` | Generated Go bindings. |
| `extra.txt` | One `protoc` command. |
| `.gitignore` | Repo hygiene. |

---

## 8. What the binary actually depends on (`go.mod`)

Do not read `go.sum`. Do read the top of `go.mod`. Direct requirements are the architectural ones:

```1:10:go.mod
module github.com/RedPaladin7/DecentralizedPokerEngine

go 1.25.7

require (
	github.com/libp2p/go-libp2p v0.48.0
	github.com/libp2p/go-libp2p-pubsub v0.15.0
	github.com/multiformats/go-multiaddr v0.16.1
	google.golang.org/protobuf v1.36.11
)
```

What each of those is *for* (you will open the corresponding packages in later phases):

| Dependency | Job in this repo |
|---|---|
| `go-libp2p` | Host identity, listen multiaddr, Noise, TCP, peerstore, streams. Relays are disabled in our wrapper. |
| `go-libp2p-pubsub` | GossipSub: topics `poker/table/<id>` and `poker/heartbeat/<id>`. Unordered, best-effort. |
| `go-multiaddr` | Parse/print `/ip4/.../tcp/.../p2p/...` |
| `protobuf` | Wire codec for `Envelope` and payloads. Read `messages.proto`, not the generated `.pb.go`. |

Bubble Tea (`github.com/charmbracelet/bubbletea`) and YAML (`gopkg.in/yaml.v3`) appear as **indirect** in this `go.mod` because they are pulled in through other modules, but `cmd/poker` and `config` import them directly. If you are grepping for “what draws the TUI,” it is Bubble Tea. If you are grepping for “what parses `config.yaml`,” it is `yaml.v3`.

`go-ethereum` is also in the indirect graph. That does **not** mean live chain mode. `internal/chain` is stubbed; `main` never talks to a node. Seeing geth in `go.mod` is not evidence of ETH payouts.

Go version **1.25** is a hard floor. If `go version` prints 1.22, the build will fail in ways that look like the project is broken. It is not.

---

## 9. Configuration: types and invariants

Package: `config`. Two files, two jobs:

- `config.go` — the struct, defaults, validation, identity seed, Ethereum keygen, the YAML template `init` writes.
- `loader.go` — find a file, unmarshal, apply env overrides, expand `~`, validate.

This is the knob panel `main` will pass into every other package. Wrong seats, wrong listen address, wrong table id: they all start here.

### 9.1 The `Config` struct

```14:53:config/config.go
type Config struct {
	PlayerName string `yaml:"player_name"`
	DataDir string `yaml:"data_dir"`
	Network NetworkConfig `yaml:"network"`
	Game GameConfig `yaml:"game"`
	Fault FaultConfig `yaml:"fault"`
	Chain ChainConfig `yaml:"chain"`
}

type NetworkConfig struct {
	ListenAddr string `yaml:"listen_addr"`
	BootstrapPeers []string `yaml:"bootstrap_peers"`
	EnableMDNS bool `yaml:"enable_mdns"`
	MaxPeers int `yaml:"max_peers"`
}

type GameConfig struct {
	TableID string `yaml:"table_id"`
	MaxSeats int `yaml:"max_seats"`
	SmallBlind int64 `yaml:"small_blind"`
	BigBlind int64 `yaml:"big_blind"`
	BuyIn int64 `yaml:"buy_in"`
	ActionTimeout time.Duration `yaml:"action_timeout"`
}

type FaultConfig struct {
	HeartbeatInterval time.Duration `yaml:"heartbeat_interval"`
	HeartbeatTimeout time.Duration `yaml:"heartbeat_timeout"`
	VoteExpiry time.Duration `yaml:"vote_expiry"`
}

type ChainConfig struct {
	Enabled bool `yaml:"enabled"`
	RPCURL string `yaml:"rpc_url"`
	ContractAddress string `yaml:"contact_address"`
	ChainID int64 `yaml:"chain_id"`
	BuyInWei string `yaml:"buy_in_wei"`
	GasLimit uint64 `yaml:"gas_limit"`
	PrivateKeyHex string `yaml:"private_key_hex"` 
}
```

Four nested blocks match the four later phases of the system:

| Block | Who consumes it (later) | Phase 1 note |
|---|---|---|
| `Network` | `internal/network` host / discovery | `listen_addr` is a **multiaddr**, not `host:port` |
| `Game` | `internal/game` + lobby | blinds, buy-in, seats, table id |
| `Fault` | `internal/fault` | unused in local mode except being constructed |
| `Chain` | `internal/chain` | **not used by host/join**. `enabled: false` |

Notice the YAML tag on `ContractAddress`: `contact_address` (a typo), while `DefaultYAML()` writes `contract_address`. Chain mode is dead in the live loop, so this does not bite play. Do not “fix” it as your first PR without checking both the tag and the template; it is a landmine for a future chain wiring, not a Phase 1 task.

### 9.2 Defaults

```55:85:config/config.go
func Default() *Config {
	home, _ := os.UserHomeDir()
	return &Config{
		PlayerName: "Player",
		DataDir: filepath.Join(home, ".poker"),
		Network: NetworkConfig{
			ListenAddr: "/ip4/0.0.0.0/tcp/9000",
			EnableMDNS: true,
			MaxPeers: 20,
		},
		Game: GameConfig{
			TableID: "default-table",
			MaxSeats: 6,
			SmallBlind: 5,
			BigBlind: 10,
			BuyIn: 1000,
			ActionTimeout: 45 * time.Second,
		}, 
		Fault: FaultConfig{
			HeartbeatInterval: 5 * time.Second,
			HeartbeatTimeout: 15 * time.Second,
			VoteExpiry: 30 * time.Second,
		},
		Chain: ChainConfig{
			Enabled: false,
			RPCURL: "http://127.0.0.1:8545",
			ChainID: 31337,
			GasLimit: 500_000,
		},
	}
}
```

`data_dir` defaults to `~/.poker`. That directory holds `identity.key` (libp2p seed). It is created on first P2P run, not on `./poker` local unless something else asks for it.

`init` does **not** call `Default()` and marshal it. It writes the string from `DefaultYAML()`, which is the human-editable template (table id `my-table`, commented bootstrap peers, commented `private_key_hex`). The in-memory defaults and the file template are *almost* the same. Table id differs (`default-table` vs `my-table`). If you run `./poker` with no file you get `default-table`; if you `init` then run, you get `my-table`. Harmless locally; confusing if you expected host/join to pick up a table id you never wrote.

### 9.3 Validation invariants

```87:112:config/config.go
func (c *Config) Validate() error {
	if c.PlayerName == "" {
		return fmt.Errorf("player_name is required")
	}
	if c.Game.MaxSeats < 2 || c.Game.MaxSeats > 9 {
		return fmt.Errorf("max_seats must be 2–9, got %d", c.Game.MaxSeats)
	}
	if c.Game.SmallBlind <= 0 {
		return fmt.Errorf("small_blind must be positive")
	}
	if c.Game.BigBlind <= 0 || c.Game.BigBlind <= c.Game.SmallBlind {
		return fmt.Errorf("big_blind must be > small_blind")
	}
	if c.Game.BuyIn < c.Game.BigBlind*10 {
		return fmt.Errorf("buy_in must be at least 10x big_blind")
	}
	if c.Chain.Enabled {
		if c.Chain.RPCURL == "" {
			return fmt.Errorf("chain.rpc_url required when chain is enabled")
		}
		if c.Chain.ContractAddress == "" {
			return fmt.Errorf("chain.contract_address required when chain is enabled")
		}
	}
	return nil
}
```

Invariants to memorize:

1. **Name required.** Empty `player_name` is invalid.
2. **Seats 2–9 at config level.** P2P then tightens to 3–9 in `requireP2PSeats`.
3. **Big blind strictly greater than small blind.** Equal blinds fail.
4. **Buy-in at least 10× big blind.** Default 1000 / 10 = 100×, so the default is fine. `buy_in: 50` with `big_blind: 10` is not.
5. **Chain fields only required if `chain.enabled` is true.** It is false. Leave it.

`Validate` does **not** check that `listen_addr` is a valid multiaddr. A typo like `0.0.0.0:9000` survives config load and dies later inside libp2p. That is why the README keeps repeating “libp2p multiaddr, not host:port.”

### 9.4 CLI flags overlay the file

`host` and `join` load config first, then overwrite fields from flags:

```233:267:cmd/poker/main.go
func applyP2PFlags(cfg *config.Config, args []string) bool {
	noCrypto := false
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--no-crypto":
			noCrypto = true
		case "--seats":
			if i+1 < len(args) {
				i++
				fmt.Sscanf(args[i], "%d", &cfg.Game.MaxSeats)
			}
		case "--listen":
			if i+1 < len(args) {
				i++
				cfg.Network.ListenAddr = args[i]
			}
		case "--name":
			if i+1 < len(args) {
				i++
				cfg.PlayerName = args[i]
			}
		case "--table":
			if i+1 < len(args) {
				i++
				cfg.Game.TableID = args[i]
			}
		case "--peer":
			if i+1 < len(args) {
				i++
				cfg.Network.BootstrapPeers = []string{args[i]}
			}
		}
	}
	return noCrypto
}
```

`--no-crypto` is **not** a `Config` field. It is a boolean returned to `runHost` / `runJoin` and passed into `runP2PMode`. Crypto-vs-debug is a process flag, not a YAML knob. That is why a mixed table is “one process passed the flag, another did not,” not “two different config files.”

Flag parsing is a hand-rolled loop, not `flag` or `cobra`. Unknown flags are silently ignored. `--seats abc` leaves `MaxSeats` unchanged (`Sscanf` fails quietly). Worth knowing when a joiner swears they passed `--seats 3` and the process still waited for six.

---

## 10. Loading config

```14:40:config/loader.go
func Load(path string) (*Config, error) {
	cfg := Default()

	if path == "" {
		path = findConfigFile()
	}
	if path != "" {
		if err := loadJSON(path, cfg); err != nil {
			return nil, fmt.Errorf("Load: %w", err)
		}
	}

	applyEnv(cfg)

	// Expand ~ in DataDir.
	if len(cfg.DataDir) > 0 && cfg.DataDir[0] == '~' {
		home, err := os.UserHomeDir()
		if err == nil {
			cfg.DataDir = filepath.Join(home, cfg.DataDir[1:])
		}
	}

	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("Load: invalid config: %w", err)
	}
	return cfg, nil
}
```

Load order, which is the precedence rule:

1. **Start from `Default()`.** Missing keys in a YAML file keep the defaults. You can write a two-line `config.yaml` with only `player_name` and still get blinds 5/10.
2. **Find a file** if the caller did not pass a path: `config.json`, `config.yaml`, `config.yml` in the cwd, then `~/.poker/config.json` / `~/.poker/config.yaml`.
3. **Unmarshal onto the default struct.** Despite the helper name `loadJSON`, `.yaml` / `.yml` go through `yaml.Unmarshal`; anything else through `json.Unmarshal`.
4. **Environment overrides** (`POKER_PLAYER_NAME`, `POKER_DATA_DIR`, `POKER_LISTEN_ADDR`, `POKER_TABLE_ID`, plus chain vars). Env wins over the file.
5. **Expand `~` in `DataDir` only.** `listen_addr` is not tilde-expanded (it should not need to be).
6. **`Validate()`.**

`LoadOrDefault` is what `main` actually calls: if the path does not exist, you get `Default()` rather than a hard error. A *malformed* file still errors. “I deleted config.yaml and it still ran” is this function, not magic.

```42:55:config/loader.go
func LoadOrDefault(path string) (*Config, error) {
	cfg, err := Load(path)
	if err != nil {
		if os.IsNotExist(err) {
			cfg = Default()
			if verr := cfg.Validate(); verr != nil {
				return nil, verr
			}
			return cfg, nil
		}
		return nil, err
	}
	return cfg, nil
}
```

`run()` (local mode) looks for `--config` / `-c` on `os.Args` before calling `LoadOrDefault`. `runHost` / `runJoin` always pass `""`, so they only see the default search path plus flags. There is no `poker host -c custom.yaml`. Flags are the P2P overlay.

`Save` marshals **JSON**, not YAML. `init` writes YAML via `DefaultYAML()`. Two serialization paths. Local play does not need `Save`.

---

## 11. Identity: two keys, two jobs

This is the Phase 1 fact people mix up most often.

### 11.1 Libp2p identity (`identity.key`) — required to play P2P

```141:162:config/config.go
func (c *Config) IdentityKeyPath() string {
	return filepath.Join(c.DataDir, "identity.key")
}

func (c *Config) LoadIdentityKey() ([]byte, error) {
	path := c.IdentityKeyPath()
	b, err := os.ReadFile(path)
	if err == nil && len(b) == 64 {
		return b, nil
	}
	seed := make([]byte, 64)
	if _, err := rand.Read(seed); err != nil {
		return nil, fmt.Errorf("")
	}
	if err := c.EnsureDataDir(); err != nil {
		return nil, err
	}
	if err := os.WriteFile(path, seed, 0o600); err != nil {
		return nil, fmt.Errorf("")
	}
	return seed, nil
}
```

- Path: `~/.poker/identity.key` (unless you changed `data_dir`).
- Contents: **64 random bytes**, a seed. Permissions `0600`. Directory `0700`.
- If the file is missing or not exactly 64 bytes, a new seed is generated and written.
- `runP2PMode` calls this **before** constructing the libp2p host. Local mode does **not** call it; bots do not need a Peer ID.

From that seed, later code (Phase 3, `internal/network/host.go`) derives:

1. An **Ed25519** keypair.
2. A **libp2p Peer ID** (the `12D3KooW…` string). That string is the player id inside the lobby and the game.
3. The key used to **sign gossip envelopes**.

If you copy `identity.key` to another laptop, you *are* the same peer. If you delete it, you are a new person as far as the table is concerned.

Local mode invents a player id `"you"` instead. No file involved:

```83:88:cmd/poker/main.go
func runLocalMode(ctx context.Context, cfg *config.Config) error {
	const humanPlayerID = "you"
	rng := rand.New(rand.NewSource(time.Now().UnixNano()))

	players := []*game.Player{
		game.NewPlayer(humanPlayerID, cfg.PlayerName, cfg.Game.BuyIn),
```

### 11.2 Ethereum key (`poker keygen`) — not required to play

```1316:1324:cmd/poker/main.go
func runKeygen() {
	_, hexKey, err := config.GenerateECDSAKey()
	if err != nil {
		fmt.Fprintf(os.Stderr, "keygen error: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("New Ethereum private key:\n  private_key_hex: \"%s\"\n\n", hexKey)
	fmt.Println("Add this to config.yaml under 'chain:'. KEEP IT SECRET.")
}
```

`GenerateECDSAKey` makes a P-256 key and prints `D` as hex. The README calls this an Ethereum key for a future chain mode. **Live play does not use it.** Running `keygen` does not create `identity.key`. Putting the hex in `config.yaml` does not enable payouts, because `chain.enabled` is false and `main` never constructs a chain client.

Two keys, two jobs:

| Artifact | Algorithm | When used |
|---|---|---|
| `~/.poker/identity.key` | Ed25519 seed (via libp2p) | Every `host` / `join` |
| `chain.private_key_hex` | ECDSA P-256 hex from `keygen` | Never, in the live loop |

---

## 12. Process entry: `cmd/poker/main.go`

`cmd/poker/main.go` is long (~1400 lines) because it is the **composition root**: CLI dispatch, local Bubble Tea loop, *and* the full P2P loop. Phase 1 reads `main`, `run`, `runLocalMode`, `runInit`, `runKeygen`, `printHelp`, and `requireP2PSeats` in detail. It skims `runHost` / `runJoin` / `runP2PMode` as a **map** of later phases, not as a protocol spec.

### 12.1 `main` is a switch on `os.Args[1]`

```24:58:cmd/poker/main.go
func main() {
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "init":
			runInit()
			return
		case "keygen":
			runKeygen()
			return
		case "version":
			fmt.Println("p2p-poker v0.7.0 (phase 7 — integration)")
			return
		case "help", "--help", "-h":
			printHelp()
			return
		case "host":
			if err := runHost(os.Args[2:]); err != nil {
				fmt.Fprintf(os.Stderr, "host error: %v\n", err)
				os.Exit(1)
			}
			return
		case "join":
			if err := runJoin(os.Args[2:]); err != nil {
				fmt.Fprintf(os.Stderr, "join error: %v\n", err)
				os.Exit(1)
			}
			return
		}
	}

	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}
```

If the first argument is not a known subcommand, execution **falls through** to `run()` (local mode). So `./poker foo` does not print “unknown command”; it tries to start a local game (and may treat `foo` as an unused arg). `./poker --seats 3` likewise starts local mode — `--seats` is only parsed inside `applyP2PFlags`, which local mode never calls.

Known subcommands:

| Arg | Function | Side effects |
|---|---|---|
| `init` | `runInit` | Writes `config.yaml` in cwd if missing |
| `keygen` | `runKeygen` | Prints ECDSA hex; does not write a file |
| `version` | inline | Prints version |
| `help` / `-h` / `--help` | `printHelp` | Usage text |
| `host` | `runHost` → `runP2PMode` | Listens, prints multiaddrs, waits for seats |
| `join` | `runJoin` → `runP2PMode` | Same loop; bootstrap peer from `--peer` |
| *(none)* | `run` → `runLocalMode` | TUI vs bots |

`init` refuses to overwrite:

```1303:1314:cmd/poker/main.go
func runInit() {
	path := "config.yaml"
	if _, err := os.Stat(path); err == nil {
		fmt.Println("config.yaml already exists. Remove it first to reinitialise.")
		return
	}
	if err := os.WriteFile(path, []byte(config.DefaultYAML()), 0o644); err != nil {
		fmt.Fprintf(os.Stderr, "error writing config.yaml: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("✓ config.yaml created. Edit it then run: poker")
}
```

### 12.2 Local entry: `run`

```60:77:cmd/poker/main.go
func run() error {
	configPath := ""
	for i, arg := range os.Args[1:] {
		if arg == "--config" || arg == "-c" {
			if i+2 < len(os.Args) {
				configPath = os.Args[i+2]
			}
		}
	}
	cfg, err := config.LoadOrDefault(configPath)
	if err != nil {
		return fmt.Errorf("config: %w", err)
	}
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()
	return runLocalMode(ctx, cfg)
}
```

Ctrl-C / SIGTERM cancel `ctx`. Bubble Tea is started with `tea.WithContext(ctx)`, so a signal tears down the TUI rather than leaving a hung alt-screen.

### 12.3 P2P entry: `runHost` and `runJoin` are almost the same

```269:291:cmd/poker/main.go
func runHost(args []string) error {
	cfg, err := config.LoadOrDefault("")
	if err != nil {
		return err
	}
	noCrypto := applyP2PFlags(cfg, args)
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()
	return runP2PMode(ctx, cfg, noCrypto)
}

func runJoin(args []string) error {
	cfg, err := config.LoadOrDefault("")
	if err != nil {
		return err
	}
	noCrypto := applyP2PFlags(cfg, args)
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()
	return runP2PMode(ctx, cfg, noCrypto)
}
```

There is no `isHost bool` passed into `runP2PMode`. Host vs join differs only in **flags**: a joiner typically passes `--peer`; a host typically passes `--seats`. After `Node.Start()`, both are publishers and subscribers. That is the code-level version of “host is not a server.”

---

## 13. Call graph from `main`

Phase 1’s graph, with later phases greyed in words rather than in boxes:

```
main
├── init        → runInit → os.WriteFile(config.yaml, DefaultYAML())
├── keygen      → runKeygen → config.GenerateECDSAKey()
├── version / help
├── host        → runHost ─┐
├── join        → runJoin ─┴→ applyP2PFlags → runP2PMode   ← Phase 3–5
│                              ├── requireP2PSeats (≥ 3)
│                              ├── LoadIdentityKey
│                              ├── network.NewNode … Start     ← Phase 3
│                              ├── lobby wait / ready
│                              ├── crypto shuffle + peels      ← Phase 4
│                              │     or shared-seed StartHand
│                              ├── fault.FaultManager.Run      ← Phase 5
│                              └── tea.NewProgram(p2pGameModel)
└── (default)   → run → LoadOrDefault → runLocalMode          ← this chapter
                       ├── game.NewPlayer / NewGameState / NewMachine
                       ├── tui.NewModel(onAction → ApplyAction)
                       └── tea.NewProgram(localGameModel)
                            ├── Init → machine.StartHand()
                            ├── human key → ui.OnAction → ApplyAction
                            ├── bot tick → check/call → ApplyAction
                            └── PhaseSettled → nextHandCmd → new Machine
```

Two facts the graph is trying to burn in:

1. **Actions leave the TUI via a callback.** `internal/tui` does not import a way to mutate `Machine` except the `OnAction` func `main` injected. The TUI draws `GameState`; it does not contain the rules.
2. **`runP2PMode` is the rest of the course.** You may read its comment block (the five “bug fixes”) as a preview. You should not yet trace `dealCryptoHand` or `actionSequencer` as if you were going to edit them.

The comment at the top of `runP2PMode` is the best one-page map of later phases that exists in source:

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

Point 2 describes the **`--no-crypto`** path (shared seed). The default crypto path does **not** shuffle a local deck from that seed; it jointly encrypts. The comment is slightly historical. Phase 4 will correct the instinct. For Phase 1, remember: **that comment is why `runP2PMode` is long, not how you shuffle in production.**

---

## 14. Worked example: one local hand

This is the Phase 1 lab, narrated against the code. Sit with `cmd/poker/main.go` open at `runLocalMode` and play along.

### 14.1 Construction

You type `./poker`. `main` does not match a subcommand, so `run()` loads config and calls `runLocalMode`.

Players are built in one process. Seat 0 is you. Seats 1…`MaxSeats-1` are bots with canned names:

```87:99:cmd/poker/main.go
	players := []*game.Player{
		game.NewPlayer(humanPlayerID, cfg.PlayerName, cfg.Game.BuyIn),
	}
	botNames := []string{"Alice (bot)", "Bob (bot)", "Carol (bot)", "Dave (bot)", "Eve (bot)"}
	for i := 1; i < cfg.Game.MaxSeats; i++ {
		id := fmt.Sprintf("bot-%d", i)
		players = append(players, game.NewPlayer(id, botNames[(i-1)%len(botNames)], cfg.Game.BuyIn))
	}

	dealerIdx := 0
	handNum := 1
	gs := game.NewGameState(cfg.Game.TableID, handNum, players, dealerIdx, cfg.Game.SmallBlind, cfg.Game.BigBlind)
	m := game.NewMachine(gs, rng)
```

`NewGameState` (you will read this properly in Phase 2) starts in `PhaseWaiting` and **allocates a real 52-card deck**:

```84:101:internal/game/state.go
func NewGameState(tableID string, handNum int, players []*Player, dealerIdx int, sb, bb int64) *GameState {
	gs := &GameState{
		TableID: tableID,
		HandNum: handNum,
		SmallBlind: sb,
		BigBlind: bb,
		Players: players,
		DealerIdx: dealerIdx,
		Phase: PhaseWaiting,
		Deck: NewDeck(),
		Payouts: make(map[string]int64),
		MinRaise: bb,
	}
	for _, p := range players {
		p.ResetForNewHand()
	}
	return gs
}
```

`NewMachine` stores that state plus the RNG. Local mode’s RNG is `time.Now().UnixNano()` — **not** shared, **not** reproducible. That is fine: there is only one process, so there is nobody to disagree with.

A `FaultManager` is constructed and then discarded (`_ = fm`). Local mode does not run heartbeats. The construction is leftover wiring; do not look for timeout-folds against bots.

### 14.2 The TUI does not own the rules

```112:128:cmd/poker/main.go
	var gameModel *localGameModel
	ui := tui.NewModel(humanPlayerID, func(a game.Action) {
		if gameModel != nil {
			gameModel.applyHumanAction(a)
		}
	})
	ui.LobbyStatus = fmt.Sprintf("Local game — %d players — %s/%s blinds",
		cfg.Game.MaxSeats, formatChips(cfg.Game.SmallBlind), formatChips(cfg.Game.BigBlind))

	gameModel = &localGameModel{
		ui: ui, gs: gs, machine: m, players: players,
		dealerIdx: dealerIdx, handNum: handNum, rng: rng, cfg: cfg,
	}

	p := tea.NewProgram(gameModel, tea.WithAltScreen(), tea.WithContext(ctx))
	_, err := p.Run()
	return err
```

`tui.NewModel` takes `(localPlayerID, onAction)`. When you confirm Fold/Check/Raise in the widget, the TUI calls `onAction`. It does not call `machine.ApplyAction` itself. The closure above forwards to:

```194:194:cmd/poker/main.go
func (gm *localGameModel) applyHumanAction(a game.Action) { _ = gm.machine.ApplyAction(a) }
```

Errors from `ApplyAction` are ignored (`_ =`). An illegal click is a no-op. Phase 2 will show what “illegal” means. For now: **the engine is the only mutator.**

`localGameModel` wraps the TUI model so Bubble Tea’s `Init` / `Update` / `View` can also drive bots and the next hand. `View` is a one-liner: `return gm.ui.View()`. Drawing is 100% `internal/tui`.

### 14.3 Hand start

Bubble Tea calls `Init` once:

```142:152:cmd/poker/main.go
func (gm *localGameModel) Init() tea.Cmd {
	return tea.Batch(
		tea.EnterAltScreen,
		func() tea.Msg {
			if err := gm.machine.StartHand(); err != nil {
				return tui.ErrorMsg{Text: err.Error()}
			}
			return tui.GameStateMsg{State: gm.gs}
		},
	)
}
```

`StartHand` (Phase 2 in full) refuses to run without a deck:

```20:32:internal/game/machine.go
func (m *Machine) StartHand() error {
	if m.State.Phase != PhaseWaiting {
		return fmt.Errorf("StartHand: expected PhaseWaiting, got %s", m.State.Phase)
	}
	if len(m.State.Players) < 2 {
		return fmt.Errorf("StartHand: need at least 2 players")
	}
	if m.State.Deck == nil {
		return fmt.Errorf("StartHand: no deck; use StartHandCrypto")
	}

	m.State.Deck.Shuffle(m.rng)
```

That `Deck == nil` branch is the crypto path, which local mode never takes. Local mode Fisher–Yates-shuffles, posts blinds, deals two hole cards per seat from the **same** deck the UI will later display. Every bot hole card is sitting in `Player.HoleCards` in this process. The TUI in local mode is allowed to show them (Phase 2: `player_panel.go` hides opponent holes unless local or winner).

`StartHand` returns, `Init` emits `tui.GameStateMsg{State: gm.gs}`, and `Update` pushes that into the TUI.

### 14.4 Your turn, then the bots

`Update` on `GameStateMsg`:

```154:169:cmd/poker/main.go
	case tui.GameStateMsg:
		gm.gs = msg.State
		newUI, _ := gm.ui.Update(msg)
		gm.ui = newUI.(tui.Model)
		if gm.gs.Phase == game.PhaseSettled {
			winnerIDs, handRanks := buildWinnerInfo(gm.gs)
			newUI2, _ := gm.ui.Update(tui.WinnerMsg{WinnerIDs: winnerIDs, HandRanks: handRanks, Payouts: gm.gs.Payouts})
			gm.ui = newUI2.(tui.Model)
			return gm, gm.nextHandCmd()
		}
		if cur := gm.gs.CurrentPlayer(); cur != nil && cur.ID != "you" {
			return gm, gm.botActionCmd()
		}
		return gm, nil
```

Three branches:

1. **Hand over** (`PhaseSettled`) → show winners → schedule next hand in 1.5 s.
2. **Someone else’s turn** → schedule a bot action in 600 ms.
3. **Your turn** → no command; the TUI widget is live.

Bot policy is deliberately dumb: check if the bet is matched, else call. Bots never fold, never raise, never all-in on their own:

```196:210:cmd/poker/main.go
func (gm *localGameModel) botActionCmd() tea.Cmd {
	return tea.Tick(600*time.Millisecond, func(_ time.Time) tea.Msg {
		cur := gm.gs.CurrentPlayer()
		if cur == nil || cur.ID == "you" {
			return nil
		}
		toCall := gm.gs.CurrentBet - cur.CurrentBet
		a := game.Action{PlayerID: cur.ID, Type: game.ActionCheck}
		if toCall > 0 {
			a.Type = game.ActionCall
		}
		gm.machine.ApplyAction(a)
		return tui.GameStateMsg{State: gm.gs}
	})
}
```

Walk a pre-flop with default 6 seats, dealer index 0 (you):

1. `StartHand` posts blinds and deals. First to act is some bot (UTG), not you.
2. `Update` sees `CurrentPlayer().ID != "you"` → 600 ms → bot calls or checks.
3. That emits another `GameStateMsg` → next bot → … until it is `"you"`.
4. You press `c`. TUI builds `game.Action{PlayerID: "you", Type: ActionCall}` (or Check) and calls `OnAction` → `ApplyAction`.
5. The TUI then typically re-emits state (via key handling / spectate). The next `GameStateMsg` schedules the next bot.

You never sent a network message. There is no sequence number. `ApplyAction` ran in the TUI goroutine. That is the entire “distributed systems” content of local mode: **none.** Which is the point.

### 14.5 Next hand

```212:227:cmd/poker/main.go
func (gm *localGameModel) nextHandCmd() tea.Cmd {
	return tea.Tick(1500*time.Millisecond, func(_ time.Time) tea.Msg {
		gm.handNum++
		gm.dealerIdx = (gm.dealerIdx + 1) % len(gm.players)
		for _, p := range gm.players {
			p.ResetForNewHand()
		}
		gm.gs = game.NewGameState(gm.cfg.Game.TableID, gm.handNum, gm.players,
			gm.dealerIdx, gm.cfg.Game.SmallBlind, gm.cfg.Game.BigBlind)
		gm.machine = game.NewMachine(gm.gs, gm.rng)
		if err := gm.machine.StartHand(); err != nil {
			return tui.ErrorMsg{Text: err.Error()}
		}
		return tui.GameStateMsg{State: gm.gs}
	})
}
```

Stacks persist (`ResetForNewHand` clears holes and bets, not `Stack`). The dealer button rotates. A **new** `GameState` and **new** `Machine` are allocated. Same pattern P2P will use, without the mutex and pointer indirection, because no network goroutine is racing you.

`buildWinnerInfo` (shared with P2P) computes who got a payout and, if five community cards exist, a rank string for the TUI. Local mode can always evaluate: every hole card is present.

### 14.6 What you should have observed

After one local hand you have seen:

- Config → seats, blinds, buy-in, your display name.
- A pure engine call (`StartHand`, `ApplyAction`) with a real deck.
- A TUI that only submits actions through a callback.
- Bots as timers, not peers.
- Chips moving as **int64 counters** on `Player.Stack`. Nothing hit Ethereum.

If that loop is clear, Phase 2 (reading `machine.go` as a reducer) will feel like zooming in, not starting over.

---

## 15. The shape of P2P mode (preview only)

Read this once so `runP2PMode` is not a 900-line cliff. Do not deep-dive the helpers.

`runHost` / `runJoin` both call `runP2PMode(ctx, cfg, noCrypto)` after flags. The first executable line is the 3-seat gate. Then, in order:

1. **Load `identity.key`.** Derive the libp2p host from it (Phase 3).
2. **Maybe generate an SRA key.** Skipped when `--no-crypto`. The private exponent `d` must never leave this process. Phase 4.
3. **Construct `network.Node` and wire callbacks *before* `Start()`.** If a join arrives in the gap, a nil handler would drop it. The comment in §13 is about this.
4. **Print Peer ID and multiaddrs. Broadcast join. Poll until `Lobby.Count() >= MaxSeats`. Broadcast ready. Sleep 2 s.** That sleep is a LAN fudge so ready messages propagate; it is not consensus.
5. **Build `game.Player`s from lobby seats** (canonical order). Same `NewPlayer` / `NewGameState` types as local mode.
6. **Start `FaultManager` + heartbeats** before shuffle (Phase 5), so a hang during shuffle can still abort.
7. **Either** `--no-crypto`: shared seed from `Lobby.SessionNonce()`, `NewMachine(gs, rng)`, later `StartHand` in the TUI `Init` **or** default crypto: `KeyringFromLobby` → unicast Shamir shares → `dealCryptoHand` (shuffle, hole peels, `StartHandCrypto` with only *local* holes filled).
8. **Swap `liveMachine` / `liveGS` under `machineMu`.** Network callbacks and the TUI share those pointers.
9. **TUI `OnAction` becomes `applyAndBroadcast`:** apply locally first, then gossip the action with a table-wide seq. GossipSub will echo your own message; the receive path drops self. Apply-local-first is why your UI does not wait for a round trip to yourself.

You do not need to memorize that list. You need to recognize that **P2P is local mode plus: identity, mesh, ordered actions, optional crypto, faults.** Each “plus” is a later phase.

Hands-on for this preview: one `poker host --seats 3 --name Alice` until you see a multiaddr, then Ctrl-C. That is enough.

---

## 16. Tests in this phase

[`cmd/poker/main_test.go`](./cmd/poker/main_test.go) is small. It covers the seat gate, not the TUI:

```8:21:cmd/poker/main_test.go
func TestRequireP2PSeats_RejectsTwo(t *testing.T) {
	err := requireP2PSeats(2)
	if err == nil {
		t.Fatal("expected error for 2 seats")
	}
	if !strings.Contains(err.Error(), "3") {
		t.Fatalf("error %q should mention 3", err.Error())
	}
}

func TestRequireP2PSeats_AcceptsThree(t *testing.T) {
	if err := requireP2PSeats(3); err != nil {
		t.Fatalf("3 seats should be accepted: %v", err)
	}
}
```

[`READ_GUIDE.md`](./READ_GUIDE.md) calls this “CLI and mode-flag coverage.” In the current tree it is **only** the 3-seat invariant. There is no test that `./poker` without args enters local mode, no test for `applyP2PFlags`, no test for `init` refusing to overwrite. If you add those, this is the file. If you change `minP2PSeats`, this is the test that should fail.

There is no `config/*_test.go`. `Validate` and `Load` are untested at unit level. That is a gap, not a mystery.

Run from repo root (full suite is slow because of 2048-bit crypto tests; you do not need that yet):

```bash
go test ./cmd/poker -count=1
```

---

## 17. Common mistakes

These are the mistakes people make **in the first week**, including while editing `config/` and `cmd/poker`.

1. **Treating `poker host` as a game server.** After join, Alice publishes and subscribes like everyone else. Seat 0 shuffles first because of canonical seat order, not because the CLI said `host`. Do not add “if isHost { deal cards }” in `main`.

2. **Putting sockets in `internal/game`.** The engine must stay a reducer. If you need a card in crypto mode, it arrives as an input (`ApplyStreet`, `ApplyHoleReveal`) in Phase 4. Local mode is allowed to have a `Deck` precisely because there is one process.

3. **Collapsing the two seat validators.** `Validate` allows 2–9 (local heads-up). `requireP2PSeats` requires 3+ (Shamir after a drop). “Just make Validate require 3” breaks local heads-up. “Just allow `--seats 2` on host” breaks disconnect recovery.

4. **Confusing `identity.key` with `poker keygen`.** The first is how the mesh knows who you are. The second is an unused Ethereum hex string. Deleting `identity.key` makes you a new Peer ID. Running `keygen` does not.

5. **Expecting chips to be ETH.** `BuyIn` is `int64` chips. `chain.enabled` is false. `main` never imports `internal/chain`. Demonstrating the TUI is not demonstrating a payout.

6. **Using `host:port` as `--listen`.** The field is a multiaddr: `/ip4/0.0.0.0/tcp/9000`. Config validation will not catch the typo.

7. **Two processes, one port.** Default listen is TCP 9000. On one machine, joiners need `--listen /ip4/0.0.0.0/tcp/9001` (and 9002, …). Local mode does not bind 9000, so `./poker` plus `./poker host` is fine.

8. **A mixed crypto table.** One peer `--no-crypto`, others default, exits with `crypto dealing requires every seat to publish e`. All crypto or all debug.

9. **Reading `ISSUES_AND_RECOMMENDATIONS.md` as current status.** It is a historical gap analysis. Prefer README + this chapter + `HOW_IT_WORKS.md` for what runs today.

10. **Starting a feature in `internal/crypto` this week.** Phase 1’s exit check does not include SRA. The suggested calendar in the read guide is: afternoon = Phase 1 + 2; do not skip ahead.

11. **Assuming `Load`’s helper `loadJSON` means YAML is ignored.** `.yaml` is the path `init` writes. The helper name is stale.

12. **Falling through `main`’s switch accidentally.** Unknown first args start local mode. `poker host` must be the literal subcommand `host`.

---

## 18. Exit check

You can explain, **without notes**:

1. **Host is not a game server.** It is the first listener / bootstrap peer. After the lobby fills, every seated process is a replica.
2. **P2P needs 3–9 seats.** Shamir recovery after one disconnect needs leftover shares. `requireP2PSeats` enforces this. Local vs bots allows 2–9 via `Validate`.
3. **Local mode has no SRA.** One process, one `Deck`, Fisher–Yates, bots as 600 ms timers. You can see every hole card. That is expected.
4. **Chips in the demo are local counters.** `Player.Stack` is `int64`. Solidity escrow exists; the Go client is not on the `host`/`join` path.
5. **Identity lives in `~/.poker/identity.key`.** 64-byte seed, Ed25519 / libp2p Peer ID. `poker keygen` is a different key for a chain mode that is not live.

You have **run** `go build -o poker ./cmd/poker` and `./poker` (or `.\poker.exe`) through at least one local hand.

You have **not** yet explained `ApplyAction` case by case, GossipSub topics, or a peel. That is Phases 2–4.

When the five bullets are true, open [`PHASE_2.md`](./PHASE_2.md), starting at `internal/game/deck.go`.

---

## 19. Phase 1 glossary

A subset of [`HOW_IT_WORKS.md`](./HOW_IT_WORKS.md) §26, limited to words this chapter used. Full glossary lives there.

| Term | Meaning in this project |
|---|---|
| **LAN** | Local network. Machines can usually dial each other by private IP |
| **IP address** | Number identifying a machine on a network |
| **Port** | Number identifying a program on that machine (default 9000) |
| **TCP** | Reliable, ordered byte pipe. No encryption by itself |
| **Peer** | One running `poker` process. Both sender and receiver |
| **Peer ID** | libp2p identity derived from an Ed25519 public key (`12D3KooW…`) |
| **Multiaddr** | Self-describing address: IP + TCP port + peer id |
| **mDNS** | LAN multicast discovery (`p2p-poker-v1`) |
| **Host (CLI)** | First listener / bootstrap peer. **Not** a game server |
| **Replica** | Every peer runs the same state machine on the same log |
| **SRA** | Commutative encryption used as a lock on cards (Phase 4) |
| **`--no-crypto`** | Debug: public shared seed, all cards visible |
| **Mental poker** | Cryptographic dealing among mutually distrusting players |
| **Escrow** | On-chain pot of ETH; not wired into live play |
| **Bubble Tea** | TUI library. `Init` / `Update` / `View`. Actions leave via callback |

---

## Companion reading (this phase only)

| File | Why |
|---|---|
| [`README.md`](./README.md) | Commands, seats, crypto vs `--no-crypto`, known limits |
| [`HOW_IT_WORKS.md`](./HOW_IT_WORKS.md) §§1–9, 23, 25–26 | Vocabulary, what “decentralized” means, repo map, local mode, glossary |
| [`SYSTEM_DESIGN_OVERVIEW.md`](./SYSTEM_DESIGN_OVERVIEW.md) §§1–2 | Goals, non-goals, mesh diagram, how a table forms |
| [`go.mod`](./go.mod) | Go 1.25, libp2p, GossipSub, protobuf |
| [`config/config.go`](./config/config.go) | YAML shape; identity seed vs Ethereum keygen |
| [`config/loader.go`](./config/loader.go) | Load / default / validate / env |
| [`cmd/poker/main.go`](./cmd/poker/main.go) | `main`, `runLocalMode`; shape of `runP2PMode` |
| [`cmd/poker/main_test.go`](./cmd/poker/main_test.go) | 3-seat gate |

Next: Phase 2 — the pure Hold’em engine and the local TUI, still with zero sockets in the rules.
