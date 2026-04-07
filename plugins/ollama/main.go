package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"strings"

	"github.com/8op-org/gl1tch/glitchctx"
)

func main() {
	if err := run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr, os.Getenv); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// modelEntry is a single model entry for --list-models JSON output.
type modelEntry struct {
	ID    string `json:"id"`
	Label string `json:"label"`
}

// listModels runs `ollama list`, parses its output, and writes JSON to stdout.
func listModels(stdout io.Writer, stderr io.Writer) error {
	out, err := exec.Command("ollama", "list").Output()
	if err != nil {
		return fmt.Errorf("ollama list: %w", err)
	}
	var models []modelEntry
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	for i, line := range lines {
		if i == 0 { // skip header
			continue
		}
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}
		id := fields[0]
		models = append(models, modelEntry{ID: id, Label: id})
	}
	if models == nil {
		models = []modelEntry{}
	}
	data, err := json.Marshal(models)
	if err != nil {
		return fmt.Errorf("marshalling models: %w", err)
	}
	fmt.Fprintln(stdout, string(data))
	return nil
}

// isModelPresentFn is the function used to check whether a model is available
// locally. It is a package-level variable so tests can override it.
var isModelPresentFn = isModelPresent

// firstInstalledModelFn returns the first model name reported by `ollama list`,
// or "" if none are installed / ollama isn't reachable. Package-level so tests
// can override it. Used as the fallback when neither --model nor GLITCH_MODEL
// is set, so chain runs that route to ollama don't require the caller to pin
// a model name they may not know.
var firstInstalledModelFn = firstInstalledModel

func firstInstalledModel() string {
	out, err := exec.Command("ollama", "list").Output()
	if err != nil {
		return ""
	}
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	for i, line := range lines {
		if i == 0 { // skip "NAME" header
			continue
		}
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}
		return fields[0]
	}
	return ""
}

// isModelPresent returns true if the given model is already available locally
// according to `ollama list`. It performs a case-insensitive match and handles
// the common case where `ollama list` shows "llama3.2:latest" when the caller
// requests "llama3.2" (prefix match before the colon).
func isModelPresent(model string) bool {
	out, err := exec.Command("ollama", "list").Output()
	if err != nil {
		return false
	}
	modelLower := strings.ToLower(model)
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	for i, line := range lines {
		if i == 0 { // skip header (starts with "NAME")
			continue
		}
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}
		listed := strings.ToLower(fields[0])
		// Exact match.
		if listed == modelLower {
			return true
		}
		// Prefix match: "llama3.2:latest" matches request for "llama3.2".
		if strings.HasPrefix(listed, modelLower+":") {
			return true
		}
	}
	return false
}

// parseOptions converts a slice of "key=value" strings into a map suitable for
// the Ollama options field. Integer-parseable values are stored as int;
// all other values are stored as string.
func parseOptions(opts []string) (map[string]any, error) {
	if len(opts) == 0 {
		return nil, nil
	}
	m := make(map[string]any, len(opts))
	for _, o := range opts {
		k, v, ok := strings.Cut(o, "=")
		if !ok || k == "" {
			return nil, fmt.Errorf("option %q: expected key=value format", o)
		}
		if n, err := strconv.Atoi(v); err == nil {
			m[k] = n
		} else {
			m[k] = v
		}
	}
	return m, nil
}

// createModelFn is the function used to create Ollama models. Package-level so
// tests can override it without shelling out to the real ollama binary.
var createModelFn = createModel

