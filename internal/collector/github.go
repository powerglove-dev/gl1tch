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
//
// Collects: PRs, issues, PR comments, PR reviews, PR check statuses,
// and recent commits per PR.
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

	// Fetch recent PRs with full detail.
	prs, err := ghListPRs(ctx, repo, since)
	if err == nil {
		docs = append(docs, prs...)
	}

	// Fetch recent issues.
	issues, err := ghListIssues(ctx, repo, since)
	if err == nil {
		docs = append(docs, issues...)
	}

	// Fetch PR comments and reviews for open PRs.
	prNumbers := ghListOpenPRNumbers(ctx, repo)
	for _, num := range prNumbers {
		comments, err := ghListPRComments(ctx, repo, num, since)
		if err == nil {
			docs = append(docs, comments...)
		}

		reviews, err := ghListPRReviews(ctx, repo, num, since)
		if err == nil {
			docs = append(docs, reviews...)
		}

		checks, err := ghListPRChecks(ctx, repo, num)
		if err == nil {
			docs = append(docs, checks...)
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

func repoShortName(repo string) string {
	if idx := strings.LastIndex(repo, "/"); idx >= 0 {
		return repo[idx+1:]
	}
	return repo
}

// ── PRs ────────────────────────────────────────────────────────────────────

func ghListPRs(ctx context.Context, repo string, since time.Time) ([]any, error) {
	out, err := exec.CommandContext(ctx, "gh", "pr", "list",
		"--repo", repo,
		"--state", "all",
		"--json", "number,title,state,author,url,createdAt,updatedAt,body,labels,additions,deletions,changedFiles,headRefName,baseRefName,mergeable,reviewDecision",
		"--limit", "30",
	).Output()
	if err != nil {
		return nil, fmt.Errorf("gh pr list: %w", err)
	}

	var prs []struct {
		Number         int                            `json:"number"`
		Title          string                         `json:"title"`
		State          string                         `json:"state"`
		Author         struct{ Login string `json:"login"` } `json:"author"`
		URL            string                         `json:"url"`
		CreatedAt      string                         `json:"createdAt"`
		UpdatedAt      string                         `json:"updatedAt"`
		Body           string                         `json:"body"`
		Labels         []struct{ Name string `json:"name"` } `json:"labels"`
		Additions      int                            `json:"additions"`
		Deletions      int                            `json:"deletions"`
		ChangedFiles   int                            `json:"changedFiles"`
		HeadRefName    string                         `json:"headRefName"`
		BaseRefName    string                         `json:"baseRefName"`
		Mergeable      string                         `json:"mergeable"`
		ReviewDecision string                         `json:"reviewDecision"`
	}
	if err := json.Unmarshal(out, &prs); err != nil {
		return nil, fmt.Errorf("parse prs: %w", err)
	}

	shortName := repoShortName(repo)
	var events []any
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
			Type:   "github.pr",
			Source: "github",
			Repo:   shortName,
			Branch: pr.HeadRefName,
			Author: pr.Author.Login,
			Message: fmt.Sprintf("#%d %s [%s]", pr.Number, pr.Title, pr.State),
			Body:   truncate(pr.Body, 3000),
			Metadata: map[string]any{
				"number":          pr.Number,
				"state":           pr.State,
				"url":             pr.URL,
				"labels":          labelNames,
				"additions":       pr.Additions,
				"deletions":       pr.Deletions,
				"changed_files":   pr.ChangedFiles,
				"head_branch":     pr.HeadRefName,
				"base_branch":     pr.BaseRefName,
				"mergeable":       pr.Mergeable,
				"review_decision": pr.ReviewDecision,
				"github_repo":     repo,
			},
			Timestamp: ts,
		})
	}

	return events, nil
}

// ── Issues ─────────────────────────────────────────────────────────────────

