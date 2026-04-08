// stepthrough.go is the glitchd-side glue for interactive workflow authoring.
// The actual session state machine lives in internal/pipeline; this file
// constructs sessions, ties them to a workspace's executor manager, and
// exposes a save-as path that writes the original YAML back to disk.
//
// Why the YAML round-trip on save: a step-through session executes the
// pipeline the user gave us. Hand-edits to step outputs are session-level
// metadata, not pipeline structure — they don't change which steps run or
// which prompts get sent. Saving back the original YAML keeps the artifact
// honest: re-running the saved workflow calls the LLM fresh, exactly as the
// user explored it. The provenance of which steps were hand-edited is
// surfaced via the session's Snapshot() so the UI can mark them.
package glitchd

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/8op-org/gl1tch/internal/pipeline"
)

// StepThroughEvent re-exports pipeline.StepThroughEvent so callers in
// glitch-desktop don't need to import internal/pipeline directly.
type StepThroughEvent = pipeline.StepThroughEvent

// StepThroughHandle is the glitchd-side wrapper around a running step-through
// session. It carries either the original YAML (for file-backed sessions) or
// the originating chain JSON + provider defaults (for chain-backed sessions),
// so SaveStepThroughAs can persist the pipeline back to disk through the
// right serializer.
type StepThroughHandle struct {
	ID          string
	WorkspaceID string

	// YAML is set when the session was started from a .workflow.yaml file.
	// SaveStepThroughAs writes this verbatim.
	YAML string

	// ChainJSON + DefaultProvider + DefaultModel are set when the session
	// was started from a desktop chain (StartStepThroughFromChain). At save
	// time the chain is run back through ChainStepsToYAML to produce a
	// clean workflow file — same serializer the chain bar's Save button
	// uses, so the two save paths stay byte-for-byte identical.
	ChainJSON       string
	DefaultProvider string
	DefaultModel    string

	Session *pipeline.StepThroughSession
}

// StartStepThrough parses the given pipeline YAML, builds an executor
// manager, constructs a step-through session, and starts it. The returned
// handle exposes the session's event channel for the desktop app to pump
// into Wails runtime events.
//
// userInput is forwarded to the runner as the seed value for any input step
// — typically the user's free-text prompt from the chat box. Pass "" if the
// pipeline does not declare an input step.
func StartStepThrough(ctx context.Context, workspaceID, yamlContent, userInput string) (*StepThroughHandle, error) {
	if strings.TrimSpace(yamlContent) == "" {
		return nil, errors.New("step-through: yaml content is empty")
	}

	p, err := pipeline.Load(strings.NewReader(yamlContent))
	if err != nil {
		return nil, fmt.Errorf("step-through: parse yaml: %w", err)
	}

	mgr := buildManager()
	sess := pipeline.NewStepThroughSession(p, mgr)

	if err := sess.Start(ctx, userInput); err != nil {
		return nil, fmt.Errorf("step-through: start: %w", err)
	}

	return &StepThroughHandle{
		ID:          sess.ID,
		WorkspaceID: workspaceID,
		YAML:        yamlContent,
		Session:     sess,
	}, nil
}

// StepThroughChainOpts configures a step-through session started directly
// from a desktop chain (rather than a YAML string on disk). This is the
// canonical path the desktop app takes when a multi-step chain is sent —
// it parallels RunChainOpts so callers already familiar with the chain
// runner can reuse the same wiring.
//
// TokenCh / EventCh are optional and, when supplied, receive the per-step
// stdout stream and structured BlockEvents respectively. The caller owns
// their lifetime — this function does NOT close them. A typical desktop
// caller pumps TokenCh into `chat:chunk` and EventCh into `chat:event`,
// then closes both when the session's Events channel closes.
type StepThroughChainOpts struct {
	WorkspaceID     string
	StepsJSON       string
	UserText        string
	DefaultProvider string
	DefaultModel    string
	Cwd             string
	TokenCh         chan<- string
	EventCh         chan<- BlockEvent
}

