package query_test

import (
	"strings"
	"testing"
	"time"

	"github.com/WompratHunter/wheelman/internal/cluster"
	"github.com/WompratHunter/wheelman/internal/domain"
	"github.com/WompratHunter/wheelman/internal/query"
)

func newTestEngine(t *testing.T, apps []domain.AppConfig, client cluster.ClusterClient, now time.Time) *query.Engine {
	t.Helper()
	e := query.NewEngine(apps, client)
	e.Now = func() time.Time { return now }
	return e
}

func TestEngine_Run_searchesAllConfiguredAppsByDefault(t *testing.T) {
	now := time.Date(2026, 8, 30, 12, 0, 0, 0, time.UTC)

	checkoutWorkload := cluster.Workload{Kind: cluster.WorkloadKindDeployment, Namespace: "default", Name: "checkout", Selector: cluster.Selector{"app": "checkout"}}
	workerWorkload := cluster.Workload{Kind: cluster.WorkloadKindStatefulSet, Namespace: "default", Name: "worker", Selector: cluster.Selector{"app": "worker"}}

	checkoutPod := cluster.Pod{Namespace: "default", Name: "checkout-0"}
	workerPod := cluster.Pod{Namespace: "default", Name: "worker-0"}

	fake := cluster.NewFakeClusterClient()
	fake.SetPodsForWorkload(checkoutWorkload, []cluster.Pod{checkoutPod})
	fake.SetPodsForWorkload(workerWorkload, []cluster.Pod{workerPod})
	fake.SetLogsForPod(checkoutPod, []cluster.LogLine{
		{Timestamp: now.Add(-10 * time.Minute), Text: "boom: payment failed"},
	})
	fake.SetLogsForPod(workerPod, []cluster.LogLine{
		{Timestamp: now.Add(-5 * time.Minute), Text: "boom: queue drained"},
	})

	apps := []domain.AppConfig{
		{Name: "checkout", Workload: checkoutWorkload},
		{Name: "worker", Workload: workerWorkload},
	}

	e := newTestEngine(t, apps, fake, now)

	result, err := e.Run("boom")
	if err != nil {
		t.Fatalf("Run() returned error: %v", err)
	}

	if len(result.Lines) != 2 {
		t.Fatalf("Run() returned %d lines, want 2: %+v", len(result.Lines), result.Lines)
	}

	gotApps := map[string]bool{}
	for _, l := range result.Lines {
		gotApps[l.App] = true
	}
	if !gotApps["checkout"] || !gotApps["worker"] {
		t.Errorf("Run() lines = %+v, want lines tagged with both checkout and worker", result.Lines)
	}
}

func TestEngine_Run_defaultsToLastHourWindow(t *testing.T) {
	now := time.Date(2026, 8, 30, 12, 0, 0, 0, time.UTC)

	workload := cluster.Workload{Kind: cluster.WorkloadKindDeployment, Namespace: "default", Name: "checkout", Selector: cluster.Selector{"app": "checkout"}}
	pod := cluster.Pod{Namespace: "default", Name: "checkout-0"}

	fake := cluster.NewFakeClusterClient()
	fake.SetPodsForWorkload(workload, []cluster.Pod{pod})
	fake.SetLogsForPod(pod, []cluster.LogLine{
		{Timestamp: now.Add(-2 * time.Hour), Text: "match but too old"},
		{Timestamp: now.Add(-30 * time.Minute), Text: "match within window"},
	})

	apps := []domain.AppConfig{{Name: "checkout", Workload: workload}}
	e := newTestEngine(t, apps, fake, now)

	result, err := e.Run("match")
	if err != nil {
		t.Fatalf("Run() returned error: %v", err)
	}

	if len(result.Lines) != 1 {
		t.Fatalf("Run() returned %d lines, want 1: %+v", len(result.Lines), result.Lines)
	}
	if result.Lines[0].Text != "match within window" {
		t.Errorf("Run() line = %q, want %q", result.Lines[0].Text, "match within window")
	}
}

