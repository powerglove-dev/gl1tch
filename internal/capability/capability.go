// Package capability is gl1tch's unified primitive for "things the assistant
// can do." It collapses what used to be two separate systems — background
// collectors that index documents into Elasticsearch, and on-demand executors
// that spawn subprocesses for AI providers and pipeline steps — into one
// interface with two axes: when it runs (Trigger) and where its output goes
// (Sink).
//
// A Capability is anything that produces Events. The Runner schedules
// interval-triggered capabilities, dispatches on-demand calls, and routes Doc
// events into ES while forwarding Stream events to the caller. Capabilities
// can be implemented in Go (for stateful or complex logic) or declared as
// markdown skill files with YAML frontmatter (for the long tail of shell
// collectors and simple AI provider wrappers).
//
// The local LLM is never in the execution path. The runner parses manifests
// and runs commands deterministically. The model only enters when the
// assistant needs to pick a capability by name from the registry to satisfy
// an on-demand user request — and even then it picks a name, the runner
// takes over.
package capability

import (
	"context"
	"time"
)

// TriggerMode declares when a capability runs.
type TriggerMode string

const (
	// TriggerOnDemand capabilities are invoked explicitly by the assistant
	// or by user action. They never run on their own.
	TriggerOnDemand TriggerMode = "on-demand"
	// TriggerInterval capabilities are scheduled by the runner on a fixed
	// cadence. Used for indexers and pollers.
	TriggerInterval TriggerMode = "interval"
	// TriggerDaemon capabilities are invoked exactly once at runner Start
	// and run for the lifetime of the runner context. Used by long-running
	// adapters that own their own internal scheduling — most importantly,
	// the legacy collector wrapper, which delegates to a Collector.Start
	// that blocks until cancellation. The runner does not reschedule
	// daemon-mode capabilities and does not collect Doc events from them
	// (legacy collectors index directly via their own ES client).
	TriggerDaemon TriggerMode = "daemon"
)

// Trigger declares when a capability runs.
type Trigger struct {
	Mode  TriggerMode
	Every time.Duration // for interval mode; ignored otherwise
}

// Sink declares where a capability's events flow.
//
// A capability with Index=true emits Doc events that the runner bulk-indexes
// into Elasticsearch. A capability with Stream=true emits Stream events that
// the runner forwards to the invocation caller (e.g. a TUI, an HTTP client,
// the assistant's reasoning loop). Both can be true: an AI provider invocation
// can stream tokens back to the user AND index a doc representing the
// completed exchange.
type Sink struct {
	Index  bool
	Stream bool
}

// ParserKind tells the runner how to convert subprocess stdout into Events.
// Only meaningful for script-backed capabilities; Go-implemented capabilities
// emit Events directly and ignore Invocation entirely.
type ParserKind string

const (
	// ParserRaw treats the entire stdout as one Stream event.
	ParserRaw ParserKind = "raw"
	// ParserLines emits one Stream event per output line.
	ParserLines ParserKind = "lines"
	// ParserPipeLines splits each line on "|" and emits one Doc event per
	// line, using Invocation.Fields as column names. Useful for shell
	// collectors that pipe `git log --pretty=format:%H|%an|%ct|%s`.
	ParserPipeLines ParserKind = "pipe-lines"
	// ParserJSONL parses each line as a JSON object and emits one Doc event
	// per line. The standard format for "shell collector in any language."
	ParserJSONL ParserKind = "jsonl"
)

// Invocation describes how a script-backed capability spawns its subprocess.
// Built-in Go capabilities leave this zero-valued.
type Invocation struct {
	Command string
	Args    []string
	Parser  ParserKind
	// Fields names the columns when Parser is ParserPipeLines.
	Fields []string
	// Index is the ES index name to write Doc events into. Defaults to
	// "glitch-events" via the runner if empty.
	Index string
	// DocType is the value used for the "type" field of emitted Doc events.
	// Lets a capability stamp its output without coding a Go struct.
	DocType string
}

// Manifest is the static description of a capability. It is loaded once at
// registration time and never changes during the capability's lifetime.
//
// For skill-loaded capabilities the manifest comes from the YAML frontmatter
// of the markdown file; the Description field is the markdown body, which the
// assistant uses when picking a capability for an on-demand request. For
// Go-implemented capabilities the manifest is constructed in code.
type Manifest struct {
	Name        string
	Description string
	Category    string
	Trigger     Trigger
	Sink        Sink
	Invocation  Invocation
}

// Input is the runtime input to one capability call. Stdin is written to a
// subprocess's stdin (script capabilities) or passed as a freeform string to
// Go capabilities. Vars are exposed to subprocesses as GLITCH_<KEY>=<value>
// environment variables and passed through to Go capabilities as a map.
type Input struct {
	Stdin string
	Vars  map[string]string
}

// EventKind classifies events emitted by a capability.
type EventKind int

const (
	// EventDoc carries a document destined for an ES index. The runner
	// collects Doc events from one Invoke call and bulk-indexes them.
	EventDoc EventKind = iota
	// EventStream carries a chunk of text destined for the invocation
	// caller. Used for AI provider streaming output, log tails, etc.
	EventStream
	// EventError carries a non-fatal error. The runner logs it and
	// continues draining the channel.
	EventError
)

// Event is one thing produced by a capability invocation. The channel
// returned from Invoke is closed when the capability is done — that's how the
// runner knows the invocation has finished.
type Event struct {
	Kind EventKind
	// Doc is set when Kind == EventDoc. Must be JSON-marshalable for the ES
	// bulk indexer.
	Doc any
	// Index optionally overrides the manifest's default ES index for this
	// specific Doc event. Empty falls back to Invocation.Index, then the
	// runner's default.
	Index string
	// Text is set when Kind == EventStream.
	Text string
	// Err is set when Kind == EventError.
	Err error
}

// Capability is the unified primitive: anything that can produce events on
// demand or on a schedule. Implementations should be cheap to construct and
// safe to invoke concurrently — the runner may call Invoke from multiple
// goroutines for on-demand capabilities, though interval invocations are
// serialised per-capability.
type Capability interface {
	Manifest() Manifest
	// Invoke starts one execution of the capability. The returned channel
	// must be closed when the execution is complete. Invoke itself should
	// return quickly — the actual work happens in a goroutine that writes
	// to the channel.
	Invoke(ctx context.Context, in Input) (<-chan Event, error)
}
