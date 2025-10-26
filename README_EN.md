# Go Code Obfuscator (Cross-File Obfuscator)

[中文][url-doczh]

A Go code obfuscation tool using AST (Abstract Syntax Tree) technology to implement cross-file code obfuscation while ensuring the obfuscated code is **compilable and executable**.

## Key Highlights

- **One-Click Auto Mode** - Full-featured obfuscation + automatic compilation in one command
- **Third-Party Package Obfuscation** - Optional obfuscation of third-party dependencies for enhanced code hiding
- **Linker-Level Obfuscation** - Directly modify binary files to obfuscate function names and package paths
- **Auto Package Discovery** - Intelligently discover and obfuscate project packages and standard library packages
- **Smart Region Protection** - Intelligent protection of embed files without breaking embedded resources
- **Multi-Layer Obfuscation** - Variable names + function names + string encryption + control flow obfuscation + binary obfuscation
- **Smart Protection Mechanism** - Automatically identify and protect exported symbols, struct fields, methods, interfaces
- **Automatic Reflection Protection** - Detect and protect types and methods used in reflection
- **JSON/XML Smart Handling** - Automatically protect fields and types related to serialization
- **Complete Interface Protection** - Automatically protect interface definitions and implementations
- **Embedded Field Support** - Correctly handle anonymous embedded fields (structs and pointers)
- **Built-in Identifier Protection** - Protect all Go built-in types, functions, and constants
- **Smart CGO Skip** - Automatically detect and skip CGO code files
- **Generated Code Recognition** - Automatically skip generated code like protobuf
- **Flexible Exclusion Rules** - Support custom file exclusion patterns
- **High Performance** - Optimized AST traversal for fast processing
- **Strong Randomness** - All obfuscated names are completely random with no patterns
- **Detailed Reports** - Display skipped files, reflection usage, and other detailed information at runtime

## Features

### Core Features

1. **Private Function Name Obfuscation**
   - Only obfuscate unexported functions (lowercase)
   - Maintain consistency across file references
   - Generate random unpredictable function names

2. **Private Variable Name Obfuscation**
   - Obfuscate local and global unexported variables
   - Intelligently distinguish between functions and variables with the same name
   - Use random string naming

3. **Standard Library Package Aliases**
   - **Automatically identify** standard library packages (no manual list maintenance)
   - Only obfuscate standard library, keep third-party packages unchanged
   - Create consistent random aliases for all standard library packages
   - Automatically replace package references in code
   - Prevent inferring code functionality through package names

4. **Smart Protection Mechanism**
   
   **Basic Protection**:
   - **Exported Symbol Protection** - Don't obfuscate exported functions, variables, types (uppercase) by default
   - **Struct Field Protection** - Protect all struct field names (including anonymous embedded fields)
   - **Method Protection** - Protect all method names with receivers
   - **Interface Protection** - Automatically protect all method signatures in interface definitions
   
   **Smart Selector Protection**:
   - Protect selectors from external packages and standard library (like `fmt.Println`, `os.Open`)
   - Protect object method calls (like `obj.Method()`, `conn.Close()`)
   - Allow obfuscation of internal package cross-file references (maintain consistency)
   
   **Package Name Protection**:
   - Protect all imported package names (including standard library and third-party packages)
   - Standard library uses random aliases (like `import p8xK2mQr "fmt"`)
   - Third-party packages remain unchanged (ensure compatibility)
   
   **Built-in Identifier Protection** (33+ items):
   - Special identifiers: `main`, `init`, `_`
   - Built-in types: `int`, `string`, `bool`, `byte`, `rune`, `error`, `float32`, `float64`, `complex64`, `complex128`, `uint`, `int8`, `int16`, `int32`, `int64`, `uint8`, `uint16`, `uint32`, `uint64`, `uintptr`
   - Built-in functions: `len`, `cap`, `make`, `new`, `append`, `copy`, `delete`, `panic`, `recover`, `print`, `println`, `close`, `min`, `max`, `clear`
   - Built-in constants: `nil`, `true`, `false`, `iota`

### Advanced Obfuscation Features

5. **String Encryption**
   - XOR encrypt all string literals
   - Randomly generate decryption function names
   - Automatic runtime decryption
   - 64-byte random key

6. **Constant Expression Conversion**
   - Convert numeric constants to mathematical expressions
   - Example: `999999` → `(1000*1000-1)`
   - Increase static analysis difficulty

