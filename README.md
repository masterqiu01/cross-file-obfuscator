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

#### 场景 1: 源码混淆 (适用于交付源码)
**功能**: 混淆文件名，加密字符串，注入控制流混淆代码。
**命令**:
```bash
./cross-file-obfuscator -source -encrypt-strings -inject-junk -obfuscate-filenames -o out_src ./your-project
```

#### 核心可选项
- `-encrypt-strings`: XOR 加密字符串字面量，运行时解密。
- `-inject-junk`: 注入无用的垃圾代码和不透明谓词。
- `-obfuscate-filenames`: 混淆 .go 源代码文件名。
- `-obfuscate-exported`: 混淆导出函数名 (注意: 可能破坏外部引用)。
- `-exclude`: 排除特定文件/目录模式 (逗号分隔, 如 `*_test.go`)。

---

### 模式二：二进制修改 (`-binary`)

直接修改已编译的 Go 二进制文件，重写 pclntab 中的包名和函数名，并清理底层残留的文件路径。

#### 场景 2: 二进制全量混淆 (推荐)
**功能**: 读取 go.mod 自动混淆所有项目包与第三方包，替换文件路径为不可见字符。
**命令**: (需在包含 go.mod 的目录下执行)
```bash
go build -trimpath -o app.exe
./cross-file-obfuscator -binary -auto-discover-pkgs -obfuscate-third-party app.exe
```

#### 场景 3: 最小化二进制混淆 (适用于 Windows 防误报)
**功能**: 仅对项目自身包进行混淆，保留原生标准库特征以降低杀软误报率。
**命令**:
```bash
./cross-file-obfuscator -source -encrypt-strings ./my_project
cd ./my_project_obfuscated && go build -trimpath -ldflags="-s -w" -o app.exe
./cross-file-obfuscator -binary -auto-discover-pkgs -only-project app.exe
```

#### 场景 4: 无源码二进制局部混淆
**功能**: 仅通过指定关键字，对无源码的二进制文件进行混淆。
**命令**:
```bash
./cross-file-obfuscator -binary -auto-discover-pkgs -pkg-filter "mycompany,api" app.exe
```

#### 核心可选项
- `-project`: 源码根目录路径 (用于在非项目根目录下执行混淆时寻找 go.mod)。
- `-auto-discover-pkgs`: 自动识别包名 (有 go.mod 时优先解析；无源码时必须配合 `-pkg-filter`)。
- `-obfuscate-third-party`: 混淆第三方依赖包 (如 `github.com/xxx`)。
- `-obfuscate-paths`: 混淆二进制中残留的 .go 源文件绝对路径 (默认开启)。
- `-only-project`: 仅混淆项目自身包，保留标准库。
- `-pkg-filter`: 指定过滤关键字，支持逗号分隔。

---

## 命令行参数汇总 (v1.0.3)

| 参数 | 说明 |
| :--- | :--- |
| `-source` | 启动源码混淆模式 |
| `-binary` | 启动二进制修改模式 |
| `-project` | 源码根目录 (用于二进制模式寻找 go.mod) |
| `-o` | 指定输出目录 (源码模式) |
| `-encrypt-strings` | 启用字符串 XOR 加密 |
| `-inject-junk` | 注入随机化的垃圾代码和不透明谓词 |
| `-auto-discover-pkgs` | 自动发现并替换项目内所有包名 |
| `-obfuscate-third-party` | 混淆第三方依赖包 (配合二进制模式) |
| `-obfuscate-paths` | 抹除底层 .go 文件路径字符串 (配合二进制模式) |
| `-only-project` | 仅混淆项目包，保留标准库 (推荐 Windows 防误报) |
| `-h` | 显示详细帮助信息 |

---



## 许可证

MIT License
