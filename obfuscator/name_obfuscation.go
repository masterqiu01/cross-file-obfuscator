package obfuscator

import (
	"fmt"
	"strings"
)

// obfuscateName 创建一个混淆后的名称（不暴露原始名称）
func (o *Obfuscator) obfuscateName(name string, isFunc bool) string {
	mapping := o.varMapping
	prefix := "l" // 局部变量
	if isFunc {
		mapping = o.funcMapping
		prefix = "fn" // 函数
	}

	if obf, exists := mapping[name]; exists {
		return obf
	}

	// 检查是否为导出名称（首字母大写）
	isExported := len(name) > 0 && name[0] >= 'A' && name[0] <= 'Z'

	// 生成唯一的混淆名称，不暴露原始名称
	// 使用循环确保在所有映射中的唯一性
	maxAttempts := 100
	for attempt := 0; attempt < maxAttempts; attempt++ {
		randomPart := generateRandomString(12)
		var obf string
		
		if isExported {
			// 导出名称：首字母大写
			if isFunc {
				obf = fmt.Sprintf("Fn%s", randomPart)
			} else {
				obf = fmt.Sprintf("V%s", randomPart)
			}
		} else {
			// 私有名称：首字母小写
			obf = fmt.Sprintf("%s%s", prefix, randomPart)
		}

		// 检查此名称是否已在任何映射中使用
		if !o.isNameUsed(obf) {
			mapping[name] = obf
			return obf
		}
	}

	// 回退：使用基于计数器的方法
	o.namingCounter++
	var obf string
	if isExported {
		if isFunc {
			obf = fmt.Sprintf("Fn%d_%s", o.namingCounter, generateRandomString(8))
		} else {
			obf = fmt.Sprintf("V%d_%s", o.namingCounter, generateRandomString(8))
		}
	} else {
		obf = fmt.Sprintf("%s%d_%s", prefix, o.namingCounter, generateRandomString(8))
	}
	mapping[name] = obf
	return obf
}

// isNameUsed 检查名称是否已在任何映射中使用
func (o *Obfuscator) isNameUsed(name string) bool {
	// 检查变量映射
	for _, obfName := range o.varMapping {
		if obfName == name {
			return true
		}
	}

	// 检查函数映射
	for _, obfName := range o.funcMapping {
		if obfName == name {
			return true
		}
	}

	// 检查导入别名映射
	for _, obfName := range o.importAliasMapping {
		if obfName == name {
			return true
		}
	}

	return false
}

// shouldProtect 检查名称是否应受保护而不被混淆
func (o *Obfuscator) shouldProtect(name string) bool {
	// 保护特殊名称
	if name == "_" || name == "main" || name == "init" {
		return true
	}
	// 如果 obfuscateExported 为 false，保护所有导出的名称
	if !o.Config.ObfuscateExported && isExported(name) {
		return true
	}
	// 保护受保护列表中的名称（字段、方法、选择器）
	if o.protectedNames[name] {
		return true
	}
	// 保护包名称（来自导入）
	if o.packageNames[name] {
		return true
	}
	// 保护可能导致问题的常见 Go 标识符
	protectedIdentifiers := map[string]bool{
		"error":      true, // 内置错误接口
		"string":     true, // 内置类型
		"int":        true,
		"bool":       true,
		"byte":       true,
		"rune":       true,
		"float32":    true,
		"float64":    true,
		"complex64":  true,
		"complex128": true,
		"uint":       true,
		"int8":       true,
		"int16":      true,
		"int32":      true,
		"int64":      true,
		"uint8":      true,
		"uint16":     true,
		"uint32":     true,
		"uint64":     true,
		"uintptr":    true,
		"len":        true, // 内置函数
		"cap":        true,
		"make":       true,
		"new":        true,
		"append":     true,
		"copy":       true,
		"delete":     true,
		"panic":      true,
		"recover":    true,
		"print":      true,
		"println":    true,
		"close":      true,
		"min":        true, // Go 1.21+ 内置函数
		"max":        true,
		"clear":      true,
		"nil":        true, // 内置常量
		"true":       true,
		"false":      true,
		"iota":       true,
	}
	if protectedIdentifiers[name] {
		return true
	}
	return false
}

