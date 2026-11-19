package token_test

import (
	"encoding/base64"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/appifyhub/agent-cli/internal/testkit"
	"github.com/appifyhub/agent-cli/internal/token"
)

func rawPayload(json string) string {
	return "e30." + base64.RawURLEncoding.EncodeToString([]byte(json)) + ".c2ln"
}

func TestParseValidToken(t *testing.T) {
	exp := time.Date(2027, 2, 14, 9, 21, 0, 0, time.UTC)
	raw := testkit.NewToken(testkit.TokenClaims{Sub: "9f1c2d3e", Exp: exp, DeviceID: "dev-1", Hostname: "morpheus-mbp"})
	got, err := token.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	want := token.Claims{Subject: "9f1c2d3e", ExpiresAt: exp, DeviceID: "dev-1", Hostname: "morpheus-mbp"}
	if got != want {
		t.Errorf("got %+v, want %+v", got, want)
	}
}

func TestParseOptionalClaimsMayBeAbsent(t *testing.T) {
	got, err := token.Parse(testkit.NewToken(testkit.TokenClaims{Sub: "u", Exp: time.Unix(1800000000, 0)}))
	if err != nil || got.DeviceID != "" || got.Hostname != "" {
		t.Errorf("got %+v, %v", got, err)
	}
	got, err = token.Parse(rawPayload(`{"sub":"u"}`))
	if err != nil || got.Subject != "u" || !got.ExpiresAt.IsZero() {
		t.Errorf("absent expiry: %+v, %v", got, err)
	}
}

func TestParseExpAsIntegerAndFloat(t *testing.T) {
	cases := map[string]time.Time{
		`{"sub":"u","exp":1800000000}`:                 time.Unix(1800000000, 0).UTC(),
		`{"sub":"u","exp":1800000000.5}`:               time.Unix(1800000000, 500000000).UTC(),
		`{"sub":"u","exp":1.8e9}`:                      time.Unix(1800000000, 0).UTC(),
		`{"sub":"u","exp":1800000000,"x":[1,{"y":2}]}`: time.Unix(1800000000, 0).UTC(),
	}
	for payload, want := range cases {
		got, err := token.Parse(rawPayload(payload))
		if err != nil || !got.ExpiresAt.Equal(want) {
			t.Errorf("%s: got %v, %v; want %v", payload, got.ExpiresAt, err, want)
		}
	}
}

func TestParseRejectsMalformedTokens(t *testing.T) {
	cases := map[string]string{
		"empty":             "",
		"one segment":       "not-a-token",
		"two segments":      "a.b",
		"four segments":     "a.b.c.d",
		"empty header":      ".e30.c2ln",
		"empty signature":   "e30.e30.",
		"bad base64":        "e30.!!!.c2ln",
		"padded base64":     "e30.e30=.c2ln",
		"not json":          rawPayload(`not json`),
		"json array":        rawPayload(`[1,2]`),
		"missing sub":       rawPayload(`{"exp":1800000000}`),
		"empty sub":         rawPayload(`{"sub":"","exp":1800000000}`),
		"numeric sub":       rawPayload(`{"sub":5,"exp":1800000000}`),
		"string exp":        rawPayload(`{"sub":"u","exp":"1800000000"}`),
		"zero exp":          rawPayload(`{"sub":"u","exp":0}`),
		"negative exp":      rawPayload(`{"sub":"u","exp":-5}`),
		"oversized":         "a." + strings.Repeat("b", token.MaxLength) + ".c",
		"whitespace padded": " " + testkit.NewToken(testkit.TokenClaims{Sub: "u", Exp: time.Unix(1800000000, 0)}),
	}
	for name, raw := range cases {
		if _, err := token.Parse(raw); !errors.Is(err, token.ErrMalformed) {
			t.Errorf("%s: got %v, want ErrMalformed", name, err)
		}
	}
}

func TestErrorNeverEchoesInput(t *testing.T) {
	secret := "s3cr3t-looking-input"
	_, err := token.Parse(secret)
	if err == nil || strings.Contains(err.Error(), secret) {
		t.Errorf("error %v must not contain the input", err)
	}
}
