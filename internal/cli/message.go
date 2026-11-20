package cli

import (
	"fmt"
	"io"
	"strings"
	"unicode/utf8"

	"github.com/appifyhub/agent-cli/internal/term"
)

func (s *rootState) notifyMessage(args []string) (string, error) {
	cat := s.deps.Text
	if len(args) > 1 {
		return "", usageError(cat.T("notify.single_argument"))
	}
	var message string
	if len(args) == 1 {
		message = args[0]
	} else {
		if s.deps.Term.IsTTY(term.In) {
			return "", usageError(cat.T("notify.missing_message"))
		}
		data, err := io.ReadAll(io.LimitReader(s.deps.Stdin, 4*4096))
		if err != nil {
			return "", err
		}
		message = string(data)
	}
	message = strings.TrimSpace(strings.TrimPrefix(strings.ToValidUTF8(message, "\uFFFD"), "\uFEFF"))
	if message == "" {
		return "", usageError(cat.T("notify.empty_message"))
	}
	if utf8.RuneCountInString(message) > 4096 {
		if len(args) == 1 {
			return "", usageError(cat.T("notify.long_message"))
		}
		count := 0
		for index := range message {
			if count == 4096 {
				message = message[:index]
				break
			}
			count++
		}
		_, _ = fmt.Fprintln(s.deps.Stderr, s.deps.styled(term.Err, term.Warning, cat.T("notify.truncated")))
	}
	return message, nil
}
