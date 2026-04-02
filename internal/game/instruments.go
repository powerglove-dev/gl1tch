package game

import (
	"context"
	"sync"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

var (
	instrOnce      sync.Once
	xpCounter      metric.Int64Counter
	iceCounter     metric.Int64Counter
	achieveCounter metric.Int64Counter
	tunerCounter   metric.Int64Counter
	levelGauge     metric.Int64Gauge
	streakGauge    metric.Int64Gauge
)

func ensureInstruments() {
	instrOnce.Do(func() {
		m := otel.GetMeterProvider().Meter("gl1tch/game")
		xpCounter, _ = m.Int64Counter("game.xp.earned",
			metric.WithDescription("XP earned per run"))
		iceCounter, _ = m.Int64Counter("game.ice.encountered",
			metric.WithDescription("ICE encounters triggered"))
		achieveCounter, _ = m.Int64Counter("game.achievement.unlocked",
			metric.WithDescription("Achievements unlocked"))
		tunerCounter, _ = m.Int64Counter("game.tuner.invoked",
			metric.WithDescription("Pack tuner invocations"))
		levelGauge, _ = m.Int64Gauge("game.level",
			metric.WithDescription("Current player level"))
		streakGauge, _ = m.Int64Gauge("game.streak_days",
			metric.WithDescription("Current streak in days"))
	})
}

// RecordXP records XP metrics after a run is scored.
func RecordXP(ctx context.Context, result XPResult, payload GameRunScoredPayload) {
	ensureInstruments()
	xpCounter.Add(ctx, result.Final,
		metric.WithAttributes(
			attribute.String("provider", payload.Usage.Provider),
			attribute.Int64("xp.base", result.Base),
			attribute.Int64("xp.cache_bonus", result.CacheBonus),
			attribute.Int64("xp.speed_bonus", result.SpeedBonus),
			attribute.Int64("xp.retry_penalty", result.RetryPenalty),
		))
	levelGauge.Record(ctx, int64(payload.Level),
		metric.WithAttributes(attribute.String("provider", payload.Usage.Provider)))
	streakGauge.Record(ctx, int64(payload.StreakDays))
}

// RecordICE records a triggered ICE encounter.
func RecordICE(ctx context.Context, iceClass string) {
	ensureInstruments()
	iceCounter.Add(ctx, 1,
		metric.WithAttributes(attribute.String("ice_class", iceClass)))
}

// RecordAchievement records an achievement unlock.
func RecordAchievement(ctx context.Context, achievementID string) {
	ensureInstruments()
	achieveCounter.Add(ctx, 1,
		metric.WithAttributes(attribute.String("achievement_id", achievementID)))
}

// RecordTunerInvoked records a pack tuner invocation.
func RecordTunerInvoked(ctx context.Context, trigger string) {
	ensureInstruments()
	tunerCounter.Add(ctx, 1,
		metric.WithAttributes(attribute.String("trigger", trigger)))
}
