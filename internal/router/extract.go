package router

import (
	"regexp"
	"strings"
)

// extractInput attempts to pull a URL, file path, or named topic from an
// imperative prompt for use as {{param.input}} in the dispatched pipeline.
// Returns "" when no actionable target is found.
//
// Called by the embedding fast-path so pipelines receive their input variable
// even when the LLM stage is skipped.
func extractInput(prompt string) string {
	// URL takes priority — highest signal, unambiguous.
	if m := reURL.FindString(prompt); m != "" {
		return strings.TrimRight(m, ".,'\"")
	}
	// "<verb> <pipeline> on <topic>" or "<verb> <pipeline> for <topic>"
	if m := reOnFor.FindStringSubmatch(prompt); len(m) == 2 {
		v := strings.TrimSpace(m[1])
		if v != "" && !strings.EqualFold(v, "none") {
			return v
		}
	}
	return ""
}

// extractCronPhrase converts common natural-language schedule phrases to a
// 5-field cron expression. Returns "" for anything ambiguous or unrecognized.
// The return value is passed through validateCron before use.
func extractCronPhrase(prompt string) string {
	s := strings.ToLower(strings.TrimSpace(prompt))

	if m := reEveryNHours.FindStringSubmatch(s); len(m) == 2 {
		return "0 */" + m[1] + " * * *"
	}
	if m := reEveryNMinutes.FindStringSubmatch(s); len(m) == 2 {
		return "*/" + m[1] + " * * * *"
	}
	if reEveryDay.MatchString(s) {
		return "0 0 * * *"
	}
	if reEveryHour.MatchString(s) {
		return "0 * * * *"
	}
	if reEveryMorning.MatchString(s) {
		return "0 9 * * *"
	}
	if reWeekdays.MatchString(s) {
		return "0 9 * * 1-5"
	}
	if m := reDayOfWeek.FindStringSubmatch(s); len(m) == 2 {
		if n, ok := dowNumbers[m[1]]; ok {
			return "0 9 * * " + n
		}
	}
	return ""
}

var (
	reURL           = regexp.MustCompile(`https?://\S+`)
	reOnFor         = regexp.MustCompile(`(?i)\b(?:on|for)\s+(.+?)(?:\s+every\b|\s*$)`)
	reEveryNHours   = regexp.MustCompile(`\bevery\s+(\d+)\s+hours?\b`)
	reEveryNMinutes = regexp.MustCompile(`\bevery\s+(\d+)\s+minutes?\b`)
	reEveryDay      = regexp.MustCompile(`\b(?:every\s+day|every\s+\d+\s+days?|daily)\b`)
	reEveryHour     = regexp.MustCompile(`\bevery\s+hour\b`)
	reEveryMorning  = regexp.MustCompile(`\bevery\s+morning\b`)
	reWeekdays      = regexp.MustCompile(`\bevery\s+weekday(?:s)?\b`)
	reDayOfWeek     = regexp.MustCompile(`\bevery\s+(monday|tuesday|wednesday|thursday|friday|saturday|sunday)\b`)
)

var dowNumbers = map[string]string{
	"monday":    "1",
	"tuesday":   "2",
	"wednesday": "3",
	"thursday":  "4",
	"friday":    "5",
	"saturday":  "6",
	"sunday":    "0",
}
