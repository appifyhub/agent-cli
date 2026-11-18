// Package token reads the claims of the access tokens the backend issues. The signature is not
// checked and no authorization decision depends on the result: it only tells who a token claims
// to be for, when it claims to expire, and whether a string is shaped like a token at all.
package token

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"math"
	"strings"
	"time"
)

// MaxLength is the longest token accepted; real tokens are a few hundred bytes.
const MaxLength = 8192

// ErrMalformed means a string is not shaped like a CLI token.
var ErrMalformed = errors.New("malformed token")

// Claims are the fields of a token's payload the CLI uses.
type Claims struct {
	Subject   string
	ExpiresAt time.Time
	DeviceID  string
	Hostname  string
}

type payload struct {
	Sub      string          `json:"sub"`
	Exp      json.RawMessage `json:"exp"`
	DeviceID string          `json:"device_id"`
	Hostname string          `json:"hostname"`
}

// Parse splits a token into its three segments, decodes the payload and returns its claims.
// Anything that is not three non-empty dot-separated segments with a JSON object payload holding
// a string sub and a numeric exp is ErrMalformed; the error never includes any part of the input.
func Parse(raw string) (Claims, error) {
	if raw == "" || len(raw) > MaxLength {
		return Claims{}, ErrMalformed
	}
	segments := strings.Split(raw, ".")
	if len(segments) != 3 {
		return Claims{}, ErrMalformed
	}
	for _, s := range segments {
		if !isSegment(s) {
			return Claims{}, ErrMalformed
		}
	}
	body, err := base64.RawURLEncoding.DecodeString(segments[1])
	if err != nil {
		return Claims{}, ErrMalformed
	}
	var p payload
	if err := json.Unmarshal(body, &p); err != nil || p.Sub == "" {
		return Claims{}, ErrMalformed
	}
	seconds, err := expSeconds(p.Exp)
	if err != nil || math.IsNaN(seconds) || math.IsInf(seconds, 0) || seconds <= 0 {
		return Claims{}, ErrMalformed
	}
	whole, fraction := math.Modf(seconds)
	return Claims{
		Subject:   p.Sub,
		ExpiresAt: time.Unix(int64(whole), int64(fraction*1e9)).UTC(),
		DeviceID:  p.DeviceID,
		Hostname:  p.Hostname,
	}, nil
}

// isSegment reports whether s is a non-empty run of base64url characters.
func isSegment(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c >= '0' && c <= '9', c == '-', c == '_':
		default:
			return false
		}
	}
	return true
}

// expSeconds reads exp as a JSON number only; a quoted number is not accepted.
func expSeconds(raw json.RawMessage) (float64, error) {
	if len(raw) == 0 || raw[0] == '"' {
		return 0, ErrMalformed
	}
	var seconds float64
	if err := json.Unmarshal(raw, &seconds); err != nil {
		return 0, ErrMalformed
	}
	return seconds, nil
}
