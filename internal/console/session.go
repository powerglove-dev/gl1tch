package console

import (
	"errors"
	"os"
	"path/filepath"

	tea "github.com/charmbracelet/bubbletea"
	"gopkg.in/yaml.v3"
)

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
	cwd      string        // working directory for this session
	backend  glitchBackend // AI provider+model for this session
	resumeID string        // provider-side conversation ID for resumption (e.g. Claude session_id)
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

// delete removes a session by name. Returns false if not found, if the
// session is currently active, or if the name is "main".
func (r *SessionRegistry) delete(name string) bool {
	if name == "main" || name == r.active {
		return false
	}
	for i, s := range r.sessions {
		if s.name == name {
			r.sessions = append(r.sessions[:i], r.sessions[i+1:]...)
			return true
		}
	}
	return false
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

// ── Session persistence ────────────────────────────────────────────────────────

// sessionRecord is the on-disk representation of a single session.
type sessionRecord struct {
	Name     string `yaml:"name"`
	CWD      string `yaml:"cwd,omitempty"`
	Backend  string `yaml:"backend,omitempty"` // backend.name() slug, e.g. "ollama/llama3.2"
	ResumeID string `yaml:"resume_id,omitempty"`
}

// sessionFile is the top-level structure of sessions.yaml.
type sessionFile struct {
	Active   string          `yaml:"active"`
	Sessions []sessionRecord `yaml:"sessions"`
}

// loadSessions reads cfgDir/sessions.yaml and returns the stored records and
// the active session name. Returns nil records (and no error) if the file
// does not exist.
func loadSessions(cfgDir string) ([]sessionRecord, string, error) {
	data, err := os.ReadFile(filepath.Join(cfgDir, "sessions.yaml"))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, "main", nil
		}
		return nil, "main", err
	}
	var sf sessionFile
	if err := yaml.Unmarshal(data, &sf); err != nil {
		return nil, "main", err
	}
	if sf.Active == "" {
		sf.Active = "main"
	}
	return sf.Sessions, sf.Active, nil
}

// saveSessionsCmd captures the current session state synchronously (safe to
// call from the BubbleTea Update loop) and returns a Cmd that writes it to
// cfgDir/sessions.yaml in the background. activeName/activeCWD/activeBackend
// are the panel's live values for the currently displayed session, which may
// differ from what the registry holds until the next switchToSession call.
func saveSessionsCmd(cfgDir string, reg *SessionRegistry, activeName, activeCWD string, activeBackend glitchBackend) tea.Cmd {
	sf := sessionFile{Active: reg.active}
	for _, s := range reg.sessions {
		cwd := s.cwd
		backendName := ""
		if s.name == activeName {
			cwd = activeCWD
			if activeBackend != nil {
				backendName = activeBackend.name()
			}
		} else if s.backend != nil {
			backendName = s.backend.name()
		}
		sf.Sessions = append(sf.Sessions, sessionRecord{
			Name:     s.name,
			CWD:      cwd,
			Backend:  backendName,
			ResumeID: s.resumeID,
		})
	}
	return func() tea.Msg {
		data, err := yaml.Marshal(sf)
		if err != nil {
			return nil
		}
		path := filepath.Join(cfgDir, "sessions.yaml")
		_ = os.MkdirAll(filepath.Dir(path), 0o755)
		_ = os.WriteFile(path, data, 0o644)
		return nil
	}
}
