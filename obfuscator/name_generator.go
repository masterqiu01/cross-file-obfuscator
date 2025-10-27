package obfuscator

import (
	"crypto/rand"
	"math/big"
	"strings"
)

// NaturalNameGenerator 生成看起来自然的包名和函数名
type NaturalNameGenerator struct {
	usedNames map[string]bool
	
	// 包名片段
	pkgPrefixes []string
	pkgSuffixes []string
	pkgMiddles  []string
	
	// 单词库（用于生成标识符）
	words []string
}

// NewNaturalNameGenerator 创建自然名称生成器
func NewNaturalNameGenerator() *NaturalNameGenerator {
	return &NaturalNameGenerator{
		usedNames: make(map[string]bool),
		
		// 常见的包名前缀
		pkgPrefixes: []string{
			"app", "lib", "pkg", "sys", "web", "api", "net", "db",
			"svc", "core", "util", "data", "log", "auth", "cache",
		},
		
		// 常见的包名后缀
		pkgSuffixes: []string{
			"core", "util", "base", "main", "impl", "svc", "mgr",
			"handler", "service", "client", "server", "config",
		},
		
		// 中间词
		pkgMiddles: []string{
			"", "http", "grpc", "rest", "rpc", "sql", "store",
		},
		
		// 常见的英文单词（用于函数名等）
		words: []string{
			"handle", "process", "execute", "run", "start", "stop",
			"init", "setup", "config", "load", "save", "update",
			"create", "delete", "remove", "add", "set", "get",
			"fetch", "send", "receive", "parse", "format", "convert",
		},
	}
}

// GeneratePackageName 生成指定长度的看起来自然的包名
// 例如: "runtime." (8 chars) → "libcore." (8 chars)
func (g *NaturalNameGenerator) GeneratePackageName(originalName string, targetLength int) string {
	hasDot := strings.HasSuffix(originalName, ".")
	
	if hasDot {
		targetLength-- // 为末尾的点预留空间
	}
	
	var attempt int
	for attempt < 1000 { // 最多尝试1000次
		name := g.generateName(targetLength)
		
		if hasDot {
			name += "."
		}
		
		// 确保不重复
		if !g.usedNames[name] {
			g.usedNames[name] = true
			return name
		}
		attempt++
	}
	
	// 如果实在找不到，返回随机字符（但保持可读性）
	return g.generateRandomReadable(originalName, targetLength, hasDot)
}

// generateName 生成指定长度的名称
func (g *NaturalNameGenerator) generateName(length int) string {
	if length <= 3 {
		// 短名称：使用缩写
		return g.generateShortName(length)
	}
	
	if length <= 8 {
		// 中等名称：使用前缀或后缀
		return g.generateMediumName(length)
	}
	
	// 长名称：使用前缀+后缀组合
	return g.generateLongName(length)
}

// generateShortName 生成短名称 (1-3 字符)
func (g *NaturalNameGenerator) generateShortName(length int) string {
	shortNames := []string{
		"a", "b", "c", "x", "y", "z",
		"ab", "io", "os", "db", "fs", "ws",
		"app", "api", "sys", "lib", "net", "log",
	}
	
	// 过滤出符合长度的
	candidates := []string{}
	for _, name := range shortNames {
		if len(name) == length {
			candidates = append(candidates, name)
		}
	}
	
	if len(candidates) > 0 {
		idx := g.secureRandInt(len(candidates))
		return candidates[idx]
	}
	
	// 如果没有匹配的，生成随机字母
	return g.randomLetters(length)
}

// generateMediumName 生成中等长度名称 (4-8 字符)
func (g *NaturalNameGenerator) generateMediumName(length int) string {
	// 尝试使用单个词
	for _, prefix := range g.pkgPrefixes {
		if len(prefix) == length {
			return prefix
		}
	}
	
	for _, suffix := range g.pkgSuffixes {
		if len(suffix) == length {
			return suffix
		}
	}
	
	// 如果没有完全匹配的，截断或组合
	if length >= 4 {
		word := g.pkgPrefixes[g.secureRandInt(len(g.pkgPrefixes))]
		if len(word) > length {
			return word[:length]
		} else if len(word) < length {
			// 添加后缀
			suffix := g.pkgMiddles[g.secureRandInt(len(g.pkgMiddles))]
			combined := word + suffix
			if len(combined) >= length {
				return combined[:length]
			}
			// 继续添加
			return g.padWithLetters(combined, length)
		}
		return word
	}
	
	return g.randomLetters(length)
}

// generateLongName 生成长名称 (9+ 字符)
func (g *NaturalNameGenerator) generateLongName(length int) string {
	// 组合多个词
	prefix := g.pkgPrefixes[g.secureRandInt(len(g.pkgPrefixes))]
	suffix := g.pkgSuffixes[g.secureRandInt(len(g.pkgSuffixes))]
	
	combined := prefix + suffix
	
	if len(combined) > length {
		return combined[:length]
	} else if len(combined) < length {
		// 需要更多字符
		middle := g.pkgMiddles[g.secureRandInt(len(g.pkgMiddles))]
		combined = prefix + middle + suffix
		
		if len(combined) >= length {
			return combined[:length]
		}
		
		// 还是不够，继续填充
		return g.padWithLetters(combined, length)
	}
	
	return combined
}

// generateRandomReadable 生成随机但可读的名称
func (g *NaturalNameGenerator) generateRandomReadable(original string, length int, hasDot bool) string {
	// 保留原始名称的第一个字符（如果可能）
	result := ""
	if len(original) > 0 && original[0] >= 'a' && original[0] <= 'z' {
		result = string(original[0])
	} else {
		result = string(rune('a' + g.secureRandInt(26)))
	}
	
	// 交替使用辅音和元音，使其更可读
	vowels := "aeiou"
	consonants := "bcdfghjklmnpqrstvwxyz"
	
	useVowel := false
	for len(result) < length {
		if useVowel {
			result += string(vowels[g.secureRandInt(len(vowels))])
		} else {
			result += string(consonants[g.secureRandInt(len(consonants))])
		}
		useVowel = !useVowel
	}
	
	if hasDot {
		result += "."
	}
	
	return result
}

// padWithLetters 用字母填充到指定长度
func (g *NaturalNameGenerator) padWithLetters(base string, targetLength int) string {
	letters := "abcdefghijklmnopqrstuvwxyz"
	for len(base) < targetLength {
		base += string(letters[g.secureRandInt(len(letters))])
	}
	return base
}

// randomLetters 生成随机字母
func (g *NaturalNameGenerator) randomLetters(length int) string {
	letters := "abcdefghijklmnopqrstuvwxyz"
	result := ""
	for i := 0; i < length; i++ {
		result += string(letters[g.secureRandInt(len(letters))])
	}
	return result
}

// secureRandInt 生成安全的随机整数 [0, max)
func (g *NaturalNameGenerator) secureRandInt(max int) int {
	if max <= 0 {
		return 0
	}
	n, err := rand.Int(rand.Reader, big.NewInt(int64(max)))
	if err != nil {
		// 降级到不安全的随机数
		return int(n.Int64()) % max
	}
	return int(n.Int64())
}
