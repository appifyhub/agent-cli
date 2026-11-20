package tools_test

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
)

const (
	stagingURL    = "https://staging.agent.cloud.appifyhub.com"
	productionURL = "https://agent.cloud.appifyhub.com"
	localURL      = "http://localhost"
)

func skipOnWindows(t *testing.T) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("the shell tools are not run on Windows")
	}
}

func write(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func read(t *testing.T, path string) string {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(content)
}

// copyTools copies the named scripts into root/tools.
func copyTools(t *testing.T, root string, names ...string) {
	t.Helper()
	for _, name := range names {
		write(t, filepath.Join(root, "tools", name), read(t, name))
	}
}

// run executes a script with sh in dir and returns its exit code and combined output.
func run(t *testing.T, dir string, env []string, script string, args ...string) (code int, output string) {
	t.Helper()
	cmd := exec.CommandContext(t.Context(), "sh", append([]string{script}, args...)...)
	cmd.Dir = dir
	for _, kv := range os.Environ() {
		if !strings.HasPrefix(kv, "THE_AGENT_CLI_") {
			cmd.Env = append(cmd.Env, kv)
		}
	}
	cmd.Env = append(cmd.Env, "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_SYSTEM=/dev/null")
	cmd.Env = append(cmd.Env, env...)
	out, err := cmd.CombinedOutput()
	var exitErr *exec.ExitError
	switch {
	case err == nil:
	case errors.As(err, &exitErr):
		code = exitErr.ExitCode()
	default:
		t.Fatal(err)
	}
	return code, string(out)
}

func git(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.CommandContext(t.Context(), "git", append([]string{"-c", "user.name=test", "-c", "user.email=test@example.test"}, args...)...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_SYSTEM=/dev/null")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

// bump_version

func bumpLayout(t *testing.T, goMod bool, version *string) string {
	t.Helper()
	skipOnWindows(t)
	root := t.TempDir()
	copyTools(t, root, "bump_version", "messages")
	if goMod {
		write(t, filepath.Join(root, "go.mod"), "module example.test\n")
	}
	if version != nil {
		write(t, filepath.Join(root, "VERSION"), *version)
	}
	return root
}

func TestBumpSucceeds(t *testing.T) {
	cases := []struct{ kind, from, to string }{
		{"patch", "0.1.0", "0.1.1"},
		{"minor", "0.1.4", "0.2.0"},
		{"major", "1.4.7", "2.0.0"},
		{"patch", "0.9.9", "0.9.10"},
	}
	for _, c := range cases {
		version := c.from + "\n"
		root := bumpLayout(t, true, &version)
		code, out := run(t, root, nil, "tools/bump_version", c.kind)
		if code != 0 || !strings.Contains(out, "Version bumped: "+c.from+" -> "+c.to) {
			t.Errorf("%s %s: exit %d, output %q", c.kind, c.from, code, out)
		}
		if got := read(t, filepath.Join(root, "VERSION")); got != c.to+"\n" {
			t.Errorf("%s %s: VERSION = %q, want %q", c.kind, c.from, got, c.to+"\n")
		}
	}
}

func TestBumpFailuresLeaveVersionUntouched(t *testing.T) {
	good := "0.1.0\n"
	cases := []struct {
		name    string
		version string
		args    []string
	}{
		{"unknown argument", good, []string{"sideways"}},
		{"missing argument", good, nil},
		{"malformed version", "0.1\n", []string{"patch"}},
		{"prefixed version", "v1\n", []string{"patch"}},
		{"version with a suffix", "0.1.0-beta\n", []string{"patch"}},
		{"two lines", "0.1.0\n0.1.1\n", []string{"patch"}},
		{"empty version", "", []string{"patch"}},
	}
	for _, c := range cases {
		root := bumpLayout(t, true, &c.version)
		if code, out := run(t, root, nil, "tools/bump_version", c.args...); code == 0 {
			t.Errorf("%s: expected a failure, got %q", c.name, out)
		}
		if got := read(t, filepath.Join(root, "VERSION")); got != c.version {
			t.Errorf("%s: VERSION changed to %q", c.name, got)
		}
	}
}

func TestBumpMissingVersionFile(t *testing.T) {
	root := bumpLayout(t, true, nil)
	code, out := run(t, root, nil, "tools/bump_version", "patch")
	if code == 0 || !strings.Contains(out, "Missing 'VERSION'") {
		t.Errorf("exit %d, output %q", code, out)
	}
	if _, err := os.Stat(filepath.Join(root, "VERSION")); !os.IsNotExist(err) {
		t.Errorf("VERSION must not be created: %v", err)
	}
}

func TestBumpRefusesToRunOutsideTheProjectRoot(t *testing.T) {
	version := "0.1.0\n"
	root := bumpLayout(t, false, &version)
	code, out := run(t, root, nil, "tools/bump_version", "patch")
	if code == 0 || !strings.Contains(out, "go.mod") {
		t.Errorf("exit %d, output %q", code, out)
	}
	if got := read(t, filepath.Join(root, "VERSION")); got != version {
		t.Errorf("VERSION changed to %q", got)
	}
}

// build

func TestBuildVariants(t *testing.T) {
	skipOnWindows(t)
	t.Setenv("THE_AGENT_CLI_BUILD_API_URL", "https://ambient.example.test")
	repo, err := filepath.Abs("..")
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		variant, url string
		env          []string
	}{
		{"abot", productionURL, nil},
		{"abot-dev", stagingURL, nil},
		{"agent-cli", localURL, nil},
		{"agent-cli-dev", localURL, nil},
		{"abot", "https://from-env.test", []string{"THE_AGENT_CLI_BUILD_API_URL=https://from-env.test"}},
	}
	for _, c := range cases {
		out := t.TempDir()
		if code, output := run(t, repo, append([]string{"OUT_DIR=" + out}, c.env...), "tools/build", c.variant); code != 0 {
			t.Fatalf("%s: exit %d\n%s", c.variant, code, output)
		}
		binary := filepath.Join(out, c.variant)
		version, err := exec.CommandContext(t.Context(), binary, "--version").Output()
		if err != nil {
			t.Fatalf("%s: %v", c.variant, err)
		}
		if !strings.HasPrefix(string(version), c.variant+" ") {
			t.Errorf("%s: version line %q does not start with the command name", c.variant, version)
		}
		// every binary carries the localhost default, so only the hosted URLs tell the variants apart
		content := read(t, binary)
		if !strings.Contains(content, c.url) {
			t.Errorf("%s: binary lacks its API URL %s", c.variant, c.url)
		}
		for _, url := range []string{stagingURL, productionURL, "https://from-env.test", "https://ambient.example.test"} {
			if url != c.url && strings.Contains(content, url) {
				t.Errorf("%s: binary contains the foreign API URL %s", c.variant, url)
			}
		}
	}
}

func TestBuildRejectsUnknownVariants(t *testing.T) {
	skipOnWindows(t)
	repo, err := filepath.Abs("..")
	if err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{{}, {"prod"}, {"staging"}, {"abot", "abot-dev"}} {
		if code, _ := run(t, repo, []string{"OUT_DIR=" + t.TempDir()}, "tools/build", args...); code == 0 {
			t.Errorf("tools/build %v must fail", args)
		}
	}
}

// release

type releaseRepo struct {
	root, origin, stubLog, output string
}

// newReleaseRepo creates a repository with a bare origin and stub goreleaser and gh executables.
// Steps are commit subjects, or "tag:<name>" to tag the last commit and push the tag.
func newReleaseRepo(t *testing.T, version string, steps ...string) releaseRepo {
	t.Helper()
	skipOnWindows(t)
	base := t.TempDir()
	r := releaseRepo{
		root:    filepath.Join(base, "work"),
		origin:  filepath.Join(base, "origin.git"),
		stubLog: filepath.Join(base, "stub.log"),
		output:  filepath.Join(base, "github_output"),
	}
	git(t, base, "init", "-q", "--bare", r.origin)
	git(t, base, "init", "-q", "-b", "main", r.root)
	git(t, r.root, "remote", "add", "origin", r.origin)
	write(t, filepath.Join(r.root, "go.mod"), "module example.test\n")
	write(t, filepath.Join(r.root, "VERSION"), version)
	copyTools(t, r.root, "release", "variants", "messages")
	stub := `#!/bin/sh
{
  printf 'call %s %s\n' "$(basename "$0")" "$*"
  env | grep -E '^(THE_AGENT_CLI_|GORELEASER_)' | sort
} >> "$STUB_LOG"
[ "$(basename "$0")" = goreleaser ] && exit "${STUB_EXIT:-0}"
exit 0
`
	for _, name := range []string{"goreleaser", "gh"} {
		path := filepath.Join(base, "bin", name)
		write(t, path, stub)
		if err := os.Chmod(path, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	git(t, r.root, "add", "-A")
	git(t, r.root, "commit", "-q", "-m", "initial")
	for _, step := range steps {
		if tag, ok := strings.CutPrefix(step, "tag:"); ok {
			git(t, r.root, "tag", "-a", "-m", tag, tag)
			git(t, r.root, "push", "-q", "origin", "refs/tags/"+tag)
			continue
		}
		git(t, r.root, "commit", "--allow-empty", "-q", "-m", step)
	}
	git(t, r.root, "push", "-q", "origin", "main")
	return r
}

func (r releaseRepo) run(t *testing.T, env []string, args ...string) (int, string) {
	t.Helper()
	bin := filepath.Join(filepath.Dir(r.root), "bin")
	env = append([]string{
		"GORELEASER=" + filepath.Join(bin, "goreleaser"),
		"GH=" + filepath.Join(bin, "gh"),
		"STUB_LOG=" + r.stubLog,
		"GITHUB_OUTPUT=" + r.output,
		"THE_AGENT_CLI_BUILD_API_URL=",
		"THE_AGENT_CLI_BUILD_PRODUCT_URL=",
	}, env...)
	return run(t, r.root, env, "tools/release", args...)
}

func (r releaseRepo) log(t *testing.T) string {
	t.Helper()
	content, err := os.ReadFile(r.stubLog)
	if errors.Is(err, os.ErrNotExist) {
		return ""
	}
	if err != nil {
		t.Fatal(err)
	}
	return string(content)
}

func (r releaseRepo) originTags(t *testing.T) []string {
	t.Helper()
	return strings.Fields(git(t, r.origin, "tag", "--list"))
}

func (r releaseRepo) localTags(t *testing.T) []string {
	t.Helper()
	return strings.Fields(git(t, r.root, "tag", "--list"))
}

func assertContainsAll(t *testing.T, what, got string, wants ...string) {
	t.Helper()
	for _, want := range wants {
		if !strings.Contains(got, want) {
			t.Errorf("%s lacks %q:\n%s", what, want, got)
		}
	}
}

func TestReleaseGA(t *testing.T) {
	r := newReleaseRepo(t, "0.2.0\n", "first", "tag:v0.1.0-beta", "tag:v0.1.0", "second", "tag:v0.2.0-beta", "third")
	code, out := r.run(t, []string{"THE_AGENT_CLI_BUILD_API_URL=https://ignored.test"}, "ga")
	if code != 0 {
		t.Fatalf("exit %d\n%s", code, out)
	}
	assertContainsAll(t, "goreleaser call", r.log(t),
		"call goreleaser release --clean",
		"GORELEASER_CURRENT_TAG=v0.2.0\n",
		"GORELEASER_PREVIOUS_TAG=v0.1.0\n",
		"THE_AGENT_CLI_BRANDED_NAME=abot\n",
		"THE_AGENT_CLI_BRANDED_API_URL="+productionURL+"\n",
		"THE_AGENT_CLI_GENERIC_NAME=agent-cli\n",
		"THE_AGENT_CLI_GENERIC_API_URL="+localURL+"\n",
		"THE_AGENT_CLI_PRODUCT_URL=https://agent.appifyhub.com\n",
		"THE_AGENT_CLI_VERSION=0.2.0\n",
	)
	if strings.Contains(r.log(t), "--skip=nfpm") {
		t.Error("production release disabled Linux packages")
	}
	if !slices.Contains(r.originTags(t), "v0.2.0") {
		t.Errorf("tag v0.2.0 was not pushed: %q", r.originTags(t))
	}
	if head, tagged := git(t, r.root, "rev-parse", "HEAD"), git(t, r.root, "rev-list", "-n", "1", "v0.2.0"); head != tagged {
		t.Errorf("tag points at %s, want HEAD %s", tagged, head)
	}
	assertContainsAll(t, "GITHUB_OUTPUT", read(t, r.output), "release_tag=v0.2.0\n", "release_version=0.2.0\n")
}

func TestReleaseBeta(t *testing.T) {
	r := newReleaseRepo(t, "0.2.0\n", "first", "tag:v0.1.0-beta", "tag:v0.1.0", "second")
	code, out := r.run(t, nil, "beta")
	if code != 0 {
		t.Fatalf("exit %d\n%s", code, out)
	}
	assertContainsAll(t, "goreleaser call", r.log(t),
		"call goreleaser release --clean --skip=nfpm",
		"GORELEASER_CURRENT_TAG=v0.2.0-beta\n",
		"GORELEASER_PREVIOUS_TAG=v0.1.0-beta\n",
		"THE_AGENT_CLI_BRANDED_NAME=abot-dev\n",
		"THE_AGENT_CLI_BRANDED_API_URL="+stagingURL+"\n",
		"THE_AGENT_CLI_GENERIC_NAME=agent-cli-dev\n",
		"THE_AGENT_CLI_GENERIC_API_URL="+localURL+"\n",
	)
	if !slices.Contains(r.originTags(t), "v0.2.0-beta") {
		t.Errorf("tag v0.2.0-beta was not pushed: %q", r.originTags(t))
	}
}

func TestReleaseWithoutEarlierTagsLetsGoReleaserPick(t *testing.T) {
	r := newReleaseRepo(t, "0.1.0\n", "first")
	if code, out := r.run(t, nil, "ga"); code != 0 {
		t.Fatalf("exit %d\n%s", code, out)
	}
	if strings.Contains(r.log(t), "GORELEASER_PREVIOUS_TAG") {
		t.Errorf("no previous tag exists, none must be passed:\n%s", r.log(t))
	}
}

func TestReleaseRefusesToPublishAVersionTwice(t *testing.T) {
	for _, c := range []struct{ quality, tag string }{{"ga", "v0.2.0"}, {"beta", "v0.2.0-beta"}} {
		r := newReleaseRepo(t, "0.2.0\n", "first", "tag:"+c.tag)
		// the tag exists only on the remote
		git(t, r.root, "tag", "-d", c.tag)
		for _, args := range [][]string{{c.quality}, {c.quality, "--check"}} {
			code, out := r.run(t, nil, args...)
			if code == 0 || !strings.Contains(out, "Bump the version") {
				t.Errorf("%v: exit %d, output %q", args, code, out)
			}
		}
		if strings.Contains(r.log(t), "goreleaser") {
			t.Errorf("%s: goreleaser must not run:\n%s", c.quality, r.log(t))
		}
	}
}

func TestReleaseCheckChangesNothing(t *testing.T) {
	r := newReleaseRepo(t, "0.2.0\n", "first")
	code, out := r.run(t, nil, "ga", "--check")
	if code != 0 || !strings.Contains(out, "'v0.2.0'") {
		t.Fatalf("exit %d\n%s", code, out)
	}
	if r.log(t) != "" || len(r.originTags(t)) != 0 || len(r.localTags(t)) != 0 {
		t.Errorf("--check must not tag or publish: log %q", r.log(t))
	}
}

func TestReleaseSnapshotPublishesNothing(t *testing.T) {
	for _, c := range []struct {
		quality, branded, skip string
	}{
		{"ga", "abot", ""},
		{"beta", "abot-dev", " --skip=nfpm"},
	} {
		t.Run(c.quality, func(t *testing.T) {
			r := newReleaseRepo(t, "0.2.0\n", "first")
			if code, out := r.run(t, nil, c.quality, "--snapshot"); code != 0 {
				t.Fatalf("exit %d\n%s", code, out)
			}
			assertContainsAll(t, "goreleaser call", r.log(t),
				"call goreleaser release --snapshot --clean"+c.skip+"\n",
				"THE_AGENT_CLI_BRANDED_NAME="+c.branded+"\n",
			)
			if len(r.originTags(t)) != 0 || len(r.localTags(t)) != 0 {
				t.Errorf("a snapshot must not tag")
			}
		})
	}
}

func TestReleaseFailureRemovesTheTag(t *testing.T) {
	t.Setenv("THE_AGENT_CLI_TOKEN", "synthetic-ambient-token")
	t.Setenv("THE_AGENT_CLI_PASSPHRASE", "synthetic-ambient-passphrase")
	r := newReleaseRepo(t, "0.2.0\n", "first")
	code, out := r.run(t, []string{"STUB_EXIT=1"}, "ga")
	if code == 0 || !strings.Contains(out, "Publishing failed") {
		t.Fatalf("exit %d\n%s", code, out)
	}
	if len(r.originTags(t)) != 0 || len(r.localTags(t)) != 0 {
		t.Errorf("the tag must be removed after a failed publish: origin %q", r.originTags(t))
	}
	assertContainsAll(t, "cleanup", r.log(t), "call gh release delete v0.2.0 --yes")
	for _, secret := range []string{"synthetic-ambient-token", "synthetic-ambient-passphrase"} {
		if strings.Contains(r.log(t)+out, secret) {
			t.Error("ambient secret reached the release stub or its output")
		}
	}
	if _, err := os.Stat(r.output); !os.IsNotExist(err) {
		t.Errorf("no outputs must be written for a failed release")
	}
}

func TestReleaseRejectsBadInput(t *testing.T) {
	r := newReleaseRepo(t, "0.2.0\n", "first")
	for _, args := range [][]string{{}, {"pr"}, {"GA"}, {"ga", "--bogus"}, {"ga", "--check", "extra"}} {
		if code, _ := r.run(t, nil, args...); code == 0 {
			t.Errorf("%v must fail", args)
		}
	}
	bad := newReleaseRepo(t, "0.2\n", "first")
	if code, out := bad.run(t, nil, "ga"); code == 0 || !strings.Contains(out, "VERSION") {
		t.Errorf("malformed VERSION: exit %d, %q", code, out)
	}
	if r.log(t) != "" || bad.log(t) != "" {
		t.Errorf("goreleaser must not run on bad input")
	}
}

// create_pr

func createPRRepo(t *testing.T, version, branch string) string {
	t.Helper()
	skipOnWindows(t)
	root := t.TempDir()
	git(t, root, "init", "-q", "-b", "main")
	write(t, filepath.Join(root, "go.mod"), "module example.test\n")
	write(t, filepath.Join(root, "VERSION"), version)
	copyTools(t, root, "create_pr", "messages")
	git(t, root, "add", "-A")
	git(t, root, "commit", "-q", "-m", "initial")
	if branch != "main" {
		git(t, root, "checkout", "-q", "-b", branch)
	}
	return root
}

func TestCreatePRPrintsTheCompareLinkOnFeatureBranches(t *testing.T) {
	root := createPRRepo(t, "0.2.0\n", "fix-typo")
	code, out := run(t, root, []string{"GITHUB_REPOSITORY=appifyhub/agent-cli"}, "tools/create_pr")
	if code != 0 || !strings.HasSuffix(strings.TrimSpace(out), "https://github.com/appifyhub/agent-cli/compare/main...fix-typo?quick_pull=1") {
		t.Errorf("exit %d, output %q", code, out)
	}
}

func TestCreatePRRefusesOnReleaseAndWithoutVersion(t *testing.T) {
	root := createPRRepo(t, "0.2.0\n", "release")
	if code, out := run(t, root, []string{"GITHUB_REPOSITORY=appifyhub/agent-cli"}, "tools/create_pr"); code == 0 || !strings.Contains(out, "release branch") {
		t.Errorf("release branch: exit %d, output %q", code, out)
	}
	root = createPRRepo(t, "0.2\n", "main")
	if code, out := run(t, root, []string{"GITHUB_REPOSITORY=appifyhub/agent-cli"}, "tools/create_pr"); code == 0 || !strings.Contains(out, "VERSION") {
		t.Errorf("malformed VERSION: exit %d, output %q", code, out)
	}
}