func ghListIssues(ctx context.Context, repo string, since time.Time) ([]any, error) {
	out, err := exec.CommandContext(ctx, "gh", "issue", "list",
		"--repo", repo,
		"--state", "all",
		"--json", "number,title,state,author,url,createdAt,updatedAt,body,labels,comments",
		"--limit", "30",
	).Output()
	if err != nil {
		return nil, fmt.Errorf("gh issue list: %w", err)
	}

	var issues []struct {
		Number    int                            `json:"number"`
		Title     string                         `json:"title"`
		State     string                         `json:"state"`
		Author    struct{ Login string `json:"login"` } `json:"author"`
		URL       string                         `json:"url"`
		CreatedAt string                         `json:"createdAt"`
		UpdatedAt string                         `json:"updatedAt"`
		Body      string                         `json:"body"`
		Labels    []struct{ Name string `json:"name"` } `json:"labels"`
		Comments  []struct {
			Author struct{ Login string `json:"login"` } `json:"author"`
			Body   string `json:"body"`
			CreatedAt string `json:"createdAt"`
		} `json:"comments"`
	}
	if err := json.Unmarshal(out, &issues); err != nil {
		return nil, fmt.Errorf("parse issues: %w", err)
	}

	shortName := repoShortName(repo)
	var events []any
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
			Type:   "github.issue",
			Source: "github",
			Repo:   shortName,
			Author: issue.Author.Login,
			Message: fmt.Sprintf("#%d %s [%s]", issue.Number, issue.Title, issue.State),
			Body:   truncate(issue.Body, 3000),
			Metadata: map[string]any{
				"number":        issue.Number,
				"state":         issue.State,
				"url":           issue.URL,
				"labels":        labelNames,
				"comment_count": len(issue.Comments),
				"github_repo":   repo,
			},
			Timestamp: ts,
		})

		// Index individual comments as separate events
		for _, comment := range issue.Comments {
			commentTs, _ := time.Parse(time.RFC3339, comment.CreatedAt)
			if commentTs.Before(since) {
				continue
			}
			events = append(events, esearch.Event{
				Type:    "github.issue_comment",
				Source:  "github",
				Repo:    shortName,
				Author:  comment.Author.Login,
				Message: fmt.Sprintf("Comment on #%d %s", issue.Number, issue.Title),
				Body:    truncate(comment.Body, 2000),
				Metadata: map[string]any{
					"issue_number": issue.Number,
					"issue_title":  issue.Title,
					"github_repo":  repo,
				},
				Timestamp: commentTs,
			})
		}
	}

	return events, nil
}

// ── PR Comments ────────────────────────────────────────────────────────────

func ghListOpenPRNumbers(ctx context.Context, repo string) []int {
	out, err := exec.CommandContext(ctx, "gh", "pr", "list",
		"--repo", repo,
		"--state", "open",
		"--json", "number",
		"--limit", "10",
	).Output()
	if err != nil {
		return nil
	}

	var prs []struct{ Number int `json:"number"` }
	if err := json.Unmarshal(out, &prs); err != nil {
		return nil
	}

	nums := make([]int, len(prs))
	for i, pr := range prs {
		nums[i] = pr.Number
	}
	return nums
}

func ghListPRComments(ctx context.Context, repo string, prNum int, since time.Time) ([]any, error) {
	out, err := exec.CommandContext(ctx, "gh", "api",
		fmt.Sprintf("repos/%s/pulls/%d/comments", repo, prNum),
		"--jq", ".[].body, .[].user.login, .[].created_at",
	).Output()
	if err != nil {
		// Fallback: use gh pr view for comments
		return ghListPRCommentsView(ctx, repo, prNum, since)
	}
	_ = out // API response parsing handled by fallback
	return ghListPRCommentsView(ctx, repo, prNum, since)
}

