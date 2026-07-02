package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"strings"

	"cross-file-obfuscator/obfuscator"
)

func printLogo() {
	fmt.Println()
	fmt.Println("\033[1;35m ██████╗██████╗  ██████╗ ███████╗███████╗\033[0m")
	fmt.Println("\033[1;35m██╔════╝██╔══██╗██╔═══██╗██╔════╝██╔════╝\033[0m")
	fmt.Println("\033[1;35m██║     ██████╔╝██║   ██║███████╗███████╗\033[0m")
	fmt.Println("\033[1;35m██║     ██╔══██╗██║   ██║╚════██║╚════██║\033[0m")
	fmt.Println("\033[1;35m╚██████╗██║  ██║╚██████╔╝███████║███████║\033[0m")
	fmt.Println("\033[1;35m ╚═════╝╚═╝  ╚═╝ ╚═════╝ ╚══════╝╚══════╝\033[0m")
	fmt.Println()
	fmt.Println("     \033[1;33m━━━ File Obfuscator ━━━\033[0m")
	fmt.Println()
	fmt.Println("       \033[90mGo 代码混淆与保护工具\033[0m")
	fmt.Println("       \033[90mVersion 1.0.4 | By  masterqiu01\033[0m")
	fmt.Println()
}

// checkAndHandleExistingDir 检查输出目录是否存在，如果存在则询问用户是否覆盖
func checkAndHandleExistingDir(outDir string) error {
	if _, err := os.Stat(outDir); err == nil {
		// 目录存在
		fmt.Printf("\n\033[33m[!] 警告: 输出目录已存在: %s\033[0m\n", outDir)
		fmt.Print("是否删除现有目录并继续? [y/N]: ")

		var response string
		fmt.Scanln(&response)
		response = strings.ToLower(strings.TrimSpace(response))

		if response == "y" || response == "yes" {
			fmt.Printf("正在删除目录: %s\n", outDir)
			if err := os.RemoveAll(outDir); err != nil {
				return fmt.Errorf("删除目录失败: %v", err)
			}
			fmt.Println("\033[32m[OK] 目录已删除\033[0m")
		} else {
			return fmt.Errorf("用户取消操作")
		}
	}
	return nil
}

