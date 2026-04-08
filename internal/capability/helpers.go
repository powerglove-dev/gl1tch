package capability

import "sync"

// ghMu serializes gh CLI invocations across the entire process. The gh
// binary is happy to run concurrent commands but every invocation loads the
// user's keychain entry, which on macOS pops a system security prompt if the
// keychain item's ACL hasn't been granted yet — running ten parallel commands
// surfaces ten prompts. Serialising keeps the prompt count to one per
// session.
var ghMu sync.Mutex

// truncate returns s cut to at most n characters with an ellipsis suffix
// when truncation actually happens. Used by several helpers that stamp
// human-readable "message" fields into indexed documents.
func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
