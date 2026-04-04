//go:build ignore

// Run with: go run ./cmd/busd_test_main.go
package main

import (
	"fmt"
	"os"
	"time"

	"github.com/8op-org/gl1tch/internal/busd"
)

func main() {
	sockPath, err := busd.SocketPath()
	if err != nil {
		fmt.Fprintf(os.Stderr, "busd-test: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("starting bus at %s\n", sockPath)

	d := busd.New()
	if err := d.StartAt(sockPath); err != nil {
		fmt.Fprintf(os.Stderr, "busd-test: start: %v\n", err)
		os.Exit(1)
	}
	defer d.Stop()

	fmt.Println("waiting 3s for glitch-notify to connect…")
	time.Sleep(3 * time.Second)

	events := []struct {
		topic   string
		payload map[string]any
	}{
		{"pipeline.run.completed", map[string]any{"pipeline": "morning-briefing", "name": "morning-briefing"}},
		{"agent.run.clarification", map[string]any{"question": "should I archive these emails?"}},
		{"pipeline.run.failed", map[string]any{"pipeline": "github-pr-review", "name": "github-pr-review"}},
		{"game.achievement.unlocked", map[string]any{"achievement": "first pipeline", "name": "first pipeline"}},
	}

	for _, e := range events {
		fmt.Printf("→ %s\n", e.topic)
		busd.PublishEvent(sockPath, e.topic, e.payload)
		time.Sleep(2 * time.Second)
	}
	fmt.Println("done")
}
