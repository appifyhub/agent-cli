package cli

// Exit codes of the process; no other code is ever used.
const (
	ExitOK                  = 0
	ExitFailure             = 1
	ExitUsage               = 2
	ExitNotConnected        = 3
	ExitChatUnreachable     = 4
	ExitInsufficientCredits = 5
	ExitRateLimited         = 6
)
