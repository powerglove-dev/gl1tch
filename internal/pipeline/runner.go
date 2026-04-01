package pipeline

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/powerglove-dev/gl1tch/internal/activity"
	"github.com/powerglove-dev/gl1tch/internal/brainaudit"
	"github.com/powerglove-dev/gl1tch/internal/brainrag"
	"github.com/powerglove-dev/gl1tch/internal/busd/topics"
	"github.com/powerglove-dev/gl1tch/internal/clarify"
	"github.com/powerglove-dev/gl1tch/internal/executor"
	"github.com/powerglove-dev/gl1tch/internal/store"
)

// RunOption configures a pipeline Run call.
type RunOption func(*runConfig)

// resumeFromConfig holds the data needed to resume a run from a checkpointed step.
type resumeFromConfig struct {
	runID  int64
	stepID string
	prompt string
}

type runConfig struct {
	store           *store.Store
	publisher       EventPublisher
	runID           int64
	pipeName        string
	injector        BrainInjector     // optional brain context injector
	ragStore        *brainrag.RAGStore // optional RAG vector store for refresh
	ragInjector     *brainrag.BrainInjector // optional RAG-based brain injector
	resumeStepID    string
	resumePrompt    string
	resumeFrom      *resumeFromConfig
}

// WithRunStore attaches a result store to the run so results are persisted.
// The store receives a RecordRunStart call before execution and
// RecordRunComplete after, regardless of success or failure.
func WithRunStore(s *store.Store) RunOption {
	return func(c *runConfig) { c.store = s }
}

// WithEventPublisher attaches an EventPublisher that receives pipeline and step
// lifecycle events during Run.  If not set, NoopPublisher{} is used.
func WithEventPublisher(p EventPublisher) RunOption {
	return func(c *runConfig) { c.publisher = p }
}

// WithBrainInjector attaches a BrainInjector to the run. The brain is always on —
// when configured, the injector is used to assemble pre-context for every step.
func WithBrainInjector(inj BrainInjector) RunOption {
	return func(c *runConfig) { c.injector = inj }
}

// WithRAGStore attaches a RAGStore to the run. Before any step dispatch,
// the runner will call rag.RefreshStale in a goroutine with a 2-second timeout warning.
func WithRAGStore(rag *brainrag.RAGStore) RunOption {
	return func(c *runConfig) { c.ragStore = rag }
}

// WithBrainRAGInjector attaches a RAG-based BrainInjector to the run.
// Before each step dispatch, InjectInto is called to prepend relevant brain notes
// to the prompt. On error, the original prompt is used.
func WithBrainRAGInjector(inj *brainrag.BrainInjector) RunOption {
	return func(c *runConfig) { c.ragInjector = inj }
}

// WithResumeFrom instructs the runner to resume an existing run from a
// checkpointed step. It skips RecordRunStart (using the original runID),
// re-hydrates ExecutionContext from prior step checkpoints, and resumes
// execution at stepID with followUpPrompt as the prompt.
func WithResumeFrom(runID int64, stepID, followUpPrompt string) RunOption {
	return func(c *runConfig) {
		c.resumeFrom = &resumeFromConfig{runID: runID, stepID: stepID, prompt: followUpPrompt}
	}
}

// StepStatusLineFormat is the format string for structured step-status log lines.
// The switchboard log-watcher parses lines matching this pattern.
const StepStatusLineFormat = "[step:%s] status:%s"

// stepStatus represents the lifecycle state of a step.
type stepStatus int

const (
	statusWaiting  stepStatus = iota
	statusRunning
	statusDone
	statusFailed
	statusSkipped
)

// stepState holds the mutable state of a single step during execution.
type stepState struct {
	mu          sync.Mutex
	status      stepStatus
	output      map[string]any
	pendingDeps atomic.Int32
}

// stepResult carries the outcome of a completed step goroutine.
type stepResult struct {
	id         string
	output     map[string]any
	err        error
	skipped    bool      // true when the step was skipped due to a dependency failure
	startedAt  time.Time // when the step goroutine started executing
	durationMs int64     // execution duration in milliseconds
	prompt     string    // resolved (pre-brain-injection) prompt sent to the plugin
}

// Run executes a pipeline against the given plugin manager.
// userInput is the initial value injected for the first input step.
// Optional RunOption values (e.g. WithRunStore, WithEventPublisher) configure behaviour.
// Returns the final output string (last plugin step output).
func Run(ctx context.Context, p *Pipeline, mgr *executor.Manager, userInput string, opts ...RunOption) (string, error) {
	cfg := &runConfig{}
	for _, o := range opts {
		o(cfg)
	}
	if cfg.publisher == nil {
		cfg.publisher = NoopPublisher{}
	}

	cfg.pipeName = p.Name

	// Record run start in the store (nil-safe).
	// When resuming an existing run, skip RecordRunStart and re-use the original runID.
	startedAt := time.Now()
	if cfg.resumeFrom != nil {
		// Resume path: inherit the original run ID and step/prompt from config.
		cfg.runID = cfg.resumeFrom.runID
		cfg.resumeStepID = cfg.resumeFrom.stepID
		cfg.resumePrompt = cfg.resumeFrom.prompt
	} else if cfg.store != nil {
		meta := ""
		if cwd, err := os.Getwd(); err == nil && cwd != "" {
			if b, err := json.Marshal(map[string]string{"cwd": cwd}); err == nil {
				meta = string(b)
			}
		}
		id, err := cfg.store.RecordRunStart("pipeline", p.Name, meta)
		if err == nil {
			cfg.runID = id
		}
	}

	// Publish pipeline.run.started.
	if payload, err := json.Marshal(map[string]any{
		"run_id":     cfg.runID,
		"pipeline":   p.Name,
		"started_at": startedAt.Format(time.RFC3339),
	}); err == nil {
		_ = cfg.publisher.Publish(ctx, topics.RunStarted, payload)
	}
	_ = activity.AppendEvent(activity.DefaultPath(), activity.Now(
		"pipeline_started", p.Name, p.Name, "running",
	))

	// If a RAGStore is configured, refresh stale embeddings before any step executes.
	// This runs in a goroutine with a 2-second timeout warning so it does not block execution.
	if cfg.ragStore != nil && cfg.store != nil {
		done := make(chan struct{})
		go func() {
			defer close(done)
			refreshCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
			defer cancel()
			notes, _ := cfg.store.AllBrainNotes(refreshCtx)
			baseURL := brainrag.DefaultBaseURL
			model := brainrag.DefaultEmbedModel
			if cfg.ragInjector != nil {
				if cfg.ragInjector.BaseURL != "" {
					baseURL = cfg.ragInjector.BaseURL
				}
				if cfg.ragInjector.Model != "" {
					model = cfg.ragInjector.Model
				}
			}
			_ = cfg.ragStore.RefreshStale(refreshCtx, baseURL, model, notes)
		}()
		// Warn if refresh takes more than 2 seconds.
		select {
		case <-done:
		case <-time.After(2 * time.Second):
			fmt.Fprintf(os.Stderr, "[brainrag] warn: RAG refresh is taking longer than 2s, continuing without waiting\n")
		}
	}

	var result string
	var runErr error

	// Handle legacy sequential pipeline (no Needs used) plus input/output step types.
	// If none of the steps has Needs, Retry, ForEach, or builtin types, fall through
	// to the legacy runner for full backwards compatibility.
	if isLegacyPipeline(p) {
		result, runErr = runLegacy(ctx, p, mgr, userInput, cfg)
	} else {
		result, runErr = runDAG(ctx, p, mgr, userInput, cfg)
	}

	finishedAt := time.Now()
	durationMs := finishedAt.Sub(startedAt).Milliseconds()

	// Record completion in the store (nil-safe).
	if cfg.store != nil && cfg.runID > 0 {
		exitStatus := 0
		stderr := ""
		if runErr != nil {
			exitStatus = 1
			stderr = runErr.Error()
		}
		_ = cfg.store.RecordRunComplete(cfg.runID, exitStatus, result, stderr)
	}

	// Publish pipeline.run.completed or pipeline.run.failed.
	exitStatus := 0
	topic := topics.RunCompleted
	if runErr != nil {
		exitStatus = 1
		topic = topics.RunFailed
	}
	if payload, err := json.Marshal(map[string]any{
		"run_id":      cfg.runID,
		"pipeline":    p.Name,
		"exit_status": exitStatus,
		"duration_ms": durationMs,
		"started_at":  startedAt.Format(time.RFC3339),
		"finished_at": finishedAt.Format(time.RFC3339),
	}); err == nil {
		_ = cfg.publisher.Publish(ctx, topic, payload)
	}
	if runErr != nil {
		_ = activity.AppendEvent(activity.DefaultPath(), activity.Now(
			"pipeline_failed", p.Name, p.Name, "failed",
		))
	} else {
		_ = activity.AppendEvent(activity.DefaultPath(), activity.Now(
			"pipeline_finished", p.Name, p.Name, "done",
		))
	}

	return result, runErr
}

