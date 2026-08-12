package obfuscator

import (
	"go/ast"
	"strings"
)

// rewrittenLinkname 记录一条 //go:linkname 指令重写后的形式。
type rewrittenLinkname struct {
	comment *ast.Comment // 对应注释节点（输出阶段改写其 Text）

	hasTarget bool   // 是否为双参形式
	localName string // 本地符号原名称
	target    string // 目标符号（原始）
}

// collectLinknameDirectives 收集文件中的 //go:linkname 指令。
func collectLinknameDirectives(file *ast.File) []*rewrittenLinkname {
	var list []*rewrittenLinkname
	for _, cg := range file.Comments {
		for _, c := range cg.List {
			text := c.Text
			if !strings.HasPrefix(text, "//go:linkname ") {
				continue
			}
			fields := strings.Fields(text)
			if len(fields) < 2 {
				continue
			}
			r := &rewrittenLinkname{
				comment:   c,
				localName: fields[1],
			}
			if len(fields) >= 3 {
				r.hasTarget = true
				r.target = fields[2]
			}
			list = append(list, r)
		}
	}
	return list
}

// rewriteLinkname 单条指令重写：本地名始终跟随混淆映射；
// 目标名若指向本项目包内的符号（moduleName 前缀）且已在映射中，则同步重写。
// 目标为 标准库/第三方/无点号符号 时保持原样，避免破坏链接。
func (o *Obfuscator) rewriteLinkname(r *rewrittenLinkname) {
	// 本地名：函数/变量映射优先
	newLocal := o.lookupMappedName(r.localName)

	// 不求重写时直接返回
	if !r.hasTarget {
		if newLocal == "" {
			return
		}
		o.applyLinknameRewrite(r, newLocal, "")
		return
	}

	newTarget := o.mapLinknameTarget(r.target)
	o.applyLinknameRewrite(r, newLocal, newTarget)
}

// lookupMappedName 在函数/变量映射中查找混淆名。
func (o *Obfuscator) lookupMappedName(name string) string {
	if obf, ok := o.funcMapping[name]; ok {
		return obf
	}
	if obf, ok := o.varMapping[name]; ok {
		return obf
	}
	return ""
}

// mapLinknameTarget 计算链接目标是否需要重写。
// 形如 `pkg.Name` 且 pkg 属于本项目时，若 Name 已被混淆则返回新目标；
// 否则原样返回。
func (o *Obfuscator) mapLinknameTarget(target string) string {
	dot := strings.LastIndex(target, ".")
	if dot <= 0 {
		// 无点号（内含目标函数未定义声明）保持原样
		return target
	}
	pkgPath := target[:dot]
	name := target[dot+1:]
	if name == "" {
		return target
	}

	// 目标必须属于本项目（moduleName 前缀）
	module := o.moduleName
	if module == "" || pkgPath != module && !strings.HasPrefix(pkgPath, module+"/") {
		return target
	}

	if obf, ok := o.funcMapping[name]; ok {
		return pkgPath + "." + obf
	}
	// 变量映射：仅当变量装载在同一项目中
	if obf, ok := o.varMapping[name]; ok {
		return pkgPath + "." + obf
	}
	return target
}

// applyLinknameRewrite 把重写结果写回注释文本。
func (o *Obfuscator) applyLinknameRewrite(r *rewrittenLinkname, newLocal, newTarget string) {
	fields := strings.Fields(r.comment.Text)
	if len(fields) < 2 {
		return
	}
	if newLocal != "" {
		fields[1] = newLocal
	}
	if r.hasTarget && len(fields) >= 3 && newTarget != "" {
		fields[2] = newTarget
	}
	r.comment.Text = strings.Join(fields, " ")
}

// rewriteLinknameDirectivesInFile 对文件内所有 //go:linkname 指令应用重写。
// 应当在标识符混淆完成后调用（此时 funcMapping/varMapping 已就绪）。
func (o *Obfuscator) rewriteLinknameDirectivesInFile(node *ast.File) {
	for _, r := range collectLinknameDirectives(node) {
		o.rewriteLinkname(r)
	}
}
