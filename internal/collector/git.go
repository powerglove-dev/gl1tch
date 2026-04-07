// git.go holds the low-level git invocation helpers used by the
// unified WorkspaceCollector. The standalone GitCollector type was
// retired when the workspace collector replaced the split
// directories/git/github trio — these functions are still useful as
// thin wrappers around git CLI commands, just no longer their own
// goroutine.
package collector

import (
	"fmt"
	"os/exec"
	"strings"
	"time"
)

type gitCommit struct {
	sha       string
	author    string
	message   string
	body      string
	files     []string
	timestamp time.Time
}

// gitLog returns commits in reverse chronological order. rangeArg can
// either be a git range expression (e.g. "abc123..HEAD") or a "-N"
// limit to get the most recent N commits.
func gitLog(repo, rangeArg string) ([]gitCommit, error) {
	// Format: SHA\x1fauthor\x1ftimestamp\x1fsubject\x1fbody\x1e
	format := "%H%x1f%an%x1f%aI%x1f%s%x1f%b%x1e"
	args := []string{"-C", repo, "log", "--format=" + format, "--name-only"}
	args = append(args, rangeArg)

	out, err := exec.Command("git", args...).Output()
	if err != nil {
		return nil, fmt.Errorf("git log: %w", err)
	}

	raw := strings.TrimSpace(string(out))
	if raw == "" {
		return nil, nil
	}

	records := strings.Split(raw, "\x1e")
	var commits []gitCommit

	for _, rec := range records {
		rec = strings.TrimSpace(rec)
		if rec == "" {
			continue
		}

		lines := strings.SplitN(rec, "\n", 2)
		fields := strings.SplitN(lines[0], "\x1f", 5)
		if len(fields) < 4 {
			continue
		}

		ts, _ := time.Parse(time.RFC3339, fields[2])

		c := gitCommit{
			sha:       fields[0],
			author:    fields[1],
			timestamp: ts,
			message:   fields[3],
		}
		if len(fields) >= 5 {
			c.body = strings.TrimSpace(fields[4])
		}

		if len(lines) > 1 {
			for _, f := range strings.Split(strings.TrimSpace(lines[1]), "\n") {
				f = strings.TrimSpace(f)
				if f != "" {
					c.files = append(c.files, f)
				}
			}
		}

		commits = append(commits, c)
	}

	return commits, nil
}

func gitCurrentBranch(repo string) string {
	out, err := exec.Command("git", "-C", repo, "rev-parse", "--abbrev-ref", "HEAD").Output()
	if err != nil {
		return "unknown"
	}
	return strings.TrimSpace(string(out))
}
