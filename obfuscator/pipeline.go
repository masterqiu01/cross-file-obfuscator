package obfuscator

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"cross-file-obfuscator/internal/logger"
)

// Run 执行整个混淆流程
func (o *Obfuscator) Run() error {
	// 第一步：从 go.mod 读取模块名，用于后续精确判断项目内部包
	if err := o.readModuleName(); err != nil {
		logger.Warnf("无法读取 go.mod (将使用文件夹名启发式判断): %v", err)
	}

	// 识别含独立 go.mod 的嵌套子模块，为各模块根独立分发解密包
	o.discoverSubModuleRoots()

	// 如果启用了字符串加密，创建解密包并保护相关名称（干跑模式下不创建任何文件）
	if o.Config.EncryptStrings {
		// 提前保护解密函数名称和包名
		for _, name := range o.decryptFuncNames {
			o.protectedNames[name] = true
		}
		o.packageNames[o.decryptPkgName] = true

		if !o.Config.DryRun {
			if err := o.createDecryptPackage(); err != nil {
				return fmt.Errorf("创建解密包失败: %v", err)
			}
		}
	}

	logger.Infof("阶段 1/5: 扫描项目(收集导入/保护名/作用域)...")
	if err := o.scanProject(); err != nil {
		return fmt.Errorf("扫描项目失败: %v", err)
	}

	// 反射精确保护收尾：递归展开已命中的反射目标类型（字段指向的命名类型等）
	o.protectReflectionTypes()

	logger.Infof("阶段 2/5: 构建混淆映射...")
	o.buildObfuscationMapsWithScope()

	// 干跑模式：只打印会混淆的内容，不实际执行文件操作
	if o.Config.DryRun {
		return o.printDryRunReport()
	}

	logger.Infof("阶段 3/5: 复制项目文件...")
	// 构建文件名映射（原始路径 -> 混淆后路径）
	fileMapping := make(map[string]string)
	if err := o.copyProjectAndBuildMapping(fileMapping); err != nil {
		return fmt.Errorf("复制项目失败: %v", err)
	}

	logger.Infof("阶段 4/5: 混淆输出文件...")
	if err := o.obfuscateOutputFiles(fileMapping); err != nil {
		return fmt.Errorf("应用混淆失败: %v", err)
	}

	return nil
}

// scanProject 单遍扫描项目：合并导入信息收集、保护名称收集、反射保护、
// 作用域分析、平台特定文件识别，避免同一文件被多次解析。
func (o *Obfuscator) scanProject() error {
	return filepath.Walk(o.projectRoot, func(path string, info os.FileInfo, err error) error {
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
			logger.Warnf("无法解析文件 %s: %v", path, err)
			o.skippedFiles[path] = fmt.Sprintf("Parse error: %v", err)
			return nil
		}

		o.totalGoFiles++

		// 收集导入信息（包名、标准库别名）
		o.collectImportInfoFromFile(node)

		// 收集保护名称
		o.collectProtectedNames(node, path)

		// 构建作用域分析（先于反射精确分析，供标识符类型解析复用）
		analyzer := NewScopeAnalyzer(o.fset)
		analyzer.Analyze(node)
		o.fileScopes[path] = analyzer

		// 反射精确保护：收集被 reflect/JSON 实际引用的类型（garble 式）
		if o.Config.PreserveReflection {
			o.collectReflectionUsage(node, analyzer)
		}

		return nil
	})
}

// collectImportInfoFromFile 收集单个文件的导入信息
func (o *Obfuscator) collectImportInfoFromFile(node *ast.File) {
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

		// 标记包名为受保护
		o.packageNames[pkgName] = true

		// 只为标准库创建别名
		if isStandardLibrary(pkgPath) {
			if _, exists := o.importAliasMapping[pkgPath]; !exists {
				alias := fmt.Sprintf("p%s", generateRandomString(8))
				o.importAliasMapping[pkgPath] = alias
				o.usedNames[alias] = true
			}
		}
	}
}

