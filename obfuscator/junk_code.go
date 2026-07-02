package obfuscator

import (
	"crypto/rand"
	"fmt"
	"go/ast"
	"go/token"
	"math/big"
	"strings"
)

// shouldSkipJunkCodeInjection 确定是否不应向函数注入垃圾代码
func (o *Obfuscator) shouldSkipJunkCodeInjection(fn *ast.FuncDecl) bool {
	// 跳过 init 函数：它有固定调用顺序，干扰可能导致符号丢失
	if fn.Name.Name == "init" {
		return true
	}

	if fn.Name.Name == "main" && fn.Recv == nil {
		return true
	}

	// 跳过带编译器指令的函数
	if fn.Doc != nil {
		for _, comment := range fn.Doc.List {
			if strings.HasPrefix(comment.Text, "//go:") ||
				strings.HasPrefix(comment.Text, "//export ") {
				return true
			}
		}
	}

	// 跳过过于简短的函数体（避免干扰逻辑）
	if fn.Body != nil && len(fn.Body.List) <= 2 {
		return true
	}

	return false
}

// generateJunkStatements 生成更加多样化和随机的垃圾代码语句
func (o *Obfuscator) generateJunkStatements() []ast.Stmt {
	// 随机选择 2 到 4 个不同的垃圾代码块
	count := 2 + MascotRandInt(3)
	var allStmts []ast.Stmt

	// 功能池：不同的混淆模式
	generators := []func() []ast.Stmt{
		o.genOpaquePredicateMath,
		o.genOpaquePredicateBitwise,
		o.genFakeLoop,
		o.genUnreachableSwitch,
		o.genOpaqueCondition,
	}

	// 打乱并选取
	MascotShuffle(len(generators), func(i, j int) {
		generators[i], generators[j] = generators[j], generators[i]
	})

	for i := 0; i < count; i++ {
		allStmts = append(allStmts, generators[i%len(generators)]()...)
	}

	return allStmts
}

// genOpaquePredicateMath 生成基于数学恒等式的垃圾代码
func (o *Obfuscator) genOpaquePredicateMath() []ast.Stmt {
	v := fmt.Sprintf("v%s", generateRandomString(6))
	val := MascotRandInt(100) + 1

	// (x + a)(x - a) == x^2 - a^2
	a := MascotRandInt(10) + 1
	a2 := a * a

	return []ast.Stmt{
		&ast.AssignStmt{
			Lhs: []ast.Expr{&ast.Ident{Name: v}},
			Tok: token.DEFINE,
			Rhs: []ast.Expr{&ast.BasicLit{Kind: token.INT, Value: fmt.Sprintf("%d", val)}},
		},
		&ast.IfStmt{
			Cond: &ast.BinaryExpr{
				X: &ast.BinaryExpr{
					X: &ast.BinaryExpr{
						X:  &ast.Ident{Name: v},
						Op: token.ADD,
						Y:  &ast.BasicLit{Kind: token.INT, Value: fmt.Sprintf("%d", a)},
					},
					Op: token.MUL,
					Y: &ast.BinaryExpr{
						X:  &ast.Ident{Name: v},
						Op: token.SUB,
						Y:  &ast.BasicLit{Kind: token.INT, Value: fmt.Sprintf("%d", a)},
					},
				},
				Op: token.NEQ, // (x+a)(x-a) != x^2 - a^2 永远为假
				Y: &ast.BinaryExpr{
					X: &ast.BinaryExpr{
						X:  &ast.Ident{Name: v},
						Op: token.MUL,
						Y:  &ast.Ident{Name: v},
					},
					Op: token.SUB,
					Y:  &ast.BasicLit{Kind: token.INT, Value: fmt.Sprintf("%d", a2)},
				},
			},
			Body: &ast.BlockStmt{
				List: []ast.Stmt{
					// panic 是不可达的，字符串值不含多余引号
					&ast.ExprStmt{X: &ast.CallExpr{
						Fun:  &ast.Ident{Name: "panic"},
						Args: []ast.Expr{&ast.BasicLit{Kind: token.STRING, Value: `"unreachable"`}},
					}},
				},
			},
		},
	}
}

