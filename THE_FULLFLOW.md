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

## Peel

The shuffle wait is over. Before anyone peels a lock, each laptop copies that shared pile into a DealSession: the same fifty-two locked numbers, the Keyring it already has, this hand number, and who the dealer is. Nobody sends this object. All four build it from facts they already share.

The dealer index is used here to decide who gets which slot from the top of the pile. It is not “who shuffled first.” Shuffle always started at seat 0. This hand Alice is both, because she is first in the sorted list.

Then they write the hole-card jobs. Two rounds, like a real deal: first card to the seat after the dealer, then around the seat list, then a second card the same way. Alice is dealer, so slot 0 is for Bob, slot 1 for Carol, slot 2 for Dave, slot 3 for Alice. Then those same four get a second card from slots 4–7. Faces are still locked. A slot is not a readable card.

Unlocking starts right away, one slot at a time. Bob cannot unlock slot 0 by himself. That number still has Alice’s, Carol’s, and Dave’s locks.

The people who publish a peel are everyone except the person getting the card, in seat-list order. For Bob’s first card that is Alice, then Carol, then Dave — not “the next seat after Bob.” Each peel uses that player’s secret `d` and carries a small proof that the peel was real. Junk (a number that is not that lock coming off) gets caught here. The proof is not the rank and not `d`.

Each published peel is a signed `PARTIAL_DECRYPT` shout on `poker/table/friday`. They also try to send the same peel one-to-one on `/poker/1.0.0` (StreamPool, the pipes opened for Shamir shares). Gossip is the real delivery. The extra copy is a shortcut. Duplicates are ignored. A peel that arrives early is parked until its turn, same idea as a shuffle step.

Alice peels slot 0 first, publishes, and the other three copy that new number after they check the signature and the proof. Then Carol, then Dave. After Dave, only Bob’s lock is left. Only Bob’s laptop takes that last lock off. He does not publish it. Alice, Carol, and Dave keep the leftover number. They cannot turn it into a face.

Then the next job: Carol’s first card, skip Carol in the peel list. Eight jobs in all. They wait until every hole job is done — same kind of two-minute wait as shuffle. Then each laptop reads only its own two faces. Opponent holes stay empty. The table UI is still not open.

Those three shouts are not three separate peels of the original four-lock number. They are a chain. Alice’s shout says: I started from slot 0 as we all have it, I took my lock off, here is the new number. Carol must peel **that** number, not the original. Dave peels Carol’s leftover. Bob does not add three leftovers together. He waits until Alice, then Carol, then Dave have each taken one lock off, and then he takes his own lock off the last leftover, only on his laptop.

The `PARTIAL_DECRYPT` shout carries which slot, the number they started from, the number after their `d`, and the proof. The proof lets everyone check “this new number really is the old one with my lock removed” without seeing `d` or the card face. It catches junk. It does not print the rank.

The one-to-one `/poker/1.0.0` copy is the same shout, not a private card for Bob. Gossip on `poker/table/friday` is how it counts. The stream is a shortcut so a laptop might hear the peel faster. Same envelope, same signature. Duplicates are ignored.

The peel shout does not name who goes next. It is not a shuffle step. Alice’s `PARTIAL_DECRYPT` only says: I am Alice, this is slot 0, I started from this number, here is the new number, here is the proof.

Who is allowed to peel was already on every laptop, from the same DealSession recipe: seat list minus Bob, in that order. For Bob’s first card that is Alice, then Carol, then Dave. After Alice’s shout is checked, each machine ticks its own list. Carol’s laptop sees it is her turn and peels. Dave’s laptop sees it is not his turn yet and waits. Nobody sent Carol a “your turn” packet.

## Betting Start

The eight hole jobs are done. Each laptop can read only its own two faces. Opponent holes are still empty. That is not “everybody can see every hole card.”

Now they finally build the poker table itself: a GameState. This is not the Lobby. It holds stacks, blinds, the pot, and whose turn it is. Each laptop copies its own two cards into its own seat and leaves the other seats’ cards blank. There is no local deck to deal from. Cards already came from peels.

