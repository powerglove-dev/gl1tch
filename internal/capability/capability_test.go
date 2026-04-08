package capability

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

// fakeIndexer captures every BulkIndex call so tests can assert routing.
type fakeIndexer struct {
	mu   sync.Mutex
	docs map[string][]any
	err  error
}

func (f *fakeIndexer) BulkIndex(_ context.Context, idx string, docs []any) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.err != nil {
		return f.err
	}
	if f.docs == nil {
		f.docs = make(map[string][]any)
	}
	f.docs[idx] = append(f.docs[idx], docs...)
	return nil
}

func (f *fakeIndexer) count(idx string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.docs[idx])
}

// fakeCap emits a fixed list of events on Invoke.
type fakeCap struct {
	manifest Manifest
	events   []Event
	calls    int
	mu       sync.Mutex
}

func (f *fakeCap) Manifest() Manifest { return f.manifest }

func (f *fakeCap) Invoke(_ context.Context, _ Input) (<-chan Event, error) {
	f.mu.Lock()
	f.calls++
	f.mu.Unlock()
	ch := make(chan Event, len(f.events))
	for _, e := range f.events {
		ch <- e
	}
	close(ch)
	return ch, nil
}

func (f *fakeCap) callCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls
}

func TestRegistry_RegisterAndGet(t *testing.T) {
	r := NewRegistry()
	c := &fakeCap{manifest: Manifest{Name: "alpha"}}
	if err := r.Register(c); err != nil {
		t.Fatalf("register: %v", err)
	}
	if err := r.Register(c); err == nil {
		t.Fatalf("expected duplicate registration to fail")
	}
	got, ok := r.Get("alpha")
	if !ok {
		t.Fatalf("get: not found")
	}
	if got.Manifest().Name != "alpha" {
		t.Fatalf("get: name = %q", got.Manifest().Name)
	}
	names := r.Names()
	if len(names) != 1 || names[0] != "alpha" {
		t.Fatalf("names = %v", names)
	}
}

func TestRegistry_RejectsEmptyName(t *testing.T) {
	r := NewRegistry()
	if err := r.Register(&fakeCap{}); err == nil {
		t.Fatalf("expected empty-name registration to fail")
	}
}

func TestRunner_OnDemand_RoutesDocsAndStream(t *testing.T) {
	r := NewRegistry()
	idx := &fakeIndexer{}
	c := &fakeCap{
		manifest: Manifest{
			Name:    "mixed",
			Trigger: Trigger{Mode: TriggerOnDemand},
			Sink:    Sink{Index: true, Stream: true},
		},
		events: []Event{
			{Kind: EventDoc, Doc: map[string]any{"a": 1}},
			{Kind: EventStream, Text: "hello "},
			{Kind: EventDoc, Doc: map[string]any{"a": 2}},
			{Kind: EventStream, Text: "world"},
		},
	}
	if err := r.Register(c); err != nil {
		t.Fatalf("register: %v", err)
	}
	runner := NewRunner(r, idx)

	var buf bytes.Buffer
	if err := runner.Invoke(context.Background(), "mixed", Input{}, &buf); err != nil {
		t.Fatalf("invoke: %v", err)
	}

	if got, want := buf.String(), "hello world"; got != want {
		t.Fatalf("stream: got %q want %q", got, want)
	}
	if n := idx.count("glitch-events"); n != 2 {
		t.Fatalf("indexed docs: got %d want 2", n)
	}
}

func TestRunner_AfterInvokeHookFires(t *testing.T) {
	r := NewRegistry()
	idx := &fakeIndexer{}
	c := &fakeCap{
		manifest: Manifest{
			Name:    "hooked",
			Trigger: Trigger{Mode: TriggerOnDemand},
			Sink:    Sink{Index: true},
		},
		events: []Event{
			{Kind: EventDoc, Doc: map[string]any{"a": 1}},
			{Kind: EventDoc, Doc: map[string]any{"a": 2}},
		},
	}
	r.Register(c)
	runner := NewRunner(r, idx)

	var (
		gotName    string
		gotIndexed int
		gotErr     error
		fired      int
	)
	runner.SetAfterInvoke(func(name string, _ time.Duration, n int, err error) {
		fired++
		gotName = name
		gotIndexed = n
		gotErr = err
	})

	if err := runner.Invoke(context.Background(), "hooked", Input{}, nil); err != nil {
		t.Fatalf("invoke: %v", err)
	}
	if fired != 1 {
		t.Fatalf("hook fired %d times, want 1", fired)
	}
	if gotName != "hooked" {
		t.Fatalf("hook name = %q", gotName)
	}
	if gotIndexed != 2 {
		t.Fatalf("hook indexed = %d, want 2", gotIndexed)
	}
	if gotErr != nil {
		t.Fatalf("hook err = %v", gotErr)
	}
}

