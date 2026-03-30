// Package activity provides the append-only JSONL activity feed used by the
// ORCAI activity-feed TUI component. All pipeline, schedule, and agent-run
// lifecycle events are written here; the feed component tails the file via
// fsnotify and renders a real-time social-style timeline.
package activity

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/fsnotify/fsnotify"
)

// ActivityEvent is a single JSONL activity record.
// Each field is intentionally flat — no nested objects, no arrays.
type ActivityEvent struct {
	TS     string `json:"ts"`     // RFC3339 timestamp
	Kind   string `json:"kind"`   // pipeline_started | pipeline_finished | pipeline_failed | schedule_fired | agent_run_started | agent_run_finished | agent_run_failed
	Agent  string `json:"agent"`  // short human-readable agent / pipeline name
	Label  string `json:"label"`  // brief run description (job name, trigger label, etc.)
	Status string `json:"status"` // running | done | failed | scheduled
}

// Now returns an ActivityEvent timestamped to the current instant.
func Now(kind, agent, label, status string) ActivityEvent {
	return ActivityEvent{
		TS:     time.Now().UTC().Format(time.RFC3339),
		Kind:   kind,
		Agent:  agent,
		Label:  label,
		Status: status,
	}
}

// DefaultPath returns ~/.orcai/activity.jsonl.
func DefaultPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".orcai", "activity.jsonl")
}

// AppendEvent appends e as a single JSON line to path.
// The file and its parent directory are created if they do not exist.
func AppendEvent(path string, e ActivityEvent) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	b, err := json.Marshal(e)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(f, "%s\n", b)
	return err
}

// LoadRecentEvents reads the last n lines from the JSONL file at path and
// returns them in file order (oldest first). Lines that cannot be parsed are
// silently skipped. Returns nil, nil when the file does not yet exist.
func LoadRecentEvents(path string, n int) ([]ActivityEvent, error) {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	defer f.Close()

	var lines []string
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		if t := sc.Text(); t != "" {
			lines = append(lines, t)
		}
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}

	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}

	events := make([]ActivityEvent, 0, len(lines))
	for _, l := range lines {
		var e ActivityEvent
		if json.Unmarshal([]byte(l), &e) == nil {
			events = append(events, e)
		}
	}
	return events, nil
}

// WatchFeed watches path for appended lines using fsnotify, sending each
// successfully parsed ActivityEvent to ch. The goroutine exits cleanly when
// ctx is cancelled. Parent directory is created if it does not exist.
func WatchFeed(path string, ch chan<- ActivityEvent, ctx context.Context) {
	dir := filepath.Dir(path)
	_ = os.MkdirAll(dir, 0o755)

	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return
	}
	defer watcher.Close()

	// Watch the directory so we also catch file-creation events.
	if err := watcher.Add(dir); err != nil {
		return
	}

	// Seek to EOF so we only tail new content.
	var offset int64
	if f, err := os.Open(path); err == nil {
		offset, _ = f.Seek(0, io.SeekEnd)
		f.Close()
	}

	for {
		select {
		case <-ctx.Done():
			return

		case event, ok := <-watcher.Events:
			if !ok {
				return
			}
			// Normalise path for comparison (fsnotify may return relative paths on
			// some platforms).
			if !strings.HasSuffix(filepath.ToSlash(event.Name), filepath.ToSlash(path)) &&
				event.Name != path {
				continue
			}
			if event.Op&(fsnotify.Write|fsnotify.Create) == 0 {
				continue
			}
			offset = readFrom(path, offset, ch, ctx)

		case _, ok := <-watcher.Errors:
			if !ok {
				return
			}
		}
	}
}

// readFrom reads new lines from path starting at byteOffset, sends parsed
// events to ch, and returns the updated offset. Stops early when ctx is done.
func readFrom(path string, offset int64, ch chan<- ActivityEvent, ctx context.Context) int64 {
	f, err := os.Open(path)
	if err != nil {
		return offset
	}
	defer f.Close()

	if _, err := f.Seek(offset, io.SeekStart); err != nil {
		return offset
	}

	r := bufio.NewReader(f)
	for {
		line, err := r.ReadString('\n')
		offset += int64(len(line))
		trimmed := strings.TrimRight(line, "\r\n")
		if trimmed != "" {
			var e ActivityEvent
			if json.Unmarshal([]byte(trimmed), &e) == nil {
				select {
				case ch <- e:
				case <-ctx.Done():
					return offset
				}
			}
		}
		if err != nil {
			// io.EOF means we consumed all available bytes.
			break
		}
	}
	return offset
}
