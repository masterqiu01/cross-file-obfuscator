package obfuscator

import (
	"go/ast"
	"go/token"
)

// Scope 表示一个作用域
type Scope struct {
	Parent   *Scope                // 父作用域
	Children []*Scope              // 子作用域
	Objects  map[string]*Object    // 该作用域中定义的对象
	Node     ast.Node              // 关联的AST节点
	Start    token.Pos             // 作用域起始位置
	End      token.Pos             // 作用域结束位置
}

// Object 表示一个声明的对象（变量、函数等）
type Object struct {
	Name         string      // 原始名称
	ObfuscatedName string    // 混淆后的名称
	Kind         ObjectKind  // 对象类型
	Decl         ast.Node    // 声明节点
	Scope        *Scope      // 所属作用域
	IsExported   bool        // 是否导出
	Pos          token.Pos   // 声明位置
	FilePath     string      // 对象所在的文件路径（用于生成唯一标识）
}

// ObjectKind 对象类型
type ObjectKind int

const (
	ObjUnknown ObjectKind = iota
	ObjPackage            // 包名
	ObjConst              // 常量
	ObjVar                // 变量
	ObjFunc               // 函数
	ObjType               // 类型
	ObjLabel              // 标签
	ObjField              // 结构体字段
	ObjMethod             // 方法
)

// ScopeAnalyzer 作用域分析器
type ScopeAnalyzer struct {
	fset         *token.FileSet
	currentScope *Scope
	fileScope    *Scope
	packageScope *Scope
	scopes       map[ast.Node]*Scope // AST节点到作用域的映射
	objects      map[token.Pos]*Object // 位置到对象的映射
}

// NewScopeAnalyzer 创建新的作用域分析器
func NewScopeAnalyzer(fset *token.FileSet) *ScopeAnalyzer {
	packageScope := &Scope{
		Objects: make(map[string]*Object),
	}
	
	return &ScopeAnalyzer{
		fset:         fset,
		packageScope: packageScope,
		currentScope: packageScope,
		scopes:       make(map[ast.Node]*Scope),
		objects:      make(map[token.Pos]*Object),
	}
}

// Analyze 分析文件的作用域
func (sa *ScopeAnalyzer) Analyze(file *ast.File) {
	sa.fileScope = sa.newScope(file)
	sa.currentScope = sa.fileScope
	
	// 遍历所有声明
	for _, decl := range file.Decls {
		sa.analyzeDecl(decl)
	}
}

// newScope 创建新的作用域
func (sa *ScopeAnalyzer) newScope(node ast.Node) *Scope {
	scope := &Scope{
		Parent:   sa.currentScope,
		Objects:  make(map[string]*Object),
		Node:     node,
	}
	
	if node != nil {
		scope.Start = node.Pos()
		scope.End = node.End()
	}
	
	if sa.currentScope != nil {
		sa.currentScope.Children = append(sa.currentScope.Children, scope)
	}
	
	sa.scopes[node] = scope
	return scope
}

// enterScope 进入新作用域
func (sa *ScopeAnalyzer) enterScope(node ast.Node) *Scope {
	scope := sa.newScope(node)
	sa.currentScope = scope
	return scope
}

// leaveScope 离开当前作用域
func (sa *ScopeAnalyzer) leaveScope() {
	if sa.currentScope.Parent != nil {
		sa.currentScope = sa.currentScope.Parent
	}
}

// declareObject 在当前作用域中声明对象
func (sa *ScopeAnalyzer) declareObject(name string, kind ObjectKind, decl ast.Node, pos token.Pos) *Object {
	obj := &Object{
		Name:       name,
		Kind:       kind,
		Decl:       decl,
		Scope:      sa.currentScope,
		IsExported: isExported(name),
		Pos:        pos,
	}
	
	sa.currentScope.Objects[name] = obj
	sa.objects[pos] = obj
	return obj
}

// analyzeDecl 分析声明
func (sa *ScopeAnalyzer) analyzeDecl(decl ast.Decl) {
	switch d := decl.(type) {
	case *ast.FuncDecl:
		sa.analyzeFuncDecl(d)
	case *ast.GenDecl:
		sa.analyzeGenDecl(d)
	}
}

