package observer

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/8op-org/gl1tch/internal/esearch"
)

// GenerateDigest creates a daily summary of all activity and indexes it
// into the summaries index. This can be run on a cron schedule.
func (q *QueryEngine) GenerateDigest(ctx context.Context) (string, error) {
	now := time.Now()
	dayAgo := now.Add(-24 * time.Hour)

	// Query all events from the last 24 hours.
	query := map[string]any{
		"size": 100,
		"sort": []map[string]any{{"timestamp": "desc"}},
		"query": map[string]any{
			"range": map[string]any{
				"timestamp": map[string]any{
					"gte": dayAgo.Format(time.RFC3339),
					"lte": now.Format(time.RFC3339),
				},
			},
		},
	}

	// Search across all indices.
	events, err := q.es.Search(ctx, []string{
		esearch.IndexEvents,
		esearch.IndexPipelines,
	}, query)
	if err != nil {
		return "", fmt.Errorf("query events: %w", err)
	}

	// Build a categorized summary of activity.
	var gitCommits, claudePrompts, copilotCmds, githubEvents, pipelineRuns []string

	for _, r := range events.Results {
		var doc map[string]any
		if err := json.Unmarshal(r.Source, &doc); err != nil {
			continue
		}

		eventType, _ := doc["type"].(string)
		message, _ := doc["message"].(string)
		repo, _ := doc["repo"].(string)
		source, _ := doc["source"].(string)

		switch {
		case strings.HasPrefix(eventType, "git."):
			author, _ := doc["author"].(string)
			gitCommits = append(gitCommits, fmt.Sprintf("[%s] %s — %s", repo, message, author))
		case strings.HasPrefix(eventType, "claude."):
			gitCommits = append(claudePrompts, fmt.Sprintf("[%s] %s", repo, truncateDigest(message, 120)))
		case strings.HasPrefix(eventType, "copilot."):
			copilotCmds = append(copilotCmds, truncateDigest(message, 120))
		case strings.HasPrefix(eventType, "github."):
			githubEvents = append(githubEvents, fmt.Sprintf("[%s] %s", repo, message))
		case r.Index == esearch.IndexPipelines:
			name, _ := doc["name"].(string)
			status, _ := doc["status"].(string)
			pipelineRuns = append(pipelineRuns, fmt.Sprintf("%s → %s", name, status))
		default:
			if source != "" {
				gitCommits = append(gitCommits, fmt.Sprintf("[%s/%s] %s", source, repo, truncateDigest(message, 100)))
			}
		}
	}

	// Build the context for the LLM.
	var sections []string
	if len(gitCommits) > 0 {
		sections = append(sections, fmt.Sprintf("Git commits (%d):\n%s", len(gitCommits), strings.Join(limitSlice(gitCommits, 30), "\n")))
	}
	if len(claudePrompts) > 0 {
		sections = append(sections, fmt.Sprintf("Claude Code prompts (%d):\n%s", len(claudePrompts), strings.Join(limitSlice(claudePrompts, 20), "\n")))
	}
	if len(copilotCmds) > 0 {
		sections = append(sections, fmt.Sprintf("Copilot CLI commands (%d):\n%s", len(copilotCmds), strings.Join(limitSlice(copilotCmds, 20), "\n")))
	}
	if len(githubEvents) > 0 {
		sections = append(sections, fmt.Sprintf("GitHub activity (%d):\n%s", len(githubEvents), strings.Join(limitSlice(githubEvents, 20), "\n")))
	}
	if len(pipelineRuns) > 0 {
		sections = append(sections, fmt.Sprintf("Pipeline runs (%d):\n%s", len(pipelineRuns), strings.Join(limitSlice(pipelineRuns, 20), "\n")))
	}

	if len(sections) == 0 {
		return "No activity recorded in the last 24 hours.", nil
	}

	activityData := strings.Join(sections, "\n\n")

	prompt := fmt.Sprintf(`You are gl1tch, a developer's AI observer assistant. Generate a daily briefing digest for %s.

Activity data from the last 24 hours:

%s

Write a concise daily briefing that:
1. Leads with the most important thing (biggest accomplishment or critical issue)
2. Summarizes work by project/repo
3. Notes which AI tools were used and how heavily (Claude Code vs Copilot)
4. Flags anything that looks like it needs follow-up (failed pipelines, stale PRs)
5. Ends with a one-line "focus suggestion" for today

Keep it under 300 words. Use markdown formatting. Be direct and specific.`, now.Format("Monday, January 2 2006"), activityData)

	// Generate the digest.
	summary, err := ollamaGenerate(ctx, q.model, prompt, false)
	if err != nil {
		return "", fmt.Errorf("generate digest: %w", err)
	}

	// Index the summary.
	var repos []string
	repoSet := make(map[string]bool)
	for _, r := range events.Results {
		var doc map[string]any
		if err := json.Unmarshal(r.Source, &doc); err != nil {
			continue
		}
		if repo, ok := doc["repo"].(string); ok && repo != "" && !repoSet[repo] {
			repoSet[repo] = true
			repos = append(repos, repo)
		}
	}

	summaryDoc := esearch.Summary{
		Scope:       "daily",
		Date:        now.Format("2006-01-02"),
		SummaryText: summary,
		Repos:       repos,
		GeneratedBy: "ollama/" + q.model,
		Timestamp:   now,
	}

	if err := q.es.Index(ctx, esearch.IndexSummaries, "", summaryDoc); err != nil {
		// Don't fail the digest if indexing the summary fails.
		_ = err
	}

	return summary, nil
}

func truncateDigest(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

func limitSlice(s []string, max int) []string {
	if len(s) <= max {
		return s
	}
	return s[:max]
}
