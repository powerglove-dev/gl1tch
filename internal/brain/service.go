// Package brain implements glitch's autonomous self-improvement loop.
// It runs as a background supervisor service, periodically introspecting
// indexed data (chat sessions, pipeline runs, routing decisions) and
// writing improvements back to the brain store.
package brain

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"

	"github.com/8op-org/gl1tch/internal/busd"
	"github.com/8op-org/gl1tch/internal/capability"
	"github.com/8op-org/gl1tch/internal/esearch"
	"github.com/8op-org/gl1tch/internal/observer"
	"github.com/8op-org/gl1tch/internal/store"
	"github.com/8op-org/gl1tch/internal/telemetry"
)

var tracer = otel.Tracer("gl1tch/brain")

// Status values published on the busd topic "brain.status".
const (
	StatusIdle      = "idle"
	StatusImproving = "improving"
)

// BusPayload is the JSON envelope published on "brain.status".
type BusPayload struct {
	Status  string `json:"status"`
	Detail  string `json:"detail,omitempty"`
	Learned int    `json:"learned,omitempty"`
}

// Service is the autonomous brain improvement loop. It runs as a supervisor
// service, introspecting glitch's own ES data at intervals and writing
// learnings back to the brain store.
type Service struct {
	// Interval between improvement cycles. Defaults to 30m.
	Interval time.Duration
}

func (s *Service) Name() string { return "brain" }

func (s *Service) Start(ctx context.Context) error {
	if s.Interval == 0 {
		s.Interval = 30 * time.Minute
	}

	cfg, err := capability.LoadConfig()
	if err != nil {
		slog.Warn("brain: config error", "err", err)
		return nil
	}

	es, err := esearch.New(cfg.Elasticsearch.Address)
	if err != nil || es.Ping(ctx) != nil {
		slog.Info("brain: ES not available, will retry on next cycle")
		return s.runLoop(ctx, cfg)
	}

	return s.runLoop(ctx, cfg)
}

func (s *Service) runLoop(ctx context.Context, cfg *capability.Config) error {
	// Run first cycle after a short warm-up so collectors have data.
	warmup := time.NewTimer(2 * time.Minute)
	defer warmup.Stop()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-warmup.C:
	}

	s.cycle(ctx, cfg)

	ticker := time.NewTicker(s.Interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			s.cycle(ctx, cfg)
		}
	}
}

func (s *Service) cycle(ctx context.Context, cfg *capability.Config) {
	ctx, span := tracer.Start(ctx, "brain.cycle")
	defer span.End()

	publishStatus(StatusImproving, "analyzing recent activity", 0)
	defer publishStatus(StatusIdle, "", 0)

	// Panic guard — a bad observer query, a nil store, a provider
	// response that breaks one of the learnFrom* helpers should NOT
	// tear down the brain goroutine for the rest of the process
	// lifetime. Recover, log, ship to Elastic APM with a full Go
	// stack, and let the next ticker tick retry the cycle cleanly.
	defer func() {
		if r := recover(); r != nil {
			err := fmt.Errorf("brain cycle panicked: %v", r)
			slog.Error("brain: cycle panicked", "panic", r)
			span.SetStatus(codes.Error, "panic")
			span.RecordError(err)
			telemetry.CaptureError(ctx, err, map[string]any{
				"subsystem": "brain",
			}, 1)
			publishStatus(StatusIdle, "cycle failed — see logs", 0)
		}
	}()

	es, err := esearch.New(cfg.Elasticsearch.Address)
	if err != nil || es.Ping(ctx) != nil {
		slog.Debug("brain: ES not available, skipping cycle")
		span.SetStatus(codes.Error, "ES unavailable")
		return
	}

	st, err := store.Open()
	if err != nil {
		slog.Warn("brain: store error", "err", err)
		span.SetStatus(codes.Error, "store error")
		return
	}
	defer st.Close()

	model := cfg.Model
	qe := observer.NewQueryEngine(es, model)

	var learned int

	// Phase 1: Learn from recent pipeline failures
	publishStatus(StatusImproving, "reviewing pipeline outcomes", 0)
	if n, err := s.learnFromPipelines(ctx, es, qe, st); err == nil {
		learned += n
	}

	// Phase 2: Learn from chat patterns — what does the user ask most?
	publishStatus(StatusImproving, "studying chat patterns", 0)
	if n, err := s.learnFromChats(ctx, es, qe, st); err == nil {
		learned += n
	}

	// Phase 3: Learn from directory scans — what capabilities are available?
	publishStatus(StatusImproving, "mapping capabilities", 0)
	if n, err := s.learnFromDirectoryScans(ctx, es, qe, st); err == nil {
		learned += n
	}

	span.SetAttributes(attribute.Int("brain.learned", learned))
	if learned > 0 {
		slog.Info("brain: cycle complete", "learned", learned)
		span.SetStatus(codes.Ok, "")
		publishStatus(StatusIdle, fmt.Sprintf("learned %d insights", learned), learned)
	}
}

