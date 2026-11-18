// Package text holds every user-facing string of the CLI in one embedded, editable catalog.
package text

import (
	"bytes"
	_ "embed"
	"fmt"
	"strings"

	"github.com/pelletier/go-toml/v2"
)

//go:embed strings.toml
var catalogFile []byte

// Catalog resolves keys to rendered text for one operating system.
type Catalog struct {
	goos    string
	entries map[string]string
}

// New decodes the embedded catalog for the given GOOS value.
func New(goos string) (*Catalog, error) {
	entries, err := decode(catalogFile)
	if err != nil {
		return nil, err
	}
	return &Catalog{goos: goos, entries: entries}, nil
}

// MustNew is New for wiring code, where a broken embedded catalog is a build defect.
func MustNew(goos string) *Catalog {
	c, err := New(goos)
	if err != nil {
		panic(err)
	}
	return c
}

// T renders the entry for key; args are name/value pairs for the {name} placeholders.
// An unknown key panics: the catalog tests prove that every key used in the code exists.
func (c *Catalog) T(key string, args ...string) string {
	template, ok := resolve(c.entries, c.goos, key)
	if !ok {
		panic("text: unknown catalog key " + key)
	}
	return render(template, args)
}

// resolve picks the most specific variant: <key>_<goos>, then <key>_unix on every OS but windows, then <key>.
func resolve(entries map[string]string, goos, key string) (string, bool) {
	if v, ok := entries[key+"_"+goos]; ok {
		return v, true
	}
	if goos != "windows" {
		if v, ok := entries[key+"_unix"]; ok {
			return v, true
		}
	}
	v, ok := entries[key]
	return v, ok
}

func decode(raw []byte) (map[string]string, error) {
	var tree map[string]any
	if err := toml.NewDecoder(bytes.NewReader(raw)).Decode(&tree); err != nil {
		return nil, fmt.Errorf("decode strings catalog: %w", err)
	}
	entries := map[string]string{}
	if err := flatten("", tree, entries); err != nil {
		return nil, err
	}
	return entries, nil
}

func flatten(prefix string, node map[string]any, out map[string]string) error {
	for name, value := range node {
		key := name
		if prefix != "" {
			key = prefix + "." + name
		}
		switch v := value.(type) {
		case string:
			if strings.TrimSpace(v) == "" {
				return fmt.Errorf("strings catalog entry %s is empty", key)
			}
			out[key] = v
		case map[string]any:
			if err := flatten(key, v, out); err != nil {
				return err
			}
		default:
			return fmt.Errorf("strings catalog entry %s must be a string or a table, got %T", key, value)
		}
	}
	return nil
}

// render fills {name} placeholders from name/value pairs; {{ and }} are literal braces,
// and placeholders without a value are left as they are.
func render(template string, args []string) string {
	if len(args)%2 != 0 {
		panic("text: arguments must be name/value pairs")
	}
	var b strings.Builder
	b.Grow(len(template))
	for i := 0; i < len(template); i++ {
		switch {
		case strings.HasPrefix(template[i:], "{{"):
			b.WriteByte('{')
			i++
		case strings.HasPrefix(template[i:], "}}"):
			b.WriteByte('}')
			i++
		case template[i] == '{':
			end := strings.IndexByte(template[i:], '}')
			if end < 0 {
				b.WriteString(template[i:])
				return b.String()
			}
			b.WriteString(lookupArg(args, template[i+1:i+end], template[i:i+end+1]))
			i += end
		default:
			b.WriteByte(template[i])
		}
	}
	return b.String()
}

func lookupArg(args []string, name, fallback string) string {
	for i := 0; i < len(args); i += 2 {
		if args[i] == name {
			return args[i+1]
		}
	}
	return fallback
}
