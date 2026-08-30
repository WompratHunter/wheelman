# Single active kubeconfig context for v1

Wheelman's App discovery and Query execution operate against exactly one active kubeconfig context at a time, matching whatever `kubectl` is currently pointed at. We deliberately deferred multi-cluster fan-out (parallel API calls, per-cluster auth, merged Results) because it's a materially larger feature than the first working Query, and real Kubernetes users commonly need to reason across multiple clusters — so a future reader should not assume this is an oversight.
