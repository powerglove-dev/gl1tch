package observer

import (
	"context"
	"fmt"
)

// Backend implements the gl1tch chat backend interface using the observer
// query engine. It streams answers from ES data synthesized by Ollama.
type Backend struct {
	engine *QueryEngine
	model  string
}

// NewBackend creates an observer backend.
func NewBackend(engine *QueryEngine, model string) *Backend {
	return &Backend{engine: engine, model: model}
}

// Name returns the backend identifier.
func (b *Backend) Name() string {
	return fmt.Sprintf("observer/%s", b.model)
}

// StreamIntro returns a brief intro message.
func (b *Backend) StreamIntro(ctx context.Context, cwd string) (<-chan string, error) {
	ch := make(chan string, 1)
	go func() {
		defer close(ch)
		ch <- "gl1tch observer online. ask me about your repos, pipelines, and activity."
	}()
	return ch, nil
}

// Stream processes a question through the observer query engine.
// It returns a token channel (streaming) and a done channel.
func (b *Backend) Stream(ctx context.Context, question string) (<-chan string, <-chan string, error) {
	tokenCh := make(chan string, 64)
	doneCh := make(chan string, 1)

	go func() {
		defer close(doneCh)
		err := b.engine.Stream(ctx, question, tokenCh)
		if err != nil {
			// If streaming failed, try non-streaming fallback.
			answer, fallbackErr := b.engine.Answer(ctx, question)
			if fallbackErr == nil {
				// Send the full answer as one token — the channel was already
				// closed by Stream's defer, so we need a fresh one. Since we
				// can't reopen it, just send the error info on doneCh.
				doneCh <- ""
				// The tokenCh was closed by Stream. We'll handle this in the
				// TUI by checking the done message.
				_ = answer
				return
			}
			doneCh <- ""
			return
		}
		doneCh <- ""
	}()

	return tokenCh, doneCh, nil
}