// analyzeFuncDecl 分析函数声明
func (sa *ScopeAnalyzer) analyzeFuncDecl(decl *ast.FuncDecl) {
	// 在当前作用域声明函数名（如果不是方法）
	if decl.Recv == nil {
		sa.declareObject(decl.Name.Name, ObjFunc, decl, decl.Name.Pos())
	}
	
	// 进入函数作用域
	funcScope := sa.enterScope(decl)
	
	// 分析接收者
	if decl.Recv != nil {
		sa.analyzeFieldList(decl.Recv, ObjVar)
	}
	
	// 分析参数
	if decl.Type.Params != nil {
		sa.analyzeFieldList(decl.Type.Params, ObjVar)
	}
	
	// 分析返回值
	if decl.Type.Results != nil {
		sa.analyzeFieldList(decl.Type.Results, ObjVar)
	}
	
	// 分析函数体
	if decl.Body != nil {
		sa.analyzeBlockStmt(decl.Body)
	}
	
	sa.leaveScope()
	_ = funcScope
}

// analyzeGenDecl 分析通用声明
func (sa *ScopeAnalyzer) analyzeGenDecl(decl *ast.GenDecl) {
	for _, spec := range decl.Specs {
		switch s := spec.(type) {
		case *ast.ValueSpec:
			kind := ObjVar
			if decl.Tok == token.CONST {
				kind = ObjConst
			}
			for _, name := range s.Names {
				sa.declareObject(name.Name, kind, s, name.Pos())
			}
			// 分析初始化表达式
			for _, value := range s.Values {
				sa.analyzeExpr(value)
			}
			
		case *ast.TypeSpec:
			sa.declareObject(s.Name.Name, ObjType, s, s.Name.Pos())
			sa.analyzeExpr(s.Type)
		}
	}
}

// analyzeFieldList 分析字段列表（参数、返回值等）
func (sa *ScopeAnalyzer) analyzeFieldList(fields *ast.FieldList, kind ObjectKind) {
	if fields == nil {
		return
	}
	
	for _, field := range fields.List {
		for _, name := range field.Names {
			sa.declareObject(name.Name, kind, field, name.Pos())
		}
		sa.analyzeExpr(field.Type)
	}
}

// analyzeBlockStmt 分析块语句
func (sa *ScopeAnalyzer) analyzeBlockStmt(block *ast.BlockStmt) {
	// 块语句创建新作用域
	sa.enterScope(block)
	
	for _, stmt := range block.List {
		sa.analyzeStmt(stmt)
	}
	
	sa.leaveScope()
}

