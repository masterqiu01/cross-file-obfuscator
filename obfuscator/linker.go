package obfuscator

import (
	"bytes"
	"debug/elf"
	"debug/macho"
	"debug/pe"
	"encoding/binary"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"math/rand"
	"time"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
)

// Go pclntab magic values
const (
	go12magic  = 0xfffffffb
	go116magic = 0xfffffffa
	go118magic = 0xfffffff0
	go120magic = 0xfffffff1
)

// LinkerObfuscator 处理链接器级别的混淆
type LinkerObfuscator struct {
	config     *LinkConfig
	projectDir string
	outputBin  string
}

// NewLinkerObfuscator 创建新的链接器混淆器
func NewLinkerObfuscator(projectDir, outputBin string, config *LinkConfig) *LinkerObfuscator {
	if config == nil {
		config = &LinkConfig{
			RemoveFuncNames: true, // 默认混淆函数名
			EntryPackage:    ".",  // 默认当前目录
		}
	}
	// 如果没有指定入口包，默认使用当前目录
	if config.EntryPackage == "" {
		config.EntryPackage = "."
	}
	return &LinkerObfuscator{
		config:     config,
		projectDir: projectDir,
		outputBin:  outputBin,
	}
}

// BuildWithLinkerObfuscation 使用链接器混淆构建项目
func (lo *LinkerObfuscator) BuildWithLinkerObfuscation() error {
	fmt.Println("=== 链接器级别混淆 (构建并混淆) ===")
	fmt.Printf("项目目录: %s\n", lo.projectDir)
	fmt.Printf("入口包: %s\n", lo.config.EntryPackage)
	
	// 如果启用了自动包名发现且没有手动指定包名替换
	if lo.config.AutoDiscoverPackages && len(lo.config.PackageReplacements) == 0 {
		fmt.Println("第 0 步: 自动发现项目包名...")
		if err := lo.discoverAndGeneratePackageReplacements(); err != nil {
			fmt.Printf("[!] 警告: 自动发现包名失败: %v\n", err)
			fmt.Println("   将继续使用默认包名替换模式")
		}
	}
	
	fmt.Println("第 1 步: 标准编译...")
	
	// 确保输出路径是绝对路径
	outputPath := lo.outputBin
	if !filepath.IsAbs(outputPath) {
		cwd, err := os.Getwd()
		if err != nil {
			return fmt.Errorf("获取当前目录失败: %v", err)
		}
		outputPath = filepath.Join(cwd, outputPath)
	}
	
	// 构建 go build 命令
	// 使用 -ldflags="-s -w" 移除符号表和调试信息
	// -s: 禁用符号表
	// -w: 禁用 DWARF 调试信息
	buildArgs := []string{"build", "-ldflags=-s -w", "-trimpath", "-o", outputPath}
	
	// 添加入口包路径
	buildArgs = append(buildArgs, lo.config.EntryPackage)
	
	buildCmd := exec.Command("go", buildArgs...)
	buildCmd.Dir = lo.projectDir
	buildCmd.Stdout = os.Stdout
	buildCmd.Stderr = os.Stderr
	buildCmd.Env = os.Environ() // 继承当前环境变量（包括 CGO_ENABLED 等）
	
	// 打印实际执行的命令（调试用）
	fmt.Printf("   执行命令: cd %s && go %s\n", lo.projectDir, strings.Join(buildArgs, " "))
	
	if err := buildCmd.Run(); err != nil {
		return fmt.Errorf("编译失败: %v\n请尝试在项目目录手动执行: cd %s && go %s", err, lo.projectDir, strings.Join(buildArgs, " "))
	}
	
	fmt.Println("[OK] 标准编译完成")
	
	// 更新 outputBin 为绝对路径
	lo.outputBin = outputPath
	
	// 第二步：后处理二进制文件
	fmt.Println("第 2 步: 后处理二进制文件...")
	if err := lo.postProcessBinary(); err != nil {
		return fmt.Errorf("后处理失败: %v", err)
	}
	fmt.Println("[OK] 后处理完成")
	
	// 注意：由于编译时已使用 -ldflags="-s -w"应用修改，无需再执行 strip
	fmt.Println("[OK] 符号表已在编译时移除（-ldflags=\"-s -w\"）")
	
	return nil
}

// ObfuscateExistingBinary 直接混淆现有的二进制文件
func (lo *LinkerObfuscator) ObfuscateExistingBinary(binPath string) error {
	fmt.Println("=== 链接器级别混淆 (直接对二进制进行修改) ===")
	
	// 如果启用了自动包名发现且没有手动指定包名替换
	if lo.config.AutoDiscoverPackages && len(lo.config.PackageReplacements) == 0 {
		fmt.Println("第 0 步: 自动发现项目包名...")
		if err := lo.discoverAndGeneratePackageReplacements(); err != nil {
			fmt.Printf("[!] 警告: 自动发现包名失败: %v\n", err)
			fmt.Println("   将继续使用默认包名替换模式")
		}
	}

	lo.outputBin = binPath
	
	fmt.Println("第 1 步: 开始修改二进制文件...")
	if err := lo.postProcessBinary(); err != nil {
		return fmt.Errorf("混淆失败: %v", err)
	}
	fmt.Println("[OK] 混淆完成")
	
	return nil
}

// postProcessBinary 后处理二进制文件
func (lo *LinkerObfuscator) postProcessBinary() error {
	data, err := os.ReadFile(lo.outputBin)
	if err != nil {
		return err
	}
	
	// 检测二进制格式
	format := detectBinaryFormat(data)
	fmt.Printf("   检测到二进制格式: %s\n", format)
	
	var modified bool
	var newData []byte
	
	switch format {
	case "ELF":
		newData, modified, err = lo.processELF(data)
	case "PE":
		newData, modified, err = lo.processPE(data)
	case "Mach-O":
		newData, modified, err = lo.processMachO(data)
	default:
		return fmt.Errorf("不支持的二进制格式: %s", format)
	}
	
	if err != nil {
		return err
	}
	
	if lo.config.ObfuscateFilePaths {
		pathCount := lo.obfuscateFilePaths(newData)
		if pathCount > 0 {
			fmt.Printf("   [OK] 混淆了 %d 个源代码文件路径\n", pathCount)
			modified = true
		}
	}
	
	if modified {
		// 备份原文件
		backupPath := lo.outputBin + ".backup"
		if err := os.WriteFile(backupPath, data, 0755); err != nil {
			return fmt.Errorf("备份失败: %v", err)
		}
		
		// 写入修改后的文件
		if err := os.WriteFile(lo.outputBin, newData, 0755); err != nil {
			return fmt.Errorf("写入失败: %v", err)
		}
		
		fmt.Printf("   [OK] 已修改 pclntab\n")
		fmt.Printf("   [OK] 原文件已备份到: %s\n", backupPath)
		
		// macOS 系统修改二进制后会破坏签名，导致运行直接被杀(SIGKILL)，因此尝试自动重签名
		if runtime.GOOS == "darwin" {
			fmt.Printf("   正在尝试对 macOS 二进制文件重新签名...\n")
			cmd := exec.Command("codesign", "-s", "-", "-f", lo.outputBin)
			if err := cmd.Run(); err != nil {
				fmt.Printf("   [!] 警告: 自动签名失败 (%v)。请手动运行: codesign -s - -f %s\n", err, lo.outputBin)
			} else {
				fmt.Printf("   [OK] 已修复 macOS 代码签名\n")
			}
		}
	} else {
		fmt.Println("   [!] 未找到 pclntab 或无需修改")
	}
	
	return nil
}

