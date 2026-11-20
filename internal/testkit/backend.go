package testkit

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

// Request is one request the fake backend received.
type Request struct {
	Method   string
	Path     string
	RawQuery string
	Header   http.Header
	Body     string
}

// Response is what the fake backend answers.
type Response struct {
	Status      int
	Body        string
	ContentType string
	Header      map[string]string
	// Delay holds the answer back, to exercise timeouts.
	Delay time.Duration
}

// Backend is a fake of the agent's backend: an httptest server with scripted answers per route that
// records every request. A route nobody scripted answers like the real one does for an unknown path.
type Backend struct {
	server   *httptest.Server
	mu       sync.Mutex
	routes   map[string]func(Request) Response
	requests []Request
}

// NewBackend starts a fake backend and stops it when the test ends.
func NewBackend(t testing.TB) *Backend {
	t.Helper()
	b := &Backend{routes: map[string]func(Request) Response{}}
	b.server = httptest.NewServer(http.HandlerFunc(b.serve))
	t.Cleanup(b.server.Close)
	return b
}

// URL is the base address of the backend.
func (b *Backend) URL() string { return b.server.URL }

// Client is an HTTP client for the backend's address.
func (b *Backend) Client() *http.Client { return b.server.Client() }

// Handle scripts the answer of a route.
func (b *Backend) Handle(method, path string, r Response) {
	b.HandleFunc(method, path, func(Request) Response { return r })
}

// HandleFunc scripts a route with an answer computed from the request.
func (b *Backend) HandleFunc(method, path string, fn func(Request) Response) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.routes[method+" "+path] = fn
}

// Requests returns every request received so far, oldest first.
func (b *Backend) Requests() []Request {
	b.mu.Lock()
	defer b.mu.Unlock()
	return append([]Request(nil), b.requests...)
}

// Count is the number of requests received so far.
func (b *Backend) Count() int { return len(b.Requests()) }

// Last returns the most recent request, failing the test when there is none.
func (b *Backend) Last(t testing.TB) Request {
	t.Helper()
	all := b.Requests()
	if len(all) == 0 {
		t.Fatal("the backend received no request")
	}
	return all[len(all)-1]
}

func (b *Backend) serve(w http.ResponseWriter, r *http.Request) {
	raw, _ := io.ReadAll(r.Body)
	req := Request{Method: r.Method, Path: r.URL.Path, RawQuery: r.URL.RawQuery, Header: r.Header.Clone(), Body: string(raw)}
	b.mu.Lock()
	b.requests = append(b.requests, req)
	route := b.routes[r.Method+" "+r.URL.Path]
	b.mu.Unlock()

	resp := Detail(http.StatusNotFound, "Not Found")
	if route != nil {
		resp = route(req)
	}
	if resp.Delay > 0 {
		select {
		case <-time.After(resp.Delay):
		case <-r.Context().Done():
			return
		}
	}
	contentType := resp.ContentType
	if contentType == "" {
		contentType = "application/json"
	}
	w.Header().Set("Content-Type", contentType)
	for name, value := range resp.Header {
		w.Header().Set(name, value)
	}
	w.WriteHeader(resp.Status)
	_, _ = io.WriteString(w, resp.Body)
}

// JSON answers with a JSON body.
func JSON(status int, v any) Response {
	body, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return Response{Status: status, Body: string(body)}
}

// Err answers like the backend does for a service error: {error_code, message, emoji}, with the
// message carrying the [E<code>] marker and an emoji.
func Err(status, code int, message string) Response {
	return JSON(status, map[string]any{
		"error_code": code,
		"message":    fmt.Sprintf("[E%d] ⚠️ %s", code, message),
		"emoji":      "⚠️",
	})
}

// Detail answers with a framework-style {"detail": ...} body.
func Detail(status int, detail any) Response {
	return JSON(status, map[string]any{"detail": detail})
}

// Text answers with a non-JSON body, like a proxy or a gateway.
func Text(status int, body string) Response {
	return Response{Status: status, Body: body, ContentType: "text/plain"}
}

// HTML answers with an HTML page.
func HTML(status int) Response {
	return Response{Status: status, Body: "<html><body><h1>Bad Gateway</h1></body></html>", ContentType: "text/html"}
}

// ConnectOK is a successful connect answer; balance may be a number, nil, a string or anything else.
func ConnectOK(token string, expires time.Time, userID, name string, balance any) Response {
	return JSON(http.StatusOK, map[string]any{
		"token": token, "expires_at": expires.UTC().Format(time.RFC3339),
		"user_id": userID, "user_display_name": name, "credit_balance": balance,
	})
}

// ChatJSON is a chat as the backend lists it; a nil title becomes JSON null.
func ChatJSON(id, platform string, title *string, private bool) map[string]any {
	return map[string]any{"chat_id": id, "platform": platform, "title": title, "is_private": private}
}

// NotifyOK is a successful notification answer: the chat it went to and the balance after the charge.
func NotifyOK(chat map[string]any, balance any) Response {
	body := map[string]any{"credit_balance": balance}
	for k, v := range chat {
		body[k] = v
	}
	return JSON(http.StatusOK, body)
}

// HealthOK is the backend's health answer.
func HealthOK(version string) Response {
	return JSON(http.StatusOK, map[string]any{"status": "ok", "version": version})
}
