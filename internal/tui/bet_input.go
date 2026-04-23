package tui

import (
	"fmt"
	"strconv"
	"strings"
	"unicode"

	"github.com/RedPaladin7/DecentralizedPokerEngine/internal/game"
	"github.com/charmbracelet/lipgloss"
)

type BetAction struct {
	Type game.ActionType 
	Amount int64
}

type BetInputState struct {
	ToCall int64 
	MinRaise int64 
	Stack int64 
	CanCheck bool 

	Selected int 
	RaiseInput string 
	InputActive bool 
	Submitted *BetAction
}

func NewBetInputState(p *game.Player, gs *game.GameState) BetInputState {
	toCall := gs.CurrentBet - p.CurrentBet
	if toCall < 0 {
		toCall = 0 
	}
	return BetInputState{
		ToCall: toCall,
		MinRaise: gs.MinRaise,
		Stack: p.Stack,
		CanCheck: toCall == 0,
		Selected: 1,
	}
}

func RenderBetInput(s BetInputState) string {
	buttons := renderActionButtons(s)
	var raiseRow string 
	if s.Selected == 2{
		raiseRow = renderRaiseInput(s)
	}
	hint := renderHint(s)

	var lines []string 
	lines = append(lines, buttons)
	if raiseRow != "" {
		lines = append(lines, raiseRow)
	} 
	lines = append(lines, hint)
	
	content := lipgloss.JoinVertical(lipgloss.Left, lines...)
	return StyleBetPanel.Render(content)
}

func renderActionButtons(s BetInputState) string {
	type btn struct {
		label string 
		idx int 
		style lipgloss.Style
	}

	callLabel := "Call"
	if s.CanCheck {
		callLabel = "Check"
	} else if s.ToCall > 0 {
		callLabel = fmt.Sprintf("Call %d", s.ToCall)
	}

	buttons := []btn{
		{"Fold", 0, StyleBetButtonDanger},
		{callLabel, 1, StyleBetButton},
		{fmt.Sprintf("Raise"), 2, StyleBetButton},
		{"All-In", 3, StyleBetButton},
	}

	rendered := make([]string, len(buttons))
	for i, b := range buttons {
		style := b.style 
		if s.Selected == b.idx {
			style = StyleBetButtonSelected
		}
		rendered[i] = style.Render(b.label)
	}

	return lipgloss.JoinHorizontal(lipgloss.Center, rendered...)
}

func renderRaiseInput(s BetInputState) string {
	inputStyle := StyleBetInput 
	if s.InputActive {
		inputStyle = inputStyle.BorderForeground(lipgloss.Color("#f0c040"))
	}

	cursor := ""
	if s.InputActive {
		cursor = "█"
	}

	displayVal := s.RaiseInput 
	if displayVal == "" {
		displayVal = fmt.Sprintf("%d", s.MinRaise)
	}

	inputWidget := inputStyle.Render(displayVal + cursor)

	label := lipgloss.NewStyle(). 
		Foreground(lipgloss.Color("#aaaaaa")). 
		Render(fmt.Sprintf("Raise by: (min %d)", s.MinRaise))
	
	return lipgloss.JoinHorizontal(lipgloss.Left, label, " ", inputWidget)
}

func renderHint(s BetInputState) string {
	hintStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#555555"))
	if s.InputActive {
		return hintStyle.Render("type amount . Enter to confirm . Esc to cancel")
	}
	return hintStyle.Render("←/→ or h/l select · Enter confirm · r raise · a all-in · f fold")
}

func (s *BetInputState) SelectNext() {
	s.Selected = (s.Selected + 1) % 4 
	if s.Selected == 2 {
		s.InputActive = false 
	}
}

func (s *BetInputState) SelectPrev() {
	s.Selected = (s.Selected + 3) % 4
	if s.Selected == 2 {
		s.InputActive = false
	}
}

func (s *BetInputState) ActivateInput() {
	s.Selected = 2 
	s.InputActive = true 
	if s.RaiseInput == "" {
		s.RaiseInput = fmt.Sprintf("%d", s.MinRaise)
	}
}

func (s *BetInputState) AppendChar(r rune) {
	if unicode.IsDigit(r) && len(s.RaiseInput) < 10 {
		s.RaiseInput += string(r)
	}
}

func (s *BetInputState) Backspace() {
	if len(s.RaiseInput) > 0 {
		s.RaiseInput = s.RaiseInput[:len(s.RaiseInput)-1]
	}
}

func (s *BetInputState) Confirm() (*BetAction, string) {
	switch s.Selected {
	case 0:
		return &BetAction{Type: game.ActionFold}, ""

	case 1:
		if s.CanCheck {
			return &BetAction{Type: game.ActionCheck}, ""
		}
		return &BetAction{Type: game.ActionCall}, ""

	case 2:
		raw := strings.TrimSpace(s.RaiseInput)
		if raw == "" {
			raw = fmt.Sprintf("%d", s.MinRaise)
		}
		amount, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || amount <= 0 {
			return nil, "invalid raise amount"
		}
		if amount < s.MinRaise {
			return nil, fmt.Sprintf("raise must be at least %d", s.MinRaise)
		}
		if amount+s.ToCall > s.Stack {
			return nil, "insufficient funds"
		}
		return &BetAction{Type: game.ActionRaise, Amount: amount}, ""

	case 3:
		return &BetAction{Type: game.ActionAllIn}, ""
	}
	return nil, "unknown selection"
}