package store

import "time"

// ClarificationRequest is the payload published on topics.ClarificationRequested
// when an agent run needs user input before it can continue.
type ClarificationRequest struct {
	RunID    string    `json:"run_id"`
	Question string    `json:"question"`
	AskedAt  time.Time `json:"asked_at"`
}

// ClarificationReply is the payload published on topics.ClarificationReply
// when the user has answered a pending clarification question.
type ClarificationReply struct {
	RunID  string `json:"run_id"`
	Answer string `json:"answer"`
}
