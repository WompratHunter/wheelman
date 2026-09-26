package cluster

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
)

// KubeClusterClient is the production ClusterClient, backed by client-go
// against whatever kubeconfig context is currently active. Per ADR-0001,
// Wheelman operates against exactly one active context at a time, matching
// plain kubectl behavior: it honors the KUBECONFIG environment variable and
// ~/.kube/config, and whatever context is marked "current" there.
type KubeClusterClient struct {
	clientset kubernetes.Interface
}

// NewKubeClusterClient builds a KubeClusterClient from the current
// kubeconfig context, using the same loading rules as kubectl (KUBECONFIG
// env var, falling back to ~/.kube/config, honoring the config's
// current-context).
func NewKubeClusterClient() (*KubeClusterClient, error) {
	loadingRules := clientcmd.NewDefaultClientConfigLoadingRules()
	config, err := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(
		loadingRules,
		&clientcmd.ConfigOverrides{},
	).ClientConfig()
	if err != nil {
		return nil, fmt.Errorf("cluster: loading kubeconfig: %w", err)
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("cluster: building clientset: %w", err)
	}

	return NewKubeClusterClientFromClientset(clientset), nil
}

// NewKubeClusterClientFromClientset builds a KubeClusterClient around an
// already-constructed clientset. Exposed mainly for tests that want to use
// client-go's fake clientset for the parts it supports (Deployment and
// StatefulSet listing, pod resolution); it doesn't support the logs
// subresource, so FetchLogs can't be exercised this way.
func NewKubeClusterClientFromClientset(clientset kubernetes.Interface) *KubeClusterClient {
	return &KubeClusterClient{clientset: clientset}
}

// ListWorkloads returns every Deployment and StatefulSet visible in the
// current kubeconfig context, across all namespaces the active credentials
// can list.
func (k *KubeClusterClient) ListWorkloads(ctx context.Context) ([]Workload, error) {
	var workloads []Workload

	deployments, err := k.clientset.AppsV1().Deployments("").List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("cluster: listing deployments: %w", err)
	}
	for _, d := range deployments.Items {
		workloads = append(workloads, workloadFromDeployment(d))
	}

	statefulSets, err := k.clientset.AppsV1().StatefulSets("").List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("cluster: listing statefulsets: %w", err)
	}
	for _, s := range statefulSets.Items {
		workloads = append(workloads, workloadFromStatefulSet(s))
	}

	return workloads, nil
}

// ResolvePods returns the pods currently matching workload's own selector,
// scoped to workload's namespace. The selector is read fresh from the
// workload rather than trusted from the caller's copy, so a stale or
// hand-edited Workload can't be used to reach into the wrong pods.
func (k *KubeClusterClient) ResolvePods(ctx context.Context, workload Workload) ([]Pod, error) {
	selector, err := k.currentSelector(ctx, workload)
	if err != nil {
		return nil, err
	}

	pods, err := k.clientset.CoreV1().Pods(workload.Namespace).List(ctx, metav1.ListOptions{
		LabelSelector: labels.SelectorFromSet(labels.Set(selector)).String(),
	})
	if err != nil {
		return nil, fmt.Errorf("cluster: listing pods for %s/%s: %w", workload.Namespace, workload.Name, err)
	}

	result := make([]Pod, 0, len(pods.Items))
	for _, p := range pods.Items {
		result = append(result, Pod{Namespace: p.Namespace, Name: p.Name})
	}
	return result, nil
}