// StartStepThroughFromChain builds a pipeline from a desktop chain (same
// expansion rules as RunChain) and starts it as a step-through session.
//
// Why this exists: the YAML-based StartStepThrough is fine for running a
// .workflow.yaml file, but the chat bar produces a ChainStep[] that carries
// runtime concerns (cwd, per-run provider overrides) which we don't want
// to bake into saved YAML. Going through buildPipelineFromChain directly
// gets us cwd injection and the same execution semantics as RunChain
// without round-tripping through the on-disk workflow format.
//
// The returned handle has YAML == "" — save-as from a chain-started
// session is deferred to a later phase (see project_step_through_mode).
// Phase 1 exposes Accept/Abort only; editing, rewind, and save-as flow
// into later phases.
func StartStepThroughFromChain(ctx context.Context, opts StepThroughChainOpts) (*StepThroughHandle, error) {
	var steps []ChainStep
	if err := json.Unmarshal([]byte(opts.StepsJSON), &steps); err != nil {
		return nil, fmt.Errorf("step-through: parse steps: %w", err)
	}
	if strings.TrimSpace(opts.UserText) != "" {
		steps = append(steps, ChainStep{
			Type:  "prompt",
			Label: "user",
			Body:  opts.UserText,
		})
	}
	if len(steps) == 0 {
		return nil, errors.New("step-through: no steps to run")
	}

	p, err := buildPipelineFromChain(steps, RunChainOpts{
		StepsJSON:       opts.StepsJSON,
		UserText:        opts.UserText,
		WorkspaceID:     opts.WorkspaceID,
		DefaultProvider: opts.DefaultProvider,
		DefaultModel:    opts.DefaultModel,
		Cwd:             opts.Cwd,
	})
	if err != nil {
		return nil, err
	}

	mgr := buildManager()
	sess := pipeline.NewStepThroughSession(p, mgr)

	// Streaming writer — fans out to the desktop chat:chunk and chat:event
	// topics. Reuses the same chainStreamWriter that RunChain uses so token
	// framing and BlockEvent splitting behave identically across both paths.
	var runOpts []pipeline.RunOption
	if opts.TokenCh != nil || opts.EventCh != nil {
		w := newChainStreamWriter(ctx, opts.TokenCh, opts.EventCh)
		runOpts = append(runOpts, pipeline.WithStepWriter(w))
	}
	if opts.WorkspaceID != "" {
		runOpts = append(runOpts, pipeline.WithWorkspaceID(opts.WorkspaceID))
	}

	if err := sess.Start(ctx, opts.UserText, runOpts...); err != nil {
		return nil, fmt.Errorf("step-through: start: %w", err)
	}

	return &StepThroughHandle{
		ID:              sess.ID,
		WorkspaceID:     opts.WorkspaceID,
		ChainJSON:       opts.StepsJSON,
		DefaultProvider: opts.DefaultProvider,
		DefaultModel:    opts.DefaultModel,
		Session:         sess,
	}, nil
}

// SaveStepThroughAs writes the handle's original YAML to the workspace's
// .glitch/workflows/<name>.workflow.yaml file. Returns the saved path.
//
// The router's discover-and-embed path picks up the new file automatically
// on the next ask call: cmd/ask.go re-runs DiscoverPipelines() each time,
// and the embedding cache fingerprints by SHA-256 of name+description+
// trigger_phrases, so a new workflow is embedded the first time it appears.
// No explicit reindex call is needed here.
func SaveStepThroughAs(ctx context.Context, h *StepThroughHandle, name string) (string, error) {
	if h == nil {
		return "", errors.New("step-through: nil handle")
	}
	if strings.TrimSpace(h.WorkspaceID) == "" {
		return "", errors.New("step-through: handle has no workspace_id")
	}
	if strings.TrimSpace(name) == "" {
		return "", errors.New("step-through: save name is required")
	}

	// Resolve the YAML to write. File-backed sessions use their original
	// YAML verbatim. Chain-backed sessions re-run the chain → YAML
	// serializer so the saved workflow lines up byte-for-byte with what
	// the chain bar's Save button would have written — no divergence.
	yamlToWrite := h.YAML
	if strings.TrimSpace(yamlToWrite) == "" && strings.TrimSpace(h.ChainJSON) != "" {
		rendered, err := ChainStepsToYAML(h.ChainJSON, name, "", h.DefaultProvider, h.DefaultModel)
		if err != nil {
			return "", fmt.Errorf("step-through: render chain yaml: %w", err)
		}
		yamlToWrite = rendered
	}
	if strings.TrimSpace(yamlToWrite) == "" {
		return "", errors.New("step-through: session has nothing to save")
	}

	st, err := OpenStore()
	if err != nil {
		return "", fmt.Errorf("step-through: open store: %w", err)
	}
	dir := primaryWorkspaceDir(ctx, st, h.WorkspaceID)
	if dir == "" {
		return "", errWorkspaceHasNoDirs
	}

	return SaveWorkflow(dir, name, yamlToWrite)
}
