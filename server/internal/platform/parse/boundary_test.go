// Copyright (C) Magpie contributors.
// SPDX-License-Identifier: GPL-3.0-or-later

package parse

import (
	"go/parser"
	"go/token"
	"os"
	"strconv"
	"strings"
	"testing"
)

// The parser may import only the standard library, the shared kernel and the regex engine.
func TestPackageImportsStayInBoundary(t *testing.T) {
	allowed := map[string]bool{
		"magpie/internal/domain":        true,
		"github.com/dlclark/regexp2/v2": true,
	}
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), ".go") || strings.HasSuffix(e.Name(), "_test.go") {
			continue
		}
		f, err := parser.ParseFile(token.NewFileSet(), e.Name(), nil, parser.ImportsOnly)
		if err != nil {
			t.Fatal(err)
		}
		for _, imp := range f.Imports {
			path, _ := strconv.Unquote(imp.Path.Value)
			standard := !strings.Contains(strings.Split(path, "/")[0], ".") && !strings.HasPrefix(path, "magpie/")
			if !standard && !allowed[path] {
				t.Errorf("%s imports %s", e.Name(), path)
			}
		}
	}
}
