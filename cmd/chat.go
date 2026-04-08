package cmd

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/spf13/cobra"

	"github.com/8op-org/gl1tch/internal/capability"
)

var (
	chatModel    string
	chatBaseURL  string
	chatSkills   string
	chatPersona  string
	chatMaxHops  int
)

func init() {
	rootCmd.AddCommand(chatCmd)
	chatCmd.Flags().StringVarP(&chatModel, "model", "m", "",
		"tool-capable local LLM (default: qwen2.5:7b)")
	chatCmd.Flags().StringVar(&chatBaseURL, "ollama-url", "",
		"override Ollama base URL (default: http://localhost:11434)")
	chatCmd.Flags().StringVar(&chatSkills, "skills", "",
		"override skill directory (default: ~/.config/glitch/capabilities)")
	chatCmd.Flags().StringVar(&chatPersona, "persona", "",
		"optional system message (path to a markdown file; empty = no system turn)")
	chatCmd.Flags().IntVar(&chatMaxHops, "max-hops", 0,
		"maximum tool-call hops per user turn (default: 8)")
}

var chatCmd = &cobra.Command{
	Use:   "chat",
	Short: "Multi-turn chat with tool-using capabilities",
	Long: `Start an interactive chat session backed by a tool-capable local LLM.

Each on-demand capability in your skill directory is exposed to the model as
a callable tool. The model decides when to answer directly and when to invoke
a capability — gl1tch never constructs tool calls on the model's behalf.

No hardcoded system prompt is injected. Pass --persona with a path to a
markdown file if you want a specific voice; otherwise the model speaks for
itself and the tool catalog is its only context.

Type /quit or press Ctrl-D to exit. Type /reset to clear the conversation.

Examples:
  glitch chat
  glitch chat --persona ~/.config/glitch/persona.md
  glitch chat --model qwen2.5:7b --max-hops 4`,
	RunE: func(cmd *cobra.Command, args []string) error {
		reg, err := loadAssistantRegistry(chatSkills)
		if err != nil {
			return err
		}
		if len(reg.Names()) == 0 {
			fmt.Fprintln(os.Stderr,
				"[no on-demand capabilities loaded — the model will have no tools]")
		}

		runner := capability.NewRunner(reg, nil)
		provider := capability.NewOllamaToolProvider()
		if chatModel != "" {
			provider.Model = chatModel
		}
		if chatBaseURL != "" {
			provider.BaseURL = chatBaseURL
		}

		agent := capability.NewAgent(reg, runner, provider)
		if chatMaxHops > 0 {
			agent.MaxToolHops = chatMaxHops
		}
		if chatPersona != "" {
			data, err := os.ReadFile(chatPersona)
			if err != nil {
				return fmt.Errorf("chat: read persona: %w", err)
			}
			agent.System = strings.TrimSpace(string(data))
		}

		ctx, cancel := signal.NotifyContext(context.Background(),
			os.Interrupt, syscall.SIGTERM)
		defer cancel()

		return runChatLoop(ctx, agent)
	},
}

// runChatLoop drives the interactive REPL. Factored out so it can be
// exercised without spawning a real terminal — pass an Agent wired to a
// scripted ToolProvider and you can snapshot a whole conversation in a
// test.
func runChatLoop(ctx context.Context, agent *capability.Agent) error {
	in := bufio.NewReader(os.Stdin)
	fmt.Fprintln(os.Stderr, "glitch chat — /quit to exit, /reset to clear history")
	for {
		fmt.Fprint(os.Stderr, "> ")
		line, err := in.ReadString('\n')
		if errors.Is(err, io.EOF) {
			fmt.Fprintln(os.Stderr)
			return nil
		}
		if err != nil {
			return err
		}
		msg := strings.TrimSpace(line)
		if msg == "" {
			continue
		}
		switch msg {
		case "/quit", "/exit":
			return nil
		case "/reset":
			agent.Reset()
			fmt.Fprintln(os.Stderr, "[history cleared]")
			continue
		}

		if _, err := agent.Send(ctx, msg, os.Stdout); err != nil {
			if ctx.Err() != nil {
				return nil
			}
			fmt.Fprintf(os.Stderr, "\n[error: %v]\n", err)
			continue
		}
		fmt.Fprintln(os.Stdout)
	}
}
