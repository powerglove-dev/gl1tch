package main

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

// TestMain overrides isModelPresentFn so unit tests that use a mock HTTP
// server don't attempt to run the real `ollama` binary.
func TestMain(m *testing.M) {
	isModelPresentFn = func(string) bool { return true }
	os.Exit(m.Run())
}

// helper: no-op env resolver
func noEnv(key string) string { return "" }

// helper: env resolver from map
func envMap(m map[string]string) func(string) string {
	return func(key string) string { return m[key] }
}

// --- run() unit tests ---

func TestRun_EmptyStdin(t *testing.T) {
	var stdout, stderr bytes.Buffer
	err := run([]string{"--model", "llama3.2"}, strings.NewReader("   "), &stdout, &stderr, noEnv)
	if err == nil {
		t.Fatal("expected error for empty stdin, got nil")
	}
	if !strings.Contains(err.Error(), "stdin") && !strings.Contains(err.Error(), "input") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestRun_MissingModel(t *testing.T) {
	// Force the "no model anywhere" path by stubbing out the
	// first-installed-model fallback so it returns "". Otherwise the test
	// would pick up whatever the host machine has installed via `ollama list`.
	prev := firstInstalledModelFn
	firstInstalledModelFn = func() string { return "" }
	defer func() { firstInstalledModelFn = prev }()

	var stdout, stderr bytes.Buffer
	// Neither --model flag nor GLITCH_MODEL env var is set, and there are
	// no locally installed models to fall back to.
	err := run([]string{}, strings.NewReader("tell me a joke"), &stdout, &stderr, noEnv)
	if err == nil {
		t.Fatal("expected error when model is not set, got nil")
	}
	if !strings.Contains(err.Error(), "no model available") {
		t.Errorf("expected 'no model available' in error message, got: %v", err)
	}
}

func TestRun_ModelFromFlag(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req generateRequest
		json.NewDecoder(r.Body).Decode(&req)
		json.NewEncoder(w).Encode(generateResponse{Response: "got model: " + req.Model})
	}))
	defer srv.Close()

	var stdout, stderr bytes.Buffer
	err := run(
		[]string{"--model", "llama3.2"},
		strings.NewReader("ping"),
		&stdout, &stderr,
		envMap(map[string]string{"GLITCH_OLLAMA_URL": srv.URL}),
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(stdout.String(), "llama3.2") {
		t.Errorf("expected llama3.2 in output, got %q", stdout.String())
	}
}

func TestRun_ModelFromEnv(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req generateRequest
		json.NewDecoder(r.Body).Decode(&req)
		json.NewEncoder(w).Encode(generateResponse{Response: "got model: " + req.Model})
	}))
	defer srv.Close()

	var stdout, stderr bytes.Buffer
	err := run(
		[]string{},
		strings.NewReader("ping"),
		&stdout, &stderr,
		envMap(map[string]string{
			"GLITCH_MODEL":      "qwen2.5",
			"GLITCH_OLLAMA_URL": srv.URL,
		}),
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(stdout.String(), "qwen2.5") {
		t.Errorf("expected qwen2.5 in output, got %q", stdout.String())
	}
}

func TestRun_DefaultURL(t *testing.T) {
	// Verify that when GLITCH_OLLAMA_URL is absent, the default resolves to localhost:11434.
	// We test this by overriding the URL with a test server to avoid needing real Ollama.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(generateResponse{Response: "ok"})
	}))
	defer srv.Close()

	var stdout, stderr bytes.Buffer
	err := run(
		[]string{"--model", "llama3.2"},
		strings.NewReader("hi"),
		&stdout, &stderr,
		envMap(map[string]string{"GLITCH_OLLAMA_URL": srv.URL}),
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// --- --option flag tests ---

func TestRun_OptionForwardedAsInt(t *testing.T) {
	var capturedReq generateRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&capturedReq)
		json.NewEncoder(w).Encode(generateResponse{Response: "ok"})
	}))
	defer srv.Close()

	var stdout, stderr bytes.Buffer
	err := run(
		[]string{"--model", "llama3.2", "--option", "num_ctx=16384"},
		strings.NewReader("ping"),
		&stdout, &stderr,
		envMap(map[string]string{"GLITCH_OLLAMA_URL": srv.URL}),
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if capturedReq.Options == nil {
		t.Fatal("expected options in request, got nil")
	}
	// JSON numbers unmarshal as float64 when decoded into map[string]any.
	got, ok := capturedReq.Options["num_ctx"]
	if !ok {
		t.Fatal("expected num_ctx in options")
	}
	if got != float64(16384) {
		t.Errorf("expected num_ctx=16384, got %v (%T)", got, got)
	}
}

