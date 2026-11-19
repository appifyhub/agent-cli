package cli

import (
	"regexp"
	"strings"
	"testing"
)

func TestMachineHostname(t *testing.T) {
	const id = "13892199-ba28-8c7e-a6ab-399e6bfddc6f"
	for _, c := range []struct{ input, want string }{
		{"Morpheus's MacBook Pro", "Morpheus-s-MacBook-Pro"},
		{"néo 💻 mac", "n-o-mac"},
		{strings.Repeat("a", 32), strings.Repeat("a", 32)},
		{strings.Repeat("a", 40), strings.Repeat("a", 32)},
		{strings.Repeat("a", 31) + ".extra", strings.Repeat("a", 31)},
		{"", "device-13892199ba28"},
		{"💻 .---", "device-13892199ba28"},
		{"..my-host.local..", "my-host.local"},
	} {
		got := machineHostname(c.input, id)
		if got != c.want {
			t.Errorf("%q: got %q, want %q", c.input, got, c.want)
		}
		if !regexp.MustCompile(`^[A-Za-z0-9.-]{1,32}$`).MatchString(got) {
			t.Errorf("invalid hostname %q", got)
		}
	}
	if machineHostname("", id) == machineHostname("", "23892199-ba28-8c7e-a6ab-399e6bfddc6f") {
		t.Error("different device ids need different fallback names")
	}
}
