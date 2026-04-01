package assistant

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/8op-org/gl1tch/internal/store"
)

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