// analyzeStmt 分析语句
func (sa *ScopeAnalyzer) analyzeStmt(stmt ast.Stmt) {
	if stmt == nil {
		return
	}
	
	switch s := stmt.(type) {
	case *ast.DeclStmt:
		sa.analyzeDecl(s.Decl)
		
	case *ast.AssignStmt:
		// 分析赋值语句
		for i, lhs := range s.Lhs {
			if s.Tok == token.DEFINE {
				// 短变量声明 :=
				if ident, ok := lhs.(*ast.Ident); ok && ident.Name != "_" {
					sa.declareObject(ident.Name, ObjVar, s, ident.Pos())
				}
			}
			if i < len(s.Rhs) {
				sa.analyzeExpr(s.Rhs[i])
			}
		}
		for _, lhs := range s.Lhs {
			sa.analyzeExpr(lhs)
		}
		
	case *ast.IfStmt:
		sa.enterScope(s)
		if s.Init != nil {
			sa.analyzeStmt(s.Init)
		}
		sa.analyzeExpr(s.Cond)
		sa.analyzeBlockStmt(s.Body)
		if s.Else != nil {
			sa.analyzeStmt(s.Else)
		}
		sa.leaveScope()
		
	case *ast.ForStmt:
		sa.enterScope(s)
		if s.Init != nil {
			sa.analyzeStmt(s.Init)
		}
		if s.Cond != nil {
			sa.analyzeExpr(s.Cond)
		}
		if s.Post != nil {
			sa.analyzeStmt(s.Post)
		}
		sa.analyzeBlockStmt(s.Body)
		sa.leaveScope()
		
	case *ast.RangeStmt:
		sa.enterScope(s)
		if s.Tok == token.DEFINE {
			if key, ok := s.Key.(*ast.Ident); ok && key.Name != "_" {
				sa.declareObject(key.Name, ObjVar, s, key.Pos())
			}
			if value, ok := s.Value.(*ast.Ident); ok && value.Name != "_" {
				sa.declareObject(value.Name, ObjVar, s, value.Pos())
			}
		}
		sa.analyzeExpr(s.X)
		sa.analyzeBlockStmt(s.Body)
		sa.leaveScope()
		
	case *ast.SwitchStmt:
		sa.enterScope(s)
		if s.Init != nil {
			sa.analyzeStmt(s.Init)
		}
		if s.Tag != nil {
			sa.analyzeExpr(s.Tag)
		}
		if s.Body != nil {
			for _, stmt := range s.Body.List {
				if clause, ok := stmt.(*ast.CaseClause); ok {
					sa.enterScope(clause)
					for _, expr := range clause.List {
						sa.analyzeExpr(expr)
					}
					for _, stmt := range clause.Body {
						sa.analyzeStmt(stmt)
					}
					sa.leaveScope()
				}
			}
		}
		sa.leaveScope()
		
	case *ast.TypeSwitchStmt:
		sa.enterScope(s)
		if s.Init != nil {
			sa.analyzeStmt(s.Init)
		}
		sa.analyzeStmt(s.Assign)
		if s.Body != nil {
			for _, stmt := range s.Body.List {
				if clause, ok := stmt.(*ast.CaseClause); ok {
					sa.enterScope(clause)
					for _, expr := range clause.List {
						sa.analyzeExpr(expr)
					}
					for _, stmt := range clause.Body {
						sa.analyzeStmt(stmt)
					}
					sa.leaveScope()
				}
			}
		}
		sa.leaveScope()
		
	case *ast.SelectStmt:
		sa.enterScope(s)
		if s.Body != nil {
			for _, stmt := range s.Body.List {
				if clause, ok := stmt.(*ast.CommClause); ok {
					sa.enterScope(clause)
					if clause.Comm != nil {
						sa.analyzeStmt(clause.Comm)
					}
					for _, stmt := range clause.Body {
						sa.analyzeStmt(stmt)
					}
					sa.leaveScope()
				}
			}
		}
		sa.leaveScope()
		
	case *ast.BlockStmt:
		sa.analyzeBlockStmt(s)
		
	case *ast.ExprStmt:
		sa.analyzeExpr(s.X)
		
	case *ast.SendStmt:
		sa.analyzeExpr(s.Chan)
		sa.analyzeExpr(s.Value)
		
	case *ast.IncDecStmt:
		sa.analyzeExpr(s.X)
		
	case *ast.ReturnStmt:
		for _, expr := range s.Results {
			sa.analyzeExpr(expr)
		}
		
	case *ast.BranchStmt:
		// break, continue, goto, fallthrough
		
	case *ast.GoStmt:
		sa.analyzeExpr(s.Call)
		
	case *ast.DeferStmt:
		sa.analyzeExpr(s.Call)
		
	case *ast.LabeledStmt:
		sa.analyzeStmt(s.Stmt)
	}
}