// isLegacyPipeline returns true if none of the steps use DAG-only features,
// allowing the old sequential code path to handle it unmodified.
// A pipeline with MaxParallel explicitly set also uses the DAG runner.
func isLegacyPipeline(p *Pipeline) bool {
	if p.MaxParallel > 0 {
		return false
	}
	for _, s := range p.Steps {
		if len(s.Needs) > 0 || s.Retry != nil || s.ForEach != "" || s.OnFailure != "" {
			return false
		}
		if strings.HasPrefix(s.Type, "builtin.") || strings.HasPrefix(s.Executor, "builtin.") {
			return false
		}
	}
	return true
}

// runLegacy is the original sequential runner kept for backwards compatibility.
// It handles "input"/"output" step types and condition branches.
func runLegacy(ctx context.Context, p *Pipeline, mgr *executor.Manager, userInput string, cfg *runConfig) (string, error) {
	ec := NewExecutionContext(WithStore(cfg.store))
	if cfg.injector != nil {
		ec.SetBrainInjector(cfg.injector, cfg.runID)
	}
	if cfg.ragInjector != nil {
		ec.SetRAGInjector(cfg.ragInjector)
	}
	// Brain is always on: when a BrainInjector is provided, every step will receive
	// brain context. If no injector is provided, the runner proceeds without injection.

	// Expose the process working directory so pipeline steps can use {{cwd}}.
	if cwd, err := os.Getwd(); err == nil {
		ec.Set("cwd", cwd)
	}

	for k, v := range p.Vars {
		ec.Set("param."+k, v)
	}

	// Re-hydrate ExecutionContext from completed step checkpoints when resuming.
	// This restores all {{stepID.out}} interpolation values that were set by prior steps.
	if cfg.resumeFrom != nil && cfg.store != nil {
		checkpoints, _ := cfg.store.LoadStepCheckpoints(cfg.resumeFrom.runID)
		for _, cp := range checkpoints {
			if cp.Status != "done" {
				continue
			}
			var vars map[string]string
			if err := json.Unmarshal([]byte(cp.VarsJSON), &vars); err == nil {
				for k, v := range vars {
					ec.Set(k, v)
				}
			}
		}
	}

	byID := make(map[string]*Step, len(p.Steps))
	order := make([]string, 0, len(p.Steps))
	for i := range p.Steps {
		byID[p.Steps[i].ID] = &p.Steps[i]
		order = append(order, p.Steps[i].ID)
	}

	visited := make(map[string]bool)
	queue := append([]string(nil), order...)

	lastOutput := userInput
	var lastExecutorOutput string

	// resumeStarted tracks whether we have reached the resume step yet.
	// When cfg.resumeStepID is set, we skip all steps before the matching one.
	resumeStarted := cfg.resumeStepID == ""
	var forceInput string

	// stepIdx counts plugin steps executed (used for checkpoint ordering).
	stepIdx := 0

	for len(queue) > 0 {
		id := queue[0]
		queue = queue[1:]

		if visited[id] {
			continue
		}
		visited[id] = true

		step, ok := byID[id]
		if !ok {
			return "", fmt.Errorf("pipeline: unknown step id %q", id)
		}

		// Resume support: skip steps before the target step.
		if !resumeStarted {
			if step.ID != cfg.resumeStepID {
				continue
			}
			resumeStarted = true
			forceInput = cfg.resumePrompt
		}

		switch step.Type {
		case "input":
			ec.Set(step.ID+".out", userInput)
			// Also store in new-style context for template compat.
			ec.Set("step."+step.ID+".data.value", userInput)
			lastOutput = userInput

		case "output":
			return lastExecutorOutput, nil

		default:
			// Publish step.started.
			if payload, err := json.Marshal(map[string]any{
				"run_id":   cfg.runID,
				"pipeline": p.Name,
				"step":     step.ID,
				"status":   "running",
			}); err == nil {
				_ = cfg.publisher.Publish(ctx, topics.StepStarted, payload)
			}

			// Resolve the configured prompt for inbox display (pre-brain-injection).
			legacySnap := ec.Snapshot()
			rawPrompt := Interpolate(step.Prompt+step.Input, legacySnap)

			// Write-ahead checkpoint before executing.
			if cfg.store != nil {
				_ = cfg.store.StartStepCheckpoint(cfg.runID, step.ID, stepIdx, rawPrompt, step.Model)
			}
			stepIdx++

			stepStart := time.Now()
			output, err := executePluginStep(ctx, step, ec, mgr, lastOutput, p, cfg.store, cfg.runID, forceInput)
			forceInput = "" // only applies to the resume step
			stepDurationMs := time.Since(stepStart).Milliseconds()

			if err != nil {
				// Publish step.failed.
				if payload, merr := json.Marshal(map[string]any{
					"run_id":      cfg.runID,
					"pipeline":    p.Name,
					"step":        step.ID,
					"status":      "failed",
					"duration_ms": stepDurationMs,
				}); merr == nil {
					_ = cfg.publisher.Publish(ctx, topics.StepFailed, payload)
				}
				// Record step in store.
				if cfg.store != nil {
					rec := store.StepRecord{
						ID:         step.ID,
						Status:     "failed",
						Model:      step.Model,
						Prompt:     rawPrompt,
						StartedAt:  stepStart.Format(time.RFC3339),
						FinishedAt: time.Now().Format(time.RFC3339),
						DurationMs: stepDurationMs,
					}
					_ = cfg.store.RecordStepComplete(ctx, cfg.runID, rec)
					_ = cfg.store.CompleteStepCheckpoint(cfg.runID, step.ID, "failed", "", nil, stepDurationMs)
				}
				return "", fmt.Errorf("pipeline: step %q: %w", step.ID, err)
			}

			ec.Set(step.ID+".out", output)
			ec.Set("step."+step.ID+".state", "done")
			ec.Set("step."+step.ID+".data.value", output)
			lastExecutorOutput = output
			lastOutput = output

			// Store declared step outputs.
			for key := range step.Outputs {
				ec.SetStepOutput(step.ID, key, output)
			}

			outMap := map[string]any{"value": output}

			// Publish step.done.
			if payload, merr := json.Marshal(map[string]any{
				"run_id":      cfg.runID,
				"pipeline":    p.Name,
				"step":        step.ID,
				"status":      "done",
				"duration_ms": stepDurationMs,
				"output":      outMap,
			}); merr == nil {
				_ = cfg.publisher.Publish(ctx, topics.StepDone, payload)
			}

			// Record step in store.
			if cfg.store != nil {
				rec := store.StepRecord{
					ID:         step.ID,
					Status:     "done",
					Model:      step.Model,
					Prompt:     rawPrompt,
					StartedAt:  stepStart.Format(time.RFC3339),
					FinishedAt: time.Now().Format(time.RFC3339),
					DurationMs: stepDurationMs,
					Output:     outMap,
				}
				_ = cfg.store.RecordStepComplete(ctx, cfg.runID, rec)
				_ = cfg.store.CompleteStepCheckpoint(cfg.runID, step.ID, "done", output, ec.FlatStrings(), stepDurationMs)
			}

			// publish_to: if the step has a publish_to topic, publish its output.
			if step.PublishTo != "" {
				if payload, merr := json.Marshal(outMap); merr == nil {
					_ = cfg.publisher.Publish(ctx, step.PublishTo, payload) //nolint:errcheck
				}
			}

			// Evaluate branch condition if present.
			if step.Condition.If != "" {
				condVars := ec.Snapshot()
				condVars["_output"] = output
				if EvalCondition(step.Condition.If, condVars) {
					if step.Condition.Then != "" {
						queue = append([]string{step.Condition.Then}, filterOut(queue, step.Condition.Else)...)
					}
				} else {
					if step.Condition.Else != "" {
						queue = append([]string{step.Condition.Else}, filterOut(queue, step.Condition.Then)...)
					}
				}
			}
		}
	}

	return lastExecutorOutput, nil
}

