# gl1tch Internals: Intent Routing and Code Search

Technical reference for `internal/router` and the `builtin.index_code` /
`builtin.search_code` pipeline actions. Describes the algorithms, data
structures, thresholds, and design trade-offs in plain engineering terms.

---

## 1. Intent Routing

### 1.1 Overview

`HybridRouter` routes a free-text user prompt to zero or one `PipelineRef`.
It uses a two-stage strategy:

1. **Embedding negative filter** — fast local cosine similarity check.
   Primary job: skip the LLM entirely when no pipeline is topically relevant.
2. **LLM classifier** — a single structured Ollama call with an intent gate.
   Primary job: confirm the user actually wants to *invoke* a pipeline, not
   just ask a question or give a task to the AI.

The two stages are deliberately asymmetric. The embedding stage is a *negative*
filter — it only short-circuits on the "nothing relevant" case, never on a
positive match alone. This prevents topic-overlap misfires where a question
about, say, code review routes to a `code-review` pipeline. The fast path
(skip LLM, return the top embedding hit immediately) fires only when embedding
confidence is very high *and* the input is syntactically imperative.

---

### 1.2 Thresholds

All defined as package-level constants in `internal/router/router.go`:

| Constant | Value | Role |
|---|---|---|
| `DefaultCandidateGateThreshold` | `0.40` | Minimum cosine similarity to admit a pipeline as an LLM candidate. Below this → excluded from candidate list. When no pipeline clears this gate, the LLM call is skipped entirely. |
| `DefaultConfidentThreshold` | `0.85` | Minimum cosine similarity for the embedding fast path. Combined with `isImperativeInput` → skip LLM and return immediately. |
| `DefaultAmbiguousThreshold` | `0.65` | Minimum LLM-reported confidence for a match to be accepted. Below this → treated as no-match. |
| `NearMissThreshold` | `0.60` | Minimum score to report a `NearMiss` in `RouteResult`. Scores below this are noise and not surfaced. |
| `DefaultEmbeddingModel` | `"nomic-embed-text"` | Ollama model used for routing embeddings. |

Thresholds are configurable via `router.Config` — zero values are replaced by
the defaults in `New()`.

---

### 1.3 Stage 1 — EmbeddingRouter

**File**: `internal/router/embed.go`

#### Pipeline representative vector

Each pipeline is represented in embedding space by a single centroid vector.
The source text used to generate that vector follows this priority:

1. **`trigger_phrases`** (from pipeline YAML) — each phrase is embedded
   independently; the centroid of the phrase vectors is the representative.
   Phrases are short imperative invocation patterns ("run git-pulse", etc.),
   so the embedding space is command-intent–driven, not behavior-prose–driven.
2. **Auto-generated phrases** — if no `trigger_phrases` are defined and a
   `PhraseGenerator` is configured, the `LLMPhraseGenerator` is called once
   to produce 3–5 imperative phrases. These are cached alongside the pipeline
   embedding so the LLM is not called again until the pipeline changes.
3. **Description text fallback** — if neither of the above applies, the
   pipeline's `description` string is embedded directly.

#### Cache invalidation

Staleness is detected via `pipelineHash`, which is `SHA256(name + description +
strings.Join(trigger_phrases, "|"))`. If the hash in the cache entry matches
the current pipeline ref, the stored vector is reused without re-embedding.

Two layers of cache:

- **In-memory** — `map[string]pipelineEmbedding` keyed by pipeline name,
  protected by a `sync.Mutex`. Lives for the process lifetime.
- **On-disk** — `$CacheDir/routing-index.json`, written atomically via
  `path + ".tmp"` → `os.Rename`. Loaded at construction time by
  `loadDiskCache`. Persists across process restarts so embeddings are not
  recomputed on every startup.

`pipelineEmbedding` on disk:
```json
{
  "pipeline-name": {
    "desc_hash": "sha256hex",
    "vector": [0.12, -0.34, ...],
    "generated_phrases": ["run pipeline-name", "execute pipeline-name"]
  }
}
```
`generated_phrases` is omitted when `trigger_phrases` were explicit in YAML.

