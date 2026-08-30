package config_test

import (
	"path/filepath"
	"reflect"
	"testing"

	"github.com/WompratHunter/wheelman/internal/cluster"
	"github.com/WompratHunter/wheelman/internal/config"
	"github.com/WompratHunter/wheelman/internal/domain"
)

func TestFileStore_Load_missingFile(t *testing.T) {
	store := config.NewFileStore(filepath.Join(t.TempDir(), "apps.json"))

	got, err := store.Load()
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("Load() on missing file = %+v, want empty", got)
	}
}

func TestFileStore_SaveThenLoad_roundTrips(t *testing.T) {
	store := config.NewFileStore(filepath.Join(t.TempDir(), "apps.json"))

	apps := []domain.AppConfig{
		{
			Name: "checkout",
			Workload: cluster.Workload{
				Kind:      cluster.WorkloadKindDeployment,
				Namespace: "default",
				Name:      "checkout",
				Selector:  cluster.Selector{"app": "checkout"},
			},
		},
	}

	if err := store.Save(apps); err != nil {
		t.Fatalf("Save returned error: %v", err)
	}

	got, err := store.Load()
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if !reflect.DeepEqual(got, apps) {
		t.Errorf("Load() = %+v, want %+v", got, apps)
	}
}
