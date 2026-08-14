package obfuscator

import (
	"go/ast"
	"go/token"
	"path/filepath"
	"strings"
)

// 反射入口点（参考 garble cache_pkg.go 的 ReflectAPIs 种子）：
// reflect.TypeOf / reflect.ValueOf 的参数 0 是反射的根源；
// 凡进入这些函数的参数，其静态类型都属于『被反射引用』。
var reflectEntryFns = map[string]bool{
	"TypeOf":    true,
	"ValueOf":   true,
	"TypeFor":   true,
	"New":       true,
	"PtrTo":     true,
	"SliceOf":   true,
	"MapOf":     true,
	"ChanOf":    true,
	"ArrayOf":   true,
	"MakeSlice": true,
	"MakeChan":  true,
	"MakeMap":   true,
	"MakeFunc":  true,
}

// 编码入口点：encoding/json、encoding/xml、yaml 通过 reflect / 字段名
// 读取结构体字段，其参数同样视为『被反射引用』。
// 注意：NewEncoder/NewDecoder 只是创建编码器，其参数不是被序列化的数据，
// 不应作为数据入口（否则会误触发回退整文件保护）。
var jsonEntryFns = map[string]bool{
	"Marshal":       true,
	"MarshalIndent": true,
	"Unmarshal":     true,
	"Encode":        true,
	"Decode":        true,
}

// detectReflectionUsage 检查文件是否使用反射
func (o *Obfuscator) detectReflectionUsage(node *ast.File) bool {
	return hasImportPath(node, "reflect")
}

// detectJSONUsage 检查文件是否使用 JSON 编码/解码
func (o *Obfuscator) detectJSONUsage(node *ast.File) bool {
	return hasImportPathContains(node, []string{"encoding/json", "encoding/xml", "gopkg.in/yaml"})
}

// hasImportPath 判断文件是否导入指定的单一路径。
func hasImportPath(node *ast.File, target string) bool {
	for _, imp := range node.Imports {
		if imp.Path == nil {
			continue
		}
		path := strings.Trim(imp.Path.Value, `"`)
		if path == target {
			return true
		}
	}
	return false
}

// hasImportPathContains 判断文件是否导入了包含任一关键字的路径。
func hasImportPathContains(node *ast.File, targets []string) bool {
	for _, imp := range node.Imports {
		if imp.Path == nil {
			continue
		}
		path := strings.Trim(imp.Path.Value, `"`)
		for _, t := range targets {
			if path == t || strings.HasPrefix(path, t+"/") {
				return true
			}
		}
	}
	return false
}

// collectReflectionUsage 收集单个文件的反射/JSON 精确目标（garble 参考实现）。
//
// 思路与 garble reflect.go 一致：
//  1. 找到反射入口点（reflect.TypeOf/ValueOf/... 以及 json/xml/yaml 编码函数）的调用；
//  2. 对每个入口点的值参数，尝试解析其静态类型（复合字面量、类型断言、new(T)、
//     标识符声明类型/初始化表达式等）；
//  3. 命中的类型名写入 o.reflectionTargetTypes（按名全局生效，天然支持跨文件）；
//  4. 若参数是接口/外部函数调用等无法静态解析的动态用法，就地回退到整文件
//     保护（只是不能精确，不会漏保护）。
func (o *Obfuscator) collectReflectionUsage(node *ast.File, analyzer *ScopeAnalyzer) {
	if !o.Config.PreserveReflection {
		return
	}

	// 构建导入路径与项目包的判定上下文
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

	unresolved := false

	ast.Inspect(node, func(n ast.Node) bool {
		ce, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}

		// 支持两类调用形式：
		//   1. reflect.TypeOf(x)             -> Fun 为 SelectorExpr
		//   2. reflect.TypeFor[User]()       -> Fun 为 IndexExpr { SelectorExpr, 类型参数 }
		fn := ce.Fun
		var typeArgs []ast.Expr
		if idx, ok := fn.(*ast.IndexExpr); ok {
			typeArgs = []ast.Expr{idx.Index}
			fn = idx.X
		} else if il, ok := fn.(*ast.IndexListExpr); ok {
			typeArgs = il.Indices
			fn = il.X
		}

		sel, ok := fn.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		selName := sel.Sel.Name

		// 反射入口点：reflect.TypeOf(x) 这类包级函数调用
		if ident, isPkg := sel.X.(*ast.Ident); isPkg {
			pkgPath, isImport := importPaths[ident.Name]
			if isImport && pkgPath == "reflect" && reflectEntryFns[selName] {
				// 泛型形式 TypeFor[T]()：无数据参数，只有类型参数
				if len(typeArgs) > 0 {
					for _, ta := range typeArgs {
						names := make(map[string]bool)
						o.collectTypeNamesFromTypeExpr(ta, names)
						if len(names) == 0 {
							unresolved = true
							continue
						}
						for n := range names {
							o.reflectionTargetTypes[n] = true
						}
					}
					return true
				}
				if len(ce.Args) == 0 {
					return true
				}
				if !o.resolveArgToNamedTypes(ce.Args[0], analyzer, importPaths, &unresolved) {
					unresolved = true
				}
				return true
			}
		}

		// 编码入口点：json.Marshal/xml.Marshal/... 以及 Encoder.Encode(v) 这类方法调用。
		// 方法调用（Encode/Decode）的接收者来自 json.NewEncoder 等，接收者类型不固定，
		// 但只要文件导入了编码包且调用名为 Encode/Decode，其参数同样是反射目标。
		if o.detectJSONUsage(node) && jsonEntryFns[selName] {
			argIdx := 0
			if selName == "Unmarshal" {
				argIdx = 1 // &dst 是指针，仍按其指向的类型解析
			}
			if len(ce.Args) > argIdx {
				if !o.resolveArgToNamedTypes(ce.Args[argIdx], analyzer, importPaths, &unresolved) {
					unresolved = true
				}
			}
			return true
		}

		return true
	})

	if unresolved {
		// 无法精确解析：就地回退整文件保护（有 node，可直接复用旧逻辑）
		o.applyReflectionFallback(node)
	}
}

