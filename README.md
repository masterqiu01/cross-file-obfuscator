# Go 代码混淆器 (Cross-File Obfuscator)
 Go 代码混淆工具，使用 AST (抽象语法树) 技术实现跨文件的代码混淆，同时保证混淆后的代码**可编译和可执行**。

[English][url-docen]

## 核心亮点

- **一键自动模式** - 全功能混淆 + 自动编译，一条命令搞定
- **第三方包混淆** - 可选混淆第三方依赖，进一步隐藏代码信息
- **链接器级别混淆** - 直接修改二进制文件混淆函数名和包路径
- **自动包名发现** - 智能发现并混淆项目包名、标准库包名
- **智能区域保护** - embed 文件智能保护，不会破坏内嵌资源
- **多层混淆技术** - 变量名+函数名+字符串加密+控制流混淆+二进制混淆
- **智能保护机制** - 自动识别和保护导出符号、结构体字段、方法、接口等
- **反射自动保护** - 检测并保护使用反射的类型和方法
- **JSON/XML 智能处理** - 自动保护序列化相关的字段和类型
- **接口完整保护** - 自动保护接口定义和实现，确保不破坏多态性
- **嵌入字段支持** - 正确处理匿名嵌入字段（结构体和指针）
- **内置标识符保护** - 保护所有 Go 内置类型、函数和常量
- **CGO 智能跳过** - 自动检测并跳过 CGO 代码文件
- **生成代码识别** - 自动跳过 protobuf 等生成的代码
- **灵活排除规则** - 支持自定义文件排除模式
- **高性能** - AST遍历优化，处理速度快
- **强随机性** - 所有混淆名称完全随机，无规律可循
- **详细报告** - 运行时显示跳过文件、反射使用等详细信息

## 特性

### 核心功能

1. **私有函数名混淆**
   - 只混淆未导出（小写开头）的函数
   - 保持跨文件引用的一致性
   - 生成随机不可预测的函数名

2. **私有变量名混淆**
   - 混淆局部变量和全局未导出变量
   - 智能区分同名的函数和变量
   - 使用随机字符串命名

3. **标准库包别名** 
   - **自动识别**标准库包（无需手动维护列表）
   - 只混淆标准库，第三方包保持原样
   - 为所有标准库包创建一致的随机别名
   - 自动替换代码中的包引用
   - 防止通过包名推测代码功能

4. **智能保护机制**
   
   **基础保护**：
   -  **导出符号保护** - 默认不混淆导出的函数、变量、类型（大写开头）
   -  **结构体字段保护** - 保护所有结构体字段名（包括匿名嵌入字段）
   -  **方法保护** - 保护所有带接收者的方法名
   -  **接口保护** - 自动保护接口定义中的所有方法签名
   
   **选择器智能保护**：
   -  保护外部包和标准库的选择器（如 `fmt.Println`、`os.Open`）
   -  保护对象方法调用（如 `obj.Method()`、`conn.Close()`）
   -  允许混淆项目内部包的跨文件引用（保持一致性）
   
   **包名保护**：
   -  保护所有导入的包名（包括标准库和第三方包）
   -  标准库使用随机别名（如 `import p8xK2mQr "fmt"`）
   -  第三方包保持原样（确保兼容性）
   
   **内置标识符保护**（33+个）：
   -  特殊标识符：`main`、`init`、`_`
   -  内置类型：`int`、`string`、`bool`、`byte`、`rune`、`error`、`float32`、`float64`、`complex64`、`complex128`、`uint`、`int8`、`int16`、`int32`、`int64`、`uint8`、`uint16`、`uint32`、`uint64`、`uintptr`
   -  内置函数：`len`、`cap`、`make`、`new`、`append`、`copy`、`delete`、`panic`、`recover`、`print`、`println`、`close`、`min`、`max`、`clear`
   -  内置常量：`nil`、`true`、`false`、`iota`

### 高级混淆功能

5. **字符串加密** 
   - XOR 加密所有字符串字面量
   - 随机生成解密函数名
   - 运行时自动解密
   - 64字节随机密钥

6. **常量表达式化** 
   - 将数字常量转换为数学表达式
   - 例如：`999999` → `(1000*1000-1)`
   - 增加静态分析难度

7. **注释删除** 
   - 自动移除所有代码注释
   - 包括文档注释、行内注释
   - 隐藏开发者意图和说明