func TestEngine_Run_treatsFullQueryAsLiteralKeywordSearch(t *testing.T) {
	now := time.Date(2026, 8, 30, 12, 0, 0, 0, time.UTC)

	workload := cluster.Workload{Kind: cluster.WorkloadKindDeployment, Namespace: "default", Name: "checkout", Selector: cluster.Selector{"app": "checkout"}}
	pod := cluster.Pod{Namespace: "default", Name: "checkout-0"}

	fake := cluster.NewFakeClusterClient()
	fake.SetPodsForWorkload(workload, []cluster.Pod{pod})
	fake.SetLogsForPod(pod, []cluster.LogLine{
		{Timestamp: now.Add(-10 * time.Minute), Text: "ERROR db connection refused"},
		{Timestamp: now.Add(-9 * time.Minute), Text: "starting up"},
	})

	apps := []domain.AppConfig{{Name: "checkout", Workload: workload}}
	e := newTestEngine(t, apps, fake, now)

	result, err := e.Run("connection refused")
	if err != nil {
		t.Fatalf("Run() returned error: %v", err)
	}

	if len(result.Lines) != 1 {
		t.Fatalf("Run() returned %d lines, want 1: %+v", len(result.Lines), result.Lines)
	}
	if result.Lines[0].Text != "ERROR db connection refused" {
		t.Errorf("Run() line = %q, want %q", result.Lines[0].Text, "ERROR db connection refused")
	}
}

func TestEngine_Run_isCaseInsensitive(t *testing.T) {
	now := time.Date(2026, 8, 30, 12, 0, 0, 0, time.UTC)

	workload := cluster.Workload{Kind: cluster.WorkloadKindDeployment, Namespace: "default", Name: "checkout", Selector: cluster.Selector{"app": "checkout"}}
	pod := cluster.Pod{Namespace: "default", Name: "checkout-0"}

	fake := cluster.NewFakeClusterClient()
	fake.SetPodsForWorkload(workload, []cluster.Pod{pod})
	fake.SetLogsForPod(pod, []cluster.LogLine{
		{Timestamp: now.Add(-10 * time.Minute), Text: "ERROR db connection refused"},
	})

	apps := []domain.AppConfig{{Name: "checkout", Workload: workload}}
	e := newTestEngine(t, apps, fake, now)

	result, err := e.Run("error")
	if err != nil {
		t.Fatalf("Run() returned error: %v", err)
	}
	if len(result.Lines) != 1 {
		t.Fatalf("Run() returned %d lines, want 1: %+v", len(result.Lines), result.Lines)
	}
}

func TestEngine_Run_treatsInvalidRegexAsLiteralText(t *testing.T) {
	now := time.Date(2026, 8, 30, 12, 0, 0, 0, time.UTC)

	workload := cluster.Workload{Kind: cluster.WorkloadKindDeployment, Namespace: "default", Name: "checkout", Selector: cluster.Selector{"app": "checkout"}}
	pod := cluster.Pod{Namespace: "default", Name: "checkout-0"}

	fake := cluster.NewFakeClusterClient()
	fake.SetPodsForWorkload(workload, []cluster.Pod{pod})
	fake.SetLogsForPod(pod, []cluster.LogLine{
		{Timestamp: now.Add(-10 * time.Minute), Text: "cost is $5 (approx)"},
	})

	apps := []domain.AppConfig{{Name: "checkout", Workload: workload}}
	e := newTestEngine(t, apps, fake, now)

	// "(approx" is an invalid regex (unbalanced paren) but should still work
	// as a literal substring match rather than erroring out.
	result, err := e.Run("(approx")
	if err != nil {
		t.Fatalf("Run() returned error: %v", err)
	}
	if len(result.Lines) != 1 {
		t.Fatalf("Run() returned %d lines, want 1: %+v", len(result.Lines), result.Lines)
	}
}

