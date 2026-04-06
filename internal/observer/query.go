// Package observer implements the query engine that bridges natural language
// questions to Elasticsearch and synthesizes answers via Ollama.
package observer

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/8op-org/gl1tch/internal/esearch"
)

// QueryEngine translates natural language questions into ES queries, fetches
// results, and synthesizes answers using a local Ollama model.
type QueryEngine struct {
	es    *esearch.Client
	model string // Ollama model for query generation + synthesis
}

// NewQueryEngine creates a query engine backed by the given ES client.
// model is the Ollama model to use (e.g. "llama3.2", "qwen2.5").
func NewQueryEngine(es *esearch.Client, model string) *QueryEngine {
	if model == "" {
		model = "llama3.2"
	}
	return &QueryEngine{es: es, model: model}
}

// allIndices returns the list of all observer indices.
func allIndices() []string {
	return []string{
		esearch.IndexEvents,
		esearch.IndexSummaries,
		esearch.IndexPipelines,
		esearch.IndexInsights,
	}
}

// Answer processes a natural language question and returns a synthesized answer.
func (q *QueryEngine) Answer(ctx context.Context, question string) (string, error) {
	results, err := q.searchWithFallback(ctx, question)
	if err != nil {
		return "", fmt.Errorf("search: %w", err)
	}

	answer, err := q.synthesize(ctx, question, results)
	if err != nil {
		return "", fmt.Errorf("synthesize: %w", err)
	}

	return answer, nil
}

// searchWithFallback tries the LLM-generated query first, falls back to default.
func (q *QueryEngine) searchWithFallback(ctx context.Context, question string) (*esearch.SearchResponse, error) {
	esQuery, err := q.generateQuery(ctx, question)
	if err != nil {
		esQuery = defaultQuery(question)
	}

	results, err := q.es.Search(ctx, allIndices(), esQuery)
	if err != nil {
		// LLM-generated query failed — try the safe fallback.
		results, err = q.es.Search(ctx, allIndices(), defaultQuery(question))
		if err != nil {
			return nil, err
		}
	}
	return results, nil
}

// Stream processes a question and streams the synthesized answer token by token.
// Each token is sent to the tokenCh channel. The channel is closed when done.
func (q *QueryEngine) Stream(ctx context.Context, question string, tokenCh chan<- string) error {
	defer close(tokenCh)

	results, err := q.searchWithFallback(ctx, question)
	if err != nil {
		return fmt.Errorf("search: %w", err)
	}

	return q.streamSynthesize(ctx, question, results, tokenCh)
}

// defaultQuery builds a safe fallback ES query for the given question.
func defaultQuery(question string) map[string]any {
	return map[string]any{
		"size": 20,
		"sort": []map[string]any{{"timestamp": map[string]any{"order": "desc", "unmapped_type": "date"}}},
		"query": map[string]any{
			"bool": map[string]any{
				"should": []map[string]any{
					{"multi_match": map[string]any{
						"query":  question,
						"fields": []string{"message", "body", "summary", "pattern", "stdout", "name"},
						"type":   "best_fields",
					}},
					{"multi_match": map[string]any{
						"query":  question,
						"fields": []string{"type", "source", "repo", "author", "provider"},
					}},
				},
				"minimum_should_match": 1,
			},
		},
	}
}

func (q *QueryEngine) generateQuery(ctx context.Context, question string) (map[string]any, error) {
	now := time.Now()
	today := now.Format("2006-01-02")
	weekAgo := now.Add(-7 * 24 * time.Hour).Format("2006-01-02")
	dayAgo := now.Add(-24 * time.Hour).Format("2006-01-02")

	prompt := fmt.Sprintf(`You are an Elasticsearch query generator. Return ONLY valid JSON — no explanation.

Indices:
- glitch-events: type(keyword), source(keyword), repo(keyword), author(keyword), message(text), body(text), metadata(object), timestamp(date)
- glitch-summaries: scope(keyword), date(date), summary(text), timestamp(date)
- glitch-pipelines: name(keyword), status(keyword), stdout(text), model(keyword), provider(keyword), timestamp(date)
- glitch-insights: type(keyword), pattern(text), recommendation(text), timestamp(date)

Sources in glitch-events:
- source:"git" — git commits (type: git.commit)
- source:"github" — PRs and issues (type: github.pr, github.issue)
- source:"claude" — Claude Code conversations (type: claude.prompt, claude.session.*)
- source:"copilot" — Copilot CLI history (type: copilot.command, copilot.log)
- source:"mattermost" — chat messages (type: mattermost.message, mattermost.direct, mattermost.mention, mattermost.group)
  metadata fields: channel_name(keyword), channel_type(keyword), post_id(keyword)

When the user mentions a chat channel name (e.g. "devchat", "business"), filter with: {"term": {"metadata.channel_name.keyword": "DevChat"}}
When the user asks about chat/messages/mattermost, filter with: {"term": {"source": "mattermost"}}

Today: %s  Yesterday: %s  Week ago: %s

Question: "%s"

Rules:
- Always include: "sort": [{"timestamp": {"order": "desc", "unmapped_type": "date"}}]
- For time ranges use ISO dates not date math: "range": {"timestamp": {"gte": "%s"}}
- Default size: 20
- Use multi_match with fields [message, body, summary, pattern, stdout, name] for free-text search
- Use term filters on source, type, metadata.channel_name.keyword for precise filtering

JSON:`, today, dayAgo, weekAgo, question, dayAgo)

	resp, err := ollamaGenerate(ctx, q.model, prompt, false)
	if err != nil {
		return defaultQuery(question), nil
	}

	jsonStr := extractJSON(resp)
	var query map[string]any
	if err := json.Unmarshal([]byte(jsonStr), &query); err != nil {
		return defaultQuery(question), nil
	}

	// Ensure safe sort (unmapped_type prevents errors on empty indices).
	ensureSafeSort(query)

	return query, nil
}

