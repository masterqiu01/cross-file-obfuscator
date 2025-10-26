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
	fmt.Println("       \033[90mVersion 1.0.0 | By  masterqiu01\033[0m")
	fmt.Println()
}

// checkAndHandleExistingDir 检查输出目录是否存在，如果存在则询问用户是否覆盖
func checkAndHandleExistingDir(outDir string) error {
	if _, err := os.Stat(outDir); err == nil {
		// 目录存在
		fmt.Printf("\n\033[33m⚠️  警告: 输出目录已存在: %s\033[0m\n", outDir)
		fmt.Print("是否删除现有目录并继续? [y/N]: ")
		
		var response string
		fmt.Scanln(&response)
		response = strings.ToLower(strings.TrimSpace(response))
		
		if response == "y" || response == "yes" {
			fmt.Printf("正在删除目录: %s\n", outDir)
			if err := os.RemoveAll(outDir); err != nil {
				return fmt.Errorf("删除目录失败: %v", err)
			}
			fmt.Println("\033[32m✓ 目录已删除\033[0m")
		} else {
			return fmt.Errorf("用户取消操作")
		}
	}
	return nil
}

func printUsage() {
	fmt.Println("用法: cross-file-obfuscator [选项] <项目目录>")
	fmt.Println()
	fmt.Println("基础选项:")
	fmt.Println("  -h                          显示帮助信息")
	fmt.Println("  -o string                   输出目录")
	fmt.Println("  -encrypt-strings            加密字符串字面量")
	fmt.Println("  -inject-junk                注入垃圾代码")
	fmt.Println("  -obfuscate-filenames        混淆文件名")
	fmt.Println("  -obfuscate-exported         混淆导出函数 (危险!)")
	fmt.Println("  -remove-comments            移除注释 (默认: true)")
	fmt.Println("  -preserve-reflection        保留反射类型 (默认: true)")
	fmt.Println("  -skip-generated             跳过生成的代码 (默认: true)")
	fmt.Println("  -exclude string             排除文件模式")
	fmt.Println()
	fmt.Println("高级选项:")
	fmt.Println("  -build-with-linker          直接编译并应用链接器混淆 ")
	fmt.Println("  -output-bin string          输出二进制文件名 (配合 -build-with-linker)")
	fmt.Println("  -entry string               入口包路径 (默认: '.')")
	fmt.Println("  -pkg-replace string         包名替换映射 (格式: 'pkg1=a,pkg2=b')")
	fmt.Println("  -auto-discover-pkgs         自动发现并替换项目中的所有包名")
	fmt.Println("  -obfuscate-third-party      混淆第三方依赖包 (谨慎使用)")
	fmt.Println("  -auto                       自动模式：全功能混淆 + 自动编译 (推荐!)")
	fmt.Println()
	fmt.Println("示例:")
	fmt.Println("  # 🚀 自动模式 - 一键全功能混淆 (推荐！)")
	fmt.Println("  ./cross-file-obfuscator -auto -output-bin myapp ./my-project")
	fmt.Println()
	fmt.Println("  # 基础混淆")
	fmt.Println("  ./cross-file-obfuscator ./my-project")
	fmt.Println()
	fmt.Println("  # 字符串加密 + 垃圾代码")
	fmt.Println("  ./cross-file-obfuscator -encrypt-strings -inject-junk ./my-project")
	fmt.Println()
	fmt.Println("  # 自动发现并替换所有项目包名")
	fmt.Println("  ./cross-file-obfuscator -build-with-linker -auto-discover-pkgs -output-bin myapp ./my-project")
	fmt.Println()
	fmt.Println("  # 编译为 Windows 64位程序")
	fmt.Println("  GOOS=windows GOARCH=amd64 ./cross-file-obfuscator -auto -output-bin app.exe ./my-project")
	fmt.Println()
	fmt.Println("详细文档: README.md")
}