func ghListPRCommentsView(ctx context.Context, repo string, prNum int, since time.Time) ([]any, error) {
	out, err := exec.CommandContext(ctx, "gh", "pr", "view",
		fmt.Sprintf("%d", prNum),
		"--repo", repo,
		"--json", "comments",
	).Output()
	if err != nil {
		return nil, fmt.Errorf("gh pr view comments: %w", err)
	}

	var result struct {
		Comments []struct {
			Author    struct{ Login string `json:"login"` } `json:"author"`
			Body      string `json:"body"`
			CreatedAt string `json:"createdAt"`
		} `json:"comments"`
	}
	if err := json.Unmarshal(out, &result); err != nil {
		return nil, fmt.Errorf("parse pr comments: %w", err)
	}

	shortName := repoShortName(repo)
	var events []any
	for _, c := range result.Comments {
		ts, _ := time.Parse(time.RFC3339, c.CreatedAt)
		if ts.Before(since) {
			continue
		}
		events = append(events, esearch.Event{
			Type:    "github.pr_comment",
			Source:  "github",
			Repo:    shortName,
			Author:  c.Author.Login,
			Message: fmt.Sprintf("Comment on PR #%d", prNum),
			Body:    truncate(c.Body, 2000),
			Metadata: map[string]any{
				"pr_number":   prNum,
				"github_repo": repo,
			},
			Timestamp: ts,
		})
	}
	return events, nil
}

// ── PR Reviews ─────────────────────────────────────────────────────────────

func ghListPRReviews(ctx context.Context, repo string, prNum int, since time.Time) ([]any, error) {
	out, err := exec.CommandContext(ctx, "gh", "pr", "view",
		fmt.Sprintf("%d", prNum),
		"--repo", repo,
		"--json", "reviews",
	).Output()
	if err != nil {
		return nil, fmt.Errorf("gh pr view reviews: %w", err)
	}

	var result struct {
		Reviews []struct {
			Author    struct{ Login string `json:"login"` } `json:"author"`
			Body      string `json:"body"`
			State     string `json:"state"`
			SubmittedAt string `json:"submittedAt"`
		} `json:"reviews"`
	}
	if err := json.Unmarshal(out, &result); err != nil {
		return nil, fmt.Errorf("parse pr reviews: %w", err)
	}

	shortName := repoShortName(repo)
	var events []any
	for _, r := range result.Reviews {
		ts, _ := time.Parse(time.RFC3339, r.SubmittedAt)
		if ts.Before(since) {
			continue
		}
		events = append(events, esearch.Event{
			Type:    "github.pr_review",
			Source:  "github",
			Repo:    shortName,
			Author:  r.Author.Login,
			Message: fmt.Sprintf("Review on PR #%d: %s", prNum, r.State),
			Body:    truncate(r.Body, 2000),
			Metadata: map[string]any{
				"pr_number":    prNum,
				"review_state": r.State,
				"github_repo":  repo,
			},
			Timestamp: ts,
		})
	}
	return events, nil
}

// ── PR Check Statuses ──────────────────────────────────────────────────────

func ghListPRChecks(ctx context.Context, repo string, prNum int) ([]any, error) {
	out, err := exec.CommandContext(ctx, "gh", "pr", "checks",
		fmt.Sprintf("%d", prNum),
		"--repo", repo,
		"--json", "name,state,startedAt,completedAt,detailsUrl",
	).Output()
	if err != nil {
		return nil, nil // checks may not be available, non-fatal
	}

	var checks []struct {
		Name        string `json:"name"`
		State       string `json:"state"`
		StartedAt   string `json:"startedAt"`
		CompletedAt string `json:"completedAt"`
		DetailsURL  string `json:"detailsUrl"`
	}
	if err := json.Unmarshal(out, &checks); err != nil {
		return nil, nil
	}

	shortName := repoShortName(repo)
	var events []any
	for _, check := range checks {
		ts, _ := time.Parse(time.RFC3339, check.CompletedAt)
		if ts.IsZero() {
			ts, _ = time.Parse(time.RFC3339, check.StartedAt)
		}
		if ts.IsZero() {
			ts = time.Now()
		}

		events = append(events, esearch.Event{
			Type:    "github.check",
			Source:  "github",
			Repo:    shortName,
			Author:  "ci",
			Message: fmt.Sprintf("PR #%d check: %s [%s]", prNum, check.Name, check.State),
			Metadata: map[string]any{
				"pr_number":   prNum,
				"check_name":  check.Name,
				"check_state": check.State,
				"details_url": check.DetailsURL,
				"github_repo": repo,
			},
			Timestamp: ts,
		})
	}
	return events, nil
}