func TestEngine_Run_resultIsChronologicallyOrderedAcrossPods(t *testing.T) {
	now := time.Date(2026, 8, 30, 12, 0, 0, 0, time.UTC)

	checkoutWorkload := cluster.Workload{Kind: cluster.WorkloadKindDeployment, Namespace: "default", Name: "checkout", Selector: cluster.Selector{"app": "checkout"}}
	workerWorkload := cluster.Workload{Kind: cluster.WorkloadKindStatefulSet, Namespace: "default", Name: "worker", Selector: cluster.Selector{"app": "worker"}}

	checkoutPod := cluster.Pod{Namespace: "default", Name: "checkout-0"}
	workerPod := cluster.Pod{Namespace: "default", Name: "worker-0"}

	fake := cluster.NewFakeClusterClient()
	fake.SetPodsForWorkload(checkoutWorkload, []cluster.Pod{checkoutPod})
	fake.SetPodsForWorkload(workerWorkload, []cluster.Pod{workerPod})
	fake.SetLogsForPod(checkoutPod, []cluster.LogLine{
		{Timestamp: now.Add(-30 * time.Minute), Text: "match c1"},
		{Timestamp: now.Add(-10 * time.Minute), Text: "match c2"},
	})
	fake.SetLogsForPod(workerPod, []cluster.LogLine{
		{Timestamp: now.Add(-20 * time.Minute), Text: "match w1"},
	})

	apps := []domain.AppConfig{
		{Name: "checkout", Workload: checkoutWorkload},
		{Name: "worker", Workload: workerWorkload},
	}
	e := newTestEngine(t, apps, fake, now)

	result, err := e.Run("match")
	if err != nil {
		t.Fatalf("Run() returned error: %v", err)
	}

	wantOrder := []string{"match c1", "match w1", "match c2"}
	if len(result.Lines) != len(wantOrder) {
		t.Fatalf("Run() returned %d lines, want %d: %+v", len(result.Lines), len(wantOrder), result.Lines)
	}
	for i, want := range wantOrder {
		if result.Lines[i].Text != want {
			t.Errorf("Run() line[%d] = %q, want %q", i, result.Lines[i].Text, want)
		}
	}
}

func TestEngine_Run_taggedWithSourceAppAndPod(t *testing.T) {
	now := time.Date(2026, 8, 30, 12, 0, 0, 0, time.UTC)

	workload := cluster.Workload{Kind: cluster.WorkloadKindDeployment, Namespace: "default", Name: "checkout", Selector: cluster.Selector{"app": "checkout"}}
	pod := cluster.Pod{Namespace: "default", Name: "checkout-0"}

	fake := cluster.NewFakeClusterClient()
	fake.SetPodsForWorkload(workload, []cluster.Pod{pod})
	fake.SetLogsForPod(pod, []cluster.LogLine{
		{Timestamp: now.Add(-10 * time.Minute), Text: "match"},
	})

	apps := []domain.AppConfig{{Name: "checkout", Workload: workload}}
	e := newTestEngine(t, apps, fake, now)

	result, err := e.Run("match")
	if err != nil {
		t.Fatalf("Run() returned error: %v", err)
	}
	if len(result.Lines) != 1 {
		t.Fatalf("Run() returned %d lines, want 1", len(result.Lines))
	}
	line := result.Lines[0]
	if line.App != "checkout" {
		t.Errorf("line.App = %q, want %q", line.App, "checkout")
	}
	if line.Pod != pod {
		t.Errorf("line.Pod = %+v, want %+v", line.Pod, pod)
	}
}

func TestEngine_Run_scopesToNamedApp(t *testing.T) {
	now := time.Date(2026, 8, 30, 12, 0, 0, 0, time.UTC)

	checkoutWorkload := cluster.Workload{Kind: cluster.WorkloadKindDeployment, Namespace: "default", Name: "checkout", Selector: cluster.Selector{"app": "checkout"}}
	workerWorkload := cluster.Workload{Kind: cluster.WorkloadKindStatefulSet, Namespace: "default", Name: "worker", Selector: cluster.Selector{"app": "worker"}}

	checkoutPod := cluster.Pod{Namespace: "default", Name: "checkout-0"}
	workerPod := cluster.Pod{Namespace: "default", Name: "worker-0"}

	fake := cluster.NewFakeClusterClient()
	fake.SetPodsForWorkload(checkoutWorkload, []cluster.Pod{checkoutPod})
	fake.SetPodsForWorkload(workerWorkload, []cluster.Pod{workerPod})
	fake.SetLogsForPod(checkoutPod, []cluster.LogLine{
		{Timestamp: now.Add(-10 * time.Minute), Text: "boom: payment failed"},
	})
	fake.SetLogsForPod(workerPod, []cluster.LogLine{
		{Timestamp: now.Add(-5 * time.Minute), Text: "boom: queue drained"},
	})

	apps := []domain.AppConfig{
		{Name: "checkout", Workload: checkoutWorkload},
		{Name: "worker", Workload: workerWorkload},
	}
	e := newTestEngine(t, apps, fake, now)

	result, err := e.Run("app:checkout boom")
	if err != nil {
		t.Fatalf("Run() returned error: %v", err)
	}
	if len(result.Lines) != 1 {
		t.Fatalf("Run() returned %d lines, want 1: %+v", len(result.Lines), result.Lines)
	}
	if result.Lines[0].App != "checkout" {
		t.Errorf("Run() line.App = %q, want %q", result.Lines[0].App, "checkout")
	}
}

