package tui

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/WompratHunter/wheelman/internal/cluster"
	"github.com/WompratHunter/wheelman/internal/config"
	"github.com/WompratHunter/wheelman/internal/domain"
)

var checkoutWorkload = cluster.Workload{Kind: cluster.WorkloadKindDeployment, Namespace: "shop", Name: "checkout", Selector: cluster.Selector{"app": "checkout"}}

func newTestModel(t *testing.T, start Screen) (Model, *config.Configurator) {
	t.Helper()
	fake := cluster.NewFakeClusterClient()
	fake.AddWorkload(checkoutWorkload)
	pod := cluster.Pod{Namespace: "shop", Name: "checkout-abc"}
	fake.SetPodsForWorkload(checkoutWorkload, []cluster.Pod{pod})
	fake.SetLogsForPod(pod, []cluster.LogLine{
		{Timestamp: time.Now().Add(-time.Minute), Text: "ERROR payment timeout"},
		{Timestamp: time.Now().Add(-time.Minute), Text: "INFO ok"},
	})
	configurator := config.NewConfigurator(fake, config.NewFileStore(filepath.Join(t.TempDir(), "apps.json")))
	return New(configurator, fake, start), configurator
}

// send delivers msg and then drains Wheelman's own resulting messages
// synchronously, the way the bubbletea runtime would. Cursor-blink ticks
// block on a timer, so any command that doesn't return promptly is dropped.
func send(t *testing.T, m Model, msg tea.Msg) (Model, bool) {
	t.Helper()
	queue := []tea.Msg{msg}
	for len(queue) > 0 {
		next, cmd := m.Update(queue[0])
		m = next.(Model)
		queue = queue[1:]
		for _, out := range drain(cmd) {
			switch out.(type) {
			case tea.QuitMsg:
				return m, true
			case candidatesMsg, appAddedMsg, queryResultMsg, switchScreenMsg:
				queue = append(queue, out)
			}
		}
	}
	return m, false
}

func drain(cmd tea.Cmd) []tea.Msg {
	if cmd == nil {
		return nil
	}
	done := make(chan tea.Msg, 1)
	go func() { done <- cmd() }()
	select {
	case out := <-done:
		if batch, ok := out.(tea.BatchMsg); ok {
			var msgs []tea.Msg
			for _, c := range batch {
				msgs = append(msgs, drain(c)...)
			}
			return msgs
		}
		return []tea.Msg{out}
	case <-time.After(50 * time.Millisecond):
		return nil
	}
}

func key(s string) tea.KeyMsg {
	switch s {
	case "enter":
		return tea.KeyMsg{Type: tea.KeyEnter}
	case "tab":
		return tea.KeyMsg{Type: tea.KeyTab}
	case "esc":
		return tea.KeyMsg{Type: tea.KeyEsc}
	case "ctrl+c":
		return tea.KeyMsg{Type: tea.KeyCtrlC}
	}
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}
}

func typeText(t *testing.T, m Model, text string) Model {
	t.Helper()
	for _, r := range text {
		m, _ = send(t, m, key(string(r)))
	}
	return m
}

func TestModel_keys(t *testing.T) {
	tests := []struct {
		name       string
		start      Screen
		keys       []string
		wantQuit   bool
		wantScreen Screen
	}{
		{name: "ctrl+c quits from query", start: ScreenQuery, keys: []string{"ctrl+c"}, wantQuit: true},
		{name: "ctrl+c quits from apps", start: ScreenApps, keys: []string{"ctrl+c"}, wantQuit: true},
		{name: "q quits from apps", start: ScreenApps, keys: []string{"q"}, wantQuit: true},
		{name: "q is typed into focused query input", start: ScreenQuery, keys: []string{"q"}, wantScreen: ScreenQuery},
		{name: "q quits after blurring query input", start: ScreenQuery, keys: []string{"esc", "q"}, wantQuit: true},
		{name: "tab switches query to apps", start: ScreenQuery, keys: []string{"tab"}, wantScreen: ScreenApps},
		{name: "tab switches apps to query", start: ScreenApps, keys: []string{"tab"}, wantScreen: ScreenQuery},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m, _ := newTestModel(t, tt.start)
			quit := false
			for _, k := range tt.keys {
				m, quit = send(t, m, key(k))
			}
			if quit != tt.wantQuit {
				t.Fatalf("quit = %v, want %v", quit, tt.wantQuit)
			}
			if !tt.wantQuit && m.Active() != tt.wantScreen {
				t.Errorf("Active() = %v, want %v", m.Active(), tt.wantScreen)
			}
		})
	}
}

