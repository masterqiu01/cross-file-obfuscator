package obfuscator

import (
	"bytes"
	"debug/elf"
	"debug/macho"
	"debug/pe"
	"encoding/binary"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"

	"cross-file-obfuscator/internal/logger"
)

// Go pclntab magic values
// standardLibraryNames 标准库包名集合（函数名前缀可安全替换）
var standardLibraryNames = map[string]bool{
	"main": true, "runtime": true, "sync": true, "fmt": true,
	"os": true, "io": true, "net": true, "http": true,
	"bufio": true, "bytes": true, "strings": true, "strconv": true,
	"time": true, "math": true, "errors": true, "context": true,
	"encoding": true, "json": true, "xml": true, "base64": true,
	"hex": true, "unicode": true, "regexp": true, "log": true,
	"sort": true, "path": true, "filepath": true, "syscall": true,
}

// LinkerObfuscator 处理链接器级别的混淆
type LinkerObfuscator struct {
	config     *LinkConfig
	projectDir string
	outputBin  string

	// buildInfoEnd 记录 Go buildinfo 段在文件中的结束偏移（0 表示未知）。
	// 由 processMachO/processELF 解析段表时填入，用于限制 obfuscateBuildInfo
	// 的扫描范围，避免越界改写 __go_buildinfo 段之后的其他 __DATA 数据
	// （如 __itablink、运行时元数据），否则程序启动时会崩溃。
	buildInfoEnd int
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

// ObfuscateExistingBinary 直接混淆现有的二进制文件
func (lo *LinkerObfuscator) ObfuscateExistingBinary(binPath string) error {
	logger.Infof("=== 链接器级别混淆 (直接对二进制进行修改) ===")

	lo.outputBin = binPath

	// 读一次二进制数据，供发现阶段与混淆阶段复用，避免大文件被重复读入内存
	// （发现阶段枚举第三方子包、无源码模式发现包名各会再次读文件）。
	binData, err := os.ReadFile(binPath)
	if err != nil {
		return fmt.Errorf("无法读取二进制文件: %v", err)
	}

	// 如果启用了自动包名发现且没有手动指定包名替换
	if lo.config.AutoDiscoverPackages && len(lo.config.PackageReplacements) == 0 {
		logger.Infof("第 0 步: 自动发现项目包名...")
		if err := lo.discoverAndGeneratePackageReplacements(binData); err != nil {
			logger.Warnf("自动发现包名失败: %v", err)
			logger.Warnf("将继续使用默认包名替换模式")
		}
	}

	logger.Infof("第 1 步: 开始修改二进制文件...")
	if err := lo.postProcessBinary(binData); err != nil {
		return fmt.Errorf("混淆失败: %v", err)
	}
	logger.Infof("[OK] 混淆完成")

	return nil
}

// postProcessBinary 后处理二进制文件（data 已由调用方读入，原地修改）
func (lo *LinkerObfuscator) postProcessBinary(data []byte) error {
	// 检测二进制格式
	format := detectBinaryFormat(data)
	logger.Debugf("检测到二进制格式: %s", format)

	if format != "ELF" && format != "PE" && format != "Mach-O" {
		return fmt.Errorf("不支持的二进制格式: %s", format)
	}

	// 原地修改前先备份原始数据（避免额外复制一份全文件到 newData）
	backupPath := lo.outputBin + ".backup"
	if err := os.WriteFile(backupPath, data, 0755); err != nil {
		return fmt.Errorf("备份失败: %v", err)
	}

	var modified bool
	var err error

	switch format {
	case "ELF":
		modified, err = lo.processELF(data)
	case "PE":
		modified, err = lo.processPE(data)
	case "Mach-O":
		modified, err = lo.processMachO(data)
	}

	if err != nil {
		return err
	}

	if lo.config.ObfuscateFilePaths {
		pathCount := lo.obfuscateFilePaths(data)
		if pathCount > 0 {
			logger.Infof("[OK] 混淆了 %d 个源代码文件路径", pathCount)
			modified = true
		}
	}

	// 混淆 Go buildinfo 段中残留的依赖版本/hash/构建环境信息
	biCount := lo.obfuscateBuildInfo(data)
	if biCount > 0 {
		logger.Infof("[OK] 混淆了 %d 个 BuildInfo 元数据项", biCount)
		modified = true
	}

	if modified {
		// 写入修改后的文件
		if err := os.WriteFile(lo.outputBin, data, 0755); err != nil {
			return fmt.Errorf("写入失败: %v", err)
		}

		logger.Infof("[OK] 已修改 pclntab")
		logger.Infof("[OK] 原文件已备份到: %s", backupPath)

		// macOS 系统修改二进制后会破坏签名，导致运行直接被杀(SIGKILL)，因此尝试自动重签名
		if runtime.GOOS == "darwin" {
			logger.Infof("正在尝试对 macOS 二进制文件重新签名...")
			cmd := exec.Command("codesign", "-s", "-", "-f", lo.outputBin)
			if err := cmd.Run(); err != nil {
				logger.Warnf("自动签名失败 (%v)。请手动运行: codesign -s - -f %s", err, lo.outputBin)
			} else {
				logger.Infof("[OK] 已修复 macOS 代码签名")
			}
		}
	} else {
		// 未发生任何修改：outputBin 未被改动，备份与其内容相同。
		// 不删除备份，避免误删用户已有的 .backup 文件（虽然本次写入会覆盖它）。
		logger.Warnf("未找到 pclntab 或无需修改")
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

		// 过滤伪路径：URL（https://...//host/path）等会被正则误匹配为
		// "//xxx.go" 的伪 .go 路径。它们本身不是源代码文件路径，且可能落在
		// 嵌入的 yaml/json 等数据区，篡改后会导致程序运行时解析失败。
		if !isLikelyGoFilePath(path) {
			continue
		}

		// 使用不可见的高位 ASCII 字符替换，彻底避开 strings 提取
		// 这样既保持了等长，又使字符串失去了可打印特性
		for j := start; j < end-3; j++ {
			c := data[j]
			if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') {
				// 替换为 0x80 到 0xFE 之间的高位不可见/乱码字节
				data[j] = byte(rng.IntN(127) + 128)
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
		for j := start; j < end; j++ {
			c := data[j]
			if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') {
				data[j] = byte(rng.IntN(127) + 128)
			}
		}
		count++
	}

	return count
}

// obfuscateBuildInfo 混淆二进制中残留的 Go buildinfo 段（__go_buildinfo）。
// Go 工具链会把构建时所有依赖模块的路径/版本/content hash（h1:...）以及
// 构建环境（GOOS/GOARCH/编译器/GODEBUG 等）原样写入该段。其中 h1: 模块 hash
// 是依赖源码的内容指纹，攻击者可据此精确反推整棵依赖树。这里把所有可读的
// metadata 替换为等长的随机可读串，保留段布局以便程序正常启动。
func (lo *LinkerObfuscator) obfuscateBuildInfo(data []byte) int {
	// Go buildinfo 的魔数签名：\xff Go buildinf: <magic>
	sig := []byte{'G', 'o', ' ', 'b', 'u', 'i', 'l', 'd', 'i', 'n', 'f', ':'}
	idx := -1
	for i := 0; i <= len(data)-len(sig); i++ {
		if data[i] == 0xff && i+14 <= len(data) && bytes.Equal(data[i+2:i+14], sig) {
			idx = i + 2
			break
		}
	}
	if idx < 0 {
		return 0
	}

	// buildinfo 段一般很小（几 KB），从签名后开始扫描可读串
	// 保留签名本身与结构分隔符（\t \x00 等），只替换内容。
	// 若解析段表时已确认 __go_buildinfo/.go.buildinfo 段的边界，
	// 严格限制在该段内扫描，防止越界改写后续 __DATA 数据导致程序崩溃。
	const maxScan = 64 * 1024
	end := idx + maxScan
	if end > len(data) {
		end = len(data)
	}
	if lo.buildInfoEnd > idx && end > lo.buildInfoEnd {
		end = lo.buildInfoEnd
	}

	nameGen := NewNaturalNameGenerator()
	count := 0
	// 从签名之后开始扫描，保留 "Go buildinf:" 魔数本身（后续的 \x08 magic 与
	// 版本串 go1.x 等才属于待混淆的元数据）。
	j := idx + len(sig)
	for j < end {
		c := data[j]
		if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') {
			// 找到一段连续可读串
			start := j
			for j < end {
				c := data[j]
				// 可读字符集刻意排除 ':' 与 '='：':' 是 h1: 哈希前缀的分隔符，
				// '=' 是 key=value 结构及 base64 padding 的分隔符。若把它们一并
				// 替换会破坏 buildinfo 结构，导致 debug.ReadBuildInfo 解析失败。
				if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '.' || c == '/' || c == '_' || c == '-' || c == '+' || c == '@' {
					j++
				} else {
					break
				}
			}
			strLen := j - start
			// 阈值取 6：最短的语义化版本号 vX.Y.Z 恰好 6 字符（如 v1.1.0），
			// 而 buildinfo 的结构标签最长仅 5 字符（build），必须保留以便
			// debug.ReadBuildInfo 正常解析。用 6 既能覆盖所有依赖版本号，
			// 又不会误改结构标签。
			if strLen >= 6 {
				repl := nameGen.GeneratePackageName(string(data[start:j]), strLen)
				copy(data[start:j], repl)
				count++
			}
			continue
		}
		j++
	}

	// 额外处理：Go 链接器会把 buildinfo 字符串额外复制到 __TEXT,__rodata
	// （作为 runtime 引用的字符串常量）。这份副本不含 "Go buildinf:" 签名，
	// 上面的段内扫描（受 buildInfoEnd 限定）无法覆盖，导致其中的 h1 依赖
	// content hash 与版本号仍以明文残留。这里全局扫描这两类特征补齐。
	count += lo.obfuscateBuildInfoCopies(data, idx, end)

	return count
}