// runDAG executes a pipeline using the DAG execution engine with full parallelism,
// retry, on_failure routing, and for_each expansion.
func runDAG(ctx context.Context, p *Pipeline, mgr *executor.Manager, userInput string, cfg *runConfig) (string, error) {
	// Wrap ctx with a cancel so a failing step (no on_failure) can stop all
	// in-flight goroutines immediately rather than waiting for them to drain.
	ctx, cancelPipeline := context.WithCancel(ctx)
	defer cancelPipeline()

	maxParallel := p.MaxParallel
	if maxParallel <= 0 {
		maxParallel = 8
	}

	// Expand for_each steps before DAG construction.
	steps, err := expandForEachSteps(p.Steps, userInput, p.Vars)
	if err != nil {
		return "", fmt.Errorf("pipeline: for_each expansion: %w", err)
	}

	// Build DAG: dependents[id] = list of step IDs that need id to complete.
	dependents, err := buildDAG(steps)
	if err != nil {
		return "", fmt.Errorf("pipeline: %w", err)
	}

	// Set up shared execution context.
	ec := NewExecutionContext(WithStore(cfg.store))
	if cfg.injector != nil {
		ec.SetBrainInjector(cfg.injector, cfg.runID)
	}
	if cfg.ragInjector != nil {
		ec.SetRAGInjector(cfg.ragInjector)
	}
	// Brain is always on: when a BrainInjector is provided, every step will receive brain context.
	// Expose the process working directory so pipeline steps can use {{cwd}}.
	if cwd, err := os.Getwd(); err == nil {
		ec.Set("cwd", cwd)
	}
	for k, v := range p.Vars {
		ec.Set("param."+k, v)
	}
	ec.Set("param.input", userInput)

	// Collect all on_failure target IDs. These steps are held back from the
	// initial execution queue and only enqueued when their trigger step fails.
	onFailureTargets := make(map[string]struct{})
	for _, s := range steps {
		if s.OnFailure != "" {
			onFailureTargets[s.OnFailure] = struct{}{}
		}
	}

	// Index steps and initialize state.
	byID := make(map[string]*Step, len(steps))
	stepIndexByID := make(map[string]int, len(steps))
	states := make(map[string]*stepState, len(steps))
	for i := range steps {
		s := &steps[i]
		byID[s.ID] = s
		stepIndexByID[s.ID] = i
		st := &stepState{status: statusWaiting}
		st.pendingDeps.Store(int32(len(s.Needs)))
		states[s.ID] = st
	}

	// Handle input/output steps synchronously before the DAG runs.
	for _, s := range steps {
		switch s.Type {
		case "input":
			ec.Set(s.ID+".out", userInput)
			ec.Set("step."+s.ID+".data.value", userInput)
			ec.Set("step."+s.ID+".state", "done")
			st := states[s.ID]
			st.mu.Lock()
			st.status = statusDone
			st.output = map[string]any{"value": userInput}
			st.mu.Unlock()
			// Unblock dependents.
			for _, dep := range dependents[s.ID] {
				states[dep].pendingDeps.Add(-1)
			}
		case "output":
			st := states[s.ID]
			st.mu.Lock()
			st.status = statusSkipped
			st.mu.Unlock()
		}
	}

	// Identify all executable steps (non-input, non-output, non-on_failure-target).
	// on_failure targets start as skipped and are only activated when triggered.
	var normalSteps []string
	for _, s := range steps {
		if s.Type == "input" || s.Type == "output" {
			continue
		}
		if _, isOF := onFailureTargets[s.ID]; isOF {
			// Mark as skipped initially.
			st := states[s.ID]
			st.mu.Lock()
			st.status = statusSkipped
			st.mu.Unlock()
			continue
		}
		normalSteps = append(normalSteps, s.ID)
	}

	if len(normalSteps) == 0 {
		return "", nil
	}

	semaphore := make(chan struct{}, maxParallel)
	// Buffer the completion channel. Size = all steps (normal + potential on_failure triggers).
	bufSize := len(steps) + 16
	completedCh := make(chan stepResult, bufSize)
	readyCh := make(chan string, bufSize)

	var wg sync.WaitGroup
	var lastOutput string
	var lastOutputMu sync.Mutex
	var firstStepErr error
	var firstStepErrMu sync.Mutex

	// pendingFailures holds errors from steps that have an on_failure handler.
	// If the handler succeeds the entry is deleted; if it fails (or there is no
	// handler) the error is promoted to firstStepErr.
	pendingFailures := make(map[string]error) // failed-step-ID → error
	// onFailureFor maps a recovery step ID back to the step that triggered it.
	onFailureFor := make(map[string]string) // recovery-step-ID → failed-step-ID

	// quit signals the dispatcher to stop.
	quit := make(chan struct{})
	var quitOnce sync.Once

	stopDispatcher := func() {
		quitOnce.Do(func() { close(quit) })
	}

	// launchStep acquires a semaphore slot and runs a step in a goroutine.
	// If the step is a lazy for_each placeholder (Args["_lazy_foreach"] is set),
	// it evaluates the for_each expression against the live EC, runs each item
	// clone sequentially via dispatchStep, and sends a single synthetic completion.
	launchStep := func(id string) {
		st := states[id]
		step := byID[id]

		st.mu.Lock()
		if st.status != statusWaiting {
			st.mu.Unlock()
			return
		}
		st.status = statusRunning
		st.mu.Unlock()

		semaphore <- struct{}{}
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer func() { <-semaphore }()

			snap := ec.Snapshot()
			args := interpolateArgs(step.Args, snap)

			// Lazy for_each: expand at runtime using the live execution context.
			if tmplVal, ok := args["_lazy_foreach"]; ok {
				tmpl := fmt.Sprint(tmplVal)
				resolved := Interpolate(tmpl, snap)
				items := expandForEach(resolved)

				stepStart := time.Now()
				var combinedBuf strings.Builder
				var lastErr error

				for i, item := range items {
					if ctx.Err() != nil {
						lastErr = ctx.Err()
						break
					}
					cloneID := fmt.Sprintf("%s[%d]", id, i)
					clone := cloneStep(*step)
					clone.ID = cloneID
					if clone.Args == nil {
						clone.Args = make(map[string]any)
					}
					delete(clone.Args, "_lazy_foreach")
					clone.Args["_item"] = item
					clone.Args["item"] = item

					cloneSnap := ec.Snapshot()
					cloneSnap["item"] = item
					cloneArgs := map[string]any{"_item": item, "item": item}

					fmt.Printf(StepStatusLineFormat+"\n", cloneID, "running")
					cloneOut, cloneErr := dispatchStep(ctx, &clone, cloneArgs, cloneSnap, ec, mgr, cfg.runID, cfg.pipeName, cfg.publisher, p, cfg.store)
					if cloneErr != nil {
						lastErr = cloneErr
						fmt.Printf(StepStatusLineFormat+"\n", cloneID, "failed")
						fmt.Printf("  error: %v\n", cloneErr)
						break
					}
					fmt.Printf(StepStatusLineFormat+"\n", cloneID, "done")

					// Aggregate clone output under the parent placeholder ID.
					if cloneOut != nil {
						if v, ok := cloneOut["value"]; ok {
							valStr := fmt.Sprint(v)
							if combinedBuf.Len() > 0 {
								combinedBuf.WriteString("\n\n---\n\n")
							}
							combinedBuf.WriteString(valStr)
							parentKey := "step." + id + ".data.value"
							ec.Set(parentKey, combinedBuf.String())
						}
					}
				}

				combinedVal := combinedBuf.String()
				var syntheticOut map[string]any
				if lastErr == nil {
					syntheticOut = map[string]any{"value": combinedVal}
					lastOutputMu.Lock()
					lastOutput = combinedVal
					lastOutputMu.Unlock()
				}
				durationMs := time.Since(stepStart).Milliseconds()
				completedCh <- stepResult{id: id, output: syntheticOut, err: lastErr, startedAt: stepStart, durationMs: durationMs}
				return
			}

			if itemVal, ok := args["_item"]; ok {
				args["item"] = itemVal
				// Inject item into snap so {{item}} resolves in step.Vars interpolation.
				snap["item"] = itemVal
			}

			// Resolve the configured prompt for inbox display (pre-brain-injection).
			rawPrompt := Interpolate(step.Prompt+step.Input, snap)

			// Write-ahead checkpoint before executing.
			if cfg.store != nil {
				_ = cfg.store.StartStepCheckpoint(cfg.runID, step.ID, stepIndexByID[step.ID], rawPrompt, step.Model)
			}

			stepStart := time.Now()
			out, execErr := dispatchStep(ctx, step, args, snap, ec, mgr, cfg.runID, cfg.pipeName, cfg.publisher, p, cfg.store)
			stepDurationMs := time.Since(stepStart).Milliseconds()

			if execErr == nil {
				if out != nil {
					lastOutputMu.Lock()
					if v, ok := out["value"]; ok {
						lastOutput = fmt.Sprint(v)
					}
					lastOutputMu.Unlock()
				}
			}

			completedCh <- stepResult{id: id, output: out, err: execErr, startedAt: stepStart, durationMs: stepDurationMs, prompt: rawPrompt}
		}()
	}

	// Dispatcher goroutine: reads step IDs from readyCh and launches them.
	go func() {
		for {
			select {
			case id, ok := <-readyCh:
				if !ok {
					return
				}
				launchStep(id)
			case <-quit:
				return
			case <-ctx.Done():
				return
			}
		}
	}()

	// Enqueue all initially-ready normal steps (0 pending deps, not on_failure targets).
	for _, id := range normalSteps {
		step := byID[id]
		if len(step.Needs) == 0 {
			readyCh <- id
		} else {
			// Check if all deps are already done (e.g. input steps completed above).
			st := states[id]
			n := st.pendingDeps.Load()
			if n <= 0 {
				readyCh <- id
			}
		}
	}

	// Main drain loop.
	// expected tracks how many completions we need before the pipeline is done.
	// It starts as the count of normalSteps and grows if on_failure steps are triggered.
	expected := len(normalSteps)
	completed := 0

	for completed < expected {
		select {
		case res := <-completedCh:
			completed++

			// Skipped completions are synthetic — just count them.
			if res.skipped {
				continue
			}

			st := states[res.id]
			step := byID[res.id]

			if res.err != nil {
				st.mu.Lock()
				st.status = statusFailed
				st.mu.Unlock()
				ec.Set("step."+res.id+".state", "failed")

				stepErr := fmt.Errorf("pipeline: step %q: %w", res.id, res.err)

				// Record step failure in store.
				if cfg.store != nil {
					rec := store.StepRecord{
						ID:         res.id,
						Status:     "failed",
						Model:      step.Model,
						Prompt:     res.prompt,
						StartedAt:  res.startedAt.Format(time.RFC3339),
						FinishedAt: time.Now().Format(time.RFC3339),
						DurationMs: res.durationMs,
					}
					_ = cfg.store.RecordStepComplete(ctx, cfg.runID, rec)
					_ = cfg.store.CompleteStepCheckpoint(cfg.runID, res.id, "failed", "", nil, res.durationMs)
				}

				// Skip transitive dependents. Send synthetic completions for each
				// since they were counted in normalSteps/expected.
				newlySkipped := skipTransitive(res.id, dependents, states, ec)
				for _, sid := range newlySkipped {
					completedCh <- stepResult{id: sid, skipped: true}
				}

				// Enqueue on_failure step if present.
				if step.OnFailure != "" {
					ofID := step.OnFailure
					if ofSt, ok := states[ofID]; ok {
						ofSt.mu.Lock()
						// Reset from skipped to waiting (on_failure targets start skipped).
						if ofSt.status == statusSkipped {
							ofSt.status = statusWaiting
							ofSt.mu.Unlock()
							// Hold this error pending recovery — promoted if handler fails.
							firstStepErrMu.Lock()
							pendingFailures[res.id] = stepErr
							onFailureFor[ofID] = res.id
							firstStepErrMu.Unlock()
							// Add to expected since it's a new execution.
							expected++
							readyCh <- ofID
						} else {
							ofSt.mu.Unlock()
							// Handler already running/done — treat as unhandled.
							firstStepErrMu.Lock()
							if firstStepErr == nil {
								firstStepErr = stepErr
							}
							firstStepErrMu.Unlock()
						}
					}
				} else {
					// No on_failure handler — propagate immediately and cancel
					// the pipeline context so all in-flight steps are stopped.
					firstStepErrMu.Lock()
					if firstStepErr == nil {
						firstStepErr = stepErr
					}
					firstStepErrMu.Unlock()
					cancelPipeline()
				}
			} else {
				st.mu.Lock()
				st.status = statusDone
				st.output = res.output
				st.mu.Unlock()

				// Record step completion in store.
				if cfg.store != nil {
					rec := store.StepRecord{
						ID:         res.id,
						Status:     "done",
						Model:      step.Model,
						Prompt:     res.prompt,
						StartedAt:  res.startedAt.Format(time.RFC3339),
						FinishedAt: time.Now().Format(time.RFC3339),
						DurationMs: res.durationMs,
						Output:     res.output,
					}
					_ = cfg.store.RecordStepComplete(ctx, cfg.runID, rec)
					outStr := ""
					if res.output != nil {
						if v, ok := res.output["value"]; ok {
							outStr = fmt.Sprint(v)
						}
					}
					_ = cfg.store.CompleteStepCheckpoint(cfg.runID, res.id, "done", outStr, ec.FlatStrings(), res.durationMs)
				}

				if res.output != nil {
					ec.Set("step."+res.id+".data", res.output)
					ec.Set("step."+res.id+".state", "done")
					if v, ok := res.output["value"]; ok {
						ec.Set(res.id+".out", fmt.Sprint(v))
					}
					// Aggregate for_each clone output under the parent step ID so
					// templates can reference {{step.<orig>.data.value}} directly.
					if bracketIdx := strings.LastIndex(res.id, "["); bracketIdx != -1 && strings.HasSuffix(res.id, "]") {
						parentID := res.id[:bracketIdx]
						if v, ok := res.output["value"]; ok {
							parentKey := "step." + parentID + ".data.value"
							if existing, ok := ec.Get(parentKey); ok {
								ec.Set(parentKey, fmt.Sprint(existing)+"\n\n---\n\n"+fmt.Sprint(v))
							} else {
								ec.Set(parentKey, fmt.Sprint(v))
							}
						}
					}
					// Store declared step outputs.
					if step := byID[res.id]; step != nil {
						if v, ok := res.output["value"]; ok {
							outStr := fmt.Sprint(v)
							for key := range step.Outputs {
								ec.SetStepOutput(res.id, key, outStr)
							}
						}
					}
				} else {
					ec.Set("step."+res.id+".state", "done")
				}

				// If this step was an on_failure recovery step and it succeeded,
				// clear the pending failure for the original step.
				firstStepErrMu.Lock()
				if originID, ok := onFailureFor[res.id]; ok {
					delete(pendingFailures, originID)
					delete(onFailureFor, res.id)
				}
				firstStepErrMu.Unlock()

				// Unblock dependents.
				for _, dep := range dependents[res.id] {
					depSt := states[dep]
					if n := depSt.pendingDeps.Add(-1); n == 0 {
						depSt.mu.Lock()
						depStatus := depSt.status
						depSt.mu.Unlock()
						if depStatus == statusWaiting {
							readyCh <- dep
						}
					}
				}
			}

		case <-ctx.Done():
			stopDispatcher()
			wg.Wait()
			return "", ctx.Err()
		}
	}

	stopDispatcher()
	wg.Wait()

	// Promote any pending failures whose recovery step never succeeded.
	if firstStepErr == nil {
		for _, err := range pendingFailures {
			firstStepErr = err
			break
		}
	}

	return lastOutput, firstStepErr
}

