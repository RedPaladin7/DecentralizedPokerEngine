package tui

import (
	"fmt"
	"strings"

	"github.com/RedPaladin7/DecentralizedPokerEngine/internal/game"
	"github.com/charmbracelet/lipgloss"
)

type TableViewOpts struct {
	LocalPlayerID string 
	ActingPlayerID string 
	DealerIdx int 
	WinnerIDs map[string]bool 
	HandRanks map[string]string
}

func RenderTable(gs *game.GameState, opts TableViewOpts) string {
	if gs == nil {
		return StyleTable.Render("  no game in progress")
	}
	n := len(gs.Players)
	infoBar := renderInfoBar(gs)
	communityArea := renderCommunityArea(gs)
 
	panels := make([]string, n)
	for i, p := range gs.Players {
		popts := PlayerPanelOpts{
			IsLocalPlayer: p.ID == opts.LocalPlayerID,
			IsActing:      p.ID == opts.ActingPlayerID,
			IsDealer:      i == opts.DealerIdx,
			IsSmallBlind:  i == smallBlindIdx(opts.DealerIdx, n),
			IsBigBlind:    i == bigBlindIdx(opts.DealerIdx, n),
			IsWinner:      opts.WinnerIDs != nil && opts.WinnerIDs[p.ID],
		}
		if opts.HandRanks != nil {
			popts.ShowHandRank = opts.HandRanks[p.ID]
		}
		panels[i] = RenderPlayerPanel(p, popts)
	}
 
	rows := arrangeSeatRows(panels, n, communityArea)
 
	body := lipgloss.JoinVertical(lipgloss.Center, rows...)
	table := StyleTable.Render(lipgloss.JoinVertical(lipgloss.Left, infoBar, body))
	return table
}

func renderInfoBar(gs *game.GameState) string {
	handStr := fmt.Sprintf("Hand #%d", gs.HandNum)
	phaseStr := StylePhase.Render(gs.Phase.String())
	potStr := StylePot.Render(fmt.Sprintf("Pot: $%d", game.TotalPot(gs.Pots)))
	blindStr := fmt.Sprintf("Blinds $%d/$%d", gs.SmallBlind, gs.BigBlind)

	left := lipgloss.NewStyle().Foreground(lipgloss.Color("#aaaaaa")).Render(
		handStr + " " + blindStr,
	)

	right := potStr

	gap := TableWidth - lipgloss.Width(left) - lipgloss.Width(phaseStr) - lipgloss.Width(right) - 2
	if gap < 1 {
		gap = 1
	}
	return StyleInfoBar.Render(
		left + strings.Repeat(" ", gap/2) + phaseStr + strings.Repeat(" ", gap-gap/2) + right,
	)
}

func renderCommunityArea(gs *game.GameState) string {
	cards := RenderCommunityCards(gs.CommunityCards)

	var potLines []string 
	for i, pot := range gs.Pots {
		label := "Main pot"
		if i > 0 {
			label = fmt.Sprintf("Side pot %d", i)
		}
		potLines = append(potLines, fmt.Sprintf("%s: %d", label, pot.Amount))
	}

	potInfo := ""
	if len(potLines) > 0 {
		potStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#f0c040")).Align(lipgloss.Center)
		potInfo = potStyle.Render(strings.Join(potLines, ", "))
	}

	currentBet := ""
	if gs.CurrentBet > 0 {
		currentBet = lipgloss.NewStyle(). 
			Foreground(lipgloss.Color("#aaaaaa")). 
			Render(fmt.Sprintf("Current bet: %d", gs.CurrentBet))
	}

	lines := []string{cards}
	if potInfo != "" {
		lines = append(lines, potInfo)
	}
	if currentBet != "" {
		lines = append(lines, currentBet)
	} 

	inner := lipgloss.JoinVertical(lipgloss.Center, lines...)
	return lipgloss.NewStyle(). 
		Background(feltGreenDark). 
		Padding(1, 4). 
		Border(lipgloss.RoundedBorder()). 
		BorderForeground(lipgloss.Color("2a6b3c")). 
		Align(lipgloss.Center).
		Render(inner)
}

func arrangeSeatRows(panels []string, n int, centre string) []string {
	pad := lipgloss.NewStyle().Width(4).Render("") 
 
	switch {
	case n == 2:
		row := lipgloss.JoinHorizontal(lipgloss.Center, panels[0], pad, centre, pad, panels[1])
		return []string{row}
 
	case n == 3:
		topRow := lipgloss.JoinHorizontal(lipgloss.Center, panels[0], pad, centre, pad, panels[1])
		botRow := centreRow([]string{panels[2]})
		return []string{topRow, botRow}
 
	case n == 4:
		topRow := lipgloss.JoinHorizontal(lipgloss.Center, panels[3], pad, centre, pad, panels[1])
		botRow := lipgloss.JoinHorizontal(lipgloss.Center, panels[2], pad, pad, pad, panels[0])
		_ = botRow
		midRow := centreRow([]string{panels[2], panels[0]})
		return []string{topRow, midRow}
 
	case n <= 6:
		topSeats := panels[3:n]
		topRow := centreRow(topSeats)
		midRow := lipgloss.JoinHorizontal(lipgloss.Center, panels[2], pad, centre, pad, panels[0])
		botRow := centreRow([]string{panels[1]})
		return []string{topRow, midRow, botRow}
 
	default:
		third := n / 3
		topSeats := panels[n-third:]
		midLeftSeats := panels[:1]
		midRightSeats := panels[third+1 : third+2]
		botSeats := panels[1 : n-third]
 
		topRow := centreRow(topSeats)
		midRow := lipgloss.JoinHorizontal(lipgloss.Center,
			centreRow(midLeftSeats), pad, centre, pad, centreRow(midRightSeats))
		botRow := centreRow(botSeats)
		return []string{topRow, midRow, botRow}
	}
}
 
func centreRow(panels []string) string {
	if len(panels) == 0 {
		return ""
	}
	pad := lipgloss.NewStyle().Width(2).Render("")
	parts := []string{panels[0]}
	for _, p := range panels[1:] {
		parts = append(parts, pad, p)
	}
	row := lipgloss.JoinHorizontal(lipgloss.Center, parts...)
	return lipgloss.NewStyle().Width(TableWidth).Align(lipgloss.Center).Render(row)
}

func smallBlindIdx(dealerIdx, n int) int {
	if n == 2 {
		return dealerIdx 
	}
	return (dealerIdx + 1) % n
}
 
func bigBlindIdx(dealerIdx, n int) int {
	if n == 2 {
		return (dealerIdx + 1) % n
	}
	return (dealerIdx + 2) % n
}