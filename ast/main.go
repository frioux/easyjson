package main

import (
	"fmt"
	"go/ast"
	"os"

	"golang.org/x/tools/go/packages"

	"github.com/mailru/easyjson/herp"
)

type zip struct {
	woot string
}

type foo struct {
	bar, baz string
	zip
	herp.External
}

type visitor func(n ast.Node) ast.Visitor

func (v visitor) Visit(n ast.Node) ast.Visitor {
	return v(n)
}

func main() {
	cfg := &packages.Config{Mode: packages.NeedTypes | packages.NeedSyntax | packages.NeedTypesInfo}
	pkgs, err := packages.Load(cfg, "pattern=.")
	if err != nil {
		fmt.Fprintf(os.Stderr, "load: %v\n", err)
		os.Exit(1)
	}
	if packages.PrintErrors(pkgs) > 0 {
		os.Exit(1)
	}

	fmt.Println(pkgs)
	var v visitor

	fset := pkgs[0].Fset
	i := pkgs[0].TypesInfo

	v = func(n ast.Node) ast.Visitor {
		e, ok := n.(ast.Expr)
		if !ok {
			return v
		}
		t := i.Types[e].Type
		switch v := n.(type) {
		case *ast.ArrayType:
			fmt.Printf("%s, %T %+v %v\n", fset.Position(n.Pos()), n, n, t)
		case *ast.ChanType:
			fmt.Printf("%s, %T %+v %v\n", fset.Position(n.Pos()), n, n, t)
		case *ast.FuncType:
			fmt.Printf("%s, %T %+v %v\n", fset.Position(n.Pos()), n, n, t)
		case *ast.InterfaceType:
			fmt.Printf("%s, %T %+v %v\n", fset.Position(n.Pos()), n, n, t)
		case *ast.MapType:
			fmt.Printf("%s, %T %+v %v\n", fset.Position(n.Pos()), n, n, t)
		case *ast.StructType:
			fmt.Printf("%s, %T %+v %v\n", fset.Position(n.Pos()), n, n, t)

			for _, f := range v.Fields.List {
				fmt.Printf(" * %s %s\n", f.Names, f.Type)
			}
		}
		// case *ast.TypeSpec: // TODO: allow anonymous?

		// if s, ok := n.(*ast.StructType); ok {
		// }

		return v
	}

	ast.Walk(v, pkgs[0].Syntax[0])
}
