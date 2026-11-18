package config

import (
	"net"
	"os"
	"os/user"
	"runtime"
	"slices"
	"strings"
)

// Machine holds what a device id is derived from. ID is the identifier the operating system keeps for
// this installation (empty when it cannot be read); the other fields describe the machine and are used
// only when there is no ID.
type Machine struct {
	ID       string
	MACs     []string
	Hostname string
	User     string
	GOOS     string
	GOARCH   string
}

// LocalMachine reads the properties of the machine the program runs on. Anything that cannot be read stays empty.
func LocalMachine() Machine {
	m := Machine{ID: osMachineID(), GOOS: runtime.GOOS, GOARCH: runtime.GOARCH}
	m.Hostname, _ = os.Hostname()
	if current, err := user.Current(); err == nil {
		m.User = current.Username
	}
	if ifaces, err := net.Interfaces(); err == nil {
		m.MACs = hardwareAddresses(ifaces)
	}
	return m
}

// hardwareAddresses returns the sorted, distinct MAC addresses that identify hardware: loopback and
// address-less interfaces are skipped, and so are locally administered addresses, which virtual
// interfaces, VPNs and per-network privacy features make up and change.
func hardwareAddresses(ifaces []net.Interface) []string {
	var macs []string
	for _, iface := range ifaces {
		hw := iface.HardwareAddr
		if iface.Flags&net.FlagLoopback != 0 || len(hw) == 0 || hw[0]&0x02 != 0 {
			continue
		}
		macs = append(macs, strings.ToLower(hw.String()))
	}
	slices.Sort(macs)
	return slices.Compact(macs)
}

// cleanMachineID normalizes an identifier read from the operating system and rejects the placeholder
// values that some virtual machines and images ship instead of a real one.
func cleanMachineID(raw string) string {
	id := strings.ToLower(strings.TrimSpace(raw))
	if strings.Trim(id, "0-{}") == "" {
		return ""
	}
	return id
}

// firstMachineID returns the first usable identifier among the files.
func firstMachineID(paths []string) string {
	for _, path := range paths {
		raw, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		if id := cleanMachineID(string(raw)); id != "" {
			return id
		}
	}
	return ""
}
