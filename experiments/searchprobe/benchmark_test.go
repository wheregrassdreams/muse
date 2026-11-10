package searchprobe

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"
)

func BenchmarkProbe(b *testing.B) {
	for _, n := range []int{1000, 10000} {
		b.Run(fmt.Sprint(n), func(b *testing.B) {
			docs := Synthetic(n)
			ctx := context.Background()
			b.Run("Build", func(b *testing.B) {
				b.ReportAllocs()
				b.SetBytes(int64(n * 2048))
				for k := 0; k < b.N; k++ {
					b.StopTimer()
					p := filepath.Join(b.TempDir(), "index")
					b.StartTimer()
					i, e := Open(p)
					if e != nil {
						b.Fatal(e)
					}
					if e = i.Batch(docs, nil); e != nil {
						b.Fatal(e)
					}
					if e = i.Close(); e != nil {
						b.Fatal(e)
					}
				}
			})
			p := filepath.Join(b.TempDir(), "index")
			i, e := Open(p)
			if e != nil {
				b.Fatal(e)
			}
			if e = i.Batch(docs, nil); e != nil {
				b.Fatal(e)
			}
			for _, q := range []string{"commonword", "rareword", "absentword", "中文"} {
				b.Run("Search/"+q, func(b *testing.B) {
					b.ReportAllocs()
					for k := 0; k < b.N; k++ {
						if _, e := i.Search(ctx, q, 500); e != nil {
							b.Fatal(e)
						}
					}
				})
				b.Run("Literal/"+q, func(b *testing.B) {
					b.ReportAllocs()
					for k := 0; k < b.N; k++ {
						if _, e := i.Literal(ctx, q, 500); e != nil {
							b.Fatal(e)
						}
					}
				})
			}
			b.Run("Update", func(b *testing.B) {
				d := docs[n-1]
				b.ReportAllocs()
				for k := 0; k < b.N; k++ {
					d.Body = fmt.Sprintf("revision%d ", k) + docs[n-1].Body
					if e := i.Batch([]Document{d}, nil); e != nil {
						b.Fatal(e)
					}
				}
			})
			if e = i.Close(); e != nil {
				b.Fatal(e)
			}
			b.Run("Reopen", func(b *testing.B) {
				b.ReportAllocs()
				for k := 0; k < b.N; k++ {
					i, e := Open(p)
					if e != nil {
						b.Fatal(e)
					}
					if e = i.Close(); e != nil {
						b.Fatal(e)
					}
				}
			})
		})
	}
}
