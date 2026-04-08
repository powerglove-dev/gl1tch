package glitchd

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/8op-org/gl1tch/internal/capability"
	"github.com/8op-org/gl1tch/internal/esearch"
	"github.com/8op-org/gl1tch/internal/pipeline"
	"github.com/8op-org/gl1tch/pkg/glitchproto"
)

// BlockEvent re-exports glitchproto.BlockEvent so callers in this package
// don't need to import the proto package directly.
type BlockEvent = glitchproto.BlockEvent

// Block lifecycle phases re-exported from glitchproto.
const (
	BlockStart = glitchproto.BlockStart
	BlockChunk = glitchproto.BlockChunk
	BlockEnd   = glitchproto.BlockEnd
)

// ChainStep is a single step in a desktop builder chain. It mirrors the
// frontend ChainStep type so the JSON wire format stays in sync.
type ChainStep struct {
	Type string `json:"type"` // "prompt" | "agent" | "pipeline"

	// Prompt step fields
	ID               int64  `json:"id,omitempty"`
	Label            string `json:"label"`
	Body             string `json:"body,omitempty"`
	ExecutorOverride string `json:"executorOverride,omitempty"`
	ModelOverride    string `json:"modelOverride,omitempty"`

	// Agent step fields
	Name   string `json:"name,omitempty"`
	Kind   string `json:"kind,omitempty"`
	Invoke string `json:"invoke,omitempty"`

	// Pipeline step fields
	Path string `json:"path,omitempty"`
}

// RunChainOpts configures a chain run.
type RunChainOpts struct {
	StepsJSON       string // JSON-encoded []ChainStep
	UserText        string // optional trailing free-text from the chat box
	WorkspaceID     string
	DefaultProvider string
	DefaultModel    string
	SystemCtx       string // glitch system context to inject
	// Cwd is the working directory that shell steps and CLI-backed provider
	// steps should execute in. When empty, executors fall back to the
	// glitch-desktop process cwd — which is almost never what the user wants
	// (it pins every workflow run to the directory the app was launched from,
	// not the active workspace). Callers should set this to the workspace's
	// primary directory.
	Cwd string
	// EventCh, if non-nil, receives structured BlockEvents parsed from the
	// step writer's stdout via the gl1tch output protocol. Callers like the
	// desktop app use this to render typed blocks (notes, tables, status
	// pings) instead of raw text. The channel is closed by RunChain when
	// the run finishes.
	EventCh chan<- BlockEvent
	// StepEvents, if non-nil, receives lifecycle pings for each step in the
	// chain — Phase = "start" before the executor runs, Phase = "end" after
	// it returns. The desktop app uses these to drive a "gl1tch is thinking"
	// indicator while waiting on a provider for first output.
	StepEvents chan<- StepEvent
}

// StepEvent is a coarse lifecycle ping for a single chain step. It carries
// just enough metadata for the chat to label the in-flight provider call.
type StepEvent struct {
	Phase    string // "start" | "end"
	StepID   string // pipeline step ID, e.g. "step-0"
	Label    string // human label, e.g. "Security Scan"
	Provider string // executor name when known
}

