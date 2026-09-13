// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package configschema

import (
	"bufio"
	"errors"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
)

// docMap stores documentation comments discovered from the Jaeger source tree.
type docMap struct {
	mu        sync.RWMutex
	typeDocs  map[string]string // key: importPath.TypeName
	fieldDocs map[string]string // key: importPath.TypeName.FieldName
}

func newDocMap() *docMap {
	return &docMap{
		typeDocs:  make(map[string]string),
		fieldDocs: make(map[string]string),
	}
}

// load scans the provided source directories (recursively) and extracts
// type and field docs. Hidden directories and directories named *_test are
// skipped, as are _test.go files.
func (d *docMap) load(modulePath string, dirs []string) error {
	fset := token.NewFileSet()
	var firstErr error
	for _, dir := range dirs {
		err := filepath.WalkDir(dir, func(path string, entry fs.DirEntry, err error) error {
			if err != nil {
				if path == dir {
					// Skip a root directory that does not exist.
					return nil
				}
				return err
			}
			if !entry.IsDir() {
				return nil
			}
			if name := entry.Name(); name != "." && (strings.HasPrefix(name, ".") || strings.HasSuffix(name, "_test")) {
				return fs.SkipDir
			}
			return d.parseDir(fset, modulePath, path)
		})
		if err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

func (d *docMap) parseDir(fset *token.FileSet, modulePath, dir string) error {
	info, err := os.Stat(dir)
	if errors.Is(err, os.ErrNotExist) || (err == nil && !info.IsDir()) {
		return nil
	}
	if err != nil {
		return err
	}

	rel, err := filepath.Rel(moduleRoot(), dir)
	if err != nil {
		rel = dir
	}
	rel = filepath.ToSlash(rel)
	importPath := modulePath
	if rel != "." && rel != "" {
		importPath = importPath + "/" + rel
	}

	filter := func(info fs.FileInfo) bool {
		return !strings.HasSuffix(info.Name(), "_test.go")
	}
	pkgs, err := parser.ParseDir(fset, dir, filter, parser.ParseComments)
	if err != nil {
		// Ignore parse errors for directories we do not strictly need (e.g. test only).
		return nil
	}
	for _, pkg := range pkgs {
		if strings.HasSuffix(pkg.Name, "_test") {
			continue
		}
		for _, f := range pkg.Files {
			d.parseFile(importPath, f)
		}
	}
	return nil
}

func (d *docMap) parseFile(importPath string, f *ast.File) {
	for _, decl := range f.Decls {
		genDecl, ok := decl.(*ast.GenDecl)
		if !ok || genDecl.Tok != ast.TYPE {
			continue
		}
		for _, spec := range genDecl.Specs {
			ts, ok := spec.(*ast.TypeSpec)
			if !ok {
				continue
			}
			typeKey := importPath + "." + ts.Name.Name
			if doc := firstDoc(genDecl.Doc, ts.Doc); doc != "" {
				d.setTypeDoc(typeKey, doc)
			}
			st, ok := ts.Type.(*ast.StructType)
			if !ok {
				continue
			}
			for _, field := range st.Fields.List {
				if len(field.Names) == 0 {
					// Embedded field: doc key is the embedded type name.
					name := embeddedName(field.Type)
					if name != "" {
						d.setFieldDoc(typeKey, name, fieldDoc(field))
					}
					continue
				}
				for _, ident := range field.Names {
					d.setFieldDoc(typeKey, ident.Name, fieldDoc(field))
				}
			}
		}
	}
}

func (d *docMap) setTypeDoc(key, doc string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if _, ok := d.typeDocs[key]; !ok {
		d.typeDocs[key] = doc
	}
}

func (d *docMap) setFieldDoc(typeKey, fieldName, doc string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	key := typeKey + "." + fieldName
	if _, ok := d.fieldDocs[key]; !ok {
		d.fieldDocs[key] = doc
	}
}

func (d *docMap) typeDoc(key string) string {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.typeDocs[key]
}

func (d *docMap) fieldDoc(typeKey, fieldName string) string {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.fieldDocs[typeKey+"."+fieldName]
}

func firstDoc(group ...*ast.CommentGroup) string {
	for _, cg := range group {
		if cg != nil {
			return normalizeDoc(cg.Text())
		}
	}
	return ""
}

func fieldDoc(field *ast.Field) string {
	if field.Doc != nil {
		return normalizeDoc(field.Doc.Text())
	}
	return ""
}

func embeddedName(expr ast.Expr) string {
	switch e := expr.(type) {
	case *ast.Ident:
		return e.Name
	case *ast.StarExpr:
		return embeddedName(e.X)
	case *ast.SelectorExpr:
		return e.Sel.Name
	}
	return ""
}

// normalizeDoc returns the first paragraph of the comment, joining its lines
// with single spaces.
func normalizeDoc(text string) string {
	text = strings.TrimSpace(text)
	if text == "" {
		return ""
	}
	var out []string
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			if len(out) > 0 {
				// End of the first paragraph.
				break
			}
			continue
		}
		out = append(out, line)
	}
	return strings.Join(out, " ")
}

func isDeprecatedDoc(doc string) bool {
	return strings.Contains(strings.ToLower(doc), "deprecated:")
}

// moduleRoot returns the root directory of the jaeger module by walking up
// from this source file until go.mod is found.
func moduleRoot() string {
	_, file, _, _ := callerFrame(1)
	dir := filepath.Dir(file)
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return dir
		}
		dir = parent
	}
}

// callerFrame is a tiny wrapper around runtime.Caller so it can be stubbed in tests.
var callerFrame = func(skip int) (uintptr, string, int, bool) {
	return runtime.Caller(skip)
}

// modulePath reads the module declaration from go.mod.
func modulePath() string {
	f, err := os.Open(filepath.Join(moduleRoot(), "go.mod"))
	if err != nil {
		return ""
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(line, "module ") {
			return strings.TrimSpace(strings.TrimPrefix(line, "module"))
		}
	}
	return ""
}
