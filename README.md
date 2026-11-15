# Muse

**What you write is what you see.** A fully native Markdown editor with a tool-using AI writing agent — the whole app is drawn by Go on the GPU: no WebView, no Electron.

[![Release](https://img.shields.io/github/v/release/CarlOpenLab/muse?color=24292f)](https://github.com/CarlOpenLab/muse/releases/latest)
[![License](https://img.shields.io/badge/license-MIT-24292f)](LICENSE)
[![Go](https://img.shields.io/badge/Go-1.27+-24292f)](native/go.mod)

English | [简体中文](README.zh-CN.md)

Website: **https://carlopenlab.github.io/muse/**

Built with [MyGo](https://mygo.egoist.dev/). The window, icon rail, file tree, outline, search, the AI agent panel and the editor itself are all drawn by Go with MyGo's `ui` package (Metal / Direct3D 11 / OpenGL), with text laid out by the system's own engine (Core Text / DirectWrite / Pango). One Go binary — no browser engine and no Node at runtime.

## The editor

The editor (`native/editor`) is one surface the whole document is written on, behaving as Typora does:

- **Seamless WYSIWYG** — the document *is* the rendering: the caret moves continuously through paragraphs, headings, lists, quotes, tables, code and math, and selections cross blocks to copy, cut and delete.
- **Syntax only at the caret** — block syntax (`#`, list markers, `>`, fences, table pipes) never shows; the markers of `**bold**`, `` `code` ``, `[links](url)`, `$math$` and `![images](src)` appear in light gray only while the caret is in them, and render again when it leaves.
- **Markdown as you type** — `# `, `- `, `1. `, `- [ ] `, `> ` at the start of a line become a heading, list, task or quote at once; ```` ``` ```` opens a closed code block, `$$` a math block, and `| a | b |` + Enter a table.
- **Typora's keys** — Enter continues a list (and leaves it on an empty item) or a quote; `Shift+Enter` breaks the line; Backspace at the start of a block turns a heading into a paragraph and an item out of its list; `Tab`/`Shift+Tab` nest list items and walk table cells; `⌘B/I/U/E/K`, `⌘0–6`, `⌘⇧Q/[/]/K/M` and `⌘⌥T` format; full undo and redo — the agent's edits included.
- **Native rendering** — headings, task lists, code highlighted in 30+ languages, tables (alignment, row/column commands), images (local, remote, pasted or dropped — saved into `assets/`), block and inline math typeset with go-latex on the text's baseline, footnotes, YAML front matter and inline HTML formatting tags.
- **Mermaid, natively** — flowcharts, sequence, state, pie, class and Gantt subsets, drawn whenever the caret is elsewhere.
- **Input methods** — Chinese and Japanese input methods compose at the caret, with their candidate window beside it.
- **Workspace panel** — file tree, an outline that follows the caret, find and replace, and the AI assistant share one sidebar, switched from the icon rail; text files that are not Markdown (`.json`, `.go`, …) open as highlighted code.

## The agent

The AI panel is a real writing agent, not a chat box:

- **13 document tools** — list, read, search, outline, workspace tree, stats, open, edit (exact-match replace), insert, write, rename, trash-delete, create folder.
- **Tool approval cards** — before any write, the agent shows what it is about to change. Approve once, always for this session, or deny; denials flow back into the conversation.
- **Permission presets** — *Ask every time* / *Full access* / *Read-only*, one click next to the composer.
- **Capability groups** — browse, edit and file-management tools can be switched on/off per group in settings; disabled tools are never sent to the model.
- **Transparent turns** — collapsible thinking, per-tool status rows with arguments and results, streamed Markdown answers; your words sit right, the agent's steps sit left.
- **OpenAI-compatible** — any endpoint (OpenAI, DeepSeek, local gateways); endpoints without function calling degrade gracefully to plain chat.

Privacy: agent tools are sandboxed to the workspace root; API keys and chat history stay in local config files (mode 0600).

## Install

Grab a build from [Releases](https://github.com/CarlOpenLab/muse/releases/latest) (Windows / macOS / Linux), or from source:

```bash
sh scripts/dev.sh             # run (signed, so macOS keeps permission grants)
sh scripts/build.sh           # build a binary, signed with the same identity
sh scripts/go.sh -C native test ./... # tests, with native search library
```

The scripts sign with the first `Apple Development` identity in your keychain
(override with `MACOS_SIGNING_IDENTITY`), so macOS permission grants survive
rebuilds. On Windows, use Git Bash and `sh scripts/go.sh -C native run .` /
`sh scripts/go.sh -C native build -o Muse.exe .`.

Requires Go 1.27.3+, CGO and a C compiler. Experimental full-text search uses
Anytype's Tantivy binding; the scripts download and checksum its pinned static
library on first build. Linux uses the musl artifact and requires `CC=musl-gcc`.
See [search build and runtime notes](docs/tantivy-search.md).
No Node, no pnpm, no vite — there is no frontend build at all.

## Project layout

```
native/
├── cmd/muse-native/   thin executable entry point
├── app/               process composition root and platform adapter wiring
├── application/       document, vault and workspace use cases
├── document/          note, revision, metadata and persistence ports
├── editor/            WYSIWYG editing, block systems and text layout
├── markdown/          Markdown syntax, rendering, images, math and diagrams
├── search/            query language, metadata filters, ranking and full-text index
├── vault/             filesystem catalog, links, index and file watching
├── workspace/         persisted session state and repository contract
├── platform/          filesystem and OS-backed adapters
├── nativeui/          native window and UI composition
├── agent/             AI client and tool workflows
├── config/            application settings and theme configuration
├── events/            typed, instance-scoped application events
└── ui/                shared native UI infrastructure, including scrollbars
```

## Known limits

- Math is typeset with go-latex; rarer TeX commands may fall back to source.
- Mermaid covers flowcharts, sequence, state, pie, a class-diagram subset and a Gantt subset; advanced features may render as code.
- Chat history is capped at the last 100 messages.

## License

[MIT](LICENSE) © Carl Open Lab