7. **Comment Removal**
   - Automatically remove all code comments
   - Including doc comments and inline comments
   - Hide developer intentions and explanations

8. **Opaque Predicates**
   - Inject math-based opaque predicates
   - Use always-true/always-false conditions to obfuscate control flow
   - Including: x²≥0, (x²+x)%2==0, 2x>x, etc.
   - All variable names randomly generated

## Usage

### Build

```bash
go build -o cross-file-obfuscator cmd/obfuscator/main.go
```

#### Notes

1. **CGO Code**:
   - If project contains CGO code, cross-compilation needs corresponding platform C compiler
   - Recommended to disable CGO for cross-compilation: `CGO_ENABLED=0`
   - Or use `-exclude` parameter to exclude CGO files

2. **Platform-Specific Code**:
   - Build tag files (`_linux.go`, `_windows.go`) are automatically handled
   - Go compiler automatically selects files based on GOOS/GOARCH

3. **File Extensions**:
   - Windows platform recommends `.exe` extension
   - Linux/macOS platforms typically don't need extensions

4. **Strip Tool**:
   - Linker obfuscation automatically calls `strip` to remove symbol tables
   - For cross-compilation, strip may not support target platform format
   - Tool will attempt to use it; warnings on failure won't affect obfuscation

### Basic Usage

```bash
# Basic obfuscation (variable names, function names, package aliases)
./cross-file-obfuscator <project-directory>

# Use all advanced features (recommended)
./cross-file-obfuscator -obfuscate-exported -obfuscate-filenames -encrypt-strings -inject-junk -remove-comments <project-directory>
```

### Command Line Options

#### Basic Options

```bash
-o <directory>              Specify output directory (default: project_name_obfuscated)
-obfuscate-exported         Obfuscate exported functions and variables (may break API)
-obfuscate-filenames        Obfuscate Go file names
-encrypt-strings            Encrypt string literals
-inject-junk                Inject junk code (opaque predicates)
-remove-comments            Remove all comments (default: true)
-preserve-reflection        Protect reflection-related types and methods (default: true)
-skip-generated             Skip auto-generated code files (default: true)
-exclude <patterns>         Exclude file patterns, comma-separated (e.g.: '*_test.go,*.pb.go,tools/*')
```

#### Advanced Options (Linker Obfuscation)

```bash
-auto                       Auto mode: full obfuscation + automatic compilation (recommended)
-build-with-linker          Directly compile and apply linker obfuscation
-output-bin <filename>      Output binary filename (use with -build-with-linker or -auto)
-entry <package-path>       Entry package path (must specify for multi-entry projects)
                            - Default: "." (root directory)
                            - Example: "./cmd/server", "./cmd/gost"
                            - Used to specify main package location
-pkg-replace <mapping>      Package name replacement mapping (format: 'original1=new1,original2=new2')
-auto-discover-pkgs         Auto-discover and replace all package names in project (recommended)
-obfuscate-third-party      Obfuscate third-party dependency packages (use cautiously, may affect stability)
```

**`-entry` Parameter Explanation**:

- **When Needed**: When your main package is not in project root directory (like `cmd/app/main.go`)
- **How to Determine**: Run `find ./project -name "*.go" -exec grep -l "func main()" {} \;`
- **Common Scenarios**:
  - `main.go` in root → No need to specify
  - `cmd/server/main.go` → Need `-entry "./cmd/server"`
  - `cmd/gost/main.go` → Need `-entry "./cmd/gost"`

### Usage Examples

#### 🚀 Recommended: Auto Mode

```bash
# Standard project (main in root directory)
./cross-file-obfuscator -auto -output-bin myapp ./my-project

# Multi-entry project (main in subdirectory) - Must specify -entry
./cross-file-obfuscator -auto -entry "./cmd/server" -output-bin server ./my-project

# Auto mode + third-party package obfuscation (use cautiously)
./cross-file-obfuscator -auto -obfuscate-third-party -output-bin myapp ./my-project
```

#### Linker Obfuscation Examples

