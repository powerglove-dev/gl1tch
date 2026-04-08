package capability

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// LoadSkill parses a markdown-with-frontmatter file into a Capability backed
// by the script runtime. The frontmatter (YAML between `---` fences) becomes
// the manifest; the markdown body becomes Manifest.Description so the
// assistant can show it to the local model when picking a capability.
//
// The split-on-fences design lets one file serve two audiences: the runner
// reads only the frontmatter and never executes the body, while the assistant
// reads only the body and never sees the runtime details. The local LLM is
// never asked to construct shell commands from prose — it picks a name from
// the registry and the runner executes the declared invocation directly.
func LoadSkill(path string) (Capability, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("skill: read %s: %w", path, err)
	}
	front, body, err := splitFrontmatter(data)
	if err != nil {
		return nil, fmt.Errorf("skill: %s: %w", path, err)
	}
	var fm skillFrontmatter
	if err := yaml.Unmarshal(front, &fm); err != nil {
		return nil, fmt.Errorf("skill: %s: parse frontmatter: %w", path, err)
	}
	manifest, err := fm.toManifest(string(body))
	if err != nil {
		return nil, fmt.Errorf("skill: %s: %w", path, err)
	}
	return &scriptCapability{manifest: manifest}, nil
}

// LoadSkillsFromDir scans dir for *.md files and loads each as a capability.
// Per-file errors are returned alongside the successfully loaded capabilities
// so a single broken file does not prevent the rest from registering.
func LoadSkillsFromDir(dir string) ([]Capability, []error) {
	matches, err := filepath.Glob(filepath.Join(dir, "*.md"))
	if err != nil {
		return nil, []error{err}
	}
	var caps []Capability
	var errs []error
	for _, m := range matches {
		c, err := LoadSkill(m)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		caps = append(caps, c)
	}
	return caps, errs
}

// skillFrontmatter is the YAML schema embedded at the top of a skill file.
// Mirrors the Manifest structure but uses string types where the runtime
// uses richer types (durations, enums) so YAML stays human-friendly.
type skillFrontmatter struct {
	Name        string `yaml:"name"`
	Description string `yaml:"description"` // optional one-liner; body is the long form
	Category    string `yaml:"category"`
	Trigger     struct {
		Mode  string `yaml:"mode"`
		Every string `yaml:"every"`
	} `yaml:"trigger"`
	Sink struct {
		Index  bool `yaml:"index"`
		Stream bool `yaml:"stream"`
	} `yaml:"sink"`
	Invoke struct {
		Command string   `yaml:"command"`
		Args    []string `yaml:"args"`
		Parser  string   `yaml:"parser"`
		Fields  []string `yaml:"fields"`
		Index   string   `yaml:"index"`
		DocType string   `yaml:"doc_type"`
	} `yaml:"invoke"`
}

func (fm skillFrontmatter) toManifest(body string) (Manifest, error) {
	if fm.Name == "" {
		return Manifest{}, fmt.Errorf("name is required")
	}
	if fm.Invoke.Command == "" {
		return Manifest{}, fmt.Errorf("invoke.command is required")
	}

	mode := TriggerMode(fm.Trigger.Mode)
	if mode == "" {
		mode = TriggerOnDemand
	}
	if mode != TriggerOnDemand && mode != TriggerInterval {
		return Manifest{}, fmt.Errorf("trigger.mode: invalid %q", fm.Trigger.Mode)
	}

	var every time.Duration
	if fm.Trigger.Every != "" {
		d, err := time.ParseDuration(fm.Trigger.Every)
		if err != nil {
			return Manifest{}, fmt.Errorf("trigger.every: %w", err)
		}
		every = d
	}
	if mode == TriggerInterval && every == 0 {
		return Manifest{}, fmt.Errorf("trigger.every: required for interval mode")
	}

	parser := ParserKind(fm.Invoke.Parser)
	if parser == "" {
		parser = ParserRaw
	}
	switch parser {
	case ParserRaw, ParserLines, ParserPipeLines, ParserJSONL:
	default:
		return Manifest{}, fmt.Errorf("invoke.parser: unknown %q", fm.Invoke.Parser)
	}

	// Description prefers the markdown body (richer prose for the
	// assistant) and falls back to the one-line frontmatter description.
	desc := strings.TrimSpace(body)
	if desc == "" {
		desc = fm.Description
	}

	return Manifest{
		Name:        fm.Name,
		Description: desc,
		Category:    fm.Category,
		Trigger:     Trigger{Mode: mode, Every: every},
		Sink:        Sink{Index: fm.Sink.Index, Stream: fm.Sink.Stream},
		Invocation: Invocation{
			Command: fm.Invoke.Command,
			Args:    fm.Invoke.Args,
			Parser:  parser,
			Fields:  fm.Invoke.Fields,
			Index:   fm.Invoke.Index,
			DocType: fm.Invoke.DocType,
		},
	}, nil
}

// splitFrontmatter separates a YAML frontmatter block from the body of a
// markdown file. The frontmatter must start on the first line with `---` and
// end with another `---` line. Anything after the closing fence is the body.
// Returns an error if the file does not begin with a fence.
func splitFrontmatter(data []byte) (front, body []byte, err error) {
	const fence = "---"
	trimmed := bytes.TrimLeft(data, " \t\r\n")
	if !bytes.HasPrefix(trimmed, []byte(fence)) {
		return nil, nil, fmt.Errorf("missing frontmatter fence")
	}
	// Drop the opening fence line.
	rest := trimmed[len(fence):]
	// Skip the rest of the opening fence line.
	if i := bytes.IndexByte(rest, '\n'); i >= 0 {
		rest = rest[i+1:]
	} else {
		return nil, nil, fmt.Errorf("frontmatter not terminated")
	}
	// Find the closing fence at the start of a line.
	closeIdx := bytes.Index(rest, []byte("\n"+fence))
	if closeIdx < 0 {
		// Maybe the file starts the closing fence on the very first line
		// of the body — handle that too.
		if bytes.HasPrefix(rest, []byte(fence)) {
			closeIdx = 0
		} else {
			return nil, nil, fmt.Errorf("frontmatter not terminated")
		}
	} else {
		closeIdx++ // step past the leading newline
	}
	front = rest[:closeIdx]
	after := rest[closeIdx+len(fence):]
	// Drop one trailing newline after the closing fence if present.
	if len(after) > 0 && after[0] == '\n' {
		after = after[1:]
	}
	return front, after, nil
}

