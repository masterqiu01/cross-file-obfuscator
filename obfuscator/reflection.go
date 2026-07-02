package obfuscator

import (
	"go/ast"
	"strings"
)

// detectReflectionUsage 检查文件是否使用反射
func (o *Obfuscator) detectReflectionUsage(node *ast.File) bool {
	usesReflection := false

	for _, imp := range node.Imports {
		if imp.Path != nil && strings.Contains(imp.Path.Value, "reflect") {
			usesReflection = true
			break
		}
	}

	return usesReflection
}

// detectJSONUsage 检查文件是否使用 JSON 编码/解码
func (o *Obfuscator) detectJSONUsage(node *ast.File) bool {
	usesJSON := false

	for _, imp := range node.Imports {
		if imp.Path != nil {
			path := strings.Trim(imp.Path.Value, "\"")
			if strings.Contains(path, "encoding/json") ||
				strings.Contains(path, "encoding/xml") ||
				strings.Contains(path, "gopkg.in/yaml") {
				usesJSON = true
				break
			}
		}
	}

	return usesJSON
}

// protectReflectionTypes 将反射上下文中使用的类型、字段和方法标记为受保护
func (o *Obfuscator) protectReflectionTypes(node *ast.File) {
	if !o.Config.PreserveReflection {
		return
	}

	usesReflection := o.detectReflectionUsage(node)
	usesJSON := o.detectJSONUsage(node)

	if !usesReflection && !usesJSON {
		return
	}

	ast.Inspect(node, func(n ast.Node) bool {
		switch x := n.(type) {
		case *ast.TypeSpec:
			if usesReflection {
				o.protectedNames[x.Name.Name] = true
			}

			if structType, ok := x.Type.(*ast.StructType); ok {
				if structType.Fields != nil {
					for _, field := range structType.Fields.List {
						for _, fieldName := range field.Names {
							if usesReflection {
								o.protectedNames[fieldName.Name] = true
							} else if usesJSON {
								hasJSONTag := false
								if field.Tag != nil {
									tagValue := field.Tag.Value
									if strings.Contains(tagValue, "json:") {
										hasJSONTag = true
									}
								}
								if !hasJSONTag {
									o.protectedNames[fieldName.Name] = true
								}
							}
						}
					}
				}
			}
		case *ast.FuncDecl:
			if x.Recv != nil && usesReflection {
				o.protectedNames[x.Name.Name] = true
			}
		}
		return true
	})

	if usesReflection {
		o.reflectionPackages[node.Name.Name] = true
	}
}