```bash
# Standard project - auto-discover package names (recommended)
./cross-file-obfuscator -build-with-linker -auto-discover-pkgs -output-bin myapp ./my-project

# Multi-entry project - need to specify entry
./cross-file-obfuscator -build-with-linker -auto-discover-pkgs -entry "./cmd/server" -output-bin server ./my-project

# Linker obfuscation + third-party package obfuscation
./cross-file-obfuscator -build-with-linker -auto-discover-pkgs -obfuscate-third-party -output-bin myapp ./my-project

# Manually specify package name replacement
./cross-file-obfuscator -build-with-linker -pkg-replace 'main=m,github.com/user/project=a' -output-bin myapp ./my-project
```

#### Combined Use (Maximum Obfuscation)

```bash
# Step 1: Source code obfuscation
./cross-file-obfuscator -encrypt-strings -inject-junk -obfuscate-exported ./my-project

# Step 2: Linker obfuscation (on obfuscated source)
./cross-file-obfuscator -build-with-linker -auto-discover-pkgs -output-bin myapp ./my-project_obfuscated

# Or use auto mode directly (all-in-one)
./cross-file-obfuscator -auto -output-bin myapp ./my-project
```

### 🚀 Auto Mode - One-Click Full Obfuscation

**The simplest way to use!** Auto mode automatically executes all obfuscation steps:

**Features Included in Auto Mode**:
- String encryption (`-encrypt-strings`)
- Junk code injection (`-inject-junk`)
- Exported symbol obfuscation (`-obfuscate-exported`)
- Filename obfuscation (`-obfuscate-filenames`)
- Auto package discovery (`-auto-discover-pkgs`)
- Linker obfuscation (`-build-with-linker`)
- Third-party package obfuscation (`-obfuscate-third-party`)

**Output**:
- `./my-project_obfuscated` - Obfuscated source code
- `myapp` - Obfuscated executable

---

### Important: Multi-Entry Project Configuration

**How to determine if you need to specify `-entry` parameter?**

Check your project structure:

```bash
# Method 1: Find main function location
find ./your-project -name "*.go" -exec grep -l "func main()" {} \;

# Method 2: Check project structure
ls -la ./your-project/*.go  # If root has main.go, no need to specify entry
ls -la ./your-project/cmd/  # If main in cmd subdirectory, need to specify entry
```

**Usage Rules**:

| Project Structure | Need -entry? | Command Example |
|-------------------|--------------|-----------------|
| `main.go` in root | No | `./cross-file-obfuscator -auto -output-bin app ./project` |
| `cmd/app/main.go` | **Yes** | `./cross-file-obfuscator -auto -entry "./cmd/app" -output-bin app ./project` |
| `cmd/server/main.go` | **Yes** | `./cross-file-obfuscator -auto -entry "./cmd/server" -output-bin server ./project` |

**Common Errors**:

If you see the following error, it means you need to specify `-entry` parameter:

```
Detected binary format: Unknown
Post-processing failed: unsupported binary format: Unknown
```

Or generated file is `ar archive` instead of executable:

```bash
$ file output
output: current ar archive  # Error: this is archive, not executable
```

**Solution**:

1. Find main package location:
```bash
find ./project -name "*.go" -exec grep -l "func main()" {} \;
# Output: ./project/cmd/gost/main.go
```

2. Add `-entry` parameter:
```bash
./cross-file-obfuscator -auto -entry "./cmd/gost" -output-bin output ./project
```

---

### Third-Party Package Obfuscation

By default, the obfuscator only obfuscates project package names and standard library package names, **not third-party dependencies** (like `github.com/xxx`).

If you need deeper obfuscation, enable third-party package obfuscation:

```bash
# Auto mode
./cross-file-obfuscator -auto -output-bin myapp ./my-project

# Linker obfuscation + third-party package obfuscation
./cross-file-obfuscator -build-with-linker -auto-discover-pkgs -obfuscate-third-party -output-bin myapp ./my-project
```

**Effects**:
- Project package name: `github.com/xxxx/xxxx` → `a`
- Standard library: `runtime.` → `b.`
- Third-party package: `github.com/xxx/xxx/runtime/Go` → `c`

**Notes**:
- Third-party package obfuscation may affect program stability
- Recommend testing without this option first
- Disable this option if program crashes

---

#### Features

- **Cross-Platform Support** - Supports ELF (Linux), PE (Windows), Mach-O (macOS)
- **Function Name Obfuscation** - Replace all package name prefixes (main., runtime., fmt., etc.)
- **Auto Package Discovery** - Automatically scan project and replace all internal package names (38+ standard library packages)
- **Smart Path Replacement** - Only replace project package paths, protect system symbols
- **Symbol Table Removal** - Automatically use strip to remove debug symbols
- **Runnable Programs** - Obfuscated programs can run normally

