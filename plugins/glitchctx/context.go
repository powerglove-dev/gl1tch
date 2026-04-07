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

	output = writeBlockRe.ReplaceAllStringFunc(output, func(match string) string {
		sub := writeBlockRe.FindStringSubmatch(match)
		if len(sub) < 3 {
			return match
		}
		path := ExpandHome(strings.TrimSpace(sub[1]))
		content := sub[2]
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
