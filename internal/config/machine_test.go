package config

import (
	"net"
	"os"
	"path/filepath"
	"testing"
)

func iface(flags net.Flags, hw string) net.Interface {
	parsed, _ := net.ParseMAC(hw)
	return net.Interface{Flags: flags, HardwareAddr: parsed}
}

func TestHardwareAddressesKeepOnlyUniversalHardware(t *testing.T) {
	got := hardwareAddresses([]net.Interface{
		iface(net.FlagLoopback, "00:00:00:00:00:00"),
		iface(net.FlagUp, "A4:83:E7:AA:BB:CC"),
		iface(net.FlagUp, "02:42:ac:11:00:02"),
		iface(net.FlagUp, "a6:83:e7:aa:bb:cc"),
		iface(net.FlagUp, ""),
		iface(net.FlagUp, "3c:22:fb:11:22:33"),
		iface(net.FlagUp, "a4:83:e7:aa:bb:cc"),
	})
	if want := []string{"3c:22:fb:11:22:33", "a4:83:e7:aa:bb:cc"}; len(got) != 2 || got[0] != want[0] || got[1] != want[1] {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestCleanMachineID(t *testing.T) {
	for raw, want := range map[string]string{
		"  5F1C0D2E-1111-2222-3333-444455556666\n": "5f1c0d2e-1111-2222-3333-444455556666",
		"4b3a9c0d5e6f47a8b1c2d3e4f5a6b7c8\n":       "4b3a9c0d5e6f47a8b1c2d3e4f5a6b7c8",
		"":                                         "",
		"   \n":                                    "",
		"00000000-0000-0000-0000-000000000000":     "",
		"{00000000-0000-0000-0000-000000000000}":   "",
		"00000000":                                 "",
	} {
		if got := cleanMachineID(raw); got != want {
			t.Errorf("cleanMachineID(%q) = %q, want %q", raw, got, want)
		}
	}
}

func TestFirstMachineIDSkipsMissingAndPlaceholderFiles(t *testing.T) {
	dir := t.TempDir()
	write := func(name, content string) string {
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
		return path
	}
	missing := filepath.Join(dir, "absent")
	zero := write("zero", "00000000000000000000000000000000\n")
	real := write("real", "4B3A9C0D5E6F47A8B1C2D3E4F5A6B7C8\n")
	if got := firstMachineID([]string{missing, zero, real}); got != "4b3a9c0d5e6f47a8b1c2d3e4f5a6b7c8" {
		t.Errorf("got %q", got)
	}
	if got := firstMachineID([]string{missing, zero}); got != "" {
		t.Errorf("got %q, want none", got)
	}
}

func TestLocalMachineDescribesThisMachine(t *testing.T) {
	m := LocalMachine()
	if m.GOOS == "" || m.GOARCH == "" {
		t.Errorf("GOOS %q, GOARCH %q", m.GOOS, m.GOARCH)
	}
	if m.ID == "" && len(m.MACs) == 0 {
		t.Skip("this machine offers neither a machine identifier nor a hardware address")
	}
	if again := LocalMachine(); again.ID != m.ID {
		t.Errorf("the identifier is not stable: %q then %q", m.ID, again.ID)
	}
}
