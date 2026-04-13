package game

import "sort"

type HandRank uint8 

const (
	HighCard HandRank = iota 
	OnePair 
	TwoPair
	ThreeOfAKind
	Straight
	Flush
	FullHouse
	FourOfAKind
	StraightFlush
	RoyalFlush
)

func (h HandRank) String() string {
	return [...]string{
		"High Card", "One Pair", "Two Pair", "Three of a Kind",
		"Straight", "Flush", "Full House", "Four of a Kind",
		"Straight Flush", "Royal Flush",
	}[h]
}

type EvaluatedHand struct {
	Rank HandRank
	Cards [5]Card
	Kickers []Rank 
}

func (h EvaluatedHand) Compare(other EvaluatedHand) int {
	if h.Rank > other.Rank {
		return 1
	}
	if h.Rank < other.Rank {
		return -1
	}
	for i := 0; i < len(h.Kickers) && i < len(other.Kickers); i++ {
		if h.Kickers[i] > other.Kickers[i] {
			return 1 
		}
		if h.Kickers[i] < other.Kickers[i] {
			return -1
		}
	}
	return 0 
}

func EvaluateBest7(cards [7]Card) EvaluatedHand {
	best := EvaluatedHand{Rank: HighCard}
	for i := 0; i < 7; i++ {
		for j := i + 1; j < 7; j++ {
			var five [5]Card
			k := 0
			for idx := 0; idx < 7; idx++ {
				if idx == i || idx == j {
					continue
				}
				five[k] = cards[idx]
				k++
			}
			h := evaluate5(five)
			if h.Compare(best) > 0 {
				best = h
			}
		}
	}
	return best
}

func evaluate5(cards [5]Card) EvaluatedHand {
	sorted := cards
	sort.Slice(sorted[:], func(i, j int) bool {
		return sorted[i].Rank > sorted[j].Rank
	})
 
	isFlush := checkFlush(sorted)
	isStraight, straightHigh := checkStraight(sorted)
 
	switch {
	case isFlush && isStraight && straightHigh == Ace:
		return EvaluatedHand{Rank: RoyalFlush, Cards: sorted, Kickers: []Rank{Ace}}
	case isFlush && isStraight:
		return EvaluatedHand{Rank: StraightFlush, Cards: sorted, Kickers: []Rank{straightHigh}}
	}
 
	groups := groupByRank(sorted)
 
	switch {
	case groups[0].count == 4:
		return EvaluatedHand{
			Rank:    FourOfAKind,
			Cards:   sorted,
			Kickers: []Rank{groups[0].rank, groups[1].rank},
		}
	case groups[0].count == 3 && groups[1].count == 2:
		return EvaluatedHand{
			Rank:    FullHouse,
			Cards:   sorted,
			Kickers: []Rank{groups[0].rank, groups[1].rank},
		}
	case isFlush:
		kickers := rankDesc(sorted)
		return EvaluatedHand{Rank: Flush, Cards: sorted, Kickers: kickers}
	case isStraight:
		return EvaluatedHand{Rank: Straight, Cards: sorted, Kickers: []Rank{straightHigh}}
	case groups[0].count == 3:
		return EvaluatedHand{
			Rank:    ThreeOfAKind,
			Cards:   sorted,
			Kickers: []Rank{groups[0].rank, groups[1].rank, groups[2].rank},
		}
	case groups[0].count == 2 && groups[1].count == 2:
		high, low := groups[0].rank, groups[1].rank
		if low > high {
			high, low = low, high
		}
		return EvaluatedHand{
			Rank:    TwoPair,
			Cards:   sorted,
			Kickers: []Rank{high, low, groups[2].rank},
		}
	case groups[0].count == 2:
		kickers := make([]Rank, 0, 4)
		kickers = append(kickers, groups[0].rank)
		for _, g := range groups[1:] {
			kickers = append(kickers, g.rank)
		}
		return EvaluatedHand{Rank: OnePair, Cards: sorted, Kickers: kickers}
	default:
		return EvaluatedHand{Rank: HighCard, Cards: sorted, Kickers: rankDesc(sorted)}
	}
}

type rankGroup struct {
	rank Rank 
	count int
}

func groupByRank(cards [5]Card) []rankGroup {
	counts := make(map[Rank]int)
	for _, c := range cards {
		counts[c.Rank]++
	}
	groups := make([]rankGroup, 0, len(counts))
	for r, n := range counts {
		groups = append(groups, rankGroup{r, n})
	}
	sort.Slice(groups, func(i, j int) bool {
		if groups[i].count != groups[j].count {
			return groups[i].count > groups[j].count
		}
		return groups[i].rank > groups[j].rank
	})
	return groups
}

func checkFlush (cards [5]Card) bool {
	s := cards[0].Suit 
	for _, c := range cards[1:] {
		if c.Suit != s {
			return false
		}
	}
	return true 
}

func checkStraight(cards [5]Card) (bool, Rank) {
	for i := 1; i < 5; i++ {
		if int(cards[i-1].Rank)-int(cards[i].Rank)!=1 {
			goto wheel 
		}
	}
	return true, cards[0].Rank 
	wheel:
	if cards[0].Rank == Ace &&
		cards[1].Rank == Five &&
		cards[2].Rank == Four &&
		cards[3].Rank == Three &&
		cards[4].Rank == Two {
		return true, Five
	}
	return false, 0
}

func rankDesc(cards [5]Card) []Rank {
	out := make([]Rank, 5)
	for i, c := range cards {
		out[i] = c.Rank
	}
	return out
}

