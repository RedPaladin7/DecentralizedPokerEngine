package game

import "fmt"

type Phase uint8 

const (
	PhaseWaiting Phase = iota 
	PhasePreFlop
	PhaseFlop
	PhaseTurn
	PhaseRiver
	PhaseShowdown	
	PhaseSettled
)

func (p Phase) String() string {
	return [...]string{
		"Waiting",
		"Pre-Flop",
		"Flop",
		"Turn",
		"River",
		"Showdown",
		"Settled",
	}[p]
}

type ActionType uint8 

const (
	ActionFold ActionType = iota 
	ActionCheck 
	ActionCall 
	ActionRaise 
	ActionAllIn
)

func (a ActionType) String() string {
	return [...]string{
		"Fold",
		"Check",
		"Call",
		"Raise",
		"All-In",
	}[a]
}

type Action struct {
	PlayerID 	string 
	Type 		ActionType
	Amount 		int64
}

type GameState struct {
	TableID string 
	HandNum int 
	SmallBlind int64 
	BigBlind int64 

	Players []*Player
	
	DealerIdx int 
	ActionIdx int 
	LastRaiserIdx int 

	Phase Phase

	CommunityCards []Card 
	Deck *Deck 

	Pots []PotSlice

	CurrentBet int64 
	MinRaise int64 
	RoundActionCount int 

	Log []Action 
	Payouts map[string]int64
}

func NewGameSatte(tableID string, handNum int, players []*Player, dealerIdx int, sb, bb int64) *GameState {
	gs := &GameState{
		TableID: tableID,
		HandNum: handNum,
		SmallBlind: sb,
		BigBlind: bb,
		Players: players,
		DealerIdx: dealerIdx,
		Phase: PhaseWaiting,
		Deck: NewDeck(),
		Payouts: make(map[string]int64),
		MinRaise: bb,
	}
	for _, p := range players {
		p.ResetForNewHand()
	}
	return gs
}

func (gs *GameState) ActivePlayers() []*Player {
	var active []*Player
	for _, p := range gs.Players{
		if p.isActive() {
			active = append(active, p)
		}
	}
	return active 
}

func (gs *GameState) PlayersInHand() []*Player {
	var inHand []*Player
	for _, p := range gs.Players {
		if p.Status == StatusActive || p.Status == StatusAllIn {
			inHand = append(inHand, p)
		}
	}
	return inHand
}

func (gs *GameState) CurrentPlayer() *Player {
	if gs.ActionIdx < 0 || gs.ActionIdx >= len(gs.Players) {
		return nil
	}
	return gs.Players[gs.ActionIdx]
}

func (gs *GameState) nextActiveIndex(fromIdx int) int {
	n := len(gs.Players) 
	for i := 1; i <= n; i++ {
		idx := (fromIdx + 1) % n 
		if gs.Players[idx].CanAct() {
			return idx 
		}
	}
	return -1
}

func (gs *GameState) SeatIndex(playerID string) int {
	for i, p := range gs.Players {
		if p.ID == playerID {
			return i
		}
	}
	return -1
}

func (gs *GameState) String() string {
	return fmt.Sprintf("Hand#%d Phase=%s Pot=%d CurrentBet=%d ActivePlayers=%d",
		gs.HandNum, gs.Phase, TotalPot(gs.Pots), gs.CurrentBet, len(gs.ActivePlayers()))
}