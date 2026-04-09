package research

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"time"
)

// LearnWeights reads the research event log, correlates feedback
// events with their corresponding score events via query_id, and
// derives optimal weights by measuring which signals best predict
// user acceptance. Returns the learned Weights and the number of
// labelled samples used. Returns an error when the event log is
// unreadable or there are too few labelled samples to learn from.
//
// Algorithm: for each of the four signals, compute the mean value
// in accepted samples minus the mean value in rejected samples.
// The resulting "lift" measures how predictive each signal is of
// user acceptance. Lifts are clamped to [0, ∞) (a negative lift
// means the signal is anti-predictive — we zero it rather than
// penalise it) and normalised so they sum to 1.0.
//
// MinSamples is the minimum number of labelled events (accepted +
// rejected combined) required before learning kicks in. Below this
// threshold the function returns ErrTooFewSamples so the caller
// can skip the write without treating it as a real error.
func LearnWeights(eventPath string, minSamples int) (Weights, int, error) {
	if eventPath == "" {
		eventPath = defaultEventPath()
	}
	if minSamples <= 0 {
		minSamples = 10
	}

	data, err := os.ReadFile(eventPath)
	if err != nil {
		return Weights{}, 0, fmt.Errorf("learn weights: read events: %w", err)
	}

	// Pass 1: build feedback index (query_id → accepted bool).
	// Last-write-wins for repeated feedback on the same query.
	verdicts := make(map[string]bool)
	for _, line := range splitLines(data) {
		if len(line) == 0 {
			continue
		}
		var ev Event
		if err := json.Unmarshal(line, &ev); err != nil {
			continue
		}
		if ev.Type != EventTypeFeedback || ev.QueryID == "" {
			continue
		}
		verdicts[ev.QueryID] = ev.Accepted
	}

	if len(verdicts) < minSamples {
		return Weights{}, len(verdicts), ErrTooFewSamples
	}

	// Pass 2: collect per-signal values grouped by verdict.
	// Only score events are needed (they carry the per-signal
	// breakdown without the heavyweight bundle). We take the
	// highest-iteration score per query_id as the final score.
	type signalSet struct {
		sc *float64
		ec *float64
		ca *float64
		js *float64
	}
	best := make(map[string]signalSet) // query_id → best signals
	bestIter := make(map[string]int)

	cutoff := time.Now().Add(-30 * 24 * time.Hour)
	for _, line := range splitLines(data) {
		if len(line) == 0 {
			continue
		}
		var ev Event
		if err := json.Unmarshal(line, &ev); err != nil {
			continue
		}
		if ev.Type != EventTypeScore || ev.QueryID == "" {
			continue
		}
		// Only consider events that have a feedback label.
		if _, ok := verdicts[ev.QueryID]; !ok {
			continue
		}
		// Age filter.
		if t, err := time.Parse(time.RFC3339, ev.Timestamp); err == nil && t.Before(cutoff) {
			continue
		}
		// Keep the highest iteration for each query_id.
		if ev.Iteration <= bestIter[ev.QueryID] {
			if _, exists := best[ev.QueryID]; exists {
				continue
			}
		}
		bestIter[ev.QueryID] = ev.Iteration
		best[ev.QueryID] = signalSet{
			sc: ev.Score.SelfConsistency,
			ec: ev.Score.EvidenceCoverage,
			ca: ev.Score.CrossCapabilityAgree,
			js: ev.Score.JudgeScore,
		}
	}

	// Compute per-signal means for accepted and rejected groups.
	type accum struct {
		sum   float64
		count int
	}
	var (
		accSC, rejSC accum
		accEC, rejEC accum
		accCA, rejCA accum
		accJS, rejJS accum
		totalLabelled int
	)
	for qid, ss := range best {
		accepted := verdicts[qid]
		totalLabelled++
		if ss.sc != nil {
			if accepted {
				accSC.sum += *ss.sc
				accSC.count++
			} else {
				rejSC.sum += *ss.sc
				rejSC.count++
			}
		}
		if ss.ec != nil {
			if accepted {
				accEC.sum += *ss.ec
				accEC.count++
			} else {
				rejEC.sum += *ss.ec
				rejEC.count++
			}
		}
		if ss.ca != nil {
			if accepted {
				accCA.sum += *ss.ca
				accCA.count++
			} else {
				rejCA.sum += *ss.ca
				rejCA.count++
			}
		}
		if ss.js != nil {
			if accepted {
				accJS.sum += *ss.js
				accJS.count++
			} else {
				rejJS.sum += *ss.js
				rejJS.count++
			}
		}
	}

	if totalLabelled < minSamples {
		return Weights{}, totalLabelled, ErrTooFewSamples
	}

	// Compute lift = mean(accepted) - mean(rejected) for each signal.
	// Clamp negative lifts to 0 (anti-predictive signals get zeroed).
	lift := func(acc, rej accum) float64 {
		if acc.count == 0 && rej.count == 0 {
			return 0
		}
		meanAcc := 0.0
		if acc.count > 0 {
			meanAcc = acc.sum / float64(acc.count)
		}
		meanRej := 0.0
		if rej.count > 0 {
			meanRej = rej.sum / float64(rej.count)
		}
		return math.Max(0, meanAcc-meanRej)
	}

	scLift := lift(accSC, rejSC)
	ecLift := lift(accEC, rejEC)
	caLift := lift(accCA, rejCA)
	jsLift := lift(accJS, rejJS)

	total := scLift + ecLift + caLift + jsLift
	if total == 0 {
		// No signal differentiates — fall back to equal weights.
		return Weights{
			SelfConsistency:          0.25,
			EvidenceCoverage:         0.25,
			CrossCapabilityAgreement: 0.25,
			JudgeScore:               0.25,
		}, totalLabelled, nil
	}

	// Normalise so weights sum to 1.0. Round to 2 decimal places
	// for clean YAML output.
	round2 := func(v float64) float64 {
		return math.Round(v*100) / 100
	}

	w := Weights{
		SelfConsistency:          round2(scLift / total),
		EvidenceCoverage:         round2(ecLift / total),
		CrossCapabilityAgreement: round2(caLift / total),
		JudgeScore:               round2(jsLift / total),
	}

	return w, totalLabelled, nil
}

// ErrTooFewSamples is returned by LearnWeights when there are not
// enough labelled events (feedback + score pairs) to derive
// meaningful weights.
var ErrTooFewSamples = fmt.Errorf("too few labelled samples to learn weights")
