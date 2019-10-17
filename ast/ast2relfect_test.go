package main

import (
	"fmt"
	"go/ast"
	"os"
	"testing"

	"golang.org/x/tools/go/packages"

	"github.com/mailru/easyjson/gen"
)

func TestExprConv(t *testing.T) {
	cfg := &packages.Config{Mode: packages.NeedTypes | packages.NeedSyntax | packages.NeedTypesInfo}
	pkgs, err := packages.Load(cfg, ".")
	if err != nil {
		fmt.Fprintf(os.Stderr, "load: %v\n", err)
		t.Fatal("^^")
	}
	if packages.PrintErrors(pkgs) > 0 {
		t.Fatal("^^")
	}

	fmt.Println(pkgs)
	var v visitor

	i := pkgs[0].TypesInfo

	genny := gen.NewGenerator("x.go")

	v = func(n ast.Node) ast.Visitor {
		_, ok := n.(ast.Expr)
		if !ok {
			return v
		}
		switch v := n.(type) {
		case *ast.StructType:
			r, err := exprConv(i, v)
			if err != nil {
				fmt.Println("oh no ", err)
				return nil
			}

			genny.AddType(r)
			fmt.Printf("%s (%#v)\n", r, r)
		}
		// case *ast.TypeSpec: // TODO: allow anonymous?

		// if s, ok := n.(*ast.StructType); ok {
		// }

		return v
	}

	ast.Walk(v, pkgs[0].Syntax[0])

	genny.Run(os.Stdout)
}
