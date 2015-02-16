package main

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	_ "path"
	"path/filepath"

	"flag"

	_ "golang.org/x/tools/go/gcimporter"
	"golang.org/x/tools/go/types"
	"os"
)

var debug = flag.Bool("debug", false, "Enable debug printing.")
var hasErrors = false

type fileMetadata struct {
	info *types.Info
	name string
	fset *token.FileSet
}

func main() {
	flag.Parse()
	if len(flag.Args()) < 1 {
		fmt.Println("No path given")
		os.Exit(1)
	}
	st, err := os.Stat(flag.Arg(0))
	if err != nil {
		fmt.Println("Error:", err)
		os.Exit(1)
	}
	if st.IsDir() {
		dirMain(flag.Arg(0))
		return
	}
	fileMain(flag.Arg(0))
}

func fileMain(path string) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, path, nil, parser.AllErrors)
	if err != nil {
		fmt.Println("Error parsing:", err)
		os.Exit(1)
	}
	info := types.Info{
		Types: make(map[ast.Expr]types.TypeAndValue),
	}
	config := new(types.Config)
	_, err = config.Check(path, fset, []*ast.File{file}, &info)
	if err != nil {
		fmt.Println("Error package checker:", err)
	}
	fileinfo := fileMetadata{
		info: &info,
		name: file.Name.Name,
		fset: fset,
	}
	CheckNoShadow(fileinfo, file)
	if hasErrors {
		os.Exit(1)
	}
}

func dirMain(path string) {
	fset := token.NewFileSet()
	pkgs, err := parser.ParseDir(fset, path, nil, parser.AllErrors)
	if err != nil {
		fmt.Println("Error parsing:", err)
		os.Exit(1)
	}
	for _, pkg := range pkgs {
		//fmt.Println(pName)
		info := types.Info{
			Types: make(map[ast.Expr]types.TypeAndValue),
		}
		var files []*ast.File
		for _, f := range pkg.Files {
			files = append(files, f)
		}
		config := new(types.Config)
		_, err := config.Check(path, fset, files, &info)
		if err != nil {
			fmt.Println("Error package checker:", err)
			//os.Exit(1)
		}
		//TODO(barakmich): Consider root scope.
		for fName, file := range pkg.Files {
			fileinfo := fileMetadata{
				info: &info,
				name: fName,
				fset: fset,
			}
			CheckNoShadow(fileinfo, file)
		}
	}
	if hasErrors {
		os.Exit(1)
	}
}

func visit(path string, f os.FileInfo, err error) error {
	if err != nil {
		fmt.Println("Walk error:", err)
		return err
	}
	if !f.IsDir() {
		return nil
	}
	//doPackageDir(path)
	return nil
}

func dirRec(path string) {
	filepath.Walk(path, visit)
}