// createModel builds a Modelfile from base + options, writes it to a temp
// file, and runs `ollama create <name> -f <path>`. On success it prints
// "Created model '<name>'" to stdout.
func createModel(base, name string, options map[string]any, stdout, stderr io.Writer) error {
	var sb strings.Builder
	fmt.Fprintf(&sb, "FROM %s\n", base)
	for k, v := range options {
		fmt.Fprintf(&sb, "PARAMETER %s %v\n", k, v)
	}

	tmp, err := os.CreateTemp("", "orcai-modelfile-*.txt")
	if err != nil {
		return fmt.Errorf("creating temp Modelfile: %w", err)
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.WriteString(sb.String()); err != nil {
		tmp.Close()
		return fmt.Errorf("writing Modelfile: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("closing Modelfile: %w", err)
	}

	cmd := exec.Command("ollama", "create", name, "-f", tmp.Name())
	cmd.Stdout = stderr
	cmd.Stderr = stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("ollama create %s: %w", name, err)
	}
	fmt.Fprintf(stdout, "Created model '%s'\n", name)
	return nil
}

// run is the testable entry point.
// args are the CLI arguments (os.Args[1:] in production).
// getenv is the environment resolver (os.Getenv in production).
//
// Model resolution priority: --model flag > GLITCH_MODEL env var.
// URL resolution priority:   GLITCH_OLLAMA_URL env var > default http://localhost:11434.
func run(args []string, stdin io.Reader, stdout io.Writer, stderr io.Writer, getenv func(string) string) error {
	// Handle --list-models before flag parsing.
	for _, arg := range args {
		if arg == "--list-models" {
			return listModels(stdout, stderr)
		}
	}

	fs := flag.NewFlagSet("orcai-ollama", flag.ContinueOnError)
	fs.SetOutput(stderr)
	modelFlag := fs.String("model", "", "Ollama model name (e.g. llama3.2)")
	createModelFlag := fs.String("create-model", "", "Create a named Ollama model alias from --model + options, then exit")

	var optionPairs []string
	fs.Func("option", "key=value pair to pass as Ollama inference option (repeatable)", func(s string) error {
		optionPairs = append(optionPairs, s)
		return nil
	})

	if err := fs.Parse(args); err != nil {
		return err
	}

	// Resolve model in priority order:
	//   1. --model flag                  (explicit caller intent)
	//   2. GLITCH_MODEL env var          (set by glitchd / chain runner)
	//   3. first locally-installed model (fallback so chains routing through
	//      ollama don't need the caller to pin a name they may not know).
	model := *modelFlag
	if model == "" {
		model = getenv("GLITCH_MODEL")
	}

	// Parse options from --option flags.
	options, err := parseOptions(optionPairs)
	if err != nil {
		return err
	}

	// --create-model: build Modelfile alias and exit without running inference.
	// Creating an alias still requires an explicit name — we don't fall back
	// here since the user must know which existing model they're aliasing.
	if *createModelFlag != "" {
		if model == "" {
			return fmt.Errorf("model is required: set --model flag or GLITCH_MODEL environment variable")
		}
		return createModelFn(model, *createModelFlag, options, stdout, stderr)
	}

	if model == "" {
		model = firstInstalledModelFn()
	}
	if model == "" {
		return fmt.Errorf("no model available: set --model, GLITCH_MODEL, or `ollama pull <model>` first")
	}

	// Read prompt from stdin.
	promptBytes, err := io.ReadAll(stdin)
	if err != nil {
		return fmt.Errorf("reading stdin: %w", err)
	}
	prompt := strings.TrimSpace(string(promptBytes))
	if prompt == "" {
		return fmt.Errorf("prompt is required: no input received on stdin")
	}

	// Resolve base URL from GLITCH_OLLAMA_URL, defaulting to localhost.
	baseURL := getenv("GLITCH_OLLAMA_URL")
	if baseURL == "" {
		baseURL = "http://localhost:11434"
	}
	baseURL = strings.TrimRight(baseURL, "/")

	// Proactive check: pull the model before inference if it isn't present locally.
	if !isModelPresentFn(model) {
		fmt.Fprintf(stderr, "Model %q not found locally — pulling...\n", model)
		pull := exec.Command("ollama", "pull", model)
		pull.Stdout = stderr
		pull.Stderr = stderr
		if pullErr := pull.Run(); pullErr != nil {
			return fmt.Errorf("ollama pull %s: %w", model, pullErr)
		}
	}

	// Both protocols: input lets the model write files / run shell, output
	// makes its reply land in the chat as structured blocks. Order matters:
	// instructions first, then shell snapshot, then user request.
	fullPrompt := glitchctx.ProtocolInstructions +
		glitchctx.OutputProtocolInstructions +
		glitchctx.BuildShellContext() +
		"\n## User Request\n" + prompt
	result, err := callOllama(baseURL, model, fullPrompt, options)
	if err != nil {
		// Reactive fallback: model may have become unavailable between the check and inference.
		if strings.Contains(err.Error(), "not found") {
			fmt.Fprintf(stderr, "Model %q not found locally — pulling...\n", model)
			pull := exec.Command("ollama", "pull", model)
			pull.Stdout = stderr
			pull.Stderr = stderr
			if pullErr := pull.Run(); pullErr != nil {
				return fmt.Errorf("ollama pull %s: %w", model, pullErr)
			}
			result, err = callOllama(baseURL, model, prompt, options)
		}
		if err != nil {
			return err
		}
	}

	fmt.Fprint(stdout, glitchctx.ProcessBlocks(result.Response, stdout, stderr))

	// Emit token usage as a gl1tch-stats line so the gl1tch game engine can
	// award XP based on actual token counts from the Ollama API response.
	if result.EvalCount > 0 || result.PromptEvalCount > 0 {
		statsModel := result.Model
		if statsModel == "" {
			statsModel = model
		}
		statsJSON, merr := json.Marshal(map[string]any{
			"type":          "gl1tch-stats",
			"model":         statsModel,
			"input_tokens":  result.PromptEvalCount,
			"output_tokens": result.EvalCount,
			"duration_ms":   result.TotalDuration / 1_000_000,
		})
		if merr == nil {
			fmt.Fprintf(stdout, "\n%s", statsJSON)
		}
	}

	return nil
}

// generateRequest is the JSON body for POST /api/generate.
type generateRequest struct {
	Model   string         `json:"model"`
	Prompt  string         `json:"prompt"`
	Stream  bool           `json:"stream"`
	Options map[string]any `json:"options,omitempty"`
}

// generateResponse is the JSON body returned by Ollama when stream=false.
type generateResponse struct {
	Model           string `json:"model"`
	Response        string `json:"response"`
	Error           string `json:"error,omitempty"`
	PromptEvalCount int64  `json:"prompt_eval_count"` // input tokens
	EvalCount       int64  `json:"eval_count"`        // output tokens
	TotalDuration   int64  `json:"total_duration"`    // nanoseconds
}

// callOllama sends a prompt to the Ollama /api/generate endpoint and returns the full response.
func callOllama(baseURL, model, prompt string, options map[string]any) (generateResponse, error) {
	body, err := json.Marshal(generateRequest{
		Model:   model,
		Prompt:  prompt,
		Stream:  false,
		Options: options,
	})
	if err != nil {
		return generateResponse{}, fmt.Errorf("marshalling request: %w", err)
	}

	resp, err := http.Post(baseURL+"/api/generate", "application/json", bytes.NewReader(body))
	if err != nil {
		return generateResponse{}, fmt.Errorf("connecting to Ollama at %s: %w", baseURL, err)
	}
	defer resp.Body.Close()

	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return generateResponse{}, fmt.Errorf("reading response: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return generateResponse{}, fmt.Errorf("Ollama returned HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(respBytes)))
	}

	var genResp generateResponse
	if err := json.Unmarshal(respBytes, &genResp); err != nil {
		return generateResponse{}, fmt.Errorf("parsing response: %w", err)
	}
	if genResp.Error != "" {
		return generateResponse{}, fmt.Errorf("Ollama error: %s", genResp.Error)
	}

	return genResp, nil
}
