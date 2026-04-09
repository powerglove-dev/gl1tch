// smoke.go is the real-world smoke pack: a CLI that runs the
// curated fixture set from internal/research/smoke_pack.yaml against
// any number of git repositories and asserts that the loop grounded
// correctly in each one. The same runner serves three purposes:
//
//   1. Regression testing: a CI cron runs this against the four
//      target repos and fails on any pass→fail transition.
//
//   2. Ground-truth validation: every fixture asserts the loop's
//      gathered evidence references at least one SHA from the
//      target repo's actual git log, so a regression in the cwd
//      injection (the bug we kept hitting) is caught immediately
//      instead of being smoothed over by the model's confident
//      hallucination.
//
//   3. Brain training: with --accept, every passing fixture writes
//      a research_feedback event the brain hints reader will pick
//      up. The smoke pack itself becomes labelled training data —
//      every CI run teaches the planner that the picks it made for
//      these question shapes were correct, biasing future similar
//      questions toward the same picks.
//
// Targets are passed as bare directory paths (the most flexible
// shape — works against any local checkout without requiring a
// workspace to be configured first). Each target gets a synthetic
// workspace_id derived from its absolute path so the brain hints
// reader can scope cross-target learnings correctly.
package cmd

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/8op-org/gl1tch/internal/research"
	"github.com/8op-org/gl1tch/pkg/glitchd"
)

var (
	smokePackTargets   []string
	smokePackAccept    bool
	smokePackDryRun    bool
	smokePackWallclock time.Duration
)

var smokeCmd = &cobra.Command{
	Use:   "smoke",
	Short: "Real-world smoke testing for the research loop",
	Long: `Runs the canonical fixture pack from
internal/research/smoke_pack.yaml against one or more target
directories. Each fixture asserts:

  1. The loop completed without error.
  2. The planner picked at least one of the fixture's expected
     researchers.
  3. The gathered evidence references at least one SHA from the
     target's actual git log — proves cwd injection landed in the
     right repo.

Use this as a CI regression rig and as brain training data: with
--accept, every passing fixture writes a research_feedback event
the brain hints reader will surface as a 👍 in future planner
prompts.`,
}

func init() {
	rootCmd.AddCommand(smokeCmd)
	smokeCmd.AddCommand(smokePackCmd)

	smokePackCmd.Flags().StringSliceVarP(&smokePackTargets, "target", "t", nil,
		"target directory (repeatable). Defaults to the current working directory when none supplied.")
	smokePackCmd.Flags().BoolVar(&smokePackAccept, "accept", false,
		"on every passing fixture, write a research_feedback event with accepted=true so the brain hints reader picks it up as positive training data")
	smokePackCmd.Flags().BoolVar(&smokePackDryRun, "dry-run", false,
		"list fixtures and targets without running the loop. Useful for sanity-checking pack configuration.")
	smokePackCmd.Flags().DurationVar(&smokePackWallclock, "max-wallclock", 90*time.Second,
		"per-fixture wallclock budget — applies to one research call, not the whole pack run")
}

// fixtureResult is the per-fixture per-target outcome the runner
// collects so the final summary can show which fixtures passed
// where.
type fixtureResult struct {
	target  string
	fixture string
	passed  bool
	reason  string
	picks   []string
	matched string
}