// learnFromPipelines analyzes recent pipeline runs to identify failure patterns.
func (s *Service) learnFromPipelines(ctx context.Context, es *esearch.Client, qe *observer.QueryEngine, st *store.Store) (int, error) {
	query := map[string]any{
		"size": 20,
		"sort": []map[string]any{{"timestamp": map[string]any{"order": "desc", "unmapped_type": "date"}}},
		"query": map[string]any{
			"bool": map[string]any{
				"filter": []map[string]any{
					{"range": map[string]any{"timestamp": map[string]any{"gte": "now-24h"}}},
				},
			},
		},
	}

	results, err := es.Search(ctx, []string{esearch.IndexPipelines}, query)
	if err != nil || results.Total == 0 {
		return 0, err
	}

	// Count failures
	var failures, successes int
	var failedNames []string
	for _, r := range results.Results {
		var run struct {
			Name   string `json:"name"`
			Status string `json:"status"`
		}
		if err := unmarshalSource(r.Source, &run); err == nil {
			if run.Status == "failure" {
				failures++
				failedNames = append(failedNames, run.Name)
			} else {
				successes++
			}
		}
	}

	if failures == 0 {
		return 0, nil
	}

	// Ask Ollama to analyze the failure pattern
	answer, err := qe.Answer(ctx, fmt.Sprintf(
		"Analyze these pipeline failures from the last 24h and suggest what to fix. "+
			"Failed pipelines: %s. Success rate: %d/%d. "+
			"Give a single concise insight I should remember.",
		strings.Join(failedNames, ", "), successes, successes+failures))
	if err != nil {
		return 0, err
	}

	note := store.BrainNote{
		StepID:    "brain:pipelines",
		CreatedAt: time.Now().Unix(),
		Tags:      "type:capability,source:brain,category:pipeline-health",
		Body:      answer,
	}
	return 1, st.UpsertCapabilityNote(ctx, note)
}

// learnFromChats analyzes chat session patterns.
func (s *Service) learnFromChats(ctx context.Context, es *esearch.Client, qe *observer.QueryEngine, st *store.Store) (int, error) {
	query := map[string]any{
		"size": 30,
		"sort": []map[string]any{{"timestamp": map[string]any{"order": "desc", "unmapped_type": "date"}}},
		"query": map[string]any{
			"bool": map[string]any{
				"should": []map[string]any{
					{"term": map[string]any{"type": "claude.prompt"}},
					{"term": map[string]any{"type": "claude.session.human"}},
				},
				"minimum_should_match": 1,
				"filter": []map[string]any{
					{"range": map[string]any{"timestamp": map[string]any{"gte": "now-7d"}}},
				},
			},
		},
	}

	results, err := es.Search(ctx, []string{esearch.IndexEvents}, query)
	if err != nil || results.Total < 5 {
		return 0, err // not enough data to learn from
	}

	// Extract user prompts
	var prompts []string
	for _, r := range results.Results {
		var evt struct {
			Message string `json:"message"`
		}
		if err := unmarshalSource(r.Source, &evt); err == nil && evt.Message != "" {
			prompts = append(prompts, evt.Message)
		}
	}

	if len(prompts) < 3 {
		return 0, nil
	}

	answer, err := qe.Answer(ctx, fmt.Sprintf(
		"Analyze these %d recent user prompts to glitch and identify the top themes/patterns. "+
			"What does the user care about most? What should glitch get better at? "+
			"Give a single concise insight.\n\nPrompts:\n- %s",
		len(prompts), strings.Join(prompts, "\n- ")))
	if err != nil {
		return 0, err
	}

	note := store.BrainNote{
		StepID:    "brain:chat-patterns",
		CreatedAt: time.Now().Unix(),
		Tags:      "type:capability,source:brain,category:user-patterns",
		Body:      answer,
	}
	return 1, st.UpsertCapabilityNote(ctx, note)
}

