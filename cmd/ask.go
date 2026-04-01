package cmd

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/8op-org/gl1tch/internal/cron"
	"github.com/8op-org/gl1tch/internal/executor"
	"github.com/8op-org/gl1tch/internal/picker"
	"github.com/8op-org/gl1tch/internal/pipeline"
	"github.com/8op-org/gl1tch/internal/store"
)

// localProviderIDs are providers that run on the local machine — no remote API calls.
var localProviderIDs = map[string]bool{
	"ollama": true,
	"shell":  true,
}

var (
	askProvider        string
	askModel           string
	askPipelineFlag    string
	askInputVars       []string
	askBrain           bool
	askWriteBrain      bool
	askSynthesize      bool
	askSynthesizeModel string
	askJSON            bool
	askAuto            bool
	askDryRun          bool
	askRoute           bool
)

func init() {
	rootCmd.AddCommand(askCmd)
	askCmd.Flags().StringVarP(&askProvider, "provider", "p", "", "provider ID (e.g. ollama, claude); defaults to first available local provider")
	askCmd.Flags().StringVarP(&askModel, "model", "m", "", "model name (e.g. llama3.2, mistral)")
	askCmd.Flags().StringVar(&askPipelineFlag, "pipeline", "", "run a named pipeline or file path instead of a raw prompt")
	askCmd.Flags().StringArrayVar(&askInputVars, "input", nil, "input vars as key=value (repeatable)")
	askCmd.Flags().BoolVar(&askBrain, "brain", true, "inject brain context from store")
	askCmd.Flags().BoolVar(&askWriteBrain, "write-brain", false, "write response back to brain store")
	askCmd.Flags().BoolVar(&askSynthesize, "synthesize", false, "pass response through claude to clean up without adding new information")
	askCmd.Flags().StringVar(&askSynthesizeModel, "synthesize-model", "", "model to use for synthesis (defaults to claude provider default)")
	askCmd.Flags().BoolVar(&askJSON, "json", false, "output response as JSON envelope")
	askCmd.Flags().BoolVarP(&askAuto, "auto", "y", false, "skip confirmation for generated pipelines")
	askCmd.Flags().BoolVar(&askDryRun, "dry-run", false, "show what would run without executing")
	askCmd.Flags().BoolVar(&askRoute, "route", true, "route prompt through intent classifier to find a matching pipeline")
}

var askCmd = &cobra.Command{
	Use:   "ask [prompt]",
	Short: "Ask glitch a question — routes to matching pipelines automatically",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		// ── explicit pipeline flag: delegate entirely ─────────────────────────────
		if askPipelineFlag != "" {
			return runAskPipeline(cmd, args)
		}

		if len(args) == 0 {
			return fmt.Errorf("a prompt is required (or use --pipeline to run a named pipeline)")
		}
		prompt := args[0]

		inputVars, err := parseInputVars(askInputVars)
		if err != nil {
			return err
		}

		mgr, providerID, resolvedModel, err := buildAskManager(askProvider, askModel)
		if err != nil {
			return err
		}

		if askSynthesize {
			if _, ok := mgr.Get("claude"); !ok {
				return fmt.Errorf("--synthesize requires a claude provider; none found")
			}
		}

		// ── intent routing ────────────────────────────────────────────────────────
		if askRoute {
			configDir, _ := glitchConfigDir()
			if configDir != "" {
				pipelinesDir := filepath.Join(configDir, "pipelines")
				refs, _ := pipeline.DiscoverPipelines(pipelinesDir)

				if len(refs) > 0 {
					matched, _ := routeIntent(cmd.Context(), prompt, refs, mgr, resolvedModel)
					if matched != nil {
						return dispatchMatched(cmd, prompt, matched, inputVars)
					}
					// No match — try to generate a pipeline on the fly.
					return dispatchGenerated(cmd, prompt, mgr, providerID, resolvedModel, inputVars)
				}
			}
		}

		// ── one-shot fallback ─────────────────────────────────────────────────────
		return runOneShot(cmd, prompt, providerID, resolvedModel, mgr, inputVars)
	},
}

