package esearch

// Index mappings for gl1tch's Elasticsearch indices.

const eventsMapping = `{
  "settings": {
    "number_of_shards": 1,
    "number_of_replicas": 0
  },
  "mappings": {
    "properties": {
      "type":          { "type": "keyword" },
      "source":        { "type": "keyword" },
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
