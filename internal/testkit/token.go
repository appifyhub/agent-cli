package testkit

import (
	"encoding/base64"
	"encoding/json"
	"time"
)

// TokenClaims are the payload fields NewToken puts into a token; empty fields are omitted.
type TokenClaims struct {
	Sub      string
	Exp      time.Time
	DeviceID string
	Hostname string
}

// NewToken builds an HS256-shaped token for tests. The signature is a fixed placeholder: the CLI
// never verifies it.
func NewToken(c TokenClaims) string {
	payload := map[string]any{"sub": c.Sub, "exp": c.Exp.Unix(), "cli_generation": 1}
	if c.DeviceID != "" {
		payload["device_id"] = c.DeviceID
	}
	if c.Hostname != "" {
		payload["hostname"] = c.Hostname
	}
	return RawToken(map[string]any{"alg": "HS256", "typ": "JWT"}, payload)
}

// RawToken encodes any header and payload as a token, for building malformed ones.
func RawToken(header, payload map[string]any) string {
	encode := func(v any) string {
		body, err := json.Marshal(v)
		if err != nil {
			panic(err)
		}
		return base64.RawURLEncoding.EncodeToString(body)
	}
	return encode(header) + "." + encode(payload) + "." + base64.RawURLEncoding.EncodeToString([]byte("not-a-real-signature"))
}
