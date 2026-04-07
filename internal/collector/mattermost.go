package collector

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/8op-org/gl1tch/internal/esearch"
)

// MattermostCollector indexes messages from Mattermost channels the bot has
// access to. It polls the REST API v4 for new posts, categorising them as
// mentions, direct messages, or regular channel messages.
type MattermostCollector struct {
	URL      string        // Mattermost server URL (e.g. https://chat.example.com)
	Token    string        // Bot or personal-access token
	Channels []string      // channel names to auto-join and monitor (empty = all)
	Interval time.Duration // poll interval (default 60s)

	client *http.Client
	userID string // authenticated user/bot ID
}

func (m *MattermostCollector) Name() string { return "mattermost" }

func (m *MattermostCollector) Start(ctx context.Context, es *esearch.Client) error {
	if m.Interval == 0 {
		m.Interval = 60 * time.Second
	}
	m.client = &http.Client{Timeout: 15 * time.Second}

	// Verify credentials and get our user ID.
	me, err := m.getMe(ctx)
	if err != nil {
		return fmt.Errorf("mattermost collector: auth failed: %w", err)
	}
	m.userID = me.ID
	slog.Info("mattermost collector: authenticated", "user", me.Username)

	// Auto-join configured channels.
	if len(m.Channels) > 0 {
		m.autoJoinChannels(ctx)
	}

	// Track last poll time per channel.
	lastPoll := make(map[string]int64) // channel_id -> unix ms

	ticker := time.NewTicker(m.Interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			tickStart := time.Now()
			tickIndexed := 0
			var tickErr error
			channels, err := m.getMyChannels(ctx)
			if err != nil {
				slog.Warn("mattermost collector: list channels", "err", err)
				RecordRun("mattermost", tickStart, 0, err)
				continue
			}

			// Filter to configured channels if specified.
			if len(m.Channels) > 0 {
				channels = m.filterChannels(channels)
			}

			var docs []any
			for _, ch := range channels {
				since := lastPoll[ch.ID]
				if since == 0 {
					// First run: backfill last hour.
					since = time.Now().Add(-1 * time.Hour).UnixMilli()
				}

				posts, err := m.getPostsSince(ctx, ch.ID, since)
				if err != nil {
					slog.Warn("mattermost collector: poll channel", "channel", ch.DisplayName, "err", err)
					continue
				}

				for _, post := range posts {
					// Skip system messages (joins/leaves/pins) but
					// DO index the user's own messages — we want the
					// brain to have the full conversational context,
					// not just one side of every thread.
					if post.Type != "" {
						continue
					}
					if strings.TrimSpace(post.Message) == "" {
						continue
					}

					eventType := mmEventType(ch.Type, post.Message, me.Username)
					sender := m.resolveUsername(ctx, post.UserID)

					docs = append(docs, esearch.Event{
						Type:    eventType,
						Source:  "mattermost",
						Author:  sender,
						Message: post.Message, // full body in the indexed field
						Body:    post.Message,
						Metadata: map[string]any{
							"post_id":      post.ID,
							"channel_id":   ch.ID,
							"channel_name": ch.DisplayName,
							"channel_type": ch.Type,
							"root_id":      post.RootID,
						},
						Timestamp: time.UnixMilli(post.CreateAt),
					})

					// Emit a chat-style log line so the user can tail
					// real conversations from the brain popover's
					// logs panel. Format: "#channel <author> HH:MM
					// said <message>". Multi-line messages collapse
					// to a single line with " / " as a soft break
					// so the log row stays scan-readable.
					ts := time.UnixMilli(post.CreateAt).Format("15:04")
					body := strings.ReplaceAll(strings.TrimSpace(post.Message), "\n", " / ")
					if len(body) > 400 {
						body = body[:397] + "…"
					}
					channelLabel := ch.DisplayName
					if channelLabel == "" {
						channelLabel = ch.ID
					}
					slog.Info(
						fmt.Sprintf("mattermost: #%s <%s> %s said %s",
							channelLabel, sender, ts, body),
					)
				}

				lastPoll[ch.ID] = time.Now().UnixMilli()
			}

			if len(docs) > 0 {
				slog.Info("mattermost collector: new messages", "count", len(docs))
				if err := es.BulkIndex(ctx, esearch.IndexEvents, docs); err != nil {
					slog.Warn("mattermost collector: bulk index", "err", err)
					tickErr = err
				}
				tickIndexed = len(docs)
			}
			RecordRun("mattermost", tickStart, tickIndexed, tickErr)
		}
	}
}

