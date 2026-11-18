package render_test

import (
	"testing"
	"time"

	"github.com/appifyhub/agent-cli/internal/render"
	"github.com/appifyhub/agent-cli/internal/text"
)

func TestRelativeTime(t *testing.T) {
	location, err := time.LoadLocation("Europe/Berlin")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 11, 14, 10, 21, 0, 0, location)
	cat := text.MustNew("linux")
	for _, c := range []struct {
		target time.Time
		want   string
	}{
		{now.Add(40 * time.Second), "in less than a minute"},
		{now.Add(time.Minute), "in 1 minute"},
		{now.Add(59 * time.Minute), "in 59 minutes"},
		{now.Add(time.Hour), "in 1 hour"},
		{now.Add(90 * time.Minute), "in 2 hours"},
		{now.Add(5 * time.Hour), "in 5 hours"},
		{now.Add(24 * time.Hour), "in 1 day"},
		{now.Add(10 * 24 * time.Hour), "in 10 days"},
		{now.Add(29 * 24 * time.Hour), "in 29 days"},
		{now.Add(30 * 24 * time.Hour), "in 1 month"},
		{now.AddDate(0, 3, 0).Add(-5 * time.Second), "in 3 months"},
		{now.AddDate(0, 3, 0).Add(5 * time.Second), "in 3 months"},
		{now.AddDate(0, 12, 0), "in 1 year"},
		{now.AddDate(0, 18, 0), "in 18 months"},
		{now.AddDate(0, 24, 0), "in 2 years"},
		{now.Add(-48 * time.Hour), "2 days ago"},
		{now.Add(-40 * time.Second), "less than a minute ago"},
		{now.Add(-time.Minute), "1 minute ago"},
		{now.AddDate(0, -3, 0), "3 months ago"},
	} {
		if got := render.RelativeTime(c.target, now, cat); got != c.want {
			t.Errorf("target %v: got %q, want %q", c.target, got, c.want)
		}
	}
	target := time.Date(2027, 2, 14, 9, 21, 0, 0, time.UTC)
	got := render.LocalTime(target, location) + " (" + render.RelativeTime(target, now, cat) + ")"
	if got != "2027-02-14 10:21 (in 3 months)" {
		t.Errorf("got %q", got)
	}
}

func TestRelativeTimeClampsMonthsAndHandlesDST(t *testing.T) {
	loc, err := time.LoadLocation("Europe/Berlin")
	if err != nil {
		t.Fatal(err)
	}
	cat := text.MustNew("linux")
	for _, c := range []struct {
		start, end time.Time
		want       string
	}{
		{time.Date(2027, 1, 31, 10, 0, 0, 0, loc), time.Date(2027, 3, 31, 10, 0, 0, 0, loc), "in 2 months"},
		{time.Date(2027, 1, 31, 10, 0, 0, 0, loc), time.Date(2027, 3, 14, 10, 0, 0, 0, loc), "in 1 month"},
		{time.Date(2027, 1, 31, 10, 0, 0, 0, loc), time.Date(2027, 3, 17, 10, 0, 0, 0, loc), "in 2 months"},
		{time.Date(2027, 3, 27, 10, 0, 0, 0, loc), time.Date(2027, 3, 28, 10, 0, 0, 0, loc), "in 23 hours"},
		{time.Date(2027, 2, 28, 10, 0, 0, 0, loc), time.Date(2027, 4, 28, 10, 0, 0, 0, loc), "in 2 months"},
	} {
		if got := render.RelativeTime(c.end, c.start, cat); got != c.want {
			t.Errorf("%v to %v: %q, want %q", c.start, c.end, got, c.want)
		}
	}
}
