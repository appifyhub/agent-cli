package config

import (
	"crypto/sha256"
	"slices"
	"strings"
	"uuid"
)

// deviceNamespace separates these ids from any other use of the same machine identifier; changing it
// changes every derived id.
const deviceNamespace = "agent-cli/device-id/v1"

// DeviceID derives the device id of a machine and home directory: a UUID built from a SHA-256 hash, so
// the same machine and home always give the same id, different home directories give different ids, and
// the raw machine identifier and hardware addresses never leave the machine. The seed is the identifier
// the operating system keeps for the installation; when there is none, the hardware addresses of the
// network interfaces together with the host name, user name, operating system and architecture; when
// the machine offers neither, the id is a random version-4 UUID.
func DeviceID(m Machine, home string) string {
	var seed []string
	macs := nonBlank(m.MACs)
	switch {
	case strings.TrimSpace(m.ID) != "":
		seed = []string{"machine-id", strings.TrimSpace(m.ID)}
	case len(macs) > 0:
		slices.Sort(macs)
		seed = []string{"hardware", strings.Join(macs, ","), m.Hostname, m.User, m.GOOS, m.GOARCH}
	default:
		return uuid.NewV4().String()
	}
	hash := sha256.New()
	for _, part := range append([]string{deviceNamespace}, append(seed, home)...) {
		hash.Write([]byte(part))
		hash.Write([]byte{0})
	}
	var id uuid.UUID
	copy(id[:], hash.Sum(nil))
	id[6] = id[6]&0x0f | 0x80 // version 8, the custom-data version of RFC 9562
	id[8] = id[8]&0x3f | 0x80 // RFC 9562 variant
	return id.String()
}

// nonBlank returns a copy of values without the empty and whitespace-only entries.
func nonBlank(values []string) []string {
	kept := make([]string, 0, len(values))
	for _, v := range values {
		if v = strings.TrimSpace(v); v != "" {
			kept = append(kept, v)
		}
	}
	return kept
}

// ValidDeviceID reports whether id is a UUID in the canonical hyphenated form.
func ValidDeviceID(id string) bool {
	parsed, err := uuid.Parse(id)
	return err == nil && parsed.String() == strings.ToLower(id)
}

// EnsureDeviceID derives a device id when the file has none and reports whether it did.
func (f *File) EnsureDeviceID(m Machine, home string) bool {
	if f.DeviceID != "" {
		return false
	}
	f.DeviceID = DeviceID(m, home)
	return true
}
