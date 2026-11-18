package cli

import (
	"fmt"

	"github.com/appifyhub/agent-cli/internal/render"
)

// LowCreditThreshold is the credit balance below which the user is warned.
const LowCreditThreshold = 100

func (s *rootState) warnLowCredit(balance *float64) {
	if balance == nil || *balance >= LowCreditThreshold {
		return
	}
	credits := render.Credits(*balance)
	_, _ = fmt.Fprintln(s.deps.Stderr, s.deps.Text.T("warning.low_credit", "credits", credits))
}
