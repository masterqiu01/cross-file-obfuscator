package obfuscator

import (
	"go/ast"
	"go/parser"
	"go/scanner"
	"go/token"
	"strings"
)

// obfuscatePositions 用 //line 指令把源码中的函数调用位置换成随机伪文件名，
// 破坏栈追踪与调试器暴露的真实源码位置（参考 garble 的 position.go）。
// 仅处理可由 go/parser 正常解析的源码；解析失败时原样返回，保证安全。
func (o *Obfuscator) obfuscatePositions(source string) string {
	if !o.Config.ObfuscatePositions {
		return source
	}

	fset := token.NewFileSet()
	node, err := parser.ParseFile(fset, "", source, parser.ParseComments)
	if err != nil {
		// 源码经过混淆/加密后个别文件无法解析属正常情况，跳过位置混淆
		return source
	}

	// 记录所有函数调用点（CallExpr 的 Fun 标识符位置）。
	// 我们记录 Fun 表达式最左标识符的起点偏移，token 化后据此插入 //line 指令。
	callOffsets := make(map[token.Pos]bool)
	ast.Inspect(node, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		callOffsets[call.Fun.Pos()] = true
		return true
	})

	// 用随机伪文件名替换真实文件基名（与文件名混淆相互独立）
	pseudoName := generateRandomString(12) + ".go"

	var sb strings.Builder
	copied := 0

	file := fset.File(node.Pos())
	var s scanner.Scanner
	s.Init(file, []byte(source), nil, 0)

	for {
		pos, tok, _ := s.Scan()
		if tok == token.EOF {
			sb.WriteString(source[copied:])
			break
		}
		if tok == token.COMMENT {
			// 保留所有注释（注释本身不产生位置信息，扫描跳过即可）
			continue
		}
		if callOffsets[pos] {
			offset := file.Offset(pos)
			sb.WriteString(source[copied:offset])
			copied = offset
			// 插入 //line 指令：格式为 `/*line NAME.go:1*/`，要求两侧有空白。
			sb.WriteString(" /*line ")
			sb.WriteString(pseudoName)
			sb.WriteString(":1*/ ")
		}
	}
	return sb.String()
}
