package game

import "fmt"

type PlayerStatus uint8 

const (
	StatusActive PlayerStatus = iota 
	StatusFolded
	StatusAllIn
	StatusSittingOut
)

func (s PlayerStatus) String() string {
	return [...]string{"Active", "Folded", "All-In", "Sitting Out"}[s]
}

type Player struct {
	ID string 
	Name string 
	Stack int64 
	HoleCards [2]Card 
	Status PlayerStatus
	CurrentBet int64 
	TotalBet int64 
}

func NewPlayer(id, name string, stack int64) *Player {
	return &Player{
		ID: id,
		Name: name,
		Stack: stack,
		Status: StatusActive,
	}
}

func (p *Player) isActive() bool {
	return p.Status == StatusActive
}

func (p *Player) CanAct() bool {
	return p.Status == StatusActive
}

func (p *Player) PlaceBet(amount int64) int64 {
	if amount >= p.Stack {
		amount = p.Stack
		p.Status = StatusAllIn
	}
	p.Stack -= amount 
	p.CurrentBet += amount 
	p.TotalBet += amount 
	return amount
}

func (p *Player) ResetForNewHand() {
	p.HoleCards = [2]Card{}
	p.Status = StatusActive
	p.CurrentBet = 0
	p.TotalBet = 0
}

func (p *Player) ResetForNewRound() {
	p.CurrentBet = 0 
}

func (p *Player) String() string {
	return fmt.Sprintf("%s(%s) stack=%d status=%s", p.Name, p.ID, p.Stack, p.Status)
}