package cli_test

import (
	"context"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/appifyhub/agent-cli/internal/buildinfo"
	"github.com/appifyhub/agent-cli/internal/cli"
	"github.com/appifyhub/agent-cli/internal/config"
	"github.com/appifyhub/agent-cli/internal/term"
	"github.com/appifyhub/agent-cli/internal/testkit"
)

const (
	statusTestURL     = "https://agent.example.test"
	statusNewer       = `{"status":"ok","service_version":"5.41.0","cli_version":"v1.10.0"}`
	statusOK          = "\n  Backend status    ok\n  Backend version   5.41.0\n"
	statusUnavailable = "\n  Backend status    unavailable\n  Backend version   unavailable\n  Latest CLI        unavailable\n"
	statusUpdate      = "  CLI update        update available · https://github.com/appifyhub/agent-cli/releases\n"
	statusChecked     = "  Last checked      2026-11-14 10:21\n"
	statusNever       = "  Last checked      never checked\n"
)

func remoteStatusDeps(t *testing.T, current string, ready bool) (cli.Deps, string) {
	t.Helper()
	env := map[string]string{
		config.EnvHome:   filepath.Join(t.TempDir(), "state"),
		config.EnvAPIURL: statusTestURL,
	}
	var raw string
	if ready {
		raw = testkit.RawToken(map[string]any{"alg": "HS256"}, map[string]any{"sub": "u1"})
		env[config.EnvToken] = raw
	}
	return testkit.NewDeps(testkit.WithEnv(env), testkit.WithBuild(buildinfo.Info{Version: current})), raw
}

type statusResponse struct {
	code int
	json string
	err  string
	read bool
}

func (r statusResponse) answer() (*http.Response, error) {
	if r.err != "" && !r.read {
		return nil, errors.New(r.err)
	}
	body := io.NopCloser(strings.NewReader(r.json))
	if r.read {
		body = io.NopCloser(statusReader(func([]byte) (int, error) { return 0, errors.New(r.err) }))
	}
	header := make(http.Header)
	if r.code == http.StatusFound {
		header.Set("Location", "/must-not-follow")
	}
	return &http.Response{StatusCode: r.code, Header: header, Body: body}, nil
}

func statusFixtureTransport(t *testing.T, response statusResponse) http.RoundTripper {
	t.Helper()
	var requests atomic.Int32
	t.Cleanup(func() {
		if got := requests.Load(); got != 1 {
			t.Errorf("got %d backend requests, want one", got)
		}
	})
	return commandRoundTrip(func(r *http.Request) (*http.Response, error) {
		requests.Add(1)
		checkStatusRequest(t, r)
		return response.answer()
	})
}

func checkStatusRequest(t *testing.T, req *http.Request) {
	t.Helper()
	if req.Method != http.MethodGet || req.URL.RequestURI() != "/cli/status" || req.URL.User != nil || req.Body != nil {
		t.Errorf("unexpected status request: %s %s", req.Method, req.URL)
	}
	if _, auth := req.Header["Authorization"]; auth || req.Header.Get("Cookie") != "" {
		t.Error("public status sent credentials")
	}
}

func TestStatusRemoteReport(t *testing.T) {
	for _, tc := range []struct {
		name string
		resp statusResponse
		want string
	}{
		{"combined", statusResponse{code: 200, json: statusNewer}, statusOK +
			"  Latest CLI        v1.10.0\n" + statusUpdate + "  Response          0 ms\n"},
		{
			"degraded with null CLI version",
			statusResponse{code: 200, json: `{"status":"degraded","service_version":"5.41.0","cli_version":null}`},
			"\n  Backend status    degraded\n  Backend version   5.41.0\n  Latest CLI        unavailable\n  Response          0 ms\n",
		},
		{
			"terminal cleaning and ignored fields",
			statusResponse{code: 200, json: `{"status":"de\u001b[31mgraded","service_version":"5.41.0\u001b[2J","cli_version":"v1.10.0\u202e","region":"eu","extra":null}`},
			"\n  Backend status    degraded\n  Backend version   5.41.0\n  Latest CLI        v1.10.0\n  Response          0 ms\n",
		},
		{"503", statusResponse{code: 503, json: "raw server detail"}, statusUnavailable},
		{"429", statusResponse{code: 429, json: "raw server detail"}, statusUnavailable},
		{"401", statusResponse{code: 401, json: statusNewer}, statusUnavailable},
		{"404 on an older backend", statusResponse{code: 404, json: statusNewer}, statusUnavailable},
		{"malformed", statusResponse{code: 200, json: "<html>raw server detail</html>"}, statusUnavailable},
		{
			"missing CLI version",
			statusResponse{code: 200, json: `{"status":"ok","service_version":"5.41.0"}`},
			statusOK + "  Latest CLI        unavailable\n  Response          0 ms\n",
		},
		{"redirect", statusResponse{code: 302}, statusUnavailable},
		{"transport", statusResponse{err: "raw transport detail"}, statusUnavailable},
		{"body read", statusResponse{code: 200, err: "raw body detail", read: true}, statusUnavailable},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, ready := range []bool{true, false} {
				deps, raw := remoteStatusDeps(t, "1.9.9", ready)
				local := testkit.Run(t, deps, "status", "--offline")
				deps.HTTP = statusFixtureTransport(t, tc.resp)
				got := testkit.Run(t, deps, "status")
				want := strings.TrimSuffix(local.Stdout, statusNever) + tc.want + statusChecked
				if got.Code != local.Code || got.Stdout != want || got.Stderr != "" {
					t.Errorf("ready=%t: got %+v; want exit %d, stdout %q and no stderr", ready, got, local.Code, want)
				}
				if ready {
					testkit.AssertNoSecret(t, raw, got.Stdout, got.Stderr)
				}
			}
		})
	}
}

