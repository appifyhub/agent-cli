package api_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/appifyhub/agent-cli/internal/api"
	"github.com/appifyhub/agent-cli/internal/testkit"
)

func connectRequest() api.ConnectRequest {
	return api.ConnectRequest{ConnectKey: "K7Q2-M9XA-P3RT", DeviceID: "dev-1", Hostname: "milos-mbp", OS: "darwin"}
}

func bodyOf(t *testing.T, r testkit.Request) map[string]any {
	t.Helper()
	var body map[string]any
	if err := json.Unmarshal([]byte(r.Body), &body); err != nil {
		t.Fatalf("request body %q: %v", r.Body, err)
	}
	return body
}

func TestConnect(t *testing.T) {
	b := testkit.NewBackend(t)
	b.Handle("POST", "/cli/connect", testkit.ConnectOK(secretToken, expiry, "u1", "Milos", 12.5))
	got, err := newClient(b).Connect(t.Context(), connectRequest())
	if err != nil {
		t.Fatal(err)
	}
	if got.Token != secretToken || !got.ExpiresAt.Equal(expiry) || got.UserID != "u1" || got.UserDisplayName != "Milos" || got.CreditBalance == nil || *got.CreditBalance != 12.5 {
		t.Errorf("got %+v", got)
	}
	body := bodyOf(t, b.Last(t))
	want := map[string]any{"connect_key": "K7Q2-M9XA-P3RT", "device_id": "dev-1", "hostname": "milos-mbp", "os": "darwin"}
	if len(body) != len(want) {
		t.Errorf("body %v must not carry a ttl_as_text without --ttl", body)
	}
	for k, v := range want {
		if body[k] != v {
			t.Errorf("body[%s] = %v, want %v", k, body[k], v)
		}
	}
}

func TestConnectForwardsTheTTLVerbatim(t *testing.T) {
	b := testkit.NewBackend(t)
	b.Handle("POST", "/cli/connect", testkit.ConnectOK(secretToken, expiry, "u1", "Milos", 1))
	r := connectRequest()
	r.TTL = "1h30m"
	if _, err := newClient(b).Connect(t.Context(), r); err != nil {
		t.Fatal(err)
	}
	if got := bodyOf(t, b.Last(t))["ttl_as_text"]; got != "1h30m" {
		t.Errorf("ttl_as_text = %v", got)
	}
}

func TestConnectTimestamps(t *testing.T) {
	for ts, want := range map[string]time.Time{
		"2027-02-14T09:21:00Z":        expiry,
		"2027-02-14T10:21:00+01:00":   expiry,
		"2027-02-14T09:21:00.123456Z": expiry.Add(123456 * time.Microsecond),
		"2027-02-14T04:21:00.5-05:00": expiry.Add(500 * time.Millisecond),
	} {
		b := testkit.NewBackend(t)
		b.Handle("POST", "/cli/connect", testkit.JSON(200, map[string]any{"token": "t", "expires_at": ts, "user_id": "u", "user_display_name": "n"}))
		got, err := newClient(b).Connect(t.Context(), connectRequest())
		if err != nil || !got.ExpiresAt.Equal(want) {
			t.Errorf("%s: got %v, %v; want %v", ts, got.ExpiresAt, err, want)
		}
	}
}

func TestConnectRejectsUnusableAnswers(t *testing.T) {
	good := map[string]any{"token": "t", "expires_at": "2027-02-14T09:21:00Z", "user_id": "u", "user_display_name": "n"}
	without := func(key string) map[string]any {
		m := map[string]any{}
		for k, v := range good {
			if k != key {
				m[k] = v
			}
		}
		return m
	}
	with := func(key string, v any) map[string]any {
		m := without(key)
		m[key] = v
		return m
	}
	for name, body := range map[string]any{
		"missing token":   without("token"),
		"empty token":     with("token", ""),
		"null token":      with("token", nil),
		"missing expiry":  without("expires_at"),
		"bad timestamp":   with("expires_at", "tomorrow"),
		"date only":       with("expires_at", "2027-02-14"),
		"unix timestamp":  with("expires_at", 1800000000),
		"missing user id": without("user_id"),
		"missing name":    without("user_display_name"),
		"array":           []any{},
		"string":          "ok",
	} {
		b := testkit.NewBackend(t)
		b.Handle("POST", "/cli/connect", testkit.JSON(200, body))
		_, err := newClient(b).Connect(t.Context(), connectRequest())
		if apiErr := wantKind(t, err, api.KindUnexpected); apiErr.Status != 200 {
			t.Errorf("%s: %+v", name, apiErr)
		}
	}
}