// obfuscateFilePaths 全局搜索并混淆二进制中残留的 .go 源文件绝对或相对路径
func (lo *LinkerObfuscator) obfuscateFilePaths(data []byte) int {
	// 匹配具有至少一个目录层级的 .go 路径 (支持 UNIX、Windows 和包含 @ 版本号的 go mod 缓存路径)
	re := regexp.MustCompile(`(?:[a-zA-Z]:\\|/|[a-zA-Z0-9_\-\.\@]+/)[a-zA-Z0-9_\-\.\@/\\]+\.go`)
	matches := re.FindAllIndex(data, -1)
	count := 0
	
	for _, match := range matches {
		start := match[0]
		end := match[1]
		path := string(data[start:end])
		
		if !strings.Contains(path, "/") && !strings.Contains(path, "\\") {
			continue
		}
		
		// 使用不可见的高位 ASCII 字符替换，彻底避开 strings 提取
		// 这样既保持了等长，又使字符串失去了可打印特性
		rng := rand.New(rand.NewSource(time.Now().UnixNano() + int64(start)))
		
		for j := start; j < end-3; j++ {
			c := data[j]
			if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') {
				// 替换为 0x80 到 0xFE 之间的高位不可见/乱码字节
				data[j] = byte(rng.Intn(127) + 128)
			}
		}
		count++
	}

	// 额外清理 Go 1.18+ 泛型引入的 go.shape 相关内部字符串
	shapeRe := regexp.MustCompile(`go\.shape[a-zA-Z0-9_\.\*\[\]\{\}\s\(\)\-/]*`)
	shapeMatches := shapeRe.FindAllIndex(data, -1)
	
	for _, match := range shapeMatches {
		start := match[0]
		end := match[1]
		
		// 同样使用高位不可见字符，等长替换整个 go.shape 字符串
		rng := rand.New(rand.NewSource(time.Now().UnixNano() + int64(start)))
		
		for j := start; j < end; j++ {
			c := data[j]
			if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') {
				data[j] = byte(rng.Intn(127) + 128)
			}
		}
		count++
	}

	return count
}

// detectBinaryFormat 检测二进制文件格式
func detectBinaryFormat(data []byte) string {
	if len(data) < 4 {
		return "Unknown"
	}
	
	// ELF magic: 0x7F 'E' 'L' 'F'
	if data[0] == 0x7F && data[1] == 'E' && data[2] == 'L' && data[3] == 'F' {
		return "ELF"
	}
	
	// PE magic: 'M' 'Z'
	if data[0] == 'M' && data[1] == 'Z' {
		return "PE"
	}
	
	// Mach-O magic (multiple variants)
	if len(data) >= 4 {
		magic := binary.LittleEndian.Uint32(data[0:4])
		switch magic {
		case 0xfeedface, 0xcefaedfe, 0xfeedfacf, 0xcffaedfe:
			return "Mach-O"
		}
	}
	
	return "Unknown"
}

// processELF 处理 ELF 格式的二进制文件
func (lo *LinkerObfuscator) processELF(data []byte) ([]byte, bool, error) {
	// 打开 ELF 文件
	elfFile, err := elf.NewFile(bytes.NewReader(data))
	if err != nil {
		return data, false, fmt.Errorf("解析 ELF 失败: %v", err)
	}
	defer elfFile.Close()
	
	// 查找 .gopclntab 或 .data.rel.ro 段
	var pclntabData []byte
	var pclntabOffset int64
	
	for _, section := range elfFile.Sections {
		if section.Name == ".gopclntab" || section.Name == ".data.rel.ro" {
			sectionData, err := section.Data()
			if err != nil {
				continue
			}
			
			// 在段中搜索 pclntab magic
			offset := findPclntabMagic(sectionData)
			if offset >= 0 {
				pclntabData = sectionData
				pclntabOffset = int64(section.Offset) + int64(offset)
				fmt.Printf("   找到 pclntab 在段 %s，偏移: 0x%x\n", section.Name, pclntabOffset)
				break
			}
		}
	}
	
	if pclntabData == nil {
		// 在整个文件中搜索
		offset := findPclntabMagic(data)
		if offset < 0 {
			return data, false, nil
		}
		pclntabOffset = int64(offset)
		fmt.Printf("   找到 pclntab 在文件偏移: 0x%x\n", pclntabOffset)
	}
	
	// 修改二进制数据
	return lo.modifyPclntab(data, pclntabOffset)
}

// processPE 处理 PE 格式的二进制文件
func (lo *LinkerObfuscator) processPE(data []byte) ([]byte, bool, error) {
	peFile, err := pe.NewFile(bytes.NewReader(data))
	if err != nil {
		return data, false, fmt.Errorf("解析 PE 失败: %v", err)
	}
	defer peFile.Close()
	
	// 在 .rdata 或 .data 段中查找 pclntab
	var pclntabOffset int64 = -1
	
	for _, section := range peFile.Sections {
		if section.Name == ".rdata" || section.Name == ".data" {
			sectionData, err := section.Data()
			if err != nil {
				continue
			}
			
			offset := findPclntabMagic(sectionData)
			if offset >= 0 {
				pclntabOffset = int64(section.Offset) + int64(offset)
				fmt.Printf("   找到 pclntab 在段 %s，偏移: 0x%x\n", section.Name, pclntabOffset)
				break
			}
		}
	}
	
	if pclntabOffset < 0 {
		// 在整个文件中搜索
		offset := findPclntabMagic(data)
		if offset < 0 {
			return data, false, nil
		}
		pclntabOffset = int64(offset)
		fmt.Printf("   找到 pclntab 在文件偏移: 0x%x\n", pclntabOffset)
	}
	
	return lo.modifyPclntab(data, pclntabOffset)
}

