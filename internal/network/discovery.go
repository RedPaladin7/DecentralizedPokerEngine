package network
// "/internal/network/discovery.go"
// how peers find each other before actually connecting

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

// broadcasting on the network is anyone running p2p-poker-v1, if yes automatic response with address
type MDNSDiscovery struct {
	h host.Host // node who is doing the discovering
	svc mdns.Service // underlying mdns service
	mu sync.Mutex 
	found []peer.AddrInfo // list of already found peers
	onFound PeerFoundFunc // what to do when you come across a new peer
}

// libp2p requires to implement a notifee interface, done for mdns service starting order
type mdnsNotifee struct {
	disc *MDNSDiscovery
}

// required method of the interface
func (n *mdnsNotifee) HandlePeerFound(pi peer.AddrInfo) {
	n.disc.mu.Lock()
	n.disc.found = append(n.disc.found, pi)
	cb := n.disc.onFound
	n.disc.mu.Unlock()
	if cb != nil {
		go cb(pi) // peer found runs on its owne go routine
	}
}

func NewMDNSDiscovery(h host.Host, onFound PeerFoundFunc) (*MDNSDiscovery, error) {
	disc := &MDNSDiscovery{
		h: h,
		onFound: onFound,
	} // first creating inner struct without setting svc
	svc := mdns.NewMdnsService(h, PokerServiceTag, &mdnsNotifee{disc: disc}) // takes outer struct as argument
	if err := svc.Start(); err != nil {
		return nil, fmt.Errorf("")
	}
	disc.svc = svc // after service has started, set it as variable for inner struct
	return disc, nil
}

// returns copy of peers found
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
		select { // waits for multiple channels at once
		case <-ctx.Done(): // first channel, context cancelled or time out
			return fmt.Errorf("")
		case <-ticker.C: // second channel 100ms tick
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