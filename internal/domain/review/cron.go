package review

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// CronSchedule represents a minimal parsed cron schedule.
type CronSchedule struct {
	Hour    int
	Minute  int
	Weekday *time.Weekday // nil = daily, non-nil = specific weekday
}

// ParseCronExpr parses a simple cron expression.
// Supported formats:
//   - "daily"             → every day at 00:00 UTC
//   - "weekly"            → every Monday at 00:00 UTC
//   - "HH:MM"            → every day at HH:MM UTC
//   - "daily:HH:MM"      → every day at HH:MM UTC
//   - "weekly:Day"        → every Day at 00:00 UTC (e.g. "weekly:Fri")
//   - "weekly:Day:HH:MM"  → every Day at HH:MM UTC
func ParseCronExpr(expr string) (CronSchedule, error) {
	expr = strings.TrimSpace(expr)
	if expr == "" {
		return CronSchedule{}, fmt.Errorf("empty cron expression")
	}

	switch {
	case expr == "daily":
		return CronSchedule{Hour: 0, Minute: 0}, nil

	case expr == "weekly":
		mon := time.Monday
		return CronSchedule{Hour: 0, Minute: 0, Weekday: &mon}, nil

	case strings.HasPrefix(expr, "daily:"):
		h, m, err := parseHHMM(strings.TrimPrefix(expr, "daily:"))
		if err != nil {
			return CronSchedule{}, err
		}
		return CronSchedule{Hour: h, Minute: m}, nil

	case strings.HasPrefix(expr, "weekly:"):
		rest := strings.TrimPrefix(expr, "weekly:")
		parts := strings.SplitN(rest, ":", 2)
		day, err := parseWeekday(parts[0])
		if err != nil {
			return CronSchedule{}, err
		}
		h, m := 0, 0
		if len(parts) == 2 {
			h, m, err = parseHHMM(parts[1])
			if err != nil {
				return CronSchedule{}, err
			}
		}
		return CronSchedule{Hour: h, Minute: m, Weekday: &day}, nil

	default:
		// Try HH:MM
		h, m, err := parseHHMM(expr)
		if err != nil {
			return CronSchedule{}, fmt.Errorf("unrecognized cron expression: %q", expr)
		}
		return CronSchedule{Hour: h, Minute: m}, nil
	}
}

// NextAfter returns the next occurrence of this schedule after the given time.
func (c CronSchedule) NextAfter(t time.Time) time.Time {
	t = t.UTC()

	// Start from the target time today
	candidate := time.Date(t.Year(), t.Month(), t.Day(), c.Hour, c.Minute, 0, 0, time.UTC)

	if c.Weekday == nil {
		// Daily: if today's slot has passed, move to tomorrow
		if !candidate.After(t) {
			candidate = candidate.AddDate(0, 0, 1)
		}
		return candidate
	}

	// Weekly: find the next matching weekday
	for i := range 8 {
		check := candidate.AddDate(0, 0, i)
		if check.Weekday() == *c.Weekday && check.After(t) {
			return check
		}
	}

	// Should not reach here, but just in case
	return candidate.AddDate(0, 0, 7)
}

// ValidateCronExpr checks if a cron expression is syntactically valid.
func ValidateCronExpr(expr string) error {
	_, err := ParseCronExpr(expr)
	return err
}

func parseHHMM(s string) (hour, minute int, err error) {
	parts := strings.SplitN(s, ":", 2)
	if len(parts) != 2 {
		return 0, 0, fmt.Errorf("expected HH:MM, got %q", s)
	}
	h, err := strconv.Atoi(parts[0])
	if err != nil || h < 0 || h > 23 {
		return 0, 0, fmt.Errorf("invalid hour %q", parts[0])
	}
	m, err := strconv.Atoi(parts[1])
	if err != nil || m < 0 || m > 59 {
		return 0, 0, fmt.Errorf("invalid minute %q", parts[1])
	}
	return h, m, nil
}

func parseWeekday(s string) (time.Weekday, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "sun", "sunday":
		return time.Sunday, nil
	case "mon", "monday":
		return time.Monday, nil
	case "tue", "tuesday":
		return time.Tuesday, nil
	case "wed", "wednesday":
		return time.Wednesday, nil
	case "thu", "thursday":
		return time.Thursday, nil
	case "fri", "friday":
		return time.Friday, nil
	case "sat", "saturday":
		return time.Saturday, nil
	default:
		return 0, fmt.Errorf("unknown weekday %q", s)
	}
}
