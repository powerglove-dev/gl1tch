// pipeline_bus.go — busd subscription and feed seeding for the Deck.
//
// Tasks 7.1–7.8: authoritative feed updates driven by busd pipeline/step/cron
// events, with the log-line parser demoted to a fallback for terminals.
package console

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net"
	"strings"
	"time"
	"unicode/utf8"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/8op-org/gl1tch/internal/activity"
	"github.com/8op-org/gl1tch/internal/busd"
	"github.com/8op-org/gl1tch/internal/busd/topics"
	"github.com/8op-org/gl1tch/internal/npcname"
	"github.com/8op-org/gl1tch/internal/store"
	"github.com/8op-org/gl1tch/internal/telemetry"
)

// ── orphan recovery ───────────────────────────────────────────────────────────

// orphanRecoveryMsg is returned by recoverOrphanedRunsCmd with the IDs of
// runs that were marked interrupted on startup.
type orphanRecoveryMsg struct{ recoveredIDs []int64 }

// recoverOrphanedRunsCmd marks any runs that were left in-flight when glitch
// last closed (finished_at=NULL, exit_status=NULL, no pending clarification)
// as interrupted. Safe to call with a nil store.
func recoverOrphanedRunsCmd(st *store.Store) tea.Cmd {
	return func() tea.Msg {
		if st == nil {
			return orphanRecoveryMsg{}
		}
		ids, _ := st.RecoverOrphanedRuns()
		return orphanRecoveryMsg{recoveredIDs: ids}
	}
}

// ── agent run event publishing ────────────────────────────────────────────────

// pendingClarificationsMsg carries pending clarification requests loaded from
// the DB on TUI startup.
type pendingClarificationsMsg struct{ reqs []store.ClarificationRequest }

// loadPendingClarificationsCmd returns a tea.Cmd that queries the store for
// any unanswered clarification requests and delivers them as a
// pendingClarificationsMsg. Safe to call with a nil store.
func loadPendingClarificationsCmd(st *store.Store) tea.Cmd {
	if st == nil {
		return nil
	}
	return func() tea.Msg {
		reqs, _ := st.LoadPendingClarifications()
		return pendingClarificationsMsg{reqs: reqs}
	}
}

// publishClarificationReplyCmd returns a tea.Cmd that publishes a
// ClarificationReply event on the bus so the blocking AskClarification call in
// the pipeline runner can unblock and resume execution.
func publishClarificationReplyCmd(reply store.ClarificationReply) tea.Cmd {
	return func() tea.Msg {
		sockPath, err := busd.SocketPath()
		if err != nil {
			return nil
		}
		_ = busd.PublishEvent(sockPath, topics.ClarificationReply, reply)
		return nil
	}
}

// publishAgentEventCmd returns a tea.Cmd that publishes an agent run lifecycle
// event to busd. Errors are silently ignored so the job lifecycle is unaffected
// when the bus is unavailable.
func publishAgentEventCmd(topic string, payload any) tea.Cmd {
	return func() tea.Msg {
		sockPath, err := busd.SocketPath()
		if err != nil {
			return nil
		}
		_ = busd.PublishEvent(sockPath, topic, payload)
		return nil
	}
}

// appendActivityCmd returns a tea.Cmd that appends an ActivityEvent to the
// default JSONL path. Errors are silently ignored so the job lifecycle is
// unaffected when the file system is unavailable.
func appendActivityCmd(agent, label, kind, status string) tea.Cmd {
	return func() tea.Msg {
		_ = activity.AppendEvent(activity.DefaultPath(), activity.Now(kind, agent, label, status))
		return nil
	}
}