8. **不透明谓词** 
   - 注入基于数学的不透明谓词
   - 使用永真/永假条件混淆控制流
   - 包括：x²≥0, (x²+x)%2==0, 2x>x 等
   - 所有变量名随机生成

## 使用方法

### 编译

```bash
go build -o cross-file-obfuscator cmd/obfuscator/main.go
```

#### 注意事项

1. **CGO 代码**：
   - 如果项目包含 CGO 代码，交叉编译需要对应平台的 C 编译器
   - 建议交叉编译时禁用 CGO：`CGO_ENABLED=0`
   - 或使用 `-exclude` 参数排除 CGO 文件

2. **平台特定代码**：
   - Build 标签文件（`_linux.go`, `_windows.go`）会自动处理
   - Go 编译器会根据 GOOS/GOARCH 自动选择对应文件

3. **文件扩展名**：
   - Windows 平台建议使用 `.exe` 扩展名
   - Linux/macOS 平台通常不需要扩展名

4. **strip 工具**：
   - 链接器混淆会自动调用 `strip` 移除符号表
   - 交叉编译时，strip 可能不支持目标平台格式
   - 工具会尝试使用，失败时会给出警告但不影响混淆

### 基本用法

```bash
# 基础混淆（变量名、函数名、包别名）
./cross-file-obfuscator <项目目录>

# 使用所有高级功能（推荐）
./cross-file-obfuscator -obfuscate-exported -obfuscate-filenames -encrypt-strings -inject-junk -remove-comments <项目目录>
```

### 命令行选项

#### 基础选项

```bash
-o <目录>                    指定输出目录（默认：项目名_obfuscated）
-obfuscate-exported          混淆导出的函数和变量（可能破坏API）
-obfuscate-filenames         混淆 Go 文件名
-encrypt-strings             加密字符串字面量
-inject-junk                 注入垃圾代码（不透明谓词）
-remove-comments             删除所有注释（默认：true）
-preserve-reflection         保护反射相关的类型和方法（默认：true）
-skip-generated              跳过自动生成的代码文件（默认：true）
-exclude <模式>              排除文件模式，逗号分隔（如：'*_test.go,*.pb.go,tools/*'）
```

#### 高级选项（链接器混淆）

```bash
-auto                        自动模式：全功能混淆 + 自动编译（推荐）
-build-with-linker           直接编译并应用链接器混淆
-output-bin <文件名>         输出二进制文件名（配合 -build-with-linker 或 -auto 使用）
-entry <包路径>              入口包路径（多入口项目必须指定）
                             - 默认: "." (根目录)
                             - 示例: "./cmd/server", "./cmd/gost"
                             - 用于指定main包位置
-pkg-replace <映射>          包名替换映射（格式: 'original1=new1,original2=new2'）
-auto-discover-pkgs          自动发现并替换项目中的所有包名（推荐）
-obfuscate-third-party       混淆第三方依赖包（谨慎使用，可能影响稳定性）
```

**`-entry` 参数说明**：

- **何时需要**：当你的main包不在项目根目录时（如 `cmd/app/main.go`）
- **如何确定**：运行 `find ./project -name "*.go" -exec grep -l "func main()" {} \;`
- **常见场景**：
  - `main.go` 在根目录 → 无需指定
  - `cmd/server/main.go` → 需要 `-entry "./cmd/server"`
  - `cmd/gost/main.go` → 需要 `-entry "./cmd/gost"`

### 使用示例

#### 🚀 推荐：自动模式

```bash
# 标准项目（main在根目录）
./cross-file-obfuscator -auto -output-bin myapp ./my-project

# 多入口项目（main在子目录）- 必须指定 -entry
./cross-file-obfuscator -auto -entry "./cmd/server" -output-bin server ./my-project

# 自动模式 + 第三方包混淆（谨慎使用）
./cross-file-obfuscator -auto -obfuscate-third-party -output-bin myapp ./my-project
```

#### 链接器混淆示例

```bash
# 标准项目 - 自动发现包名（推荐）
./cross-file-obfuscator -build-with-linker -auto-discover-pkgs -output-bin myapp ./my-project

# 多入口项目 - 需要指定entry
./cross-file-obfuscator -build-with-linker -auto-discover-pkgs -entry "./cmd/server" -output-bin server ./my-project

# 链接器混淆 + 第三方包混淆
./cross-file-obfuscator -build-with-linker -auto-discover-pkgs -obfuscate-third-party -output-bin myapp ./my-project

# 手动指定包名替换
./cross-file-obfuscator -build-with-linker -pkg-replace 'main=m,github.com/user/project=a' -output-bin myapp ./my-project
```

