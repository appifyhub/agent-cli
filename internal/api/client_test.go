package api_test

import (
	"context"
	"errors"
	"io"
	"net/http"
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
	_, err := client.Status(t.Context())
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
	entered := make(chan struct{})
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	client := api.New(api.Options{BaseURL: "http://backend.test", Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		close(entered)
		<-r.Context().Done()
		return nil, r.Context().Err()
	})})
	done := make(chan error, 1)
	go func() {
		_, err := client.Chats(ctx, secretToken)
		done <- err
	}()
	<-entered
	cancel()
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Errorf("got %v, want context.Canceled", err)
	}
}

func TestVerboseHooks(t *testing.T) {
	b := testkit.NewBackend(t)
	b.Handle("GET", "/cli/chats", testkit.Err(422, 1034, "Not enough credits"))
	b.Handle("GET", "/cli/status", testkit.StatusOK("1", "2.0.0"))
	b.Handle("DELETE", "/cli/user/u1/devices", testkit.Detail(403, "nope"))
	var requests int
	var errs [][2]int
	client := newClient(b, func(o *api.Options) {
		o.OnRequest = func() { requests++ }
		o.OnError = func(status, code int) { errs = append(errs, [2]int{status, code}) }
	})
	_, _ = client.Status(t.Context())
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
	_, _ = client.Status(t.Context())
	if called {
		t.Error("OnError is for backend answers only")
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

type failingBodyReader struct{}

func (failingBodyReader) Read([]byte) (int, error) { return 0, errors.New("response read failed") }

type trackedResponseBody struct {
	io.Reader
	closes int
}

func (b *trackedResponseBody) Close() error {
	b.closes++
	return nil
}

func unusableBody(content string, oversized bool) *trackedResponseBody {
	var tail io.Reader = failingBodyReader{}
	if oversized {
		tail = strings.NewReader(strings.Repeat(" ", (4<<20)+1))
	}
	return &trackedResponseBody{Reader: io.MultiReader(strings.NewReader(content), tail)}
}

func TestUnusableErrorBodiesPreserveStatus(t *testing.T) {
	for _, oversized := range []bool{false, true} {
		for _, c := range []struct {
			status int
			token  string
			kind   api.Kind
		}{
			{401, secretToken, api.KindTokenRejected},
			{429, secretToken, api.KindRateLimited},
			{503, secretToken, api.KindServerFault},
			{401, "", api.KindInvalidConnectKey},
		} {
			body := unusableBody(`{"error_code":1034,"message":"must not survive"}`, oversized)
			requests, hooks, sends := 0, 0, 0
			client := api.New(api.Options{
				BaseURL: "http://backend.test",
				Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
					sends++
					return &http.Response{StatusCode: c.status, Body: body}, nil
				}),
				OnRequest: func() { requests++ },
				OnError: func(status, code int) {
					hooks++
					if status != c.status || code != 0 {
						t.Errorf("hook status/code = %d/%d", status, code)
					}
				},
			})
			var err error
			if c.token == "" {
				_, err = client.Connect(t.Context(), connectRequest())
			} else {
				_, err = client.Chats(t.Context(), c.token)
			}
			got := wantKind(t, err, c.kind)
			if got.Status != c.status || got.Code != 0 || got.Message != "" || got.Detail || requests != 1 || hooks != 1 || sends != 1 || body.closes != 1 {
				t.Errorf("oversized=%v: error %#v, requests/hooks/sends/closes %d/%d/%d/%d", oversized, got, requests, hooks, sends, body.closes)
			}
		}
	}
}

func TestRequiredSuccessBodiesRemainStrict(t *testing.T) {
	for _, oversized := range []bool{false, true} {
		for _, endpoint := range []string{"connect", "chats", "status"} {
			content := `{"token":"t","expires_at":"2027-02-14T09:21:00Z","user_id":"u","user_display_name":"Neo"}`
			switch endpoint {
			case "chats":
				content = "[]"
			case "status":
				content = `{"status":"ok","service_version":"5.41.0","cli_version":null}`
			}
			body := unusableBody(content, oversized)
			requests := 0
			client := api.New(api.Options{BaseURL: "http://backend.test", Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
				requests++
				return &http.Response{StatusCode: http.StatusOK, Body: body}, nil
			})})
			var err error
			switch endpoint {
			case "connect":
				_, err = client.Connect(t.Context(), connectRequest())
			case "chats":
				_, err = client.Chats(t.Context(), secretToken)
			case "status":
				_, err = client.Status(t.Context())
			}
			kind := api.KindUnreachable
			if oversized {
				kind = api.KindUnexpected
			}
			got := wantKind(t, err, kind)
			if got.Status != 200 || requests != 1 || body.closes != 1 {
				t.Errorf("%s oversized=%v: error %#v, requests/closes %d/%d", endpoint, oversized, got, requests, body.closes)
			}
		}
	}
}

