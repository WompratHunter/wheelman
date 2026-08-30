package cluster_test

import (
	"context"
	"reflect"
	"testing"
	"time"

	"github.com/WompratHunter/wheelman/internal/cluster"
)

func TestFakeClusterClient_ListWorkloads(t *testing.T) {
	checkout := cluster.Workload{
		Kind:      cluster.WorkloadKindDeployment,
		Namespace: "default",
		Name:      "checkout",
		Selector:  cluster.Selector{"app": "checkout"},
	}
	worker := cluster.Workload{
		Kind:      cluster.WorkloadKindStatefulSet,
		Namespace: "default",
		Name:      "worker",
		Selector:  cluster.Selector{"app": "worker"},
	}

	fake := cluster.NewFakeClusterClient()
	fake.AddWorkload(checkout)
	fake.AddWorkload(worker)

	got, err := fake.ListWorkloads(context.Background())
	if err != nil {
		t.Fatalf("ListWorkloads returned error: %v", err)
	}

	want := []cluster.Workload{checkout, worker}
	if len(got) != len(want) {
		t.Fatalf("ListWorkloads() = %d workloads, want %d", len(got), len(want))
	}
	for i := range want {
		if !reflect.DeepEqual(got[i], want[i]) {
			t.Errorf("ListWorkloads()[%d] = %+v, want %+v", i, got[i], want[i])
		}
	}
}

func TestFakeClusterClient_ResolvePods(t *testing.T) {
	sel := cluster.Selector{"app": "checkout"}
	pods := []cluster.Pod{
		{Namespace: "default", Name: "checkout-0"},
		{Namespace: "default", Name: "checkout-1"},
	}

	fake := cluster.NewFakeClusterClient()
	fake.SetPodsForSelector(sel, pods)

	got, err := fake.ResolvePods(context.Background(), sel)
	if err != nil {
		t.Fatalf("ResolvePods returned error: %v", err)
	}
	if !reflect.DeepEqual(got, pods) {
		t.Errorf("ResolvePods() = %+v, want %+v", got, pods)
	}
}

func TestFakeClusterClient_ResolvePods_unconfiguredSelector(t *testing.T) {
	fake := cluster.NewFakeClusterClient()

	_, err := fake.ResolvePods(context.Background(), cluster.Selector{"app": "unknown"})
	if err == nil {
		t.Fatal("ResolvePods() with unconfigured selector: want error, got nil")
	}
}

func TestFakeClusterClient_FetchLogs(t *testing.T) {
	pod := cluster.Pod{Namespace: "default", Name: "checkout-0"}
	lines := []cluster.LogLine{
		{Timestamp: time.Date(2026, 8, 30, 12, 0, 0, 0, time.UTC), Text: "starting up"},
		{Timestamp: time.Date(2026, 8, 30, 12, 0, 1, 0, time.UTC), Text: "ERROR db connection refused"},
	}

	fake := cluster.NewFakeClusterClient()
	fake.SetLogsForPod(pod, lines)

	got, err := fake.FetchLogs(context.Background(), pod)
	if err != nil {
		t.Fatalf("FetchLogs returned error: %v", err)
	}
	if !reflect.DeepEqual(got, lines) {
		t.Errorf("FetchLogs() = %+v, want %+v", got, lines)
	}
}

func TestFakeClusterClient_FetchLogs_unconfiguredPod(t *testing.T) {
	fake := cluster.NewFakeClusterClient()

	_, err := fake.FetchLogs(context.Background(), cluster.Pod{Namespace: "default", Name: "unknown"})
	if err == nil {
		t.Fatal("FetchLogs() with unconfigured pod: want error, got nil")
	}
}