// processMachO 处理 Mach-O 格式的二进制文件
func (lo *LinkerObfuscator) processMachO(data []byte) ([]byte, bool, error) {
	machoFile, err := macho.NewFile(bytes.NewReader(data))
	if err != nil {
		return data, false, fmt.Errorf("解析 Mach-O 失败: %v", err)
	}
	defer machoFile.Close()
	
	// 在 __gopclntab 或 __data 段中查找
	var pclntabOffset int64 = -1
	
	for _, section := range machoFile.Sections {
		if section.Name == "__gopclntab" || section.Name == "__data" {
			sectionData, err := section.Data()
			if err != nil {
				continue
			}
			
			offset := findPclntabMagic(sectionData)
			if offset >= 0 {
				pclntabOffset = int64(section.Offset) + int64(offset)
				fmt.Printf("   找到 pclntab 在段 %s，偏移: 0x%x\n", section.Name, pclntabOffset)
				break
			}
		}
	}
	
	if pclntabOffset < 0 {
		// 在整个文件中搜索
		offset := findPclntabMagic(data)
		if offset < 0 {
			return data, false, nil
		}
		pclntabOffset = int64(offset)
		fmt.Printf("   找到 pclntab 在文件偏移: 0x%x\n", pclntabOffset)
	}
	
	return lo.modifyPclntab(data, pclntabOffset)
}

// findPclntabMagic 在数据中查找 pclntab magic value
func findPclntabMagic(data []byte) int {
	magics := []uint32{go12magic, go116magic, go118magic, go120magic}
	
	for i := 0; i <= len(data)-4; i++ {
		value := binary.LittleEndian.Uint32(data[i : i+4])
		for _, magic := range magics {
			if value == magic {
				return i
			}
		}
	}
	
	return -1
}

// modifyPclntab 修改 pclntab 的内容
func (lo *LinkerObfuscator) modifyPclntab(data []byte, offset int64) ([]byte, bool, error) {
	if offset < 0 || offset+4 > int64(len(data)) {
		return data, false, fmt.Errorf("无效的 pclntab 偏移")
	}
	
	// 复制数据以避免修改原始数据
	newData := make([]byte, len(data))
	copy(newData, data)
	
	// 读取原始 magic value（仅用于显示）
	originalMagic := binary.LittleEndian.Uint32(newData[offset : offset+4])
	fmt.Printf("   原始 magic value: 0x%08x\n", originalMagic)
	
	// 检查是否完全禁用 pclntab 修改
	if lo.config.DisablePclntab {
		fmt.Printf("   [!] pclntab 修改已禁用（避免杀软误报）\n")
		return data, false, nil
	}
	
	// 混淆函数名
	if lo.config.RemoveFuncNames {
		if err := lo.obfuscateFunctionNames(newData, offset); err != nil {
			return data, false, fmt.Errorf("函数名混淆失败: %v", err)
		}
		fmt.Printf("   [OK] 已混淆函数名\n")
		return newData, true, nil
	}
	
	return data, false, nil
}

// obfuscateFunctionNames 混淆二进制中的函数名（使用等长自然混淆）
func (lo *LinkerObfuscator) obfuscateFunctionNames(data []byte, pclntabOffset int64) error {
	var patterns []string
	var replacements []string
	
	// 创建自然名称生成器
	nameGen := NewNaturalNameGenerator()
	
	// 标准库包列表
	standardLibs := map[string]bool{
		"main": false, "runtime": true, "sync": true, "fmt": true,
		"os": true, "io": true, "net": true, "http": true,
		"bufio": true, "bytes": true, "strings": true, "strconv": true,
		"time": true, "math": true, "errors": true, "context": true,
		"encoding": true, "json": true, "xml": true, "base64": true,
		"hex": true, "unicode": true, "regexp": true, "log": true,
		"sort": true, "path": true, "filepath": true, "syscall": true,
	}
	
	// 如果用户提供了自定义包名替换映射，使用它
	if len(lo.config.PackageReplacements) > 0 {
		if lo.config.OnlyObfuscateProject {
			fmt.Println("   [!] 最小化混淆模式：只混淆项目包，保留标准库")
		} else {
			fmt.Println("   使用自定义包名替换映射（等长模式）:")
		}
		
		for original, replacement := range lo.config.PackageReplacements {
			// 检查是否是标准库
			pkgName := strings.TrimSuffix(original, ".")
			isStdLib := standardLibs[pkgName]
			
			// 如果启用了 OnlyObfuscateProject，跳过标准库
			if lo.config.OnlyObfuscateProject && isStdLib {
				continue
			}
			
			// 确保包名后缀
			originalPatternDot := original
			if !strings.HasSuffix(originalPatternDot, ".") {
				originalPatternDot += "."
			}
			replacementPatternDot := replacement
			if !strings.HasSuffix(replacementPatternDot, ".") {
				replacementPatternDot += "."
			}
			
			// 确保长度一致
			if len(replacementPatternDot) != len(originalPatternDot) {
				replacementPatternDot = nameGen.GeneratePackageName(originalPatternDot, len(originalPatternDot))
			}
			
			patterns = append(patterns, originalPatternDot)
			replacements = append(replacements, replacementPatternDot)
			
			if !lo.config.OnlyObfuscateProject || (lo.config.OnlyObfuscateProject && !isStdLib) {
				fmt.Printf("     %s -> %s (均为 %d 字节)\n", originalPatternDot, replacementPatternDot, len(originalPatternDot))
			}

			// 如果包名包含 /，说明可能是带有子包的模块，额外添加 / 后缀的匹配规则
			if strings.Contains(original, "/") && !strings.HasSuffix(original, "/") {
				originalPatternSlash := original + "/"
				replacementPatternSlash := nameGen.GeneratePackageName(originalPatternSlash, len(originalPatternSlash))
				// 将生成的后缀 . 替换为 /
				replacementPatternSlash = replacementPatternSlash[:len(replacementPatternSlash)-1] + "/"
				
				patterns = append(patterns, originalPatternSlash)
				replacements = append(replacements, replacementPatternSlash)
			}
		}
		
		if lo.config.OnlyObfuscateProject {
			fmt.Printf("   [OK] 已过滤标准库，只混淆项目包（共 %d 个）\n", len(patterns))
		}
	} else {
		// 使用默认的包名替换模式（等长自然混淆）
		if lo.config.OnlyObfuscateProject {
			fmt.Println("   [!] 最小化混淆模式：只混淆项目包，保留标准库（减少杀软误报）")
			// 只混淆 main 包，保留所有标准库
			defaultPatterns := []string{
				"main.",
			}
			
			for _, pattern := range defaultPatterns {
				replacement := nameGen.GeneratePackageName(pattern, len(pattern))
				patterns = append(patterns, pattern)
				replacements = append(replacements, replacement)
			}
		} else {
			fmt.Println("   使用等长自然混淆模式（标准）")
			defaultPatterns := []string{
				"main.",
				"runtime.",
				"sync.",
				"fmt.",
				"os.",
				"io.",
				"net.",
				"http.",
			}
			
			for _, pattern := range defaultPatterns {
				// 生成等长的自然名称
				replacement := nameGen.GeneratePackageName(pattern, len(pattern))
				patterns = append(patterns, pattern)
				replacements = append(replacements, replacement)
			}
		}
		
		fmt.Println("   等长替换映射:")
		for i, pattern := range patterns {
			fmt.Printf("     %s -> %s\n", pattern, replacements[i])
		}
	}
	
	count := 0
	replacedPatterns := make(map[string]int)
	
	// 第一阶段：替换 "包名." 模式（函数名）
	// 策略：只在 pclntab 区域内替换，避免破坏 embed 文件内容
	// pclntab 区域通常在文件的特定位置，我们需要更精确的定位
	
	// 估算 pclntab 的大小（通常不超过几MB）
	// 为了安全，我们只在 pclntabOffset 后的合理范围内搜索
	pclntabSearchEnd := int(pclntabOffset) + 10*1024*1024 // 10MB 应该足够大
	if pclntabSearchEnd > len(data) {
		pclntabSearchEnd = len(data)
	}
	
	for i, pattern := range patterns {
		if i >= len(replacements) {
			break
		}
		
		patternBytes := []byte(pattern)
		replacement := []byte(replacements[i])
		patternCount := 0
		
		// 只在 pclntab 区域内查找并替换
		for j := int(pclntabOffset); j < pclntabSearchEnd-len(patternBytes); j++ {
			if bytes.Equal(data[j:j+len(patternBytes)], patternBytes) {
				// 更严格的上下文检查
				if !lo.isSafeFunctionNamePrefix(data, j, patternBytes) {
					continue
				}
				
				// ⭐ 等长替换：确保替换字符串与原字符串长度完全相同
				if len(replacement) == len(patternBytes) {
					// 直接替换，无需填充
					copy(data[j:j+len(replacement)], replacement)
					count++
					patternCount++
				} else if len(replacement) < len(patternBytes) {
					// 如果替换字符串较短（不应该发生，但作为保护）
					// 使用原始字符串填充方式而不是 0x00
					copy(data[j:j+len(replacement)], replacement)
					// 不填充，保持原有字符（更安全）
					count++
					patternCount++
				}
				// 忽略替换字符串过长的情况
			}
		}
		
		if patternCount > 0 {
			replacedPatterns[pattern] = patternCount
		}
	}
	
	// 替换项目包路径（在整个二进制文件中替换，但有严格的安全检查）
	fmt.Println("   替换项目包路径...")
	pathCount := lo.replaceProjectPackagePathsGlobal(data)
	
	if count > 0 {
		fmt.Printf("   [OK] 替换了 %d 个函数名前缀:\n", count)
		for pattern, cnt := range replacedPatterns {
			fmt.Printf("      %s: %d 次\n", pattern, cnt)
		}
	} else {
		fmt.Println("   [!] 未找到匹配的包名前缀")
	}
	
	if pathCount > 0 {
		fmt.Printf("   [OK] 替换了 %d 个项目包路径引用\n", pathCount)
	}
	
	return nil
}

