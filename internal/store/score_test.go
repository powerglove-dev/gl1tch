package store

import (
	"context"
	"path/filepath"
	"testing"
)

func TestRecordScoreEvent(t *testing.T) {
	s, err := OpenAt(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("OpenAt: %v", err)
	}
	defer s.Close()

	ctx := context.Background()
	e := ScoreEvent{
		RunID:               42,
		XP:                  100,
		InputTokens:         1000,
		OutputTokens:        200,
		CacheReadTokens:     500,
		CacheCreationTokens: 300,
		CostUSD:             0.05,
		Provider:            "providers.claude",
		Model:               "claude-sonnet-4-6",
		CreatedAt:           1000000,
	}
	if err := s.RecordScoreEvent(ctx, e); err != nil {
		t.Fatalf("RecordScoreEvent: %v", err)
	}

	// Verify a second insert works fine.
	e2 := e
	e2.XP = 50
	if err := s.RecordScoreEvent(ctx, e2); err != nil {
		t.Fatalf("RecordScoreEvent second: %v", err)
	}
}

func TestGetUserScore_InitialState(t *testing.T) {
	s, err := OpenAt(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("OpenAt: %v", err)
	}
	defer s.Close()

	ctx := context.Background()
	us, err := s.GetUserScore(ctx)
	if err != nil {
		t.Fatalf("GetUserScore: %v", err)
	}
	if us.TotalXP != 0 {
		t.Errorf("initial TotalXP = %d, want 0", us.TotalXP)
	}
	if us.Level != 1 {
		t.Errorf("initial Level = %d, want 1", us.Level)
	}
	if us.StreakDays != 0 {
		t.Errorf("initial StreakDays = %d, want 0", us.StreakDays)
	}
	if us.TotalRuns != 0 {
		t.Errorf("initial TotalRuns = %d, want 0", us.TotalRuns)
	}
}

func TestUpdateUserScore(t *testing.T) {
	s, err := OpenAt(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("OpenAt: %v", err)
	}
	defer s.Close()

	ctx := context.Background()

	// Ensure row exists.
	_, err = s.GetUserScore(ctx)
	if err != nil {
		t.Fatalf("GetUserScore: %v", err)
	}

	updated := UserScore{
		TotalXP:     1500,
		Level:       3,
		StreakDays:  5,
		LastRunDate: "2026-04-01",
		TotalRuns:   10,
	}
	if err := s.UpdateUserScore(ctx, updated); err != nil {
		t.Fatalf("UpdateUserScore: %v", err)
	}

	got, err := s.GetUserScore(ctx)
	if err != nil {
		t.Fatalf("GetUserScore after update: %v", err)
	}
	if got.TotalXP != 1500 {
		t.Errorf("TotalXP = %d, want 1500", got.TotalXP)
	}
	if got.Level != 3 {
		t.Errorf("Level = %d, want 3", got.Level)
	}
	if got.StreakDays != 5 {
		t.Errorf("StreakDays = %d, want 5", got.StreakDays)
	}
	if got.LastRunDate != "2026-04-01" {
		t.Errorf("LastRunDate = %q, want %q", got.LastRunDate, "2026-04-01")
	}
	if got.TotalRuns != 10 {
		t.Errorf("TotalRuns = %d, want 10", got.TotalRuns)
	}
}

func TestRecordAchievement_Idempotent(t *testing.T) {
	s, err := OpenAt(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("OpenAt: %v", err)
	}
	defer s.Close()

	ctx := context.Background()
	id := "ghost-runner"

	// First call.
	if err := s.RecordAchievement(ctx, id); err != nil {
		t.Fatalf("RecordAchievement first: %v", err)
	}
	// Second call must not error (INSERT OR IGNORE).
	if err := s.RecordAchievement(ctx, id); err != nil {
		t.Fatalf("RecordAchievement second: %v", err)
	}
}

func TestGetUnlockedAchievements(t *testing.T) {
	s, err := OpenAt(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("OpenAt: %v", err)
	}
	defer s.Close()

	ctx := context.Background()

	// Empty initially.
	ids, err := s.GetUnlockedAchievements(ctx)
	if err != nil {
		t.Fatalf("GetUnlockedAchievements empty: %v", err)
	}
	if len(ids) != 0 {
		t.Errorf("expected 0 achievements, got %d", len(ids))
	}

	// Add two.
	for _, id := range []string{"ghost-runner", "cache-warlock"} {
		if err := s.RecordAchievement(ctx, id); err != nil {
			t.Fatalf("RecordAchievement %s: %v", id, err)
		}
	}
	// Duplicate — should not add.
	if err := s.RecordAchievement(ctx, "ghost-runner"); err != nil {
		t.Fatalf("RecordAchievement duplicate: %v", err)
	}

	ids, err = s.GetUnlockedAchievements(ctx)
	if err != nil {
		t.Fatalf("GetUnlockedAchievements: %v", err)
	}
	if len(ids) != 2 {
		t.Errorf("expected 2 achievements, got %d: %v", len(ids), ids)
	}
}

func TestScoreEventsByProvider(t *testing.T) {
	s, err := OpenAt(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("OpenAt: %v", err)
	}
	defer s.Close()

	ctx := context.Background()

	// Empty case.
	result, err := s.ScoreEventsByProvider(ctx)
	if err != nil {
		t.Fatalf("ScoreEventsByProvider empty: %v", err)
	}
	if len(result) != 0 {
		t.Errorf("expected empty map, got %d entries", len(result))
	}

	// Insert events for two providers.
	events := []ScoreEvent{
		{RunID: 1, XP: 100, Provider: "providers.claude", CreatedAt: 1},
		{RunID: 2, XP: 200, Provider: "providers.claude", CreatedAt: 2},
		{RunID: 3, XP: 50, Provider: "providers.codex", CreatedAt: 3},
	}
	for _, e := range events {
		if err := s.RecordScoreEvent(ctx, e); err != nil {
			t.Fatalf("RecordScoreEvent: %v", err)
		}
	}

	result, err = s.ScoreEventsByProvider(ctx)
	if err != nil {
		t.Fatalf("ScoreEventsByProvider: %v", err)
	}
	if len(result) != 2 {
		t.Fatalf("expected 2 providers, got %d", len(result))
	}
	claude := result["providers.claude"]
	if claude.TotalXP != 300 {
		t.Errorf("claude TotalXP = %d, want 300", claude.TotalXP)
	}
	if claude.TotalRuns != 2 {
		t.Errorf("claude TotalRuns = %d, want 2", claude.TotalRuns)
	}
	codex := result["providers.codex"]
	if codex.TotalXP != 50 {
		t.Errorf("codex TotalXP = %d, want 50", codex.TotalXP)
	}
	if codex.TotalRuns != 1 {
		t.Errorf("codex TotalRuns = %d, want 1", codex.TotalRuns)
	}
}
