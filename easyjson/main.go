package main

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
	"go/ast"
	"go/format"
	"go/token"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	"github.com/mailru/easyjson/gen"
	"golang.org/x/tools/go/packages"
)

var (
	buildTags             = flag.String("build_tags", "", "build tags to add to generated file")
	snakeCase             = flag.Bool("snake_case", false, "use snake_case names instead of CamelCase by default")
	lowerCamelCase        = flag.Bool("lower_camel_case", false, "use lowerCamelCase names instead of CamelCase by default")
	noStdMarshalers       = flag.Bool("no_std_marshalers", false, "don't generate MarshalJSON/UnmarshalJSON funcs")
	omitEmpty             = flag.Bool("omit_empty", false, "omit empty fields by default")
	allStructs            = flag.Bool("all", false, "generate marshaler/unmarshalers for all structs in a file") // XXX
	stubs                 = flag.Bool("stubs", false, "only generate stubs for marshaler/unmarshaler funcs")     // XXX
	noformat              = flag.Bool("noformat", false, "do not run 'gofmt -w' on output file")
	specifiedName         = flag.String("output_filename", "", "specify the filename of the output")
	processPkg            = flag.Bool("pkg", false, "process the whole package instead of just the given file")
	disallowUnknownFields = flag.Bool("disallow_unknown_fields", false, "return error if any unknown field in json appeared")
)

func generate(fname string) (err error) {
	cfg := &packages.Config{Mode: packages.NeedName | packages.NeedTypes | packages.NeedSyntax | packages.NeedTypesInfo | packages.NeedTypesSizes}

	var pkgs []*packages.Package

	if *processPkg {
		pkgs, err = packages.Load(cfg, "pattern="+fname)
		if err != nil {
			return fmt.Errorf("load: %s", err)
		}
	} else {
		pkgs, err = packages.Load(cfg, "file="+fname)
		if err != nil {
			return fmt.Errorf("load: %s", err)
		}
	}

	if packages.PrintErrors(pkgs) > 0 {
		os.Exit(1)
	}

	if len(pkgs) > 1 {
		return errors.New("more than one package found")
	}

	if len(pkgs) == 0 {
		return errors.New("no packages found")
	}

	pkg := pkgs[0]

	genny := gen.NewGenerator(fname)
	genny.SetPkg(pkg.Name, pkg.PkgPath)
	if *buildTags != "" {
		genny.SetBuildTags(fmt.Sprintf("%q", *buildTags))
	}
	if *snakeCase {
		genny.UseSnakeCase()
	}
	if *lowerCamelCase {
		genny.UseLowerCamelCase()
	}
	if *omitEmpty {
		genny.OmitEmpty()
	}
	if *noStdMarshalers {
		genny.NoStdMarshalers()
	}
	if *disallowUnknownFields {
		genny.DisallowUnknownFields()
	}

	for _, f := range pkg.Syntax {
		cmap := ast.NewCommentMap(pkg.Fset, f, f.Comments)

		for n, cg := range cmap {
			for _, c := range cg {
				for _, l := range c.List {
					if l.Text != "//easyjson:json" {
						continue
					}
					if g, ok := n.(*ast.GenDecl); ok {
						if g.Tok != token.TYPE {
							continue
						}
						for _, s := range g.Specs {
							ts := s.(*ast.TypeSpec)
							typ := pkg.TypesInfo.Defs[ts.Name].Type()
							genny.Add(typ)
						}
					}
				}
			}
		}
	}

	fInfo, err := os.Stat(fname)
	if err != nil {
		return err
	}

	var outName string
	if fInfo.IsDir() {
		outName = filepath.Join(fname, pkg.Name+"_easyjson.go")
	} else {
		if s := strings.TrimSuffix(fname, ".go"); s == fname {
			return errors.New("Filename must end in '.go'")
		} else {
			outName = s + "_easyjson.go"
		}
	}

	if *specifiedName != "" {
		outName = *specifiedName
	}

	buf := &bytes.Buffer{}

	if err := genny.Run(buf); err != nil {
		fmt.Fprintf(os.Stderr, "Couldn't run: %s", err)
	}

	f, err := os.Create(outName)
	if err != nil {
		return err
	}
	defer f.Close()
	if *noformat {
		_, err := io.Copy(f, buf)
		return err
	}

	out, err := format.Source(buf.Bytes())
	if err != nil {
		return err
	}
	return ioutil.WriteFile(outName, out, 0644)
}

func main() {
	flag.Parse()

	files := flag.Args()

	for _, f := range files {
		if err := generate(f); err != nil {
			fmt.Fprintf(os.Stderr, "Couldn't generate for %s: %s\n", f, err)
			os.Exit(1)
		}
	}
}
