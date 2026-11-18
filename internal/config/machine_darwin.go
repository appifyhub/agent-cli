//go:build darwin

package config

import "golang.org/x/sys/unix"

// osMachineID is the hardware UUID of the Mac (the IOPlatformUUID that "kern.uuid" reports).
func osMachineID() string {
	id, err := unix.Sysctl("kern.uuid")
	if err != nil {
		return ""
	}
	return cleanMachineID(id)
}