// obfuscateBuildInfoCopies 全局混淆 buildinfo 段之外的副本（如 __TEXT,__rodata）。
// 只处理两类特征：\th1: 后的 base64 content hash、\tvX.Y.Z 依赖版本号。
// skipStart/skipEnd 为 __go_buildinfo 段已处理范围，避免重复替换。
func (lo *LinkerObfuscator) obfuscateBuildInfoCopies(data []byte, skipStart, skipEnd int) int {
	nameGen := NewNaturalNameGenerator()
	count := 0

	// 1. 扫描 "\th1:" 特征，替换其后的 base64 content hash（保留 h1: 前缀与 = padding）
	h1 := []byte("h1:")
	for pos := 0; pos <= len(data)-len(h1); {
		i := bytes.Index(data[pos:], h1)
		if i < 0 {
			break
		}
		j := pos + i
		pos = j + len(h1)

		if j >= skipStart && j < skipEnd {
			continue
		}
		// 前置必须是 \t（buildinfo 中 h1 紧跟制表符），降低误伤普通数据
		if j == 0 || data[j-1] != '\t' {
			continue
		}
		start := j + len(h1)
		e := start
		for e < len(data) {
			c := data[e]
			if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '+' || c == '/' {
				e++
			} else {
				break
			}
		}
		if e-start >= 6 {
			repl := nameGen.GeneratePackageName(string(data[start:e]), e-start)
			copy(data[start:e], repl)
			count++
		}
	}

	// 2. 扫描 "\tv<digit>" 特征，替换依赖版本号（semver 或 pseudo-version）
	tabV := []byte{'\t', 'v'}
	for pos := 0; pos <= len(data)-len(tabV); {
		i := bytes.Index(data[pos:], tabV)
		if i < 0 {
			break
		}
		j := pos + i
		pos = j + len(tabV)

		if j >= skipStart && j < skipEnd {
			continue
		}
		// 从 'v' 本身开始（j+1），保证 vX.Y.Z 整体满足 >= 6 阈值。
		// 版本号必须同时满足：
		//   1. v 后紧跟数字（避免误伤 \tvalue/\tvariable 等普通文本）
		//   2. 扫描区间内必须包含 '.'（真实 semver v1.2.3 / pseudo-version 必有，
		//      而代码段指令巧合形成的 \t v <数字> 后不跟 '.'，如 09 76 36 48...）
		//   3. 区间内只允许版本号字符（字母数字 . - +），防止代码段长串被误判
		start := j + 1
		if start+1 >= len(data) || data[start+1] < '0' || data[start+1] > '9' {
			continue
		}
		e := start
		hasDot := false
		for e < len(data) {
			c := data[e]
			if c == '\t' || c == '\n' || c == 0 {
				break
			}
			// 版本号只允许字母数字与 . - +；遇到其他字符（代码指令等）终止
			if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') ||
				(c >= '0' && c <= '9') || c == '.' || c == '-' || c == '+' {
				if c == '.' {
					hasDot = true
				}
				e++
			} else {
				break
			}
		}
		if e-start >= 6 && hasDot {
			repl := nameGen.GeneratePackageName(string(data[start:e]), e-start)
			copy(data[start:e], repl)
			count++
		}
	}

	// 3. 处理 rodata 副本中 dep/mod/path 条目里残留的无域名模块名。
	//    含 "/" 的模块名已被 replaceProjectPackagePathsGlobal 全局替换，
	//    但无域名模块名（如 grpc-c）不含 "/"，仍以明文残留，此处补齐。
	for _, tag := range [][]byte{[]byte("dep\t"), []byte("mod\t"), []byte("path\t")} {
		for pos := 0; pos <= len(data)-len(tag); {
			i := bytes.Index(data[pos:], tag)
			if i < 0 {
				break
			}
			j := pos + i
			pos = j + len(tag)

			if j >= skipStart && j < skipEnd {
				continue
			}
			// tag 前一个字符必须是分隔符（非标识符字符），避免误伤 "remod\t" 等
			if j > 0 {
				prev := data[j-1]
				if (prev >= 'a' && prev <= 'z') || (prev >= 'A' && prev <= 'Z') ||
					(prev >= '0' && prev <= '9') || prev == '.' || prev == '-' || prev == '_' {
					continue
				}
			}
			start := j + len(tag)
			e := start
			for e < len(data) {
				c := data[e]
				if c == '\t' || c == '\n' || c == 0 {
					break
				}
				e++
			}
			strLen := e - start
			if strLen >= 6 && !bytes.Contains(data[start:e], []byte("/")) {
				repl := nameGen.GeneratePackageName(string(data[start:e]), strLen)
				copy(data[start:e], repl)
				count++
			}
		}
	}

	return count
}

