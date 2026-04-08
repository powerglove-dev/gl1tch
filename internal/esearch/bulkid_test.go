package esearch

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

// TestBulkEventID_CollectorContract pins the contract between the
// github/git collectors and bulkEventID's switch statement. The
// type strings in the switch MUST match whatever the collector
// actually writes into esearch.Event.Type, otherwise BulkIndex
// falls through to an auto-generated _id and every collector poll
// duplicates the same event.
//
// This test encodes each event type as the collector produces it
// and asserts bulkEventID returns a non-empty, stable id. A silent
// rename on either side will fail here loudly instead of
// re-appearing as the "why is #1246 listed six times?" bug we
// already shipped once.
func TestBulkEventID_CollectorContract(t *testing.T) {
	// Stable timestamps so the comment/review/check branches
	// produce deterministic hash suffixes we can assert on.
	ts := time.Date(2026, 4, 8, 11, 45, 0, 0, time.UTC)

	cases := []struct {
		name       string
		event      Event
		wantPrefix string // id must start with this
		wantNonEmpty bool // id must be non-empty (for cases where
		                  // the suffix is a hash we don't pin)
	}{
		{
			name: "git.commit uses sha",
			event: Event{
				Type: "git.commit",
				SHA:  "abc1234567890",
			},
			wantPrefix: "git.commit:abc1234567890",
		},
		{
			name: "git.push uses sha",
			event: Event{
				Type: "git.push",
				SHA:  "deadbeef",
			},
			wantPrefix: "git.push:deadbeef",
		},
		{
			name: "github.pr uses metadata.url — NOT github.pullrequest",
			event: Event{
				Type: "github.pr",
				Metadata: map[string]any{
					"url": "https://github.com/elastic/ensemble/pull/1246",
				},
			},
			wantPrefix: "github.pr:https://github.com/elastic/ensemble/pull/1246",
		},
		{
			name: "github.issue uses metadata.url",
			event: Event{
				Type: "github.issue",
				Metadata: map[string]any{
					"url": "https://github.com/elastic/ensemble/issues/42",
				},
			},
			wantPrefix: "github.issue:https://github.com/elastic/ensemble/issues/42",
		},
		{
			name: "github.issue_comment hashes author+body+timestamp",
			event: Event{
				Type:      "github.issue_comment",
				Repo:      "elastic/ensemble",
				Author:    "someone",
				Body:      "LGTM",
				Timestamp: ts,
				Metadata: map[string]any{
					"issue_number": 42,
				},
			},
			wantPrefix:   "github.issue_comment:elastic/ensemble:42:",
			wantNonEmpty: true,
		},
		{
			name: "github.pr_comment hashes author+body+timestamp",
			event: Event{
				Type:      "github.pr_comment",
				Repo:      "elastic/ensemble",
				Author:    "someone",
				Body:      "nit",
				Timestamp: ts,
				Metadata: map[string]any{
					"pr_number": 1246,
				},
			},
			wantPrefix:   "github.pr_comment:elastic/ensemble:1246:",
			wantNonEmpty: true,
		},
		{
			name: "github.pr_review hashes author+body+timestamp",
			event: Event{
				Type:      "github.pr_review",
				Repo:      "elastic/ensemble",
				Author:    "amannocci",
				Body:      "left some comments",
				Timestamp: ts,
				Metadata: map[string]any{
					"pr_number": 1246,
				},
			},
			wantPrefix:   "github.pr_review:elastic/ensemble:1246:",
			wantNonEmpty: true,
		},
		{
			name: "github.check uses pr_number hash — NOT github.pr_check",
			event: Event{
				Type:      "github.check",
				Repo:      "elastic/ensemble",
				Author:    "ci",
				Body:      "smoke test failed",
				Timestamp: ts,
				Metadata: map[string]any{
					"pr_number": 1246,
				},
			},
			wantPrefix:   "github.check:elastic/ensemble:1246:",
			wantNonEmpty: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			raw, err := json.Marshal(tc.event)
			if err != nil {
				t.Fatalf("marshal: %v", err)
			}
			got := bulkEventID(raw)
			if got == "" {
				t.Fatalf("empty id — collector type %q no longer matches the switch in bulkEventID. "+
					"If you renamed the event type, update client.go's switch AND this test in lockstep.",
					tc.event.Type)
			}
			if !strings.HasPrefix(got, tc.wantPrefix) {
				t.Errorf("id prefix mismatch for %q:\n  got  %q\n  want prefix %q",
					tc.event.Type, got, tc.wantPrefix)
			}
			if tc.wantNonEmpty && len(got) <= len(tc.wantPrefix) {
				t.Errorf("id for %q should have a hash suffix, got %q", tc.event.Type, got)
			}
		})
	}
}

// TestBulkEventID_UnknownTypeFallsThrough documents that novel
// event types return an empty id so BulkIndex falls back to ES
// auto-generated ids. The caller-side behavior (non-idempotent
// writes for new types) is intentional per the comment on
// bulkEventID — we'd rather have duplicates than accidentally
// overwrite a new event type we haven't thought about.
func TestBulkEventID_UnknownTypeFallsThrough(t *testing.T) {
	raw, _ := json.Marshal(Event{Type: "some.future.type", SHA: "x"})
	if got := bulkEventID(raw); got != "" {
		t.Errorf("unknown type should return empty id, got %q", got)
	}
}

// TestBulkEventID_StableAcrossPolls simulates two collector polls
// seeing the same PR and asserts the id is identical. This is the
// direct assertion that the duplication bug cannot come back via
// an unstable hash (e.g. if someone adds time.Now() to the inputs).
func TestBulkEventID_StableAcrossPolls(t *testing.T) {
	ts := time.Date(2026, 4, 8, 11, 45, 0, 0, time.UTC)
	ev := Event{
		Type: "github.pr",
		Metadata: map[string]any{
			"url": "https://github.com/elastic/ensemble/pull/1246",
		},
		Timestamp: ts,
	}
	raw1, _ := json.Marshal(ev)
	raw2, _ := json.Marshal(ev)
	id1 := bulkEventID(raw1)
	id2 := bulkEventID(raw2)
	if id1 != id2 {
		t.Errorf("id drifted between identical polls:\n  first: %q\n  second: %q", id1, id2)
	}
	if id1 == "" {
		t.Errorf("stable id should be non-empty")
	}
}