// genOpaquePredicateBitwise 生成基于位运算的垃圾代码
func (o *Obfuscator) genOpaquePredicateBitwise() []ast.Stmt {
	v := fmt.Sprintf("b%s", generateRandomString(6))
	val := MascotRandInt(1000)

	return []ast.Stmt{
		&ast.AssignStmt{
			Lhs: []ast.Expr{&ast.Ident{Name: v}},
			Tok: token.DEFINE,
			Rhs: []ast.Expr{&ast.BasicLit{Kind: token.INT, Value: fmt.Sprintf("%d", val)}},
		},
		&ast.IfStmt{
			Cond: &ast.BinaryExpr{
				X: &ast.BinaryExpr{
					X:  &ast.Ident{Name: v},
					Op: token.XOR,
					Y:  &ast.Ident{Name: v},
				},
				Op: token.GTR, // x ^ x > 0 永远为假
				Y:  &ast.BasicLit{Kind: token.INT, Value: "0"},
			},
			Body: &ast.BlockStmt{
				List: []ast.Stmt{
					&ast.AssignStmt{
						Lhs: []ast.Expr{&ast.Ident{Name: v}},
						Tok: token.ASSIGN,
						Rhs: []ast.Expr{&ast.BasicLit{Kind: token.INT, Value: "1"}},
					},
				},
			},
		},
		// 确保 v 在作用域内被使用，避免编译报「declared and not used」
		&ast.AssignStmt{
			Lhs: []ast.Expr{&ast.Ident{Name: "_"}},
			Tok: token.ASSIGN,
			Rhs: []ast.Expr{&ast.Ident{Name: v}},
		},
	}
}

// genFakeLoop 生成一个看起来复杂但执行次数固定的循环
func (o *Obfuscator) genFakeLoop() []ast.Stmt {
	i := fmt.Sprintf("i%s", generateRandomString(4))
	s := fmt.Sprintf("s%s", generateRandomString(4))
	limit := 5 + MascotRandInt(10)
	
	return []ast.Stmt{
		&ast.AssignStmt{
			Lhs: []ast.Expr{&ast.Ident{Name: s}},
			Tok: token.DEFINE,
			Rhs: []ast.Expr{&ast.BasicLit{Kind: token.INT, Value: "0"}},
		},
		&ast.ForStmt{
			Init: &ast.AssignStmt{
				Lhs: []ast.Expr{&ast.Ident{Name: i}},
				Tok: token.DEFINE,
				Rhs: []ast.Expr{&ast.BasicLit{Kind: token.INT, Value: "0"}},
			},
			Cond: &ast.BinaryExpr{
				X:  &ast.Ident{Name: i},
				Op: token.LSS,
				Y:  &ast.BasicLit{Kind: token.INT, Value: fmt.Sprintf("%d", limit)},
			},
			Post: &ast.IncDecStmt{
				X:   &ast.Ident{Name: i},
				Tok: token.INC,
			},
			Body: &ast.BlockStmt{
				List: []ast.Stmt{
					&ast.AssignStmt{
						Lhs: []ast.Expr{&ast.Ident{Name: s}},
						Tok: token.ASSIGN,
						Rhs: []ast.Expr{&ast.BinaryExpr{X: &ast.Ident{Name: s}, Op: token.ADD, Y: &ast.Ident{Name: i}}},
					},
				},
			},
		},
	}
}

