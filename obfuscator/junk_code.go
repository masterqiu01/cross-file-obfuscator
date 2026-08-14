package obfuscator

import (
	"fmt"
	"go/ast"
	"go/token"
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

// collectUsedNames 收集文件中所有已使用的标识符与标签名，用于保证垃圾代码随机名不冲突
func collectUsedNames(node *ast.File) map[string]bool {
	used := make(map[string]bool)
	ast.Inspect(node, func(n ast.Node) bool {
		switch x := n.(type) {
		case *ast.Ident:
			if x.Name != "" {
				used[x.Name] = true
			}
		case *ast.LabeledStmt:
			if x.Label != nil {
				used[x.Label.Name] = true
			}
		}
		return true
	})
	return used
}

// newJunkName 生成不与现有标识符冲突的随机垃圾代码变量名
func (o *Obfuscator) newJunkName(used map[string]bool, prefix string) string {
	for {
		name := prefix + generateRandomString(6)
		if !used[name] {
			used[name] = true
			return name
		}
	}
}

// junkAnchor 表示可在垃圾代码谓词中引用的运行时锚点（随机值、函数参数等）。
// 引用它们的谓词无法被编译器常量折叠，从而保证垃圾代码真正留存在二进制中。
type junkAnchor struct {
	name string
	kind junkAnchorKind
}

type junkAnchorKind int

const (
	anchorUnknown junkAnchorKind = iota
	anchorInt
	anchorString
)

// generateJunkStatementsWithAnchors 生成垃圾代码；anchors 提供运行时锚点，
// 有锚点时优先注入锚点谓词（不可折叠），其余为常规结构垃圾块。
func (o *Obfuscator) generateJunkStatementsWithAnchors(used map[string]bool, anchors []junkAnchor) []ast.Stmt {
	// 随机选择 2 到 4 个不同的垃圾代码块
	count := 2 + MascotRandInt(3)
	var allStmts []ast.Stmt

	// 功能池：不同的混淆模式
	generators := []func(map[string]bool) []ast.Stmt{
		o.genOpaquePredicateMath,
		o.genOpaquePredicateBitwise,
		o.genFakeLoop,
		o.genUnreachableSwitch,
		o.genOpaqueCondition,
		o.genStringOpaque,
		o.genLenOpaque,
		o.genOverflowOpaque,
		o.genNestedOpaque,
		o.genIfElseOpaque,
		o.genSliceOpaque,
		o.genGotoDeadCode,
	}

	// 打乱并选取
	MascotShuffle(len(generators), func(i, j int) {
		generators[i], generators[j] = generators[j], generators[i]
	})

	for i := 0; i < count; i++ {
		if anchors != nil && MascotRandInt(3) == 0 {
			// 有可用锚点时，用锚点谓词替换其中一个常规垃圾块，
			// 保证至少一部分垃圾代码依赖运行时值而无法被常量折叠。
			allStmts = append(allStmts, o.genAnchorOpaque(used, anchors)...)
			continue
		}
		allStmts = append(allStmts, generators[i](used)...)
	}

	return allStmts
}

// genAnchorOpaque 生成引用运行时锚点的不可折叠谓词垃圾块。
// 锚点（函数参数）取值在编译期不可知，因此这些分支不会被 SSA 常量折叠。
func (o *Obfuscator) genAnchorOpaque(used map[string]bool, anchors []junkAnchor) []ast.Stmt {
	if len(anchors) == 0 {
		return nil
	}
	var cand []junkAnchor
	for _, a := range anchors {
		if a.kind == anchorInt || a.kind == anchorString {
			cand = append(cand, a)
		}
	}
	if len(cand) == 0 {
		return nil
	}
	a := cand[MascotRandInt(len(cand))]

	if a.kind == anchorString {
		// s + "!" == s 永远为假（字符串不能能自相等且加长），依赖运行时值。
		return []ast.Stmt{
			&ast.IfStmt{
				Cond: &ast.BinaryExpr{
					X: &ast.BinaryExpr{
						X:  &ast.Ident{Name: a.name},
						Op: token.ADD,
						Y:  &ast.BasicLit{Kind: token.STRING, Value: `"!"`},
					},
					Op: token.EQL,
					Y:  &ast.Ident{Name: a.name},
				},
				Body: &ast.BlockStmt{
					List: []ast.Stmt{
						&ast.ExprStmt{X: &ast.CallExpr{
							Fun:  &ast.Ident{Name: "panic"},
							Args: []ast.Expr{&ast.BasicLit{Kind: token.STRING, Value: `"unreachable"`}},
						}},
					},
				},
			},
		}
	}

	// int 锚点：p+1 == p 永远为假（任何整数值都不成立），依赖运行时值。
	return []ast.Stmt{
		&ast.IfStmt{
			Cond: &ast.BinaryExpr{
				X: &ast.BinaryExpr{
					X:  &ast.Ident{Name: a.name},
					Op: token.ADD,
					Y:  &ast.BasicLit{Kind: token.INT, Value: "1"},
				},
				Op: token.EQL,
				Y:  &ast.Ident{Name: a.name},
			},
			Body: &ast.BlockStmt{
				List: []ast.Stmt{
					&ast.ExprStmt{X: &ast.CallExpr{
						Fun:  &ast.Ident{Name: "panic"},
						Args: []ast.Expr{&ast.BasicLit{Kind: token.STRING, Value: `"unreachable"`}},
					}},
				},
			},
		},
	}
}

// collectJunkAnchors 从函数声明收集可作垃圾谓词锚点的参数（int/string 类型）。
// 函数参数在注入点必然在作用域内，且其值编译期不可知。
func collectJunkAnchors(fn *ast.FuncDecl) []junkAnchor {
	if fn.Type == nil || fn.Type.Params == nil {
		return nil
	}
	var anchors []junkAnchor
	for _, field := range fn.Type.Params.List {
		if len(field.Names) == 0 {
			continue
		}
		kind := anchorUnknown
		switch t := field.Type.(type) {
		case *ast.Ident:
			switch t.Name {
			case "int", "int8", "int16", "int32", "int64",
				"uint", "uint8", "uint16", "uint32", "uint64", "uintptr",
				"byte", "rune":
				kind = anchorInt
			case "string":
				kind = anchorString
			}
		}
		if kind == anchorUnknown {
			continue
		}
		for _, name := range field.Names {
			anchors = append(anchors, junkAnchor{name: name.Name, kind: kind})
		}
	}
	return anchors
}

// genOpaquePredicateMath 生成基于数学恒等式的垃圾代码
func (o *Obfuscator) genOpaquePredicateMath(used map[string]bool) []ast.Stmt {
	v := o.newJunkName(used, "v")
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
					// panic 是不可达的
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
func (o *Obfuscator) genOpaquePredicateBitwise(used map[string]bool) []ast.Stmt {
	v := o.newJunkName(used, "b")
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
func (o *Obfuscator) genFakeLoop(used map[string]bool) []ast.Stmt {
	i := o.newJunkName(used, "i")
	s := o.newJunkName(used, "s")
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
func (o *Obfuscator) genUnreachableSwitch(used map[string]bool) []ast.Stmt {
	v := o.newJunkName(used, "sw")

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
func (o *Obfuscator) genOpaqueCondition(used map[string]bool) []ast.Stmt {
	v := o.newJunkName(used, "c")
	w := o.newJunkName(used, "c")

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

// genStringOpaque 生成基于字符串拼接的永不成立谓词
func (o *Obfuscator) genStringOpaque(used map[string]bool) []ast.Stmt {
	s := o.newJunkName(used, "str")
	base := "x" + generateRandomString(6)
	expect := base + "!"

	return []ast.Stmt{
		&ast.AssignStmt{
			Lhs: []ast.Expr{&ast.Ident{Name: s}},
			Tok: token.DEFINE,
			Rhs: []ast.Expr{&ast.BasicLit{Kind: token.STRING, Value: fmt.Sprintf("%q", base)}},
		},
		&ast.IfStmt{
			Cond: &ast.BinaryExpr{
				X: &ast.BinaryExpr{
					X:  &ast.Ident{Name: s},
					Op: token.ADD,
					Y:  &ast.BasicLit{Kind: token.STRING, Value: `"!"`},
				},
				Op: token.NEQ, // s + "!" != base + "!" 永远为假
				Y:  &ast.BasicLit{Kind: token.STRING, Value: fmt.Sprintf("%q", expect)},
			},
			Body: &ast.BlockStmt{
				List: []ast.Stmt{
					&ast.ExprStmt{X: &ast.CallExpr{
						Fun:  &ast.Ident{Name: "panic"},
						Args: []ast.Expr{&ast.BasicLit{Kind: token.STRING, Value: `"unreachable"`}},
					}},
				},
			},
		},
	}
}

// genLenOpaque 生成基于 len 的永不成立谓词
func (o *Obfuscator) genLenOpaque(used map[string]bool) []ast.Stmt {
	n := o.newJunkName(used, "n")
	content := generateRandomString(8)

	return []ast.Stmt{
		&ast.AssignStmt{
			Lhs: []ast.Expr{&ast.Ident{Name: n}},
			Tok: token.DEFINE,
			Rhs: []ast.Expr{&ast.CallExpr{
				Fun:  &ast.Ident{Name: "len"},
				Args: []ast.Expr{&ast.BasicLit{Kind: token.STRING, Value: fmt.Sprintf("%q", content)}},
			}},
		},
		&ast.IfStmt{
			Cond: &ast.BinaryExpr{
				X:  &ast.Ident{Name: n},
				Op: token.EQL, // len(...) == 99999 永远为假
				Y:  &ast.BasicLit{Kind: token.INT, Value: "99999"},
			},
			Body: &ast.BlockStmt{
				List: []ast.Stmt{
					&ast.ExprStmt{X: &ast.CallExpr{
						Fun:  &ast.Ident{Name: "panic"},
						Args: []ast.Expr{&ast.BasicLit{Kind: token.STRING, Value: `"unreachable"`}},
					}},
				},
			},
		},
	}
}

// genOverflowOpaque 生成基于整数溢出的永不成立谓词（MaxInt64+1 回绕为负数）
func (o *Obfuscator) genOverflowOpaque(used map[string]bool) []ast.Stmt {
	m := o.newJunkName(used, "m")

	return []ast.Stmt{
		&ast.AssignStmt{
			Lhs: []ast.Expr{&ast.Ident{Name: m}},
			Tok: token.DEFINE,
			Rhs: []ast.Expr{&ast.CallExpr{
				Fun:  &ast.Ident{Name: "int64"},
				Args: []ast.Expr{&ast.BasicLit{Kind: token.INT, Value: "9223372036854775807"}},
			}},
		},
		&ast.IfStmt{
			Cond: &ast.BinaryExpr{
				X: &ast.BinaryExpr{
					X:  &ast.Ident{Name: m},
					Op: token.ADD,
					Y:  &ast.BasicLit{Kind: token.INT, Value: "1"},
				},
				Op: token.GTR, // m+1 溢出回绕后不可能大于 m，永远为假
				Y:  &ast.Ident{Name: m},
			},
			Body: &ast.BlockStmt{
				List: []ast.Stmt{
					&ast.ExprStmt{X: &ast.CallExpr{
						Fun:  &ast.Ident{Name: "panic"},
						Args: []ast.Expr{&ast.BasicLit{Kind: token.STRING, Value: `"unreachable"`}},
					}},
				},
			},
		},
	}
}

// genNestedOpaque 生成嵌套多层的不透明谓词
func (o *Obfuscator) genNestedOpaque(used map[string]bool) []ast.Stmt {
	k := o.newJunkName(used, "k")

	return []ast.Stmt{
		&ast.AssignStmt{
			Lhs: []ast.Expr{&ast.Ident{Name: k}},
			Tok: token.DEFINE,
			Rhs: []ast.Expr{&ast.BasicLit{Kind: token.INT, Value: "3"}},
		},
		&ast.IfStmt{
			Cond: &ast.BinaryExpr{
				X: &ast.BinaryExpr{
					X:  &ast.Ident{Name: k},
					Op: token.MUL,
					Y:  &ast.Ident{Name: k},
				},
				Op: token.EQL, // k*k == 9 永远为真
				Y:  &ast.BasicLit{Kind: token.INT, Value: "9"},
			},
			Body: &ast.BlockStmt{
				List: []ast.Stmt{
					&ast.IfStmt{
						Cond: &ast.BinaryExpr{
							X:  &ast.Ident{Name: k},
							Op: token.NEQ, // k != k 永远为假
							Y:  &ast.Ident{Name: k},
						},
						Body: &ast.BlockStmt{
							List: []ast.Stmt{
								&ast.ExprStmt{X: &ast.CallExpr{
									Fun:  &ast.Ident{Name: "panic"},
									Args: []ast.Expr{&ast.BasicLit{Kind: token.STRING, Value: `"unreachable"`}},
								}},
							},
						},
					},
				},
			},
		},
	}
}

// genIfElseOpaque 生成 if/else 双向死分支（一个恒真、一个恒假）
func (o *Obfuscator) genIfElseOpaque(used map[string]bool) []ast.Stmt {
	t := o.newJunkName(used, "t")
	u := o.newJunkName(used, "u")

	return []ast.Stmt{
		&ast.AssignStmt{
			Lhs: []ast.Expr{&ast.Ident{Name: t}},
			Tok: token.DEFINE,
			Rhs: []ast.Expr{&ast.BasicLit{Kind: token.INT, Value: "5"}},
		},
		&ast.IfStmt{
			Cond: &ast.BinaryExpr{
				X: &ast.BinaryExpr{
					X:  &ast.Ident{Name: t},
					Op: token.MUL,
					Y:  &ast.BasicLit{Kind: token.INT, Value: "2"},
				},
				Op: token.EQL, // t*2 == 10 永远为真
				Y:  &ast.BasicLit{Kind: token.INT, Value: "10"},
			},
			Body: &ast.BlockStmt{
				List: []ast.Stmt{
					&ast.AssignStmt{
						Lhs: []ast.Expr{&ast.Ident{Name: u}},
						Tok: token.DEFINE,
						Rhs: []ast.Expr{&ast.BinaryExpr{
							X:  &ast.Ident{Name: t},
							Op: token.ADD,
							Y:  &ast.BasicLit{Kind: token.INT, Value: "1"},
						}},
					},
					&ast.AssignStmt{
						Lhs: []ast.Expr{&ast.Ident{Name: "_"}},
						Tok: token.ASSIGN,
						Rhs: []ast.Expr{&ast.Ident{Name: u}},
					},
				},
			},
			Else: &ast.BlockStmt{
				List: []ast.Stmt{
					&ast.ExprStmt{X: &ast.CallExpr{
						Fun:  &ast.Ident{Name: "panic"},
						Args: []ast.Expr{&ast.BasicLit{Kind: token.STRING, Value: `"unreachable"`}},
					}},
				},
			},
		},
	}
}

// genSliceOpaque 生成基于切片 len 的永不成立谓词
func (o *Obfuscator) genSliceOpaque(used map[string]bool) []ast.Stmt {
	sl := o.newJunkName(used, "sl")

	return []ast.Stmt{
		&ast.AssignStmt{
			Lhs: []ast.Expr{&ast.Ident{Name: sl}},
			Tok: token.DEFINE,
			Rhs: []ast.Expr{&ast.CompositeLit{
				Type: &ast.ArrayType{
					Elt: &ast.Ident{Name: "int"},
				},
				Elts: []ast.Expr{
					&ast.BasicLit{Kind: token.INT, Value: "1"},
					&ast.BasicLit{Kind: token.INT, Value: "2"},
					&ast.BasicLit{Kind: token.INT, Value: "3"},
				},
			}},
		},
		&ast.IfStmt{
			Cond: &ast.BinaryExpr{
				X: &ast.CallExpr{
					Fun:  &ast.Ident{Name: "len"},
					Args: []ast.Expr{&ast.Ident{Name: sl}},
				},
				Op: token.GTR, // len(sl) > 10 永远为假
				Y:  &ast.BasicLit{Kind: token.INT, Value: "10"},
			},
			Body: &ast.BlockStmt{
				List: []ast.Stmt{
					&ast.ExprStmt{X: &ast.CallExpr{
						Fun:  &ast.Ident{Name: "panic"},
						Args: []ast.Expr{&ast.BasicLit{Kind: token.STRING, Value: `"unreachable"`}},
					}},
				},
			},
		},
	}
}

// genGotoDeadCode 生成 goto 跳跃到标签、中间夹带死代码的混淆
func (o *Obfuscator) genGotoDeadCode(used map[string]bool) []ast.Stmt {
	label := o.newJunkName(used, "jnk")
	dead := o.newJunkName(used, "d")

	return []ast.Stmt{
		&ast.BranchStmt{
			Tok:   token.GOTO,
			Label: &ast.Ident{Name: label},
		},
		// 死代码块：goto 越过它，永远不会执行
		&ast.BlockStmt{
			List: []ast.Stmt{
				&ast.AssignStmt{
					Lhs: []ast.Expr{&ast.Ident{Name: dead}},
					Tok: token.DEFINE,
					Rhs: []ast.Expr{&ast.BasicLit{Kind: token.STRING, Value: `"dead"`}},
				},
				&ast.AssignStmt{
					Lhs: []ast.Expr{&ast.Ident{Name: "_"}},
					Tok: token.ASSIGN,
					Rhs: []ast.Expr{&ast.Ident{Name: dead}},
				},
			},
		},
		&ast.LabeledStmt{
			Label: &ast.Ident{Name: label},
			Stmt:  &ast.EmptyStmt{},
		},
	}
}

// 以下是用于生成随机性的辅助函数
func MascotRandInt(n int) int {
	if n <= 0 {
		return 0
	}
	return rng.IntN(n)
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
