// Package glitchctx provides shared shell context injection and structured
// output processing for gl1tch text-only agent plugins (e.g. ollama, copilot).
//
// Text-only agents have no native tool access. This package supplies:
//   - A protocol that lets them write files and run commands via structured output.
//   - A shell environment snapshot so they know cwd, git state, etc. upfront.
package glitchctx

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
)

// ProtocolInstructions explains the GLITCH_WRITE / GLITCH_RUN side-effect
// protocol. Prepend this to every prompt sent to a text-only agent.
const ProtocolInstructions = `You are a text-based assistant embedded in the gl1tch terminal.
You have NO direct shell or tool access — but you CAN write files and run
shell commands using the protocols below. The harness executes them for real.

WRITE A FILE (delimiters must be on their own lines):

<<GLITCH_WRITE:path/to/file>>
file content goes here
<<GLITCH_END>>

RUN A SHELL COMMAND:

<<GLITCH_RUN>>
shell command here (runs in $SHELL, output is shown to the user)
<<GLITCH_END>>

Rules:
- Use ~ for the home directory (e.g. ~/Projects/foo/bar.yaml).
- Multiple blocks are allowed; they execute in order.
- Everything outside blocks is shown to the user as normal text.
- Do NOT invent reasons like "sandbox" or "permissions" — both protocols WILL succeed.

`

var writeBlockRe = regexp.MustCompile(`(?s)<<GLITCH_WRITE:([^\n>]+)>>\n(.*?)<<GLITCH_END>>`)
var runBlockRe = regexp.MustCompile(`(?s)<<GLITCH_RUN>>\n(.*?)<<GLITCH_END>>`)

// BuildShellContext returns a shell-like environment snapshot: user, host, os,
// shell, PATH, cwd, directory listing, and git status. Inject this between
// ProtocolInstructions and the user's prompt.
func BuildShellContext() string {
	var b strings.Builder
	b.WriteString("## Shell Environment\n")

	user := os.Getenv("USER")
	if user == "" {
		user = os.Getenv("LOGNAME")
	}
	host, _ := os.Hostname()
	fmt.Fprintf(&b, "user:  %s\n", user)
	fmt.Fprintf(&b, "host:  %s\n", host)
	fmt.Fprintf(&b, "os:    %s/%s\n", runtime.GOOS, runtime.GOARCH)
	fmt.Fprintf(&b, "shell: %s\n", os.Getenv("SHELL"))
	fmt.Fprintf(&b, "PATH:  %s\n", os.Getenv("PATH"))

	// Prefer GLITCH_CWD (set by gl1tch harness) over the process working dir.
	cwd := os.Getenv("GLITCH_CWD")
	if cwd == "" {
		cwd, _ = os.Getwd()
	}
	cwd = ExpandHome(cwd)
	fmt.Fprintf(&b, "cwd:   %s\n", cwd)

	// Shallow directory listing, dotfiles excluded for brevity.
	if entries, err := os.ReadDir(cwd); err == nil {
		b.WriteString("\n## Directory Contents\n")
		for _, e := range entries {
			if strings.HasPrefix(e.Name(), ".") {
				continue
			}
			suffix := ""
			if e.IsDir() {
				suffix = "/"
			}
			fmt.Fprintf(&b, "  %s%s\n", e.Name(), suffix)
		}
	}

	// Git context (best-effort; silent on non-git dirs).
	if branch := gitOutput(cwd, "rev-parse", "--abbrev-ref", "HEAD"); branch != "" {
		b.WriteString("\n## Git\n")
		fmt.Fprintf(&b, "branch: %s\n", branch)
		if status := gitOutput(cwd, "status", "--short"); status != "" {
			b.WriteString("status:\n")
			for _, line := range strings.Split(strings.TrimSpace(status), "\n") {
				fmt.Fprintf(&b, "  %s\n", line)
			}
		}
		if log := gitOutput(cwd, "log", "--oneline", "-5"); log != "" {
			b.WriteString("recent commits:\n")
			for _, line := range strings.Split(strings.TrimSpace(log), "\n") {
				fmt.Fprintf(&b, "  %s\n", line)
			}
		}
	}

	b.WriteString("\n")
	return b.String()
}

