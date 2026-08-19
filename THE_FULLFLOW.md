# The Full Flow

A step-by-step account of how this project works, in simple words. Each section was checked against the code before it was written down.

---

## Intro

This project is a peer-to-peer Texas Hold’em engine. There is no dealer and no game server. Every computer runs the same program.

Cards are shuffled and dealt with mental poker: encryption so that no single player is the dealer, and no one can peek at another player’s hole cards.

Each computer keeps its own copy of the table. When something happens, it does not take another machine’s word for the new state. It checks the message, then updates its own copy.

The aim is this: if everybody starts from the same state, and applies the same actions in the same order, everybody ends in the same state.

---

## Alice starts the table

Say four friends on the same network want to play. Someone has to start first, so the others have a place to connect. That person is called the host. Host only means “I started first.” It is not a server and not the dealer.

Alice types:

`poker host --seats 4 --name Alice --table friday`

Her program reads those flags: 4 seats, display name Alice, table named friday.

Then it loads her identity key (Ed25519) from disk, or creates one if this is the first time and saves it. From that key, libp2p derives a Peer ID. The Peer ID is her address on the network. Alice is just the name on the screen.

The key has a private part and a public part. Any message she sends is signed with the private part. Anyone else can check that signature with the public part.

Next, Alice’s program makes a second key pair, SRA: `(e, d)`. This is not the identity key. Identity signs messages. SRA locks and unlocks cards. The pair is created now and stored on her node. The actual shuffling and dealing come later.

Then it builds the player object, called a Node. A Node is this computer’s full player. Inside it:

- **PokerHost** — starts listening on TCP so others can connect. When a connection happens later, Noise encrypts that pipe. Right now she is only listening.
- **GossipManager** — joins and subscribes to two rooms for this table: `poker/table/friday` (game talk) and `poker/heartbeat/friday` (I’m alive).
- **StreamPool** — a box for later one-to-one pipes. Empty, because she is alone.
- **Lobby** — the waiting room. It knows the table is `friday` and that 4 seats are wanted. The seat list is empty. Alice has not sat down yet. Lobby is not the poker table itself; pots and turns live in a different object, created when a hand starts.
- **Gamelog** — an empty notebook for evidence. Also created now.

These pieces live on the Node. The network is built, but she has not announced herself to anyone yet.

Before Alice tells anyone she has joined, she starts the network for real.

She is already listening on TCP port 9000. `Start()` then turns on LAN discovery (mDNS): her laptop advertises that a poker process is here, and watches for others. When another laptop is found, its Peer ID and address are written into the **peerstore** — a phone book inside PokerHost: Peer ID → how to reach that machine. Then she can dial them. The TCP connection is encrypted with Noise.

The peerstore is not the lobby. The peerstore is “where is this Peer ID?” The lobby is “who has a seat?” StreamPool is still empty; pipes are opened later, using those addresses.

She also starts loops that listen on the two gossip rooms, and she prints her address so a friend who is not on the same LAN can paste it.

Now she sits down. She **publishes** a `JOIN_TABLE` message on `poker/table/friday`. In this project that is the broadcast: one signed shout into the table room.

That shout includes her name, buy-in, and her public SRA `e` (never `d`). She also writes herself into her own Lobby right away. The first join time is saved. While the table is not full, she repeats the same join every 2 seconds with that **same** time, so a late friend can still hear her and seat order does not change.

Gossip is not a history book. Old shouts disappear. That is why she keeps repeating. What stays is the seat list in each person’s Lobby, not a pile of packets.

Then she waits for the other three. She has not said “ready” yet.

Alice’s laptop is one program doing several jobs at once. That is not one thread, and it is not “the Node running as a thread.” The Node is the shared object. Inside the process, Go runs **goroutines** (lightweight workers). The Go runtime puts those onto a few OS threads.

Right now those workers include:

