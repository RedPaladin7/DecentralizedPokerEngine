package network

// "/internal/network/host.go"
// Network foundation for peer to peer comm

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
	Host host.Host // network and connection manager, peerstore, mux (multiple independant streams over single connection)
	Ed25519PK ed25519.PrivateKey // privvate key for signing message (verification not encryption)
	PeerID string // derived from public key, permanent unique address on p2p network
}

func NewPokerHost(ctx context.Context, listenAddr string, seed []byte) (*PokerHost, error) {
	var libPrivKey libp2pcrypto.PrivKey // private key to be passed to host 
	var rawEd ed25519.PrivateKey // also the same key, used in the envelope
	var err error 

	if len(seed) == 64 {
		rawEd = ed25519.PrivateKey(seed) // generating already existing key if you want to restore
		libPrivKey, _, err = libp2pcrypto.KeyPairFromStdKey(rawEd)
		if err != nil {
			return nil, fmt.Errorf("")
		}
	} else {
		libPrivKey, _, err = libp2pcrypto.GenerateEd25519Key(nil) // generating fresh key pair
		if err != nil {
			return nil, fmt.Errorf("")
		}
		raw, err := libPrivKey.Raw() 
		if err != nil {
			return nil, fmt.Errorf("")
		}
		rawEd = ed25519.PrivateKey(raw)
	}

	maddr, err := ma.NewMultiaddr(listenAddr) // self describing, composable address format
	// along with address also tells the name of the peer and protocol to use
	// also tells you who you expect to find at the address you are connecting to
	if err != nil {
		return nil, fmt.Errorf("")
	}

	// creating the host struct
	h, err := libp2p.New(
		libp2p.Identity(libPrivKey),
		libp2p.ListenAddrs(maddr),
		libp2p.Security(noise.ID, noise.New), // noise protocol, encryption and authentication of connection between 2 peers
		libp2p.Transport(tcp.NewTCPTransport),
		libp2p.DisableRelay(), // direction comm not middle men
		libp2p.NATPortMap(), // asking home router to open hole in network firewall to let players from internet connect
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
	maddr, err := ma.NewMultiaddr(addStr) // string to multiaddr object
	if err != nil {
		return fmt.Errorf("")
	}
	peerInfo, err := peer.AddrInfoFromP2pAddr(maddr) // splits multiaddr into addrinfo
	// seperates peer id from addr+port
	if err != nil {
		return fmt.Errorf("")
	}
	// adds to local address book, maps peer id to address
	ph.Host.Peerstore().AddAddrs(peerInfo.ID, peerInfo.Addrs, peerstore.PermanentAddrTTL)
	// connecting to the peer
	if err := ph.Host.Connect(ctx, *peerInfo); err != nil {
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