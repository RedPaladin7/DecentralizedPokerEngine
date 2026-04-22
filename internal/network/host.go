package network

import (
	"context"
	"crypto/ed25519"
	"fmt"

	libp2p "github.com/libp2p/go-libp2p"
	libp2pcrypto "github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/core/peerstore"
	"github.com/libp2p/go-libp2p/p2p/security/noise"
	"github.com/libp2p/go-libp2p/p2p/transport/tcp"

	ma "github.com/multiformats/go-multiaddr"
)

type PokerHost struct {
	Host host.Host
	Ed25519PK ed25519.PrivateKey
	PeerID string 
}

func NewPokerHost(ctx context.Context, listenAddr string, seed []byte) (*PokerHost, error) {
	var libPrivKey libp2pcrypto.PrivKey
	var rawEd ed25519.PrivateKey
	var err error 

	if len(seed) == 64 {
		rawEd = ed25519.PrivateKey(seed)
		libPrivKey, _, err = libp2pcrypto.KeyPairFromStdKey(rawEd)
		if err != nil {
			return nil, fmt.Errorf("")
		}
	} else {
		libPrivKey, _, err = libp2pcrypto.GenerateEd25519Key(nil)
		if err != nil {
			return nil, fmt.Errorf("")
		}
		raw, err := libPrivKey.Raw() 
		if err != nil {
			return nil, fmt.Errorf("")
		}
		rawEd = ed25519.PrivateKey(raw)
	}

	maddr, err := ma.NewMultiaddr(listenAddr)
	if err != nil {
		return nil, fmt.Errorf("")
	}

	h, err := libp2p.New(
		libp2p.Identity(libPrivKey),
		libp2p.ListenAddrs(maddr),
		libp2p.Security(noise.ID, noise.New),
		libp2p.Transport(tcp.NewTCPTransport),
		libp2p.DisableRelay(),
		libp2p.NATPortMap(),
	)
	if err != nil {
		return nil, fmt.Errorf("")
	}

	return &PokerHost{
		Host: h,
		Ed25519PK: rawEd,
		PeerID: h.ID().String(),
	}, nil
}

func (ph *PokerHost) Connect(ctx context.Context, addStr string) error {
	maddr, err := ma.NewMultiaddr(addStr)
	if err != nil {
		return fmt.Errorf("")
	}
	perrInfo, err := peer.AddrInfoFromP2pAddr(maddr)
	if err != nil {
		return fmt.Errorf("")
	}
	ph.Host.Peerstore().AddAddrs(perrInfo.ID, perrInfo.Addrs, peerstore.PermanentAddrTTL)
	if err := ph.Host.Connect(ctx, *perrInfo); err != nil {
		return fmt.Errorf("")
	}
	return nil
}

func (ph *PokerHost) Addrs() []string {
	suffix := fmt.Sprintf("/p2p/%s", ph.Host.ID())
	out := make([]string, 0, len(ph.Host.Addrs()))
	for _, addr := range ph.Host.Addrs() {
		out = append(out, addr.String()+suffix)
	}
	return out
}

func (ph *PokerHost) ConnectedPeers() []peer.ID {
	return ph.Host.Network().Peers()
}

func (ph *PokerHost) Close() error {
	return ph.Host.Close()
}