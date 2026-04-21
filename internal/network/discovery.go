package network

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/p2p/discovery/mdns"
)

const PokerServiceTag = "p2p-poker-v1"

type PeerFoundFunc func(peer.AddrInfo)

type MDNSDiscovery struct {
	h host.Host
	svc mdns.Service
	mu sync.Mutex
	found []peer.AddrInfo
	onFound PeerFoundFunc
}

type mdnsNotifee struct {
	disc *MDNSDiscovery
}

func (n *mdnsNotifee) HandlePeerFound(pi peer.AddrInfo) {
	n.disc.mu.Lock()
	n.disc.found = append(n.disc.found, pi)
	cb := n.disc.onFound
	n.disc.mu.Unlock()
	if cb != nil {
		go cb(pi)
	}
}

func NewMDNSDiscovery(h host.Host, onFound PeerFoundFunc) (*MDNSDiscovery, error) {
	disc := &MDNSDiscovery{
		h: h,
		onFound: onFound,
	}
	svc := mdns.NewMdnsService(h, PokerServiceTag, &mdnsNotifee{disc: disc})
	if err := svc.Start(); err != nil {
		return nil, fmt.Errorf("")
	}
	disc.svc = svc 
	return disc, nil
}

func (d *MDNSDiscovery) Peers() []peer.AddrInfo {
	d.mu.Lock()
	defer d.mu.Unlock()
	out := make([]peer.AddrInfo, len(d.found))
	copy(out, d.found)
	return out
}

func (d *MDNSDiscovery) WaitForPeers(ctx context.Context, n int) error {
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return fmt.Errorf("")
		case <-ticker.C:
			d.mu.Lock()
			count := len(d.found)
			d.mu.Unlock() 
			if count >= n {
				return nil
			}
		}
	}
}

func (d *MDNSDiscovery) Close() error {
	if d.svc != nil {
		return d.svc.Close()
	}
	return nil
}