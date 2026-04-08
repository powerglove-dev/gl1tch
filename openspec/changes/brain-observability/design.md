## Context

The brain currently produces answers and persists events but does not summarize itself. There is no honest answer to "is glitch getting smarter?" Once `glitch-research-loop` lands, every answer will emit a per-signal score event — that gives us the raw data. `chat-first-ui` gives us the rendering surface (`score_card`, `widget_card`, `action_chips`). This change is the analytics layer in the middle: turning raw events into a small set of carefully chosen metrics, surfacing them in chat where the user lives, letting the user tune the brain from the same surface, and capturing the feedback signals needed to eventually learn the composite confidence weights.

The biggest decision in this change is *which metrics*, not how to compute them. The user explicitly said "stats for everything" — but the right answer is the opposite: pick 3–5 metrics that actually predict "glitch is getting smarter," and refuse to ship vanity numbers.

## Goals / Non-Goals

**Goals:**
- A small, defensible set of metrics that measure brain quality: `accept_rate`, `confidence_calibration`, `retrieval_precision`, `iteration_count`, `escalation_rate`.
- A stats engine that can answer "what is metric X right now and over the last 30 days" without re-scanning every event.
- `/brain stats` returns a `score_card` per metric in chat.
- `/brain config` lets the user tune brain knobs from chat, and every edit is logged so we can correlate config changes with metric movement.
- Every assistant answer is followed by 2–3 next-prompt chips that nudge the user toward the next useful question.
- Explicit (thumbs) and implicit (contradicting-follow-up) feedback is captured as events, ready to drive weight learning later.

**Non-Goals:**
- No machine-learned weights for the composite confidence score in this change. We *capture* the signal; learning is a follow-up.
- No multi-user analytics. glitch is single-user.
- No external analytics service or dashboard outside of chat.
- No per-researcher quality scoring until we have enough events to be statistically meaningful (defer to a follow-up after a few weeks of dogfooding).
- No vanity metrics like "total questions asked," "tokens used today," or "uptime" — interesting maybe, but not signals of getting smarter.
- No retroactive computation for events emitted before this change ships.

## Decisions

### Decision 1: Five metrics, defensible reasons each

| Metric | What it measures | Why it matters |
|---|---|---|
| `accept_rate` | Fraction of assistant answers the user accepted (explicit thumbs-up *or* did not contradict in the next 5 min) | Direct proxy for "did this answer actually help?" |
| `confidence_calibration` | Brier score of stated composite confidence vs. accept outcome | Tells us whether "85% confident" actually means 85% of the time accepted. The whole loop is built on a confidence threshold; if the threshold is dishonest, everything downstream is broken. |
| `retrieval_precision` | Fraction of researcher evidence items cited in the final accepted draft | Tells us whether the loop is gathering useful evidence or just noise. Trends down → the planner is over-fetching. |
| `iteration_count` | Average research-loop iterations per accepted answer | Should trend down as the brain learns which evidence to ask for first. A flat line means the planner isn't learning. |
| `escalation_rate` | Fraction of accepted answers that required paid-model escalation | Should trend down as local capability grows. The whole point of the loop is to make this number small. |

**Why these and not others:** every metric has a clear "smarter" direction (up for accept and calibration, down for the others) and a clear computation from existing events. Anything that doesn't have both is out.

**Alternative considered:** include `time_to_answer` and `tokens_per_answer`. Rejected for v1 — they're cost metrics, not quality metrics. They belong on a separate cost dashboard if we ever need one, not next to quality.

### Decision 2: Computation is event-driven, results are cached as time-series

The stats engine is a consumer of the brain event store. It maintains rolling daily buckets for the last 30 days and weekly buckets beyond that, per metric, in a small sidecar table or JSON file. On every relevant event (`research_score`, `research_escalation`, `brain_feedback`), the engine updates the affected bucket. `/brain stats` reads from the cached series, never from raw events.

**Why:** raw event scans get expensive fast, and the user will run `/brain stats` interactively. Cached series are cheap to read and cheap to update incrementally.

**Alternative considered:** compute on demand from raw events. Rejected — performance cliff once the event store grows.

### Decision 3: Brier score for calibration

Brier = mean((stated_confidence - accept_outcome)²). Lower is better. This is the standard probabilistic-forecast metric and matches exactly what we want: "when you say 85%, are you right 85% of the time?"

