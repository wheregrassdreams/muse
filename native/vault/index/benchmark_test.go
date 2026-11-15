package vaultindex

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const benchmarkFileBytes = 2 * 1024

func benchmarkVault(b *testing.B, count int) (string, *Index) {
	b.Helper()
	root := b.TempDir()
	writeBenchmarkVault(b, root, count)
	idx := New()
	if err := idx.Rebuild(root); err != nil {
		b.Fatal(err)
	}
	return root, idx
}

func BenchmarkRebuild(b *testing.B) {
	for _, count := range []int{1_000, 10_000} {
		b.Run(fmt.Sprintf("files=%d", count), func(b *testing.B) {
			root := b.TempDir()
			writeBenchmarkVault(b, root, count)
			b.ReportAllocs()
			b.SetBytes(int64(count * benchmarkFileBytes))
			b.ResetTimer()
			for n := 0; n < b.N; n++ {
				idx := New()
				if err := idx.Rebuild(root); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func BenchmarkQueryCommon(b *testing.B) {
	for _, count := range []int{1_000, 10_000} {
		b.Run(fmt.Sprintf("files=%d", count), func(b *testing.B) {
			_, idx := benchmarkVault(b, count)
			b.ReportAllocs()
			b.ResetTimer()
			for n := 0; n < b.N; n++ {
				benchmarkMatches = idx.Query("commonword")
			}
		})
	}
}

func BenchmarkQueryLimit500(b *testing.B) {
	for _, count := range []int{1_000, 10_000} {
		b.Run(fmt.Sprintf("files=%d", count), func(b *testing.B) {
			_, idx := benchmarkVault(b, count)
			b.ReportAllocs()
			b.ResetTimer()
			for n := 0; n < b.N; n++ {
				benchmarkMatches = idx.QueryLimit("commonword", 500)
			}
		})
	}
}

func BenchmarkBacklinks(b *testing.B) {
	for _, count := range []int{1_000, 10_000} {
		b.Run(fmt.Sprintf("files=%d", count), func(b *testing.B) {
			_, idx := benchmarkVault(b, count)
			b.ReportAllocs()
			b.ResetTimer()
			for n := 0; n < b.N; n++ {
				benchmarkLinks = idx.Backlinks("notes/00000.md")
			}
		})
	}
}

func BenchmarkBacklinksSparse(b *testing.B) {
	for _, count := range []int{1_000, 10_000} {
		b.Run(fmt.Sprintf("files=%d", count), func(b *testing.B) {
			root := b.TempDir()
			writeSparseBacklinkVault(b, root, count)
			idx := New()
			if err := idx.Rebuild(root); err != nil {
				b.Fatal(err)
			}
			b.Run("candidate-index", func(b *testing.B) {
				b.ReportAllocs()
				b.ResetTimer()
				for n := 0; n < b.N; n++ {
					benchmarkLinks = idx.Backlinks(fmt.Sprintf("Target.md#uncached-%d", n))
				}
			})
			b.Run("cached-result", func(b *testing.B) {
				_ = idx.Backlinks("Target.md")
				b.ReportAllocs()
				b.ResetTimer()
				for n := 0; n < b.N; n++ {
					benchmarkLinks = idx.Backlinks("Target.md")
				}
			})
			b.Run("full-scan-reference", func(b *testing.B) {
				b.ReportAllocs()
				b.ResetTimer()
				for n := 0; n < b.N; n++ {
					benchmarkLinks = backlinksFullScan(idx, fmt.Sprintf("Target.md#uncached-%d", n))
				}
			})
		})
	}
}

func writeSparseBacklinkVault(b *testing.B, root string, count int) {
	b.Helper()
	if err := os.WriteFile(filepath.Join(root, "Target.md"), []byte("# Target\n"), 0o644); err != nil {
		b.Fatal(err)
	}
	for n := 0; n < count; n++ {
		target := fmt.Sprintf("unique-%05d", n)
		if n%100 == 0 {
			target = "Target"
		}
		path := filepath.Join(root, "notes", fmt.Sprintf("%05d.md", n))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			b.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("[["+target+"]]\n"), 0o644); err != nil {
			b.Fatal(err)
		}
	}
}

func backlinksFullScan(idx *Index, target string) []Link {
	idx.mu.RLock()
	defer idx.mu.RUnlock()
	var out []Link
	for _, path := range sortedPaths(idx.docs) {
		for _, link := range idx.docs[path].Links {
			if resolvesLocked(idx, path, link.Target, target) {
				out = append(out, link)
			}
		}
	}
	return out
}

var benchmarkMatches []Match
var benchmarkLinks []Link

func writeBenchmarkVault(b *testing.B, root string, count int) {
	b.Helper()
	padding := strings.Repeat("commonword searchable body text for benchmark allocation and scan measurements. ", 40)
	for n := 0; n < count; n++ {
		path := filepath.Join(root, "notes", fmt.Sprintf("%05d.md", n))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			b.Fatal(err)
		}
		prefix := fmt.Sprintf("# Note %05d\n## section\n[[notes/00000#section|target]] and [reference](notes/00000.md).\n", n)
		content := prefix + padding
		if len(content) > benchmarkFileBytes {
			content = content[:benchmarkFileBytes]
		} else {
			content += strings.Repeat("x", benchmarkFileBytes-len(content))
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkQueryLimitSparse(b *testing.B) {
	for _, count := range []int{1000, 10000} {
		b.Run(fmt.Sprintf("files=%d", count), func(b *testing.B) {
			_, idx := benchmarkVault(b, count)
			for _, query := range []string{"term-that-does-not-exist", fmt.Sprintf("Note %05d", count-1)} {
				b.Run(query, func(b *testing.B) {
					b.ReportAllocs()
					b.ResetTimer()
					for n := 0; n < b.N; n++ {
						benchmarkMatches = idx.QueryLimit(query, 500)
					}
				})
			}
		})
	}
}