- The **main** one, still waiting for friends. It wakes on timers: rebroadcast join every 2 seconds, and check whether 4 seats are filled.
- One that **listens to** `poker/table/friday`. It sleeps until a gossip message arrives, then handles it.
- One that **listens to** `poker/heartbeat/friday` the same way (quiet for now).
- One that every few seconds scans the Gamelog for cheating (two different stories with the same sequence number).
- **libp2p’s** workers: accept new TCP connections on port 9000, encrypt with Noise, keep GossipSub going, and run mDNS.

So “listening on two topics” and “listening on port 9000” are different workers, not two extra programs.

Incoming table talk is not stored in a channel we created. That worker just **blocks** until GossipSub has a message, then processes it. Channels *are* used for timers and for “stop” (Ctrl-C). The lobby also has a wake-up channel for “everyone is ready,” but the live wait-for-players loop does not sit on that channel; it checks the seat count on a timer.

That is all Alice can do by herself. The rest needs other players.

Until then she only keeps the lights on: listen on 9000, stay in the two gossip rooms, keep mDNS running, and repeat her join shout every 2 seconds. She has not said ready, has not started heartbeats, has not opened the table UI, and has not shuffled any cards.

---

## Bob joins

Alice tells Bob the table is named `friday`. On his laptop he types:

`poker join --name Bob --table friday --seats 4`

`join` is not a special client. It is the same program. The word `join` only means he is not the first listener.

Bob does the same setup Alice did: load or create **his** identity key, derive **his** Peer ID, make a fresh SRA pair `(e, d)`, build a Node (PokerHost, GossipManager on the `friday` rooms, empty StreamPool, empty Lobby, empty Gamelog), start listening, start mDNS, start the receive workers.

He does not look up “tables named friday.” mDNS finds other **poker processes** on the LAN. `--table friday` is the room he will talk in after he is connected. (If he were on another network, he would paste Alice’s address with `--peer`.)

When he finds Alice, he writes her into his peerstore and dials. TCP connects; Noise encrypts the pipe. She may dial him too. Now they have a connection. They are still two equal peers.

Then Bob sits down the same way Alice did: he publishes `JOIN_TABLE` on `poker/table/friday` and writes himself into his Lobby. He hears Alice’s repeated join; she hears his. Each adds the other to their own seat list. Neither sends the other a copy of “the whole table.”

They still wait. The table wants 4 people. Heartbeats, ready, shuffle, and the UI have not started.

When Bob’s mDNS starts, he is not looking for `friday`. He is looking for anyone advertising `p2p-poker-v1`. Alice is already doing that. He is too. mDNS is both “here I am” and “is anyone else here?”

He hears Alice: her Peer ID and the address she is listening on. He writes that into **his** peerstore, then dials. TCP handshake, then Noise. Now they have one encrypted pipe. She may hear him the same way and dial back. Either way they are two equal peers, not client and server.

That pipe is the connection. Two more things sit on it, and they are not the same:

- **Gossip** — both already subscribed to `poker/table/friday` and `poker/heartbeat/friday` when they built their Nodes, from `--table friday`. After the pipe is up, GossipSub compares topic names. Same string → they are in the same gossip room. If Bob had used `--table saturday`, the pipe could still exist and they would still never hear each other’s joins.
- **Unicast** — the `/poker/1.0.0` path used later for one-to-one messages (key shares, extra peels). The handler is already registered, but StreamPool is still empty. No poker unicast stream is opened just because they connected. When that happens later, it reuses this same pipe. No second TCP handshake.

They still have not said ready. They still wait for Carol and Dave.

Bob’s mDNS is: “I run poker. Is anyone else?” Not “who is at friday.” He hears Alice’s Peer ID and listen address, writes them into his peerstore, and dials. TCP, then Noise. One pipe.

That pipe is not a key exchange. Two different public pieces:

- **Identity.** Alice’s Peer ID comes from her Ed25519 public key. Noise proves she owns it. Bob can pull that public key out of the Peer ID when he checks her signatures. No extra identity packet.
- **SRA `e`.** That arrives in her `JOIN_TABLE` on `poker/table/friday`. Not at connect. `d` never goes out.