// resolveArgToNamedTypes 解析反射/编码调用的参数表达式，尝试得到其命名的类型。
// 返回值 false 表示参数是动态用法（接口、外部函数调用、无法定位声明等），
// 无法确知具体类型，调用方应将所在文件标记为回退整文件保护。
func (o *Obfuscator) resolveArgToNamedTypes(expr ast.Expr, analyzer *ScopeAnalyzer, importPaths map[string]string, unresolved *bool) bool {
	names := make(map[string]bool)
	ok := o.collectTypeNamesFromExpr(expr, analyzer, importPaths, names)
	if !ok {
		return false
	}
	if len(names) == 0 {
		// 解析成功但没有任何命名类型（例如空接口、匿名结构体、预声明基本类型），
		// 保守处理：无法确知被反射的具体类型，交由调用方决定是否回退。
		*unresolved = true
		return true
	}
	for name := range names {
		o.reflectionTargetTypes[name] = true
	}
	return true
}

// collectTypeNamesFromExpr 从值表达式/类型表达式中提取候选的命名类型名。
// 参考 garble reflect.go 的 recordArgReflected + recursivelyRecordUsedForReflect：
// 解析 &x、*x、T{...}、[]T、new(T)、T(x)、x。(T) 以及标识符的声明类型。
// 返回 false 表示无法静态解析（应回退）。
func (o *Obfuscator) collectTypeNamesFromExpr(expr ast.Expr, analyzer *ScopeAnalyzer, importPaths map[string]string, out map[string]bool) bool {
	if expr == nil {
		return false
	}

	switch e := expr.(type) {
	case *ast.ParenExpr:
		return o.collectTypeNamesFromExpr(e.X, analyzer, importPaths, out)

	case *ast.UnaryExpr:
		if e.Op == token.AND {
			return o.collectTypeNamesFromExpr(e.X, analyzer, importPaths, out)
		}
		return false

	case *ast.StarExpr:
		// &x / *x 以及指针类型 *T：直接深入
		return o.collectTypeNamesFromExpr(e.X, analyzer, importPaths, out)

	case *ast.CompositeLit:
		return o.collectTypeNamesFromTypeExpr(e.Type, out)

	case *ast.TypeAssertExpr:
		return o.collectTypeNamesFromTypeExpr(e.Type, out)

	case *ast.ArrayType:
		// 数组与切片统一用 ArrayType（Len==nil 表示切片）
		return o.collectTypeNamesFromTypeExpr(e.Elt, out)

	case *ast.MapType:
		okV := o.collectTypeNamesFromTypeExpr(e.Value, out)
		okK := o.collectTypeNamesFromTypeExpr(e.Key, out)
		return okV || okK

	case *ast.ChanType:
		return o.collectTypeNamesFromTypeExpr(e.Value, out)

	case *ast.Ident:
		// 表达式中出现的标识符优先按变量/常量/参数解析（可能遮蔽类型名）。
		if obj := analyzer.ResolveIdent(e.Pos(), e.Name); obj != nil {
			if obj.Kind == ObjType {
				// 类型本身（如 (T)(nil) 转换中的 T）
				out[e.Name] = true
				return true
			}
			if obj.Kind == ObjVar || obj.Kind == ObjConst || obj.Kind == ObjField || obj.Kind == ObjMethod {
				return o.collectNamesFromIdentDecl(e, analyzer, importPaths, out)
			}
			return false
		}
		// 无法解析（可能是包级变量声明在其它文件，或预声明名）。
		// 不确定是类型还是值，保守返回 false 交由调用方决定是否回退。
		return false

	case *ast.SelectorExpr:
		// pkg.Type -> 若为项目包则记录 Sel（跨文件按名保护）
		if pkgIdent, ok := e.X.(*ast.Ident); ok {
			if pkgPath, isImport := importPaths[pkgIdent.Name]; isImport {
				if o.isProjectImportPathResolved(pkgPath) && o.collectTypeNamesFromTypeExpr(e, out) {
					return true
				}
			}
		}
		// 若 X 是本作用域变量/参数（字段访问 x.Field -> 无法静态得知所属
		// 结构体类型），保守回退；包级引用已在上面处理。
		return false

	case *ast.CallExpr:
		// new(T) — 参数是类型表达式
		if ident, ok := e.Fun.(*ast.Ident); ok && ident.Name == "new" {
			if len(e.Args) == 1 {
				return o.collectTypeNamesFromTypeExpr(e.Args[0], out)
			}
			return false
		}

		// 嵌套反射调用：reflect.New(reflect.TypeOf(User{})) / reflect.ValueOf(...).Call(...)
		// 等，参数本身又是一个反射入口点 -> 深入其数据参数。
		if sel, ok := e.Fun.(*ast.SelectorExpr); ok {
			if pkgIdent, isPkg := sel.X.(*ast.Ident); isPkg {
				if pkgPath, isImport := importPaths[pkgIdent.Name]; isImport && pkgPath == "reflect" && reflectEntryFns[sel.Sel.Name] {
					if len(e.Args) > 0 {
						return o.collectTypeNamesFromExpr(e.Args[0], analyzer, importPaths, out)
					}
					return false
				}
			}
		}

		// 转换表达式 T(x)：Fun 是类型表达式（非包函数调用）-> 记录类型
		if o.collectTypeNamesFromTypeExpr(e.Fun, out) {
			return true
		}
		// 一般函数调用 -> 无法静态解析返回类型
		return false

	case *ast.IndexExpr:
		// 泛型类型参数：reflect.TypeFor[User]() 中的 User
		return o.collectTypeNamesFromTypeExpr(e.Index, out)
	}

	return false
}

