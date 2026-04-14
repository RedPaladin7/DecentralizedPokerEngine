package crypto

import "math/big"


type ShuffleStep struct {
	PlayerID string 
	InputDeck []*big.Int
	Outputdeck []*big.Int
	Permutation []int 
	Commitment *Commitment
}

type ShuffleProtocol struct {
	P *big.Int
	SessionID []byte 
	NumCards int 
}

func NewShuffleProtocol(p *big.Int, sessionID []byte) *ShuffleProtocol {
	return &ShuffleProtocol{P: p, SessionID: sessionID, NumCards: 52}
}