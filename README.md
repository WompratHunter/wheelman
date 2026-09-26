# Wheelman

A terminal UI for interrogating Kubernetes clusters in plain English — starting with searching logs across pods without hand-writing label selectors or `kubectl` incantations.

> **Status:** pre-1.0. The domain/config/query engine exists and is unit-tested; there is no `main` package or TUI yet.

```
app:checkout app:payments last 30 minutes error
```

You configure a small set of **Apps** — friendly names pointing at Deployments or StatefulSets you actually care about — then type a query like the one above. Wheelman resolves each App to its live pods, pulls their logs, and hands back one chronological, source-tagged stream. No label selectors to author, no per-pod `kubectl logs` loop, no LLM round-trip.

## Domain glossary

See [`CONTEXT.md`](CONTEXT.md) for the canonical definitions.

| Term | Meaning | Avoid |
|---|---|---|
| **App** | A user-named reference to one Deployment or StatefulSet, chosen from a discovered list at setup time. Wheelman reads the workload's own selector — users never author a label selector by hand. | `Namespace`, `Workload`, `Label selector` |
| **Query** | A read-only, natural-language request that compiles to a Filter and runs against a historical window of already-emitted logs. Parsed locally by a fixed grammar, never by an LLM. | `Command` (reserved for a future mutating concept), `Search` |
| **Filter** | The structured, deterministic form a Query compiles down to before execution — scoped Apps, time range, and keyword/regex/severity conditions — so intent is inspectable, not opaque. | `Structured filter`, `Search criteria` |
| **Result** | The flat, chronologically-ordered stream of log lines a Filter produces, each line tagged with its source App and pod. Grouping by App/pod is a display concern, not a Result property. | `Output`, `Response` |

## Architecture

Domain logic never talks to Kubernetes directly — everything crosses through `ClusterClient`, the sole boundary between Wheelman and a real cluster. That seam is what makes the engine and configurator testable without a cluster at all.

```
internal/domain            internal/query          internal/cluster
 AppConfig                  Engine        ───────▶   ClusterClient (seam)
 Filter                                                 ├── production: client-go-backed
 Result           internal/config                       │   (planned — issue #8)
                    Configurator ────────────────────▶  └── tests: FakeClusterClient
                    FileStore
```

`Engine` and `Configurator` both depend on the `ClusterClient` interface, never on a concrete implementation. Today the only implementation is `FakeClusterClient` (`internal/cluster/fake.go`), an in-memory double configured per-test with canned workloads, pods, and logs. A real client-go-backed client is scoped but not yet built.

## Query pipeline

Everything happens inside `internal/query/engine.go`'s `Engine.Run(queryText string)`, the single entry point for executing a Query:

1. Extract `app:<Name>` phrases
2. Resolve them against configured Apps
3. Extract a relative time phrase
4. Extract severity terms
5. Build the `Filter`
6. Resolve pods per App and fetch logs via `ClusterClient`
7. Filter by timestamp, severity, and keyword, then sort into a chronological `Result`

Steps 1–4 are pure text-to-`Filter` compilation and are unit-tested independently of any cluster. Steps 5–7 execute the `Filter` through `ClusterClient`.

## Query grammar

The full set of phrases the parser currently recognizes, applied regardless of where they appear in the query text:

- **Unscoped by default** — no named App means all configured Apps; no time phrase means the last `1 hour`.
- **App scoping** — `app:<Name>`, matched case-insensitively; repeatable to scope multiple Apps. This is a deliberate, explicit marker rather than bare-word matching, so an App reference is always unambiguous against ordinary keyword text.
- **Time phrase** — `last 30 minutes`, `in the last hour`; overrides the 1-hour default. A bare unit with no count (`the last hour`) implies a count of 1. Only the first recognized phrase is consumed.
- **Severity** — `error`/`errors`, `warn`/`warns`/`warning`/`warnings`, `info`/`infos`, `debug`/`debugs`, matched via keyword/regex against raw log text uniformly, whether the line is plain text or structured JSON. Multiple severity terms OR against each other; that combined condition then ANDs with everything else.
- **Unconfigured App is an error** — never fuzzy-matched; the error lists every configured App name.
- **Fallback to keyword search** — any text not recognized as an App, time, or severity phrase becomes a literal keyword/regex term. No query is ever rejected as unparseable.
- **Regex with a safety net** — the remaining text is compiled as a case-insensitive regex; if it isn't valid regex, it's matched as a literal case-insensitive substring instead.
- **AND-only** — all recognized conditions combine with AND (severities OR internally, then AND with the rest). No negation/exclusion syntax in v1.

### Worked examples

| Query | App scope | Window | Severity | Keyword |
|---|---|---|---|---|
| `timeout connecting to redis` | all configured | last 1h (default) | — | `timeout connecting to redis` |
| `app:checkout the last hour error` | checkout | last 1h (bare unit) | `ERROR` | — |
| `app:auth app:gateway last 15 minutes warn 5\d\d` | auth, gateway | last 15m | `WARN` | `5\d\d` (regex) |

## Package reference

| Package | Purpose |
|---|---|
| [`internal/domain`](internal/domain) | Core vocabulary shared by every other package: `AppConfig`, `Filter`, `ResultLine`, `Result`. |
| [`internal/cluster`](internal/cluster) | The `ClusterClient` seam (`ListWorkloads`, `ResolvePods`, `FetchLogs`) and `FakeClusterClient`, its in-memory test double. |
| [`internal/config`](internal/config) | `Configurator` (App discovery + configuration) and `FileStore` (JSON persistence). `AddApp` always takes the selector from the freshly-discovered candidate workload, never from caller input. |
| [`internal/query`](internal/query) | `Engine`, which compiles a Query to a `Filter` and executes it end to end. `Engine.Now` defaults to `time.Now` and is overridable in tests. |

## Testing

Because `Engine` and `Configurator` only ever see the `ClusterClient` interface, tests configure a `FakeClusterClient` with canned workloads, pod resolutions, and per-pod log lines, then assert on the resulting `Result` or error — no real kubeconfig, no network call, no test cluster.

```sh
go test ./...
```

## Design decisions

See [`docs/adr/`](docs/adr) for the full records.

- **[ADR-0001](docs/adr/0001-single-cluster-context-for-v1.md) — Single active kubeconfig context for v1.** App discovery and Query execution operate against exactly one active kubeconfig context — whatever `kubectl` is currently pointed at. Multi-cluster fan-out is deliberately deferred as a materially larger feature, not an oversight.
- **[ADR-0002](docs/adr/0002-rule-based-nl-parsing.md) — Rule-based natural language parsing, not LLM-backed.** Queries are parsed with a local, fixed grammar rather than an LLM, so Wheelman works fully offline against any cluster — no external API dependency, no API key, no per-query network round-trip or cost.

## Roadmap

Tracked as [GitHub issues](https://github.com/WompratHunter/wheelman/issues):

- [x] #2 — Prefactor: ClusterClient seam + core types
- [x] #3 — App discovery & configuration
- [x] #4 — Baseline query engine: keyword search over default scope/window
- [x] #5 — Query grammar: App-name scoping
- [x] #6 — Query grammar: time phrase parsing
- [x] #7 — Query grammar: severity keyword matching
- [ ] #8 — Real ClusterClient (client-go-backed)
- [ ] #1 — NL log query engine: App config + Query/Filter/Result pipeline (tracking issue)
