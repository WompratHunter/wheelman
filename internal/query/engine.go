// Package query implements the Engine that runs a Query end to end: compile
// to a Filter, resolve Apps/pods and fetch logs via the ClusterClient seam,
// apply the Filter client-side, and assemble a Result. See CONTEXT.md.
package query

import (
	"context"
	"fmt"
	"regexp"
	"sort"
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
// Beyond App-name recognition (via "app:<Name>" phrases), v1 has no further
// query grammar yet: any remaining query text is treated as a single literal
// keyword/regex search term, over the last 1 hour. See CONTEXT.md's
// "unrecognized text falls back to keyword search" rule.
func (e *Engine) Run(queryText string) (domain.Result, error) {
	now := e.Now()

	remaining, names := extractAppNames(queryText)
	scopedApps, canonicalNames, err := matchConfiguredApps(names, e.apps)
	if err != nil {
		return domain.Result{}, err
	}

	filter := domain.Filter{
		Apps:  canonicalNames,
		Since: now.Add(-defaultWindow),
		Until: now,
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

// matchConfiguredApps resolves names (as extracted by extractAppNames)
// against apps, matching case-insensitively. It returns the scoped Apps and
// their canonical (as-configured) names, or an error listing the configured
// App names if any name isn't configured. No names scopes to all of apps,
// per CONTEXT.md's "no named Apps -> all configured Apps" default.
func matchConfiguredApps(names []string, apps []domain.AppConfig) (scoped []domain.AppConfig, canonicalNames []string, err error) {
	if len(names) == 0 {
		return apps, nil, nil
	}
	for _, name := range names {
		app, ok := findAppByName(apps, name)
		if !ok {
			return nil, nil, fmt.Errorf("query: app %q is not configured; configured apps: %s", name, configuredAppNamesList(apps))
		}
		scoped = append(scoped, app)
		canonicalNames = append(canonicalNames, app.Name)
	}
	return scoped, canonicalNames, nil
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