// RunChain executes a desktop builder chain sequentially. Each step's output
// is captured and made available to the next step via {{ steps.step-N.value }}.
//
// Execution rules:
//   - prompt step: invoked via the step's executorOverride (or default provider).
//     If preceded by an agent step, the agent's instructions are prepended.
//   - agent step: not executed alone — sets context for the next prompt step.
//   - pipeline step: loaded from disk and inlined into the run as a sub-pipeline.
//   - User text (if any) becomes a final implicit prompt step.
func RunChain(ctx context.Context, opts RunChainOpts, tokenCh chan<- string) error {
	if tokenCh != nil {
		defer close(tokenCh)
	}
	if opts.EventCh != nil {
		defer close(opts.EventCh)
	}
	if opts.StepEvents != nil {
		defer close(opts.StepEvents)
	}

	var steps []ChainStep
	if err := json.Unmarshal([]byte(opts.StepsJSON), &steps); err != nil {
		return fmt.Errorf("chain: parse steps: %w", err)
	}

	// If user typed extra text, treat it as a final prompt step.
	if strings.TrimSpace(opts.UserText) != "" {
		steps = append(steps, ChainStep{
			Type:  "prompt",
			Label: "user",
			Body:  opts.UserText,
		})
	}

	if len(steps) == 0 {
		return fmt.Errorf("chain: no steps to run")
	}

	// Build an ad-hoc pipeline from the chain.
	p, err := buildPipelineFromChain(steps, opts)
	if err != nil {
		return err
	}

	mgr := buildManager()
	w := newChainStreamWriter(ctx, tokenCh, opts.EventCh)
	defer w.Close()

	runOpts := []pipeline.RunOption{
		pipeline.WithStepWriter(w),
		pipeline.WithSilentStatus(),
	}
	// Stamp the run row with the chain's workspace id so the
	// per-workspace PipelineIndexer can filter to its own runs and
	// avoid cross-workspace contamination in glitch-pipelines.
	if opts.WorkspaceID != "" {
		runOpts = append(runOpts, pipeline.WithWorkspaceID(opts.WorkspaceID))
	}

	startedAt := time.Now()
	_, err = pipeline.Run(ctx, p, mgr, "", runOpts...)
	emitBrainDecision(ctx, p, opts, err, time.Since(startedAt))
	return err
}

// emitBrainDecision indexes one glitch-brain-decisions doc per chain
// run so Kibana can chart "is the brain handling more work locally
// over time, or escalating to paid models more often?". Best-effort:
// any failure to write is logged and swallowed — chain runs must never
// fail because the audit log is unreachable.
//
// What gets recorded:
//   - chosen_provider/chosen_model: the *root* step's executor (the one
//     the brain picked first). For multi-step chains the rest live in
//     all_providers / all_models so dashboards can answer both "what
//     was the brain's first instinct" and "what did the run actually
//     touch end-to-end".
//   - escalated: true iff any step ran on a non-local provider. Local
//     == Ollama (see esearch.IsLocalProvider).
//   - question: the user's free-text instruction (or the first prompt
//     body when no free-text was provided), capped to 4 KiB.
//   - confidence/resolved: written as zero-values for now. The brain
//     self-rating path will populate them once it lands; existing
//     dashboards keep working in the meantime.
func emitBrainDecision(
	ctx context.Context,
	p *pipeline.Pipeline,
	opts RunChainOpts,
	runErr error,
	dur time.Duration,
) {
	if p == nil || len(p.Steps) == 0 {
		return
	}

	cfg, cerr := capability.LoadConfig()
	if cerr != nil || cfg.Elasticsearch.Address == "" {
		return
	}
	es, eerr := esearch.New(cfg.Elasticsearch.Address)
	if eerr != nil {
		return
	}

	// Per-step provider/model rollup. Pipelines built from chains use
	// the Executor field as the provider name and Model as the model.
	// Shell-only steps have no executor and don't count toward routing
	// decisions.
	var allProviders, allModels []string
	seenProv := map[string]bool{}
	seenModel := map[string]bool{}
	escalated := false
	for _, st := range p.Steps {
		if st.Executor == "" {
			continue
		}
		if !seenProv[st.Executor] {
			seenProv[st.Executor] = true
			allProviders = append(allProviders, st.Executor)
		}
		if st.Model != "" && !seenModel[st.Model] {
			seenModel[st.Model] = true
			allModels = append(allModels, st.Model)
		}
		if !esearch.IsLocalProvider(st.Executor) {
			escalated = true
		}
	}

	// Find the root provider — the first step that actually has an
	// executor. Skips leading shell steps so the "what did the brain
	// pick" column reflects the routing decision, not pipeline plumbing.
	chosenProvider := ""
	chosenModel := ""
	for _, st := range p.Steps {
		if st.Executor != "" {
			chosenProvider = st.Executor
			chosenModel = st.Model
			break
		}
	}

	question := strings.TrimSpace(opts.UserText)
	if question == "" {
		// Fall back to the first step's prompt body so saved chains
		// without a trailing user message still log a recognizable
		// question. Capped so a giant pipeline prompt doesn't bloat
		// the index.
		question = strings.TrimSpace(p.Steps[0].Prompt)
	}
	if len(question) > 4096 {
		question = question[:4096]
	}

	status := "success"
	if runErr != nil {
		status = "failure"
	}

	doc := esearch.BrainDecision{
		WorkspaceID:    opts.WorkspaceID,
		Question:       question,
		ChosenProvider: chosenProvider,
		ChosenModel:    chosenModel,
		AllProviders:   allProviders,
		AllModels:      allModels,
		Escalated:      escalated,
		Status:         status,
		StepCount:      len(p.Steps),
		DurationMs:     dur.Milliseconds(),
		Timestamp:      time.Now().UTC(),
	}

	// Use a short detached context so a cancelled chain (user closed
	// the chat) still gets its decision logged. The audit doc is small
	// and ES is local — 2s is plenty.
	writeCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := es.Index(writeCtx, esearch.IndexBrainDecisions, "", doc); err != nil {
		slog.Warn("brain-decisions: index failed", "err", err)
	}
	_ = ctx
}

