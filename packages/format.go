// Some of this code in this file is simplified from:
// github.com/golang/tools/tree/master/cmd/vet
// Copyright 2010 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package packages

import (
	"fmt"
	"go/ast"
	"go/token"
	"regexp"
	"strings"

	"golang.org/x/tools/go/exact"
)

var (
	// verbRe is a regular expression to find verbs (eg "%2.3f")
	// It is not syntactically correct, but fine enough for this use case.
	verbRe = regexp.MustCompile(`[%][.*+_ 0-9\[\]]*[a-z]`)
	// verbRune maps a type string to a verb rune
	verbRune = map[string]uint8{
		"byte":    'd',
		"int":     'd',
		"int8":    'd',
		"int16":   'd',
		"int32":   'd',
		"int64":   'd',
		"uint8":   'd',
		"uint16":  'd',
		"uint32":  'd',
		"uint64":  'd',
		"float32": 'f',
		"float64": 'f',
	}
)

// Format fixes string format functions such as Printf, Errorf, ...
// in all packages.
func (pkgs *Packages) Format(fromType string, formatVar map[string]struct{}, formatFunc map[string]string, printf map[string]int) error {
	for _, pkg := range *pkgs {
		if err := pkg.Format(fromType, formatVar, formatFunc, printf); err != nil {
			return err
		}
	}
	return nil
}

// Format fixes string format functions such as Printf, Errorf, ...
// in a package.
func (pkg *Package) Format(fromType string,
	formatVar map[string]struct{}, formatFunc map[string]string,
	printf map[string]int) error {
	return pkg.Walk(newFormatter(pkg, fromType, formatVar, formatFunc, printf))
}

// formatter fixes format strings in the files of a package
// based on the types of the arguments. formatter is an implementation
// of the Visitor interface.
type formatter struct {
	pkg         *Package
	fromRune    uint8
	formatVars  map[string]struct{}
	formatVar   bool
	formatFuncs map[string]string
	formatFunc  string
	printf      map[string]int
	err         error
}

// newFormatter creates a new formatter.
func newFormatter(pkg *Package, fromType string,
	formatVar map[string]struct{}, formatFunc map[string]string,
	printf map[string]int) *formatter {
	return &formatter{
		pkg:         pkg,
		fromRune:    verbRune[fromType],
		formatVars:  formatVar,
		formatFuncs: formatFunc,
		printf:      printf,
		err:         nil,
	}
}

// Error implements the packages.Visitor interface.
func (f *formatter) Error() error {
	return f.err
}

// Visit implements the ast.Visitor interface.
func (f *formatter) Visit(node ast.Node) ast.Visitor {
	switch n := node.(type) {
	case *ast.CallExpr:
		return f.callExpr(n)
	case *ast.File:
		base := base(Filename(f.pkg.Fset, n))
		_, f.formatVar = f.formatVars[base]
		f.formatFunc = f.formatFuncs[base]
	}
	return f
}

// setError sets the string as an error
func (f *formatter) setError(format string, a ...interface{}) {
	f.err = fmt.Errorf(format, a...)
}

// callExpr handles a CallExpr node
func (f *formatter) callExpr(call *ast.CallExpr) ast.Visitor {
	var name string
	switch fun := call.Fun.(type) {
	case *ast.Ident:
		name = fun.Name
	case *ast.SelectorExpr:
		name = fun.Sel.Name
	default:
		return f
	}
	formatIndex, ok := f.printf[strings.ToLower(name)]
	if !ok {
		return f
	}
	f.checkPrintf(call, name, formatIndex)
	if f.err != nil {
		return nil
	}
	return f
}

// checkPrintf checks a call to a formatted print routine such as
// Printf. call.Args[formatIndex] is (well, should be) the format
// argument.
// see https://github.com/golang/tools/blob/master/cmd/vet/print.go#L149
func (f *formatter) checkPrintf(call *ast.CallExpr, name string,
	formatIndex int) {
	n := len(call.Args)
	if formatIndex >= n {
		f.setError("too few arguments in call to", name)
		return
	}
	arg := call.Args[formatIndex]
	lit := f.pkg.Info.Types[arg].Value
	if lit == nil {
		// allow non-constant format by filename
		if !f.formatVar {
			// same warning as eg "go tool vet -v planets.go"
			src, _ := str(f.pkg.Fset, arg)
			f.setError("%s: can't check non-constant format %q in call to %s.",
				f.pkg.Fset.Position(arg.Pos()), src, name)
		}
		return
	}
	if lit.Kind() != exact.String {
		f.setError("Format is not string", call.Args[formatIndex])
		return
	}
	format := exact.StringVal(lit)
	// Arguments are immediately after format string.
	firstArg := formatIndex + 1
	if !strings.Contains(format, "%") {
		if n > firstArg {
			f.setError("no formatting directive in %s call", name)
		}
		return
	}
	verbs := verbRe.FindAllStringIndex(format, -1)
	argNum := firstArg + len(verbs)
	if argNum != n {
		expect := argNum - firstArg
		numArgs := n - firstArg
		f.setError("wrong number of args for format in %s call: "+
			"%d needed but %d args", name, expect, numArgs)
		return
	}
	args := call.Args[:firstArg]
	for i, vIndex := range verbs {
		start := vIndex[0]
		end := vIndex[1]
		verb := format[start:end]
		arg := call.Args[firstArg+i]
		formatRune := verb[len(verb)-1]
		if formatRune != f.fromRune {
			args = append(args, arg)
			continue
		}
		argType := f.pkg.Info.TypeOf(arg)
		if argType == nil {
			// unknown type, keep as it is
			args = append(args, arg)
			continue
		}
		argType = argType.Underlying()
		argTypeStr := argType.String()
		argRune, ok := verbRune[argTypeStr]
		if !ok {
			f.setError("No rune for type %q %q", argTypeStr)
			return
		}
		if formatRune == argRune {
			args = append(args, arg)
			continue
		}
		if f.formatFunc == "" {
			// only replace verb rune (eg %d -> %f)
			// leave arg untouched
			format = format[:end-1] + string(argRune) + format[end:]
			args = append(args, arg)
		} else {
			// replace whole verb with %s
			// replace arg with formatFunc(arg)
			format = format[:start] + "%s" + format[end:]
			args = append(args, &ast.CallExpr{
				Fun:  &ast.Ident{Name: f.formatFunc},
				Args: []ast.Expr{arg},
			})
		}
	}
	call.Args = args
	call.Args[formatIndex] = &ast.BasicLit{
		Kind: token.STRING, Value: fmt.Sprintf("%q", format)}
}
