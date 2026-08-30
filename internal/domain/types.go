// Package domain holds Wheelman's core vocabulary: AppConfig, Filter, and
// Result. See CONTEXT.md for the canonical definitions.
package domain

import (
	"time"

	"github.com/WompratHunter/wheelman/internal/cluster"
)

// AppConfig is a user-named App: a pointer to one Deployment or StatefulSet,
// chosen by the user from a discovered list, together with the selector
// read from that workload's own spec.
type AppConfig struct {
	Name      string
	Kind      cluster.WorkloadKind
	Namespace string
	Workload  string
	Selector  cluster.Selector
}

// Filter is the structured, deterministic form a Query compiles down to:
// the scoped Apps, time range, and keyword/regex/severity conditions
// actually applied to log lines.
type Filter struct {
	// Apps are the App names in scope. Empty means all configured Apps.
	Apps []string

	// Since and Until bound the time range searched.
	Since time.Time
	Until time.Time

	// Severities are keyword/regex patterns (e.g. "ERROR", "WARN") matched
	// against raw log text.
	Severities []string

	// Keywords are literal keyword/regex search terms.
	Keywords []string
}

// ResultLine is one log line produced by running a Filter, tagged with its
// source App and pod so provenance stays visible when lines from multiple
// Apps are interleaved.
type ResultLine struct {
	Timestamp time.Time
	App       string
	Pod       string
	Text      string
}

// Result is the flat, chronologically-ordered stream of log lines produced
// by running a Filter.
type Result struct {
	Lines []ResultLine
}