We display Brier directly (it's a number between 0 and 1, where 0 is perfect and 0.25 is "no better than guessing 50/50"). A reliability diagram would be nicer but is bigger than a `score_card`; defer to a follow-up if Brier alone isn't actionable.

### Decision 4: Implicit feedback = "did the user run a contradicting follow-up within 5 minutes"

Defining "contradicting" is the hard part. v1 rule: if within 5 minutes of an assistant answer the user runs another `glitch ask` whose qwen2.5:7b-classified intent is "rephrase", "challenge", "correct", or "ignore previous", we count the original answer as not-accepted. Otherwise, absent an explicit thumbs-down, we count it as accepted.

**Why:** explicit feedback is sparse; implicit is plentiful. The classifier is cheap (one local call per follow-up) and AI-first per the project rule (no keyword tables).

**Alternative considered:** only count explicit feedback. Rejected — too sparse to drive any of the metrics in a useful timeframe.

### Decision 5: Next-prompt chips are a cheap second pass, not a separate loop call

After every assistant answer, the chip generator runs one qwen2.5:7b call: "given the question and answer, suggest 2–3 short follow-up prompts the user might want to run next." Output is parsed into an `action_chips` payload and rendered under the answer.

**Why:** one extra cheap call is acceptable per answer; running the full research loop a second time is not. Chips are a nudge, not an oracle.

### Decision 6: `/brain config` writes are logged as events, not just config diffs

Every edit to a brain or research-loop knob via the config widget emits a `brain_config_change` event with the field name, old value, new value, and timestamp. The event is what lets us correlate "I lowered the threshold to 0.8 on day 12" with "accept rate jumped on day 13."

**Why:** without the events, we can't tell whether a metric movement was caused by config changes or by the brain learning. With them, the stats engine can annotate the time-series.

### Decision 7: Retention — score breakdowns are tiny, full bundles are not

- `research_score` events (small): kept indefinitely.
- `brain_feedback` events (tiny): kept indefinitely.
- `brain_config_change` events (tiny): kept indefinitely.
- `research_attempt` events with full evidence bundles (potentially large): kept 30 days, then payload truncated to a hash.

**Why:** the signals we need for weight learning are the small events. The big payloads are for debugging individual answers, which is a recent-history concern.

### Decision 8: Stats engine lives in `internal/brainaudit`, not a new package

`internal/brainaudit` already owns brain event consumption. Adding the stats engine here keeps the consumer model in one place. The API is small: `Stats.Get(metric, range) → series`, `Stats.Update(event)`.

## Risks / Trade-offs

- **[Risk] The five metrics are wrong** → Mitigation: every metric is computed from logged events, so we can re-derive a different metric retroactively without changing the schema. If a metric turns out to be unactionable after a few weeks of use, replace it.
- **[Risk] Implicit feedback classifier mislabels follow-ups** → Mitigation: log the classifier's verdict alongside the feedback event so we can audit and re-classify later. Don't make the classifier the only signal — explicit thumbs are still recorded separately.
- **[Risk] Brier score is hard to interpret** → Mitigation: `/brain stats` renders Brier with a one-line interpretation ("0.12 — calibration is good") and a sparkline so trend matters more than absolute value.
- **[Risk] Config edits via widget could let the user enter invalid values** → Mitigation: the `widget_card` for config uses constrained inputs (min/max for numbers, picker for enums) and validates server-side before emitting the change event.
- **[Risk] Chip generator adds latency to every answer** → Mitigation: chips render asynchronously after the answer is shown; the answer is not blocked on chip generation. If the user clicks a chip before chips render, no problem — they're just typing.
- **[Risk] Retention of `research_attempt` payloads at 30d may not be long enough for debugging** → Accepted. We can extend it; storage cost is the only concern.
- **[Trade-off] No machine-learned weights yet** → Accepted. The point is to start collecting the data correctly. Premature weight learning would mean making decisions without enough samples.
- **[Trade-off] Reliability diagram deferred** → Accepted. Brier is enough for v1; diagram is a follow-up if calibration becomes the focus of tuning.

## Migration Plan

No data migration. Rollout:

1. Land event schema additions (`brain_feedback`, `brain_config_change`) in `internal/brain` — additive, no schema change.
2. Land the stats engine in `internal/brainaudit` and start consuming events from `glitch-research-loop`.
3. Land `/brain stats` once at least one metric has a few days of data to render.
4. Land `/brain config` widget and wire it through the slash dispatcher from `chat-first-ui`.
5. Land the next-prompt chip generator and wire it into the assistant's answer flow.
6. Land the explicit thumbs-up/down controls on assistant messages and the implicit contradicting-follow-up classifier.
7. Run for two weeks, audit which metrics are moving, and write a follow-up proposal for weight learning if and when there's enough signal.

**Rollback:** every step is additive. The widgets and slash commands can be removed without touching event emission; the stats engine can be paused without losing anything because events are still in the store.

## Open Questions

- **OQ1: What window is "rolling" for accept_rate — last 7 days, last 30, last N answers?** Lean: 7-day rolling for the headline number, with the 30-day sparkline for trend.
- **OQ2: Should config edits be confirmable (preview the change, click confirm)?** Lean: yes for threshold and budgets, no for enable/disable toggles.
- **OQ3: How do we attribute an answer to the *current* config when config has just changed?** Lean: every `research_score` event embeds the config hash at the time of emission, so we can join later without ambiguity.
- **OQ4: Should next-prompt chips ever include destructive actions (like "delete this file")?** No — chips are read-only or research prompts; never destructive. Add this as a hard rule in the chip generator system prompt.
- **OQ5: Where does the implicit feedback classifier live — inline in `internal/assistant`, or as its own researcher?** Lean: inline in assistant, because it's an audit pass on conversation flow, not an information-gathering step.