#### 组合使用（最强混淆）

```bash
# 第一步：源码混淆
./cross-file-obfuscator -encrypt-strings -inject-junk -obfuscate-exported ./my-project

# 第二步：链接器混淆（对混淆后的源码）
./cross-file-obfuscator -build-with-linker -auto-discover-pkgs -output-bin myapp ./my-project_obfuscated

# 或者直接使用auto模式（一步到位）
./cross-file-obfuscator -auto -output-bin myapp ./my-project
```

### 🚀 自动模式（Auto Mode）- 一键全功能混淆

**最简单的使用方式！** 自动模式会自动执行所有混淆步骤：

**自动模式包含的功能**：
- 字符串加密（`-encrypt-strings`）
- 垃圾代码注入（`-inject-junk`）
- 导出符号混淆（`-obfuscate-exported`）
- 文件名混淆（`-obfuscate-filenames`）
- 自动包名发现（`-auto-discover-pkgs`）
- 链接器混淆（`-build-with-linker`）
- 第三方包混淆（`-obfuscate-third-party`）

**输出**：
- `./my-project_obfuscated` - 混淆后的源码
- `myapp` - 混淆后的可执行文件

---

### 重要：多入口项目配置

**如何判断是否需要指定 `-entry` 参数？**

检查你的项目结构：

```bash
# 方法1: 查找main函数位置
find ./your-project -name "*.go" -exec grep -l "func main()" {} \;

# 方法2: 检查项目结构
ls -la ./your-project/*.go  # 如果根目录有main.go，无需指定entry
ls -la ./your-project/cmd/  # 如果main在cmd子目录，需要指定entry
```

**使用规则**：

| 项目结构 | 是否需要-entry | 命令示例 |
|---------|---------------|---------|
| `main.go` 在根目录 | 不需要 | `./cross-file-obfuscator -auto -output-bin app ./project` |
| `cmd/app/main.go` | **需要** | `./cross-file-obfuscator -auto -entry "./cmd/app" -output-bin app ./project` |
| `cmd/server/main.go` | **需要** | `./cross-file-obfuscator -auto -entry "./cmd/server" -output-bin server ./project` |

**常见错误**：

如果看到以下错误，说明需要指定 `-entry` 参数：

```
检测到二进制格式: Unknown
后处理失败: 不支持的二进制格式: Unknown
```

或者生成的文件是 `ar archive` 而不是可执行文件：

```bash
$ file output
output: current ar archive  # 错误：这是archive，不是可执行文件
```

**解决方案**：

1. 查找main包位置：
```bash
find ./project -name "*.go" -exec grep -l "func main()" {} \;
# 输出: ./project/cmd/gost/main.go
```

2. 添加 `-entry` 参数：
```bash
./cross-file-obfuscator -auto -entry "./cmd/gost" -output-bin output ./project
```

---

### 第三方包混淆

默认情况下，混淆器只混淆项目包名和标准库包名，**不混淆第三方依赖**（如 `github.com/xxx`）。

如果需要更深度的混淆，可以启用第三方包混淆：

```bash
# 自动模式
./cross-file-obfuscator -auto -output-bin myapp ./my-project

# 链接器混淆 + 第三方包混淆
./cross-file-obfuscator -build-with-linker -auto-discover-pkgs -obfuscate-third-party -output-bin myapp ./my-project
```

**效果**：
- 项目包名：`github.com/xxxx/xxxx` → `a`
- 标准库：`runtime.` → `b.`
- 第三方包：`github.com/xxx/xxx/runtime/Go` → `c`

**注意事项**：
- 第三方包混淆可能影响程序稳定性
- 建议先测试不启用此选项的效果
- 如果程序崩溃，请禁用此选项

---

#### 特性

- **跨平台支持** - 支持 ELF (Linux)、PE (Windows)、Mach-O (macOS)
- **函数名混淆** - 替换所有包名前缀（main., runtime., fmt. 等）
- **自动包名发现** - 自动扫描项目并替换所有内部包名（38+ 标准库包）
- **智能路径替换** - 只替换项目包路径，保护系统符号不被破坏
- **符号表移除** - 自动使用 strip 移除调试符号
- **程序可运行** - 混淆后的程序完全可以正常运行  

#### 自动包名发现功能

