package cli

import "strings"

func machineHostname(hostname, deviceID string) string {
	var cleaned strings.Builder
	separator := false
	for _, c := range hostname {
		valid := c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '.' || c == '-'
		if valid {
			if separator {
				cleaned.WriteByte('-')
				separator = false
			}
			cleaned.WriteRune(c)
		} else {
			separator = true
		}
	}
	name := strings.Trim(cleaned.String(), "-.")
	if len(name) > 32 {
		name = strings.Trim(name[:32], "-.")
	}
	if name == "" {
		return "device-" + strings.ReplaceAll(deviceID, "-", "")[:12]
	}
	return name
}
