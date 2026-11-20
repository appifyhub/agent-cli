package render_test

import (
	"testing"

	"github.com/appifyhub/agent-cli/internal/render"
)

func TestCredits(t *testing.T) {
	for _, c := range []struct {
		value   float64
		credits string
	}{
		{42.37, "42.37"},
		{99.999, "99.99"},
		{100, "100.00"},
		{500, "500.00"},
		{0, "0.00"},
		{-3.5, "-3.50"},
		{1.15, "1.15"},
		{12345.678, "12345.67"},
		{0.009, "0.00"},
	} {
		credits := render.Credits(c.value)
		if credits != c.credits {
			t.Errorf("%v: %s; want %s", c.value, credits, c.credits)
		}
	}
}
