package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
)

// Kind is the outcome of a failed call, decided by Classify.
type Kind int

const (
	KindFailure Kind = iota
	KindUnreachable
	KindRateLimited
	KindUnexpected
	KindServerFault
	KindAccountGate
	KindTokenRejected
	KindChatUnreachable
	KindInsufficientCredits
	KindInvalidConnectKey
	KindInvalidTTL
	KindTTLTooLong
	KindDeviceRejected
	KindValidation
)

var kindNames = [...]string{
	KindFailure:             "failure",
	KindUnreachable:         "unreachable",
	KindRateLimited:         "rate limited",
	KindUnexpected:          "unexpected response",
	KindServerFault:         "server fault",
	KindAccountGate:         "account gate",
	KindTokenRejected:       "token rejected",
	KindChatUnreachable:     "chat unreachable",
	KindInsufficientCredits: "insufficient credits",
	KindInvalidConnectKey:   "invalid connect key",
	KindInvalidTTL:          "invalid TTL",
	KindTTLTooLong:          "TTL too long",
	KindDeviceRejected:      "device rejected",
	KindValidation:          "validation",
}

func (k Kind) String() string { return kindNames[k] }

// Error is a failed call to the backend. It carries what the backend said and what that means,
// never the request, the token or the raw body.
type Error struct {
	Kind Kind
	// Status is the HTTP status, zero when no answer arrived.
	Status int
	// Code is the backend's error code, zero when the body had none.
	Code int
	// Message is the cleaned text the backend gave, empty when it gave none.
	Message string
	// Detail is true when the body was a framework {"detail": ...} body.
	Detail bool
	// Cause is the transport failure behind KindUnreachable.
	Cause error
}

func (e *Error) Error() string {
	switch {
	case e.Kind == KindUnreachable:
		return "could not reach the server"
	case e.Status == 0:
		return "backend error: " + e.Kind.String()
	case e.Code != 0:
		return fmt.Sprintf("backend error: %s (HTTP %d, code %d)", e.Kind, e.Status, e.Code)
	}
	return fmt.Sprintf("backend error: %s (HTTP %d)", e.Kind, e.Status)
}

func (e *Error) Unwrap() error { return e.Cause }

// Classify decides the outcome of an error response; the first matching row wins. authenticated is
// true for calls that carry the token, and code is zero when the body had no error code.
func Classify(authenticated bool, status, code int) Kind {
	switch {
	case status == 429:
		return KindRateLimited
	case status >= 300 && status < 400:
		return KindUnexpected
	case status >= 500:
		return KindServerFault
	case status == 403 && (code == CodePoliciesRequired || code == CodeWaitlistNotActive):
		return KindAccountGate
	case authenticated && tokenRejected(status, code):
		return KindTokenRejected
	case status == 403 && (code == CodeNotChatAdmin || code == CodeNoPrivateChat || code == CodeNotChatMember || code == CodeDeliveryRefused),
		status == 404 && code == CodeChatNotFound:
		return KindChatUnreachable
	case status == 422 && code == CodeInsufficientCredits:
		return KindInsufficientCredits
	case !authenticated && (status == 404 && code == CodeConnectKeyNotFound || status == 401 || status == 403):
		return KindInvalidConnectKey
	case !authenticated && status == 400 && code == CodeInvalidTTL:
		return KindInvalidTTL
	case !authenticated && status == 400 && code == CodeTTLTooLong:
		return KindTTLTooLong
	case !authenticated && status == 422 && code == CodeInvalidDevice:
		return KindDeviceRejected
	case status == 400 || status == 422:
		return KindValidation
	}
	return KindFailure
}

func tokenRejected(status, code int) bool {
	switch status {
	case 401:
		return true
	case 404:
		return code == CodeUserNotFound
	case 403:
		switch code {
		case CodeNotChatAdmin, CodeNoPrivateChat, CodeWaitlistNotActive, CodePoliciesRequired, CodeNotChatMember, CodeDeliveryRefused:
			return false
		}
		return true
	}
	return false
}

// errorFromResponse builds the Error of a non-success answer from its status and body.
func errorFromResponse(authenticated bool, status int, body []byte) *Error {
	code, message, detail := decodeErrorBody(body)
	return &Error{
		Kind:    Classify(authenticated, status, code),
		Status:  status,
		Code:    code,
		Message: CleanMessage(message),
		Detail:  detail,
	}
}

// decodeErrorBody tries {error_code, message}, then {detail}; anything else is opaque.
func decodeErrorBody(body []byte) (code int, message string, detail bool) {
	var structured struct {
		ErrorCode *json.Number    `json:"error_code"`
		Message   string          `json:"message"`
		Detail    json.RawMessage `json:"detail"`
	}
	if err := json.Unmarshal(body, &structured); err != nil {
		return 0, "", false
	}
	if structured.ErrorCode != nil {
		if n, err := structured.ErrorCode.Int64(); err == nil && n > 0 && n < 1_000_000 {
			return int(n), structured.Message, false
		}
	}
	return 0, "", len(structured.Detail) > 0
}

var (
	errorMarker = regexp.MustCompile(`^\s*\[E\d+\][^\p{L}\p{N}]*`)
	causeSuffix = regexp.MustCompile(`\s+# Caused by:.*$`)
)

// CleanMessage reduces a server message to its text: the leading [E<digits>] marker with the emoji
// or symbols after it and the trailing " # Caused by:" part are removed and whitespace is collapsed.
// Escape sequences and control characters are the caller's business (term.Clean).
func CleanMessage(raw string) string {
	s := causeSuffix.ReplaceAllString(raw, "")
	s = errorMarker.ReplaceAllString(s, "")
	return strings.Join(strings.Fields(s), " ")
}

// unexpected is the error of a success answer that cannot be used.
func unexpected(status int) *Error {
	return &Error{Kind: KindUnexpected, Status: status}
}

// IsKind reports whether err is a backend Error of the given kind.
func IsKind(err error, kind Kind) bool {
	var apiErr *Error
	return errors.As(err, &apiErr) && apiErr.Kind == kind
}