func TestCreditBalanceIsKeptOnlyWhenItIsANumber(t *testing.T) {
	for name, c := range map[string]struct {
		balance any
		want    *float64
	}{
		"float":          {12.75, ptr(12.75)},
		"integer":        {3, ptr(3.0)},
		"zero":           {0, ptr(0.0)},
		"null":           {nil, nil},
		"string":         {"lots", nil},
		"numeric string": {"12", nil},
		"bool":           {true, nil},
		"object":         {map[string]any{"a": 1}, nil},
	} {
		body := map[string]any{"token": "t", "expires_at": "2027-02-14T09:21:00Z", "user_id": "u", "user_display_name": "n", "credit_balance": c.balance}
		b := testkit.NewBackend(t)
		b.Handle("POST", "/cli/connect", testkit.JSON(200, body))
		got, err := newClient(b).Connect(t.Context(), connectRequest())
		if err != nil || (got.CreditBalance == nil) != (c.want == nil) || (c.want != nil && *got.CreditBalance != *c.want) {
			t.Errorf("%s: got %v, %v", name, got.CreditBalance, err)
		}
	}

	b := testkit.NewBackend(t)
	b.Handle("POST", "/cli/connect", testkit.JSON(200, map[string]any{"token": "t", "expires_at": "2027-02-14T09:21:00Z", "user_id": "u", "user_display_name": "n"}))
	if got, err := newClient(b).Connect(t.Context(), connectRequest()); err != nil || got.CreditBalance != nil {
		t.Errorf("absent balance: got %v, %v", got.CreditBalance, err)
	}
}

func TestRevokeAll(t *testing.T) {
	b := testkit.NewBackend(t)
	b.Handle("DELETE", "/cli/user/u1/devices", testkit.JSON(200, map[string]any{"status": "OK"}))
	if err := newClient(b).RevokeAll(t.Context(), secretToken, "u1"); err != nil {
		t.Fatal(err)
	}
	r := b.Last(t)
	if r.Body != "" || r.Header.Get("Authorization") != "Bearer "+secretToken {
		t.Errorf("request %+v", r)
	}
	b.Handle("DELETE", "/cli/user/u1/devices", testkit.Response{Status: 204})
	if err := newClient(b).RevokeAll(t.Context(), secretToken, "u1"); err != nil {
		t.Errorf("any 2xx is success: %v", err)
	}
}

func TestChats(t *testing.T) {
	b := testkit.NewBackend(t)
	b.Handle("GET", "/cli/chats", testkit.JSON(200, []any{
		testkit.ChatJSON("c1", "telegram", ptr("Milos"), true),
		testkit.ChatJSON("c2", "whatsapp", nil, false),
		map[string]any{"chat_id": "c3", "platform": "telegram", "title": "Team", "is_private": false, "added_later": []int{1}},
	}))
	got, err := newClient(b).Chats(t.Context(), secretToken)
	if err != nil {
		t.Fatal(err)
	}
	want := []api.Chat{{ID: "c1", Platform: "telegram", Title: "Milos", Private: true}, {ID: "c2", Platform: "whatsapp"}, {ID: "c3", Platform: "telegram", Title: "Team"}}
	if len(got) != 3 || got[0] != want[0] || got[1] != want[1] || got[2] != want[2] {
		t.Errorf("got %+v", got)
	}
}

func TestEmptyChatList(t *testing.T) {
	b := testkit.NewBackend(t)
	b.Handle("GET", "/cli/chats", testkit.JSON(200, []any{}))
	got, err := newClient(b).Chats(t.Context(), secretToken)
	if err != nil || got == nil || len(got) != 0 {
		t.Errorf("got %v, %v", got, err)
	}
}

func TestChatsRejectsUnusableAnswers(t *testing.T) {
	for name, body := range map[string]any{
		"object":           map[string]any{},
		"null":             nil,
		"empty chat id":    []any{testkit.ChatJSON("", "telegram", nil, true)},
		"missing chat id":  []any{map[string]any{"platform": "telegram", "is_private": true}},
		"missing platform": []any{map[string]any{"chat_id": "c1", "is_private": true}},
		"missing privacy":  []any{map[string]any{"chat_id": "c1", "platform": "telegram"}},
		"non-object item":  []any{"c1"},
		"one bad item":     []any{testkit.ChatJSON("c1", "telegram", nil, true), testkit.ChatJSON("", "telegram", nil, true)},
	} {
		b := testkit.NewBackend(t)
		b.Handle("GET", "/cli/chats", testkit.JSON(200, body))
		_, err := newClient(b).Chats(t.Context(), secretToken)
		if apiErr := asError(t, err); apiErr.Kind != api.KindUnexpected {
			t.Errorf("%s: %+v", name, apiErr)
		}
	}
}