// printDryRunReport 打印干跑报告：展示将要混淆的内容，不实际写入任何文件
func (o *Obfuscator) printDryRunReport() error {
	fmt.Println()
	fmt.Println("============================================================")
	fmt.Println("   [DRY-RUN 模式] 以下是将要执行的混淆操作（不会实际写入）")
	fmt.Println("============================================================")

	if o.moduleName != "" {
		fmt.Printf("  模块: %s\n", o.moduleName)
	}

	if len(o.skippedFiles) > 0 {
		fmt.Printf("\n  [跳过文件] 共 %d 个:\n", len(o.skippedFiles))
		for path, reason := range o.skippedFiles {
			relPath, _ := filepath.Rel(o.projectRoot, path)
			fmt.Printf("    - %-40s  (%s)\n", relPath, reason)
		}
	}

	if len(o.mainFiles) > 0 {
		fmt.Printf("\n  [保护文件名 - 含 main 函数] 共 %d 个:\n", len(o.mainFiles))
		for path := range o.mainFiles {
			relPath, _ := filepath.Rel(o.projectRoot, path)
			fmt.Printf("    - %s\n", relPath)
		}
	}

	if len(o.embedFiles) > 0 {
		fmt.Printf("\n  [保护文件名 - go:embed 引用] 共 %d 个:\n", len(o.embedFiles))
		for path := range o.embedFiles {
			relPath, _ := filepath.Rel(o.projectRoot, path)
			fmt.Printf("    - %s\n", relPath)
		}
	}

	funcCount := 0
	varCount := 0
	printed := 0
	fmt.Println("\n  [将要混淆的函数/变量] (最多显示前 20 条):")
	for obj, obfName := range o.objectMapping {
		if obfName == "" {
			continue
		}
		if obj.Kind == ObjFunc {
			funcCount++
			if printed < 20 {
				fmt.Printf("    函数: %-30s  ->  %s\n", obj.Name, obfName)
				printed++
			}
		} else if obj.Kind == ObjVar || obj.Kind == ObjConst {
			varCount++
		}
	}
	if funcCount > 20 {
		fmt.Printf("    ... 共 %d 个函数 (仅显示前 20 条)\n", funcCount)
	}

	fmt.Println()
	fmt.Printf("  统计: 函数 %d 个, 变量/常量 %d 个, 保护名称 %d 个, 跳过文件 %d 个\n",
		funcCount, varCount, len(o.protectedNames), len(o.skippedFiles))
	fmt.Println("============================================================")
	fmt.Println("  [DRY-RUN 完成] 未写入任何文件。去掉 -dry-run 后正式执行。")
	fmt.Println("============================================================")
	return nil
}

// collectProtectedNames 收集所有不应被混淆的名称，并记录特殊文件
func (o *Obfuscator) collectProtectedNames(node *ast.File, filePath string) {
	isTestFile := strings.HasSuffix(filePath, "_test.go")

	// 记录 main 文件
	if node.Name.Name == "main" {
		for _, decl := range node.Decls {
			if fn, ok := decl.(*ast.FuncDecl); ok && fn.Name.Name == "main" && fn.Recv == nil {
				o.mainFiles[filePath] = true
				break
			}
		}
	}

	// 收集注释中的特殊指令 (//export, //go:linkname, //go:embed)
	for _, cg := range node.Comments {
		for _, c := range cg.List {
			text := c.Text
			if strings.HasPrefix(text, "//export ") {
				parts := strings.Fields(text)
				if len(parts) >= 2 {
					o.protectedNames[parts[1]] = true
				}
			} else if strings.HasPrefix(text, "//go:linkname ") {
				parts := strings.Fields(text)
				// 单参形式（`//go:linkname x`，仅为本地符号挂接 asm/cgo）：加保护，
				// 避免混淆破坏链接；双参形式（`//go:linkname x [pkg.]y`）不在此保护，
				// 由 obfuscateFileWithMapping 阶段重写本地名与项目内目标名。
				if len(parts) == 2 {
					o.protectedNames[parts[1]] = true
				}
			} else if strings.HasPrefix(text, "//go:embed ") {
				parts := strings.Fields(text[len("//go:embed "):])
				for _, p := range parts {
					if strings.HasSuffix(p, ".go") && !strings.Contains(p, "*") {
						embedPath := filepath.Join(filepath.Dir(filePath), p)
						o.embedFiles[embedPath] = true
					} else if strings.HasSuffix(p, "*.go") {
						// 记录带有 *.go 通配符的目录，这里用目录名加上 /*.go 作为标记
						dirPath := filepath.Dir(filePath)
						if strings.Contains(p, "/") {
							dirPath = filepath.Join(dirPath, filepath.Dir(p))
						}
						o.embedFiles[filepath.Join(dirPath, "*.go")] = true
					}
				}
			}
		}
	}

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
			// 如果是测试文件，保护 Test/Benchmark/Fuzz/Example 函数
			if isTestFile && x.Recv == nil {
				name := x.Name.Name
				if strings.HasPrefix(name, "Test") ||
					strings.HasPrefix(name, "Benchmark") ||
					strings.HasPrefix(name, "Fuzz") ||
					strings.HasPrefix(name, "Example") {
					o.protectedNames[name] = true
				}
			}
		}
		return true
	})
}