// isLikelyGoFilePath 判断一个被 .go 正则匹配到的字符串是否为真正的源代码文件路径，
// 防止误篡改嵌入的数据区（yaml/json）导致运行时解析失败。
func isLikelyGoFilePath(path string) bool {
	// 含 "//" 的多半是 URL（scheme://host），而真实源文件路径不会出现双斜杠
	if strings.Contains(path, "//") {
		return false
	}
	// 以 scheme: 开头的也不是文件路径（如 http:、https:、file:）
	if idx := strings.Index(path, ":"); idx >= 0 {
		// Windows 盘符如 C:\ 开头的允许（如 C:\src\main.go），其余拒绝
		if idx == 1 && len(path) > 2 && path[1] == ':' &&
			((path[0] >= 'A' && path[0] <= 'Z') || (path[0] >= 'a' && path[0] <= 'z')) {
			return true
		}
		return false
	}
	return true
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

// processELF 处理 ELF 格式的二进制文件（原地修改）
func (lo *LinkerObfuscator) processELF(data []byte) (bool, error) {
	elfFile, err := elf.NewFile(bytes.NewReader(data))
	if err != nil {
		return false, fmt.Errorf("解析 ELF 失败: %v", err)
	}
	defer elfFile.Close()

	// 在 .gopclntab 或 .data.rel.ro 段中查找 pclntab
	var candidates []pclntabSegment
	for _, section := range elfFile.Sections {
		if section.Name == ".gopclntab" || section.Name == ".data.rel.ro" {
			// 直接引用原文件数据的切片，避免 section.Data() 复制大段数据
			off := int(section.Offset)
			size := int(section.Size)
			if off >= 0 && size > 0 && off+size <= len(data) {
				candidates = append(candidates, pclntabSegment{
					name: section.Name, data: data[off : off+size], fileOffset: uint64(section.Offset),
				})
			}
		}
		// 记录 .go.buildinfo 段边界，供 obfuscateBuildInfo 限定扫描范围
		if section.Name == ".go.buildinfo" {
			lo.buildInfoEnd = int(section.Offset + section.Size)
		}
	}
	return lo.processPclntabBinary(data, candidates)
}

// processPE 处理 PE 格式的二进制文件（原地修改）
func (lo *LinkerObfuscator) processPE(data []byte) (bool, error) {
	peFile, err := pe.NewFile(bytes.NewReader(data))
	if err != nil {
		return false, fmt.Errorf("解析 PE 失败: %v", err)
	}
	defer peFile.Close()

	// 在 .rdata 或 .data 段中查找 pclntab
	var candidates []pclntabSegment
	for _, section := range peFile.Sections {
		if section.Name == ".rdata" || section.Name == ".data" {
			// 直接引用原文件数据的切片，避免 section.Data() 复制大段数据
			off := int(section.Offset)
			size := int(section.Size)
			if off >= 0 && size > 0 && off+size <= len(data) {
				candidates = append(candidates, pclntabSegment{
					name: section.Name, data: data[off : off+size], fileOffset: uint64(section.Offset),
				})
			}
		}
	}
	return lo.processPclntabBinary(data, candidates)
}

// processMachO 处理 Mach-O 格式的二进制文件（原地修改）
func (lo *LinkerObfuscator) processMachO(data []byte) (bool, error) {
	machoFile, err := macho.NewFile(bytes.NewReader(data))
	if err != nil {
		return false, fmt.Errorf("解析 Mach-O 失败: %v", err)
	}
	defer machoFile.Close()

	// 在 __gopclntab 或 __data 段中查找
	var candidates []pclntabSegment
	for _, section := range machoFile.Sections {
		if section.Name == "__gopclntab" || section.Name == "__data" {
			// 直接引用原文件数据的切片，避免 section.Data() 复制大段数据
			off := int(section.Offset)
			size := int(section.Size)
			if off >= 0 && size > 0 && off+size <= len(data) {
				candidates = append(candidates, pclntabSegment{
					name: section.Name, data: data[off : off+size], fileOffset: uint64(section.Offset),
				})
			}
		}
		// 记录 __go_buildinfo 段边界，供 obfuscateBuildInfo 限定扫描范围
		if section.Name == "__go_buildinfo" {
			lo.buildInfoEnd = int(uint64(section.Offset) + uint64(section.Size))
		}
	}
	return lo.processPclntabBinary(data, candidates)
}

// pclntabSegment 表示一个候选段（段名、数据及段在文件中的偏移）
type pclntabSegment struct {
	name       string
	data       []byte
	fileOffset uint64
}

// processPclntabBinary 在候选段中搜索 pclntab magic，找不到再全文件搜索，然后原地修改数据
func (lo *LinkerObfuscator) processPclntabBinary(data []byte, candidates []pclntabSegment) (bool, error) {
	var pclntabOffset int64 = -1

	for _, seg := range candidates {
		offset := findPclntabMagic(seg.data)
		if offset >= 0 {
			pclntabOffset = int64(seg.fileOffset) + int64(offset)
			logger.Debugf("找到 pclntab 在段 %s，偏移: 0x%x", seg.name, pclntabOffset)
			break
		}
	}

	if pclntabOffset < 0 {
		// 在整个文件中搜索
		offset := findPclntabMagic(data)
		if offset < 0 {
			return false, nil
		}
		pclntabOffset = int64(offset)
		logger.Debugf("找到 pclntab 在文件偏移: 0x%x", pclntabOffset)
	}

	return lo.modifyPclntab(data, pclntabOffset)
}

// findPclntabMagic 在数据中查找 pclntab magic value（用 bytes.Index 快速定位，
// 避免对超大文件逐字节扫描）。
func findPclntabMagic(data []byte) int {
	// 各 magic 值的小端字节序列
	magics := [][]byte{
		{0xfb, 0xff, 0xff, 0xff}, // go12magic
		{0xfa, 0xff, 0xff, 0xff}, // go116magic
		{0xf0, 0xff, 0xff, 0xff}, // go118magic
		{0xf1, 0xff, 0xff, 0xff}, // go120magic
	}

	best := -1
	for _, m := range magics {
		if i := bytes.Index(data, m); i >= 0 {
			if best < 0 || i < best {
				best = i
			}
		}
	}
	return best
}

// modifyPclntab 原地修改 pclntab 的内容
func (lo *LinkerObfuscator) modifyPclntab(data []byte, offset int64) (bool, error) {
	if offset < 0 || offset+4 > int64(len(data)) {
		return false, fmt.Errorf("无效的 pclntab 偏移")
	}

	// 读取原始 magic value（仅用于显示）
	originalMagic := binary.LittleEndian.Uint32(data[offset : offset+4])
	logger.Debugf("原始 magic value: 0x%08x", originalMagic)

	// 检查是否完全禁用 pclntab 修改
	if lo.config.DisablePclntab {
		logger.Warnf("pclntab 修改已禁用（避免杀软误报）")
		return false, nil
	}

	// 混淆函数名
	if lo.config.RemoveFuncNames {
		if err := lo.obfuscateFunctionNames(data, offset); err != nil {
			return false, fmt.Errorf("函数名混淆失败: %v", err)
		}
		logger.Infof("[OK] 已混淆函数名")
		return true, nil
	}

	return false, nil
}

// obfuscateFunctionNames 混淆二进制中的函数名（使用等长自然混淆）
func (lo *LinkerObfuscator) obfuscateFunctionNames(data []byte, pclntabOffset int64) error {
	var patterns []string
	var replacements []string

	// 创建自然名称生成器
	nameGen := NewNaturalNameGenerator()

	// 如果用户提供了自定义包名替换映射，使用它
	if len(lo.config.PackageReplacements) > 0 {
		if lo.config.OnlyObfuscateProject {
			logger.Infof("[!] 最小化混淆模式：只混淆项目包，保留标准库")
		} else {
			logger.Infof("使用自定义包名替换映射（等长模式）:")
		}

		for original, replacement := range lo.config.PackageReplacements {
			// 检查是否是标准库（main 是项目自身包，永远需要混淆）
			pkgName := strings.TrimSuffix(original, ".")
			isStdLib := pkgName != "main" && standardLibraryNames[pkgName]

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
				logger.Debugf("%s -> %s (均为 %d 字节)", originalPatternDot, replacementPatternDot, len(originalPatternDot))
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
			logger.Infof("[OK] 已过滤标准库，只混淆项目包（共 %d 个）", len(patterns))
		}
	} else {
		// 使用默认的包名替换模式（等长自然混淆）
		if lo.config.OnlyObfuscateProject {
			logger.Infof("[!] 最小化混淆模式：只混淆项目包，保留标准库（减少杀软误报）")
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
			logger.Infof("使用等长自然混淆模式（标准）")
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

		logger.Debugf("等长替换映射:")
		for i, pattern := range patterns {
			logger.Debugf("%s -> %s", pattern, replacements[i])
		}
	}

	// 按 pattern 长度降序排序，确保更长（更具体）的子包路径优先替换。
	// 原因：模块根路径（如 github.com/x/y/）与子包路径（如 github.com/x/y/sub/）
	// 同时存在时，若短的模块前缀先被替换，子包长 pattern 将无法再匹配，
	// 导致 pclntab 中残留可读的子包段名（如 /lib、/types、/antlr 等）。
	{
		type pr struct {
			pat string
			rep string
		}
		prs := make([]pr, 0, len(patterns))
		for i := range patterns {
			prs = append(prs, pr{patterns[i], replacements[i]})
		}
		sort.SliceStable(prs, func(i, j int) bool {
			if len(prs[i].pat) != len(prs[j].pat) {
				return len(prs[i].pat) > len(prs[j].pat)
			}
			return prs[i].pat < prs[j].pat
		})
		for i := range prs {
			patterns[i] = prs[i].pat
			replacements[i] = prs[i].rep
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

	// 用 Aho-Corasick 多模式匹配：单次扫描 pclntab 区域收集所有匹配位置，
	// 替代逐 pattern 的 bytes.Index 扫描（每个 pattern 各扫一遍整段数据）。
	patternBytes := make([][]byte, len(patterns))
	for i := range patterns {
		patternBytes[i] = []byte(patterns[i])
	}
	matcher := newACMatcher(patternBytes)

	type funcMatch struct {
		pos int
		idx int
	}
	var matches []funcMatch
	matcher.match(data, int(pclntabOffset), pclntabSearchEnd, func(pos, idx int) bool {
		// 过滤跨 pclntab 起点的匹配：AC 可能报告起始位置在 pclntabOffset
		// 之前的 pattern（其尾部落在搜索范围内），这类匹配不在 pclntab 区域，
		// 原逐 pattern 扫描也不会命中，需排除。
		if pos >= int(pclntabOffset) {
			matches = append(matches, funcMatch{pos: pos, idx: idx})
		}
		return true
	})

	// 按位置升序、同位置按长度降序排序，保证长 pattern 优先替换；
	// 短 pattern 与已替换的长 pattern 重叠时被跳过（与原长度降序语义一致）。
	sort.SliceStable(matches, func(i, j int) bool {
		if matches[i].pos != matches[j].pos {
			return matches[i].pos < matches[j].pos
		}
		return len(patternBytes[matches[i].idx]) > len(patternBytes[matches[j].idx])
	})

	lastEnd := -1
	for _, mm := range matches {
		pb := patternBytes[mm.idx]
		plen := len(pb)
		if mm.pos < lastEnd {
			continue
		}
		// 更严格的上下文检查
		if !lo.isSafeFunctionNamePrefix(data, mm.pos, pb) {
			continue
		}

		replacement := []byte(replacements[mm.idx])
		// 等长替换：确保替换字符串与原字符串长度完全相同
		if len(replacement) == plen {
			// 直接替换，无需填充
			copy(data[mm.pos:mm.pos+len(replacement)], replacement)
			count++
			replacedPatterns[patterns[mm.idx]]++
		} else if len(replacement) < plen {
			// 如果替换字符串较短（不应该发生，但作为保护）
			// 使用原始字符串填充方式而不是 0x00
			copy(data[mm.pos:mm.pos+len(replacement)], replacement)
			// 不填充，保持原有字符（更安全）
			count++
			replacedPatterns[patterns[mm.idx]]++
		}
		// 忽略替换字符串过长的情况
		lastEnd = mm.pos + plen
	}

	// 替换项目包路径（在整个二进制文件中替换，但有严格的安全检查）
	logger.Infof("替换项目包路径...")
	pathCount := lo.replaceProjectPackagePathsGlobal(data)

	if count > 0 {
		logger.Infof("[OK] 替换了 %d 个函数名前缀:", count)
		for pattern, cnt := range replacedPatterns {
			logger.Debugf("%s: %d 次", pattern, cnt)
		}
	} else {
		logger.Warnf("[!] 未找到匹配的包名前缀")
	}

	if pathCount > 0 {
		logger.Infof("[OK] 替换了 %d 个项目包路径引用", pathCount)
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

	// 按路径长度降序排序，确保更长（更具体）的子包路径先替换，
	// 避免模块根路径先替换后，子包路径在二进制中无法再匹配而残留可读子段名。
	type or struct {
		orig string
		repl string
	}
	entries := make([]or, 0, len(lo.config.PackageReplacements))
	for original, replacement := range lo.config.PackageReplacements {
		entries = append(entries, or{orig: original, repl: replacement})
	}
	sort.SliceStable(entries, func(i, j int) bool {
		if len(entries[i].orig) != len(entries[j].orig) {
			return len(entries[i].orig) > len(entries[j].orig)
		}
		return entries[i].orig < entries[j].orig
	})

	// 收集所有含 "/" 的非标准库包路径，构建 AC 自动机。
	// 过滤标准库与无 "/" 的单词路径，避免误替换系统符号。
	type pathEntry struct {
		origPath []byte
		replPath []byte
		origStr  string
	}
	var pathEntries []pathEntry
	var pathPatterns [][]byte
	for _, e := range entries {
		originalPath := strings.TrimSuffix(e.orig, ".")
		if standardLibraryNames[originalPath] {
			continue
		}
		if !strings.Contains(originalPath, "/") {
			continue
		}
		replacementPath := strings.TrimSuffix(e.repl, ".")
		pathEntries = append(pathEntries, pathEntry{
			origPath: []byte(originalPath),
			replPath: []byte(replacementPath),
			origStr:  originalPath,
		})
		pathPatterns = append(pathPatterns, []byte(originalPath))
	}

	// 用 Aho-Corasick 单次扫描全文件收集所有匹配位置，
	// 替代逐路径的 bytes.Index 扫描。
	matcher := newACMatcher(pathPatterns)
	type pathMatch struct {
		pos int
		idx int
	}
	var matches []pathMatch
	matcher.match(data, 0, len(data), func(pos, idx int) bool {
		matches = append(matches, pathMatch{pos: pos, idx: idx})
		return true
	})

	// 按位置升序、同位置按长度降序排序，长路径优先替换，重叠的短路径跳过。
	sort.SliceStable(matches, func(i, j int) bool {
		if matches[i].pos != matches[j].pos {
			return matches[i].pos < matches[j].pos
		}
		return len(pathPatterns[matches[i].idx]) > len(pathPatterns[matches[j].idx])
	})

	lastEnd := -1
	for _, mm := range matches {
		pb := pathEntries[mm.idx].origPath
		plen := len(pb)
		if mm.pos < lastEnd {
			continue
		}
		// 严格的安全检查
		if !lo.isSafePackagePathReplacement(data, mm.pos, plen) {
			continue
		}
		rb := pathEntries[mm.idx].replPath
		// 只有当替换字符串不长于原字符串时才替换
		if len(rb) <= plen {
			copy(data[mm.pos:mm.pos+len(rb)], rb)
			// 用空字节填充剩余部分
			for k := mm.pos + len(rb); k < mm.pos+plen; k++ {
				data[k] = 0
			}
			count++
			replacedPaths[pathEntries[mm.idx].origStr]++
		}
		lastEnd = mm.pos + plen
	}

	if count > 0 {
		logger.Infof("[OK] 替换了 %d 个包路径:", count)
		for path, cnt := range replacedPaths {
			logger.Debugf("%s: %d 次", path, cnt)
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
		// 拒绝明确是标识符一部分的字符：字母、数字、下划线、连字符。
		// 连字符原本被允许，但会导致 github.com/foo 误匹配 github.com/foo-bar
		// 这类同前缀不同包（后缀段以 - 开头时实为另一个包）。
		// 包路径后合法的分隔符是 /、.、@、空格、\t、NUL、引号等，不含 -。
		if (nextChar >= 'a' && nextChar <= 'z') ||
			(nextChar >= 'A' && nextChar <= 'Z') ||
			(nextChar >= '0' && nextChar <= '9') ||
			nextChar == '_' ||
			nextChar == '-' {
			return false
		}
		// 其他字符都允许（包括 /、.、空格、null、引号等）
	}

	return true
}

// discoverAndGeneratePackageReplacements 自动发现项目包名并生成替换映射
func (lo *LinkerObfuscator) discoverAndGeneratePackageReplacements(data []byte) error {
	var packages []string
	var err error
	var moduleName string

	// 尝试从项目目录发现包名 (即使是默认的 "." 目录)
	if lo.projectDir != "" {
		// 1. 读取 go.mod 获取模块名
		moduleName, err = lo.getModuleName()
		if err == nil && moduleName != "" {
			logger.Infof("发现源码模块名: %s", moduleName)
			// 2. 扫描项目目录，查找所有子包
			packages, err = lo.discoverProjectPackages(moduleName)
			if err == nil && len(packages) > 0 {
				logger.Infof("从源码成功发现 %d 个项目包，将跳过二进制分析以确保准确度", len(packages))
			}
		}
	}

	// 如果没有提供源码目录，或者从源码中未发现包，则尝试从二进制文件发现
	if len(packages) == 0 {
		// 如果从 go.mod 中读到了模块名，自动作为关键字过滤
		filterKey := lo.config.PackageFilter
		if filterKey == "" && moduleName != "" {
			logger.Infof("[i] 未指定过滤关键字，自动使用从 go.mod 提取的模块名 '%s' 作为二进制扫描关键字", moduleName)
			filterKey = moduleName
		}

		if filterKey == "" {
			return fmt.Errorf("无源码模式下自动发现包名必须通过 -pkg-filter 指定过滤关键字，或者在包含 go.mod 的目录下运行")
		}

		logger.Infof("[!] 未提供源码或源码分析失败，正在从二进制文件中通过关键字 '%s' 自动发现包名...", filterKey)

		// 临时将 config.PackageFilter 设为 filterKey，以便后续逻辑使用
		lo.config.PackageFilter = filterKey

		packages, err = lo.discoverPackagesFromBinary(data)
		if err != nil {
			return err
		}
		logger.Infof("从二进制发现 %d 个匹配关键字的包名", len(packages))
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
		logger.Infof("应用过滤器 '%s' 后保留了 %d 个项目包", lo.config.PackageFilter, len(packages))
	}

	if len(packages) == 0 {
		return fmt.Errorf("无法自动发现匹配过滤器 '%s' 的包名", lo.config.PackageFilter)
	}

	logger.Infof("发现 %d 个项目包:", len(packages))
	for _, pkg := range packages {
		logger.Infof("- %s", pkg)
	}

	// 3. 添加常见的标准库包名
	standardPackages := lo.getStandardPackages()

	// 4. 合并项目包和标准库包
	allPackages := append(packages, standardPackages...)

	// 5. 如果启用第三方包混淆，发现并添加第三方包
	var thirdPartyPackages []string
	if lo.config.ObfuscateThirdParty && moduleName != "" {
		thirdPartyPackages, err = lo.discoverThirdPartyPackages(moduleName, data)
		if err != nil {
			logger.Warnf("[!] 发现第三方包失败: %v", err)
		} else {
			logger.Infof("发现 %d 个第三方包（包括子包）:", len(thirdPartyPackages))
			// 显示前 10 个包
			displayCount := 10
			if len(thirdPartyPackages) < displayCount {
				displayCount = len(thirdPartyPackages)
			}
			for i := 0; i < displayCount; i++ {
				logger.Debugf("- %s", thirdPartyPackages[i])
			}
			if len(thirdPartyPackages) > displayCount {
				logger.Debugf("- ... 还有 %d 个", len(thirdPartyPackages)-displayCount)
			}
			allPackages = append(allPackages, thirdPartyPackages...)
		}
	}

	// 6. 生成替换映射
	replacements := lo.generateReplacements(allPackages)

	// 7. 应用替换映射
	lo.config.PackageReplacements = replacements

	if lo.config.ObfuscateThirdParty {
		logger.Infof("[OK] 生成了 %d 个包名替换映射 (项目包: %d, 标准库: %d, 第三方: %d)",
			len(replacements), len(packages), len(standardPackages), len(thirdPartyPackages))
	} else {
		logger.Infof("[OK] 生成了 %d 个包名替换映射 (项目包: %d, 标准库: %d)",
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
		"syscall", // 系统调用，函数名前缀可以安全替换

		// I/O 和格式化（安全）
		"fmt",
		"io",
		"bufio",
		"os",
		"log",

		// 网络相关（安全）
		"net",
		"http", // 实际是 net/http，但在符号表中可能显示为 http

		// 字符串和数据处理（安全）
		"strings",
		"bytes",
		"strconv",
		"unicode",
		"regexp",

		// 编码（安全）
		"encoding",
		"json",   // encoding/json
		"xml",    // encoding/xml
		"base64", // encoding/base64
		"hex",    // encoding/hex

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
	return readModuleNameFromFile(filepath.Join(lo.projectDir, "go.mod"))
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
	sort.Slice(result, func(i, j int) bool {
		return len(result[i]) > len(result[j])
	})

	return result, nil
}

// hasGoFiles 检查目录中是否有 .go 文件
func (lo *LinkerObfuscator) hasGoFiles(dir string) (bool, error) {
	files, err := os.ReadDir(dir)
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

// generateShortName 生成短名称 (a, b, c, ..., z, aa, ab, ..., zz, aaa, ...)
// 使用双射 base-26 计数，可正确支持任意数量的包，避免超出 26^2 后
// 生成 '{'、'|'、'~' 乃至高位字节等非法字符（破坏等长替换假设）。
func (lo *LinkerObfuscator) generateShortName(index int) string {
	if index < 0 {
		index = 0
	}
	var buf [16]byte
	i := len(buf)
	n := index + 1 // 双射计数从 1 开始（1->a, 26->z, 27->aa）
	for n > 0 {
		i--
		n--
		buf[i] = byte('a' + n%26)
		n /= 26
	}
	return string(buf[i:])
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
		// - 不可打印控制字符 (< 0x20)
		//
		// 除此之外的任何可打印字符（字母、数字、点号、斜杠、空格、引号、
		// 括号、下划线、连字符等）都说明这不是符号边界，而是某个文本/标识符
		// 的一部分，替换会误伤数据区，因此一律拒绝。
		if prevChar == 0 {
			return true
		}
		if prevChar < 0x20 {
			return true
		}
		return false
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
func (lo *LinkerObfuscator) discoverThirdPartyPackages(moduleName string, data []byte) ([]string, error) {
	packages := make(map[string]bool)

	// 读取 go.mod 文件（注意：用独立变量 goModData，避免遮蔽参数 data——后者是
	// 二进制内容，需传给 discoverThirdPartySubPackages 用于枚举子包）。
	goModPath := filepath.Join(lo.projectDir, "go.mod")
	goModData, err := os.ReadFile(goModPath)
	if err != nil {
		return nil, err
	}

	// 改进的正则表达式：匹配所有 require 行（包括 indirect 注释）
	// 匹配格式: github.com/xxx/yyy v1.2.3 或 github.com/xxx/yyy v1.2.3 // indirect
	requireRe := regexp.MustCompile(`(?m)^\s*([a-zA-Z0-9\-_\.]+/[a-zA-Z0-9\-_\./]+?)\s+v[^\s]+`)
	requireMatches := requireRe.FindAllSubmatch(goModData, -1)

	// 同时匹配 replace 指令，格式: replace github.com/xxx/yyy => github.com/aaa/bbb v1.2.3
	replaceRe := regexp.MustCompile(`(?m)^replace\s+([a-zA-Z0-9\-_\.]+/[a-zA-Z0-9\-_\./]+)\s+[^\n]*=>\s+([a-zA-Z0-9\-_\.]+/[a-zA-Z0-9\-_\./]+)\s+v`)
	replaceMatches := replaceRe.FindAllSubmatch(goModData, -1)

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

	// 从二进制的 pclntab 符号中枚举第三方模块根下的实际子包路径。
	// go.mod 的 require 只列模块根，但被引用的子包（如
	// google.golang.org/protobuf/types/descriptorpb、go.uber.org/zap/zapcore、
	// golang.org/x/crypto/md4）并不在 require 中。若子包路径不在替换映射里，
	// 其最后一段（types/descriptorpb/zapcore/md4）混淆后仍明文残留在 pclntab。
	if lo.outputBin != "" {
		if subs := lo.discoverThirdPartySubPackages(packages, data); len(subs) > 0 {
			logger.Infof("从二进制枚举到 %d 个第三方子包路径，合并进替换映射", len(subs))
			for _, sp := range subs {
				packages[sp] = true
			}
		}
	}

	// 转换为切片并排序
	result := make([]string, 0, len(packages))
	for pkg := range packages {
		result = append(result, pkg)
	}

	// 为含 "." 的路径追加 %2e 编码变体：Go 编译器会把 import 路径中
	// 子包/包名段的 "." 编码为 %2e 写入 pclntab（如 go.uuid -> go%2euuid、
	// yaml.v2 -> yaml%2ev2），但顶级域名段（如 github.com 的第一段）保持点号。
	// 普通点号形态的替换 pattern 匹配不到 %2e 形态，导致符号混淆后仍明文残留。
	extra := make([]string, 0)
	for _, pkg := range result {
		if !strings.ContainsAny(pkg, ".") {
			continue
		}
		// 仅编码第一段 "/" 之后的 "."，保留顶级域名段
		slash := strings.Index(pkg, "/")
		if slash < 0 {
			continue
		}
		head, tail := pkg[:slash+1], pkg[slash+1:]
		if strings.Contains(tail, ".") {
			extra = append(extra, head+strings.ReplaceAll(tail, ".", "%2e"))
		}
	}
	result = append(result, extra...)

	// 按长度降序排序（确保子包在前，避免替换冲突）
	sort.Slice(result, func(i, j int) bool {
		return len(result[i]) > len(result[j])
	})

	return result, nil
}

// discoverThirdPartySubPackages 从二进制的 pclntab 符号中枚举第三方模块根下的实际子包路径。
// pclntab 中每个符号是 "<完整包路径>.<函数名>" 形式，符号之间以 \x00 分隔。
// go.mod 的 require 只列模块根，但被引用的子包（如 google.golang.org/protobuf/types/descriptorpb、
// go.uber.org/zap/zapcore、golang.org/x/crypto/md4）并不在 require 中。若子包路径不在替换
// 映射里，其最后一段（types/descriptorpb/zapcore/md4）混淆后仍明文残留在 pclntab。
// 这里对每个以已知模块根开头的符号提取完整包路径（含各级子段），全部返回给调用方合并。
func (lo *LinkerObfuscator) discoverThirdPartySubPackages(roots map[string]bool, data []byte) []string {
	rootList := make([]string, 0, len(roots))
	for r := range roots {
		if strings.Contains(r, "/") {
			rootList = append(rootList, r)
		}
	}
	if len(rootList) == 0 {
		return nil
	}
	// 长根路径优先，确保符号能匹配到最具体的模块根
	sort.Slice(rootList, func(i, j int) bool { return len(rootList[i]) > len(rootList[j]) })

	found := make(map[string]bool)

	// 以 NUL 为界切分符号串，逐个检查是否以某个模块根开头
	tokenStart := 0
	for i := 0; i <= len(data); i++ {
		if i == len(data) || data[i] == 0 {
			if i > tokenStart {
				token := string(data[tokenStart:i])
				pkg := matchThirdPartyRoot(token, rootList)
				if pkg == "" {
					tokenStart = i + 1
					continue
				}
				// 拆出各级 "/" 子段前缀（如 types、types/descriptorpb），
				// 连同完整包路径一起加入，覆盖不同深度的符号引用。
				for idx := 0; idx < len(pkg); idx++ {
					if pkg[idx] == '/' {
						found[pkg[:idx]] = true
					}
				}
				found[pkg] = true
			}
			tokenStart = i + 1
		}
	}

	if len(found) == 0 {
		return nil
	}
	result := make([]string, 0, len(found))
	for p := range found {
		result = append(result, p)
	}
	return result
}

// matchThirdPartyRoot 若符号 token 以某个模块根开头，提取其完整包路径（含子段），
// 否则返回空字符串。包路径是 token 中最后一个 "/" 之后第一个 "." 之前的部分。
// root 匹配要求紧接的字符必须是 "/"、"." 或 token 结束，避免
// github.com/fatih/color 误匹配 github.com/fatih/colorable 这类同前缀不同包。
func matchThirdPartyRoot(token string, rootList []string) string {
	for _, root := range rootList {
		if len(token) <= len(root) || !strings.HasPrefix(token, root) {
			continue
		}
		// root 必须落在路径段边界上
		next := token[len(root)]
		if next != '/' && next != '.' {
			continue
		}
		lastSlash := strings.LastIndexByte(token, '/')
		end := len(token)
		if lastSlash >= 0 {
			if dot := strings.IndexByte(token[lastSlash+1:], '.'); dot >= 0 {
				end = lastSlash + 1 + dot
			}
		}
		pkg := token[:end]
		if len(pkg) <= len(root) {
			// 符号包路径就是模块根本身（如 <root>.init），不是子包
			continue
		}
		return pkg
	}
	return ""
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
	m := make(map[string]bool, len(pkgs)+8)
	for _, p := range pkgs {
		m[p] = true
	}
	// 补充一些常见的标准库前缀
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