func TestStatusComparesSemanticVersions(t *testing.T) {
	for _, tc := range []struct {
		local, remote string
		want          string
	}{
		{"1.2.3", "1.2.3", "  CLI update        latest\n"},
		{"1.2.3", "1.2.4", statusUpdate},
		{"1.2.4", "1.2.3", ""},
		{"1.2.3", "v1.2.3", "  CLI update        latest\n"},
		{"1.2.3+local", "1.2.3+remote", "  CLI update        latest\n"},
		{"1.2.3-rc.1", "1.2.3", statusUpdate},
		{"1.2.3", "1.2.3-rc.1", ""},
		{"dev", "1.2.3", ""},
	} {
		t.Run(tc.local+"/"+tc.remote, func(t *testing.T) {
			deps, _ := remoteStatusDeps(t, tc.local, true)
			deps.HTTP = statusFixtureTransport(t, statusResponse{
				code: http.StatusOK,
				json: `{"status":"ok","service_version":"5.41.0","cli_version":"` + tc.remote + `"}`,
			})
			got := testkit.Run(t, deps, "status")
			if got.Code != cli.ExitOK || got.Stderr != "" || !strings.Contains(got.Stdout, "  Local CLI         "+tc.local+"\n") ||
				!strings.Contains(got.Stdout, "  Latest CLI        "+tc.remote+"\n"+tc.want+"  Response") {
				t.Errorf("status = %+v; want both versions followed by %q", got, tc.want)
			}
		})
	}
}

func TestStatusColorsOnlySignalRows(t *testing.T) {
	for _, disable := range []bool{false, true} {
		deps, _ := remoteStatusDeps(t, "1.9.9", true)
		local := strings.TrimSuffix(testkit.Run(t, deps, "status", "--offline").Stdout, statusNever)
		deps.Term = &testkit.FakeTerm{TTYOut: true, ANSIOK: true}
		if disable {
			getenv := deps.Getenv
			deps.Getenv = func(key string) string {
				if key == "NO_COLOR" {
					return "1"
				}
				return getenv(key)
			}
		}
		deps.HTTP = statusFixtureTransport(t, statusResponse{code: http.StatusOK, json: statusNewer})
		got := testkit.Run(t, deps, "status")
		want := local + "\n" + term.Paint("  Backend status    ok", term.Success, !disable) +
			"\n  Backend version   5.41.0\n  Latest CLI        v1.10.0\n" +
			term.Paint(strings.TrimSuffix(statusUpdate, "\n"), term.Warning, !disable) +
			"\n  Response          0 ms\n" + statusChecked
		if got.Code != cli.ExitOK || got.Stdout != want || got.Stderr != "" {
			t.Errorf("NO_COLOR=%v: got %+v; want %q", disable, got, want)
		}
	}
}

type statusReader func([]byte) (int, error)

func (r statusReader) Read(p []byte) (int, error) { return r(p) }

type statusBody struct {
	io.Reader
	close func()
}

func (b statusBody) Close() error {
	b.close()
	return nil
}

type statusWriter func([]byte) (int, error)

func (w statusWriter) Write(p []byte) (int, error) { return w(p) }

type statusOutput struct {
	mu  sync.Mutex
	log strings.Builder
}

