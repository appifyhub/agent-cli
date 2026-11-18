//go:build windows

package config

import "golang.org/x/sys/windows/registry"

// osMachineID is the MachineGuid that Windows creates at installation.
func osMachineID() string {
	key, err := registry.OpenKey(registry.LOCAL_MACHINE, `SOFTWARE\Microsoft\Cryptography`, registry.QUERY_VALUE|registry.WOW64_64KEY)
	if err != nil {
		return ""
	}
	defer key.Close()
	guid, _, err := key.GetStringValue("MachineGuid")
	if err != nil {
		return ""
	}
	return cleanMachineID(guid)
}
