package obfuscator

import (
	"fmt"
	"go/ast"
	"go/token"
	"strings"
)

// shouldSkipJunkCodeInjection 确定是否不应向函数注入垃圾代码
func (o *Obfuscator) shouldSkipJunkCodeInjection(fn *ast.FuncDecl) bool {
	if fn.Name.Name == "init" {
		return true
	}

	if fn.Name.Name == "main" && fn.Recv == nil {
		return true
	}

	if fn.Doc != nil {
		for _, comment := range fn.Doc.List {
			if strings.HasPrefix(comment.Text, "//go:") {
				return true
			}
		}
	}

	if fn.Body != nil && len(fn.Body.List) <= 2 {
		return true
	}

	return false
}

// generateJunkStatements 生成带有不透明谓词的垃圾代码语句
func (o *Obfuscator) generateJunkStatements(hasReturn bool) []ast.Stmt {
	junkVarName1 := fmt.Sprintf("l%s", generateRandomString(8))
	junkVarName2 := fmt.Sprintf("l%s", generateRandomString(8))
	junkVarName3 := fmt.Sprintf("l%s", generateRandomString(8))

	stmts := []ast.Stmt{
		// 不透明谓词 1: x*x >= 0 (总是为真)
		&ast.AssignStmt{
			Lhs: []ast.Expr{&ast.Ident{Name: junkVarName1}},
			Tok: token.DEFINE,
			Rhs: []ast.Expr{&ast.BasicLit{Kind: token.INT, Value: "42"}},
		},
		&ast.IfStmt{
			Cond: &ast.BinaryExpr{
				X: &ast.BinaryExpr{
					X:  &ast.Ident{Name: junkVarName1},
					Op: token.MUL,
					Y:  &ast.Ident{Name: junkVarName1},
				},
				Op: token.GEQ,
				Y:  &ast.BasicLit{Kind: token.INT, Value: "0"},
			},
			Body: &ast.BlockStmt{
				List: []ast.Stmt{
					&ast.AssignStmt{
						Lhs: []ast.Expr{&ast.Ident{Name: junkVarName1}},
						Tok: token.ASSIGN,
						Rhs: []ast.Expr{
							&ast.BinaryExpr{
								X:  &ast.Ident{Name: junkVarName1},
								Op: token.ADD,
								Y:  &ast.BasicLit{Kind: token.INT, Value: "1"},
							},
						},
					},
				},
			},
		},

		// 不透明谓词 2: (x^2 + x) % 2 == 0 (偶数 x 总是为真)
		&ast.AssignStmt{
			Lhs: []ast.Expr{&ast.Ident{Name: junkVarName2}},
			Tok: token.DEFINE,
			Rhs: []ast.Expr{&ast.BasicLit{Kind: token.INT, Value: "10"}},
		},
		&ast.IfStmt{
			Cond: &ast.BinaryExpr{
				X: &ast.BinaryExpr{
					X: &ast.BinaryExpr{
						X: &ast.BinaryExpr{
							X:  &ast.Ident{Name: junkVarName2},
							Op: token.MUL,
							Y:  &ast.Ident{Name: junkVarName2},
						},
						Op: token.ADD,
						Y:  &ast.Ident{Name: junkVarName2},
					},
					Op: token.REM,
					Y:  &ast.BasicLit{Kind: token.INT, Value: "2"},
				},
				Op: token.EQL,
				Y:  &ast.BasicLit{Kind: token.INT, Value: "0"},
			},
			Body: &ast.BlockStmt{
				List: []ast.Stmt{
					&ast.AssignStmt{
						Lhs: []ast.Expr{&ast.Ident{Name: junkVarName2}},
						Tok: token.ASSIGN,
						Rhs: []ast.Expr{
							&ast.BinaryExpr{
								X:  &ast.Ident{Name: junkVarName2},
								Op: token.MUL,
								Y:  &ast.BasicLit{Kind: token.INT, Value: "2"},
							},
						},
					},
				},
			},
		},

		// 不透明谓词 3: 2*x > x (正数 x 总是为真)
		&ast.AssignStmt{
			Lhs: []ast.Expr{&ast.Ident{Name: junkVarName3}},
			Tok: token.DEFINE,
			Rhs: []ast.Expr{&ast.BasicLit{Kind: token.INT, Value: "5"}},
		},
		&ast.IfStmt{
			Cond: &ast.BinaryExpr{
				X: &ast.BinaryExpr{
					X:  &ast.BasicLit{Kind: token.INT, Value: "2"},
					Op: token.MUL,
					Y:  &ast.Ident{Name: junkVarName3},
				},
				Op: token.GTR,
				Y:  &ast.Ident{Name: junkVarName3},
			},
			Body: &ast.BlockStmt{
				List: []ast.Stmt{
					&ast.AssignStmt{
						Lhs: []ast.Expr{&ast.Ident{Name: junkVarName3}},
						Tok: token.ASSIGN,
						Rhs: []ast.Expr{
							&ast.BinaryExpr{
								X:  &ast.Ident{Name: junkVarName3},
								Op: token.SUB,
								Y:  &ast.BasicLit{Kind: token.INT, Value: "1"},
							},
						},
					},
				},
			},
		},

		// 永远不会执行的死代码
		&ast.ForStmt{
			Cond: &ast.BinaryExpr{
				X: &ast.BinaryExpr{
					X:  &ast.Ident{Name: junkVarName1},
					Op: token.LSS,
					Y:  &ast.BasicLit{Kind: token.INT, Value: "0"},
				},
				Op: token.LAND,
				Y: &ast.BinaryExpr{
					X:  &ast.Ident{Name: junkVarName2},
					Op: token.GTR,
					Y:  &ast.BasicLit{Kind: token.INT, Value: "1000000"},
				},
			},
			Body: &ast.BlockStmt{
				List: []ast.Stmt{
					&ast.BranchStmt{Tok: token.BREAK},
				},
			},
		},
	}

	return stmts
}

// injectJunkCodeToAST 向 AST 注入垃圾代码
func (o *Obfuscator) injectJunkCodeToAST(node ast.Node) {
	ast.Inspect(node, func(n ast.Node) bool {
		if fn, ok := n.(*ast.FuncDecl); ok {
			if o.shouldSkipJunkCodeInjection(fn) {
				return true
			}

			if fn.Body != nil && len(fn.Body.List) > 0 {
				hasReturn := fn.Type.Results != nil && len(fn.Type.Results.List) > 0
				junkStmts := o.generateJunkStatements(hasReturn)
				fn.Body.List = append(junkStmts, fn.Body.List...)
			}
		}
		return true
	})
}