// skipTransitive marks all transitive dependents of failedID as skipped.
// It returns the list of step IDs that were newly skipped (were in statusWaiting before).
// Callers are responsible for updating expected and sending synthetic completions.
func skipTransitive(failedID string, dependents map[string][]string, states map[string]*stepState, ec *ExecutionContext) []string {
	var newly []string
	queue := []string{failedID}
	visited := map[string]bool{failedID: true}
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		for _, dep := range dependents[cur] {
			if visited[dep] {
				continue
			}
			visited[dep] = true
			st := states[dep]
			st.mu.Lock()
			wasWaiting := st.status == statusWaiting
			if wasWaiting {
				st.status = statusSkipped
				ec.Set("step."+dep+".state", "skipped")
				newly = append(newly, dep)
			}
			st.mu.Unlock()
			queue = append(queue, dep)
		}
	}
	return newly
}

// expandForEachSteps replaces steps with ForEach set by N cloned steps.
// Each clone gets the ID "<orig>[i]" and an "_item" arg set to the item value.
//
// If the ForEach expression contains a "{{step." reference (i.e. depends on a
// runtime step output), expansion is deferred to runtime: the step is kept as a
// single placeholder with Args["_lazy_foreach"] set to the original expression.
// The DAG treats the placeholder as a normal node; when it becomes ready the
// runDAG loop expands it lazily against the live ExecutionContext.
func expandForEachSteps(steps []Step, userInput string, pipeVars map[string]any) ([]Step, error) {
	// Build a minimal vars map for interpolating ForEach expressions.
	vars := make(map[string]any)
	for k, v := range pipeVars {
		vars["param."+k] = v
	}
	vars["param.input"] = userInput

	// First pass: compute expansions so we can rewrite Needs refs.
	// Steps with runtime-deferred for_each are NOT expanded here — they remain
	// as single placeholder nodes so downstream Needs refs stay intact.
	origForEach := make(map[string][]string) // origID → expanded IDs
	for _, s := range steps {
		if s.ForEach != "" {
			resolved := Interpolate(s.ForEach, vars)
			if strings.Contains(s.ForEach, "{{step.") {
				// Deferred — will be expanded at runtime; keep original ID as a
				// single placeholder so Needs rewriting below leaves it alone.
				origForEach[s.ID] = []string{s.ID}
				continue
			}
			items := expandForEach(resolved)
			ids := make([]string, len(items))
			for i := range items {
				ids[i] = fmt.Sprintf("%s[%d]", s.ID, i)
			}
			origForEach[s.ID] = ids
		}
	}

	out := make([]Step, 0, len(steps))
	for _, s := range steps {
		if s.ForEach == "" {
			// Rewrite Needs that reference for_each origins.
			newNeeds := rewriteNeeds(s.Needs, origForEach)
			s.Needs = newNeeds
			out = append(out, s)
			continue
		}
		// Deferred for_each: keep as a placeholder and let the runner expand at runtime.
		if strings.Contains(s.ForEach, "{{step.") {
			placeholder := cloneStep(s)
			placeholder.ForEach = ""
			if placeholder.Args == nil {
				placeholder.Args = make(map[string]any)
			}
			placeholder.Args["_lazy_foreach"] = s.ForEach
			placeholder.Needs = rewriteNeeds(s.Needs, origForEach)
			out = append(out, placeholder)
			continue
		}
		// Static expansion into N clones.
		resolved := Interpolate(s.ForEach, vars)
		items := expandForEach(resolved)
		if len(items) == 0 {
			continue
		}
		for i, item := range items {
			clone := cloneStep(s)
			clone.ID = fmt.Sprintf("%s[%d]", s.ID, i)
			clone.ForEach = ""
			if clone.Args == nil {
				clone.Args = make(map[string]any)
			}
			clone.Args["_item"] = item
			clone.Needs = rewriteNeeds(s.Needs, origForEach)
			out = append(out, clone)
		}
	}

	return out, nil
}

