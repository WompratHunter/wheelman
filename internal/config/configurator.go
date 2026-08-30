package config

import (
	"context"
	"errors"
	"fmt"

	"github.com/WompratHunter/wheelman/internal/cluster"
	"github.com/WompratHunter/wheelman/internal/domain"
)

// Configurator drives App discovery and configuration: listing candidate
// workloads through a ClusterClient, and persisting the Apps a user
// configures from that list through a FileStore.
type Configurator struct {
	client cluster.ClusterClient
	store  *FileStore
}

// NewConfigurator returns a Configurator that discovers candidate workloads
// through client and persists configured Apps through store.
func NewConfigurator(client cluster.ClusterClient, store *FileStore) *Configurator {
	return &Configurator{client: client, store: store}
}

// ListCandidates returns the Deployments and StatefulSets discoverable in
// the current kubeconfig context, for the user to pick an App from.
func (c *Configurator) ListCandidates(ctx context.Context) ([]cluster.Workload, error) {
	return c.client.ListWorkloads(ctx)
}

// AddApp names workload as a new App and persists it. workload should come
// from a prior call to ListCandidates, so its selector is the one read from
// the workload's own spec rather than hand-authored.
func (c *Configurator) AddApp(ctx context.Context, name string, workload cluster.Workload) (domain.AppConfig, error) {
	if name == "" {
		return domain.AppConfig{}, errors.New("config: app name must not be empty")
	}

	apps, err := c.store.Load()
	if err != nil {
		return domain.AppConfig{}, err
	}
	for _, existing := range apps {
		if existing.Name == name {
			return domain.AppConfig{}, fmt.Errorf("config: app %q is already configured", name)
		}
	}

	app := domain.AppConfig{Name: name, Workload: workload}
	apps = append(apps, app)
	if err := c.store.Save(apps); err != nil {
		return domain.AppConfig{}, err
	}
	return app, nil
}

// ListApps returns the currently configured Apps.
func (c *Configurator) ListApps() ([]domain.AppConfig, error) {
	return c.store.Load()
}
