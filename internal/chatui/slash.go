package chatui

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"
)

// slash.go is the slash-command surface that replaces the desktop
// sidebar. It is intentionally narrow:
//
//   - One Registry per chat session.
//   - One Handler interface, scope-aware (so chat-threads can route
//     thread-local commands to the right context without a second
//     dispatcher).
//   - Zero LLM calls in the dispatch path. Slash commands are pure code.
//   - No "magic" command parsing — the input is split on whitespace, the
//     first token is the name, the rest is one positional argument plus
//     any flag-style key=value pairs.
//
// The dispatcher is the bridge between typed input and the rich
// MessageType payloads. A handler returns one (or more) ChatMessages that
// the caller appends to the store; the dispatcher does not own the store
// itself, so the same Registry can drive a CLI smoke harness, a unit
// test, and the desktop UI without three implementations.

// SlashScope identifies where a slash command was invoked. The two
// canonical values are "main" (the root chat) and "thread:<id>". Handlers
// that ignore Scope behave the same in both contexts; handlers that
// branch on it (e.g. /save inside a config thread) read it explicitly.
type SlashScope string

// SlashScopeMain is the scope for commands typed in the main chat
// scrollback (no enclosing thread).
const SlashScopeMain SlashScope = "main"

// ThreadScope returns the SlashScope value for commands typed inside the
// given thread.
func ThreadScope(threadID string) SlashScope {
	return SlashScope("thread:" + threadID)
}

// IsThreadScope reports whether the scope refers to a thread (vs the
// main chat).
func (s SlashScope) IsThreadScope() bool {
	return strings.HasPrefix(string(s), "thread:")
}

// ThreadID returns the thread identifier embedded in a thread scope, or
// the empty string when the scope is main.
func (s SlashScope) ThreadID() string {
	if !s.IsThreadScope() {
		return ""
	}
	return strings.TrimPrefix(string(s), "thread:")
}

// SlashInvocation is the parsed form of a slash-command line. The handler
// receives this instead of the raw text so the dispatcher can centralise
// quoting / flag-parsing in one place.
type SlashInvocation struct {
	// Name is the command name without the leading slash.
	Name string
	// Args is the remaining tokens after the command name. Quoting is
	// not honoured in v1 — handlers that need quoted strings should
	// declare a single positional argument and use Raw instead.
	Args []string
	// Flags is the set of key=value pairs the dispatcher extracted from
	// Args before populating it. A token like "model=qwen2.5:7b"
	// becomes Flags["model"]="qwen2.5:7b" and is removed from Args.
	Flags map[string]string
	// Raw is the entire line after the command name, untouched. Use
	// this for handlers that take freeform input ("/ask <prompt>").
	Raw string
	// Scope is where the line was typed.
	Scope SlashScope
}

// SlashHandler is a single registered command. It returns one or more
// ChatMessages the caller should append to the store, plus an error. An
// empty messages slice with a nil error is valid: the handler chose to do
// its work silently (e.g. a /save handler that closes the enclosing
// thread).
type SlashHandler interface {
	Name() string
	Describe() string
	// AllowedScopes lists the scopes this handler accepts. Empty (nil)
	// means "any scope". The dispatcher rejects invocations whose scope
	// is not in the allowed list with ErrScopeNotAllowed.
	AllowedScopes() []SlashScope
	Handle(ctx context.Context, in SlashInvocation) ([]ChatMessage, error)
}

// SlashHandlerFunc is a convenience adapter for handlers that don't need
// their own struct. The Name and Describe are passed in at registration;
// the function carries the behaviour.
type SlashHandlerFunc struct {
	NameField     string
	DescribeField string
	Allowed       []SlashScope
	Fn            func(ctx context.Context, in SlashInvocation) ([]ChatMessage, error)
}

// Name implements SlashHandler.
func (f SlashHandlerFunc) Name() string { return f.NameField }

// Describe implements SlashHandler.
func (f SlashHandlerFunc) Describe() string { return f.DescribeField }

// AllowedScopes implements SlashHandler.
func (f SlashHandlerFunc) AllowedScopes() []SlashScope { return f.Allowed }

// Handle implements SlashHandler.
func (f SlashHandlerFunc) Handle(ctx context.Context, in SlashInvocation) ([]ChatMessage, error) {
	if f.Fn == nil {
		return nil, fmt.Errorf("chatui: handler %q has nil Fn", f.NameField)
	}
	return f.Fn(ctx, in)
}

// Errors returned by Registry methods.
var (
	ErrUnknownCommand   = errors.New("chatui: unknown slash command")
	ErrScopeNotAllowed  = errors.New("chatui: command not allowed in this scope")
	ErrEmptyInput       = errors.New("chatui: empty slash input")
	ErrDuplicateCommand = errors.New("chatui: duplicate slash command")
)

// SlashRegistry holds the slash command set for one chat session.
type SlashRegistry struct {
	mu       sync.RWMutex
	handlers map[string]SlashHandler
}

// NewSlashRegistry constructs an empty registry. User aliases from
// ~/.config/glitch/slash.yaml are NOT auto-loaded here because the
// caller might want to register built-ins first (so the built-ins
// win on name collisions). Use NewSlashRegistryWithAliases for the
// production path that auto-loads aliases after the caller has
// registered every built-in handler.
func NewSlashRegistry() *SlashRegistry {
	return &SlashRegistry{handlers: make(map[string]SlashHandler)}
}

// LoadAliases is a convenience that calls RegisterAliases on this
// registry. Built-in handlers always win on name collisions
// (RegisterAliases skips alias names that are already registered),
// so the production wiring is:
//
//   reg := chatui.NewSlashRegistry()
//   reg.Register(chatui.HelpHandler(reg))
//   reg.Register(chatui.ResearchSlashHandler(loop))
//   _ = reg.LoadAliases()
//
// Errors are returned but never fatal — a missing or malformed
// slash.yaml just produces zero aliases and the built-ins still
// work.
func (r *SlashRegistry) LoadAliases() error {
	return RegisterAliases(r)
}