func printUsage() {
	fmt.Println("用法: \033[1mcross-file-obfuscator [选项] <路径>\033[0m")
	fmt.Println()
	fmt.Println("\033[1m■ 主要功能模式:\033[0m")
	fmt.Println("  -source                     模式1：对 Go 源码进行 AST 混淆（生成新项目）")
	fmt.Println("  -binary                     模式2：对已编译二进制进行 PCLNTAB 修改（直接修改文件）")
	fmt.Println()
	fmt.Println("\033[1m■ 源码混淆选项 (配合 -source):\033[0m")
	fmt.Println("  -o string                   输出目录 (默认: '项目名_obfuscated')")
	fmt.Println("  -encrypt-strings            加密所有字符串字面量 (运行时解密)")
	fmt.Println("  -inject-junk                注入随机化的垃圾代码和不透明谓词")
	fmt.Println("  -obfuscate-filenames        混淆 .go 源代码文件名")
	fmt.Println("  -obfuscate-exported         混淆导出函数名 (注意: 可能破坏外部引用)")
	fmt.Println("  -remove-comments            移除代码注释 (默认: true)")
	fmt.Println("  -exclude string             排除特定文件/目录模式 (逗号分隔, 如 '*_test.go,internal/*')")
	fmt.Println("  -dry-run                    干跑模式：只打印将要混淆的内容，不实际写入任何文件")
	fmt.Println()
	fmt.Println("\033[1m■ 二进制修改选项 (配合 -binary):\033[0m")
	fmt.Println("  -project string             源码根目录路径 (非项目目录执行时，用于定位 go.mod)")
	fmt.Println("  -auto-discover-pkgs         自动识别项目包名 (有源码时优先扫描源码; 无源码时必须配合 -pkg-filter)")
	fmt.Println("  -pkg-filter string          包名过滤关键字 (无源码模式下必填, 支持逗号分隔, 如 'pkg1,pkg2')")
	fmt.Println("  -pkg-replace string         手动指定包名映射 (格式: 'oldpkg=newpkg,lib/math=a/m')")
	fmt.Println("  -obfuscate-third-party      混淆第三方依赖包 (如 github.com/xxx，谨慎使用)")
	fmt.Println("  -only-project               仅混淆项目自身包，保留标准库 (增强 Windows 兼容性)")
	fmt.Println("  -disable-pclntab            仅执行基础符号操作，不修改 pclntab 结构")
	fmt.Println("  -obfuscate-paths            混淆二进制中残留的 .go 源文件绝对路径 (极力推荐)")
	fmt.Println()
	fmt.Println("\033[1m■ 常见使用场景:\033[0m")
	
	fmt.Println("  \033[36m1. 源码混淆 (适用于交付源码)\033[0m")
	fmt.Println("  功能: 混淆文件名，加密字符串，注入控制流混淆代码。")
	fmt.Println("  命令:")
	fmt.Println("    ./cross-file-obfuscator -source -encrypt-strings -inject-junk -obfuscate-filenames -o out_src ./my_project")
	fmt.Println()
	
	fmt.Println("  \033[36m2. 二进制全量混淆 (推荐)\033[0m")
	fmt.Println("  功能: 读取 go.mod 自动混淆所有项目包与第三方包，移除可读文件路径。")
	fmt.Println("  命令: (需在包含 go.mod 的目录下执行)")
	fmt.Println("    go build -trimpath -o app.exe")
	fmt.Println("    ./cross-file-obfuscator -binary -auto-discover-pkgs -obfuscate-third-party app.exe")
	fmt.Println()
	
	fmt.Println("  \033[36m3. 最小化二进制混淆 (降低杀软误报)\033[0m")
	fmt.Println("  功能: 仅对项目自身包进行混淆，保留原生标准库特征。")
	fmt.Println("  命令:")
	fmt.Println("    ./cross-file-obfuscator -source -encrypt-strings ./my_project")
	fmt.Println("    cd ./my_project_obfuscated && go build -trimpath -ldflags=\"-s -w\" -o app.exe")
	fmt.Println("    ./cross-file-obfuscator -binary -auto-discover-pkgs -only-project app.exe")
	fmt.Println()
	
	fmt.Println("  \033[36m4. 无源码二进制局部混淆\033[0m")
	fmt.Println("  功能: 仅通过指定关键字，对无源码的二进制文件进行混淆。")
	fmt.Println("  命令:")
	fmt.Println("    ./cross-file-obfuscator -binary -auto-discover-pkgs -pkg-filter \"mycompany,api\" app.exe")
	fmt.Println()
	fmt.Println("更多信息请访问: \033[4mhttps://github.com/masterqiu01/cross-file-obfuscator\033[0m")
}

