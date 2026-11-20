//go:build !darwin && !windows

package config

// machineIDFiles are where Linux (systemd and D-Bus) and FreeBSD keep the identifier of the installation.
var machineIDFiles = []string{"/etc/machine-id", "/var/lib/dbus/machine-id", "/etc/hostid"}

func osMachineID() string {
	return firstMachineID(machineIDFiles)
}
