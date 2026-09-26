package cluster

import (
	"strings"
	"testing"
	"time"
)

func TestParseLogLines(t *testing.T) {
	raw := "2026-08-30T12:00:00.000000000Z starting up\n" +
		"2026-08-30T12:00:01.000000000Z ERROR db connection refused\n"

	got, err := parseLogLines(strings.NewReader(raw))
	if err != nil {
		t.Fatalf("parseLogLines returned error: %v", err)
	}

	want := []LogLine{
		{Timestamp: time.Date(2026, 8, 30, 12, 0, 0, 0, time.UTC), Text: "starting up"},
		{Timestamp: time.Date(2026, 8, 30, 12, 0, 1, 0, time.UTC), Text: "ERROR db connection refused"},
	}

	if len(got) != len(want) {
		t.Fatalf("parseLogLines() = %d lines, want %d: %+v", len(got), len(want), got)
	}
	for i := range want {
		if !got[i].Timestamp.Equal(want[i].Timestamp) || got[i].Text != want[i].Text {
			t.Errorf("parseLogLines()[%d] = %+v, want %+v", i, got[i], want[i])
		}
	}
}

func TestParseLogLines_skipsBlankLines(t *testing.T) {
	raw := "2026-08-30T12:00:00.000000000Z line one\n\n2026-08-30T12:00:01.000000000Z line two\n"

	got, err := parseLogLines(strings.NewReader(raw))
	if err != nil {
		t.Fatalf("parseLogLines returned error: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("parseLogLines() = %d lines, want 2: %+v", len(got), got)
	}
}

func TestParseLogLines_lineWithoutTimestamp(t *testing.T) {
	got, err := parseLogLines(strings.NewReader("not a timestamped line\n"))
	if err != nil {
		t.Fatalf("parseLogLines returned error: %v", err)
	}
	if len(got) != 1 || got[0].Text != "not a timestamped line" || !got[0].Timestamp.IsZero() {
		t.Errorf("parseLogLines() = %+v, want single zero-timestamp line", got)
	}
}
