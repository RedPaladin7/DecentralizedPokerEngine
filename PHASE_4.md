# Phase 4 — Mental poker (hidden cards, no dealer)

This is the fourth onboarding chapter. After it you should be able to **walk a hole-card deal: who peels on the wire, who peels last locally, why opponent holes stay empty, and why a mixed table (one peer `--no-crypto`) must exit.** You still do not need timeout-folds, escrow, or the integration tests.

The reading list this chapter expands is in [`READ_GUIDE.md`](./READ_GUIDE.md). The teaching narrative it sits next to is [`HOW_IT_WORKS.md`](./HOW_IT_WORKS.md) §§16 and 20–21. Phase 2 taught you that crypto-mode cards are *inputs*. Phase 3 taught you how unordered gossip becomes a total order of `PLAYER_ACTION`s. This chapter is how those card inputs are produced without a dealer.

**You are here to learn:** commutative encryption lets everyone lock the deck; selective peels let only the rightful player see a hole card; ZK proofs stop garbage decrypts; `CryptoHand` is the live replica protocol, while `CryptoGame` is an in-process oracle with every key.

**Do this with your hands before you finish the chapter:** three-player **crypto** table (default, no `--no-crypto`). Expect several seconds of `Shuffling…`. Confirm your TUI does not show opponents’ holes. Then run:

```bash
go test ./internal/crypto ./internal/network -count=1
```

Optional: one debug table with `--no-crypto` on **every** peer to contrast a public deck.

**Do not deep-read yet:** `internal/fault` policy (heartbeats, 2/3 votes, reconstruction after silence), `contracts/`, `internal/chain`. Know that Shamir *math* lives in `commit.go` and that shares are unicasted before the shuffle; *when* a reconstructed `d` is used to finish peels is Phase 5.

**Architectural rule to keep in your head the whole time:** `internal/game` never imports `internal/network`. `internal/crypto` never imports GossipSub. Networking produces authenticated, ordered *card inputs*. The engine reduces them. Mixing those layers is how “the host accidentally becomes the dealer” happens.

---

## Table of contents