// currentSelector re-reads workload's selector from the cluster, rather than
// trusting the Selector field on the Workload the caller passed in.
func (k *KubeClusterClient) currentSelector(ctx context.Context, workload Workload) (Selector, error) {
	switch workload.Kind {
	case WorkloadKindDeployment:
		d, err := k.clientset.AppsV1().Deployments(workload.Namespace).Get(ctx, workload.Name, metav1.GetOptions{})
		if err != nil {
			return nil, fmt.Errorf("cluster: getting deployment %s/%s: %w", workload.Namespace, workload.Name, err)
		}
		return selectorFromMatchLabels(d.Spec.Selector), nil
	case WorkloadKindStatefulSet:
		s, err := k.clientset.AppsV1().StatefulSets(workload.Namespace).Get(ctx, workload.Name, metav1.GetOptions{})
		if err != nil {
			return nil, fmt.Errorf("cluster: getting statefulset %s/%s: %w", workload.Namespace, workload.Name, err)
		}
		return selectorFromMatchLabels(s.Spec.Selector), nil
	default:
		return nil, fmt.Errorf("cluster: unsupported workload kind %q", workload.Kind)
	}
}

// FetchLogs returns pod's historical (non-streaming) log lines, via the pod
// logs subresource. Each line's timestamp comes from Kubernetes itself
// (requested via Timestamps: true), rather than being assigned locally.
func (k *KubeClusterClient) FetchLogs(ctx context.Context, pod Pod) ([]LogLine, error) {
	req := k.clientset.CoreV1().Pods(pod.Namespace).GetLogs(pod.Name, &corev1.PodLogOptions{
		Timestamps: true,
	})

	stream, err := req.Stream(ctx)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return nil, fmt.Errorf("cluster: pod %s/%s not found: %w", pod.Namespace, pod.Name, err)
		}
		return nil, fmt.Errorf("cluster: fetching logs for %s/%s: %w", pod.Namespace, pod.Name, err)
	}
	defer stream.Close()

	var buf bytes.Buffer
	if _, err := io.Copy(&buf, stream); err != nil {
		return nil, fmt.Errorf("cluster: reading logs for %s/%s: %w", pod.Namespace, pod.Name, err)
	}

	return parseLogLines(&buf)
}

// parseLogLines splits raw, RFC3339Nano-timestamped log output (as produced
// by PodLogOptions.Timestamps) into LogLines. A line that can't be parsed as
// "<timestamp> <text>" (e.g. an empty trailing line) is skipped rather than
// causing the whole fetch to fail.
func parseLogLines(r io.Reader) ([]LogLine, error) {
	var lines []LogLine
	scanner := bufio.NewScanner(r)
	// Log lines can be long; grow the buffer generously beyond bufio's default.
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}
		ts, text, ok := splitTimestamp(line)
		if !ok {
			lines = append(lines, LogLine{Text: line})
			continue
		}
		lines = append(lines, LogLine{Timestamp: ts, Text: text})
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("cluster: scanning log output: %w", err)
	}
	return lines, nil
}

// splitTimestamp splits a line of the form "<RFC3339Nano timestamp> <rest>"
// as emitted when PodLogOptions.Timestamps is set.
func splitTimestamp(line string) (time.Time, string, bool) {
	idx := strings.IndexByte(line, ' ')
	if idx < 0 {
		return time.Time{}, "", false
	}
	ts, err := time.Parse(time.RFC3339Nano, line[:idx])
	if err != nil {
		return time.Time{}, "", false
	}
	return ts, line[idx+1:], true
}

func workloadFromDeployment(d appsv1.Deployment) Workload {
	return Workload{
		Kind:      WorkloadKindDeployment,
		Namespace: d.Namespace,
		Name:      d.Name,
		Selector:  selectorFromMatchLabels(d.Spec.Selector),
	}
}

func workloadFromStatefulSet(s appsv1.StatefulSet) Workload {
	return Workload{
		Kind:      WorkloadKindStatefulSet,
		Namespace: s.Namespace,
		Name:      s.Name,
		Selector:  selectorFromMatchLabels(s.Spec.Selector),
	}
}

func selectorFromMatchLabels(selector *metav1.LabelSelector) Selector {
	if selector == nil || selector.MatchLabels == nil {
		return Selector{}
	}
	out := make(Selector, len(selector.MatchLabels))
	for k, v := range selector.MatchLabels {
		out[k] = v
	}
	return out
}

var _ ClusterClient = (*KubeClusterClient)(nil)
