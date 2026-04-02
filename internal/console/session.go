package console

// SessionStatus describes the attention state of a chat session.
type SessionStatus int

const (
	// SessionActive is the session currently being viewed.
	SessionActive SessionStatus = iota
	// SessionIdle is a background session with no new activity.
	SessionIdle
	// SessionUnread has new messages since the user last viewed it.
	SessionUnread
	// SessionAttention has activity requiring action (errors, failures, etc.).
	SessionAttention
)

// chatSession holds the conversation history and status for a named session.
type chatSession struct {
	name     string
	messages []glitchEntry
	turns    []glitchTurn
	status   SessionStatus
}

// SessionRegistry manages named chat sessions.
type SessionRegistry struct {
	sessions []*chatSession
	active   string
}

func newSessionRegistry() *SessionRegistry {
	return &SessionRegistry{
		sessions: []*chatSession{{name: "main", status: SessionActive}},
		active:   "main",
	}
}

// Active returns the currently active session.
func (r *SessionRegistry) Active() *chatSession {
	return r.get(r.active)
}

// NeedsFooter reports whether the session footer should be rendered (2+ sessions).
func (r *SessionRegistry) NeedsFooter() bool {
	return len(r.sessions) >= 2
}

// AttentionSession returns the name of the first non-active session needing
// attention or having unread messages, or "" if none.
func (r *SessionRegistry) AttentionSession() string {
	for _, s := range r.sessions {
		if s.name != r.active && s.status >= SessionUnread {
			return s.name
		}
	}
	return ""
}

func (r *SessionRegistry) get(name string) *chatSession {
	for _, s := range r.sessions {
		if s.name == name {
			return s
		}
	}
	return nil
}

// create adds a new idle session. Caller must verify name is unique.
func (r *SessionRegistry) create(name string) *chatSession {
	s := &chatSession{name: name, status: SessionIdle}
	r.sessions = append(r.sessions, s)
	return s
}

// switchTo makes the named session active and marks the previous one idle.
// Returns false if the session does not exist.
func (r *SessionRegistry) switchTo(name string) bool {
	s := r.get(name)
	if s == nil {
		return false
	}
	if cur := r.Active(); cur != nil && cur.name != name {
		cur.status = SessionIdle
	}
	r.active = name
	s.status = SessionActive
	return true
}

// markUnread bumps a non-active session to unread (no-op if already higher).
func (r *SessionRegistry) markUnread(name string) {
	s := r.get(name)
	if s == nil || s.name == r.active {
		return
	}
	if s.status < SessionUnread {
		s.status = SessionUnread
	}
}

// markAttention bumps a non-active session to needs-attention.
func (r *SessionRegistry) markAttention(name string) {
	s := r.get(name)
	if s == nil || s.name == r.active {
		return
	}
	s.status = SessionAttention
}
