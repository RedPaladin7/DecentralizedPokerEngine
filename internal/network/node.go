package network

// "/internal/network/node.go"
// representation of complete player

import (
	"context"
	"crypto/ed25519"
	"encoding/binary"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	pokercrypto "github.com/RedPaladin7/DecentralizedPokerEngine/internal/crypto"
	"github.com/RedPaladin7/DecentralizedPokerEngine/internal/game"
	"github.com/libp2p/go-libp2p/core/peer"
	"google.golang.org/protobuf/proto"
)

// Node represents a complete player in the P2P network.
// It is both a client (sends messages) and a server (receives messages).
type Node struct {
	Host      *PokerHost     // libp2p host — manages connections, peerstore, mux
	Gossip    *GossipManager // GossipSub publisher/subscriber
	Lobby     *Lobby         // tracks who has joined and who is ready
	Log       *Gamelog       // append-only per-hand evidence log
	Discovery *MDNSDiscovery // LAN peer discovery

	tableID    string
	playerName string
	buyIn      int64
	sraKey     *pokercrypto.SRAKey

	seq           int64 // monotonic outbound message counter (atomic)
	joinTimestamp int64 // first local JOIN_TABLE stamp; reused on rebroadcast
	mu            sync.RWMutex
	peers         map[string]ed25519.PublicKey // cached public keys for signature verification

	started bool

	bootstrapPeers []string

	// Callbacks — set these BEFORE calling Start().
	OnJoinTable      func(*JoinTable, string)
	OnPlayerReady    func(*PlayerReady, string)
	OnShuffleStep    func(*ShuffleStep)
	OnPartialDecrypt func(*PartialDecrypt)
	OnPlayerAction   func(*PlayerAction)
	OnGameStateSync  func(*GameStateSync)
	OnHeartbeat      func(*Heartbeat, string)
	OnTimeoutVote    func(*TimeoutVote)
	OnHandResult     func(*HandResult)
	OnEquivocation   func(senderID string, envA, envB *Envelope)
	OnKeyShare       func(msg *KeyShare, viaGossip bool)

	streamPool *StreamPool
}

// NewNode constructs a Node but does NOT start the network.
// Call Start() after wiring all OnXxx callbacks.
//
// FIX: the original code passed maxSeats=9 hardcoded to NewLobby regardless
// of the configured value, making Lobby.Count() >= maxSeats never trigger for
// tables smaller than 9.  Now uses the actual maxSeats parameter.
func NewNode(
	ctx context.Context,
	tableID, playerName string,
	buyIn int64,
	maxSeats int,
	sraKey *pokercrypto.SRAKey,
	listenAddr string,
	seed []byte,
	bootstrapPeers ...string,
) (*Node, error) {
	if maxSeats < 2 || maxSeats > 9 {
		maxSeats = 6
	}
	ph, err := NewPokerHost(ctx, listenAddr, seed)
	if err != nil {
		return nil, fmt.Errorf("NewPokerHost: %w", err)
	}
	gm, err := NewGossipManager(ctx, ph.Host, tableID)
	if err != nil {
		ph.Close()
		return nil, fmt.Errorf("NewGossipManager: %w", err)
	}
	return &Node{
		Host:           ph,
		Gossip:         gm,
		Lobby:          NewLobby(tableID, maxSeats), // FIX: was hardcoded 9
		Log:            NewGameLog(tableID, 0),
		tableID:        tableID,
		playerName:     playerName,
		buyIn:          buyIn,
		sraKey:         sraKey,
		peers:          make(map[string]ed25519.PublicKey),
		bootstrapPeers: bootstrapPeers,
		streamPool:     NewStreamPool(ph.Host),
	}, nil
}