1. [How to use this chapter](#1-how-to-use-this-chapter)
2. [The one idea: commutative encryption](#2-the-one-idea-commutative-encryption)
3. [Four objects you must never confuse](#3-four-objects-you-must-never-confuse)
4. [Package map](#4-package-map)
5. [Shared prime and card encoding](#5-shared-prime-and-card-encoding)
6. [SRA keys: public `e`, private `d`](#6-sra-keys-public-e-private-d)
7. [Shuffle commitments (and Shamir math)](#7-shuffle-commitments-and-shamir-math)
8. [Zero-knowledge proofs on peels](#8-zero-knowledge-proofs-on-peels)
9. [The Keyring invariant](#9-the-keyring-invariant)
10. [Library shuffle vs `ShuffleSession`](#10-library-shuffle-vs-shufflesession)
11. [Library peels vs `DealSession`](#11-library-peels-vs-dealsession)
12. [Deck indexes: the contract with the engine](#12-deck-indexes-the-contract-with-the-engine)
13. [`CryptoGame`: the in-process oracle](#13-cryptogame-the-in-process-oracle)
14. [`CryptoHand`: the live replica protocol](#14-cryptohand-the-live-replica-protocol)
15. [Call graph from `runP2PMode`](#15-call-graph-from-runp2pmode)
16. [Worked example: the joint shuffle](#16-worked-example-the-joint-shuffle)
17. [Worked example: Bob’s first hole card](#17-worked-example-bobs-first-hole-card)
18. [Worked example: the flop appears together](#18-worked-example-the-flop-appears-together)
19. [Showdown reveals](#19-showdown-reveals)
20. [`--no-crypto` and mixed tables](#20---no-crypto-and-mixed-tables)
21. [`HandCoordinator`: the trap](#21-handcoordinator-the-trap)
22. [Tests in this phase](#22-tests-in-this-phase)
23. [Historical plans (read last)](#23-historical-plans-read-last)
24. [Common mistakes](#24-common-mistakes)
25. [Exit check](#25-exit-check)
26. [Phase 4 glossary](#26-phase-4-glossary)

---

## 1. How to use this chapter

Read top to bottom once. When a code excerpt appears, open that file in the editor and match the excerpt to the live source. Line numbers here were accurate when this chapter was written; if they drift, trust the file.

This chapter is **not** a rewrite of [`HOW_IT_WORKS.md`](./HOW_IT_WORKS.md) §16 or §21. Those sections are the short math and the four-player story. This file is the types, the call graph, three worked hops, and the mistakes people make the week they first edit `keyring.go` or `crypto_hand.go`.

Suggested time: one sitting after Phase 3, including a three-process **crypto** table. Stop when the [exit check](#25-exit-check) is true.

File order matches the read guide. Do not skip to `crypto_hand.go` before `params.go` / `sra.go` / `keyring.go` make sense — `CryptoHand` is a composition of those types, not a second crypto engine.

Two splits keep coming back. Learn them now:

| Split | Library (one process, all keys) | Protocol (one replica, own `d` only) |
|---|---|---|
| Shuffle | `shuffle.go` `ExecuteStep` / `RunFullShuffle` | `shuffle_session.go` `ShuffleSession` |
| Deal | `deal.go` `RevealToPlayer` / `RevealCommunity` | `deal_session.go` `DealSession` |
| Whole hand | `crypto_game.go` `CryptoGame` | `network/crypto_hand.go` `CryptoHand` |

If you grep `RunFullShuffle` and think that is how `poker host` works, you are looking at the simulator.

---

## 2. The one idea: commutative encryption

A classic online poker site is a **trusted dealer**. It shuffles a plaintext deck, deals Alice two cards only she sees, and later burns and turns community cards. You have to trust that operator: that they shuffled fairly, that they did not peek at hole cards, that they will not rewrite the flop.

This repository’s multiplayer path has **no dealer process**. Every seated program is a replica of the same Hold’em engine. Cards cannot come from `Deck.Deal()` on one laptop — that laptop would know the order. They also cannot come from a shared RNG seed — every replica could print every hole card (that is the `--no-crypto` debug path).

Mental poker here is **Shamir–Rivest–Adleman commutative encryption**, abbreviated **SRA**. If you remember one math fact, remember commutativity:

```
((m ^ e_A) ^ e_B)  ≡  ((m ^ e_B) ^ e_A)   (mod p)
```

Encrypting with Alice then Bob is the same ciphertext as Bob then Alice. Therefore the group can lock the whole deck under **all** public exponents, permute in between, and later peel layers in **any order**. Nobody needs a distinguished dealer who saw the plaintext order.

[`HOW_IT_WORKS.md`](./HOW_IT_WORKS.md) §16 puts the protocol in three sentences:

1. Everyone encrypts-then-permutes the same 52 field elements, in seat order. After *n* steps the deck is locked under every `e` and shuffled by *n* secret permutations.
2. To show Bob a hole card, everyone **except Bob** publishes a partial decrypt of that index. Bob peels the last layer **locally** and does not publish the rank.
3. Community cards and showdown holes are public peels: everyone peels; plaintext appears on every replica together.

The engine from Phase 2 never samples those cards. `StartHandCrypto` posts blinds with `Deck == nil`. Later `ApplyStreet` / `ApplyHoleReveal` consume peeled values.

```451:476:internal/game/machine.go
func (m *Machine) StartHandCrypto() error {
	gs := m.State
	if gs.Phase != PhaseWaiting {
		return fmt.Errorf("StartHandCrypto: expected PhaseWaiting, got %s", gs.Phase)
	}
	// ...
	// Cards are inputs in crypto mode. Callers should fill local holes first;
	// opponent holes stay empty until ApplyHoleReveal at showdown.
	gs.Deck = nil

	if err := m.postBlinds(); err != nil {
		return err
	}
	// ...
	gs.Phase = PhasePreFlop
	return nil
}
```

That is the contract this chapter exists to honour: **produce local holes and later streets as inputs, without ever putting opponent ranks into this replica’s `GameState` until a public reveal.**

---

## 3. Four objects you must never confuse

New joiners mix these up because the names sound similar and the oracle tests pass first.

| Object | Where | Who has whose `d` | Used by live `host`/`join`? |
|---|---|---|---|
| `ShuffleProtocol.ExecuteStep` | `shuffle.go` | Caller must pass a **private** key | Indirectly, via `ShuffleSession` |
| `ShuffleSession` | `shuffle_session.go` | Local `d` only (`Keyring.LocalKey()`) | Yes, inside `CryptoHand` |
| `CryptoGame` | `crypto_game.go` | **Every** `(e, d)` on one machine | **No.** Tests / `HandCoordinator` |
| `CryptoHand` | `network/crypto_hand.go` | Local `d` only; reconstructed gone keys sit in a side map | **Yes.** `dealCryptoHand` |

`DealProtocol` vs `DealSession` is the same split for peels.

`HandCoordinator.RunHand` constructs a `CryptoGame`. Phase 3 already warned you not to call it from the live loop. This chapter is why: it generates every key in one process, then fills **all** seats’ holes.

```20:41:internal/crypto/crypto_game.go
func NewCryptoGame(playersIDs []string, nonce []byte) (*CryptoGame, error) {
	// ...
	keys := make([]*SRAKey, len(playersIDs))
	for i, pid := range playersIDs {
		k, err := GenerateSRAKey(p)
		if err != nil {
			return nil, fmt.Errorf("NewCryptoGame: key gen for %s: %w", pid, err)
		}
		keys[i] = k
	}
	// ...
}
```

If you “helpfully” route `poker host` through that, every laptop that ran the coordinator would know every hole card. The product claim would be false.

Keep the oracle. Do not promote it.

---

## 4. Package map

Read these in this order. Tests sit after the code they cover.

| File | Why this file, now |
|---|---|
| `internal/crypto/params.go` | Shared `p`; cards as numbers; session id |
| `internal/crypto/sra.go` | `(e, d)` per peer; public-only `SRAKey` |
| `internal/crypto/commit.go` | Bind a shuffle step without publishing the permutation; Shamir split/reconstruct **math** |
| `internal/crypto/zkp.go` | Proof that a peel used the claimed `d` |
| `internal/crypto/keyring.go` | **Invariant:** no API returns another peer’s `d` |
| `internal/crypto/shuffle.go` | Encrypt-then-permute **in one process** |
| `internal/crypto/shuffle_session.go` | Distributed shuffle FSM (one step per seat over gossip) |
| `internal/crypto/deal.go` | Partial decrypt / peel **in one process**; deck indexes |
| `internal/crypto/deal_session.go` | Distributed peel FSM (holes, streets, showdown) |
| `internal/crypto/crypto_game.go` | Oracle: all keys on one machine — tests only |
| crypto `*_test.go` | Smallest working examples of shuffle + peel |
| `internal/network/crypto_hand.go` (+ test) | Live path: gossip shuffle, gossip+stream peels, gates |
| `cmd/poker/main.go` (`runP2PMode`, `dealCryptoHand`) | Where Keyring, shares, and `CryptoHand` are actually started |
| `plans/phase-1-keyring.md` … `plans/phase-5-wire-p2p.md` | How this was built, **after** you already understand the code |

`internal/crypto` imports `internal/game` only for `game.Card` (ranks after a peel finishes). It does not import `internal/network`. `CryptoHand` sits in `network` because it talks to wait-gates, early-message buffers, and (later) designated-survivor peels — still without putting sockets inside `game`.

---

## 5. Shared prime and card encoding

All players use the same 2048-bit prime `p` from RFC 3526 (the MODP group). It is a public parameter, like “we all agreed to use this deck of 52 physical cards.” It is **not** sent on the wire. An honest node that used a different `p` would produce ciphertexts nobody else could peel.

```22:26:internal/crypto/params.go
func SharedPrime() *big.Int {
	p := new(big.Int)
	p.SetString(p2048Hex, 16)
	return p
}
```

A card id `i` (the same `0..51` Phase 2 taught you) becomes a field element:

```
m_i = 2^(i+1)  mod p
```

```28:47:internal/crypto/params.go
func CardToField(cardID int, p *big.Int) *big.Int {
	if cardID < 0 || cardID > 51 {
		panic(fmt.Sprintf(""))
	}
	g := big.NewInt(2)
	exp := big.NewInt(int64(cardID + 1))
	return new(big.Int).Exp(g, exp, p)
}

func FieldToCard(val *big.Int, p *big.Int) int {
	g := big.NewInt(2)
	for id := 0; id <= 51; id++ {
		exp := big.NewInt(int64(id + 1))
		candidate := new(big.Int).Exp(g, exp, p)
		if candidate.Cmp(val) == 0 {
			return id
		}
	}
	return -1
}
```

That puts plaintext in the multiplicative group so exponentiation is a valid lock. After all layers are peeled, `FieldToCard` brute-forces which of the 52 encodings matches. Fifty-two modular exponentiations is cheap next to the shuffle.

`BuildPlaintextDeck` is the **starting** deck for every shuffle: cards `0..51` in order. Shuffling is the secret permutations, not a Fisher–Yates on plaintext.

```49:55:internal/crypto/params.go
func BuildPlaintextDeck(p *big.Int) []*big.Int {
	deck := make([]*big.Int, 52)
	for i := range deck {
		deck[i] = CardToField(i, p)
	}
	return deck
}
```

Session id binds proofs and peels to *this* table and *this* hand. Live `CryptoHand` mixes lobby nonce with `handNum` so a peel from hand 1 cannot be replayed as a peel from hand 2.

```57:65:internal/crypto/params.go
func SessionID(playerIDs []string, nonce []byte) []byte {
	h := sha256.New()
	for _, id := range playerIDs {
		h.Write([]byte(id))
		h.Write([]byte{0x00})
	}
	h.Write(nonce)
	return h.Sum(nil)
}
```

```48:49:internal/network/crypto_hand.go
	nonce := append(append([]byte{}, lobbyNonce...), byte(handNum>>8), byte(handNum))
	sid := pokercrypto.SessionID(kr.SeatOrder(), nonce)
```

Keys `(e, d)` do **not** rotate between hands. The session id does.

---

## 6. SRA keys: public `e`, private `d`

Each peer generates `(e, d)` with `gcd(e, p-1) = 1` and `d = e⁻¹ mod (p-1)`:

```
Encrypt:  c = m^e  mod p
Decrypt:  m = c^d  mod p
```

`e` is published in `JOIN_TABLE.sra_pub_key_e`. `d` never leaves the node except as Shamir shares (unicast, before shuffle; reconstruction policy is Phase 5).

```16:46:internal/crypto/sra.go
func GenerateSRAKey(p *big.Int) (*SRAKey, error) {
	if p == nil || !p.ProbablyPrime(20) {
		return nil, errors.New("GenerateSRAKey: p must be a prime")
	}
	phi := new(big.Int).Sub(p, big.NewInt(1))
	// ... sample e with gcd(e, phi) == 1 ...
	d := new(big.Int).ModInverse(e, phi)
	return &SRAKey{E: e, D: d, P: p}, nil
}
```

A key that can encrypt but not decrypt is a first-class type, not a missing field you hope nobody reads:

```48:70:internal/crypto/sra.go
func PublicSRAKey(p, e *big.Int) (*SRAKey, error) {
	// ...
	return &SRAKey{
		E: new(big.Int).Set(e),
		D: nil,
		P: new(big.Int).Set(p),
	}, nil
}

func (k *SRAKey) IsPrivate() bool {
	return k != nil && k.D != nil
}
```

`Encrypt` needs `e`. `Decrypt` **refuses** without `d`. That is the whole privacy story at the arithmetic layer:

```80:101:internal/crypto/sra.go
func (k *SRAKey) Encrypt(m *big.Int) (*big.Int, error) {
	if k == nil || k.E == nil || k.P == nil {
		return nil, errors.New("SRAKey.Encrypt: key, e, or p is not present")
	}
	// ...
	return new(big.Int).Exp(m, k.E, k.P), nil
}

func (k *SRAKey) Decrypt(c *big.Int) (*big.Int, error) {
	if k == nil || k.D == nil {
		return nil, errors.New("SRAKey.Decrypt: private exponent d is not present")
	}
	// ...
	return new(big.Int).Exp(c, k.D, k.P), nil
}
```

`EncryptAll` / `DecryptAll` are the 52-card versions. Shuffle uses `EncryptAll`. A peel uses a single `Decrypt`.

The commutativity test is the smallest proof that this math is the right math:

```61:80:internal/crypto/crypto_test.go
func TestSRA_Commutativity(t *testing.T) {
	// Core property: E_A(E_B(x)) == E_B(E_A(x))
	// ...
	eb, _ := keyB.Encrypt(m)
	eaeb, _ := keyA.Encrypt(eb)

	ea, _ := keyA.Encrypt(m)
	ebea, _ := keyB.Encrypt(ea)

	if eaeb.Cmp(ebea) != 0 {
		t.Errorf("commutativity violated: ...")
	}
}
```

`TestSRA_DecryptInOrder` then peels Bob’s layer first, then Alice’s, and still recovers `m`. Peel order on the wire is canonical for *sequencing*, not because the algebra requires it.

`PublicView` copies a key with `D == nil`. Safe to hand to other packages. If you ever return `cloneSRAKey` (full copy) from a “public” API, you have leaked `d` into whoever called you.

---

## 7. Shuffle commitments (and Shamir math)

A shuffle step publishes the **output** deck. Without a commitment, a shuffler could later claim a different output (“that wasn’t the permutation I meant”). With a commitment, the published bytes are bound.

```12:24:internal/crypto/commit.go
type Commitment struct {
	Hash  []byte
	Nonce []byte
}

func NewCommitment(data []byte) (*Commitment, error) {
	nonce := make([]byte, 32)
	if _, err := rand.Read(nonce); err != nil {
		return nil, fmt.Errorf("")
	}
	hash := computeCommitmentHash(data, nonce)
	return &Commitment{Hash: hash, Nonce: nonce}, nil
}
```

The hash is `SHA-256(len ‖ data ‖ nonce)` with a 4-byte big-endian length prefix so the concatenation is not ambiguous. Deck serialisation is 52 × 256-byte big-endian limbs (2048-bit field elements, left-padded). Everyone verifies `VerifyDeck` against the published integers. They do **not** learn the permutation.

**Why a commitment is not a fairness proof.** It binds the published bytes. It does not prove the permutation was random. Fairness comes from “I do not know your permutation, and you encrypted first.” One honest random permutation randomizes the order. Collusion of **all** seated players still breaks this — do not oversell.

The same file holds **Shamir secret sharing** of `d`: `SplitSecret` / `ReconstructSecret`. Threshold `t=2`, `n=` seat count on the live path. The math is here because `d` is a `*big.Int` and the polynomial is over the same prime. **Do not** treat this chapter as the liveness spec. Live unicast of shares happens in `runP2PMode` via `DistributeLocalShares` *before* the shuffle; using reconstructed `d` to `PeelOnBehalf` after a timeout-fold is Phase 5.

What you need now: one share is useless; two shares reconstruct `d`; reconstructing `d` does **not** reconstruct a mid-shuffle permutation. That last sentence is why a disconnect *during* shuffle aborts the hand.

---

## 8. Zero-knowledge proofs on peels

A partial decrypt is a pair of big integers (`ciphertext`, `result`). Without a proof, Dave can publish garbage and freeze the hand, or try to steer a community card.

Each peel carries a Schnorr-style proof of correct exponentiation, Fiat–Shamir hashed with the session id:

```20:51:internal/crypto/zkp.go
func ProveDecryption(key *SRAKey, ciphertext, result *big.Int, sessionID []byte) (*ZKProof, error) {
	// h = g^d
	// A = g^r,  B = ciphertext^r
	// c = SHA256(p, h, ciphertext, result, A, B, sessionID) mod (p-1)
	// s = r + c·d  (mod p-1)
	return &ZKProof{A: A, B: B, S: s, H: h}, nil
}
```

Verifier checks two equalities:

```
g^s           =  h^c · A              (mod p)
ciphertext^s  =  result^c · B         (mod p)
```

```53:78:internal/crypto/zkp.go
func VerifyDecryption(proof *ZKProof, ciphertext, result *big.Int, P *big.Int, sessionID []byte) error {
	c := computeChallenge(P, proof.H, ciphertext, result, proof.A, proof.B, sessionID)
	// g^s == h^c · A
	// ciphertext^s == result^c · B
	// ...
}
```

You do not need to memorize the equations. You need the sentence: **anyone can check that this peel used the claimed `d` without learning `d`.**

A failed proof is a slashable event in the fault package (`SlashBadZKProof`). In the live loop it is logged; it is not yet submitted on-chain. `TestZKProof_WrongResult_Detected` and `TestDeal_MaliciousDecryption_Detected` are the worked cheats.

`PartialDecryption` is the in-process struct. `PeelMessage` is what leaves a `DealSession`. Same numbers; the wire type also carries `HandNum` / `CardIndex`.

---

## 9. The Keyring invariant

`Keyring` is the data structure that makes “I only have my own `d`” hard to get wrong.

```9:16:internal/crypto/keyring.go
// Keyring holds this node's full SRA key and public-only keys for every seat.
// There is no API that returns another peer's private exponent d.
type Keyring struct {
	localID string
	local   *SRAKey            // full key, D != nil
	pubs    map[string]*SRAKey // peerID → public-only (D == nil), includes local
	order   []string           // canonical seat order
}
```

Construction checks:

- local key is private (`D != nil`);
- `localID` is in canonical `seatOrder`;
- every seat has a non-empty `e`;
- the `e` this node published in the lobby **matches** the local key (so you cannot sit down with one `e` and shuffle with another).

```61:64:internal/crypto/keyring.go
	localEFromLobby := new(big.Int).SetBytes(publicE[localID])
	if localEFromLobby.Cmp(local.E) != 0 {
		return nil, errors.New("NewKeyring: local public e does not match local key")
	}
```

The API that matters:

| Method | Returns | `D` present? |
|---|---|---|
| `LocalKey()` | copy of this node’s full key | yes |
| `Public(id)` | public-only key for **any** seat, including self | **never** |
| `PublicExponents()` | `e` values in seat order | n/a |
| `SeatOrder()` / `SeatIndex` | canonical order from the lobby | n/a |

```116:126:internal/crypto/keyring.go
func (kr *Keyring) Public(peerID string) (*SRAKey, bool) {
	if kr == nil {
		return nil, false
	}
	pub, ok := kr.pubs[peerID]
	if !ok {
		return nil, false
	}
	return cloneSRAKeyPublic(pub), true
}
```

`TestKeyring_Public_NeverReturnsD` is the invariant as a test. If you add a method that returns another peer’s `d`, you have broken the product. Reconstructed keys after a timeout live on `CryptoHand.gone`, **not** on the Keyring (`MarkGone` documents this).

Live construction is `KeyringFromLobby`: snapshot seats, require `AllSeatsHavePublicE`, call `NewKeyring`. An empty `sra_pub_key_e` is the `--no-crypto` signal. Mixing those at one table is a hard error, not a fallback.

```267:286:internal/network/lobby.go
func KeyringFromLobby(localID string, local *pokercrypto.SRAKey, lobby *Lobby) (*pokercrypto.Keyring, error) {
	if !lobby.AllSeatsHavePublicE() {
		return nil, fmt.Errorf("KeyringFromLobby: not every seat has a public e")
	}
	// ... copy seat order and each SRAKeyE ...
	return pokercrypto.NewKeyring(localID, local, pubs, order)
}
```

---

## 10. Library shuffle vs `ShuffleSession`

### 10.1 One process: `ExecuteStep`

On a player’s turn the library does four things:

1. `EncryptAll` with **their** `e`.
2. Draw a secret random permutation (never sent).
3. Apply it to the 52 ciphertexts.
4. Commit to the output.

```29:63:internal/crypto/shuffle.go
func (sp *ShuffleProtocol) ExecuteStep(playerID string, deck []*big.Int, key *SRAKey) (*ShuffleStep, error) {
	if key == nil || !key.IsPrivate() {
		return nil, errors.New("ExecuteStep: private exponent d is not present")
	}
	encrypted, err := key.EncryptAll(deck)
	// ...
	perm, err := randomPermutation(sp.NumCards)
	// apply perm to encrypted → permuted
	commitment, err := NewDeckCommitment(permuted)
	return &ShuffleStep{
		PlayerID:    playerID,
		InputDeck:   copyDeck(deck),
		OutputDeck:  copyDeck(permuted),
		Permutation: copyInts(perm),
		Commitment:  commitment,
	}, nil
}
```

`VerifyStep` checks the commitment against `OutputDeck`. It does **not** check that the permutation was random, and it cannot: the permutation is not on the message.

`RunFullShuffle` loops that for every seat with a slice of keys. That is the oracle. `CryptoGame.RunShuffle` calls it.

### 10.2 What actually goes on the wire

`ShuffleMessage` is the published step. It **must not** contain a permutation or the input deck.

```11:18:internal/crypto/shuffle_session.go
type ShuffleMessage struct {
	HandNum    int64
	PlayerID   string
	OutputDeck []*big.Int
	Commitment *Commitment
}
```

`ShuffleMessageFromStep` copies output + commitment and drops the rest. `TestShuffleMessageFromStep_OmitsPermutation` exists so a future refactor cannot “helpfully” put `Permutation` back.

The proto payload is `ShuffleStep` (confusing name: it is the **wire** type, not `crypto.ShuffleStep`):

```44:51:internal/network/messages.proto
message ShuffleStep {
    string table_id = 1;
    int64 hand_num = 2;
    string player_id = 3;
    repeated bytes deck = 4;
    bytes commitment_hash = 5;
    bytes commitment_nonce = 6;
}
```

No permutation field. Codec maps `ShuffleMessage` ↔ proto in `codec.go`. Skip `messages.pb.go`.

### 10.3 Distributed FSM: `ShuffleSession`

Every replica starts from the **same** plaintext encoding of cards 0–51. `Start()` adopts that deck. If we are seat 0, we execute and return our `ShuffleMessage`. Otherwise we wait.

`HandleMessage` is the sequencer — same idea as Phase 3’s `actionSequencer`, keyed by **seat index** instead of action seq:

| Incoming seat vs `nextIndex` | Behaviour |
|---|---|
| `seat < nextIndex` | identical duplicate → ignore; conflicting → error |
| `seat == nextIndex` | verify commitment, adopt output, drain `pending`, maybe execute locally |
| `seat > nextIndex` | buffer in `pending[seat]` |

Gossip can deliver Dave’s step before Carol’s. The session parks Dave in `pending[3]` and refuses to apply it until `nextIndex == 3`.

```170:181:internal/crypto/shuffle_session.go
	// seat > nextIndex: buffer future steps from other players.
	if msg.PlayerID == s.kr.LocalID() {
		return nil, errors.New("ShuffleSession.HandleMessage: wrong seat")
	}
	if prev, exists := s.pending[seat]; exists {
		if shuffleMessagesEqual(prev, msg) {
			return nil, nil
		}
		return nil, errors.New("ShuffleSession.HandleMessage: conflicting buffered step")
	}
	s.pending[seat] = copyShuffleMessage(msg)
	return nil, nil
```

Local steps are **produced locally**, never accepted from gossip. GossipSub echoes you; Phase 3 already taught you that `dispatch` drops self. The shuffle FSM is stricter: even if a copy of “our” step arrived from the network, `HandleMessage` rejects it. `Start` / `executeLocalLocked` is the only path that may use `LocalKey()`.

After `n` seats, `EncryptedDeck()` returns the agreed 52 ciphertexts. `Done()` is `nextIndex >= kr.Len()`.

**Why mid-shuffle disconnect aborts.** The permutation lives only in that player’s RAM (`localPerm`). Reconstructing `d` does not reconstruct the permutation. Survivors cannot finish a consistent encrypted deck. `CryptoHand.AbortShuffle` fails `WaitShuffle`. Restart the table. Phase 5 is where timeout detection fires this; the *reason* it cannot recover is this chapter.

---

## 11. Library peels vs `DealSession`

### 11.1 One peel

`Peel` decrypts one layer with the local private key and attaches a ZK proof. Ciphertext and result on the returned value are copies.

```27:61:internal/crypto/deal.go
func Peel(key *SRAKey, ciphertext *big.Int, cardIndex int, playerID string, sessionID []byte) (*PartialDecryption, error) {
	if key == nil || !key.IsPrivate() {
		return nil, errors.New("Peel: private exponent d is not present")
	}
	result, err := key.Decrypt(ciphertext)
	proof, err := ProveDecryption(key, ciphertext, result, sessionID)
	return &PartialDecryption{
		PlayerID:   playerID,
		CardIndex:  cardIndex,
		Ciphertext: copyBig(ciphertext),
		Result:     copyBig(result),
		Proof:      proof,
	}, nil
}
```

`VerifyAndApply` checks `pd.Verify` **and** that `pd.Ciphertext` equals the replica’s current value. A peel for the wrong layer (or a replay from another card) fails the mismatch check.

```65:82:internal/crypto/deal.go
func VerifyAndApply(current *big.Int, pd *PartialDecryption, p *big.Int, sessionID []byte) (*big.Int, error) {
	if current.Cmp(pd.Ciphertext) != 0 {
		return nil, errors.New("VerifyAndApply: ciphertext mismatch")
	}
	if err := pd.Verify(p, sessionID); err != nil {
		return nil, fmt.Errorf("VerifyAndApply: %w", err)
	}
	return copyBig(pd.Result), nil
}
```

`FinishHole` is the recipient’s last local decrypt. It is **not** a `PartialDecryption`. `FinishPublic` maps a fully peeled field element to `game.Card` via `FieldToCard`.

```84:94:internal/crypto/deal.go
func FinishHole(key *SRAKey, remaining *big.Int, p *big.Int) (game.Card, error) {
	if key == nil || !key.IsPrivate() {
		return game.Card{}, errors.New("FinishHole: private exponent d is not present")
	}
	plain, err := key.Decrypt(remaining)
	if err != nil {
		return game.Card{}, fmt.Errorf("FinishHole: decrypt: %w", err)
	}
	return FinishPublic(plain, p)
}
```

`DealProtocol.RevealToPlayer` / `RevealCommunity` loop those helpers with **every** key. Oracle. `DealSession` is the protocol.

### 11.2 Peel order

```163:184:internal/crypto/deal.go
func PeelOrder(seatOrder []string, recipient string) ([]string, error) {
	if recipient == "" {
		out := make([]string, len(seatOrder))
		copy(out, seatOrder)
		return out, nil
	}
	// skip recipient; error if not seated
}
```

- Hole card for Bob: `recipient = Bob` → everyone except Bob publishes.
- Flop / turn / river / showdown: `recipient == ""` → everyone publishes, including the player whose hole index is being revealed.

Folding is a **game** status. Alice still peels the flop after she folded — her **key layer** is still on the ciphertext. Deleting her `e` would deadlock the street.

### 11.3 Distributed FSM: `DealSession`

One session per replica, after the shuffle completes. Three sequences, never overlapping (`seqIdle` between them):

| Call | Sequence | Jobs |
|---|---|---|
| `BeginHoles` | `seqHoles` | `2 * n` hole jobs, left of dealer, two rounds |
| `BeginStreet(Flop/Turn/River)` | `seqStreet` | 3 or 1 public jobs; burns skipped by index |
| `BeginReveal(playerID)` | `seqReveal` | that player’s two hole **indexes**, public peels |

`HandlePeel` is the same sequencer pattern as shuffle: apply if it is this peeler’s turn, buffer if early, reject wrong hand / wrong card index / conflicting duplicate. Local peels are produced locally.

When a **hole** job completes and we are the recipient, `finishJobLocked` decrypts the last layer into `localHoles[round]` and does **not** publish:

```692:703:internal/crypto/deal_session.go
func (s *DealSession) finishJobLocked() error {
	j := s.currentJob
	if j.kind == jobHole {
		if s.recipient == s.kr.LocalID() {
			plain, err := s.kr.LocalKey().Decrypt(s.current)
			if err != nil {
				return fmt.Errorf("DealSession: recipient decrypt: %w", err)
			}
			s.localHoles[j.round] = copyBig(plain)
			s.decoded[j.cardIndex] = copyBig(plain)
		}
		return nil
	}
	// public: store in community or revealed[revealID]
```

`TestDealSession_RecipientDoesNotPublishLastDecrypt` asserts Bob never appears as `PlayerID` on a peel for his own first card.

`Outbound()` is extra locally-produced peels that must be sent after the value returned by `Begin*` / `HandlePeel`. If applying Carol’s peel makes it *our* turn on the next card, that second peel is not the return value of `HandlePeel` — it is drained from `outbound`. `CryptoHand` always `collectPeels`s both.

`PeelOnBehalf(playerID, key)` publishes a peel **as** a gone player using a reconstructed `d`. `PlayerID` on the message is the gone seat, not `LocalID`. Rejects local id (use the normal path). Phase 5 drives this after timeout-fold; the hook is already on the session.

---

## 12. Deck indexes: the contract with the engine

Deck **indexes** match the engine’s deal order so crypto and rules cannot drift. For 4 players, dealer index 0:

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

Burns are **skipped indexes**, not peeled. `HoleCardIndex` implements the same walk as `dealHoleCards`:

```111:129:internal/crypto/deal.go
func HoleCardIndex(nPlayers, dealerIdx, playerIdx, round int) (int, error) {
	// playerIdx is an index into canonical seat order.
	// Same walk as DealHoleCards:
	//   playerIdx := (dealerIdx + 1 + i) % n  at  deckPos := round*n + i
	i := (playerIdx - dealerIdx - 1 + nPlayers) % nPlayers
	return round*nPlayers + i, nil
}
```

`CommunityStartPos` is `2*nPlayers` (first burn before the flop). `FlopIndexes` / `TurnIndex` / `RiverIndex` skip burns. `TestHoleCardIndex_MatchesDealHoleCards` and `TestCommunityIndexes_MatchDealCommunityCards` are the lock against a future off-by-one that would deal the turn as the flop.

If you change plaintext `dealHoleCards` without changing these helpers, crypto tables and local tables would silently play different cards. Do not.

---

## 13. `CryptoGame`: the in-process oracle

`CryptoGame` is the full-hand simulator: generate every key, `RunFullShuffle`, `DealToEngine` fills **all** `HoleCards`, `Deck = nil`.

```60:81:internal/crypto/crypto_game.go
func (cg *CryptoGame) DealToEngine(gs *game.GameState) error {
	dp := NewDealProtocol(cg.Deck, cg.Players, cg.Keys)
	holeCards, err := dp.DealHoleCards(gs.DealerIdx)
	// ...
	for _, p := range gs.Players {
		pIdx := cg.playerIndex(p.ID)
		p.HoleCards = holeCards[pIdx]
	}
	gs.Deck = nil
	return nil
}
```

That is legal in a unit test. It is the **leak** on a live replica: Alice’s process would hold Bob’s ranks. `TestCryptoGame_FullProtocol` is a useful end-to-end check of commitments and keygen. It is not a privacy test. Privacy tests live on `DealSession` / `CryptoHand` (`TestDealSession_HolePrivacy_PublicCannotFinish`, `TestDeal_HoleCardsPrivate`).

Keep `NewCryptoGame` / `RunShuffle` / `DealToEngine`. Do not call them from `runP2PMode`.

---

## 14. `CryptoHand`: the live replica protocol

`CryptoHand` is one replica’s shuffle + peels for **one hand**. It produces cards; callers feed them into `game.Machine`. It does not call `ApplyAction`.

```16:33:internal/network/crypto_hand.go
// CryptoHand runs one replica's shuffle + peels for a single hand.
// It produces cards; callers feed them into game.Machine.
type CryptoHand struct {
	kr           *pokercrypto.Keyring
	handNum      int64
	dealerIdx    int
	sessionID    []byte
	shuffle      *pokercrypto.ShuffleSession
	deal         *pokercrypto.DealSession
	shuffleGate  *waitGate
	holesGate    *waitGate
	streetGate   *waitGate
	revealGate   *waitGate
	earlyShuffle []*pokercrypto.ShuffleMessage
	earlyPeels   []*pokercrypto.PeelMessage
	gone         map[string]*pokercrypto.SRAKey
}
```

Construction builds a `ShuffleSession`. The `DealSession` is created only after the shuffle completes (`ensureDealLocked`), from `shuffle.EncryptedDeck()`.

### 14.1 Gates and early buffers

Gossip can deliver Bob’s shuffle step before this process has called `StartShuffle`. `OnShuffleStep` in `main` may also fire before `dealCryptoHand` has assigned `liveHand`. Two buffers exist:

1. **`main` `earlyShuffle` / `earlyPeels`** — until `installHand` sets `liveHand`.
2. **`CryptoHand.earlyShuffle` / `earlyPeels`** — if the session is not started yet, or a peel arrives for a job not yet `Begin*`’d (`isPrematureShuffle` / `isPrematurePeel`).

`WaitShuffle` / `WaitHoles` / `WaitStreet` / `WaitReveal` block on `waitGate` until the corresponding `Done` flag, or fail on `AbortShuffle`. Live waits use a 2-minute context (`cryptoDealTimeout`, `kickCryptoAdvance`).

### 14.2 Sending peels: gossip is authoritative

`Node.SendPeel` always **broadcasts** `PARTIAL_DECRYPT` on the table topic, then best-effort unicasts a copy on `/poker/1.0.0` to every other seat.

```494:519:internal/network/node.go
func (n *Node) SendPeel(ctx context.Context, msg *pokercrypto.PeelMessage) error {
	pd := PeelMessageToPD(msg)
	if err := n.BroadcastPartialDecrypt(ctx, msg.HandNum, pd); err != nil {
		return err
	}
	for _, seat := range n.Lobby.Seats() {
		if seat.PlayerID == n.Host.PeerID {
			continue
		}
		// best-effort: _ = n.SendDirectPartialDecrypt(...)
	}
}
```

Direct streams try to send hole peels to the recipient faster. If the stream drops, gossip still delivers. Do not make the unicast the source of truth.

Shuffle steps are gossip only (`BroadcastShuffleMessage`). They are large (52 × ~256-byte limbs). That is why `Shuffling…` takes several seconds: 52 × 2048-bit modexp × *n* players, sequential.

### 14.3 `AdvanceCryptoLocked`

After pre-flop betting ends, the machine is in `PhaseAwaitingStreet` (Phase 2). `kickCryptoAdvance` calls `AdvanceCryptoLocked`:

1. While `NeedsStreet`: map board length → `StreetFlop` / `Turn` / `River`, `StartStreet`, `send` peels, **release `machineMu`**, `WaitStreet`, re-acquire, `ApplyStreet` with `NewCommunityCards(already)`.
2. If `NeedsReveal`: `RemainingShowdownIDs` in **table order**, `StartReveal` each, wait, `ApplyHoleReveal`.

Waits run unlocked so a timeout-fold (Phase 5) can take the same mutex. Do not hold `machineMu` across a network wait.

```424:437:internal/network/crypto_hand.go
func RemainingShowdownIDs(gs *game.GameState) []string {
	var ids []string
	for _, p := range gs.Players {
		if p.Status == game.StatusActive || p.Status == game.StatusAllIn {
			ids = append(ids, p.ID)
		}
	}
	return ids
}
```

**Use this for `BeginReveal`, not `Machine.MissingRevealIDs()`.** Alice already has her own holes. If she peeled “whoever is missing on this replica,” she would choose a different order than Bob, and `ApplyHoleReveal` would diverge. `TestCryptoHand_RevealOrderIndependentOfLocalHoles` is the lock.

Fold-to-winner skips reveals (`NeedsReveal` is false). Next hand starts a **new** `CryptoHand`. You do not reuse the old encrypted deck.

---

## 15. Call graph from `runP2PMode`

Phase 3 already taught lobby fill and the action sequencer. This is the crypto spine after `PLAYER_READY`.

```
runP2PMode
  GenerateSRAKey(SharedPrime)          // unless --no-crypto
  NewNode(..., sraKey, ...)
  assign OnShuffleStep / OnPartialDecrypt   // BEFORE Start()
  node.Start()
  BroadcastJoin → wait lobby → BroadcastReady
  KeyringFromLobby
  DistributeLocalShares                 // unicast Shamir of d; policy in Phase 5
  dealCryptoHand
      NewCryptoHand(kr, nonce, handNum, dealerIdx)
      installHand                       // drain early gossip
      StartShuffle → BroadcastShuffleMessage
      WaitShuffle
      StartHoles → SendPeel
      WaitHoles
      LocalHoles                        // this replica only
      NewGameState; fill local holes; opponent holes = {}
      NewMachine(gs, nil); StartHandCrypto
  TUI loop
      applyAndBroadcast → ApplyAction
      kickCryptoAdvance → AdvanceCryptoLocked
          StartStreet / WaitStreet / ApplyStreet
          StartReveal / WaitReveal / ApplyHoleReveal
  startNextHand
      new CryptoHand, same Keyring, session id mixes handNum
```

Callbacks **must** be set before `Start()` so the receive loop cannot drop early shuffle/peel/join messages into nil handlers. `runP2PMode` comments this explicitly:

```413:414:cmd/poker/main.go
	// Crypto callbacks MUST be set before Start so early SHUFFLE_STEP /
	// PARTIAL_DECRYPT are not dropped. The session exists only after lobby fill.
```

`OnShuffleStep` converts proto → `ShuffleMessage`, `HandleShuffle`, broadcasts any locally produced follow-up step. `OnPartialDecrypt` does the same with `SendPeel`.

`dealCryptoHand` is the function that makes the product claim true:

```1201:1267:cmd/poker/main.go
func dealCryptoHand(...) (*game.Machine, *game.GameState, *network.CryptoHand, error) {
	h, err := network.NewCryptoHand(kr, nonce, int64(handNum), dealerIdx)
	install(h)
	outs, err := h.StartShuffle()
	// broadcast each ShuffleMessage
	h.WaitShuffle(waitCtx)
	peels, err := h.StartHoles()
	// SendPeel each
	h.WaitHoles(waitCtx)
	holes, err := h.LocalHoles()
	gs := game.NewGameState(...)
	for _, p := range gs.Players {
		if p.ID == localID {
			p.HoleCards = holes
		} else {
			p.HoleCards = [2]game.Card{}
		}
	}
	machine := game.NewMachine(gs, nil)
	machine.StartHandCrypto()
	return machine, gs, h, nil
}
```

Opponent holes are the zero `Card`, not secret ranks the TUI politely hides. Phase 2’s `player_panel.go` hides opponent holes in the UI; crypto mode makes the **memory** empty too. That is the difference between “we don’t draw them” and “we don’t have them.”

Next hand (`startNextHand`) calls `dealCryptoHand` again with the same `keyring` and `lobbyNonce`. Shamir shares are **not** redistributed (same `d`). Dealer index advances. Sequencer resets.

---

## 16. Worked example: the joint shuffle

Four people, one LAN, default crypto. Canonical seats (join order): Alice 0, Bob 1, Carol 2, Dave 3. Dealer = Alice. This is [`HOW_IT_WORKS.md`](./HOW_IT_WORKS.md) §21.5 with file names.

Every replica: `NewCryptoHand` → `ShuffleSession` from `BuildPlaintextDeck`. Session id = SHA-256(seat ids ‖ lobby nonce ‖ hand number).

**Turn 0 — Alice.** `StartShuffle` sees `expectedPlayer == LocalID`. She:

1. Encrypts all 52 values with `e_Alice`. Each card is now `m^{e_A}`.
2. Draws secret π_A. Applies it.
3. Commits. `ShuffleMessageFromStep` drops π_A and the input deck.
4. `BroadcastShuffleMessage` on `poker/table/friday`.

Bob, Carol, Dave: `OnShuffleStep` → `HandleShuffle`. Verify commitment. Adopt Alice’s output. It is not their turn, so they return no message.

**Turn 1 — Bob.** After adopting Alice’s step, `afterApplyLocked` sees `expectedPlayer == Bob`. Bob’s replica executes locally and publishes. Cards are now `m^{e_A e_B}` in an order neither Alice nor Bob can invert alone.

**Turn 2 — Carol. Turn 3 — Dave.** Same.

If Dave’s step arrives at Alice **before** Carol’s, Alice’s session parks it in `pending[3]`. When Carol’s step lands, Alice applies it, then drains Dave. All four finish with **byte-identical** 52 ciphertexts.

Wall-clock: **several seconds**. The status line says `Shuffling…` so nobody thinks the program froze.

**What each person knows after the shuffle:**

- Everyone: the same 52 big integers, and that they are a jointly encrypted, jointly shuffled deck.
- Nobody: which ciphertext is the Ace of spades.
- Nobody: anyone else’s permutation.
- Nobody: anyone else’s `d`.

Then `ensureDealLocked` builds a `DealSession` from that deck. `WaitShuffle` returns. `StartHoles` begins.

---

## 17. Worked example: Bob’s first hole card

Holes are dealt left of the dealer, two rounds. Recipient order per round: Bob, Carol, Dave, Alice. Bob’s first card is deck **index 0**.

Peel order = seats minus Bob = Alice, Carol, Dave.

```
  ciphertext[0]   (public, 4 layers)
        │
        │  Alice peels d_A + ZK  →  gossip PARTIAL_DECRYPT
        ▼
  3 layers left
        │  Carol peels d_C + ZK
        ▼
  2 layers left
        │  Dave peels d_D + ZK
        ▼
  1 layer left (Bob’s)
        │  Bob FinishHole locally  — not published
        ▼
  K♣  in Bob’s HoleCards[0] only
```

Hop by hop on the live path:

1. `StartHoles` → `BeginHoles` installs job `{kind: hole, cardIndex: 0, recipient: Bob}`. First peeler is Alice. Alice’s replica returns a `PeelMessage`; Bob/Carol/Dave return `nil` and wait.
2. Alice `SendPeel`: gossip + best-effort streams.
3. Every replica `HandlePeel`: `VerifyAndApply`. Next peeler Carol. Carol’s replica produces her peel as the *return* of `HandlePeel` (or via `Outbound` if a drain advanced the job).
4. After Dave’s verified peel, the hole job completes. **Only Bob** runs `LocalKey().Decrypt` into `localHoles[0]`. Alice’s replica stores nothing for that index (she is not the recipient).
5. Repeat for indexes 1–7. `WaitHoles` unblocks. `dealCryptoHand` copies Bob’s two cards into `gs.Players[Bob]` **on Bob’s machine only**.

After `WaitHoles`:

| Replica | Alice’s holes | Bob’s | Carol’s | Dave’s |
|---|---|---|---|---|
| Alice’s laptop | known | empty | empty | empty |
| Bob’s laptop | empty | known | empty | empty |
| Carol’s laptop | empty | empty | known | empty |
| Dave’s laptop | empty | empty | empty | empty → then known |

Each person also posted blinds **after** `StartHandCrypto`. `Deck == nil` on every replica. `StartHandCrypto` does **not** require opponent holes. Requiring them would force a leak — that was the old machine bug the historical `plans/phase-4-machine-streets.md` exists to record.

Alice still sees a leftover ciphertext at Bob’s indexes. She cannot map it to a card (`FieldToCard` would not match until Bob’s layer is gone). The TUI draws a face-down card.

`TestDealSession_HolePrivacy_PublicCannotFinish`: a replica that is not the recipient cannot `FinishHole` the leftover value.

---

## 18. Worked example: the flop appears together

Pre-flop betting is Phase 3’s replicated log. When the round ends, crypto mode sets `PhaseAwaitingStreet` and returns. No replica samples a card.

`kickCryptoAdvance` / `AdvanceCryptoLocked` sees `NeedsStreet`. `StreetFromPending` maps “0 community cards, pending 3” → `StreetFlop`. `BeginStreet(Flop)` starts public peels of indexes 9, 10, 11 (index 8 is the burn, skipped).

For flop card 1 (index 9), peel order is **all four** seats: Alice, Bob, Carol, Dave. Alice still peels even though she folded.

Each peel is verified. After four peels, `finishJobLocked` stores the plaintext field element in `community`. Repeat for 10 and 11. `StreetDone` signals the gate. Every replica calls `ApplyStreet` with the same three `game.Card`s. Phase = Flop. New betting round.

Alice’s TUI shows the flop. She still does not see Bob’s holes. That is the product working.

Turn and river are the same with one index each (`TurnIndex`, `RiverIndex`).

---

## 19. Showdown reveals

Remaining players’ hole **indexes** are peeled publicly, in **seat order on every replica**.

Suppose Alice folded, Carol folded, Bob and Dave go to showdown. `RemainingShowdownIDs` = `[Bob, Dave]` (table order, Active/All-In only).

For Bob’s two hole indexes, **everyone including Bob** peels publicly (community-style peel of private indexes). After four layers, both cards are plaintext on **every** replica. `ApplyHoleReveal(Bob, {K♣, 9♣})`. Repeat for Dave.

Now every honest machine has the same five community cards, Bob’s two holes, Dave’s two holes, and Alice/Carol still unrevealed. `EvaluateBest7` as Phase 2 taught you. `HAND_RESULT` is gossiped for logs. TUI shows winners face-up.

Alice, looking at her screen, sees Bob’s cards **now**, not earlier. That is showdown, not a leak.

If the hand ended as fold-to-winner, `NeedsReveal` is false. No public peel of holes. Next hand: new shuffle.

---

## 20. `--no-crypto` and mixed tables

Every node on a debug table:

```
seed = mix(sessionNonce)
deck = NewDeck(); rng = rand.New(seed); deck.Shuffle(rng)
StartHand()  // local deal, all holes filled on every replica
```

Identical, fast, and **zero privacy**. Any peer can print the entire deck. Use it to test that the sequencer and pots stay in lockstep without waiting seconds for SRA. Phase 3’s lab was this path.

Live crypto refuses a mixed table. Empty `e` plus real `e` must error:

```650:658:cmd/poker/main.go
	if noCrypto {
		fmt.Println("DEBUG  ·  --no-crypto  ·  shared-seed plaintext  ·  all cards visible")
		// StartHand via shared seed
	} else {
		if !node.Lobby.AllSeatsHavePublicE() {
			return fmt.Errorf("runP2PMode: crypto dealing requires every seat to publish e; a peer joined with --no-crypto")
		}
```

Do not “helpfully” fall back to plaintext if one joiner forgot the flag. That replica would see a public deck while others believed holes were hidden.

`--no-crypto` on **every** peer is the supported debug mode. One peer is not.

---

## 21. `HandCoordinator`: the trap

```27:58:internal/network/coordinator.go
func (hc *HandCoordinator) RunHand(...) (*game.GameState, *game.Machine, *pokercrypto.CryptoGame, error) {
	cg, err := pokercrypto.NewCryptoGame(playerIDs, nonce)
	cg.RunShuffle()
	cg.DealToEngine(gs)   // fills ALL holes
	m := game.NewMachine(gs, nil)
	m.StartHandCrypto()
	return gs, m, cg, nil
}
```

This is an in-process oracle helper for tests / local simulation. It is **not** the live loop. Grep `RunHand` from `cmd/poker` and you should find nothing.

If you wire it into `host`/`join`, you get a fast “crypto” hand where every seat’s cards exist on the machine that ran the coordinator. That is a house. The whole design is “there is no house.”

Live path: `CryptoHand`. Debug path: `--no-crypto` shared seed. Oracle path: `CryptoGame` / `HandCoordinator` in tests.

---

## 22. Tests in this phase

Run:

```bash
go test ./internal/crypto ./internal/network -count=1
```

Read a test when you want a worked example, not before the types make sense.

| Test | What it proves |
|---|---|
| `TestSRA_Commutativity` | Encrypt order does not matter |
| `TestPublicSRAKey_DecryptErrors` | Public-only keys cannot peel |
| `TestKeyring_Public_NeverReturnsD` | The invariant |
| `TestKeyring_RejectsEmptyPeerE` | Empty `e` is not a seated crypto player |
| `TestShuffleMessageFromStep_OmitsPermutation` | Wire shape |
| `TestShuffleSession_BuffersOutOfOrder` | Gossip-swapped steps still converge |
| `TestShuffleSession_PublicCannotRecoverPlaintext` | After shuffle, public `e`s are not enough |
| `TestZKProof_WrongResult_Detected` | Garbage peel fails verify |
| `TestDeal_MaliciousDecryption_Detected` | Tampered result does not become a card |
| `TestHoleCardIndex_MatchesDealHoleCards` | Crypto indexes = engine walk |
| `TestDealSession_RecipientDoesNotPublishLastDecrypt` | Bob’s last layer stays local |
| `TestDealSession_HolePrivacy_PublicCannotFinish` | Alice cannot decode Bob’s leftover |
| `TestDealSession_FakeNet_3Players_Holes` | In-process bus, three replicas |
| `TestDealSession_Showdown_EvaluateBest7Agrees` | Public reveals → same eval |
| `TestCryptoGame_FullProtocol` | Oracle still works (not privacy) |
| `TestCryptoHand_FakeNet_2Players_Showdown` | Live-shaped glue, two replicas |
| `TestCryptoHand_RevealOrderIndependentOfLocalHoles` | Do not use `MissingRevealIDs` |

`shuffle_session_test.go` / `deal_session_test.go` use a **fake net**: channels of `ShuffleMessage` / `PeelMessage` between N in-process sessions. That is the right unit for the FSM. `crypto_hand_test.go` adds wait-gates and `AdvanceCrypto`. Multi-process gossip is `internal/integration` (Phase 5).

---

## 23. Historical plans (read last)

The `plans/phase-*.md` files were written **while** crypto was being wired. They describe a world where live dealing was still shared-seed. Read them after the code, as archaeology, not as current status.

| Plan | What it was specifying | What you should take from it now |
|---|---|---|
| `plans/phase-1-keyring.md` | Public-only `SRAKey`, `Keyring`, nil-safe `--no-crypto` join | The invariant is still the invariant |
| `plans/phase-2-shuffle.md` | `ShuffleSession`, no permutation on the wire | Library vs FSM split |
| `plans/phase-3-deal-peels.md` | `DealSession`, recipient last decrypt local | Same split for peels |
| `plans/phase-4-machine-streets.md` | `StartHandCrypto` must not require opponent holes; streets as inputs | Engine contract; already in Phase 2 |
| `plans/phase-5-wire-p2p.md` | `runP2PMode` actually calls the above | That wiring is `dealCryptoHand` today |

Those plan numbers are **not** this onboarding series. Onboarding Phase 4 (this file) covers all five of those implementation specs. Onboarding Phase 5 is liveness, escrow, and honest gaps (`plans/phase-6-liveness.md` plus `contracts/`).

[`CRYPTO_DEAL_PLAN.md`](./CRYPTO_DEAL_PLAN.md) is the index of how crypto was planned. Prefer this chapter and [`HOW_IT_WORKS.md`](./HOW_IT_WORKS.md) §16 for “what runs today.”

---

## 24. Common mistakes

1. **Routing live P2P through `CryptoGame` or `HandCoordinator.RunHand`.** Every `(e, d)` on one machine. Live path is `CryptoHand` + `Keyring.LocalKey()`.

2. **Putting another peer’s `d` on the Keyring.** Reconstructed keys after timeout go in `CryptoHand.gone` via `MarkGone`. `Public(id)` must keep returning `D == nil`.

3. **Publishing the recipient’s last hole decrypt.** `FinishHole` / `finishJobLocked` is local. If Bob’s rank hits `PARTIAL_DECRYPT`, the product claim is false.

4. **Requiring opponent holes in `StartHandCrypto`.** That was the leak. Fill **local** holes; leave others empty; `ApplyHoleReveal` at showdown.

5. **Revealing showdown in `MissingRevealIDs()` order.** Replica-local. Alice would skip herself; Bob would not. Use `RemainingShowdownIDs` (table order of Active/All-In).

6. **Peeling burns.** Indexes 8, 12, 14 (4-player) are skipped. `FlopIndexes` already adds one after `CommunityStartPos`.

7. **Treating a shuffle commitment as a fairness proof.** It binds output bytes. Randomness is “I don’t know your permutation.”

8. **Applying shuffle/peel messages in GossipSub arrival order.** Buffer by seat / peeler index, same as `actionSequencer`. `TestShuffleSession_BuffersOutOfOrder`.

9. **Accepting our own shuffle/peel from the network.** Produce locally. Echo is dropped; the FSM also rejects `PlayerID == LocalID` on `Handle*`.

10. **A mixed `--no-crypto` table.** Empty `e` plus real `e` must error. Do not fall back to plaintext.

11. **Reusing the previous hand’s encrypted deck.** New `CryptoHand`, new shuffle from plaintext encodings. Same keys. Session id mixes `handNum`.

12. **Holding `machineMu` across `WaitStreet` / `WaitHoles`.** `AdvanceCryptoLocked` releases the mutex during waits. Blocking the fold path deadlocks liveness (Phase 5).

13. **Making direct streams authoritative for peels.** Gossip is the log. Unicast is a hint to the recipient.

14. **Changing `dealHoleCards` without `HoleCardIndex`.** Crypto and plaintext would deal different indexes.

15. **Confusing `crypto.ShuffleStep` with proto `ShuffleStep`.** The former still holds a permutation in-process. The latter is output deck + hash + nonce. `ShuffleMessage` is the type that crosses the session boundary.

16. **Calling `ExecuteStep` with `Public(id)`.** Needs a private key. `TestExecuteStep_RejectsPublicOnlyKey`.

17. **Skipping ZK verify because Noise exists.** Noise proves the last hop. Carol can forward Alice’s peel. `VerifyAndApply` is what stops garbage.

18. **Starting a feature in `internal/fault` this week.** Phase 4’s exit check is a hole-card peel, not a timeout-fold.

---

## 25. Exit check

You can explain, **without notes**:

1. **Who peels on the wire for Bob’s hole card, and who peels last locally.** Everyone except Bob publishes `PARTIAL_DECRYPT` with a ZK proof. Bob’s last `Decrypt` stays on Bob’s laptop. Opponent hole slots are empty `Card{}`, not hidden ranks.
2. **Why opponent holes stay empty until showdown.** This replica never had the missing `d`. The TUI is not “being polite.” `StartHandCrypto` must not demand those cards.
3. **Why a mixed table (one peer `--no-crypto`) must exit.** Crypto dealing requires every seat to publish `e`. An empty `e` is the plaintext-debug signal. Mixing them would desync decks and leak cards to the debug peer.
4. **Why `CryptoGame` is not `poker host`.** It generates every `(e, d)` and fills every hole. `CryptoHand` is one replica with one `d`.
5. **Why mid-shuffle disconnect aborts.** The secret permutation is only in that player’s RAM. Reconstructing `d` does not reconstruct π. Survivors cannot agree on a deck.

You have **run** a three-process crypto table (waited through `Shuffling…`, confirmed opponent holes face-down) and `go test ./internal/crypto ./internal/network -count=1`.

You have **not** yet walked a timeout-fold, Shamir reconstruction used as `PeelOnBehalf`, or the escrow contract. That is Phase 5.

When the five bullets are true, open [`PHASE_5.md`](./PHASE_5.md), starting at `internal/fault/types.go`.

---

## 26. Phase 4 glossary

A subset of [`HOW_IT_WORKS.md`](./HOW_IT_WORKS.md) §26, limited to words this chapter used.

| Term | Meaning in this project |
|---|---|
| **SRA** | Shamir–Rivest–Adleman commutative encryption on a shared prime |
| **`p` / MODP** | RFC 3526 2048-bit prime from `SharedPrime()`. Public parameter |
| **`e`** | Public exponent. Published in `JOIN_TABLE`. Used to encrypt |
| **`d`** | Private exponent. Decrypts this node’s layer. Never on gossip |
| **Commutativity** | `E_A(E_B(m)) = E_B(E_A(m))`. Peel order is a sequencer choice |
| **Keyring** | Local full key + public-only map. No API returns another peer’s `d` |
| **Plaintext deck** | Field encodings of cards 0–51 in order. Not a shuffled deck |
| **Shuffle step** | EncryptAll + secret permute + commit. One per seat |
| **`ShuffleMessage`** | Output deck + commitment. No permutation, no input deck |
| **Peel** | One `Decrypt` + ZK proof. Published as `PARTIAL_DECRYPT` |
| **Selective peel** | Hole card: everyone except recipient publishes; recipient finishes locally |
| **Public peel** | Community / showdown: everyone publishes; plaintext on every replica |
| **Burn index** | Skipped deck position. Not peeled |
| **Session id** | SHA-256(seat ids ‖ nonce). Live nonce mixes `handNum`. Binds ZK |
| **`CryptoGame`** | In-process oracle with every `d`. Tests / `HandCoordinator` |
| **`CryptoHand`** | Live per-replica shuffle + peels for one hand |
| **`DealSession`** | Distributed peel FSM. Holes, streets, reveals |
| **`ShuffleSession`** | Distributed shuffle FSM. Seat-order sequencer |
| **`gone` map** | Reconstructed keys for timed-out peers. Not on the Keyring |
| **`--no-crypto`** | Debug shared-seed plaintext. All peers or none |

---

## Companion reading (this phase only)

| File | Why |
|---|---|
| [`HOW_IT_WORKS.md`](./HOW_IT_WORKS.md) §16 | Mental poker in one sitting: commutativity, shuffle, peels, ZK |
| [`HOW_IT_WORKS.md`](./HOW_IT_WORKS.md) §§20–21 | Wiring diagram; full four-player hand including shuffle and holes |
| [`PHASE_2.md`](./PHASE_2.md) §13 | Crypto mode: cards as inputs; `StartHandCrypto` / `ApplyStreet` |
| [`PHASE_3.md`](./PHASE_3.md) §§10–11 | Gossip vs streams; why peels need both |
| [`internal/crypto/params.go`](./internal/crypto/params.go) | Shared `p`; card encoding; session id |
| [`internal/crypto/sra.go`](./internal/crypto/sra.go) | `(e, d)`; public-only keys |
| [`internal/crypto/commit.go`](./internal/crypto/commit.go) | Deck bind; Shamir math |
| [`internal/crypto/zkp.go`](./internal/crypto/zkp.go) | Peel correctness |
| [`internal/crypto/keyring.go`](./internal/crypto/keyring.go) | The invariant |
| [`internal/crypto/shuffle.go`](./internal/crypto/shuffle.go) / [`shuffle_session.go`](./internal/crypto/shuffle_session.go) | Library vs FSM |
| [`internal/crypto/deal.go`](./internal/crypto/deal.go) / [`deal_session.go`](./internal/crypto/deal_session.go) | Same split for peels |
| [`internal/crypto/crypto_game.go`](./internal/crypto/crypto_game.go) | Oracle — tests only |
| [`internal/network/crypto_hand.go`](./internal/network/crypto_hand.go) | Live replica protocol |
| [`cmd/poker/main.go`](./cmd/poker/main.go) `dealCryptoHand`, `runP2PMode` crypto branch | Composition root |
| [`plans/phase-1-keyring.md`](./plans/phase-1-keyring.md) … [`phase-5-wire-p2p.md`](./plans/phase-5-wire-p2p.md) | History, after the code |

Next: Phase 5 — liveness, settlement, tests, honest gaps. After the shuffle, a silent peer can be timeout-folded and their `d` reconstructed from Shamir shares so remaining peels finish. Mid-shuffle disconnect still aborts. Money, if it ever ships, sits in escrow off the hot path.