#### Cosine similarity

```go
func cosineSimilarity(a, b []float32) float64 {
    var dot, normA, normB float64
    for i := range a {
        dot  += float64(a[i]) * float64(b[i])
        normA += float64(a[i]) * float64(a[i])
        normB += float64(b[i]) * float64(b[i])
    }
    return dot / (math.Sqrt(normA) * math.Sqrt(normB))
}
```

Both the routing and brainrag packages implement cosine similarity directly on
`[]float32`; computation is widened to `float64` to reduce accumulated
floating-point error. No SIMD or BLAS dependency — the vector dimensions from
`nomic-embed-text` are 768 and from `mxbai-embed-large` are 1024, which are
fast enough in scalar Go on a laptop.

#### Centroid

```go
func centroid(vecs [][]float32) []float32 {
    result := make([]float32, len(vecs[0]))
    for _, v := range vecs {
        for i, x := range v { result[i] += x }
    }
    for i := range result { result[i] /= float32(len(vecs)) }
    return result
}
```

The centroid of `trigger_phrases` embeddings is not normalized after averaging.
This is intentional: length variation in the centroid is informative (a pipeline
with highly coherent phrases produces a longer centroid; a pipeline with
semantically scattered phrases produces a shorter one). The cosine similarity
formula normalizes at query time regardless.

#### FindCandidates

`EmbeddingRouter.FindCandidates` returns all pipelines with cosine similarity
≥ `CandidateGateThreshold` (0.40), sorted descending by score. The result is
the input to Stage 2. When the result is empty, the caller (`HybridRouter.Route`)
returns immediately — the LLM is never called.

---

### 1.4 Fast Path

Within `HybridRouter.Route`, after `FindCandidates` returns at least one
candidate:

```
topScore := candidates[0].Score

if topScore >= ConfidentThreshold (0.85) && isImperativeInput(prompt):
    return RouteResult{Pipeline: best, Method: "embedding", ...}
```