// routeIntent classifies the user's prompt against known pipelines using the local model.
// Returns the matching PipelineRef, or nil if no match (NONE or garbage response).
func routeIntent(ctx context.Context, prompt string, refs []pipeline.PipelineRef, mgr *executor.Manager, model string) (*pipeline.PipelineRef, error) {
	var sb strings.Builder
	sb.WriteString("You are a router. Reply with EXACTLY ONE pipeline name from the list below that best matches the user request, or reply NONE if nothing matches.\n\n")
	sb.WriteString("Available pipelines:\n")
	for _, r := range refs {
		fmt.Fprintf(&sb, "- %s: %s\n", r.Name, r.Description)
	}
	sb.WriteString("\nUser request: ")
	sb.WriteString(prompt)
	sb.WriteString("\n\nReply with only the pipeline name or NONE:")

	classifyPipeline := &pipeline.Pipeline{
		Name:    "ask-route",
		Version: "1",
		Steps: []pipeline.Step{
			{
				ID:       "classify",
				Executor: "ollama",
				Model:    model,
				Prompt:   sb.String(),
			},
		},
	}

	result, err := pipeline.Run(ctx, classifyPipeline, mgr, "")
	if err != nil {
		return nil, nil // classifier failure is non-fatal — fall through
	}

	response := strings.TrimSpace(result)
	// Strip any surrounding punctuation a model might add.
	response = strings.Trim(response, `"'.`)

	if strings.EqualFold(response, "NONE") || response == "" {
		return nil, nil
	}

	// Case-insensitive match against known pipeline names.
	for i, r := range refs {
		if strings.EqualFold(r.Name, response) {
			return &refs[i], nil
		}
	}

	// Garbage response — treat as NONE.
	return nil, nil
}

// matchedIntent holds structured parameters extracted from a routed prompt.
type matchedIntent struct {
	Input    string // focus/topic to pass as {{param.input}}
	CronExpr string // 5-field cron expression, or "" if not scheduled
}

// extractIntent uses the local model to parse focus and schedule out of a
// natural-language prompt that has already matched a pipeline.
// Uses two simple single-answer questions rather than JSON for reliability
// with small local models. Returns safe defaults on failure.
func extractIntent(ctx context.Context, prompt string, mgr *executor.Manager, model string) matchedIntent {
	// Ask two independent questions — simpler prompts are more reliable on 3B models.
	p := &pipeline.Pipeline{
		Name:    "ask-extract-intent",
		Version: "1",
		Steps: []pipeline.Step{
			{
				ID:       "extract_cron",
				Executor: "ollama",
				Model:    model,
				Prompt: `Does this request ask for a repeating schedule? If yes, reply with ONLY a standard 5-field cron expression. If no, reply with NONE.

Examples:
  "run every hour" → 0 * * * *
  "repeat every 2 hours" → 0 */2 * * *
  "run daily" → 0 0 * * *
  "run now" → NONE
  "just run it" → NONE

Request: ` + prompt + `

Reply (cron expression or NONE):`,
			},
			{
				ID:       "extract_focus",
				Executor: "ollama",
				Model:    model,
				Prompt: `What topic or area should this pipeline focus on? Reply with ONLY the focus phrase, or NONE if not mentioned.

Examples:
  "focus on themes documentation" → themes documentation
  "I want it to focus on the executor docs" → executor docs
  "run my pipeline" → NONE

Request: ` + prompt + `

Reply (focus phrase or NONE):`,
			},
		},
	}

	// Run both questions in parallel (no needs dependency).
	type intentResult struct {
		cron  string
		focus string
	}
	// Run as a pipeline — both steps run concurrently.
	// We retrieve both outputs from the combined result via a two-step pipeline
	// that writes to a shared context, so use a sequential approach instead.
	cronResult, _ := pipeline.Run(ctx, &pipeline.Pipeline{
		Name: "ask-extract-cron", Version: "1",
		Steps: []pipeline.Step{p.Steps[0]},
	}, mgr, "")

	focusResult, _ := pipeline.Run(ctx, &pipeline.Pipeline{
		Name: "ask-extract-focus", Version: "1",
		Steps: []pipeline.Step{p.Steps[1]},
	}, mgr, "")

	_ = p // suppress unused warning

	cronExpr := extractCronExpr(cronResult)

	focus := strings.TrimSpace(focusResult)
	if strings.EqualFold(focus, "none") || focus == "" {
		focus = ""
	} else {
		// Strip any leading/trailing punctuation the model might add.
		focus = strings.Trim(focus, `"'.`)
	}

	return matchedIntent{Input: focus, CronExpr: cronExpr}
}