// obfuscateFileName 混淆 Go 文件名（不暴露原始名称）
func (o *Obfuscator) obfuscateFileName(fileName string) string {
	// 不混淆特殊文件
	if fileName == "main.go" || fileName == "go.mod" || fileName == "go.sum" {
		return fileName
	}
	
	// 不混淆带平台后缀的 main 文件（如 main_windows.go, main_linux.go 等）
	if strings.HasPrefix(fileName, "main_") && strings.HasSuffix(fileName, ".go") {
		return fileName
	}

	// 检查是否已有此文件名的映射
	if obfuscated, exists := o.fileNameMapping[fileName]; exists {
		return obfuscated
	}

	// ✅ 提取平台特定后缀（如 _linux, _windows, _darwin 等）
	// 这些后缀用于build标签，必须保留以避免同名函数冲突
	platformSuffixes := []string{
		"_linux", "_windows", "_darwin", "_freebsd", "_openbsd", "_netbsd",
		"_dragonfly", "_solaris", "_plan9", "_aix", "_android", "_ios",
		"_js", "_wasm", "_unix", "_other",
		"_amd64", "_386", "_arm", "_arm64", "_ppc64", "_ppc64le",
		"_mips", "_mipsle", "_mips64", "_mips64le", "_s390x", "_riscv64",
	}
	
	suffix := ""
	if strings.HasSuffix(fileName, ".go") {
		nameWithoutExt := strings.TrimSuffix(fileName, ".go")
		for _, ps := range platformSuffixes {
			if strings.HasSuffix(nameWithoutExt, ps) {
				suffix = ps
				break
			}
		}
	}

	// 生成随机文件名
	maxAttempts := 100
	for attempt := 0; attempt < maxAttempts; attempt++ {
		randomPart := generateRandomString(10)
		var obfuscatedName string
		if suffix != "" {
			// 保留平台后缀
			obfuscatedName = fmt.Sprintf("f%s%s.go", randomPart, suffix)
		} else {
			obfuscatedName = fmt.Sprintf("f%s.go", randomPart)
		}

		// 检查此名称是否已被使用
		nameUsed := false
		for _, existingName := range o.fileNameMapping {
			if existingName == obfuscatedName {
				nameUsed = true
				break
			}
		}

		if !nameUsed {
			// 存储映射并返回
			o.fileNameMapping[fileName] = obfuscatedName
			return obfuscatedName
		}
	}

	// 回退：如果随机生成失败，使用基于计数器的方法
	var obfuscatedName string
	if suffix != "" {
		obfuscatedName = fmt.Sprintf("f%d_%s%s.go", len(o.fileNameMapping), generateRandomString(6), suffix)
	} else {
		obfuscatedName = fmt.Sprintf("f%d_%s.go", len(o.fileNameMapping), generateRandomString(6))
	}
	o.fileNameMapping[fileName] = obfuscatedName
	return obfuscatedName
}

// generateUniqueObfuscatedNameForObject 为对象生成全局唯一的混淆名称
// 这是修复导出函数混淆冲突的核心函数
func (o *Obfuscator) generateUniqueObfuscatedNameForObject(obj *Object) string {
	// 检查是否已经有混淆名
	if existingName, exists := o.objectMapping[obj]; exists && existingName != "" {
		return existingName
	}

	// 检查是否为导出名称
	isExported := len(obj.Name) > 0 && obj.Name[0] >= 'A' && obj.Name[0] <= 'Z'

	// 尝试生成唯一的混淆名称
	maxAttempts := 100
	for attempt := 0; attempt < maxAttempts; attempt++ {
		randomPart := generateRandomString(12)
		var obfName string
		
		if obj.Kind == ObjFunc {
			if isExported {
				obfName = fmt.Sprintf("Fn%s", randomPart)
			} else {
				obfName = fmt.Sprintf("fn%s", randomPart)
			}
		} else { // ObjVar or ObjConst
			if isExported {
				obfName = fmt.Sprintf("V%s", randomPart)
			} else {
				obfName = fmt.Sprintf("l%s", randomPart)
			}
		}

		// 检查此名称是否在整个项目中已被使用
		if !o.isObfuscatedNameUsedInProject(obfName) {
			return obfName
		}
	}

	// 回退：使用基于计数器的方法
	o.namingCounter++
	var obfName string
	if obj.Kind == ObjFunc {
		if isExported {
			obfName = fmt.Sprintf("Fn%d_%s", o.namingCounter, generateRandomString(8))
		} else {
			obfName = fmt.Sprintf("fn%d_%s", o.namingCounter, generateRandomString(8))
		}
	} else {
		if isExported {
			obfName = fmt.Sprintf("V%d_%s", o.namingCounter, generateRandomString(8))
		} else {
			obfName = fmt.Sprintf("l%d_%s", o.namingCounter, generateRandomString(8))
		}
	}
	return obfName
}

// isObfuscatedNameUsedInProject 检查混淆名在整个项目中是否已被使用
func (o *Obfuscator) isObfuscatedNameUsedInProject(name string) bool {
	// 检查objectMapping中的所有混淆名
	for _, obfName := range o.objectMapping {
		if obfName == name {
			return true
		}
	}

	// 检查funcMapping和varMapping（向后兼容）
	for _, obfName := range o.funcMapping {
		if obfName == name {
			return true
		}
	}
	for _, obfName := range o.varMapping {
		if obfName == name {
			return true
		}
	}

	// 检查导入别名映射
	for _, obfName := range o.importAliasMapping {
		if obfName == name {
			return true
		}
	}

	return false
}

