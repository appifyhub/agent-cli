// Package api talks to the agent's backend: one HTTP client per run, the endpoint calls with their
// response validation, and the classification of every error answer.
package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/appifyhub/agent-cli/internal/buildinfo"
)

const (
	// DefaultTimeout bounds every call.
	DefaultTimeout = 30 * time.Second
	// maxBody is the most that is read from any answer.
	maxBody = 4 << 20
)

// Options configure a Client.
type Options struct {
	// BaseURL is the validated API address; a trailing slash is ignored.
	BaseURL string
	// Build names the program in the User-Agent header.
	Build  buildinfo.Info
	GOOS   string
	GOARCH string
	// Transport defaults to the default HTTP transport, which honors the proxy environment.
	Transport http.RoundTripper
	// Timeout defaults to DefaultTimeout.
	Timeout time.Duration
	// OnRequest runs before each request is sent; OnError runs when the backend answers with an
	// error status. The client itself never writes anywhere.
	OnRequest func()
	OnError   func(status, code int)
}

// Client makes the calls to the backend. It never retries.
type Client struct {
	base      string
	userAgent string
	http      *http.Client
	onRequest func()
	onError   func(status, code int)
}

// New builds a Client.
func New(o Options) *Client {
	transport := o.Transport
	if transport == nil {
		transport = http.DefaultTransport
	}
	timeout := o.Timeout
	if timeout == 0 {
		timeout = DefaultTimeout
	}
	return &Client{
		base:      strings.TrimRight(o.BaseURL, "/"),
		userAgent: fmt.Sprintf("%s/%s (%s/%s)", o.Build.Name, o.Build.Version, o.GOOS, o.GOARCH),
		http: &http.Client{
			Transport: transport,
			Timeout:   timeout,
			CheckRedirect: func(*http.Request, []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
		onRequest: o.OnRequest,
		onError:   o.OnError,
	}
}

// call sends one request and returns the body of a 2xx answer. A non-2xx answer is a classified
// *Error; a transport failure is KindUnreachable. token is sent as a bearer token when not empty.
func (c *Client) call(ctx context.Context, method, path, token string, payload any) (status int, body []byte, err error) {
	var reader io.Reader
	if payload != nil {
		encoded, err := json.Marshal(payload)
		if err != nil {
			return 0, nil, err
		}
		reader = bytes.NewReader(encoded)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.base+path, reader)
	if err != nil {
		return 0, nil, &Error{Kind: KindUnreachable, Cause: err}
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", c.userAgent)
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	if c.onRequest != nil {
		c.onRequest()
	}
	resp, err := c.http.Do(req)
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil && errors.Is(err, ctxErr) {
			return 0, nil, ctxErr
		}
		return 0, nil, &Error{Kind: KindUnreachable, Cause: err}
	}
	defer func() { _ = resp.Body.Close() }()

	body, err = io.ReadAll(io.LimitReader(resp.Body, maxBody+1))
	if err != nil || len(body) > maxBody {
		body = nil
	}
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		if err != nil {
			return resp.StatusCode, nil, &Error{Kind: KindUnreachable, Status: resp.StatusCode, Cause: err}
		}
		if body == nil {
			return resp.StatusCode, nil, unexpected(resp.StatusCode)
		}
		return resp.StatusCode, body, nil
	}
	apiErr := errorFromResponse(token, resp.StatusCode, body)
	if c.onError != nil {
		c.onError(apiErr.Status, apiErr.Code)
	}
	return resp.StatusCode, nil, apiErr
}
