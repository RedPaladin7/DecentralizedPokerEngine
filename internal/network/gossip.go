package network

import (
	"context"
	"fmt"
	"sync"

	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/peer"
)

func topicName(tableID string) string {
	return "poker/table/" + tableID
}

func heartbeatTopicName(tableID string) string {
	return "poker/heartbeat/" + tableID
}

type GossipManager struct {
	ps *pubsub.PubSub
	tableID string 
	tableTopic *pubsub.Topic
	heartbeatTopic *pubsub.Topic
	tableSub *pubsub.Subscription
	heartbeatSub *pubsub.Subscription
	mu sync.Mutex
	seqNums map[string]int64
}

func NewGossipManager(ctx context.Context, h host.Host, tableID string) (*GossipManager, error) {
	ps, err := pubsub.NewGossipSub(ctx, h)
	if err != nil {
		return nil, fmt.Errorf("")
	}
	tt, err := ps.Join(topicName(tableID))
	if err != nil {
		return nil, fmt.Errorf("")
	}
	ht, err := ps.Join(heartbeatTopicName(tableID))
	if err != nil {
		return nil, fmt.Errorf("")
	}	
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

func (gm *GossipManager) NewTableMessage(ctx context.Context) ([]byte, peer.ID, error)  {
	mgs, err := gm.tableSub.Next(ctx)
	if err != nil {
		return nil, "", fmt.Errorf("")
	}
	return mgs.Data, mgs.ReceivedFrom, nil
}

func (gm *GossipManager) NewHeartbeatMessage(ctx context.Context) ([]byte, peer.ID, error) {
	mgs, err := gm.heartbeatSub.Next(ctx)
	if err != nil {
		return nil, "", fmt.Errorf("")
	}
	return mgs.Data, mgs.ReceivedFrom, nil
}

func (gm *GossipManager) CheckAndUpdateSeq(senderID string, seq int64) error {
	gm.mu.Lock()
	defer gm.mu.Unlock()
	last, exists := gm.seqNums[senderID]
	if exists && seq <= last {
		return fmt.Errorf("")
	}
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