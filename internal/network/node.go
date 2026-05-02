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

// Every player is both client and server simultaneously
// Represents a complete player in the game
type Node struct {
	Host *PokerHost // host responsible for connections, holds private key for signing outgoing messages
	Gossip *GossipManager // subscribes to table and game topics, and publishes to them
	Lobby *Lobby // track of who has joined and who is ready
	Log *Gamelog // append only per hand evidence log
	Discovery *MDNSDiscovery // discovering new peers with the same tag

	tableID string 
	playerName string 
	buyIn int64 
	sraKey *pokercrypto.SRAKey // complete key pair for the player 

	seq int64 // monotonic outbound counter, atomic
	mu sync.RWMutex
	peers map[string]ed25519.PublicKey // public key of all the other players
	started bool 

	bootstrapPeers []string

	OnJoinTable func(*JoinTable, string)
	OnPlayerReady func(*PlayerReady, string)
	OnShuffleStep func(*ShuffleStep)
	OnPartialDecrypt func(*PartialDecrypt)
	OnPlayerAction func(*PlayerAction)
	OnGameStateSync func(*GameStateSync)
	OnHeartbeat func(*Heartbeat)
	OnTimeoutVote func(*TimeoutVote)
	OnHandResult func(*HandResult)

	OnEquivocation func(senderID string, envA, envB *Envelope)
}

func NewNode(
	ctx context.Context,
	tableID, playerName string,
	buyIn int64,
	sraKey *pokercrypto.SRAKey,
	listedAddr string,
	seed []byte,
	bootstrapPeers[] string,
) (*Node, error) {
	ph, err := NewPokerHost(ctx, listedAddr, seed)
	if err != nil {
		return nil, fmt.Errorf("")
	}
	gm, err := NewGossipManager(ctx, ph.Host, tableID)
	if err != nil {
		ph.Close()
		return nil, fmt.Errorf("")
	}

	// note we don't start the mdns service yet
	return &Node{
		Host: ph,
		Gossip: gm,
		Lobby: NewLobby(tableID, 9),
		Log: NewGameLog(tableID, 0),
		tableID: tableID,
		playerName: playerName,
		buyIn: buyIn,
		sraKey: sraKey,
		peers: make(map[string]ed25519.PublicKey),
		bootstrapPeers: bootstrapPeers,
	}, nil
}

func (n *Node) Start(ctx context.Context) error {
	n.mu.Lock()
	if n.started {
		n.mu.Unlock()
		return fmt.Errorf("")
	}
	n.started = true
	n.mu.Unlock()

	// setting up direct comm, takes hanlder func => what to do with direct message
	// direct comm only ever used for partial decrypts
	RegisterProtocolHandler(n.Host.Host, func(env *Envelope, from peer.ID){
		_ = n.Log.Append(env)
		if env.Type == MsgType_PARTIAL_DECRYPT && n.OnPartialDecrypt != nil {
			// calling partial decrypt callback 
			msg := &PartialDecrypt{}
			if proto.Unmarshal(env.Payload, msg) == nil {
				n.OnPartialDecrypt(msg)
			}
		}
	})

	// starting the mdns service
	disc, err := NewMDNSDiscovery(n.Host.Host, func(pi peer.AddrInfo) {
		n.Host.Host.Peerstore().AddAddrs(pi.ID, pi.Addrs, 10*time.Minute) 
		// establishing underlying tcp connection with all peers in the peerstore
		// all other connections are multiplexed over this
		_ = n.Host.Host.Connect(ctx, pi)
	})
	if err != nil {
		return fmt.Errorf("")
	}
	n.Discovery = disc 

	for _, addr := range n.bootstrapPeers {
		if err := n.Host.Connect(ctx, addr); err != nil {
			_ = err
		}
	}

	// starting the receivers loop
	go n.receiveLoop(ctx)
	go n.equivocationScanLoop(ctx)
	return nil
}

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

func (n *Node) dispatch(data []byte) {
	env, err := DecodeEnvelope(data, n.lookupPubKey)
	if err != nil {
		return 
	}

	if env.SenderId == n.Host.PeerID {
		return 
	}

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
		_ = n.Lobby.HandleJoin(msg, env.SenderId)
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

	case MsgType_HEARTBEAT:
		if n.OnHeartbeat == nil {
			return 
		}
		msg := &Heartbeat{}
		if proto.Unmarshal(env.Payload, msg) != nil {
			return 
		}
		n.OnHeartbeat(msg)

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
	}
}

func (n *Node) nextSeq() int64 {
	return atomic.AddInt64(&n.seq, 1)
}

func (n *Node) publish(ctx context.Context, msgType MsgType, payload []byte) error {
	env := NewEnvelope(msgType, n.Host.PeerID, n.nextSeq(), payload)
	frame, err := EncodeEnvelope(env, n.Host.Ed25519PK)
	if err != nil {
		return fmt.Errorf("")
	}
	return n.Gossip.Publish(ctx, frame)
}

func (n *Node) BroadcastEquivocationEvidence(ctx context.Context, handNum int64, senderID string, envA, envB *Envelope) error {
	payloadA, err := proto.Marshal(envA)
	if err != nil {
		return fmt.Errorf("")
	}
	payloadB, err := proto.Marshal(envB)
	if err != nil {
		return fmt.Errorf("")
	}
	combined := make([]byte, 4+len(payloadA)+4+len(payloadB))
	binary.BigEndian.PutUint32(combined[0:4], uint32(len(payloadA)))
	copy(combined[4:], payloadA)
	binary.BigEndian.PutUint32(combined[4+len(payloadA):], uint32(len(payloadB)))
	copy(combined[4+len(payloadA)+4:], payloadB)
	return n.publish(ctx, MsgType_HAND_RESULT, combined)
}