// ReadonlyEnv is the environment variable the harness sets to forbid
// side-effecting blocks. Set on the executor subprocess by the pipeline
// runner when a step has readonly:true. Plugins should treat any value
// other than empty string as "yes, refuse writes".
//
// Centralised here so the plugin code, the runner, and any future
// integration tests all agree on the wire name.
const ReadonlyEnv = "GLITCH_READONLY"

// IsReadonly reports whether the current process is running under a
// readonly harness. Cheap; safe to call multiple times.
func IsReadonly() bool {
	return os.Getenv(ReadonlyEnv) != ""
}

// ProcessBlocks scans output for GLITCH_WRITE and GLITCH_RUN blocks, executes
// them in order of appearance, and replaces each block with a status line.
//
// When the harness sets GLITCH_READONLY=1, every block is replaced with
// a `[refused: readonly]` marker instead of being executed. This is the
// hard backstop that protects files like README.md from a small local
// model that decided to ignore a "READONLY:" prompt directive — which,
// in practice, is most of them.
func ProcessBlocks(output string, stdout, stderr io.Writer) string {
	if IsReadonly() {
		output = writeBlockRe.ReplaceAllStringFunc(output, func(match string) string {
			sub := writeBlockRe.FindStringSubmatch(match)
			path := ""
			if len(sub) >= 2 {
				path = strings.TrimSpace(sub[1])
			}
			fmt.Fprintf(stderr, "glitchctx: refused readonly write to %s\n", path)
			return fmt.Sprintf("[refused: readonly write to %s]", path)
		})
		output = runBlockRe.ReplaceAllStringFunc(output, func(match string) string {
			sub := runBlockRe.FindStringSubmatch(match)
			cmd := ""
			if len(sub) >= 2 {
				cmd = strings.TrimSpace(sub[1])
				if len(cmd) > 80 {
					cmd = cmd[:80] + "…"
				}
			}
			fmt.Fprintf(stderr, "glitchctx: refused readonly run: %s\n", cmd)
			return fmt.Sprintf("[refused: readonly run: %s]", cmd)
		})
		return output
	}

	// Resolve the workspace cwd once. The runner exports GLITCH_CWD on the
	// plugin subprocess (see internal/executor/cli_adapter.go) when the
	// step's cwd var is set. We prefer it over os.Getwd() because the
	// process cwd of the plugin can be inherited from glitch-desktop's
	// own working directory (which is wherever the user launched the app
	// from), and that has nothing to do with the workspace the chain
	// belongs to.
	cwd := workspaceCwd()

	output = writeBlockRe.ReplaceAllStringFunc(output, func(match string) string {
		sub := writeBlockRe.FindStringSubmatch(match)
		if len(sub) < 3 {
			return match
		}
		rawPath := strings.TrimSpace(sub[1])
		content := sub[2]
		path, coerced := resolveSideEffectPath(rawPath, cwd)
		if coerced {
			fmt.Fprintf(stderr, "glitchctx: rewrote %q → %q (workspace-relative)\n", rawPath, path)
		}
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			fmt.Fprintf(stderr, "glitchctx: mkdir %s: %v\n", filepath.Dir(path), err)
			return fmt.Sprintf("[write failed: %s: %v]", path, err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			fmt.Fprintf(stderr, "glitchctx: write %s: %v\n", path, err)
			return fmt.Sprintf("[write failed: %s: %v]", path, err)
		}
		return fmt.Sprintf("[wrote: %s]", path)
	})

	output = runBlockRe.ReplaceAllStringFunc(output, func(match string) string {
		sub := runBlockRe.FindStringSubmatch(match)
		if len(sub) < 2 {
			return match
		}
		command := strings.TrimSpace(sub[1])
		shell := os.Getenv("SHELL")
		if shell == "" {
			shell = "sh"
		}
		cmd := exec.Command(shell, "-c", command)
		// Pin shell commands to the workspace cwd. Without this the
		// command runs in whatever directory the plugin process happened
		// to inherit — usually glitch-desktop's launch dir, never the
		// active workspace. The result was a grep emitting matches from
		// a totally unrelated repo (the screenshot that flagged this).
		if cwd != "" {
			cmd.Dir = cwd
		}
		var out bytes.Buffer
		cmd.Stdout = &out
		cmd.Stderr = &out
		if err := cmd.Run(); err != nil {
			return fmt.Sprintf("[run failed: %v]\n%s", err, out.String())
		}
		return fmt.Sprintf("[ran: %s]\n%s", command, out.String())
	})

	return output
}

