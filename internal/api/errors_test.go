package api_test

import (
	"net/http"
	"strings"
	"testing"

	"github.com/appifyhub/agent-cli/internal/api"
	"github.com/appifyhub/agent-cli/internal/testkit"
)

func TestClassifyTable(t *testing.T) {
	for _, c := range []struct {
		name   string
		auth   bool
		status int
		code   int
		want   api.Kind
	}{
		{"rate limit without code", true, 429, 0, api.KindRateLimited},
		{"rate limit with provider code", true, 429, 6004, api.KindRateLimited},
		{"rate limit on connect", false, 429, 0, api.KindRateLimited},
		{"redirect", true, 302, 0, api.KindUnexpected},
		{"server error", true, 500, 0, api.KindServerFault},
		{"delivery failure", true, 502, 5018, api.KindServerFault},
		{"gateway", false, 503, 0, api.KindServerFault},
		{"policies not accepted", true, 403, 3008, api.KindAccountGate},
		{"waitlist", true, 403, 3007, api.KindAccountGate},
		{"401 invalid token", true, 401, 4004, api.KindTokenRejected},
		{"401 revoked", true, 401, 4005, api.KindTokenRejected},
		{"401 without code", true, 401, 0, api.KindTokenRejected},
		{"404 user gone", true, 404, 2001, api.KindTokenRejected},
		{"403 other user", true, 403, 3002, api.KindTokenRejected},
		{"403 without code", true, 403, 0, api.KindTokenRejected},
		{"403 unknown code", true, 403, 3999, api.KindTokenRejected},
		{"not chat admin", true, 403, 3001, api.KindChatUnreachable},
		{"no private chat", true, 403, 3004, api.KindChatUnreachable},
		{"not chat member", true, 403, 3009, api.KindChatUnreachable},
		{"delivery refused", true, 403, 3010, api.KindChatUnreachable},
		{"unknown chat", true, 404, 2002, api.KindChatUnreachable},
		{"insufficient credits", true, 422, 1034, api.KindInsufficientCredits},
		{"connect: unknown key", false, 404, 2015, api.KindInvalidConnectKey},
		{"connect: 401", false, 401, 0, api.KindInvalidConnectKey},
		{"connect: 403", false, 403, 3002, api.KindInvalidConnectKey},
		{"connect: invalid TTL", false, 400, 1043, api.KindInvalidTTL},
		{"connect: TTL too long", false, 400, 1044, api.KindTTLTooLong},
		{"connect: device rejected", false, 422, 1045, api.KindDeviceRejected},
		{"invalid platform", true, 422, 1019, api.KindValidation},
		{"malformed chat id", true, 422, 1030, api.KindValidation},
		{"bad message", true, 422, 1046, api.KindValidation},
		{"bare 400", true, 400, 0, api.KindValidation},
		{"framework 422", true, 422, 0, api.KindValidation},
		{"authenticated unknown 404", true, 404, 0, api.KindFailure},
		{"authenticated 404 with another code", true, 404, 2015, api.KindFailure},
		{"conflict", true, 409, 1999, api.KindFailure},
		{"unauthenticated unknown 404", false, 404, 0, api.KindFailure},
		{"1034 with the wrong status", true, 400, 1034, api.KindValidation},
	} {
		if got := api.Classify(c.auth, c.status, c.code); got != c.want {
			t.Errorf("%s: Classify(%v, %d, %d) = %v, want %v", c.name, c.auth, c.status, c.code, got, c.want)
		}
	}
}

func TestClassificationOfRealBodies(t *testing.T) {
	cases := []struct {
		name string
		resp testkit.Response
		kind api.Kind
		code int
		msg  string
		det  bool
	}{
		{"structured insufficient credits", testkit.Err(422, 1034, "Not enough credits"), api.KindInsufficientCredits, 1034, "Not enough credits", false},
		{"plain-text rate limit", testkit.Text(429, "Too Many Requests"), api.KindRateLimited, 0, "", false},
		{"HTML gateway error", testkit.HTML(502), api.KindServerFault, 0, "", false},
		{"framework validation body", testkit.Detail(422, []map[string]any{{"loc": []string{"body", "message"}, "msg": "field required"}}), api.KindValidation, 0, "", true},
		{"unknown route", testkit.Detail(404, "Not Found"), api.KindFailure, 0, "", true},
		{"conflict with a code", testkit.Err(409, 1999, "Clash"), api.KindFailure, 1999, "Clash", false},
		{"forbidden without a code", testkit.Text(403, ""), api.KindTokenRejected, 0, "", false},
		{"unclean message", testkit.JSON(422, map[string]any{"error_code": 1999, "message": "[E1999] ✏️ Something is off # Caused by: ValueError('x')"}), api.KindValidation, 1999, "Something is off", false},
		{"empty body", testkit.Response{Status: 500}, api.KindServerFault, 0, "", false},
		{"error code of the wrong type", testkit.JSON(422, map[string]any{"error_code": "x", "message": "m"}), api.KindValidation, 0, "", false},
	}
	for _, c := range cases {
		b := testkit.NewBackend(t)
		b.Handle(http.MethodGet, "/cli/chats", c.resp)
		_, err := newClient(b).Chats(t.Context(), secretToken)
		apiErr := wantKind(t, err, c.kind)
		if apiErr.Code != c.code || apiErr.Message != c.msg || apiErr.Detail != c.det {
			t.Errorf("%s: got %+v, want code %d message %q detail %v", c.name, apiErr, c.code, c.msg, c.det)
		}
	}
}

func TestErrorTextNeverHoldsTheTokenOrTheBody(t *testing.T) {
	b := testkit.NewBackend(t)
	b.Handle(http.MethodGet, "/cli/chats", testkit.Text(500, "echo "+secretToken))
	_, err := newClient(b).Chats(t.Context(), secretToken)
	testkit.AssertNoSecret(t, secretToken, err.Error())
	for _, secret := range []string{secretToken, `header."payload".signature`} {
		for _, prefix := range []string{"Invalid token ", strings.Repeat("x", 187)} {
			b := testkit.NewBackend(t)
			b.Handle(http.MethodGet, "/cli/chats", testkit.JSON(422, map[string]any{"error_code": 1999, "message": prefix + secret + " rejected"}))
			_, err := newClient(b).Chats(t.Context(), secret)
			got := wantKind(t, err, api.KindValidation)
			if got.Status != 422 || got.Code != 1999 || got.Message != prefix+"*** rejected" {
				t.Errorf("incorrect redacted validation error: %#v", got)
			}
			testkit.AssertNoSecret(t, secret, got.Message, err.Error())
			if b.Count() != 1 || b.Last(t).Header.Get("Authorization") != "Bearer "+secret {
				t.Error("request token was changed or request retried")
			}
		}
	}
}

func TestCleanMessage(t *testing.T) {
	for in, want := range map[string]string{
		"[E1999] ✏️ Something is off # Caused by: ValueError('x')": "Something is off",
		"[E1034] ⚠️ Not enough credits":                            "Not enough credits",
		"[E12]Plain":                                               "Plain",
		"  spaced \n out\ttext ":                                   "spaced out text",
		"no marker # Caused by: boom":                              "no marker",
		"keeps [E5] in the middle":                                 "keeps [E5] in the middle",
		"":                                                         "",
		"[E1999] ⚠️ 2 items failed":                                "2 items failed",
		"[E1999] ✏️ Ünïcode stays":                                 "Ünïcode stays",
	} {
		if got := api.CleanMessage(in); got != want {
			t.Errorf("CleanMessage(%q) = %q, want %q", in, got, want)
		}
	}
}