func main() {
	// 显示 Logo
	printLogo()

	// 功能模式
	var (
		sourceMode = flag.Bool("source", false, "源码混淆模式")
		binaryMode = flag.Bool("binary", false, "二进制修改模式")
	)

	// 源码混淆选项
	var (
		outputDir          = flag.String("o", "", "输出目录 (默认: project_directory_obfuscated)")
		obfuscateExported  = flag.Bool("obfuscate-exported", false, "混淆导出的函数和变量 (可能破坏外部引用)")
		obfuscateFileNames = flag.Bool("obfuscate-filenames", false, "混淆 Go 文件名")
		encryptStrings     = flag.Bool("encrypt-strings", false, "加密字符串字面量并运行时解密")
		injectJunkCode     = flag.Bool("inject-junk", false, "注入垃圾代码以混淆分析")
		removeComments     = flag.Bool("remove-comments", true, "移除所有注释")
		preserveReflection = flag.Bool("preserve-reflection", true, "保留反射中使用的类型/方法")
		skipGeneratedCode  = flag.Bool("skip-generated", true, "跳过自动生成的代码文件")
		excludePatterns    = flag.String("exclude", "", "要排除的文件模式 (逗号分隔)")
		dryRun             = flag.Bool("dry-run", false, "干跑模式：只打印将要混淆的内容，不实际写入文件")
	)

	// 二进制修改选项
	var (
		projectRoot          = flag.String("project", ".", "项目根目录 (用于自动发现包名)")
		packageReplacements  = flag.String("pkg-replace", "", "包名替换映射 (格式: 'original1=new1,original2=new2')")
		autoDiscoverPkgs     = flag.Bool("auto-discover-pkgs", false, "自动发现并替换项目中的所有包名")
		obfuscateThirdParty  = flag.Bool("obfuscate-third-party", false, "混淆第三方依赖包")
		onlyObfuscateProject = flag.Bool("only-project", false, "只混淆项目包，保留标准库")
		disablePclntab       = flag.Bool("disable-pclntab", false, "完全禁用 pclntab 修改")
		packageFilter        = flag.String("pkg-filter", "", "包名过滤关键字")
		obfuscatePaths       = flag.Bool("obfuscate-paths", true, "混淆二进制中残留的 .go 源文件绝对路径")
		showHelp             = flag.Bool("h", false, "显示帮助信息")
	)

	// 自定义 Usage 函数
	flag.Usage = printUsage

	flag.Parse()

	// 如果用户使用 -h 参数，显示帮助并退出
	if *showHelp || flag.NArg() < 1 || (!*sourceMode && !*binaryMode) {
		printUsage()
		os.Exit(0)
	}

	if *sourceMode && *binaryMode {
		log.Fatal("错误: 不能同时启用 -source 和 -binary 模式，请选择其一")
	}

	target := flag.Arg(0)

	// 源码混淆模式
	if *sourceMode {
		// 验证项目目录
		info, err := os.Stat(target)
		if err != nil {
			log.Fatalf("错误: 无法访问项目根目录 %s: %v", target, err)
		}
		if !info.IsDir() {
			log.Fatalf("错误: 源码混淆的项目根路径必须是一个目录: %s", target)
		}

		// 设置输出目录
		if *outputDir == "" {
			*outputDir = target + "_obfuscated"
		}

		// 检查输出目录是否已存在（仅在非干跑模式下）
		if !*dryRun {
			if err := checkAndHandleExistingDir(*outputDir); err != nil {
				log.Fatalf("错误: %v", err)
			}

			if err := os.MkdirAll(*outputDir, 0755); err != nil {
				log.Fatalf("错误: 无法创建输出目录 %s: %v", *outputDir, err)
			}
		}

		// 解析排除模式
		var excludePatternsList []string
		if *excludePatterns != "" {
			excludePatternsList = strings.Split(*excludePatterns, ",")
			for i := range excludePatternsList {
				excludePatternsList[i] = strings.TrimSpace(excludePatternsList[i])
			}
		}

		// 创建配置
		config := &obfuscator.Config{
			ObfuscateExported:  *obfuscateExported,
			ObfuscateFileNames: *obfuscateFileNames,
			EncryptStrings:     *encryptStrings,
			InjectJunkCode:     *injectJunkCode,
			RemoveComments:     *removeComments,
			PreserveReflection: *preserveReflection,
			SkipGeneratedCode:  *skipGeneratedCode,
			ExcludePatterns:    excludePatternsList,
			DryRun:             *dryRun,
		}

		// 创建混淆器
		obf := obfuscator.New(target, *outputDir, config)

		// 打印配置
		printConfiguration(target, *outputDir, config, excludePatternsList)

		// 执行混淆
		fmt.Println("开始源码混淆...")
		if err := obf.Run(); err != nil {
			log.Fatalf("错误: %v", err)
		}

		// 打印统计信息
		stats := obf.GetStatistics()
		printStatistics(stats)

		fmt.Println("\n[OK] 源码混淆完成!")
		fmt.Printf("输出目录: %s\n", *outputDir)
		return
	}

	// 二进制修改模式
	if *binaryMode {
		// 验证二进制文件
		info, err := os.Stat(target)
		if err != nil {
			log.Fatalf("错误: 无法访问二进制文件 %s: %v", target, err)
		}
		if info.IsDir() {
			log.Fatalf("错误: 二进制修改的目标必须是一个文件，而非目录: %s", target)
		}

		// 解析包名替换映射
		pkgReplaceMap := make(map[string]string)
		if *packageReplacements != "" {
			pairs := strings.Split(*packageReplacements, ",")
			for _, pair := range pairs {
				parts := strings.SplitN(strings.TrimSpace(pair), "=", 2)
				if len(parts) == 2 {
					original := strings.TrimSpace(parts[0])
					replacement := strings.TrimSpace(parts[1])
					if original != "" && replacement != "" {
						pkgReplaceMap[original] = replacement
					}
				}
			}
		}

		// 创建链接器混淆器
		linkConfig := &obfuscator.LinkConfig{
			RemoveFuncNames:      true,                  // 混淆函数名
			PackageReplacements:  pkgReplaceMap,         // 包名替换映射
			AutoDiscoverPackages: *autoDiscoverPkgs,     // 自动发现并替换项目中的所有包名
			ObfuscateThirdParty:  *obfuscateThirdParty,  // 混淆第三方依赖包
			OnlyObfuscateProject: *onlyObfuscateProject, // 只混淆项目包，保留标准库
			DisablePclntab:       *disablePclntab,       // 完全禁用 pclntab
			PackageFilter:        *packageFilter,        // 包名过滤关键字
			ObfuscateFilePaths:   *obfuscatePaths,       // 混淆二进制中残留的 .go 源文件绝对路径
		}

		// 验证无源码模式下的自动发现 (交由 linker.go 内部基于是否存在 go.mod 进行最终验证)
		if *autoDiscoverPkgs && *projectRoot == "" && *packageFilter == "" {
			log.Fatal("错误: 在没有指定源码目录 (-project) 的情况下使用自动发现，如果当前目录无 go.mod，必须提供 -pkg-filter 关键字")
		}

		// 执行混淆
		linkerObf := obfuscator.NewLinkerObfuscator(*projectRoot, target, linkConfig)

		// 执行混淆
		if err := linkerObf.ObfuscateExistingBinary(target); err != nil {
			log.Fatalf("二进制混淆失败: %v", err)
		}

		fmt.Printf("\n[OK] 二进制修改成功! 文件: %s\n", target)
		return
	}
}