// collectTypeNamesFromTypeExpr 从类型表达式中提取所有命名类型名。
// 与 markTypeIdents 类似，但只关心命名类型（Ident / SelectorExpr.Sel）。
func (o *Obfuscator) collectTypeNamesFromTypeExpr(expr ast.Expr, out map[string]bool) bool {
	if expr == nil {
		return false
	}
	found := false
	ast.Inspect(expr, func(n ast.Node) bool {
		switch node := n.(type) {
		case *ast.Ident:
			// 只记录命名类型（跳过内置类型在 collectNamesFromIdentDecl 处理）
			if isLikelyTypeIdent(node.Name) {
				out[node.Name] = true
				found = true
			}
		case *ast.SelectorExpr:
			out[node.Sel.Name] = true
			found = true
			return false // 不深入 X（包名已被收走）
		}
		return true
	})
	return found
}

// isLikelyTypeIdent 判断可能是命名类型的标识符（排除内置类型名）。
func isLikelyTypeIdent(name string) bool {
	switch name {
	case "bool", "string", "int", "int8", "int16", "int32", "int64",
		"uint", "uint8", "uint16", "uint32", "uint64", "uintptr",
		"float32", "float64", "complex64", "complex128", "byte", "rune",
		"error":
		return false
	}
	return name != ""
}

