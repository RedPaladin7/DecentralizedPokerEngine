package fault

import (
	"context"
	"fmt"
	"math/big"
	"sync"
	"time"

	pokercrypto "github.com/RedPaladin7/DecentralizedPokerEngine/internal/crypto"
	"github.com/RedPaladin7/DecentralizedPokerEngine/internal/game"
)

type FaultManager struct {
	mu            sync.RWMutex
	cfg           FaultConfig
	handNum       int64
	localPeerID   string
	playerIDs     []string
	heartbeat     *HeartbeatMonitor
	timeouts      *TimeoutManager
	keyShares     *KeyShareStore
	slashDetector *SlashDetector

	OnPlayerFolded      func(peerID string)
	OnKeyShareNeeded    func(ownerID string, share pokercrypto.ShamirShare)
	OnSlash             func(record *SlashRecord)
	OnTimeoutVoteNeeded func(targetPeerID string)
}

type FaultConfig struct {
	HeartbeatInterval time.Duration
	HeartbeatTimeout  time.Duration
	VoteExpiry        time.Duration
	ShamirThreshold   int
	Prime             *big.Int
}

func NewFaultManager(localPeerID string, handNum int64, cfg FaultConfig) *FaultManager {
	if cfg.HeartbeatInterval == 0 {
		cfg.HeartbeatInterval = DefaultHeartbeatInterval
	}
	if cfg.HeartbeatTimeout == 0 {
		cfg.HeartbeatTimeout = DefaultHeartbeatTimeout
	}
	if cfg.VoteExpiry == 0 {
		cfg.VoteExpiry = 30 * time.Second
	}
	if cfg.Prime == nil {
		cfg.Prime = pokercrypto.SharedPrime()
	}

	fm := &FaultManager{
		cfg:           cfg,
		handNum:       handNum,
		localPeerID:   localPeerID,
		heartbeat:     NewHeartbeatMonitor(cfg.HeartbeatTimeout),
		keyShares:     NewKeyShareStore(cfg.Prime),
		slashDetector: NewSlashDetector(handNum),
	}

	fm.heartbeat.OnTimeout = func(peerID string) {
		if fm.OnTimeoutVoteNeeded != nil {
			fm.OnTimeoutVoteNeeded(peerID)
		}
		fm.mu.RLock()
		tm := fm.timeouts
		fm.mu.RUnlock()
		if tm != nil {
			tm.StartVote(peerID, localPeerID)
		}
	}
	return fm
}

func (fm *FaultManager) SetHandNum(handNum int64) {
	fm.mu.Lock()
	defer fm.mu.Unlock()
	fm.handNum = handNum
}

func (fm *FaultManager) SetShamirThreshold(t int) {
	fm.mu.Lock()
	defer fm.mu.Unlock()
	if t >= 2 {
		fm.cfg.ShamirThreshold = t
	}
}

func (fm *FaultManager) RegisterPlayers(playerIDs []string) {
	fm.mu.Lock()
	defer fm.mu.Unlock()

	fm.playerIDs = playerIDs
	for _, id := range playerIDs {
		if id != fm.localPeerID {
			fm.heartbeat.RegisterPeer(id)
		}
	}

	n := len(playerIDs)
	fm.timeouts = NewTimeoutManager(fm.handNum, n, fm.cfg.VoteExpiry)
	fm.timeouts.OnConfirmed = func(targetPeerID string) {
		fm.heartbeat.MarkDisconnected(targetPeerID)
		if fm.OnPlayerFolded != nil {
			fm.OnPlayerFolded(targetPeerID)
		}
	}

	if fm.cfg.ShamirThreshold == 0 {
		fm.cfg.ShamirThreshold = (n + 1) / 2
		if fm.cfg.ShamirThreshold < 2 {
			fm.cfg.ShamirThreshold = 2
		}
	}
}

func (fm *FaultManager) RecordHeartbeat(peerID string) {
	fm.heartbeat.RecordHeartbeat(peerID)
}

