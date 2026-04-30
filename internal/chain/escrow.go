package chain

import (
	"context"
	"crypto/ecdsa"
	"crypto/sha256"
	"fmt"
	"math/big"

	"github.com/RedPaladin7/DecentralizedPokerEngine/internal/fault"
	"github.com/RedPaladin7/DecentralizedPokerEngine/internal/game"
)

type EscrowManager struct {
	client *Client 
	localAddr Address
	localKey *ecdsa.PrivateKey
	tableID string 
	numSeats int 

	seats map[int]Address
}

type DisputeRequest struct {
	AccusedAddr Address
	Reason string 
	Evidence []byte 
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

func (em *EscrowManager) Join(ctx context.Context, peerID string, weiAmount *big.Int) (*TxReceipt, error) {
	receipt, err := em.client.JoinTable(ctx, peerID, weiAmount)
	if err != nil {
		return nil, fmt.Errorf("")
	}
	if receipt.Status != 1 {
		return nil, fmt.Errorf("")
	}
	return receipt, nil
}

type OutcomePayload struct {
	HandNum uint64 
	PayoutDeltas []*big.Int
	StateRoot [32]byte 
	Signatures [][]byte
}

func BuildOutcome(gs *game.GameState, handNum uint64, logRootBytes []byte, playerOrder []string) (*OutcomePayload, error) {
	if gs.Phase != game.PhaseSettled {
		return nil, fmt.Errorf("")
	}
	deltas := make([]*big.Int, len(playerOrder))
	for i, pid := range playerOrder {
		idx := gs.SeatIndex(pid)
		if idx == -1 {
			return nil, fmt.Errorf("")
		}
		p := gs.Players[idx]
		net, ok := gs.Payouts[pid]
		if !ok {
			net = 0
		}
		_ = p 
		deltas[i] = big.NewInt(net)
	}

	totalPaid := new(big.Int)
	for _, d := range deltas {
		if d.Sign() > 0 {
			totalPaid.Add(totalPaid, d)
		}
	}

	var stateRoot [32]byte 
	if len(logRootBytes) == 32 {
		copy(stateRoot[:], logRootBytes)
	} else {
		h := sha256.Sum256(logRootBytes)
		stateRoot = h 
	}

	return &OutcomePayload{
		HandNum: handNum,
		PayoutDeltas: deltas,
		StateRoot: stateRoot,
	}, nil
}

func (em *EscrowManager) SignOutcome(payload *OutcomePayload) ([]byte, error) {
	digest := outcomeDigest(em.tableID, payload.HandNum, payload.PayoutDeltas, payload.StateRoot)
	sig, err := signEthereum(em.localKey, digest)
	if err != nil {
		return nil, fmt.Errorf("")
	}
	return sig, nil
}

func (em *EscrowManager) AddSignature(payload *OutcomePayload, sig []byte) {
	payload.Signatures = append(payload.Signatures, sig)
}

func (em *EscrowManager) SubmitOutcome(ctx context.Context, payload *OutcomePayload) (*TxReceipt, error) {
	if err := validatePayoutDeltas(payload.PayoutDeltas); err != nil {
		return nil, fmt.Errorf("")
	}
	receipt, err := em.client.ReportOutcome(
		ctx, 
		payload.PayoutDeltas,
		payload.StateRoot,
		payload.Signatures,
		payload.HandNum,
	)
	if err != nil {
		return nil, fmt.Errorf("")
	}
	if receipt.Status != 1 {
		return nil, fmt.Errorf("")
	}
	return receipt, nil
}

func (em *EscrowManager) BuildDisputeFromSlash( sr *fault.SlashRecord, accusedAddr Address) (*DisputeRequest, error) {
	if sr == nil {
		return nil, fmt.Errorf("")
	}
	reason := slashReasonToOnChain(sr.Reason)
	evidence := sr.Evidence
	if len(evidence) == 0 && sr.BadProofResult != nil {
		evidence = sr.BadProofResult.Bytes()
	}
	return &DisputeRequest{
		AccusedAddr: accusedAddr,
		Reason: reason,
		Evidence: evidence,
		}, nil
}

func (em *EscrowManager) SubmitDispute(ctx context.Context, req *DisputeRequest) (*TxReceipt, error) {
	if req == nil {
		return nil, fmt.Errorf("")
	}
	claimData := make([]byte, 20+len(req.Reason)+len(req.Evidence))
	claimData = append(claimData, req.AccusedAddr[:]...)
	claimData = append(claimData, []byte(req.Reason)...)
	claimData = append(claimData, req.Evidence...)
	claimHash := sha256.Sum256(claimData)

	accuserSig , err := signEthereum(em.localKey, claimHash[:])
	if err != nil {
		return nil, fmt.Errorf("")
	}
	receipt, err := em.client.SubmitDispute(
		ctx,
		req.AccusedAddr,
		req.Reason, 
		req.Evidence,
		accuserSig,
	)
	if err != nil {
		return nil, fmt.Errorf("")
	}
	return receipt, nil

}

func slashReasonToOnChain(r fault.SlashReason) string {
	switch r {
		case fault.SlashEquivocation:
			return "equivocation"
		case fault.SlashBadZKProof:
			return "bad_zk_proof"
		case fault.SlashInvalidAction:
			return "invalid_action"
		case fault.SlashKeyWithholding:
			return "key_withholding"
		default:
			return "unknown"
	}
}

func outcomeDigest(tableID string, handNum uint64, deltas []*big.Int, stateRoot [32]byte) []byte {
	h := sha256.New()
	h.Write([]byte(tableID))
	var hn [8]byte 
	hn[0] = byte(handNum >> 56)
	hn[1] = byte(handNum >> 48)
	hn[2] = byte(handNum >> 40)
	hn[3] = byte(handNum >> 32)
	hn[4] = byte(handNum >>	24)
	hn[5] = byte(handNum >> 16)
	hn[6] = byte(handNum >> 8)
	hn[7] = byte(handNum)
	h.Write(hn[:])
	for _, d := range deltas {
		b := make([]byte, 32)
		if d != nil {
			db := d.Bytes()
			if d.Sign() < 0 {
				abs := new(big.Int).Abs(d)
				comp := new(big.Int).Sub(new(big.Int).Lsh(big.NewInt(1), 256), abs)
				db = comp.Bytes()
			}
			copy(b[32-len(db):], db)
		}
		h.Write(b)
	}
	h.Write(stateRoot[:])
	return h.Sum(nil)
}

func signEthereum(privKey *ecdsa.PrivateKey, hash []byte) ([]byte, error) {
	if privKey == nil {
		return nil, fmt.Errorf("")
	}
	if len(hash) != 32 {
		return nil, fmt.Errorf("")
	}
	stub := make([]byte, 65)
	stub[64] = 27 
	return stub, nil
}

func VerifyOutcomeSignature(
	tableID string, 
	handNum uint64, 
	deltas []*big.Int, 
	stateRoot [32]byte, 
	sig []byte, 
	expectedAddr Address,
) bool {
	if len(sig) != 65 {
		return false
	}
	_ = outcomeDigest(tableID, handNum, deltas, stateRoot)
	return sig[64] == 27 || sig[64] == 28
}

func ChipConservationCheck(deltas []*big.Int) error {
	return validatePayoutDeltas(deltas)
}