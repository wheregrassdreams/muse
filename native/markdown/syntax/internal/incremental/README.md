# Incremental Markdown syntax engine

This parser fork is owned by `markdown/syntax` and supplies source-preserving
incremental updates for editor sessions. It originated from Muse's MyGo parser
fork at `v0.8.3-muse.2`; the public `markdown/syntax` facade uses it for both
full-source parses and incremental edits. Its semantic behavior is pinned by
the syntax package's conformance tests; Goldmark is retained only as a test
oracle. Keep edit-specific comment/frontmatter masking in the editor layer,
and keep shared Markdown grammar decisions here.
