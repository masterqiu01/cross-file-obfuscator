package obfuscator

import (
	"fmt"
	"go/token"
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

	encryptionKey := generateRandomString(64)
	// 生成完全随机的导出函数名（首字母大写）
	decryptFuncName := fmt.Sprintf("%c%s", 'A'+rng.IntN(26), generateRandomString(11))
	decryptPkgName := fmt.Sprintf("p%s", generateRandomString(8))

	// 为每种字符串解密策略生成独立的随机函数名
	decryptFuncNames := make([]string, numDecryptStrategies)
	for i := range decryptFuncNames {
		decryptFuncNames[i] = fmt.Sprintf("%c%s", 'A'+rng.IntN(26), generateRandomString(11))
	}

	return &Obfuscator{
		varMapping:            make(map[string]string),
		funcMapping:           make(map[string]string),
		importAliasMapping:    make(map[string]string),
		fileNameMapping:       make(map[string]string),
		usedNames:             make(map[string]bool),
		fset:                  token.NewFileSet(),
		encryptionKey:         encryptionKey,
		projectRoot:           projectRoot,
		outputDir:             outputDir,
		protectedNames:        make(map[string]bool),
		packageNames:          make(map[string]bool),
		Config:                config,
		decryptFuncName:       decryptFuncName,
		decryptFuncNames:      decryptFuncNames,
		decryptPkgName:        decryptPkgName,
		fileScopes:            make(map[string]*ScopeAnalyzer),
		objectMapping:         make(map[*Object]string),
		mainFiles:             make(map[string]bool),
		embedFiles:            make(map[string]bool),
		skippedFiles:          make(map[string]string),
		reflectionTargetTypes: make(map[string]bool),
	}
}

// GetStatistics 返回混淆统计信息
func (o *Obfuscator) GetStatistics() *Statistics {
	funcCount, varCount := 0, 0
	if len(o.objectMapping) > 0 {
		for obj, obfName := range o.objectMapping {
			if obfName != "" {
				switch obj.Kind {
				case ObjFunc:
					funcCount++
				case ObjVar, ObjConst:
					varCount++
				}
			}
		}
	} else {
		funcCount = len(o.funcMapping)
		varCount = len(o.varMapping)
	}

	return &Statistics{
		TotalFiles:      o.totalGoFiles,
		ObfuscatedFiles: o.obfuscatedGoFiles,
		ProtectedNames:  len(o.protectedNames),
		FunctionsObf:    funcCount,
		VariablesObf:    varCount,
		StringsEncrypt:  o.encryptedStringCount,
		SkippedFiles:    len(o.skippedFiles),
	}
}