// replaceProjectPackagePathsGlobal 在整个二进制文件中替换项目包路径（全局版本，带严格安全检查）
func (lo *LinkerObfuscator) replaceProjectPackagePathsGlobal(data []byte) int {
	if len(lo.config.PackageReplacements) == 0 {
		return 0
	}
	
	count := 0
	replacedPaths := make(map[string]int)
	
	// 标准库包名列表（这些不进行路径替换，只替换函数名前缀）
	standardLibs := map[string]bool{
		"main": true, "runtime": true, "sync": true, "fmt": true,
		"os": true, "io": true, "net": true, "http": true,
		"bufio": true, "bytes": true, "strings": true, "strconv": true,
		"time": true, "math": true, "errors": true, "context": true,
		"encoding": true, "json": true, "xml": true, "base64": true,
		"hex": true, "unicode": true, "regexp": true, "log": true,
		"sort": true, "path": true, "filepath": true, "syscall": true,
	}
	
	// 对每个包路径进行替换
	for original, replacement := range lo.config.PackageReplacements {
		// 移除尾部的 "." 如果有的话
		originalPath := strings.TrimSuffix(original, ".")
		
		// 跳过标准库包（标准库包名太短，可能误替换系统符号）
		if standardLibs[originalPath] {
			continue
		}
		
		// 只替换包含 "/" 的路径（项目包路径）
		// 这样可以避免替换单个单词，减少误替换风险
		if !strings.Contains(originalPath, "/") {
			continue
		}
		
		replacementPath := strings.TrimSuffix(replacement, ".")
		patternBytes := []byte(originalPath)
		replacementBytes := []byte(replacementPath)
		
		// 在整个二进制文件中搜索并替换，但要进行严格的安全检查
		for j := 0; j < len(data)-len(patternBytes); j++ {
			if bytes.Equal(data[j:j+len(patternBytes)], patternBytes) {
				// 严格的安全检查
				if !lo.isSafePackagePathReplacement(data, j, len(patternBytes)) {
					continue
				}
				
				// 只有当替换字符串不长于原字符串时才替换
				if len(replacementBytes) <= len(patternBytes) {
					copy(data[j:j+len(replacementBytes)], replacementBytes)
					// 用空字节填充剩余部分
					for k := j + len(replacementBytes); k < j + len(patternBytes); k++ {
						data[k] = 0
					}
					count++
					replacedPaths[originalPath]++
					// 跳过已替换的部分
					j += len(patternBytes) - 1
				}
			}
		}
	}
	
	if count > 0 {
		fmt.Printf("   [OK] 替换了 %d 个包路径:\n", count)
		for path, cnt := range replacedPaths {
			fmt.Printf("      %s: %d 次\n", path, cnt)
		}
	}
	
	return count
}

// isSafePackagePathReplacement 检查是否可以安全地替换包路径
// 这个检查比 isSafeToReplace 更宽松，因为包路径可能出现在多种上下文中
func (lo *LinkerObfuscator) isSafePackagePathReplacement(data []byte, pos int, length int) bool {
	// 不检查系统路径（因为包路径通常包含特殊域名如 github.com）
	// 只检查是否在合理的上下文中
	
	// 检查前面是否是合理的分隔符或开始
	if pos > 0 {
		prevChar := data[pos-1]
		// 放宽检查：只拒绝明确是标识符一部分的字符（字母、下划线）
		// 数字前缀是允许的（如 "0github.com/..."），这些是 Go 编译器添加的标记
		// 拒绝的字符：字母、下划线（说明是某个标识符的一部分）
		if (prevChar >= 'a' && prevChar <= 'z') ||
		   (prevChar >= 'A' && prevChar <= 'Z') ||
		   prevChar == '_' {
			return false
		}
		// 数字、分隔符和不可打印字符都允许
	}
	
	// 检查后面的字符
	if pos+length < len(data) {
		nextChar := data[pos+length]
		// 放宽检查：只拒绝明确是标识符一部分的字符
		// 拒绝的字符：字母、数字、下划线（说明是某个标识符的一部分）
		// 连字符不拒绝，因为包路径后面可能跟版本号
		if (nextChar >= 'a' && nextChar <= 'z') ||
		   (nextChar >= 'A' && nextChar <= 'Z') ||
		   (nextChar >= '0' && nextChar <= '9') ||
		   nextChar == '_' {
			return false
		}
		// 其他字符都允许（包括 /、.、-、空格、null、引号等）
	}
	
	return true
}



