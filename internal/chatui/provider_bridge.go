package chatui

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	bridgepb "github.com/adam-stokes/orcai/proto/bridgepb"
	pb "github.com/adam-stokes/orcai/proto/orcai/v1"
)

// BridgeProvider implements Provider by forwarding requests over gRPC
// to a ProviderBridge adapter subprocess managed by bridge.Manager.
type BridgeProvider struct {
	mu         sync.Mutex
	client     bridgepb.ProviderBridgeClient
	name       string
	cwd        string
	sessionID  string
	model      string
	busClient  pb.EventBusClient
	windowName string
}

// connectBus reads ~/.config/orcai/bus.addr and returns a connected EventBusClient.
// Returns nil if the address file is missing or the connection fails.
func connectBus() (pb.EventBusClient, *grpc.ClientConn) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, nil
	}
	data, err := os.ReadFile(filepath.Join(home, ".config", "orcai", "bus.addr"))
	if err != nil || len(strings.TrimSpace(string(data))) == 0 {
		return nil, nil
	}
	addr := strings.TrimSpace(string(data))
	conn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, nil
	}
	return pb.NewEventBusClient(conn), conn
}

// currentWindowName returns the tmux window name for the current pane, or "".
func currentWindowName() string {
	out, err := exec.Command("tmux", "display-message", "-p", "#{window_name}").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// NewBridgeProvider creates a BridgeProvider wrapping the given gRPC client.
// It also attempts to connect to the event bus for telemetry publishing.
func NewBridgeProvider(client bridgepb.ProviderBridgeClient, name, cwd string) *BridgeProvider {
	busClient, _ := connectBus()
	return &BridgeProvider{
		client:     client,
		name:       name,
		cwd:        cwd,
		busClient:  busClient,
		windowName: currentWindowName(),
	}
}

// publishTelemetry marshals payload and publishes it to the orcai.telemetry bus topic.
// It is a no-op if busClient is nil.
func (p *BridgeProvider) publishTelemetry(payload TelemetryPayload) {
	if p.busClient == nil {
		return
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return
	}
	p.busClient.Publish(context.Background(), &pb.Event{ //nolint:errcheck
		Topic:   "orcai.telemetry",
		Source:  "chatui",
		Payload: data,
	})
}

func (p *BridgeProvider) Name() string { return p.name }

func (p *BridgeProvider) SetModel(m string) {
	p.mu.Lock()
	p.model = m
	p.mu.Unlock()
}

func (p *BridgeProvider) Model() string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.model
}

// Send opens a bidirectional gRPC Send stream and relays responses via the returned channel.
//
// If the adapter sends a WaitingPayload, a StreamWaiting event is emitted so the caller
// can collect user input. The goroutine then blocks on the returned InputCh until the
// user provides a reply, which is forwarded to the adapter.
func (p *BridgeProvider) Send(ctx context.Context, _ []message, text string) <-chan StreamEvent {
	ch := make(chan StreamEvent, 64)

	p.mu.Lock()
	sid := p.sessionID
	mdl := p.model
	p.mu.Unlock()

	go func() {
		defer close(ch)

		stream, err := p.client.Send(ctx)
		if err != nil {
			ch <- StreamEvent{Err: &StreamErr{Err: err.Error()}}
			return
		}

		// Send the initial request.
		if err := stream.Send(&bridgepb.SendRequest{
			Prompt:    text,
			SessionId: sid,
			Cwd:       p.cwd,
			Model:     mdl,
		}); err != nil {
			ch <- StreamEvent{Err: &StreamErr{Err: err.Error()}}
			return
		}

		// Publish streaming-start telemetry event.
		p.publishTelemetry(TelemetryPayload{
			SessionID:  sid,
			WindowName: p.windowName,
			Provider:   p.name,
			Status:     "streaming",
		})

		var done StreamDone
		for {
			resp, err := stream.Recv()
			if err == io.EOF {
				break
			}
			if err != nil {
				ch <- StreamEvent{Err: &StreamErr{Err: err.Error()}}
				return
			}

			switch payload := resp.Payload.(type) {
			case *bridgepb.SendResponse_Chunk:
				chunkText := payload.Chunk
				if strings.HasPrefix(chunkText, "\x02") {
					// Status event encoded as \x02{"tool":"...","input":"..."}
					var ev struct {
						Tool  string `json:"tool"`
						Input string `json:"input"`
					}
					if err := json.Unmarshal([]byte(chunkText[1:]), &ev); err == nil {
						ch <- StreamEvent{Status: &StreamStatus{Tool: ev.Tool, Input: ev.Input}}
					}
					continue
				}
				ch <- StreamEvent{Chunk: &StreamChunk{Text: chunkText}}

			case *bridgepb.SendResponse_Done:
				d := payload.Done
				done = StreamDone{
					SessionID:     d.SessionId,
					Model:         d.Model,
					InputTokens:   int(d.InputTokens),
					CacheTokens:   int(d.CacheTokens),
					OutputTokens:  int(d.OutputTokens),
					ContextWindow: int(d.ContextWindow),
				}
				p.mu.Lock()
				p.sessionID = d.SessionId
				p.mu.Unlock()
				// Publish done telemetry with cost estimate.
				cost := CostEstimate(d.Model, int(d.InputTokens), int(d.OutputTokens))
				p.publishTelemetry(TelemetryPayload{
					SessionID:    d.SessionId,
					WindowName:   p.windowName,
					Provider:     p.name,
					Status:       "done",
					InputTokens:  int(d.InputTokens),
					OutputTokens: int(d.OutputTokens),
					CostUSD:      cost,
				})

			case *bridgepb.SendResponse_Waiting:
				// Adapter subprocess needs user input. Signal the caller to
				// collect a reply, then block until it arrives.
				inputCh := make(chan string, 1)
				ch <- StreamEvent{Waiting: &StreamWaiting{Hint: payload.Waiting.Hint, InputCh: inputCh}}
				input := <-inputCh
				// Forward the reply to the adapter.
				if err := stream.Send(&bridgepb.SendRequest{Input: input}); err != nil {
					ch <- StreamEvent{Err: &StreamErr{Err: err.Error()}}
					return
				}

			case *bridgepb.SendResponse_Error:
				ch <- StreamEvent{Err: &StreamErr{Err: payload.Error}}
				return
			}
		}
		ch <- StreamEvent{Done: &done}
	}()

	return ch
}

// SetSession sets the session ID to resume on the next Send.
func (p *BridgeProvider) SetSession(id string) {
	p.mu.Lock()
	p.sessionID = id
	p.mu.Unlock()
}

// SessionID returns the current session ID.
func (p *BridgeProvider) SessionID() string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.sessionID
}

func (p *BridgeProvider) Close() {}