// isProjectImportPath 检查导入路径是否属于项目内部
// （根模块 + 所有嵌套子模块的导入路径均视为项目内部）
func (o *Obfuscator) isProjectImportPath(importPath string) bool {

	// 优先使用 go.mod 中的模块名进行精确匹配（最可靠）
	if o.moduleName != "" {
		if importPath == o.moduleName || strings.HasPrefix(importPath, o.moduleName+"/") {
			return true
		}
		// 嵌套子模块的导入路径同样属于项目内部
		for _, subModuleName := range o.subModuleRoots {
			if importPath == subModuleName || strings.HasPrefix(importPath, subModuleName+"/") {
				return true
			}
		}
		return false
	}

	// 回退：文件夹名启发式判断（go.mod 不存在时）
	if strings.HasPrefix(importPath, "golang.org/x/") ||
		strings.HasPrefix(importPath, "gopkg.in/") {
		return false
	}
	projectName := filepath.Base(o.projectRoot)
	return importPath == projectName ||
		strings.HasPrefix(importPath, projectName+"/") ||
		strings.Contains(importPath, "/"+projectName+"/") ||
		strings.HasSuffix(importPath, "/"+projectName)
}

// readModuleName 从 go.mod 读取模块名并缓存到 o.moduleName
func (o *Obfuscator) readModuleName() error {
	name, err := readModuleNameFromFile(filepath.Join(o.projectRoot, "go.mod"))
	if err != nil {
		return err
	}
	o.moduleName = name
	logger.Infof("读取模块名成功: %s", o.moduleName)
	return nil
}

// discoverSubModuleRoots 扫描项目内所有含独立 go.mod 的嵌套子模块目录，
// 记录 相对路径 -> 模块名，用于向各子模块根分发解密包，保证子模块独立可编译。
func (o *Obfuscator) discoverSubModuleRoots() {
	o.subModuleRoots = make(map[string]string)
	err := filepath.Walk(o.projectRoot, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() || path == o.projectRoot {
			return nil
		}
		if info.Name() == "vendor" || info.Name() == ".git" || info.Name() == "node_modules" {
			return filepath.SkipDir
		}
		goMod := filepath.Join(path, "go.mod")
		if _, statErr := os.Stat(goMod); statErr != nil {
			return nil
		}
		modName, modErr := readModuleNameFromFile(goMod)
		if modErr != nil {
			return nil
		}
		if strings.HasPrefix(modName, o.moduleName+"/") {
			logger.Infof("跳过子模块 %s (模块名 %s 与根模块冲突, 视为根模块一部分)", path, modName)
			return nil
		}
		rel, _ := filepath.Rel(o.projectRoot, path)
		o.subModuleRoots[filepath.ToSlash(rel)] = modName
		logger.Infof("发现嵌套子模块: %s (模块名: %s)", rel, modName)
		return nil
	})
	if err != nil {
		logger.Warnf("扫描嵌套子模块失败: %v", err)
	}
}

// moduleNameForFile 返回文件路径所属模块的模块名。
// 优先匹配最近嵌套子模块根，否则返回根模块名。
// absPath 可为项目根或输出目录下的路径（两者目录结构一致）。
func (o *Obfuscator) moduleNameForFile(absPath string) string {
	if len(o.subModuleRoots) == 0 {
		return o.moduleName
	}
	rel, err := filepath.Rel(o.projectRoot, absPath)
	if err != nil || strings.HasPrefix(rel, "..") {
		rel, err = filepath.Rel(o.outputDir, absPath)
		if err != nil {
			return o.moduleName
		}
	}
	fileDir := filepath.ToSlash(filepath.Dir(rel))
	bestRel := ""
	for subRel := range o.subModuleRoots {
		if (subRel == fileDir || strings.HasPrefix(fileDir, subRel+"/")) && len(subRel) > len(bestRel) {
			bestRel = subRel
		}
	}
	if bestRel == "" {
		return o.moduleName
	}
	return o.subModuleRoots[bestRel]
}

// decryptPkgImportPathForFile 返回文件所属模块内解密包的导入路径。
// 不同模块使用各自模块名拼接，保证独立编译时能被解析。
func (o *Obfuscator) decryptPkgImportPathForFile(absPath string) string {
	return o.moduleNameForFile(absPath) + "/" + o.decryptPkgName
}

// isInsideDecryptPkg 判断相对路径是否位于任意一个实际创建的解密包目录内
// （基于 dropDirs 精确匹配，避免随机包名与用户目录同名时误跳过）
func (o *Obfuscator) isInsideDecryptPkg(relPath string) bool {
	relPath = filepath.ToSlash(relPath)
	dir := filepath.ToSlash(filepath.Dir(relPath))
	if o.decryptPkgDirs[dir] {
		return true
	}
	// 逐层向上：解密包目录下的深层子目录（理论上解密包内部仅单层文件）也应覆盖
	cur := dir
	for {
		if o.decryptPkgDirs[cur] {
			return true
		}
		parent := filepath.ToSlash(filepath.Dir(cur))
		if parent == "." || parent == cur {
			break
		}
		cur = parent
	}
	return false
}

