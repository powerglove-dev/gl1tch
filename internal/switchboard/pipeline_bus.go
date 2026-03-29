// pipeline_bus.go — busd subscription and feed seeding for the Switchboard.
//
// Tasks 7.1–7.8: authoritative feed updates driven by busd pipeline/step/cron
// events, with the log-line parser demoted to a fallback for terminals.
package switchboard

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/adam-stokes/orcai/internal/busd"
	"github.com/adam-stokes/orcai/internal/busd/topics"
	"github.com/adam-stokes/orcai/internal/store"
)

// ── feed cap ──────────────────────────────────────────────────────────────────

// feedMaxCap is the maximum number of entries the ring buffer may hold.
const feedMaxCap = 200

// evictFeedIfNeeded trims the feed to feedMaxCap, preferring to evict the
// oldest non-running entry. Falls back to hard truncation when all entries are
// still running (pathological case).
func (m Model) evictFeedIfNeeded() Model {
	if len(m.feed) <= feedMaxCap {
		return m
	}
	// Walk from the oldest end (tail) looking for a non-running entry.
	for i := len(m.feed) - 1; i >= 0; i-- {
		if m.feed[i].status != FeedRunning {
			m.feed = append(m.feed[:i], m.feed[i+1:]...)
			return m
		}
	}
	// All running — hard-truncate the tail.
	m.feed = m.feed[:feedMaxCap]
	return m
}

// ── 7.1: feed seeding from store ─────────────────────────────────────────────

// feedSeedMsg carries the initial batch of feed entries loaded from the store.
type feedSeedMsg struct{ entries []feedEntry }

// seedFeedFromStoreCmd returns a tea.Cmd that queries the store for the 50
// most recent runs and returns them as a feedSeedMsg. Safe to call with a nil
// store — returns a no-op command.
func seedFeedFromStoreCmd(s *store.Store) tea.Cmd {
	if s == nil {
		return nil
	}
	return func() tea.Msg {
		runs, err := s.QueryRuns(50)
		if err != nil || len(runs) == 0 {
			return nil
		}
		entries := make([]feedEntry, 0, len(runs))
		for _, r := range runs {
			status := FeedDone
			if r.ExitStatus != nil && *r.ExitStatus != 0 {
				status = FeedFailed
			} else if r.ExitStatus == nil {
				status = FeedRunning
			}
			e := feedEntry{
				id:     fmt.Sprintf("run-%d", r.ID),
				title:  r.Name,
				status: status,
				ts:     time.UnixMilli(r.StartedAt),
			}
			for _, sr := range r.Steps {
				e.steps = append(e.steps, StepInfo{id: sr.ID, status: sr.Status})
			}
			entries = append(entries, e)
		}
		return feedSeedMsg{entries: entries}
	}
}

// handleFeedSeedMsg merges store-seeded entries into the feed without
// duplicating entries already present (matched by id).
func (m Model) handleFeedSeedMsg(msg feedSeedMsg) Model {
	// Build a set of ids already in the feed.
	existing := make(map[string]struct{}, len(m.feed))
	for _, e := range m.feed {
		existing[e.id] = struct{}{}
	}
	var toAdd []feedEntry
	for _, e := range msg.entries {
		if _, ok := existing[e.id]; !ok {
			toAdd = append(toAdd, e)
		}
	}
	if len(toAdd) == 0 {
		return m
	}
	// Append (seeded runs go after any live entries so live entries stay at top).
	m.feed = append(m.feed, toAdd...)
	m = m.evictFeedIfNeeded()
	return m
}

// ── 7.2: busd subscription infrastructure ────────────────────────────────────

// pipelineBusEventMsg carries one decoded event frame from the busd socket.
type pipelineBusEventMsg struct {
	topic   string
	payload json.RawMessage
}

// pipelineBusDisconnectedMsg signals that the busd connection was lost.
type pipelineBusDisconnectedMsg struct{}

// pipelineBusConnectMsg is returned by tryPipelineBusSubscribeCmd on success.
type pipelineBusConnectMsg struct {
	ch chan pipelineBusEventMsg
}