func (o *statusOutput) writer(stream string) io.Writer {
	return statusWriter(func(p []byte) (int, error) {
		o.mu.Lock()
		defer o.mu.Unlock()
		o.log.WriteString(stream)
		o.log.WriteByte(':')
		return o.log.Write(p)
	})
}

func (o *statusOutput) String() string {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.log.String()
}

func statusSignal[T any](t *testing.T, ctx context.Context, ch <-chan T) T {
	t.Helper()
	select {
	case value := <-ch:
		return value
	case <-ctx.Done():
		t.Fatalf("waiting for status test signal: %v", ctx.Err())
		var zero T
		return zero
	}
}

func TestStatusWritesLocalReportBeforeOneBoundedRequest(t *testing.T) {
	deps, raw := remoteStatusDeps(t, "1.9.9", true)
	local := strings.TrimSuffix(testkit.Run(t, deps, "status", "--offline").Stdout, statusNever)
	var out statusOutput
	deps.Stdout, deps.Stderr = out.writer("stdout"), out.writer("stderr")
	ctx, cancel := context.WithTimeout(t.Context(), 6*time.Second)
	defer cancel()
	started := make(chan *http.Request, 1)
	release := make(chan struct{})
	var requests atomic.Int32
	var elapsed atomic.Int64
	base := deps.Now()
	deps.Now = func() time.Time { return base.Add(time.Duration(elapsed.Load())) }
	deps.HTTP = commandRoundTrip(func(req *http.Request) (*http.Response, error) {
		if requests.Add(1) != 1 {
			return nil, errors.New("repeated status request")
		}
		started <- req
		select {
		case <-release:
		case <-req.Context().Done():
			return nil, req.Context().Err()
		}
		return &http.Response{StatusCode: http.StatusOK, Body: statusBody{
			Reader: strings.NewReader(statusNewer),
			close:  func() { elapsed.Store(int64(123 * time.Millisecond)) },
		}}, nil
	})
	done := make(chan int, 1)
	earliest := time.Now().Add(3 * time.Second)
	go func() { done <- cli.Run(ctx, deps, []string{"status", "--verbose"}) }()
	req := statusSignal(t, ctx, started)
	latest := time.Now().Add(3 * time.Second)
	if deadline, ok := req.Context().Deadline(); !ok || deadline.Before(earliest) || deadline.After(latest) {
		t.Errorf("request deadline %v; want between %v and %v", deadline, earliest, latest)
	}
	checkStatusRequest(t, req)
	if req.URL.Scheme+"://"+req.URL.Host != statusTestURL || req.Header.Get("User-Agent") != "agent-cli/1.9.9 (linux/amd64)" {
		t.Errorf("wrong address or User-Agent for status request: %s, %q", req.URL, req.Header.Get("User-Agent"))
	}
	if got := out.String(); got != "stdout:"+local {
		t.Errorf("before the response, output = %q; want local report alone", got)
	}
	close(release)
	code := statusSignal(t, ctx, done)
	want := "stdout:" + local + "stdout:" + statusOK + "  Latest CLI        v1.10.0\n" +
		statusUpdate + "  Response          123 ms\n" + statusChecked + "stderr:Calling backend…\n"
	if code != cli.ExitOK || out.String() != want || requests.Load() != 1 || req.Context().Err() == nil {
		t.Errorf("exit %d, output %q, requests %d, context error %v; want exit 0, one released request and %q",
			code, out.String(), requests.Load(), req.Context().Err(), want)
	}
	testkit.AssertNoSecret(t, raw, out.String())
	if deps.Getenv(config.EnvToken) != raw {
		t.Error("environment token changed")
	}
}

func TestStatusVerboseUsesOnlySafeDiagnostics(t *testing.T) {
	deps, raw := remoteStatusDeps(t, "1.9.9", true)
	deps.HTTP = statusFixtureTransport(t, statusResponse{
		code: http.StatusServiceUnavailable,
		json: `{"error_code":1234,"message":"secret response: ` + raw + `"}`,
	})
	got := testkit.Run(t, deps, "status", "--verbose")
	const want = "Calling backend…\n  (HTTP 503, code 1234)\n"
	if got.Code != cli.ExitOK || !strings.HasSuffix(got.Stdout, statusUnavailable+statusChecked) || got.Stderr != want {
		t.Errorf("got %+v; want ready, fallback and stderr %q", got, want)
	}
	testkit.AssertNoSecret(t, raw, got.Stdout, got.Stderr)
}

