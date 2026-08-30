// Package query implements the Engine that runs a Query end to end: compile
// to a Filter, resolve Apps/pods and fetch logs via the ClusterClient seam,
// apply the Filter client-side, and assemble a Result. See CONTEXT.md.
package query

import (
	"context"
	"fmt"
	"regexp"
	"sort"
	"time"

	"github.com/WompratHunter/wheelman/internal/cluster"
	"github.com/WompratHunter/wheelman/internal/domain"
)

// defaultWindow is the search window applied when a query names no time
// phrase. See CONTEXT.md's Query-to-Filter compilation rules.
const defaultWindow = time.Hour

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
// v1 has no query grammar yet: the entire query text is treated as a single
// literal keyword/regex search term, scoped to all configured Apps over the
// last 1 hour. See CONTEXT.md's "unrecognized text falls back to keyword
// search" rule.
func (e *Engine) Run(queryText string) (domain.Result, error) {
	now := e.Now()
	filter := domain.Filter{
		Since:    now.Add(-defaultWindow),
		Until:    now,
		Keywords: []string{queryText},
	}

	match := keywordMatcher(filter.Keywords[0])

	ctx := context.Background()
	var lines []domain.ResultLine
	for _, app := range e.apps {
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