func TestEngine_Run_scopesToMultipleNamedApps(t *testing.T) {
	now := time.Date(2026, 8, 30, 12, 0, 0, 0, time.UTC)

	checkoutWorkload := cluster.Workload{Kind: cluster.WorkloadKindDeployment, Namespace: "default", Name: "checkout", Selector: cluster.Selector{"app": "checkout"}}
	workerWorkload := cluster.Workload{Kind: cluster.WorkloadKindStatefulSet, Namespace: "default", Name: "worker", Selector: cluster.Selector{"app": "worker"}}
	billingWorkload := cluster.Workload{Kind: cluster.WorkloadKindDeployment, Namespace: "default", Name: "billing", Selector: cluster.Selector{"app": "billing"}}

	checkoutPod := cluster.Pod{Namespace: "default", Name: "checkout-0"}
	workerPod := cluster.Pod{Namespace: "default", Name: "worker-0"}
	billingPod := cluster.Pod{Namespace: "default", Name: "billing-0"}

	fake := cluster.NewFakeClusterClient()
	fake.SetPodsForWorkload(checkoutWorkload, []cluster.Pod{checkoutPod})
	fake.SetPodsForWorkload(workerWorkload, []cluster.Pod{workerPod})
	fake.SetPodsForWorkload(billingWorkload, []cluster.Pod{billingPod})
	fake.SetLogsForPod(checkoutPod, []cluster.LogLine{
		{Timestamp: now.Add(-10 * time.Minute), Text: "boom: payment failed"},
	})
	fake.SetLogsForPod(workerPod, []cluster.LogLine{
		{Timestamp: now.Add(-5 * time.Minute), Text: "boom: queue drained"},
	})
	fake.SetLogsForPod(billingPod, []cluster.LogLine{
		{Timestamp: now.Add(-5 * time.Minute), Text: "boom: invoice failed"},
	})

	apps := []domain.AppConfig{
		{Name: "checkout", Workload: checkoutWorkload},
		{Name: "worker", Workload: workerWorkload},
		{Name: "billing", Workload: billingWorkload},
	}
	e := newTestEngine(t, apps, fake, now)

	result, err := e.Run("app:checkout app:worker boom")
	if err != nil {
		t.Fatalf("Run() returned error: %v", err)
	}

	gotApps := map[string]bool{}
	for _, l := range result.Lines {
		gotApps[l.App] = true
	}
	if len(result.Lines) != 2 || !gotApps["checkout"] || !gotApps["worker"] {
		t.Errorf("Run() lines = %+v, want lines tagged with checkout and worker only", result.Lines)
	}
}

func TestEngine_Run_namedAppWithNoRemainingKeywordMatchesAllLines(t *testing.T) {
	now := time.Date(2026, 8, 30, 12, 0, 0, 0, time.UTC)

	checkoutWorkload := cluster.Workload{Kind: cluster.WorkloadKindDeployment, Namespace: "default", Name: "checkout", Selector: cluster.Selector{"app": "checkout"}}
	workerWorkload := cluster.Workload{Kind: cluster.WorkloadKindStatefulSet, Namespace: "default", Name: "worker", Selector: cluster.Selector{"app": "worker"}}

	checkoutPod := cluster.Pod{Namespace: "default", Name: "checkout-0"}
	workerPod := cluster.Pod{Namespace: "default", Name: "worker-0"}

	fake := cluster.NewFakeClusterClient()
	fake.SetPodsForWorkload(checkoutWorkload, []cluster.Pod{checkoutPod})
	fake.SetPodsForWorkload(workerWorkload, []cluster.Pod{workerPod})
	fake.SetLogsForPod(checkoutPod, []cluster.LogLine{
		{Timestamp: now.Add(-10 * time.Minute), Text: "anything at all"},
	})
	fake.SetLogsForPod(workerPod, []cluster.LogLine{
		{Timestamp: now.Add(-5 * time.Minute), Text: "should not appear"},
	})

	apps := []domain.AppConfig{
		{Name: "checkout", Workload: checkoutWorkload},
		{Name: "worker", Workload: workerWorkload},
	}
	e := newTestEngine(t, apps, fake, now)

	result, err := e.Run("app:checkout")
	if err != nil {
		t.Fatalf("Run() returned error: %v", err)
	}
	if len(result.Lines) != 1 {
		t.Fatalf("Run() returned %d lines, want 1: %+v", len(result.Lines), result.Lines)
	}
	if result.Lines[0].App != "checkout" {
		t.Errorf("Run() line.App = %q, want %q", result.Lines[0].App, "checkout")
	}
}

