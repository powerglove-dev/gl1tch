package console

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/8op-org/gl1tch/internal/executor"
	"github.com/8op-org/gl1tch/internal/game"
	"github.com/8op-org/gl1tch/internal/pipeline"
	"github.com/8op-org/gl1tch/internal/store"
)

// SignalHandlerRegistry maps handler names to dispatch functions.
// Plugins reference handlers by name in their signals block.
type SignalHandlerRegistry map[string]func(topic, payload string)

// BuildSignalHandlerRegistry constructs a registry with the built-in handlers.
// narrationCh receives companion narration strings; st is used by the score handler.
func BuildSignalHandlerRegistry(narrationCh chan<- string, st *store.Store) SignalHandlerRegistry {
	eng := game.NewGameEngine()
	return SignalHandlerRegistry{
		"companion":   companionHandler(eng, narrationCh),
		"score":       scoreHandler(st),
		"log":         logHandler(),
		"npc-memory":  npcMemoryHandler(st),
		"npc-narrate": npcNarrateHandler(narrationCh, st),
	}
}

// Dispatch looks up the handler for name and calls it with topic and payload.
// Unknown handlers emit a debug log and drop the event.
func (r SignalHandlerRegistry) Dispatch(name, topic, payload string) {
	h, ok := r[name]
	if !ok {
		log.Printf("[DEBUG] signal_handlers: unknown handler %q for topic %s — event dropped", name, topic)
		return
	}
	h(topic, payload)
}

const pluginCompanionPrompt = `You are gl1tch, a cynical AI companion watching a plugin event.
React to what just happened in 2-4 lines. Terse. Dry. Occasionally helpful. Never cheerful.
Reference the event naturally — don't just repeat the JSON. No markdown. No bullet points.`

func companionHandler(eng *game.GameEngine, ch chan<- string) func(topic, payload string) {
	return func(topic, payload string) {
		if ch == nil {
			return
		}
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			userMsg := fmt.Sprintf("Event: %s\nPayload: %s", topic, payload)
			result := eng.Respond(ctx, pluginCompanionPrompt, userMsg)
			if result != "" && ch != nil {
				ch <- result
			}
		}()
	}
}

// tokenUsagePayload is the expected JSON shape for the score handler.
type tokenUsagePayload struct {
	Input  int64  `json:"input"`
	Output int64  `json:"output"`
	Model  string `json:"model"`
}

func scoreHandler(st *store.Store) func(topic, payload string) {
	return func(topic, payload string) {
		if st == nil {
			return
		}
		var usage tokenUsagePayload
		if err := json.Unmarshal([]byte(payload), &usage); err != nil {
			log.Printf("[DEBUG] signal_handlers: score handler: cannot parse payload for %s: %v", topic, err)
			return
		}
		xpResult := game.ComputeXP(game.TokenUsage{
			InputTokens:  usage.Input,
			OutputTokens: usage.Output,
		}, 0)
		ev := store.ScoreEvent{
			XP:           xpResult.Final,
			InputTokens:  usage.Input,
			OutputTokens: usage.Output,
			Provider:     topic,
			Model:        usage.Model,
			CreatedAt:    time.Now().Unix(),
		}
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := st.RecordScoreEvent(ctx, ev); err != nil {
			log.Printf("[DEBUG] signal_handlers: score handler: record event: %v", err)
		}
	}
}

// npcMemoryPayload is the expected JSON shape for the npc-memory handler.
type npcMemoryPayload struct {
	NPCID        string `json:"npc_id"`
	NPCName      string `json:"npc_name"`
	Trigger      string `json:"trigger"`
	Text         string `json:"text"`
	StealthLevel int    `json:"stealth_level"`
}

