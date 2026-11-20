package api_test

import (
	"testing"

	"github.com/appifyhub/agent-cli/internal/api"
	"github.com/appifyhub/agent-cli/internal/testkit"
)

func TestHealthy(t *testing.T) {
	b := testkit.NewBackend(t)
	b.Handle("GET", "/health", testkit.HealthOK("5.41.0"))
	got, err := newClient(b).Health(t.Context())
	if err != nil || got.Status != "ok" || got.Version != "5.41.0" || len(got.Extra) != 0 {
		t.Fatalf("got %+v, %v", got, err)
	}
	r := b.Last(t)
	if _, present := r.Header["Authorization"]; present || r.Body != "" || r.RawQuery != "" {
		t.Errorf("request %+v", r)
	}
}

func TestHealthKeepsScalarsInAnswerOrderAndDropsNestedValues(t *testing.T) {
	b := testkit.NewBackend(t)
	b.Handle("GET", "/health", testkit.Response{Status: 200, Body: `{"zone":"eu","status":"ok","uptime_seconds":86400,"checks":{"db":"ok"},"version":"1.2.3","debug":false,"load":0.25,"tags":["a"],"note":null}`})
	got, err := newClient(b).Health(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	want := []api.Field{{"zone", "eu"}, {"uptime_seconds", "86400"}, {"debug", "false"}, {"load", "0.25"}}
	if got.Status != "ok" || got.Version != "1.2.3" || len(got.Extra) != len(want) {
		t.Fatalf("got %+v", got)
	}
	for i, f := range want {
		if got.Extra[i] != f {
			t.Errorf("extra[%d] = %+v, want %+v", i, got.Extra[i], f)
		}
	}
}

func TestHealthWithoutAVersion(t *testing.T) {
	b := testkit.NewBackend(t)
	b.Handle("GET", "/health", testkit.JSON(200, map[string]any{"status": "ok"}))
	if got, err := newClient(b).Health(t.Context()); err != nil || got.Version != "" {
		t.Errorf("got %+v, %v", got, err)
	}
}

func TestHealthVersionThatIsNotAScalarIsIgnored(t *testing.T) {
	b := testkit.NewBackend(t)
	b.Handle("GET", "/health", testkit.JSON(200, map[string]any{"status": "ok", "version": map[string]any{"a": 1}}))
	if got, err := newClient(b).Health(t.Context()); err != nil || got.Version != "" || len(got.Extra) != 0 {
		t.Errorf("got %+v, %v", got, err)
	}
}

func TestHealthReportsAnyStatusText(t *testing.T) {
	b := testkit.NewBackend(t)
	b.Handle("GET", "/health", testkit.JSON(200, map[string]any{"status": "degraded", "version": "1"}))
	if got, err := newClient(b).Health(t.Context()); err != nil || got.Status != "degraded" {
		t.Errorf("got %+v, %v", got, err)
	}
}

func TestHealthRejectsOtherAnswers(t *testing.T) {
	for name, resp := range map[string]testkit.Response{
		"missing status": testkit.JSON(200, map[string]any{"version": "1"}),
		"numeric status": testkit.JSON(200, map[string]any{"status": 1}),
		"null status":    testkit.JSON(200, map[string]any{"status": nil}),
		"empty status":   testkit.JSON(200, map[string]any{"status": ""}),
		"web page":       testkit.HTML(200),
		"plain text":     testkit.Text(200, "OK"),
		"array":          testkit.JSON(200, []any{"ok"}),
		"empty body":     {Status: 200},
		"truncated":      {Status: 200, Body: `{"status":"ok"`},
	} {
		b := testkit.NewBackend(t)
		b.Handle("GET", "/health", resp)
		_, err := newClient(b).Health(t.Context())
		if apiErr := wantKind(t, err, api.KindUnexpected); apiErr.Status != 200 {
			t.Errorf("%s: %+v", name, apiErr)
		}
	}
}

func TestHealthErrorStatuses(t *testing.T) {
	for status, want := range map[int]api.Kind{503: api.KindServerFault, 429: api.KindRateLimited, 404: api.KindFailure, 401: api.KindFailure, 403: api.KindFailure} {
		b := testkit.NewBackend(t)
		b.Handle("GET", "/health", testkit.Text(status, "nope"))
		_, err := newClient(b).Health(t.Context())
		_ = wantKind(t, err, want)
	}
}