// genUnreachableSwitch 生成不可达的 switch 分支
func (o *Obfuscator) genUnreachableSwitch() []ast.Stmt {
	v := fmt.Sprintf("sw%s", generateRandomString(5))

	// switch v { case 99: ... } 当 v == 1 时， case 99 永达不到
	return []ast.Stmt{
		&ast.AssignStmt{
			Lhs: []ast.Expr{&ast.Ident{Name: v}},
			Tok: token.DEFINE,
			Rhs: []ast.Expr{&ast.BasicLit{Kind: token.INT, Value: "1"}},
		},
		&ast.SwitchStmt{
			Tag: &ast.Ident{Name: v},
			Body: &ast.BlockStmt{
				List: []ast.Stmt{
					&ast.CaseClause{
						List: []ast.Expr{
							&ast.BasicLit{Kind: token.INT, Value: "99"},
						},
						Body: []ast.Stmt{
							&ast.AssignStmt{
								Lhs: []ast.Expr{&ast.Ident{Name: v}},
								Tok: token.ASSIGN,
								Rhs: []ast.Expr{&ast.BasicLit{Kind: token.INT, Value: "0"}},
							},
						},
					},
				},
			},
		},
		// 使用 _ = v 避免未使用变量编译错误
		&ast.AssignStmt{
			Lhs: []ast.Expr{&ast.Ident{Name: "_"}},
			Tok: token.ASSIGN,
			Rhs: []ast.Expr{&ast.Ident{Name: v}},
		},
	}
}

// genOpaqueCondition 生成其他不透明谓词
func (o *Obfuscator) genOpaqueCondition() []ast.Stmt {
	v := fmt.Sprintf("c%s", generateRandomString(6))
	w := fmt.Sprintf("c%s", generateRandomString(6))

	// v*v >= 0 永远为真；内层 if v != v 永远为假
	return []ast.Stmt{
		&ast.AssignStmt{
			Lhs: []ast.Expr{&ast.Ident{Name: v}},
			Tok: token.DEFINE,
			Rhs: []ast.Expr{&ast.BasicLit{Kind: token.INT, Value: "7"}},
		},
		&ast.AssignStmt{
			Lhs: []ast.Expr{&ast.Ident{Name: w}},
			Tok: token.DEFINE,
			Rhs: []ast.Expr{&ast.BinaryExpr{
				X:  &ast.Ident{Name: v},
				Op: token.MUL,
				Y:  &ast.Ident{Name: v},
			}},
		},
		&ast.IfStmt{
			Cond: &ast.BinaryExpr{
				X:  &ast.Ident{Name: w},
				Op: token.LSS, // v*v < 0 永远为假
				Y:  &ast.BasicLit{Kind: token.INT, Value: "0"},
			},
			Body: &ast.BlockStmt{
				List: []ast.Stmt{
					&ast.AssignStmt{
						Lhs: []ast.Expr{&ast.Ident{Name: "_"}},
						Tok: token.ASSIGN,
						Rhs: []ast.Expr{&ast.Ident{Name: w}},
					},
				},
			},
		},
	}
}

// 以下是用于生成随机性的辅助函数
func MascotRandInt(n int) int {
	if n <= 0 {
		return 0
	}
	res, err := rand.Int(rand.Reader, big.NewInt(int64(n)))
	if err != nil {
		return 0
	}
	return int(res.Int64())
}

func MascotShuffle(n int, swap func(i, j int)) {
	if n <= 0 {
		return
	}
	for i := n - 1; i > 0; i-- {
		j := MascotRandInt(i + 1)
		swap(i, j)
	}
}

// injectJunkCodeToAST 向 AST 注入垃圾代码
func (o *Obfuscator) injectJunkCodeToAST(node ast.Node) {
	ast.Inspect(node, func(n ast.Node) bool {
		if fn, ok := n.(*ast.FuncDecl); ok {
			if o.shouldSkipJunkCodeInjection(fn) {
				return true
			}

			if fn.Body != nil && len(fn.Body.List) > 0 {
				junkStmts := o.generateJunkStatements()
				fn.Body.List = append(junkStmts, fn.Body.List...)
			}
		}
		return true
	})
}