使用 `-auto-discover-pkgs` 参数时，混淆器会：

1. **自动扫描项目**
   - 读取 go.mod 获取模块名
   - 递归扫描项目目录
   - 发现所有内部包

2. **添加标准库包**（38+ 个）
   - 核心：main, runtime, sync, syscall
   - I/O：fmt, io, bufio, os, log
   - 网络：net, http
   - 字符串：strings, bytes, strconv, unicode, regexp
   - 编码：encoding, json, xml, base64, hex
   - 其他：time, math, sort, errors, context, compress, hash 等

3. **智能替换策略**
   - 替换函数名前缀（`runtime.GC` → `i.GC`）
   - 替换项目包路径（`github.com/user/project/pkg` → `a`）
   - 不替换标准库路径（避免破坏系统符号）
   - 不替换第三方包（不是你的代码）

**示例输出**：
```
发现模块名: github.com/xxxxx/xxxxx
发现 7 个项目包
添加 38 个标准库包
生成了 45 个包名替换映射
替换了 17,090 个函数名前缀
替换了 51 个项目包路径引用
```
## 技术细节

### 工作流程

混淆器采用五阶段流程，确保安全可靠的代码混淆：

#### 第一阶段：收集保护名称

遍历所有 Go 文件，识别需要保护的标识符：

1. **结构体字段保护**
   - 收集所有命名字段
   - 识别匿名嵌入字段（结构体和指针）
   - 标记为受保护名称

2. **接口方法保护**
   - 扫描接口定义
   - 收集所有方法签名
   - 确保接口实现不被破坏

3. **方法保护**
   - 识别带接收者的函数
   - 保护所有方法名
   - 维护类型方法集

4. **选择器表达式保护**
   - 收集 `obj.Field`、`pkg.Function` 等选择器
   - 区分项目内部和外部包
   - 保护外部包和对象的选择器

5. **反射和序列化检测**
   - 检测 `reflect` 包的使用
   - 检测 `encoding/json`、`encoding/xml` 等
   - 自动保护相关类型和字段

#### 第二阶段：作用域分析

使用 AST 分析构建完整的作用域树：

1. **包级作用域**
   - 识别所有包级声明
   - 构建对象映射（函数、变量、类型）
   - 记录对象的文件位置

2. **函数作用域**
   - 分析函数参数和返回值
   - 识别局部变量
   - 处理闭包和嵌套函数

3. **块级作用域**
   - 处理 if/for/switch 等块
   - 正确处理变量遮蔽
   - 维护作用域层次结构

4. **跨文件引用分析**
   - 识别同包不同文件的引用
   - 处理 build 标签文件（如 `_linux.go`、`_windows.go`）
   - 确保同名对象使用相同的混淆名

#### 第三阶段：构建混淆映射

生成唯一的混淆名称：

1. **私有函数混淆**
   - 只混淆未导出的函数（小写开头）
   - 为每个唯一函数生成随机名称
   - 处理同名函数（不同 build 标签文件）

2. **私有变量混淆**
   - 混淆包级和局部变量
   - 避免与函数名冲突
   - 保持跨文件引用一致性

3. **标准库包别名**
   - 自动识别标准库（无域名的导入路径）
   - 生成随机别名（如 `p8xK2mQr`）
   - 第三方包保持原样

4. **文件名混淆**（可选）
   - 生成随机文件名
   - 保留平台后缀（`_linux`、`_windows`、`_amd64` 等）
   - 确保 build 标签正常工作

#### 第四阶段：复制项目文件

1. **目录结构复制**
   - 完整复制项目目录树
   - 保持相对路径关系
   - 跳过 vendor 和隐藏目录

2. **文件过滤**
   - 跳过 CGO 文件（包含 `import "C"`）
   - 跳过生成代码（包含 `// Code generated`）
   - 应用用户排除规则（`-exclude` 参数）

3. **非 Go 文件处理**
   - 复制 go.mod、go.sum
   - 复制配置文件、资源文件
   - 保持文件权限

#### 第五阶段：应用混淆

1. **导入语句处理**
   - 为标准库添加随机别名
   - 保持第三方包导入不变
   - 更新包引用

2. **标识符替换**
   - 使用作用域分析精确替换
   - 优先使用对象映射（避免局部变量冲突）
   - 回退到名称映射（跨文件引用）
   - 跳过所有受保护的名称

