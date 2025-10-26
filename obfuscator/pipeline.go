package obfuscator

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"io"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"strings"
)

// Run 执行整个混淆流程
func (o *Obfuscator) Run() error {
	log.Println("阶段 0/5: 收集导入信息...")
	if err := o.collectImportInfo(); err != nil {
		return fmt.Errorf("收集导入信息失败: %v", err)
	}

	log.Println("阶段 1/5: 扫描项目并收集保护名称...")
	err := filepath.Walk(o.projectRoot, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() || !strings.HasSuffix(path, ".go") {
			return nil
		}
		if strings.Contains(path, "vendor/") {
			return nil
		}

		// 检查是否跳过生成代码
		if o.Config.SkipGeneratedCode && o.isGeneratedFile(path) {
			o.skippedFiles[path] = "Generated code"
			return nil
		}

		// 检查是否排除文件
		if o.isExcluded(path) {
			o.skippedFiles[path] = "Excluded by pattern"
			return nil
		}

		// 解析文件
		node, err := parser.ParseFile(o.fset, path, nil, parser.ParseComments)
		if err != nil {
			log.Printf("警告: 无法解析文件 %s: %v", path, err)
			o.skippedFiles[path] = fmt.Sprintf("Parse error: %v", err)
			return nil
		}

		// 收集保护名称
		o.collectProtectedNames(node)

		// 检查反射使用
		if o.Config.PreserveReflection {
			o.protectReflectionTypes(node)
		}

		return nil
	})
	if err != nil {
		return fmt.Errorf("扫描项目失败: %v", err)
	}

	log.Println("阶段 2/5: 构建作用域分析...")
	if err := o.buildScopeAnalysis(); err != nil {
		return fmt.Errorf("作用域分析失败: %v", err)
	}

	log.Println("阶段 3/5: 构建混淆映射...")
	o.buildObfuscationMapsWithScope()

	log.Println("阶段 4/5: 复制项目文件...")
	// 构建文件名映射（原始路径 -> 混淆后路径）
	fileMapping := make(map[string]string)
	if err := o.copyProjectAndBuildMapping(fileMapping); err != nil {
		return fmt.Errorf("复制项目失败: %v", err)
	}

	log.Println("阶段 5/5: 应用混淆...")
	err = filepath.Walk(o.outputDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() || !strings.HasSuffix(path, ".go") {
			return nil
		}

		// 检查是否跳过文件
		relPath, _ := filepath.Rel(o.outputDir, path)
		originalPath := filepath.Join(o.projectRoot, relPath)
		if _, skipped := o.skippedFiles[originalPath]; skipped {
			return nil
		}

		// 应用混淆（传递原始文件路径）
		if err := o.obfuscateFileWithMapping(path, fileMapping); err != nil {
			return fmt.Errorf("混淆文件 %s 失败: %v", path, err)
		}

		return nil
	})
	if err != nil {
		return fmt.Errorf("应用混淆失败: %v", err)
	}

	return nil
}

// collectImportInfo 收集所有文件的导入信息
func (o *Obfuscator) collectImportInfo() error {
	return filepath.Walk(o.projectRoot, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || !strings.HasSuffix(path, ".go") {
			return err
		}
		if strings.Contains(path, "vendor/") {
			return nil
		}

		// 跳过排除的文件
		if shouldExclude, _ := o.shouldExcludeFile(path); shouldExclude {
			return nil
		}

		node, err := parser.ParseFile(o.fset, path, nil, parser.ParseComments)
		if err != nil {
			return nil // 跳过无法解析的文件
		}

		// 处理所有导入
		for _, imp := range node.Imports {
			if imp.Path == nil {
				continue
			}

			pkgPath := strings.Trim(imp.Path.Value, `"`)

			// 确定代码中使用的包名
			var pkgName string
			if imp.Name != nil {
				// 跳过空白导入和点导入
				if imp.Name.Name == "_" || imp.Name.Name == "." {
					continue
				}
				pkgName = imp.Name.Name
			} else {
				// 使用基础名称
				pkgName = filepath.Base(pkgPath)
			}

			// 存储映射
			if existingName, exists := o.importPathToName[pkgPath]; exists {
				if existingName != pkgName {
					log.Printf("警告: 包 %s 的名称不一致: %s vs %s", pkgPath, existingName, pkgName)
				}
			} else {
				o.importPathToName[pkgPath] = pkgName
			}

			// 标记包名为受保护
			o.packageNames[pkgName] = true

			// 只为标准库创建别名
			if isStandardLibrary(pkgPath) {
				if _, exists := o.importAliasMapping[pkgPath]; !exists {
					alias := fmt.Sprintf("p%s", generateRandomString(8))
					o.importAliasMapping[pkgPath] = alias
				}
			}
		}

		return nil
	})
}

// collectProtectedNames 收集所有不应被混淆的名称
func (o *Obfuscator) collectProtectedNames(node *ast.File) {
	// 构建导入包的映射（包名/别名 -> 导入路径）
	importPaths := make(map[string]string)
	for _, imp := range node.Imports {
		if imp.Path == nil {
			continue
		}
		pkgPath := strings.Trim(imp.Path.Value, `"`)
		var pkgName string
		if imp.Name != nil {
			pkgName = imp.Name.Name
		} else {
			pkgName = filepath.Base(pkgPath)
		}
		importPaths[pkgName] = pkgPath
	}
	
	ast.Inspect(node, func(n ast.Node) bool {
		switch x := n.(type) {
		case *ast.TypeSpec:
			// 保护结构体字段名
			if structType, ok := x.Type.(*ast.StructType); ok {
				if structType.Fields != nil {
					for _, field := range structType.Fields.List {
						// 保护命名字段
						for _, fieldName := range field.Names {
							o.protectedNames[fieldName.Name] = true
						}
						// 保护匿名字段
						if len(field.Names) == 0 {
							if ident, ok := field.Type.(*ast.Ident); ok {
								o.protectedNames[ident.Name] = true
							}
							if starExpr, ok := field.Type.(*ast.StarExpr); ok {
								if ident, ok := starExpr.X.(*ast.Ident); ok {
									o.protectedNames[ident.Name] = true
								}
							}
						}
					}
				}
			}
			// 保护接口方法
			if interfaceType, ok := x.Type.(*ast.InterfaceType); ok {
				if interfaceType.Methods != nil {
					for _, method := range interfaceType.Methods.List {
						for _, methodName := range method.Names {
							o.protectedNames[methodName.Name] = true
						}
					}
				}
			}

		case *ast.SelectorExpr:
			// 只混淆项目内部包的选择器，其他所有选择器都保护
			shouldProtect := true
			
			if ident, ok := x.X.(*ast.Ident); ok {
				pkgName := ident.Name
				// 检查是否是导入的包
				if pkgPath, exists := importPaths[pkgName]; exists {
					// 如果是项目内部的包，不保护（允许混淆）
					if o.isProjectImportPath(pkgPath) {
						shouldProtect = false
					}
				}
				// 注意：如果不在importPaths中，可能是局部变量，保持保护
			}
			
			if shouldProtect {
				o.protectedNames[x.Sel.Name] = true
			}

		case *ast.FuncDecl:
			// 保护方法名
			if x.Recv != nil {
				o.protectedNames[x.Name.Name] = true
			}
		}
		return true
	})
}