// replaceBytesStrict 严格的字节替换，用于包路径替换
func (lo *LinkerObfuscator) replaceBytesStrict(data []byte, pattern []byte, replacement []byte) int {
	if len(replacement) > len(pattern) {
		return 0
	}
	
	// 额外检查：pattern 必须足够长（至少 10 个字符）
	// 避免替换短字符串
	if len(pattern) < 10 {
		return 0
	}
	
	count := 0
	for i := 0; i < len(data)-len(pattern); i++ {
		if bytes.Equal(data[i:i+len(pattern)], pattern) {
			// 更严格的安全检查
			if !lo.isSafeToReplaceStrict(data, i, len(pattern)) {
				continue
			}
			
			// 执行替换
			copy(data[i:i+len(replacement)], replacement)
			for j := i + len(replacement); j < i+len(pattern); j++ {
				data[j] = 0
			}
			count++
			i += len(pattern) - 1
		}
	}
	
	return count
}

// isSafeToReplaceStrict 更严格的安全检查
func (lo *LinkerObfuscator) isSafeToReplaceStrict(data []byte, pos int, length int) bool {
	// 首先使用基本的安全检查
	if !lo.isSafeToReplace(data, pos, length) {
		return false
	}
	
	// 额外检查：确保前后都是合理的字符
	// 检查前一个字符
	if pos > 0 {
		prevChar := data[pos-1]
		// 如果前一个字符是字母或数字，可能是其他标识符的一部分
		if (prevChar >= 'a' && prevChar <= 'z') ||
			(prevChar >= 'A' && prevChar <= 'Z') ||
			(prevChar >= '0' && prevChar <= '9') ||
			prevChar == '_' {
			return false
		}
	}
	
	// 检查后一个字符
	if pos+length < len(data) {
		nextChar := data[pos+length]
		// 如果后一个字符是字母或数字，可能是其他标识符的一部分
		if (nextChar >= 'a' && nextChar <= 'z') ||
			(nextChar >= 'A' && nextChar <= 'Z') ||
			(nextChar >= '0' && nextChar <= '9') ||
			nextChar == '_' {
			return false
		}
	}
	
	return true
}

// isSafeToReplace 检查是否可以安全地替换该位置的字节
// 避免破坏系统路径（如 /System/Library/Frameworks/...）
func (lo *LinkerObfuscator) isSafeToReplace(data []byte, pos int, length int) bool {
	// 检查前面的上下文（最多往前看 100 字节）
	contextStart := pos - 100
	if contextStart < 0 {
		contextStart = 0
	}
	
	contextBefore := data[contextStart:pos]
	
	// 危险模式：系统路径
	dangerousPatterns := [][]byte{
		[]byte("/System/Library/"),
		[]byte("/usr/lib/"),
		[]byte("/usr/local/"),
		[]byte("Library/Frameworks/"),
		[]byte(".framework/"),
		[]byte(".dylib"),
		[]byte("/Cryptexes/"),
	}
	
	// 检查前面的上下文是否包含危险模式
	for _, dangerous := range dangerousPatterns {
		if bytes.Contains(contextBefore, dangerous) {
			return false
		}
	}
	
	// 检查后面的上下文（最多往后看 50 字节）
	contextEnd := pos + length + 50
	if contextEnd > len(data) {
		contextEnd = len(data)
	}
	
	contextAfter := data[pos+length : contextEnd]
	
	// 检查后面是否紧跟着系统路径特征
	afterDangerousPatterns := [][]byte{
		[]byte(".framework"),
		[]byte(".dylib"),
	}
	
	for _, dangerous := range afterDangerousPatterns {
		if bytes.HasPrefix(contextAfter, dangerous) {
			return false
		}
	}
	
	return true
}

