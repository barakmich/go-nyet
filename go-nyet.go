package main

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	_ "path"
	"strings"

	"flag"

	_ "golang.org/x/tools/go/gcimporter"
	"golang.org/x/tools/go/types"
	"os"
	"reflect"
)

var debug = flag.Bool("debug", false, "Enable debug printing.")
var hasErrors = false

func copyMap(a map[string]bool) map[string]bool {
	b := make(map[string]bool)
	for k, v := range a {
		b[k] = v
	}
	return b
}

func checkNoShadowBody(info fileMetadata, body []ast.Stmt, declared map[string]bool) {
	localScope := copyMap(declared)
	for _, st := range body {
		updateOrFailFromStatement(info, st, localScope)
	}
}

func updateOrFailFromExpr(info fileMetadata, exp ast.Expr, localScope map[string]bool) {
	if exp == nil {
		return
	}
	switch expv := exp.(type) {
	case *ast.Ident:
		if localScope[expv.Name] {
			pos := info.fset.Position(expv.Pos())
			fmt.Printf("%s:%d:%d:Shadowing variable `%s`\n", pos.Filename, pos.Line, pos.Column, expv.Name)
			hasErrors = true
		}
		if expv.Name != "_" && expv.Name != "err" {
			localScope[expv.Name] = true
		}
	case *ast.StarExpr:
		fmt.Println(expv.X.(*ast.SelectorExpr).Sel.String())
	default:
		fmt.Println("Err: weird assign type", reflect.TypeOf(exp))
	}
}

func updateOrFailFromStatement(info fileMetadata, st ast.Stmt, localScope map[string]bool) {
	if st == nil {
		return
	}

	switch v := st.(type) {
	// Things which we're checking
	case *ast.AssignStmt:
		if v.Tok == token.DEFINE {
			var typelist []string
			for _, rhs := range v.Rhs {
				pos := info.fset.Position(rhs.Pos())
				t := info.info.TypeOf(rhs)
				if t == nil {
					typelist = append(typelist, "meta.(type)")
					continue
				}
				typeStr := t.String()
				if strings.HasPrefix(typeStr, "(") && strings.HasSuffix(typeStr, ")") {
					typeListStr := typeStr[1 : len(typeStr)-1]
					typeList := strings.Split(typeListStr, ",")
					for _, typedecl := range typeList {
						typeNames := strings.Split(strings.TrimSpace(typedecl), " ")
						switch len(typeNames) {
						case 0:
							fmt.Printf("%s:Unknown types???\n", pos)
						case 1:
							typelist = append(typelist, typeNames[0])
						case 2:
							typelist = append(typelist, typeNames[1])
						default:
							fmt.Printf("%s:Unknown types???\n", pos)
						}
					}
				} else {
					typelist = append(typelist, typeStr)
				}

			}
			for i, expr := range v.Lhs {
				if i < len(typelist) && typelist[i] == "error" {
					continue
				}
				updateOrFailFromExpr(info, expr, localScope)
			}
		}
	case *ast.DeclStmt:
		decl := v.Decl.(*ast.GenDecl)
		if decl.Tok == token.VAR {
			for _, spec := range decl.Specs {
				s := spec.(*ast.ValueSpec)
				if typ, ok := s.Type.(*ast.Ident); ok {
					if typ.Name == "error" {
						continue
					}
				}
				for _, varname := range s.Names {
					if localScope[varname.Name] {
						pos := info.fset.Position(varname.Pos())
						fmt.Printf("%s:Shadowing variable `%s`\n", pos, varname.Name)
						hasErrors = true
					}
					localScope[varname.Name] = true
				}
			}
		} else {
			fmt.Printf("%s:Odd decl (const, import, or type) in body\n", info.fset.Position(decl.Pos()))
		}
		// Things which may recurse
	case *ast.BlockStmt:
		checkNoShadowBody(info, v.List, localScope)
	case *ast.DeferStmt:
		//TODO(barakmich): Check this
		if *debug {
			fmt.Println("TODO: Check a defer statement")
		}
	case *ast.ForStmt:
		forScope := copyMap(localScope)
		updateOrFailFromStatement(info, v.Init, forScope)
		updateOrFailFromStatement(info, v.Post, forScope)
		//TODO(barakmich): deal with cond
		//updateOrFailFromExpr(name, v.Cond, forScope)
		checkNoShadowBody(info, v.Body.List, forScope)
	case *ast.GoStmt:
		//TODO(barakmich): Check this
		if *debug {
			fmt.Println("TODO: Check a go statement")
		}
	case *ast.IfStmt:
		ifScope := copyMap(localScope)
		updateOrFailFromStatement(info, v.Init, ifScope)
		//TODO(barakmich): deal with cond
		//updateOrFailFromExpr(name, v.Cond, ifScope)
		checkNoShadowBody(info, v.Body.List, ifScope)
		ifScope = copyMap(localScope)
		updateOrFailFromStatement(info, v.Else, ifScope)
	case *ast.RangeStmt:
		rangeScope := copyMap(localScope)
		if v.Tok == token.ILLEGAL {
			pos := info.fset.Position(v.Pos())
			fmt.Printf("%s:Illegal range\n", pos)
		}
		if v.Tok == token.DEFINE {
			updateOrFailFromExpr(info, v.Key, rangeScope)
			updateOrFailFromExpr(info, v.Value, rangeScope)
		}
		checkNoShadowBody(info, v.Body.List, rangeScope)
	case *ast.SelectStmt:
		checkNoShadowBody(info, v.Body.List, localScope)
	case *ast.SwitchStmt:
		switchScope := copyMap(localScope)
		updateOrFailFromStatement(info, v.Init, switchScope)
		checkNoShadowBody(info, v.Body.List, switchScope)
	case *ast.TypeSwitchStmt:
		switchScope := copyMap(localScope)
		updateOrFailFromStatement(info, v.Init, switchScope)
		updateOrFailFromStatement(info, v.Assign, switchScope)
		checkNoShadowBody(info, v.Body.List, switchScope)
	case *ast.CaseClause:
		caseScope := copyMap(localScope)
		// Deal with cond
		//for _, expr := range v.List {
		//}
		checkNoShadowBody(info, v.Body, caseScope)
		// Things which are easy
	case *ast.CommClause:
		commScope := copyMap(localScope)
		// Deal with cond
		//for _, expr := range v.List {
		//}
		checkNoShadowBody(info, v.Body, commScope)
		// Things which are easy
	case *ast.IncDecStmt:
	case *ast.ReturnStmt:
	case *ast.LabeledStmt:
	case *ast.BranchStmt:
	case *ast.EmptyStmt:
	case *ast.SendStmt:
	case *ast.ExprStmt:
	case *ast.BadStmt:
		pos := info.fset.Position(v.Pos())
		if *debug {
			fmt.Printf("%s:Bad statement?\n", pos)
		}
	default:
		pos := info.fset.Position(v.Pos())
		if *debug {
			fmt.Println("The hell is", reflect.TypeOf(st), pos)
		}
	}
}

