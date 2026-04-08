package clarify

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"

	"github.com/8op-org/gl1tch/internal/store"
)

// CLIAnswerer is the command-line counterpart to the desktop's pollClarifications:
// it watches the store for pending ClarificationRequest rows, prints each
// question to stderr, reads a line from stdin, and writes the answer back with
// store.AnswerClarification so the blocked pipeline step can resume.
//
// One instance lives for the duration of a single `glitch ask` invocation.
// Closing the context stops the poll loop cleanly.
type CLIAnswerer struct {
	st       *store.Store
	stdin    io.Reader
	stderr   io.Writer
	interval time.Duration

	mu       sync.Mutex
	answered map[string]bool
}

// NewCLIAnswerer wires the answerer to an open store and stdin/stderr. The
// caller retains ownership of the store — the answerer will not close it.
func NewCLIAnswerer(st *store.Store, stdin io.Reader, stderr io.Writer) *CLIAnswerer {
	return &CLIAnswerer{
		st:       st,
		stdin:    stdin,
		stderr:   stderr,
		interval: time.Second,
		answered: map[string]bool{},
	}
}

// Run blocks until ctx is cancelled, polling for clarification requests. It is
// safe to call once per ask invocation; spawn it in a goroutine:
//
//	go answerer.Run(ctx)
//
// The answerer serialises stdin reads, so even if multiple steps emit
// GLITCH_CLARIFY concurrently they are prompted one at a time.
func (c *CLIAnswerer) Run(ctx context.Context) {
	if c == nil || c.st == nil {
		return
	}
	reader := bufio.NewReader(c.stdin)
	ticker := time.NewTicker(c.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			reqs, err := c.st.LoadPendingClarifications()
			if err != nil {
				continue
			}
			for _, req := range reqs {
				c.mu.Lock()
				already := c.answered[req.RunID]
				c.mu.Unlock()
				if already {
					continue
				}
				answer, ok := c.prompt(ctx, reader, req.Question)
				if !ok {
					return
				}
				if err := c.st.AnswerClarification(req.RunID, answer); err != nil {
					fmt.Fprintf(c.stderr, "clarify: write answer: %v\n", err)
					continue
				}
				c.mu.Lock()
				c.answered[req.RunID] = true
				c.mu.Unlock()
			}
		}
	}
}

// prompt shows the question and reads one line from stdin. Returns (answer,
// true) on success, or ("", false) if the context is cancelled or stdin is
// closed. A blank answer is replaced with "(no response)" so the pipeline
// still unblocks — AnswerClarification treats empty strings as unanswered.
func (c *CLIAnswerer) prompt(ctx context.Context, reader *bufio.Reader, question string) (string, bool) {
	fmt.Fprintf(c.stderr, "\n\x1b[33m?\x1b[0m %s\n> ", strings.TrimSpace(question))

	type lineResult struct {
		line string
		err  error
	}
	ch := make(chan lineResult, 1)
	go func() {
		line, err := reader.ReadString('\n')
		ch <- lineResult{line: line, err: err}
	}()

	select {
	case <-ctx.Done():
		return "", false
	case res := <-ch:
		if res.err != nil && res.line == "" {
			return "", false
		}
		answer := strings.TrimRight(res.line, "\r\n")
		if strings.TrimSpace(answer) == "" {
			answer = "(no response)"
		}
		return answer, true
	}
}