// buildObfuscationMapsWithScope 使用作用域分析构建混淆映射
// 修复版本：为每个对象独立生成混淆名，避免同名冲突
func (o *Obfuscator) buildObfuscationMapsWithScope() {
	// 第一步：收集所有文件中的包级别对象（不分组）
	var allPackageLevelObjects []*Object

	for filePath, analyzer := range o.fileScopes {
		fileScope := analyzer.GetFileScope()
		if fileScope == nil {
			logger.Warnf("文件 %s 没有文件作用域", filePath)
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

		// 方案1：为所有同名对象生成相同的混淆名（支持build-tag场景）
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
			logger.Debugf("同名对象使用相同混淆名: %s -> %s (在 %d 个文件中定义)", name, obfName, len(objects))
		}
	}

	// 同步到funcMapping和varMapping：
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

	logger.Infof("收集到 %d 个包级别名称（函数: %d, 变量: %d）",
		len(nameToObjects), funcCount, varCount)
	logger.Debugf("同步了 %d 个名称到名称映射（用于跨文件引用）", syncCount)
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

			// 检查是否受保护的文件 (main 或被 embed 引用)
			isProtectedFile := o.mainFiles[path] || o.embedFiles[path]
			if !isProtectedFile {
				// 检查通配符 embed
				dirPath := filepath.Dir(path)
				if o.embedFiles[filepath.Join(dirPath, "*.go")] {
					isProtectedFile = true
				}
			}

			if !isSkipped && !isProtectedFile {
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

// copyFile 复制单个文件（保留源文件的权限位）
func (o *Obfuscator) copyFile(src, dst string) error {
	info, err := os.Stat(src)
	if err != nil {
		return err
	}

	srcFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer srcFile.Close()

	if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
		return err
	}

	dstFile, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, info.Mode().Perm())
	if err != nil {
		return err
	}
	defer dstFile.Close()

	_, err = io.Copy(dstFile, srcFile)
	return err
}

// obfuscateOutputFiles 单遍遍历输出目录并应用混淆（不区分平台特定文件）
func (o *Obfuscator) obfuscateOutputFiles(fileMapping map[string]string) error {
	return filepath.Walk(o.outputDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() || !strings.HasSuffix(path, ".go") {
			return nil
		}

		// 跳过解密包自身（精确匹配主模块根与各嵌套子模块根的实际解密包目录）
		if o.Config.EncryptStrings && o.decryptPkgName != "" {
			relPath, _ := filepath.Rel(o.outputDir, path)
			if o.isInsideDecryptPkg(relPath) {
				return nil
			}
		}

		// 通过映射获取原始路径
		originalPath := fileMapping[path]
		if originalPath == "" {
			relPath, _ := filepath.Rel(o.outputDir, path)
			originalPath = filepath.Join(o.projectRoot, relPath)
		}

		if _, skipped := o.skippedFiles[originalPath]; skipped {
			return nil
		}

		if err := o.obfuscateFileWithMapping(path, fileMapping); err != nil {
			return fmt.Errorf("混淆文件 %s 失败: %v", path, err)
		}
		o.obfuscatedGoFiles++

		return nil
	})
}

// obfuscateFileWithMapping 使用文件映射混淆单个文件
func (o *Obfuscator) obfuscateFileWithMapping(filePath string, fileMapping map[string]string) error {
	// 兜底：解密包文件禁止二次混淆，防止 ensureDecryptPackageImport 让解密包导入自己
	// 造成 import cycle，也防止垃圾注入对解密函数注入递归 panic(decrypt(...))。
	if o.Config.EncryptStrings && o.decryptPkgName != "" {
		relPath, _ := filepath.Rel(o.outputDir, filePath)
		if o.isInsideDecryptPkg(relPath) {
			return nil
		}
	}

	// 解析文件
	node, err := parser.ParseFile(o.fset, filePath, nil, parser.ParseComments)
	if err != nil {
		return fmt.Errorf("解析文件失败: %v", err)
	}

	// 移除注释（保留构建标签和编译指令）
	if o.Config.RemoveComments {
		o.removeCommentsFromAST(node)
	}

	// 从文件映射获取原始文件路径
	originalPath, exists := fileMapping[filePath]
	if !exists {
		// 如果映射中没有，尝试从输出目录映射回项目根目录
		relPath, _ := filepath.Rel(o.outputDir, filePath)
		originalPath = filepath.Join(o.projectRoot, relPath)
	}

	// 应用转换（使用作用域信息）
	o.applyTransformationsWithScope(node)

	// 重写 //go:linkname 指令：本地名跟随混淆映射，项目内目标名同步重写
	o.rewriteLinknameDirectivesInFile(node)

	// 格式化并写入
	var buf bytes.Buffer
	if err := format.Node(&buf, o.fset, node); err != nil {
		return fmt.Errorf("格式化失败: %v", err)
	}

	source := buf.String()

	// 字符串加密：按 AST 精确位置加密字符串字面量，有加密时才注入解密包导入
	// （连续等号对齐不受影响；注释、导入路径、结构体标签、const 值不会被触碰）
	if o.Config.EncryptStrings {
		var encryptedCount int
		decryptImportPath := o.decryptPkgImportPathForFile(filePath)
		source, encryptedCount = o.encryptStringsInSource(source, decryptImportPath)
		o.encryptedStringCount += encryptedCount
	}

	// 位置信息混淆：为函数调用点插入 //line 伪文件名，隐藏真实源码位置
	source = o.obfuscatePositions(source)

	// 写回文件（保留原始权限位）
	mode := os.FileMode(0644)
	if originalPath != "" {
		if st, err := os.Stat(originalPath); err == nil {
			mode = st.Mode().Perm()
		}
	}
	return os.WriteFile(filePath, []byte(source), mode)
}

