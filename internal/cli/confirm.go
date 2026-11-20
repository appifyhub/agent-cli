package cli

import (
	"context"
	"strings"

	"github.com/appifyhub/agent-cli/internal/term"
)

func (s *rootState) confirm(ctx context.Context, prompt string) error {
	answer, err := s.deps.Term.ReadLine(ctx, prompt)
	if err != nil {
		return err
	}
	if !strings.EqualFold(answer, "y") && !strings.EqualFold(answer, "yes") {
		return term.ErrAborted
	}
	return nil
}
