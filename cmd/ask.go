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
	"github.com/8op-org/gl1tch/internal/router"
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

				// Augment refs with synthetic entries for installed APM executors
				// that don't already have a materialized pipeline file.
				existingNames := make(map[string]bool, len(refs))
				for _, r := range refs {
					existingNames[r.Name] = true
				}
				for _, ex := range mgr.List() {
					name := ex.Name()
					if !strings.HasPrefix(name, "apm.") {
						continue
					}
					// Derive canonical pipeline name: "apm.<base>" → same as executor ID.
					if existingNames[name] {
						continue
					}
					refs = append(refs, pipeline.PipelineRef{
						Name:        name,
						Description: "[apm] " + name + ": " + ex.Description(),
						Path:        "", // synthetic — no file on disk
					})
				}

				if len(refs) > 0 {
					embedder := &router.OllamaEmbedder{Model: router.DefaultEmbeddingModel}
					r := router.New(mgr, embedder, router.Config{
						Model:    resolvedModel,
						CacheDir: filepath.Join(configDir, "cache"),
					})
					result, _ := r.Route(cmd.Context(), prompt, refs)
					if result != nil && result.Pipeline != nil {
						// Synthetic APM ref (no pipeline file) — dispatch as one-shot.
						if result.Pipeline.Path == "" && strings.HasPrefix(result.Pipeline.Name, "apm.") {
							return dispatchAPMAgent(cmd, prompt, result.Pipeline.Name, mgr, inputVars)
						}
						return dispatchMatched(cmd, prompt, result, inputVars)
					}
					// Near-miss: close match that didn't meet the confidence bar — ask user.
					if result != nil && result.NearMiss != nil {
						if confirmed, err := confirmNearMiss(result.NearMiss.Name, result.NearMissScore); err == nil && confirmed {
							nearResult := &router.RouteResult{
								Pipeline:   result.NearMiss,
								Confidence: result.NearMissScore,
								Method:     "near-miss",
							}
							return dispatchMatched(cmd, prompt, nearResult, inputVars)
						}
					}
					// No match — try to generate a pipeline on the fly.
					return dispatchGenerated(cmd, prompt, mgr, providerID, resolvedModel, inputVars)
				}
			}
		}

		// ── one-shot fallback ─────────────────────────────────────────────────────
		// Inject last_run.json context when the question is about a recent pipeline run.
		if lastRunCtx := loadLastRunContext(prompt); lastRunCtx != "" {
			if inputVars == nil {
				inputVars = make(map[string]string)
			}
			inputVars["_last_run_context"] = lastRunCtx
		}
		return runOneShot(cmd, prompt, providerID, resolvedModel, mgr, inputVars)
	},
}


// confirmNearMiss prints a near-miss prompt and reads a y/N answer from stdin.
// Returns (true, nil) when the user confirms, (false, nil) otherwise.
// Errors (e.g. EOF on non-interactive stdin) are treated as "no".
func confirmNearMiss(name string, score float64) (bool, error) {
	fmt.Fprintf(os.Stderr, "[route] near match: %q (%.0f%%) — run it? [y/N] ", name, score*100)
	scanner := bufio.NewScanner(os.Stdin)
	if !scanner.Scan() {
		return false, nil
	}
	answer := strings.ToLower(strings.TrimSpace(scanner.Text()))
	return answer == "y" || answer == "yes", nil
}

// upsertCronEntry adds or updates a cron entry for the given pipeline in cron.yaml.
func upsertCronEntry(ref *pipeline.PipelineRef, input, cronExpr string) error {
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
		Schedule:   cronExpr,
		Kind:       "pipeline",
		Target:     ref.Name,
		Input:      input,
		Timeout:    "15m",
		WorkingDir: func() string { wd, _ := os.Getwd(); return wd }(),
	}

	entries = cron.UpsertEntry(entries, e)
	return cron.SaveConfigTo(cronPath, entries)
}

