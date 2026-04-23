package tui

import (
	"fmt"

	"github.com/charmbracelet/lipgloss"
	"github.com/RedPaladin7/DecentralizedPokerEngine/internal/game"
)

type PlayerPanelOpts struct {
	IsLocalPlayer bool   // true = show hole cards face-up
	IsActing      bool   // true = it's this player's turn (highlighted border)
	IsDealer      bool   // show dealer chip "D"
	IsSmallBlind  bool   // show "SB" label
	IsBigBlind    bool   // show "BB" label
	IsWinner      bool   // golden border at showdown
	ShowHandRank  string // non-empty = show hand rank label below cards
}

func RenderPlayerPanel(p *game.Player, opts PlayerPanelOpts) string {
	if p == nil {
		return renderEmptySeat()
	}

	panelStyle := StylePlayerPanel
	switch {
	case opts.IsWinner:
		panelStyle = StyleWinnerPanel
	case opts.IsActing:
		panelStyle = StylePlayerPanelActing
	case p.Status == game.StatusFolded:
		panelStyle = StylePlayerPanelFolded
	}

	posLine := renderPositionChips(opts)
	nameStyle := StylePlayerName
	if p.Status == game.StatusFolded {
		nameStyle = StylePlayerNameMuted
	}
	name := nameStyle.Render(truncate(p.Name, PlayerWidth-6))

	headerLine := lipgloss.JoinHorizontal(lipgloss.Left, posLine, name)

	stack := StylePlayerStack.Render(fmt.Sprintf("$%-6d", p.Stack))
	badge := renderStatusBadge(p.Status, opts.IsActing)
	stackLine := lipgloss.JoinHorizontal(lipgloss.Left, stack, " ", badge)

	cardsLine := RenderHoleCards(p.HoleCards, opts.IsLocalPlayer || opts.IsWinner)

	var betLine string
	if p.CurrentBet > 0 {
		betLine = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#aaaaaa")).
			Render(fmt.Sprintf("bet: $%d", p.CurrentBet))
	}

	var handLine string
	if opts.ShowHandRank != "" {
		handLine = StyleHandRank.Render(opts.ShowHandRank)
	}

	lines := []string{headerLine, stackLine, cardsLine}
	if betLine != "" {
		lines = append(lines, betLine)
	}
	if handLine != "" {
		lines = append(lines, handLine)
	}

	content := lipgloss.JoinVertical(lipgloss.Left, lines...)
	return panelStyle.Render(content)
}

func renderPositionChips(opts PlayerPanelOpts) string {
	chips := ""
	if opts.IsDealer {
		chips += StyleDealerChip.Render("D") + " "
	}
	if opts.IsSmallBlind {
		chips += StyleSmallBlindLabel.Render("SB") + " "
	}
	if opts.IsBigBlind {
		chips += StyleBigBlindLabel.Render("BB") + " "
	}
	return chips
}

func renderStatusBadge(status game.PlayerStatus, isActing bool) string {
	if isActing {
		return StyleBadgeActing.Render("► ACT")
	}
	switch status {
	case game.StatusActive:
		return StyleBadgeActive.Render("●")
	case game.StatusFolded:
		return StyleBadgeFolded.Render("✗ fold")
	case game.StatusAllIn:
		return StyleBadgeAllIn.Render("⬆ all-in")
	case game.StatusSittingOut:
		return StyleBadgeSittingOut.Render("⏸ away")
	}
	return ""
}

func renderEmptySeat() string {
	style := StylePlayerPanel.BorderForeground(lipgloss.Color("#333333"))
	return style.Render(
		lipgloss.NewStyle().Foreground(lipgloss.Color("#333333")).Render("  empty seat  "),
	)
}

func truncate(s string, maxLen int) string {
	runes := []rune(s)
	if len(runes) <= maxLen {
		return s
	}
	return string(runes[:maxLen-1]) + "…"
}