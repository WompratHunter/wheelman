// Package tui is Wheelman's interactive terminal UI: a Query screen for
// running Queries and reading Results, and an Apps screen for configuring
// Apps from discovered workloads.
package tui

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/WompratHunter/wheelman/internal/cluster"
	"github.com/WompratHunter/wheelman/internal/config"
)

// Screen identifies which of the two screens is active.
type Screen int

const (
	ScreenQuery Screen = iota
	ScreenApps
)

var (
	headerStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("230")).Background(lipgloss.Color("62")).Padding(0, 1)
	statusStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("252")).Background(lipgloss.Color("237")).Padding(0, 1)
	errorStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("203"))
	dimStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("243"))
	cursorStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("212")).Bold(true)
)

// switchScreenMsg asks the top-level Model to make screen active.
type switchScreenMsg struct{ screen Screen }

func switchTo(screen Screen) tea.Cmd {
	return func() tea.Msg { return switchScreenMsg{screen: screen} }
}

// Model is the top-level bubbletea model: it owns both screens and routes
// input to whichever is active.
type Model struct {
	active Screen
	query  queryModel
	apps   appsModel
	width  int
	height int
}

// New returns a Model that opens on the given screen. Both screens share
// configurator (for listing and adding Apps) and client (for running
// Queries).
func New(configurator *config.Configurator, client cluster.ClusterClient, start Screen) Model {
	m := Model{
		active: start,
		query:  newQueryModel(configurator, client),
		apps:   newAppsModel(configurator),
	}
	m.setFocus()
	return m
}

// Active reports which screen is currently shown.
func (m Model) Active() Screen { return m.active }

func (m Model) Init() tea.Cmd {
	return m.apps.loadCandidates()
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c":
			return m, tea.Quit
		case "q":
			if !m.typing() {
				return m, tea.Quit
			}
		case "tab":
			return m.switchScreen(m.other()), nil
		}
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.query.setSize(msg.Width, msg.Height)
		m.apps.setSize(msg.Height)
		return m, nil
	case switchScreenMsg:
		return m.switchScreen(msg.screen), nil
	case candidatesMsg, appAddedMsg:
		var cmd tea.Cmd
		m.apps, cmd = m.apps.update(msg)
		m.apps.setSize(m.height)
		return m, cmd
	case queryResultMsg:
		var cmd tea.Cmd
		m.query, cmd = m.query.update(msg)
		return m, cmd
	}

	var cmd tea.Cmd
	if m.active == ScreenQuery {
		m.query, cmd = m.query.update(msg)
	} else {
		m.apps, cmd = m.apps.update(msg)
	}
	return m, cmd
}

func (m Model) View() string {
	title := "Wheelman · Query"
	hint := "tab: apps · esc then q / ctrl+c: quit"
	body := ""
	if m.active == ScreenApps {
		title = "Wheelman · Apps"
		hint = "tab: query · q / ctrl+c: quit"
		body = m.apps.view()
	} else {
		body = m.query.view()
	}
	header := headerStyle.Render(title) + " " + dimStyle.Render(hint)
	return header + "\n" + body
}

// typing reports whether a text input currently has focus, in which case
// "q" is ordinary input rather than a quit key.
func (m Model) typing() bool {
	if m.active == ScreenQuery {
		return m.query.input.Focused()
	}
	return m.apps.input.Focused()
}

func (m Model) other() Screen {
	if m.active == ScreenQuery {
		return ScreenApps
	}
	return ScreenQuery
}

func (m Model) switchScreen(screen Screen) Model {
	m.active = screen
	m.setFocus()
	return m
}

func (m *Model) setFocus() {
	if m.active == ScreenQuery {
		m.query.input.Focus()
	} else {
		m.query.input.Blur()
	}
}
