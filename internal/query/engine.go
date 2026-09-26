// Package query implements the Engine that runs a Query end to end: compile
// to a Filter, resolve Apps/pods and fetch logs via the ClusterClient seam,
// apply the Filter client-side, and assemble a Result. See CONTEXT.md.
package query

import (
	"context"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/WompratHunter/wheelman/internal/cluster"
	"github.com/WompratHunter/wheelman/internal/domain"
)

// defaultWindow is the search window applied when a query names no time
// phrase. See CONTEXT.md's Query-to-Filter compilation rules.
const defaultWindow = time.Hour

// appPhrase recognizes an App-name phrase: the literal "app:" prefix
// immediately followed by the named App, e.g. "app:checkout". See
// CONTEXT.md's Query-to-Filter compilation rules.
var appPhrase = regexp.MustCompile(`(?i)\bapp:(\S+)`)

// timePhrase recognizes a relative time phrase, e.g. "last 30 minutes" or
// "in the last hour". The count is optional (defaulting to 1) so bare-unit
// phrases like "the last hour" are recognized. See CONTEXT.md's
// Query-to-Filter compilation rules.
var timePhrase = regexp.MustCompile(`(?i)\b(?:in\s+)?(?:the\s+)?last\s+(\d+\s+)?(minutes?|mins?|hours?|hrs?)\b`)

// timeUnits maps a recognized time phrase's unit token to its duration.
var timeUnits = map[string]time.Duration{
	"minute": time.Minute, "minutes": time.Minute, "min": time.Minute, "mins": time.Minute,
	"hour": time.Hour, "hours": time.Hour, "hr": time.Hour, "hrs": time.Hour,
}

// Engine is the single entry point for running a Query against Wheelman's
// configured Apps.
type Engine struct {
	apps   []domain.AppConfig
	client cluster.ClusterClient

	// Now returns the current time, used to anchor the default 1-hour
	// search window. Defaults to time.Now; tests may override it.
	Now func() time.Time
}

// NewEngine returns an Engine that searches the given Apps via client.
func NewEngine(apps []domain.AppConfig, client cluster.ClusterClient) *Engine {
	return &Engine{
		apps:   apps,
		client: client,
		Now:    time.Now,
	}
}

// Run parses queryText into a Filter and executes it, returning a flat,
// chronologically-ordered, App/pod-tagged Result.
//
// Beyond App-name recognition (via "app:<Name>" phrases) and relative time
// phrases (e.g. "last 30 minutes", "in the last hour", overriding the
// default 1-hour window), v1 has no further query grammar yet: any remaining
// query text is treated as a single literal keyword/regex search term. See
// CONTEXT.md's "unrecognized text falls back to keyword search" rule.
func (e *Engine) Run(queryText string) (domain.Result, error) {
	now := e.Now()

	remaining, names := extractAppNames(queryText)
	scopedApps, err := matchConfiguredApps(names, e.apps)
	if err != nil {
		return domain.Result{}, err
	}

	remaining, window := extractTimeWindow(remaining)

	filter := domain.Filter{
		Since: now.Add(-window),
		Until: now,
	}
	if len(names) > 0 {
		filter.Apps = make([]string, len(scopedApps))
		for i, app := range scopedApps {
			filter.Apps[i] = app.Name
		}
	}
	if remaining != "" {
		filter.Keywords = []string{remaining}
	}

	match := func(string) bool { return true }
	if len(filter.Keywords) > 0 {
		match = keywordMatcher(filter.Keywords[0])
	}

	ctx := context.Background()
	var lines []domain.ResultLine
	for _, app := range scopedApps {
		pods, err := e.client.ResolvePods(ctx, app.Workload)
		if err != nil {
			return domain.Result{}, fmt.Errorf("resolving pods for App %q: %w", app.Name, err)
		}
		for _, pod := range pods {
			logLines, err := e.client.FetchLogs(ctx, pod)
			if err != nil {
				return domain.Result{}, fmt.Errorf("fetching logs for App %q pod %s/%s: %w", app.Name, pod.Namespace, pod.Name, err)
			}
			for _, l := range logLines {
				if l.Timestamp.Before(filter.Since) || l.Timestamp.After(filter.Until) {
					continue
				}
				if !match(l.Text) {
					continue
				}
				lines = append(lines, domain.ResultLine{
					Timestamp: l.Timestamp,
					App:       app.Name,
					Pod:       pod,
					Text:      l.Text,
				})
			}
		}
	}

	sort.SliceStable(lines, func(i, j int) bool {
		return lines[i].Timestamp.Before(lines[j].Timestamp)
	})

	return domain.Result{Lines: lines}, nil
}