// rewriteNeeds rewrites a Needs list, expanding any for_each origin IDs to their expansions.
func rewriteNeeds(needs []string, origForEach map[string][]string) []string {
	if len(needs) == 0 {
		return needs
	}
	out := make([]string, 0, len(needs))
	for _, need := range needs {
		if expansions, ok := origForEach[need]; ok {
			out = append(out, expansions...)
		} else {
			out = append(out, need)
		}
	}
	return out
}

// cloneStep makes a shallow copy of a Step.
func cloneStep(s Step) Step {
	clone := s
	if s.Args != nil {
		clone.Args = make(map[string]any, len(s.Args))
		for k, v := range s.Args {
			clone.Args[k] = v
		}
	}
	if s.Needs != nil {
		clone.Needs = append([]string(nil), s.Needs...)
	}
	if s.Vars != nil {
		clone.Vars = make(map[string]string, len(s.Vars))
		for k, v := range s.Vars {
			clone.Vars[k] = v
		}
	}
	return clone
}

// interpolateArgs walks args and interpolates any string values using vars.
func interpolateArgs(args map[string]any, vars map[string]any) map[string]any {
	if args == nil {
		return make(map[string]any)
	}
	out := make(map[string]any, len(args))
	for k, v := range args {
		switch tv := v.(type) {
		case string:
			out[k] = Interpolate(tv, vars)
		case map[string]any:
			out[k] = interpolateArgs(tv, vars)
		default:
			out[k] = v
		}
	}
	return out
}

