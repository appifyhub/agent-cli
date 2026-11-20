// Package render turns domain values into the text users read.
package render

import (
	"path/filepath"
	"strings"
)

// DisplayPath shows path with the user home directory replaced by "~", using the path separator of goos.
// Paths outside the home directory are returned unchanged. On Windows both separators and letter case
// are ignored when matching, as the file system does.
func DisplayPath(path, home, goos string) string {
	windows := goos == "windows"
	normalized, prefix := path, home
	if filepath.VolumeName(home) != "" || filepath.Separator == '\\' && strings.HasPrefix(path, `\`) {
		normalized, prefix = filepath.ToSlash(path), filepath.ToSlash(home)
	}
	normalized, prefix = slashes(normalized, windows), strings.TrimRight(slashes(prefix, windows), "/")
	if prefix == "" || len(normalized) < len(prefix) {
		return path
	}
	head, rest := normalized[:len(prefix)], normalized[len(prefix):]
	if head != prefix && (!windows || !strings.EqualFold(head, prefix)) {
		return path
	}
	if rest != "" && rest[0] != '/' {
		return path
	}
	if windows {
		rest = strings.ReplaceAll(rest, "/", `\`)
	}
	return "~" + rest
}

func slashes(path string, windows bool) string {
	if windows {
		return strings.ReplaceAll(path, `\`, "/")
	}
	return path
}