func (n *Node) equivocationScanLoop(ctx context.Context) {
	ticker := time.NewTicker(5 *time.Second)
	defer ticker.Stop() 
	for {
		select {
		case <- ctx.Done():
			return 
		case <- ticker.C:
			adapter := NewGameLogFaultAdaptor(n.Log)
			senderID, _, _ := adapter.DetectEquivocation()
			if senderID != "" && n.OnEquivocation != nil {
				n.OnEquivocation(senderID, nil, nil)
			}
		}
	}
}

func (n *Node) BroadcastJoin(ctx context.Context, handNum int64) error {
	msg := &JoinTable{
		TableId: n.tableID,
		PlayerName: n.playerName,
		BuyIn: n.buyIn,
		SraPubKeyE: n.sraKey.PublicKey().Bytes(),
		SessionNonce: []byte(n.Host.PeerID),
	}
	b, err := proto.Marshal(msg)
	if err != nil {
		return fmt.Errorf("")
	}
	return n.publish(ctx, MsgType_JOIN_TABLE, b)
}

func (n *Node) BroadcastReady(ctx context.Context, handNum int64) error {
	msg := &PlayerReady{
		TableId: n.tableID,
		HandNum: handNum,
	}
	b, err := proto.Marshal(msg)
	if err != nil {
		return fmt.Errorf("")
	}
	return n.publish(ctx, MsgType_PLAYER_READY, b)
}

func (n *Node) BroadcastShuffleStep(ctx context.Context, handNum int64, step *pokercrypto.ShuffleStep) error {
	msg := &ShuffleStep{
		TableId: n.tableID,
		HandNum: handNum,
		PlayerId: n.Host.PeerID,
		Deck: DeckToWire(step.OutputDeck),
		CommitmentHash: step.Commitment.Hash,
		CommitmentNonce: step.Commitment.Nonce,
	}
	b, err := proto.Marshal(msg)
	if err != nil {
		return fmt.Errorf("")
	}
	return n.publish(ctx, MsgType_SHUFFLE_STEP, b)
}

func (n *Node) BroadcastPartialDecrypt(ctx context.Context, handNum int64, pd *pokercrypto.PartialDecryption) error {
	msg := PartialDecryptToWire(n.tableID, handNum, pd)
	b, err := proto.Marshal(msg)
	if err != nil {
		return fmt.Errorf("")
	}
	return n.publish(ctx, MsgType_PARTIAL_DECRYPT, b)
}

func (n *Node) BroadcastAction(ctx context.Context, handNum int64, a game.Action, actionSeq int64) error {
	msg := &PlayerAction{
		TableId: n.tableID,
		HandNum: handNum,
		PlayerId: n.Host.PeerID,
		Action: int32(a.Type),
		Amount: a.Amount,
		Seq: actionSeq,
	}
	b, err := proto.Marshal(msg)
	if err != nil {
		return fmt.Errorf("")
	}
	return n.publish(ctx, MsgType_PLAYER_ACTION, b)
}

func (n *Node) BroadcastHeartbeat(ctx context.Context, handNum, hbSeq int64) error {
	msg := &Heartbeat{
		TableId: n.tableID,
		HandNum: handNum,
		Seq: hbSeq,
	}
	b, err := proto.Marshal(msg)
	if err != nil {
		return fmt.Errorf("")
	}
	env := NewEnvelope(MsgType_HEARTBEAT, n.Host.PeerID, n.nextSeq(), b)
	frame, err := EncodeEnvelope(env, n.Host.Ed25519PK)
	if err != nil {
		return err 
	}
	return n.Gossip.PublishHeartbeat(ctx, frame)
}

func (n *Node) BroadcastTimeoutVote(ctx context.Context, handNum int64, timeOutPeerID string) error {
	msg := &TimeoutVote{
		TableId: n.tableID,
		HandNum: handNum,
		VotingPlayerId: n.Host.PeerID,
		TimeoutPlayerId: timeOutPeerID,
	}
	b, err := proto.Marshal(msg)
	if err != nil {
		return fmt.Errorf("")
	}
	return n.publish(ctx, MsgType_TIMEOUT_VOTE, b)
}

func (n *Node) BroadcastHandResult(ctx context.Context, handNum int64, pots []*PotResult, stateRoot []byte) error {
	msg := &HandResult{
		TableId: n.tableID,
		HandNum: handNum,
		Pots: pots,
		StateRoot: stateRoot,
	}
	b, err := proto.Marshal(msg)
	if err != nil {
		return fmt.Errorf("")
		}
	return n.publish(ctx, MsgType_HAND_RESULT, b)
}

func (n *Node) BroadcastStateSync(ctx context.Context, sync *GameStateSync) error {
	b, err := proto.Marshal(sync)
	if err != nil {
		return fmt.Errorf("")
	}
	return n.publish(ctx, MsgType_GAME_STATE_SYNC, b)
}

func (n *Node) SendDirectPartialDecrypt(ctx context.Context, toPeerID peer.ID, handNum int64,pd *pokercrypto.PartialDecryption) error {
	msg := PartialDecryptToWire(n.tableID, handNum, pd)
	b, err := proto.Marshal(msg)
	if err != nil {
		return fmt.Errorf("")
	}
	env := NewEnvelope(MsgType_PARTIAL_DECRYPT, n.Host.PeerID, n.nextSeq(), b)
	frame, err := EncodeEnvelope(env, n.Host.Ed25519PK)
	if err != nil {
		return err 
	}
	return SendDirect(ctx, n.Host.Host, toPeerID, frame)
}

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
		return nil, nil
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