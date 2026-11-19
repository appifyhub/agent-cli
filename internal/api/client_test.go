package api_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/appifyhub/agent-cli/internal/api"
	"github.com/appifyhub/agent-cli/internal/testkit"
)

func TestAuthenticatedRequestHeaders(t *testing.T) {
	b := testkit.NewBackend(t)
	b.Handle("GET", "/cli/chats", testkit.JSON(200, []any{}))
	if _, err := newClient(b).Chats(t.Context(), secretToken); err != nil {
		t.Fatal(err)
	}
	r := b.Last(t)
	if got := r.Header.Get("Authorization"); got != "Bearer "+secretToken {
		t.Errorf("Authorization = %q", got)
	}
	if got := r.Header.Get("User-Agent"); got != "agent-cli/0.1.2 (linux/amd64)" {
		t.Errorf("User-Agent = %q", got)
	}
	if got := r.Header.Get("Accept"); got != "application/json" {
		t.Errorf("Accept = %q", got)
	}
	if r.RawQuery != "" || strings.Contains(r.Path, secretToken) {
		t.Errorf("token or query in the URL: %q %q", r.Path, r.RawQuery)
	}
	if r.Header.Get("Content-Type") != "" {
		t.Errorf("a bodyless request must not claim a content type")
	}
}

func TestConnectSendsNoAuthorization(t *testing.T) {
	b := testkit.NewBackend(t)
	b.Handle("POST", "/cli/connect", testkit.ConnectOK(secretToken, expiry, "u1", "Morpheus", 1.5))
	if _, err := newClient(b).Connect(t.Context(), api.ConnectRequest{ConnectKey: "K7Q2", DeviceID: "d", Hostname: "h", OS: "linux"}); err != nil {
		t.Fatal(err)
	}
	r := b.Last(t)
	if _, present := r.Header["Authorization"]; present {
		t.Error("connect must not send an Authorization header")
	}
	if got := r.Header.Get("Content-Type"); got != "application/json" {
		t.Errorf("Content-Type = %q", got)
	}
}

func TestTrailingSlashBaseURL(t *testing.T) {
	b := testkit.NewBackend(t)
	b.Handle("GET", "/cli/chats", testkit.JSON(200, []any{}))
	client := newClient(b, func(o *api.Options) { o.BaseURL += "/" })
	if _, err := client.Chats(t.Context(), secretToken); err != nil {
		t.Fatal(err)
	}
	if got := b.Last(t).Path; got != "/cli/chats" {
		t.Errorf("path %q", got)
	}
}

func TestRedirectIsNotFollowed(t *testing.T) {
	b := testkit.NewBackend(t)
	b.Handle("GET", "/cli/chats", testkit.Response{Status: 302, Header: map[string]string{"Location": "/elsewhere"}})
	b.Handle("GET", "/elsewhere", testkit.JSON(200, []any{}))
	_, err := newClient(b).Chats(t.Context(), secretToken)
	_ = wantKind(t, err, api.KindUnexpected)
	if b.Count() != 1 {
		t.Errorf("%d requests reached the backend, want 1", b.Count())
	}
}

func TestNoAutomaticRetry(t *testing.T) {
	b := testkit.NewBackend(t)
	b.Handle("POST", "/cli/notify/user/u1", testkit.Text(502, "Bad Gateway"))
	_, err := newClient(b).NotifyUser(t.Context(), secretToken, "u1", "hi", "")
	_ = wantKind(t, err, api.KindServerFault)
	if b.Count() != 1 {
		t.Errorf("%d requests, want exactly 1", b.Count())
	}
}

func TestSlowServerTimesOut(t *testing.T) {
	b := testkit.NewBackend(t)
	b.Handle("GET", "/cli/chats", testkit.Response{Status: 200, Body: "[]", Delay: 2 * time.Second})
	client := newClient(b, func(o *api.Options) { o.Timeout = 50 * time.Millisecond })
	start := time.Now()
	_, err := client.Chats(t.Context(), secretToken)
	_ = wantKind(t, err, api.KindUnreachable)
	if time.Since(start) > time.Second {
		t.Error("the client kept waiting past its timeout")
	}
}

func TestNothingListeningIsUnreachable(t *testing.T) {
	b := testkit.NewBackend(t)
	client := newClient(b, func(o *api.Options) { o.BaseURL = "http://127.0.0.1:1" })
	_, err := client.Health(t.Context())
	apiErr := wantKind(t, err, api.KindUnreachable)
	if apiErr.Status != 0 || apiErr.Cause == nil {
		t.Errorf("got %+v", apiErr)
	}
}

func TestOversizedBodyIsUnexpected(t *testing.T) {
	b := testkit.NewBackend(t)
	b.Handle("GET", "/cli/chats", testkit.Response{Status: 200, Body: "[" + strings.Repeat(" ", 5<<20) + "]"})
	_, err := newClient(b).Chats(t.Context(), secretToken)
	_ = wantKind(t, err, api.KindUnexpected)
}

func TestCancelledContextIsReportedAsSuch(t *testing.T) {
	b := testkit.NewBackend(t)
	b.Handle("GET", "/cli/chats", testkit.Response{Status: 200, Body: "[]", Delay: 2 * time.Second})
	ctx, cancel := context.WithCancel(t.Context())
	time.AfterFunc(30*time.Millisecond, cancel)
	_, err := newClient(b).Chats(ctx, secretToken)
	if err == nil || api.IsKind(err, api.KindUnreachable) {
		t.Errorf("got %v, want the context error", err)
	}
}

func TestVerboseHooks(t *testing.T) {
	b := testkit.NewBackend(t)
	b.Handle("GET", "/cli/chats", testkit.Err(422, 1034, "Not enough credits"))
	b.Handle("GET", "/health", testkit.HealthOK("1"))
	b.Handle("DELETE", "/cli/user/u1/devices", testkit.Detail(403, "nope"))
	var requests int
	var errs [][2]int
	client := newClient(b, func(o *api.Options) {
		o.OnRequest = func() { requests++ }
		o.OnError = func(status, code int) { errs = append(errs, [2]int{status, code}) }
	})
	_, _ = client.Health(t.Context())
	_, _ = client.Chats(t.Context(), secretToken)
	_ = client.RevokeAll(t.Context(), secretToken, "u1")
	if requests != 3 {
		t.Errorf("OnRequest ran %d times, want 3", requests)
	}
	if len(errs) != 2 || errs[0] != [2]int{422, 1034} || errs[1] != [2]int{403, 0} {
		t.Errorf("OnError calls %v", errs)
	}
}

func TestVerboseHookIsSilentForTransportFailures(t *testing.T) {
	b := testkit.NewBackend(t)
	called := false
	client := newClient(b, func(o *api.Options) {
		o.BaseURL = "http://127.0.0.1:1"
		o.OnError = func(int, int) { called = true }
	})
	_, _ = client.Health(t.Context())
	if called {
		t.Error("OnError is for backend answers only")
	}
}
