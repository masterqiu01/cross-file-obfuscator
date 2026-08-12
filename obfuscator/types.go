package obfuscator

import "go/token"

// Obfuscator 是混淆器的主结构体
type Obfuscator struct {
	// 名称映射
	varMapping         map[string]string
	funcMapping        map[string]string
	importAliasMapping map[string]string
	fileNameMapping    map[string]string

	// 已使用的混淆名称集合（保证唯一性的 O(1) 判断）
	usedNames map[string]bool

	// 保护名称
	protectedNames map[string]bool
	packageNames   map[string]bool
	skippedFiles   map[string]string

	// Token 文件集
	fset *token.FileSet

	encryptionKey string
	namingCounter int

	// 路径配置
	projectRoot string
	outputDir   string

	// 配置选项
	Config *Config

	// 字符串加密追踪
	decryptFuncName   string
	decryptFuncNames  []string
	decryptPkgName    string
	decryptPkgPath    string
	decryptPkgCreated bool

	// 作用域分析
	fileScopes    map[string]*ScopeAnalyzer // 文件路径 -> 作用域分析器
	objectMapping map[*Object]string        // 对象 -> 混淆后的名称

	// 保护特殊文件
	mainFiles  map[string]bool // 包含 func main 的文件
	embedFiles map[string]bool // 被 //go:embed 引用的文件

	// 反射精确保护（garble 式：只保护实际被反射/JSON 引用的类型，
	// 而非『import reflect 即保护整文件』；无法静态解析时回退整文件保护）
	reflectionTargetTypes map[string]bool // 类型名 → 该类型确实被 reflect/JSON 引用

	// go.mod 解析缓存
	moduleName string

	// 嵌套模块识别：项目内含独立 go.mod 的目录（相对路径） -> 其模块名。
	// 用于按模块向各模块根分发解密包，确保每个子模块能独立编译。
	subModuleRoots map[string]string

	// 解密包实际创建的目录（相对输出目录），用于精确跳过而非路径段猜测
	decryptPkgDirs map[string]bool

	// 统计信息
	totalGoFiles         int
	obfuscatedGoFiles    int
	encryptedStringCount int
}

// Config 存储混淆配置
type Config struct {
	// 基础混淆选项
	ObfuscateExported  bool     // 是否混淆导出的函数（危险！）
	ObfuscateFileNames bool     // 是否混淆文件名
	EncryptStrings     bool     // 是否加密字符串字面量
	InjectJunkCode     bool     // 是否注入垃圾代码
	RemoveComments     bool     // 是否移除注释
	ObfuscatePositions bool     // 是否用 //line 伪文件名混淆源码位置信息
	PreserveReflection bool     // 是否保留反射相关代码
	SkipGeneratedCode  bool     // 是否跳过自动生成的代码
	ExcludePatterns    []string // 要排除的文件模式
	DryRun             bool     // 干跑模式：只打印将要混淆的内容，不实际写入文件
}

// Statistics 存储混淆统计信息
type Statistics struct {
	TotalFiles      int
	ObfuscatedFiles int
	SkippedFiles    int
	ProtectedNames  int
	FunctionsObf    int
	VariablesObf    int
	StringsEncrypt  int
}

// LinkConfig 链接器混淆配置
type LinkConfig struct {
	RemoveFuncNames      bool              // 是否混淆函数名（替换包名前缀）
	EntryPackage         string            // 入口包路径，例如: "./cmd/server" 或 "." (当前目录)
	PackageReplacements  map[string]string // 自定义包名替换映射，例如: {"github.com/user/project": "a", "main": "m"}
	AutoDiscoverPackages bool              // 是否自动发现并替换项目中的所有包名
	ObfuscateThirdParty  bool              // 是否混淆第三方依赖包（谨慎使用）
	OnlyObfuscateProject bool              // 只混淆项目包，保留标准库（减少杀软误报）
	DisablePclntab       bool              // 完全禁用 pclntab 修改（最安全）
	PackageFilter        string            // 包名过滤关键字，只发现包含该字符串的包
	ObfuscateFilePaths   bool              // 混淆二进制中包含 .go 的所有绝对/相对文件路径
}
