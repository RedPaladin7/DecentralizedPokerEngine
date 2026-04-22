package network

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
	entries []*Envelope
	byKey map[string]struct{}
}

func NewGameLog(tableID string, handNum int64) *Gamelog {
	return &Gamelog{
		tableID: tableID,
		handNum: handNum,
		byKey: make(map[string]struct{}),
	}
}

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

func (gl *Gamelog) StateRoot() []byte {
	gl.mu.RLock()
	defer gl.mu.RUnlock()
	
	h := sha256.New()
	var seq [8]byte 
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

func (gl *Gamelog) DetectEquivocation() (string, *Envelope, *Envelope, error) {
	gl.mu.RLock()
	defer gl.mu.RUnlock()
	
	type seqKey struct {
		senderID string 
		seq int64 
	}
	seen := make(map[seqKey]*Envelope, len(gl.entries))

	for _, env := range gl.entries {
		k := seqKey{
			senderID: env.SenderId,
			seq: env.Seq,
		}
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

func (gl *Gamelog) ValidateSequences(playerIDs []string) error {
	gl.mu.RLock()
	defer gl.mu.RUnlock()
	
	perPlayer := make(map[string][]int64, len(playerIDs))
	for _, env := range gl.entries {
		perPlayer[env.SenderId] = append(perPlayer[env.SenderId], env.Seq)
	}

	for _, pid := range playerIDs {
		seqs := perPlayer[pid]
		if len(seqs) == 0 {
			continue 
		}
		for i := 1; i < len(seqs); i++ {
			for j := i; j > 0 && seqs[j] < seqs[j-1]; j-- {
				seqs[j], seqs[j-1] = seqs[j-1], seqs[j]
			}
		}
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