package cluster

import (
	"context"
	"fmt"
	"reflect"
)

// FakeClusterClient is an in-memory ClusterClient for tests. It is
// configured with canned workloads, pod resolutions, and per-pod log lines,
// so downstream tests never need a real cluster.
type FakeClusterClient struct {
	workloads      []Workload
	podsBySelector []fakePodsEntry
	logsByPod      map[Pod][]LogLine
}

type fakePodsEntry struct {
	selector Selector
	pods     []Pod
}

// NewFakeClusterClient returns an empty FakeClusterClient. Callers configure
// it with AddWorkload, SetPodsForSelector, and SetLogsForPod before use.
func NewFakeClusterClient() *FakeClusterClient {
	return &FakeClusterClient{
		logsByPod: make(map[Pod][]LogLine),
	}
}

// AddWorkload adds a candidate workload to be returned by ListWorkloads.
func (f *FakeClusterClient) AddWorkload(w Workload) {
	f.workloads = append(f.workloads, w)
}

// SetPodsForSelector configures the pods ResolvePods returns for a given
// selector.
func (f *FakeClusterClient) SetPodsForSelector(selector Selector, pods []Pod) {
	f.podsBySelector = append(f.podsBySelector, fakePodsEntry{selector: selector, pods: pods})
}

// SetLogsForPod configures the log lines FetchLogs returns for a given pod.
func (f *FakeClusterClient) SetLogsForPod(pod Pod, lines []LogLine) {
	f.logsByPod[pod] = lines
}

func (f *FakeClusterClient) ListWorkloads(ctx context.Context) ([]Workload, error) {
	return f.workloads, nil
}

func (f *FakeClusterClient) ResolvePods(ctx context.Context, selector Selector) ([]Pod, error) {
	for _, entry := range f.podsBySelector {
		if reflect.DeepEqual(entry.selector, selector) {
			return entry.pods, nil
		}
	}
	return nil, fmt.Errorf("fake cluster client: no pods configured for selector %v", selector)
}

func (f *FakeClusterClient) FetchLogs(ctx context.Context, pod Pod) ([]LogLine, error) {
	lines, ok := f.logsByPod[pod]
	if !ok {
		return nil, fmt.Errorf("fake cluster client: no logs configured for pod %s/%s", pod.Namespace, pod.Name)
	}
	return lines, nil
}

var _ ClusterClient = (*FakeClusterClient)(nil)
