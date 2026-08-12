# cross-file-obfuscator

一款针对 Go 语言的代码混淆工具。支持源码级 AST 混淆与编译后二进制文件 (pclntab) 混淆。

---

## 编译

```bash
go build -o cross-file-obfuscator cmd/main.go
```

---

## 使用教程

工具提供两种独立的工作模式：

### 模式一：源码混淆 (`-source`)

在源码级别进行标识符替换、字符串加密和控制流混淆。输出一份全新的可编译项目。

#### 场景 1: 交付前全量源码混淆 (推荐)
**适用**: 把整套源码交付给第三方，不想让对方直接看懂业务逻辑。
**效果**: 生成一份全新的可编译项目 (默认输出到 `my_project_obfuscated/`)，文件名、字符串、函数名/变量名全部乱码，并混入垃圾代码干扰阅读。
**流程**:
```bash
# 1) 混淆源码
./cross-file-obfuscator -source -encrypt-strings -inject-junk \
    -obfuscate-filenames -obfuscate-positions ./my_project

# 2) 进到输出目录编译，验证生成物可正常构建即可交付
cd my_project_obfuscated && go build ./...
```

#### 核心可选项 (源码模式)
- `-encrypt-strings`: 字符串加密 (3 种随机加密策略，独立解密包)，运行时解密。自动跳过导入路径、结构体标签、const 初始化、反引号原始串、含转义符及过短的字符串。
- `-inject-junk`: 注入无用的垃圾代码和不透明谓词 (含锚点防折叠机制)。
- `-obfuscate-filenames`: 混淆 .go 源代码文件名。
- `-obfuscate-positions`: 用 `//line` 伪文件名混淆源码位置信息，混淆栈追踪中的真实路径。
- `-obfuscate-exported`: 混淆导出函数名 (注意: 可能破坏外部引用)。
- `-preserve-reflection`: 保留反射/JSON 实际引用的类型与字段 (默认: true，精确到被引用的类型；静态不可达处整文件保护)。
- `-skip-generated`: 跳过自动生成的代码文件 (如 `*.pb.go`，默认: true)。
- `-remove-comments`: 移除代码注释 (默认: true)。
- `-exclude`: 排除特定文件/目录模式 (逗号分隔, 如 `*_test.go`)。
- `-dry-run`: 干跑模式，只打印将要混淆的内容，不写入任何文件。

---

### 模式二：二进制修改 (`-binary`)

直接修改已编译的 Go 二进制文件，重写 pclntab 中的包名和函数名，并清理底层残留的文件路径。

#### 场景 2: 仅交付二进制，做全量混淆
**适用**: 只给对方可执行文件，同时连标准库/第三方库的符号也一并打乱。
**效果**: pclntab 里的包名、函数名、文件路径全部替换，`strings` 命令几乎看不到可读信息。混淆力度最大。用 `-project` 指定源码目录后，可在任意位置执行。
**流程**:
```bash
# 1) 在项目目录里编译出二进制
cd /path/to/my_project && go build -trimpath -o app.exe

# 2) 回到任意目录，用 -project 指到源码，对二进制做全量混淆 (会原地修改 app.exe)
./cross-file-obfuscator -binary -auto-discover-pkgs -obfuscate-third-party \
    -project /path/to/my_project /path/to/app.exe
```

#### 场景 3: 只想让杀软不误报 (最小改动)
**适用**: 程序本身正常，只是被 Windows 杀软经常误报，想保留标准库特征。
**效果**: 只混淆你项目自己的包，标准库保持原样，最大限度降低误报率。
**流程**:
```bash
# 1) 混淆源码后编译
./cross-file-obfuscator -source -encrypt-strings ./my_project
cd my_project_obfuscated && go build -trimpath -ldflags="-s -w" -o app.exe

# 2) 用 -project 指到源码，去掉可读符号但保留标准库 (可在任意目录执行)
/path/to/cross-file-obfuscator -binary -auto-discover-pkgs -only-project \
    -project ./my_project /path/to/app.exe
```

#### 场景 4: 手上只有二进制、没有源码
**适用**: 拿到别人编译好的二进制，没有源码可指，只想打乱其中一部分包名做干扰。
**效果**: 只用关键词精确命中要混淆的包 (如包名含 `mycompany` 或 `api` 的)，其余符号保持不动，出错了也不影响整体。
**命令**:
```bash
./cross-file-obfuscator -binary -auto-discover-pkgs -pkg-filter "mycompany,api" app.exe
```

#### 核心可选项 (二进制模式)
- `-project`: 源码根目录路径 (用于在非项目根目录下执行混淆时寻找 go.mod)。
- `-auto-discover-pkgs`: 自动识别包名 (有 go.mod 时优先解析；无源码时必须配合 `-pkg-filter`)。
- `-obfuscate-third-party`: 混淆第三方依赖包 (如 `github.com/xxx`)。
- `-obfuscate-paths`: 混淆二进制中残留的 .go 源文件绝对路径 (默认开启)。
- `-only-project`: 仅混淆项目自身包，保留标准库。
- `-pkg-filter`: 指定过滤关键字，支持逗号分隔。
- `-pkg-replace`: 手动指定包名映射 (格式: `oldpkg=newpkg`)。
- `-disable-pclntab`: 仅执行基础符号操作，不修改 pclntab 结构。

---

## 命令行参数汇总 (v1.0.5)

| 参数 | 模式 | 说明 |
| :--- | :--- | :--- |
| `-source` | 源码 | 启动源码混淆模式 |
| `-binary` | 二进制 | 启动二进制修改模式 |
| `-o` | 源码 | 指定输出目录 (默认: `项目名_obfuscated`) |
| `-encrypt-strings` | 源码 | 加密字符串字面量 (3 种随机策略，独立解密包；跳过导入路径/标签/const) |
| `-inject-junk` | 源码 | 注入随机化的垃圾代码和不透明谓词 |
| `-obfuscate-filenames` | 源码 | 混淆 .go 文件名 |
| `-obfuscate-positions` | 源码 | 用 `//line` 混淆位置信息 (栈追踪伪装) |
| `-obfuscate-exported` | 源码 | 混淆导出函数名 (注意: 可能破坏外部引用) |
| `-preserve-reflection` | 源码 | 保留反射/JSON 引用类型 (默认: true) |
| `-skip-generated` | 源码 | 跳过生成的代码文件 (默认: true) |
| `-remove-comments` | 源码 | 移除注释 (默认: true) |
| `-exclude` | 源码 | 排除文件/目录模式 |
| `-dry-run` | 源码 | 干跑模式，预览不写入 |
| `-project` | 二进制 | 源码根目录 (用于寻找 go.mod) |
| `-auto-discover-pkgs` | 二进制 | 自动发现并替换项目内所有包名 |
| `-obfuscate-third-party` | 二进制 | 混淆第三方依赖包 |
| `-obfuscate-paths` | 二进制 | 抹除底层 .go 文件路径 (默认开启) |
| `-only-project` | 二进制 | 仅混淆项目包，保留标准库 |
| `-pkg-filter` | 二进制 | 包名过滤关键字 (逗号分隔) |
| `-pkg-replace` | 二进制 | 手动包名映射 (`oldpkg=newpkg`) |
| `-disable-pclntab` | 二进制 | 跳过 pclntab 修改 |
| `-log-level` | 全局 | 日志级别 (debug/info/warn/error) |
| `-h` | 全局 | 显示详细帮助信息 |

---



## 许可证

MIT License