// learnFromDirectoryScans synthesizes capabilities from scanned directories.
func (s *Service) learnFromDirectoryScans(ctx context.Context, es *esearch.Client, qe *observer.QueryEngine, st *store.Store) (int, error) {
	query := map[string]any{
		"size": 50,
		"sort": []map[string]any{{"timestamp": map[string]any{"order": "desc", "unmapped_type": "date"}}},
		"query": map[string]any{
			"bool": map[string]any{
				"should": []map[string]any{
					{"term": map[string]any{"type": "directory.skill"}},
					{"term": map[string]any{"type": "directory.agent"}},
					{"term": map[string]any{"type": "directory.provider_config"}},
					{"term": map[string]any{"type": "directory.structure"}},
					{"term": map[string]any{"type": "directory.remote"}},
				},
				"minimum_should_match": 1,
			},
		},
	}

	results, err := es.Search(ctx, []string{esearch.IndexEvents}, query)
	if err != nil || results.Total == 0 {
		return 0, err
	}

	// Build a capability summary
	var skills, agents, repos []string
	for _, r := range results.Results {
		var evt struct {
			Type    string         `json:"type"`
			Repo    string         `json:"repo"`
			Message string         `json:"message"`
			Meta    map[string]any `json:"metadata"`
		}
		if err := unmarshalSource(r.Source, &evt); err != nil {
			continue
		}
		switch evt.Type {
		case "directory.skill":
			if name, ok := evt.Meta["skill_name"].(string); ok {
				skills = append(skills, name)
			}
		case "directory.agent":
			if name, ok := evt.Meta["agent_name"].(string); ok {
				agents = append(agents, name)
			}
		case "directory.remote":
			if repo, ok := evt.Meta["github_repo"].(string); ok {
				repos = append(repos, repo)
			}
		}
	}

	if len(skills)+len(agents)+len(repos) == 0 {
		return 0, nil
	}

	summary := fmt.Sprintf("Available skills: %s\nAvailable agents: %s\nMonitored repos: %s",
		strings.Join(dedup(skills), ", "),
		strings.Join(dedup(agents), ", "),
		strings.Join(dedup(repos), ", "))

	note := store.BrainNote{
		StepID:    "brain:capabilities",
		CreatedAt: time.Now().Unix(),
		Tags:      "type:capability,source:brain,category:capabilities",
		Body:      summary,
	}
	return 1, st.UpsertCapabilityNote(ctx, note)
}

func publishStatus(status, detail string, learned int) {
	sockPath, err := busd.SocketPath()
	if err != nil {
		return
	}
	_ = busd.PublishEvent(sockPath, "brain.status", BusPayload{
		Status:  status,
		Detail:  detail,
		Learned: learned,
	})
}

func dedup(ss []string) []string {
	seen := make(map[string]bool, len(ss))
	var out []string
	for _, s := range ss {
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	return out
}

func unmarshalSource(raw []byte, v any) error {
	return json.Unmarshal(raw, v)
}