// Start begins MDNS discovery, the GossipSub table and heartbeat receive
// loops, and the equivocation scanner.
//
// IMPORTANT: Set all OnXxx callbacks before calling Start().
// The receive goroutines start immediately and will silently drop any message
// whose callback is nil at the time the message arrives.
func (n *Node) Start(ctx context.Context) error {
	n.mu.Lock()
	if n.started {
		n.mu.Unlock()
		return fmt.Errorf("node already started")
	}
	n.started = true
	n.mu.Unlock()

	// Register the direct-stream handler for partial decrypts.
	RegisterProtocolHandler(n.Host.Host, func(env *Envelope, from peer.ID) {
		_ = n.Log.Append(env)
		switch env.Type {
		case MsgType_PARTIAL_DECRYPT:
			if n.OnPartialDecrypt == nil {
				return
			}
			msg := &PartialDecrypt{}
			if proto.Unmarshal(env.Payload, msg) == nil {
				n.OnPartialDecrypt(msg)
			}
		case MsgType_KEY_SHARE:
			if n.OnKeyShare == nil {
				return
			}
			msg, err := UnmarshalKeyShare(env.Payload)
			if err == nil {
				n.OnKeyShare(msg, false)
			}
		}
	})

	// Start MDNS so peers on the same LAN find each other automatically.
	disc, err := NewMDNSDiscovery(n.Host.Host, func(pi peer.AddrInfo) {
		n.Host.Host.Peerstore().AddAddrs(pi.ID, pi.Addrs, 10*time.Minute)
		_ = n.Host.Host.Connect(ctx, pi)
	})
	if err != nil {
		return fmt.Errorf("MDNSDiscovery: %w", err)
	}
	n.Discovery = disc

	// Connect to any explicitly provided bootstrap peers.
	for _, addr := range n.bootstrapPeers {
		if err := n.Host.Connect(ctx, addr); err != nil {
			_ = err // non-fatal: mesh will form once both sides are up
		}
	}

	go n.receiveLoop(ctx)
	go n.heartbeatReceiveLoop(ctx)
	go n.equivocationScanLoop(ctx)
	return nil
}

// receiveLoop reads from the GossipSub table topic and dispatches messages.
func (n *Node) receiveLoop(ctx context.Context) {
	for {
		data, _, err := n.Gossip.NewTableMessage(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			continue
		}
		n.dispatch(data)
	}
}

// heartbeatReceiveLoop reads poker/heartbeat/<table> so last-seen is refreshed
// without sharing the table-topic seq watermark.
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

// decodeRemoteEnvelope verifies the signature and drops self-echo.
func (n *Node) decodeRemoteEnvelope(data []byte) *Envelope {
	env, err := DecodeEnvelope(data, n.lookupPubKey)
	if err != nil {
		return nil
	}
	if env.SenderId == n.Host.PeerID {
		return nil
	}
	return env
}

// dispatchHeartbeat handles HEARTBEAT envelopes from the heartbeat topic.
// Replay protection uses hbSeqNums. Beats are not appended to Gamelog.
func (n *Node) dispatchHeartbeat(data []byte) {
	env := n.decodeRemoteEnvelope(data)
	if env == nil || env.Type != MsgType_HEARTBEAT {
		return
	}
	if err := n.Gossip.CheckAndUpdateHeartbeatSeq(env.SenderId, env.Seq); err != nil {
		return
	}
	if n.OnHeartbeat == nil {
		return
	}
	msg := &Heartbeat{}
	if proto.Unmarshal(env.Payload, msg) != nil {
		return
	}
	n.OnHeartbeat(msg, env.SenderId)
}

// dispatch decodes a table-topic envelope and calls the appropriate OnXxx callback.
func (n *Node) dispatch(data []byte) {
	env := n.decodeRemoteEnvelope(data)
	if env == nil {
		return
	}
	// Heartbeats belong on the heartbeat topic. Ignore them here so they
	// cannot advance the table seq watermark or enter Gamelog.
	if env.Type == MsgType_HEARTBEAT {
		return
	}
	// Replay protection: drop old or duplicate sequence numbers.
	if err := n.Gossip.CheckAndUpdateSeq(env.SenderId, env.Seq); err != nil {
		return
	}
	_ = n.Log.Append(env)

	switch env.Type {
	case MsgType_JOIN_TABLE:
		msg := &JoinTable{}
		if proto.Unmarshal(env.Payload, msg) != nil {
			return
		}
		if err := n.Lobby.HandleJoin(msg, env.SenderId, env.Timestamp); err != nil {
			return
		}
		if n.OnJoinTable != nil {
			n.OnJoinTable(msg, env.SenderId)
		}

	case MsgType_PLAYER_READY:
		msg := &PlayerReady{}
		if proto.Unmarshal(env.Payload, msg) != nil {
			return
		}
		_ = n.Lobby.HandleReady(msg, env.SenderId)
		if n.OnPlayerReady != nil {
			n.OnPlayerReady(msg, env.SenderId)
		}

	case MsgType_SHUFFLE_STEP:
		if n.OnShuffleStep == nil {
			return
		}
		msg := &ShuffleStep{}
		if proto.Unmarshal(env.Payload, msg) != nil {
			return
		}
		n.OnShuffleStep(msg)

	case MsgType_PARTIAL_DECRYPT:
		if n.OnPartialDecrypt == nil {
			return
		}
		msg := &PartialDecrypt{}
		if proto.Unmarshal(env.Payload, msg) != nil {
			return
		}
		n.OnPartialDecrypt(msg)

	case MsgType_PLAYER_ACTION:
		if n.OnPlayerAction == nil {
			return
		}
		msg := &PlayerAction{}
		if proto.Unmarshal(env.Payload, msg) != nil {
			return
		}
		n.OnPlayerAction(msg)

	case MsgType_GAME_STATE_SYNC:
		if n.OnGameStateSync == nil {
			return
		}
		msg := &GameStateSync{}
		if proto.Unmarshal(env.Payload, msg) != nil {
			return
		}
		n.OnGameStateSync(msg)

	case MsgType_TIMEOUT_VOTE:
		if n.OnTimeoutVote == nil {
			return
		}
		msg := &TimeoutVote{}
		if proto.Unmarshal(env.Payload, msg) != nil {
			return
		}
		n.OnTimeoutVote(msg)

	case MsgType_HAND_RESULT:
		if n.OnHandResult == nil {
			return
		}
		msg := &HandResult{}
		if proto.Unmarshal(env.Payload, msg) != nil {
			return
		}
		n.OnHandResult(msg)

	case MsgType_KEY_SHARE:
		if n.OnKeyShare == nil {
			return
		}
		msg, err := UnmarshalKeyShare(env.Payload)
		if err != nil {
			return
		}
		n.OnKeyShare(msg, true)
	}
}