#### Auto Package Discovery Feature

When using `-auto-discover-pkgs` parameter, the obfuscator will:

1. **Auto Scan Project**
   - Read go.mod to get module name
   - Recursively scan project directory
   - Discover all internal packages

2. **Add Standard Library Packages** (38+ packages)
   - Core: main, runtime, sync, syscall
   - I/O: fmt, io, bufio, os, log
   - Network: net, http
   - Strings: strings, bytes, strconv, unicode, regexp
   - Encoding: encoding, json, xml, base64, hex
   - Others: time, math, sort, errors, context, compress, hash, etc.

3. **Smart Replacement Strategy**
   - Replace function name prefixes (`runtime.GC` → `i.GC`)
   - Replace project package paths (`github.com/user/project/pkg` → `a`)
   - Don't replace standard library paths (avoid breaking system symbols)
   - Don't replace third-party packages (not your code)

**Example Output**:
```
Discovered module name: github.com/xxxxx/xxxxx
Discovered 7 project packages
Added 38 standard library packages
Generated 45 package name replacement mappings
Replaced 17,090 function name prefixes
Replaced 51 project package path references
```

## Technical Details

### Workflow

The obfuscator uses a five-phase workflow to ensure safe and reliable code obfuscation:

#### Phase 1: Collect Protected Names

Traverse all Go files to identify identifiers that need protection:

1. **Struct Field Protection**
   - Collect all named fields
   - Identify anonymous embedded fields (structs and pointers)
   - Mark as protected names

2. **Interface Method Protection**
   - Scan interface definitions
   - Collect all method signatures
   - Ensure interface implementations aren't broken

3. **Method Protection**
   - Identify functions with receivers
   - Protect all method names
   - Maintain type method sets

4. **Selector Expression Protection**
   - Collect selectors like `obj.Field`, `pkg.Function`
   - Distinguish internal and external packages
   - Protect external package and object selectors

5. **Reflection and Serialization Detection**
   - Detect usage of `reflect` package
   - Detect `encoding/json`, `encoding/xml`, etc.
   - Automatically protect related types and fields

#### Phase 2: Scope Analysis

Build complete scope tree using AST analysis:

1. **Package-Level Scope**
   - Identify all package-level declarations
   - Build object mapping (functions, variables, types)
   - Record object file locations

2. **Function Scope**
   - Analyze function parameters and return values
   - Identify local variables
   - Handle closures and nested functions

3. **Block-Level Scope**
   - Handle if/for/switch blocks
   - Correctly handle variable shadowing
   - Maintain scope hierarchy

4. **Cross-File Reference Analysis**
   - Identify references across files in same package
   - Handle build tag files (like `_linux.go`, `_windows.go`)
   - Ensure same-name objects use same obfuscated name

#### Phase 3: Build Obfuscation Mapping

Generate unique obfuscated names:

1. **Private Function Obfuscation**
   - Only obfuscate unexported functions (lowercase)
   - Generate random name for each unique function
   - Handle same-name functions (different build tag files)

2. **Private Variable Obfuscation**
   - Obfuscate package-level and local variables
   - Avoid conflicts with function names
   - Maintain cross-file reference consistency

3. **Standard Library Package Aliases**
   - Auto-identify standard library (import paths without domain)
   - Generate random aliases (like `p8xK2mQr`)
   - Keep third-party packages unchanged

4. **Filename Obfuscation** (optional)
   - Generate random filenames
   - Preserve platform suffixes (`_linux`, `_windows`, `_amd64`, etc.)
   - Ensure build tags work normally

#### Phase 4: Copy Project Files

1. **Directory Structure Copy**
   - Complete copy of project directory tree
   - Maintain relative path relationships
   - Skip vendor and hidden directories

2. **File Filtering**
   - Skip CGO files (containing `import "C"`)
   - Skip generated code (containing `// Code generated`)
   - Apply user exclusion rules (`-exclude` parameter)

3. **Non-Go File Handling**
   - Copy go.mod, go.sum
   - Copy config files, resource files
   - Maintain file permissions

#### Phase 5: Apply Obfuscation

1. **Import Statement Processing**
   - Add random aliases for standard library
   - Keep third-party package imports unchanged
   - Update package references

