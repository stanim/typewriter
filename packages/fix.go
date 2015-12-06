package packages

import (
	"fmt"
	"go/ast"
	"go/token"
	"regexp"
	"strings"

	"golang.org/x/tools/go/ast/astutil"
	"golang.org/x/tools/go/types"
)

type (
	// conflict gives context to a types.Error
	conflict struct {
		file   *ast.File
		path   []ast.Node
		err    types.Error
		fixErr error // non nil if error during fixing
	}
	// regexHandler used for convenience by pkg.re()
	regexHandler func(pkg *Package, confl conflict, fromType,
		toType string, matches []string) error
)

// regular expressions to diagnose type errors
var (
	argRe      = regexp.MustCompile(`cannot pass argument .+? of type (\*?\w+)\) to parameter of type (\*?\w+)`)
	chanRe     = regexp.MustCompile(`cannot send .+?variable of type (\*?\w+)\) to channel uniq \(variable of type chan (\*?\w+)\)`)
	indexRe    = regexp.MustCompile(`index .+? must be integer`)
	mismatchRe = regexp.MustCompile(`mismatched types (\w+) and (\w+)`)
	modRe      = regexp.MustCompile(`operator [%] not defined`)
	returnRe   = regexp.MustCompile(`cannot return (.+?) \(variable of type (\w+)\) as value of type (\w+)`)
	truncRe    = regexp.MustCompile(`truncated to int`)
)

// Fix type conflicts in all packages
func (pkgs *Packages) Fix(fromType, toType string) (int, error) {
	count := 0
	for _, pkg := range *pkgs {
		n, err := pkg.Fix(fromType, toType)
		if err != nil {
			return count, err
		}
		count += n
	}
	return count, nil
}

// Fix type conflicts in a package
func (pkg *Package) Fix(fromType, toType string) (int, error) {
	if len(pkg.Errors) == 0 {
		return 0, nil
	}
	conflicts, err := pkg.conflicts()
	if err != nil {
		return 0, err
	}
	// fix type conflicts
	fixedFiles := map[*ast.File]struct{}{}
	for _, confl := range conflicts {
		// find the appropriate fix with regular expressions
		switch {
		case pkg.re(argRe, fixArg, confl, fromType, toType),
			pkg.re(chanRe, fixChan, confl, fromType, toType),
			pkg.re(indexRe, fixIndex, confl, fromType, toType),
			pkg.re(mismatchRe, fixMismatch, confl, fromType, toType),
			pkg.re(modRe, fixMod, confl, fromType, toType),
			pkg.re(returnRe, fixReturn, confl, fromType, toType),
			pkg.re(truncRe, fixTrunc, confl, fromType, toType):
		default:
			confl.fixErr = fmt.Errorf("fix unknown: %s", confl.err)
		}
		if confl.fixErr != nil {
			// do not return immediately, first save fixes
			err = confl.fixErr
			break
		}
		fixedFiles[confl.file] = struct{}{}
	}
	// save fixes & snippets
	for f := range fixedFiles {
		SaveFile(pkg.Fset, f, Filename(pkg.Fset, f))
	}
	pkg.saveSnippets()
	if err != nil {
		return 0, err
	}
	return len(pkg.Errors), nil
}