// dispatchMatched runs the pipeline identified by result, using the pre-extracted
// input and cron schedule from the router — no additional LLM calls needed.
func dispatchMatched(cmd *cobra.Command, prompt string, result *router.RouteResult, inputVars map[string]string) error {
	ref := result.Pipeline
	if askDryRun {
		fmt.Printf("would run pipeline: %s\n  path: %s\n", ref.Name, ref.Path)
		return nil
	}

	fmt.Fprintf(os.Stderr, "[route] → %s (%.0f%%)\n", ref.Name, result.Confidence*100)

	// Merge router-extracted input with any explicit --input flags (explicit wins).
	if _, hasInput := inputVars["input"]; !hasInput && result.Input != "" {
		if inputVars == nil {
			inputVars = make(map[string]string)
		}
		inputVars["input"] = result.Input
	}

	// Schedule via cron if the router extracted a schedule.
	if result.CronExpr != "" {
		if err := upsertCronEntry(ref, result.Input, result.CronExpr); err != nil {
			fmt.Fprintf(os.Stderr, "[route] warn: could not update cron.yaml: %v\n", err)
		} else {
			fmt.Fprintf(os.Stderr, "[cron] scheduled %s (%s)\n", ref.Name, result.CronExpr)
			if result.Input != "" {
				fmt.Fprintf(os.Stderr, "[cron] focus: %s\n", result.Input)
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
	output, err := pipeline.Run(cmd.Context(), p, runMgr, prompt, runOpts...)
	if err != nil {
		return err
	}
	if askJSON {
		return printJSON(output, "", ref.Name, "")
	}
	fmt.Println(output)
	return nil
}

// dispatchGenerated generates a pipeline on the fly and presents it for confirmation.
// dispatchAPMAgent builds and runs a synthetic single-step pipeline targeting
// the named APM executor. Used when the router matches a synthetic APM ref
// (one with no pipeline YAML file on disk).
func dispatchAPMAgent(cmd *cobra.Command, prompt, executorID string, mgr *executor.Manager, inputVars map[string]string) error {
	if askDryRun {
		fmt.Printf("would dispatch apm agent: %s\n", executorID)
		return nil
	}
	fmt.Fprintf(os.Stderr, "[route] → %s (apm agent)\n", executorID)

	p := &pipeline.Pipeline{
		Name:    executorID,
		Version: "1",
		Steps: []pipeline.Step{
			{
				ID:       "run",
				Executor: executorID,
				Prompt:   prompt,
				Vars:     inputVars,
			},
		},
	}

	runOpts := buildRunOpts(inputVars)
	output, err := pipeline.Run(cmd.Context(), p, mgr, prompt, runOpts...)
	if err != nil {
		return err
	}
	if askJSON {
		return printJSON(output, "", executorID, "")
	}
	fmt.Println(output)
	return nil
}

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

	if !askJSON {
		runOpts = append(runOpts, pipeline.WithStepWriter(os.Stdout))
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
	// Response was already streamed to stdout via WithStepWriter; just ensure
	// the terminal prompt starts on a fresh line.
	if !strings.HasSuffix(result, "\n") {
		fmt.Println()
	}
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

// loadLastRunContext reads ~/.config/glitch/last_run.json and returns a
// formatted context string when the prompt appears to be asking about a recent
// pipeline run. Returns "" when the file is absent or the prompt is unrelated.
func loadLastRunContext(prompt string) string {
	lower := strings.ToLower(prompt)
	triggers := []string{"last run", "last pipeline", "why did it fail", "what happened", "that pipeline", "previous run", "failed run"}
	matched := false
	for _, t := range triggers {
		if strings.Contains(lower, t) {
			matched = true
			break
		}
	}
	if !matched {
		return ""
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	data, err := os.ReadFile(filepath.Join(home, ".config", "glitch", "last_run.json"))
	if err != nil {
		return ""
	}
	var lr struct {
		Name       string `json:"name"`
		ExitStatus *int   `json:"exit_status"`
		Stdout     string `json:"stdout"`
		Stderr     string `json:"stderr"`
	}
	if err := json.Unmarshal(data, &lr); err != nil {
		return ""
	}
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("[last pipeline run: %s]", lr.Name))
	if lr.ExitStatus != nil {
		sb.WriteString(fmt.Sprintf(" exit=%d", *lr.ExitStatus))
	}
	trim := func(s string, max int) string {
		r := []rune(strings.TrimSpace(s))
		if len(r) > max {
			return "..." + string(r[len(r)-max:])
		}
		return string(r)
	}
	if out := trim(lr.Stdout, 1500); out != "" {
		sb.WriteString("\nstdout:\n" + out)
	}
	if err := trim(lr.Stderr, 1000); err != "" {
		sb.WriteString("\nstderr:\n" + err)
	}
	return sb.String()
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