// scriptCapability runs a subprocess and converts its stdout into events
// according to the manifest's Invocation.Parser. This single implementation
// handles every skill-loaded capability AND every existing CliAdapter sidecar
// once it's reshaped into a manifest. The unifying observation is that an
// AI provider plugin (like the claude wrapper) is just a script capability
// with parser=raw and sink=stream, while a shell collector (like a tail of
// ~/.zsh_history) is a script capability with parser=jsonl and sink=index.
type scriptCapability struct {
	manifest Manifest
}

func (s *scriptCapability) Manifest() Manifest { return s.manifest }

func (s *scriptCapability) Invoke(ctx context.Context, in Input) (<-chan Event, error) {
	ch := make(chan Event, 16)
	go func() {
		defer close(ch)
		s.run(ctx, in, ch)
	}()
	return ch, nil
}

func (s *scriptCapability) run(ctx context.Context, in Input, ch chan<- Event) {
	inv := s.manifest.Invocation

	// Allow on-demand callers to append extra args via vars["args"]. Used by
	// the assistant when it wants to pass user-supplied parameters to a
	// generic capability without invoking a shell.
	args := append([]string{}, inv.Args...)
	if extra := in.Vars["args"]; extra != "" {
		args = append(args, strings.Fields(extra)...)
	}

	cmd := exec.CommandContext(ctx, inv.Command, args...)
	cmd.Stdin = strings.NewReader(in.Stdin)

	// Inherit the parent environment and overlay GLITCH_<KEY>=<value> for
	// every var. Matches the existing CliAdapter convention so capabilities
	// ported from the executor world keep working.
	env := os.Environ()
	for k, v := range in.Vars {
		env = append(env, "GLITCH_"+strings.ToUpper(k)+"="+v)
	}
	cmd.Env = env

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		ch <- Event{Kind: EventError, Err: fmt.Errorf("stdout pipe: %w", err)}
		return
	}
	// Stderr is captured into a buffer and emitted as one error event at
	// the end if the command fails. Avoids interleaving stderr noise into
	// stdout-driven parsing.
	var stderrBuf bytes.Buffer
	cmd.Stderr = &stderrBuf

	if err := cmd.Start(); err != nil {
		ch <- Event{Kind: EventError, Err: fmt.Errorf("start: %w", err)}
		return
	}

	s.parseStdout(stdout, ch)

	if err := cmd.Wait(); err != nil {
		stderr := strings.TrimSpace(stderrBuf.String())
		if stderr != "" {
			ch <- Event{Kind: EventError, Err: fmt.Errorf("%w: %s", err, stderr)}
		} else {
			ch <- Event{Kind: EventError, Err: err}
		}
	}
}

func (s *scriptCapability) parseStdout(r io.Reader, ch chan<- Event) {
	inv := s.manifest.Invocation
	switch inv.Parser {
	case ParserRaw, "":
		buf, err := io.ReadAll(r)
		if err != nil {
			ch <- Event{Kind: EventError, Err: err}
			return
		}
		if len(buf) > 0 {
			ch <- Event{Kind: EventStream, Text: string(buf)}
		}
	case ParserLines:
		scanner := bufio.NewScanner(r)
		scanner.Buffer(make([]byte, 1024*256), 1024*256)
		for scanner.Scan() {
			ch <- Event{Kind: EventStream, Text: scanner.Text() + "\n"}
		}
		if err := scanner.Err(); err != nil {
			ch <- Event{Kind: EventError, Err: err}
		}
	case ParserPipeLines:
		scanner := bufio.NewScanner(r)
		scanner.Buffer(make([]byte, 1024*256), 1024*256)
		for scanner.Scan() {
			line := scanner.Text()
			if line == "" {
				continue
			}
			parts := strings.Split(line, "|")
			doc := make(map[string]any, len(inv.Fields)+1)
			for i, f := range inv.Fields {
				if i < len(parts) {
					doc[f] = parts[i]
				}
			}
			if inv.DocType != "" {
				doc["type"] = inv.DocType
			}
			ch <- Event{Kind: EventDoc, Doc: doc}
		}
		if err := scanner.Err(); err != nil {
			ch <- Event{Kind: EventError, Err: err}
		}
	case ParserJSONL:
		scanner := bufio.NewScanner(r)
		scanner.Buffer(make([]byte, 1024*1024), 1024*1024)
		for scanner.Scan() {
			line := scanner.Bytes()
			if len(bytes.TrimSpace(line)) == 0 {
				continue
			}
			var doc map[string]any
			if err := json.Unmarshal(line, &doc); err != nil {
				ch <- Event{Kind: EventError, Err: fmt.Errorf("jsonl parse: %w", err)}
				continue
			}
			if inv.DocType != "" {
				if _, present := doc["type"]; !present {
					doc["type"] = inv.DocType
				}
			}
			ch <- Event{Kind: EventDoc, Doc: doc}
		}
		if err := scanner.Err(); err != nil {
			ch <- Event{Kind: EventError, Err: err}
		}
	}
}