// conflicts collects all type errors
func (pkg *Package) conflicts() ([]conflict, error) {
	var conflicts []conflict
	// construct paths first (correct pos)
	for _, e := range pkg.Errors {
		// error
		err, ok := e.(types.Error)
		if !ok {
			return nil, fmt.Errorf("Expected types.Error, got %q.", e)
		}
		confl := conflict{err: err}
		// file
		pos := err.Pos
		confl.file = pkg.File(pos)
		if confl.file == nil {
			return nil, fmt.Errorf("No enclosing file found for error %q.", e)
		}
		// path
		path, _ := astutil.PathEnclosingInterval(confl.file, pos, pos+1)
		if len(path) == 0 {
			return nil, fmt.Errorf("No path found for %q",
				pkg.Fset.Position(pos))
		}
		prev := path[0].Pos()
		// filter out irrelevant path components with same pos
		for i := 1; i < len(path); i++ {
			_, basicOk := path[i-1].(*ast.BasicLit)
			_, callOk := path[i-1].(*ast.CallExpr)
			_, identOk := path[i-1].(*ast.Ident)
			_, parenOk := path[i-1].(*ast.ParenExpr)
			_, selectorOk := path[i-1].(*ast.SelectorExpr)
			if path[i].Pos() != prev || !(basicOk || callOk || identOk || parenOk || selectorOk) {
				if i-1 > 0 {
					path = path[i-1:]
				}
				break
			}
		}
		confl.path = path

		if true {
			fmt.Println(err)
			pkg.printPath(path)
			fmt.Println()
		}

		conflicts = append(conflicts, confl)
	}
	return conflicts, nil
}

// re checks conflict error message with a regular expression. If it
// matches, it invokes the corresponding fix* handler. If the fix
// is succesful, return true otherwise false so it can be sequenced
// in a switch statement.
func (pkg *Package) re(re *regexp.Regexp, handler regexHandler,
	confl conflict, fromType, toType string) bool {
	if matches := re.FindAllStringSubmatch(confl.err.Msg, 1); len(matches) > 0 {
		if err := handler(pkg, confl, fromType, toType,
			matches[0]); err != nil {
			confl.fixErr = err
		}
		return true
	}
	return false
}

// fixArg fixes argument type errors
func fixArg(pkg *Package, confl conflict, fromType, toType string,
	matches []string) error {
	p1 := confl.path[1]
	callExpr1, ok := p1.(*ast.CallExpr)
	if !ok {
		return fmt.Errorf("fixArgument expects CallExpr for error %q", confl.err)
	}
	args := callExpr1.Args
	p0 := confl.path[0]
	ia := indexExpr(args, p0)
	switch n0 := p0.(type) {
	case *ast.CallExpr:
		switch fun := n0.Fun.(type) {
		case *ast.Ident:
			if len(n0.Args) == 1 && fun.Name == fromType {
				if pkg.TypeStringOf(n0.Args[0]) == toType {
					args[ia] = n0.Args[0]
				} else {
					fun.Name = toType
				}
				return nil
			}
		}
	}
	if !strings.HasPrefix(matches[2], "*") {
		// pkg.printPath(confl.path)
		args[ia] = &ast.CallExpr{
			Fun:  &ast.Ident{Name: matches[2]},
			Args: []ast.Expr{astutil.Unparen(args[ia])},
		}
		return nil
	}
	return fmt.Errorf("fixArgument unknown: %s", confl.err)
}

// fixChan fixes channel type errors
func fixChan(pkg *Package, confl conflict, fromType, toType string,
	matches []string) error {
	switch p0 := confl.path[0].(type) {
	case *ast.SendStmt:
		p0.Value = convert(p0.Value, matches[2])
	}
	return nil
}

// fixIndex fixes index type errors (should always be int)
func fixIndex(pkg *Package, confl conflict, fromType, toType string,
	matches []string) error {
	switch p1 := confl.path[1].(type) {
	case *ast.IndexExpr:
		p1.Index = convert(p1.Index, "int")
	case *ast.SliceExpr:
		if pkg.TypeStringOf(p1.Low) != "int" {
			p1.Low = convert(p1.Low, "int")
		}
		if pkg.TypeStringOf(p1.High) != "int" {
			p1.High = convert(p1.High, "int")
		}
	case *ast.CallExpr:
		p0 := confl.path[0]
		i := indexExpr(p1.Args, p0)
		p1.Args[i] = convert(p1.Args[i], "int")
	default:
		return fmt.Errorf("fixIndex expects CallExpr, IndexExpr or SliceExpr, got %#v: %s",
			confl.path[1], confl.err)
	}
	return nil
}

