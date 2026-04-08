package esearch

// Index mappings for gl1tch's Elasticsearch indices.

// eventsMapping is the schema for the glitch-events index.
//
// workspace_id ties each event to the workspace whose collector pod
// produced it. The brain query engine filters on this so workspace A
// never sees workspace B's commits/messages/etc. Empty workspace_id
// means "global / unattributed" — used for legacy events from before
// the workspace split and for collectors running outside any pod.
const eventsMapping = `{
  "settings": {
    "number_of_shards": 1,
    "number_of_replicas": 0
  },
  "mappings": {
    "properties": {
      "type":          { "type": "keyword" },
      "source":        { "type": "keyword" },
      "workspace_id":  { "type": "keyword" },
      "repo":          { "type": "keyword" },
      "branch":        { "type": "keyword" },
      "author":        { "type": "keyword" },
      "message":       { "type": "text" },
      "body":          { "type": "text" },
      "files_changed": { "type": "keyword" },
      "sha":           { "type": "keyword" },
      "metadata":      { "type": "object", "enabled": false },
      "timestamp":     { "type": "date" }
    }
  }
}`

const summariesMapping = `{
  "settings": {
    "number_of_shards": 1,
    "number_of_replicas": 0
  },
  "mappings": {
    "properties": {
      "scope":          { "type": "keyword" },
      "date":           { "type": "date", "format": "yyyy-MM-dd" },
      "summary":        { "type": "text" },
      "key_decisions":  { "type": "text" },
      "repos":          { "type": "keyword" },
      "generated_by":   { "type": "keyword" },
      "timestamp":      { "type": "date" }
    }
  }
}`

const pipelinesMapping = `{
  "settings": {
    "number_of_shards": 1,
    "number_of_replicas": 0
  },
  "mappings": {
    "properties": {
      "name":         { "type": "keyword" },
      "status":       { "type": "keyword" },
      "workspace_id": { "type": "keyword" },
      "exit_code":    { "type": "integer" },
      "steps":        { "type": "object", "enabled": false },
      "stdout":       { "type": "text" },
      "stderr":       { "type": "text" },
      "duration_ms":  { "type": "long" },
      "model":        { "type": "keyword" },
      "provider":     { "type": "keyword" },
      "tokens_in":    { "type": "long" },
      "tokens_out":   { "type": "long" },
      "cost_usd":     { "type": "float" },
      "timestamp":    { "type": "date" }
    }
  }
}`

// vectorsMapping is the dense_vector + metadata mapping for the
// brainrag → ES migration. Replaces the SQLite brain_vectors table.
//
// Notes on the schema:
//   - We deliberately omit "dims" so ES 8.11+ infers the dimensionality
//     from the first indexed document. This lets nomic-embed-text (768)
//     coexist with text-embedding-3-small (1536), voyage-code-3 (1024),
//     and synthetic test vectors without requiring a re-create per
//     embedder. The trade-off: every doc in the index must have the
//     SAME dimensionality after the first one. We segment by embed_id
//     in the bool filter on every query so cross-embedder noise can't
//     leak in, and we recommend wiping the index when changing the
//     primary embedder model.
//   - similarity=cosine because all our embedders return unit-ish
//     vectors and cosine is what brainrag's old SQLite path used.
//   - index=true enables HNSW kNN search (the whole point of the
//     migration). Without it, knn queries return errors.
//   - scope is "cwd:/abs/path" for code chunks and "workspace:<id>"
//     for brain notes — single field with a prefix discriminator so
//     we can filter by either with one keyword field.
const vectorsMapping = `{
  "settings": {
    "number_of_shards": 1,
    "number_of_replicas": 0
  },
  "mappings": {
    "properties": {
      "scope":      { "type": "keyword" },
      "note_id":    { "type": "keyword" },
      "text":       { "type": "text" },
      "vector":     {
        "type": "dense_vector",
        "index": true,
        "similarity": "cosine"
      },
      "hash":       { "type": "keyword" },
      "embed_id":   { "type": "keyword" },
      "indexed_at": { "type": "date" }
    }
  }
}`

// brainDecisionsMapping is the schema for glitch-brain-decisions, the
// per-decision audit log the brain emits every time it picks a chain
// to answer a question. Each doc represents one "the brain decided to
// route this work to provider X" event so Kibana can chart things like:
//
//   - average confidence over time, filtered to escalated:false (i.e.
//     "is the local model getting more or less trustworthy at handling
//     work without falling back to a paid model?")
//   - escalation rate per workspace per day
//   - cost saved by staying local vs. cost paid when escalating
//   - which question types ELSER clusters together force escalation
//
// chosen_provider/chosen_model are the *first* (root) provider used by
// the chain — for multi-step chains we also store all_providers as a
// keyword array so a "did this run touch any paid model?" filter is a
// single terms query. escalated is a derived bool: true iff any step
// in the chain ran on a non-local provider. Local is currently just
// "ollama" (per the gl1tch hard requirement that internal intelligence
// runs on local Ollama).
//
// confidence starts as 0 until we wire the brain's self-rating into
// chain.go — the field exists now so dashboards built today don't need
// to be re-pointed when that lands.
const brainDecisionsMapping = `{
  "settings": {
    "number_of_shards": 1,
    "number_of_replicas": 0
  },
  "mappings": {
    "properties": {
      "issue_id":        { "type": "keyword" },
      "workspace_id":    { "type": "keyword" },
      "question":        { "type": "text" },
      "chosen_provider": { "type": "keyword" },
      "chosen_model":    { "type": "keyword" },
      "all_providers":   { "type": "keyword" },
      "all_models":      { "type": "keyword" },
      "escalated":       { "type": "boolean" },
      "confidence":      { "type": "float" },
      "resolved":        { "type": "boolean" },
      "status":          { "type": "keyword" },
      "step_count":      { "type": "integer" },
      "duration_ms":     { "type": "long" },
      "cost_usd":        { "type": "float" },
      "timestamp":       { "type": "date" }
    }
  }
}`

