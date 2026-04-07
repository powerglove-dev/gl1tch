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