3. **高级混淆应用**（可选）
   - 字符串加密：XOR + Base64
   - 垃圾代码注入：不透明谓词
   - 注释移除：清理所有注释

4. **代码格式化**
   - 使用 `go/format` 格式化
   - 确保语法正确
   - 保持代码可读性（用于调试）

### 核心算法

#### 1. 作用域感知替换

```
替换标识符时的决策树：
1. 是否为受保护名称？→ 跳过
2. 是否为导出名称且未启用导出混淆？→ 跳过
3. 在当前文件的包级作用域查找对象
   - 找到且有混淆名？→ 使用对象的混淆名
4. 在跨文件映射中查找（funcMapping/varMapping）
   - 找到？→ 使用映射的混淆名
5. 否则 → 保持原样
```

#### 2. Build 标签处理

对于包含 build 标签的文件（如 `file_linux.go`、`file_windows.go`）：

```
同名对象策略：
- 识别：检测同一包中不同文件的同名函数/变量
- 映射：所有同名对象使用相同的混淆名
- 原因：Go 编译器只会编译匹配平台的文件，不会有冲突
- 示例：
  - gost/sockopts_linux.go:  func setSocketMark() { ... }
  - gost/sockopts_other.go:  func setSocketMark() { ... }
  - 两者都映射到：fnXXXXXXXXXX
```

#### 3. 文件名混淆策略

```
保留平台后缀：
- 检测：识别 _linux, _windows, _darwin, _amd64, _arm64 等后缀
- 保留：混淆文件名但保留后缀
- 示例：
  - 原文件：conn_linux_amd64.go
  - 混淆后：fXXXXXXXX_linux_amd64.go
- 原因：Go 编译器依赖这些后缀进行条件编译
```

#### 4. 字符串加密算法

```
加密流程：
1. 生成 64 字节随机密钥
2. 对每个字符串：
   - XOR 加密：plaintext[i] ^ key[i % len(key)]
   - Base64 编码：避免二进制数据
   - 替换：原字符串 → decrypt("base64data")
3. 注入解密函数（每个包一次）
4. 运行时解密：程序启动时自动解密
```

### 保护机制层次

混淆器使用五层保护机制，确保代码安全：

| 层次 | 保护内容 | 实现位置 | 优先级 |
|------|---------|---------|--------|
| 第一层 | 特殊标识符（`main`、`init`、`_`） | `shouldProtect()` | 最高 |
| 第二层 | Go 内置标识符（33+个） | `shouldProtect()` | 最高 |
| 第三层 | 导出名称（大写开头） | `shouldProtect()` | 高 |
| 第四层 | 结构化保护（字段、方法、接口） | `collectProtectedNames()` | 高 |
| 第五层 | 上下文保护（反射、JSON、CGO） | `protectReflectionTypes()` | 中 |

**保护优先级说明**：
- 任何层次的保护都会阻止混淆
- 多层保护提供冗余安全
- 即使某层失效，其他层仍然保护

## 设计原则

### 1. 可靠性优先 - 100%编译成功

**核心承诺**：混淆后的代码必须能够编译和运行

**实现方式**：
- **五层保护机制**：多重保护确保关键标识符不被误混淆
- **作用域分析**：精确识别标识符的作用域，避免错误替换
- **Build 标签支持**：正确处理平台相关代码（`_linux.go`、`_windows.go`）
- **文件名后缀保留**：保持 Go 编译器的条件编译功能
- **保守策略**：遇到不确定的情况，选择不混淆而不是冒险

---

### 2. 跨文件一致性 - 同名同混淆

**核心原则**：同一个标识符在所有文件中使用相同的混淆名

**为什么重要**：
- Go 允许同一包的多个文件共享包级声明
- 一个文件中定义的函数可以在另一个文件中调用
- Build 标签文件可能定义同名函数（但只有一个会被编译）

**实现方式**：
```
包级对象映射：
- 扫描所有文件，收集包级声明
- 为每个唯一名称生成一个混淆名
- 所有文件共享同一个映射表

特殊处理：Build 标签文件
- file_linux.go: func connect() { ... }
- file_windows.go: func connect() { ... }
- 两者映射到同一个混淆名：fnXXXXXXXXXX
- 原因：Go 编译器只会选择一个文件编译
```

**示例**：
```go
// file1.go
func helper() { ... }  // → fnAbc123xyz

// file2.go  
func process() {
    helper()  // → fnAbc123xyz（相同的混淆名）
}
```