// extractAppNames pulls every "app:<Name>" phrase out of queryText, returning
// the named Apps (in first-seen order, deduplicated) and the remaining text
// with those phrases removed and whitespace collapsed.
func extractAppNames(queryText string) (remaining string, names []string) {
	seen := make(map[string]bool)
	remaining = appPhrase.ReplaceAllStringFunc(queryText, func(match string) string {
		name := appPhrase.FindStringSubmatch(match)[1]
		if !seen[name] {
			seen[name] = true
			names = append(names, name)
		}
		return ""
	})
	remaining = strings.TrimSpace(strings.Join(strings.Fields(remaining), " "))
	return remaining, names
}

// extractTimeWindow pulls the first recognized relative time phrase out of
// queryText, returning the remaining text with that phrase removed and
// whitespace collapsed, and the search window it names. A phrase with no
// explicit count (e.g. "the last hour") defaults to a count of 1. No
// recognized phrase returns queryText unchanged and defaultWindow, per
// CONTEXT.md's "no time phrase -> last 1 hour" default.
func extractTimeWindow(queryText string) (remaining string, window time.Duration) {
	window = defaultWindow
	found := false
	remaining = timePhrase.ReplaceAllStringFunc(queryText, func(match string) string {
		if found {
			return match
		}
		found = true
		groups := timePhrase.FindStringSubmatch(match)
		count := 1
		if countStr := strings.TrimSpace(groups[1]); countStr != "" {
			n, err := strconv.Atoi(countStr)
			if err == nil {
				count = n
			}
		}
		window = time.Duration(count) * timeUnits[strings.ToLower(groups[2])]
		return ""
	})
	remaining = strings.TrimSpace(strings.Join(strings.Fields(remaining), " "))
	return remaining, window
}

// matchConfiguredApps resolves names (as extracted by extractAppNames)
// against apps, matching case-insensitively. It returns the scoped Apps, or
// an error listing the configured App names if any name isn't configured. No
// names scopes to all of apps, per CONTEXT.md's "no named Apps -> all
// configured Apps" default.
func matchConfiguredApps(names []string, apps []domain.AppConfig) (scoped []domain.AppConfig, err error) {
	if len(names) == 0 {
		return apps, nil
	}
	for _, name := range names {
		app, ok := findAppByName(apps, name)
		if !ok {
			return nil, fmt.Errorf("query: app %q is not configured; configured apps: %s", name, configuredAppNamesList(apps))
		}
		scoped = append(scoped, app)
	}
	return scoped, nil
}

func findAppByName(apps []domain.AppConfig, name string) (domain.AppConfig, bool) {
	for _, app := range apps {
		if strings.EqualFold(app.Name, name) {
			return app, true
		}
	}
	return domain.AppConfig{}, false
}

func configuredAppNamesList(apps []domain.AppConfig) string {
	if len(apps) == 0 {
		return "(none configured)"
	}
	names := make([]string, len(apps))
	for i, app := range apps {
		names[i] = app.Name
	}
	sort.Strings(names)
	return strings.Join(names, ", ")
}

// keywordMatcher compiles term as a case-insensitive regex and returns a
// function testing whether a log line matches it. If term isn't a valid
// regex, it falls back to a literal (case-insensitive) substring match, so a
// query is never rejected merely for containing regex metacharacters.
func keywordMatcher(term string) func(text string) bool {
	if re, err := regexp.Compile("(?i)" + term); err == nil {
		return re.MatchString
	}
	literal := regexp.MustCompile("(?i)" + regexp.QuoteMeta(term))
	return literal.MatchString
}