2. **Identifier Replacement**
   - Use scope analysis for precise replacement
   - Prioritize object mapping (avoid local variable conflicts)
   - Fall back to name mapping (cross-file references)
   - Skip all protected names

3. **Advanced Obfuscation Application** (optional)
   - String encryption: XOR + Base64
   - Junk code injection: opaque predicates
   - Comment removal: clean all comments

4. **Code Formatting**
   - Format using `go/format`
   - Ensure correct syntax
   - Maintain code readability (for debugging)

### Core Algorithms

#### 1. Scope-Aware Replacement

```
Decision tree for replacing identifiers:
1. Is it a protected name? → Skip
2. Is it exported and export obfuscation not enabled? → Skip
3. Look up object in current file's package-level scope
   - Found with obfuscated name? → Use object's obfuscated name
4. Look up in cross-file mapping (funcMapping/varMapping)
   - Found? → Use mapped obfuscated name
5. Otherwise → Keep original
```

#### 2. Build Tag Handling

For files with build tags (like `file_linux.go`, `file_windows.go`):

```
Same-name object strategy:
- Identify: Detect same-name functions/variables in different files of same package
- Map: All same-name objects use same obfuscated name
- Reason: Go compiler only compiles matching platform files, no conflicts
- Example:
  - gost/sockopts_linux.go:  func setSocketMark() { ... }
  - gost/sockopts_other.go:  func setSocketMark() { ... }
  - Both map to: fnXXXXXXXXXX
```

#### 3. Filename Obfuscation Strategy

```
Preserve platform suffixes:
- Detect: Identify _linux, _windows, _darwin, _amd64, _arm64, etc. suffixes
- Preserve: Obfuscate filename but keep suffix
- Example:
  - Original: conn_linux_amd64.go
  - Obfuscated: fXXXXXXXX_linux_amd64.go
- Reason: Go compiler relies on these suffixes for conditional compilation
```

#### 4. String Encryption Algorithm

```
Encryption flow:
1. Generate 64-byte random key
2. For each string:
   - XOR encryption: plaintext[i] ^ key[i % len(key)]
   - Base64 encoding: avoid binary data
   - Replace: original string → decrypt("base64data")
3. Inject decryption function (once per package)
4. Runtime decryption: automatic decryption at program startup
```

### Protection Mechanism Layers

The obfuscator uses five layers of protection mechanisms to ensure code safety:

| Layer | Protected Content | Implementation Location | Priority |
|-------|------------------|------------------------|----------|
| Layer 1 | Special identifiers (`main`, `init`, `_`) | `shouldProtect()` | Highest |
| Layer 2 | Go built-in identifiers (33+) | `shouldProtect()` | Highest |
| Layer 3 | Exported names (uppercase) | `shouldProtect()` | High |
| Layer 4 | Structured protection (fields, methods, interfaces) | `collectProtectedNames()` | High |
| Layer 5 | Context protection (reflection, JSON, CGO) | `protectReflectionTypes()` | Medium |

**Protection Priority Explanation**:
- Protection at any layer prevents obfuscation
- Multi-layer protection provides redundant safety
- Even if one layer fails, other layers still protect

## Design Principles

### 1. Reliability First - 100% Compilation Success

**Core Promise**: Obfuscated code must be able to compile and run

**Implementation**:
- **Five-layer protection mechanism**: Multiple protections ensure key identifiers aren't mistakenly obfuscated
- **Scope analysis**: Precisely identify identifier scopes to avoid wrong replacements
- **Build tag support**: Correctly handle platform-specific code (`_linux.go`, `_windows.go`)
- **Filename suffix preservation**: Maintain Go compiler's conditional compilation feature
- **Conservative strategy**: When uncertain, choose not to obfuscate rather than risk it

---

### 2. Cross-File Consistency - Same Name Same Obfuscation

**Core Principle**: Same identifier uses same obfuscated name across all files

**Why Important**:
- Go allows multiple files in same package to share package-level declarations
- Function defined in one file can be called in another file
- Build tag files may define same-name functions (but only one will be compiled)

**Implementation**:
```
Package-level object mapping:
- Scan all files, collect package-level declarations
- Generate one obfuscated name for each unique name
- All files share same mapping table

Special handling: Build tag files
- file_linux.go: func connect() { ... }
- file_windows.go: func connect() { ... }
- Both map to same obfuscated name: fnXXXXXXXXXX
- Reason: Go compiler only selects one file to compile
```