// isProjectImportPath 检查导入路径是否属于项目内部
func (o *Obfuscator) isProjectImportPath(importPath string) bool {
	// 项目内部的导入路径通常包含项目的模块路径
	// 例如：github.com/shadow1ng/fscan/Common
	
	// 标准库：不包含点号的路径（如fmt, os, net等）或golang.org/x/开头的
	if !strings.Contains(importPath, ".") {
		return false
	}
	
	// golang.org/x/ 开头的是Go官方扩展库，不是项目内部的
	if strings.HasPrefix(importPath, "golang.org/x/") {
		return false
	}
	
	// gopkg.in 是第三方包托管，不是项目内部的
	if strings.HasPrefix(importPath, "gopkg.in/") {
		return false
	}
	
	// 检查是否包含项目的模块路径
	// 从go.mod中读取模块路径会更准确，但这里我们使用简单的启发式方法
	// 如果导入路径包含项目根目录的名称，认为是项目内部的
	projectName := filepath.Base(o.projectRoot)
	if strings.Contains(importPath, "/"+projectName+"/") ||
		strings.HasSuffix(importPath, "/"+projectName) {
		return true
	}
	
	// 其他情况认为是外部包
	return false
}

// isProjectPackage 检查包名是否属于项目内部
func (o *Obfuscator) isProjectPackage(pkgName string) bool {
	// 检查是否在项目的包名列表中
	// 项目内部的包通常不包含点号（如Common, Core等）
	// 而外部包通常有点号或是标准库名称
	
	// 如果包名包含点号，很可能是外部包
	if strings.Contains(pkgName, ".") {
		return false
	}
	
	// 检查是否是标准库包名
	stdLibPackages := map[string]bool{
		"fmt": true, "os": true, "io": true, "net": true, "http": true,
		"time": true, "strings": true, "bytes": true, "bufio": true,
		"encoding": true, "json": true, "xml": true, "base64": true,
		"crypto": true, "errors": true, "flag": true, "log": true,
		"math": true, "rand": true, "reflect": true, "regexp": true,
		"runtime": true, "sort": true, "strconv": true, "sync": true,
		"syscall": true, "testing": true, "unicode": true, "unsafe": true,
		"context": true, "database": true, "sql": true, "path": true,
		"filepath": true, "template": true, "text": true, "html": true,
		"url": true, "ioutil": true, "atomic": true, "binary": true,
	}
	
	if stdLibPackages[pkgName] {
		return false
	}
	
	// 其他情况认为是项目内部包
	return true
}

// buildObfuscationMaps 构建混淆映射（旧版本，保留用于向后兼容）
func (o *Obfuscator) buildObfuscationMaps() {
	filepath.Walk(o.projectRoot, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || !strings.HasSuffix(path, ".go") {
			return err
		}
		if strings.Contains(path, "vendor/") {
			return nil
		}

		// 跳过排除的文件
		if _, excluded := o.skippedFiles[path]; excluded {
			return nil
		}

		node, err := parser.ParseFile(o.fset, path, nil, parser.ParseComments)
		if err != nil {
			return nil
		}

		// 只收集包级别的函数和变量
		// 不收集局部变量以避免作用域冲突
		for _, decl := range node.Decls {
			switch d := decl.(type) {
			case *ast.FuncDecl:
				// 收集函数名（跳过方法）
				if d.Recv == nil && !o.shouldProtect(d.Name.Name) {
					// 根据配置决定是否混淆导出函数
					if !isExported(d.Name.Name) || o.Config.ObfuscateExported {
						o.obfuscateName(d.Name.Name, true)
					}
				}

			case *ast.GenDecl:
				// 只处理包级别的变量和常量声明
				if d.Tok == token.VAR || d.Tok == token.CONST {
					for _, spec := range d.Specs {
						if valueSpec, ok := spec.(*ast.ValueSpec); ok {
							for _, name := range valueSpec.Names {
								if !o.shouldProtect(name.Name) {
									if !isExported(name.Name) || o.Config.ObfuscateExported {
										o.obfuscateName(name.Name, false)
									}
								}
							}
						}
					}
				}
			}
		}

		return nil
	})
}

// buildScopeAnalysis 为所有文件构建作用域分析
func (o *Obfuscator) buildScopeAnalysis() error {
	return filepath.Walk(o.projectRoot, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || !strings.HasSuffix(path, ".go") {
			return err
		}
		if strings.Contains(path, "vendor/") {
			return nil
		}

		// 跳过排除的文件
		if _, excluded := o.skippedFiles[path]; excluded {
			return nil
		}

		node, err := parser.ParseFile(o.fset, path, nil, parser.ParseComments)
		if err != nil {
			return nil
		}

		// 创建作用域分析器并分析文件
		analyzer := NewScopeAnalyzer(o.fset)
		analyzer.Analyze(node)
		o.fileScopes[path] = analyzer

		return nil
	})
}