// collectNamesFromIdentDecl 解析标识符（变量/常量/参数/接收者）的声明，
// 从声明类型或初始化表达式中收集命名类型名。
func (o *Obfuscator) collectNamesFromIdentDecl(ident *ast.Ident, analyzer *ScopeAnalyzer, importPaths map[string]string, out map[string]bool) bool {
	if analyzer == nil {
		return false
	}
	obj := analyzer.ResolveIdent(ident.Pos(), ident.Name)
	if obj == nil {
		return false
	}

	switch d := obj.Decl.(type) {
	case *ast.ValueSpec:
		if d.Type != nil {
			return o.collectTypeNamesFromTypeExpr(d.Type, out)
		}
		// 无显式类型：从初始化表达式中推断（如 x := User{}）
		for _, v := range d.Values {
			names := make(map[string]bool)
			if o.collectTypeNamesFromExpr(v, analyzer, importPaths, names) && len(names) > 0 {
				for n := range names {
					out[n] = true
				}
				return true
			}
		}
		return false

	case *ast.Field:
		// 参数 / 返回值 / 接收者 / 局部变量（range 声明的字段类型）
		if d.Type != nil {
			return o.collectTypeNamesFromTypeExpr(d.Type, out)
		}
		return false

	case *ast.AssignStmt:
		// := 短声明：从同一赋值语句 RHS 推断
		for i, lhs := range d.Lhs {
			ld, ok := lhs.(*ast.Ident)
			if !ok || ld.Name != ident.Name {
				continue
			}
			if i < len(d.Rhs) {
				return o.collectTypeNamesFromExpr(d.Rhs[i], analyzer, importPaths, out)
			}
		}
		return false

	case *ast.RangeStmt:
		// for k, v := range T{...} / []T{...}：值类型取元素类型
		names := make(map[string]bool)
		if o.collectTypeNamesFromExpr(d.X, analyzer, importPaths, names) {
			for n := range names {
				out[n] = true
			}
			return true
		}
		return false
	}

	return false
}

// isProjectImportPathResolved 使用已解析过的模块名判断导入路径是否属于项目内部。
func (o *Obfuscator) isProjectImportPathResolved(importPath string) bool {
	if o.moduleName != "" {
		return importPath == o.moduleName || strings.HasPrefix(importPath, o.moduleName+"/")
	}
	return o.isProjectImportPath(importPath)
}

// protectReflectionTypes 应用反射精确保护：对反射目标类型进行交叉引用展开。
//
// 参考 garble reflect.go 的 recursivelyRecordUsedForReflect：
// 命中的命名类型不但保护其类型名，还要递归保护其结构体字段（含匿名字段/嵌入类型）、
// 以及字段指向的命名类型，保证 json/reflect 在运行时能读到真实字段名。
func (o *Obfuscator) protectReflectionTypes() {
	if !o.Config.PreserveReflection {
		return
	}

	// 收集已在扫描阶段命中（被反射引用）的类型名
	worklist := make([]string, 0, len(o.reflectionTargetTypes))
	for name := range o.reflectionTargetTypes {
		worklist = append(worklist, name)
	}

	done := make(map[string]bool)
	for len(worklist) > 0 {
		name := worklist[len(worklist)-1]
		worklist = worklist[:len(worklist)-1]
		if done[name] {
			continue
		}
		done[name] = true

		o.protectedNames[name] = true

		ts := o.findTypeSpecByName(name)
		if ts == nil {
			continue
		}
		if st, ok := ts.Type.(*ast.StructType); ok {
			worklist = append(worklist, collectStructFieldTypes(st, done)...)
		}
	}
}

// collectStructFieldTypes 遍历结构体字段，返回字段指向的命名类型列表，
// 由调用方加入待展开队列（slice 值传递无法原地追加，改为返回值）。
func collectStructFieldTypes(st *ast.StructType, done map[string]bool) []string {
	if st.Fields == nil {
		return nil
	}
	var worklist []string
	for _, field := range st.Fields.List {
		var names []string
		ast.Inspect(field.Type, func(n ast.Node) bool {
			switch node := n.(type) {
			case *ast.Ident:
				if isLikelyTypeIdent(node.Name) {
					names = append(names, node.Name)
				}
			case *ast.SelectorExpr:
				names = append(names, node.Sel.Name)
			}
			return true
		})
		for _, tn := range names {
			if !done[tn] {
				worklist = append(worklist, tn)
			}
		}
	}
	return worklist
}

// findTypeSpecByName 在已扫描文件中查找指定名字的 TypeSpec（跨文件）。
func (o *Obfuscator) findTypeSpecByName(name string) *ast.TypeSpec {
	for filePath, sa := range o.fileScopes {
		_ = filePath
		if obj, ok := sa.GetFileScope().Objects[name]; ok && obj.Kind == ObjType {
			if ts, ok := obj.Decl.(*ast.TypeSpec); ok {
				return ts
			}
		}
	}
	return nil
}

// applyReflectionFallback 对无法静态解析反射使用方式的文件执行整文件保护
// （兼容旧行为，只是无法精确，不能漏保护）。
func (o *Obfuscator) applyReflectionFallback(node *ast.File) {
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
							o.protectedNames[fieldName.Name] = true
						}
						// 匿名字段：保护其类型名
						if len(field.Names) == 0 {
							names := make(map[string]bool)
							o.collectTypeNamesFromTypeExpr(field.Type, names)
							for n := range names {
								o.protectedNames[n] = true
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
}
