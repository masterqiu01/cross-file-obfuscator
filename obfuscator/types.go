package obfuscator

import (
	"go/token"
	"math/big"
)

// Obfuscator 是混淆器的主结构体
type Obfuscator struct {
	// 名称映射
	varMapping          map[string]string
	funcMapping         map[string]string
	exportedFuncMapping map[string]string
	importAliasMapping  map[string]string
	importPathToName    map[string]string
	fileNameMapping     map[string]string
	filePathMapping     map[string]string

	// 保护名称
	protectedNames      map[string]bool
	packageNames        map[string]bool
	reflectionPackages  map[string]bool
	skippedFiles        map[string]string

	// Token 文件集
	fset *token.FileSet

	// 随机种子和计数器
	randomSeed    *big.Int
	encryptionKey string
	namingCounter int

	// 路径配置
	projectRoot string
	outputDir   string

	// 配置选项
	Config *Config

	// 字符串加密追踪
	encryptedStrings map[string]bool
	decryptFuncAdded map[string]bool
	decryptFuncName  string

	// 作用域分析
	fileScopes       map[string]*ScopeAnalyzer // 文件路径 -> 作用域分析器
	objectMapping    map[*Object]string        // 对象 -> 混淆后的名称
}

// Config 存储混淆配置
type Config struct {
	// 基础混淆选项
	ObfuscateExported  bool     // 是否混淆导出的函数（危险！）
	ObfuscateFileNames bool     // 是否混淆文件名
	EncryptStrings     bool     // 是否加密字符串字面量
	InjectJunkCode     bool     // 是否注入垃圾代码
	RemoveComments     bool     // 是否移除注释
	PreserveReflection bool     // 是否保留反射相关代码
	SkipGeneratedCode  bool     // 是否跳过自动生成的代码
	ExcludePatterns    []string // 要排除的文件模式
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
	RemoveFuncNames       bool              // 是否混淆函数名（替换包名前缀）
	EntryPackage          string            // 入口包路径，例如: "./cmd/server" 或 "." (当前目录)
	PackageReplacements   map[string]string // 自定义包名替换映射，例如: {"github.com/user/project": "a", "main": "m"}
	AutoDiscoverPackages  bool              // 是否自动发现并替换项目中的所有包名
	ObfuscateThirdParty   bool              // 是否混淆第三方依赖包（谨慎使用）
}

