// threads.go is the desktop App's Wails-bindable surface for the
// chat-threads + chat-first-ui openspec changes. It is intentionally
// thin: every method is a one-line delegation to a glitchd.ThreadHosts
// method, so the actual data model and slash dispatcher live in
// pkg/glitchd/threads.go where they're testable in Go and reusable from
// non-desktop callers (e.g. a future glitch-tui or HTTP frontend).
package main

import (
	"github.com/8op-org/gl1tch/pkg/glitchd"
)

// ensureThreads is the lazy accessor App methods use. It constructs the
// thread-host registry on first access so the App constructor does not
// have to know about glitchd.ThreadHosts at all.
func (a *App) ensureThreads() *glitchd.ThreadHosts {
	a.threadsOnce.Do(func() {
		a.threads = glitchd.NewThreadHosts()
	})
	return a.threads
}

// DispatchSlash runs a slash command line through the workspace's slash
// dispatcher and appends the resulting messages to the store. Returns a
// JSON envelope ({ok:true,detail} or {ok:false,error}) the frontend can
// branch on. Lines that don't start with `/` are appended as a free-form
// user message.
func (a *App) DispatchSlash(workspaceID, line, scope string) string {
	return a.ensureThreads().DispatchSlash(workspaceID, line, scope)
}

// GetMainScrollback returns the workspace's main-chat messages as JSON.
func (a *App) GetMainScrollback(workspaceID string) string {
	return a.ensureThreads().MainScrollback(workspaceID)
}

// GetThreadMessages returns the messages inside one thread as JSON.
func (a *App) GetThreadMessages(workspaceID, threadID string) string {
	return a.ensureThreads().ThreadMessages(workspaceID, threadID)
}

// ListThreads returns every thread in a workspace.
func (a *App) ListThreads(workspaceID string) string {
	return a.ensureThreads().ListThreads(workspaceID)
}

// SpawnDrillThreadFromEvidence spawns a drill thread under the parent
// message and seeds it with the supplied evidence. evidenceJSON is the
// JSON shape of chatui.EvidenceBundleItem (a single click on an evidence
// row in the bundle widget serialises one of these).
func (a *App) SpawnDrillThreadFromEvidence(workspaceID, parentMessageID, evidenceJSON string) string {
	return a.ensureThreads().SpawnDrillThreadFromEvidence(workspaceID, parentMessageID, evidenceJSON)
}

// SpawnThreadOnMessage spawns (or returns the existing) thread under any
// chat message in the main scrollback. The frontend calls this on
// click-to-thread.
func (a *App) SpawnThreadOnMessage(workspaceID, parentMessageID string) string {
	return a.ensureThreads().SpawnThreadOnMessage(workspaceID, parentMessageID)
}

// CloseThread freezes a thread and stamps a one-line summary on it.
func (a *App) CloseThread(workspaceID, threadID, summary string) string {
	return a.ensureThreads().CloseThread(workspaceID, threadID, summary)
}

// ReopenThread transitions a closed thread back to open.
func (a *App) ReopenThread(workspaceID, threadID string) string {
	return a.ensureThreads().ReopenThread(workspaceID, threadID)
}
