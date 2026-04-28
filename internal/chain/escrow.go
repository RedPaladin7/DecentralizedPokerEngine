package chain

import (
	"context"
	"crypto/ecdsa"
	"math/big"
)

type EscrowManager struct {
	client *Client 
	localAddr Address
	localKey *ecdsa.PrivateKey
	tableID string 
	numSeats int 

	seats map[int]Address
}

func NewEscrowManager(client *Client, localAddr Address, localKey *ecdsa.PrivateKey, tableID string, numSeats int) *EscrowManager {
	return &EscrowManager{
		client: client,
		localAddr: localAddr,
		localKey: localKey,
		tableID: tableID,
		numSeats: numSeats,
		seats: make(map[int]Address),
	}
}

func (em *EscrowManager) Join(ctx context.Context, peerID string, weiAmount *big.Int) (*)