func TestStatusDeadlineIncludesBodyRead(t *testing.T) {
	t.Parallel()
	deps, _ := remoteStatusDeps(t, "1.9.9", false)
	var requests, closed int
	deps.HTTP = commandRoundTrip(func(req *http.Request) (*http.Response, error) {
		requests++
		checkStatusRequest(t, req)
		return &http.Response{StatusCode: http.StatusOK, Body: statusBody{
			Reader: statusReader(func([]byte) (int, error) {
				<-req.Context().Done()
				err := req.Context().Err()
				if !errors.Is(err, context.DeadlineExceeded) {
					t.Errorf("body released with %v; want deadline exceeded", err)
				}
				return 0, err
			}),
			close: func() { closed++ },
		}}, nil
	})
	var stdout, stderr strings.Builder
	deps.Stdout, deps.Stderr = &stdout, &stderr
	ctx, cancel := context.WithTimeout(t.Context(), 6*time.Second)
	defer cancel()
	code := cli.Run(ctx, deps, []string{"status"})
	if ctx.Err() != nil || requests != 1 || closed != 1 || code != cli.ExitNotConnected ||
		!strings.HasSuffix(stdout.String(), statusUnavailable+statusChecked) || stderr.Len() != 0 {
		t.Errorf("status: exit %d, stdout %q, stderr %q, caller error %v, requests %d, bodies closed %d",
			code, stdout.String(), stderr.String(), ctx.Err(), requests, closed)
	}
	if offline := testkit.Run(t, deps, "status", "--offline"); !strings.HasSuffix(offline.Stdout, statusChecked) || requests != 1 {
		t.Errorf("offline = %+v, requests %d; want saved timeout check with no second request", offline, requests)
	}
}

func TestStatusPropagatesCallerCancellationDuringBodyRead(t *testing.T) {
	deps, _ := remoteStatusDeps(t, "1.9.9", true)
	local := strings.TrimSuffix(testkit.Run(t, deps, "status", "--offline").Stdout, statusNever)
	var out statusOutput
	deps.Stdout, deps.Stderr = out.writer("stdout"), out.writer("stderr")
	entered := make(chan struct{}, 1)
	var closeCount atomic.Int32
	deps.HTTP = commandRoundTrip(func(req *http.Request) (*http.Response, error) {
		checkStatusRequest(t, req)
		return &http.Response{StatusCode: http.StatusOK, Body: statusBody{
			Reader: statusReader(func([]byte) (int, error) {
				entered <- struct{}{}
				<-req.Context().Done()
				return 0, req.Context().Err()
			}),
			close: func() { closeCount.Add(1) },
		}}, nil
	})
	guard, stop := context.WithTimeout(t.Context(), 6*time.Second)
	defer stop()
	ctx, cancel := context.WithCancel(guard)
	defer cancel()
	done := make(chan int, 1)
	go func() { done <- cli.Run(ctx, deps, []string{"status", "--verbose"}) }()
	statusSignal(t, guard, entered)
	cancel()
	code := statusSignal(t, guard, done)
	want := "stdout:" + local + "stderr:error: operation cancelled\n"
	if code != cli.ExitFailure || out.String() != want || closeCount.Load() != 1 {
		t.Errorf("exit %d, output %q, %d bodies closed; want exit 1, output %q and the body closed",
			code, out.String(), closeCount.Load(), want)
	}
	if _, err := os.Stat(deps.Getenv(config.EnvHome)); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("cancelled check created state: %v", err)
	}
}

func TestStatusPropagatesRemoteWriteFailures(t *testing.T) {
	for _, tc := range []struct {
		name   string
		resp   statusResponse
		stderr bool
		verb   bool
		match  string
	}{
		{"status report", statusResponse{code: 200, json: statusNewer}, false, false, statusOK},
		{"unavailable fallback", statusResponse{code: 503, json: "down"}, false, false, statusUnavailable},
		{"verbose diagnostics", statusResponse{code: 200, json: statusNewer}, true, true, "Calling backend…"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			deps, _ := remoteStatusDeps(t, "1.9.9", true)
			deps.HTTP = statusFixtureTransport(t, tc.resp)
			var stdout, stderr strings.Builder
			var failed bool
			fail := func(b *strings.Builder) io.Writer {
				return statusWriter(func(p []byte) (int, error) {
					if strings.HasPrefix(string(p), tc.match) {
						failed = true
						return 0, errors.New("write sentinel")
					}
					return b.Write(p)
				})
			}
			deps.Stdout, deps.Stderr = &stdout, &stderr
			if tc.stderr {
				deps.Stderr = fail(&stderr)
			} else {
				deps.Stdout = fail(&stdout)
			}
			args := []string{"status"}
			if tc.verb {
				args = append(args, "--verbose")
			}
			if code := cli.Run(t.Context(), deps, args); code != cli.ExitFailure || !failed || !strings.HasPrefix(stderr.String(), "error: ") ||
				strings.Contains(stderr.String(), "write sentinel") {
				t.Errorf("exit %d, failed write %v, stdout %q, stderr %q", code, failed, stdout.String(), stderr.String())
			}
			if offline := testkit.Run(t, deps, "status", "--offline"); !strings.HasSuffix(offline.Stdout, statusChecked) {
				t.Errorf("completed check was not saved before output failed: %+v", offline)
			}
		})
	}
}