func TestRunner_NotFound(t *testing.T) {
	runner := NewRunner(NewRegistry(), nil)
	err := runner.Invoke(context.Background(), "nope", Input{}, nil)
	var nfe *NotFoundError
	if !errors.As(err, &nfe) {
		t.Fatalf("expected NotFoundError, got %v", err)
	}
}

func TestRunner_DocsRoutedToCustomIndex(t *testing.T) {
	r := NewRegistry()
	idx := &fakeIndexer{}
	c := &fakeCap{
		manifest: Manifest{
			Name:    "custom-idx",
			Trigger: Trigger{Mode: TriggerOnDemand},
			Sink:    Sink{Index: true},
			Invocation: Invocation{
				Index: "glitch-pipelines",
			},
		},
		events: []Event{
			{Kind: EventDoc, Doc: map[string]any{"x": 1}},
			{Kind: EventDoc, Doc: map[string]any{"x": 2}, Index: "glitch-summaries"},
		},
	}
	r.Register(c)
	runner := NewRunner(r, idx)
	if err := runner.Invoke(context.Background(), "custom-idx", Input{}, nil); err != nil {
		t.Fatalf("invoke: %v", err)
	}
	if n := idx.count("glitch-pipelines"); n != 1 {
		t.Fatalf("pipelines index: got %d want 1", n)
	}
	if n := idx.count("glitch-summaries"); n != 1 {
		t.Fatalf("summaries index: got %d want 1", n)
	}
}

func TestRunner_IntervalSchedulesInvocations(t *testing.T) {
	r := NewRegistry()
	idx := &fakeIndexer{}
	c := &fakeCap{
		manifest: Manifest{
			Name:    "ticker",
			Trigger: Trigger{Mode: TriggerInterval, Every: 20 * time.Millisecond},
			Sink:    Sink{Index: true},
		},
		events: []Event{
			{Kind: EventDoc, Doc: map[string]any{"n": 1}},
		},
	}
	r.Register(c)
	runner := NewRunner(r, idx)

	ctx, cancel := context.WithCancel(context.Background())
	runner.Start(ctx)
	// Initial run + at least two ticks within 80ms.
	time.Sleep(80 * time.Millisecond)
	cancel()
	runner.Stop()

	if got := c.callCount(); got < 3 {
		t.Fatalf("expected at least 3 invocations, got %d", got)
	}
	if got := idx.count("glitch-events"); got < 3 {
		t.Fatalf("expected at least 3 indexed docs, got %d", got)
	}
}

func TestSkillLoader_ParsesFrontmatterAndBody(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "git-recent.md")
	content := `---
name: git.recent
category: vcs
trigger:
  mode: interval
  every: 5m
sink:
  index: true
invoke:
  command: git
  args: [log, --oneline, -n, "10"]
  parser: pipe-lines
  fields: [sha, message]
  index: glitch-events
  doc_type: git.commit
---

# git.recent

Indexes the most recent commits in the current repo. Use this when the user
asks about recent code changes.
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	c, err := LoadSkill(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	m := c.Manifest()
	if m.Name != "git.recent" {
		t.Fatalf("name = %q", m.Name)
	}
	if m.Category != "vcs" {
		t.Fatalf("category = %q", m.Category)
	}
	if m.Trigger.Mode != TriggerInterval {
		t.Fatalf("trigger mode = %q", m.Trigger.Mode)
	}
	if m.Trigger.Every != 5*time.Minute {
		t.Fatalf("trigger every = %v", m.Trigger.Every)
	}
	if !m.Sink.Index {
		t.Fatalf("sink.index = false")
	}
	if m.Invocation.Command != "git" {
		t.Fatalf("invoke.command = %q", m.Invocation.Command)
	}
	if m.Invocation.Parser != ParserPipeLines {
		t.Fatalf("parser = %q", m.Invocation.Parser)
	}
	if len(m.Invocation.Fields) != 2 || m.Invocation.Fields[0] != "sha" {
		t.Fatalf("fields = %v", m.Invocation.Fields)
	}
	if m.Invocation.DocType != "git.commit" {
		t.Fatalf("doc_type = %q", m.Invocation.DocType)
	}
	// Body should be present in description.
	if !contains(m.Description, "Indexes the most recent commits") {
		t.Fatalf("description missing body: %q", m.Description)
	}
}

func TestSkillLoader_RejectsMissingCommand(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "broken.md")
	content := `---