// applyImportAliases 更新导入语句为标准库别名，返回文件中包名 -> 混淆别名的映射
func (o *Obfuscator) applyImportAliases(node *ast.File) map[string]string {
	filePackages := make(map[string]string) // 代码中的包名 -> 混淆别名

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
	return filePackages
}

// applyTransformationsWithScope 使用作用域信息应用 AST 转换
func (o *Obfuscator) applyTransformationsWithScope(node *ast.File) {
	// 步骤 1: 更新导入语句为标准库别名
	filePackages := o.applyImportAliases(node)

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

	// 步骤 3: 重新做当前文件的作用域分析（obfuscateOutputFiles 解析的是输出目录的副本，
	// token.Pos 与 scanProject 阶段的源文件不一致，必须基于当前 node 重建作用域，
	// 才能按位置正确解析标识符所属作用域并处理遮蔽）。
	analyzer := NewScopeAnalyzer(o.fset)
	analyzer.Analyze(node)
	o.obfuscateIdentifiersWithScope(node, analyzer)

	// 步骤 4: 注入垃圾代码（收集全文件已用标识符，保证注入名不冲突）
	if o.Config.InjectJunkCode {
		used := collectUsedNames(node)
		for _, decl := range node.Decls {
			if fn, ok := decl.(*ast.FuncDecl); ok {
				if fn.Body != nil && len(fn.Body.List) > 0 && !o.shouldSkipJunkCodeInjection(fn) {
					anchors := collectJunkAnchors(fn)
					// 50% 概率在函数体内部再插入一段垃圾（增加多样性）。
					// 必须在前置前进行，保证每个垃圾块内部的 goto/label 保持相邻；
					// 若函数体本身含 goto/标签，跳过中插避免破坏原有控制流。
					if !bodyHasGoto(fn.Body) && len(fn.Body.List) >= 3 && MascotRandInt(2) == 0 {
						pos := 1 + MascotRandInt(2)
						mid := o.generateJunkStatementsWithAnchors(used, anchors)
						prefix := append([]ast.Stmt(nil), fn.Body.List[:pos]...)
						rest := append([]ast.Stmt(nil), fn.Body.List[pos:]...)
						fn.Body.List = append(prefix, append(mid, rest...)...)
					}

					junkStmts := o.generateJunkStatementsWithAnchors(used, anchors)
					fn.Body.List = append(junkStmts, fn.Body.List...)
				}
			}
		}
	}
}

// bodyHasGoto 判断函数体是否包含 goto 语句或标签（避免插入垃圾破坏既有控制流）
func bodyHasGoto(body *ast.BlockStmt) bool {
	has := false
	ast.Inspect(body, func(n ast.Node) bool {
		switch x := n.(type) {
		case *ast.BranchStmt:
			if x.Tok == token.GOTO {
				has = true
				return false
			}
		case *ast.LabeledStmt:
			has = true
			return false
		}
		return true
	})
	return has
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
		case *ast.CompositeLit:
			// 复合字面量：T{...} 中的 T 是类型引用（可能与局部变量同名，如 `x := x{}`）
			if x.Type != nil {
				o.markTypeIdents(x.Type, typeRefs)
			}
		}
		return true
	})

	// 第二遍：替换标识符
	// 辨析：混淆阶段的 ResolveIdent 使用的是输出副本上重建的作用域分析器，
	// 其对象指针与 scanProject 阶段预构建的 objectMapping 不同，因此这里：
	//   - 包级对象（analysis-time 找到且属于 fileScope）按名字走 funcMapping/varMapping；
	//   - 局部对象惰性生成混淆名并缓存在 localNames（同文件一致、正确处理遮蔽）。
	localNames := make(map[*Object]string)
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

			obj := analyzer.ResolveIdent(x.Pos(), x.Name)
			if obj != nil {
				// 跳过类型对象
				if obj.Kind == ObjType {
					return true
				}

				// 包级对象：按名字映射（跨文件一致，支持 build-tag 同名场景）
				if obj.Scope == analyzer.GetFileScope() {
					if obfName, exists := o.funcMapping[x.Name]; exists {
						x.Name = obfName
						return true
					}
					if obfName, exists := o.varMapping[x.Name]; exists {
						x.Name = obfName
						return true
					}
					return true
				}

				// 局部对象：同一对象缓存同一个混淆名（同一文件内引用一致）
				if obfName, hasObf := localNames[obj]; hasObf {
					x.Name = obfName
					return true
				}
				obfName := o.generateUniqueObfuscatedNameForObject(obj)
				localNames[obj] = obfName
				x.Name = obfName
				return true
			}

			// 找不到对象（如跨文件引用的包级函数）：使用名称映射兜底
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