func TestStatusEndpoint(t *testing.T) {
	const valid = `{"status":"ok","service_version":"9.9.9","cli_version":"v1.2.3","extra":{"ignored":true}}`
	const success api.Kind = -1
	for _, tc := range []struct {
		name   string
		status int
		body   string
		want   api.Status
		kind   api.Kind
	}{
		{"version", 200, valid, api.Status{Status: "ok", Version: "9.9.9", CLIVersion: "v1.2.3"}, success},
		{"null CLI version", 200, `{"status":"degraded","service_version":"9.9.9","cli_version":null}`, api.Status{Status: "degraded", Version: "9.9.9"}, success},
		{"empty backend version", 200, `{"status":"ok","service_version":"","cli_version":null}`, api.Status{Status: "ok"}, success},
		{"missing status", 200, `{"service_version":"9.9.9","cli_version":null}`, api.Status{}, api.KindUnexpected},
		{"non-string status", 200, `{"status":false,"service_version":"9.9.9","cli_version":null}`, api.Status{}, api.KindUnexpected},
		{"null status", 200, `{"status":null,"service_version":"9.9.9","cli_version":null}`, api.Status{}, api.KindUnexpected},
		{"empty status", 200, `{"status":"","service_version":"9.9.9","cli_version":null}`, api.Status{}, api.KindUnexpected},
		{"missing backend version", 200, `{"status":"ok","cli_version":null}`, api.Status{}, api.KindUnexpected},
		{"non-string backend version", 200, `{"status":"ok","service_version":{},"cli_version":null}`, api.Status{}, api.KindUnexpected},
		{"null backend version", 200, `{"status":"ok","service_version":null,"cli_version":null}`, api.Status{}, api.KindUnexpected},
		{"missing CLI version", 200, `{"status":"ok","service_version":"9.9.9"}`, api.Status{Status: "ok", Version: "9.9.9"}, success},
		{"unknown uppercase field ignored", 200, `{"status":"ok","service_version":"9.9.9","CLI_VERSION":"v1.2.3"}`, api.Status{Status: "ok", Version: "9.9.9"}, success},
		{"non-string CLI version", 200, `{"status":"ok","service_version":"9.9.9","cli_version":123}`, api.Status{}, api.KindUnexpected},
		{"malformed", 200, `<html>`, api.Status{}, api.KindUnexpected},
		{"array", 200, `[]`, api.Status{}, api.KindUnexpected},
		{"truncated", 200, strings.TrimSuffix(valid, "}"), api.Status{}, api.KindUnexpected},
		{"trailing JSON", 200, valid + `{}`, api.Status{}, api.KindUnexpected},
		{"accepted", 202, valid, api.Status{}, api.KindUnexpected},
		{"no content", 204, "", api.Status{}, api.KindUnexpected},
		{"unauthorized", 401, valid, api.Status{}, api.KindFailure},
		{"forbidden", 403, valid, api.Status{}, api.KindFailure},
		{"not found", 404, valid, api.Status{}, api.KindFailure},
		{"rate limited", 429, valid, api.Status{}, api.KindRateLimited},
		{"service unavailable", 503, valid, api.Status{}, api.KindServerFault},
		{"redirect", 302, "", api.Status{}, api.KindUnexpected},
	} {
		t.Run(tc.name, func(t *testing.T) {
			requests := 0
			client := api.New(api.Options{BaseURL: "http://backend.test", Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
				requests++
				_, auth := r.Header["Authorization"]
				if r.Method != http.MethodGet || r.URL.RequestURI() != "/cli/status" || auth || r.Body != nil {
					t.Errorf("unexpected request: %s %s, auth=%v body=%v", r.Method, r.URL.RequestURI(), auth, r.Body != nil)
				}
				return &http.Response{
					StatusCode: tc.status,
					Header:     http.Header{"Location": {"/elsewhere"}},
					Body:       io.NopCloser(strings.NewReader(tc.body)),
				}, nil
			})})
			got, err := client.Status(t.Context())
			if tc.kind == success {
				if err != nil {
					t.Fatal(err)
				}
			} else if apiErr := wantKind(t, err, tc.kind); apiErr.Status != tc.status {
				t.Errorf("error status = %d, want %d", apiErr.Status, tc.status)
			}
			if got != tc.want || requests != 1 {
				t.Errorf("status = %+v, requests = %d; want %+v and one request", got, requests, tc.want)
			}
		})
	}
}

func TestStatusEndpointPropagatesCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	client := api.New(api.Options{BaseURL: "http://backend.test", Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		cancel()
		return nil, r.Context().Err()
	})})
	if got, err := client.Status(ctx); got != (api.Status{}) || !errors.Is(err, context.Canceled) {
		t.Errorf("got %+v, %v; want empty status and context.Canceled", got, err)
	}
}
