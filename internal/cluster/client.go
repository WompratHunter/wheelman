// Package cluster defines the ClusterClient seam: the sole boundary between
// Wheelman's domain logic and Kubernetes. Production code talks to a real
// cluster through a client-go-backed implementation; tests talk to an
// in-memory FakeClusterClient instead.
package cluster

import (
	"context"
	"time"
)

// WorkloadKind is the kind of workload an App can point at.
type WorkloadKind string

const (
	WorkloadKindDeployment  WorkloadKind = "Deployment"
	WorkloadKindStatefulSet WorkloadKind = "StatefulSet"
)

// Selector is a workload's pod label selector, read from the workload's own
// spec rather than authored by hand.
type Selector map[string]string

// Workload is a candidate Deployment or StatefulSet discovered in the
// current kubeconfig context, offered to the user when they set up an App.
type Workload struct {
	Kind      WorkloadKind
	Namespace string
	Name      string
	Selector  Selector
}

// Pod identifies a single pod backing an App.
type Pod struct {
	Namespace string
	Name      string
}

// LogLine is one raw, timestamped line from a pod's log stream.
type LogLine struct {
	Timestamp time.Time
	Text      string
}

// ClusterClient is the seam between domain logic and Kubernetes. It exposes
// exactly what App discovery and Query execution need: listing candidate
// workloads, resolving the pods behind a workload's selector, and fetching a
// pod's logs.
type ClusterClient interface {
	// ListWorkloads returns the Deployments and StatefulSets discoverable in
	// the current kubeconfig context, for App setup.
	ListWorkloads(ctx context.Context) ([]Workload, error)

	// ResolvePods returns the pods currently matching a workload's selector.
	ResolvePods(ctx context.Context, selector Selector) ([]Pod, error)

	// FetchLogs returns a pod's historical log lines.
	FetchLogs(ctx context.Context, pod Pod) ([]LogLine, error)
}