// fixMismatch fixes type mismatch errors
func fixMismatch(pkg *Package, confl conflict, fromType, toType string,
	matches []string) error {
	for _, p := range confl.path {
		switch n := p.(type) {
		case *ast.AssignStmt:
			if len(n.Rhs) > 1 {
				return fmt.Errorf("fixMismatch expects single assignment: %s",
					confl.err)
			}
			if matches[1] != toType || matches[2] != fromType {
				return fmt.Errorf("fixMismatch for AssignStmt unknown: %s",
					confl.err)
			}
			n.Rhs[0] = convert(n.Rhs[0], toType)
			return nil
		case *ast.BinaryExpr:
			to := toType
			if n.Op == token.REM {
				to = "int"
			}
			if matches[1] != to {
				n.X = convert(n.X, to)
			}
			if matches[2] != to {
				n.Y = convert(n.Y, to)
			}
			return nil
		}
	}
	return fmt.Errorf("fixMismatch expect AssignStmt or BinaryExpr: %s", confl.err)
}

/*
func fixMismatch(pkg *Package, confl conflict, fromType, toType string,
	matches []string) error {
	switch n := confl.path[0].(type) {
	case *ast.AssignStmt:
		if len(n.Rhs) > 1 {
			return fmt.Errorf("expected single assignment: %s",
				confl.err)
		}
		if matches[1] == toType && matches[2] == fromType {
			n.Rhs[0] = convert(n.Rhs[0], toType)
		} else {
			return fmt.Errorf("fix for AssignStmt unknown: %s",
				confl.err)
		}
	case *ast.BinaryExpr:
		to := toType
		if n.Op == token.REM {
			to = "int"
		}
		if matches[1] != to {
			n.X = convert(n.X, to)
		}
		if matches[2] != to {
			n.Y = convert(n.Y, to)
		}
	default:
		pos := confl.err.Pos
		f := pkg.File(pos)
		path, _ := astutil.PathEnclosingInterval(f, pos, pos+1)
		pkg.printPath(path)
		return fmt.Errorf("expected AssignStmt or BinaryExpr: %s", confl.err)
	}
	return nil
}
*/
// fixMod fixes mod type errors (should be int)
func fixMod(pkg *Package, confl conflict, fromType, toType string,
	matches []string) error {
	var (
		node ast.Node
		n    *ast.BinaryExpr
		ok   bool
	)
	for _, node = range confl.path {
		if n, ok = node.(*ast.BinaryExpr); ok && n.Op == token.REM {
			break
		}
	}
	if n == nil {
		return fmt.Errorf("fixMod expects %%: %s", confl.err)
	}
	n.X = convert(n.X, "int")
	n.Y = convert(n.Y, "int")
	return nil
}

// fixReturn fixes a wrong return type
func fixReturn(pkg *Package, confl conflict, fromType, toType string,
	matches []string) error {
	fmt.Printf("%#v", matches)
	return fmt.Errorf("bla")
}

// fixTrunc fixes truncate to int for float constants
func fixTrunc(pkg *Package, confl conflict, fromType, toType string,
	matches []string) error {
	p0 := confl.path[0]
	switch p1 := confl.path[1].(type) {
	case *ast.CallExpr:
		i := indexExpr(p1.Args, p0)
		e := p1.Args[i]
		var name string
		typ := pkg.TypeStringOf(e)
		if typ == "untyped float" {
			typ = toType
		}
		switch typ {
		case "float32":
			name = "i32"
		case "float64":
			name = "i64"
		default:
			return fmt.Errorf(
				"fixTrunc expects type float32 or float64 to trunc to int, got %q: %s",
				typ, confl.err)
		}
		pkg.AddSnippet(name)
		p1.Args[i] = callIdent(name, []ast.Expr{e})
	default:
		return fmt.Errorf("fixTrunc expects CallExpr to trunc to int, got %#v: %s",
			confl.path[1], confl.err)
	}
	return nil
}
