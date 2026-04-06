package cmd

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/8op-org/gl1tch/internal/executor"
	"github.com/8op-org/gl1tch/internal/picker"
	"github.com/8op-org/gl1tch/internal/pipeline"
	"github.com/8op-org/gl1tch/internal/store"
)

func init() {
	rootCmd.AddCommand(workflowCmd)
	workflowCmd.AddCommand(workflowRunCmd)
	workflowRunCmd.Flags().StringVar(&workflowRunInput, "input", "", "user input passed to the workflow as {{param.input}}")
	workflowCmd.AddCommand(workflowResumeCmd)
	workflowResumeCmd.Flags().Int64Var(&workflowResumeRunID, "run-id", 0, "Store run ID to resume")
	_ = workflowResumeCmd.MarkFlagRequired("run-id")
	workflowCmd.AddCommand(workflowResultsCmd)
	workflowResultsCmd.Flags().IntVar(&workflowResultsLimit, "limit", 1, "number of matching runs to show")
}

var workflowCmd = &cobra.Command{
	Use:   "workflow",
	Short: "Manage and run AI workflows",
}

var workflowRunCmd = &cobra.Command{
	Use:   "run <name|file>",
	Short: "Run a workflow by name or file path",
	Long: `Run a workflow.

If <name|file> is a path or ends in .yaml, it is loaded directly. Otherwise
it is treated as a bare name and resolved by walking up from the current
directory looking for a .glitch/workflows/<name>.workflow.yaml file.`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		arg := args[0]

		yamlPath, err := resolveWorkflowArg(arg)
		if err != nil {
			return err
		}

		f, err := os.Open(yamlPath)
		if err != nil {
			return fmt.Errorf("workflow %q not found: %w", arg, err)
		}
		defer f.Close()

		p, err := pipeline.Load(f)
		if err != nil {
			return err
		}

		if os.Getenv("FORCE_COLOR") == "1" {
			toANSI := func(envKey, fallback string) string {
				hex := os.Getenv(envKey)
				if len(hex) != 6 {
					hex = fallback
				}
				var r, g, b uint64
				fmt.Sscanf(hex[0:2], "%x", &r)
				fmt.Sscanf(hex[2:4], "%x", &g)
				fmt.Sscanf(hex[4:6], "%x", &b)
				return fmt.Sprintf("\033[38;2;%d;%d;%dm", r, g, b)
			}
			dim := toANSI("GLITCH_COL_DIM", "6272a4")
			accent := toANSI("GLITCH_COL_ACCENT", "bd93f9")
			reset := "\033[0m"
			fmt.Printf("%s[workflow]%s starting: %s%s%s\n", dim, reset, accent, p.Name, reset)
		} else {
			fmt.Printf("[workflow] starting: %s\n", p.Name)
		}

		runProviders := picker.BuildProviders()
		mgr := executor.NewManager()
		for _, prov := range runProviders {
			// Sidecar-backed providers are fully registered by LoadWrappersFromDir below.
			if prov.SidecarPath != "" {
				continue
			}
			binary := prov.Command
			if binary == "" {
				binary = prov.ID
			}
			if err := mgr.Register(executor.NewCliAdapter(prov.ID, prov.Label+" CLI adapter", binary, prov.PipelineArgs...)); err != nil {
				fmt.Fprintf(os.Stderr, "workflow: register provider %q: %v\n", prov.ID, err)
			}
		}

		// Load sidecar plugins from ~/.config/glitch/wrappers/.
		wrappersConfigDir, _ := glitchConfigDir()
		if wrappersConfigDir != "" {
			wrappersDir := filepath.Join(wrappersConfigDir, "wrappers")
			if errs := mgr.LoadWrappersFromDir(wrappersDir); len(errs) > 0 {
				for _, e := range errs {
					fmt.Fprintf(os.Stderr, "workflow: sidecar load warning: %v\n", e)
				}
			}
		}

		// Open the result store so this run is recorded in the inbox.
		// A failure to open the store is non-fatal — the workflow still runs.
		var storeOpts []pipeline.RunOption
		if s, serr := store.Open(); serr == nil {
			defer s.Close()
			storeOpts = append(storeOpts, pipeline.WithRunStore(s))
			// Wire brain context injection: use_brain / write_brain flags on workflow
			// steps will prepend DB context and parse <brain> notes from responses.
			storeOpts = append(storeOpts, pipeline.WithBrainInjector(pipeline.NewStoreBrainInjector(s)))
		} else {
			fmt.Fprintf(os.Stderr, "workflow: store unavailable: %v\n", serr)
		}

		// Wire busd publisher if the daemon is reachable.
		if pub := newBusPublisher(); pub != nil {
			storeOpts = append(storeOpts, pipeline.WithEventPublisher(pub))
		}

		result, err := pipeline.Run(cmd.Context(), p, mgr, workflowRunInput, storeOpts...)
		if err != nil {
			return err
		}
		fmt.Println(result)
		return nil
	},
}