// buildObfuscationMapsWithScope 使用作用域分析构建混淆映射
// 修复版本：为每个对象独立生成混淆名，避免同名冲突
func (o *Obfuscator) buildObfuscationMapsWithScope() {
	// 第一步：收集所有文件中的包级别对象（不分组）
	var allPackageLevelObjects []*Object
	
	for filePath, analyzer := range o.fileScopes {
		fileScope := analyzer.GetFileScope()
		if fileScope == nil {
			log.Printf("警告: 文件 %s 没有文件作用域", filePath)
			continue
		}

		// 收集文件级别的对象，并记录文件路径
		for _, obj := range fileScope.Objects {
			if obj.Kind == ObjFunc || obj.Kind == ObjVar || obj.Kind == ObjConst {
				// 为对象添加文件路径信息（用于调试）
				if obj.FilePath == "" {
					obj.FilePath = filePath
				}
				allPackageLevelObjects = append(allPackageLevelObjects, obj)
			}
			// 跳过类型定义 (ObjType)
		}
		
		// 收集局部作用域的对象
		o.collectObjectsForObfuscation(fileScope)
	}

	// 第二步：按名称分组对象（方案1 + build-tag支持）
	// 同名的对象将使用相同的混淆名（支持build-tag场景）
	nameToObjects := make(map[string][]*Object)
	for _, obj := range allPackageLevelObjects {
		nameToObjects[obj.Name] = append(nameToObjects[obj.Name], obj)
	}
	
	// 第三步：为每个名称生成混淆名，同名对象使用相同的混淆名
	funcCount := 0
	varCount := 0
	nameCount := make(map[string]int) // 用于后续的同步逻辑
	
	for name, objects := range nameToObjects {
		if len(objects) == 0 {
			continue
		}
		
		// 检查是否应该保护
		if o.shouldProtect(name) {
			continue
		}

		// 检查是否应该混淆导出的名称
		firstObj := objects[0]
		if firstObj.IsExported && !o.Config.ObfuscateExported {
			continue
		}

		// ✅ 方案1：为所有同名对象生成相同的混淆名
		// 这样可以支持build-tag场景（不同文件的同名函数）
		obfName := o.generateUniqueObfuscatedNameForObject(firstObj)
		
		// 将相同的混淆名应用到所有同名对象
		for _, obj := range objects {
			o.objectMapping[obj] = obfName
		}
		
		// 记录名称计数（用于后续同步）
		nameCount[name] = len(objects)
		
		// 统计
		if firstObj.Kind == ObjFunc {
			funcCount++
		} else if firstObj.Kind == ObjVar || firstObj.Kind == ObjConst {
			varCount++
		}
		
		// 如果有多个同名对象，打印日志
		if len(objects) > 1 {
			log.Printf("同名对象使用相同混淆名: %s → %s (在 %d 个文件中定义)", name, obfName, len(objects))
		}
	}

	// 第三步：为局部变量生成混淆名称（每个对象独立生成）
	localVarCount := 0
	for obj, obfName := range o.objectMapping {
		// 如果已经有混淆名称，跳过
		if obfName != "" {
			continue
		}
		
		// 检查是否应该保护
		if o.shouldProtectObject(obj) {
			delete(o.objectMapping, obj)
			continue
		}
		
		// 为局部变量生成唯一的混淆名称
		obfuscatedName := o.generateUniqueObfuscatedNameForObject(obj)
		o.objectMapping[obj] = obfuscatedName
		localVarCount++
	}

	// 同步到funcMapping和varMapping（方案1版本）：
	// 所有包级别的名称都同步（包括同名的，因为它们现在使用相同的混淆名）
	syncCount := 0
	for name, objects := range nameToObjects {
		if len(objects) == 0 {
			continue
		}
		firstObj := objects[0]
		obfName, exists := o.objectMapping[firstObj]
		if !exists || obfName == "" {
			continue
		}
		
		// 同步到funcMapping/varMapping
		if firstObj.Kind == ObjFunc {
			o.funcMapping[name] = obfName
			syncCount++
		} else if firstObj.Kind == ObjVar || firstObj.Kind == ObjConst {
			o.varMapping[name] = obfName
			syncCount++
		}
	}
	
	log.Printf("收集到 %d 个包级别名称（函数: %d, 变量: %d），%d 个局部变量", 
		len(nameToObjects), funcCount, varCount, localVarCount)
	log.Printf("同步了 %d 个名称到名称映射（用于跨文件引用）", syncCount)
}

// collectObjectsForObfuscation 递归收集作用域中需要混淆的对象
func (o *Obfuscator) collectObjectsForObfuscation(scope *Scope) {
	// 收集当前作用域的对象
	for _, obj := range scope.Objects {
		// 跳过类型定义，因为类型名可能在多个地方被引用
		if obj.Kind == ObjType {
			continue
		}
		
		// 判断是否为文件级别：检查是否为文件作用域（通过检查节点类型）
		isFileLevel := false
		if scope.Node != nil {
			_, isFileLevel = scope.Node.(*ast.File)
		}
		
		if isFileLevel {
			// 文件级别的声明（包级别）
			if obj.Kind == ObjFunc || obj.Kind == ObjVar || obj.Kind == ObjConst {
				o.objectMapping[obj] = "" // 先标记，稍后生成名称
			}
		} else {
			// 局部作用域的变量（函数内部、块内部等）
			// 也收集局部变量进行混淆
			if obj.Kind == ObjVar || obj.Kind == ObjConst {
				o.objectMapping[obj] = "" // 先标记，稍后生成名称
			}
		}
	}

	// 递归处理子作用域
	for _, child := range scope.Children {
		o.collectObjectsForObfuscation(child)
	}
}

// shouldProtectObject 检查对象是否应该被保护
func (o *Obfuscator) shouldProtectObject(obj *Object) bool {
	return o.shouldProtect(obj.Name)
}

