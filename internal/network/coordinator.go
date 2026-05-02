package network

import (
	"context"
	"fmt"
	"sync"

	pokercrypto "github.com/RedPaladin7/DecentralizedPokerEngine/internal/crypto"
	"github.com/RedPaladin7/DecentralizedPokerEngine/internal/game"
)

type HandCoordinator struct {
	mu sync.RWMutex
	node *Node
	lobby *Lobby
	handNum int64 
}

func NewHandCoordinator(node *Node, lobby *Lobby, handNum int64) *HandCoordinator {
	return &HandCoordinator{
		node: node,
		lobby: lobby,
		handNum: handNum,
	}
}

func (hc *HandCoordinator) RunHand(ctx context.Context, dealerIdx int, sb, bb int64) (*game.GameState, *game.Machine, *pokercrypto.CryptoGame, error) {
	if err := hc.lobby.WaitReady(ctx); err != nil {
		return nil, nil, nil, fmt.Errorf("")
	}
	playerIDs := hc.lobby.CanonicalPlayerOrder()
	nonce := hc.lobby.SessionNonce()

	seats := hc.lobby.Seats()
	players := make([]*game.Player, len(seats))
	for i, s := range seats {
		players[i] = game.NewPlayer(s.PlayerID, s.PlayerName, s.BuyIn)
	}
	cg, err := pokercrypto.NewCryptoGame(playerIDs, nonce)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("")
	}
	if err := cg.RunShuffle(); err != nil {
		return nil, nil, nil, fmt.Errorf("")
	}

	gs := game.NewGameState(hc.node.tableID, int(hc.handNum), players, dealerIdx, sb, bb)

	if err := cg.DealToEngine(gs); err != nil {
		return nil, nil, nil, fmt.Errorf("")
	}

	m := game.NewMachine(gs, nil)
	if err := m.StartHandCrypto(); err != nil {
		return nil, nil, nil, fmt.Errorf("")
	}
	hc.lobby.SetPlaying()
	return gs, m, cg, nil
}