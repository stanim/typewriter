// (c) 2015 Stani Michiels
// printast prints the ast tree to stdout of a single file
package main

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
)

func print(path string) error {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, path, nil, 0)
	if err != nil {
		return err
	}
	return ast.Print(fset, f)
}

func main() {
	if len(os.Args) != 2 {
		fmt.Println("Please provide a single filename.")
		os.Exit(1)
	}
	err := print(os.Args[1])
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}
