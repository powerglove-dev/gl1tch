package store

import (
	"database/sql"
	"time"
)

// ClarificationRequest is the payload published on topics.ClarificationRequested
// when an agent run needs user input before it can continue.
type ClarificationRequest struct {
	RunID    string    `json:"run_id"`
	StepID   string    `json:"step_id"`
	Question string    `json:"question"`
	AskedAt  time.Time `json:"asked_at"`
	Output   string    `json:"output"` // partial output up to the question
	Answer   string    `json:"answer,omitempty"`
}

// ClarificationReply is the payload published on topics.ClarificationReply
// when the user has answered a pending clarification question.
type ClarificationReply struct {
	RunID  string `json:"run_id"`
	Answer string `json:"answer"`
}

// SaveClarification persists a clarification question to the DB so it survives
// TUI restarts. Uses INSERT OR REPLACE so repeated calls for the same runID
// are idempotent.
func (s *Store) SaveClarification(runID, stepID, question, output string) error {
	_, err := s.db.Exec(
		`INSERT OR REPLACE INTO clarifications (run_id, step_id, question, output, asked_at) VALUES (?, ?, ?, ?, ?)`,
		runID, stepID, question, output, time.Now().UnixMilli(),
	)
	return err
}

// LoadPendingClarifications returns all clarification requests that have not
// yet been answered, ordered by asked_at descending (most recent first).
func (s *Store) LoadPendingClarifications() ([]ClarificationRequest, error) {
	rows, err := s.db.Query(
		`SELECT run_id, step_id, question, output, asked_at FROM clarifications WHERE answer IS NULL ORDER BY asked_at DESC`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var reqs []ClarificationRequest
	for rows.Next() {
		var runID, question, output string
		var stepID sql.NullString
		var askedAtMs int64
		if err := rows.Scan(&runID, &stepID, &question, &output, &askedAtMs); err != nil {
			return nil, err
		}
		reqs = append(reqs, ClarificationRequest{
			RunID:    runID,
			StepID:   stepID.String,
			Question: question,
			Output:   output,
			AskedAt:  time.UnixMilli(askedAtMs),
		})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return reqs, nil
}

// LoadClarificationForRun returns the clarification for the given runID whether
// or not it has been answered. Returns nil, nil if no row exists.
func (s *Store) LoadClarificationForRun(runID string) (*ClarificationRequest, error) {
	row := s.db.QueryRow(
		`SELECT run_id, step_id, question, output, asked_at, answer FROM clarifications WHERE run_id = ? LIMIT 1`,
		runID,
	)
	var req ClarificationRequest
	var askedAtMs int64
	var stepID, answer sql.NullString
	err := row.Scan(&req.RunID, &stepID, &req.Question, &req.Output, &askedAtMs, &answer)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	req.StepID = stepID.String
	req.Answer = answer.String
	req.AskedAt = time.UnixMilli(askedAtMs)
	return &req, nil
}

// AnswerClarification records the user's answer for the given runID.
func (s *Store) AnswerClarification(runID, answer string) error {
	_, err := s.db.Exec(
		`UPDATE clarifications SET answer = ?, answered_at = ? WHERE run_id = ?`,
		answer, time.Now().UnixMilli(), runID,
	)
	return err
}

// PollClarificationAnswer checks whether a non-empty answer has been recorded
// for the given runID. found is true only when the row exists and the answer
// column is non-NULL and non-empty.
func (s *Store) PollClarificationAnswer(runID string) (answer string, found bool, err error) {
	var ans sql.NullString
	row := s.db.QueryRow(`SELECT answer FROM clarifications WHERE run_id = ?`, runID)
	if scanErr := row.Scan(&ans); scanErr != nil {
		if scanErr == sql.ErrNoRows {
			return "", false, nil
		}
		return "", false, scanErr
	}
	if ans.Valid && ans.String != "" {
		return ans.String, true, nil
	}
	return "", false, nil
}

// DeleteClarification removes the clarification row for the given runID.
func (s *Store) DeleteClarification(runID string) error {
	_, err := s.db.Exec(`DELETE FROM clarifications WHERE run_id = ?`, runID)
	return err
}
