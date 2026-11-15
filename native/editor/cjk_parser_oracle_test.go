package editor

import (
	"fmt"
	"math/rand/v2"
	"strings"
	"testing"

	"muse/editor/internal/parser"
	"muse/markdown"
)

func TestInternalParserCJKFlankingMatchesSourceNormalization(t *testing.T) {
	inputs := []string{
		"**注意：**这是**重点**内容。",
		"中文**（括号）**中文",
		"**English:**中文",
		"“我说**是**”",
		"**“名称”**：**值**",
		"> **引用：**内容\n> ~~删除：~~项\n- **列表（步骤一）：**安装",
		"中文__下划线__以及~~删除：~~，再来***三重强调***。",
		"snake_case_word 和 中文_斜体_中文",
		"emoji 😀**中文**与 𝄀**符号**。",
		"\u201c**全角：**\u201d\ufe01\n\n>   **引用标题：**\r\n",
		"\\**转义：**和 \\\\**未转义：**内容\r\n",
	}
	inputs = append(inputs, strings.Repeat("**注意：**与 _中文_、~~删除：~~\nplain **English**\r\n> **引用：**\n", 40))
	edges := []string{"中", "。", "“", "a", ":", "!", " ", "\t", "😀", "_"}
	delimiters := []string{"*", "**", "***", "_", "__", "___", "~~"}
	for _, before := range edges {
		for _, delim := range delimiters {
			for _, after := range edges {
				inputs = append(inputs, before+delim+"中文"+delim+after)
			}
		}
	}
	delimiterPairs := []string{"**", "*", "_", "__", "~~", "***", "\\*", "\\_", "`"}
	parts := []string{"中", "漢", "。", "，", "（", "）", "“", "”", "a", ":", "!", "?", " ", "\t", "😀"}
	rng := rand.New(rand.NewPCG(7, 11))
	for n := 0; n < 500; n++ {
		var b strings.Builder
		for i := 0; i < 4; i++ {
			b.WriteString(delimiterPairs[rng.IntN(len(delimiterPairs))])
			b.WriteString(parts[rng.IntN(len(parts))])
		}
		inputs = append(inputs, b.String())
	}

	shape := func(src string) []string {
		d := parser.New(exts)
		d.Set(src)
		out := make([]string, 0, d.Len())
		for i := 1; i < d.Len(); i++ {
			b := d.Block(i)
			out = append(out, fmt.Sprintf("b:%d:%d:%d", b.Kind, b.Start, b.End))
			for j, in := range d.Inlines(i) {
				out = append(out, fmt.Sprintf("i:%d:%d:%d:%d:%t", j, in.Kind, in.Start, in.End, in.Display))
			}
		}
		return out
	}

	for _, src := range inputs {
		direct, normalized := shape(src), shape(markdown.CJKFriendly(src))
		if len(direct) != len(normalized) {
			t.Errorf("shape length differs for %q: direct=%v normalized=%v", src, direct, normalized)
			continue
		}
		for i := range direct {
			if direct[i] != normalized[i] {
				t.Errorf("shape differs for %q at %d: direct=%v normalized=%v", src, i, direct, normalized)
				break
			}
		}
	}
}
