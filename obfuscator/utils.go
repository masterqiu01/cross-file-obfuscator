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

	// 检查排除模式
	for _, pattern := range o.Config.ExcludePatterns {
		// 尝试匹配相对路径（用于 "tools/*" 这样的模式）
		matched, err := filepath.Match(pattern, relPath)
		if err == nil && matched {
			return true, "matches pattern: " + pattern
		}

		// 也尝试只匹配文件名（用于 "*.pb.go" 这样的模式）
		matched, err = filepath.Match(pattern, filepath.Base(filePath))
		if err == nil && matched {
			return true, "matches pattern: " + pattern
		}

		// 检查模式是否包含路径分隔符，处理 "tools/*" 或 "**/test/**" 这样的模式
		if strings.Contains(pattern, string(filepath.Separator)) || strings.Contains(pattern, "/") {
			// 转换模式使用正确的分隔符
			normalizedPattern := filepath.FromSlash(pattern)
			
			// 检查相对路径是否以模式前缀开始或包含模式
			if strings.HasSuffix(normalizedPattern, "/*") {
				// 类似 "tools/*" 的模式 - 检查文件是否在此目录中
				dirPattern := strings.TrimSuffix(normalizedPattern, "/*")
				if strings.HasPrefix(relPath, dirPattern+string(filepath.Separator)) {
					return true, "matches pattern: " + pattern
				}
				// 也检查完全匹配目录名的情况
				if relPath == dirPattern {
					return true, "matches pattern: " + pattern
				}
			}
			
			// 处理 "*/certs/*" 这样的模式
			if strings.HasPrefix(normalizedPattern, "*/") {
				// 移除前缀 "*/"
				subPattern := strings.TrimPrefix(normalizedPattern, "*/")
				// 检查路径中是否包含这个子模式
				if strings.HasSuffix(subPattern, "/*") {
					// 模式是 "*/dirname/*" - 检查是否在任何 dirname 目录下
					dirName := strings.TrimSuffix(subPattern, "/*")
					pathParts := strings.Split(relPath, string(filepath.Separator))
					for _, part := range pathParts {
						if part == dirName {
							return true, "matches pattern: " + pattern
						}
					}
				} else {
					// 模式是 "*/filename" - 直接匹配
					if strings.Contains(relPath, string(filepath.Separator)+subPattern) {
						return true, "matches pattern: " + pattern
					}
				}
			}
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
	excluded, _ := o.shouldExcludeFile(path)
	return excluded
}