func TestRun_MultipleOptionsForwarded(t *testing.T) {
	var capturedReq generateRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&capturedReq)
		json.NewEncoder(w).Encode(generateResponse{Response: "ok"})
	}))
	defer srv.Close()

	var stdout, stderr bytes.Buffer
	err := run(
		[]string{"--model", "llama3.2", "--option", "num_ctx=16384", "--option", "temperature=0"},
		strings.NewReader("ping"),
		&stdout, &stderr,
		envMap(map[string]string{"GLITCH_OLLAMA_URL": srv.URL}),
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if capturedReq.Options == nil {
		t.Fatal("expected options in request, got nil")
	}
	if _, ok := capturedReq.Options["num_ctx"]; !ok {
		t.Error("expected num_ctx in options")
	}
	if _, ok := capturedReq.Options["temperature"]; !ok {
		t.Error("expected temperature in options")
	}
}

func TestRun_NoOptionsAbsentFromRequest(t *testing.T) {
	var capturedBody []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		buf := new(bytes.Buffer)
		buf.ReadFrom(r.Body)
		capturedBody = buf.Bytes()
		json.NewEncoder(w).Encode(generateResponse{Response: "ok"})
	}))
	defer srv.Close()

	var stdout, stderr bytes.Buffer
	err := run(
		[]string{"--model", "llama3.2"},
		strings.NewReader("ping"),
		&stdout, &stderr,
		envMap(map[string]string{"GLITCH_OLLAMA_URL": srv.URL}),
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if strings.Contains(string(capturedBody), `"options"`) {
		t.Errorf("expected options to be absent from request, got: %s", capturedBody)
	}
}

func TestRun_StringOptionPreserved(t *testing.T) {
	var capturedReq generateRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&capturedReq)
		json.NewEncoder(w).Encode(generateResponse{Response: "ok"})
	}))
	defer srv.Close()

	var stdout, stderr bytes.Buffer
	err := run(
		[]string{"--model", "llama3.2", "--option", "stop=</s>"},
		strings.NewReader("ping"),
		&stdout, &stderr,
		envMap(map[string]string{"GLITCH_OLLAMA_URL": srv.URL}),
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got, ok := capturedReq.Options["stop"]
	if !ok {
		t.Fatal("expected stop in options")
	}
	if got != "</s>" {
		t.Errorf("expected stop=</s>, got %v", got)
	}
}

// --- --create-model tests ---