func checkNoShadowFuncDecl(info fileMetadata, decl *ast.FuncDecl, declared map[string]bool) {
	localScope := copyMap(declared)
	for _, r := range decl.Type.Params.List {
		for _, n := range r.Names {
			localScope[n.Name] = true
		}
		//switch v := r.Type.(type) {
		//case *ast.Ident:
		//fmt.Println(v.String())
		//case *ast.StarExpr:
		//fmt.Println(v.X.(*ast.SelectorExpr).Sel.String())
		//case *ast.MapType:
		//fmt.Println(v.)
		//default:
		//}
	}
	checkNoShadowBody(info, decl.Body.List, localScope)
}

func checkNoShadowHelper(info fileMetadata, scope *ast.Scope, declared map[string]bool) {
	localScope := copyMap(declared)
	for _, obj := range scope.Objects {
		if obj.Kind == ast.Fun {
			decl := obj.Decl.(*ast.FuncDecl)
			checkNoShadowFuncDecl(info, decl, localScope)
		}
	}
}

func CheckNoShadow(info fileMetadata, file *ast.File) {
	declared := make(map[string]bool)
	checkNoShadowHelper(info, file.Scope, declared)
}

type fileMetadata struct {
	info *types.Info
	name string
	fset *token.FileSet
}

func main() {

	flag.Parse()
	fset := token.NewFileSet()
	if len(flag.Args()) < 1 {
		fmt.Println("No path given")
		os.Exit(1)
	}
	pkgs, err := parser.ParseDir(fset, flag.Arg(0), nil, parser.AllErrors)
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
		_, err := config.Check(os.Args[1], fset, files, &info)
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
