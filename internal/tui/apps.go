package tui

import (
	"context"
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/WompratHunter/wheelman/internal/cluster"
	"github.com/WompratHunter/wheelman/internal/config"
	"github.com/WompratHunter/wheelman/internal/domain"
)

// appsStep is where the user is in the add-App flow.
type appsStep int

const (
	stepPick appsStep = iota
	stepName
	stepConfirm
	stepAdded
)

type candidatesMsg struct {
	candidates []cluster.Workload
	err        error
}

type appAddedMsg struct {
	app domain.AppConfig
	err error
}

type appsModel struct {
	configurator *config.Configurator

	configured []domain.AppConfig
	loadErr    error

	candidates    []cluster.Workload
	candidatesErr error
	loading       bool
	cursor        int
	maxVisible    int

	step     appsStep
	input    textinput.Model
	selected cluster.Workload
	addErr   error
	added    domain.AppConfig
}

func newAppsModel(configurator *config.Configurator) appsModel {
	input := textinput.New()
	input.Prompt = "Name: "
	input.Placeholder = "e.g. checkout"

	m := appsModel{configurator: configurator, input: input, loading: true, maxVisible: 10}
	m.configured, m.loadErr = configurator.ListApps()
	return m
}

// setSize fits the candidate list to the space left after the configured
// Apps list and the surrounding headings and hints.
func (m *appsModel) setSize(height int) {
	if height <= 0 {
		return
	}
	m.maxVisible = max(height-len(m.configured)-12, 3)
}

func (m appsModel) loadCandidates() tea.Cmd {
	configurator := m.configurator
	return func() tea.Msg {
		candidates, err := configurator.ListCandidates(context.Background())
		return candidatesMsg{candidates: candidates, err: err}
	}
}

func (m appsModel) addApp(app domain.AppConfig) tea.Cmd {
	configurator := m.configurator
	return func() tea.Msg {
		added, err := configurator.AddApp(context.Background(), app)
		return appAddedMsg{app: added, err: err}
	}
}

func (m appsModel) update(msg tea.Msg) (appsModel, tea.Cmd) {
	switch msg := msg.(type) {
	case candidatesMsg:
		m.loading = false
		m.candidates, m.candidatesErr = msg.candidates, msg.err
		return m, nil

	case appAddedMsg:
		if msg.err != nil {
			m.addErr = msg.err
			m.step = stepName
			return m, m.input.Focus()
		}
		m.added = msg.app
		m.step = stepAdded
		m.configured, m.loadErr = m.configurator.ListApps()
		return m, nil

	case tea.KeyMsg:
		switch m.step {
		case stepPick, stepAdded:
			return m.updatePick(msg)
		case stepName:
			return m.updateName(msg)
		case stepConfirm:
			return m.updateConfirm(msg)
		}
	}
	return m, nil
}

func (m appsModel) updatePick(msg tea.KeyMsg) (appsModel, tea.Cmd) {
	if m.step == stepAdded && msg.String() == "enter" {
		m.step = stepPick
		return m, switchTo(ScreenQuery)
	}
	switch msg.String() {
	case "up", "k":
		m.step = stepPick
		if m.cursor > 0 {
			m.cursor--
		}
	case "down", "j":
		m.step = stepPick
		if m.cursor < len(m.candidates)-1 {
			m.cursor++
		}
	case "r":
		m.step = stepPick
		m.loading = true
		return m, m.loadCandidates()
	case "enter":
		if len(m.candidates) == 0 {
			return m, nil
		}
		m.selected = m.candidates[m.cursor]
		m.addErr = nil
		m.step = stepName
		m.input.SetValue("")
		return m, m.input.Focus()
	}
	return m, nil
}

func (m appsModel) updateName(msg tea.KeyMsg) (appsModel, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.step = stepPick
		m.input.Blur()
		return m, nil
	case "enter":
		if strings.TrimSpace(m.input.Value()) == "" {
			return m, nil
		}
		m.step = stepConfirm
		m.input.Blur()
		return m, nil
	}
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	return m, cmd
}

func (m appsModel) updateConfirm(msg tea.KeyMsg) (appsModel, tea.Cmd) {
	switch msg.String() {
	case "y", "enter":
		return m, m.addApp(domain.AppConfig{Name: strings.TrimSpace(m.input.Value()), Workload: m.selected})
	case "n", "esc":
		m.step = stepName
		return m, m.input.Focus()
	}
	return m, nil
}

func (m appsModel) view() string {
	var b strings.Builder

	b.WriteString("\nConfigured apps\n")
	switch {
	case m.loadErr != nil:
		b.WriteString("  " + errorStyle.Render("error: "+m.loadErr.Error()) + "\n")
	case len(m.configured) == 0:
		b.WriteString("  " + dimStyle.Render("(none)") + "\n")
	default:
		for _, app := range m.configured {
			b.WriteString(fmt.Sprintf("  %s  %s\n", app.Name, dimStyle.Render(formatWorkload(app.Workload))))
		}
	}

	b.WriteString("\nAdd app\n")
	switch m.step {
	case stepPick, stepAdded:
		b.WriteString(m.candidatesView())
		if m.step == stepAdded {
			b.WriteString("\n" + cursorStyle.Render(fmt.Sprintf("Added %q.", m.added.Name)) + " Press Enter or Tab to go to the Query screen.\n")
		} else {
			b.WriteString("\n" + dimStyle.Render("↑/↓: move · enter: select · r: refresh") + "\n")
		}
	case stepName:
		b.WriteString("  Workload: " + formatWorkload(m.selected) + "\n  " + m.input.View() + "\n")
		if m.addErr != nil {
			b.WriteString("  " + errorStyle.Render("error: "+m.addErr.Error()) + "\n")
		}
		b.WriteString("\n" + dimStyle.Render("enter: continue · esc: back") + "\n")
	case stepConfirm:
		b.WriteString(fmt.Sprintf("  Add App %q → %s? (y/n)\n", strings.TrimSpace(m.input.Value()), formatWorkload(m.selected)))
	}
	return b.String()
}

func (m appsModel) candidatesView() string {
	switch {
	case m.loading:
		return "  " + dimStyle.Render("Discovering workloads…") + "\n"
	case m.candidatesErr != nil:
		return "  " + errorStyle.Render("error: "+m.candidatesErr.Error()) + "\n"
	case len(m.candidates) == 0:
		return "  " + dimStyle.Render("(no Deployments or StatefulSets found)") + "\n"
	}
	start := max(0, min(m.cursor-m.maxVisible/2, len(m.candidates)-m.maxVisible))
	end := min(len(m.candidates), start+m.maxVisible)

	var b strings.Builder
	if start > 0 {
		b.WriteString("  " + dimStyle.Render(fmt.Sprintf("↑ %d more", start)) + "\n")
	}
	for i := start; i < end; i++ {
		w := m.candidates[i]
		line := "  " + formatWorkload(w)
		if i == m.cursor {
			line = cursorStyle.Render("› " + formatWorkload(w))
		}
		b.WriteString(line + "\n")
	}
	if end < len(m.candidates) {
		b.WriteString("  " + dimStyle.Render(fmt.Sprintf("↓ %d more", len(m.candidates)-end)) + "\n")
	}
	return b.String()
}

func formatWorkload(w cluster.Workload) string {
	return fmt.Sprintf("%s %s/%s", w.Kind, w.Namespace, w.Name)
}