// ensureSafeSort patches the sort clause to include unmapped_type.
func ensureSafeSort(query map[string]any) {
	sortRaw, ok := query["sort"]
	if !ok {
		query["sort"] = []map[string]any{{"timestamp": map[string]any{"order": "desc", "unmapped_type": "date"}}}
		return
	}
	sortArr, ok := sortRaw.([]any)
	if !ok {
		return
	}
	for i, item := range sortArr {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		if ts, exists := m["timestamp"]; exists {
			switch v := ts.(type) {
			case string:
				// "desc" → proper object
				sortArr[i] = map[string]any{"timestamp": map[string]any{"order": v, "unmapped_type": "date"}}
			case map[string]any:
				v["unmapped_type"] = "date"
			}
		}
	}
}

func (q *QueryEngine) synthesize(ctx context.Context, question string, results *esearch.SearchResponse) (string, error) {
	context := formatResults(results)

	prompt := fmt.Sprintf(`You are gl1tch, an AI observer assistant for a developer. You have access to indexed data from git repos, GitHub PRs/issues, Claude Code sessions, Copilot CLI, Mattermost chat channels, pipelines, and other sources. Each result includes a "source" field indicating where it came from.

Based on the following data from your observation indices, answer the user's question concisely and helpfully.

Question: %s

Observed data (%d results):
%s

Rules:
- Be direct and specific — cite repos, commits, timestamps when relevant
- If the data doesn't contain enough information, say so honestly
- Format with markdown for readability
- Keep answers concise but complete`, question, results.Total, context)

	return ollamaGenerate(ctx, q.model, prompt, false)
}

func (q *QueryEngine) streamSynthesize(ctx context.Context, question string, results *esearch.SearchResponse, tokenCh chan<- string) error {
	context := formatResults(results)

	prompt := fmt.Sprintf(`You are gl1tch, an AI observer assistant for a developer. You have access to indexed data from git repos, GitHub PRs/issues, Claude Code sessions, Copilot CLI, Mattermost chat channels, pipelines, and other sources. Each result includes a "source" field indicating where it came from.

Based on the following data from your observation indices, answer the user's question concisely and helpfully.

Question: %s

Observed data (%d results):
%s

Rules:
- Be direct and specific — cite repos, commits, timestamps when relevant
- If the data doesn't contain enough information, say so honestly
- Format with markdown for readability
- Keep answers concise but complete`, question, results.Total, context)

	return ollamaStream(ctx, q.model, prompt, tokenCh)
}

func formatResults(results *esearch.SearchResponse) string {
	if results == nil || len(results.Results) == 0 {
		return "(no results found)"
	}

	var sb strings.Builder
	for i, r := range results.Results {
		if i >= 15 { // Cap context to avoid blowing the prompt.
			fmt.Fprintf(&sb, "\n... and %d more results", len(results.Results)-15)
			break
		}
		fmt.Fprintf(&sb, "\n[%s] ", r.Index)
		sb.Write(r.Source)
		sb.WriteByte('\n')
	}
	return sb.String()
}

// extractJSON finds the first JSON object in a string, handling markdown fences.
func extractJSON(s string) string {
	s = strings.TrimSpace(s)
	// Strip markdown code fences.
	if strings.HasPrefix(s, "```") {
		lines := strings.SplitN(s, "\n", 2)
		if len(lines) > 1 {
			s = lines[1]
		}
		if idx := strings.LastIndex(s, "```"); idx >= 0 {
			s = s[:idx]
		}
	}
	s = strings.TrimSpace(s)

	// Find the first { and last }.
	start := strings.Index(s, "{")
	end := strings.LastIndex(s, "}")
	if start >= 0 && end > start {
		return s[start : end+1]
	}
	return s
}

// ollamaGenerate calls the Ollama generate API.
func ollamaGenerate(ctx context.Context, model, prompt string, stream bool) (string, error) {
	body, _ := json.Marshal(map[string]any{
		"model":  model,
		"prompt": prompt,
		"stream": stream,
	})

	req, err := http.NewRequestWithContext(ctx, "POST", "http://localhost:11434/api/generate", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("ollama unavailable: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		b, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("ollama %d: %s", resp.StatusCode, b)
	}

	var result struct {
		Response string `json:"response"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("decode ollama response: %w", err)
	}
	return result.Response, nil
}

// ollamaStream calls the Ollama generate API in streaming mode and sends
// tokens to the provided channel.
func ollamaStream(ctx context.Context, model, prompt string, tokenCh chan<- string) error {
	body, _ := json.Marshal(map[string]any{
		"model":  model,
		"prompt": prompt,
		"stream": true,
	})

	req, err := http.NewRequestWithContext(ctx, "POST", "http://localhost:11434/api/generate", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("ollama unavailable: %w", err)
	}
	defer resp.Body.Close()

	dec := json.NewDecoder(resp.Body)
	for {
		var chunk struct {
			Response string `json:"response"`
			Done     bool   `json:"done"`
		}
		if err := dec.Decode(&chunk); err != nil {
			if err == io.EOF {
				return nil
			}
			return err
		}
		if chunk.Response != "" {
			select {
			case tokenCh <- chunk.Response:
			case <-ctx.Done():
				return ctx.Err()
			}
		}
		if chunk.Done {
			return nil
		}
	}
}