// mmEventType classifies a post based on channel type and content.
func mmEventType(channelType, message, myUsername string) string {
	switch channelType {
	case "D":
		return "mattermost.direct"
	case "G":
		return "mattermost.group"
	default:
		if strings.Contains(message, "@"+myUsername) {
			return "mattermost.mention"
		}
		return "mattermost.message"
	}
}

// --- Mattermost API types ---

type mmUser struct {
	ID       string `json:"id"`
	Username string `json:"username"`
}

type mmChannel struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	DisplayName string `json:"display_name"`
	Type        string `json:"type"` // O=open, P=private, D=direct, G=group
	TeamID      string `json:"team_id"`
}

type mmPost struct {
	ID       string `json:"id"`
	UserID   string `json:"user_id"`
	ChannelID string `json:"channel_id"`
	RootID   string `json:"root_id"`
	Message  string `json:"message"`
	Type     string `json:"type"` // "" for normal, system_* for system msgs
	CreateAt int64  `json:"create_at"`
}

type mmPostList struct {
	Order []string           `json:"order"`
	Posts map[string]*mmPost `json:"posts"`
}

// --- API methods ---

func (m *MattermostCollector) apiGet(ctx context.Context, path string) ([]byte, error) {
	url := strings.TrimRight(m.URL, "/") + "/api/v4" + path
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+m.Token)

	resp, err := m.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("mattermost API %s: %d %s", path, resp.StatusCode, string(body))
	}
	return body, nil
}

func (m *MattermostCollector) apiPost(ctx context.Context, path string, body string) error {
	url := strings.TrimRight(m.URL, "/") + "/api/v4" + path
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, strings.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+m.Token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := m.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("mattermost API POST %s: %d %s", path, resp.StatusCode, string(b))
	}
	return nil
}

func (m *MattermostCollector) getMe(ctx context.Context) (*mmUser, error) {
	data, err := m.apiGet(ctx, "/users/me")
	if err != nil {
		return nil, err
	}
	var user mmUser
	return &user, json.Unmarshal(data, &user)
}

func (m *MattermostCollector) getMyChannels(ctx context.Context) ([]mmChannel, error) {
	// Get teams first, then channels per team. Also include DM/group channels.
	data, err := m.apiGet(ctx, "/users/me/teams")
	if err != nil {
		return nil, err
	}
	var teams []struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(data, &teams); err != nil {
		return nil, err
	}

	var all []mmChannel
	for _, team := range teams {
		data, err := m.apiGet(ctx, fmt.Sprintf("/users/me/teams/%s/channels", team.ID))
		if err != nil {
			continue
		}
		var channels []mmChannel
		if err := json.Unmarshal(data, &channels); err != nil {
			continue
		}
		all = append(all, channels...)
	}

	// Also fetch DM/group channels (not team-scoped).
	data, err = m.apiGet(ctx, fmt.Sprintf("/users/%s/channels", m.userID))
	if err == nil {
		var dmChannels []mmChannel
		if json.Unmarshal(data, &dmChannels) == nil {
			for _, ch := range dmChannels {
				if ch.Type == "D" || ch.Type == "G" {
					all = append(all, ch)
				}
			}
		}
	}

	return dedupeChannels(all), nil
}

func (m *MattermostCollector) getPostsSince(ctx context.Context, channelID string, sinceMs int64) ([]*mmPost, error) {
	path := fmt.Sprintf("/channels/%s/posts?since=%d", channelID, sinceMs)
	data, err := m.apiGet(ctx, path)
	if err != nil {
		return nil, err
	}

	var pl mmPostList
	if err := json.Unmarshal(data, &pl); err != nil {
		return nil, err
	}

	// Return in chronological order.
	var posts []*mmPost
	for i := len(pl.Order) - 1; i >= 0; i-- {
		if p, ok := pl.Posts[pl.Order[i]]; ok {
			posts = append(posts, p)
		}
	}
	return posts, nil
}

// resolveUsername fetches a username by ID, caching results for the session.
var mmUserCache = make(map[string]string)

func (m *MattermostCollector) resolveUsername(ctx context.Context, userID string) string {
	if name, ok := mmUserCache[userID]; ok {
		return name
	}
	data, err := m.apiGet(ctx, "/users/"+userID)
	if err != nil {
		return userID
	}
	var u mmUser
	if json.Unmarshal(data, &u) == nil && u.Username != "" {
		mmUserCache[userID] = u.Username
		return u.Username
	}
	return userID
}