// generateObfuscatedNameForObject 为对象生成混淆名称
func (o *Obfuscator) generateObfuscatedNameForObject(obj *Object) string {
	// 检查是否为导出名称（首字母大写）
	isExported := obj.IsExported
	
	// 根据对象类型和是否导出选择前缀
	var prefix string
	if isExported {
		// 导出名称：使用大写前缀
		if obj.Kind == ObjFunc {
			prefix = "Fn" // 导出函数
		} else if obj.Kind == ObjConst {
			prefix = "C" // 导出常量
		} else if obj.Kind == ObjVar {
			prefix = "V" // 导出变量
		} else {
			prefix = "X" // 其他导出对象
		}
	} else {
		// 私有名称：使用小写前缀
		if obj.Kind == ObjFunc {
			prefix = "fn" // 私有函数
		} else if obj.Kind == ObjConst {
			prefix = "c" // 私有常量
		} else if obj.Kind == ObjVar {
			prefix = "v" // 私有变量
		} else {
			prefix = "l" // 其他私有对象
		}
	}

	// 生成唯一的混淆名称
	maxAttempts := 100
	for attempt := 0; attempt < maxAttempts; attempt++ {
		obf := fmt.Sprintf("%s%s", prefix, generateRandomString(12))

		// 检查此名称是否已被使用
		if !o.isObfuscatedNameUsed(obf) {
			return obf
		}
	}

	// 回退：使用基于计数器的方法
	o.namingCounter++
	return fmt.Sprintf("%s%d_%s", prefix, o.namingCounter, generateRandomString(8))
}

// isObfuscatedNameUsed 检查混淆名称是否已被使用
func (o *Obfuscator) isObfuscatedNameUsed(name string) bool {
	// 检查对象映射
	for _, obfName := range o.objectMapping {
		if obfName == name {
			return true
		}
	}

	// 检查旧的映射（向后兼容）
	for _, obfName := range o.varMapping {
		if obfName == name {
			return true
		}
	}
	for _, obfName := range o.funcMapping {
		if obfName == name {
			return true
		}
	}
	for _, obfName := range o.importAliasMapping {
		if obfName == name {
			return true
		}
	}

	return false
}

// copyProject 复制项目到输出目录
func (o *Obfuscator) copyProject() error {
	return filepath.Walk(o.projectRoot, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		relPath, err := filepath.Rel(o.projectRoot, path)
		if err != nil {
			return err
		}

		// 处理文件名混淆（只对未被排除的 Go 文件）
		outputPath := filepath.Join(o.outputDir, relPath)
		if o.Config.ObfuscateFileNames && strings.HasSuffix(path, ".go") {
			// 检查文件是否被排除
			_, isSkipped := o.skippedFiles[path]
			if !isSkipped {
				dir := filepath.Dir(outputPath)
				base := filepath.Base(outputPath)
				// 使用 obfuscateFileName 函数，它会保护 main.go 等特殊文件
				obfuscatedName := o.obfuscateFileName(base)
				if obfuscatedName != base {
					outputPath = filepath.Join(dir, obfuscatedName)
				}
			}
		}

		if info.IsDir() {
			return os.MkdirAll(outputPath, info.Mode())
		}

		// 复制文件（包括被排除的文件）
		return o.copyFile(path, outputPath)
	})
}

// copyProjectAndBuildMapping 复制项目到输出目录并构建文件映射
func (o *Obfuscator) copyProjectAndBuildMapping(fileMapping map[string]string) error {
	return filepath.Walk(o.projectRoot, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		relPath, err := filepath.Rel(o.projectRoot, path)
		if err != nil {
			return err
		}

		// 处理文件名混淆（只对未被排除的 Go 文件）
		outputPath := filepath.Join(o.outputDir, relPath)
		if o.Config.ObfuscateFileNames && strings.HasSuffix(path, ".go") {
			// 检查文件是否被排除
			_, isSkipped := o.skippedFiles[path]
			if !isSkipped {
				dir := filepath.Dir(outputPath)
				base := filepath.Base(outputPath)
				// 使用 obfuscateFileName 函数，它会保护 main.go 等特殊文件
				obfuscatedName := o.obfuscateFileName(base)
				if obfuscatedName != base {
					outputPath = filepath.Join(dir, obfuscatedName)
				}
			}
		}

		// 记录映射关系（混淆后路径 -> 原始路径）
		if strings.HasSuffix(path, ".go") {
			fileMapping[outputPath] = path
		}

		if info.IsDir() {
			return os.MkdirAll(outputPath, info.Mode())
		}

		// 复制文件（包括被排除的文件）
		return o.copyFile(path, outputPath)
	})
}

// copyFile 复制单个文件
func (o *Obfuscator) copyFile(src, dst string) error {
	srcFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer srcFile.Close()

	if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
		return err
	}

	dstFile, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer dstFile.Close()

	_, err = io.Copy(dstFile, srcFile)
	return err
}

// obfuscateFile 混淆单个文件
func (o *Obfuscator) obfuscateFile(filePath string) error {
	// 解析文件
	node, err := parser.ParseFile(o.fset, filePath, nil, parser.ParseComments)
	if err != nil {
		return fmt.Errorf("解析文件失败: %v", err)
	}

	// 移除注释（保留构建标签和编译指令）
	if o.Config.RemoveComments {
		var filteredComments []*ast.CommentGroup
		for _, cg := range node.Comments {
			var keepComments []*ast.Comment
			for _, c := range cg.List {
				if o.shouldKeepComment(c.Text) {
					keepComments = append(keepComments, c)
				}
			}
			if len(keepComments) > 0 {
				cg.List = keepComments
				filteredComments = append(filteredComments, cg)
			}
		}
		node.Comments = filteredComments

		// 清除文档注释
		for _, decl := range node.Decls {
			if genDecl, ok := decl.(*ast.GenDecl); ok {
				genDecl.Doc = nil
				for _, spec := range genDecl.Specs {
					if typeSpec, ok := spec.(*ast.TypeSpec); ok {
						typeSpec.Doc = nil
						typeSpec.Comment = nil
					}
					if valueSpec, ok := spec.(*ast.ValueSpec); ok {
						valueSpec.Doc = nil
						valueSpec.Comment = nil
					}
					if importSpec, ok := spec.(*ast.ImportSpec); ok {
						importSpec.Doc = nil
						importSpec.Comment = nil
					}
				}
			}
			if funcDecl, ok := decl.(*ast.FuncDecl); ok {
				funcDecl.Doc = nil
			}
		}
	}

	// 获取原始文件路径（从输出目录映射回项目根目录）
	relPath, _ := filepath.Rel(o.outputDir, filePath)
	originalPath := filepath.Join(o.projectRoot, relPath)

	// 应用转换（使用作用域信息）
	o.applyTransformationsWithScope(node, originalPath)

	// 格式化并写入
	var buf bytes.Buffer
	if err := format.Node(&buf, o.fset, node); err != nil {
		return fmt.Errorf("格式化失败: %v", err)
	}

	source := buf.String()

	// 字符串加密
	if o.Config.EncryptStrings {
		packageName := node.Name.Name
		hadEncryption := o.encryptStringsInAST(node)

		// 检查文件是否有平台专用的 build tags
		hasPlatformBuildTag := o.hasPlatformSpecificBuildTag(node)

		// ✅ 修复：所有包都应该检查是否已添加解密函数
		// 之前main包的特殊逻辑会导致每个main文件都添加解密函数，导致redeclared错误
		shouldAddDecryptFunc := hadEncryption && !o.decryptFuncAdded[packageName] && !hasPlatformBuildTag

		if shouldAddDecryptFunc {
			// 确保有 base64 导入
			source = o.ensureBase64ImportInSource(source)

			// 获取 base64 别名
			base64Alias := "base64"
			for _, imp := range node.Imports {
				if imp.Path != nil && strings.Trim(imp.Path.Value, `"`) == "encoding/base64" {
					if imp.Name != nil {
						base64Alias = imp.Name.Name
					}
					break
				}
			}

			// 添加解密函数
			decryptFunc := o.generateDecryptFunction(base64Alias)
			source = source + "\n" + decryptFunc

			o.decryptFuncAdded[packageName] = true
		}

		// 加密字符串字面量
		source, _ = o.encryptStringsInSource(source)
	}

	// 写回文件
	return ioutil.WriteFile(filePath, []byte(source), 0644)
}

