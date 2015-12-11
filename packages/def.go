/* TODO
- move patch to packages
- move config to packages?
- create a package test
- build up 100% code coverage+additional tests:
  - channel recv
  - what is needed for float32
*/

/*
Package packages provides the basic utilities for any type conversion.
There are the following main functions ...

Convert

During this phase no errors should occur.

Fix

Fix type conflicts. If an error occur during this phase, the command
(e.g. gofloat) should quit immediately.

Format

Convert format arguments in calls to printf functions. If an error
occurs during this phase, the package should be skipped.


*/
package packages

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"reflect"
	"strings"

	"golang.org/x/tools/go/ast/astutil"
	"golang.org/x/tools/go/types"

	// initialize DefaultImport to gcimporter.Import
	// see https://godoc.org/golang.org/x/tools/go/types#Importer
	_ "golang.org/x/tools/go/gcimporter"
)

const star = "*"

type (
	// Visitor extends the ast.Visitor interface with an Error method.
	Visitor interface {
		ast.Visitor
		Error() error
	}
	// Package contains all data parsed by the ast and types packages.
	Package struct {
		Name     string
		Ast      *ast.Package
		Types    *types.Package
		Fset     *token.FileSet
		Info     *types.Info
		Errors   []error
		snippets Set
	}
	// Packages is a collection of Package in the same directory.
	// (For example "foo" and "foo_test".)
	Packages []Package
	// Skip describes which items have to be skipped during conversion.
	Skip struct {
		data    map[string]Set
		current string
	}
	// Set emulates an unordered set of strings
	Set map[string]struct{}
)

// AddSnippet permits to add necessary code for the conversion to
// a package. (For example "i64" to convert float64 constants to int.)
func (pkg *Package) AddSnippet(name string) {
	pkg.snippets[name] = struct{}{}
}

// File returns the ast.File which contains pos.
func (pkg *Package) File(pos token.Pos) *ast.File {
	for _, f := range pkg.Ast.Files {
		if f.Pos() <= pos && pos < f.End() {
			return f
		}
	}
	return nil
}

// Path returns the package path.
func (pkg *Package) Path() string {
	return pkg.Types.Path()
}

// printPath prints a path of nodes (for debugging purposes only)
func (pkg *Package) printPath(path []ast.Node) {
	typStr := "nil"
	if expr, ok := path[0].(ast.Expr); ok {
		typ := pkg.Info.TypeOf(expr)
		if typ != nil {
			typStr = typ.String()
		}
	}
	source, _ := str(pkg.Fset, path[0])
	fmt.Printf("\t%q [%s]\n", source, typStr)
	for i, n := range path {
		fmt.Printf("\t%d: %s [%v] @ %d\n", i,
			astutil.NodeDescription(n), reflect.TypeOf(n), n.Pos())
	}
}

// Repo returns the package import path.
// (For example "github.com/stanim/packages".)
func (pkg *Package) Repo() string {
	return Repo(pkg.Types.Path())
}

// TypeStringOf returns the type string of an expression.
func (pkg *Package) TypeStringOf(e ast.Expr) string {
	tp := pkg.Info.TypeOf(e)
	if tp == nil {
		return "<nil>"
	}
	return tp.String()
}

// Save the package go files to another dir.
func (pkg *Package) Save(dirname string) error {
	pkgDir := pkg.Path()
	for _, f := range pkg.Ast.Files {
		filename := strings.Replace(Filename(pkg.Fset, f), pkgDir, dirname, 1)
		if err := SaveFile(pkg.Fset, f, filename); err != nil {
			return err
		}
	}
	if err := pkg.saveSnippets(); err != nil {
		return err
	}
	return nil
}