Then each machine posts the blinds by itself. Alice is dealer, so Bob is small blind and Carol is big blind. Chips come off those two stacks on every laptop the same way. Nobody publishes a blind shout. Same start state, same rule.

The table UI opens now. You see fold, call, and raise, and your own two cards. The others show face-down.

Pre-flop, the first person to act is the seat after the big blind: Dave. He picks fold, call, or raise. That click becomes a signed `PLAYER_ACTION` on `poker/table/friday`: who, what, how much, and a sequence number (gossip can arrive out of order). Each laptop checks it and applies it to its own GameState. Alice is not a referee. The next seat then acts the same way.

The same worker that has been listening on `poker/table/friday` since the start is what hears Dave. It is not a new betting thread. It sleeps until a gossip frame arrives, then handles it.

Checkout is two steps. First the envelope: `sender_id` is Dave, the signature matches his identity. Bad signature → drop. That only proves Dave signed those bytes. Then the GameState: the turn pointer is already on Dave. If the shout is not from that seat, it is rejected. A shout that arrives early (Alice before Dave) is parked until the sequence number lines up.

Only after both checks does that laptop apply the fold, call, or raise to its own table, then step the pointer to the next seat who can still act. Alice’s UI flips to betting when her copy has done that. Nobody published “now Alice.”

## Pre flop card reveal

The pre-flop betting round is over. Each laptop saw that locally when the last action was applied. It does not deal three cards from a pile in RAM. There is no local deck. The phase is just “waiting for the next street.” Betting clicks are refused until those cards exist. Nobody published “deal the flop.”

Each laptop already has the locked pile in its DealSession. It now starts public peels for the flop: three jobs, one card at a time. The burn under the flop is a skipped index. They do not peel it.

This is the same peel chain as hole cards, with one change. For Bob’s hole card, Bob was skipped and took his last lock off only on his laptop. For a board card, nobody is skipped. All four peel, in seat order: Alice, then Bob, then Carol, then Dave. After Dave’s shout, the number has no locks left. Every laptop can turn it into a face. There is no unpublished last step.

Still a chain: Alice peels the four-lock number, Bob peels Alice’s leftover, Carol peels Bob’s, Dave peels Carol’s. Same signed `PARTIAL_DECRYPT` on `poker/table/friday`, plus the best-effort one-to-one copy. Same proof: this new number really is the old one with my lock removed. The shout does not name who goes next. Each DealSession already has the recipe.

They do that for three cards. Early peels park. They wait until all three jobs are done. Then each laptop writes those three faces onto its own GameState. Nobody sent a copy of “the flop.” Same three cards on Alice, Bob, Carol, and Dave.

The table UI can show them now. Opponent hole cards stay face-down.

## Flop

The three flop cards are already on every GameState. Betting starts again by itself. Nobody published “flop betting.” First to act is the seat left of the dealer: Bob, not Dave. Dave was first before the flop because he sat after the big blind.

Same mechanics as pre-flop. Bob picks fold, call, or raise. His laptop applies it, then publishes `PLAYER_ACTION` on `poker/table/friday`. The others wait for that frame, check it, apply it, and step the pointer. Alice, then Carol, then Dave, around the table the same way.

When the last action of this round is applied, each laptop ends the round locally. It does not deal the next card from RAM. Waiting for the next street again. Betting clicks are refused until that card exists.

That next card is the turn, not a second flop. One public peel job, not three. The burn under the turn is a skipped index. They do not peel it.

Same chain as the flop cards: Alice, then Bob, then Carol, then Dave, each taking one lock off the leftover. Same `PARTIAL_DECRYPT` on `poker/table/friday`. After Dave, the number is a face on every laptop. They write that one card onto their own GameState. Nobody sent a copy of “the turn.” Four community cards, the same four, on Alice, Bob, Carol, and Dave.

## Turn, River, Showdown

The turn card is already on every GameState. Betting starts again, same as the flop. First to act is still Bob, left of the dealer. Same `PLAYER_ACTION` shouts on `poker/table/friday`. When that round ends, each laptop waits for a street again. It does not deal from RAM.