// obfuscateFileWithMapping 使用文件映射混淆单个文件
func (o *Obfuscator) obfuscateFileWithMapping(filePath string, fileMapping map[string]string) error {
	// 解析文件
	node, err := parser.ParseFile(o.fset, filePath, nil, parser.ParseComments)
	if err != nil {
		return fmt.Errorf("解析文件失败: %v", err)
	}

	// 移除注释（保留构建标签和编译指令）
	if o.Config.RemoveComments {
		var filteredComments []*ast.CommentGroup
		for _, cg := range node.Comments {
			var keepComments []*ast.Comment
			for _, c := range cg.List {
				if o.shouldKeepComment(c.Text) {
					keepComments = append(keepComments, c)
				}
			}
			if len(keepComments) > 0 {
				cg.List = keepComments
				filteredComments = append(filteredComments, cg)
			}
		}
		node.Comments = filteredComments

		// 清除文档注释
		for _, decl := range node.Decls {
			if genDecl, ok := decl.(*ast.GenDecl); ok {
				genDecl.Doc = nil
				for _, spec := range genDecl.Specs {
					if typeSpec, ok := spec.(*ast.TypeSpec); ok {
						typeSpec.Doc = nil
						typeSpec.Comment = nil
					}
					if valueSpec, ok := spec.(*ast.ValueSpec); ok {
						valueSpec.Doc = nil
						valueSpec.Comment = nil
					}
					if importSpec, ok := spec.(*ast.ImportSpec); ok {
						importSpec.Doc = nil
						importSpec.Comment = nil
					}
				}
			}
			if funcDecl, ok := decl.(*ast.FuncDecl); ok {
				funcDecl.Doc = nil
			}
		}
	}

	// 从文件映射获取原始文件路径
	originalPath, exists := fileMapping[filePath]
	if !exists {
		// 如果映射中没有，尝试从输出目录映射回项目根目录
		relPath, _ := filepath.Rel(o.outputDir, filePath)
		originalPath = filepath.Join(o.projectRoot, relPath)
	}

	// 应用转换（使用作用域信息）
	o.applyTransformationsWithScope(node, originalPath)

	// 格式化并写入
	var buf bytes.Buffer
	if err := format.Node(&buf, o.fset, node); err != nil {
		return fmt.Errorf("格式化失败: %v", err)
	}

	source := buf.String()

	// 字符串加密
	if o.Config.EncryptStrings {
		packageName := node.Name.Name
		hadEncryption := o.encryptStringsInAST(node)

		// 检查文件是否有平台专用的 build tags
		hasPlatformBuildTag := o.hasPlatformSpecificBuildTag(node)

		// ✅ 修复：所有包都应该检查是否已添加解密函数
		// 之前main包的特殊逻辑会导致每个main文件都添加解密函数，导致redeclared错误
		shouldAddDecryptFunc := hadEncryption && !o.decryptFuncAdded[packageName] && !hasPlatformBuildTag

		if shouldAddDecryptFunc {
			// 确保有 base64 导入
			source = o.ensureBase64ImportInSource(source)

			// 获取 base64 别名
			base64Alias := "base64"
			for _, imp := range node.Imports {
				if imp.Path != nil && strings.Trim(imp.Path.Value, `"`) == "encoding/base64" {
					if imp.Name != nil {
						base64Alias = imp.Name.Name
					}
					break
				}
			}

			// 添加解密函数
			decryptFunc := o.generateDecryptFunction(base64Alias)
			source = source + "\n" + decryptFunc

			o.decryptFuncAdded[packageName] = true
		}

		// 加密字符串字面量
		source, _ = o.encryptStringsInSource(source)
	}

	// 写回文件
	return ioutil.WriteFile(filePath, []byte(source), 0644)
}