func TestEngine_Run_appNameMatchingIsCaseInsensitive(t *testing.T) {
	now := time.Date(2026, 8, 30, 12, 0, 0, 0, time.UTC)

	workload := cluster.Workload{Kind: cluster.WorkloadKindDeployment, Namespace: "default", Name: "checkout", Selector: cluster.Selector{"app": "checkout"}}
	pod := cluster.Pod{Namespace: "default", Name: "checkout-0"}

	fake := cluster.NewFakeClusterClient()
	fake.SetPodsForWorkload(workload, []cluster.Pod{pod})
	fake.SetLogsForPod(pod, []cluster.LogLine{
		{Timestamp: now.Add(-10 * time.Minute), Text: "boom"},
	})

	apps := []domain.AppConfig{{Name: "checkout", Workload: workload}}
	e := newTestEngine(t, apps, fake, now)

	result, err := e.Run("app:CHECKOUT boom")
	if err != nil {
		t.Fatalf("Run() returned error: %v", err)
	}
	if len(result.Lines) != 1 {
		t.Fatalf("Run() returned %d lines, want 1: %+v", len(result.Lines), result.Lines)
	}
}

func TestEngine_Run_unknownAppNameReturnsErrorAndNoResults(t *testing.T) {
	now := time.Date(2026, 8, 30, 12, 0, 0, 0, time.UTC)

	checkoutWorkload := cluster.Workload{Kind: cluster.WorkloadKindDeployment, Namespace: "default", Name: "checkout", Selector: cluster.Selector{"app": "checkout"}}
	checkoutPod := cluster.Pod{Namespace: "default", Name: "checkout-0"}

	fake := cluster.NewFakeClusterClient()
	fake.SetPodsForWorkload(checkoutWorkload, []cluster.Pod{checkoutPod})
	fake.SetLogsForPod(checkoutPod, []cluster.LogLine{
		{Timestamp: now.Add(-10 * time.Minute), Text: "boom"},
	})

	apps := []domain.AppConfig{
		{Name: "checkout", Workload: checkoutWorkload},
		{Name: "worker", Workload: cluster.Workload{Kind: cluster.WorkloadKindStatefulSet, Namespace: "default", Name: "worker"}},
	}
	e := newTestEngine(t, apps, fake, now)

	result, err := e.Run("app:nonexistent boom")
	if err == nil {
		t.Fatalf("Run() returned no error, want an error naming the unconfigured App")
	}
	if !strings.Contains(err.Error(), "nonexistent") {
		t.Errorf("Run() error = %q, want it to mention the unconfigured App name %q", err.Error(), "nonexistent")
	}
	if !strings.Contains(err.Error(), "checkout") || !strings.Contains(err.Error(), "worker") {
		t.Errorf("Run() error = %q, want it to list configured App names checkout and worker", err.Error())
	}
	if len(result.Lines) != 0 {
		t.Errorf("Run() returned %d lines, want 0 on error", len(result.Lines))
	}
}

func TestEngine_Run_unknownAppNameWithNoConfiguredApps(t *testing.T) {
	now := time.Date(2026, 8, 30, 12, 0, 0, 0, time.UTC)
	fake := cluster.NewFakeClusterClient()
	e := newTestEngine(t, nil, fake, now)

	_, err := e.Run("app:checkout boom")
	if err == nil {
		t.Fatalf("Run() returned no error, want an error naming the unconfigured App")
	}
}

func TestEngine_Run_noConfiguredApps(t *testing.T) {
	now := time.Date(2026, 8, 30, 12, 0, 0, 0, time.UTC)
	fake := cluster.NewFakeClusterClient()
	e := newTestEngine(t, nil, fake, now)

	result, err := e.Run("anything")
	if err != nil {
		t.Fatalf("Run() returned error: %v", err)
	}
	if len(result.Lines) != 0 {
		t.Errorf("Run() returned %d lines, want 0", len(result.Lines))
	}
}