// dispatchStep determines the executor for a step and runs it (with retries).
// runID and pipeName are used to populate event payloads; pub receives the events.
func dispatchStep(ctx context.Context, step *Step, args map[string]any, snap map[string]any, ec *ExecutionContext, mgr *executor.Manager, runID int64, pipeName string, pub EventPublisher, p *Pipeline, st *store.Store) (map[string]any, error) {
	executor, err := resolveExecutor(ctx, step, args, snap, ec, mgr, p, st)
	if err != nil {
		return nil, err
	}

	if err := executor.Init(ctx); err != nil {
		return nil, fmt.Errorf("init: %w", err)
	}

	var out map[string]any
	var execErr error

	maxAttempts := 1
	var interval time.Duration
	retryOn := "always"
	if step.Retry != nil {
		if step.Retry.MaxAttempts > 0 {
			maxAttempts = step.Retry.MaxAttempts
		}
		interval = step.Retry.Interval.Duration
		if step.Retry.On != "" {
			retryOn = step.Retry.On
		}
	}

	// Publish step.started before the retry loop.
	if payload, merr := json.Marshal(map[string]any{
		"run_id":   runID,
		"pipeline": pipeName,
		"step":     step.ID,
		"status":   "running",
	}); merr == nil {
		_ = pub.Publish(ctx, topics.StepStarted, payload)
	}

	fmt.Printf(StepStatusLineFormat+"\n", step.ID, "running")

	stepStart := time.Now()
	for attempt := 0; attempt < maxAttempts; attempt++ {
		out, execErr = executor.Execute(ctx, args)
		if execErr == nil {
			break
		}
		if !conditionMatchesError(retryOn, execErr) {
			break
		}
		if attempt < maxAttempts-1 {
			select {
			case <-time.After(interval):
			case <-ctx.Done():
				execErr = ctx.Err()
				goto done
			}
		}
	}

	// Clarification: if the executor output contains GLITCH_CLARIFY: and the executor
	// type is reactive, publish a ClarificationRequest via busd and block until the
	// user answers via the TUI. Then re-run with the full conversation context.
	if execErr == nil {
		typeName := step.Executor
		if typeName == "" {
			typeName = step.Type
		}
		if clarify.IsReactive(typeName) {
			valueStr, _ := extractOutputValue(out)
			detector := clarify.Get(typeName)
			for _, line := range strings.Split(valueStr, "\n") {
				if q, found := detector.Detect(line); found {
					// Mark the step as paused waiting for clarification.
					if st != nil {
						_ = st.PauseStepCheckpoint(runID, step.ID)
					}
					_ = activity.AppendEvent(activity.DefaultPath(), activity.ActivityEvent{
						TS:     time.Now().Format(time.RFC3339),
						Kind:   "pipeline_paused",
						Agent:  step.ID,
						Label:  "waiting for reply",
						Status: "paused",
					})
					answer, cerr := AskClarification(ctx, strconv.FormatInt(runID, 10), step.ID, q, valueStr)
					if cerr == nil && answer != "" {
						_ = activity.AppendEvent(activity.DefaultPath(), activity.ActivityEvent{
							TS:     time.Now().Format(time.RFC3339),
							Kind:   "pipeline_running",
							Agent:  step.ID,
							Label:  "clarification answered — resuming",
							Status: "running",
						})
						// Build follow-up: assistant response + user answer, then re-run.
						followUp := BuildClarificationFollowUp(valueStr, answer)
						if pl, ok := mgr.Get(typeName); ok {
							stepVars := ec.FlatStrings()
							stepVars["model"] = step.Model
							for k, v := range step.Vars {
								stepVars[k] = Interpolate(v, snap)
							}
							fe := newRegisteredExecutor(pl, followUp, stepVars, nil)
							if err2 := fe.Init(ctx); err2 == nil {
								out, execErr = fe.Execute(ctx, args)
							}
						}
					}
					break
				}
			}
		}
	}

done:
	stepDurationMs := time.Since(stepStart).Milliseconds()
	cleanupErr := executor.Cleanup(ctx)
	// Parse <brain> blocks from any output (including partial on failure).
	// Best-effort: called regardless of step success/failure.
	// Block-level opt-in: any step output containing a <brain> block is stored
	// when a store is available — the agent decides what's worth remembering.
	if ec.DB() != nil {
		if valueStr, ok := extractOutputValue(out); ok && valueStr != "" {
			parseBrainBlock(ctx, valueStr, step.ID, ec)
		}
	}

	if execErr != nil {
		// Wrap execErr with any captured output so the failure reason is visible.
		if out != nil {
			if v, ok := out["value"]; ok {
				if s := fmt.Sprint(v); s != "" {
					execErr = fmt.Errorf("%w\noutput: %s", execErr, s)
				}
			}
		}
		fmt.Printf(StepStatusLineFormat+"\n", step.ID, "failed")
		fmt.Printf("  error: %v\n", execErr)
		// Publish step.failed.
		if payload, merr := json.Marshal(map[string]any{
			"run_id":      runID,
			"pipeline":    pipeName,
			"step":        step.ID,
			"status":      "failed",
			"duration_ms": stepDurationMs,
			"error":       execErr.Error(),
		}); merr == nil {
			_ = pub.Publish(ctx, topics.StepFailed, payload)
		}
		return nil, execErr
	}
	if cleanupErr != nil {
		fmt.Printf(StepStatusLineFormat+"\n", step.ID, "failed")
		// Publish step.failed for cleanup error too.
		if payload, merr := json.Marshal(map[string]any{
			"run_id":      runID,
			"pipeline":    pipeName,
			"step":        step.ID,
			"status":      "failed",
			"duration_ms": stepDurationMs,
		}); merr == nil {
			_ = pub.Publish(ctx, topics.StepFailed, payload)
		}
		return nil, cleanupErr
	}
	fmt.Printf(StepStatusLineFormat+"\n", step.ID, "done")

	// Publish step.done.
	if payload, merr := json.Marshal(map[string]any{
		"run_id":      runID,
		"pipeline":    pipeName,
		"step":        step.ID,
		"status":      "done",
		"duration_ms": stepDurationMs,
		"output":      out,
	}); merr == nil {
		_ = pub.Publish(ctx, topics.StepDone, payload)
	}

	// publish_to: publish step output to a custom topic if configured.
	if step.PublishTo != "" {
		if payload, merr := json.Marshal(out); merr == nil {
			_ = pub.Publish(ctx, step.PublishTo, payload) //nolint:errcheck
		}
	}

	return out, nil
}

