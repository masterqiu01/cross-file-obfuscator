package obfuscator

import (
	"crypto/rand"
	"log"
	"math/big"
	"path/filepath"
	"strings"
)

// generateRandomString 生成随机字母数字字符串
func generateRandomString(length int) string {
	const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	result := make([]byte, length)
	for i := range result {
		num, err := rand.Int(rand.Reader, big.NewInt(int64(len(charset))))
		if err != nil {
			log.Printf("Warning: Failed to generate random number: %v, using fallback", err)
			result[i] = charset[i%len(charset)]
			continue
		}
		result[i] = charset[num.Int64()]
	}
	return string(result)
}

// isStandardLibrary 检查导入路径是否属于 Go 标准库
func isStandardLibrary(importPath string) bool {
	// 特殊情况：内部 Go 包
	if strings.HasPrefix(importPath, "internal/") || strings.HasPrefix(importPath, "vendor/") {
		return true
	}

	// 获取路径的第一个组件（第一个斜杠之前）
	firstComponent := importPath
	if idx := strings.Index(importPath, "/"); idx != -1 {
		firstComponent = importPath[:idx]
	}

	// 标准库包的第一个组件不包含点
	// 第三方包有域名（github.com, gopkg.in 等）
	return !strings.Contains(firstComponent, ".")
}

// isExported 检查名称是否为导出的（以大写字母开头）
func isExported(name string) bool {
	if len(name) == 0 {
		return false
	}
	return name[0] >= 'A' && name[0] <= 'Z'
}

// shouldExcludeFile 检查文件是否应该被排除
func (o *Obfuscator) shouldExcludeFile(filePath string) (bool, string) {
	// 获取相对于项目根目录的路径用于模式匹配
	relPath, err := filepath.Rel(o.projectRoot, filePath)
	if err != nil {
		relPath = filePath
	}

	// 规范化路径，统一使用 /，方便匹配
	relPathSlash := filepath.ToSlash(relPath)

	for _, pattern := range o.Config.ExcludePatterns {
		pattern = strings.TrimSpace(pattern)
		if pattern == "" {
			continue
		}

		patternSlash := filepath.ToSlash(pattern)

		// 1. 完全匹配或直接通配符匹配 (如 "tools/*" 或 "*.go")
		matched, _ := filepath.Match(patternSlash, relPathSlash)
		if matched {
			return true, "matches full path pattern: " + pattern
		}

		// 2. 文件名匹配 (如 "*.pb.go")
		matched, _ = filepath.Match(patternSlash, filepath.Base(relPathSlash))
		if matched {
			return true, "matches filename pattern: " + pattern
		}

		// 3. 目录或子字符串匹配
		// 如果 pattern 是一个明确的目录名 (如 "internal" 或 "vendor")
		// 我们检查路径中是否包含作为独立部分存在的这个目录
		parts := strings.Split(relPathSlash, "/")
		for _, part := range parts {
			if part == patternSlash {
				return true, "matches directory name: " + pattern
			}
		}

		// 4. 前缀匹配 (如 "tools/" 匹配 "tools/a/b.go")
		if strings.HasPrefix(relPathSlash, patternSlash) {
			return true, "matches prefix: " + pattern
		}

		// 5. 特殊前缀匹配：如果用户输入 "tools"，也应排除 "tools/a.go"
		if strings.HasPrefix(relPathSlash, patternSlash+"/") {
			return true, "matches directory prefix: " + pattern
		}
	}

	return false, ""
}

// isGeneratedFile 检查文件是否为自动生成的
func (o *Obfuscator) isGeneratedFile(path string) bool {
	// 检查文件名模式
	if strings.HasSuffix(path, ".pb.go") ||
		strings.HasSuffix(path, ".gen.go") ||
		strings.HasSuffix(path, "_generated.go") {
		return true
	}
	return false
}

// isExcluded 检查文件是否被排除
func (o *Obfuscator) isExcluded(path string) bool {
	excluded, reason := o.shouldExcludeFile(path)
	if excluded {
		log.Printf("排查功能: 排除文件 %s (原因: %s)", path, reason)
	}
	return excluded
}
