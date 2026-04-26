package fault

import (
	"context"
	"fmt"
	"sync"
	"time"
)

const DefaultHeartbeatInterval = 5 * time.Second
const DefaultHeartbeatTimeout = 15 * time.Second

type PeerStatus uint8 

const (
	PeerAlive PeerStatus = iota 
	PeerSuspect 
	PeerTimedOut 
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

	OnTimeout func(peerID string)
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
			wasAlive := pl.Status != PeerTimedOut
			pl.Status = PeerTimedOut
			pl.MissedBeats = int(elapsed / DefaultHeartbeatInterval)
			timedOut = append(timedOut, pl.PeerID)
			if wasAlive && hm.OnTimeout != nil {
				peerID := pl.PeerID
				go hm.OnTimeout(peerID)
			}
		} else if elapsed >= DefaultHeartbeatInterval {
			pl.Status = PeerSuspect
			pl.MissedBeats++
		}
	}
	return timedOut
}

func (hm *HeartbeatMonitor) MarkDisconnected(peerID string) {
	hm.mu.Lock()
	defer hm.mu.Unlock()
	if pl, ok := hm.peers[peerID]; ok {
		pl.Status = PeerDisconnected
	}
}

func (hm *HeartbeatMonitor) Status(peerID string) PeerStatus {
	hm.mu.RLock()
	defer hm.mu.RUnlock()
	if pl, ok := hm.peers[peerID]; ok {
		return pl.Status
	}
	return PeerTimedOut
}

func (hm *HeartbeatMonitor) AllStatuses() map[string]PeerLiveness {
	hm.mu.RLock()
	defer hm.mu.RUnlock()
	out := make(map[string]PeerLiveness, len(hm.peers))
	for k, v := range hm.peers {
		out[k] = *v
	}
	return out 
}

func (hm *HeartbeatMonitor) AlivePeers() []string {
	hm.mu.RLock()
	defer hm.mu.RUnlock()
	var out []string 
	for id, pl := range hm.peers {
		if pl.Status == PeerAlive || pl.Status == PeerSuspect {
			out = append(out, id)
		}
	}
	return out 
}

func (hm *HeartbeatMonitor) Run(ctx context.Context, interval time.Duration) {
	if interval == 0{
		interval = DefaultHeartbeatInterval
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <- ctx.Done():
			return 
		case <- ticker.C:
			hm.CheckTimeouts()
		}
	}
}

type HeartbeatSender struct {
	peerID string 
	interval time.Duration
	seq int64 
	send func(seq int64) error 
}

func NewHeartbeatSender(peerID string, interval time.Duration, send func(seq int64) error) *HeartbeatSender {
	if interval == 0{
		interval = DefaultHeartbeatInterval
	}
	return &HeartbeatSender{
		peerID: peerID,
		interval: interval,
		send: send,
	}
}

func (hs *HeartbeatSender) Run(ctx context.Context) error {
	ticker := time.NewTicker(hs.interval)
	defer ticker.Stop()
	for {
		select {
		case <- ctx.Done():
			return ctx.Err()
		case <- ticker.C:
			hs.seq++ 
			if err := hs.send(hs.seq); err != nil {
				return fmt.Errorf("")
			}
		}
	}
}
