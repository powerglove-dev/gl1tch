package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/8op-org/gl1tch/internal/capability"
	"github.com/8op-org/gl1tch/internal/esearch"
)

var (
	securityInjectCount     int
	securityInjectIncludeBreach bool
	securityInjectAddress   string
)

var (
	securityAlertsWindow string
)

func init() {
	rootCmd.AddCommand(securityCmd)
	securityCmd.AddCommand(securityInjectCmd)
	securityCmd.AddCommand(securityAlertsCmd)
	securityInjectCmd.Flags().IntVar(&securityInjectCount, "count", 3,
		"number of auth-failure events to inject")
	securityInjectCmd.Flags().BoolVar(&securityInjectIncludeBreach, "breach", true,
		"include a critical 'successful root login from unknown IP' breach event")
	securityInjectCmd.Flags().StringVar(&securityInjectAddress, "es", "",
		"Elasticsearch address (default: from ~/.config/glitch/observer.yaml)")
	securityAlertsCmd.Flags().StringVar(&securityAlertsWindow, "window", "24h",
		"time window to look back for alerts")
	securityAlertsCmd.Flags().StringVar(&securityInjectAddress, "es", "",
		"Elasticsearch address (default: from ~/.config/glitch/observer.yaml)")
}

var securityCmd = &cobra.Command{
	Use:   "security",
	Short: "Security event tooling (fake-event injection, queries)",
	Long: `Commands for exercising gl1tch's security_alerts capability.

Primarily used as a smoke-test surface: inject a handful of fake SSH
auth-failure docs (and optionally a "successful root login from
unknown IP" critical breach) into the glitch-security Elasticsearch
index, then run ` + "`glitch chat`" + ` and ask "any security alerts?"
to see what the assistant says.

The injected docs are plainly fake — they use TEST-NET-3 addresses
(203.0.113.0/24, RFC 5737) so they cannot collide with real traffic.`,
}

var securityInjectCmd = &cobra.Command{
	Use:   "inject",
	Short: "Inject fake SSH auth-failure events into glitch-security",
	Long: `Writes a handful of fake SSH auth-failure documents into the
glitch-security index so the security_alerts capability has something
to report. Use --count to control how many noisy auth failures, and
--breach=false to suppress the critical "root login succeeded" event.

Examples:
  glitch security inject                      # 3 failures + 1 critical breach
  glitch security inject --count 10           # 10 failures + 1 critical breach
  glitch security inject --breach=false       # failures only, no critical event`,
	RunE: func(cmd *cobra.Command, args []string) error {
		addr := securityInjectAddress
		if addr == "" {
			if cfg, err := capability.LoadConfig(); err == nil {
				addr = cfg.Elasticsearch.Address
			}
		}
		if addr == "" {
			addr = "http://localhost:9200"
		}

		es, err := esearch.New(addr)
		if err != nil {
			return fmt.Errorf("security inject: connect to %s: %w", addr, err)
		}

		ctx := context.Background()
		if err := ensureSecurityIndex(ctx, es); err != nil {
			return err
		}

		docs := buildFakeSecurityDocs(securityInjectCount, securityInjectIncludeBreach)
		if err := es.BulkIndex(ctx, capability.IndexSecurity, docs); err != nil {
			return fmt.Errorf("security inject: bulk: %w", err)
		}

		fmt.Printf("injected %d security event(s) into %s at %s\n",
			len(docs), capability.IndexSecurity, addr)
		fmt.Println("try: glitch chat  →  'any security alerts?'")
		return nil
	},
}

// securityAlertsCmd runs the security_alerts capability directly
// (without going through the assistant router / chat loop) and
// prints its formatted output. Useful for smoke-testing the full
// ES → capability → summary path without needing a local LLM.
var securityAlertsCmd = &cobra.Command{
	Use:   "alerts",
	Short: "Run the security_alerts capability directly and print its output",
	RunE: func(cmd *cobra.Command, args []string) error {
		addr := securityInjectAddress
		if addr == "" {
			if cfg, err := capability.LoadConfig(); err == nil {
				addr = cfg.Elasticsearch.Address
			}
		}
		if addr == "" {
			addr = "http://localhost:9200"
		}
		es, err := esearch.New(addr)
		if err != nil {
			return fmt.Errorf("security alerts: connect: %w", err)
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
					if err := json.Unmarshal(h.Source, &m); err == nil {
						out = append(out, m)
					}
				}
				return out, nil
			},
		}
		ch, err := cap.Invoke(context.Background(), capability.Input{Stdin: securityAlertsWindow})
		if err != nil {
			return err
		}
		for ev := range ch {
			if ev.Kind == capability.EventStream {
				fmt.Fprint(os.Stdout, ev.Text)
			}
		}
		fmt.Println()
		return nil
	},
}

// ensureSecurityIndex creates the glitch-security index with a small
// hand-rolled mapping if it does not already exist. We define the
// mapping inline here (rather than in internal/esearch) because this
// index is owned by the capability layer, not by the legacy collector
// schema set.
func ensureSecurityIndex(ctx context.Context, es *esearch.Client) error {
	return es.EnsureCustomIndex(ctx, capability.IndexSecurity, securityIndexMapping)
}

const securityIndexMapping = `{
  "settings": {
    "number_of_shards": 1,
    "number_of_replicas": 0
  },
  "mappings": {
    "properties": {
      "timestamp":  { "type": "date" },
      "severity":   { "type": "keyword" },
      "event_type": { "type": "keyword" },
      "user":       { "type": "keyword" },
      "source_ip":  { "type": "ip" },
      "host":       { "type": "keyword" },
      "message":    { "type": "text" }
    }
  }
}`

// buildFakeSecurityDocs synthesises a plausible burst of SSH auth
// failures plus (optionally) one critical breach event. All source
// IPs come from the TEST-NET-3 block (RFC 5737) so they can never
// collide with real traffic on any network.
func buildFakeSecurityDocs(failureCount int, includeBreach bool) []any {
	rng := rand.New(rand.NewSource(time.Now().UnixNano()))
	users := []string{"root", "admin", "ubuntu", "pi", "ec2-user", "oracle"}
	ips := []string{
		"203.0.113.42", "203.0.113.57", "203.0.113.88",
		"203.0.113.101", "203.0.113.143",
	}

	var docs []any
	now := time.Now().UTC()
	for i := 0; i < failureCount; i++ {
		ts := now.Add(-time.Duration(i*2) * time.Minute)
		user := users[rng.Intn(len(users))]
		ip := ips[rng.Intn(len(ips))]
		docs = append(docs, map[string]any{
			"timestamp":  ts.Format(time.RFC3339),
			"severity":   "high",
			"event_type": "ssh.auth_failure",
			"user":       user,
			"source_ip":  ip,
			"host":       "localhost",
			"message":    fmt.Sprintf("Failed password for %s from %s port %d ssh2", user, ip, 40000+rng.Intn(20000)),
		})
	}

	if includeBreach {
		docs = append(docs, map[string]any{
			"timestamp":  now.Format(time.RFC3339),
			"severity":   "critical",
			"event_type": "ssh.login_anomaly",
			"user":       "root",
			"source_ip":  "203.0.113.42",
			"host":       "localhost",
			"message":    "Accepted password for root from 203.0.113.42 — unknown source, first-seen login",
		})
	}

	return docs
}
