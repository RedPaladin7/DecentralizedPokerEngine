package chain

import (
	"crypto/ecdsa"
	"fmt"
	"math/big"
	"time"
)

type Address [20]byte

func (a Address) Hex() string {
	return fmt.Sprintf("0x%x", a[:])
}

type Hash [32]byte 

type Wi = big.Int

type TxHash = Hash 

type ChainConfig struct {
	RECURL string 
	ContractAddress string 
	PrivateKey *ecdsa.PrivateKey
	ChainID *big.Int
	GasLimit uint64 
	ConfirmTimeout time.Duration
}