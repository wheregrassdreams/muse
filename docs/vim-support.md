# Vim and Helix key support

Muse keymaps translate input into the editor's existing source ranges, display navigation, edit history, and clipboard APIs. This page describes the implemented subset; it does not claim full Vim or Helix compatibility.

## Vim support

| Area | Supported behavior |
| --- | --- |
| Modes | Normal, Insert, characterwise Visual (`v`), linewise Visual (`V`), and geometric blockwise Visual (`Ctrl+v`) |
| Movement | `h j k l`, `w b e`, `^`, `0`, `$`, `gg`, `G`; counts repeat supported motions. `j/k` follow displayed text rows and preserve the caret's screen X through vertical movement. `gj/gk` are aliases for that displayed-row movement. |
| Linewise selection | `V` immediately selects the complete Markdown source line, including its newline when present. `j/k` and counts extend or shrink by source lines; reversing across the starting line preserves the complete range. `Esc` leaves Visual mode. |
| Blockwise selection | Outside tables, `Ctrl+v` selects a rectangle in displayed rows and geometric columns; `h/j/k/l` extend and shrink it. Short/empty rows retain insertion points. `y/d/c`, `I/A`, `p/P`, native Copy/Cut/Paste and Undo share the editor's multi-range APIs. Soft-wrapped rows count as separate block rows. |
| Tables | `V` starts with the current structural row, and `j/k` extend/shrink whole rows without selecting Markdown separators. `h/l` move the structural boundary side; word/line motions preserve row structure. `Ctrl+v` starts with the current whole column; `h/l` retain the existing reversible structure/content transition into adjacent columns, while `j/k` move the active row without changing the selected columns. Copy produces a table; edits and Undo preserve structural semantics. |
| Operators | `d`, `c`, `y`; doubled operators (`dd`, `cc`, `yy`) and operator plus supported motion (for example, `dw`). Visual `d/c/y` operate on the active native selection. |
| Editing | `i`, `a`, `I`, `A`, `o`, `O`, `x`, `X`, `D`, `C` |
| Clipboard and history | `p`, `P`, `u`, `Ctrl+r`; linewise yank and paste retain linewise content. Editing and history use the editor's native transaction and clipboard hooks. |
| Input safety | Normal-mode printable commands are parsed only from single-rune text events. Multi-rune IME commits are not interpreted as commands; Insert mode passes composition through to native input. |

Visual `V` uses logical Markdown source lines. Ordinary `j/k` use the actual displayed layout, including soft-wrapped rows; the two notions of line are intentionally distinct. `^` moves to the first non-space or tab character, while `0` moves to the source line start. `D` and `C` stop at the current source line end and do not remove its newline.

Characterwise Visual selection includes both endpoint graphemes, in either direction. Painting, copying and editing use the same native range; combining characters and ZWJ emoji remain intact. Linewise selection retains the actual caret on the active source line rather than placing it at the half-open range's next-line boundary.

Leaving a table with linewise `V` continues a mixed document selection; reversing into the table restores structural rows. Leaving the top/bottom table boundary with columnwise `Ctrl+v` ends the structural selection and places Normal-mode focus in adjacent visible text. Text rectangles skip table cells. Native clipboard block metadata preserves row shape; explicit plain-text paste uses the ordinary text representation.

Chinese `w/b/e`, operators and native word selection use the shared `textnav` token index with an embedded simplified-Chinese dictionary. UTF-8 source offsets and UTF-16 display offsets are converted by the editor adapter. The provider and target-query API are described in [text-navigation-architecture.md](text-navigation-architecture.md).

## Helix support

The implemented Helix subset shares the editor's movement, source selections, operators, clipboard, and history paths. It provides Normal and Insert plus the existing persistent Select mode (`v`); selection followed by `d/c/y` acts on the native range. Helix-specific selection anchoring remains in the modal adapter. This is a small subset rather than a claim of complete Helix emulation.

## Not implemented

Vim text-object operators (`ciw`, `di(`), `f/F/t/T` motions, search and repeat (`/`, `n`, `N`), replace (`r`), change case, macros, named registers, marks, Ex commands, dot-repeat, and the full collection of Visual and blockwise operations are not implemented. EasyMotion/Flash labels and interactive jump modes are not implemented; the shared token target API is available for future consumers.
