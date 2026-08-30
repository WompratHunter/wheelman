package config_test

import (
	"context"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/WompratHunter/wheelman/internal/cluster"
	"github.com/WompratHunter/wheelman/internal/config"
	"github.com/WompratHunter/wheelman/internal/domain"
)

func newConfigurator(t *testing.T, client cluster.ClusterClient) *config.Configurator {
	t.Helper()
	store := config.NewFileStore(filepath.Join(t.TempDir(), "apps.json"))
	return config.NewConfigurator(client, store)
}

func TestConfigurator_ListCandidates(t *testing.T) {
	checkout := cluster.Workload{
		Kind:      cluster.WorkloadKindDeployment,
		Namespace: "default",
		Name:      "checkout",
		Selector:  cluster.Selector{"app": "checkout"},
	}
	fake := cluster.NewFakeClusterClient()
	fake.AddWorkload(checkout)

	configurator := newConfigurator(t, fake)

	got, err := configurator.ListCandidates(context.Background())
	if err != nil {
		t.Fatalf("ListCandidates returned error: %v", err)
	}
	want := []cluster.Workload{checkout}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("ListCandidates() = %+v, want %+v", got, want)
	}
}

func TestConfigurator_AddApp(t *testing.T) {
	checkout := cluster.Workload{
		Kind:      cluster.WorkloadKindDeployment,
		Namespace: "default",
		Name:      "checkout",
		Selector:  cluster.Selector{"app": "checkout"},
	}
	fake := cluster.NewFakeClusterClient()
	fake.AddWorkload(checkout)
	configurator := newConfigurator(t, fake)

	got, err := configurator.AddApp(context.Background(), domain.AppConfig{Name: "checkout", Workload: checkout})
	if err != nil {
		t.Fatalf("AddApp returned error: %v", err)
	}
	want := domain.AppConfig{Name: "checkout", Workload: checkout}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("AddApp() = %+v, want %+v", got, want)
	}

	apps, err := configurator.ListApps()
	if err != nil {
		t.Fatalf("ListApps returned error: %v", err)
	}
	if !reflect.DeepEqual(apps, []domain.AppConfig{want}) {
		t.Errorf("ListApps() after AddApp = %+v, want %+v", apps, []domain.AppConfig{want})
	}
}

func TestConfigurator_AddApp_usesDiscoveredSelector(t *testing.T) {
	// The selector on the discovered candidate is the source of truth, even
	// if a caller passes a workload with a different (hand-authored) one.
	discovered := cluster.Workload{
		Kind:      cluster.WorkloadKindDeployment,
		Namespace: "default",
		Name:      "checkout",
		Selector:  cluster.Selector{"app": "checkout"},
	}
	fake := cluster.NewFakeClusterClient()
	fake.AddWorkload(discovered)
	configurator := newConfigurator(t, fake)

	handAuthored := discovered
	handAuthored.Selector = cluster.Selector{"app": "not-what-was-discovered"}

	got, err := configurator.AddApp(context.Background(), domain.AppConfig{Name: "checkout", Workload: handAuthored})
	if err != nil {
		t.Fatalf("AddApp returned error: %v", err)
	}
	if !reflect.DeepEqual(got.Workload.Selector, discovered.Selector) {
		t.Errorf("AddApp() selector = %+v, want the discovered selector %+v", got.Workload.Selector, discovered.Selector)
	}
}

func TestConfigurator_AddApp_notADiscoveredCandidate(t *testing.T) {
	configurator := newConfigurator(t, cluster.NewFakeClusterClient())

	notDiscovered := cluster.Workload{Kind: cluster.WorkloadKindDeployment, Namespace: "default", Name: "checkout"}
	_, err := configurator.AddApp(context.Background(), domain.AppConfig{Name: "checkout", Workload: notDiscovered})
	if err == nil {
		t.Fatal("AddApp() with a workload that isn't a discovered candidate: want error, got nil")
	}
}

func TestConfigurator_AddApp_multipleAppsPersist(t *testing.T) {
	checkout := cluster.Workload{Kind: cluster.WorkloadKindDeployment, Namespace: "default", Name: "checkout"}
	worker := cluster.Workload{Kind: cluster.WorkloadKindStatefulSet, Namespace: "default", Name: "worker"}
	fake := cluster.NewFakeClusterClient()
	fake.AddWorkload(checkout)
	fake.AddWorkload(worker)
	configurator := newConfigurator(t, fake)

	if _, err := configurator.AddApp(context.Background(), domain.AppConfig{Name: "checkout", Workload: checkout}); err != nil {
		t.Fatalf("AddApp(checkout) returned error: %v", err)
	}
	if _, err := configurator.AddApp(context.Background(), domain.AppConfig{Name: "worker", Workload: worker}); err != nil {
		t.Fatalf("AddApp(worker) returned error: %v", err)
	}

	got, err := configurator.ListApps()
	if err != nil {
		t.Fatalf("ListApps returned error: %v", err)
	}
	want := []domain.AppConfig{
		{Name: "checkout", Workload: checkout},
		{Name: "worker", Workload: worker},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("ListApps() = %+v, want %+v", got, want)
	}
}

func TestConfigurator_AddApp_emptyName(t *testing.T) {
	configurator := newConfigurator(t, cluster.NewFakeClusterClient())

	_, err := configurator.AddApp(context.Background(), domain.AppConfig{Name: "", Workload: cluster.Workload{Name: "checkout"}})
	if err == nil {
		t.Fatal("AddApp() with empty name: want error, got nil")
	}
}

func TestConfigurator_AddApp_duplicateName(t *testing.T) {
	first := cluster.Workload{Kind: cluster.WorkloadKindDeployment, Namespace: "default", Name: "checkout"}
	second := cluster.Workload{Kind: cluster.WorkloadKindDeployment, Namespace: "default", Name: "checkout-v2"}
	fake := cluster.NewFakeClusterClient()
	fake.AddWorkload(first)
	fake.AddWorkload(second)
	configurator := newConfigurator(t, fake)

	if _, err := configurator.AddApp(context.Background(), domain.AppConfig{Name: "checkout", Workload: first}); err != nil {
		t.Fatalf("AddApp(first) returned error: %v", err)
	}

	_, err := configurator.AddApp(context.Background(), domain.AppConfig{Name: "checkout", Workload: second})
	if err == nil {
		t.Fatal("AddApp() with duplicate name: want error, got nil")
	}

	apps, err := configurator.ListApps()
	if err != nil {
		t.Fatalf("ListApps returned error: %v", err)
	}
	if len(apps) != 1 {
		t.Errorf("ListApps() after rejected duplicate = %+v, want 1 entry unchanged", apps)
	}
}