func (n *Node) nextSeq() int64 {
	return atomic.AddInt64(&n.seq, 1)
}

func (n *Node) publish(ctx context.Context, msgType MsgType, payload []byte) error {
	env := NewEnvelope(msgType, n.Host.PeerID, n.nextSeq(), payload)
	frame, err := EncodeEnvelope(env, n.Host.Ed25519PK)
	if err != nil {
		return fmt.Errorf("EncodeEnvelope: %w", err)
	}
	return n.Gossip.Publish(ctx, frame)
}

func (n *Node) equivocationScanLoop(ctx context.Context) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			adapter := NewGameLogFaultAdaptor(n.Log)
			senderID, _, _ := adapter.DetectEquivocation()
			if senderID != "" && n.OnEquivocation != nil {
				n.OnEquivocation(senderID, nil, nil)
			}
		}
	}
}

// ── Broadcast helpers ─────────────────────────────────────────────────────────

func (n *Node) BroadcastJoin(ctx context.Context, handNum int64) error {
	var eBytes []byte
	if n.sraKey != nil {
		eBytes = n.sraKey.PublicKey().Bytes()
	}
	msg := &JoinTable{
		TableId:      n.tableID,
		PlayerName:   n.playerName,
		BuyIn:        n.buyIn,
		SraPubKeyE:   eBytes, // nil/empty when --no-crypto
		SessionNonce: []byte(n.Host.PeerID),
	}
	b, err := proto.Marshal(msg)
	if err != nil {
		return fmt.Errorf("marshal JoinTable: %w", err)
	}
	if n.joinTimestamp == 0 {
		n.joinTimestamp = time.Now().UnixMilli()
	}
	selfTimestamp := n.joinTimestamp
	_ = n.Lobby.HandleJoin(msg, n.Host.PeerID, selfTimestamp)

	env := NewEnvelope(MsgType_JOIN_TABLE, n.Host.PeerID, n.nextSeq(), b)
	env.Timestamp = selfTimestamp
	frame, err := EncodeEnvelope(env, n.Host.Ed25519PK)
	if err != nil {
		return fmt.Errorf("EncodeEnvelope join: %w", err)
	}
	return n.Gossip.Publish(ctx, frame)
}

func (n *Node) BroadcastReady(ctx context.Context, handNum int64) error {
	msg := &PlayerReady{TableId: n.tableID, HandNum: handNum}
	b, err := proto.Marshal(msg)
	if err != nil {
		return fmt.Errorf("marshal PlayerReady: %w", err)
	}
	_ = n.Lobby.HandleReady(msg, n.Host.PeerID)
	return n.publish(ctx, MsgType_PLAYER_READY, b)
}

