package render

import (
	"math"
	"strconv"
	"time"

	"github.com/appifyhub/agent-cli/internal/text"
)

// LocalTime formats a timestamp in the user's time zone.
func LocalTime(target time.Time, location *time.Location) string {
	return target.In(location).Format("2006-01-02 15:04")
}

// RelativeTime describes the distance to target using elapsed minutes, hours and days,
// then calendar months with month-end clamping.
func RelativeTime(target, now time.Time, cat *text.Catalog) string {
	past := target.Before(now)
	earlier, later := now, target.In(now.Location())
	if past {
		earlier, later = later, earlier
	}
	distance := later.Sub(earlier)
	var value string
	switch {
	case distance < time.Minute:
		value = cat.T("time.less_than_minute")
	case distance < time.Hour:
		value = relativeUnit(cat, "minute", int(math.Round(distance.Minutes())))
	case distance < 24*time.Hour:
		value = relativeUnit(cat, "hour", int(math.Round(distance.Hours())))
	case distance < 30*24*time.Hour:
		value = relativeUnit(cat, "day", int(math.Round(distance.Hours()/24)))
	default:
		months := (later.Year()-earlier.Year())*12 + int(later.Month()-earlier.Month())
		anchor := calendarMonth(earlier, months)
		if anchor.After(later) {
			months--
			anchor = calendarMonth(earlier, months)
		}
		next := calendarMonth(earlier, months+1)
		if later.Sub(anchor) >= next.Sub(anchor)/2 {
			months++
		}
		if months >= 12 && months%12 == 0 {
			value = relativeUnit(cat, "year", months/12)
		} else {
			value = relativeUnit(cat, "month", months)
		}
	}
	if past {
		return cat.T("time.past", "value", value)
	}
	return cat.T("time.future", "value", value)
}

func relativeUnit(cat *text.Catalog, unit string, count int) string {
	if count == 1 {
		return cat.T("time." + unit)
	}
	return cat.T("time."+unit+"s", "count", strconv.Itoa(count))
}

func calendarMonth(t time.Time, months int) time.Time {
	first := time.Date(t.Year(), t.Month()+time.Month(months), 1, t.Hour(), t.Minute(), t.Second(), t.Nanosecond(), t.Location())
	lastDay := time.Date(first.Year(), first.Month()+1, 0, 0, 0, 0, 0, t.Location()).Day()
	return time.Date(first.Year(), first.Month(), min(t.Day(), lastDay), t.Hour(), t.Minute(), t.Second(), t.Nanosecond(), t.Location())
}
