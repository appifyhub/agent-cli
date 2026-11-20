package api_test

import (
	"errors"
	"testing"
	"time"

	"github.com/appifyhub/agent-cli/internal/api"
	"github.com/appifyhub/agent-cli/internal/buildinfo"
	"github.com/appifyhub/agent-cli/internal/testkit"
)

const secretToken = "header.payload.signature"

var expiry = time.Date(2027, 2, 14, 9, 21, 0, 0, time.UTC)

func newClient(b *testkit.Backend, mutate ...func(*api.Options)) *api.Client {
	o := api.Options{
		BaseURL:   b.URL(),
		Build:     buildinfo.Resolve(buildinfo.Info{Name: "agent-cli", Version: "0.1.2"}),
		GOOS:      "linux",
		GOARCH:    "amd64",
		Transport: b.Client().Transport,
	}
	for _, m := range mutate {
		m(&o)
	}
	return api.New(o)
}

func asError(t *testing.T, err error) *api.Error {
	t.Helper()
	var apiErr *api.Error
	if !errors.As(err, &apiErr) {
		t.Fatalf("got %v, want an *api.Error", err)
	}
	return apiErr
}

func wantKind(t *testing.T, err error, kind api.Kind) *api.Error {
	t.Helper()
	apiErr := asError(t, err)
	if apiErr.Kind != kind {
		t.Fatalf("kind %v, want %v (%v)", apiErr.Kind, kind, err)
	}
	return apiErr
}

func ptr[T any](v T) *T { return &v }