func TestStatusRemembersCompletedChecksPerBackend(t *testing.T) {
	deps, raw := remoteStatusDeps(t, "1.9.9", true)
	getenv := deps.Getenv
	home := getenv(config.EnvHome)
	address := statusTestURL
	deps.Getenv = func(key string) string {
		if key == config.EnvAPIURL {
			return address
		}
		return getenv(key)
	}
	now := deps.Now()
	deps.Now = func() time.Time { return now }
	var requests []string
	failLookup := false
	deps.HTTP = commandRoundTrip(func(req *http.Request) (*http.Response, error) {
		requests = append(requests, req.URL.String())
		now = now.Add(2 * time.Minute)
		if failLookup {
			return nil, errors.New("offline backend")
		}
		return (statusResponse{code: 200, json: statusNewer}).answer()
	})
	offline := func(want string) {
		t.Helper()
		count, before := len(requests), statusSnapshot(t, home)
		got := testkit.Run(t, deps, "status", "--offline", "--verbose")
		if got.Code != cli.ExitOK || !strings.HasSuffix(got.Stdout, want) || strings.Contains(got.Stdout, "Latest CLI") ||
			got.Stderr != "" || len(requests) != count || statusSnapshot(t, home) != before {
			t.Fatalf("offline = %+v; want timestamp %q, zero network and unchanged state", got, want)
		}
	}
	offline(statusNever)
	if _, err := os.Stat(home); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("offline status created the state directory: %v", err)
	}
	first := testkit.Run(t, deps, "status")
	const firstTime = "  Last checked      2026-11-14 10:23\n"
	if first.Code != cli.ExitOK || !strings.HasSuffix(first.Stdout, firstTime) {
		t.Fatalf("first completion = %+v; want %q", first, firstTime)
	}
	address += "/"
	offline(firstTime)
	address = statusTestURL + "/other"
	offline(statusNever)
	failLookup = true
	failed := testkit.Run(t, deps, "status")
	const secondTime = "  Last checked      2026-11-14 10:25\n"
	if failed.Code != cli.ExitOK || !strings.HasSuffix(failed.Stdout, statusUnavailable+secondTime) || failed.Stderr != "" {
		t.Fatalf("failed completion = %+v; want unavailable and %q", failed, secondTime)
	}
	offline(secondTime)
	address = statusTestURL
	offline(firstTime)
	refreshed := testkit.Run(t, deps, "status")
	const thirdTime = "  Last checked      2026-11-14 10:27\n"
	if refreshed.Code != cli.ExitOK || !strings.HasSuffix(refreshed.Stdout, statusUnavailable+thirdTime) {
		t.Fatalf("refreshed completion = %+v; want unavailable and %q", refreshed, thirdTime)
	}
	offline(thirdTime)
	if len(requests) != 3 || requests[0] != statusTestURL+"/cli/status" ||
		requests[1] != statusTestURL+"/other/cli/status" || requests[2] != requests[0] {
		t.Fatalf("unexpected online requests: %v", requests)
	}
	if _, err := os.Stat(config.Path(home)); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("status wrote config.toml: %v", err)
	}
	testkit.AssertNoSecret(t, raw, statusSnapshot(t, home))
}

func TestStatusPropagatesCheckTimeWriteFailure(t *testing.T) {
	deps, _ := remoteStatusDeps(t, "1.9.9", true)
	home := deps.Getenv(config.EnvHome)
	hits := 0
	deps.HTTP = commandRoundTrip(func(req *http.Request) (*http.Response, error) {
		checkStatusRequest(t, req)
		hits++
		if err := os.WriteFile(home, []byte("not a directory"), 0o600); err != nil {
			t.Fatal(err)
		}
		return (statusResponse{code: 200, json: statusNewer}).answer()
	})
	got := testkit.Run(t, deps, "status")
	if got.Code != cli.ExitFailure || hits != 1 || !strings.Contains(got.Stderr, home) || strings.Contains(got.Stdout, "Last checked") {
		t.Errorf("status = %+v, requests %d; want local write failure without a saved check", got, hits)
	}
}
