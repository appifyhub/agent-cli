package internal_test

import (
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"
)

const module = "github.com/appifyhub/agent-cli/"

// allowed lists the internal packages each package may import; absent packages are skipped.
// cli, testkit and cmd/agent-cli are the top of the stack and may import any lower layer.
var allowed = map[string][]string{
	"buildinfo":   {},
	"text":        {},
	"term":        {},
	"token":       {},
	"config":      {"buildinfo"},
	"api":         {"buildinfo"},
	"credentials": {"config", "token", "term"},
	"session":     {"credentials", "api", "config", "token"},
	"render":      {"buildinfo", "text", "term"},
}

const cobraPath = "github.com/spf13/cobra"

func TestImportDirection(t *testing.T) {
	violations := scanImports(t)
	for _, v := range violations {
		t.Error(v)
	}
}

func scanImports(t *testing.T) []string {
	t.Helper()
	var violations []string
	fset := token.NewFileSet()
	err := filepath.WalkDir(".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		parts := strings.Split(filepath.ToSlash(path), "/")
		if len(parts) < 2 {
			return nil
		}
		pkg := parts[0]
		file, err := parser.ParseFile(fset, path, nil, parser.ImportsOnly)
		if err != nil {
			return err
		}
		for _, spec := range file.Imports {
			imported, _ := strconv.Unquote(spec.Path.Value)
			if strings.HasPrefix(imported, cobraPath) && pkg != "cli" {
				violations = append(violations, path+": only internal/cli may import Cobra")
			}
			target, ok := strings.CutPrefix(imported, module+"internal/")
			if !ok {
				continue
			}
			target, _, _ = strings.Cut(target, "/")
			if target == "testkit" {
				violations = append(violations, path+": testkit is imported only from tests")
				continue
			}
			rule, restricted := allowed[pkg]
			if !restricted {
				continue
			}
			if !contains(rule, target) {
				violations = append(violations, path+": "+pkg+" must not import "+target)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(violations)
	return violations
}

func contains(list []string, s string) bool {
	for _, item := range list {
		if item == s {
			return true
		}
	}
	return false
}
