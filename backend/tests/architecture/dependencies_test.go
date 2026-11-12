package architecture_test

import (
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestCoreDependencyDirection(t *testing.T) {
	err := filepath.WalkDir("../../internal/core", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		f, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
		if err != nil {
			return err
		}
		for _, imp := range f.Imports {
			name, err := strconv.Unquote(imp.Path.Value)
			if err != nil {
				return err
			}
			for _, forbidden := range []string{"gorm.io/", "github.com/gofiber/", "/internal/adapter/", "/internal/composition", "/dataset/config", "/pkg/logger"} {
				if strings.Contains(name, forbidden) {
					t.Errorf("%s imports infrastructure %s", path, name)
				}
			}
			if strings.Contains(path, "/domain/") && strings.Contains(name, ".") {
				t.Errorf("domain imports external dependency: %s", name)
			}
			if strings.Contains(path, "/usecase/") && strings.Contains(name, "/core/usecase/") {
				t.Errorf("use case depends on another use case: %s -> %s", path, name)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
