package chain

import (
	"context"
	"crypto/ecdsa"
	"fmt"
	"math/big"
	"time"

	contractabi "github.com/RedPaladin7/DecentralizedPokerEngine/internal/chain/abi"
)

type Address [20]byte

func (a Address) Hex() string {
	return fmt.Sprintf("0x%x", a[:])
}

type Hash [32]byte 

type Wi = big.Int

type TxHash = Hash 

type ChainConfig struct {
	RPCURL string 
	ContractAddress string 
	PrivateKey *ecdsa.PrivateKey
	ChainID *big.Int
	GasLimit uint64 
	ConfirmTimeout time.Duration
}

type PlayerRecord struct {
	Address Address
	PeerID string 
	BuyIn *big.Int
	Withdrawn bool 
	Slashed bool
}

type TxReceipt struct {
	TxHash TxHash
	Status uint64 
	GasUsed uint64 
	BlockNumber *big.Int
}

func DefaultConfig(contractAddr string, privKey *ecdsa.PrivateKey) ChainConfig {
	return ChainConfig{
		RPCURL: "http://127.0.0.1:8545",
		ContractAddress: contractAddr,
		PrivateKey: privKey,
		ChainID: big.NewInt(31337),
		GasLimit: 500_000,
		ConfirmTimeout: 30 *time.Second,
	}
}

type Client struct {
	cfg ChainConfig
	abi string 
	address string 
}

func NewClient(ctx context.Context, cfg ChainConfig) (*Client, error) {
	if cfg.RPCURL == "" {
		return nil, fmt.Errorf("")
	}
	if cfg.ContractAddress == "" {
		return nil, fmt.Errorf("")
	}
	return &Client{
		cfg: cfg,
		abi: contractabi.PokerEscrowABI,
		address: cfg.ContractAddress,
	}, nil
}

func (c *Client) Close() {

}

func (c *Client) PlayerCount(ctx context.Context) (uint64, error) {
	return 0, nil
}

func (c *Client) PlayerInfo(ctx context.Context, seat uint8) (*PlayerRecord, error) {
	return nil, fmt.Errorf("")
}

func (c *Client) StateRoot(ctx context.Context) (uint64, error) {
	return 0, nil
}

func (c *Client) JoinTable(ctx context.Context, peerID string, weiAmount *big.Int) (*TxReceipt, error) {
	if peerID == "" {
		return nil, fmt.Errorf("")
	}
	if weiAmount == nil || weiAmount.Sign() <= {
		return nil, fmt.Errorf("")
	}
	return &TxReceipt{
		Status: 1,
		GasUsed: 50000,
		BlockNumber: big.NewInt(1),
	}, nil
}

func (c *Client) ReportOutcome (ctx context.Context, payoutDeltas []*big.Int)