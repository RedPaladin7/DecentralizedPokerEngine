package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/RedPaladin7/DecentralizedPokerEngine/internal/game"
)


type GameStateMsg struct {
	State *game.GameState
}

type ActionResultMsg struct {
	Err string // empty = success
}

type WinnerMsg struct {
	WinnerIDs map[string]bool
	HandRanks map[string]string // playerID → "Full House, Aces full of Kings"
	Payouts   map[string]int64
}

type NetworkMsg struct {
	Text string
}

type ErrorMsg struct {
	Text string
}


type UIMode uint8

const (
	ModeSpectate UIMode = iota // watching, not our turn
	ModeBetting                // our turn — bet input widget active
	ModeShowdown               // hand over, showing results
	ModeLobby                  // waiting for players to join
	ModeError                  // fatal error overlay
)

type Model struct {
	GameState     *game.GameState
	LocalPlayerID string

	Mode      UIMode
	BetInput  BetInputState
	Log       *LogView
	WinnerIDs map[string]bool
	HandRanks map[string]string

	ErrorText string

	LobbyStatus string

	Width  int
	Height int

	OnAction func(game.Action)
}

func NewModel(localPlayerID string, onAction func(game.Action)) Model {
	return Model{
		LocalPlayerID: localPlayerID,
		Mode:          ModeLobby,
		Log:           NewLogView(),
		OnAction:      onAction,
		Width:         TableWidth,
		Height:        TableHeight + LogHeight + 4,
	}
}

func (m Model) Init() tea.Cmd {
	return nil
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {

	case tea.WindowSizeMsg:
		m.Width = msg.Width
		m.Height = msg.Height
		return m, nil

	case tea.KeyMsg:
		if msg.String() == "ctrl+c" || msg.String() == "q" && m.Mode != ModeBetting {
			return m, tea.Quit
		}
		return m.handleKey(msg)

	case GameStateMsg:
		prev := m.GameState
		m.GameState = msg.State

		if prev == nil || prev.Phase != msg.State.Phase {
			m.Log.AddPhase(msg.State.Phase)
		}

		switch msg.State.Phase {
		case game.PhaseSettled:
			m.Mode = ModeShowdown
		case game.PhaseWaiting:
			m.Mode = ModeLobby
		default:
			current := msg.State.CurrentPlayer()
			if current != nil && current.ID == m.LocalPlayerID {
				m.Mode = ModeBetting
				m.BetInput = NewBetInputState(current, msg.State)
			} else {
				m.Mode = ModeSpectate
			}
		}
		return m, nil

	case ActionResultMsg:
		if msg.Err != "" {
			m.Log.AddError(msg.Err)
			m.BetInput.Submitted = nil
		} else {
			m.Mode = ModeSpectate
		}
		return m, nil

	case WinnerMsg:
		m.WinnerIDs = msg.WinnerIDs
		m.HandRanks = msg.HandRanks
		m.Mode = ModeShowdown
		for id, payout := range msg.Payouts {
			if payout > 0 {
				name := m.playerName(id)
				rank := msg.HandRanks[id]
				m.Log.AddWinner(name, payout, rank)
			}
		}
		return m, nil

	case NetworkMsg:
		m.Log.AddNetwork(msg.Text)
		return m, nil

	case ErrorMsg:
		m.ErrorText = msg.Text
		m.Mode = ModeError
		m.Log.AddError(msg.Text)
		return m, nil
	}

	return m, nil
}

func (m Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch m.Mode {

	case ModeBetting:
		return m.handleBettingKey(msg)

	case ModeShowdown, ModeSpectate:
		switch msg.String() {
		case "up", "k":
			m.Log.ScrollUp()
		case "down", "j":
			m.Log.ScrollDown()
		}

	case ModeLobby:

	case ModeError:
		if msg.String() == "enter" || msg.String() == "esc" {
			m.Mode = ModeSpectate
			m.ErrorText = ""
		}
	}

	return m, nil
}

func (m Model) handleBettingKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.BetInput.InputActive {
		switch msg.String() {
		case "enter":
			m.BetInput.InputActive = false
			return m.submitAction()
		case "esc":
			m.BetInput.InputActive = false
			m.BetInput.RaiseInput = ""
		case "backspace":
			m.BetInput.Backspace()
		default:
			if len(msg.String()) == 1 {
				m.BetInput.AppendChar(rune(msg.String()[0]))
			}
		}
		return m, nil
	}

	switch msg.String() {
	case "right", "l", "tab":
		m.BetInput.SelectNext()
	case "left", "h", "shift+tab":
		m.BetInput.SelectPrev()
	case "f":
		m.BetInput.Selected = 0 // fold
	case "c":
		m.BetInput.Selected = 1 // check/call
	case "r":
		m.BetInput.ActivateInput()
	case "a":
		m.BetInput.Selected = 3 // all-in
	case "enter", " ":
		if m.BetInput.Selected == 2 && !m.BetInput.InputActive {
			m.BetInput.ActivateInput()
		} else {
			return m.submitAction()
		}
	case "up", "k":
		m.Log.ScrollUp()
	case "down", "j":
		m.Log.ScrollDown()
	}

	return m, nil
}

