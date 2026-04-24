package network
// "/internal/network/discovery.go"
// sending messages to all peers simultaneously

import (
	"context"
	"fmt"
	"sync"

	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/peer"
)

// important distinction for signature: 
// for direct messages signature verification is not requried, noise already ensures the stream is coming from the correct person
// for gossip, signature verification is required because the message can be delivered through an intermediate peer in mesh
// we must ensure that the original sender sent it
func topicName(tableID string) string {
	return "poker/table/" + tableID
}

func heartbeatTopicName(tableID string) string {
	return "poker/heartbeat/" + tableID
}

// pub sub architecture
// messages broadcasted to topic, listeners can subscribe to it
// mesh -> not every peer is connected to everyone else
type GossipManager struct {
	ps *pubsub.PubSub // Gossip sub engine
	tableID string 
	tableTopic *pubsub.Topic
	heartbeatTopic *pubsub.Topic
	tableSub *pubsub.Subscription
	heartbeatSub *pubsub.Subscription
	mu sync.Mutex
	seqNums map[string]int64 // last accepted seq per sender, solely for replay protection
}

func NewGossipManager(ctx context.Context, h host.Host, tableID string) (*GossipManager, error) {
	ps, err := pubsub.NewGossipSub(ctx, h)
	if err != nil {
		return nil, fmt.Errorf("")
	}
	// joining topics so we can publish to them
	tt, err := ps.Join(topicName(tableID))
	if err != nil {
		return nil, fmt.Errorf("")
	}
	ht, err := ps.Join(heartbeatTopicName(tableID))
	if err != nil {
		return nil, fmt.Errorf("")
	}	
	// subscribing to topics to get messages from them
	ts, err := tt.Subscribe()
	if err != nil {
		return nil, fmt.Errorf("")
	}
	hs, err := ht.Subscribe()
	if err != nil {
		return nil, fmt.Errorf("")
	}
	return &GossipManager{
		ps: ps,
		tableID: tableID,
		tableTopic: tt,
		heartbeatTopic: ht,
		tableSub: ts,
		heartbeatSub: hs,
		seqNums: make(map[string]int64),
	}, nil
}

// publishing messages
func (gm *GossipManager) Publish(ctx context.Context, frame []byte) error {
	if err := gm.tableTopic.Publish(ctx, frame); err != nil {
		return fmt.Errorf("")
	}
	return nil
}

func (gm *GossipManager) PublishHeartbeat(ctx context.Context, frame []byte) error {
	if err := gm.heartbeatTopic.Publish(ctx, frame); err != nil {
		return fmt.Errorf("")
	}
	return nil
}

// receiving messages
func (gm *GossipManager) NewTableMessage(ctx context.Context) ([]byte, peer.ID, error)  {
	msg, err := gm.tableSub.Next(ctx)
	if err != nil {
		return nil, "", fmt.Errorf("")
	}
	// received from is not original sender, it is direct neighbor who you get the message from in the mesh
	return msg.Data, msg.ReceivedFrom, nil
}

func (gm *GossipManager) NewHeartbeatMessage(ctx context.Context) ([]byte, peer.ID, error) {
	msg, err := gm.heartbeatSub.Next(ctx)
	if err != nil {
		return nil, "", fmt.Errorf("")
	}
	return msg.Data, msg.ReceivedFrom, nil
}

func (gm *GossipManager) CheckAndUpdateSeq(senderID string, seq int64) error {
	gm.mu.Lock()
	defer gm.mu.Unlock()
	last, exists := gm.seqNums[senderID]
	if exists && seq <= last {
		return fmt.Errorf("")
	}
	// new message must have greater seq num than the last received
	gm.seqNums[senderID] = seq
	return nil
}

func (gm *GossipManager) Peers() []peer.ID {
	return gm.tableTopic.ListPeers()
}

func (gm *GossipManager) Close() error {
	gm.tableSub.Cancel()
	gm.heartbeatSub.Cancel()
	_ = gm.tableTopic.Close()
	_ = gm.heartbeatTopic.Close()
	return nil
}