package network
// "/internal/network/gamelog.go"
// paper trail of whole hand -> replay mechanism and evidence for dispute resolution

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"sync"
)

type Gamelog struct {
	mu sync.RWMutex
	tableID string 
	handNum int64 
	entries []*Envelope // ordered list of every envelope
	byKey map[string]struct{} // set repr using empty struct, we only care if a key exists, key format senderId:seq
}

func NewGameLog(tableID string, handNum int64) *Gamelog {
	return &Gamelog{
		tableID: tableID,
		handNum: handNum,
		byKey: make(map[string]struct{}),
	}
}

// append envelope to entries -> makes sure for no duplicacy
func (gl *Gamelog) Append(env *Envelope) error {
	gl.mu.Lock()
	defer gl.mu.Unlock()
	
	key := fmt.Sprintf("%s:%d", env.SenderId, env.Seq)
	if _, exists := gl.byKey[key]; exists {
		return fmt.Errorf("")
	}
	gl.entries = append(gl.entries, env)
	gl.byKey[key] = struct{}{}
	return nil
}

func (gl *Gamelog) Entries() []*Envelope {
	gl.mu.RLock()
	defer gl.mu.RUnlock()
	out := make([]*Envelope, len(gl.entries))
	copy(out, gl.entries)
	return out
}

func (gl *Gamelog) Len() int {
	gl.mu.RLock()
	defer gl.mu.RUnlock()
	return len(gl.entries)
}

// fingerprint for the state -> hash of all the entries
func (gl *Gamelog) StateRoot() []byte {
	gl.mu.RLock()
	defer gl.mu.RUnlock()
	
	// streaming hasher sha256, no need to provide all bytes at once, internal accumulation entry by entry
	h := sha256.New()
	var seq [8]byte 
	// imp: also includes the signature => proof of entire observale record including proof that message was authentic
	for _, env := range gl.entries {
		h.Write([]byte{byte(env.Type)})
		h.Write([]byte(env.SenderId))
		h.Write([]byte{0x00})
		binary.BigEndian.PutUint64(seq[:], uint64(env.Seq))
		h.Write(seq[:])
		h.Write(env.Payload)
		h.Write(env.Signature)
	}
	return h.Sum(nil)
}

func (gl *Gamelog) StateRootHex() string {
	return hex.EncodeToString(gl.StateRoot())
}

// detecting policy violation -> same sequence number but different payload from the same person
func (gl *Gamelog) DetectEquivocation() (string, *Envelope, *Envelope, error) {
	gl.mu.RLock()
	defer gl.mu.RUnlock()
	
	type seqKey struct {
		senderID string 
		seq int64 
	}
	// record of already encountered entries
	seen := make(map[seqKey]*Envelope, len(gl.entries))

	for _, env := range gl.entries {
		k := seqKey{
			senderID: env.SenderId,
			seq: env.Seq,
		}
		// only return if the payload is different
		// duplication is possible, not neccessary violation of policy
		if prev, exists := seen[k]; exists {
			if string(prev.Payload) != string(env.Payload) {
				return env.SenderId, prev, env, nil
			}
		} else {
			seen[k] = env 
		}
	}
	return "", nil, nil, nil
}

// check for structural completeness -> done based only on seq num
// has every player sent gapless sequence of numbers starting from 1
func (gl *Gamelog) ValidateSequences(playerIDs []string) error {
	gl.mu.RLock()
	defer gl.mu.RUnlock()
	
	perPlayer := make(map[string][]int64, len(playerIDs))
	// grouping entries per player
	for _, env := range gl.entries {
		perPlayer[env.SenderId] = append(perPlayer[env.SenderId], env.Seq)
	}

	for _, pid := range playerIDs {
		seqs := perPlayer[pid]
		if len(seqs) == 0 {
			continue 
		}
		// sorting the sequences (insertion sort)
		for i := 1; i < len(seqs); i++ {
			for j := i; j > 0 && seqs[j] < seqs[j-1]; j-- {
				seqs[j], seqs[j-1] = seqs[j-1], seqs[j]
			}
		}
		// checking for gaps
		for i, v := range seqs {
			expected := int64(i+1)
			if v != expected {
				return fmt.Errorf("")
			}
		}
	}
	return nil
}

func (gl *Gamelog) EntriesBySender(senderID string) []*Envelope {
	gl.mu.RLock()
	defer gl.mu.RUnlock()
	
	var out []*Envelope
	for _, env := range gl.entries {
		if env.SenderId == senderID {
			out = append(out, env)
		}
	}
	return out
}

var (
	ErrDuplicateEntry = errors.New("duplicate log entry")
	ErrEquivocation = errors.New("equivocation detected")
)