// resolveExecutor builds the appropriate StepExecutor for a step.
func resolveExecutor(ctx context.Context, step *Step, args map[string]any, snap map[string]any, ec *ExecutionContext, mgr *executor.Manager, p *Pipeline, st *store.Store) (StepExecutor, error) {
	// Determine the executor name: Executor takes precedence over Type.
	typeName := step.Executor
	if typeName == "" {
		typeName = step.Type
	}

	// Check builtin registry first.
	if fn, ok := builtinRegistry[typeName]; ok {
		return &builtinExecutor{fn: fn, w: nil}, nil
	}

	// Validate: unknown builtin.* prefix returns an error.
	if strings.HasPrefix(typeName, "builtin.") {
		return nil, fmt.Errorf("unknown builtin type %q", typeName)
	}

	// Look up in executor manager.
	pl, ok := mgr.Get(typeName)
	if !ok {
		return nil, fmt.Errorf("executor %q not found", typeName)
	}

	// Build executor input string from prompt/input fields.
	raw := step.Prompt + step.Input
	if raw == "" {
		if v, ok := snap["param.input"]; ok {
			raw = fmt.Sprint(v)
		}
	}
	promptOrInput := Interpolate(raw, snap)

	// Resolve {{ steps.<id>.<key> }} patterns from accumulated step outputs.
	runIDStr := strconv.FormatInt(ec.RunID(), 10)
	resolved, resolveErr := ResolveStepInputs(promptOrInput, ec, step.ID, runIDStr)
	if resolveErr != nil {
		return nil, resolveErr
	}
	promptOrInput = resolved

	// Also resolve declared Inputs map values.
	for _, tmpl := range step.Inputs {
		resolvedInput, inputErr := ResolveStepInputs(tmpl, ec, step.ID, runIDStr)
		if inputErr != nil {
			return nil, inputErr
		}
		_ = resolvedInput // inputs are available via snap but we resolve them for validation
	}

	// Prepend saved prompt body if prompt_id is set.
	if step.PromptID != "" {
		if st != nil {
			saved, err := st.GetPromptByTitle(ctx, step.PromptID)
			if err != nil {
				return nil, fmt.Errorf("pipeline: step %q: prompt %q not found in store", step.ID, step.PromptID)
			}
			promptOrInput = saved.Body + "\n\n" + promptOrInput
		}
	}

	promptOrInput = injectBrainContext(ctx, promptOrInput, p, step, ec)

	// RAG injection: if a BrainRAGInjector is configured, inject relevant brain notes.
	var ragNoteIDs []string
	if ragInj := ec.GetRAGInjector(); ragInj != nil {
		injected, injErr := ragInj.InjectInto(ctx, promptOrInput)
		if injErr != nil {
			fmt.Fprintf(os.Stderr, "[brainrag] warn: RAG inject failed for step %q: %v\n", step.ID, injErr)
		} else {
			promptOrInput = injected
		}
	}

	// Audit log: best-effort, never fail the step.
	go func() {
		_ = brainaudit.Append(brainaudit.AuditEntry{
			RunID:              strconv.FormatInt(ec.RunID(), 10),
			StepName:           step.ID,
			BrainNotesInjected: ragNoteIDs,
			PromptLengthChars:  len(promptOrInput),
		})
	}()

	// Append the GLITCH_CLARIFY instruction for executors that support reactive
	// clarification. Pipelines using unregistered executors are unaffected.
	if clarify.IsReactive(typeName) {
		promptOrInput = strings.TrimRight(promptOrInput, "\n") + clarify.Instruction()
	}

	// Build stepVars for the executor.
	// For tool-kind plugins (kind: tool in sidecar YAML), only pass the
	// interpolated step.Vars plus cwd and model. This prevents large step
	// outputs from being dumped as GLITCH_* env vars into every subprocess.
	// Agent plugins still get the full EC so they have full context.
	var stepVars map[string]string
	type kinder interface{ Kind() string }
	if k, ok := pl.(kinder); ok && k.Kind() == "tool" {
		stepVars = make(map[string]string, len(step.Vars)+3)
		if cwd, ok := ec.Get("cwd"); ok {
			stepVars["cwd"] = fmt.Sprint(cwd)
		}
	} else {
		stepVars = ec.FlatStrings()
	}
	stepVars["model"] = step.Model
	for k, v := range step.Vars {
		stepVars[k] = Interpolate(v, snap)
	}

	return newRegisteredExecutor(pl, promptOrInput, stepVars, nil), nil
}