func TestRun_CreateModel_Success(t *testing.T) {
	var gotBase, gotName string
	var gotOptions map[string]any
	createModelFn = func(base, name string, options map[string]any, stdout, stderr io.Writer) error {
		gotBase = base
		gotName = name
		gotOptions = options
		_, err := stdout.Write([]byte("Created model '" + name + "'\n"))
		return err
	}
	t.Cleanup(func() { createModelFn = createModel })

	var stdout, stderr bytes.Buffer
	err := run(
		[]string{"--model", "llama3.2", "--option", "num_ctx=16384", "--create-model", "llama3.2-16k"},
		strings.NewReader(""), // stdin not used for create-model
		&stdout, &stderr,
		noEnv,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotBase != "llama3.2" {
		t.Errorf("base: got %q, want %q", gotBase, "llama3.2")
	}
	if gotName != "llama3.2-16k" {
		t.Errorf("name: got %q, want %q", gotName, "llama3.2-16k")
	}
	if gotOptions["num_ctx"] != 16384 {
		t.Errorf("options num_ctx: got %v, want 16384", gotOptions["num_ctx"])
	}
	if !strings.Contains(stdout.String(), "llama3.2-16k") {
		t.Errorf("expected model name in stdout, got %q", stdout.String())
	}
}

func TestRun_CreateModel_MissingBaseModel(t *testing.T) {
	var stdout, stderr bytes.Buffer
	err := run(
		[]string{"--create-model", "llama3.2-16k"},
		strings.NewReader(""),
		&stdout, &stderr,
		noEnv,
	)
	if err == nil {
		t.Fatal("expected error when --model is missing for --create-model")
	}
	if !strings.Contains(err.Error(), "model") {
		t.Errorf("expected 'model' in error, got: %v", err)
	}
}

func TestRun_CreateModel_SkipsInference(t *testing.T) {
	inferenceHit := false
	createModelFn = func(base, name string, options map[string]any, stdout, stderr io.Writer) error {
		stdout.Write([]byte("Created model '" + name + "'\n")) //nolint
		return nil
	}
	t.Cleanup(func() { createModelFn = createModel })

	// Track whether the HTTP server is hit (it shouldn't be).
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		inferenceHit = true
		json.NewEncoder(w).Encode(generateResponse{Response: "should not be called"})
	}))
	defer srv.Close()

	var stdout, stderr bytes.Buffer
	err := run(
		[]string{"--model", "llama3.2", "--create-model", "llama3.2-16k"},
		strings.NewReader(""),
		&stdout, &stderr,
		envMap(map[string]string{"GLITCH_OLLAMA_URL": srv.URL}),
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if inferenceHit {
		t.Error("expected inference to be skipped when --create-model is set")
	}
}

// --- parseOptions unit tests ---

func TestParseOptions_Empty(t *testing.T) {
	opts, err := parseOptions(nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if opts != nil {
		t.Errorf("expected nil options, got %v", opts)
	}
}

func TestParseOptions_IntCoercion(t *testing.T) {
	opts, err := parseOptions([]string{"num_ctx=16384"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if opts["num_ctx"] != 16384 {
		t.Errorf("expected int 16384, got %v (%T)", opts["num_ctx"], opts["num_ctx"])
	}
}

func TestParseOptions_StringPreserved(t *testing.T) {
	opts, err := parseOptions([]string{"stop=</s>"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if opts["stop"] != "</s>" {
		t.Errorf("expected string </s>, got %v", opts["stop"])
	}
}

func TestParseOptions_InvalidFormat(t *testing.T) {
	_, err := parseOptions([]string{"noequals"})
	if err == nil {
		t.Fatal("expected error for missing '=', got nil")
	}
}

// --- callOllama() unit tests ---

func TestCallOllama_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.URL.Path != "/api/generate" {
			t.Errorf("expected /api/generate, got %s", r.URL.Path)
		}

		var req generateRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("decode request: %v", err)
		}
		if req.Stream {
			t.Error("expected stream=false")
		}
		if req.Model != "llama3.2" {
			t.Errorf("expected model llama3.2, got %q", req.Model)
		}

		json.NewEncoder(w).Encode(generateResponse{Response: "It works!"})
	}))
	defer srv.Close()

	result, err := callOllama(srv.URL, "llama3.2", "say hello", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Response != "It works!" {
		t.Errorf("expected 'It works!', got %q", result.Response)
	}
}

func TestCallOllama_Non2xxError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte(`{"error":"model not found"}`))
	}))
	defer srv.Close()

	_, err := callOllama(srv.URL, "nonexistent-model", "hello", nil)
	if err == nil {
		t.Fatal("expected error for 404, got nil")
	}
	if !strings.Contains(err.Error(), "404") {
		t.Errorf("expected 404 in error message, got: %v", err)
	}
}

func TestCallOllama_ConnectionRefused(t *testing.T) {
	_, err := callOllama("http://127.0.0.1:19999", "llama3.2", "hello", nil)
	if err == nil {
		t.Fatal("expected error for connection refused, got nil")
	}
	if !strings.Contains(err.Error(), "19999") && !strings.Contains(err.Error(), "connecting") {
		t.Errorf("expected connection error, got: %v", err)
	}
}