// upsertCronEntry adds or updates a cron entry for the given pipeline in cron.yaml.
func upsertCronEntry(ref *pipeline.PipelineRef, intent matchedIntent) error {
	configDir, err := glitchConfigDir()
	if err != nil {
		return err
	}
	cronPath := filepath.Join(configDir, "cron.yaml")

	entries, err := cron.LoadConfigFrom(cronPath)
	if err != nil {
		return fmt.Errorf("load cron config: %w", err)
	}

	e := cron.Entry{
		Name:       ref.Name,
		Schedule:   intent.CronExpr,
		Kind:       "pipeline",
		Target:     ref.Name,
		Input:      intent.Input,
		Timeout:    "15m",
		WorkingDir: func() string { wd, _ := os.Getwd(); return wd }(),
	}

	entries = cron.UpsertEntry(entries, e)
	return cron.SaveConfigTo(cronPath, entries)
}

// dispatchMatched extracts structured intent from the prompt, optionally
// schedules the pipeline via cron, then runs it if requested.
func dispatchMatched(cmd *cobra.Command, prompt string, ref *pipeline.PipelineRef, inputVars map[string]string) error {
	if askDryRun {
		fmt.Printf("would run pipeline: %s\n  path: %s\n", ref.Name, ref.Path)
		return nil
	}

	fmt.Fprintf(os.Stderr, "[route] → %s\n", ref.Name)

	// Use local model to extract focus input and cron schedule from the prompt.
	mgr, _, resolvedModel, _ := buildAskManager(askProvider, askModel)
	intent := extractIntent(cmd.Context(), prompt, mgr, resolvedModel)

	// Merge extracted input with any explicit --input flags (explicit wins).
	if _, hasInput := inputVars["input"]; !hasInput && intent.Input != "" {
		if inputVars == nil {
			inputVars = make(map[string]string)
		}
		inputVars["input"] = intent.Input
	}

	// Schedule via cron if requested.
	if intent.CronExpr != "" {
		if err := upsertCronEntry(ref, intent); err != nil {
			fmt.Fprintf(os.Stderr, "[route] warn: could not update cron.yaml: %v\n", err)
		} else {
			fmt.Fprintf(os.Stderr, "[cron] scheduled %s (%s)\n", ref.Name, intent.CronExpr)
			if intent.Input != "" {
				fmt.Fprintf(os.Stderr, "[cron] focus: %s\n", intent.Input)
			}
		}
	}

	f, err := os.Open(ref.Path)
	if err != nil {
		return fmt.Errorf("open matched pipeline: %w", err)
	}
	defer f.Close()

	p, err := pipeline.Load(f)
	if err != nil {
		return fmt.Errorf("load matched pipeline: %w", err)
	}

	runMgr := buildFullManager()
	runOpts := buildRunOpts(inputVars)
	result, err := pipeline.Run(cmd.Context(), p, runMgr, prompt, runOpts...)
	if err != nil {
		return err
	}
	if askJSON {
		return printJSON(result, "", ref.Name, "")
	}
	fmt.Println(result)
	return nil
}

