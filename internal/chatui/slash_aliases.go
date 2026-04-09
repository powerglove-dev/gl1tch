package chatui

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"gopkg.in/yaml.v3"
)

// slash_aliases.go is the externalized loader for user-defined slash
// command aliases. The Go-registered slash commands (/help, /research)
// stay in code because they ARE Go logic — they call into registries
// the user can't construct from a YAML file. But the most useful kind
// of slash command is an alias: short name, long expansion, no
// behavior of its own beyond rewriting the input.
//
// Example user file (~/.config/glitch/slash.yaml):
//
//   aliases:
//     - name: prs
//       describe: List open pull requests in the current repo
//       expand: "/research what pull requests are currently open"
//
//     - name: standup
//       describe: Recent commits + open PRs in one shot
//       expand: "/research summarise the last 10 commits and any open PRs"
//
// At dispatch time, the registry rewrites the line "/prs" → the
// expand value, then re-dispatches. The expansion can target any
// other slash command (including built-ins like /research) so users
// can build muscle memory without recompiling.
//
// File is optional — a missing or malformed file silently produces
// zero aliases, never breaks the dispatcher.

// SlashAlias is one entry in the aliases YAML. Exported so callers
// that build their own loader (tests, plugin systems) can compose
// from the same shape.
type SlashAlias struct {
	Name     string `yaml:"name"`
	Describe string `yaml:"describe"`
	Expand   string `yaml:"expand"`
}

// slashAliasDoc is the on-disk YAML shape.
type slashAliasDoc struct {
	Aliases []SlashAlias `yaml:"aliases"`
}

// LoadSlashAliases reads ~/.config/glitch/slash.yaml and returns the
// alias entries. Returns nil + nil error when no file exists — the
// "no aliases configured" path is the common case for new installs.
// Errors only when a file exists but fails to parse.
func LoadSlashAliases() ([]SlashAlias, error) {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return nil, nil
	}
	path := filepath.Join(home, ".config", "glitch", "slash.yaml")
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	return parseSlashAliasesYAML(data)
}

func parseSlashAliasesYAML(data []byte) ([]SlashAlias, error) {
	var doc slashAliasDoc
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("chatui: parse slash aliases yaml: %w", err)
	}
	clean := make([]SlashAlias, 0, len(doc.Aliases))
	for _, a := range doc.Aliases {
		a.Name = strings.TrimSpace(a.Name)
		a.Describe = strings.TrimSpace(a.Describe)
		a.Expand = strings.TrimSpace(a.Expand)
		if a.Name == "" || a.Expand == "" {
			continue
		}
		clean = append(clean, a)
	}
	return clean, nil
}

// SlashAliasOverridePath returns the absolute path the user file
// would live at. Used by `glitch slash edit` to seed it.
func SlashAliasOverridePath() string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return ""
	}
	return filepath.Join(home, ".config", "glitch", "slash.yaml")
}

// EmbeddedSlashAliasesDefault returns the canonical empty-but-
// commented YAML body the `glitch slash edit` command seeds when no
// override exists. It's a starter file with example entries
// commented out so the user has a working template to copy from
// rather than a blank page.
func EmbeddedSlashAliasesDefault() []byte {
	return []byte(slashAliasesSeedYAML)
}

const slashAliasesSeedYAML = `# slash.yaml — user-defined slash command aliases.
#
# Each alias maps a short name to an expansion string. Typing the
# alias in the chat input rewrites the line to the expansion and
# re-dispatches, so an alias can target any other slash command —
# including built-ins like /research.
#
# Editing this file takes effect on the next slash registry
# construction (next desktop session, next CLI invocation). No
# recompile needed.
#
# Examples (uncomment to enable):

aliases: []

# - name: prs
#   describe: List open pull requests in the current repo
#   expand: "/research what pull requests are currently open"
#
# - name: standup
#   describe: Recent commits + open PRs in one shot
#   expand: "/research summarise the last 10 commits and any open PRs"
#
# - name: blame
#   describe: Drill into the most recent commit
#   expand: "/research who made the most recent commit and what did it change"
`

// RegisterAliases reads slash aliases from disk and registers each
// one with the supplied registry as a SlashHandlerFunc whose Handle
// method re-dispatches the expansion through the same registry.
// Aliases that collide with already-registered names lose to the
// existing handler — the built-ins always win, so a user alias
// can't accidentally hide /help or /research.
//
// Called once by NewSlashRegistry on construction; tests can call
// it directly with a controlled registry to verify alias behavior.
func RegisterAliases(reg *SlashRegistry) error {
	aliases, err := LoadSlashAliases()
	if err != nil {
		return err
	}
	for _, alias := range aliases {
		// Skip if a built-in already owns the name. The slash
		// dispatcher's Lookup is the source of truth.
		if _, exists := reg.Lookup(alias.Name); exists {
			continue
		}
		registerOneAlias(reg, alias)
	}
	return nil
}

// registerOneAlias is the per-alias closure builder. Pulled out so
// the loop variable doesn't accidentally close over the wrong
// alias struct.
func registerOneAlias(reg *SlashRegistry, alias SlashAlias) {
	expand := alias.Expand
	_ = reg.Register(SlashHandlerFunc{
		NameField:     alias.Name,
		DescribeField: alias.Describe,
		Fn: func(ctx context.Context, in SlashInvocation) ([]ChatMessage, error) {
			// Append any args the user passed to the alias so a
			// templated alias like `/research <topic>` can carry
			// extra context. The expansion line is the alias body
			// followed by the alias's args (not the dispatcher's
			// flag form — flags don't compose cleanly).
			line := expand
			if in.Raw != "" {
				line = expand + " " + in.Raw
			}
			return reg.Dispatch(ctx, line, in.Scope)
		},
	})
}

// aliasOnce gates the disk-read so a process that constructs many
// SlashRegistries doesn't re-read the file each time. Tests that
// need a fresh load can construct their own registry and call
// RegisterAliases directly (which always re-reads).
var aliasOnce sync.Once