**Example**:
```go
// file1.go
func helper() { ... }  // → fnAbc123xyz

// file2.go  
func process() {
    helper()  // → fnAbc123xyz (same obfuscated name)
}
```

---

### 3. Smart Protection - Better Less Obfuscation Than Breakage

**Core Principle**: When uncertain, choose protection over obfuscation

**Protection Priority**:
1. **Must protect**: Go built-in identifiers, `main`, `init`
2. **Default protection**: Exported names, struct fields, methods
3. **Context protection**: Reflection, JSON, CGO related
4. **Smart protection**: Selector expressions, interface methods

---

### 4. Progressive Obfuscation - From Conservative to Aggressive

**Core Concept**: Provide multiple obfuscation levels for users to test progressively

**Obfuscation Levels**:

| Level | Obfuscation Content | Risk | Recommended Scenario |
|-------|-------------------|------|---------------------|
| **Basic** | Private functions, variables, package aliases | Low | All projects |
| **Intermediate** | + String encryption, junk code | Low | Most projects |
| **Advanced** | + Exported symbol obfuscation | Medium | Projects without external dependencies |
| **Complete** | + Filename obfuscation, third-party packages | High | Monolithic applications |

**Recommended Flow**:
```bash
# Step 1: Basic obfuscation test
./cross-file-obfuscator ./project
cd project_obfuscated && go build

# Step 2: Intermediate obfuscation test
./cross-file-obfuscator -encrypt-strings -inject-junk ./project
cd project_obfuscated && go build

# Step 3: Advanced obfuscation test (cautiously)
./cross-file-obfuscator -obfuscate-exported -encrypt-strings -inject-junk ./project
cd project_obfuscated && go build

# Step 4: Complete obfuscation (auto mode)
./cross-file-obfuscator -auto -output-bin app ./project
```

---

### 5. Performance and Security Balance

**Core Goal**: Maximize obfuscation effect while ensuring performance

**Performance Considerations**:
- **Compilation time**: Obfuscation adds 5-10% compilation time
- **Runtime performance**:
  - Basic obfuscation: 0% performance impact (just renaming)
  - String encryption: < 1% performance impact (decryption at startup)
  - Junk code: < 1% performance impact (compiler optimization)
- **Binary size**: Increases 6-9% (junk code and string encryption)

**Security Effect**:
- **Function name leakage**: Greatly reduced (linker obfuscation)
- **Package path leakage**: Reduced by 99% (linker obfuscation)
- **String leakage**: Reduced by 100% (string encryption)
- **Reverse engineering difficulty**: Significantly increased (combined effect)

**Trade-off Example**:
```
Opaque predicate injection:
- Code increase: +5-10%
- Binary size increase: +3-5%
- Runtime overhead: 0% (compiler optimizes out)
- Reverse engineering difficulty: +20% (obfuscate control flow)
→ Conclusion: Worth using
```

---

### Special Scenario Handling

#### Build Tag Files

**Scenario**: Multiple platform-specific files in same package

```go
// file_linux.go
//go:build linux
func connect() { ... }

// file_windows.go  
//go:build windows
func connect() { ... }
```

**Handling**:
- Both `connect` functions use same obfuscated name
- Filenames preserve platform suffix: `fXXX_linux.go`, `fYYY_windows.go`
- Go compiler only selects one file, no conflicts

---

#### Reflection Usage

**Scenario**: Code uses `reflect` package

```go
import "reflect"

type User struct {
    Name string  // Automatically protected
}

func process() {
    t := reflect.TypeOf(User{})
    // Reflection needs original names
}
```

**Handling**:
- Auto-detect usage of `reflect` package
- Protect all types, fields, method names
- Use `-preserve-reflection=true` (default)

---

#### CGO Code

**Scenario**: Code contains C interop

```go
// #include <stdio.h>
import "C"

func callC() {
    C.printf(...)
}
```

**Handling**:
- Auto-detect `import "C"`
- Skip obfuscation of entire file
- Avoid breaking C interoperability

---

#### Generated Code

**Scenario**: Code generated by protobuf, gRPC, etc.

```go
// Code generated by protoc-gen-go. DO NOT EDIT.
package pb

type Message struct { ... }
```

