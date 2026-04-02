package cron

import (
	"fmt"
	"strings"
	"time"

	robfigcron "github.com/robfig/cron/v3"
)

// cronParser is the standard 5-field parser shared across the package.
var cronParser = robfigcron.NewParser(
	robfigcron.Minute | robfigcron.Hour | robfigcron.Dom | robfigcron.Month | robfigcron.Dow,
)

// NextRun parses entry.Schedule and returns the next scheduled fire time after
// time.Now(). Returns an error if the schedule expression is invalid.
func NextRun(entry Entry) (time.Time, error) {
	sched, err := cronParser.Parse(entry.Schedule)
	if err != nil {
		return time.Time{}, fmt.Errorf("cron: parse schedule %q: %w", entry.Schedule, err)
	}
	return sched.Next(time.Now()), nil
}

// FormatRelative returns a concise human-readable relative duration from now
// to t, e.g. "in 4m", "in 2h 30m", "in 3d". Returns "now" if t is in the
// past or within 1 second.
func FormatRelative(t time.Time) string {
	d := time.Until(t)
	if d <= time.Second {
		return "now"
	}

	days := int(d.Hours()) / 24
	hours := int(d.Hours()) % 24
	mins := int(d.Minutes()) % 60

	switch {
	case days > 0 && hours > 0:
		return fmt.Sprintf("in %dd %dh", days, hours)
	case days > 0:
		return fmt.Sprintf("in %dd", days)
	case hours > 0 && mins > 0:
		return fmt.Sprintf("in %dh %dm", hours, mins)
	case hours > 0:
		return fmt.Sprintf("in %dh", hours)
	default:
		return fmt.Sprintf("in %dm", max1(mins, 1))
	}
}

// max1 returns the larger of a and b (avoids importing slices for a trivial op).
func max1(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// HumanSchedule converts a standard 5-field cron expression into a concise
// human-readable description. Unrecognised patterns fall back to the raw expr.
//
// Examples:
//
//	"0 * * * *"  → "hourly at :00"
//	"30 * * * *" → "hourly at :30"
//	"0 5 * * *"  → "daily at 05:00"
//	"*/15 * * * *" → "every 15 min"
func HumanSchedule(expr string) string {
	f := strings.Fields(expr)
	if len(f) != 5 {
		return expr
	}
	min, hour, dom, month, dow := f[0], f[1], f[2], f[3], f[4]

	allWild := dom == "*" && month == "*" && dow == "*"

	// every minute
	if min == "*" && hour == "*" && allWild {
		return "every minute"
	}
	// every N minutes  */N * * * *
	if strings.HasPrefix(min, "*/") && hour == "*" && allWild {
		return "every " + strings.TrimPrefix(min, "*/") + " min"
	}
	// hourly at :MM  — fixed minute, wildcard hour
	if isDigits(min) && hour == "*" && allWild {
		return fmt.Sprintf("hourly at :%s", zeroPad(min))
	}
	// daily at HH:MM  — fixed minute + hour, all else wildcard
	if isDigits(min) && isDigits(hour) && allWild {
		return fmt.Sprintf("daily at %02s:%02s", zeroPad(hour), zeroPad(min))
	}
	// weekly: fixed minute + hour + dow
	if isDigits(min) && isDigits(hour) && dom == "*" && month == "*" && isDigits(dow) {
		days := []string{"Sun", "Mon", "Tue", "Wed", "Thu", "Fri", "Sat"}
		d := 0
		fmt.Sscanf(dow, "%d", &d) //nolint:errcheck
		dayName := days[d%7]
		return fmt.Sprintf("%s at %02s:%02s", dayName, zeroPad(hour), zeroPad(min))
	}
	// monthly: fixed minute + hour + dom
	if isDigits(min) && isDigits(hour) && isDigits(dom) && month == "*" && dow == "*" {
		return fmt.Sprintf("monthly day %s at %02s:%02s", dom, zeroPad(hour), zeroPad(min))
	}

	return expr
}

func isDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, c := range s {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

// zeroPad ensures a numeric string is at least 2 chars wide with a leading zero.
func zeroPad(s string) string {
	if len(s) < 2 {
		return "0" + s
	}
	return s
}
