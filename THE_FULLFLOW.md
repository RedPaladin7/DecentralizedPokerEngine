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