// shouldKeepComment 判断是否应保留注释
func (o *Obfuscator) shouldKeepComment(text string) bool {
	// 保留构建标签和编译指令，以及 CGO 和导出等重要标记
	// 注意：对于多行纯 C 语言块（可能不包含 #cgo 或 #include），
	// 这里放宽了限制，只要不是简单的中文/英文说明注释就尽量保留（避免误删 C 代码）。
	// 目前先基于已知特征进行保留。
	if strings.HasPrefix(text, "//go:") ||
		strings.HasPrefix(text, "// +build") ||
		strings.HasPrefix(text, "//+build") ||
		strings.HasPrefix(text, "//export ") ||
		strings.HasPrefix(text, "//sys ") ||
		strings.HasPrefix(text, "//line ") ||
		strings.Contains(text, "#cgo ") ||
		strings.Contains(text, "#include ") {
		return true
	}

	// 多行纯 C 代码块（cgo 前奏，常见于 import "C" 之前）：
	// 保留含预处理指令行（#define/#ifdef/#pragma/#include/#cgo 等）或以分号/花括号
	// 结尾的 C 代码，避免误删真正参与编译的宏/声明。
	if strings.HasPrefix(text, "/*") {
		for _, line := range strings.Split(text, "\n") {
			if trimmed := strings.TrimSpace(line); strings.HasPrefix(trimmed, "#") {
				return true
			}
		}
		if strings.Contains(text, ";") || strings.Contains(text, "{") || strings.Contains(text, "}") {
			return true
		}
	}

	return false
}

// filterComments 过滤注释组，仅保留必要的编译指令类注释
func (o *Obfuscator) filterComments(groups []*ast.CommentGroup) []*ast.CommentGroup {
	var result []*ast.CommentGroup
	for _, cg := range groups {
		var keep []*ast.Comment
		for _, c := range cg.List {
			if o.shouldKeepComment(c.Text) {
				keep = append(keep, c)
			}
		}
		if len(keep) > 0 {
			cg.List = keep
			result = append(result, cg)
		}
	}
	return result
}

// removeCommentsFromAST 移除 AST 中所有被过滤的注释
//
// 注意：go/format 打印时会同时读取 file.Comments 与节点上的 Doc/Comment 字段，
// 仅清空 file.Comments 无法删除挂在节点上的注释（如类型/函数的 Doc 注释、结构体字段的尾注释），
// 因此这里对节点上的注释组一并清理。
func (o *Obfuscator) removeCommentsFromAST(node *ast.File) {
	kept := o.filterComments(node.Comments)
	keepSet := make(map[*ast.CommentGroup]bool, len(kept))
	for _, cg := range kept {
		keepSet[cg] = true
	}

	clearGroup := func(cg *ast.CommentGroup) bool {
		return cg != nil && !keepSet[cg]
	}

	ast.Inspect(node, func(n ast.Node) bool {
		switch x := n.(type) {
		case *ast.FuncDecl:
			if clearGroup(x.Doc) {
				x.Doc = nil
			}
		case *ast.GenDecl:
			if clearGroup(x.Doc) {
				x.Doc = nil
			}
		case *ast.Field:
			if clearGroup(x.Doc) {
				x.Doc = nil
			}
			if clearGroup(x.Comment) {
				x.Comment = nil
			}
		case *ast.ImportSpec:
			if clearGroup(x.Doc) {
				x.Doc = nil
			}
			if clearGroup(x.Comment) {
				x.Comment = nil
			}
		case *ast.ValueSpec:
			if clearGroup(x.Doc) {
				x.Doc = nil
			}
			if clearGroup(x.Comment) {
				x.Comment = nil
			}
		case *ast.TypeSpec:
			if clearGroup(x.Doc) {
				x.Doc = nil
			}
			if clearGroup(x.Comment) {
				x.Comment = nil
			}
		}
		return true
	})

	node.Comments = kept
}