func npcMemoryHandler(st *store.Store) func(topic, payload string) {
	return func(topic, payload string) {
		if st == nil {
			return
		}
		var p npcMemoryPayload
		if err := json.Unmarshal([]byte(payload), &p); err != nil {
			fmt.Fprintf(os.Stderr, "[npc-memory] cannot parse payload for %s: %v\n", topic, err)
			return
		}
		if p.NPCID == "" || p.NPCName == "" {
			fmt.Fprintf(os.Stderr, "[npc-memory] missing required fields (npc_id, npc_name) for %s\n", topic)
			return
		}
		body := fmt.Sprintf(
			"Player triggered %q with NPC %s (%s). NPC said: %q. Stealth: %d.",
			p.Trigger, p.NPCName, p.NPCID, p.Text, p.StealthLevel,
		)
		note := store.BrainNote{
			RunID:     0,
			StepID:    "npc-" + p.NPCID,
			CreatedAt: time.Now().Unix(),
			Tags:      "mud,npc-" + p.NPCID,
			Body:      body,
		}
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if _, err := st.InsertBrainNote(ctx, note); err != nil {
			fmt.Fprintf(os.Stderr, "[npc-memory] failed to write brain note: %v\n", err)
		}
	}
}

// npcNarrateHandler runs the mud-npc-narrate pipeline with brain injection so
// the Ollama narration has access to prior interaction notes for this NPC.
func npcNarrateHandler(narrationCh chan<- string, st *store.Store) func(topic, payload string) {
	pipelinePath := filepath.Join(os.Getenv("HOME"), "Projects", "gl1tch-mud", "pipelines", "mud-npc-narrate.pipeline.yaml")
	wrappersDir := filepath.Join(os.Getenv("HOME"), ".config", "glitch", "wrappers")

	return func(topic, payload string) {
		if narrationCh == nil {
			return
		}
		var p npcMemoryPayload
		if err := json.Unmarshal([]byte(payload), &p); err != nil || p.NPCID == "" {
			return
		}
		go func() {
			f, err := os.Open(pipelinePath)
			if err != nil {
				fmt.Fprintf(os.Stderr, "[npc-narrate] pipeline not found: %v\n", err)
				return
			}
			defer f.Close()

			pipe, err := pipeline.Load(f)
			if err != nil {
				fmt.Fprintf(os.Stderr, "[npc-narrate] pipeline load error: %v\n", err)
				return
			}

			mgr := executor.NewManager()
			if errs := mgr.LoadWrappersFromDir(wrappersDir); len(errs) > 0 {
				for _, e := range errs {
					fmt.Fprintf(os.Stderr, "[npc-narrate] sidecar warn: %v\n", e)
				}
			}

			userInput := fmt.Sprintf("NPC: %s\nWhat they said: %q\nTrigger: %s\nPlayer stealth: %d",
				p.NPCName, p.Text, p.Trigger, p.StealthLevel)

			var opts []pipeline.RunOption
			opts = append(opts, pipeline.WithNoClarification())
			if st != nil {
				opts = append(opts, pipeline.WithRunStore(st))
				opts = append(opts, pipeline.WithBrainInjector(pipeline.NewStoreBrainInjector(st)))
			}

			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()

			result, err := pipeline.Run(ctx, pipe, mgr, userInput, opts...)
			if err != nil {
				fmt.Fprintf(os.Stderr, "[npc-narrate] run error: %v\n", err)
				return
			}
			if result != "" {
				narrationCh <- result
			}
		}()
	}
}

func logHandler() func(topic, payload string) {
	logDir := filepath.Join(os.Getenv("HOME"), ".local", "share", "glitch")
	logPath := filepath.Join(logDir, "plugin-signals.log")
	return func(topic, payload string) {
		if err := os.MkdirAll(logDir, 0o755); err != nil {
			log.Printf("[WARN] signal_handlers: log handler: mkdir %s: %v", logDir, err)
			return
		}
		f, err := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
		if err != nil {
			log.Printf("[WARN] signal_handlers: log handler: open %s: %v", logPath, err)
			return
		}
		defer f.Close()
		line := fmt.Sprintf("%s %s %s\n", time.Now().UTC().Format(time.RFC3339), topic, payload)
		if _, err := f.WriteString(line); err != nil {
			log.Printf("[WARN] signal_handlers: log handler: write %s: %v", logPath, err)
		}
	}
}