// buildPipelineFromChain converts a desktop chain into a runnable pipeline.
// Prompt steps become provider executor steps, with previous step output
// threaded in via {{ steps.step-N.value }} references.
func buildPipelineFromChain(steps []ChainStep, opts RunChainOpts) (*pipeline.Pipeline, error) {
	p := &pipeline.Pipeline{
		Name:    "desktop-chain",
		Version: "1",
	}

	var pendingAgent string
	var prevStepID string
	stepIndex := 0

	for _, s := range steps {
		switch s.Type {
		case "agent":
			// Agent attaches to the next prompt step.
			pendingAgent = s.Invoke

		case "prompt":
			provider := s.ExecutorOverride
			model := s.ModelOverride
			if provider == "" {
				provider = opts.DefaultProvider
			}
			if model == "" && provider == opts.DefaultProvider {
				model = opts.DefaultModel
			}
			if provider == "" {
				return nil, fmt.Errorf("chain: prompt step %q has no executor (set a provider in the chat picker or override per-step)", s.Label)
			}

			body := s.Body
			if pendingAgent != "" {
				body = BuildAgentPrompt(pendingAgent, body)
				pendingAgent = ""
			}

			// Inject system context on the first executable step only.
			if stepIndex == 0 && opts.SystemCtx != "" {
				body = opts.SystemCtx + "\n\n---\n\n" + body
			}

			// If there's a previous step, append its output as context.
			if prevStepID != "" {
				body = body + "\n\n## Previous step output\n{{ steps." + prevStepID + ".value }}"
			}

			stepID := fmt.Sprintf("step-%d", stepIndex)
			step := pipeline.Step{
				ID:        stepID,
				Executor:  provider,
				Model:     model,
				Prompt:    body,
				Outputs:   map[string]string{"value": "string"},
				NoClarify: true,
			}
			// Force sequential execution: this step waits for the previous one.
			// Without Needs, the DAG runner schedules steps in parallel and the
			// next step can read step-N.value before step-N has produced it.
			if prevStepID != "" {
				step.Needs = []string{prevStepID}
			}
			p.Steps = append(p.Steps, step)
			prevStepID = stepID
			stepIndex++

		case "pipeline":
			// Inline an external workflow file as a sub-step. Saved chains may
			// embed legacy paths from before the workflow rename, so we resolve
			// the literal path first and fall back to the new layout.
			resolvedPath, err := resolveLegacyWorkflowPath(s.Path)
			if err != nil {
				return nil, fmt.Errorf("chain: open workflow %q: %w", s.Path, err)
			}
			subFile, err := os.Open(resolvedPath)
			if err != nil {
				return nil, fmt.Errorf("chain: open workflow %q: %w", resolvedPath, err)
			}
			sub, err := pipeline.Load(subFile)
			subFile.Close()
			if err != nil {
				return nil, fmt.Errorf("chain: load workflow %q: %w", resolvedPath, err)
			}

			prefix := fmt.Sprintf("sub%d-", stepIndex)
			// Rewrite IDs to avoid collisions. We DO NOT rewrite refs inside
			// step prompts — that would require template surgery. Sub-pipeline
			// steps already reference their own siblings by original ID; we
			// just need IDs to be unique across the merged pipeline. Since
			// pipeline runners look up steps by ID inside their own struct,
			// we keep the inner refs as-is and only prefix the IDs to avoid
			// collisions with other chain steps.
			for i := range sub.Steps {
				origID := sub.Steps[i].ID
				newID := prefix + origID
				sub.Steps[i].ID = newID
				// Update needs[] within this sub-pipeline.
				for j, n := range sub.Steps[i].Needs {
					sub.Steps[i].Needs[j] = prefix + n
				}
			}
			// Rewrite {{ steps.<orig>.<key> }} refs inside prompts so they
			// match the prefixed IDs.
			for i := range sub.Steps {
				sub.Steps[i].Prompt = rewriteStepRefs(sub.Steps[i].Prompt, sub.Steps, prefix)
				sub.Steps[i].Input = rewriteStepRefs(sub.Steps[i].Input, sub.Steps, prefix)
			}

			// Force sequential execution: each root sub-step (one with no
			// internal needs) depends on the previous chain step. This prevents
			// the DAG runner from starting the sub-pipeline before the upstream
			// chain step has emitted its output.
			if prevStepID != "" {
				for i := range sub.Steps {
					if len(sub.Steps[i].Needs) == 0 {
						sub.Steps[i].Needs = []string{prevStepID}
					}
				}
			}

			// The last sub-step becomes the new prevStepID.
			lastID := ""
			for _, st := range sub.Steps {
				p.Steps = append(p.Steps, st)
				lastID = st.ID
			}
			if lastID != "" {
				prevStepID = lastID
			}
			stepIndex++

		default:
			return nil, fmt.Errorf("chain: unknown step type %q", s.Type)
		}
	}

	if len(p.Steps) == 0 {
		return nil, fmt.Errorf("chain: no executable steps after expansion")
	}

	// Pin every step to the workspace cwd unless the step already declares one.
	// This is what makes "run workflow X from workspace Y" actually execute
	// inside Y's directory rather than whatever cwd glitch-desktop happened to
	// launch from. Applies to shell steps (relative paths like .github/...) and
	// provider CLIs (claude, opencode, etc.) whose tool use is scoped to cwd.
	if opts.Cwd != "" {
		for i := range p.Steps {
			if p.Steps[i].Vars == nil {
				p.Steps[i].Vars = map[string]string{}
			}
			if _, ok := p.Steps[i].Vars["cwd"]; !ok {
				p.Steps[i].Vars["cwd"] = opts.Cwd
			}
		}
	}
	return p, nil
}

