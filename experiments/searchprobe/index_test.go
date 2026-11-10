package searchprobe

import (
	"context"
	"path/filepath"
	"reflect"
	"testing"
)

func openTest(t *testing.T) (*Index, string) {
	t.Helper()
	p := filepath.Join(t.TempDir(), "idx")
	i, e := Open(p)
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { i.Close() })
	return i, p
}
func TestLiteral(t *testing.T) {
	for _, tc := range []struct {
		body, q string
		limit   int
		want    []int
	}{
		{"foobar foo", "foo", 9, []int{0, 3, 7, 10}}, {"中文中", "中", 9, []int{0, 3, 6, 9}},
		{"x，\ny", "，\n", 9, []int{1, 4}}, {"İxİ", "i", 9, []int{0, 2, 3, 5}},
		{"aaaa", "aa", 9, []int{0, 2, 2, 4}}, {"😀foo", "foo", 9, []int{4, 7}},
		{"foofoo", " foo ", 1, []int{0, 3}}, {"foo", " \n", 9, nil}, {"foo", "foo", 0, nil}, {"foo", "foo", -1, nil},
	} {
		t.Run(tc.body+tc.q, func(t *testing.T) {
			i, _ := openTest(t)
			if e := i.Batch([]Document{{ID: "x", Path: "x", Body: tc.body}}, nil); e != nil {
				t.Fatal(e)
			}
			var got []int
			for _, m := range literal(t, i, tc.q, tc.limit) {
				got = append(got, m.Start, m.End)
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("got %v want %v", got, tc.want)
			}
		})
	}
}
func TestLifecycle(t *testing.T) {
	p := filepath.Join(t.TempDir(), "idx")
	i, e := Open(p)
	if e != nil {
		t.Fatal(e)
	}
	docs := []Document{{ID: "a", Path: "z.md", Title: "中文搜索", Tags: []string{"标签"}, Aliases: []string{"别名"}, Body: "old old"}, {ID: "b", Path: "a.md", Body: "old"}}
	if e = i.Batch(docs, nil); e != nil {
		t.Fatal(e)
	}
	for _, q := range []string{"中文", "标签", "别名", "old", "z.md"} {
		h, e := i.Search(context.Background(), q, 10)
		if e != nil || len(h) == 0 {
			t.Fatalf("%s: %v %v", q, h, e)
		}
	}
	if m := literal(t, i, "old", 2); len(m) != 2 || m[0].ID != "b" || m[1].Start != 0 {
		t.Fatal(m)
	}
	buffer, e := i.UpdateText("a", "fresh fresh")
	if e != nil {
		t.Fatal(e)
	}
	h, e := i.Search(context.Background(), "old", 10, buffer)
	if e != nil || len(h) != 1 || h[0].ID != "b" {
		t.Fatal(h, e)
	}
	if m := literal(t, i, "fresh", 10, buffer); len(m) != 2 {
		t.Fatal(m)
	}
	if e = i.Batch([]Document{{ID: "a", Path: "z.md", Body: "external"}}, []string{"b"}); e != nil {
		t.Fatal(e)
	}
	if m := literal(t, i, "fresh", 10, buffer); len(m) != 2 {
		t.Fatal("external refresh cleared caller buffer", m)
	}
	if e = i.Close(); e != nil {
		t.Fatal(e)
	}
	i, e = Open(p)
	if e != nil {
		t.Fatal(e)
	}
	defer i.Close()
	if m := literal(t, i, "fresh", 10); len(m) != 0 {
		t.Fatal("buffer persisted", m)
	}
	h, e = i.Search(context.Background(), "external", 10)
	if e != nil || len(h) != 1 {
		t.Fatal(h, e)
	}
	if e = i.Batch([]Document{{ID: "bad"}}, []string{"a"}); e == nil {
		t.Fatal("expected invalid batch")
	}
	if len(literal(t, i, "external", 10)) != 1 {
		t.Fatal("invalid batch mutated state")
	}
	if e = i.Batch(nil, []string{"a"}); e != nil {
		t.Fatal(e)
	}
	i.Close()
	i, e = Open(p)
	if e != nil {
		t.Fatal(e)
	}
	if len(literal(t, i, "external", 10)) != 0 {
		t.Fatal("deleted document returned")
	}
}

func literal(t *testing.T, i *Index, q string, n int, b ...Document) []Match {
	t.Helper()
	m, e := i.Literal(context.Background(), q, n, b...)
	if e != nil {
		t.Fatal(e)
	}
	return m
}

func TestCancellationAndOverlayValidation(t *testing.T) {
	i, _ := openTest(t)
	if e := i.Batch([]Document{{ID: "x", Path: "x", Body: "hello"}}, nil); e != nil {
		t.Fatal(e)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, e := i.Search(ctx, "hello", 10); e == nil {
		t.Fatal("missing cancellation")
	}
	if _, e := i.Literal(ctx, "hello", 10); e == nil {
		t.Fatal("missing cancellation")
	}
	for _, d := range []Document{{ID: "x"}, {ID: "x", Path: "other"}} {
		if _, e := i.Search(context.Background(), "hello", 10, d); e == nil {
			t.Fatal("bad overlay")
		}
		if _, e := i.Literal(context.Background(), "hello", 10, d); e == nil {
			t.Fatal("bad overlay")
		}
	}
}
