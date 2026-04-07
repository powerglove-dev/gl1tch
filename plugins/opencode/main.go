package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"

	"github.com/8op-org/gl1tch/glitchctx"
)

// pullOllamaModel ensures the Ollama model is present, pulling it only when necessary.
// It checks "ollama list" first; if the model is already installed it returns nil immediately.
// Progress output is written to stderr. An error is returned only if the pull fails.
func pullOllamaModel(model string, stderr io.Writer) error {
	if !strings.HasPrefix(model, "ollama/") {
		return nil // not an Ollama model; skip
	}
	// Strip the "ollama/" prefix to get the bare model name.
	name := strings.TrimPrefix(model, "ollama/")

	// Check whether the model is already installed via "ollama list".
	listOut, err := exec.Command("ollama", "list").Output()
	if err == nil {
		for i, line := range strings.Split(string(listOut), "\n") {
			// Skip the header line (starts with "NAME").
			if i == 0 && strings.HasPrefix(strings.TrimSpace(line), "NAME") {
				continue
			}
			fields := strings.Fields(line)
			if len(fields) == 0 {
				continue
			}
			listed := fields[0] // e.g. "llama3.2:latest"
			// Exact match or base-name match (strip :<tag>).
			if listed == name {
				return nil
			}
			if idx := strings.Index(listed, ":"); idx >= 0 {
				if listed[:idx] == name {
					return nil
				}
			}
		}
	}

	fmt.Fprintf(stderr, "Ensuring Ollama model %q is available — pulling if needed...\n", name)
	cmd := exec.Command("ollama", "pull", name)
	cmd.Stdout = stderr
	cmd.Stderr = stderr
	if err := cmd.Run(); err != nil {
		// Pull failed. Check once more if the model is actually present — it may
		// be a locally-created alias that has no registry entry (e.g. created via
		// `orcai-ollama --create-model`). If it is present, the pull error is
		// harmless and we can proceed.
		if listOut2, listErr := exec.Command("ollama", "list").Output(); listErr == nil {
			for i, line := range strings.Split(string(listOut2), "\n") {
				if i == 0 {
					continue
				}
				fields := strings.Fields(line)
				if len(fields) == 0 {
					continue
				}
				listed := fields[0]
				if listed == name {
					return nil
				}
				if idx := strings.Index(listed, ":"); idx >= 0 {
					if listed[:idx] == name {
						return nil
					}
				}
			}
		}
		return err
	}
	return nil
}

func main() {
	code, err := run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr, os.Getenv, execOpencode)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
	}
	os.Exit(code)
}

// execOpencode is the production executor: runs the real opencode binary.
// Uses --format json and extracts only "text" event parts so that tool calls,
// step markers, and other internal events never reach the caller's writer.
func execOpencode(model, prompt string, stdout, stderr io.Writer) int {
	pr, pw, err := os.Pipe()
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}

	cmd := exec.Command("opencode", "run", "--model", model, "--format", "json", "--", prompt)
	cmd.Stdout = pw
	cmd.Stderr = stderr

	if err := cmd.Start(); err != nil {
		pw.Close()
		pr.Close()
		fmt.Fprintln(stderr, err)
		return 1
	}

	pw.Close() // parent no longer writes; child holds the write end

	// Stream only text content from the JSON event line.
	scanner := bufio.NewScanner(pr)
	for scanner.Scan() {
		var event struct {
			Type string `json:"type"`
			Part struct {
				Text string `json:"text"`
			} `json:"part"`
		}
		if json.Unmarshal(scanner.Bytes(), &event) == nil && event.Type == "text" && event.Part.Text != "" {
			fmt.Fprint(stdout, event.Part.Text)
		}
	}
	pr.Close()

	if err := cmd.Wait(); err != nil {
		if exit, ok := err.(*exec.ExitError); ok {
			return exit.ExitCode()
		}
		fmt.Fprintln(stderr, err)
		return 1
	}
	return 0
}

// run is the testable entry point.
// executor is injectable so tests can replace opencode with a stub.
func run(
	args []string,
	stdin io.Reader,
	stdout io.Writer,
	stderr io.Writer,
	getenv func(string) string,
	executor func(model, prompt string, stdout, stderr io.Writer) int,
) (int, error) {
	fs := flag.NewFlagSet("orcai-opencode", flag.ContinueOnError)
	fs.SetOutput(stderr)
	modelFlag := fs.String("model", "", "Model in provider/model format (e.g. ollama/llama3.2)")
	if err := fs.Parse(args); err != nil {
		return 1, nil // flag already wrote the error
	}

	// Read prompt from stdin.
	promptBytes, err := io.ReadAll(stdin)
	if err != nil {
		return 1, fmt.Errorf("reading stdin: %w", err)
	}
	prompt := strings.TrimSpace(string(promptBytes))
	if prompt == "" {
		return 1, fmt.Errorf("prompt is required: no input received on stdin")
	}

	// Resolve model: --model flag takes precedence over GLITCH_MODEL env var.
	model := *modelFlag
	if model == "" {
		model = getenv("GLITCH_MODEL")
	}
	if model == "" {
		return 1, fmt.Errorf("model is required: set --model flag or GLITCH_MODEL environment variable")
	}

	// For Ollama-backed models, pull the model if not already present.
	if err := pullOllamaModel(model, stderr); err != nil {
		return 1, fmt.Errorf("pulling Ollama model: %w", err)
	}

	// opencode is an agentic CLI with native tool access, so we don't need
	// the input protocol (GLITCH_WRITE/RUN). We DO need the output protocol
	// so its replies land in the chat as structured blocks instead of raw
	// stdout the splitter has to guess at.
	fullPrompt := glitchctx.OutputProtocolInstructions +
		glitchctx.BuildShellContext() +
		"\n## User Request\n" + prompt
	code := executor(model, fullPrompt, stdout, stderr)
	return code, nil
}