`isImperativeInput` checks whether the lowercased prompt starts with one of:
`run`, `execute`, `launch`, `rerun`, `re-run`, `start`, `trigger`, `kick off`,
`kick-off`. This guards against high-similarity questions ("what does the
code-review pipeline do?") fast-pathing to a pipeline match.

When the fast path fires, `extractInput` and `extractCronPhrase` are called
on the prompt directly (no LLM) to populate `RouteResult.Input` and
`RouteResult.CronExpr`:

- `extractInput` — tries URL regex first (`https?://\S+`), then the pattern
  `\b(?:on|for)\s+(.+?)(?:\s+every\b|\s*$)`.
- `extractCronPhrase` — maps natural language schedules to 5-field cron
  expressions via a set of compiled regexes: "every N hours", "every N
  minutes", "daily", "every hour", "every morning", "every weekday",
  "every \<day-of-week\>".

---

### 1.5 Stage 2 — LLMClassifier

**File**: `internal/router/classify.go`

#### Pipeline

The classifier executes a single-step Ollama pipeline at runtime:

```go
classifyPipeline := &pipeline.Pipeline{
    Steps: []pipeline.Step{{
        ID:       "classify",
        Executor: "ollama",
        Model:    c.cfg.Model,
        Prompt:   buildPrompt(userPrompt, candidatePipelines),
    }},
}
raw, err := pipeline.Run(ctx, classifyPipeline, c.mgr, "",
    pipeline.WithSilentStatus(), pipeline.WithNoClarification())
```

`WithSilentStatus` suppresses step-progress output. `WithNoClarification`
disables the interactive clarification flow so the classifier never blocks
waiting for user input.

#### Prompt structure

`buildPrompt` constructs a prompt with an explicit two-step intent gate:

```
Step 1 — Is the user explicitly asking to run a pipeline (by name or with
run/execute/launch/rerun)?
If NO, output {"pipeline":"NONE","confidence":0.05,"input":"","cron":""} immediately.

Step 2 — Only if YES: select the matching pipeline by name.
```

The prompt includes:
- The RULE: only select when user explicitly invokes by name or uses
  run/execute/launch/rerun language. Generic task requests return NONE even
  when a relevant pipeline exists.
- 5 NONE examples covering task requests, questions, and observations.
- 4 affirmative examples with expected JSON outputs including `confidence`,
  `input`, and `cron` fields.
- The available pipeline list: `- name: description` for each candidate.
- `User request: <prompt>` followed by `Respond with ONLY a single JSON object:`.

#### Response schema

```go
type classifyResponse struct {
    Pipeline   string  `json:"pipeline"`   // pipeline name or "NONE"
    Confidence float64 `json:"confidence"` // [0.0, 1.0]
    Input      string  `json:"input"`      // {{param.input}} value or ""
    Cron       string  `json:"cron"`       // 5-field cron or ""
}
```

#### Parsing

`parseClassifyResponse` runs `extractFirstJSONObject` on the raw LLM output
before unmarshalling. `extractFirstJSONObject` walks the string byte-by-byte
tracking brace depth to find the first balanced `{...}`, discarding any
trailing metadata lines emitted by the Ollama plugin (token counts, timing).

Confidence is clamped to `[0, 1]` after unmarshal.

#### Hallucination guard

After parsing, the classifier checks the returned pipeline name against the
candidate list using a case-insensitive match:

```go
for i := range pipelines {
    if strings.EqualFold(pipelines[i].Name, resp.Pipeline) {
        matched = &pipelines[i]; break
    }
}
if matched == nil { return &RouteResult{Method: "llm"} }
```

A pipeline name that does not appear in the candidate list is treated as a
hallucination and produces a no-match result.

---

### 1.6 RouteResult

```go
type RouteResult struct {
    Pipeline      *pipeline.PipelineRef  // nil = no match
    Confidence    float64
    Input         string                 // {{param.input}} for the pipeline
    CronExpr      string                 // validated 5-field cron, or ""
    Method        string                 // "embedding" | "llm" | "none"
    NearMiss      *pipeline.PipelineRef  // best candidate that didn't clear threshold
    NearMissScore float64
}
```

`NearMiss` is populated when the top candidate's score falls in
`[NearMissThreshold (0.60), AmbiguousThreshold (0.65))` — close enough to
surface to the caller for "did you mean X?" handling, but not confident enough
to dispatch.

---

### 1.7 Phrase Generation

**File**: `internal/router/phrases.go`

`LLMPhraseGenerator.GeneratePhrases` makes a single Ollama call to produce
3–5 short imperative trigger phrases for pipelines that define none. The
prompt:

```
Generate 3 to 5 short imperative phrases a user would type to explicitly invoke this pipeline.
Each phrase should start with a verb like "run", "execute", "launch", or "trigger".
Return ONLY a JSON array of strings — no explanation, no other text.

Pipeline name: {name}
Description: {description}

Example output: ["run {name}", "execute {name}", "launch {name} pipeline"]
```

`parsePhrasesResponse` extracts the first `[...]` substring from the response
and unmarshals it. Up to 8 phrases are accepted; empty strings are discarded.

Generated phrases are cached in the `pipelineEmbedding.GeneratedPhrases` field
(both in-memory and on-disk). They are keyed by the same `pipelineHash` as the
embedding vector, so a change to the pipeline's name or description invalidates
both the phrases and the vector simultaneously.

---

### 1.8 Feedback Logging

**File**: `internal/router/feedback.go`

Every routing decision is appended to `$CacheDir/routing-feedback.jsonl` as a
`FeedbackRecord`:

```go
type FeedbackRecord struct {
    Timestamp  string  `json:"ts"`
    Prompt     string  `json:"prompt"`
    Pipeline   string  `json:"pipeline"`            // "" = no match
    Confidence float64 `json:"confidence"`
    Method     string  `json:"method"`              // "embedding" | "llm" | "none"
    NearMiss   string  `json:"near_miss,omitempty"`
}
```

The logger opens the file in append mode (`O_APPEND|O_CREATE|O_WRONLY`) on
every write — no persistent file handle. All errors are silently discarded.
The `Record` method acquires a mutex before the `os.OpenFile` call so
concurrent goroutines writing to the same log do not interleave partial lines.

The log is append-only JSONL: one record per line, newest at the bottom. It is
never read by gl1tch itself — it exists for offline threshold tuning and misfire
analysis.

---

### 1.9 OTel Instrumentation

**File**: `internal/router/router.go`

Two instruments on the `gl1tch/router` meter:

```go
routerSimilarity  metric.Float64Histogram // gl1tch.router.similarity_score
routerStrategyUsed metric.Int64Counter    // gl1tch.router.strategy_used
```

The `router.classify` span records:
- `router.strategy` — `"embedding"`, `"embedding-negative"`, or `"llm"`
- `router.matched_pipeline` — pipeline name, or `""` for no-match
- `router.confidence` — final score

`routerStrategyUsed` is incremented with a `strategy` attribute on every
routing path so the distribution of embedding-negative / fast-path / LLM calls
is visible in aggregate.

---

## 2. Code Indexing

**File**: `internal/pipeline/action_index_code.go`

`builtin.index_code` is a registered builtin pipeline action. It walks a
directory tree, chunks source files, embeds each chunk, and stores the result
in the `brain_vectors` table via `brainrag.RAGStore`.

### 2.1 Pre-scan and Model Recommendation

Before indexing, the action walks the tree once (skipping the same `skipDirs`)
to count eligible files. It then calls `recommendEmbedModel`:

| File count | Recommended model | Rationale |
|---|---|---|
| ≤ 500 | `nomic-embed-text` | Fast, low memory, sufficient recall |
| ≤ 5,000 | `nomic-embed-text` | Works well; expect 1–3 min |
| ≤ 20,000 | `mxbai-embed-large` | Better recall justifies extra time |
| > 20,000 | `mxbai-embed-large` | Warn to narrow path or raise `chunk_size` |

The recommendation is printed to `w` (the step output writer). If the caller
specified a different model, a note is printed but the specified model is used.

### 2.2 Directory Skip List

`skipDirs` is a `map[string]bool` of directory names skipped during the walk.
It covers: version-control dirs (`.git`, `.svn`, `.hg`), dependency trees
(`vendor`, `node_modules`, `Pods`, etc.), build/compiled output (`build`,
`dist`, `target`, `out`, `bin`, `_build`, `release`, `debug`, etc.),
framework SSR caches (`.next`, `.nuxt`, `.svelte-kit`, `.astro`), Python
virtualenvs and caches (`venv`, `.venv`, `__pycache__`, `.pytest_cache`, etc.),
Java/Kotlin artefacts (`.gradle`, `.kotlin`), editor dirs (`.idea`, `.vscode`),
OS artefacts (`__MACOSX`), and gl1tch-specific dirs (`.worktrees`,
`systemprompts`).

In addition to the named list, any directory whose name starts with `"."` is
unconditionally skipped (catches the long tail of hidden tool dirs). The check
is `strings.HasPrefix(name, ".") && name != "."`.

### 2.3 Chunking Algorithm

```go
func chunkText(text string, chunkSize int) []string {
    overlap := chunkSize / 10   // ~10% overlap
    step    := chunkSize - overlap
    runes   := []rune(text)
    for start := 0; start < len(runes); start += step {
        end := min(start+chunkSize, len(runes))
        chunks = append(chunks, string(runes[start:end]))
    }
}
```

Default `chunkSize` is 1500 characters (~300–400 tokens for most code).
Overlap is `chunkSize / 10` (150 chars), ensuring a definition that spans a
chunk boundary appears in at least one chunk in full context. Chunking operates
on Unicode rune boundaries, not bytes.

### 2.4 Note ID Format

Each chunk is stored under a note ID:

```
file:<relative-path>:L<lineStart>-L<lineEnd>
```

Example: `file:internal/router/router.go:L47-L78`

`lineStart` and `lineEnd` are computed by counting `\n` characters in the text
up to the chunk start offset (via `chunkStart`) and through the chunk body.
These IDs are used as stable keys in the `brain_vectors` table and appear in
`QueryWithText` results so callers know which file and line range each chunk
came from.

### 2.5 Staleness Detection

`RAGStore.IndexNote` checks whether an entry already exists with the same
`note_id`, `hash` (SHA256 of chunk text), and `embed_id` (embedder identity
string). If all three match, the row is not re-embedded. This makes repeated
runs of `index_code` on an unchanged codebase nearly free — only changed or new
files produce embedding calls.

The upsert uses SQLite's `ON CONFLICT(cwd, note_id) DO UPDATE` to replace
stale rows in-place without a separate delete step.

---

## 3. Code Search

**File**: `internal/pipeline/action_search_code.go`

`builtin.search_code` embeds the query string and performs an in-process
cosine similarity scan against all `brain_vectors` rows for the current `cwd`
and `embed_id`.

### 3.1 Query Flow

```
query text
  → embedder.Embed(query)
  → SELECT note_id, text, vector FROM brain_vectors WHERE cwd=? AND embed_id=?
  → CosineSimilarity(queryVec, decodeVector(blob)) for each row
  → sort descending by score
  → return top-K (default 6) entries as VectorEntry{NoteID, Text}
```

The scan is full-table for the given `(cwd, embed_id)` scope — there is no ANN
index. For typical developer-machine repos (a few thousand chunks) a full scan
is fast enough (< 50ms) and avoids the complexity and write overhead of an
approximate index like HNSW or IVFFlat.

### 3.2 cwd and embed_id Scoping

All `brain_vectors` rows are scoped to two columns:

- `cwd` — absolute path of the working directory at index time. Prevents
  vectors from project A being returned when searching from project B even
  when they share the same database file (`~/.local/share/glitch/glitch.db`).
- `embed_id` — `"provider:model"` string (e.g. `"ollama:nomic-embed-text"`).
  Prevents cross-model comparisons: a vector from `nomic-embed-text` and a
  query embedded with `mxbai-embed-large` live in different vector spaces and
  their cosine similarity is meaningless.

Switching embedding model requires re-indexing; the old rows remain in the
table under the old `embed_id` and are never returned by queries using the new
embedder.

### 3.3 Output Format

`QueryWithText` returns `[]VectorEntry`. The `search_code` action formats
results as:

```
=== file:path/to/file.go:L12-L44 ===
<chunk text>

=== file:path/to/other.go:L88-L120 ===
<chunk text>
```

This format is designed to be injected directly into LLM prompts as context.
The `===` delimiters and note IDs give the model provenance without needing
a separate citation step.

---

## 4. Vector Storage

**File**: `internal/brainrag/store.go`

### 4.1 Schema

```sql
CREATE TABLE brain_vectors (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    cwd        TEXT NOT NULL,
    note_id    TEXT NOT NULL,
    text       TEXT NOT NULL,
    vector     BLOB NOT NULL,
    hash       TEXT NOT NULL,     -- SHA256(text) hex
    embed_id   TEXT NOT NULL,     -- "provider:model"
    indexed_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(cwd, note_id)
);
```

Applied by `store.Open` / `store.OpenAt` at database initialization time.

### 4.2 Vector Encoding

Vectors are serialized as little-endian IEEE 754 binary blobs:

```go
func encodeVector(v []float32) []byte {
    buf := make([]byte, len(v)*4)
    for i, f := range v {
        binary.LittleEndian.PutUint32(buf[i*4:], math.Float32bits(f))
    }
    return buf
}
```

A 768-dimension `nomic-embed-text` vector occupies 3,072 bytes. A
1024-dimension `mxbai-embed-large` vector occupies 4,096 bytes. Storing as
raw float32 blobs (not JSON arrays) keeps storage compact and eliminates
float-to-string precision loss at encode/decode time.

### 4.3 RefreshStale

`RefreshStale` iterates a list of `BrainNote` records (from the `brain_notes`
table), checks each against the stored `hash`, and re-embeds any where the
body has changed. Embed errors are printed to stderr but do not abort the
loop — partial re-indexing is better than a hard failure.

---

## 5. Embedder Stack

**File**: `internal/brainrag/embedder_*.go`

All embedders implement:

```go
type Embedder interface {
    Embed(ctx context.Context, text string) ([]float32, error)
    ID() string  // "provider:model" — used as embed_id in brain_vectors
}
```

### OllamaEmbedder

Default. Calls `POST http://localhost:11434/api/embeddings` with:

```json
{"model": "nomic-embed-text", "prompt": "<text>"}
```

Returns `[]float32` from `response.embedding`. Zero cost, zero latency beyond
local inference, no data leaves the machine.

### OpenAIEmbedder

`POST https://api.openai.com/v1/embeddings`. Default model
`text-embedding-3-small`. Accepts an optional `Dimensions` field for
truncated embeddings. `ID()` returns `"openai:<model>"`.

### VoyageEmbedder

`POST https://api.voyageai.com/v1/embeddings`. Default model `voyage-code-3`,
which is specialized for code retrieval. `ID()` returns `"voyage:<model>"`.

### buildEmbedder (arg resolution)

In `action_index_code.go` and `action_search_code.go`, the embedder is built
from pipeline step args by `buildEmbedder`:

1. If `embed_provider` is set, use it with `embed_model`, `embed_api_key`
   (resolved from `$ENV_VAR` if prefixed with `$`), and `embed_base_url`.
2. If `embed_provider` is absent, default to `"ollama"`.
3. Legacy `base_url` and `model` args map to the Ollama provider if
   `embed_base_url` / `embed_model` are not set.

---

## 6. Local-First Design and Dev-Machine Optimization

All intelligence operations default to Ollama on `localhost:11434`. Cloud
providers (OpenAI, Voyage) require explicit opt-in via `embed_provider`. This
means:

- **No network required for core operation.** Routing, indexing, and search
  all run offline.
- **No API key required by default.** A developer on a new machine needs only
  `ollama pull nomic-embed-text` to get full functionality.
- **Predictable latency.** Local embedding calls are 5–50ms per chunk
  depending on model and hardware. No cold-start or rate-limit variance.

Key optimizations for constrained hardware:

**Embedding cache** — `routing-index.json` is loaded at startup and
re-persisted on change. Pipeline vectors are never recomputed unless the
pipeline's content hash changes. On a warm cache, routing requires one
embedding call (the user prompt) regardless of how many pipelines exist.

**Negative gate** — The embedding stage eliminates the LLM call for prompts
with no topically relevant pipeline. On a typical session the majority of
prompts are conversational, not pipeline invocations, so the LLM is rarely
called for routing. This is the largest single latency saving.

**Fast path** — At cosine ≥ 0.85 + imperative input, routing completes with
zero LLM calls. The Ollama model for routing doesn't need to be loaded.

**Model auto-selection** — `recommendEmbedModel` advises `nomic-embed-text`
for repos under 5,000 files. At 768 dimensions and fast inference,
`nomic-embed-text` is appropriate for most developer workstations. The heavier
`mxbai-embed-large` (1,024 dimensions) is only recommended when recall at
scale justifies the additional memory and time.

**Full-scan instead of ANN** — For typical developer repos the `brain_vectors`
table holds tens of thousands of rows at most. A full cosine scan is
sub-50ms in Go at this scale. Avoiding an approximate nearest-neighbor
index (HNSW, IVFFLAT) eliminates a dependency, removes write amplification
on insert, and eliminates recall loss from approximate indexing. If corpus
size grows to the point where this becomes a bottleneck, the scan in
`RAGStore.Query` is the right place to add an index.

**Feedback log is never read at runtime** — `routing-feedback.jsonl` is
append-only. The logger acquires its mutex only to write, never to read.
There is no in-memory accumulation, no background flush goroutine, and no
impact on routing latency.
