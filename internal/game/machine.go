package game

import (
	"fmt"
	"math/rand"
)

type Machine struct {
	State *GameState
	rng   *rand.Rand
}

func NewMachine(gs *GameState, rng *rand.Rand) *Machine {
	return &Machine{
		State: gs,
		rng:   rng,
	}
}

func (m *Machine) StartHand() error {
	if m.State.Phase != PhaseWaiting {
		return fmt.Errorf("StartHand: expected PhaseWaiting, got %s", m.State.Phase)
	}
	if len(m.State.Players) < 2 {
		return fmt.Errorf("StartHand: need at least 2 players")
	}

	m.State.Deck.Shuffle(m.rng)

	if err := m.postBlinds(); err != nil {
		return err
	}

	if err := m.dealHoleCards(); err != nil {
		return err
	}
	bbIdx := m.bigBlindIndex()
	m.State.ActionIdx = m.State.nextActiveIndex(bbIdx)
	m.State.LastRaiserIdx = bbIdx
	m.State.RoundActionCount = 0 
	m.State.Phase = PhasePreFlop
	return nil
}

func (m *Machine) ApplyAction (a Action) error {
	gs := m.State
	
	if gs.Phase == PhaseShowdown || gs.Phase == PhaseSettled || gs.Phase == PhaseWaiting {
		return fmt.Errorf("ApplyAcction: no actions allowed in phase %s", gs.Phase)
	}
	
	current := gs.CurrentPlayer()
	if current == nil || current.ID != a.PlayerID {
		return fmt.Errorf("ApplyAction: player %s cannot act (status %s)", a.PlayerID, current.Status)

	}

	switch a.Type {
	case ActionFold:
		current.Status = StatusFolded
		
	case ActionCheck:
		toCall := gs.CurrentBet - current.CurrentBet 
		if toCall != 0{
			return fmt.Errorf("ApplyAction: cannot check with a bet of %d to call", toCall)
		}

	case ActionCall:
		toCall := gs.CurrentBet - current.CurrentBet
		if toCall <= 0 {
			return fmt.Errorf("ApplyAction: nothing to call (use Check)")
		}
		current.PlaceBet(toCall)

	case ActionRaise:
		toCall := gs.CurrentBet - current.CurrentBet
		totalNeeded := toCall + a.Amount
		if a.Amount < gs.MinRaise {
			return fmt.Errorf("ApplyAction: raise of %d is below minimum %d", a.Amount, gs.MinRaise)
		}
		if totalNeeded > current.Stack + current.CurrentBet {
			return fmt.Errorf("ApplyAction: insufficient stack for raise")
		}
		gs.MinRaise = a.Amount
		gs.CurrentBet += a.Amount
		current.PlaceBet(totalNeeded)
		gs.LastRaiserIdx = gs.ActionIdx

	case ActionAllIn:
		allin := current.Stack
		total := current.CurrentBet + allin 
		if total > gs.CurrentBet + allin {
			raise := total - gs.CurrentBet
			if raise > gs.MinRaise {
				gs.MinRaise = raise 
			}
			gs.CurrentBet = total 
			gs.LastRaiserIdx = gs.ActionIdx
		}
		current.PlaceBet(allin)

	default:
		return fmt.Errorf("ApplyAction: unknown action type %d", a.Type)
	}

	gs.Log = append(gs.Log, a)
	gs.RoundActionCount++

	if m.onlyOneRemaining() {
		return m.resolveSingleWinner()
	}
	return m.advanceAction()

}

func (m *Machine) advanceAction() error {
	gs := m.State
	nextIdx := gs.nextActiveIndex(gs.ActionIdx)

	if nextIdx == -1 || !gs.Players[nextIdx].CanAct() {
		return m.endBettingRound()
	}
	if m.bettingRoundComplete(nextIdx) {
		return m.endBettingRound()
	}
	gs.ActionIdx = nextIdx 
	return nil
}

func (m *Machine) bettingRoundComplete(nextIdx int) bool {
	gs := m.State
	next := gs.Players[nextIdx]

	if next.CurrentBet < gs.CurrentBet{
		return false 
	}
	if gs.RoundActionCount > 0 && nextIdx == gs.LastRaiserIdx{
		return true 
	}
	active := m.countCanAct() 
	return gs.RoundActionCount >= active 
}

func (m *Machine) countCanAct() int {
	n := 0 
	for _, p := range m.State.Players {
		if p.CanAct(){
			n++
		}
	}
	return n
}

func (m *Machine) endBettingRound() error {
	gs := m.State
	gs.Pots = CalculatePots(gs.Players)

	switch gs.Phase {
	case PhasePreFlop:
		return m.dealFlop()
	case PhaseFlop:
		return m.dealTurn()
	case PhaseTurn:
		return m.dealRiver()
	case PhaseRiver:
		return m.startShowdown()
	}
	return nil
}

func (m *Machine) dealFlop() error {
	gs := m.State
	if _, err := gs.Deck.Deal(); err != nil {
		return err
	}
	for i := 0; i < 3; i++ {
		c, err := gs.Deck.Deal()
		if err != nil {
			return err
		}
		gs.CommunityCards = append(gs.CommunityCards, c)
	}
	gs.Phase = PhaseFlop
	return m.startNewBettingRound()
}

func (m *Machine) dealTurn() error {
	gs := m.State
	if _, err := gs.Deck.Deal(); err != nil {
		return err
	}
	c, err := gs.Deck.Deal()
	if err != nil {
		return err
	}
	gs.CommunityCards = append(gs.CommunityCards, c)
	gs.Phase = PhaseTurn
	return m.startNewBettingRound()
}