// resolveWorkflowArg turns a bare name or file path into an absolute path to a
// .workflow.yaml file. Bare names are looked up by walking up from the current
// directory until a .glitch/workflows/<name>.workflow.yaml file is found.
func resolveWorkflowArg(arg string) (string, error) {
	if strings.Contains(arg, string(filepath.Separator)) || strings.HasSuffix(arg, ".yaml") {
		return arg, nil
	}

	cwd, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("workflow: get cwd: %w", err)
	}

	dir := cwd
	for {
		candidate := filepath.Join(dir, ".glitch", "workflows", arg+".workflow.yaml")
		if _, err := os.Stat(candidate); err == nil {
			return candidate, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}

	return "", fmt.Errorf("workflow %q not found: no .glitch/workflows/%s.workflow.yaml in %q or any parent", arg, arg, cwd)
}

// findAncestorWorkflowsDir walks up from the current directory looking for a
// .glitch/workflows/ directory. Returns "" if none is found. Used by the ask
// command to discover workflows in the surrounding workspace (typically the
// gl1tch repo, since `glitch ask` is scoped to self-improvement).
func findAncestorWorkflowsDir() string {
	cwd, err := os.Getwd()
	if err != nil {
		return ""
	}
	dir := cwd
	for {
		candidate := filepath.Join(dir, ".glitch", "workflows")
		if info, err := os.Stat(candidate); err == nil && info.IsDir() {
			return candidate
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return ""
		}
		dir = parent
	}
}

func glitchConfigDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", "glitch"), nil
}

var workflowRunInput string

var workflowResumeRunID int64

var workflowResultsLimit int

// extractStreamResult scans NDJSON stdout from a Claude/Ollama executor run and
// returns the human-readable result text. Falls back to raw stdout if no
// stream-json result line is found (e.g. shell executor output).
func extractStreamResult(stdout string) string {
	if stdout == "" {
		return ""
	}
	// Fast path: if the output doesn't look like NDJSON, return as-is.
	if len(stdout) == 0 || stdout[0] != '{' {
		return stdout
	}
	type resultLine struct {
		Type   string `json:"type"`
		Result string `json:"result"`
	}
	var last string
	scanner := bufio.NewScanner(strings.NewReader(stdout))
	scanner.Buffer(make([]byte, 1<<20), 1<<20)
	for scanner.Scan() {
		line := scanner.Text()
		var r resultLine
		if err := json.Unmarshal([]byte(line), &r); err == nil && r.Type == "result" && r.Result != "" {
			last = r.Result
		}
	}
	if last != "" {
		return last
	}
	return stdout
}

var workflowResultsCmd = &cobra.Command{
	Use:   "results [name]",
	Short: "Show the output of the most recent workflow run, optionally filtered by name",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		s, err := store.Open()
		if err != nil {
			return fmt.Errorf("results: open store: %w", err)
		}
		defer s.Close()

		runs, err := s.QueryRuns(200)
		if err != nil {
			return fmt.Errorf("results: query store: %w", err)
		}

		nameFilter := ""
		if len(args) > 0 {
			nameFilter = args[0]
		}

		var matched []store.Run
		for _, r := range runs {
			if nameFilter == "" || r.Name == nameFilter {
				matched = append(matched, r)
				if len(matched) >= workflowResultsLimit {
					break
				}
			}
		}

		if len(matched) == 0 {
			if nameFilter != "" {
				return fmt.Errorf("no runs found for workflow %q", nameFilter)
			}
			return fmt.Errorf("no workflow runs recorded yet")
		}

		for i, r := range matched {
			if i > 0 {
				fmt.Println("---")
			}
			status := "in-flight"
			if r.ExitStatus != nil {
				if *r.ExitStatus == 0 {
					status = "ok"
				} else {
					status = fmt.Sprintf("exit %d", *r.ExitStatus)
				}
			}
			startedAt := time.UnixMilli(r.StartedAt).Format(time.DateTime)
			fmt.Printf("workflow: %s  |  run: %d  |  started: %s  |  status: %s\n", r.Name, r.ID, startedAt, status)
			if len(r.Steps) > 0 {
				for _, step := range r.Steps {
					dur := ""
					if step.DurationMs > 0 {
						dur = fmt.Sprintf("  %dms", step.DurationMs)
					}
					fmt.Printf("  %-20s %s%s\n", step.ID, step.Status, dur)
				}
			}
			fmt.Println()
			if r.Stdout != "" {
				fmt.Println(extractStreamResult(r.Stdout))
			}
			if r.Stderr != "" {
				fmt.Fprintf(os.Stderr, "%s\n", r.Stderr)
			}
		}
		return nil
	},
}