func (n *Node) BroadcastShuffleStep(ctx context.Context, handNum int64, step *pokercrypto.ShuffleStep) error {
	msg := &ShuffleStep{
		TableId:         n.tableID,
		HandNum:         handNum,
		PlayerId:        n.Host.PeerID,
		Deck:            DeckToWire(step.OutputDeck),
		CommitmentHash:  step.Commitment.Hash,
		CommitmentNonce: step.Commitment.Nonce,
	}
	b, err := proto.Marshal(msg)
	if err != nil {
		return fmt.Errorf("marshal ShuffleStep: %w", err)
	}
	return n.publish(ctx, MsgType_SHUFFLE_STEP, b)
}

// BroadcastShuffleMessage publishes a library ShuffleMessage. It never has
// access to a permutation — only output deck + commitment go on the wire.
func (n *Node) BroadcastShuffleMessage(ctx context.Context, msg *pokercrypto.ShuffleMessage) error {
	if msg == nil {
		return nil
	}
	limb := 0
	if len(msg.OutputDeck) > 0 && msg.OutputDeck[0] != nil {
		limb = len(msg.OutputDeck[0].Bytes())
	}
	fmt.Printf("[crypto] shuffle hand=%d player=%s deck_cards=%d limb_bytes=%d\n",
		msg.HandNum, msg.PlayerID, len(msg.OutputDeck), limb)
	pb := ShuffleMessageToWire(n.tableID, msg)
	b, err := proto.Marshal(pb)
	if err != nil {
		return fmt.Errorf("marshal ShuffleMessage: %w", err)
	}
	return n.publish(ctx, MsgType_SHUFFLE_STEP, b)
}

func (n *Node) BroadcastPartialDecrypt(ctx context.Context, handNum int64, pd *pokercrypto.PartialDecryption) error {
	msg := PartialDecryptToWire(n.tableID, handNum, pd)
	b, err := proto.Marshal(msg)
	if err != nil {
		return fmt.Errorf("marshal PartialDecrypt: %w", err)
	}
	return n.publish(ctx, MsgType_PARTIAL_DECRYPT, b)
}

func (n *Node) BroadcastAction(ctx context.Context, handNum int64, a game.Action, actionSeq int64) error {
	playerID := a.PlayerID
	if playerID == "" {
		playerID = n.Host.PeerID
	}
	msg := &PlayerAction{
		TableId:  n.tableID,
		HandNum:  handNum,
		PlayerId: playerID,
		Action:   int32(a.Type),
		Amount:   a.Amount,
		Seq:      actionSeq,
	}
	b, err := proto.Marshal(msg)
	if err != nil {
		return fmt.Errorf("marshal PlayerAction: %w", err)
	}
	return n.publish(ctx, MsgType_PLAYER_ACTION, b)
}

func (n *Node) BroadcastHeartbeat(ctx context.Context, handNum, hbSeq int64) error {
	msg := &Heartbeat{TableId: n.tableID, HandNum: handNum, Seq: hbSeq}
	b, err := proto.Marshal(msg)
	if err != nil {
		return fmt.Errorf("marshal Heartbeat: %w", err)
	}
	env := NewEnvelope(MsgType_HEARTBEAT, n.Host.PeerID, n.nextSeq(), b)
	frame, err := EncodeEnvelope(env, n.Host.Ed25519PK)
	if err != nil {
		return fmt.Errorf("EncodeEnvelope heartbeat: %w", err)
	}
	return n.Gossip.PublishHeartbeat(ctx, frame)
}

func (n *Node) BroadcastTimeoutVote(ctx context.Context, handNum int64, timeoutPeerID string) error {
	msg := &TimeoutVote{
		TableId:         n.tableID,
		HandNum:         handNum,
		VotingPlayerId:  n.Host.PeerID,
		TimeoutPlayerId: timeoutPeerID,
	}
	b, err := proto.Marshal(msg)
	if err != nil {
		return fmt.Errorf("marshal TimeoutVote: %w", err)
	}
	return n.publish(ctx, MsgType_TIMEOUT_VOTE, b)
}

func (n *Node) BroadcastHandResult(ctx context.Context, handNum int64, pots []*PotResult, stateRoot []byte) error {
	msg := &HandResult{TableId: n.tableID, HandNum: handNum, Pots: pots, StateRoot: stateRoot}
	b, err := proto.Marshal(msg)
	if err != nil {
		return fmt.Errorf("marshal HandResult: %w", err)
	}
	return n.publish(ctx, MsgType_HAND_RESULT, b)
}

func (n *Node) BroadcastStateSync(ctx context.Context, sync *GameStateSync) error {
	b, err := proto.Marshal(sync)
	if err != nil {
		return fmt.Errorf("marshal GameStateSync: %w", err)
	}
	return n.publish(ctx, MsgType_GAME_STATE_SYNC, b)
}

