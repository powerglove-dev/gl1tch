package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/8op-org/gl1tch/internal/capability"
	"github.com/8op-org/gl1tch/internal/esearch"
)

var (
	assistantModel   string
	assistantBaseURL string
	assistantPick    bool
	assistantSkills  string
)

func init() {
	rootCmd.AddCommand(assistantCmd)
	assistantCmd.Flags().StringVarP(&assistantModel, "model", "m", "",
		"local LLM used for routing (default: qwen2.5:7b)")
	assistantCmd.Flags().StringVar(&assistantBaseURL, "ollama-url", "",
		"override Ollama base URL (default: http://localhost:11434)")
	assistantCmd.Flags().BoolVar(&assistantPick, "pick", false,
		"only show which capability would be picked; do not invoke it")
	assistantCmd.Flags().StringVar(&assistantSkills, "skills", "",
		"override skill directory (default: ~/.config/glitch/capabilities)")
}

var assistantCmd = &cobra.Command{
	Use:   "assistant [message]",
	Short: "Route a message to the best-matching capability",
	Long: `Ask the gl1tch assistant to pick a capability for your message and run it.

The assistant loads on-demand capabilities from ~/.config/glitch/capabilities
(markdown files with frontmatter), asks a local LLM to pick the best one, and
invokes it. The model only sees capability names and descriptions — it never
constructs shell commands or arguments. The runner executes each capability's
declared invocation directly.

Examples:
  glitch assistant "summarize the recent git log"
  glitch assistant --pick "what's my zsh history look like"
  glitch assistant --model qwen2.5:7b "translate this to French: hello"`,
	Args: cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		message := strings.Join(args, " ")
		if strings.TrimSpace(message) == "" {
			return fmt.Errorf("empty message")
		}

		reg, err := loadAssistantRegistry(assistantSkills)
		if err != nil {
			return err
		}
		if len(reg.Names()) == 0 {
			return fmt.Errorf("no on-demand capabilities available — drop skill markdown files into ~/.config/glitch/capabilities or register built-ins")
		}

		runner := capability.NewRunner(reg, nil)
		router := capability.NewRouter(reg, runner)
		if assistantModel != "" {
			router.Model = assistantModel
		}
		if assistantBaseURL != "" {
			router.BaseURL = assistantBaseURL
		}

		ctx := context.Background()

		if assistantPick {
			name, err := router.Pick(ctx, message)
			if err != nil {
				if errors.Is(err, capability.ErrNoMatch) {
					fmt.Fprintln(os.Stderr, "no capability matched")
					return nil
				}
				return err
			}
			fmt.Println(name)
			return nil
		}

		name, err := router.Route(ctx, message, os.Stdout)
		if err != nil {
			if errors.Is(err, capability.ErrNoMatch) {
				fmt.Fprintln(os.Stderr, "no capability matched — try `glitch ask` for a direct model answer")
				return nil
			}
			return fmt.Errorf("route: %w", err)
		}
		fmt.Fprintf(os.Stderr, "\n[capability: %s]\n", name)
		return nil
	},
}

// loadAssistantRegistry builds a capability registry from the user's skill
// directory. Unlike the background runner (which is built from workspace
// config in the pod manager), the assistant registry contains only
// on-demand capabilities the user has authored or installed — it is
// intentionally narrow so the routing model has a short, high-signal list
// to pick from.
func loadAssistantRegistry(dir string) (*capability.Registry, error) {
	if dir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, err
		}
		dir = filepath.Join(home, ".config", "glitch", "capabilities")
	}
	reg := capability.NewRegistry()
	caps, errs := capability.LoadSkillsFromDir(dir)
	for _, c := range caps {
		// Only on-demand skills belong in the router registry. A user
		// writing an interval-trigger skill by mistake would otherwise
		// start a background goroutine just because they ran
		// `glitch assistant` — surprising and wasteful.
		if c.Manifest().Trigger.Mode != capability.TriggerOnDemand {
			continue
		}
		if err := reg.Register(c); err != nil {
			errs = append(errs, err)
		}
	}
	if len(errs) > 0 && len(reg.Names()) == 0 {
		// Only surface errors when nothing loaded successfully —
		// otherwise a single broken file shouldn't stop the rest of
		// the registry from working.
		return reg, fmt.Errorf("load skills from %s: %v", dir, errs[0])
	}

	// Register Go-native on-demand capabilities alongside the skill
	// files. These are the built-ins that need live dependencies
	// (Elasticsearch, local state) and don't fit the "markdown +
	// subprocess" skill model.
	registerSecurityAlertsCap(reg)

	return reg, nil
}

// registerSecurityAlertsCap wires the built-in security_alerts
// capability into the assistant registry, bound to the user's
// configured Elasticsearch endpoint. Silently skipped when ES is not
// reachable or not configured — the assistant will simply not have a
// security tool to offer, which is better than refusing to start.
func registerSecurityAlertsCap(reg *capability.Registry) {
	cfg, err := capability.LoadConfig()
	if err != nil {
		return
	}
	addr := cfg.Elasticsearch.Address
	if addr == "" {
		addr = "http://localhost:9200"
	}
	es, err := esearch.New(addr)
	if err != nil {
		slog.Debug("security_alerts: ES client init failed", "err", err)
		return
	}
	cap := &capability.SecurityAlertsCapability{
		Searcher: func(ctx context.Context, query map[string]any) ([]map[string]any, error) {
			resp, err := es.Search(ctx, []string{capability.IndexSecurity}, query)
			if err != nil {
				return nil, err
			}
			out := make([]map[string]any, 0, len(resp.Results))
			for _, h := range resp.Results {
				var m map[string]any
				if err := json.Unmarshal(h.Source, &m); err != nil {
					continue
				}
				out = append(out, m)
			}
			return out, nil
		},
	}
	if err := reg.Register(cap); err != nil {
		slog.Debug("security_alerts: register failed", "err", err)
	}
}