name: broken
trigger:
  mode: on-demand
---

body
`
	os.WriteFile(path, []byte(content), 0o644)
	if _, err := LoadSkill(path); err == nil {
		t.Fatalf("expected error for missing command")
	}
}

func TestSkillLoader_RejectsMissingIntervalDuration(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "broken.md")
	content := `---
name: broken
trigger:
  mode: interval
invoke:
  command: echo
---

body
`
	os.WriteFile(path, []byte(content), 0o644)
	if _, err := LoadSkill(path); err == nil {
		t.Fatalf("expected error for missing interval duration")
	}
}

func TestScriptCapability_EchoStream(t *testing.T) {
	// Round-trip: skill loader → registry → runner.Invoke → real subprocess.
	dir := t.TempDir()
	path := filepath.Join(dir, "echo.md")
	content := `---
name: echo.test
trigger:
  mode: on-demand
sink:
  stream: true
invoke:
  command: echo
  args: [hello-from-capability]
  parser: raw
---

Test capability that echoes a fixed string.
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	c, err := LoadSkill(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	r := NewRegistry()
	r.Register(c)
	runner := NewRunner(r, nil)

	var buf bytes.Buffer
	if err := runner.Invoke(context.Background(), "echo.test", Input{}, &buf); err != nil {
		t.Fatalf("invoke: %v", err)
	}
	if !contains(buf.String(), "hello-from-capability") {
		t.Fatalf("stream output missing expected text: %q", buf.String())
	}
}

func TestScriptCapability_JSONLDocs(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "jsonl.md")
	// printf inside sh -c so the test is portable across darwin/linux.
	content := `---
name: jsonl.test
trigger:
  mode: on-demand
sink:
  index: true
invoke:
  command: sh
  args: ["-c", "printf '{\"a\":1}\n{\"a\":2}\n{\"a\":3}\n'"]
  parser: jsonl
  doc_type: test.row
---

Emits three JSONL docs.
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	c, err := LoadSkill(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	r := NewRegistry()
	r.Register(c)
	idx := &fakeIndexer{}
	runner := NewRunner(r, idx)
	if err := runner.Invoke(context.Background(), "jsonl.test", Input{}, nil); err != nil {
		t.Fatalf("invoke: %v", err)
	}
	if got := idx.count("glitch-events"); got != 3 {
		t.Fatalf("indexed docs: got %d want 3", got)
	}
}

func TestPathInDirs(t *testing.T) {
	cases := []struct {
		path string
		dirs []string
		want bool
	}{
		{"/foo/bar", nil, true},                  // empty filter = include all
		{"", []string{"/x"}, false},              // empty path with filter = exclude
		{"/foo/bar", []string{"/foo"}, true},     // child match
		{"/foo", []string{"/foo"}, true},         // exact match
		{"/foobar", []string{"/foo"}, false},     // prefix-only is not a match
		{"/baz", []string{"/foo", "/bar"}, false},
	}
	for _, tc := range cases {
		if got := pathInDirs(tc.path, tc.dirs); got != tc.want {
			t.Errorf("pathInDirs(%q, %v) = %v want %v", tc.path, tc.dirs, got, tc.want)
		}
	}
}

func contains(haystack, needle string) bool {
	return bytes.Contains([]byte(haystack), []byte(needle))
}