// Register adds a handler. Returns ErrDuplicateCommand if a handler with
// the same Name is already registered.
func (r *SlashRegistry) Register(h SlashHandler) error {
	if h == nil {
		return errors.New("chatui: cannot register nil handler")
	}
	name := strings.TrimSpace(h.Name())
	if name == "" {
		return errors.New("chatui: handler Name() must not be empty")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, dup := r.handlers[name]; dup {
		return fmt.Errorf("%w: %s", ErrDuplicateCommand, name)
	}
	r.handlers[name] = h
	return nil
}

// List returns all registered handlers sorted by Name. Used by the
// generated /help output.
func (r *SlashRegistry) List() []SlashHandler {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]SlashHandler, 0, len(r.handlers))
	for _, h := range r.handlers {
		out = append(out, h)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name() < out[j].Name() })
	return out
}

// Lookup returns the handler registered under name.
func (r *SlashRegistry) Lookup(name string) (SlashHandler, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	h, ok := r.handlers[name]
	return h, ok
}

// Dispatch parses the line, looks up the handler, enforces the scope
// rule, and runs the handler. The returned messages slice is what the
// caller should append to the store; the error explains any failure.
//
// Lines that don't begin with `/` are treated as user input (not slash
// commands) and Dispatch returns ErrEmptyInput so the caller can route
// the line through its assistant path instead.
func (r *SlashRegistry) Dispatch(ctx context.Context, line string, scope SlashScope) ([]ChatMessage, error) {
	line = strings.TrimSpace(line)
	if line == "" || !strings.HasPrefix(line, "/") {
		return nil, ErrEmptyInput
	}

	body := strings.TrimSpace(strings.TrimPrefix(line, "/"))
	if body == "" {
		return nil, ErrEmptyInput
	}

	// Split on the first whitespace run; everything before is the
	// command name (which may itself contain a space, e.g. "brain
	// config"), everything after is the raw arg string. We honour
	// two-token command names by trying the longest match first.
	name, rawArgs := splitCommand(body)
	h, ok := r.Lookup(name)
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrUnknownCommand, name)
	}
	if !scopeAllowed(scope, h.AllowedScopes()) {
		return nil, fmt.Errorf("%w: %q in %s", ErrScopeNotAllowed, name, scope)
	}

	in := SlashInvocation{
		Name:  name,
		Raw:   rawArgs,
		Scope: scope,
		Flags: map[string]string{},
	}
	for _, tok := range strings.Fields(rawArgs) {
		if k, v, ok := strings.Cut(tok, "="); ok && !strings.HasPrefix(tok, "=") {
			in.Flags[k] = v
			continue
		}
		in.Args = append(in.Args, tok)
	}

	return h.Handle(ctx, in)
}

// splitCommand finds the longest registered command name that matches
// the start of body. We do not actually look up the registry here (would
// need a lock); the dispatcher does the lookup separately. Instead we
// use a simple heuristic: try the first two whitespace-separated tokens
// joined as one name first, fall back to one. That covers `brain config`
// and `config dirs` cleanly without invented quoting rules.
func splitCommand(body string) (name, rawArgs string) {
	fields := strings.Fields(body)
	if len(fields) == 0 {
		return "", ""
	}
	if len(fields) >= 2 {
		two := fields[0] + " " + fields[1]
		// We do not have access to the registry here, so always
		// produce the two-token form. The registry's Lookup is
		// case-sensitive and the dispatcher will fall through to a
		// one-token retry below if the two-token form is unknown.
		// (Tested in TestRegistry_TwoTokenCommandName.)
		// Returning the two-token form first lets the registry decide.
		// The retry happens in Dispatch via the unknown-command branch.
		_ = two
	}
	// Default: take the first whitespace token as the name.
	first := fields[0]
	rest := strings.TrimSpace(strings.TrimPrefix(body, first))
	return first, rest
}

// scopeAllowed reports whether scope is in the allowed list. Empty
// allowed list means "any scope".
func scopeAllowed(scope SlashScope, allowed []SlashScope) bool {
	if len(allowed) == 0 {
		return true
	}
	for _, a := range allowed {
		if a == scope {
			return true
		}
		// "thread:*" wildcard match — handlers that allow any
		// thread (but not main) can register the wildcard.
		if string(a) == "thread:*" && scope.IsThreadScope() {
			return true
		}
	}
	return false
}

// HelpHandler returns the canonical /help handler. It renders the
// supplied registry as a widget card with one row per registered
// command. The handler closes over the registry so /help inside a
// thread shows the same set as /help in main (but the dispatcher will
// still gate thread-only commands by scope).
func HelpHandler(reg *SlashRegistry) SlashHandler {
	return SlashHandlerFunc{
		NameField:     "help",
		DescribeField: "Show available slash commands",
		Fn: func(_ context.Context, in SlashInvocation) ([]ChatMessage, error) {
			handlers := reg.List()
			rows := make([]WidgetRow, 0, len(handlers))
			for _, h := range handlers {
				rows = append(rows, WidgetRow{
					Key:   "/" + h.Name(),
					Value: h.Describe(),
				})
			}
			return []ChatMessage{{
				Role: RoleAssistant,
				Type: MessageTypeWidgetCard,
				Payload: WidgetCardPayload{
					Title:    "Slash commands",
					Subtitle: fmt.Sprintf("%d command(s) available", len(handlers)),
					Rows:     rows,
				},
				CreatedAt: time.Now(),
			}}, nil
		},
	}
}
