package nativeui

import "testing"

func TestSnippetParts(t *testing.T) {
	before, hit, after := snippetParts("Muse 是原生 Markdown 编辑器", "markdown")
	if before != "Muse 是原生 " || hit != "Markdown" || after != " 编辑器" {
		t.Fatalf("大小写不敏感切分错误: %q %q %q", before, hit, after)
	}
	if b, h, a := snippetParts("没有命中的行", "xyz"); h != "" || b != "没有命中的行" || a != "" {
		t.Fatalf("未命中应原样返回: %q %q %q", b, h, a)
	}
	if b, h, a := snippetParts("aa bb aa", "aa"); b != "" || h != "aa" || a != " bb aa" {
		t.Fatalf("只切首个命中: %q %q %q", b, h, a)
	}
	if b, h, a := snippetParts("任意内容", "  "); h != "" || b != "任意内容" || a != "" {
		t.Fatalf("空查询不高亮: %q %q %q", b, h, a)
	}
}