// saveSnippets saves the package snippets in a "snippets.go" file,
// which gets added to the package.
func (pkg *Package) saveSnippets() error {
	if len(pkg.snippets) > 0 {
		fo, err := os.Create(filepath.Join(pkg.Path(), "snippets.go"))
		if err != nil {
			return err
		}
		_, err = fo.WriteString(fmt.Sprintf("package %s\n", pkg.Name))
		if err != nil {
			return err
		}
		// collect & write imports
		var imports Set
		for s := range pkg.snippets {
			for _, imp := range snippets[s].imports {
				imports[imp] = struct{}{}
			}
		}
		if len(imports) > 0 {
			_, err = fo.WriteString("import (\n")
			if err != nil {
				return err
			}
			for imp := range imports {
				_, err = fo.WriteString(fmt.Sprintf("\t%q\n", imp))
				if err != nil {
					return err
				}
			}
			_, err = fo.WriteString(")\n")
			if err != nil {
				return err
			}
		}
		// write snippets
		for s := range pkg.snippets {
			_, err = fo.WriteString(snippets[s].source)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

// Walk traverses an AST in depth-first order: It starts by calling
// v.Visit(pkg.Ast). See ast.Walk for more information.
func (pkg *Package) Walk(v Visitor) error {
	ast.Walk(v, pkg.Ast)
	if err := v.Error(); err != nil {
		return err
	}
	return nil
}

// New parses all packages from go files in the directory.
func New(dirname string) (Packages, error) {
	var pkgs Packages
	fset := token.NewFileSet()
	pkgMap, err := parser.ParseDir(fset, dirname, nil, parser.ParseComments)
	if err != nil {
		return pkgs, err
	}
	for name, pkgAst := range pkgMap {
		var (
			errs    []error
			collect = func(err error) {
				errs = append(errs, err)
			}
		)
		info := &types.Info{
			Types:      map[ast.Expr]types.TypeAndValue{},
			Defs:       map[*ast.Ident]types.Object{},
			Uses:       map[*ast.Ident]types.Object{},
			Implicits:  map[ast.Node]types.Object{},
			Selections: map[*ast.SelectorExpr]*types.Selection{},
			Scopes:     map[ast.Node]*types.Scope{},
			InitOrder:  []*types.Initializer{}}
		pkgTypes, _ := (&types.Config{ // error should be handled by check
			Error:  collect,
			Import: nil,
			DisableUnusedImportCheck: true,
		}).Check(dirname, fset, files(pkgAst), info)
		pkgs = append(pkgs, Package{
			Name:     name,
			Ast:      pkgAst,
			Types:    pkgTypes,
			Fset:     fset,
			Info:     info,
			Errors:   errs,
			snippets: Set{},
		})
	}
	return pkgs, nil
}

// Error returns the first error found.
func (pkgs *Packages) Error() error {
	for _, pkg := range *pkgs {
		if len(pkg.Errors) > 0 {
			return pkg.Errors[0]
		}
	}
	return nil
}

// Save the packages go files to another dir.
func (pkgs *Packages) Save(dirname string) error {
	for _, pkg := range *pkgs {
		if err := pkg.Save(dirname); err != nil {
			return err
		}
	}
	return nil
}

// Snippets returns a map of snippets by package name.
func (pkgs *Packages) Snippets() map[string]Set {
	snippetsMap := map[string]Set{}
	for _, pkg := range *pkgs {
		snippetsMap[pkg.Name] = pkg.snippets
	}
	return snippetsMap
}

// SetSnippets sets a map of snippets to its packages.
func (pkgs *Packages) SetSnippets(snippetsMap map[string]Set) {
	for _, pkg := range *pkgs {
		name := pkg.Name
		if snippets, ok := snippetsMap[name]; ok {
			pkg.snippets = snippets
		}
	}
}

// NewSkip creates a Skip object from a data map, which comes for
// example from a JSON configuration file..
func NewSkip(data map[string][]string) Skip {
	str := data["*"]
	for base, lst := range data {
		data[base] = append(lst, str...)
	}
	return Skip{data: mapset(data), current: star}
}

// Update the current file, which is usefull during visitor walking
// an AST tree.
func (s *Skip) Update(fset *token.FileSet, f *ast.File) {
	s.current = base(Filename(fset, f))
	_, ok := s.data[s.current]
	if !ok {
		s.current = "*"
	}
}

// Check checks if a certain variable (var/const/field),
// function (of fromType) or Type should be skipped.
// For variables (var/const/field) use the CheckSuffix method.
// pkg.Check("foo|var") checks only "foo|var".
func (s *Skip) Check(name string) bool {
	_, ok := s.data[s.current][name]
	return ok
}

// CheckSuffix checks if a certain variable (var/const/field) (of
// fromType) should be skipped. pkg.CheckSuffix("foo","var") checks
// both "foo" and "foo|var".
func (s *Skip) CheckSuffix(name, suffix string) bool {
	if s.Check(name) {
		return true
	}
	return s.Check(name + suffix)
}
