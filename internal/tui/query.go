package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/WompratHunter/wheelman/internal/cluster"
	"github.com/WompratHunter/wheelman/internal/config"
	"github.com/WompratHunter/wheelman/internal/domain"
	"github.com/WompratHunter/wheelman/internal/query"
)

// queryChromeHeight is the rows the Query screen spends outside the results
// viewport: header, status line, and input bar.
const queryChromeHeight = 3

type queryResultMsg struct {
	result domain.Result
	err    error
}

type queryModel struct {
	configurator *config.Configurator
	client       cluster.ClusterClient

	input   textinput.Model
	results viewport.Model
	status  string
	failed  bool
	running bool
	width   int
}

func newQueryModel(configurator *config.Configurator, client cluster.ClusterClient) queryModel {
	input := textinput.New()
	input.Prompt = "› "
	input.Placeholder = "e.g. app:checkout errors timeout last 30 minutes"

	results := viewport.New(80, 20)
	results.SetContent(dimStyle.Render("Type a Query and press Enter."))

	return queryModel{
		configurator: configurator,
		client:       client,
		input:        input,
		results:      results,
		status:       "No Query run yet",
		width:        80,
	}
}

func (m *queryModel) setSize(width, height int) {
	m.width = width
	m.input.Width = max(width-4, 1)
	m.results.Width = width
	m.results.Height = max(height-queryChromeHeight, 1)
}

func (m queryModel) update(msg tea.Msg) (queryModel, tea.Cmd) {
	switch msg := msg.(type) {
	case queryResultMsg:
		m.running = false
		if msg.err != nil {
			m.failed = true
			m.status = msg.err.Error()
			return m, nil
		}
		m.failed = false
		m.status = formatFilter(msg.result.Filter, len(msg.result.Lines))
		m.results.SetContent(formatResult(msg.result))
		m.results.GotoBottom()
		return m, nil

	case tea.KeyMsg:
		switch msg.String() {
		case "enter":
			if !m.input.Focused() {
				m.input.Focus()
				return m, nil
			}
			if m.running {
				return m, nil
			}
			m.running = true
			m.failed = false
			m.status = "Running…"
			return m, m.run(m.input.Value())
		case "esc":
			m.input.Blur()
			return m, nil
		case "pgup", "pgdown", "up", "down":
			var cmd tea.Cmd
			m.results, cmd = m.results.Update(msg)
			return m, cmd
		}
		if !m.input.Focused() {
			var cmd tea.Cmd
			m.results, cmd = m.results.Update(msg)
			return m, cmd
		}
	}

	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	return m, cmd
}

// run reloads the configured Apps on every Query so an App added on the
// Apps screen is queryable immediately, without restarting.
func (m queryModel) run(queryText string) tea.Cmd {
	configurator, client := m.configurator, m.client
	return func() tea.Msg {
		apps, err := configurator.ListApps()
		if err != nil {
			return queryResultMsg{err: fmt.Errorf("loading apps: %w", err)}
		}
		result, err := query.NewEngine(apps, client).Run(queryText)
		return queryResultMsg{result: result, err: err}
	}
}

func (m queryModel) view() string {
	status := m.status
	if m.failed {
		status = errorStyle.Render("error: ") + status
	}
	return m.results.View() + "\n" +
		statusStyle.Width(m.width).Render(status) + "\n" +
		m.input.View()
}

// formatFilter summarises a resolved Filter for the status line.
func formatFilter(f domain.Filter, lineCount int) string {
	apps := "all"
	if len(f.Apps) > 0 {
		apps = strings.Join(f.Apps, ", ")
	}
	parts := []string{
		"apps: " + apps,
		fmt.Sprintf("window: %s–%s (%s)", f.Since.Local().Format("15:04:05"), f.Until.Local().Format("15:04:05"), formatDuration(f.Until.Sub(f.Since))),
	}
	if len(f.Severities) > 0 {
		parts = append(parts, "severities: "+strings.Join(f.Severities, ", "))
	}
	if len(f.Keywords) > 0 {
		parts = append(parts, "keywords: "+strings.Join(f.Keywords, ", "))
	}
	parts = append(parts, fmt.Sprintf("%d lines", lineCount))
	return strings.Join(parts, " · ")
}

func formatDuration(d time.Duration) string {
	switch {
	case d >= time.Hour && d%time.Hour == 0:
		return fmt.Sprintf("%dh", d/time.Hour)
	case d >= time.Minute && d%time.Minute == 0:
		return fmt.Sprintf("%dm", d/time.Minute)
	default:
		return d.String()
	}
}

func formatResult(r domain.Result) string {
	if len(r.Lines) == 0 {
		return dimStyle.Render("No matching log lines.")
	}
	lines := make([]string, len(r.Lines))
	for i, l := range r.Lines {
		lines[i] = formatResultLine(l)
	}
	return strings.Join(lines, "\n")
}

func formatResultLine(l domain.ResultLine) string {
	return fmt.Sprintf("%s  %s/%s  %s", l.Timestamp.Local().Format("15:04:05"), l.App, l.Pod.Name, l.Text)
}
