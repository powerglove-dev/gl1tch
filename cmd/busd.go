package cmd

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/8op-org/gl1tch/internal/busd"
)

func init() {
	rootCmd.AddCommand(busdCmd)
	busdCmd.AddCommand(busdPublishCmd)
}

var busdCmd = &cobra.Command{
	Use:   "busd",
	Short: "Interact with the gl1tch event bus",
}

var busdPublishCmd = &cobra.Command{
	Use:   "publish <topic> [json-payload]",
	Short: "Publish an event to the gl1tch event bus",
	Long: `Publishes a JSON event to the gl1tch BUSD socket.
Payload must be valid JSON or is treated as a plain string value.

Examples:
  glitch busd publish mud.chat.reply '{"text":"hello","world":"blockhaven"}'
  glitch busd publish my.custom.event '{"key":"value"}'`,
	Args: cobra.RangeArgs(1, 2),
	RunE: func(cmd *cobra.Command, args []string) error {
		topic := args[0]
		var payload any = map[string]any{}
		if len(args) == 2 {
			payload = json.RawMessage(args[1])
		}

		sockPath, err := busd.SocketPath()
		if err != nil {
			return fmt.Errorf("busd: %w", err)
		}
		if err := busd.PublishEvent(sockPath, topic, payload); err != nil {
			fmt.Fprintf(os.Stderr, "busd: publish failed (is glitch running?): %v\n", err)
			os.Exit(1)
		}
		return nil
	},
}
