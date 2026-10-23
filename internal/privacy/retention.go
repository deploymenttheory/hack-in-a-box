package privacy

import "time"

// RetentionDeadline matches the native SQL privacy deadline: calendar days in
// the database session's canonical UTC timezone, independent of scan location.
func RetentionDeadline(completed time.Time, days int32) time.Time {
	return completed.UTC().AddDate(0, 0, int(days))
}
