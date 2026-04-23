package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/RedPaladin7/DecentralizedPokerEngine/internal/game"
)

type LogEntryKind uint8

const (
	LogKindAction  LogEntryKind = iota // player betting action
	LogKindSystem                      // phase transitions, deal events
	LogKindWinner                      // pot award message
	LogKindError                       // fault / timeout / invalid action
	LogKindNetwork                     // peer join/leave/disconnect
)

type LogEntry struct {
	Kind      LogEntryKind
	Timestamp time.Time
	Text      string
}

type LogView struct {
	Entries    []LogEntry
	ScrollTop  int // index of the first visible entry
	MaxVisible int // how many lines fit (= LogHeight - 2 for borders)
}

func NewLogView() *LogView {
	return &LogView{MaxVisible: LogHeight - 2}
}

func (lv *LogView) Add(kind LogEntryKind, text string) {
	lv.Entries = append(lv.Entries, LogEntry{
		Kind:      kind,
		Timestamp: time.Now(),
		Text:      text,
	})
	if len(lv.Entries) > lv.MaxVisible {
		lv.ScrollTop = len(lv.Entries) - lv.MaxVisible
	}
}

func (lv *LogView) AddAction(playerName string, a game.Action) {
	var text string
	switch a.Type {
	case game.ActionFold:
		text = fmt.Sprintf("%s folds", playerName)
	case game.ActionCheck:
		text = fmt.Sprintf("%s checks", playerName)
	case game.ActionCall:
		text = fmt.Sprintf("%s calls", playerName)
	case game.ActionRaise:
		text = fmt.Sprintf("%s raises $%d", playerName, a.Amount)
	case game.ActionAllIn:
		text = fmt.Sprintf("%s goes ALL-IN", playerName)
	}
	lv.Add(LogKindAction, text)
}

func (lv *LogView) AddPhase(phase game.Phase) {
	lv.Add(LogKindSystem, fmt.Sprintf("── %s ──", strings.ToUpper(phase.String())))
}

func (lv *LogView) AddWinner(playerName string, amount int64, handRank string) {
	if handRank != "" {
		lv.Add(LogKindWinner, fmt.Sprintf("%s wins $%d with %s", playerName, amount, handRank))
	} else {
		lv.Add(LogKindWinner, fmt.Sprintf("%s wins $%d", playerName, amount))
	}
}

func (lv *LogView) AddSystem(text string) {
	lv.Add(LogKindSystem, text)
}

func (lv *LogView) AddError(text string) {
	lv.Add(LogKindError, text)
}

func (lv *LogView) AddNetwork(text string) {
	lv.Add(LogKindNetwork, text)
}

func (lv *LogView) ScrollUp() {
	if lv.ScrollTop > 0 {
		lv.ScrollTop--
	}
}

func (lv *LogView) ScrollDown() {
	max := len(lv.Entries) - lv.MaxVisible
	if max < 0 {
		max = 0
	}
	if lv.ScrollTop < max {
		lv.ScrollTop++
	}
}

func (lv *LogView) Render() string {
	end := lv.ScrollTop + lv.MaxVisible
	if end > len(lv.Entries) {
		end = len(lv.Entries)
	}
	visible := lv.Entries[lv.ScrollTop:end]

	lines := make([]string, lv.MaxVisible)
	for i := range lines {
		if i < len(visible) {
			lines[i] = renderLogEntry(visible[i])
		} else {
			lines[i] = "" // empty filler
		}
	}

	scrollIndicator := ""
	if len(lv.Entries) > lv.MaxVisible {
		total := len(lv.Entries)
		pos := lv.ScrollTop + lv.MaxVisible
		if pos > total {
			pos = total
		}
		scrollIndicator = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#444444")).
			Render(fmt.Sprintf(" [%d/%d] ↑↓ scroll", pos, total))
	}

	title := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#666666")).
		Render("ACTION LOG") + scrollIndicator

	content := title + "\n" + strings.Join(lines, "\n")
	return StyleLogPanel.Render(content)
}

func renderLogEntry(e LogEntry) string {
	ts := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#444444")).
		Render(e.Timestamp.Format("15:04:05") + " ")

	var body string
	switch e.Kind {
	case LogKindAction:
		body = StyleLogEntryAction.Render(e.Text)
	case LogKindSystem:
		body = StyleLogEntryHighlight.Render(e.Text)
	case LogKindWinner:
		body = StyleLogEntryWinner.Render(e.Text)
	case LogKindError:
		body = StyleLogEntryError.Render(e.Text)
	case LogKindNetwork:
		body = lipgloss.NewStyle().Foreground(lipgloss.Color("#5090e0")).Render(e.Text)
	default:
		body = StyleLogEntry.Render(e.Text)
	}

	return ts + body
}

func (lv *LogView) Len() int {
    return len(lv.Entries)
}