// builtinExecutor adapts a BuiltinFunc to StepExecutor.
type builtinExecutor struct {
	fn BuiltinFunc
	w  io.Writer
}

func (b *builtinExecutor) Init(_ context.Context) error { return nil }
func (b *builtinExecutor) Execute(ctx context.Context, args map[string]any) (map[string]any, error) {
	return b.fn(ctx, args, b.w)
}
func (b *builtinExecutor) Cleanup(_ context.Context) error { return nil }

// executePluginStep runs a single plugin step and returns its string output.
// This is used by the legacy runner only.
// forceInput, when non-empty, overrides step.Prompt/step.Input completely
// (used when resuming from a clarification answer).
func executePluginStep(ctx context.Context, step *Step, ec *ExecutionContext, mgr *executor.Manager, defaultInput string, p *Pipeline, st *store.Store, runID int64, forceInput string) (string, error) {
	executorName := step.Executor

	pl, ok := mgr.Get(executorName)
	if !ok {
		return "", fmt.Errorf("executor %q not found", executorName)
	}

	snap := ec.Snapshot()

	var raw string
	if forceInput != "" {
		raw = forceInput
	} else {
		raw = step.Prompt + step.Input
		if raw == "" {
			raw = defaultInput
		}
	}
	promptOrInput := Interpolate(raw, snap)

	// Resolve {{ steps.<id>.<key> }} patterns from accumulated step outputs.
	runIDStr := strconv.FormatInt(ec.RunID(), 10)
	resolvedPrompt, resolveErr := ResolveStepInputs(promptOrInput, ec, step.ID, runIDStr)
	if resolveErr != nil {
		return "", resolveErr
	}
	promptOrInput = resolvedPrompt

	// Prepend saved prompt body if prompt_id is set.
	if step.PromptID != "" {
		if st != nil {
			saved, err := st.GetPromptByTitle(ctx, step.PromptID)
			if err != nil {
				return "", fmt.Errorf("pipeline: step %q: prompt %q not found in store", step.ID, step.PromptID)
			}
			promptOrInput = saved.Body + "\n\n" + promptOrInput
		}
	}

	promptOrInput = injectBrainContext(ctx, promptOrInput, p, step, ec)

	// RAG injection: if a BrainRAGInjector is configured, inject relevant brain notes.
	if ragInj := ec.GetRAGInjector(); ragInj != nil {
		injected, injErr := ragInj.InjectInto(ctx, promptOrInput)
		if injErr != nil {
			fmt.Fprintf(os.Stderr, "[brainrag] warn: RAG inject failed for step %q: %v\n", step.ID, injErr)
		} else {
			promptOrInput = injected
		}
	}

	// Audit log: best-effort.
	go func() {
		_ = brainaudit.Append(brainaudit.AuditEntry{
			RunID:              strconv.FormatInt(ec.RunID(), 10),
			StepName:           step.ID,
			BrainNotesInjected: []string{},
			PromptLengthChars:  len(promptOrInput),
		})
	}()

	// Append GLITCH_CLARIFY instruction for reactive executors so the model
	// knows the protocol for requesting user input.
	if clarify.IsReactive(executorName) {
		promptOrInput = strings.TrimRight(promptOrInput, "\n") + clarify.Instruction()
	}

	stepVars := ec.FlatStrings()
	stepVars["model"] = step.Model
	for k, v := range step.Vars {
		stepVars[k] = Interpolate(v, snap)
	}

	var buf bytes.Buffer
	execErr := pl.Execute(ctx, promptOrInput, stepVars, &buf)
	output := buf.String()

	// Clarification: if the executor output contains GLITCH_CLARIFY:, surface it
	// via AskClarification (busd), then re-execute with the full conversation context.
	if execErr == nil && clarify.IsReactive(executorName) {
		detector := clarify.Get(executorName)
		for _, line := range strings.Split(output, "\n") {
			if q, found := detector.Detect(line); found {
				// Mark the step as paused waiting for clarification.
				if st != nil {
					_ = st.PauseStepCheckpoint(runID, step.ID)
				}
				_ = activity.AppendEvent(activity.DefaultPath(), activity.ActivityEvent{
					TS:     time.Now().Format(time.RFC3339),
					Kind:   "pipeline_paused",
					Agent:  step.ID,
					Label:  "waiting for reply",
					Status: "paused",
				})
				answer, cerr := AskClarification(ctx, strconv.FormatInt(runID, 10), step.ID, q, output)
				if cerr == nil && answer != "" {
					_ = activity.AppendEvent(activity.DefaultPath(), activity.ActivityEvent{
						TS:     time.Now().Format(time.RFC3339),
						Kind:   "pipeline_running",
						Agent:  step.ID,
						Label:  "clarification answered — resuming",
						Status: "running",
					})
					followUp := BuildClarificationFollowUp(output, answer)
					var buf2 bytes.Buffer
					execErr = pl.Execute(ctx, followUp, stepVars, &buf2)
					output = buf2.String()
				}
				break
			}
		}
	}

	// Parse <brain> blocks from output on every step when a store is available,
	// even on failure (best-effort from any partial output).
	// Block-level opt-in: the agent decides what's worth remembering.
	if ec.DB() != nil {
		parseBrainBlock(ctx, output, step.ID, ec)
	}
	return output, execErr
}

// extractOutputValue extracts the "value" string from a step output map.
func extractOutputValue(out map[string]any) (string, bool) {
	if out == nil {
		return "", false
	}
	if v, ok := out["value"]; ok {
		if s, ok := v.(string); ok {
			return s, true
		}
	}
	return "", false
}

// filterOut removes a single value from a slice (used by legacy runner).
func filterOut(ss []string, remove string) []string {
	if remove == "" {
		return ss
	}
	out := ss[:0:len(ss)]
	for _, s := range ss {
		if s != remove {
			out = append(out, s)
		}
	}
	return out
}

// BuildClarificationFollowUp builds the conversation context to send for the
// follow-up execution after a clarification exchange. It appends the user's
// answer after the assistant's response so the executor has full context.
func BuildClarificationFollowUp(assistantResponse, userAnswer string) string {
	var sb strings.Builder
	sb.WriteString(assistantResponse)
	sb.WriteString("\n\n---\nUser: ")
	sb.WriteString(userAnswer)
	sb.WriteString("\n\nPlease continue with the task.")
	return sb.String()
}