func TestNotifyUser(t *testing.T) {
	b := testkit.NewBackend(t)
	b.Handle("POST", "/cli/notify/user/u1", testkit.NotifyOK(testkit.ChatJSON("c1", "whatsapp", ptr("Milos"), true), 9.25))
	got, err := newClient(b).NotifyUser(t.Context(), secretToken, "u1", "hi there", "whatsapp")
	if err != nil {
		t.Fatal(err)
	}
	if got.Chat == nil || *got.Chat != (api.Chat{ID: "c1", Platform: "whatsapp", Title: "Milos", Private: true}) || got.CreditBalance == nil || *got.CreditBalance != 9.25 {
		t.Errorf("got %+v", got)
	}
	body := bodyOf(t, b.Last(t))
	if body["message"] != "hi there" || body["platform"] != "whatsapp" || len(body) != 2 {
		t.Errorf("body %v", body)
	}
}

func TestNotifyUserWithoutAPlatformSendsNull(t *testing.T) {
	b := testkit.NewBackend(t)
	b.Handle("POST", "/cli/notify/user/u1", testkit.NotifyOK(testkit.ChatJSON("c1", "telegram", nil, true), 1))
	if _, err := newClient(b).NotifyUser(t.Context(), secretToken, "u1", "hi", ""); err != nil {
		t.Fatal(err)
	}
	body := bodyOf(t, b.Last(t))
	if value, present := body["platform"]; !present || value != nil {
		t.Errorf("platform must be an explicit null: %v", body)
	}
}

func TestNotifyChat(t *testing.T) {
	b := testkit.NewBackend(t)
	b.Handle("POST", "/cli/notify/chat/c1", testkit.NotifyOK(testkit.ChatJSON("c1", "telegram", ptr("Team"), false), 2))
	got, err := newClient(b).NotifyChat(t.Context(), secretToken, "c1", "deploy done")
	if err != nil || got.Chat == nil || got.Chat.ID != "c1" {
		t.Fatalf("got %+v, %v", got, err)
	}
	if body := bodyOf(t, b.Last(t)); len(body) != 1 || body["message"] != "deploy done" {
		t.Errorf("body %v", body)
	}
}

func TestChatIDIsEscapedInThePath(t *testing.T) {
	b := testkit.NewBackend(t)
	b.Handle("POST", "/cli/notify/chat/../x", testkit.Response{Status: 200})
	_, _ = newClient(b).NotifyChat(t.Context(), secretToken, "../x?y=1", "hi")
	r := b.Last(t)
	if r.RawQuery != "" || r.Path != "/cli/notify/chat/../x?y=1" {
		t.Errorf("path %q query %q", r.Path, r.RawQuery)
	}
}

func TestNotifySuccessNeverFailsOnItsBody(t *testing.T) {
	for name, resp := range map[string]testkit.Response{
		"empty body":       {Status: 200},
		"empty object":     testkit.JSON(200, map[string]any{}),
		"204":              {Status: 204},
		"not json":         testkit.Text(200, "sent"),
		"array":            testkit.JSON(200, []any{}),
		"chat without id":  testkit.JSON(200, map[string]any{"platform": "telegram", "is_private": true}),
		"unusable balance": testkit.JSON(200, map[string]any{"credit_balance": "lots"}),
	} {
		b := testkit.NewBackend(t)
		b.Handle("POST", "/cli/notify/user/u1", resp)
		got, err := newClient(b).NotifyUser(t.Context(), secretToken, "u1", "hi", "")
		if err != nil || got.Chat != nil || got.CreditBalance != nil {
			t.Errorf("%s: got %+v, %v", name, got, err)
		}
	}
}

func TestNotifyErrorsAreClassified(t *testing.T) {
	b := testkit.NewBackend(t)
	b.Handle("POST", "/cli/notify/user/u1", testkit.Err(422, 1034, "Not enough credits"))
	_, err := newClient(b).NotifyUser(t.Context(), secretToken, "u1", "hi", "")
	_ = wantKind(t, err, api.KindInsufficientCredits)
}