// analyzeExpr 分析表达式
func (sa *ScopeAnalyzer) analyzeExpr(expr ast.Expr) {
	if expr == nil {
		return
	}
	
	switch e := expr.(type) {
	case *ast.FuncLit:
		// 函数字面量创建新作用域
		funcScope := sa.enterScope(e)
		if e.Type.Params != nil {
			sa.analyzeFieldList(e.Type.Params, ObjVar)
		}
		if e.Type.Results != nil {
			sa.analyzeFieldList(e.Type.Results, ObjVar)
		}
		sa.analyzeBlockStmt(e.Body)
		sa.leaveScope()
		_ = funcScope
		
	case *ast.CompositeLit:
		sa.analyzeExpr(e.Type)
		for _, elt := range e.Elts {
			if kv, ok := elt.(*ast.KeyValueExpr); ok {
				sa.analyzeExpr(kv.Key)
				sa.analyzeExpr(kv.Value)
			} else {
				sa.analyzeExpr(elt)
			}
		}
		
	case *ast.BinaryExpr:
		sa.analyzeExpr(e.X)
		sa.analyzeExpr(e.Y)
		
	case *ast.UnaryExpr:
		sa.analyzeExpr(e.X)
		
	case *ast.CallExpr:
		sa.analyzeExpr(e.Fun)
		for _, arg := range e.Args {
			sa.analyzeExpr(arg)
		}
		
	case *ast.IndexExpr:
		sa.analyzeExpr(e.X)
		sa.analyzeExpr(e.Index)
		
	case *ast.SliceExpr:
		sa.analyzeExpr(e.X)
		if e.Low != nil {
			sa.analyzeExpr(e.Low)
		}
		if e.High != nil {
			sa.analyzeExpr(e.High)
		}
		if e.Max != nil {
			sa.analyzeExpr(e.Max)
		}
		
	case *ast.SelectorExpr:
		sa.analyzeExpr(e.X)
		
	case *ast.StarExpr:
		sa.analyzeExpr(e.X)
		
	case *ast.TypeAssertExpr:
		sa.analyzeExpr(e.X)
		if e.Type != nil {
			sa.analyzeExpr(e.Type)
		}
		
	case *ast.ParenExpr:
		sa.analyzeExpr(e.X)
		
	case *ast.ArrayType:
		if e.Len != nil {
			sa.analyzeExpr(e.Len)
		}
		sa.analyzeExpr(e.Elt)
		
	case *ast.StructType:
		if e.Fields != nil {
			sa.analyzeFieldList(e.Fields, ObjField)
		}
		
	case *ast.FuncType:
		if e.Params != nil {
			sa.analyzeFieldList(e.Params, ObjVar)
		}
		if e.Results != nil {
			sa.analyzeFieldList(e.Results, ObjVar)
		}
		
	case *ast.InterfaceType:
		if e.Methods != nil {
			sa.analyzeFieldList(e.Methods, ObjMethod)
		}
		
	case *ast.MapType:
		sa.analyzeExpr(e.Key)
		sa.analyzeExpr(e.Value)
		
	case *ast.ChanType:
		sa.analyzeExpr(e.Value)
	}
}

// LookupObject 在作用域链中查找对象
func (s *Scope) LookupObject(name string) *Object {
	for scope := s; scope != nil; scope = scope.Parent {
		if obj, ok := scope.Objects[name]; ok {
			return obj
		}
	}
	return nil
}

// GetScopeAt 获取指定位置的作用域
func (sa *ScopeAnalyzer) GetScopeAt(pos token.Pos) *Scope {
	// 从当前作用域开始向上查找
	for scope := sa.currentScope; scope != nil; scope = scope.Parent {
		if pos >= scope.Start && pos <= scope.End {
			// 检查子作用域
			for _, child := range scope.Children {
				if pos >= child.Start && pos <= child.End {
					return sa.getScopeAtInTree(child, pos)
				}
			}
			return scope
		}
	}
	return sa.fileScope
}

// getScopeAtInTree 在作用域树中递归查找
func (sa *ScopeAnalyzer) getScopeAtInTree(scope *Scope, pos token.Pos) *Scope {
	if pos < scope.Start || pos > scope.End {
		return nil
	}
	
	// 检查子作用域
	for _, child := range scope.Children {
		if pos >= child.Start && pos <= child.End {
			if result := sa.getScopeAtInTree(child, pos); result != nil {
				return result
			}
		}
	}
	
	return scope
}

// GetFileScope 获取文件作用域
func (sa *ScopeAnalyzer) GetFileScope() *Scope {
	return sa.fileScope
}

// GetPackageScope 获取包作用域
func (sa *ScopeAnalyzer) GetPackageScope() *Scope {
	return sa.packageScope
}

