# Searchprobe: isolated Bleve PoC

Independent module, pinned Bleve **v2.5.7**, no native dependency or integration.
Caller supplies Document ID/Path/Title/Tags/Aliases/Body. No vault walking, file reading,
Markdown parsing, watcher, link resolution or backlinks. Index path is storage only.

```sh
cd experiments/searchprobe
CGO_ENABLED=0 go test ./...
CGO_ENABLED=0 go build ./...
CGO_ENABLED=0 go run ./cmd/searchprobe -n 10000
# Keep/reopen a caller-selected index:
CGO_ENABLED=0 go run ./cmd/searchprobe -index /tmp/searchprobe-index -n 1000
CGO_ENABLED=0 go run ./cmd/searchprobe -index /tmp/searchprobe-index -n 0 -query 中文
CGO_ENABLED=0 go test -run '^$' -bench BenchmarkProbe -benchtime=1x -count=1 -benchmem
```

API: Open; Batch(upserts,deletes); UpdateText(id,body) returns a caller-owned
Document; Search(ctx,text,limit,buffers...); Literal(ctx,text,limit,buffers...); Close.
Batch validates first, then submits one Bleve batch including per-ID JSON snapshots
and an ID-only manifest. Deletes win; repeated upserts use the last value. Reopen
loads snapshots by ID, never vault files. Single updates serialize only changed
source, but still copy the document map and sort/rewrite all manifest IDs: O(N).
Reopen eagerly loads all bodies; this is not a lazy storage design.

Search is **one result per document**, CJK analyzer on path/title/tags/aliases/body.
Plain text is AND within a field, OR across fields. It is not query-string syntax,
substring search, or a phrase guarantee. CJK bigrams may miss single characters;
Literal is the exact fallback. Results currently sort by path then ID, NOT score.
Scores are exposed for inspection; temporary overlay and disk scores are not
comparable ranking evidence. It retrieves all matching documents before applying
limit, so common queries allocate with total candidate count.

Literal matches Body only. Pass the complete original source as Body to mirror
vaultindex.SearchText, including Markdown/frontmatter/code. It trims query,
uses Go Unicode lowercase (not normalization/full case folding), returns every
non-overlapping occurrence in path/ID/byte order, and uses original UTF-8 byte
Start/End offsets. Limit counts occurrences; nonpositive limits/empty trimmed
queries produce none. No token candidate filter: complete scan preserves substring,
single CJK character and punctuation/newline matches. ASCII and length-preserving
Unicode use direct offsets; byte maps are only allocated for length-changing
lowercase documents (e.g. İ). No snippets/highlighting are implemented.

Buffers are per-call immutable document values. Existing IDs must retain their
persisted Path; ID/Path must be nonempty; duplicate overlay IDs are rejected.
New IDs are permitted. Overrides suppress old disk matches even when new text
has no match. UpdateText neither publishes nor persists anything. Caller retains
its returned buffer across external Batch refreshes; queries without it and reopen
see persisted snapshots. Caller owns generation/version checks and dirty-buffer
lifetime. Do not mutate slices concurrently with a call.

Search uses Bleve SearchInContext; Literal checks cancellation during document
scans. Literal releases the read lock after snapshotting documents. Search retains
a read lock to protect the Bleve handle and coherent metadata: it can block Batch
and Close until query cancellation/completion. Context does not interrupt lock
acquisition, overlay indexing, sorting, or individual strings operations. This is
an explicit non-production limitation, not a full concurrent lifecycle design.

Benchmark setup (corpus generation, query index construction) is outside query
subbenchmark timing. Build measures creation+batch+close; update measures one
persisted document + manifest; reopen includes close. Every synthetic Body is
exactly 2048 UTF-8 bytes, with English common/rare terms and Chinese text. Queries
use limit 500. The checked-in run is **1 iteration, hot synthetic**, not statistical
or production evidence; no direct speedup claim against vaultindex. B/op includes
cumulative allocations, not peak RSS. CLI emits durations, build cumulative Go
allocations, current Go heap, and index file bytes after close (not RSS/physical
blocks/fsync durability). Bleve background merges/cache state affect measurements.
See benchmark-results.txt and cli-results.json for this machine's actual run.

Tests cover literal edge cases and offsets, metadata CJK search, path ordering,
occurrence limits, update/delete/reopen, rejected batches, overlay shadowing and
non-persistence, validation and cancellation. Crash consistency, schema migration,
production ranking, full Unicode normalization, large overlays, and concurrent
load acceptance are outside this PoC. Native/product tests are separate evidence.

Version choice: v2.5.7 is a fixed release used as this experiment's reproducible
baseline, not a claim to be latest or a production dependency recommendation.
Default Bleve text mappings retain stored fields and term vectors; together with
per-ID snapshots this is intentionally unoptimized and can duplicate source data.
The first measured CLI 10k run used about 2.286 GB cumulative build allocations,
1.665 GB heap before explicit post-build GC, and 242.08 MB index files for 20.48 MB
body data (~11.8x). Heap is neither RSS nor a peak. The CLI additionally reports
retained_heap_after_gc_bytes after stopping the build timer; this GC is excluded
from all reported operation times. It does not isolate Bleve from caller memory.

Recorded 1x benchmark (Apple M3, darwin/arm64):

| operation | 1k | 10k |
|---|---:|---:|
| create/batch/close | 187.47 ms | 1957.90 ms |
| ordinary common (limit 500) | 0.783 ms | 6.324 ms |
| ordinary rare | 0.148 ms | 0.122 ms |
| ordinary no hit | 0.105 ms | 0.113 ms |
| literal common (limit 500) | 7.596 ms | 9.644 ms |
| literal rare | 14.695 ms | 152.453 ms |
| literal no hit | 15.886 ms | 159.750 ms |
| single persisted update | 14.542 ms | 210.348 ms |
| reopen + close | 51.148 ms | 181.529 ms |

Concurrent Bleve background work can contribute allocations even during literal
subbenchmarks. Update cost and index amplification make this unsuitable as a
production adoption result without further investigation. Supplied vaultindex
baseline timings use different semantics/workloads and are background only.