---

### 3. 智能保护 - 宁可少混淆，不可破坏

**核心原则**：遇到不确定的情况，选择保护而不是混淆

**保护优先级**：
1. **必须保护**：Go 内置标识符、`main`、`init`
2. **默认保护**：导出名称、结构体字段、方法
3. **上下文保护**：反射、JSON、CGO 相关
4. **智能保护**：选择器表达式、接口方法

---

### 4. 渐进式混淆 - 从保守到激进

**核心理念**：提供多个混淆级别，用户可以逐步测试

**混淆级别**：

| 级别 | 混淆内容 | 风险 | 推荐场景 |
|------|---------|------|---------|
| **基础** | 私有函数、变量、包别名 | 低 | 所有项目 |
| **中级** | + 字符串加密、垃圾代码 | 低 | 大多数项目 |
| **高级** | + 导出符号混淆 | 中 | 无外部依赖的项目 |
| **完整** | + 文件名混淆、第三方包 | 高 | 单体应用 |

**推荐流程**：
```bash
# 第一步：基础混淆测试
./cross-file-obfuscator ./project
cd project_obfuscated && go build

# 第二步：中级混淆测试
./cross-file-obfuscator -encrypt-strings -inject-junk ./project
cd project_obfuscated && go build

# 第三步：高级混淆测试（谨慎）
./cross-file-obfuscator -obfuscate-exported -encrypt-strings -inject-junk ./project
cd project_obfuscated && go build

# 第四步：完整混淆（auto模式）
./cross-file-obfuscator -auto -output-bin app ./project
```

---

### 5. 性能与安全平衡

**核心目标**：在保证性能的前提下，最大化混淆效果

**性能考虑**：
- **编译时间**：混淆增加 5-10% 的编译时间
- **运行时性能**：
  - 基础混淆：0% 性能影响（只是重命名）
  - 字符串加密：< 1% 性能影响（启动时解密）
  - 垃圾代码：< 1% 性能影响（编译器优化）
- **二进制大小**：增加 6-9%（垃圾代码和字符串加密）

**安全效果**：
- **函数名泄漏**：大幅减少（链接器混淆）
- **包路径泄漏**：减少 99%（链接器混淆）
- **字符串泄漏**：减少 100%（字符串加密）
- **逆向难度**：显著提升（综合效果）

**权衡示例**：
```
不透明谓词注入：
- 增加代码量：+5-10%
- 增加二进制大小：+3-5%
- 运行时开销：0%（编译器优化掉）
- 逆向难度：+20%（混淆控制流）
→ 结论：值得使用
```

---

### 特殊场景处理

#### Build 标签文件

**场景**：同一包中有多个平台相关文件

```go
// file_linux.go
//go:build linux
func connect() { ... }

// file_windows.go  
//go:build windows
func connect() { ... }
```

**处理**：
- 两个 `connect` 函数使用相同的混淆名
- 文件名保留平台后缀：`fXXX_linux.go`, `fYYY_windows.go`
- Go 编译器只会选择一个文件，不会冲突

---

#### 反射使用

**场景**：代码使用 `reflect` 包

```go
import "reflect"

type User struct {
    Name string  // 自动保护
}

func process() {
    t := reflect.TypeOf(User{})
    // 反射需要原始名称
}
```

**处理**：
- 自动检测 `reflect` 包的使用
- 保护所有类型、字段、方法名
- 使用 `-preserve-reflection=true`（默认）

---

#### CGO 代码

**场景**：代码包含 C 互操作

```go
// #include <stdio.h>
import "C"

func callC() {
    C.printf(...)
}
```

**处理**：
- 自动检测 `import "C"`
- 跳过整个文件的混淆
- 避免破坏 C 互操作性

---

#### 生成代码

**场景**：protobuf、gRPC 等生成的代码

```go
// Code generated by protoc-gen-go. DO NOT EDIT.
package pb

type Message struct { ... }
```

**处理**：
- 自动检测生成代码标记
- 跳过混淆（使用 `-skip-generated=true`，默认）
- 或使用 `-exclude "*.pb.go"` 手动排除

### 已解决的问题

以下问题已在最新版本中解决：

1. **使用反射的代码** 
   - 自动检测使用 `reflect` 包的文件
   - 自动保护所有类型名、字段名和方法名
   - 使用 `-preserve-reflection` 标志控制（默认开启）
   - 运行时显示使用反射的包列表