// discoverAndGeneratePackageReplacements 自动发现项目包名并生成替换映射
func (lo *LinkerObfuscator) discoverAndGeneratePackageReplacements() error {
	var packages []string
	var err error
	var moduleName string

	// 尝试从项目目录发现包名 (即使是默认的 "." 目录)
	if lo.projectDir != "" {
		// 1. 读取 go.mod 获取模块名
		moduleName, err = lo.getModuleName()
		if err == nil && moduleName != "" {
			fmt.Printf("   发现源码模块名: %s\n", moduleName)
			// 2. 扫描项目目录，查找所有子包
			packages, err = lo.discoverProjectPackages(moduleName)
			if err == nil && len(packages) > 0 {
				fmt.Printf("   从源码成功发现 %d 个项目包，将跳过二进制分析以确保准确度\n", len(packages))
			}
		}
	}

	// 如果没有提供源码目录，或者从源码中未发现包，则尝试从二进制文件发现
	if len(packages) == 0 {
		// 如果从 go.mod 中读到了模块名，自动作为关键字过滤
		filterKey := lo.config.PackageFilter
		if filterKey == "" && moduleName != "" {
			fmt.Printf("   [i] 未指定过滤关键字，自动使用从 go.mod 提取的模块名 '%s' 作为二进制扫描关键字\n", moduleName)
			filterKey = moduleName
		}

		if filterKey == "" {
			return fmt.Errorf("无源码模式下自动发现包名必须通过 -pkg-filter 指定过滤关键字，或者在包含 go.mod 的目录下运行")
		}

		fmt.Printf("   [!] 未提供源码或源码分析失败，正在从二进制文件中通过关键字 '%s' 自动发现包名...\n", filterKey)
		binData, err := os.ReadFile(lo.outputBin)
		if err != nil {
			return fmt.Errorf("无法读取二进制文件: %v", err)
		}

		// 临时将 config.PackageFilter 设为 filterKey，以便后续逻辑使用
		lo.config.PackageFilter = filterKey

		packages, err = lo.discoverPackagesFromBinary(binData)
		if err != nil {
			return err
		}
		fmt.Printf("   从二进制发现 %d 个匹配关键字的包名\n", len(packages))
	}

	// 应用过滤 (如果指定了 PackageFilter)
	if lo.config.PackageFilter != "" {
		filters := lo.getFilters()
		filtered := make([]string, 0)
		for _, pkg := range packages {
			match := false
			for _, f := range filters {
				if strings.Contains(pkg, f) {
					match = true
					break
				}
			}
			if match {
				filtered = append(filtered, pkg)
			}
		}
		packages = filtered
		fmt.Printf("   应用过滤器 '%s' 后保留了 %d 个项目包\n", lo.config.PackageFilter, len(packages))
	}

	if len(packages) == 0 {
		return fmt.Errorf("无法自动发现匹配过滤器 '%s' 的包名", lo.config.PackageFilter)
	}
	
	fmt.Printf("   发现 %d 个项目包:\n", len(packages))
	for _, pkg := range packages {
		fmt.Printf("     - %s\n", pkg)
	}
	
	// 3. 添加常见的标准库包名
	standardPackages := lo.getStandardPackages()
	
	// 4. 合并项目包和标准库包
	allPackages := append(packages, standardPackages...)
	
	// 5. 如果启用第三方包混淆，发现并添加第三方包
	var thirdPartyPackages []string
	if lo.config.ObfuscateThirdParty && moduleName != "" {
		thirdPartyPackages, err = lo.discoverThirdPartyPackages(moduleName)
		if err != nil {
			fmt.Printf("   [!] 发现第三方包失败: %v\n", err)
		} else {
			fmt.Printf("   发现 %d 个第三方包（包括子包）:\n", len(thirdPartyPackages))
			// 显示前 10 个包
			displayCount := 10
			if len(thirdPartyPackages) < displayCount {
				displayCount = len(thirdPartyPackages)
			}
			for i := 0; i < displayCount; i++ {
				fmt.Printf("     - %s\n", thirdPartyPackages[i])
			}
			if len(thirdPartyPackages) > displayCount {
				fmt.Printf("     - ... 还有 %d 个\n", len(thirdPartyPackages)-displayCount)
			}
			allPackages = append(allPackages, thirdPartyPackages...)
		}
	}
	
	// 6. 生成替换映射
	replacements := lo.generateReplacements(allPackages)
	
	// 7. 应用替换映射
	lo.config.PackageReplacements = replacements
	
	if lo.config.ObfuscateThirdParty {
		fmt.Printf("   [OK] 生成了 %d 个包名替换映射 (项目包: %d, 标准库: %d, 第三方: %d)\n", 
			len(replacements), len(packages), len(standardPackages), len(thirdPartyPackages))
	} else {
		fmt.Printf("   [OK] 生成了 %d 个包名替换映射 (项目包: %d, 标准库: %d)\n", 
			len(replacements), len(packages), len(standardPackages))
	}
	
	return nil
}

// getStandardPackages 返回常见的标准库包名列表
// 这些包名是经过验证的，替换后不会影响程序运行
// 只替换函数名前缀（如 "fmt."），不替换包路径本身（如 "fmt"）
func (lo *LinkerObfuscator) getStandardPackages() []string {
	return []string{
		// 核心运行时（安全）
		"main",
		"runtime",
		"sync",
		"syscall",  // 系统调用，函数名前缀可以安全替换
		
		// I/O 和格式化（安全）
		"fmt",
		"io",
		"bufio",
		"os",
		"log",
		
		// 网络相关（安全）
		"net",
		"http",  // 实际是 net/http，但在符号表中可能显示为 http
		
		// 字符串和数据处理（安全）
		"strings",
		"bytes",
		"strconv",
		"unicode",
		"regexp",
		
		// 编码（安全）
		"encoding",
		"json",    // encoding/json
		"xml",     // encoding/xml
		"base64",  // encoding/base64
		"hex",     // encoding/hex
		
		// 时间和数学（安全）
		"time",
		"math",
		
		// 容器和算法（安全）
		"sort",
		"container",
		"list",
		"heap",
		
		// 路径处理（安全）
		"path",
		"filepath",
		
		// 错误处理（安全）
		"errors",
		
		// 上下文（安全）
		"context",
		
		// 压缩（安全）
		"compress",
		"gzip",
		"zlib",
		
		// 哈希（安全）
		"hash",
		"crc32",
		"crc64",
		"fnv",
		
		// 注意：以下包不包含，因为可能影响程序
		// - reflect: 反射包，可能依赖包名
		// - unsafe: 不安全操作
		// - crypto/*: 加密包，某些实现可能依赖包名
		// - runtime/debug: 调试相关
		// - plugin: 插件系统
	}
}

// getModuleName 从 go.mod 文件中读取模块名
func (lo *LinkerObfuscator) getModuleName() (string, error) {
	goModPath := filepath.Join(lo.projectDir, "go.mod")
	
	data, err := ioutil.ReadFile(goModPath)
	if err != nil {
		return "", err
	}
	
	// 使用正则表达式匹配 module 行
	re := regexp.MustCompile(`(?m)^module\s+([^\s]+)`)
	matches := re.FindSubmatch(data)
	
	if len(matches) < 2 {
		return "", fmt.Errorf("go.mod 中未找到 module 声明")
	}
	
	return string(matches[1]), nil
}

// discoverProjectPackages 扫描项目目录，发现所有子包
func (lo *LinkerObfuscator) discoverProjectPackages(moduleName string) ([]string, error) {
	packages := make(map[string]bool)
	
	// 添加主模块
	packages[moduleName] = true
	
	// 遍历项目目录
	err := filepath.Walk(lo.projectDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		
		// 跳过非目录
		if !info.IsDir() {
			return nil
		}
		
		// 跳过隐藏目录、vendor、测试数据等
		dirName := info.Name()
		if strings.HasPrefix(dirName, ".") || 
		   dirName == "vendor" || 
		   dirName == "testdata" ||
		   dirName == "node_modules" {
			return filepath.SkipDir
		}
		
		// 检查目录中是否有 .go 文件
		hasGoFiles, err := lo.hasGoFiles(path)
		if err != nil || !hasGoFiles {
			return nil
		}
		
		// 计算相对路径
		relPath, err := filepath.Rel(lo.projectDir, path)
		if err != nil {
			return nil
		}
		
		// 跳过根目录（已经添加了主模块）
		if relPath == "." {
			return nil
		}
		
		// 构建完整包路径
		pkgPath := moduleName + "/" + filepath.ToSlash(relPath)
		packages[pkgPath] = true
		
		return nil
	})
	
	if err != nil {
		return nil, err
	}
	
	// 转换为切片并排序（长的包名在前，避免替换冲突）
	result := make([]string, 0, len(packages))
	for pkg := range packages {
		result = append(result, pkg)
	}
	
	// 按长度降序排序，确保子包先被替换
	for i := 0; i < len(result); i++ {
		for j := i + 1; j < len(result); j++ {
			if len(result[j]) > len(result[i]) {
				result[i], result[j] = result[j], result[i]
			}
		}
	}
	
	return result, nil
}