// tracesMapping is the schema for glitch-traces, the destination
// index for the OTel SpanExporter implemented in
// internal/telemetry/elasticsearch_exporter.go. Every span produced
// by the gl1tch process (pipeline runs, brain cycles, executor
// dispatches, collector pod startup, refine loops, etc.) lands here
// as one document.
//
// We deliberately don't try to fit the upstream traces-apm-* schema:
// gl1tch doesn't run an APM Server, and a custom flat document is
// faster to query in Kibana Discover for the "what's happening
// right now in this process" use case that motivates the index.
// The shape mirrors the OTel data model just enough to be familiar
// (trace_id, span_id, parent_span_id, status, attributes) without
// dragging in the resource semantic conventions schema.
//
// attributes is "object enabled=false" so we keep the original
// key=value structure on disk for inspection but don't blow up the
// mapping with every span attribute every collector ever emitted.
// trace_id and span_id are keyword so trace-id-jump queries work.
// Resource attributes (service name, host, version, pid) are
// flattened to top-level keyword fields because they're high-value
// for slicing across many runs.
const tracesMapping = `{
  "settings": {
    "number_of_shards": 1,
    "number_of_replicas": 0
  },
  "mappings": {
    "properties": {
      "trace_id":        { "type": "keyword" },
      "span_id":         { "type": "keyword" },
      "parent_span_id":  { "type": "keyword" },
      "name":            { "type": "keyword" },
      "scope_name":      { "type": "keyword" },
      "kind":            { "type": "keyword" },
      "service_name":    { "type": "keyword" },
      "service_version": { "type": "keyword" },
      "host_name":       { "type": "keyword" },
      "process_pid":     { "type": "long" },
      "workspace_id":    { "type": "keyword" },
      "collector":       { "type": "keyword" },
      "status_code":     { "type": "keyword" },
      "status_message":  { "type": "text" },
      "start_time":      { "type": "date" },
      "end_time":        { "type": "date" },
      "duration_ms":     { "type": "long" },
      "attributes":      { "type": "object", "enabled": false },
      "resource":        { "type": "object", "enabled": false },
      "events":          { "type": "object", "enabled": false }
    }
  }
}`

// logsMapping is the schema for glitch-logs, the destination index
// the slog teeHandler ships every captured log record into. The
// in-process LogBuffer in pkg/glitchd/logbuffer.go is a per-process
// ring (great for the live brain popover, useless across restarts);
// shipping the same records to ES gives us queryable history that
// survives wails dev rebuilds, lets multiple gl1tch processes be
// disambiguated by process_pid, and means "show me the last ten
// times a workspace pod started for robots" is a Kibana query, not
// a screenshot round-trip.
//
// process_pid is critical: when wails dev orphans a stale binary
// or the user has a `glitch serve` running alongside the desktop,
// the same source field can come from two different processes with
// completely different in-memory state. Filtering by process_pid
// in Kibana lets us tell them apart.
//
// attrs is the slog key=value pairs flattened to a single string
// (matching the LogBuffer's existing UI shape). attrs_json keeps
// the structured form so dashboards can drill into specific keys
// without re-parsing.
const logsMapping = `{
  "settings": {
    "number_of_shards": 1,
    "number_of_replicas": 0
  },
  "mappings": {
    "properties": {
      "timestamp":   { "type": "date" },
      "level":       { "type": "keyword" },
      "source":      { "type": "keyword" },
      "message":     { "type": "text" },
      "attrs":       { "type": "text" },
      "attrs_json":  { "type": "object", "enabled": false },
      "process_pid": { "type": "long" },
      "host_name":   { "type": "keyword" },
      "service":     { "type": "keyword" }
    }
  }
}`

// analysesMapping is the schema for the glitch-analyses index. Each
// doc is one deep-analysis run produced by the analyzer service in
// pkg/glitchd/deep_analysis.go. Source-agnostic — the same shape
// works for github PRs, git commits, claude sessions, or any future
// collector type that emits events the analyzer can chew on.
const analysesMapping = `{
  "settings": {
    "number_of_shards": 1,
    "number_of_replicas": 0
  },
  "mappings": {
    "properties": {
      "event_key":    { "type": "keyword" },
      "source":       { "type": "keyword" },
      "type":         { "type": "keyword" },
      "repo":         { "type": "keyword" },
      "title":        { "type": "text" },
      "model":        { "type": "keyword" },
      "markdown":     { "type": "text" },
      "exit_code":    { "type": "integer" },
      "duration_ms":  { "type": "long" },
      "workspace_id": { "type": "keyword" },
      "created_at":   { "type": "date" }
    }
  }
}`

const insightsMapping = `{
  "settings": {
    "number_of_shards": 1,
    "number_of_replicas": 0
  },
  "mappings": {
    "properties": {
      "type":             { "type": "keyword" },
      "pattern":          { "type": "text" },
      "confidence":       { "type": "float" },
      "evidence_count":   { "type": "integer" },
      "evidence":         { "type": "text" },
      "recommendation":   { "type": "text" },
      "repos":            { "type": "keyword" },
      "first_seen":       { "type": "date" },
      "last_seen":        { "type": "date" },
      "timestamp":        { "type": "date" }
    }
  }
}`