func printConfiguration(projectRoot, outputDir string, config *obfuscator.Config, excludePatterns []string) {
	fmt.Println("========================================")
	fmt.Println("   Go 代码混淆器")
	fmt.Println("========================================")
	fmt.Printf("输入:  %s\n", projectRoot)
	fmt.Printf("输出:  %s\n", outputDir)
	fmt.Println()
	fmt.Println("配置选项:")
	fmt.Printf("  混淆导出函数:     %v", config.ObfuscateExported)
	if config.ObfuscateExported {
		fmt.Printf(" [!] 警告: 可能破坏外部引用!\n")
	} else {
		fmt.Println()
	}
	fmt.Printf("  混淆文件名:       %v\n", config.ObfuscateFileNames)
	fmt.Printf("  加密字符串:       %v\n", config.EncryptStrings)
	fmt.Printf("  注入垃圾代码:     %v\n", config.InjectJunkCode)
	fmt.Printf("  移除注释:         %v\n", config.RemoveComments)
	fmt.Printf("  保留反射:         %v\n", config.PreserveReflection)
	fmt.Printf("  跳过生成代码:     %v\n", config.SkipGeneratedCode)
	if config.DryRun {
		fmt.Println("  \033[33m[DRY-RUN]           true  — 只预览，不写入文件\033[0m")
	}
	if len(excludePatterns) > 0 {
		fmt.Printf("  排除模式:         %v\n", excludePatterns)
	}
	fmt.Println()
}

func printStatistics(stats *obfuscator.Statistics) {
	fmt.Println()
	fmt.Println("========================================")
	fmt.Println("   混淆统计")
	fmt.Println("========================================")
	fmt.Printf("受保护名称: %d\n", stats.ProtectedNames)
	fmt.Printf("混淆函数:   %d\n", stats.FunctionsObf)
	fmt.Printf("混淆变量:   %d\n", stats.VariablesObf)
	if stats.SkippedFiles > 0 {
		fmt.Printf("跳过文件:   %d\n", stats.SkippedFiles)
	}
}
