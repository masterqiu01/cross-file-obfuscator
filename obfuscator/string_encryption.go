package obfuscator

import (
	"encoding/base64"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
)

// encryptString 使用 XOR 加密字符串
func (o *Obfuscator) encryptString(text string) string {
	key := o.encryptionKey
	textBytes := []byte(text)
	encryptedBytes := make([]byte, len(textBytes))

	for i, b := range textBytes {
		keyByte := key[i%len(key)]
		encryptedBytes[i] = b ^ keyByte
	}

	return base64.StdEncoding.EncodeToString(encryptedBytes)
}

// generateDecryptFunction 生成解密函数源代码
func (o *Obfuscator) generateDecryptFunction(base64Alias string) string {
	keyBytes := []byte(o.encryptionKey)
	keyLiteral := "[]byte{"
	for i, b := range keyBytes {
		if i > 0 {
			keyLiteral += ", "
		}
		keyLiteral += fmt.Sprintf("%d", b)
	}
	keyLiteral += "}"

	funcBody := fmt.Sprintf("func %s(encrypted string) string {\n", o.decryptFuncName)
	funcBody += fmt.Sprintf("\tdata, err := %s.StdEncoding.DecodeString(encrypted)\n", base64Alias)
	funcBody += "\tif err != nil {\n"
	funcBody += "\t\treturn \"\"\n"
	funcBody += "\t}\n"
	funcBody += fmt.Sprintf("\tkey := %s\n", keyLiteral)
	funcBody += "\tresult := make([]byte, len(data))\n"
	funcBody += "\tfor i, b := range data {\n"
	funcBody += "\t\tresult[i] = b ^ key[i%len(key)]\n"
	funcBody += "\t}\n"
	funcBody += "\treturn string(result)\n"
	funcBody += "}\n"
	return funcBody
}

// addDecryptFunction 添加解密函数到 AST
func (o *Obfuscator) addDecryptFunction(node *ast.File) {
	// 获取 base64 别名
	base64Alias := "base64"
	for _, imp := range node.Imports {
		if imp.Path != nil && imp.Path.Value == `"encoding/base64"` {
			if imp.Name != nil {
				base64Alias = imp.Name.Name
			}
			break
		}
	}
	
	// 生成解密函数代码
	funcCode := o.generateDecryptFunction(base64Alias)
	
	// 解析函数代码为 AST
	fset := token.NewFileSet()
	funcNode, err := parser.ParseFile(fset, "", "package temp\n"+funcCode, 0)
	if err != nil {
		return
	}
	
	// 提取函数声明并添加到文件
	if len(funcNode.Decls) > 0 {
		if funcDecl, ok := funcNode.Decls[0].(*ast.FuncDecl); ok {
			node.Decls = append(node.Decls, funcDecl)
		}
	}
}

// ensureBase64Import 确保 AST 中存在 base64 导入
func (o *Obfuscator) ensureBase64Import(node *ast.File) {
	hasBase64 := false
	for _, imp := range node.Imports {
		if imp.Path != nil && imp.Path.Value == `"encoding/base64"` {
			hasBase64 = true
			break
		}
	}

	if hasBase64 {
		return
	}

	base64Import := &ast.ImportSpec{
		Path: &ast.BasicLit{
			Kind:  token.STRING,
			Value: `"encoding/base64"`,
		},
	}

	var importDecl *ast.GenDecl
	for _, decl := range node.Decls {
		if genDecl, ok := decl.(*ast.GenDecl); ok && genDecl.Tok == token.IMPORT {
			importDecl = genDecl
			break
		}
	}

	if importDecl != nil {
		importDecl.Specs = append(importDecl.Specs, base64Import)
	} else {
		importDecl = &ast.GenDecl{
			Tok:   token.IMPORT,
			Specs: []ast.Spec{base64Import},
		}
		node.Decls = append([]ast.Decl{importDecl}, node.Decls...)
	}

	node.Imports = append(node.Imports, base64Import)
}