// hasGoFiles 检查目录中是否有 .go 文件
func (lo *LinkerObfuscator) hasGoFiles(dir string) (bool, error) {
	files, err := ioutil.ReadDir(dir)
	if err != nil {
		return false, err
	}
	
	for _, file := range files {
		if !file.IsDir() && strings.HasSuffix(file.Name(), ".go") {
			// 排除测试文件
			if !strings.HasSuffix(file.Name(), "_test.go") {
				return true, nil
			}
		}
	}
	
	return false, nil
}

// generateReplacements 为包名生成短替换名
func (lo *LinkerObfuscator) generateReplacements(packages []string) map[string]string {
	replacements := make(map[string]string)
	
	// 生成简短的替换名
	counter := 0
	for _, pkg := range packages {
		// 生成替换名: a, b, c, ..., z, aa, ab, ...
		replacement := lo.generateShortName(counter)
		replacements[pkg] = replacement
		counter++
	}
	
	return replacements
}

// generateShortName 生成短名称 (a, b, c, ..., z, aa, ab, ...)
func (lo *LinkerObfuscator) generateShortName(index int) string {
	if index < 26 {
		return string(rune('a' + index))
	}
	
	// 对于超过26的，使用两个字母
	first := index / 26 - 1
	second := index % 26
	return string(rune('a'+first)) + string(rune('a'+second))
}

// isSafeFunctionNamePrefix 检查是否是安全的函数名前缀位置
// 在 pclntab 中，函数名前缀通常前面是：
// 1. 空字节 (\x00)
// 2. 不可打印字符
// 3. 路径分隔符后的完整包名
func (lo *LinkerObfuscator) isSafeFunctionNamePrefix(data []byte, pos int, pattern []byte) bool {
	// 检查前一个字符
	if pos > 0 {
		prevChar := data[pos-1]
		
		// 允许的前置字符：
		// - 空字节 (0x00)
		// - 不可打印字符 (< 0x20，除了空格)
		// - 路径分隔符 (/)
		// 
		// 不允许的前置字符：
		// - 字母、数字（说明是某个标识符的一部分）
		// - 点号（说明是包路径的一部分，如 commons.io.）
		// - 其他可打印字符（说明可能是文本内容）
		
		if prevChar == 0 {
			// 空字节，安全
			return true
		}
		
		if prevChar < 0x20 && prevChar != ' ' {
			// 不可打印字符（除了空格），安全
			return true
		}
		
		// 如果是字母、数字、点号、斜杠、下划线、连字符，不安全
		if (prevChar >= 'a' && prevChar <= 'z') ||
		   (prevChar >= 'A' && prevChar <= 'Z') ||
		   (prevChar >= '0' && prevChar <= '9') ||
		   prevChar == '.' ||
		   prevChar == '/' ||
		   prevChar == '_' ||
		   prevChar == '-' {
			return false
		}
	}
	
	// 检查后一个字符（在点号之后）
	// 函数名前缀后面应该是大写字母（导出函数）或小写字母（未导出函数）
	dotPos := bytes.IndexByte(pattern, '.')
	if dotPos >= 0 && pos+len(pattern) < len(data) {
		nextChar := data[pos+len(pattern)]
		
		// 函数名后面通常是：
		// - 大写或小写字母（函数名开始）
		// - 空字节（字符串结束）
		if nextChar == 0 {
			return true
		}
		
		if (nextChar >= 'a' && nextChar <= 'z') ||
		   (nextChar >= 'A' && nextChar <= 'Z') {
			return true
		}
		
		// 其他字符，可能不是函数名
		return false
	}
	
	return true
}

// discoverThirdPartyPackages 发现第三方依赖包
func (lo *LinkerObfuscator) discoverThirdPartyPackages(moduleName string) ([]string, error) {
	packages := make(map[string]bool)
	
	// 读取 go.mod 文件
	goModPath := filepath.Join(lo.projectDir, "go.mod")
	data, err := ioutil.ReadFile(goModPath)
	if err != nil {
		return nil, err
	}
	
	// 改进的正则表达式：匹配所有 require 行（包括 indirect 注释）
	// 匹配格式: github.com/xxx/yyy v1.2.3 或 github.com/xxx/yyy v1.2.3 // indirect
	requireRe := regexp.MustCompile(`(?m)^\s*([a-zA-Z0-9\-_\.]+/[a-zA-Z0-9\-_\./]+?)\s+v[^\s]+`)
	requireMatches := requireRe.FindAllSubmatch(data, -1)
	
	// 同时匹配 replace 指令，格式: replace github.com/xxx/yyy => github.com/aaa/bbb v1.2.3
	replaceRe := regexp.MustCompile(`(?m)^replace\s+([a-zA-Z0-9\-_\.]+/[a-zA-Z0-9\-_\./]+)\s+[^\n]*=>\s+([a-zA-Z0-9\-_\.]+/[a-zA-Z0-9\-_\./]+)\s+v`)
	replaceMatches := replaceRe.FindAllSubmatch(data, -1)
	
	// 处理 require 的包
	for _, match := range requireMatches {
		if len(match) >= 2 {
			pkgPath := string(match[1])
			
			// 排除标准库（不包含域名）
			if !strings.Contains(pkgPath, ".") {
				continue
			}
			
			// 排除项目自身
			if strings.HasPrefix(pkgPath, moduleName) {
				continue
			}
			
			packages[pkgPath] = true
			
			// 对于有子包的路径（如 github.com/antlr/antlr4/runtime/Go/antlr）
			// 也添加其父级路径，以便能匹配更多变体
			parts := strings.Split(pkgPath, "/")
			if len(parts) > 3 {
				// 添加顶层包路径，例如 github.com/antlr/antlr4
				topLevel := strings.Join(parts[:3], "/")
				packages[topLevel] = true
				
				// 添加中间路径，例如 github.com/antlr/antlr4/runtime
				for i := 3; i < len(parts); i++ {
					midLevel := strings.Join(parts[:i+1], "/")
					packages[midLevel] = true
				}
			}
			
			// 添加可能的 internal 子包路径
			// 例如: github.com/xxx/yyy/internal, github.com/xxx/yyy/internal/pkg
			lo.addCommonSubPackages(pkgPath, packages)
		}
	}
	
	// 处理 replace 指令中的包（原始包和替换后的包都添加）
	for _, match := range replaceMatches {
		if len(match) >= 3 {
			originalPkg := string(match[1])
			replacementPkg := string(match[2])
			
			// 处理原始包
			if strings.Contains(originalPkg, ".") && !strings.HasPrefix(originalPkg, moduleName) {
				packages[originalPkg] = true
				parts := strings.Split(originalPkg, "/")
				if len(parts) > 3 {
					for i := 3; i <= len(parts); i++ {
						packages[strings.Join(parts[:i], "/")] = true
					}
				}
				lo.addCommonSubPackages(originalPkg, packages)
			}
			
			// 处理替换后的包
			if strings.Contains(replacementPkg, ".") && !strings.HasPrefix(replacementPkg, moduleName) {
				packages[replacementPkg] = true
				parts := strings.Split(replacementPkg, "/")
				if len(parts) > 3 {
					for i := 3; i <= len(parts); i++ {
						packages[strings.Join(parts[:i], "/")] = true
					}
				}
				lo.addCommonSubPackages(replacementPkg, packages)
			}
		}
	}
	
	// 转换为切片并排序
	result := make([]string, 0, len(packages))
	for pkg := range packages {
		result = append(result, pkg)
	}
	
	// 按长度降序排序（确保子包在前，避免替换冲突）
	sort.Slice(result, func(i, j int) bool {
		return len(result[i]) > len(result[j])
	})
	
	return result, nil
}

