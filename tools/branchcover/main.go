// branchcover instruments a temporary source tree, never the delivered binary.
package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strconv"
)

type Branch struct {
	ID        string   `json:"id"`
	File      string   `json:"file"`
	Line      int      `json:"line"`
	Function  string   `json:"function"`
	Kind      string   `json:"kind"`
	Condition string   `json:"condition"`
	Outcomes  []string `json:"outcomes"`
}

func must(err error) {
	if err != nil {
		panic(err)
	}
}
func literal(s string) ast.Expr { return &ast.BasicLit{Kind: token.STRING, Value: strconv.Quote(s)} }
func call(name, id string, args ...ast.Expr) *ast.CallExpr {
	return &ast.CallExpr{Fun: &ast.SelectorExpr{X: ast.NewIdent("bp"), Sel: ast.NewIdent(name)}, Args: append([]ast.Expr{literal(id)}, args...)}
}
func main() {
	root := flag.String("root", ".", "temporary project tree")
	manifest := flag.String("manifest", "branches-manifest.json", "output path")
	flag.Parse()
	var branches []Branch
	for _, pkg := range []string{"service", "repository", "handler"} {
		relative := "internal/" + pkg + "/" + map[string]string{"service": "service.go", "repository": "sqlite.go", "handler": "handler.go"}[pkg]
		path := filepath.Join(*root, relative)
		fs := token.NewFileSet()
		file, err := parser.ParseFile(fs, path, nil, parser.ParseComments)
		must(err)
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil {
				continue
			}
			ast.Inspect(fn.Body, func(node ast.Node) bool {
				switch n := node.(type) {
				case *ast.IfStmt:
					pos := fs.Position(n.If)
					id := fmt.Sprintf("%s:%d:if", relative, pos.Line)
					var condition bytes.Buffer
					must(format.Node(&condition, fs, n.Cond))
					branches = append(branches, Branch{id, relative, pos.Line, fn.Name.Name, "if", condition.String(), []string{"true", "false"}})
					n.Cond = call("Test", id, n.Cond)
				case *ast.ForStmt:
					if n.Cond == nil {
						break
					}
					pos := fs.Position(n.For)
					id := fmt.Sprintf("%s:%d:for", relative, pos.Line)
					var condition bytes.Buffer
					must(format.Node(&condition, fs, n.Cond))
					branches = append(branches, Branch{id, relative, pos.Line, fn.Name.Name, "for", condition.String(), []string{"true", "false"}})
					n.Cond = call("Test", id, n.Cond)
				case *ast.SwitchStmt:
					hasDefault := false
					for i, entry := range n.Body.List {
						clause := entry.(*ast.CaseClause)
						outcome := fmt.Sprintf("case-%d", i)
						if clause.List == nil {
							hasDefault = true
							outcome = "default"
						}
						pos := fs.Position(clause.Case)
						id := fmt.Sprintf("%s:%d:%s", relative, pos.Line, outcome)
						var condition bytes.Buffer
						for _, v := range clause.List {
							must(format.Node(&condition, fs, v))
						}
						branches = append(branches, Branch{id, relative, pos.Line, fn.Name.Name, "switch", condition.String(), []string{"taken"}})
						clause.Body = append([]ast.Stmt{&ast.ExprStmt{X: call("Case", id)}}, clause.Body...)
					}
					if !hasDefault {
						pos := fs.Position(n.Switch)
						id := fmt.Sprintf("%s:%d:implicit-default", relative, pos.Line)
						branches = append(branches, Branch{id, relative, pos.Line, fn.Name.Name, "switch", "implicit default", []string{"taken"}})
						n.Body.List = append(n.Body.List, &ast.CaseClause{Body: []ast.Stmt{&ast.ExprStmt{X: call("Case", id)}}})
					}
				}
				return true
			})
		}
		importDecl := &ast.GenDecl{Tok: token.IMPORT, Specs: []ast.Spec{&ast.ImportSpec{Name: ast.NewIdent("bp"), Path: &ast.BasicLit{Kind: token.STRING, Value: strconv.Quote("example.com/tasks/internal/branchprobe")}}}}
		file.Decls = append([]ast.Decl{importDecl}, file.Decls...)
		var output bytes.Buffer
		must(format.Node(&output, fs, file))
		must(os.WriteFile(path, output.Bytes(), 0600))
	}
	data, err := json.MarshalIndent(branches, "", "  ")
	must(err)
	must(os.WriteFile(*manifest, append(data, '\n'), 0600))
}
