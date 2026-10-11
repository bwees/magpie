package app

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	"magpie/internal/platform/owner"
)

const (
	internalDir     = ".."
	migrateDir      = "platform/migrate"
	goFileSuffix    = ".go"
	readerName      = "Reader"
	importerName    = "Importer"
	contractTestDir = "/contracttest"
)

// apiImport matches another context's api package, or its contract-test package with the Reader fake.
var apiImport = regexp.MustCompile(`^magpie/internal/([a-z]+)/api(/contracttest)?$`)

// TestReadersFollowTheQueryGraph enforces the Reader graph of 00-context-map.md, which depguard can't:
// a context imports another's api for its events too, but may use its Reader only along the graph.
// Only the platform migration job may use an Importer.
func TestReadersFollowTheQueryGraph(t *testing.T) {
	graph := queryDependencies(t)
	if len(graph) != len(owner.All) {
		t.Fatalf("graph names %d owners, want %d: %v", len(graph), len(owner.All), graph)
	}

	for from, allowed := range graph {
		root := filepath.Join(internalDir, string(from))
		err := filepath.WalkDir(root, func(file string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() && filepath.ToSlash(file) == filepath.ToSlash(filepath.Join(internalDir, migrateDir)) {
				return filepath.SkipDir
			}
			if d.IsDir() || !strings.HasSuffix(file, goFileSuffix) {
				return nil
			}
			parsed, err := parser.ParseFile(token.NewFileSet(), file, nil, parser.SkipObjectResolution)
			if err != nil {
				return err
			}
			for _, violation := range readerViolations(parsed, from, allowed) {
				t.Errorf("%s: %s", file, violation)
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
}

func TestReaderViolationsCatchesAReaderOutsideTheGraph(t *testing.T) {
	const source = `package transcode

import (
	catalogapi "magpie/internal/catalog/api"
	subtitlesapi "magpie/internal/subtitles/api"
	librarytest "magpie/internal/library/api/contracttest"
	acquisitionapi "magpie/internal/acquisition/api"
)

type deps struct {
	catalog   catalogapi.Reader
	subtitles subtitlesapi.Reader
	importer  catalogapi.Importer
	fake      *librarytest.Fake
	event     acquisitionapi.CutoffMet
}
`
	file, err := parser.ParseFile(token.NewFileSet(), "fixture.go", source, parser.SkipObjectResolution)
	if err != nil {
		t.Fatal(err)
	}

	got := readerViolations(file, owner.Transcode, []owner.Name{owner.Catalog})
	want := []string{
		"uses catalog's Importer; only the platform migration job may",
		"uses library's Reader fake but transcode doesn't depend on library",
		"uses subtitles's Reader but transcode doesn't depend on subtitles",
	}
	if !slices.Equal(got, want) {
		t.Errorf("violations = %q\nwant %q", got, want)
	}
}

func readerViolations(file *ast.File, from owner.Name, allowed []owner.Name) []string {
	may := func(target owner.Name) bool { return target == from || slices.Contains(allowed, target) }

	apis := map[string]owner.Name{}
	var violations []string
	for _, imp := range file.Imports {
		importPath := strings.Trim(imp.Path.Value, `"`)
		match := apiImport.FindStringSubmatch(importPath)
		if match == nil {
			continue
		}
		target := owner.Name(match[1])
		if strings.HasSuffix(importPath, contractTestDir) {
			if !may(target) {
				violations = append(violations, fmt.Sprintf("uses %s's Reader fake but %s doesn't depend on %s", target, from, target))
			}
			continue
		}
		name := path.Base(importPath)
		if imp.Name != nil {
			name = imp.Name.Name
		}
		apis[name] = target
	}

	ast.Inspect(file, func(n ast.Node) bool {
		sel, ok := n.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		pkg, ok := sel.X.(*ast.Ident)
		if !ok {
			return true
		}
		target, ok := apis[pkg.Name]
		if !ok || target == from {
			return true
		}
		switch {
		case sel.Sel.Name == readerName && !may(target):
			violations = append(violations, fmt.Sprintf("uses %s's Reader but %s doesn't depend on %s", target, from, target))
		case sel.Sel.Name == importerName:
			violations = append(violations, fmt.Sprintf("uses %s's Importer; only the platform migration job may", target))
		}
		return true
	})
	slices.Sort(violations)
	return violations
}