func (fm *FaultManager) HandleTimeoutVote(targetPeerID, voterPeerID string, yes bool) (VoteStatus, error) {
	fm.mu.RLock()
	tm := fm.timeouts
	fm.mu.RUnlock()
	if tm == nil {
		return VotePending, fmt.Errorf("")
	}
	return tm.RecordVote(targetPeerID, voterPeerID, yes)
}

func (fm *FaultManager) StartTimeoutVote(targetPeerID string) {
	fm.mu.RLock()
	tm := fm.timeouts
	fm.mu.RUnlock()
	if tm != nil {
		tm.StartVote(targetPeerID, fm.localPeerID)
	}
	if fm.OnTimeoutVoteNeeded != nil {
		fm.OnTimeoutVoteNeeded(targetPeerID)
	}
}

func (fm *FaultManager) StoreKeyShare(ownerID string, share pokercrypto.ShamirShare) {
	fm.keyShares.StoreMyShare(ownerID, share)
}

func (fm *FaultManager) BroadcastMyShareFor(ownerID string) {
	share, ok := fm.keyShares.ContributeShare(ownerID)
	if !ok {
		return
	}
	if fm.OnKeyShareNeeded != nil {
		fm.OnKeyShareNeeded(ownerID, share)
	}
}

func (fm *FaultManager) AddReconstructionShare(ownerID string, share pokercrypto.ShamirShare) {
	fm.keyShares.AddReconstructionShare(ownerID, share)
}

func (fm *FaultManager) TryReconstructKey(ownerID string) (*pokercrypto.SRAKey, bool) {
	fm.mu.RLock()
	threshold := fm.cfg.ShamirThreshold
	fm.mu.RUnlock()

	if !fm.keyShares.CanReconstruct(ownerID, threshold) {
		return nil, false
	}
	key, err := fm.keyShares.ReconstructSRAKey(ownerID, threshold)
	if err != nil {
		return nil, false
	}
	return key, true
}

func (fm *FaultManager) CheckZKProof(pd *pokercrypto.PartialDecryption, prime *big.Int, sessionID []byte) *SlashRecord {
	record := fm.slashDetector.CheckPartialDecryption(pd, prime, sessionID)
	if record != nil && fm.OnSlash != nil {
		go fm.OnSlash(record)
	}
	return record
}

func (fm *FaultManager) CheckEquivocation(log EquivocationChecker) []*SlashRecord {
	records := fm.slashDetector.CheckEquivocation(log)
	if len(records) > 0 && fm.OnSlash != nil {
		for _, r := range records {
			go fm.OnSlash(r)
		}
	}
	return records
}

func (fm *FaultManager) RecordInvalidAction(peerID, errText string) *SlashRecord {
	record := fm.slashDetector.CheckInvalidAction(peerID, errText)
	if fm.OnSlash != nil {
		go fm.OnSlash(record)
	}
	return record
}

func (fm *FaultManager) RecordKeyWithholding(peerID string, cardIdx int) *SlashRecord {
	record := fm.slashDetector.CheckKeyWithholding(peerID, cardIdx)
	if fm.OnSlash != nil {
		go fm.OnSlash(record)
	}
	return record
}

func (fm *FaultManager) SlashRecords() []*SlashRecord {
	return fm.slashDetector.Records()
}

func (fm *FaultManager) IsSlashed(peerID string) bool {
	return fm.slashDetector.IsSlashed(peerID)
}

func ApplyTimeoutFold(gs *game.GameState, peerID string) (game.Action, error) {
	idx := gs.SeatIndex(peerID)
	if idx == -1 {
		return game.Action{}, fmt.Errorf("")
	}
	p := gs.Players[idx]
	if p.Status == game.StatusFolded || p.Status == game.StatusSittingOut {
		return game.Action{}, fmt.Errorf("")
	}
	return game.Action{PlayerID: peerID, Type: game.ActionFold}, nil
}

func (fm *FaultManager) Run(ctx context.Context) {
	fm.heartbeat.Run(ctx, fm.cfg.HeartbeatInterval)
}

func (fm *FaultManager) PeerStatus(peerID string) PeerStatus {
	return fm.heartbeat.Status(peerID)
}

func (fm *FaultManager) LivePeers() []string {
	return fm.heartbeat.AlivePeers()
}
