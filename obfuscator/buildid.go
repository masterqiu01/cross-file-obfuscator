package obfuscator

import (
	"bytes"
	"debug/elf"
	"encoding/binary"

	"cross-file-obfuscator/internal/logger"
)

// ELF note 常量，与 Go 链接器 src/cmd/link/internal/ld/elf.go 保持一致：
//
//	const (
//		ELF_NOTE_GOPKGLIST_TAG = 1
//		ELF_NOTE_GOABIHASH_TAG = 2
//		ELF_NOTE_GODEPS_TAG    = 3
//		ELF_NOTE_GOBUILDID_TAG = 4
//	)
//	var ELF_NOTE_GO_NAME = []byte("Go\x00\x00")
const (
	elfNoteGoName          = "Go"
	elfNoteGoBuildIDTag    = 4 // NT_GO_BUILDID，file 命令据此输出 "Go BuildID=<id>"
	elfNoteGNUBuildIDTag   = 3 // NT_GNU_BUILD_ID
	elfNoteGNUBuildIDName  = "GNU"
)

// obfuscateBuildIDs 清除 ELF 二进制中的 build-id notes：
//  1. .note.go.buildid（名字 "Go"、type 4）：原地改写 name/type/desc，使新版 file
//     不再输出 "Go BuildID=..."，readelf -n 不再标注 GO BUILDID，strings 也提取不到
//     可读的 buildid（buildid 是依赖内容的指纹，可被用来反推构建环境）。
//  2. .note.gnu.build-id（名字 "GNU"、type 3）：抹掉 desc，隐藏 BuildID[sha1] 指纹。
//
// 通过 PT_NOTE program header 定位 note（二进制被 strip 掉 section header 后依然
// 有效，而 file 正是在 ELF header 无 section 时从 PT_NOTE 读出 Go BuildID）。
// 所有改动均为等长原地改写，不改变 note 布局，运行时不会读取该 note，程序可正常运行。
func (lo *LinkerObfuscator) obfuscateBuildIDs(data []byte, format string) int {
	if format != "ELF" {
		return 0
	}

	elfFile, err := elf.NewFile(bytes.NewReader(data))
	if err != nil {
		logger.Warnf("obfuscateBuildIDs: 解析 ELF 失败: %v", err)
		return 0
	}
	defer elfFile.Close()

	count := 0
	// 1) PT_NOTE program header：strip 掉 section header 后依然有效，
	//    file 正是从这里读出 "Go BuildID="。Go 链接器只把 .note.go.buildid
	//    放进 PT_NOTE，.note.gnu.build-id 不在其中。
	for _, p := range elfFile.Progs {
		if p.Type != elf.PT_NOTE {
			continue
		}
		off := int(p.Off)
		size := int(p.Filesz)
		if off < 0 || size <= 0 || off+size > len(data) {
			continue
		}
		// 与 file 的实现一致：用段 p_align 对齐 name/desc，缺省为 4。
		// Go 链接器写入的 note 均为 4 字节对齐。
		align := int(p.Align)
		if align < 4 {
			align = 4
		}
		count += lo.scrubELFNotes(data, off, size, align)
	}

	// 2) SHT_NOTE section：未 strip 的二进制中 .note.gnu.build-id 是独立 section，
	//    file 会通过 section 扫描额外输出 "BuildID[sha1]=..."，一并抹掉。
	for _, s := range elfFile.Sections {
		if s.Type != elf.SHT_NOTE {
			continue
		}
		off := int(s.Offset)
		size := int(s.Size)
		if off < 0 || size <= 0 || off+size > len(data) {
			continue
		}
		align := int(s.Addralign)
		if align < 4 {
			align = 4
		}
		count += lo.scrubELFNotes(data, off, size, align)
	}
	return count
}

// scrubELFNotes 解析一个 PT_NOTE 段内的所有 note，清除 Go/GNU build-id 信息。
func (lo *LinkerObfuscator) scrubELFNotes(data []byte, off, size, align int) int {
	count := 0
	end := off + size
	o := off

	// 位运算对齐只对 2 的幂有效；防御非法的对齐值（规范要求 p_align/sh_addralign
	// 为 2 的幂，Go 的 note 固定用 4），否则退回 4 字节对齐。
	if align < 4 || align&(align-1) != 0 {
		align = 4
	}
	alignFn := func(a int) int {
		return (a + align - 1) &^ (align - 1)
	}

	for o+12 <= end {
		namesz := int(binary.LittleEndian.Uint32(data[o : o+4]))
		descsz := int(binary.LittleEndian.Uint32(data[o+4 : o+8]))
		typ := binary.LittleEndian.Uint32(data[o+8 : o+12])

		nameOff := o + 12
		descOff := alignFn(nameOff + namesz)
		next := alignFn(descOff + descsz)
		if descOff > end || next > end || next <= o {
			break
		}

		nameLen := namesz
		if nameLen > 3 {
			nameLen = 3
		}
		// note name 形如 "Go\x00\x00"/"GNU\x00"，用字节比较前缀。
		// file 的 do_bid_note 用 memcmp(nbuf+noff, "Go", 3) 匹配（NUL 结尾字面量），
		// 因此这里同样只需检查前几个字节。
		name := data[nameOff : nameOff+nameLen]

		switch {
		// Go buildid note：破坏 file 的 "Go BuildID=" 匹配（name 不再是 "Go"），
		// 改掉 type 让 readelf 不再标注 GO BUILDID，并抹掉 buildid 明文。
		case nameLen >= 2 && name[0] == 'G' && name[1] == 'o' && typ == elfNoteGoBuildIDTag && descsz > 0:
			for i := nameOff; i < nameOff+namesz; i++ {
				data[i] = byte(0x80 + rng.IntN(0x7e))
			}
			binary.LittleEndian.PutUint32(data[o+8:o+12], 0x10)
			for i := descOff; i < descOff+descsz; i++ {
				data[i] = byte(0x80 + rng.IntN(0x7e))
			}
			logger.Debugf("清除 Go buildid note (文件偏移 0x%x)", o)
			count++

		// GNU build-id note：抹掉 sha1 指纹。同时改写 name/type，
		// 让 file 不再输出 "BuildID[sha1]=..."（其条件为 name=="GNU" &&
		// type==NT_GNU_BUILD_ID && 4<=descsz<=20）。
		case nameLen >= 3 && bytes.Equal(name, []byte("GNU")) && typ == elfNoteGNUBuildIDTag && descsz > 0:
			for i := nameOff; i < nameOff+namesz; i++ {
				data[i] = byte(0x80 + rng.IntN(0x7e))
			}
			binary.LittleEndian.PutUint32(data[o+8:o+12], 0x11)
			for i := descOff; i < descOff+descsz; i++ {
				data[i] = byte(rng.IntN(256))
			}
			logger.Debugf("清除 GNU build-id note (文件偏移 0x%x)", o)
			count++
		}

		o = next
	}
	return count
}