// dispatchGenerated generates a pipeline on the fly and presents it for confirmation.
func dispatchGenerated(cmd *cobra.Command, prompt string, mgr *executor.Manager, providerID, model string, inputVars map[string]string) error {
	yamlOutput, err := generatePipeline(cmd.Context(), prompt, model, mgr)
	if err != nil || yamlOutput == "" {
		// Generation failed — fall back to one-shot.
		fmt.Fprintf(os.Stderr, "[route] pipeline generation failed, falling back to one-shot\n")
		return runOneShot(cmd, prompt, providerID, model, mgr, inputVars)
	}

	// Parse to validate and extract the name.
	p, err := pipeline.Load(strings.NewReader(yamlOutput))
	if err != nil {
		fmt.Fprintf(os.Stderr, "[route] generated YAML invalid (%v), falling back to one-shot\n", err)
		return runOneShot(cmd, prompt, providerID, model, mgr, inputVars)
	}

	if askDryRun {
		fmt.Println(yamlOutput)
		return nil
	}

	// Confirmation unless --auto.
	if !askAuto {
		desc := p.Description
		if desc == "" {
			desc = p.Name
		}
		fmt.Fprintf(os.Stderr, "[route] generated pipeline: %s — %s\n", p.Name, desc)
		fmt.Fprint(os.Stderr, "Proceed? [y/N] ")
		scanner := bufio.NewScanner(os.Stdin)
		scanner.Scan()
		answer := strings.TrimSpace(scanner.Text())
		if !strings.EqualFold(answer, "y") {
			fmt.Println(yamlOutput)
			return nil
		}
	}

	runOpts := buildRunOpts(inputVars)
	result, err := pipeline.Run(cmd.Context(), p, mgr, prompt, runOpts...)
	if err != nil {
		return err
	}
	if askJSON {
		return printJSON(result, providerID, model, "")
	}
	fmt.Println(result)
	return nil
}

// generatePipeline uses the local model to generate a pipeline YAML for the given prompt.
// Returns the raw YAML string (fences stripped), or ("", err) on failure.
func generatePipeline(ctx context.Context, prompt, model string, mgr *executor.Manager) (string, error) {
	genPrompt := "Output ONLY valid YAML. No markdown fences. No explanation. No commentary.\n\n" +
		"Generate a glitch pipeline YAML that accomplishes this task: " + prompt + "\n\n" +
		"Rules:\n" +
		"- Use executor: shell for any system commands (git, npm, bun, curl, etc.)\n" +
		"- Use executor: ollama for any AI reasoning steps\n" +
		"- Set version: \"1\"\n" +
		"- Set a description: field that explains what the pipeline does\n" +
		"- Use needs: to express step dependencies\n\n" +
		"Begin YAML now:"

	genPipeline := &pipeline.Pipeline{
		Name:    "ask-generate",
		Version: "1",
		Steps: []pipeline.Step{
			{
				ID:       "generate",
				Executor: "ollama",
				Model:    model,
				Prompt:   genPrompt,
			},
		},
	}

	result, err := pipeline.Run(ctx, genPipeline, mgr, "")
	if err != nil {
		return "", err
	}
	return stripFences(strings.TrimSpace(result)), nil
}

// extractCronExpr scans s for the first line that looks like a 5-field cron
// expression and returns it, or "" if none found. Tolerates surrounding text,
// comments, and extra punctuation that small models commonly add.
func extractCronExpr(s string) string {
	for _, line := range strings.Split(s, "\n") {
		line = strings.TrimSpace(line)
		// Skip obvious non-answers.
		if strings.EqualFold(line, "none") || line == "" {
			continue
		}
		// Take the first run of up to 5 tokens; ignore trailing comment/text.
		fields := strings.Fields(line)
		if len(fields) >= 5 {
			candidate := strings.Join(fields[:5], " ")
			// Rough validity: each field must be a cron token (digits, *, /, -, ,).
			valid := true
			for _, f := range fields[:5] {
				for _, c := range f {
					if c != '*' && c != '/' && c != '-' && c != ',' && (c < '0' || c > '9') {
						valid = false
						break
					}
				}
				if !valid {
					break
				}
			}
			if valid {
				return candidate
			}
		}
	}
	return ""
}