Alice does not have to dial Bob back. One pipe is two-way. When he connects, her PokerHost accepts and libp2p writes his Peer ID (and the address it saw) into **her** peerstore. She may also hear him on mDNS and dial; if the pipe is already there, that is the same path again.

The join shout is already running, on the **table topic**, not on mDNS. Alice has been publishing `JOIN_TABLE` every 2 seconds with the same first timestamp. Gossip is not a log, so Bob missed the shouts from before he was in the room. After the pipe and `friday` match, her next repeat is how he seats her. His `JOIN_TABLE` is how she seats him and learns his `e`. That is now, while they wait for four people — not later at shuffle.

Either laptop can dial. Both run the same mDNS callback. Bob may find Alice first, or Alice may find Bob first. One TCP+Noise pipe is enough. They do not both need to succeed.

Once that pipe exists, Alice and Bob do not need mDNS to keep talking to each other. Gossip and later unicast use the pipe. mDNS is **not** turned off. Carol and Dave still have to be found the same way, and the process only stops mDNS when it exits.

They already subscribed to `poker/table/friday`. That is where they hear each other’s `JOIN_TABLE` shouts. The shout carries name, buy-in, and public SRA `e`. The join time is the envelope timestamp — the same frozen first-join time on every repeat. Each Lobby sorts seats by that time, and by Peer ID if two times match. Same shouts, same order.

The identity public key is not a separate gift in the shout. It can be derived from the Peer ID they already have. When a signed message says it is from Alice, the other laptop pulls the public key out of that Peer ID and checks the signature. If it does not match, the message is dropped. The program may cache that derived key so it does not redo the extract every time. That cache is not a second identity. SRA `e` is different: that one is stored on the seat, from the join shout.

Subscribing does not reach Alice by itself. Joining `poker/table/friday` is **local**: Bob’s GossipSub now cares about that name. It is not a global radio and not a directory. This project has no DHT and no rendezvous server.

GossipSub only moves bytes over **existing libp2p pipes**. Until Bob has TCP+Noise to someone already in that room (usually Alice), his subscription has nobody to talk to. His receive loop just blocks. Alice’s join shouts from before he connected are already gone.

mDNS (or `--peer`) is the missing first step: **who is here, and what address do I dial?** After the pipe is up, GossipSub compares topic names, grafts him into `friday`, and *then* Alice’s next `JOIN_TABLE` can arrive.

So: mDNS finds a machine. The topic finds a room **on that machine**. Without the first, the second is an empty room.

---

## Carol joins

Carol types the same kind of command Bob did:

`poker join --name Carol --table friday --seats 4`

Same program. Own identity, own SRA, own Node: PokerHost, GossipManager already on `poker/table/friday` and `poker/heartbeat/friday`, empty StreamPool, empty Lobby, empty Gamelog. Then `Start()`: mDNS, receive loops, listen.

mDNS finds poker processes, not friday. She hears Alice and Bob, writes them into **her** peerstore, and dials. TCP, then Noise. On a LAN she usually gets a pipe to both. She does not need both pipes to hear the table talk — a join can hop — but the usual case is she is connected to both.

She sits down the same way: publish `JOIN_TABLE` on `poker/table/friday`, seat herself in her Lobby. Alice and Bob are still repeating their joins, so she hears those shouts, checks the signatures, and adds them to **her** seat list, each with their public `e`. They hear her shout and add her to **theirs**. Nobody sends Carol a copy of the whole waiting room.

Each Lobby sorts seats by the join time in the shout, then by Peer ID if two times match. That is why the three lists match, even if Carol heard Bob’s shout before Alice’s.

They still wait. The table wants 4. Dave is not here yet. Heartbeats, ready, shuffle, and the UI have not started.

Carol dialing Alice and Bob does not mean this project built a star, and it does not mean a mesh is still waiting to be turned on.

There are two graphs, both already live:

- **Pipes.** mDNS plus `Connect` to whoever is found. On this LAN that usually means everybody has a TCP+Noise pipe to everybody. That is just “I dialed who I saw.”
- **Gossip.** GossipSub started when each Node was built. A shout on `poker/table/friday` goes to neighbors on that topic. A neighbor may forward it. The laptop you heard it from might not be the author. That is why the envelope is signed.

With four friends, the gossip neighborhood is big enough that it often looks like “everyone to everyone” too. Hopping still works if someone only has a pipe to Alice (`--peer`), or mDNS missed a laptop. There is no later step that builds a different mesh. Unicast later is the other path: it wants a direct pipe, not a hop.

If John starts poker with `--table friday2`, mDNS still finds him. Same tag: `p2p-poker-v1`. Alice, Bob, or Carol can TCP+Noise to him and put him in the peerstore. He is not in their Lobby. He subscribed to `poker/table/friday2`. They subscribed to `poker/table/friday`. His join shouts never hit their receive loop. A pipe is not a seat. `--table` is the room name, not a lock on the TCP connection.

What they publish in the room is not a raw string. It is a framed protobuf **Envelope**: who sent it (Peer ID), a sequence number, the join time, the inner message, and an Ed25519 signature. The inner message is another protobuf — for a join, name, buy-in, and public `e`.

Bob already has Alice’s Peer ID from mDNS so he can dial her. That is not how he checks her shout. The shout carries her Peer ID as `sender_id`. He pulls the public key out of **that** id and verifies the signature. Match → it was signed by Alice’s identity. No match → drop. The bytes might have hopped through Carol. The signature still says Alice.

---

## Dave joins

Dave types the same kind of command:

`poker join --name Dave --table friday --seats 4`

Same program, same setup, same two `friday` rooms, own Node. mDNS, peerstore, TCP+Noise, then `JOIN_TABLE`. Alice, Bob, and Carol are still repeating their joins, so he hears all three, checks the signatures, and builds **his** seat list with their public `e` values. They hear him and add him. Four Lobbies sort by join time, then Peer ID. Same four people, same order.

The wait loop was only watching the seat count. It now sees 4. Join shouts stop. A fifth laptop can still connect on mDNS, but `HandleJoin` will not give it a seat.

Four seats is not yet “ready.” Ready is a second signed shout, `PLAYER_READY`, on `poker/table/friday`. There is no button. As soon as a process sees 4 seats, it publishes ready and marks itself ready locally. Alice is not in charge of that. All four do it when their own loop notices.

The Lobby becomes ready only when every seat has that flag. The live wait loop does not sit on that wake-up channel. After it broadcasts ready it just pauses two seconds so the other readys can arrive, then it leaves the waiting room. Cards, heartbeats, and the table UI have not started. Those come next.

After the two-second pause, nobody has clicked anything. Each laptop still builds the same shared facts from the seat list it already has.

Every join shout carried a small blob: that player’s Peer ID bytes. Each Lobby now glues those blobs together, in the same seat order (join time, then Peer ID). That combined blob is the session nonce. From it they also mix a shared seed. The seed is what `--no-crypto` uses as the fake deck shuffle. The real crypto path still builds the seed; it just does not deal from it.

Then they copy the lobby seats into a player list, still in that same order. This is hand 1. The dealer is index 0 — the first name in that sorted list. That is not “Alice the host.” If Dave’s join time was first, Dave is dealer.

Now they start heartbeats, before any cards. The heartbeat room `poker/heartbeat/friday` was already subscribed from the start. What is new is they begin shouting “I am here” on it, every few seconds, signed like the rest. A FaultManager on each laptop watches those shouts so a silent player can later be called out. Shuffle has not started.

Still no cards. Each laptop now builds a Keyring: its own full SRA pair `(e, d)`, plus everyone else’s public `e` from the join shouts, in the same seat order. Nobody else’s `d` is in that box. All four do this from the seat list they already have. Nobody sends a copy of the Keyring.

If any seat is missing `e` — someone sat down with `--no-crypto` while the others did not — the process exits. Mixed tables are not allowed.