// addCommonSubPackages 添加常见的子包路径（如 internal, pkg, cmd 等）
func (lo *LinkerObfuscator) addCommonSubPackages(basePath string, packages map[string]bool) {
	// 常见的子包目录名
	commonSubDirs := []string{
		"internal",
		"pkg",
		"cmd",
		"api",
		"lib",
		"core",
		"common",
		"util",
		"utils",
		"proto",
		"protobuf",
	}
	
	// 为基础路径添加常见子目录
	for _, subDir := range commonSubDirs {
		subPath := basePath + "/" + subDir
		packages[subPath] = true
		
		// 对于 internal，还要添加一些常见的更深层路径
		if subDir == "internal" {
			internalCommon := []string{
				"spnego",
				"decimal",
				"querytext",
				"crypto",
				"crypto/ccm",
				"crypto/cmac",
				"x",
				"x/crypto",
				"x/crypto/cryptobyte",
			}
			for _, inner := range internalCommon {
				packages[subPath+"/"+inner] = true
			}
		}
	}
}

// discoverPackagesFromBinary 通过扫描二进制文件中的 pclntab 发现项目包名
func (lo *LinkerObfuscator) discoverPackagesFromBinary(data []byte) ([]string, error) {
	// 查找 pclntab
	offset := findPclntabMagic(data)
	if offset < 0 {
		return nil, fmt.Errorf("未在二进制中找到 pclntab 结构")
	}

	// 搜索二进制中的所有字符串，通过启发式方法识别包路径
	// 匹配规则：包含 "/"，且在 "." 之前的标识符看起来像包路径
	// 示例：github.com/user/project.(*Type).Method -> 提取 github.com/user/project
	
	pkgMap := make(map[string]bool)
	stdLibs := lo.getStandardPackagesMap()

	// 简单的扫描所有可能是包名的字符串
	// 包名特征：[a-zA-Z0-9._/-]+
	re := regexp.MustCompile(`[a-zA-Z0-9_\-.]+(/[a-zA-Z0-9_\-.]+)+`)
	matches := re.FindAll(data, -1)

	for _, m := range matches {
		s := string(m)
		
		// 过滤掉已知的标准库路径特征
		if lo.isProbablyStdLib(s, stdLibs) {
			continue
		}

		// 包名必须包含用户指定的过滤关键字之一
		filters := lo.getFilters()
		match := false
		for _, f := range filters {
			if strings.Contains(s, f) {
				match = true
				break
			}
		}
		if !match {
			continue
		}

		// 提取基本包路径（截断到第一个括弧、点号或空格之前的最后一个斜杠部分）
		// 实际上，我们要的是类似 "github.com/xxx/yyy" 这样的路径
		pkgPath := s
		
		// 如果包含点号，可能带有了函数名，尝试截断
		if dotIdx := strings.LastIndex(pkgPath, "."); dotIdx > 0 {
			pkgPath = pkgPath[:dotIdx]
		}
		
		// 再次验证
		if len(pkgPath) > 3 && strings.Contains(pkgPath, "/") && !stdLibs[pkgPath] {
			pkgMap[pkgPath] = true
			
			// 同时添加其父路径
			parts := strings.Split(pkgPath, "/")
			if len(parts) > 3 {
				// 添加顶层包路径，例如 github.com/antlr/antlr4
				topLevel := strings.Join(parts[:3], "/")
				pkgMap[topLevel] = true
				
				// 添加中间路径，例如 github.com/antlr/antlr4/runtime
				for i := 3; i < len(parts); i++ {
					midLevel := strings.Join(parts[:i], "/")
					pkgMap[midLevel] = true
				}
			}
		}
	}

	if len(pkgMap) == 0 {
		return nil, fmt.Errorf("二进制分析未发现任何项目包")
	}

	result := make([]string, 0, len(pkgMap))
	for pkg := range pkgMap {
		result = append(result, pkg)
	}

	// 按长度降序排序
	sort.Slice(result, func(i, j int) bool {
		return len(result[i]) > len(result[j])
	})

	return result, nil
}

// getStandardPackagesMap 返回标准库包名的 Map 形式以便快速查找
func (lo *LinkerObfuscator) getStandardPackagesMap() map[string]bool {
	pkgs := lo.getStandardPackages()
	m := make(map[string]bool)
	for _, p := range pkgs {
		m[p] = true
	}
	// 补充一些常见的标准库前缀
	m["runtime"] = true
	m["reflect"] = true
	m["internal"] = true
	m["vendor"] = true
	m["google.golang.org"] = true
	m["golang.org"] = true
	return m
}

// isProbablyStdLib 启发式判断是否为标准库或系统路径
func (lo *LinkerObfuscator) isProbablyStdLib(s string, stdLibs map[string]bool) bool {
	if strings.HasPrefix(s, "runtime/") || 
	   strings.HasPrefix(s, "internal/") || 
	   strings.HasPrefix(s, "reflect/") ||
	   strings.HasPrefix(s, "sync/") ||
	   strings.HasPrefix(s, "syscall/") ||
	   strings.HasPrefix(s, "net/") ||
	   strings.HasPrefix(s, "os/") ||
	   strings.HasPrefix(s, "time/") {
		return true
	}
	
	for std := range stdLibs {
		if strings.HasPrefix(s, std+"/") || s == std {
			return true
		}
	}
	
	// 排除绝对路径（可能是编译环境路径）
	if strings.HasPrefix(s, "/") || (len(s) > 2 && s[1] == ':') {
		return true
	}

	return false
}

// getFilters 将逗号分隔的过滤关键字拆分为切片并修剪空格
func (lo *LinkerObfuscator) getFilters() []string {
	if lo.config.PackageFilter == "" {
		return nil
	}
	parts := strings.Split(lo.config.PackageFilter, ",")
	result := make([]string, 0, len(parts))
	for _, p := range parts {
		trimmed := strings.TrimSpace(p)
		if trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}
