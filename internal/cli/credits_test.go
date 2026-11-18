package cli

import (
	"bytes"
	"testing"

	"github.com/appifyhub/agent-cli/internal/text"
)

func TestWarnLowCredit(t *testing.T) {
	for _, c := range []struct {
		balance *float64
		want    string
	}{
		{nil, ""},
		{new(100.0), ""},
		{new(500.0), ""},
		{new(42.37), "warning: low credit balance: 42.37 credits left; top up in the agent's chat settings\n"},
		{new(99.999), "warning: low credit balance: 99.99 credits left; top up in the agent's chat settings\n"},
		{new(0.0), "warning: low credit balance: 0.00 credits left; top up in the agent's chat settings\n"},
		{new(-3.5), "warning: low credit balance: -3.50 credits left; top up in the agent's chat settings\n"},
	} {
		var out bytes.Buffer
		s := rootState{deps: Deps{Text: text.MustNew("linux"), Stderr: &out}}
		s.warnLowCredit(c.balance)
		if out.String() != c.want {
			t.Errorf("got %q, want %q", out.String(), c.want)
		}
	}
}
