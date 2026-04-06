package collector

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os/exec"
	"strings"
	"time"

	"github.com/8op-org/gl1tch/internal/esearch"
)

// GitHubCollector indexes PR and issue activity from configured repos via the
// gh CLI. Requires gh to be installed and authenticated.
type GitHubCollector struct {
	// Repos in "owner/repo" format.
	Repos    []string
	Interval time.Duration
}

func (g *GitHubCollector) Name() string { return "github" }

func (g *GitHubCollector) Start(ctx context.Context, es *esearch.Client) error {
	if g.Interval == 0 {
		g.Interval = 300 * time.Second // every 5 min
	}

	// Verify gh is available.
	if _, err := exec.LookPath("gh"); err != nil {
		return fmt.Errorf("github collector: gh CLI not found")
	}

	// Track last poll time per repo.
	lastPoll := make(map[string]time.Time)

	ticker := time.NewTicker(g.Interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			for _, repo := range g.Repos {
				since := lastPoll[repo]
				if since.IsZero() {
					since = time.Now().Add(-24 * time.Hour) // backfill 1 day on first run
				}
				if err := g.pollRepo(ctx, es, repo, since); err != nil {
					slog.Warn("github collector: poll error", "repo", repo, "err", err)
					continue
				}
				lastPoll[repo] = time.Now()
			}
		}
	}
}

func (g *GitHubCollector) pollRepo(ctx context.Context, es *esearch.Client, repo string, since time.Time) error {
	var docs []any

	// Fetch recent PRs.
	prs, err := ghListPRs(ctx, repo, since)
	if err == nil {
		for _, pr := range prs {
			docs = append(docs, pr)
		}
	}

	// Fetch recent issues.
	issues, err := ghListIssues(ctx, repo, since)
	if err == nil {
		for _, issue := range issues {
			docs = append(docs, issue)
		}
	}

	if len(docs) > 0 {
		slog.Info("github collector: new activity", "repo", repo, "events", len(docs))
		if err := es.BulkIndex(ctx, esearch.IndexEvents, docs); err != nil {
			return fmt.Errorf("bulk index: %w", err)
		}
	}

	return nil
}

type ghPR struct {
	Number    int    `json:"number"`
	Title     string `json:"title"`
	State     string `json:"state"`
	Author    string `json:"author"`
	URL       string `json:"url"`
	CreatedAt string `json:"createdAt"`
	UpdatedAt string `json:"updatedAt"`
	Body      string `json:"body"`
	Labels    []struct {
		Name string `json:"name"`
	} `json:"labels"`
}

type ghIssue struct {
	Number    int    `json:"number"`
	Title     string `json:"title"`
	State     string `json:"state"`
	Author    string `json:"author"`
	URL       string `json:"url"`
	CreatedAt string `json:"createdAt"`
	UpdatedAt string `json:"updatedAt"`
	Body      string `json:"body"`
	Labels    []struct {
		Name string `json:"name"`
	} `json:"labels"`
}

func ghListPRs(ctx context.Context, repo string, since time.Time) ([]esearch.Event, error) {
	// gh pr list --repo owner/repo --json number,title,state,author,url,createdAt,updatedAt,body,labels --limit 20
	out, err := exec.CommandContext(ctx, "gh", "pr", "list",
		"--repo", repo,
		"--state", "all",
		"--json", "number,title,state,author,url,createdAt,updatedAt,body,labels",
		"--limit", "20",
	).Output()
	if err != nil {
		return nil, fmt.Errorf("gh pr list: %w", err)
	}

	var prs []struct {
		Number    int    `json:"number"`
		Title     string `json:"title"`
		State     string `json:"state"`
		Author    struct{ Login string `json:"login"` } `json:"author"`
		URL       string `json:"url"`
		CreatedAt string `json:"createdAt"`
		UpdatedAt string `json:"updatedAt"`
		Body      string `json:"body"`
		Labels    []struct{ Name string `json:"name"` } `json:"labels"`
	}
	if err := json.Unmarshal(out, &prs); err != nil {
		return nil, fmt.Errorf("parse prs: %w", err)
	}

	repoName := repo
	if idx := strings.LastIndex(repo, "/"); idx >= 0 {
		repoName = repo[idx+1:]
	}

	var events []esearch.Event
	for _, pr := range prs {
		ts, _ := time.Parse(time.RFC3339, pr.UpdatedAt)
		if ts.Before(since) {
			continue
		}

		var labelNames []string
		for _, l := range pr.Labels {
			labelNames = append(labelNames, l.Name)
		}

		events = append(events, esearch.Event{
			Type:    "github.pr",
			Source:  "github",
			Repo:    repoName,
			Author:  pr.Author.Login,
			Message: fmt.Sprintf("#%d %s [%s]", pr.Number, pr.Title, pr.State),
			Body:    truncate(pr.Body, 2000),
			Metadata: map[string]any{
				"number": pr.Number,
				"state":  pr.State,
				"url":    pr.URL,
				"labels": labelNames,
			},
			Timestamp: ts,
		})
	}

	return events, nil
}

func ghListIssues(ctx context.Context, repo string, since time.Time) ([]esearch.Event, error) {
	out, err := exec.CommandContext(ctx, "gh", "issue", "list",
		"--repo", repo,
		"--state", "all",
		"--json", "number,title,state,author,url,createdAt,updatedAt,body,labels",
		"--limit", "20",
	).Output()
	if err != nil {
		return nil, fmt.Errorf("gh issue list: %w", err)
	}

	var issues []struct {
		Number    int    `json:"number"`
		Title     string `json:"title"`
		State     string `json:"state"`
		Author    struct{ Login string `json:"login"` } `json:"author"`
		URL       string `json:"url"`
		CreatedAt string `json:"createdAt"`
		UpdatedAt string `json:"updatedAt"`
		Body      string `json:"body"`
		Labels    []struct{ Name string `json:"name"` } `json:"labels"`
	}
	if err := json.Unmarshal(out, &issues); err != nil {
		return nil, fmt.Errorf("parse issues: %w", err)
	}

	repoName := repo
	if idx := strings.LastIndex(repo, "/"); idx >= 0 {
		repoName = repo[idx+1:]
	}

	var events []esearch.Event
	for _, issue := range issues {
		ts, _ := time.Parse(time.RFC3339, issue.UpdatedAt)
		if ts.Before(since) {
			continue
		}

		var labelNames []string
		for _, l := range issue.Labels {
			labelNames = append(labelNames, l.Name)
		}

		events = append(events, esearch.Event{
			Type:    "github.issue",
			Source:  "github",
			Repo:    repoName,
			Author:  issue.Author.Login,
			Message: fmt.Sprintf("#%d %s [%s]", issue.Number, issue.Title, issue.State),
			Body:    truncate(issue.Body, 2000),
			Metadata: map[string]any{
				"number": issue.Number,
				"state":  issue.State,
				"url":    issue.URL,
				"labels": labelNames,
			},
			Timestamp: ts,
		})
	}

	return events, nil
}