Then each player splits their own secret `d` into four shares. They keep one share. The other three go one-to-one to the other seats. That is when StreamPool finally fills. Until now the box was empty: the `/poker/1.0.0` handler was registered at connect, but no unicast stream was opened just because a TCP+Noise pipe existed. Sending a share is the first `Send`. StreamPool opens `/poker/1.0.0` on the pipe they already have — no second TCP handshake — and keeps that stream so later one-to-one messages (peels) can reuse it. The shares are not shouted on `poker/table/friday`. They are sent once for the table, not every hand. Later, if someone vanishes, enough of those shares can rebuild that player’s `d`. Shuffle has not started.

## Game Start

Now shuffling starts. There is still no table UI and nobody clicks a button. All four laptops enter the same card pipeline on their own. Each builds a CryptoHand and starts the shuffle machine.

The starting deck is not Alice’s secret. It is fifty-two known numbers, the same on every laptop, one value per card in a fixed order everyone already knows. Anyone can rebuild that list. What must stay secret is the mixing.

Only seat 0 has a first message to send. In this friday story that is Alice, because she joined first. She is also dealer this hand, because the dealer is index 0. She is not special because she typed `host`.

Alice locks every card with her public SRA `e`. Then she secretly rearranges the fifty-two locked cards. That new order stays on her laptop. It is a fresh random mix, not the shared seed. She also makes a fingerprint of the deck she is about to publish, so nobody can swap a card later and claim it was the same shout.

She publishes that locked-and-moved deck, plus the fingerprint, as a signed `SHUFFLE_STEP` on `poker/table/friday`. Not the permutation. Not `d`. Bob, Carol, and Dave’s machines have already started their shuffle and are waiting. If Alice’s shout arrives before someone’s shuffle machine exists, that laptop parks it and applies it a moment later. Alice now waits for their steps. The deck is not fully shuffled yet.

That shout is not a recipe for the mix. It is the fifty-two locked numbers themselves, in the new order, plus the fingerprint. Bob, Carol, and Dave do not undo Alice’s mix. They check the signature, check it is her turn, check the fingerprint matches those fifty-two numbers, then replace their current deck with her array. Same pile on every laptop. Unknown faces.

They all copy it on purpose. This is not one physical deck handed to Bob. The numbers are locked; copying them does not unlock them. If only Bob received the pile, the others would have to take his word later. Each laptop keeps its own table: same pile, same signed steps, same order. So Alice publishes on `poker/table/friday`, and all four photocopies update.

That check does not prove each new number came from the old one. If Alice’s first locked card was 227, and Bob later publishes 787, nobody checks `787 = 227` raised to Bob’s `e`. Checking that pairing would reveal where he put the card — that *is* his mix. The fingerprint only binds the bytes he published. A proper “I mixed your pile, and I will not say how” proof is not in this program. Peels later can catch junk. They do not catch a swapped real card, and they do not stop someone who matches public `e`s to recover the order. Four honest copies of this program stay in lockstep. That is the demo. It is not a proof the shuffle was honest.

Now it is Bob’s turn. He locks every number in Alice’s pile with **his** `e`, secretly rearranges, fingerprints, and publishes his own `SHUFFLE_STEP` on the same table room. Everyone copies his array. Carol does the same to Bob’s pile. Dave does the same to Carol’s. A step that arrives early is parked until the gap fills.

After Dave’s shout, all four laptops hold the same fifty-two numbers, each locked by all four `e`s, in an order nobody is supposed to map back. That shared pile is the deck they will peel from. The table UI is still not open. Hole cards have not been dealt.

Each laptop then waits until it has seen every step, not just its own. Alice does not start dealing after her shout. Only when all four `SHUFFLE_STEP`s are in — Alice, Bob, Carol, Dave — is the shuffle done. They sit on that wait for up to two minutes. If someone vanishes in the middle of their mix, the wait fails and the hand dies. That player’s secret order was only in their RAM. The Shamir shares can rebuild a missing `d` later; they cannot rebuild a mix that was never published. Restart the table. The shared pile is ready. Peels come next.