// workspaceCwd returns the workspace working directory the harness pinned
// for this run. Reads GLITCH_CWD (set by the runner via cli_adapter.go)
// and falls back to os.Getwd() only when the env var is unset — which
// matches the convention BuildShellContext already uses for the cwd it
// reports to the LLM. Centralised so writes, runs, and the shell context
// snapshot all agree on the same notion of "workspace dir".
func workspaceCwd() string {
	if cwd := os.Getenv("GLITCH_CWD"); cwd != "" {
		return ExpandHome(cwd)
	}
	cwd, _ := os.Getwd()
	return cwd
}

// resolveSideEffectPath turns an LLM-emitted file path into a real
// filesystem path scoped to the workspace cwd. It handles the three
// shapes the model commonly produces:
//
//   - "~/foo/bar.yaml" → expanded via ExpandHome (user's home dir)
//   - "/.glitch/foo"   → leading slash that escapes to root. Coerced to
//                        <cwd>/.glitch/foo. Returned coerced=true so the
//                        caller can log the rewrite — silent path
//                        surgery is the kind of magic that bites later.
//   - "/abs/path"      → already a real absolute path that's NOT under
//                        cwd. Left as-is. The user/LLM took explicit
//                        responsibility, and we don't want to second-
//                        guess legitimate absolute writes.
//   - "relative/path"  → joined onto cwd.
//
// The "starts with /." heuristic catches the common LLM failure mode
// (model thinks "the workspace root" looks like a leading slash) without
// hijacking real absolute paths the user might want.
func resolveSideEffectPath(raw, cwd string) (string, bool) {
	path := ExpandHome(strings.TrimSpace(raw))
	if path == "" {
		return path, false
	}

	if !filepath.IsAbs(path) {
		if cwd != "" {
			return filepath.Join(cwd, path), false
		}
		return path, false
	}

	// Absolute path. The common LLM failure mode is "/.glitch/..." or
	// "/cmd/...": a leading slash the model added thinking it meant
	// "the root of the workspace". Detect by looking at the *real*
	// existence of the parent directory. If the path's parent doesn't
	// exist on disk AND we have a cwd, prefer the workspace-rooted
	// version. Otherwise leave the absolute path alone.
	if cwd != "" && !filepath.HasPrefix(path, cwd) {
		// Trim the leading slash and re-anchor under cwd.
		rel := strings.TrimPrefix(path, "/")
		candidate := filepath.Join(cwd, rel)
		// Only rewrite if the original parent doesn't exist (i.e. it
		// would have failed anyway with a "no such directory" or
		// "read-only filesystem" error). Real absolute writes to
		// existing directories pass through unchanged.
		if _, err := os.Stat(filepath.Dir(path)); err != nil {
			return candidate, true
		}
	}
	return path, false
}

// ExpandHome replaces a leading ~ with the user's home directory.
func ExpandHome(path string) string {
	if !strings.HasPrefix(path, "~") {
		return path
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return path
	}
	return filepath.Join(home, path[1:])
}

// gitOutput runs a git subcommand in dir and returns trimmed stdout, or "".
func gitOutput(dir string, args ...string) string {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}