// rewriteStepRefs replaces {{ steps.<id>.<key> }} occurrences for any id that
// belongs to the prefixed sub-pipeline. We do a naive string replace per
// known step ID; this is sufficient for the simple template syntax we use.
func rewriteStepRefs(s string, subSteps []pipeline.Step, prefix string) string {
	// Build set of original IDs (before this function is called the IDs have
	// already been prefixed in subSteps, so we strip the prefix to recover the
	// original).
	for _, st := range subSteps {
		origID := strings.TrimPrefix(st.ID, prefix)
		if origID == st.ID {
			continue
		}
		old := "steps." + origID + "."
		new := "steps." + st.ID + "."
		s = strings.ReplaceAll(s, old, new)
	}
	return s
}

// stepLabelsByID maps each pipeline step ID to a human label for the streamer.
func stepLabelsByID(p *pipeline.Pipeline) map[string]string {
	out := map[string]string{}
	for _, s := range p.Steps {
		out[s.ID] = s.ID
	}
	return out
}

// chainStreamWriter receives raw bytes from the pipeline runner and fans
// them out two ways:
//
//   - tokenCh (legacy): each Write() is forwarded verbatim as a string. Used
//     by older callers that still treat the chat as a plain text stream.
//   - eventCh (new):    bytes are routed through a glitchproto.StreamSplitter
//     so the desktop chat receives structured BlockEvents (notes, tables,
//     status pings) instead of raw text. The splitter handles markers that
//     straddle Write boundaries.
//
// Either channel may be nil. The writer is safe to call from a single
// goroutine; pipeline.Run guarantees serialized writes per step.
type chainStreamWriter struct {
	ctx      context.Context
	tokenCh  chan<- string
	eventCh  chan<- BlockEvent
	splitter *glitchproto.StreamSplitter
}

