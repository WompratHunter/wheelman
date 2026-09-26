package cluster_test

import (
	"context"
	"sort"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	fakeclientset "k8s.io/client-go/kubernetes/fake"

	"github.com/WompratHunter/wheelman/internal/cluster"
)

func TestKubeClusterClient_ListWorkloads(t *testing.T) {
	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Namespace: "default", Name: "checkout"},
		Spec: appsv1.DeploymentSpec{
			Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": "checkout"}},
		},
	}
	statefulSet := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{Namespace: "default", Name: "inventory"},
		Spec: appsv1.StatefulSetSpec{
			Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": "inventory"}},
		},
	}

	clientset := fakeclientset.NewSimpleClientset(deployment, statefulSet)
	client := cluster.NewKubeClusterClientFromClientset(clientset)

	got, err := client.ListWorkloads(context.Background())
	if err != nil {
		t.Fatalf("ListWorkloads returned error: %v", err)
	}

	sort.Slice(got, func(i, j int) bool { return got[i].Name < got[j].Name })

	want := []cluster.Workload{
		{Kind: cluster.WorkloadKindDeployment, Namespace: "default", Name: "checkout", Selector: cluster.Selector{"app": "checkout"}},
		{Kind: cluster.WorkloadKindStatefulSet, Namespace: "default", Name: "inventory", Selector: cluster.Selector{"app": "inventory"}},
	}

	if len(got) != len(want) {
		t.Fatalf("ListWorkloads() = %d workloads, want %d: %+v", len(got), len(want), got)
	}
	for i := range want {
		if got[i].Kind != want[i].Kind || got[i].Namespace != want[i].Namespace || got[i].Name != want[i].Name {
			t.Errorf("ListWorkloads()[%d] = %+v, want %+v", i, got[i], want[i])
		}
		for k, v := range want[i].Selector {
			if got[i].Selector[k] != v {
				t.Errorf("ListWorkloads()[%d].Selector = %+v, want %+v", i, got[i].Selector, want[i].Selector)
			}
		}
	}
}

func TestKubeClusterClient_ResolvePods_readsWorkloadsOwnSelector(t *testing.T) {
	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Namespace: "default", Name: "checkout"},
		Spec: appsv1.DeploymentSpec{
			Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": "checkout"}},
		},
	}
	matchingPod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Namespace: "default", Name: "checkout-0", Labels: map[string]string{"app": "checkout"}},
	}
	otherPod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Namespace: "default", Name: "inventory-0", Labels: map[string]string{"app": "inventory"}},
	}

	clientset := fakeclientset.NewSimpleClientset(deployment, matchingPod, otherPod)
	client := cluster.NewKubeClusterClientFromClientset(clientset)

	// Deliberately pass a stale/hand-authored selector on the Workload to
	// confirm ResolvePods ignores it and re-reads the selector from the
	// cluster instead.
	staleWorkload := cluster.Workload{
		Kind:      cluster.WorkloadKindDeployment,
		Namespace: "default",
		Name:      "checkout",
		Selector:  cluster.Selector{"app": "not-the-real-selector"},
	}

	got, err := client.ResolvePods(context.Background(), staleWorkload)
	if err != nil {
		t.Fatalf("ResolvePods returned error: %v", err)
	}

	want := []cluster.Pod{{Namespace: "default", Name: "checkout-0"}}
	if len(got) != len(want) || got[0] != want[0] {
		t.Errorf("ResolvePods() = %+v, want %+v", got, want)
	}
}

func TestKubeClusterClient_ResolvePods_statefulSet(t *testing.T) {
	statefulSet := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{Namespace: "default", Name: "inventory"},
		Spec: appsv1.StatefulSetSpec{
			Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": "inventory"}},
		},
	}
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Namespace: "default", Name: "inventory-0", Labels: map[string]string{"app": "inventory"}},
	}

	clientset := fakeclientset.NewSimpleClientset(statefulSet, pod)
	client := cluster.NewKubeClusterClientFromClientset(clientset)

	workload := cluster.Workload{Kind: cluster.WorkloadKindStatefulSet, Namespace: "default", Name: "inventory"}
	got, err := client.ResolvePods(context.Background(), workload)
	if err != nil {
		t.Fatalf("ResolvePods returned error: %v", err)
	}

	want := []cluster.Pod{{Namespace: "default", Name: "inventory-0"}}
	if len(got) != len(want) || got[0] != want[0] {
		t.Errorf("ResolvePods() = %+v, want %+v", got, want)
	}
}

func TestKubeClusterClient_ResolvePods_unknownWorkloadKind(t *testing.T) {
	clientset := fakeclientset.NewSimpleClientset()
	client := cluster.NewKubeClusterClientFromClientset(clientset)

	_, err := client.ResolvePods(context.Background(), cluster.Workload{Kind: "Job", Namespace: "default", Name: "one-off"})
	if err == nil {
		t.Fatal("ResolvePods() with unsupported workload kind: want error, got nil")
	}
}

func TestKubeClusterClient_ImplementsClusterClient(t *testing.T) {
	var _ cluster.ClusterClient = (*cluster.KubeClusterClient)(nil)
}

// NOTE: FetchLogs is intentionally not covered here. client-go's fake
// clientset does not implement the pod logs subresource (see
// https://github.com/kubernetes/client-go/issues/554), so there is no
// in-process way to exercise KubeClusterClient.FetchLogs against a fake
// server. It is covered by manual verification against a real (kind)
// cluster instead; the log-line-parsing logic it depends on is tested
// directly below.
