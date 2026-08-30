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

// AddApp names app.Workload as a new App and persists it. app.Workload must
// identify one of the workloads currently returned by ListCandidates; its
// selector is always taken from that discovered workload, never from
// app.Workload.Selector, so a caller can never hand-author one.
func (c *Configurator) AddApp(ctx context.Context, app domain.AppConfig) (domain.AppConfig, error) {
	if app.Name == "" {
		return domain.AppConfig{}, errors.New("config: app name must not be empty")
	}

	candidates, err := c.client.ListWorkloads(ctx)
	if err != nil {
		return domain.AppConfig{}, err
	}
	workload, ok := findCandidate(candidates, app.Workload)
	if !ok {
		return domain.AppConfig{}, fmt.Errorf("config: %s/%s is not a discovered candidate", app.Workload.Namespace, app.Workload.Name)
	}

	apps, err := c.store.Load()
	if err != nil {
		return domain.AppConfig{}, err
	}
	for _, existing := range apps {
		if existing.Name == app.Name {
			return domain.AppConfig{}, fmt.Errorf("config: app %q is already configured", app.Name)
		}
	}

	configured := domain.AppConfig{Name: app.Name, Workload: workload}
	apps = append(apps, configured)
	if err := c.store.Save(apps); err != nil {
		return domain.AppConfig{}, err
	}
	return configured, nil
}

// findCandidate returns the discovered candidate matching want's identity
// (kind, namespace, name), ignoring want.Selector.
func findCandidate(candidates []cluster.Workload, want cluster.Workload) (cluster.Workload, bool) {
	for _, candidate := range candidates {
		if candidate.Kind == want.Kind && candidate.Namespace == want.Namespace && candidate.Name == want.Name {
			return candidate, true
		}
	}
	return cluster.Workload{}, false
}

// ListApps returns the currently configured Apps.
func (c *Configurator) ListApps() ([]domain.AppConfig, error) {
	return c.store.Load()
}
