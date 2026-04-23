package tui

import (
	"fmt"

	"github.com/RedPaladin7/DecentralizedPokerEngine/internal/game"
	"github.com/charmbracelet/lipgloss"
)

func RenderCard(c game.Card) string {
	text := fmt.Sprintf("%s%s", c.Rank, c.Suit)
	switch c.Suit {
	case game.Hearts, game.Diamonds:
		return StyleCardRed.Render(text)
	default:
		return StyleCardBlack.Render(text)
	}
}

func RenderCardBack() string {
	return StyleCardBack.Render("??")
}

func RenderCardPlaceHolder() string {
	style := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#2a5530")). 
		Background(feltGreen). 
		Padding(0, 1)
	return style.Render(" ")
}

func RenderHoleCards(cards [2]game.Card, reveal bool) string {
	zero := game.Card{}
	if cards[0] == zero && cards[1] == zero {
		return RenderCardPlaceHolder() + " " + RenderCardPlaceHolder()
	}
	if !reveal {
		return RenderCardBack() + " " + RenderCardBack()
	}

	return RenderCard(cards[0]) + " " + RenderCard(cards[1])
}

func RenderCommunityCards(cards []game.Card) string {
	slots := make([]string, 5)
	for i := range slots {
		if i < len(cards) {
			slots[i] = RenderCard(cards[i])
		} else {
			slots[i] = RenderCardPlaceHolder()
		}
	}

	flop := lipgloss.JoinHorizontal(lipgloss.Center, slots[0], " ", slots[1], " ", slots[2])
	turnRiver := lipgloss.JoinHorizontal(lipgloss.Center, slots[3], " ", slots[4])
	gap := lipgloss.NewStyle().Foreground(lipgloss.Color("#2a5540")).Render(" ")
	return lipgloss.JoinHorizontal(lipgloss.Center, flop, gap, turnRiver)
}

func RenderWinningHand(cards[5]game.Card) string {
	rendered := make([]string, 5)
	for i, c := range cards {
		rendered[i] = RenderCard(c)
	}
	return lipgloss.JoinHorizontal(
		lipgloss.Center, rendered[0], " ", rendered[1], " ", rendered[2], " ", rendered[3], " ", rendered[4],
	)
}