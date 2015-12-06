package packages

import (
	"bytes"
	"go/ast"
	"go/format"
	"go/token"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/tools/go/ast/astutil"
)

// goPathSrc is the source gopath based on environment variable.
var goPathSrc = filepath.Join(os.Getenv("GOPATH"), "src")

// base returns the last element of path without the extension.
func base(path string) string {
	return strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
}

// callIdent wraps an argument list in another function defined by an
// ast.Ident
func callIdent(name string, args []ast.Expr) ast.Expr {
	return &ast.CallExpr{
		Fun:  &ast.Ident{Name: name},
		Args: args,
	}
}

// callIdent wraps an argument list in another function defined by a
// ast.SelectorExpr.
func callSelector(x, sel string, args []ast.Expr) ast.Expr {
	return &ast.CallExpr{
		Fun: &ast.SelectorExpr{
			X:   &ast.Ident{Name: x},
			Sel: &ast.Ident{Name: sel}},
		Args: args,
	}
}

// convert an expression to another type, for example e -> int(e)
func convert(e ast.Expr, toType string) ast.Expr {
	if e == nil {
		return e
	}
	return &ast.CallExpr{
		Fun:  &ast.Ident{Name: toType},
		Args: []ast.Expr{astutil.Unparen(e)},
	}
}

// Filename of an ast.Node.
func Filename(fset *token.FileSet, node ast.Node) string {
	return fset.Position(node.Pos()).Filename
}

// GoSrcPath returns the full file path of a go repository.
func GoPathSrc(pkg string) string {
	return filepath.Join(goPathSrc, pkg)
}

// files returns the files of a package as a list (instead of map).
func files(pkg *ast.Package) []*ast.File {
	mp := pkg.Files
	lst := make([]*ast.File, len(mp))
	i := 0
	for _, f := range mp {
		lst[i] = f
		i++
	}
	return lst
}

// index is a generic function to find an element in a list.
func index(length int, cond func(i int) bool) int {
	for i := 0; i < length; i++ {
		if cond(i) {
			return i
		}
	}
	return -1
}

// indexExpr find the index of a node in a list of expressions.
func indexExpr(lst []ast.Expr, node ast.Node) int {
	return index(len(lst), func(i int) bool { return lst[i] == node })
}

// mapset converts a map of string slices to map of sets.
func mapset(mapstrs map[string][]string) map[string]Set {
	mms := map[string]Set{}
	for key, strs := range mapstrs {
		mms[key] = newSet(strs)
	}
	return mms
}

// Repo converts an absolute path to an import path.
func Repo(path string) string {
	return strings.Replace(path, goPathSrc, "", 1)[1:]
}

// SaveFile saves an ast.File to a specific filename.
func SaveFile(fset *token.FileSet, f *ast.File, filename string) error {
	fo, err := os.Create(filename)
	if err != nil {
		return err
	}
	defer fo.Close()
	if err := format.Node(fo, fset, f); err != nil {
		return err
	}
	return nil
}

// newSet converts list of strings into map for member checking.
func newSet(strs []string) Set {
	ms := Set{}
	for _, s := range strs {
		ms[s] = struct{}{}
	}
	return ms
}

// str returns the source code of a node as string.
func str(fset *token.FileSet, node ast.Node) (string, error) {
	var buf bytes.Buffer
	if err := format.Node(&buf, fset, node); err != nil {
		return "", err
	}
	return buf.String(), nil
}
