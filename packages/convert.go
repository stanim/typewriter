package packages

import (
	"go/ast"
	"go/token"
	"strings"
)

// kind used by genDecl
var kind = map[string]token.Token{
	"int":     token.INT,
	"int8":    token.INT,
	"int16":   token.INT,
	"int32":   token.INT,
	"int64":   token.INT,
	"uint8":   token.INT,
	"uint16":  token.INT,
	"uint32":  token.INT,
	"uint64":  token.INT,
	"float32": token.FLOAT,
	"float64": token.FLOAT,
}

// Convert source code of all packages from one type to another
// and save the converted files in toDir.
func (pkgs *Packages) Convert(fromType, toType, toDir string,
	skip Skip, imports map[string]string) error {
	for _, pkg := range *pkgs {
		if err := pkg.Convert(fromType, toType, toDir, skip,
			imports); err != nil {
			return err
		}
	}
	return nil
}

// Convert source code of a package from one type to another
// and save the converted files in toDir.
func (pkg *Package) Convert(fromType, toType, toDir string, skip Skip,
	imports map[string]string) error {
	return pkg.Walk(newConvertor(pkg, fromType, toType, toDir, skip,
		imports))
}

// identConvertor converts all ast.Ident from one type to another.
type identConvertor struct {
	fromType string
	toType   string
}

// Visit implements the ast.Visitor interface.
func (in identConvertor) Visit(node ast.Node) ast.Visitor {
	switch x := node.(type) {
	case *ast.Ident:
		if x.Name == in.fromType {
			x.Name = in.toType
		}
	}
	return in
}

// convertor converts one type to another inside package.
// (convertor is an implementation of the Visitor interface.)
type convertor struct {
	pkg      *Package
	fromType string
	toType   string
	skip     Skip
	err      error
	fromRepo string
	toRepo   string
	imports  map[string]string
}

// newConvertor creates a new convertor.
func newConvertor(pkg *Package, fromType, toType, toDir string,
	skip Skip, imports map[string]string) *convertor {
	return &convertor{
		pkg:      pkg,
		fromType: fromType,
		toType:   toType,
		skip:     skip,
		err:      nil,
		imports:  imports}
}

// Error implements the visit.Visitor interface
func (c *convertor) Error() error {
	return c.err
}

// Visit implements the ast.Visitor interface
func (c *convertor) Visit(node ast.Node) ast.Visitor {
	switch n := node.(type) {
	case *ast.AssignStmt:
		c.assignStmt(n)
	case *ast.CallExpr:
		c.callExpr(n)
	case *ast.FieldList:
		c.fieldList(n)
		return nil
	case *ast.File:
		c.skip.Update(c.pkg.Fset, n)
	case *ast.FuncDecl:
		if c.skipFunc(n.Name.Name) {
			return nil
		}
	case *ast.GenDecl:
		c.genDecl(n)
	case *ast.ImportSpec:
		c.importSpec(n)
	}
	return c
}

// convertBasicLit converts literals, for examle "5" to "5.0".
// (This is used by assignStmt and genDecl.)
func (c *convertor) convertBasicLit(expr ast.Expr) ast.Expr {
	basicLit, ok := expr.(*ast.BasicLit)
	fk, fok := kind[c.fromType]
	tk, tok := kind[c.toType]
	if !ok || !fok || !tok || basicLit.Kind != fk {
		return expr
	}
	if fk == token.INT && tk == token.FLOAT {
		basicLit.Value += ".0"
	}
	return basicLit
}

// skipField checks if a certain field of fromType should be skipped
func (c *convertor) skipField(name string) bool {
	return c.skip.CheckSuffix(name, "|field")
}

// skipField checks if a certain func should be skipped
func (c *convertor) skipFunc(name string) bool {
	return c.skip.Check(name + "|func")
}

// skipField checks if a certain func should be skipped
func (c *convertor) skipType(name string) bool {
	return c.skip.Check(name + "|type")
}

// skipField checks if a certain field of fromType should be skipped
func (c *convertor) skipVar(name string) bool {
	return c.skip.CheckSuffix(name, "|var")
}

// assignStmt converts variable assignment eg "a:=5" to "a:=5.0"
func (c *convertor) assignStmt(as *ast.AssignStmt) {
	for i, rh := range as.Rhs {
		lh, ok := as.Lhs[i].(*ast.Ident)
		if ok && c.skipVar(lh.Name) {
			continue
		}
		switch x := rh.(type) {
		case *ast.BasicLit:
			fk, fok := kind[c.fromType]
			tk, tok := kind[c.toType]
			if !ok || !fok || !tok || x.Kind != fk {
				break
			}
			if fk == token.INT && tk == token.FLOAT {
				x.Value += ".0"
			}
		case *ast.CompositeLit:
			if x.Type != nil {
				ast.Walk(identConvertor{fromType: c.fromType, toType: c.toType}, x.Type)
			}
		}
	}
}