func (m Model) submitAction() (tea.Model, tea.Cmd) {
	betAction, errStr := m.BetInput.Confirm()
	if errStr != "" {
		m.Log.AddError(errStr)
		return m, nil
	}

	a := game.Action{
		PlayerID: m.LocalPlayerID,
		Type:     betAction.Type,
		Amount:   betAction.Amount,
	}

	m.Log.AddAction(m.playerName(m.LocalPlayerID), a)

	if m.OnAction != nil {
		m.OnAction(a)
	}

	m.Mode = ModeSpectate
	return m, nil
}


func (m Model) View() string {
	switch m.Mode {
	case ModeLobby:
		return m.viewLobby()
	case ModeError:
		return m.viewError()
	}

	tableOpts := TableViewOpts{
		LocalPlayerID:  m.LocalPlayerID,
		WinnerIDs:      m.WinnerIDs,
		HandRanks:      m.HandRanks,
	}
	if m.GameState != nil {
		tableOpts.DealerIdx = m.GameState.DealerIdx
		current := m.GameState.CurrentPlayer()
		if current != nil {
			tableOpts.ActingPlayerID = current.ID
		}
	}

	table := RenderTable(m.GameState, tableOpts)

	var overlay string
	switch m.Mode {
	case ModeBetting:
		overlay = "\n" + centreInWidth(RenderBetInput(m.BetInput), m.Width)
	case ModeShowdown:
		overlay = "\n" + centreInWidth(m.renderShowdownBanner(), m.Width)
	}

	logPanel := m.Log.Render()
	keybinds := m.renderKeybinds()

	return lipgloss.JoinVertical(lipgloss.Left,
		table,
		overlay,
		logPanel,
		keybinds,
	)
}

func (m Model) viewLobby() string {
	title := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("#f0c040")).
		Align(lipgloss.Center).
		Width(TableWidth).
		Render("♠ ♥  P2P POKER  ♦ ♣")

	status := m.LobbyStatus
	if status == "" {
		status = "Waiting for players to join..."
	}

	statusLine := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#aaaaaa")).
		Align(lipgloss.Center).
		Width(TableWidth).
		Render(status)

	hint := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#555555")).
		Align(lipgloss.Center).
		Width(TableWidth).
		Render("ctrl+c to quit")

	felt := lipgloss.NewStyle().
		Background(feltGreen).
		Width(TableWidth).
		Height(TableHeight).
		Padding(TableHeight/3, 0)

	inner := lipgloss.JoinVertical(lipgloss.Center, title, "", statusLine, "", hint)
	return felt.Render(inner) + "\n" + m.Log.Render()
}

func (m Model) viewError() string {
	box := lipgloss.NewStyle().
		Border(lipgloss.DoubleBorder()).
		BorderForeground(lipgloss.Color("#e05050")).
		Padding(1, 3).
		Render(
			lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#e05050")).Render("ERROR") +
				"\n\n" + m.ErrorText + "\n\nPress Enter to continue",
		)
	return centreInWidth(box, m.Width) + "\n" + m.Log.Render()
}

func (m Model) renderShowdownBanner() string {
	if m.WinnerIDs == nil || m.GameState == nil {
		return ""
	}

	var winners []string
	for id := range m.WinnerIDs {
		name := m.playerName(id)
		if rank, ok := m.HandRanks[id]; ok && rank != "" {
			winners = append(winners, fmt.Sprintf("%s  (%s)", name, rank))
		} else {
			winners = append(winners, name)
		}
	}

	text := "🏆  " + strings.Join(winners, "  •  ")
	return lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("#f0c040")).
		Background(feltGreenDark).
		Padding(0, 3).
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("#f0c040")).
		Render(text)
}

func (m Model) renderKeybinds() string {
	var hints string
	switch m.Mode {
	case ModeBetting:
		hints = "f fold  c check/call  r raise  a all-in  ←/→ select  Enter confirm  q quit"
	case ModeShowdown:
		hints = "↑/↓ scroll log  q quit"
	default:
		hints = "↑/↓ scroll log  q quit"
	}
	return lipgloss.NewStyle().
		Foreground(lipgloss.Color("#444444")).
		Background(lipgloss.Color("#111111")).
		Width(m.Width).
		Padding(0, 1).
		Render(hints)
}


func (m Model) playerName(playerID string) string {
	if m.GameState == nil {
		return playerID
	}
	for _, p := range m.GameState.Players {
		if p.ID == playerID {
			return p.Name
		}
	}
	return playerID
}

func centreInWidth(content string, width int) string {
	return lipgloss.NewStyle().Width(width).Align(lipgloss.Center).Render(content)
}