// activityLabel returns the first 60 runes of s, suitable as an activity label.
func activityLabel(s string) string {
	const max = 60
	if utf8.RuneCountInString(s) <= max {
		return s
	}
	runes := []rune(s)
	return string(runes[:max-1]) + "…"
}

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
	// Walk from the oldest end (tail) looking for a non-running/non-paused entry.
	for i := len(m.feed) - 1; i >= 0; i-- {
		if m.feed[i].status != FeedRunning && m.feed[i].status != FeedPaused {
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

		// Build a set of run IDs that have a pending (unanswered) clarification
		// so we can seed them as FeedPaused instead of FeedRunning.
		pausedRunIDs := make(map[string]struct{})
		if pending, perr := s.LoadPendingClarifications(); perr == nil {
			for _, req := range pending {
				pausedRunIDs[req.RunID] = struct{}{}
			}
		}

		entries := make([]feedEntry, 0, len(runs))
		for _, r := range runs {
			runIDStr := fmt.Sprintf("%d", r.ID)
			status := FeedDone
			if r.ExitStatus != nil && *r.ExitStatus != 0 {
				status = FeedFailed
			} else if r.ExitStatus == nil {
				if _, paused := pausedRunIDs[runIDStr]; paused {
					status = FeedPaused
				} else {
					status = FeedRunning
				}
			}
			e := feedEntry{
				id:     npcname.FromID(r.ID),
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
	topic       string
	payload     json.RawMessage
	traceparent string
}

// pipelineBusDisconnectedMsg signals that the busd connection was lost.
type pipelineBusDisconnectedMsg struct{}

// pipelineBusConnectMsg is returned by tryPipelineBusSubscribeCmd on success.
type pipelineBusConnectMsg struct {
	ch chan pipelineBusEventMsg
}

// tryPipelineBusSubscribeCmd attempts to connect to busd and subscribe to
// pipeline.run.*, pipeline.step.*, and cron.job.* topics, plus any extra
// topics derived from the widget registry. Returns pipelineBusConnectMsg on
// success or pipelineBusDisconnectedMsg on failure.
func tryPipelineBusSubscribeCmd(extraTopics ...string) tea.Cmd {
	return func() tea.Msg {
		sockPath, err := busd.SocketPath()
		if err != nil {
			return pipelineBusDisconnectedMsg{}
		}
		conn, err := net.DialTimeout("unix", sockPath, 200*time.Millisecond)
		if err != nil {
			return pipelineBusDisconnectedMsg{}
		}
		subs := append(
			[]string{"pipeline.run.*", "pipeline.step.*", "cron.job.*", "cron.entry.*", topics.ClarificationRequested, topics.GameRunScored, "notification.*"},
			extraTopics...,
		)
		reg, _ := json.Marshal(map[string]any{
			"name":      "deck",
			"subscribe": subs,
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
					Event       string          `json:"event"`
					Payload     json.RawMessage `json:"payload"`
					Traceparent string          `json:"traceparent,omitempty"`
				}
				if json.Unmarshal(scanner.Bytes(), &frame) == nil {
					ch <- pipelineBusEventMsg{topic: frame.Event, payload: frame.Payload, traceparent: frame.Traceparent}
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

// waitForNarrationCmd returns a tea.Cmd that blocks until a narration string is
// available on ch, then delivers it as a glitchNarrationMsg.
func waitForNarrationCmd(ch chan string) tea.Cmd {
	return func() tea.Msg {
		text, ok := <-ch
		if !ok || text == "" {
			return nil
		}
		return glitchNarrationMsg{text: text}
	}
}

// waitForGameFeedCmd returns a tea.Cmd that blocks until the next OTel game
// span event arrives on ch (from telemetry.FeedEvents()), then delivers it.
func waitForGameFeedCmd(ch <-chan telemetry.FeedSpanEvent) tea.Cmd {
	return func() tea.Msg {
		evt, ok := <-ch
		if !ok {
			return nil
		}
		return evt
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
			// Inherit the tmux window from the active job handle so signal board
			// "enter" can navigate to the running pipeline window.
			for _, jh := range m.activeJobs {
				if jh.pipelineName == payload.Pipeline {
					entry.tmuxWindow = jh.tmuxWindow
					break
				}
			}
			m = m.prependFeedEntry(entry)
		}

	case topics.RunCompleted:
		var payload struct {
			RunID int64 `json:"run_id"`
		}
		if json.Unmarshal(msg.payload, &payload) == nil {
			id := fmt.Sprintf("run-%d", payload.RunID)
			// If any step recorded a failure, promote the overall status to FeedFailed.
			finalStatus := FeedDone
			for _, e := range m.feed {
				if e.id == id {
					for _, s := range e.steps {
						if s.status == "failed" {
							finalStatus = FeedFailed
						}
					}
					break
				}
			}
			m = m.updateFeedEntryStatus(id, finalStatus)
			// Settle any steps still in "running" state — they completed since we
			// received no explicit StepDone event for them.
			m = m.settleRunningSteps(id, "done")
		}

	case topics.RunFailed:
		var payload struct {
			RunID int64 `json:"run_id"`
		}
		if json.Unmarshal(msg.payload, &payload) == nil {
			id := fmt.Sprintf("run-%d", payload.RunID)
			m = m.updateFeedEntryStatus(id, FeedFailed)
			m = m.settleRunningSteps(id, "failed")
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
			RunID  int64  `json:"run_id"`
			Step   string `json:"step"`
			Output struct {
				Value string `json:"value"`
			} `json:"output"`
		}
		if json.Unmarshal(msg.payload, &payload) == nil {
			entryID := fmt.Sprintf("run-%d", payload.RunID)
			m = m.updateFeedEntryStep(entryID, payload.Step, "done")
			if lines := splitStepOutput(payload.Output.Value); len(lines) > 0 {
				m = m.appendStepLines(entryID, payload.Step, lines)
			}
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

	// ── cron.entry.* ──────────────────────────────────────────────────────

	case topics.CronEntryUpdated:
		// A cron entry was renamed. Update any in-memory feed/signal entries
		// that still carry the old name so they reflect the rename immediately.
		// The cron panel itself reads LoadConfig() on each render and self-heals.
		var payload struct {
			OldName string `json:"old_name"`
			NewName string `json:"new_name"`
		}
		if json.Unmarshal(msg.payload, &payload) == nil && payload.OldName != "" && payload.NewName != "" {
			oldCronTitle := "cron: " + payload.OldName
			newCronTitle := "cron: " + payload.NewName
			// Signal board reads from m.feed; updating feed entries updates both.
			for i := range m.feed {
				if m.feed[i].title == oldCronTitle {
					m.feed[i].title = newCronTitle
				}
			}
		}

	// ── notification.* ────────────────────────────────────────────────────────

	case topics.NotificationErrorDiagnosed, topics.NotificationAgentLoopComplete:
		var payload struct {
			Session  string `json:"session"`
			Title    string `json:"title"`
			Body     string `json:"body"`
			Severity string `json:"severity"`
		}
		if json.Unmarshal(msg.payload, &payload) == nil {
			// Prepend a feed entry showing the notification title.
			id := fmt.Sprintf("notif-%d", time.Now().UnixNano())
			title := payload.Title
			if title == "" {
				title = msg.topic
			}
			entry := feedEntry{
				id:     id,
				title:  title,
				status: FeedDone,
				ts:     time.Now(),
			}
			m = m.prependFeedEntry(entry)

			// Mark the target session as needing attention.
			sessionName := payload.Session
			if sessionName == "" {
				sessionName = "main"
			}
			if m.glitchChat.sessions != nil {
				m.glitchChat.sessions.markAttention(sessionName)
			}
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

// updateFeedEntryStatusByRunID sets the FeedStatus of the entry whose id is
// "run-<runID>". Useful when only the numeric run ID string is available (e.g.
// from ClarificationRequested payloads).
func (m Model) updateFeedEntryStatusByRunID(runID string, status FeedStatus) Model {
	return m.updateFeedEntryStatus("run-"+runID, status)
}

// settleRunningSteps updates all steps in the feed entry that are still in
// "running" state to finalStatus ("done" or "failed"). Called when a RunCompleted
// or RunFailed event arrives but some steps never received a StepDone/StepFailed
// event, leaving them stuck in "running".
func (m Model) settleRunningSteps(entryID, finalStatus string) Model {
	for i := range m.feed {
		if m.feed[i].id == entryID {
			for j := range m.feed[i].steps {
				if m.feed[i].steps[j].status == "running" {
					m.feed[i].steps[j].status = finalStatus
				}
			}
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

// splitStepOutput splits a step output value into trimmed non-empty lines,
// capping at the last maxStepOutputLines lines.
func splitStepOutput(value string) []string {
	const maxStepOutputLines = 5
	raw := strings.Split(value, "\n")
	var lines []string
	for _, l := range raw {
		if t := strings.TrimSpace(l); t != "" {
			lines = append(lines, t)
		}
	}
	if len(lines) > maxStepOutputLines {
		lines = lines[len(lines)-maxStepOutputLines:]
	}
	return lines
}

// appendStepLines stores output lines into the matching step within the feed
// entry identified by entryID. If the step is not found, the call is a no-op.
func (m Model) appendStepLines(entryID, stepID string, lines []string) Model {
	for i := range m.feed {
		if m.feed[i].id == entryID {
			for j := range m.feed[i].steps {
				if m.feed[i].steps[j].id == stepID {
					m.feed[i].steps[j].lines = append(m.feed[i].steps[j].lines, lines...)
					return m
				}
			}
			return m
		}
	}
	return m
}