2. **CGO 代码** 
   - 自动检测包含 `import "C"` 的文件
   - 自动跳过混淆，避免破坏 C 互操作性
   - 运行时显示跳过的文件及原因

3. **生成的代码（如 protobuf）** 
   - 自动检测包含 `Code generated` 等标记的文件
   - 使用 `-skip-generated` 标志控制（默认开启）
   - 支持 `-exclude` 参数手动排除特定模式（如 `*.pb.go`）
   - 运行时显示跳过的文件列表

4. **JSON/XML 序列化** 
   - 自动检测使用 `encoding/json`、`encoding/xml`、`yaml` 的文件
   - 智能处理结构体标签：
     - 有显式标签的字段：使用标签名称，字段名可以混淆
     - 无标签的字段：保护字段名，确保序列化正确
   - 自动保护在序列化中使用的类型和字段

5. **接口实现** 
   - 自动检测并保护接口方法名
   - 确保接口实现不会被破坏
   - 保护所有接口定义中的方法签名

6. **嵌入字段（Embedded Fields）** 
   - 自动检测并保护匿名嵌入字段
   - 支持结构体嵌入：`type A struct { B }`
   - 支持指针嵌入：`type A struct { *B }`

7. **内置类型和函数** 
   - 保护所有 Go 内置类型（`int`、`string`、`error` 等）
   - 保护所有内置函数（`len`、`make`、`append` 等）
   - 保护内置常量（`nil`、`true`、`false`、`iota`）
   - 避免与 Go 关键字冲突

### 包混淆策略说明

本工具采用**智能识别**策略：

- **标准库包**（如 `fmt`, `net/http`, `encoding/json`）：自动识别并混淆
  - 识别规则：import路径不包含域名（如 `github.com`, `gopkg.in`）
  - 混淆方式：生成随机别名，如 `pkgB2nM5wQr "fmt"`

- **第三方包**（如 `github.com/go-ldap/ldap/v3`）：保持原样
  - 原因：避免版本后缀（v2, v3等）和实际包名不匹配的复杂性
  - 好处：确保编译100%成功，无需维护包名映射表

## 智能保护功能

### 反射保护（Reflection Protection）

工具会自动检测使用 `reflect` 包的代码，并保护相关类型和方法：

- **自动检测**：扫描所有文件的 import 语句
- **全面保护**：
  - 类型名称（`type MyStruct struct`）
  - 所有结构体字段
  - 所有方法（包括接收者方法）
- **智能提示**：运行时显示使用反射的包列表

### CGO 代码跳过（CGO Code Skip）

自动检测并跳过包含 C 代码的文件：

- **检测方式**：查找 `import "C"` 语句
- **跳过原因**：避免破坏 Go 与 C 的互操作性
- **透明提示**：显示跳过的文件及原因


### 生成代码跳过（Generated Code Skip）

自动识别并跳过自动生成的代码文件：

- **识别标记**：
  - `// Code generated`
  - `// DO NOT EDIT`
  - `// autogenerated`
  - `// AUTO-GENERATED`
  - `// Code generated by`
- **常见文件类型**：
  - Protobuf 文件（`*.pb.go`）
  - gRPC 文件（`*.pb.gw.go`）
  - mock 文件（`*_mock.go`）
  - 其他代码生成工具的输出

### 自定义排除模式（Custom Exclude Patterns）

使用 `-exclude` 参数手动排除文件：

```bash
# 排除测试文件
./cross-file-obfuscator -exclude "*_test.go" myproject

# 排除多种文件类型
./cross-file-obfuscator -exclude "*.pb.go,*.pb.gw.go,*_mock.go" myproject

# 排除特定文件
./cross-file-obfuscator -exclude "config.go,version.go" myproject
```

## 故障排除

### Archive输出错误（常见问题）

**症状**：编译输出是 `ar archive` 而不是可执行文件

```bash
$ file output
output: current ar archive  # 错误
```

或者看到错误：
```
检测到二进制格式: Unknown
后处理失败: 不支持的二进制格式: Unknown
```

**原因**：main包不在项目根目录，但没有指定 `-entry` 参数

**解决方案**：

1. **查找main包位置**：
```bash
find ./your-project -name "*.go" -exec grep -l "func main()" {} \;
# 输出示例: ./your-project/cmd/gost/main.go
```