func main() {
	// 显示 Logo
	printLogo()
	
	// 基础选项
	var (
		outputDir          = flag.String("o", "", "输出目录 (默认: project_directory_obfuscated)")
		obfuscateExported  = flag.Bool("obfuscate-exported", false, "混淆导出的函数和变量 (可能破坏外部引用)")
		obfuscateFileNames = flag.Bool("obfuscate-filenames", false, "混淆 Go 文件名")
		encryptStrings     = flag.Bool("encrypt-strings", false, "加密字符串字面量并运行时解密")
		injectJunkCode     = flag.Bool("inject-junk", false, "注入垃圾代码以混淆分析")
		removeComments     = flag.Bool("remove-comments", true, "移除所有注释")
		preserveReflection = flag.Bool("preserve-reflection", true, "保留反射中使用的类型/方法")
		skipGeneratedCode  = flag.Bool("skip-generated", true, "跳过自动生成的代码文件")
		excludePatterns    = flag.String("exclude", "", "要排除的文件模式 (逗号分隔, 例如: -exclude '*_test.go,*.pb.go')")
		showHelp           = flag.Bool("h", false, "显示帮助信息")
	)

	// 高级选项
	var (
		buildWithLinker     = flag.Bool("build-with-linker", false, "直接编译并应用链接器混淆")
		outputBinary        = flag.String("output-bin", "", "输出二进制文件名 (配合 -build-with-linker 使用)")
		entryPackage        = flag.String("entry", ".", "入口包路径，例如: './cmd/server' ")
		packageReplacements = flag.String("pkg-replace", "", "包名替换映射 (格式: 'original1=new1,original2=new2')")
		autoDiscoverPkgs    = flag.Bool("auto-discover-pkgs", false, "自动发现并替换项目中的所有包名")
		obfuscateThirdParty = flag.Bool("obfuscate-third-party", false, "混淆第三方依赖包（谨慎使用）")
		autoMode            = flag.Bool("auto", false, "自动模式：全功能混淆 + 自动编译（包含所有混淆功能）")
	)

	// 自定义 Usage 函数
	flag.Usage = printUsage

	flag.Parse()

	// 如果用户使用 -h 参数，显示帮助并退出
	if *showHelp || flag.NArg() < 1 {
		printUsage()
		os.Exit(0)
	}

	// 如果使用 auto 模式，执行全功能混淆
	if *autoMode {
		if flag.NArg() < 1 {
			log.Fatal("错误: 请指定项目目录")
		}
		projectRoot := flag.Arg(0)

		fmt.Println("╔══════════════════════════════════════════════════════════════╗")
		fmt.Println("                🚀 自动模式：全功能混淆 + 自动编译                ")
		fmt.Println("╚══════════════════════════════════════════════════════════════╝")
		fmt.Println()
		fmt.Println("混淆配置（已启用所有功能）：")
		fmt.Println("  ✅ 字符串加密")
		fmt.Println("  ✅ 垃圾代码注入")
		fmt.Println("  ✅ 导出符号混淆")
		fmt.Println("  ✅ 文件名混淆")
		fmt.Println("  ✅ 注释移除")
		fmt.Println("  ✅ 自动发现包名")
		fmt.Println("  ✅ 链接器混淆")
		fmt.Println("  ✅ 第三方包混淆")
		fmt.Println()

		// 第一步：源码混淆
		fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		fmt.Println("第 1/2 步：源码混淆")
		fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		fmt.Println()

		outDir := projectRoot + "_obfuscated"
		if *outputDir != "" {
			outDir = *outputDir
		}

		// 检查输出目录是否已存在
		if err := checkAndHandleExistingDir(outDir); err != nil {
			log.Fatalf("错误: %v", err)
		}

		// 解析排除模式
		var excludeList []string
		if *excludePatterns != "" {
			excludeList = strings.Split(*excludePatterns, ",")
			for i := range excludeList {
				excludeList[i] = strings.TrimSpace(excludeList[i])
			}
		}

		// 创建源码混淆器
		sourceObf := obfuscator.New(projectRoot, outDir, &obfuscator.Config{
			ObfuscateExported:  true,
			ObfuscateFileNames: true,
			EncryptStrings:     true,
			InjectJunkCode:     true,
			RemoveComments:     *removeComments,
			PreserveReflection: *preserveReflection,
			SkipGeneratedCode:  *skipGeneratedCode,
			ExcludePatterns:    excludeList,
		})

		// 执行源码混淆
		if err := sourceObf.Run(); err != nil {
			log.Fatalf("源码混淆失败: %v", err)
		}

		fmt.Println()
		fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		fmt.Println("第 2/2 步：链接器混淆 + 编译")
		fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		fmt.Println()

		// 第二步：链接器混淆
		binName := *outputBinary
		if binName == "" {
			binName = "output_obfuscated"
		}

		linkConfig := &obfuscator.LinkConfig{
			RemoveFuncNames:      true,
			EntryPackage:         *entryPackage,
			AutoDiscoverPackages: true,
			ObfuscateThirdParty:  true, // auto 模式自动启用第三方包混淆
		}

		linkerObf := obfuscator.NewLinkerObfuscator(outDir, binName, linkConfig)

		if err := linkerObf.BuildWithLinkerObfuscation(); err != nil {
			log.Fatalf("链接器混淆失败: %v", err)
		}

		fmt.Println()
		fmt.Println("╔══════════════════════════════════════════════════════════════╗")
		fmt.Println("║                    ✅ 全功能混淆完成！                        ║")
		fmt.Println("╚══════════════════════════════════════════════════════════════╝")
		fmt.Printf("\n📦 混淆后的二进制文件: %s\n", binName)
		fmt.Printf("📁 混淆后的源码目录: %s\n", outDir)
		fmt.Println("\n验证混淆效果:")
		fmt.Printf("  strings %s | grep -i 'main\\.' | wc -l\n", binName)
		fmt.Printf("  strings %s | grep -i 'runtime\\.' | wc -l\n", binName)
		return
	}

	// 如果使用链接器构建模式
	if *buildWithLinker {
		if flag.NArg() < 1 {
			log.Fatal("错误: 请指定项目目录")
		}
		projectRoot := flag.Arg(0)

		// 设置输出二进制文件名
		binName := *outputBinary
		if binName == "" {
			binName = "output_obfuscated"
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
			RemoveFuncNames:      true,                 // 混淆函数名
			EntryPackage:         *entryPackage,        // 入口包路径
			PackageReplacements:  pkgReplaceMap,        // 包名替换映射
			AutoDiscoverPackages: *autoDiscoverPkgs,    // 自动发现包名
			ObfuscateThirdParty:  *obfuscateThirdParty, // 混淆第三方包
		}

		linkerObf := obfuscator.NewLinkerObfuscator(projectRoot, binName, linkConfig)

		// 执行构建和混淆
		if err := linkerObf.BuildWithLinkerObfuscation(); err != nil {
			log.Fatalf("链接器混淆失败: %v", err)
		}

		fmt.Printf("\n✅ 成功! 混淆后的二进制文件: %s\n", binName)
		fmt.Println("\n验证混淆效果:")
		fmt.Printf("  strings %s | grep -i 'main\\.' | head -20\n", binName)
		fmt.Printf("  strings %s | grep -i 'runtime\\.' | head -20\n", binName)
		return
	}

	projectRoot := flag.Arg(0)

	// 验证项目目录
	info, err := os.Stat(projectRoot)
	if err != nil {
		log.Fatalf("错误: 无法访问项目根目录 %s: %v", projectRoot, err)
	}
	if !info.IsDir() {
		log.Fatalf("错误: 项目根路径必须是一个目录: %s", projectRoot)
	}

	// 设置输出目录
	if *outputDir == "" {
		*outputDir = projectRoot + "_obfuscated"
	}

	// 检查输出目录是否已存在
	if err := checkAndHandleExistingDir(*outputDir); err != nil {
		log.Fatalf("错误: %v", err)
	}

	if err := os.MkdirAll(*outputDir, 0755); err != nil {
		log.Fatalf("错误: 无法创建输出目录 %s: %v", *outputDir, err)
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
	}

	// 创建混淆器
	obf := obfuscator.New(projectRoot, *outputDir, config)

	// 打印配置
	printConfiguration(projectRoot, *outputDir, config, excludePatternsList)

	// 执行混淆
	fmt.Println("开始混淆...")
	if err := runObfuscation(obf, config, projectRoot, *outputDir); err != nil {
		log.Fatalf("错误: %v", err)
	}

	// 打印统计信息
	stats := obf.GetStatistics()
	printStatistics(stats)

	fmt.Println("\n✅ 混淆完成!")
	fmt.Println("请在输出目录中运行 'go build' 以验证编译。")
	fmt.Println("\n提示: 使用 -build-with-linker 可以直接编译并应用链接器级别混淆")
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
		fmt.Printf(" ⚠️  警告: 可能破坏外部引用!\n")
	} else {
		fmt.Println()
	}
	fmt.Printf("  混淆文件名:       %v\n", config.ObfuscateFileNames)
	fmt.Printf("  加密字符串:       %v\n", config.EncryptStrings)
	fmt.Printf("  注入垃圾代码:     %v\n", config.InjectJunkCode)
	fmt.Printf("  移除注释:         %v\n", config.RemoveComments)
	fmt.Printf("  保留反射:         %v\n", config.PreserveReflection)
	fmt.Printf("  跳过生成代码:     %v\n", config.SkipGeneratedCode)
	if len(excludePatterns) > 0 {
		fmt.Printf("  排除模式:         %v\n", excludePatterns)
	}
	fmt.Println()
}

func runObfuscation(obf *obfuscator.Obfuscator, config *obfuscator.Config, projectRoot, outputDir string) error {
	return obf.Run()
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
