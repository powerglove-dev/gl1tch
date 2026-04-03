// gl1tch-ollama is the gl1tch sidecar binary for the Ollama provider.
//
// Sidecar protocol (gl1tch runner contract):
//   - Prompt text is written to stdin.
//   - --model <name> selects the Ollama model; falls back to GLITCH_MODEL env var.
//   - Response is streamed to stdout.
//   - --list-models prints installed models as JSON and exits.
//
// Usage in a pipeline step:
//
//	- id: summarize
//	  executor: ollama
//	  model: qwen2.5:latest
//	  prompt: "Summarize: {{steps.fetch.output}}"
package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
)

func main() {
	model := flag.String("model", "", "Ollama model name (e.g. qwen2.5:latest)")
	listModels := flag.Bool("list-models", false, "print installed models as JSON and exit")
	flag.Parse()

	baseURL := os.Getenv("GLITCH_OLLAMA_URL")
	if baseURL == "" {
		baseURL = "http://localhost:11434"
	}

	if *listModels {
		if err := printModels(baseURL); err != nil {
			fmt.Fprintln(os.Stderr, "gl1tch-ollama:", err)
			os.Exit(1)
		}
		return
	}

	m := *model
	if m == "" {
		m = os.Getenv("GLITCH_MODEL")
	}
	if m == "" {
		fmt.Fprintln(os.Stderr, "gl1tch-ollama: --model or GLITCH_MODEL required")
		os.Exit(1)
	}

	prompt, err := io.ReadAll(os.Stdin)
	if err != nil {
		fmt.Fprintln(os.Stderr, "gl1tch-ollama: read stdin:", err)
		os.Exit(1)
	}

	if err := generate(baseURL, m, string(prompt)); err != nil {
		fmt.Fprintln(os.Stderr, "gl1tch-ollama:", err)
		os.Exit(1)
	}
}

func generate(baseURL, model, prompt string) error {
	payload, _ := json.Marshal(map[string]any{
		"model":  model,
		"prompt": prompt,
		"stream": true,
	})
	resp, err := http.Post(baseURL+"/api/generate", "application/json", bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("connect to ollama at %s: %w", baseURL, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("ollama returned %d: %s", resp.StatusCode, bytes.TrimSpace(body))
	}

	dec := json.NewDecoder(resp.Body)
	for {
		var chunk struct {
			Response string `json:"response"`
			Done     bool   `json:"done"`
			Error    string `json:"error"`
		}
		if err := dec.Decode(&chunk); err == io.EOF {
			break
		} else if err != nil {
			return fmt.Errorf("decode stream: %w", err)
		}
		if chunk.Error != "" {
			return fmt.Errorf("ollama: %s", chunk.Error)
		}
		fmt.Print(chunk.Response)
		if chunk.Done {
			break
		}
	}
	fmt.Println()
	return nil
}

// printModels fetches the list of installed Ollama models and prints them
// as JSON in the format gl1tch expects: [{"id":"...","label":"..."}].
func printModels(baseURL string) error {
	resp, err := http.Get(baseURL + "/api/tags")
	if err != nil {
		// Not running — print empty list, not an error.
		fmt.Println("[]")
		return nil
	}
	defer resp.Body.Close()

	var result struct {
		Models []struct {
			Name string `json:"name"`
		} `json:"models"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		fmt.Println("[]")
		return nil
	}

	type entry struct {
		ID    string `json:"id"`
		Label string `json:"label"`
	}
	entries := make([]entry, 0, len(result.Models))
	for _, m := range result.Models {
		entries = append(entries, entry{ID: m.Name, Label: m.Name})
	}
	return json.NewEncoder(os.Stdout).Encode(entries)
}