func (m *Machine) dealRiver() error {
	gs := m.State
	if _, err := gs.Deck.Deal(); err != nil {
		return err 
	}
	c, err := gs.Deck.Deal()
	if err != nil {
		return err
	}
	gs.CommunityCards = append(gs.CommunityCards, c)
	gs.Phase = PhaseRiver
	return m.startNewBettingRound()
}

func (m *Machine) startNewBettingRound() error {
	gs := m.State
	gs.CurrentBet = 0 
	gs.MinRaise = gs.BigBlind 
	gs.RoundActionCount = 0 
	gs.LastRaiserIdx = -1 

	for _, p := range gs.Players {
		if p.Status == StatusActive || p.Status == StatusAllIn {
			p.ResetForNewRound()
		}
	}

	first := gs.nextActiveIndex(gs.DealerIdx)
	if first == -1 {
		return m.startShowdown()
	}
	gs.ActionIdx = first 

	if m.countCanAct() <= 1{
		return m.endBettingRound()
	}
	return nil
}


func (m *Machine) postBlinds() error {
	gs := m.State 
	n := len(gs.Players)

	sbIdx := (gs.DealerIdx + 1) % n 
	bbIdx := (gs.DealerIdx + 2) % n 
	
	if n == 2 {
		sbIdx = gs.DealerIdx
		bbIdx = (gs.DealerIdx + 1) % n
	}

	sb := gs.Players[sbIdx]
	bb := gs.Players[bbIdx]

	sbAmount := sb.PlaceBet(gs.SmallBlind)
	bbAmount := bb.PlaceBet(gs.BigBlind)

	gs.CurrentBet = bbAmount 
	if sbAmount > gs.CurrentBet {
		gs.CurrentBet = sbAmount
	}

	gs.Log = append(gs.Log, Action{PlayerID: sb.ID, Type: ActionRaise, Amount: sbAmount})
	gs.Log = append(gs.Log, Action{PlayerID: bb.ID, Type: ActionRaise, Amount: bbAmount})
	return nil
}

func (m *Machine) dealHoleCards() error {
	gs := m.State
	n := len(gs.Players)
	start := (gs.DealerIdx + 1) % n
	for round := 0; round < 2; round++ {
		for i := 0; i < n; i++ {
			idx := (start + i) % n
			c, err := gs.Deck.Deal()
			if err != nil {
				return fmt.Errorf("dealHoleCards: %w", err)
			}
			gs.Players[idx].HoleCards[round] = c
		}
	}
	return nil
}

func (m *Machine) bigBlindIndex() int {
	n := len(m.State.Players)
	if n == 2 {
		return (m.State.DealerIdx + 1) % n
	}
	return (m.State.DealerIdx + 2) % 2
}

func (m *Machine) onlyOneRemaining() bool {
	count := 0 
	for _, p := range m.State.Players {
		if p.Status != StatusFolded && p.Status != StatusSittingOut {
			count++
		}
	}
	return count == 1
}

func (m *Machine) resolveSingleWinner() error {
	gs := m.State
	gs.Pots = CalculatePots(gs.Players)
	total := TotalPot(gs.Pots)
	for _, p := range gs.Players {
		if p.Status != StatusFolded && p.Status != StatusSittingOut {
			p.Stack += total 
			gs.Payouts[p.ID] += total 
			break 
		}
	}
	gs.Phase = PhaseSettled
	return nil
}

func (m *Machine) startShowdown() error {
	gs := m.State
	gs.Phase = PhaseShowdown
	gs.Pots = CalculatePots(gs.Players)
	return m.distributePots()
}

func (m *Machine) distributePots() error {
	gs := m.State
	comm := gs.CommunityCards 

	for _, pot := range gs.Pots {
		winners := m.potWinners(pot, comm)
		if len(winners) == 0 {
			continue 
		}
		share := pot.Amount / int64(len(winners))
		remainder := pot.Amount % int64(len(winners))
		for _, w := range winners {
			w.Stack += share 
			gs.Payouts[w.ID] += share 
		}
		if remainder > 0 {
			closest := m.closestLeftOfDealer(winners) 
			closest.Stack += remainder 
			gs.Payouts[closest.ID] += remainder 
		}
	}

	gs.Phase = PhaseSettled 
	return nil
}

func (m *Machine) potWinners(pot PotSlice, comm[]Card) []*Player {
	gs := m.State
	
	type entry struct {
		player *Player 
		hand EvaluatedHand
	}

	var candidates []entry 
	for _, pid := range pot.EligibleIDs {
		idx := gs.SeatIndex(pid)
		if idx == -1 {
			continue 
		}
		p := gs.Players[idx]
		if p.Status == StatusFolded {
			continue 
		}
		var seven [7]Card 
		seven[0] = p.HoleCards[0]
		seven[1] = p.HoleCards[1]
		for i, c := range comm {
			if i+2 < 7 {
				seven[i+2] = c 
			}
		}
		candidates = append(candidates, entry{p, EvaluateBest7(seven)})
	}

	if len(candidates) == 0 {
		return nil 
	}

	best := candidates[0].hand
	for _, e := range candidates[1:]{
		if e.hand.Compare(best) > 0 {
			best = e.hand
		}
	}

	var winners []*Player
	for _, e := range candidates {
		if e.hand.Compare(best) == 0 {
			winners = append(winners, e.player)
		}
	}
	return winners
}

func (m *Machine) closestLeftOfDealer(winners []*Player) *Player {
	gs := m.State
	n := len(gs.Players)
	winSet := make(map[string]*Player, len(winners))
	for _, w := range winners {
		winSet[w.ID] = w 
	}
	for i := 1; i <= n; i++ {
		idx := (gs.DealerIdx + i) % n 
		if p, ok := winSet[gs.Players[idx].ID]; ok {
			return p
		}
	}
	return winners[0]
}