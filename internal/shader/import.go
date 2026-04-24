// Copyright 2026 The Ebiten Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package shader

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/parser"
	"go/printer"
	"go/token"
	"strconv"
	"strings"
)

type resolvedImport struct {
	members map[string]string
	decls   []ast.Decl
}

type importResolver struct {
	fs          *token.FileSet
	rootPackage string
	sources     map[string][]byte
	imports     map[string]*resolvedImport
	order       []string
	visiting    []string
	nextID      int
}

func ResolveImports(src []byte, imports map[string][]byte) ([]byte, map[string]map[string]string, error) {
	fs := token.NewFileSet()
	f, err := parser.ParseFile(fs, "", src, parser.ParseComments|parser.AllErrors)
	if err != nil {
		return nil, nil, err
	}

	r := &importResolver{
		fs:          fs,
		rootPackage: f.Name.Name,
		sources:     imports,
		imports:     map[string]*resolvedImport{},
	}

	if err := r.resolveFileImports(f); err != nil {
		return nil, nil, err
	}
	f.Decls = removeImportDecls(f.Decls)
	for _, path := range r.order {
		f.Decls = append(f.Decls, r.imports[path].decls...)
	}

	var buf bytes.Buffer
	if err := printer.Fprint(&buf, fs, f); err != nil {
		return nil, nil, err
	}

	mappings := map[string]map[string]string{}
	for path, imp := range r.imports {
		members := map[string]string{}
		for name, renamed := range imp.members {
			members[name] = renamed
		}
		mappings[path] = members
	}
	return buf.Bytes(), mappings, nil
}

func (r *importResolver) resolveFileImports(f *ast.File) error {
	for _, imp := range f.Imports {
		path, err := importPath(imp)
		if err != nil {
			return err
		}
		if err := r.resolve(path); err != nil {
			return err
		}
	}
	return nil
}

func (r *importResolver) resolve(path string) error {
	if _, ok := r.imports[path]; ok {
		return nil
	}
	for i, visiting := range r.visiting {
		if visiting != path {
			continue
		}
		cycle := append(append([]string{}, r.visiting[i:]...), path)
		return fmt.Errorf("shader: circular import: %s", strings.Join(cycle, " -> "))
	}

	src, ok := r.sources[path]
	if !ok {
		return fmt.Errorf("shader: import %q not found", path)
	}

	r.visiting = append(r.visiting, path)
	defer func() {
		r.visiting = r.visiting[:len(r.visiting)-1]
	}()

	f, err := parser.ParseFile(r.fs, "", src, parser.ParseComments|parser.AllErrors)
	if err != nil {
		return err
	}
	if f.Name.Name != path {
		return fmt.Errorf("shader: import %q must declare package %s but declares package %s", path, path, f.Name.Name)
	}

	if err := r.resolveFileImports(f); err != nil {
		return err
	}

	topLevel := collectTopLevelObjects(f)
	prefix := fmt.Sprintf("__imp%d_", r.nextID)
	r.nextID++

	members := map[string]string{}
	objNames := map[*ast.Object]string{}
	for name, obj := range topLevel {
		renamed := prefix + name
		members[name] = renamed
		objNames[obj] = renamed
	}
	renameObjects(f, objNames)

	f.Name.Name = r.rootPackage
	f.Decls = removeImportAndVariableDecls(f.Decls)
	r.imports[path] = &resolvedImport{
		members: members,
		decls:   f.Decls,
	}
	r.order = append(r.order, path)
	return nil
}

func importPath(imp *ast.ImportSpec) (string, error) {
	if imp.Name != nil {
		return "", fmt.Errorf("shader: import aliases are not supported: %s", imp.Name.Name)
	}
	path, err := strconv.Unquote(imp.Path.Value)
	if err != nil {
		return "", err
	}
	return path, nil
}

func collectTopLevelObjects(f *ast.File) map[string]*ast.Object {
	objs := map[string]*ast.Object{}
	for _, decl := range f.Decls {
		switch decl := decl.(type) {
		case *ast.FuncDecl:
			if decl.Name != nil && decl.Name.Name != "_" && decl.Name.Obj != nil {
				objs[decl.Name.Name] = decl.Name.Obj
			}
		case *ast.GenDecl:
			for _, spec := range decl.Specs {
				switch spec := spec.(type) {
				case *ast.TypeSpec:
					if spec.Name != nil && spec.Name.Name != "_" && spec.Name.Obj != nil {
						objs[spec.Name.Name] = spec.Name.Obj
					}
				case *ast.ValueSpec:
					if decl.Tok != token.CONST {
						continue
					}
					for _, name := range spec.Names {
						if name.Name != "_" && name.Obj != nil {
							objs[name.Name] = name.Obj
						}
					}
				}
			}
		}
	}
	return objs
}

func renameObjects(f *ast.File, renames map[*ast.Object]string) {
	ast.Inspect(f, func(n ast.Node) bool {
		ident, ok := n.(*ast.Ident)
		if !ok || ident.Obj == nil {
			return true
		}
		if renamed, ok := renames[ident.Obj]; ok {
			ident.Name = renamed
		}
		return true
	})
}

func removeImportDecls(decls []ast.Decl) []ast.Decl {
	out := decls[:0]
	for _, decl := range decls {
		if gen, ok := decl.(*ast.GenDecl); ok && gen.Tok == token.IMPORT {
			continue
		}
		out = append(out, decl)
	}
	return out
}

func removeImportAndVariableDecls(decls []ast.Decl) []ast.Decl {
	out := decls[:0]
	for _, decl := range decls {
		gen, ok := decl.(*ast.GenDecl)
		if !ok {
			out = append(out, decl)
			continue
		}
		switch gen.Tok {
		case token.IMPORT, token.VAR:
			continue
		}
		out = append(out, decl)
	}
	return out
}
