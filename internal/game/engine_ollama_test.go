package game

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// ollamaResponse builds a non-streaming Ollama /api/chat response body.
func ollamaResponse(content string) []byte {
	resp := map[string]any{
		"model": "llama3.2",
		"message": map[string]string{
			"role":    "assistant",
			"content": content,
		},
	}
	b, _ := json.Marshal(resp)
	return b
}

func TestEvaluate_SuccessFirstAttempt(t *testing.T) {
	validJSON := `{"achievements":["ghost-runner"],"ice_class":null,"quest_events":[]}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(ollamaResponse(validJSON))
	}))
	defer srv.Close()

	engine := &GameEngine{baseURL: srv.URL, model: "llama3.2", client: &http.Client{}}
	pack := DefaultWorldPackLoader{}.ActivePack()
	result, err := engine.Evaluate(context.Background(), `{"xp":100}`, pack)
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	if len(result.Achievements) != 1 || result.Achievements[0] != "ghost-runner" {
		t.Errorf("Achievements = %v, want [ghost-runner]", result.Achievements)
	}
}

func TestEvaluate_RetryOnBadJSON(t *testing.T) {
	callCount := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.Header().Set("Content-Type", "application/json")
		if callCount == 1 {
			// First call returns malformed JSON.
			w.Write(ollamaResponse("This is not JSON at all."))
		} else {
			// Second call returns valid JSON.
			w.Write(ollamaResponse(`{"achievements":["cache-warlock"],"ice_class":"black-ice","quest_events":[]}`))
		}
	}))
	defer srv.Close()

	engine := &GameEngine{baseURL: srv.URL, model: "llama3.2", client: &http.Client{}}
	pack := DefaultWorldPackLoader{}.ActivePack()
	result, err := engine.Evaluate(context.Background(), `{}`, pack)
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	if callCount != 2 {
		t.Errorf("expected 2 calls (retry), got %d", callCount)
	}
	if len(result.Achievements) != 1 || result.Achievements[0] != "cache-warlock" {
		t.Errorf("Achievements = %v, want [cache-warlock]", result.Achievements)
	}
}

func TestEvaluate_GracefulFailureOnBothBadJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(ollamaResponse("still not json"))
	}))
	defer srv.Close()

	engine := &GameEngine{baseURL: srv.URL, model: "llama3.2", client: &http.Client{}}
	pack := DefaultWorldPackLoader{}.ActivePack()
	result, err := engine.Evaluate(context.Background(), `{}`, pack)
	if err != nil {
		t.Fatalf("Evaluate should not return error on double failure: %v", err)
	}
	// Should return empty result.
	if len(result.Achievements) != 0 {
		t.Errorf("expected empty achievements on failure, got %v", result.Achievements)
	}
}

func TestEvaluate_OllamaUnavailable(t *testing.T) {
	// Point to a port where nothing is listening.
	engine := &GameEngine{baseURL: "http://127.0.0.1:19999", model: "llama3.2", client: &http.Client{}}
	pack := DefaultWorldPackLoader{}.ActivePack()
	result, err := engine.Evaluate(context.Background(), `{}`, pack)
	// Must not return error — game is optional.
	if err != nil {
		t.Fatalf("Evaluate should not return error when Ollama unavailable: %v", err)
	}
	if len(result.Achievements) != 0 {
		t.Errorf("expected empty achievements when Ollama unavailable, got %v", result.Achievements)
	}
}

func TestNarrate_ReturnsText(t *testing.T) {
	narration := "You jacked in clean. The Gibson recognized your token efficiency."
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(ollamaResponse(narration))
	}))
	defer srv.Close()

	engine := &GameEngine{baseURL: srv.URL, model: "llama3.2", client: &http.Client{}}
	pack := DefaultWorldPackLoader{}.ActivePack()
	got := engine.Narrate(context.Background(), `{}`, EvaluateResult{}, pack)
	if got != narration {
		t.Errorf("Narrate = %q, want %q", got, narration)
	}
}

func TestNarrate_OllamaUnavailable(t *testing.T) {
	engine := &GameEngine{baseURL: "http://127.0.0.1:19999", model: "llama3.2", client: &http.Client{}}
	pack := DefaultWorldPackLoader{}.ActivePack()
	got := engine.Narrate(context.Background(), `{}`, EvaluateResult{}, pack)
	if got != "" {
		t.Errorf("Narrate should return empty string on error, got %q", got)
	}
}

func TestParseEvaluateResult_MarkdownFence(t *testing.T) {
	content := "```json\n{\"achievements\":[\"speed-demon\"],\"ice_class\":null,\"quest_events\":[]}\n```"
	var result EvaluateResult
	if err := parseEvaluateResult(content, &result); err != nil {
		t.Fatalf("parseEvaluateResult with fence: %v", err)
	}
	if len(result.Achievements) != 1 || result.Achievements[0] != "speed-demon" {
		t.Errorf("Achievements = %v, want [speed-demon]", result.Achievements)
	}
}
