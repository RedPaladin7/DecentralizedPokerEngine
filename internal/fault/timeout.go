package fault

import (
	"fmt"
	"sync"
	"time"
)

const TimeoutVoteThreshold = 2.0 / 3.0 

type VoteStatus uint8 

const (
	VotePending VoteStatus = iota 
	VoteConfirmed
	VoteRejected 
)

type TimeoutVote struct {
	TargetPeerID string 
	HandNum int64 
	Votes map[string]bool 
	TotalVoters int 
	Status VoteStatus
	CreatedAt time.Time
	ConfirmedAt time.Time
}

func (tv *TimeoutVote) AddVote(voterID string, yes bool) VoteStatus {
	if tv.Status != VotePending {
		return tv.Status
	}
	tv.Votes[voterID] = yes 

	yesCount := 0 
	for _, v := range tv.Votes {
		if v {
			yesCount++
		}
	}

	threshold := int(float64(tv.TotalVoters)*TimeoutVoteThreshold + 0.5)
	if threshold < 1 {
		threshold = 1
	}

	if yesCount >= threshold {
		tv.Status = VoteConfirmed
		tv.ConfirmedAt = time.Now()
	} else if len(tv.Votes) == tv.TotalVoters {
		tv.Status = VoteRejected
	}
	return tv.Status
}

func (tv *TimeoutVote) YesCount() int {
	n := 0 
	for _, v := range tv.Votes {
		if v {
			n++
		}
	}
	return n
}

type TimeoutManager struct {
	mu sync.Mutex
	handNum int64 
	totalPeers int 
	votes map[string]*TimeoutVote
	voteExpiry time.Duration
	OnConfirmed func(targetPeerID string)
}

func NewTimeoutManager(handNum int64, totalPeers int, voteExpiry time.Duration) *TimeoutManager {
	if voteExpiry == 0 {
		voteExpiry = 30 * time.Second
	}
	return &TimeoutManager{
		handNum: handNum,
		totalPeers: totalPeers,
		votes: make(map[string]*TimeoutVote),
		voteExpiry: voteExpiry,
	}
}

func (tm *TimeoutManager) StartVote(targetPeerID, callerPeerID string) *TimeoutVote {
	tm.mu.Lock()
	defer tm.mu.Unlock()
	
	if v, exists := tm.votes[targetPeerID]; exists && v.Status == VotePending {
		return v
	}

	tv := &TimeoutVote{
		TargetPeerID: targetPeerID,
		HandNum: tm.handNum,
		Votes: make(map[string]bool),
		TotalVoters: tm.totalPeers -1 ,
		Status: VotePending,
		CreatedAt: time.Now(),
	}
	tm.votes[targetPeerID] = tv 

	tv.AddVote(callerPeerID, true)
	if tv.Status == VoteConfirmed && tm.OnConfirmed != nil {
		go tm.OnConfirmed(targetPeerID)
	}
	return tv
}

func (tm *TimeoutManager) RecordVote(targetPeerID, voterPeerID string, yes bool) (VoteStatus, error) {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	tv, exists := tm.votes[targetPeerID]
	if !exists {
		tv = &TimeoutVote{
			TargetPeerID: targetPeerID,
			HandNum: tm.handNum,
			Votes: make(map[string]bool),
			TotalVoters: tm.totalPeers -1 ,
			Status: VotePending,
			CreatedAt: time.Now(),
		}
		tm.votes[targetPeerID] = tv 
	}
	if tv.Status != VotePending {
		return tv.Status,  nil
	}

	status := tv.AddVote(voterPeerID, yes)
	if status == VoteConfirmed && tm.OnConfirmed != nil {
		go tm.OnConfirmed(targetPeerID)
	}
	return status, nil
}

func (tm *TimeoutManager) ExpireStaleVotes() []string {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	var expired []string 
	cutoff := time.Now().Add(-tm.voteExpiry)
	for id, v := range tm.votes {
		if v.Status == VotePending && v.CreatedAt.Before(cutoff) {
			v.Status = VoteRejected
			expired = append(expired, id)
		}
	}
	return expired
}

func (tm *TimeoutManager) Summary() string {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	if len(tm.votes) == 0 {
		return "no timeout votes"
	}
	var out string 
	for id, v := range tm.votes {
		out += fmt.Sprintf("%s: %d/%d votes, status=%v\n", id[:min(16, len(id))], v.YesCount(), v.TotalVoters, v.Status)
	}
	return out
}

func (tm *TimeoutManager) VoteFor(targetPeerID string) *TimeoutVote {
	tm.mu.Lock()
	defer tm.mu.Unlock()
	return tm.votes[targetPeerID]
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}