// ensureDecryptPackageImport 确保源代码中导入了指定路径的解密包
func (o *Obfuscator) ensureDecryptPackageImport(source string, decryptPkgPath string) string {
	importLine := fmt.Sprintf(`%s "%s"`, o.decryptPkgName, decryptPkgPath)

	// 检查是否已经导入
	if strings.Contains(source, decryptPkgPath) {
		return source
	}

	lines := strings.Split(source, "\n")
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "import (") {
			// 多行 import 块
			rest := strings.TrimSuffix(strings.TrimPrefix(trimmed, "import ("), ")")
			rest = strings.TrimSpace(rest)
			if rest != "" {
				// 单行块形式 import ("fmt")：展开为多行，避免新导入落在闭合括号外
				lines[i] = "import (\n\t" + importLine + "\n\t" + rest + "\n)"
			} else {
				// 常规多行块：在 "import (" 后追加
				lines[i] = line + "\n\t" + importLine
			}
			return strings.Join(lines, "\n")
		}
		if strings.HasPrefix(trimmed, "import ") {
			// 单行 import（可能带缩进），转换为块；用 trimmed 提取路径，
			// 避免原 line 带前导空白时 TrimPrefix 失效生成嵌套非法 import。
			path := strings.TrimSpace(strings.TrimPrefix(trimmed, "import "))
			lines[i] = "import (\n\t" + importLine + "\n\t" + path + "\n)"
			return strings.Join(lines, "\n")
		}
	}

	// 没有找到 import，在 package 后添加
	for i, line := range lines {
		if strings.HasPrefix(strings.TrimSpace(line), "package ") {
			lines = append(lines[:i+1], append([]string{"\nimport " + importLine + "\n"}, lines[i+1:]...)...)
			return strings.Join(lines, "\n")
		}
	}

	return source
}

// encryptStringsInSource 在源代码中加密字符串字面量（使用解密包的函数），返回修改后的源码和加密数量。
// 基于 AST 精确定位字符串，天然排除注释、导入路径、结构体标签与 const 常量的字符串，
// 不会破坏注释内容，也没有逐行解析时的状态泄漏问题（如反引号跨越、注释内引号等）。
func (o *Obfuscator) encryptStringsInSource(source string, decryptPkgPath string) (string, int) {
	fset := token.NewFileSet()
	node, err := parser.ParseFile(fset, "", source, parser.ParseComments)
	if err != nil {
		logger.Warnf("字符串加密: 解析源码失败，跳过: %v", err)
		return source, 0
	}

	// 收集需要跳过的字符串：导入路径、结构体标签、常量上下文中的字符串
	// （const 初始化表达式、数组长度表达式），这些位置必须是常量表达式，
	// 不能被解密函数调用替换。
	skipLiterals := make(map[*ast.BasicLit]bool)
	for _, imp := range node.Imports {
		if imp.Path != nil {
			skipLiterals[imp.Path] = true
		}
	}

	// markSubtreeStrings 将表达式子树内所有字符串字面量标记为跳过
	markSubtreeStrings := func(expr ast.Expr) {
		ast.Inspect(expr, func(n ast.Node) bool {
			if lit, ok := n.(*ast.BasicLit); ok && lit.Kind == token.STRING {
				skipLiterals[lit] = true
			}
			return true
		})
	}

	ast.Inspect(node, func(n ast.Node) bool {
		switch x := n.(type) {
		case *ast.Field:
			if x.Tag != nil {
				skipLiterals[x.Tag] = true
			}
		case *ast.GenDecl:
			// const 的值必须是常量表达式，递归标记整个初始化表达式中的字符串
			if x.Tok == token.CONST {
				for _, spec := range x.Specs {
					if vs, ok := spec.(*ast.ValueSpec); ok {
						for _, val := range vs.Values {
							markSubtreeStrings(val)
						}
					}
				}
				return true
			}
		case *ast.ArrayType:
			// 数组长度必须是常量表达式
			if x.Len != nil {
				markSubtreeStrings(x.Len)
			}
		}
		return true
	})

	// 收集可加密的字符串及其字节区间
	type replacement struct {
		start, end int
		text       string
	}
	var repls []replacement

	ast.Inspect(node, func(n ast.Node) bool {
		lit, ok := n.(*ast.BasicLit)
		if !ok {
			return true
		}
		if skipLiterals[lit] {
			return true
		}
		// 仅双引号字符串；跳过原始字符串（反引号）与含转义的字符串
		if lit.Kind != token.STRING || len(lit.Value) < 3 || lit.Value[0] != '"' {
			return true
		}
		content, err := strconv.Unquote(lit.Value)
		if err != nil || len(content) < 3 || strings.Contains(content, "\\") {
			return true
		}

		// 随机选择解密策略（与生成包中的解密函数一一对应）
		strategy := decryptStrategy(MascotRandInt(numDecryptStrategies))
		decryptName := o.decryptFuncNames[strategy]

		pos := fset.Position(lit.Pos())
		end := fset.Position(lit.End())
		encryptedText := o.encryptStringWithStrategy(content, strategy)
		repls = append(repls, replacement{
			start: pos.Offset,
			end:   end.Offset,
			text:  fmt.Sprintf(`%s.%s("%s")`, o.decryptPkgName, decryptName, encryptedText),
		})
		return true
	})

	if len(repls) == 0 {
		return source, 0
	}

	// 按原始字节偏移拼接替换（替换文本比字面量更长也安全）
	var sb strings.Builder
	last := 0
	for _, r := range repls {
		sb.WriteString(source[last:r.start])
		sb.WriteString(r.text)
		last = r.end
	}
	sb.WriteString(source[last:])

	newSource := sb.String()
	newSource = o.ensureDecryptPackageImport(newSource, decryptPkgPath)
	// 重新格式化，修复文本级导入注入导致的缩进问题
	if formatted, err := format.Source([]byte(newSource)); err == nil {
		newSource = string(formatted)
	}
	return newSource, len(repls)
}

