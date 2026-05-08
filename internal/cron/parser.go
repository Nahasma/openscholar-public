package cron

import (
	"fmt"
	"time"

	"github.com/robfig/cron/v3"
)

// cronParser supports standard 5-field expressions as well as the optional
// leading seconds field (6 fields). Descriptors like @daily are also accepted.
var cronParser = cron.NewParser(
	cron.SecondOptional | cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow | cron.Descriptor,
)

// NextRunTime calculates the next scheduled time after from for the given spec.
func NextRunTime(spec string, from time.Time) (time.Time, error) {
	schedule, err := cronParser.Parse(spec)
	if err != nil {
		return time.Time{}, fmt.Errorf("cron parse %q: %w", spec, err)
	}
	return schedule.Next(from), nil
}

// ValidateSpec checks whether spec is a valid cron expression.
func ValidateSpec(spec string) error {
	if _, err := cronParser.Parse(spec); err != nil {
		return fmt.Errorf("invalid cron spec %q: %w", spec, err)
	}
	return nil
}