func (n *Node) BroadcastEquivocationEvidence(ctx context.Context, handNum int64, senderID string, envA, envB *Envelope) error {
	payloadA, err := proto.Marshal(envA)
	if err != nil {
		return fmt.Errorf("marshal envA: %w", err)
	}
	payloadB, err := proto.Marshal(envB)
	if err != nil {
		return fmt.Errorf("marshal envB: %w", err)
	}
	combined := make([]byte, 4+len(payloadA)+4+len(payloadB))
	binary.BigEndian.PutUint32(combined[0:4], uint32(len(payloadA)))
	copy(combined[4:], payloadA)
	binary.BigEndian.PutUint32(combined[4+len(payloadA):], uint32(len(payloadB)))
	copy(combined[4+len(payloadA)+4:], payloadB)
	return n.publish(ctx, MsgType_HAND_RESULT, combined)
}

// SendPeel always gossips the peel. Direct streams to other seats are
// best-effort; a failure must not fail the hand (duplicates are ignored).
func (n *Node) SendPeel(ctx context.Context, msg *pokercrypto.PeelMessage) error {
	if msg == nil {
		return nil
	}
	rb := 0
	if msg.Result != nil {
		rb = len(msg.Result.Bytes())
	}
	fmt.Printf("[crypto] peel hand=%d player=%s card=%d result_bytes=%d\n",
		msg.HandNum, msg.PlayerID, msg.CardIndex, rb)
	pd := PeelMessageToPD(msg)
	if err := n.BroadcastPartialDecrypt(ctx, msg.HandNum, pd); err != nil {
		return err
	}
	if n.Lobby == nil {
		return nil
	}
	for _, seat := range n.Lobby.Seats() {
		if seat.PlayerID == n.Host.PeerID {
			continue
		}
		pid, err := PeerIDFromString(seat.PlayerID)
		if err != nil {
			continue
		}
		_ = n.SendDirectPartialDecrypt(ctx, pid, msg.HandNum, pd)
	}
	return nil
}

func (n *Node) SendDirectPartialDecrypt(ctx context.Context, toPeerID peer.ID, handNum int64, pd *pokercrypto.PartialDecryption) error {
	msg := PartialDecryptToWire(n.tableID, handNum, pd)
	b, err := proto.Marshal(msg)
	if err != nil {
		return fmt.Errorf("marshal direct PartialDecrypt: %w", err)
	}
	env := NewEnvelope(MsgType_PARTIAL_DECRYPT, n.Host.PeerID, n.nextSeq(), b)
	frame, err := EncodeEnvelope(env, n.Host.Ed25519PK)
	if err != nil {
		return fmt.Errorf("EncodeEnvelope direct: %w", err)
	}
	return n.streamPool.Send(ctx, toPeerID, frame)
}

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
	b, err := MarshalKeyShare(msg)
	if err != nil {
		return fmt.Errorf("marshal direct KeyShare: %w", err)
	}
	env := NewEnvelope(MsgType_KEY_SHARE, n.Host.PeerID, n.nextSeq(), b)
	frame, err := EncodeEnvelope(env, n.Host.Ed25519PK)
	if err != nil {
		return fmt.Errorf("EncodeEnvelope direct KeyShare: %w", err)
	}
	return n.streamPool.Send(ctx, toPeerID, frame)
}

// ── Peer management ───────────────────────────────────────────────────────────

func (n *Node) RegisterPeer(peerID string, pub ed25519.PublicKey) {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.peers[peerID] = pub
}

func (n *Node) lookupPubKey(peerID string) (ed25519.PublicKey, error) {
	n.mu.RLock()
	if k, ok := n.peers[peerID]; ok {
		n.mu.RUnlock()
		return k, nil
	}
	n.mu.RUnlock()

	pid, err := PeerIDFromString(peerID)
	if err != nil {
		return nil, err
	}
	pub, err := ExtractEd25519PubKey(pid)
	if err != nil {
		return nil, nil // non-fatal: message will be dropped
	}
	n.mu.Lock()
	n.peers[peerID] = pub
	n.mu.Unlock()
	return pub, nil
}

func (n *Node) SetHandNum(handNum int64) {
	n.Log = NewGameLog(n.tableID, handNum)
}

func (n *Node) Close() error {
	if n.Discovery != nil {
		_ = n.Discovery.Close()
	}
	_ = n.Gossip.Close()
	return n.Host.Close()
}

func (n *Node) CloseHandStream() {
	n.streamPool.CloseAll()
}