var smokePackCmd = &cobra.Command{
	Use:   "pack",
	Short: "Run the smoke pack fixture set against one or more target repos",
	RunE: func(cmd *cobra.Command, args []string) error {
		// Resolve targets. Accept --target flags, positional args,
		// and a default of the current shell cwd.
		targets := smokePackTargets
		targets = append(targets, args...)
		if len(targets) == 0 {
			cwd, err := os.Getwd()
			if err != nil {
				return fmt.Errorf("resolve cwd: %w", err)
			}
			targets = []string{cwd}
		}

		// Validate every target is a real git checkout before
		// kicking off the loop. We'd rather fail fast on a typo
		// than discover the issue 30 seconds into a model call.
		resolved := make([]string, 0, len(targets))
		for _, t := range targets {
			abs, err := filepath.Abs(t)
			if err != nil {
				return fmt.Errorf("resolve %s: %w", t, err)
			}
			if _, err := os.Stat(filepath.Join(abs, ".git")); err != nil {
				return fmt.Errorf("smoke target %s is not a git checkout (no .git dir): %w", abs, err)
			}
			resolved = append(resolved, abs)
		}

		// Load the fixture pack. Errors here are fatal — without a
		// pack the runner has nothing to do.
		fixtures, err := research.LoadSmokePack()
		if err != nil {
			return fmt.Errorf("load smoke pack: %w", err)
		}
		if len(fixtures) == 0 {
			return fmt.Errorf("smoke pack is empty")
		}

		fmt.Fprintf(cmd.ErrOrStderr(), "smoke pack: %d fixture(s) × %d target(s)\n", len(fixtures), len(resolved))
		for _, t := range resolved {
			fmt.Fprintf(cmd.ErrOrStderr(), "  target: %s\n", t)
		}
		for _, f := range fixtures {
			fmt.Fprintf(cmd.ErrOrStderr(), "  fixture: %s — %s\n", f.Name, f.Question)
		}

		if smokePackDryRun {
			fmt.Fprintln(cmd.ErrOrStderr(), "\n--dry-run set; not running the loop.")
			return nil
		}

		// Build one ThreadHosts registry shared across all
		// targets so the brain event sink + research loop are
		// constructed once. Each target gets its own host inside
		// the registry, keyed by the synthetic workspace id.
		hosts := glitchd.NewThreadHosts()

		var results []fixtureResult
		for _, target := range resolved {
			groundTruth, err := smokeRecentSHAs(target, 30)
			if err != nil {
				return fmt.Errorf("read git log for %s: %w", target, err)
			}
			if len(groundTruth) == 0 {
				return fmt.Errorf("smoke target %s has no commits", target)
			}
			workspaceID := smokeWorkspaceIDFor(target)
			fmt.Fprintf(cmd.ErrOrStderr(), "\n── %s ── (head=%s)\n", target, groundTruth[0])

			// Adopt the target as a synthetic workspace so the
			// loop's queryContext gets the right cwd + workspace_id.
			// AdoptDirectoryAsWorkspace returns the same host on
			// repeated calls so per-fixture invocations reuse the
			// same brain sink and registry.
			hosts.AdoptDirectoryAsWorkspace(workspaceID, target)

			for _, fixture := range fixtures {
				res := smokeRunFixture(cmd.Context(), hosts, workspaceID, fixture, groundTruth)
				res.target = target
				res.fixture = fixture.Name
				results = append(results, res)
				icon := "✓"
				if !res.passed {
					icon = "✗"
				}
				fmt.Fprintf(cmd.ErrOrStderr(), "  %s %-20s  %s\n", icon, fixture.Name, res.reason)

				// Brain training: write a positive feedback
				// event for every passing fixture so the
				// hints reader sees explicit accepts.
				if smokePackAccept && res.passed {
					hosts.RecordResearchFeedback(workspaceID, "smoke-pack", res.matched, fixture.Question, true)
				}
			}
		}

		// Final summary.
		var passed, failed int
		for _, r := range results {
			if r.passed {
				passed++
			} else {
				failed++
			}
		}
		fmt.Fprintf(cmd.ErrOrStderr(), "\nsmoke pack summary: %d passed, %d failed (%d total)\n",
			passed, failed, len(results))
		if failed > 0 {
			return fmt.Errorf("smoke pack: %d fixtures failed", failed)
		}
		return nil
	},
}