var workflowResumeCmd = &cobra.Command{
	Use:   "resume",
	Short: "Resume a workflow that is paused waiting for a clarification answer",
	RunE: func(cmd *cobra.Command, args []string) error {
		s, err := store.Open()
		if err != nil {
			return fmt.Errorf("resume: open store: %w", err)
		}
		defer s.Close()

		runIDStr := strconv.FormatInt(workflowResumeRunID, 10)
		clarif, err := s.LoadClarificationForRun(runIDStr)
		if err != nil {
			return fmt.Errorf("resume: load clarification: %w", err)
		}
		if clarif == nil {
			return fmt.Errorf("resume: no pending clarification found for run %d", workflowResumeRunID)
		}
		if clarif.Answer == "" {
			return fmt.Errorf("resume: clarification for run %d has no answer yet", workflowResumeRunID)
		}

		run, err := s.GetRun(workflowResumeRunID)
		if err != nil {
			return fmt.Errorf("resume: load run: %w", err)
		}

		// Parse pipeline_file and cwd from run metadata. The metadata key is
		// kept as "pipeline_file" for store-format compatibility with existing
		// run rows; on disk these always point to .workflow.yaml files.
		type runMeta struct {
			PipelineFile string `json:"pipeline_file"`
			CWD          string `json:"cwd"`
		}
		var meta runMeta
		_ = json.Unmarshal([]byte(run.Metadata), &meta)
		if meta.PipelineFile == "" {
			return fmt.Errorf("resume: run %d has no workflow file in metadata", workflowResumeRunID)
		}

		f, err := os.Open(meta.PipelineFile)
		if err != nil {
			return fmt.Errorf("resume: open workflow %q: %w", meta.PipelineFile, err)
		}
		defer f.Close()

		p, err := pipeline.Load(f)
		if err != nil {
			return fmt.Errorf("resume: load workflow: %w", err)
		}

		// Build the executor manager (same as workflowRunCmd).
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
				fmt.Fprintf(os.Stderr, "resume: register provider %q: %v\n", prov.ID, err)
			}
		}
		wrappersConfigDir, _ := glitchConfigDir()
		if wrappersConfigDir != "" {
			if errs := mgr.LoadWrappersFromDir(filepath.Join(wrappersConfigDir, "wrappers")); len(errs) > 0 {
				for _, e := range errs {
					fmt.Fprintf(os.Stderr, "resume: sidecar load warning: %v\n", e)
				}
			}
		}

		followUp := pipeline.BuildClarificationFollowUp(clarif.Output, clarif.Answer)

		// Delete clarification before running to prevent re-entrant resumes.
		_ = s.DeleteClarification(runIDStr)

		var runOpts []pipeline.RunOption
		runOpts = append(runOpts, pipeline.WithRunStore(s))
		runOpts = append(runOpts, pipeline.WithResumeFrom(workflowResumeRunID, clarif.StepID, followUp))
		if pub := newBusPublisher(); pub != nil {
			runOpts = append(runOpts, pipeline.WithEventPublisher(pub))
		}

		fmt.Printf("[workflow] resuming run %d from step %q\n", workflowResumeRunID, clarif.StepID)
		result, err := pipeline.Run(cmd.Context(), p, mgr, "", runOpts...)
		if err != nil {
			return err
		}

		fmt.Println(result)
		return nil
	},
}