// applyTransformations 应用 AST 转换
func (o *Obfuscator) applyTransformations(node *ast.File) {
	// 步骤 1: 构建此文件中的包映射
	filePackages := make(map[string]string) // 代码中的包名 -> 混淆别名

	// 更新导入语句并记录包
	for _, decl := range node.Decls {
		if genDecl, ok := decl.(*ast.GenDecl); ok && genDecl.Tok == token.IMPORT {
			for _, spec := range genDecl.Specs {
				if importSpec, ok := spec.(*ast.ImportSpec); ok {
					if importSpec.Path != nil {
						// 跳过空白导入和点导入
						if importSpec.Name != nil && (importSpec.Name.Name == "_" || importSpec.Name.Name == ".") {
							continue
						}

						pkgPath := strings.Trim(importSpec.Path.Value, `"`)

						// 确定代码中使用的包名
						var pkgNameInCode string
						if importSpec.Name != nil {
							pkgNameInCode = importSpec.Name.Name
						} else {
							pkgNameInCode = filepath.Base(pkgPath)
						}

						// 只为标准库应用别名
						if alias, exists := o.importAliasMapping[pkgPath]; exists {
							// 更新导入语句
							if importSpec.Name != nil {
								importSpec.Name.Name = alias
							} else {
								importSpec.Name = &ast.Ident{Name: alias}
							}
							// 记录此包供后续使用
							filePackages[pkgNameInCode] = alias
						}
					}
				}
			}
		}
	}

	// 步骤 2: 替换包引用（仅限此文件中导入的包）
	ast.Inspect(node, func(n ast.Node) bool {
		if sel, ok := n.(*ast.SelectorExpr); ok {
			if ident, ok := sel.X.(*ast.Ident); ok {
				// 只有当满足以下条件时才替换：
				// 1. 此包名在此文件中被导入
				// 2. 标识符没有 Object（包标识符没有设置 Obj）
				//    如果 Obj 不为 nil，说明它是局部声明的变量/参数
				if alias, exists := filePackages[ident.Name]; exists {
					if ident.Obj == nil {
						ident.Name = alias
					}
				}
			}
		}
		return true
	})

	// 步骤 3: 混淆函数声明
	ast.Inspect(node, func(n ast.Node) bool {
		if fn, ok := n.(*ast.FuncDecl); ok {
			if fn.Recv == nil && !o.shouldProtect(fn.Name.Name) {
				if obf, exists := o.funcMapping[fn.Name.Name]; exists {
					fn.Name.Name = obf
				}
			}
		}
		return true
	})

	// 步骤 4: 混淆变量声明和赋值
	ast.Inspect(node, func(n ast.Node) bool {
		switch x := n.(type) {
		case *ast.ValueSpec:
			for _, name := range x.Names {
				if !o.shouldProtect(name.Name) {
					if obf, exists := o.varMapping[name.Name]; exists {
						name.Name = obf
					}
				}
			}
		case *ast.AssignStmt:
			for _, lhs := range x.Lhs {
				if ident, ok := lhs.(*ast.Ident); ok {
					if !o.shouldProtect(ident.Name) {
						if obf, exists := o.varMapping[ident.Name]; exists {
							ident.Name = obf
						}
					}
				}
			}
		case *ast.Ident:
			// 混淆标识符引用
			if !o.shouldProtect(x.Name) {
				if obf, exists := o.varMapping[x.Name]; exists {
					x.Name = obf
				}
				if obf, exists := o.funcMapping[x.Name]; exists {
					x.Name = obf
				}
			}
		}
		return true
	})

	// 步骤 5: 注入垃圾代码
	if o.Config.InjectJunkCode {
		for _, decl := range node.Decls {
			if fn, ok := decl.(*ast.FuncDecl); ok {
				if fn.Body != nil && len(fn.Body.List) > 0 && !o.shouldSkipJunkCodeInjection(fn) {
					junkStmts := o.generateJunkStatements(false)
					fn.Body.List = append(junkStmts, fn.Body.List...)
				}
			}
		}
	}
}

// applyTransformationsWithScope 使用作用域信息应用 AST 转换
func (o *Obfuscator) applyTransformationsWithScope(node *ast.File, originalFilePath string) {
	// 步骤 1: 构建此文件中的包映射
	filePackages := make(map[string]string) // 代码中的包名 -> 混淆别名

	// 更新导入语句并记录包
	for _, decl := range node.Decls {
		if genDecl, ok := decl.(*ast.GenDecl); ok && genDecl.Tok == token.IMPORT {
			for _, spec := range genDecl.Specs {
				if importSpec, ok := spec.(*ast.ImportSpec); ok {
					if importSpec.Path != nil {
						// 跳过空白导入和点导入
						if importSpec.Name != nil && (importSpec.Name.Name == "_" || importSpec.Name.Name == ".") {
							continue
						}

						pkgPath := strings.Trim(importSpec.Path.Value, `"`)

						// 确定代码中使用的包名
						var pkgNameInCode string
						if importSpec.Name != nil {
							pkgNameInCode = importSpec.Name.Name
						} else {
							pkgNameInCode = filepath.Base(pkgPath)
						}

						// 只为标准库应用别名
						if alias, exists := o.importAliasMapping[pkgPath]; exists {
							// 更新导入语句
							if importSpec.Name != nil {
								importSpec.Name.Name = alias
							} else {
								importSpec.Name = &ast.Ident{Name: alias}
							}
							// 记录此包供后续使用
							filePackages[pkgNameInCode] = alias
						}
					}
				}
			}
		}
	}

	// 步骤 2: 替换包引用（仅限此文件中导入的包）
	ast.Inspect(node, func(n ast.Node) bool {
		if sel, ok := n.(*ast.SelectorExpr); ok {
			if ident, ok := sel.X.(*ast.Ident); ok {
				// 只有当满足以下条件时才替换：
				// 1. 此包名在此文件中被导入
				// 2. 标识符没有 Object（包标识符没有设置 Obj）
				//    如果 Obj 不为 nil，说明它是局部声明的变量/参数
				if alias, exists := filePackages[ident.Name]; exists {
					if ident.Obj == nil {
						ident.Name = alias
					}
				}
			}
		}
		return true
	})

	// 获取此文件的作用域分析器
	analyzer, hasScope := o.fileScopes[originalFilePath]
	
	if hasScope {
		// 步骤 3: 使用作用域信息混淆标识符
		o.obfuscateIdentifiersWithScope(node, analyzer)
	} else {
		// 回退到旧的混淆方法（如果没有作用域信息）
		log.Printf("警告: 文件 %s 没有作用域信息，使用旧的混淆方法", originalFilePath)
		
		// 步骤 3: 混淆函数声明
		ast.Inspect(node, func(n ast.Node) bool {
			if fn, ok := n.(*ast.FuncDecl); ok {
				if fn.Recv == nil && !o.shouldProtect(fn.Name.Name) {
					if obf, exists := o.funcMapping[fn.Name.Name]; exists {
						fn.Name.Name = obf
					}
				}
			}
			return true
		})

		// 步骤 4: 混淆变量声明和赋值
		ast.Inspect(node, func(n ast.Node) bool {
			switch x := n.(type) {
			case *ast.ValueSpec:
				for _, name := range x.Names {
					if !o.shouldProtect(name.Name) {
						if obf, exists := o.varMapping[name.Name]; exists {
							name.Name = obf
						}
					}
				}
			case *ast.AssignStmt:
				for _, lhs := range x.Lhs {
					if ident, ok := lhs.(*ast.Ident); ok {
						if !o.shouldProtect(ident.Name) {
							if obf, exists := o.varMapping[ident.Name]; exists {
								ident.Name = obf
							}
						}
					}
				}
			case *ast.Ident:
				// 混淆标识符引用
				if !o.shouldProtect(x.Name) {
					if obf, exists := o.varMapping[x.Name]; exists {
						x.Name = obf
					}
					if obf, exists := o.funcMapping[x.Name]; exists {
						x.Name = obf
					}
				}
			}
			return true
		})
	}

	// 步骤 5: 注入垃圾代码
	if o.Config.InjectJunkCode {
		for _, decl := range node.Decls {
			if fn, ok := decl.(*ast.FuncDecl); ok {
				if fn.Body != nil && len(fn.Body.List) > 0 && !o.shouldSkipJunkCodeInjection(fn) {
					junkStmts := o.generateJunkStatements(false)
					fn.Body.List = append(junkStmts, fn.Body.List...)
				}
			}
		}
	}
}