func TestModel_addAppThenQuery(t *testing.T) {
	m, configurator := newTestModel(t, ScreenApps)
	for _, msg := range drain(m.Init()) {
		m, _ = send(t, m, msg)
	}

	if !strings.Contains(m.View(), "(none)") {
		t.Fatalf("apps view missing (none) before add:\n%s", m.View())
	}

	m, _ = send(t, m, key("enter"))
	m = typeText(t, m, "checkout")
	m, _ = send(t, m, key("enter"))
	m, _ = send(t, m, key("y"))

	apps, err := configurator.ListApps()
	if err != nil || len(apps) != 1 || apps[0].Name != "checkout" {
		t.Fatalf("ListApps() = %+v, %v; want one App named checkout", apps, err)
	}
	if !strings.Contains(m.View(), `Added "checkout"`) {
		t.Errorf("apps view missing confirmation:\n%s", m.View())
	}

	m, _ = send(t, m, key("enter"))
	if m.Active() != ScreenQuery {
		t.Fatalf("Active() = %v after add+enter, want ScreenQuery", m.Active())
	}

	m = typeText(t, m, "app:checkout errors")
	m, _ = send(t, m, key("enter"))

	view := m.View()
	for _, want := range []string{"checkout/checkout-abc  ERROR payment timeout", "apps: checkout", "severities: ERROR", "1 lines"} {
		if !strings.Contains(view, want) {
			t.Errorf("query view missing %q:\n%s", want, view)
		}
	}
	if strings.Contains(view, "INFO ok") {
		t.Errorf("query view contains filtered-out line:\n%s", view)
	}
}

func TestModel_queryErrorShownInStatus(t *testing.T) {
	m, _ := newTestModel(t, ScreenQuery)
	m = typeText(t, m, "app:nope")
	m, _ = send(t, m, key("enter"))

	if !strings.Contains(m.View(), `error: query: app "nope" is not configured`) {
		t.Errorf("query view missing error:\n%s", m.View())
	}
}

func TestFormatFilter(t *testing.T) {
	until := time.Date(2026, 9, 26, 12, 0, 0, 0, time.Local)
	tests := []struct {
		name   string
		filter domain.Filter
		want   string
	}{
		{
			name:   "defaults",
			filter: domain.Filter{Since: until.Add(-time.Hour), Until: until},
			want:   "apps: all · window: 11:00:00–12:00:00 (1h) · 0 lines",
		},
		{
			name: "all conditions",
			filter: domain.Filter{
				Apps:       []string{"checkout", "worker"},
				Since:      until.Add(-30 * time.Minute),
				Until:      until,
				Severities: []string{"ERROR", "WARN"},
				Keywords:   []string{"timeout"},
			},
			want: "apps: checkout, worker · window: 11:30:00–12:00:00 (30m) · severities: ERROR, WARN · keywords: timeout · 0 lines",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := formatFilter(tt.filter, 0); got != tt.want {
				t.Errorf("formatFilter() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestFormatResultLine(t *testing.T) {
	line := domain.ResultLine{
		Timestamp: time.Date(2026, 9, 26, 9, 5, 7, 0, time.Local),
		App:       "checkout",
		Pod:       cluster.Pod{Namespace: "shop", Name: "checkout-abc"},
		Text:      "ERROR boom",
	}
	if got, want := formatResultLine(line), "09:05:07  checkout/checkout-abc  ERROR boom"; got != want {
		t.Errorf("formatResultLine() = %q, want %q", got, want)
	}
}