The river is one public peel, like the turn. The burn under the river is a skipped index. Alice, Bob, Carol, then Dave each take a lock off. They write that one face onto their own GameState. Five community cards, the same five, on every laptop.

River betting is the same walk. Bob first again. When the last river action is applied, they do **not** wait for another street. There is no sixth board card. Each laptop starts showdown on its own copy.

If only one player is still in — everyone else folded — that player gets the pot. No hole cards are revealed. Opponent holes stay empty. The hand is over.

If two or more are still in, the remaining hole cards have to become public. That is the same public peel as a board card, not the private hole deal from earlier. For each remaining player, all four peel both of that player’s hole indexes. After Dave’s last lock, every laptop can see those two faces. Folded players’ cards stay face-down.

Each laptop then scores the hands it now has, splits the pots, and adds the chips to the winners’ stacks. Same start state, same cards, same rule — same payouts. Nobody sent a copy of “the result.” The table UI shows who won.

## Next hand

The table UI shows who won. After a short pause — about three seconds — the next hand starts by itself. Nobody clicks. Nobody published “hand 2.”

Each laptop still has the same seat list from the lobby: Alice, Bob, Carol, Dave. That order does not get rebuilt. Each machine adds one to its dealer index. Hand 1’s dealer was 0, Alice. Hand 2’s dealer is 1, Bob. Same start state, same rule.

They do not sit down again. Identity keys, SRA pairs, and Shamir shares stay where they were. The Keyring is the same box. What is new is the shuffle: a fresh mix of the same fifty-two starting numbers, then hole peels, blinds, betting, streets, showdown.

Shuffle still starts at seat 0. Alice publishes the first `SHUFFLE_STEP` again, even though Bob is dealer this hand. Bob being dealer only changes blinds and who gets which hole slot. Alice is small blind. Carol is big blind. First to act pre-flop is Dave.

Stacks are the ones left after hand 1. Hole cards and fold flags are cleared. If a seat went empty, it is still on that list — this program does not drop it. For a new group of people, restart the table.

## Quits

Dave cannot be dropped so the other three play a 3-seat game. The seat list stays Alice, Bob, Carol, Dave. Next hand still wants four locks and four shuffle steps.

If Dave hits `q` or Ctrl-C, that only closes **his** program. Nobody gets a “Dave left, now we are three” packet. His heartbeats on `poker/heartbeat/friday` stop. After a short silence the others vote that he is gone and fold his seat for the hand they are in. They can finish that hand. They cannot drop him from the next shuffle. Restart the table if you want a new group.

If he vanishes in the middle of a shuffle, the hand dies. The mix was only on his laptop. Shamir shares can rebuild a missing `d`. They cannot rebuild a mix that was never published.

Equivocation is a second kind of cheat, not a card proof. Dave signs two different shouts with the same sequence number — fold to Alice, raise to Bob. Same seat, same seq, two stories.

Each laptop already checks Ed25519 when a friday frame arrives. The public key comes from the Peer ID. The signature covers the type, who sent it, the seq, the time, and the inner bytes. That only proves Dave signed **that** envelope.

A worker started at `Start()` wakes every few seconds and walks the Gamelog. It looks for the same sender and the same seq with two different payloads. Same bytes twice is just gossip repeating. Different bytes is the cheat. There is no extra SRA math here. It is a byte compare of two already-signed envelopes.

On the live path that pair almost never sits in one notebook. The receive loop drops a seq it has already seen, and the log refuses a second row at that slot. Alice may only have the fold. Bob may only have the raise. Each scan sees one story. The table can already have split. Detection is after the fact, and this program does not stop the hand or take chips when it fires. Honest copies of the program do not send two stories.

Shuffle has a tighter check on the way in: if Bob already copied Alice’s pile and a second `SHUFFLE_STEP` from Alice disagrees, that is a conflicting step and the shuffle errors. Same idea at showdown if a public hole reveal disagrees with cards already written.

