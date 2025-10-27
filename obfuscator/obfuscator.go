package obfuscator

import (
	"crypto/rand"
	"fmt"
	"go/token"
	"math/big"
)

// New 创建新的混淆器实例
func New(projectRoot, outputDir string, config *Config) *Obfuscator {
	if config == nil {
		config = &Config{
			ObfuscateExported:  false,
			ObfuscateFileNames: false,
			EncryptStrings:     false,
			InjectJunkCode:     false,
			RemoveComments:     true,
			PreserveReflection: true,
			SkipGeneratedCode:  true,
			ExcludePatterns:    []string{},
		}
	}

	seed, _ := rand.Int(rand.Reader, big.NewInt(999999))
	encryptionKey := generateRandomString(64)
	// 生成完全随机的导出函数名（首字母大写）
	decryptFuncName := fmt.Sprintf("%c%s", 'A'+byte(seed.Int64()%26), generateRandomString(11))
	decryptPkgName := fmt.Sprintf("p%s", generateRandomString(8))

	return &Obfuscator{
		varMapping:          make(map[string]string),
		funcMapping:         make(map[string]string),
		exportedFuncMapping: make(map[string]string),
		fset:                token.NewFileSet(),
		randomSeed:          seed,
		encryptionKey:       encryptionKey,
		namingCounter:       0,
		projectRoot:         projectRoot,
		outputDir:           outputDir,
		importAliasMapping:  make(map[string]string),
		importPathToName:    make(map[string]string),
		fileNameMapping:     make(map[string]string),
		filePathMapping:     make(map[string]string),
		protectedNames:      make(map[string]bool),
		packageNames:        make(map[string]bool),
		Config:              config,
		encryptedStrings:    make(map[string]bool),
		decryptFuncAdded:    make(map[string]bool),
		decryptFuncName:     decryptFuncName,
		decryptPkgName:      decryptPkgName,
		decryptPkgCreated:   false,
		skippedFiles:        make(map[string]string),
		reflectionPackages:  make(map[string]bool),
		fileScopes:          make(map[string]*ScopeAnalyzer),
		objectMapping:       make(map[*Object]string),
	}
}

// GetStatistics 返回混淆统计信息
func (o *Obfuscator) GetStatistics() *Statistics {
	// 统计对象映射中的函数和变量
	funcCount := len(o.funcMapping)
	varCount := len(o.varMapping)
	
	// 如果使用了作用域分析，从objectMapping统计
	if len(o.objectMapping) > 0 {
		funcCount = 0
		varCount = 0
		for obj, obfName := range o.objectMapping {
			if obfName != "" {
				if obj.Kind == ObjFunc {
					funcCount++
				} else if obj.Kind == ObjVar || obj.Kind == ObjConst {
					varCount++
				}
			}
		}
	}
	
	return &Statistics{
		ProtectedNames: len(o.protectedNames),
		FunctionsObf:   funcCount,
		VariablesObf:   varCount,
		SkippedFiles:   len(o.skippedFiles),
	}
}

