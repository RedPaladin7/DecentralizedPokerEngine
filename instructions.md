# Prompt: Generate a Decentralized Poker Engine Systems Design & Data Flow Document

Copy everything below this line into a fresh session with an LLM that has file/tool access to this project's Go source tree and your accumulated notes file(s).

---

## Context

I've been learning Go and distributed/peer-to-peer systems concepts by reading through the source of a decentralized poker engine, file by file, with an AI tutor. For each file (or small group of related files), I built up a detailed "slice" — a long, plain-language explanation covering: what the file is for, new concepts it introduces, a function-by-function or method-by-method walkthrough, and how it connects to everything read before it. All of these slices are collected in markdown notes files somewhere in this project/workspace. **Locate every such notes file yourself** (they may be named things like `notes.md`, or split per-package/per-topic — search the workspace for markdown files that read like tutorial-style code walkthroughs rather than standard project docs like README/CONTRIBUTING) and read all of them in full before doing anything else.

This project's decentralization is built on **peer-to-peer gossip/mesh networking** — nodes discover and communicate directly with each other rather than through a central server, and game state / actions need to be propagated and agreed upon across peers without a trusted central authority.

I now have every source file for this project, including the top-level file(s) that wire everything together (the main entrypoint and/or the top-level node/server struct that assembles the networking layer, the game engine, and everything in between) — **read the full source tree**, not just my notes, for anything you need to verify, fill in, or correct.

## Your task

Read the actual Go source files in the project (not just my notes — go back to the real `.go` files for anything not fully covered in my notes, or to verify anything my notes might have gotten wrong) and my accumulated notes files, then produce **one single, comprehensive, standalone markdown document** that ties the entire system together. This document has a specific job: it should let me sit down with an interviewer, hand them nothing but this file, and walk them through the entire system's architecture, data flow, and design rationale confidently — without needing to open the source code.

Do not just summarize my slices back to me. Synthesize across all of them, cross-reference the actual source code to fill any gaps or correct anything my notes got wrong, and add the connective tissue and systems-design analysis that individual per-file slices couldn't cover, since each of those was written before I knew how the whole system fit together. Where the project has real Go-specific and distributed-systems-specific substance (goroutines, channels, mutexes/locks, the gossip protocol itself, message types, peer discovery, state synchronization, cryptographic fairness mechanisms if present), treat that with the same rigor you'd give epoll/RAII/backpressure in a C++ networking project — these are the areas most likely to be genuinely hard and most likely to come up in an interview.

## Required structure

### 1. Elevator pitch
A short (under one paragraph) plain-language description of what this poker engine is and does, as if explaining it to a friend with no context: what a "node" is, what running the software gets you, and what "decentralized" concretely means for how a hand of poker actually gets played and agreed upon.

### 2. Full system architecture diagram
One Mermaid diagram (use `graph` or `flowchart`) showing every major component discovered in the source: the networking/transport layer, the peer discovery and gossip mechanism, the game engine / poker rules logic, any cryptographic/fairness component, persistence if any, and the top-level entrypoint that owns and wires all of it together. Show ownership (who constructs/starts whom), and clearly distinguish long-running components (things that run their own goroutine or event loop) from plain library-style code that's called synchronously. If there's a clean layering (e.g. transport → gossip protocol → game logic, or similar), make that layering visually obvious in the diagram.

### 3. Full lifecycle — sequence diagrams
Using Mermaid `sequenceDiagram`, trace at least these flows end to end, participant by participant, across every relevant file/package:
- **A new node joining the network**: process startup → listening/dialing → peer discovery or bootstrap → handshake → becoming a known peer to others → what state (if any) it needs to sync before it's considered a full participant.
- **One complete poker hand, from the perspective of the gossip/consensus mechanism**: how an action (e.g. a bet, a card reveal, a showdown result) originates at one node, gets propagated to peers, and reaches whatever counts as "agreement" in this system — cover exactly how deck shuffling/dealing fairness is established and verified across untrusted peers if the source implements this, since this is usually the single hardest and most interview-relevant part of any decentralized card game.
- **A peer failing, disconnecting, or behaving maliciously/inconsistently mid-hand**: what detects this, and what the system actually does about it (timeout, exclusion, hand voided, some other recovery path) — describe only what the real source does, don't invent a recovery mechanism that isn't there.

If a fourth flow is clearly important based on what you find in the source (e.g. a specific consensus round, a reconnection/resync flow, or a spectator/observer flow), add it.

### 4. Data flow: how one game action or message actually moves through the system
A diagram or clearly structured explanation tracing a single message (e.g. one player action, or one gossip message) from the moment it's created at one node to the moment every relevant peer has processed it: what goroutine or channel it passes through, where (if anywhere) it's serialized/deserialized, where cryptographic signing/verification happens if present, and where in-memory state actually gets mutated. Be explicit about what's copied vs. shared, and where locks (if any) are acquired.

