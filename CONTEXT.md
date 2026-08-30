# Wheelman

A TUI for interacting with Kubernetes clusters via natural language, starting with querying logs across pods.

## Language

**App**:
A user-named reference to a single Deployment or StatefulSet, chosen by the user from a discovered list at setup time. Wheelman reads the underlying workload's own selector to resolve pods; the user never authors a label selector directly.
_Avoid_: Namespace, Workload, Label selector (these are the mechanisms App is built on, not the term itself)

**Query**:
A read-only, natural-language request that compiles to a Filter and runs against a historical window of already-emitted logs. Parsing is local, rule-based grammar matching, not LLM-backed (see ADR-0002).
_Avoid_: Command (reserved for a possible future cluster-mutating concept, out of scope for v1), Search

**Filter**:
The structured, deterministic form a Query compiles down to before execution: the scoped Apps, time range, and keyword/regex/severity conditions actually applied to log lines. Exists so a Query's intent is inspectable rather than opaque.
_Avoid_: Structured filter, Search criteria

**Result**:
The flat, chronologically-ordered stream of log lines produced by running a Filter, each line tagged with its source App and pod so provenance stays visible when lines from multiple Apps are interleaved. Grouping by App/pod is a display concern, not a property of Result.
_Avoid_: Output, Response

## Query-to-Filter compilation rules

- Unscoped by default: no named Apps → all configured Apps; no time phrase → last 1 hour.
- An App is named with an `app:<Name>` phrase (e.g. `app:checkout`), matched case-insensitively against configured App names; a query may repeat this phrase to scope to multiple Apps. This is a deliberate, explicit marker rather than bare-word matching, so an App reference is always unambiguous and distinguishable from ordinary keyword text.
- Severity is matched via keyword/regex ("ERROR", "WARN", ...) uniformly — most container logs are unstructured text, not JSON with a level field.
- Naming an App that isn't configured is an error listing the configured Apps, never a fuzzy-matched guess.
- Text not recognized as a time/severity/App phrase falls back to a literal keyword/regex search term — no query is ever rejected as unparseable.
- All recognized conditions combine with AND only; exclusion/negation phrasing is out of scope for v1.
