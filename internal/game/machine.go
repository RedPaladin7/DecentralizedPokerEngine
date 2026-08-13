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
	if m.State.Deck == nil {
		return fmt.Errorf("StartHand: no deck; use StartHandCrypto")
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
	
	if gs.Phase == PhaseShowdown || gs.Phase == PhaseSettled ||
		gs.Phase == PhaseWaiting || gs.Phase == PhaseAwaitingStreet {
		return fmt.Errorf("ApplyAction: no actions allowed in phase %s", gs.Phase)
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

	if m.cryptoMode() {
		switch gs.Phase {
		case PhasePreFlop, PhaseFlop, PhaseTurn:
			gs.Phase = PhaseAwaitingStreet
			return nil
		case PhaseRiver:
			return m.startShowdown()
		default:
			return fmt.Errorf("endBettingRound: unexpected phase %s", gs.Phase)
		}
	}

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
	if gs.Deck == nil {
		return fmt.Errorf("dealFlop: no local deck; use ApplyStreet")
	}
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
	if gs.Deck == nil {
		return fmt.Errorf("dealTurn: no local deck; use ApplyStreet")
	}
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
	if gs.Deck == nil {
		return fmt.Errorf("dealRiver: no local deck; use ApplyStreet")
	}
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
		if m.cryptoMode() {
			return m.endBettingRound()
		}
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
	return (m.State.DealerIdx + 2) % n
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
	if m.cryptoMode() && m.remainingHolesIncomplete() {
		return nil
	}
	return m.distributePots()
}

func (m *Machine) distributePots() error {
	gs := m.State
	if m.cryptoMode() && m.remainingHolesIncomplete() {
		return fmt.Errorf("distributePots: remaining hole cards not revealed")
	}
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

func (m *Machine) StartHandCrypto() error {
	gs := m.State
	if gs.Phase != PhaseWaiting {
		return fmt.Errorf("StartHandCrypto: expected PhaseWaiting, got %s", gs.Phase)
	}
	if len(gs.Players) < 2 {
		return fmt.Errorf("StartHandCrypto: need at least 2 players")
	}
	if len(gs.CommunityCards) != 0 {
		return fmt.Errorf("StartHandCrypto: community cards already dealt")
	}

	// Cards are inputs in crypto mode. Callers should fill local holes first;
	// opponent holes stay empty until ApplyHoleReveal at showdown.
	gs.Deck = nil

	if err := m.postBlinds(); err != nil {
		return err
	}
	bbIdx := m.bigBlindIndex()
	gs.ActionIdx = gs.nextActiveIndex(bbIdx)
	gs.LastRaiserIdx = bbIdx
	gs.RoundActionCount = 0
	gs.Phase = PhasePreFlop
	return nil
}

func (m *Machine) cryptoMode() bool {
	return m.State != nil && m.State.Deck == nil
}

func holeCardsDealt(p *Player) bool {
	return p != nil && p.HoleCards[0].dealt() && p.HoleCards[1].dealt()
}

func pendingStreetCount(nCommunity int) int {
	switch nCommunity {
	case 0:
		return 3
	case 3, 4:
		return 1
	default:
		return 0
	}
}

func (m *Machine) NeedsStreet() bool {
	return m.State != nil && m.State.Phase == PhaseAwaitingStreet
}

func (m *Machine) PendingStreetCount() int {
	if !m.NeedsStreet() {
		return 0
	}
	return pendingStreetCount(len(m.State.CommunityCards))
}

func (m *Machine) NeedsReveal() bool {
	return m.State != nil && m.State.Phase == PhaseShowdown && m.remainingHolesIncomplete()
}

func (m *Machine) MissingRevealIDs() []string {
	if m.State == nil {
		return nil
	}
	var ids []string
	for _, p := range m.State.Players {
		if p.Status != StatusActive && p.Status != StatusAllIn {
			continue
		}
		if !holeCardsDealt(p) {
			ids = append(ids, p.ID)
		}
	}
	return ids
}

func (m *Machine) remainingHolesIncomplete() bool {
	return len(m.MissingRevealIDs()) > 0
}

func (m *Machine) ApplyStreet(cards []Card) error {
	if !m.cryptoMode() {
		return fmt.Errorf("ApplyStreet: not crypto mode")
	}
	gs := m.State
	if gs.Phase != PhaseAwaitingStreet {
		return fmt.Errorf("ApplyStreet: expected PhaseAwaitingStreet, got %s", gs.Phase)
	}
	want := pendingStreetCount(len(gs.CommunityCards))
	if want == 0 {
		if len(gs.CommunityCards) >= 5 {
			return fmt.Errorf("ApplyStreet: board already complete")
		}
		return fmt.Errorf("ApplyStreet: unexpected board size %d", len(gs.CommunityCards))
	}
	if len(cards) != want {
		return fmt.Errorf("ApplyStreet: expected %d cards, got %d", want, len(cards))
	}

	seen := m.knownDealtCards()
	for i, c := range cards {
		if !c.dealt() {
			return fmt.Errorf("ApplyStreet: card %d is not a dealt card", i)
		}
		if _, dup := seen[c]; dup {
			return fmt.Errorf("ApplyStreet: duplicate card %s", c)
		}
		seen[c] = struct{}{}
	}

	gs.CommunityCards = append(gs.CommunityCards, cards...)
	switch len(gs.CommunityCards) {
	case 3:
		gs.Phase = PhaseFlop
	case 4:
		gs.Phase = PhaseTurn
	case 5:
		gs.Phase = PhaseRiver
	default:
		return fmt.Errorf("ApplyStreet: unexpected board size %d after apply", len(gs.CommunityCards))
	}
	return m.startNewBettingRound()
}

func (m *Machine) ApplyHoleReveal(playerID string, cards [2]Card) error {
	if !m.cryptoMode() {
		return fmt.Errorf("ApplyHoleReveal: not crypto mode")
	}
	gs := m.State
	if gs.Phase != PhaseShowdown {
		return fmt.Errorf("ApplyHoleReveal: expected PhaseShowdown, got %s", gs.Phase)
	}
	idx := gs.SeatIndex(playerID)
	if idx == -1 {
		return fmt.Errorf("ApplyHoleReveal: unknown player %s", playerID)
	}
	p := gs.Players[idx]
	if p.Status != StatusActive && p.Status != StatusAllIn {
		return fmt.Errorf("ApplyHoleReveal: player %s is not remaining (%s)", playerID, p.Status)
	}
	if !cards[0].dealt() || !cards[1].dealt() {
		return fmt.Errorf("ApplyHoleReveal: cards are not dealt cards")
	}
	if cards[0] == cards[1] {
		return fmt.Errorf("ApplyHoleReveal: duplicate hole card %s", cards[0])
	}

	if holeCardsDealt(p) {
		if p.HoleCards[0] == cards[0] && p.HoleCards[1] == cards[1] {
			if !m.remainingHolesIncomplete() {
				return m.distributePots()
			}
			return nil
		}
		return fmt.Errorf("ApplyHoleReveal: equivocation on %s", playerID)
	}

	seen := m.knownDealtCards()
	for _, c := range cards {
		if _, dup := seen[c]; dup {
			return fmt.Errorf("ApplyHoleReveal: duplicate card %s", c)
		}
	}
	p.HoleCards = cards
	if !m.remainingHolesIncomplete() {
		return m.distributePots()
	}
	return nil
}

func (m *Machine) knownDealtCards() map[Card]struct{} {
	seen := make(map[Card]struct{})
	if m.State == nil {
		return seen
	}
	for _, c := range m.State.CommunityCards {
		if c.dealt() {
			seen[c] = struct{}{}
		}
	}
	for _, p := range m.State.Players {
		for _, c := range p.HoleCards {
			if c.dealt() {
				seen[c] = struct{}{}
			}
		}
	}
	return seen
}