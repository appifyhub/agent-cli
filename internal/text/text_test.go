package text

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"testing"
)

var allOS = []string{"darwin", "linux", "windows"}

var variantSuffixes = []string{"_darwin", "_linux", "_windows", "_unix"}

// logicalKeys lists the keys code can ask for: entries without their OS suffix.
func logicalKeys(entries map[string]string) []string {
	seen := map[string]bool{}
	for key := range entries {
		seen[logicalKey(key)] = true
	}
	keys := make([]string, 0, len(seen))
	for key := range seen {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func logicalKey(key string) string {
	for _, suffix := range variantSuffixes {
		if trimmed, ok := strings.CutSuffix(key, suffix); ok {
			return trimmed
		}
	}
	return key
}

// placeholders returns the names used in a template, ignoring escaped braces.
func placeholders(template string) []string {
	var names []string
	for i := 0; i < len(template); i++ {
		switch {
		case strings.HasPrefix(template[i:], "{{"), strings.HasPrefix(template[i:], "}}"):
			i++
		case template[i] == '{':
			end := strings.IndexByte(template[i:], '}')
			if end < 0 {
				return names
			}
			names = append(names, template[i+1:i+end])
			i += end
		}
	}
	return names
}

func loadEntries(t *testing.T) map[string]string {
	t.Helper()
	catalog, err := New("linux")
	if err != nil {
		t.Fatalf("catalog must decode strictly: %v", err)
	}
	return catalog.entries
}

func TestCatalogLeavesAreNonEmpty(t *testing.T) {
	for key, value := range loadEntries(t) {
		if strings.TrimSpace(value) == "" {
			t.Errorf("entry %s is blank", key)
		}
	}
}

func TestDecodeRejectsBadShapes(t *testing.T) {
	cases := map[string]string{
		"empty leaf":   "[a]\nb = \"\"\n",
		"blank leaf":   "[a]\nb = \"  \"\n",
		"non string":   "[a]\nb = 3\n",
		"array":        "[a]\nb = [\"x\"]\n",
		"invalid toml": "[a\n",
	}
	for name, raw := range cases {
		if _, err := decode([]byte(raw)); err == nil {
			t.Errorf("%s: expected an error", name)
		}
	}
}

func TestVariantResolution(t *testing.T) {
	entries := map[string]string{
		"hint":         "base",
		"hint_unix":    "unix",
		"hint_windows": "windows",
		"hint_darwin":  "darwin",
		"plain":        "plain",
		"only_unix":    "x",
	}
	cases := []struct{ goos, key, want string }{
		{"darwin", "hint", "darwin"},
		{"linux", "hint", "unix"},
		{"windows", "hint", "windows"},
		{"freebsd", "hint", "unix"},
		{"windows", "plain", "plain"},
		{"freebsd", "plain", "plain"},
	}
	for _, c := range cases {
		got, ok := resolve(entries, c.goos, c.key)
		if !ok || got != c.want {
			t.Errorf("resolve(%s, %s) = %q, %v; want %q", c.goos, c.key, got, ok, c.want)
		}
	}
	if _, ok := resolve(entries, "windows", "only"); ok {
		t.Error("a key with only a _unix sibling must not resolve on windows")
	}
	if _, ok := resolve(entries, "linux", "missing"); ok {
		t.Error("unknown keys must not resolve")
	}
}

func TestRender(t *testing.T) {
	cases := []struct {
		template string
		args     []string
		want     string
	}{
		{"hello {name}", []string{"name", "Morpheus"}, "hello Morpheus"},
		{"{a}{b}{a}", []string{"a", "1", "b", "2"}, "121"},
		{"literal {{braces}} here", nil, "literal {braces} here"},
		{"{{{name}}}", []string{"name", "x"}, "{x}"},
		{"missing {name}", nil, "missing {name}"},
		{"no placeholders", nil, "no placeholders"},
		{"dangling {oops", nil, "dangling {oops"},
	}
	for _, c := range cases {
		if got := render(c.template, c.args); got != c.want {
			t.Errorf("render(%q) = %q, want %q", c.template, got, c.want)
		}
	}
}

func TestPlaceholders(t *testing.T) {
	got := placeholders("a {x} {{not}} {y}{x}")
	if !reflect.DeepEqual(got, []string{"x", "y", "x"}) {
		t.Errorf("placeholders = %v", got)
	}
}

func TestEveryEntryRendersForEveryOS(t *testing.T) {
	entries := loadEntries(t)
	for _, key := range logicalKeys(entries) {
		for _, goos := range allOS {
			value, ok := resolve(entries, goos, key)
			if !ok {
				t.Errorf("key %s does not resolve for %s", key, goos)
				continue
			}
			var args []string
			for _, name := range placeholders(value) {
				args = append(args, name, "x")
			}
			out := render(value, args)
			if out == "" {
				t.Errorf("key %s renders empty for %s", key, goos)
			}
			for _, name := range placeholders(value) {
				if strings.Contains(out, "{"+name+"}") {
					t.Errorf("key %s leaves {%s} unresolved for %s", key, name, goos)
				}
			}
		}
	}
}

type callSite struct {
	pos    string
	key    string
	prefix bool
	args   []string
	static bool
}

func TestCatalogUsageMatchesCode(t *testing.T) {
	entries := loadEntries(t)
	keys := logicalKeys(entries)
	known := map[string]bool{}
	for _, key := range keys {
		known[key] = true
	}

	used := map[string]bool{}
	for _, site := range scanCallSites(t) {
		if site.prefix {
			matched := false
			for _, key := range keys {
				if strings.HasPrefix(key, site.key) {
					used[key] = true
					matched = true
				}
			}
			if !matched {
				t.Errorf("%s: no catalog key starts with %q", site.pos, site.key)
			}
			continue
		}
		if !known[site.key] {
			t.Errorf("%s: catalog key %q does not exist", site.pos, site.key)
			continue
		}
		used[site.key] = true
		if !site.static {
			continue
		}
		want := variantPlaceholders(entries, site.key)
		got := append([]string(nil), site.args...)
		sort.Strings(got)
		if !reflect.DeepEqual(dedupe(got), want) {
			t.Errorf("%s: key %q is called with %v, entry placeholders are %v", site.pos, site.key, dedupe(got), want)
		}
	}

	for _, key := range keys {
		if !used[key] {
			t.Errorf("catalog key %q is not used by any code", key)
		}
	}
}

func variantPlaceholders(entries map[string]string, key string) []string {
	set := map[string]bool{}
	for name, value := range entries {
		if logicalKey(name) == key {
			for _, p := range placeholders(value) {
				set[p] = true
			}
		}
	}
	names := make([]string, 0, len(set))
	for name := range set {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func dedupe(sorted []string) []string {
	out := []string{}
	for i, s := range sorted {
		if i == 0 || s != sorted[i-1] {
			out = append(out, s)
		}
	}
	return out
}

func scanCallSites(t *testing.T) []callSite {
	t.Helper()
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	var sites []callSite
	fset := token.NewFileSet()
	err = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			switch d.Name() {
			case ".git", "testdata", "openspec":
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		file, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			return err
		}
		ast.Inspect(file, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok || len(call.Args) == 0 {
				return true
			}
			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok || sel.Sel.Name != "T" {
				return true
			}
			site := callSite{pos: fset.Position(call.Pos()).String(), static: true}
			switch first := call.Args[0].(type) {
			case *ast.BasicLit:
				if first.Kind != token.STRING {
					return true
				}
				site.key, _ = strconv.Unquote(first.Value)
			case *ast.BinaryExpr:
				lit, ok := first.X.(*ast.BasicLit)
				if !ok || lit.Kind != token.STRING {
					return true
				}
				site.key, _ = strconv.Unquote(lit.Value)
				site.prefix = true
			default:
				t.Errorf("%s: catalog key must be a string literal or a literal prefix plus a name", site.pos)
				return true
			}
			for i, arg := range call.Args[1:] {
				if i%2 != 0 {
					continue
				}
				lit, ok := arg.(*ast.BasicLit)
				if !ok || lit.Kind != token.STRING {
					site.static = false
					break
				}
				name, _ := strconv.Unquote(lit.Value)
				site.args = append(site.args, name)
			}
			sites = append(sites, site)
			return true
		})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return sites
}
