package obfuscator

// acMatcher 是 Aho-Corasick 多模式匹配器，用于单次扫描匹配大量字节模式，
// 替代逐 pattern 调用 bytes.Index 的低效循环（每次全量扫描数据）。
type acMatcher struct {
	patterns [][]byte
	root     *acNode
}

type acNode struct {
	next   map[byte]*acNode
	fail   *acNode
	output []int // 该节点处终止的 pattern 索引（已合并 fail 链上的 output）
}

// newACMatcher 构建包含 patterns 的 AC 自动机。
func newACMatcher(patterns [][]byte) *acMatcher {
	m := &acMatcher{
		patterns: patterns,
		root:     &acNode{next: make(map[byte]*acNode)},
	}

	// 1. 构建 trie
	for i, p := range patterns {
		cur := m.root
		for _, b := range p {
			if cur.next[b] == nil {
				cur.next[b] = &acNode{next: make(map[byte]*acNode)}
			}
			cur = cur.next[b]
		}
		cur.output = append(cur.output, i)
	}

	// 2. BFS 构建 fail 指针并合并 output
	m.root.fail = m.root
	queue := make([]*acNode, 0, len(m.root.next))
	for _, child := range m.root.next {
		child.fail = m.root
		queue = append(queue, child)
	}
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		for b, child := range cur.next {
			f := cur.fail
			for f != m.root && f.next[b] == nil {
				f = f.fail
			}
			if n, ok := f.next[b]; ok && n != child {
				child.fail = n
			} else {
				child.fail = m.root
			}
			if len(child.fail.output) > 0 {
				child.output = append(child.output, child.fail.output...)
			}
			queue = append(queue, child)
		}
	}

	return m
}

// match 在 data[start:end] 内扫描，对每个命中的 pattern 调用 cb(pos, idx)。
// pos 为 pattern 在 data 中的起始偏移，idx 为 pattern 索引；返回 false 可提前终止。
// 同一位置可能命中多个 pattern（较短者经 fail 链合并），调用方需自行处理重叠。
func (m *acMatcher) match(data []byte, start, end int, cb func(pos, idx int) bool) {
	cur := m.root
	for i := start; i < end; i++ {
		b := data[i]
		for cur != m.root && cur.next[b] == nil {
			cur = cur.fail
		}
		if next, ok := cur.next[b]; ok {
			cur = next
		}
		for _, idx := range cur.output {
			if !cb(i-len(m.patterns[idx])+1, idx) {
				return
			}
		}
	}
}
