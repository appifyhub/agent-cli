package agentcli

import (
	"regexp"
	"testing"
)

func TestVersionIsReadFromTheVersionFile(t *testing.T) {
	if !regexp.MustCompile(`^[0-9]+\.[0-9]+\.[0-9]+$`).MatchString(Version()) {
		t.Fatalf("Version() = %q, want MAJOR.MINOR.PATCH", Version())
	}
}