**Handling**:
- Auto-detect generated code markers
- Skip obfuscation (use `-skip-generated=true`, default)
- Or use `-exclude "*.pb.go"` to manually exclude

### Solved Issues

The following issues have been resolved in the latest version:

1. **Code Using Reflection**
   - Auto-detect files using `reflect` package
   - Automatically protect all type names, field names, and method names
   - Control with `-preserve-reflection` flag (enabled by default)
   - Display list of packages using reflection at runtime

2. **CGO Code**
   - Auto-detect files containing `import "C"`
   - Automatically skip obfuscation to avoid breaking C interoperability
   - Display skipped files and reasons at runtime

3. **Generated Code (like protobuf)**
   - Auto-detect files containing markers like `Code generated`
   - Control with `-skip-generated` flag (enabled by default)
   - Support `-exclude` parameter to manually exclude specific patterns (like `*.pb.go`)
   - Display list of skipped files at runtime

4. **JSON/XML Serialization**
   - Auto-detect files using `encoding/json`, `encoding/xml`, `yaml`
   - Smart handling of struct tags:
     - Fields with explicit tags: use tag names, field names can be obfuscated
     - Fields without tags: protect field names to ensure correct serialization
   - Automatically protect types and fields used in serialization

5. **Interface Implementation**
   - Auto-detect and protect interface method names
   - Ensure interface implementations aren't broken
   - Protect all method signatures in interface definitions

6. **Embedded Fields**
   - Auto-detect and protect anonymous embedded fields
   - Support struct embedding: `type A struct { B }`
   - Support pointer embedding: `type A struct { *B }`

7. **Built-in Types and Functions**
   - Protect all Go built-in types (`int`, `string`, `error`, etc.)
   - Protect all built-in functions (`len`, `make`, `append`, etc.)
   - Protect built-in constants (`nil`, `true`, `false`, `iota`)
   - Avoid conflicts with Go keywords

### Package Obfuscation Strategy

This tool uses **smart identification** strategy:

- **Standard library packages** (like `fmt`, `net/http`, `encoding/json`): Auto-identify and obfuscate
  - Identification rule: import path doesn't contain domain (like `github.com`, `gopkg.in`)
  - Obfuscation method: Generate random alias, like `pkgB2nM5wQr "fmt"`

- **Third-party packages** (like `github.com/go-ldap/ldap/v3`): Keep unchanged
  - Reason: Avoid complexity of version suffixes (v2, v3, etc.) and actual package name mismatches
  - Benefit: Ensure 100% compilation success, no need to maintain package name mapping table

## Smart Protection Features

### Reflection Protection

Tool automatically detects code using `reflect` package and protects related types and methods:

- **Auto-detection**: Scan import statements in all files
- **Comprehensive protection**:
  - Type names (`type MyStruct struct`)
  - All struct fields
  - All methods (including receiver methods)
- **Smart hints**: Display list of packages using reflection at runtime

### CGO Code Skip

Automatically detect and skip files containing C code:

- **Detection method**: Look for `import "C"` statement
- **Skip reason**: Avoid breaking Go-C interoperability
- **Transparent hints**: Display skipped files and reasons

### Generated Code Skip

Automatically identify and skip auto-generated code files:

- **Identification markers**:
  - `// Code generated`
  - `// DO NOT EDIT`
  - `// autogenerated`
  - `// AUTO-GENERATED`
  - `// Code generated by`
- **Common file types**:
  - Protobuf files (`*.pb.go`)
  - gRPC files (`*.pb.gw.go`)
  - Mock files (`*_mock.go`)
  - Output from other code generation tools

### Custom Exclude Patterns

Use `-exclude` parameter to manually exclude files:

```bash
# Exclude test files
./cross-file-obfuscator -exclude "*_test.go" myproject

# Exclude multiple file types
./cross-file-obfuscator -exclude "*.pb.go,*.pb.gw.go,*_mock.go" myproject

# Exclude specific files
./cross-file-obfuscator -exclude "config.go,version.go" myproject
```

## Troubleshooting

### Archive Output Error (Common Issue)

**Symptom**: Compilation output is `ar archive` instead of executable

```bash
$ file output
output: current ar archive  # Error
```

Or see error:
```
Detected binary format: Unknown
Post-processing failed: unsupported binary format: Unknown
```

**Cause**: main package not in project root directory, but `-entry` parameter not specified

**Solution**:

1. **Find main package location**:
```bash
find ./your-project -name "*.go" -exec grep -l "func main()" {} \;
# Output example: ./your-project/cmd/gost/main.go
```

2. **Add `-entry` parameter**:
```bash
# If main in cmd/gost/main.go
./cross-file-obfuscator -auto -entry "./cmd/gost" -output-bin output ./your-project

# If main in cmd/server/main.go
./cross-file-obfuscator -auto -entry "./cmd/server" -output-bin output ./your-project
```

3. **Verify success**:
```bash
$ file output
output: Mach-O 64-bit executable arm64  # Correct
```

**Quick checklist**:

| Project Structure | Need -entry? | Correct Command |
|-------------------|--------------|-----------------|
| `main.go` | No | `./cross-file-obfuscator -auto -output-bin app ./project` |
| `cmd/app/main.go` | Yes | `./cross-file-obfuscator -auto -entry "./cmd/app" -output-bin app ./project` |

---

### Compilation Failure

If obfuscated code can't compile:

1. **Check if entry needs to be specified**: If main package not in root, add `-entry` parameter
2. **Check reflection protection**: Ensure `-preserve-reflection=true` (default)
3. **Check exclusion rules**: Use `-exclude` to exclude problematic files
4. **View skipped files**: Check "Skipped files" list at runtime
5. **Disable advanced features**: Try without `-encrypt-strings` and `-inject-junk`
6. **Check built-in identifiers**: Ensure no conflicts with Go keywords

### Runtime Errors

If obfuscated code compiles but fails at runtime:

1. **Reflection issues**: Check if all packages using reflection are correctly identified
2. **Interface implementation**: Ensure interface method names are protected
3. **Struct tags**: Check if JSON/XML struct tags are correct
4. **Embedded fields**: Verify anonymous embedded fields work normally

### JSON/XML Serialization Issues

If serialization/deserialization fails:

1. **Check tags**: Ensure all fields needing serialization have explicit tags
2. **Verify field names**: Check if obfuscated JSON output matches expectations
3. **Exported fields**: Ensure fields needing serialization are exported (uppercase)

### Common Questions

**Q: Why aren't my private functions obfuscated?**
A: Check if they're methods (with receiver) or in protection list

**Q: What if interface implementation is broken?**
A: Tool automatically protects interface methods, check for other reasons

**Q: How to exclude all files in a directory?**
A: Use `-exclude "dirname/*"` pattern

**Q: CGO code obfuscated causing failure?**
A: Tool automatically skips CGO files, check if contains `import "C"`

### Linker Obfuscation Common Questions

**Q: Using strings to view binary, can still see `runtime` string?**
A: This is normal! What you see are error messages (like `"runtime error:"`), not function names. Check for `runtime.` function name prefixes, should be 0.

**Q: Why can I still see third-party package paths (like `github.com/antlr/...`)?**
A: These are third-party dependency library paths, not your code. Obfuscation aims to protect your code, not hide open-source libraries used.

**Q: How to verify obfuscation success?**
A: Use `strings myapp | grep '^main\.'` to check function name prefixes, should return 0. Don't check all strings containing `main`.

**Q: What packages will auto-discovery replace?**
A: Will replace your project packages and 38+ standard library packages (main, runtime, fmt, sync, etc.), but won't replace third-party dependency packages.

**Q: What if program crashes?**
A: Check if relying on package path strings in reflection. Use backup file (`.backup`) to restore original binary.

**Q: After using `embed` to embed files, obfuscated program reports "control characters are not allowed" or can't find file?**
A: This issue has been fixed! Current version uses **smart region identification** strategy, ensuring function names are only replaced within pclntab (program counter line table) region:
  - **Only replace within pclntab region**: Avoid breaking embed embedded file content
  - **Strict context checking**: Ensure only replacing real function name prefixes (like `main.Main`)
  - **Protect text content**: Package names in YAML, JSON, config files won't be replaced
  - **Before/after character verification**: Check surrounding characters to avoid wrong replacement (like `commons.io.` or `/http.`)
  - **embed package not obfuscated**: `embed` package itself not in obfuscation list

**Technical Details**:
- pclntab is a special region in Go binary files that stores function names and line number information
- We only perform replacement within this region (usually within 10MB after file offset)
- embed file content is stored in other regions and won't be touched

## License

MIT License

[url-doczh]: README.md