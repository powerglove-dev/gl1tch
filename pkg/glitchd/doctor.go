package glitchd

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/8op-org/gl1tch/internal/collector"
	"github.com/8op-org/gl1tch/internal/esearch"
	"github.com/8op-org/gl1tch/internal/store"
)

// DoctorCheck is a single health check result.
type DoctorCheck struct {
	Name   string `json:"name"`
	Status string `json:"status"` // "ok", "warn", "fail"
	Detail string `json:"detail"`
}

// Doctor runs all health checks and returns the results.
func Doctor(ctx context.Context) []DoctorCheck {
	var checks []DoctorCheck

	checks = append(checks, checkOllama())
	checks = append(checks, checkElasticsearch())
	checks = append(checks, checkBusd())
	checks = append(checks, checkGhCLI())
	checks = append(checks, checkConfig())
	checks = append(checks, checkStore())
	checks = append(checks, checkESIndices(ctx))
	checks = append(checks, checkBrainNotes(ctx))
	checks = append(checks, checkDirectories())

	return checks
}

// DoctorReport formats checks as a markdown report.
func DoctorReport(checks []DoctorCheck) string {
	var sb strings.Builder
	sb.WriteString("## gl1tch health check\n\n")

	okCount, warnCount, failCount := 0, 0, 0
	for _, c := range checks {
		var icon string
		switch c.Status {
		case "ok":
			icon = "\u2705"
			okCount++
		case "warn":
			icon = "\u26a0\ufe0f"
			warnCount++
		case "fail":
			icon = "\u274c"
			failCount++
		}
		sb.WriteString(fmt.Sprintf("%s **%s** — %s\n\n", icon, c.Name, c.Detail))
	}

	sb.WriteString(fmt.Sprintf("---\n**%d ok**, **%d warnings**, **%d failures**\n",
		okCount, warnCount, failCount))

	return sb.String()
}

func checkOllama() DoctorCheck {
	c := &http.Client{Timeout: 2 * time.Second}
	resp, err := c.Get("http://localhost:11434")
	if err != nil {
		return DoctorCheck{"Ollama", "fail", "not reachable at localhost:11434"}
	}
	resp.Body.Close()

	// Check what models are available
	out, err := exec.Command("ollama", "list").Output()
	if err != nil {
		return DoctorCheck{"Ollama", "ok", "running but couldn't list models"}
	}
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	modelCount := len(lines) - 1 // subtract header
	if modelCount <= 0 {
		return DoctorCheck{"Ollama", "warn", "running but no models pulled — run `ollama pull llama3.2`"}
	}
	return DoctorCheck{"Ollama", "ok", fmt.Sprintf("running with %d model(s)", modelCount)}
}

func checkElasticsearch() DoctorCheck {
	c := &http.Client{Timeout: 2 * time.Second}
	resp, err := c.Get("http://localhost:9200")
	if err != nil {
		return DoctorCheck{"Elasticsearch", "fail", "not reachable at localhost:9200 — run `docker compose up -d`"}
	}
	resp.Body.Close()
	return DoctorCheck{"Elasticsearch", "ok", "running"}
}

func checkBusd() DoctorCheck {
	path := busdSocketPath()
	if path == "" {
		return DoctorCheck{"busd", "warn", "socket path could not be determined"}
	}
	conn, err := net.DialTimeout("unix", path, 2*time.Second)
	if err != nil {
		return DoctorCheck{"busd", "warn", "event bus not reachable — some features may not work"}
	}
	conn.Close()
	return DoctorCheck{"busd", "ok", "event bus connected"}
}

func checkGhCLI() DoctorCheck {
	if _, err := exec.LookPath("gh"); err != nil {
		return DoctorCheck{"GitHub CLI", "warn", "gh not installed — GitHub collector disabled"}
	}
	out, err := exec.Command("gh", "auth", "status").CombinedOutput()
	if err != nil || strings.Contains(string(out), "not logged in") {
		return DoctorCheck{"GitHub CLI", "warn", "gh installed but not authenticated — run `gh auth login`"}
	}
	return DoctorCheck{"GitHub CLI", "ok", "installed and authenticated"}
}

