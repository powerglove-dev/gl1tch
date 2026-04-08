package capability

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// fakeOllama is a tiny httptest server that mimics /api/chat. Tests inject
// a canned reply by setting reply before issuing requests; the handler
// returns it in the Ollama non-stream response shape.
type fakeOllama struct {
	srv   *httptest.Server
	reply string
	// lastPrompt captures the user-turn content from the most recent
	// request so tests can assert on what the router sent to the model.
	lastPrompt string
}

func newFakeOllama(t *testing.T) *fakeOllama {
	f := &fakeOllama{}
	f.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/chat" {
			http.NotFound(w, r)
			return
		}
		var req struct {
			Messages []struct {
				Role    string `json:"role"`
				Content string `json:"content"`
			} `json:"messages"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		for _, m := range req.Messages {
			if m.Role == "user" {
				f.lastPrompt = m.Content
			}
		}
		resp := map[string]any{
			"message": map[string]string{
				"role":    "assistant",
				"content": f.reply,
			},
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	t.Cleanup(f.srv.Close)
	return f
}

// onDemandCap is a test capability with Trigger.Mode=OnDemand that echoes
// its Stdin input as a Stream event. Used to verify end-to-end routing:
// router → runner → capability → output writer.
type onDemandCap struct {
	name string
	desc string
}

func (o *onDemandCap) Manifest() Manifest {
	return Manifest{
		Name:        o.name,
		Description: o.desc,
		Trigger:     Trigger{Mode: TriggerOnDemand},
		Sink:        Sink{Stream: true},
	}
}

func (o *onDemandCap) Invoke(_ context.Context, in Input) (<-chan Event, error) {
	ch := make(chan Event, 1)
	ch <- Event{Kind: EventStream, Text: "got: " + in.Stdin}
	close(ch)
	return ch, nil
}

func newRoutedSetup(t *testing.T, caps []Capability, reply string) (*Router, *fakeOllama) {
	reg := NewRegistry()
	for _, c := range caps {
		if err := reg.Register(c); err != nil {
			t.Fatalf("register: %v", err)
		}
	}
	runner := NewRunner(reg, nil)
	fake := newFakeOllama(t)
	fake.reply = reply
	router := NewRouter(reg, runner)
	router.BaseURL = fake.srv.URL
	router.HTTPClient = fake.srv.Client()
	return router, fake
}

func TestRouter_PickFiltersOnDemandOnly(t *testing.T) {
	// Mix of trigger modes in the registry. Only the on-demand ones
	// should appear in the prompt the router builds.
	caps := []Capability{
		&onDemandCap{name: "ask", desc: "answer general questions"},
		&onDemandCap{name: "summarize", desc: "summarize a passage"},
		// Background workers — must NOT be offered to the model.
		&fakeDaemon{name: "claude"},
		&fakeInterval{name: "workspace"},
	}
	router, fake := newRoutedSetup(t, caps, "ask")

	name, err := router.Pick(context.Background(), "what's the weather")
	if err != nil {
		t.Fatalf("Pick: %v", err)
	}
	if name != "ask" {
		t.Errorf("name = %q, want ask", name)
	}
	if strings.Contains(fake.lastPrompt, "claude") {
		t.Errorf("prompt leaked daemon capability: %q", fake.lastPrompt)
	}
	if strings.Contains(fake.lastPrompt, "workspace") {
		t.Errorf("prompt leaked interval capability: %q", fake.lastPrompt)
	}
	if !strings.Contains(fake.lastPrompt, "ask") {
		t.Errorf("prompt missing candidate: %q", fake.lastPrompt)
	}
}

func TestRouter_ParsesVerboseModelReply(t *testing.T) {
	// Even when told to be terse, 7B models sometimes wrap the answer
	// in a sentence or add quotes. parsePickedName has to handle the
	// common shapes or the router will never route anything in
	// practice. The cases below are all real shapes observed from
	// qwen2.5:7b during dev.
	caps := []Capability{
		&onDemandCap{name: "summarize", desc: "summarize"},
	}
	cases := []struct {
		reply string
		want  string
	}{
		{"summarize", "summarize"},
		{"\"summarize\"", "summarize"},
		{"summarize.", "summarize"},
		{"Capability name: summarize", "summarize"},
		{"Based on the message, the answer is:\nsummarize", "summarize"},
		{"- summarize", "summarize"},
		{"Summarize", "summarize"}, // case-insensitive
		{"summarize is the best choice", "summarize"},
	}
	for _, tc := range cases {
		t.Run(tc.reply, func(t *testing.T) {
			router, _ := newRoutedSetup(t, caps, tc.reply)
			got, err := router.Pick(context.Background(), "summarize this")
			if err != nil {
				t.Fatalf("Pick: %v", err)
			}
			if got != tc.want {
				t.Errorf("Pick(%q) = %q, want %q", tc.reply, got, tc.want)
			}
		})
	}
}

func TestRouter_NoneReturnsErrNoMatch(t *testing.T) {
	caps := []Capability{
		&onDemandCap{name: "ask", desc: "answer questions"},
	}
	router, _ := newRoutedSetup(t, caps, "none")
	_, err := router.Pick(context.Background(), "hello")
	if !errors.Is(err, ErrNoMatch) {
		t.Fatalf("got %v, want ErrNoMatch", err)
	}
}

func TestRouter_HallucinatedNameReturnsErrNoMatch(t *testing.T) {
	// If the model returns a name that is not in the registry, we
	// MUST return ErrNoMatch — fuzzy-matching hallucinations would be
	// a nasty debugging surface.
	caps := []Capability{
		&onDemandCap{name: "ask", desc: "answer questions"},
	}
	router, _ := newRoutedSetup(t, caps, "translate")
	_, err := router.Pick(context.Background(), "what")
	if !errors.Is(err, ErrNoMatch) {
		t.Fatalf("got %v, want ErrNoMatch", err)
	}
}

func TestRouter_EmptyRegistryReturnsErrNoMatch(t *testing.T) {
	router, _ := newRoutedSetup(t, nil, "ask")
	_, err := router.Pick(context.Background(), "hello")
	if !errors.Is(err, ErrNoMatch) {
		t.Fatalf("got %v, want ErrNoMatch", err)
	}
}

func TestRouter_RouteInvokesPickedCapability(t *testing.T) {
	caps := []Capability{
		&onDemandCap{name: "echo", desc: "echo the input back"},
	}
	router, _ := newRoutedSetup(t, caps, "echo")
	var buf bytes.Buffer
	name, err := router.Route(context.Background(), "hello router", &buf)
	if err != nil {
		t.Fatalf("Route: %v", err)
	}
	if name != "echo" {
		t.Fatalf("name = %q, want echo", name)
	}
	if got := buf.String(); got != "got: hello router" {
		t.Errorf("stream output = %q, want %q", got, "got: hello router")
	}
}

func TestRouter_OllamaError(t *testing.T) {
	reg := NewRegistry()
	reg.Register(&onDemandCap{name: "ask", desc: "ask"})
	runner := NewRunner(reg, nil)
	// Server that always 500s.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	defer srv.Close()
	router := NewRouter(reg, runner)
	router.BaseURL = srv.URL
	router.HTTPClient = srv.Client()

	_, err := router.Pick(context.Background(), "hi")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "ollama") {
		t.Errorf("error = %v, want something mentioning ollama", err)
	}
}

// fakeDaemon / fakeInterval are minimal capabilities used by router tests
// to verify the candidate filter. They never get invoked.
type fakeDaemon struct{ name string }

func (f *fakeDaemon) Manifest() Manifest {
	return Manifest{Name: f.name, Trigger: Trigger{Mode: TriggerDaemon}}
}
func (f *fakeDaemon) Invoke(_ context.Context, _ Input) (<-chan Event, error) {
	ch := make(chan Event)
	close(ch)
	return ch, nil
}

type fakeInterval struct{ name string }

func (f *fakeInterval) Manifest() Manifest {
	return Manifest{Name: f.name, Trigger: Trigger{Mode: TriggerInterval, Every: 0}}
}
func (f *fakeInterval) Invoke(_ context.Context, _ Input) (<-chan Event, error) {
	ch := make(chan Event)
	close(ch)
	return ch, nil
}

// Ensure the package imports io for the _ usage marker below.
var _ = io.Discard