// tryPipelineBusSubscribeCmd attempts to connect to busd and subscribe to
// pipeline.run.*, pipeline.step.*, and cron.job.* topics. Returns
// pipelineBusConnectMsg on success or pipelineBusDisconnectedMsg on failure.
func tryPipelineBusSubscribeCmd() tea.Cmd {
	return func() tea.Msg {
		sockPath, err := busd.SocketPath()
		if err != nil {
			return pipelineBusDisconnectedMsg{}
		}
		conn, err := net.DialTimeout("unix", sockPath, 200*time.Millisecond)
		if err != nil {
			return pipelineBusDisconnectedMsg{}
		}
		reg, _ := json.Marshal(map[string]any{
			"name":      "switchboard",
			"subscribe": []string{"pipeline.run.*", "pipeline.step.*", "cron.job.*"},
		})
		if _, err := conn.Write(append(reg, '\n')); err != nil {
			conn.Close()
			return pipelineBusDisconnectedMsg{}
		}
		ch := make(chan pipelineBusEventMsg, 32)
		go func() {
			defer conn.Close()
			defer close(ch)
			scanner := bufio.NewScanner(conn)
			for scanner.Scan() {
				var frame struct {
					Event   string          `json:"event"`
					Payload json.RawMessage `json:"payload"`
				}
				if json.Unmarshal(scanner.Bytes(), &frame) == nil {
					ch <- pipelineBusEventMsg{topic: frame.Event, payload: frame.Payload}
				}
			}
		}()
		return pipelineBusConnectMsg{ch: ch}
	}
}

// waitForPipelineBusEvent returns a tea.Cmd that blocks until the next event
// arrives on ch, then delivers it as a pipelineBusEventMsg or
// pipelineBusDisconnectedMsg when the channel is closed.
func waitForPipelineBusEvent(ch chan pipelineBusEventMsg) tea.Cmd {
	return func() tea.Msg {
		msg, ok := <-ch
		if !ok {
			return pipelineBusDisconnectedMsg{}
		}
		return msg
	}
}

// ── 7.3–7.5 & 7.8: event dispatch ────────────────────────────────────────────

// handlePipelineBusEvent dispatches a single busd event to the appropriate
// feed or cron-panel update helper.
func (m Model) handlePipelineBusEvent(msg pipelineBusEventMsg) Model {
	switch msg.topic {

	// ── pipeline.run.* ────────────────────────────────────────────────────

	case topics.RunStarted:
		var payload struct {
			RunID    int64  `json:"run_id"`
			Pipeline string `json:"pipeline"`
		}
		if json.Unmarshal(msg.payload, &payload) == nil {
			id := fmt.Sprintf("run-%d", payload.RunID)
			for _, e := range m.feed {
				if e.id == id {
					return m // already present (seeded or duplicate event)
				}
			}
			entry := feedEntry{
				id:     id,
				title:  payload.Pipeline,
				status: FeedRunning,
				ts:     time.Now(),
			}
			m = m.prependFeedEntry(entry)
		}

	case topics.RunCompleted:
		var payload struct {
			RunID int64 `json:"run_id"`
		}
		if json.Unmarshal(msg.payload, &payload) == nil {
			m = m.updateFeedEntryStatus(fmt.Sprintf("run-%d", payload.RunID), FeedDone)
		}

	case topics.RunFailed:
		var payload struct {
			RunID int64 `json:"run_id"`
		}
		if json.Unmarshal(msg.payload, &payload) == nil {
			m = m.updateFeedEntryStatus(fmt.Sprintf("run-%d", payload.RunID), FeedFailed)
		}

	// ── pipeline.step.* ───────────────────────────────────────────────────

	case topics.StepStarted:
		var payload struct {
			RunID int64  `json:"run_id"`
			Step  string `json:"step"`
		}
		if json.Unmarshal(msg.payload, &payload) == nil {
			m = m.updateFeedEntryStep(fmt.Sprintf("run-%d", payload.RunID), payload.Step, "running")
		}

	case topics.StepDone:
		var payload struct {
			RunID int64  `json:"run_id"`
			Step  string `json:"step"`
		}
		if json.Unmarshal(msg.payload, &payload) == nil {
			m = m.updateFeedEntryStep(fmt.Sprintf("run-%d", payload.RunID), payload.Step, "done")
		}

	case topics.StepFailed:
		var payload struct {
			RunID int64  `json:"run_id"`
			Step  string `json:"step"`
		}
		if json.Unmarshal(msg.payload, &payload) == nil {
			m = m.updateFeedEntryStep(fmt.Sprintf("run-%d", payload.RunID), payload.Step, "failed")
		}

	// ── cron.job.* ────────────────────────────────────────────────────────

	case topics.CronJobStarted:
		var payload struct {
			Job string `json:"job"`
		}
		if json.Unmarshal(msg.payload, &payload) == nil {
			id := "cron-" + payload.Job
			// Only add if not already present (avoid duplicate running entries).
			for _, e := range m.feed {
				if e.id == id {
					return m
				}
			}
			entry := feedEntry{
				id:     id,
				title:  "cron: " + payload.Job,
				status: FeedRunning,
				ts:     time.Now(),
			}
			m = m.prependFeedEntry(entry)
		}

	case topics.CronJobCompleted:
		// The cron panel is rendered live from orcaicron.LoadConfig() so there
		// is no in-memory state to update. A tick refresh is sufficient; the
		// feed gets a new entry so the operator can see the outcome.
		var payload struct {
			Job        string `json:"job"`
			ExitStatus int    `json:"exit_status"`
			FinishedAt string `json:"finished_at"`
		}
		if json.Unmarshal(msg.payload, &payload) == nil {
			m = m.upsertCronFeedEntry(payload.Job, payload.ExitStatus)
		}
	}
	return m
}

