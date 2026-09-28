package service

import "time"

// TMDbSeasonCooldown follows the season's own UTC air dates, including upcoming
// episodes. A different season of the same series never changes its cadence.
func TMDbSeasonCooldown(releaseDate string, episodeDates []string, now time.Time) time.Duration {
	today := now.UTC().Truncate(24 * time.Hour)
	latest := time.Time{}
	for _, value := range episodeDates {
		date, err := time.Parse(time.DateOnly, value)
		if err != nil {
			continue
		}
		if !date.Before(today.AddDate(0, 0, -30)) && !date.After(today.AddDate(0, 0, 30)) {
			return 24 * time.Hour
		}
		if !date.After(today) && date.After(latest) {
			latest = date
		}
	}
	if latest.IsZero() {
		latest, _ = time.Parse(time.DateOnly, releaseDate)
	}
	switch {
	case latest.IsZero(), latest.After(today.AddDate(0, 0, 30)):
		return 3 * 24 * time.Hour
	case !latest.Before(today.AddDate(0, 0, -30)):
		return 24 * time.Hour
	case !latest.Before(today.AddDate(0, 0, -365)):
		return 10 * 24 * time.Hour
	default:
		return 20 * 24 * time.Hour
	}
}
