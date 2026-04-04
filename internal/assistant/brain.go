package assistant

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/8op-org/gl1tch/internal/store"
)

// userModelBrainRe matches <brain ...> blocks in AI output for user-model extraction.
var userModelBrainRe = regexp.MustCompile(`(?s)<brain\b([^>]*?)>(.*?)</brain>`)

// userModelAttrRe extracts key="value" or key='value' attribute pairs.
var userModelAttrRe = regexp.MustCompile(`(\w+)=["']([^"']*)["']`)

// SaveToBrain writes the conversation turns as a brain note tagged for assistant use.
// Skips if st is nil or turns is empty.
func SaveToBrain(ctx context.Context, st *store.Store, turns []Turn) error {
	if st == nil || len(turns) == 0 {
		return nil
	}

	var sb strings.Builder
	for i, t := range turns {
		if i > 0 {
			sb.WriteString("\n\n")
		}
		switch t.Role {
		case "user":
			sb.WriteString("USER: ")
		default:
			sb.WriteString("GLITCH: ")
		}
		sb.WriteString(t.Text)
	}

	note := store.BrainNote{
		StepID:    "glitch-assistant",
		CreatedAt: time.Now().UnixMilli(),
		Tags:      fmt.Sprintf("type:conversation title:\"GLITCH %s\" tags:\"assistant,glitch\"", time.Now().Format("2006-01-02 15:04")),
		Body:      sb.String(),
	}

	_, err := st.InsertBrainNote(ctx, note)
	return err
}

// ExtractAndSaveUserModel runs a quick AI pass over turns to identify user
// preferences and saves each as a user_model capability note (run_id=0).
// summarize should call the active LLM and return its full response.
// Runs best-effort: errors are silently dropped. Skips when st is nil,
// turns < 3, or summarize is nil.
func ExtractAndSaveUserModel(ctx context.Context, st *store.Store, turns []Turn, summarize func(context.Context, string) (string, error)) {
	if st == nil || len(turns) < 3 || summarize == nil {
		return
	}

	var conv strings.Builder
	for _, t := range turns {
		switch t.Role {
		case "user":
			conv.WriteString("USER: ")
		default:
			conv.WriteString("GLITCH: ")
		}
		conv.WriteString(t.Text)
		conv.WriteString("\n\n")
	}

	prompt := "Review this conversation and extract 0-3 concise facts about the user's preferences, " +
		"workflow, or coding style that would help a future AI session be more useful.\n\n" +
		"Format each finding as:\n" +
		"<brain type=\"user_model\" title=\"short title\" tags=\"preference\">\n" +
		"one sentence describing the preference or pattern\n</brain>\n\n" +
		"Only extract clearly stated or strongly implied patterns. " +
		"If nothing notable is apparent, output nothing — no explanation, no preamble.\n\n" +
		"CONVERSATION:\n" + conv.String()

	response, err := summarize(ctx, prompt)
	if err != nil || response == "" {
		return
	}

	for _, m := range userModelBrainRe.FindAllStringSubmatch(response, -1) {
		attrs := parseUserModelAttrs(m[1])
		body := strings.TrimSpace(m[2])
		if body == "" {
			continue
		}
		title := attrs["title"]
		stepID := "user-model"
		if title != "" {
			stepID = "user-model:" + strings.ToLower(strings.ReplaceAll(title, " ", "-"))
		}
		var parts []string
		if t := attrs["type"]; t != "" {
			parts = append(parts, "type:"+t)
		}
		if title != "" {
			parts = append(parts, "title:"+title)
		}
		if tags := attrs["tags"]; tags != "" {
			parts = append(parts, "tags:"+tags)
		}
		_ = st.UpsertCapabilityNote(ctx, store.BrainNote{
			StepID:    stepID,
			CreatedAt: time.Now().UnixMilli(),
			Tags:      strings.Join(parts, " "),
			Body:      body,
		})
	}
}

func parseUserModelAttrs(attrStr string) map[string]string {
	attrs := make(map[string]string)
	for _, m := range userModelAttrRe.FindAllStringSubmatch(attrStr, -1) {
		attrs[m[1]] = m[2]
	}
	return attrs
}

// userModelSummarize collects all tokens from a streaming channel into a string.
// It drains tokenCh until closed and discards doneCh.
func UserModelCollect(tokenCh <-chan string, doneCh <-chan string) string {
	var sb strings.Builder
	for tok := range tokenCh {
		sb.WriteString(tok)
	}
	if doneCh != nil {
		for range doneCh {
		}
	}
	return sb.String()
}
