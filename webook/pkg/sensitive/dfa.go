package sensitive

// DFAFilter 基于 DFA（确定有限自动机）字典树的敏感词过滤器。
// 构建后只读，Match 并发安全。
type DFAFilter struct {
	root *dfaNode
}

type dfaNode struct {
	children map[rune]*dfaNode
	end      bool // 标记一个敏感词在此结束
}

// NewDFAFilter 用敏感词列表构建过滤器。空词自动跳过。
func NewDFAFilter(words []string) Filter {
	root := &dfaNode{children: make(map[rune]*dfaNode)}
	for _, w := range words {
		if w == "" {
			continue
		}
		node := root
		for _, r := range w {
			next, ok := node.children[r]
			if !ok {
				next = &dfaNode{children: make(map[rune]*dfaNode)}
				node.children[r] = next
			}
			node = next
		}
		node.end = true
	}
	return &DFAFilter{root: root}
}

// Match 返回 text 是否命中任意敏感词。
// 对每个起点尝试沿字典树前进，遇到词尾（end）即命中；走不通则换下一个起点。
func (f *DFAFilter) Match(text string) bool {
	runes := []rune(text)
	for i := 0; i < len(runes); i++ {
		node := f.root
		for j := i; j < len(runes); j++ {
			next, ok := node.children[runes[j]]
			if !ok {
				break
			}
			if next.end {
				return true
			}
			node = next
		}
	}
	return false
}