// ── feed mutation helpers ─────────────────────────────────────────────────────

// prependFeedEntry adds entry at the front of the feed and evicts if needed.
func (m Model) prependFeedEntry(entry feedEntry) Model {
	m.feed = append([]feedEntry{entry}, m.feed...)
	m = m.evictFeedIfNeeded()
	return m
}

// updateFeedEntryStatus sets the FeedStatus of the entry with the given id.
func (m Model) updateFeedEntryStatus(id string, status FeedStatus) Model {
	for i := range m.feed {
		if m.feed[i].id == id {
			m.feed[i].status = status
			return m
		}
	}
	return m
}

// updateFeedEntryStep upserts a StepInfo inside the feed entry identified by
// entryID. If the step already has a terminal status it is NOT overwritten
// (authoritative busd events win over log-line parser updates; see 7.6).
func (m Model) updateFeedEntryStep(entryID, stepID, status string) Model {
	for i := range m.feed {
		if m.feed[i].id == entryID {
			for j := range m.feed[i].steps {
				if m.feed[i].steps[j].id == stepID {
					// Never downgrade a terminal status.
					if isTerminalStepStatus(m.feed[i].steps[j].status) {
						return m
					}
					m.feed[i].steps[j].status = status
					return m
				}
			}
			// Step not yet present — add it.
			m.feed[i].steps = append(m.feed[i].steps, StepInfo{id: stepID, status: status})
			return m
		}
	}
	return m
}

// isTerminalStepStatus reports whether a step status string is considered
// terminal (authoritative). Used to gate the log-line parser fallback.
func isTerminalStepStatus(s string) bool {
	return s == "done" || s == "failed" || s == "skipped"
}

// upsertCronFeedEntry adds or updates a cron feed entry for job.
// If a "cron-<job>" entry already exists it is updated in-place; otherwise a
// new entry is prepended. exitStatus == 0 → FeedDone, else → FeedFailed.
func (m Model) upsertCronFeedEntry(job string, exitStatus int) Model {
	id := "cron-" + job
	status := FeedDone
	if exitStatus != 0 {
		status = FeedFailed
	}
	for i := range m.feed {
		if m.feed[i].id == id {
			m.feed[i].status = status
			m.feed[i].ts = time.Now()
			return m
		}
	}
	entry := feedEntry{
		id:     id,
		title:  "cron: " + job,
		status: status,
		ts:     time.Now(),
	}
	return m.prependFeedEntry(entry)
}