// stripFences removes ```yaml ... ``` or ``` ... ``` wrappers.
func stripFences(s string) string {
	s = strings.TrimSpace(s)
	for _, prefix := range []string{"```yaml", "```yml", "```"} {
		if rest, ok := strings.CutPrefix(s, prefix); ok {
			if idx := strings.LastIndex(rest, "```"); idx >= 0 {
				rest = rest[:idx]
			}
			return strings.TrimSpace(rest)
		}
	}
	return s
}

// runOneShot sends the prompt directly to the local model with no routing.
func runOneShot(cmd *cobra.Command, prompt, providerID, model string, mgr *executor.Manager, inputVars map[string]string) error {
	p := buildAskPipeline(prompt, providerID, model, inputVars, askSynthesize, askSynthesizeModel)
	runOpts := buildRunOpts(inputVars)

	s, serr := store.Open()
	if serr == nil {
		defer s.Close()
		runOpts = append(runOpts, pipeline.WithRunStore(s))
		if askBrain {
			runOpts = append(runOpts, pipeline.WithBrainInjector(pipeline.NewStoreBrainInjector(s)))
			if notes, err := s.AllBrainNotes(cmd.Context()); err == nil && len(notes) < 10 {
				fmt.Fprintf(os.Stderr, "brain is young (%d entries) — it will grow smarter as you use glitch\n", len(notes))
			}
		}
	}

	result, err := pipeline.Run(cmd.Context(), p, mgr, "", runOpts...)
	if err != nil {
		return err
	}

	var brainEntryID string
	if askWriteBrain && s != nil {
		note := store.BrainNote{
			StepID:    "ask",
			CreatedAt: time.Now().Unix(),
			Body:      result,
		}
		if id, werr := s.InsertBrainNote(context.Background(), note); werr == nil {
			brainEntryID = fmt.Sprintf("%d", id)
		} else {
			fmt.Fprintf(os.Stderr, "ask: write-brain: %v\n", werr)
		}
	}

	if askJSON {
		return printJSON(result, providerID, model, brainEntryID)
	}
	fmt.Println(result)
	return nil
}

// buildRunOpts builds the base run options (bus publisher only — store/brain wired per call site).
func buildRunOpts(_ map[string]string) []pipeline.RunOption {
	var opts []pipeline.RunOption
	if pub := newBusPublisher(); pub != nil {
		opts = append(opts, pipeline.WithEventPublisher(pub))
	}
	return opts
}

// runAskPipeline handles the --pipeline flag: load and run a named pipeline or file.
func runAskPipeline(cmd *cobra.Command, args []string) error {
	arg := askPipelineFlag
	var yamlPath string
	if strings.Contains(arg, string(filepath.Separator)) || strings.HasSuffix(arg, ".yaml") {
		yamlPath = arg
	} else {
		configDir, err := glitchConfigDir()
		if err != nil {
			return err
		}
		yamlPath = filepath.Join(configDir, "pipelines", arg+".pipeline.yaml")
	}

	f, err := os.Open(yamlPath)
	if err != nil {
		return fmt.Errorf("pipeline %q not found: %w", arg, err)
	}
	defer f.Close()

	p, err := pipeline.Load(f)
	if err != nil {
		return err
	}

	mgr := buildFullManager()

	input := ""
	if len(args) > 0 {
		input = args[0]
	}

	var runOpts []pipeline.RunOption
	if s, serr := store.Open(); serr == nil {
		defer s.Close()
		runOpts = append(runOpts, pipeline.WithRunStore(s))
		if askBrain {
			runOpts = append(runOpts, pipeline.WithBrainInjector(pipeline.NewStoreBrainInjector(s)))
		}
	}
	if pub := newBusPublisher(); pub != nil {
		runOpts = append(runOpts, pipeline.WithEventPublisher(pub))
	}

	result, err := pipeline.Run(cmd.Context(), p, mgr, input, runOpts...)
	if err != nil {
		return err
	}
	if askJSON {
		return printJSON(result, "", "", "")
	}
	fmt.Println(result)
	return nil
}