// createDecryptPackage 创建独立的解密包
func (o *Obfuscator) createDecryptPackage() error {
	if o.decryptPkgCreated {
		return nil
	}

	// 设置解密包路径（在输出目录下，而不是原始项目目录）：
	// 主模块根 + 每个含独立 go.mod 的嵌套子模块根各放一份，保证子模块可独立编译。
	pkgRelDirs := []string{""}
	for subRel := range o.subModuleRoots {
		pkgRelDirs = append(pkgRelDirs, subRel)
	}

	// 生成解密包的内容（每种策略对应一个独立函数）
	keyBytes := []byte(o.encryptionKey)
	keyLiteral := "[]byte{"
	for i, b := range keyBytes {
		if i > 0 {
			keyLiteral += ", "
		}
		keyLiteral += fmt.Sprintf("%d", b)
	}
	keyLiteral += "}"

	decryptImpl := map[decryptStrategy]string{
		strategyXOR: `
	d, e := base64.StdEncoding.DecodeString(s)
	if e != nil {
		return ""
	}
	k := %s
	r := make([]byte, len(d))
	for i, b := range d {
		r[i] = b ^ k[i%%len(k)]
	}
	return string(r)
`,
		strategyXORAdd: `
	d, e := base64.StdEncoding.DecodeString(s)
	if e != nil {
		return ""
	}
	k := %s
	r := make([]byte, len(d))
	for i, b := range d {
		r[i] = (b - byte(i&0xff)) ^ k[i%%len(k)]
	}
	return string(r)
`,
		strategyXORRot: `
	d, e := base64.StdEncoding.DecodeString(s)
	if e != nil {
		return ""
	}
	k := %s
	r := make([]byte, len(d))
	for i, b := range d {
		r[i] = func() byte {
			v := b ^ k[i%%len(k)]
			shift := uint(k[i%%len(k)]) %% 8
			return (v >> shift) | (v << (8 - shift))
		}()
	}
	return string(r)
`,
	}

	var funcs strings.Builder
	for i := 0; i < numDecryptStrategies; i++ {
		fmt.Fprintf(&funcs, "func %s(s string) string {\n%s}\n\n",
			o.decryptFuncNames[i],
			fmt.Sprintf(decryptImpl[decryptStrategy(i)], keyLiteral))
	}

	// 创建解密包文件（不包含任何暴露用途的注释）
	decryptFileContent := fmt.Sprintf(`package %s

import "encoding/base64"

%s`, o.decryptPkgName, funcs.String())

	// 写入文件（使用随机文件名，各模块根使用同一文件名）
	randomFileName := generateRandomString(10) + ".go"
	o.decryptPkgDirs = make(map[string]bool, len(pkgRelDirs))
	for _, pkgRelDir := range pkgRelDirs {
		decryptPkgDir := filepath.Join(o.outputDir, pkgRelDir, o.decryptPkgName)
		if err := os.MkdirAll(decryptPkgDir, 0755); err != nil {
			return fmt.Errorf("创建解密包目录失败: %v", err)
		}
		decryptFilePath := filepath.Join(decryptPkgDir, randomFileName)
		if err := os.WriteFile(decryptFilePath, []byte(decryptFileContent), 0644); err != nil {
			return fmt.Errorf("写入解密文件失败: %v", err)
		}
		relDir, _ := filepath.Rel(o.outputDir, decryptPkgDir)
		o.decryptPkgDirs[filepath.ToSlash(relDir)] = true
	}

	// 读取 go.mod 获取模块名（从原始项目目录读取）
	moduleName, err := readModuleNameFromFile(filepath.Join(o.projectRoot, "go.mod"))
	if err != nil {
		return fmt.Errorf("读取go.mod失败: %v", err)
	}

	// 设置解密包的导入路径
	o.decryptPkgPath = moduleName + "/" + o.decryptPkgName
	o.decryptPkgCreated = true

	// 保护解密函数名称和包名，防止被混淆
	for _, name := range o.decryptFuncNames {
		o.protectedNames[name] = true
	}
	o.packageNames[o.decryptPkgName] = true

	logger.Infof("创建解密包: %d 处 (导入路径: %s, 函数名: %v)", len(pkgRelDirs), o.decryptPkgPath, o.decryptFuncNames)
	return nil
}
