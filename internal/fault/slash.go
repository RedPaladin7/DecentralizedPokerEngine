package fault

import (
	"fmt"
	"math/big"
	"sync"
	"time"

	pokercrypto "github.com/RedPaladin7/DecentralizedPokerEngine/internal/crypto"
)

type SlashReason uint8 

const (
	SlashEquivocation SlashReason = iota 
	SlashBadZKProof 
	SlashInvalidAction 
	SlashKeyWithholding
)

func (r SlashReason) String() string {
	return [...]string{"Equivocation", "Bad ZK Proof", "Invalid Action", "Key Withholding"}[r]
}

type SlashRecord struct {
	PeerID string 
	Reason SlashReason
	HandNum int64 
	DetectedAt time.Time
	Evidence []byte 

	EnvA *LogEntry
	EnvB *LogEntry


	BadProofCardIdx int 
	BadProofResult *big.Int
}

func (sr *SlashRecord) String() string {
	short := sr.PeerID
	if len(short) > 16 {
		short = short[:16]
	}
	return fmt.Sprintf("SLASH[%s] peer=%s hand=%d at=%s", sr.Reason, short, sr.HandNum, sr.DetectedAt.Format("15:04:05"))
}

type SlashDetector struct {
	mu sync.RWMutex
	handNum int64 
	records []*SlashRecord
	slashed map[string]bool 
	OnSlash func(record *SlashRecord)
}

func NewSlashDetector(handNum int64) *SlashDetector {
	return &SlashDetector{
		handNum: handNum,
		slashed: make(map[string]bool),
	}
}

func (sd *SlashDetector) CheckEquivocation(log EquivocationChecker) []*SlashRecord {
	senderID, envA, envB := log.DetectEquivocation()
	if senderID == "" {
		return nil
	}
	record := &SlashRecord{
		PeerID: senderID,
		Reason: SlashEquivocation,
		HandNum: sd.handNum,
		DetectedAt: time.Now(),
		EnvA: envA,
		EnvB: envB,
	}
	sd.addRecord(record)
	return []*SlashRecord{record}
}

func (sd *SlashDetector) CheckPartialDecryption(pd *pokercrypto.PartialDecryption, prime *big.Int, sessionID []byte) *SlashRecord {
	if err := pd.Verify(prime, sessionID); err == nil {
		return nil
	}
	record := &SlashRecord{
		PeerID: pd.PlayerID,
		Reason: SlashBadZKProof,
		HandNum: sd.handNum,
		DetectedAt: time.Now(),
		BadProofCardIdx: pd.CardIndex,
		BadProofResult: pd.Result,
		Evidence: pd.Ciphertext.Bytes(),
	}
	sd.addRecord(record)
	return record
}

func (sd *SlashDetector) CheckInvalidAction(peerID string, errText string) *SlashRecord {
	record := &SlashRecord{
		PeerID: peerID,
		Reason: SlashInvalidAction,
		HandNum: sd.handNum,
		DetectedAt: time.Now(),
		Evidence: []byte(errText),
	}
	sd.addRecord(record)
	return record
}

func (sd *SlashDetector) CheckKeyWithholding(peerID string, cardIdx int) *SlashRecord {
	record := &SlashRecord{
		PeerID: peerID,
		Reason: SlashKeyWithholding,
		HandNum: sd.handNum,
		DetectedAt: time.Now(),
		BadProofCardIdx: cardIdx,
	}
	sd.addRecord(record)
	return record
}

func (sd *SlashDetector) Records() []*SlashRecord {
	sd.mu.RLock()
	defer sd.mu.RUnlock()
	out := make([]*SlashRecord, len(sd.records))
	copy(out, sd.records)
	return out
}

func (sd *SlashDetector) IsSlashed(peerID string) bool {
	sd.mu.RLock()
	defer sd.mu.RUnlock()
	return sd.slashed[peerID]
}

func (sd *SlashDetector) HasViolations() bool {
	sd.mu.RLock()
	defer sd.mu.RUnlock()
	return len(sd.records) > 0
}

func (sd *SlashDetector) addRecord(record *SlashRecord) {
	sd.mu.Lock()
	defer sd.mu.Unlock()
	sd.records = append(sd.records, record)
	sd.slashed[record.PeerID] = true 
	if sd.OnSlash != nil {
		go sd.OnSlash(record)
	}
}