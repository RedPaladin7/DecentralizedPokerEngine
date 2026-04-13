package game

import "sort"

type PotSlice struct {
	Amount int64 
	EligibleIDs []string
}

func CalculatePots(players []*Player) [] PotSlice {
	type contrib struct {
		player *Player 
		totalBet int64 
	}
	var contribs []contrib 
	for _, p := range players {
		if p.TotalBet > 0 {
			contribs = append(contribs, contrib{p, p.TotalBet})
		}
	}

	if len(contribs) == 0 {
		return nil
	}

	sort.Slice(contribs, func(i, j int) bool {
		return contribs[i].totalBet < contribs[j].totalBet
	})

	var pots []PotSlice
	prevLevel := int64(0)

	for i, c := range contribs {
		if c.totalBet == prevLevel {
			continue 
		}
		level := c.totalBet
		diff := level - prevLevel

		var amount int64 
		var eligible []string 
		for _, rc := range contribs[i:] {
			amount += diff 
			eligible = append(eligible, rc.player.ID)
		}

		pots = append(pots, PotSlice{
			Amount: amount,
			EligibleIDs: eligible,
		})
		prevLevel = level 
	}
	return mergePots(pots)
}

func mergePots(pots []PotSlice) []PotSlice {
	if len(pots) == 0 {
		return pots
	}
	merged := []PotSlice{pots[0]}
	for _, p := range pots[1:] {
		last := &merged[len(merged)-1]
		if sameEligible(last.EligibleIDs, p.EligibleIDs) {
			last.Amount += p.Amount
		} else {
			merged = append(merged, p)
		}
	}
	return merged
}

func sameEligible(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	set := make(map[string]struct{}, len(a))
	for _, id := range a {
		set[id] = struct{}{} 
	}
	for _, id := range b {
		if _, ok := set[id]; !ok {
			return false
		}
	}
	return true
}

func TotalPot(pots []PotSlice) int64 {
	var total int64 
	for _, p := range pots {
		total += p.Amount
	}
	return total
}