// Package handlers contains concrete supervisor event handlers.
package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/8op-org/gl1tch/internal/activity"
	"github.com/8op-org/gl1tch/internal/busd"
	"github.com/8op-org/gl1tch/internal/busd/topics"
	"github.com/8op-org/gl1tch/internal/executor"
	"github.com/8op-org/gl1tch/internal/supervisor"
)

// diagnosisTopics are the busd topics that trigger error diagnosis.
var diagnosisTopics = []string{
	topics.RunFailed,
	topics.StepFailed,
	topics.AgentRunFailed,
	topics.WorkflowRunFailed,
	topics.WorkflowStepFailed,
}

// NotificationPayload is the JSON payload published to notification topics.
type NotificationPayload struct {
	Session  string `json:"session"`
	Title    string `json:"title"`
	Body     string `json:"body"`
	Severity string `json:"severity"`
}

// EventPublisher can publish events back to the bus.
type EventPublisher interface {
	Publish(ctx context.Context, topic string, payload []byte) error
}

// busPublisher implements EventPublisher by dialing the busd socket on each call.
type busPublisher struct {
	sockPath string
}

// NewBusPublisher returns an EventPublisher that publishes to the given socket path.
func NewBusPublisher(sockPath string) EventPublisher {
	return &busPublisher{sockPath: sockPath}
}

func (bp *busPublisher) Publish(ctx context.Context, topic string, payload []byte) error {
	var p any
	if err := json.Unmarshal(payload, &p); err != nil {
		p = string(payload)
	}
	return busd.PublishEventCtx(ctx, bp.sockPath, topic, p)
}

// DiagnosisHandler reacts to failure events by invoking the "diagnosis" role
// model to generate an explanation, then publishes a notification event.
type DiagnosisHandler struct {
	execMgr *executor.Manager
	pub     EventPublisher
}

// NewDiagnosisHandler creates a DiagnosisHandler.
func NewDiagnosisHandler(execMgr *executor.Manager, pub EventPublisher) *DiagnosisHandler {
	return &DiagnosisHandler{execMgr: execMgr, pub: pub}
}

func (d *DiagnosisHandler) Name() string    { return "diagnosis" }
func (d *DiagnosisHandler) Topics() []string { return diagnosisTopics }

// Handle is invoked for each matching failure event.
func (d *DiagnosisHandler) Handle(ctx context.Context, evt supervisor.Event, model supervisor.ResolvedModel) error {
	// Build a concise error context prompt from the raw payload.
	prompt := fmt.Sprintf(
		"You are a debugging assistant. A gl1tch pipeline event failed.\n"+
			"Topic: %s\n"+
			"Payload: %s\n\n"+
			"Briefly explain what likely went wrong and suggest one concrete fix (3 sentences max).",
		evt.Topic, string(evt.Payload),
	)

	// Invoke the model via the executor manager.
	response := ""
	if d.execMgr != nil && model.ProviderID != "" && model.ModelID != "" {
		executorName := model.ProviderID
		var buf bytes.Buffer
		vars := map[string]string{"model": model.ModelID}
		if err := d.execMgr.Execute(ctx, executorName, prompt, vars, &buf); err != nil {
			slog.Warn("diagnosis: model invocation failed",
				"executor", executorName,
				"model", model.ModelID,
				"err", err)
		} else {
			response = buf.String()
		}
	}

	if response == "" {
		response = fmt.Sprintf("Event %q failed. Check logs for details.", evt.Topic)
	}

	// Publish the notification event.
	if d.pub != nil {
		note := NotificationPayload{
			Session:  "",
			Title:    "Error in " + evt.Topic,
			Body:     response,
			Severity: "warning",
		}
		noteBytes, err := json.Marshal(note)
		if err == nil {
			if pubErr := d.pub.Publish(ctx, topics.NotificationErrorDiagnosed, noteBytes); pubErr != nil {
				slog.Warn("diagnosis: failed to publish notification", "err", pubErr)
			}
		}
	}

	// Log to activity feed.
	label := fmt.Sprintf("diagnosed: %s", evt.Topic)
	agent := model.ProviderID
	if model.ModelID != "" {
		agent = model.ProviderID + "/" + model.ModelID
	}
	_ = activity.AppendEvent(activity.DefaultPath(), activity.ActivityEvent{
		TS:     time.Now().UTC().Format(time.RFC3339),
		Kind:   "diagnosis",
		Agent:  agent,
		Label:  label,
		Status: "done",
	})

	return nil
}
