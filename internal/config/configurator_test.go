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
	configurator := newConfigurator(t, cluster.NewFakeClusterClient())

	got, err := configurator.AddApp(context.Background(), "checkout", checkout)
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

func TestConfigurator_AddApp_multipleAppsPersist(t *testing.T) {
	checkout := cluster.Workload{Kind: cluster.WorkloadKindDeployment, Namespace: "default", Name: "checkout"}
	worker := cluster.Workload{Kind: cluster.WorkloadKindStatefulSet, Namespace: "default", Name: "worker"}
	configurator := newConfigurator(t, cluster.NewFakeClusterClient())

	if _, err := configurator.AddApp(context.Background(), "checkout", checkout); err != nil {
		t.Fatalf("AddApp(checkout) returned error: %v", err)
	}
	if _, err := configurator.AddApp(context.Background(), "worker", worker); err != nil {
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

	_, err := configurator.AddApp(context.Background(), "", cluster.Workload{Name: "checkout"})
	if err == nil {
		t.Fatal("AddApp() with empty name: want error, got nil")
	}
}

func TestConfigurator_AddApp_duplicateName(t *testing.T) {
	configurator := newConfigurator(t, cluster.NewFakeClusterClient())
	first := cluster.Workload{Kind: cluster.WorkloadKindDeployment, Namespace: "default", Name: "checkout"}
	second := cluster.Workload{Kind: cluster.WorkloadKindDeployment, Namespace: "default", Name: "checkout-v2"}

	if _, err := configurator.AddApp(context.Background(), "checkout", first); err != nil {
		t.Fatalf("AddApp(first) returned error: %v", err)
	}

	_, err := configurator.AddApp(context.Background(), "checkout", second)
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