func newChainStreamWriter(
	ctx context.Context,
	tokenCh chan<- string,
	eventCh chan<- BlockEvent,
) *chainStreamWriter {
	w := &chainStreamWriter{ctx: ctx, tokenCh: tokenCh, eventCh: eventCh}
	if eventCh != nil {
		w.splitter = glitchproto.NewStreamSplitter(func(e BlockEvent) {
			select {
			case <-ctx.Done():
			case eventCh <- e:
			}
		})
	}
	return w
}

func (w *chainStreamWriter) Write(p []byte) (int, error) {
	if err := w.ctx.Err(); err != nil {
		return 0, err
	}
	if w.tokenCh != nil {
		select {
		case <-w.ctx.Done():
			return 0, w.ctx.Err()
		case w.tokenCh <- string(p):
		}
	}
	if w.splitter != nil {
		if _, err := w.splitter.Write(p); err != nil {
			return 0, err
		}
	}
	return len(p), nil
}

// Close flushes the splitter so any trailing partial-line bytes get
// emitted as a final text chunk. Safe to call when no splitter is set.
func (w *chainStreamWriter) Close() error {
	if w.splitter != nil {
		return w.splitter.Close()
	}
	return nil
}

var _ io.WriteCloser = (*chainStreamWriter)(nil)

// resolveLegacyWorkflowPath returns p unchanged if it exists, otherwise it
// rewrites the legacy ".glitch/pipelines/<name>.pipeline.yaml" layout to the
// current ".glitch/workflows/<name>.workflow.yaml" layout and returns that if
// it exists. Saved chains predating the workflow refactor embed the old paths.
func resolveLegacyWorkflowPath(p string) (string, error) {
	if _, err := os.Stat(p); err == nil {
		return p, nil
	}

	dir := filepath.Dir(p)
	base := filepath.Base(p)

	// Path component rewrite: .../.glitch/pipelines/X → .../.glitch/workflows/X
	if filepath.Base(dir) == "pipelines" && filepath.Base(filepath.Dir(dir)) == ".glitch" {
		dir = filepath.Join(filepath.Dir(filepath.Dir(dir)), ".glitch", "workflows")
	}

	// Filename rewrite: X.pipeline.yaml → X.workflow.yaml
	if strings.HasSuffix(base, ".pipeline.yaml") {
		base = strings.TrimSuffix(base, ".pipeline.yaml") + ".workflow.yaml"
	}

	candidate := filepath.Join(dir, base)
	if _, err := os.Stat(candidate); err == nil {
		return candidate, nil
	}

	return "", fmt.Errorf("not found: %s (also tried %s)", p, candidate)
}