// obfuscateIdentifiersWithScope 使用作用域信息混淆标识符
func (o *Obfuscator) obfuscateIdentifiersWithScope(node *ast.File, analyzer *ScopeAnalyzer) {
	// 记录哪些标识符是类型引用，不应该被混淆
	typeRefs := make(map[*ast.Ident]bool)
	
	// 第一遍：收集所有类型引用
	ast.Inspect(node, func(n ast.Node) bool {
		switch x := n.(type) {
		case *ast.Field:
			// 字段类型
			o.markTypeIdents(x.Type, typeRefs)
		case *ast.ValueSpec:
			// 变量/常量类型
			if x.Type != nil {
				o.markTypeIdents(x.Type, typeRefs)
			}
		case *ast.TypeAssertExpr:
			// 类型断言
			if x.Type != nil {
				o.markTypeIdents(x.Type, typeRefs)
			}
		case *ast.CallExpr:
			// 类型转换：只有当Fun是类型标识符时才标记
			// 例如：int(x), MyType(x)
			// 但不包括函数调用：myFunc(x)
			// 简单启发式：如果Fun是Ident且首字母小写，可能是内置类型转换
			// 更准确的方法需要类型信息，这里我们跳过CallExpr的处理
			// 因为大部分CallExpr是函数调用，不是类型转换
			_ = x // 跳过
		}
		return true
	})
	
	// 第二遍：替换标识符
	ast.Inspect(node, func(n ast.Node) bool {
		switch x := n.(type) {
		case *ast.Ident:
			// 跳过类型引用
			if typeRefs[x] {
				return true
			}
			
			// 跳过保护的名称
			if o.shouldProtect(x.Name) {
				return true
			}

			// ✅ 改进的替换策略（方案2 + 精确的作用域处理）：
			// 
			// 问题：简单的作用域查找无法区分"当前位置的作用域"和"文件级作用域"
			// 例如：局部变量 transport 和 包级函数 transport 同名
			// 
			// 解决方案：
			// 1. 先尝试在文件级作用域查找（只查找包级对象，不查找子作用域）
			// 2. 如果找到且在objectMapping中，使用它
			// 3. 否则，使用funcMapping/varMapping（跨文件引用）
			// 
			// 这样可以避免错误地匹配局部变量
			
			// 只在文件级作用域查找（不递归到子作用域）
			fileScope := analyzer.GetFileScope()
			var obj *Object
			if fileScope != nil {
				obj = fileScope.Objects[x.Name]  // 直接查找，不递归
			}
			
			if obj != nil {
				// 跳过类型对象
				if obj.Kind == ObjType {
					return true
				}
				if obfName, hasObf := o.objectMapping[obj]; hasObf && obfName != "" {
					x.Name = obfName
					return true
				}
			}

			// 跨文件引用或找不到对象：使用名称映射
			// funcMapping/varMapping 包含所有唯一名称（不包括同名的私有对象）
			if obfName, exists := o.funcMapping[x.Name]; exists {
				x.Name = obfName
				return true
			}
			if obfName, exists := o.varMapping[x.Name]; exists {
				x.Name = obfName
				return true
			}
		}
		return true
	})
}

// markTypeIdents 标记表达式中的所有类型标识符
func (o *Obfuscator) markTypeIdents(expr ast.Expr, typeRefs map[*ast.Ident]bool) {
	if expr == nil {
		return
	}
	
	switch x := expr.(type) {
	case *ast.Ident:
		typeRefs[x] = true
	case *ast.StarExpr:
		o.markTypeIdents(x.X, typeRefs)
	case *ast.ArrayType:
		o.markTypeIdents(x.Elt, typeRefs)
	case *ast.MapType:
		o.markTypeIdents(x.Key, typeRefs)
		o.markTypeIdents(x.Value, typeRefs)
	case *ast.ChanType:
		o.markTypeIdents(x.Value, typeRefs)
	case *ast.SelectorExpr:
		// 对于 pkg.Type，标记 Type
		typeRefs[x.Sel] = true
	case *ast.FuncType:
		// 函数类型的参数和返回值
		if x.Params != nil {
			for _, field := range x.Params.List {
				o.markTypeIdents(field.Type, typeRefs)
			}
		}
		if x.Results != nil {
			for _, field := range x.Results.List {
				o.markTypeIdents(field.Type, typeRefs)
			}
		}
	}
}

// findObjectInScopeRecursive 递归查找对象
func (o *Obfuscator) findObjectInScopeRecursive(name string, scope *Scope) *Object {
	if scope == nil {
		return nil
	}
	
	// 在当前作用域查找
	if obj, exists := scope.Objects[name]; exists {
		return obj
	}
	
	// 在子作用域中查找
	for _, child := range scope.Children {
		if obj := o.findObjectInScopeRecursive(name, child); obj != nil {
			return obj
		}
	}
	
	return nil
}