// buildAskManager builds an executor manager and resolves the provider/model to use.
func buildAskManager(providerFlag, modelFlag string) (mgr *executor.Manager, providerID, model string, err error) {
	mgr = buildFullManager()
	providers := picker.BuildProviders()

	if providerFlag != "" {
		for _, p := range providers {
			if p.ID == providerFlag {
				providerID = p.ID
				model = modelFlag
				if model == "" && len(p.Models) > 0 {
					model = p.Models[0].ID
				}
				return
			}
		}
		available := make([]string, 0, len(providers))
		for _, p := range providers {
			available = append(available, p.ID)
		}
		err = fmt.Errorf("provider %q not found; available: %s", providerFlag, strings.Join(available, ", "))
		return
	}

	// Local-first: prefer ollama, then any other local provider.
	for _, p := range providers {
		if localProviderIDs[p.ID] {
			providerID = p.ID
			model = modelFlag
			if model == "" && len(p.Models) > 0 {
				model = p.Models[0].ID
			}
			return
		}
	}

	err = fmt.Errorf("no local provider available (is ollama running?); use --provider to specify one explicitly")
	return
}

// buildFullManager returns an executor manager with all AI providers and sidecars loaded.
func buildFullManager() *executor.Manager {
	runProviders := picker.BuildProviders()
	mgr := executor.NewManager()
	for _, prov := range runProviders {
		if prov.SidecarPath != "" {
			continue
		}
		binary := prov.Command
		if binary == "" {
			binary = prov.ID
		}
		if err := mgr.Register(executor.NewCliAdapter(prov.ID, prov.Label+" CLI adapter", binary, prov.PipelineArgs...)); err != nil {
			fmt.Fprintf(os.Stderr, "ask: register provider %q: %v\n", prov.ID, err)
		}
	}
	configDir, _ := glitchConfigDir()
	if configDir != "" {
		if errs := mgr.LoadWrappersFromDir(filepath.Join(configDir, "wrappers")); len(errs) > 0 {
			for _, e := range errs {
				fmt.Fprintf(os.Stderr, "ask: sidecar load warning: %v\n", e)
			}
		}
	}
	return mgr
}

// buildAskPipeline constructs a one-shot (or two-step with synthesize) pipeline in memory.
func buildAskPipeline(prompt, providerID, model string, vars map[string]string, synthesize bool, synthesizeModel string) *pipeline.Pipeline {
	steps := []pipeline.Step{
		{
			ID:       "ask",
			Executor: providerID,
			Model:    model,
			Prompt:   prompt,
			Vars:     vars,
		},
	}

	if synthesize {
		steps = append(steps, pipeline.Step{
			ID:       "synthesize",
			Executor: "claude",
			Model:    synthesizeModel,
			Needs:    []string{"ask"},
			Prompt: "You are an editor. Do not add information not present in the input. " +
				"Fix grammar, remove artifacts, and improve clarity only.\n\n{{step.ask.data.value}}",
		})
	}

	return &pipeline.Pipeline{
		Name:    "ask",
		Version: "1",
		Steps:   steps,
	}
}

// parseInputVars parses []string{"key=value", ...} into a map.
func parseInputVars(raw []string) (map[string]string, error) {
	out := make(map[string]string, len(raw))
	for _, kv := range raw {
		k, v, ok := strings.Cut(kv, "=")
		if !ok {
			return nil, fmt.Errorf("--input %q must be key=value", kv)
		}
		out[k] = v
	}
	return out, nil
}

func printJSON(response, providerID, model, brainEntryID string) error {
	out := map[string]string{
		"response": response,
		"provider": providerID,
		"model":    model,
	}
	if brainEntryID != "" {
		out["brain_entry_id"] = brainEntryID
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(out)
}
