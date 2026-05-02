package chain

import (
	"context"
	"crypto/ecdsa"
	"fmt"
	"math/big"
	"strings"
	"time"

	contractabi "github.com/RedPaladin7/DecentralizedPokerEngine/internal/chain/abi"
)

const (
	TableStateOpen uint8 = 0
	TableStatePlaying uint8 = 1
	TableStateSettled uint8 = 2
	TableStateDispted uint8 = 3
	TableStateAbandoned uint8 = 4
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

func (c *Client) Deploy(ctx context.Context, tableID string, maxSeats uint8) (Address, *TxReceipt, error) {
	if tableID == "" {
		return Address{}, nil, fmt.Errorf("")
	}
	if maxSeats < 2  || maxSeats > 9{
		return Address{}, nil, fmt.Errorf("")
	}
	var addr Address
	copy(addr[:], []byte(tableID)[:min(20, len(tableID))])
	return addr, &TxReceipt{Status: 1, GasUsed: 800_000, BlockNumber: big.NewInt(1)}, nil
}

func (c *Client) Close() {

}

func (c *Client) TableState(ctx context.Context) (uint8, error) {
	return TableStatePlaying, nil
}

func (c *Client) PlayerCount(ctx context.Context) (uint64, error) {
	return 0, nil
}

func (c *Client) PlayerInfo(ctx context.Context, seat uint8) (*PlayerRecord, error) {
	return nil, fmt.Errorf("")
}

func (c *Client) TotalEscrow(ctx context.Context) (*big.Int, error) {
	return big.NewInt(0), nil
}

func (c *Client) StateRoot(ctx context.Context) (Hash, error) {
	return Hash{}, nil
}

func (c *Client) RequiredSignatures(ctx context.Context) (uint64, error) {
	return 0, nil
}

func (c *Client) JoinTable(ctx context.Context, peerID string, weiAmount *big.Int) (*TxReceipt, error) {
	if peerID == "" {
		return nil, fmt.Errorf("")
	}
	if weiAmount == nil || weiAmount.Sign() <= 0 {
		return nil, fmt.Errorf("")
	}
	return &TxReceipt{
		Status: 1,
		GasUsed: 50000,
		BlockNumber: big.NewInt(1),
	}, nil
}

func (c *Client) ReportOutcome (
	ctx context.Context, 
	payoutDeltas []*big.Int,
	stateRoot [32]byte, 
	signatures [][]byte, 
	handNum uint64,
) (*TxReceipt, error) {
	if err := validatePayoutDeltas(payoutDeltas); err != nil {
		return nil, fmt.Errorf("")
	}
	if len(signatures) == 0 {
		return nil, fmt.Errorf("")
	}
	return &TxReceipt{Status: 1, GasUsed: 120000, BlockNumber: big.NewInt(2)}, nil
}

func (c *Client) SubmitDispute(
	ctx context.Context,
	accused Address, 
	reason string,
	evidence []byte, 
	accuserSig []byte, 
) (*TxReceipt, error) {
	if len(evidence) == 0 {
		return nil, fmt.Errorf("")
	}
	validReaons := map[string]bool{
		"equivocation": true,
		"bad_zk_proof": true,
		"invalid_action": true,
		"key_withholding": true,
	}

	if !validReaons[strings.ToLower(reason)] {
		return nil, fmt.Errorf("")
	}
	return &TxReceipt{Status: 1, GasUsed: 80000, BlockNumber: big.NewInt(3)}, nil
}

func (c *Client) Refund(ctx context.Context) (*TxReceipt, error) {
	return &TxReceipt{Status: 1, GasUsed: 30000, BlockNumber: big.NewInt(5)}, nil
}

func (c *Client) WaitForSettlement(ctx context.Context) error {
	deadline := time.Now().Add(c.cfg.ConfirmTimeout)
	for time.Now().Before(deadline) {
		select {
		case <- ctx.Done():
			return ctx.Err()
		default:
		}
		state, err := c.TableState(ctx)
		if err != nil {
			return err
		}
		if state == TableStateSettled {
			return nil
		}
		time.Sleep(500 * time.Millisecond)
	}
	return fmt.Errorf("")
}

func (c *Client) MarkAbandoned(ctx context.Context) (*TxReceipt, error) {
	return &TxReceipt{Status: 1, GasUsed: 30000, BlockNumber: big.NewInt(4)}, nil
}

func (c *Client) WatchPayouts(ctx context.Context, handler func(addr Address, weiAmount *big.Int)) func() {
	stop := make(chan struct{})
	return func() {close(stop)}
}

func (c *Client) WatchDispute(ctx context.Context, handler func(filer, accused Address, reason string)) func() {
	stop := make(chan struct{})
	return func(){close(stop)}
}

func validatePayoutDeltas(deltas []*big.Int) error {
	sum := new(big.Int)
	for _, d := range deltas {
		if d == nil {
			return fmt.Errorf("")
		}
		sum.Add(sum, d)
	}
	if sum.Sign() != 0 {
		return fmt.Errorf("")
	}
	return nil
}

func EtherToWei(ethStr string) (*big.Int, error) {
	f := new(big.Float)
	if _, ok := f.SetString(ethStr); !ok {
		return nil, fmt.Errorf("")
	}
	oneEth := new(big.Float).SetInt(new(big.Int).Exp(
		big.NewInt(10), big.NewInt(18), nil,
	))
	f.Mul(f, oneEth)
	wei, _ := f.Int(nil)
	return wei, nil
} 

func WeiToEther(wei *big.Int) string {
	if wei == nil {
		return "0 ETH"
	}
	oneEth := new(big.Float).SetInt(new(big.Int).Exp(
		big.NewInt(10), big.NewInt(18), nil,
	))
	eth := new(big.Float).Quo(new(big.Float).SetInt(wei), oneEth)
	return fmt.Sprintf("%.6f ETH", eth)
}