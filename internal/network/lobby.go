package network
// "/internal/network/lobby.go"
// manages everything that happens before a hand starts

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"

	"google.golang.org/protobuf/proto"
)

type LobbyState int 

const (
	LobbyWaiting LobbyState = iota 
	LobbyReady 
	LobbyPlaying
)

// info that we have on each player in lobby
type SeatInfo struct {
	PlayerID string 
	PlayerName string 
	BuyIn int64 
	SRAKeyE []byte 
	Nonce []byte 
	IsReady bool
	JoinedAt time.Time
	JoinedAtUnixMs int64
}

// game lobby repr
type Lobby struct {
	mu sync.RWMutex
	tableID string 
	maxSeats int 
	seats map[string]*SeatInfo
	state LobbyState
	readyCh chan struct{}
	once sync.Once
}

func NewLobby(tableID string, maxSeats int) *Lobby {
	return &Lobby{
		tableID: tableID,
		maxSeats: maxSeats,
		seats: make(map[string]*SeatInfo),
		readyCh: make(chan struct{}),
	}
}

// new player joining lobby
// can be used as handler func like described in protocol.go file
func (l *Lobby) HandleJoin(msg *JoinTable, fromPeerID string, senderTimestamp ...int64) error {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.state != LobbyWaiting {
		return fmt.Errorf("")
	}
	if len(l.seats) >= l.maxSeats {
		return fmt.Errorf("")
	}
	if _, exists := l.seats[fromPeerID]; exists {
		return fmt.Errorf("")
	}
	if msg.BuyIn <= 0 {
		return fmt.Errorf("")
	}
	var ts int64 
	if len(senderTimestamp) > 0 {
		ts = senderTimestamp[0]
	} else {
		ts = time.Now().UnixMilli()
	}
	l.seats[fromPeerID] = &SeatInfo{
		PlayerID: fromPeerID,
		PlayerName: msg.PlayerName,
		BuyIn: msg.BuyIn,
		SRAKeyE: msg.SraPubKeyE,
		Nonce: msg.SessionNonce,
		JoinedAt: time.UnixMilli(ts),
		JoinedAtUnixMs: ts,
	}
	return nil
}

func (l *Lobby) HandleReady(msg *PlayerReady, fromPeerID string) error {
	l.mu.Lock()
	defer l.mu.Unlock()

	seat, ok := l.seats[fromPeerID]
	if !ok {
		return fmt.Errorf("")
	}
	seat.IsReady = true 
	// if all seats are full, close the readyCh
	l.checkAllReady() // check at every new join
	return nil
}

func (l *Lobby) checkAllReady() {
	if len(l.seats) < l.maxSeats {
		return 
	}
	for _, s := range l.seats {
		if !s.IsReady {
			return 
		}
	}
	if l.state == LobbyWaiting {
		l.state = LobbyReady
		l.once.Do(func() {close(l.readyCh)})
	}
}

func (l *Lobby) WaitReady(ctx context.Context) error {
	// event driven, no cpu usage while waiting
	select {
	case <-l.readyCh: // fired when channel closed
		return nil
	case <-ctx.Done(): // fired on time out
		return fmt.Errorf("")
	}
}

func (l *Lobby) Seats() []*SeatInfo {
	l.mu.RLock()
	defer l.mu.RUnlock()

	// return copy of all seats in order of join time
	out := make([]*SeatInfo, 0, len(l.seats))
	for _, s := range l.seats {
		cp := *s 
		out = append(out, &cp)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].JoinedAtUnixMs != out[j].JoinedAtUnixMs {
			return out[i].JoinedAtUnixMs < out[j].JoinedAtUnixMs
		}
		return out[i].PlayerID < out[j].PlayerID
	})
	return out
}

func (l *Lobby) PlayerIDs() []string {
	seats := l.Seats()
	ids := make([]string, len(seats))
	for i, s := range seats {
		ids[i] = s.PlayerID
	}
	return ids
}

func (l *Lobby) Count() int {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return len(l.seats)
}

func (l *Lobby) SetPlaying() {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.state = LobbyPlaying
}

func (l *Lobby) State() LobbyState {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return l.state
}

func MarshalJoinTable(msg *JoinTable) ([]byte, error) {
	b, err := proto.Marshal(msg) 
	if err != nil {
		return nil, fmt.Errorf("")
	}
	return b, nil
}

func UnmarshalJoinTable(b []byte) (*JoinTable, error) {
	msg := &JoinTable{}
	if err := proto.Unmarshal(b, msg); err != nil {
		return nil, fmt.Errorf("")
	}
	return msg, nil
}

func MarshalPlayerReady(msg *PlayerReady) ([]byte, error) {
	b, err := proto.Marshal(msg) 
	if err != nil {
		return nil, fmt.Errorf("")
	}
	return b, nil
}

func UnmarshalPlayerReady(b []byte) (*PlayerReady, error) {
	msg := &PlayerReady{}
	if err := proto.Unmarshal(b, msg); err != nil {
		return nil, fmt.Errorf("")
	}
	return msg, nil
}

func (l *Lobby) SessionNonce() []byte {
	seats := l.Seats()
	var combined []byte 
	for _, s := range seats {
		combined = append(combined, s.Nonce...)
	}
	return combined
}

func (l *Lobby) CanonicalPlayerOrder() []string {
	return l.PlayerIDs()
}