// callExpr changes conversion eg int(a)-> a,flag.Int->flag.Float64
func (c *convertor) callExpr(ce *ast.CallExpr) {
	switch fun := ce.Fun.(type) {
	case *ast.Ident:
		switch fun.Name {
		case c.fromType:
			fun.Name = c.toType
		case "make":
			ast.Walk(identConvertor{fromType: c.fromType, toType: c.toType}, ce.Args[0])
		}
	case *ast.SelectorExpr:
		var x string
		if ident, ok := fun.X.(*ast.Ident); ok {
			x = ident.Name
		}
		switch fun.Sel.Name {
		case strings.Title(c.fromType):
			if x == "flag" || x == "rand" {
				fun.Sel.Name = strings.Title(c.toType)
			}
		case strings.Title(c.fromType) + "Var":
			if x == "flag" || x == "rand" {
				fun.Sel.Name = strings.Title(c.toType) + "Var"
			}
		case "Intn":
			if x == "rand" {
				fun.Sel.Name = strings.Title(c.toType) + "()*"
				/*
					if c.pkg.TypeStringOf(ce.Args[0]) != c.toType {
						ce.Args[0] = convert(ce.Args[0], c.toType)
					}*/
			}
		case "Atoi":
			if x == "strconv" && c.fromType == "int" {
				fun.Sel.Name = "ParseFloat"
				var value string
				switch c.toType {
				case "float64":
					value = "64"
				case "float32":
					value = "32"
				default:
					return
				}
				ce.Args = append(ce.Args,
					&ast.BasicLit{Kind: token.INT, Value: value})
				if c.toType == "float32" {
					ce.Fun = &ast.Ident{Name: "float32"}
					ce.Args = []ast.Expr{&ast.CallExpr{
						Fun: &ast.SelectorExpr{
							X:   &ast.Ident{Name: "strconv"},
							Sel: &ast.Ident{Name: "ParseFloat"},
						},
						Args: ce.Args,
					}}
				}
			}
		}
	}
}

// appendField appends idents as typstr to lst
func appendField(lst []*ast.Field, idents []*ast.Ident,
	typstr map[bool]string, skip bool, kind,
	arrayLen string) []*ast.Field {
	var typ ast.Expr
	typ = &ast.Ident{Name: typstr[skip]}
	if kind == "array" {
		if arrayLen == "" {
			typ = &ast.ArrayType{Elt: typ}
		} else {
			typ = &ast.ArrayType{Elt: typ, Len: &ast.BasicLit{Value: arrayLen}}
		}
	}
	return append(lst, &ast.Field{
		Names: idents,
		Type:  typ})
}

// fieldlist converts a field list and spits it up by fromType and
// toType (considering skip) if necessary
func (c *convertor) fieldList(fieldList *ast.FieldList) {
	lst := []*ast.Field{}
	for _, field := range fieldList.List {
		// only handle ident types for now (not eg SelectorExpr)
		var (
			identType *ast.Ident
			kind      string
			arrayLen  string
		)
		switch fieldType := field.Type.(type) {
		case *ast.Ident:
			identType = fieldType
		case *ast.ArrayType:
			var ok bool
			identType, ok = fieldType.Elt.(*ast.Ident)
			if !ok {
				lst = append(lst, field)
				continue
			}
			kind = "array"
			if basicLit, ok := fieldType.Len.(*ast.BasicLit); ok {
				arrayLen = basicLit.Value
			}
		default:
			lst = append(lst, field)
			continue
		}

		if identType.Name != c.fromType {
			lst = append(lst, field)
			continue
		}
		if field.Names == nil {
			identType.Name = c.toType
			lst = append(lst, field)
			continue
		}
		typstr := map[bool]string{
			true:  c.fromType,
			false: c.toType}
		prev := []*ast.Ident{field.Names[0]}
		prevSkip := c.skipField(prev[0].Name)
		for _, ident := range field.Names[1:] {
			skip := c.skipField(ident.Name)
			if skip == prevSkip {
				prev = append(prev, ident)
			} else {
				lst = appendField(lst, prev, typstr, prevSkip, kind, arrayLen)
				prev = []*ast.Ident{ident}
				prevSkip = skip
			}
		}
		if len(prev) != 0 {
			lst = appendField(lst, prev, typstr, prevSkip, kind, arrayLen)
		}
	}
	fieldList.List = lst
}

// genDecl converts a ast.GenDecl
func (c *convertor) genDecl(gd *ast.GenDecl) {
	for i, spec := range gd.Specs {
		switch s := spec.(type) {
		case *ast.TypeSpec:
			if c.skipType(s.Name.Name) {
				return
			}
			ast.Walk(identConvertor{c.fromType, c.toType}, s.Type)
		case *ast.ValueSpec:
			fromTo := identConvertor{c.fromType, c.toType}
			for _, expr := range s.Values {
				switch value := expr.(type) {
				case *ast.CompositeLit:
					ast.Walk(fromTo, value.Type)
				}
			}
			if s.Type != nil {
				ast.Walk(fromTo, s.Type)
			}
			if gd.Tok == token.VAR || gd.Tok == token.CONST {
				for j, val := range s.Values {
					s.Values[j] = c.convertBasicLit(val)
				}
			}
			gd.Specs[i] = s
		}
	}
}

// importSpec renames the importp paths
func (c *convertor) importSpec(is *ast.ImportSpec) {
	for impOld, impNew := range c.imports {
		is.Path.Value = strings.Replace(is.Path.Value, impOld, impNew, 1)
	}
}
