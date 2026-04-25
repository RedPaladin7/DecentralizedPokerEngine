package fault

import (
	"sync"
	"time"
)

const DefaultHeartbeatInterval = 5 * time.Second
const DefaultHeartbeatTimeout = 15 * time.Second

type PeerStatus uint8 

const (
	PeerAlive PeerStatus = iota 
	PeerSuspect 
	PeerTimedout 
	PeerDisconnected
)

func(s PeerStatus) String() string {
	return [...]string{"Alive", "Suspect", "Timeout", "Disconnected"}[s]
}

type PeerLiveness struct {
	PeerID string 
	Status PeerStatus
	LastSeen time.Time 
	MissedBeats int
}

type HeartbeatMonitor struct {
	mu sync.RWMutex
	peers map[string]*PeerLiveness
	timeout time.Duration

	onTimeout func(peerID string)
}

func NewHeartbeatMonitor(timeout time.Duration) *HeartbeatMonitor {
	if timeout == 0 {
		timeout = DefaultHeartbeatTimeout
	}
	return &HeartbeatMonitor{
		peers: make(map[string]*PeerLiveness),
		timeout: timeout,
	}
}

func (hm *HeartbeatMonitor) RegisterPeer(peerID string){
	hm.mu.Lock()
	defer hm.mu.Unlock()
	if _, exists := hm.peers[peerID]; !exists {
		hm.peers[peerID] = &PeerLiveness{
			PeerID: peerID,
			Status: PeerAlive,
			LastSeen: time.Now(),
		}
	}
}

func (hm *HeartbeatMonitor) RecordHeartbeat(peerID string) {
	hm.mu.Lock()
	defer hm.mu.Unlock()

	pl, ok := hm.peers[peerID]
	if !ok {
		hm.peers[peerID] = &PeerLiveness{PeerID: peerID}
		pl = hm.peers[peerID]
	}
	pl.LastSeen = time.Now()
	pl.Status = PeerAlive 
	pl.MissedBeats = 0
}

func (hm *HeartbeatMonitor) CheckTimeouts() []string {
	hm.mu.Lock()
	defer hm.mu.Unlock()

	var timedOut []string 
	now := time.Now()

	for _, pl := range hm.peers {
		if pl.Status == PeerDisconnected {
			continue
		}
		elapsed := now.Sub(pl.LastSeen)
		if elapsed >= hm.timeout {
			wasAlive := pl.Status != PeerTimedout
			pl.Status = PeerTimedout
			pl.MissedBeats = int(elapsed / DefaultHeartbeatInterval)
			timedOut = append(timedOut, pl.PeerID)
			if wasAlive && hm.onTimeout != nil {
				peerID := pl.PeerID
				go hm.onTimeout(peerID)
			}
		} 
	}
}