### 5. Concurrency model
Explain plainly how concurrency actually works in this codebase, grounded in what you find in the source, not a generic Go answer:
- How many goroutines exist per peer connection / per node, and what each one is responsible for.
- What channels exist, what flows through them, and why channels were used in that spot instead of a mutex (or vice versa).
- Where shared state exists (e.g. the set of known peers, game/table state) and exactly how it's protected — mutex, channel-owned single-goroutine access, atomic values, or something else.
- What could race if the protections described were removed, to make the "why this matters" concrete.
- Whether the design is closer to "one goroutine per connection, shared state behind locks" or "share memory by communicating" (goroutines owning state, talking only via channels) — state which pattern(s) the actual code uses, since real codebases often mix both.

### 6. Design decisions and rationale
For each significant decision you find in the actual code, state: what the decision was, what problem it solved, what the alternative would have been, and what it cost. Write these as if answering "why did you do X instead of Y" in an interview. Cover at least:
- The choice of gossip/mesh topology over a central server or a strict consensus protocol (Raft/PBFT/etc.) — what this buys in terms of no single point of failure/trust, and what it costs in terms of coordination guarantees.
- How the deck is shuffled and dealt fairly when no single party is trusted — this is very likely the single most important design decision in the whole project if it's implemented; explain the actual mechanism found in the source (e.g. commit-reveal schemes, distributed shuffling, verifiable random functions, threshold cryptography, or whatever is actually there) in plain language, including what attack it defends against and what it doesn't.
- How peer discovery/bootstrap works, and what happens when the network partly disagrees about who's in the network.
- How message ordering, duplicate messages, or out-of-order delivery are handled (or aren't) — gossip protocols routinely have to deal with this.
- Concurrency primitive choices (channels vs. mutexes vs. atomics) at the specific places they're used, not in the abstract.
- Any serialization format chosen for network messages, and why.
- Error handling conventions (Go's explicit `error` returns vs. panics vs. custom error types) and where each is used.
- Any decisions specific to the top-level wiring file (startup order, shutdown/cleanup, configuration, how a node's identity/keys are established).

### 7. Data structures used, and why each was the right choice
A table or organized list: structure → where used → why chosen over the obvious alternative. Include the concrete Go types actually used (structs, maps, slices, channels, sync primitives) for at least: peer/connection tracking, table/game state, the gossip message log or dedup mechanism (if present), and anything cryptographic (keys, commitments, signatures) if present.

### 8. Problems solved — a standalone "hard parts" section
Pull together the genuinely tricky problems this specific codebase had to solve, grounded in the real source, and how it solved them. Likely candidates (only include what's actually there): fair, verifiable card dealing without a trusted dealer; reaching agreement on hand outcomes across untrusted peers; detecting and handling a disconnected or cheating peer mid-hand; avoiding duplicate/replayed gossip messages; network partition or peer churn; preventing a peer from seeing cards it shouldn't. For each, a short "the problem" / "the fix" pair, same format as before.

### 9. What this system does NOT handle
Be honest and specific, based only on what you actually see (or don't see) in the code — don't guess or assume based on what a "typical" decentralized poker project might lack. Check things like: Byzantine fault tolerance guarantees (or lack thereof), what happens under a network partition, whether there's any real economic/payout settlement or if chips are purely in-memory, whether there's persistence across restarts, what happens if a majority of peers collude, rate limiting / spam protection on the gossip layer, and anything else you can confirm is genuinely absent from the source.

### 10. Likely interview questions and strong answers
Write 8-12 realistic interview questions a systems/backend/distributed-systems engineer might ask about this project, each with a concise, confident model answer drawing on everything above. Mix conceptual questions (e.g. "how do you prevent a peer from stacking the deck in their favor?", "why gossip instead of a leader-based consensus protocol here?", "what happens if two peers disagree about the outcome of a hand?") with "what would you change" / "how would this break under load or malicious actors" style questions.

## Style requirements — read carefully, this matters as much as the content

- Write in the same plain, simple, non-jargon-first style as my slices: explain the "why" in plain language before or alongside any technical term, the way you'd explain it to someone who understands general programming and Go syntax but is newer to distributed systems and peer-to-peer networking concepts specifically. Don't assume familiarity with gossip protocols, goroutine/channel idioms, cryptographic commit-reveal schemes, Byzantine fault tolerance, etc. — earn every term before using it freely.
- Prefer short, clear sentences and concrete language over dense academic phrasing. No filler, no restating the obvious, no marketing-speak ("robust", "powerful", "seamless") — just clear explanation.
- Every diagram must be a Mermaid diagram (`flowchart`/`graph`, `sequenceDiagram`, or `classDiagram` as appropriate), in fenced ` ```mermaid ` code blocks, so it renders directly in a markdown viewer.
- The whole thing should be a single markdown document with clear headers matching the structure above, so I can read it top to bottom as one continuous story, but also jump to any section on its own.
- Where you quote or reference actual code, keep snippets short (a few lines max) and only when a snippet genuinely clarifies something a diagram or prose can't — this document is about the system, not a code listing.
- If anything in my notes files turns out to be inaccurate once you check it against the real source, correct it in this document rather than repeating the error, and don't call special attention to the correction — just get it right.
- If the source does not actually implement something a "typical" decentralized poker engine might be expected to have (e.g. no real cryptographic fairness mechanism, or gossip that isn't truly resilient to malicious peers), say so plainly in the relevant section rather than describing an idealized version of the system that isn't what's actually in the code.