func checkConfig() DoctorCheck {
	cfg, err := collector.LoadConfig()
	if err != nil {
		return DoctorCheck{"Config", "fail", "could not load observer.yaml: " + err.Error()}
	}

	var issues []string
	if len(cfg.Git.Repos) == 0 {
		issues = append(issues, "no git repos configured")
	}
	if len(cfg.Directories.Paths) == 0 {
		issues = append(issues, "no directories monitored")
	}
	if len(cfg.GitHub.Repos) == 0 {
		issues = append(issues, "no GitHub repos configured")
	}

	if len(issues) > 0 {
		return DoctorCheck{"Config", "warn", strings.Join(issues, "; ")}
	}
	return DoctorCheck{"Config", "ok", fmt.Sprintf("%d git repos, %d directories, %d GitHub repos",
		len(cfg.Git.Repos), len(cfg.Directories.Paths), len(cfg.GitHub.Repos))}
}

func checkStore() DoctorCheck {
	s, err := store.Open()
	if err != nil {
		return DoctorCheck{"Store", "fail", "SQLite store could not be opened: " + err.Error()}
	}
	defer s.Close()
	return DoctorCheck{"Store", "ok", "SQLite store accessible"}
}

func checkESIndices(ctx context.Context) DoctorCheck {
	cfg, _ := collector.LoadConfig()
	es, err := esearch.New(cfg.Elasticsearch.Address)
	if err != nil {
		return DoctorCheck{"ES Indices", "fail", "could not connect"}
	}
	if es.Ping(ctx) != nil {
		return DoctorCheck{"ES Indices", "fail", "ES not reachable"}
	}

	// Count docs in each index
	var parts []string
	for _, idx := range []string{"glitch-events", "glitch-pipelines", "glitch-summaries", "glitch-insights"} {
		query := map[string]any{"query": map[string]any{"match_all": map[string]any{}}}
		res, err := es.Search(ctx, []string{idx}, query)
		if err != nil {
			continue
		}
		parts = append(parts, fmt.Sprintf("%s: %d docs", idx, res.Total))
	}

	if len(parts) == 0 {
		return DoctorCheck{"ES Indices", "warn", "no data indexed yet — collectors may need time"}
	}
	return DoctorCheck{"ES Indices", "ok", strings.Join(parts, ", ")}
}

func checkBrainNotes(ctx context.Context) DoctorCheck {
	s, err := store.Open()
	if err != nil {
		return DoctorCheck{"Brain", "warn", "store not accessible"}
	}
	defer s.Close()

	notes, err := s.AllBrainNotes(ctx)
	if err != nil {
		return DoctorCheck{"Brain", "warn", "could not query brain notes"}
	}
	if len(notes) == 0 {
		return DoctorCheck{"Brain", "ok", "brain is empty — it will learn over time"}
	}
	return DoctorCheck{"Brain", "ok", fmt.Sprintf("%d brain notes stored", len(notes))}
}

func checkDirectories() DoctorCheck {
	cfg, _ := collector.LoadConfig()
	if len(cfg.Directories.Paths) == 0 {
		return DoctorCheck{"Directories", "warn", "no directories monitored — add some in the sidebar"}
	}

	var missing []string
	for _, d := range cfg.Directories.Paths {
		if _, err := os.Stat(d); err != nil {
			missing = append(missing, filepath.Base(d))
		}
	}
	if len(missing) > 0 {
		return DoctorCheck{"Directories", "warn", fmt.Sprintf("missing: %s", strings.Join(missing, ", "))}
	}
	return DoctorCheck{"Directories", "ok", fmt.Sprintf("%d directories monitored", len(cfg.Directories.Paths))}
}

func busdSocketPath() string {
	if xdg := os.Getenv("XDG_RUNTIME_DIR"); xdg != "" {
		return filepath.Join(xdg, "glitch", "bus.sock")
	}
	cache, err := os.UserCacheDir()
	if err != nil {
		return ""
	}
	return filepath.Join(cache, "glitch", "bus.sock")
}