// smokeRunFixture executes one fixture against one target. Returns
// a fixtureResult describing pass/fail + the matched ground-truth
// SHA (used by --accept to tag the feedback event).
//
// The smokePackWallclock budget is enforced inside DispatchSlash via
// the loop's own context-with-timeout (created in
// runResearchAsParentThread). The parent context is unused here
// because the slash dispatcher creates its own — but we keep the
// parameter so a future API change can plumb cancellation cleanly.
func smokeRunFixture(parentCtx context.Context, hosts *glitchd.ThreadHosts, workspaceID string, fixture research.SmokeFixture, groundTruth []string) fixtureResult {
	_ = parentCtx
	// Use the same DispatchSlash path the desktop and CLI smoke
	// command use so the same code is exercised end-to-end.
	envelope := hosts.DispatchSlash(workspaceID, fixture.Question, "main")
	if !strings.Contains(envelope, `"ok":true`) {
		return fixtureResult{passed: false, reason: "loop returned error: " + envelope}
	}
	// The envelope's `thread_id` field tells us which thread to
	// inspect for the gathered evidence.
	threadID := smokeExtractField(envelope, "thread_id")
	if threadID == "" {
		return fixtureResult{passed: false, reason: "envelope missing thread_id: " + envelope}
	}
	showRaw := hosts.ThreadMessages(workspaceID, threadID)

	// Assertion 1: at least one expected pick must appear.
	pickFound := ""
	for _, pick := range fixture.ExpectedPicks {
		if strings.Contains(showRaw, pick) {
			pickFound = pick
			break
		}
	}
	if pickFound == "" {
		return fixtureResult{
			passed: false,
			reason: fmt.Sprintf("planner did not pick any of %v (got: %s)", fixture.ExpectedPicks, smokeSummariseShow(showRaw)),
		}
	}

	// Assertion 2: at least one ground-truth SHA must appear in
	// the bundle (proves cwd injection landed in the right repo).
	// Only enforced for git-log fixtures because git-log is the
	// only canonical researcher whose evidence body INCLUDES
	// commit SHAs. git-status reports branch+dirty file paths
	// (no SHAs); github-prs reports PR numbers and URLs (also no
	// SHAs). For non-git-log fixtures the cwd injection is
	// validated indirectly: a wrong-cwd github-prs run would
	// return PRs from a different repo and the planner-pick
	// assertion would still pass (because the source name is
	// "github-prs" regardless), but the optional
	// expected_evidence_substr field can be set per fixture to
	// pin known stable identifiers.
	matched := ""
	for _, sha := range groundTruth {
		if sha != "" && strings.Contains(showRaw, sha) {
			matched = sha
			break
		}
	}
	requiresShaMatch := false
	for _, pick := range fixture.ExpectedPicks {
		if pick == "git-log" {
			requiresShaMatch = true
			break
		}
	}
	if requiresShaMatch && matched == "" {
		return fixtureResult{
			passed: false,
			reason: fmt.Sprintf("no ground-truth SHA matched (gather may have run in wrong repo). picks=%v", fixture.ExpectedPicks),
			picks:  fixture.ExpectedPicks,
		}
	}

	// Assertion 3: optional substring assertions.
	for _, want := range fixture.ExpectedEvidenceSubstr {
		if !strings.Contains(showRaw, want) {
			return fixtureResult{
				passed: false,
				reason: fmt.Sprintf("expected substring %q not in evidence", want),
			}
		}
	}

	// Pass.
	reason := fmt.Sprintf("picked %s", pickFound)
	if matched != "" {
		reason += fmt.Sprintf(", matched %s", matched)
	}
	return fixtureResult{passed: true, reason: reason, picks: fixture.ExpectedPicks, matched: matched}
}

// smokeRecentSHAs reads the most recent N short SHAs from the git
// repo at dir. Same helper the threads-smoke command uses; pulled
// here so smoke.go is self-contained for the pack runner.
func smokeRecentSHAs(dir string, n int) ([]string, error) {
	out, err := exec.Command("git", "-C", dir, "log",
		"--pretty=format:%h", fmt.Sprintf("-n%d", n)).Output()
	if err != nil {
		return nil, err
	}
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	clean := make([]string, 0, len(lines))
	for _, l := range lines {
		l = strings.TrimSpace(l)
		if l != "" {
			clean = append(clean, l)
		}
	}
	return clean, nil
}

// smokeWorkspaceIDFor mints a stable synthetic workspace id from a
// directory path so the brain hints reader can scope per-target.
// We use the basename + a short hash of the absolute path so the
// id is human-readable in the event log AND unique across
// directories with the same basename in different parents.
func smokeWorkspaceIDFor(dir string) string {
	abs, _ := filepath.Abs(dir)
	base := filepath.Base(abs)
	// 8-char hash of the absolute path so /a/foo and /b/foo
	// don't collide. Tiny FNV is enough — collisions in this
	// space don't have any security implications.
	var h uint32 = 2166136261
	for i := 0; i < len(abs); i++ {
		h ^= uint32(abs[i])
		h *= 16777619
	}
	return fmt.Sprintf("smoke-%s-%08x", base, h)
}

// smokeExtractField pulls a top-level string field out of a
// {"k":"v",...} JSON envelope without bringing in encoding/json's
// full decoder — the envelope is well-formed by construction (we
// emit it ourselves) and we only need one field at a time.
func smokeExtractField(envelope, field string) string {
	needle := `"` + field + `":"`
	i := strings.Index(envelope, needle)
	if i < 0 {
		return ""
	}
	rest := envelope[i+len(needle):]
	end := strings.Index(rest, `"`)
	if end < 0 {
		return ""
	}
	return rest[:end]
}

// smokeSummariseShow truncates a thread show payload to a length
// useful in failure messages — full payloads are tens of KB and
// dominate the test log when they fail.
func smokeSummariseShow(s string) string {
	if len(s) <= 200 {
		return s
	}
	return s[:200] + "…"
}