// autoJoinChannels joins channels by name across all teams the bot belongs to.
func (m *MattermostCollector) autoJoinChannels(ctx context.Context) {
	data, err := m.apiGet(ctx, "/users/me/teams")
	if err != nil {
		slog.Warn("mattermost collector: auto-join: list teams", "err", err)
		return
	}
	var teams []struct {
		ID string `json:"id"`
	}
	if json.Unmarshal(data, &teams) != nil {
		return
	}

	want := make(map[string]bool)
	for _, ch := range m.Channels {
		want[ch] = true
	}

	for _, team := range teams {
		for name := range want {
			chData, err := m.apiGet(ctx, fmt.Sprintf("/teams/%s/channels/name/%s", team.ID, name))
			if err != nil {
				continue
			}
			var ch mmChannel
			if json.Unmarshal(chData, &ch) != nil {
				continue
			}
			// Join: POST /channels/{id}/members with user_id.
			body := fmt.Sprintf(`{"user_id":"%s"}`, m.userID)
			if err := m.apiPost(ctx, fmt.Sprintf("/channels/%s/members", ch.ID), body); err != nil {
				slog.Warn("mattermost collector: auto-join failed", "channel", name, "err", err)
			} else {
				slog.Info("mattermost collector: joined channel", "channel", name)
			}
		}
	}
}

// filterChannels returns only channels whose name matches the configured list.
func (m *MattermostCollector) filterChannels(channels []mmChannel) []mmChannel {
	want := make(map[string]bool)
	for _, name := range m.Channels {
		want[name] = true
	}
	var out []mmChannel
	for _, ch := range channels {
		if want[ch.Name] || want[ch.DisplayName] {
			out = append(out, ch)
		}
	}
	return out
}

func dedupeChannels(channels []mmChannel) []mmChannel {
	seen := make(map[string]bool)
	var out []mmChannel
	for _, ch := range channels {
		if !seen[ch.ID] {
			seen[ch.ID] = true
			out = append(out, ch)
		}
	}
	return out
}

// IngestMattermost runs a one-shot backfill of recent Mattermost messages.
// Fetches the last 200 posts per channel.
func IngestMattermost(ctx context.Context, es *esearch.Client, cfg *Config) (int, error) {
	if cfg.Mattermost.URL == "" || cfg.Mattermost.Token == "" {
		return 0, fmt.Errorf("mattermost not configured")
	}

	m := &MattermostCollector{
		URL:   cfg.Mattermost.URL,
		Token: cfg.Mattermost.Token,
	}
	m.client = &http.Client{Timeout: 15 * time.Second}

	me, err := m.getMe(ctx)
	if err != nil {
		return 0, fmt.Errorf("mattermost auth: %w", err)
	}
	m.userID = me.ID

	channels, err := m.getMyChannels(ctx)
	if err != nil {
		return 0, err
	}

	total := 0
	// Backfill last 7 days per channel.
	since := time.Now().Add(-7 * 24 * time.Hour).UnixMilli()

	for _, ch := range channels {
		posts, err := m.getPostsSince(ctx, ch.ID, since)
		if err != nil {
			slog.Warn("ingest: mattermost channel", "channel", ch.DisplayName, "err", err)
			continue
		}

		var batch []any
		for _, post := range posts {
			if post.UserID == m.userID || post.Type != "" {
				continue
			}

			eventType := mmEventType(ch.Type, post.Message, me.Username)
			sender := m.resolveUsername(ctx, post.UserID)

			batch = append(batch, esearch.Event{
				Type:   eventType,
				Source: "mattermost",
				Author: sender,
				Message: truncate(post.Message, 500),
				Body:   post.Message,
				Metadata: map[string]any{
					"post_id":      post.ID,
					"channel_id":   ch.ID,
					"channel_name": ch.DisplayName,
					"channel_type": ch.Type,
					"root_id":      post.RootID,
				},
				Timestamp: time.UnixMilli(post.CreateAt),
			})
		}

		if len(batch) > 0 {
			if err := es.BulkIndex(ctx, esearch.IndexEvents, batch); err != nil {
				slog.Warn("ingest: mattermost bulk", "err", err)
				continue
			}
			total += len(batch)
		}
	}

	slog.Info("ingest: mattermost", "channels", len(channels), "docs", total)
	return total, nil
}