2. **添加 `-entry` 参数**：
```bash
# 如果main在 cmd/gost/main.go
./cross-file-obfuscator -auto -entry "./cmd/gost" -output-bin output ./your-project

# 如果main在 cmd/server/main.go
./cross-file-obfuscator -auto -entry "./cmd/server" -output-bin output ./your-project
```

3. **验证成功**：
```bash
$ file output
output: Mach-O 64-bit executable arm64  # 正确
```

**快速检查表**：

| 项目结构 | 需要-entry? | 正确命令 |
|---------|-----------|---------|
| `main.go` | | `./cross-file-obfuscator -auto -output-bin app ./project` |
| `cmd/app/main.go` | | `./cross-file-obfuscator -auto -entry "./cmd/app" -output-bin app ./project` |

---

### 编译失败

如果混淆后的代码无法编译：

1. **检查是否需要指定entry**：如果main包不在根目录，添加 `-entry` 参数
2. **检查反射保护**：确保 `-preserve-reflection=true`（默认）
3. **检查排除规则**：使用 `-exclude` 排除问题文件
4. **查看跳过文件**：检查运行时的 "Skipped files" 列表
5. **关闭高级功能**：尝试不使用 `-encrypt-strings` 和 `-inject-junk`
6. **检查内置标识符**：确保没有与 Go 关键字冲突

### 运行时错误

如果混淆后的代码编译成功但运行失败：

1. **反射问题**：检查是否所有使用反射的包都被正确识别
2. **接口实现**：确保接口方法名称被保护
3. **结构体标签**：检查 JSON/XML 等结构体标签是否正确
4. **嵌入字段**：验证匿名嵌入字段是否正常工作

### JSON/XML 序列化问题

如果序列化/反序列化失败：

1. **检查标签**：确保所有需要序列化的字段都有显式标签
2. **验证字段名**：检查混淆后的 JSON 输出是否符合预期
3. **导出字段**：确保需要序列化的字段都是导出的（大写开头）

### 常见问题

**Q: 为什么我的私有函数没有被混淆？**
A: 检查是否是方法（带接收者）或者在保护列表中

**Q: 接口实现被破坏了怎么办？**
A: 工具会自动保护接口方法，检查是否有其他原因

**Q: 如何排除某个目录的所有文件？**
A: 使用 `-exclude "dirname/*"` 模式

**Q: CGO 代码被混淆导致失败？**
A: 工具会自动跳过 CGO 文件，检查是否包含 `import "C"`

### 链接器混淆常见问题

**Q: 使用 strings 查看二进制文件，还能看到 `runtime` 字符串？**
A: 这是正常的！你看到的是错误消息（如 `"runtime error:"`），不是函数名。检查 `runtime.` 函数名前缀应该是 0 个。

**Q: 为什么还能看到第三方包路径（如 `github.com/antlr/...`）？**
A: 这些是第三方依赖库的路径，不是你的代码。混淆的目的是保护你的代码，不是隐藏使用的开源库。

**Q: 如何验证混淆是否成功？**
A: 使用 `strings myapp | grep '^main\.'` 检查函数名前缀，应该返回 0。不要检查所有包含 `main` 的字符串。

**Q: 自动发现会替换哪些包？**
A: 会替换你的项目包和 38+ 个标准库包（main, runtime, fmt, sync 等），但不会替换第三方依赖包。

**Q: 程序崩溃怎么办？**
A: 检查是否依赖反射中的包路径字符串。使用备份文件（`.backup`）恢复原始二进制。

**Q: 使用 `embed` 嵌入文件后，混淆的程序报错 "control characters are not allowed" 或找不到文件？**
A: 这个问题已经修复！当前版本采用**智能区域识别**策略，确保只在 pclntab（程序计数器行表）区域内替换函数名：
  - **只在 pclntab 区域内替换**：避免破坏 embed 嵌入的文件内容
  - **严格的上下文检查**：确保只替换真正的函数名前缀（如 `main.Main`）
  - **保护文本内容**：YAML、JSON、配置文件中的包名不会被替换
  - **前后字符验证**：检查前后字符，避免误替换（如 `commons.io.` 或 `/http.`）
  - **embed 包不混淆**：`embed` 包本身不在混淆列表中

**技术细节**：
- pclntab 是 Go 二进制文件中存储函数名和行号信息的特殊区域
- 我们只在这个区域（通常在文件偏移量后的 10MB 范围内）进行替换
- embed 文件内容存储在其他区域，不会被触及

## 许可证

MIT License


[url-docen]: README_EN.md