// shouldKeepComment 判断是否应保留注释
func (o *Obfuscator) shouldKeepComment(text string) bool {
	// 保留构建标签和编译指令
	return strings.HasPrefix(text, "//go:") ||
		strings.HasPrefix(text, "// +build") ||
		strings.HasPrefix(text, "//+build")
}

// hasPlatformSpecificBuildTag 检查文件是否有平台专用的 build tag
func (o *Obfuscator) hasPlatformSpecificBuildTag(node *ast.File) bool {
	// 平台关键词列表
	platformKeywords := []string{
		"windows", "linux", "darwin", "freebsd", "openbsd", 
		"netbsd", "dragonfly", "solaris", "android", "aix",
		"386", "amd64", "arm", "arm64", "mips", "mips64",
		"ppc64", "ppc64le", "s390x", "wasm",
	}
	
	// 检查所有注释
	for _, cg := range node.Comments {
		for _, c := range cg.List {
			text := c.Text
			// 检查 //go:build 和 // +build 标签
			if strings.HasPrefix(text, "//go:build") || strings.HasPrefix(text, "// +build") || strings.HasPrefix(text, "//+build") {
				// 检查是否包含平台关键词
				lowerText := strings.ToLower(text)
				for _, keyword := range platformKeywords {
					if strings.Contains(lowerText, keyword) {
						return true
					}
				}
			}
		}
	}
	
	return false
}

// encryptStringsInAST 在 AST 中标记需要加密的字符串
func (o *Obfuscator) encryptStringsInAST(node *ast.File) bool {
	hadEncryption := false

	// 收集需要跳过的字符串
	skipLiterals := make(map[*ast.BasicLit]bool)

	// 标记导入路径
	for _, imp := range node.Imports {
		if imp.Path != nil {
			skipLiterals[imp.Path] = true
		}
	}

	// 标记结构体标签
	ast.Inspect(node, func(n ast.Node) bool {
		if field, ok := n.(*ast.Field); ok {
			if field.Tag != nil {
				skipLiterals[field.Tag] = true
			}
		}
		return true
	})

	// 检查是否有字符串需要加密
	ast.Inspect(node, func(n ast.Node) bool {
		if lit, ok := n.(*ast.BasicLit); ok {
			if skipLiterals[lit] {
				return true
			}
			if lit.Kind == token.STRING && len(lit.Value) > 2 {
				if !strings.HasPrefix(lit.Value, "`") {
					hadEncryption = true
				}
			}
		}
		return true
	})

	return hadEncryption
}

// ensureBase64ImportInSource 确保源代码中有 base64 导入
func (o *Obfuscator) ensureBase64ImportInSource(source string) string {
	if strings.Contains(source, `"encoding/base64"`) {
		return source
	}

	lines := strings.Split(source, "\n")
	for i, line := range lines {
		if strings.HasPrefix(strings.TrimSpace(line), "import (") {
			// 在 import 块中添加
			lines[i] = line + "\n\t\"encoding/base64\""
			return strings.Join(lines, "\n")
		}
		if strings.HasPrefix(strings.TrimSpace(line), "import ") {
			// 单行 import，转换为块
			lines[i] = "import (\n\t\"encoding/base64\"\n" + strings.TrimPrefix(line, "import ") + "\n)"
			return strings.Join(lines, "\n")
		}
	}

	// 没有找到 import，在 package 后添加
	for i, line := range lines {
		if strings.HasPrefix(strings.TrimSpace(line), "package ") {
			lines = append(lines[:i+1], append([]string{"\nimport \"encoding/base64\"\n"}, lines[i+1:]...)...)
			return strings.Join(lines, "\n")
		}
	}

	return source
}

// encryptStringsInSource 在源代码中加密字符串
func (o *Obfuscator) encryptStringsInSource(source string) (string, bool) {
	lines := strings.Split(source, "\n")
	result := make([]string, len(lines))

	hadEncryption := false
	inImportBlock := false
	inConstBlock := false

	for i, line := range lines {
		trimmed := strings.TrimSpace(line)

		// 跟踪 import 块
		if strings.HasPrefix(trimmed, "import (") {
			inImportBlock = true
			result[i] = line
			continue
		}
		if inImportBlock && trimmed == ")" {
			inImportBlock = false
			result[i] = line
			continue
		}

		// 跟踪 const 块
		if strings.HasPrefix(trimmed, "const (") {
			inConstBlock = true
			result[i] = line
			continue
		}
		if inConstBlock && trimmed == ")" {
			inConstBlock = false
			result[i] = line
			continue
		}

		// 跳过特殊行
		if inImportBlock || inConstBlock || strings.Contains(trimmed, "package ") ||
			strings.HasPrefix(trimmed, "import ") || strings.HasPrefix(trimmed, "const ") {
			result[i] = line
			continue
		}

		// 跳过结构体标签
		if strings.Contains(line, "`") && strings.Contains(line, ":") {
			result[i] = line
			continue
		}

		// 处理行
		newLine := line
		inString := false
		inRune := false
		escaped := false
		stringStart := -1

		for j := 0; j < len(line); j++ {
			ch := line[j]

			if escaped {
				escaped = false
				continue
			}

			if ch == '\\' {
				escaped = true
				continue
			}

			// 处理单引号（rune）
			if ch == '\'' && !inString {
				if !inRune {
					inRune = true
				} else {
					inRune = false
				}
				continue
			}

			// 只处理双引号
			if ch == '"' && !inRune {
				if !inString {
					inString = true
					stringStart = j
				} else {
					inString = false
					if stringStart >= 0 {
						strWithQuotes := line[stringStart : j+1]
						if len(strWithQuotes) > 4 {
							strContent := strWithQuotes[1 : len(strWithQuotes)-1]
							if len(strContent) > 2 && !strings.Contains(strContent, "\\") {
								encrypted := o.encryptString(strContent)
								replacement := fmt.Sprintf(`%s("%s")`, o.decryptFuncName, encrypted)
								newLine = strings.Replace(newLine, strWithQuotes, replacement, 1)
								hadEncryption = true
							}
						}
					}
					stringStart = -1
				}
			}
		}

		result[i] = newLine
	}

	return strings.Join(result, "\n"), hadEncryption
}
