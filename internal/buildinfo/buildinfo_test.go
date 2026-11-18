package buildinfo

import (
	"testing"

	agentcli "github.com/appifyhub/agent-cli"
)

func TestDefaultsAreTheGenericBuild(t *testing.T) {
	got := Current()
	want := Info{
		Name:       "agent-cli",
		Version:    agentcli.Version(),
		Commit:     "unknown",
		Date:       "unknown",
		APIURL:     "http://localhost",
		ProductURL: "https://agent.appifyhub.com",
	}
	if got != want {
		t.Fatalf("Current() = %+v, want %+v", got, want)
	}
}

func TestOverriddenValues(t *testing.T) {
	in := Info{
		Name:       "my-agent",
		Version:    "9.9.9",
		Commit:     "1a2b3c4",
		Date:       "2026-11-14",
		APIURL:     "https://staging.agent.cloud.appifyhub.com",
		ProductURL: "https://example.test/agent",
	}
	if got := Resolve(in); got != in {
		t.Fatalf("Resolve() = %+v, want %+v", got, in)
	}
}

func TestEmptyOverrideFallsBack(t *testing.T) {
	got := Resolve(Info{})
	if got != Current() {
		t.Fatalf("Resolve(empty) = %+v, want the defaults %+v", got, Current())
	}
}

func TestDateFormats(t *testing.T) {
	cases := map[string]string{
		"2026-11-14":                "2026-11-14",
		"2026-11-14T09:21:00Z":      "2026-11-14",
		"2026-11-14T23:30:00+01:00": "2026-11-14",
		"unknown":                   "unknown",
		"last tuesday":              "last tuesday",
	}
	for in, want := range cases {
		if got := (Info{Date: in}).DisplayDate(); got != want {
			t.Errorf("DisplayDate(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestVersionLine(t *testing.T) {
	release := Resolve(Info{Version: "0.1.0", Date: "2026-11-14"})
	if got, want := release.VersionLine("darwin", "arm64"), "agent-cli 0.1.0 (darwin/arm64, 2026-11-14)"; got != want {
		t.Errorf("release line = %q, want %q", got, want)
	}
	local := Resolve(Info{})
	if got, want := local.VersionLine("linux", "amd64"), "agent-cli "+agentcli.Version()+" (linux/amd64, unknown)"; got != want {
		t.Errorf("local line = %q, want %q", got, want)
	}
}

func TestCommitLine(t *testing.T) {
	if got, want := (Info{Commit: "1a2b3c4"}).CommitLine(), "  commit: 1a2b3c4"; got != want {
		t.Errorf("CommitLine() = %q, want %q", got, want)
	}
}
