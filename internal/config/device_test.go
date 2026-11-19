package config_test

import (
	"regexp"
	"strings"
	"testing"

	"github.com/appifyhub/agent-cli/internal/config"
)

var (
	version4 = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)
	version8 = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-8[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)
)

var mac = config.Machine{
	MACs:     []string{"3c:22:fb:11:22:33", "a4:83:e7:aa:bb:cc"},
	Hostname: "morpheus-mbp",
	User:     "morpheus",
	GOOS:     "darwin",
	GOARCH:   "arm64",
}

func withID(m config.Machine, id string) config.Machine {
	m.ID = id
	return m
}

func TestDeviceIDFromTheMachineIdentifier(t *testing.T) {
	m := withID(mac, "5f1c0d2e-1111-2222-3333-444455556666")
	id := config.DeviceID(m, "/home/morpheus/.agent-cli")
	if !version8.MatchString(id) || !config.ValidDeviceID(id) {
		t.Fatalf("%q is not a valid derived UUID", id)
	}
	if again := config.DeviceID(m, "/home/morpheus/.agent-cli"); again != id {
		t.Errorf("not deterministic: %q then %q", id, again)
	}
	if strings.Contains(id, "5f1c0d2e") {
		t.Errorf("the id exposes the machine identifier: %q", id)
	}
}

func TestDeviceIDDiffersPerHomeAndPerMachine(t *testing.T) {
	a := withID(mac, "machine-a")
	b := withID(mac, "machine-b")
	ids := map[string]string{
		"a, home 1": config.DeviceID(a, "/home/morpheus/.agent-cli"),
		"a, home 2": config.DeviceID(a, "/home/morpheus/.abot"),
		"b, home 1": config.DeviceID(b, "/home/morpheus/.agent-cli"),
	}
	seen := map[string]string{}
	for name, id := range ids {
		if other, dup := seen[id]; dup {
			t.Errorf("%s and %s share the id %q", name, other, id)
		}
		seen[id] = name
	}
}

func TestMachineIdentifierWinsOverEverythingElse(t *testing.T) {
	base := withID(mac, "machine-a")
	other := base
	other.MACs, other.Hostname, other.User, other.GOOS = []string{"00:11:22:33:44:55"}, "renamed", "someone", "linux"
	if config.DeviceID(base, "/h") != config.DeviceID(other, "/h") {
		t.Error("the id changed with properties that the machine identifier already covers")
	}
}

func TestDeviceIDFromHardwareProperties(t *testing.T) {
	id := config.DeviceID(mac, "/h")
	if !version8.MatchString(id) {
		t.Fatalf("%q is not a derived UUID", id)
	}
	if again := config.DeviceID(mac, "/h"); again != id {
		t.Errorf("not deterministic: %q then %q", id, again)
	}
	reordered := mac
	reordered.MACs = []string{mac.MACs[1], mac.MACs[0]}
	if config.DeviceID(reordered, "/h") != id {
		t.Error("the order of the interfaces changes the id")
	}
	for name, change := range map[string]func(*config.Machine){
		"another interface": func(m *config.Machine) { m.MACs = []string{"3c:22:fb:11:22:34"} },
		"host name":         func(m *config.Machine) { m.Hostname = "other" },
		"user":              func(m *config.Machine) { m.User = "other" },
		"os":                func(m *config.Machine) { m.GOOS = "linux" },
		"architecture":      func(m *config.Machine) { m.GOARCH = "amd64" },
	} {
		changed := mac
		change(&changed)
		if config.DeviceID(changed, "/h") == id {
			t.Errorf("a different %s gives the same id", name)
		}
	}
	if config.DeviceID(withID(mac, "machine-a"), "/h") == id {
		t.Error("the seed of the machine identifier and the seed of the properties must not collide")
	}
}

func TestDeviceIDFallsBackToRandom(t *testing.T) {
	seen := map[string]bool{}
	for range 100 {
		id := config.DeviceID(config.Machine{Hostname: "ubuntu", User: "root", GOOS: "linux", GOARCH: "amd64"}, "/root/.agent-cli")
		if !version4.MatchString(id) {
			t.Fatalf("%q is not a version-4 UUID", id)
		}
		if seen[id] {
			t.Fatalf("%q was generated twice", id)
		}
		seen[id] = true
	}
}

func TestBlankValuesCountAsMissing(t *testing.T) {
	for name, m := range map[string]config.Machine{
		"zero value":        {},
		"blank identifier":  {ID: "  \t"},
		"blank addresses":   {MACs: []string{"", "  "}, Hostname: "ubuntu", User: "root"},
		"nil addresses":     {MACs: nil, Hostname: "ubuntu"},
		"empty addresses":   {MACs: []string{}, User: "root"},
		"identifier absent": {ID: "", MACs: nil},
	} {
		a, b := config.DeviceID(m, "/h"), config.DeviceID(m, "/h")
		if !version4.MatchString(a) || a == b {
			t.Errorf("%s: got %q and %q, want two different random ids", name, a, b)
		}
	}
	padded := withID(mac, "  machine-a\n")
	if config.DeviceID(padded, "/h") != config.DeviceID(withID(mac, "machine-a"), "/h") {
		t.Error("whitespace around the identifier changes the id")
	}
	withBlank := mac
	withBlank.MACs = append([]string{""}, mac.MACs...)
	if config.DeviceID(withBlank, "/h") != config.DeviceID(mac, "/h") {
		t.Error("a blank address changes the id")
	}
}

func TestEnsureDeviceIDKeepsAnExistingID(t *testing.T) {
	f := config.File{DeviceID: "6b8f0d3e-2a1c-4e55-9a37-0c1d2e3f4a5b"}
	if f.EnsureDeviceID(withID(mac, "machine-a"), "/h") || f.DeviceID != "6b8f0d3e-2a1c-4e55-9a37-0c1d2e3f4a5b" {
		t.Errorf("the existing id changed: %q", f.DeviceID)
	}
}

func TestEnsureDeviceIDDerivesOnce(t *testing.T) {
	var f config.File
	m := withID(mac, "machine-a")
	if !f.EnsureDeviceID(m, "/h") || f.DeviceID != config.DeviceID(m, "/h") {
		t.Fatalf("got %q", f.DeviceID)
	}
	first := f.DeviceID
	if f.EnsureDeviceID(withID(mac, "machine-b"), "/h") || f.DeviceID != first {
		t.Error("a second call must keep the id")
	}
}

func TestValidDeviceID(t *testing.T) {
	for id, want := range map[string]bool{
		"6b8f0d3e-2a1c-4e55-9a37-0c1d2e3f4a5b":          true,
		"6B8F0D3E-2A1C-4E55-9A37-0C1D2E3F4A5B":          true,
		"":                                              false,
		"not-a-uuid":                                    false,
		"6b8f0d3e2a1c4e559a370c1d2e3f4a5b":              false,
		"{6b8f0d3e-2a1c-4e55-9a37-0c1d2e3f4a5b}":        false,
		"urn:uuid:6b8f0d3e-2a1c-4e55-9a37-0c1d2e3f4a5b": false,
		"6b8f0d3e-2a1c-4e55-9a37-0c1d2e3f4a5":           false,
	} {
		if got := config.ValidDeviceID(id); got != want {
			t.Errorf("ValidDeviceID(%q) = %v, want %v", id, got, want)
		}
	}
}
