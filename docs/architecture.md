# Muse native architecture contract

Status: target architecture for the modular-monolith refactor. This document is the acceptance baseline for package moves and code wiring. If implementation disagrees with this contract, either change the implementation or record an explicit architecture decision here before continuing; do not silently preserve transitional structure.

## Goals

- Keep one native Go module and one desktop application while making feature ownership and dependency direction explicit.
- Allow roadmap features to add behavior through stable, typed module APIs rather than expanding `nativeui.App` or reaching into another module's internals.
- Keep document editing and Markdown rendering fast: avoid copies across package APIs, avoid per-frame global events, and keep caches/resource lifetimes with their owning feature.
- Complete the migration in one target architecture. Compatibility packages, duplicate implementations, old import paths, and temporary forwarding layers are not an acceptable end state.

## Target package tree

```text
native/
├── app/                           # composition root and module lifetime
├── cmd/muse-native/               # thin executable entry point
├── application/                   # cross-module user use cases
│   ├── document/                  # open/save/export note workflows
│   ├── vault/                     # open/switch/refresh vault workflows
│   └── workspace/                 # restore and persist session workflows
├── events/                        # minimal typed in-process event contract
├── document/                      # durable note model and policies
│   ├── frontmatter/
│   ├── repository.go              # note/recovery ports and domain errors
│   └── types/                     # stable value types only
├── vault/                         # vault identity and note relationships
│   ├── files/                     # discovery/catalog
│   ├── index/                     # note metadata and parsed structure projections
│   ├── links/                     # resolution/backlinks
│   └── watch/                     # watcher lifecycle and vault event policy
├── search/                        # query domain and search execution
│   ├── query/                     # syntax, AST, validation
│   ├── fields/                    # type/tags/time semantics
│   ├── plan/                      # query compilation/execution
│   ├── rank/                      # scoring and time-aware ranking
│   └── quickopen/                 # path/title/alias candidate matching and ranking
├── workspace/                     # tabs, active view, layout and restoration
├── editor/                        # public editing-session facade and composition
│   ├── input/                     # keymap, Vim modes and command interpretation
│   │   └── vim/                   # Vim mode state machine, prefixes and command mapping
│   ├── navigation/                # source-position movement and structural navigation
│   ├── layout/                    # editor block geometry and virtualized layout policy
│   ├── blocks/
│   │   ├── inline/
│   │   ├── paragraph/
│   │   └── table/                 # table editing subsystem
│   └── internal/
│       ├── core/                  # editor-private buffer/transaction primitives
│       └── textlayout/            # editor-private text shaping/layout implementation
├── markdown/                      # public Markdown parse/render facade
│   ├── syntax/
│   ├── render/
│   ├── images/
│   ├── math/
│   └── diagrams/
├── nativeui/components/
│   ├── editor/                     # MyGo input/render adapter over editor.Session
│   └── markdown/                   # MyGo painter over neutral Markdown models
├── agent/                         # AI session/tool/provider-independent flow
├── config/                        # serialized settings, defaults and migration
├── theme/                         # runtime palette and visual tokens
├── ui/                            # reusable UI primitives
│   ├── scrollbar/
│   └── iconlibrary/
├── nativeui/                      # Muse panels and presentation composition
│   ├── window/                     # generic window, pane and overlay hosting/lifecycle
│   ├── surfaces/                   # feature-owned window content implementations
│   │   ├── noteeditor/              # compose each pane's document/editor session
│   │   ├── imagepreview/
│   │   ├── imagedocument/
│   │   ├── mermaidpreview/
│   │   └── quickopen/
│   ├── workspacepresenter/         # project workspace tabs/split tree into pane slots
│   ├── panels/
│   │   ├── filetree/
│   │   ├── outline/
│   │   ├── search/
│   │   ├── naviconpicker/
│   │   ├── vaulthome/
│   │   ├── properties/
│   │   ├── agent/
│   │   └── settings/
│   ├── components/
│   └── interactions/
├── platform/                      # OS/runtime adapters
│   ├── cachepath/                  # OS/XDG cache-root policy
│   └── filesystem/
│       ├── documentstore/         # concrete note/recovery persistence
│       ├── imagecache/            # atomic image cache persistence
│       ├── naviconstore/          # concrete per-vault icon preference persistence
│       ├── recentstore/           # concrete recent-document persistence
│       ├── vaultfiles/             # concrete vault file operations
│       ├── vaultindexsource/      # concrete recursive source for vault indexing
│       ├── vaultsearchstore/      # concrete persistent search-index storage
│       ├── workspacestore/        # concrete workspace snapshot persistence
│       └── watcher/               # concrete filesystem watcher
└── locales/                       # localization data and generated catalog
```

The child entries in this tree are real Go package boundaries, not merely folders used to group files. `editor` and `markdown` are public facades; they should compose focused subsystems rather than retain the current large implementation bodies. A subsystem may stay in its parent package only when it shares private invariants that cannot yet be expressed through a narrow typed contract. Do not create one-file packages or empty forwarding shells just to match the tree.

### Editor extraction map

`native/editor` is not intended to stay flat. The current files are large and some mix policy with MyGo rendering, so migration must split by responsibility rather than move each file wholesale. The main groups are:

| Target package | Responsibility | Current flat-file families to inspect and split |
|---|---|---|
| `editor` facade | Public `Session`, neutral input/transaction/frame types, and composition only | Keep the public API small (`api.go`, `session.go`); do not leave the current `Editor` view implementation as the production facade |
| `editor/internal/core` | Buffer, selection, undo/history, source transactions, system patches | Core portions of `editor.go`, `edit.go`, `transaction.go`, `history.go`, `operations.go`, `multirange.go`, and `system_patch.go` |
| `editor/input` | Toolkit-neutral key/text event and operation contracts shared by keymaps | `event.go`, `key_event.go`, `operation.go`, and `sequence.go` |
| `editor/input/vim` | Independent Vim mode state machine, pending prefixes/counts/registers, and mapping from neutral key/text events to typed editor operations | `modal.go`, Vim-specific command interpretation, and mode/sequence tests; extract Editor-private callbacks into a narrow Host contract during wiring |
| `editor/navigation` | Source-position movement and structural navigation policy | `nav.go`, `comment_nav.go`, and movement policy currently mixed into Vim/navigation files |
| `editor/layout` | Block geometry, visible-range indexing, virtualization and scroll policy | Geometry/policy portions of `layout.go`, `horizontal_scroll.go`, and `vim_virtual_image_rows.go` |
| `editor/blocks/inline` | Inline formatting, math editing state, wiki/tag/comment behavior | Semantic portions of `format.go`, `inlinemath.go`, `wikilink.go`, `tag.go`, and `comment.go` |
| `editor/blocks/paragraph` | Heading/list/fold/break behavior and paragraph transactions | `folding.go`, `typed_breaks.go`, `paragraph`/heading/list portions of editor commands |
| `editor/blocks/table` | Table selection model, hit testing, grid geometry, and row/column operations | Semantic portions of `table_document_selection.go`, `table_region_*.go`, `table_virtual_edit.go`, and `table_visual_modes.go` |
| `nativeui/components/editor` | MyGo input/render adapter, rich-text editing, IME, painting, and block-specific UI | MyGo-dependent portions of `view.go`, `display.go`, `paint.go`, `ime.go`, `rectangle.go`, `assets.go`, `source_highlight.go`, `image_controls.go`, `table_*_ui.go`, `table_edges.go`, `table_cell_menu.go`, and `table_reorder.go` |

The table entries are responsibility guides, not mechanical move lists: files that combine model behavior and UI must be split before their owners change. Tests move with the package that owns the tested behavior; Vim mode/command tests belong with `editor/input/vim`, movement semantics belong with `editor/navigation`, UI/input adapter tests belong with the component adapter, and transaction tests stay with the core. A package is justified when it has a coherent contract and more than a forwarding file.

Vim remains a selectable editor input strategy, not a second editor core. Its state machine owns mode transitions, command prefixes/counts, register selection, and command-to-operation mapping. It calls typed host capabilities for source/selection queries and operation dispatch; it must not reach into `Editor` fields or own buffer/history. `editor/navigation` implements movement semantics, `editor/blocks/*` implements block operations, and `editor/internal/core` commits edits. The UI adapter translates MyGo events to neutral input events and renders the current mode label/command prompt from an immutable Vim view state.

Text shaping needs an explicit backend boundary. `editor/layout` owns neutral geometry and virtualization policy. Any MyGo text measurement, painter, font, or rich-text API belongs under `nativeui/components/editor`; `editor/internal/textlayout` may remain only for backend-neutral shaping algorithms and must not import MyGo. This replaces the earlier ambiguous wording that assigned all shaping implementation details to `editor/internal/textlayout`.

`editor/blocks/*` owns the behavior and session state of each block subsystem; table syntax and parsed structure remain in Markdown, while table selection and edits belong to `editor/blocks/table` and table painting/input belongs to the editor UI component.

`markdown/syntax` owns Markdown parsing and AST values. `markdown/render` maps parsed values to rendered blocks. `images`, `math`, and `diagrams` own their bounded media/rendering pipelines and caches; they expose results to the renderer without depending on the editor session. The root `markdown` package is the stable entry point for common parse/render operations.

`search/quickopen` owns fuzzy matching across candidate path, title, and aliases, including rank and rune-based highlight positions. The Quick Open surface owns only its query/selection view state and input presentation; it consumes ranked candidates and returns typed actions. It does not scan or rank the full catalog itself. Mermaid parsing and diagram models remain in `markdown`; the Mermaid Preview surface owns only its transient viewport state and rendering host integration.

`nativeui/window` owns the generic runtime contract for opening, closing, focusing and placing windows, panes and overlays. It exposes stable IDs, generic descriptors and lifecycle operations; it does not expose a feature's concrete state type. Feature constructors take their own typed inputs and return a generic hosted-content interface plus an opaque handle. The manager owns host placement, z-order, focus and lifecycle; the hosted content owns its state, rendering and feature actions. Image preview owns zoom/pan/decode state, Quick Open owns query/selection state, and Mermaid Preview owns its diagram view state behind that generic handle. `workspace` owns durable document/pane descriptors, while transient overlays such as Quick Open and previews are not restored unless a product requirement explicitly says so. Persisted layout never contains bitmaps, widgets or feature-private state.

The runtime manager exposes `FocusedPane(host)` as a feature-neutral routing context, including while an overlay owns keyboard focus. It can activate and render one generic content handle per pane, but does not own tab presentation or split geometry: `workspace` persists tab order, active document per pane, and the split tree; `nativeui/workspacepresenter` maps that model to pane slots and asks the window manager to render each slot's active content. The presenter owns MyGo rows/columns, dividers, resize gestures, and focus requests; it emits typed workspace actions instead of editing workspace state itself. Pane focus and content focus remain separate so modal and transient surfaces do not change the active document pane.

The MyGo-native [GoRex implementation](https://github.com/egoist/gorex) is a useful reference for the layer above MyGo's window and element APIs. GoRex keeps a per-tab split tree, focused/zoomed pane, and recent-focus order in app state; its view recursively maps split nodes to MyGo rows/columns and divider elements. Pane focus is updated from the focused terminal element, and directional focus uses the rendered pane bounds. Its persistence serializes tab order, split ratios, active tab, and each tab's focused pane. Muse should follow the same ownership split: `workspace` owns the durable tab/layout/focus model, `nativeui/workspacepresenter` owns MyGo layout and focus requests, and `nativeui/window` owns generic content placement/lifecycle. MyGo supplies the native window and UI primitives; the application builds the tabs/split behavior.

The sidebar `panels` are distinct from window surfaces. Each panel package owns a view model and typed actions, receives data and capabilities through its public API, and returns actions to the root controller for use-case dispatch. Panel packages must not declare `package nativeui` in a child directory or reach the root `App`'s private fields. `surfaces/noteeditor` composes one pane's `application/document` workflow and `editor.Session` into generic hosted content through `components/editor`; its source, dirty/save state, cursor, and editor instance are private to that pane surface. The `App` routes typed commands to the pane surface and does not keep parallel document or editor state. `components` owns reusable UI pieces such as document-type presentations and shared primitives; icon assets and lookup live in `ui/iconlibrary`. `interactions` owns shared focus, keyboard routing and drag/drop mechanics when more than one panel consumes them. Feature-specific policy remains in `application`, `vault`, `search`, `document`, or `agent` rather than in a panel package.

`nativeui.App` constructs a feature surface with that feature's typed constructor, opens it through the generic manager, and holds only opaque handles or generic descriptors—not fields such as `imagepreview.State`, `quickopen.State` or `diagramPreviewState`. The main editor is also a pane content surface: App asks `nativeui/surfaces/noteeditor` to compose the pane's document workflow, editor session and `nativeui/components/editor` adapter, then registers that generic content for the active workspace pane. `nativeui/workspacepresenter` renders each pane through `RenderPane`. App routes pane-scoped commands using `FocusedPane` plus typed feature-owned capabilities; it does not reach into editor/widget state. A surface maps generic open/close/focus lifecycle to its private session. The manager handles host placement and lifecycle only; it does not interpret image, document, search or editor behavior. If descriptor-based restore later needs factories, the composition root registers factories by stable kind and passes only portable descriptors; the concrete state remains private. Avoid a universal mutable state bag or a global service locator.

Source-position text navigation belongs to `editor/navigation`; MyGo event translation, focus, painting, text measurement, and viewport geometry stay in `nativeui/components/editor`. `editor.Session` exposes backend-neutral input intents, transactions, and immutable revision frames. `nativeui/components/markdown` paints the neutral Markdown model for both editor and standalone preview callers. `platform/cachepath` owns OS/XDG cache-root policy, while feature modules receive cache storage through consumer-owned ports. Media decoding/cache policy remains with the feature that owns it, and concrete filesystem access remains in platform adapters.

## Ownership and model boundaries

| Module | Owns | Must not own |
|---|---|---|
| `document` | Durable `Note` path identity, metadata, source/revision semantics, save/conflict/recovery policies | Cursor, selection, focus, scroll, widgets, parsed render nodes |
| `editor` | Mutable `Buffer`, edit transactions, undo/redo, selection, input mode, navigation and block editing | Vault indexing, persistence implementation, durable note identity, OS APIs |
| `markdown` | Markdown syntax/AST and derived rendering structures for inline, paragraph, table, image, math and diagrams | Durable note aggregate, editor selection/commands, application panel state |
| `vault` | Vault identity, file catalog, index lifecycle, link resolution/backlinks and watcher-to-vault policy | Search query language/ranking, native UI, document editing buffer |
| `search` | Query language/AST, field/operator meaning, execution plan, ranking and result contract | Filesystem traversal, concrete vault index layout, panel state |
| `workspace` | Open documents, active view, tabs/panes/layout, restoration snapshot and repository contract | Note source of truth, editor implementation internals, filesystem persistence |
| `application` | User workflows spanning modules, ordering, cancellation and transaction boundaries | Domain invariants that belong to a feature module |
| `nativeui` | Rendering, input translation, focus/panel state and calling use cases | Business rules, storage/index implementation, query execution |
| `config` / `theme` | Persisted settings schema vs derived runtime style values | Editor/document policy or interaction state machines |
| `ui` | Backend-neutral reusable UI behavior/components | Muse business concepts or platform-specific APIs |
| `platform` | OS/runtime implementations for dialogs, clipboard, filesystem, cache paths and resource access | Feature policy or view composition |

The current persistence model uses the normalized note path as identity. The repository does not persist a separate stable note ID or vault ID. Renaming or moving a note is therefore a delete plus create; do not synthesize a path hash and describe it as a stable identity. `document.Note` carries the current path, source, parsed metadata and disk revision. A future stable-ID migration must introduce persisted identity before changing this contract.

There are three separate representations of a note:

1. `document.Note` is the durable path-identified domain record and source/revision boundary.
2. `editor.Buffer` is the active mutable editing session and commits source changes as transactions.
3. `markdown`'s syntax/render tree is derived from a source revision and may be rebuilt or incrementally updated.

They must not be merged into one state object. Table syntax belongs to `markdown`; cell selection, hit testing, grid geometry and row/column commands belong to `editor/blocks/table`. Table edits commit through normal editor transactions. Fold/view mode is session state and is persisted by `workspace` only if restart restoration promises it.

## Dependency rules

```mermaid
flowchart TD
  DesktopCmd[cmd/muse-native] --> App[app module assembly]
  CLIFuture[cmd/muse-cli future] -.-> App
  DesktopCmd --> UI[nativeui desktop adapter]
  UI --> WindowHost[nativeui/window generic host]
  WindowHost --> Surfaces[feature-owned surfaces]
  UI --> UseCases[application use cases]
  App --> UseCases
  App --> Platform[platform adapters]
  UseCases --> Document[document]
  UseCases --> Vault[vault]
  UseCases --> Search[search]
  UseCases --> Workspace[workspace]
  UseCases --> Editor[editor]
  Editor --> Markdown[markdown syntax/render contracts]
  Vault --> Document
  SearchPort[search Corpus port] -.implemented by.-> Vault
  Platform -.implements consumer-owned ports.-> UseCases
  UI --> SharedUI[ui]
  UI --> Theme[theme]
  DomainEvents[typed module events] --> Dispatcher[application event dispatch]
```

- Domain packages do not import `nativeui`, MyGo widget packages, `platform`, or executable packages.
- `markdown` never imports `editor`; editor consumes parsed structures and owns edits.
- `search` owns the corpus/query contracts; `vault/index` implements the corpus contract without exposing its storage layout. `vault/index` also projects parsed link structure through an immutable narrow contract; `vault/links` owns path/title/alias resolution, outgoing links, backlinks, and their derived caches.
- Ports are declared by the module/use case that needs the capability. `document` owns note/recovery repository contracts and domain errors; `platform/filesystem/documentstore` implements them. `workspace` owns its snapshot repository contract; `platform/filesystem/workspacestore` implements it. Concrete filesystem, dialogs, clipboard, clock, image/font access and persistence implementations live in adapters.
- `app` is the only composition root that selects concrete implementations and registers extension contributions.
- `nativeui/window` owns live window/pane/overlay hosting; `workspace` owns the portable descriptors that can be restored. Feature surfaces keep their state private behind opaque handles.
- No cycles, package-level mutable service locator, or broad `common`, `model`, `strategy`, or `utils` packages.

## Future capability seams

The target should make later capabilities fit through existing module APIs without implementing them in this refactor:

| Future capability | Existing seam | Boundary to preserve |
|---|---|---|
| First-party and third-party extensions | Feature-owned typed contributions, validated and registered by `app` | Keep extension contracts in public feature APIs; never expose `internal` packages, `nativeui.App`, filesystem paths, or global service locators as plugin APIs. Add dynamic loading, permissions and compatibility/version negotiation only when an actual plugin runtime is in scope. |
| CRDT-backed collaboration with Markdown files retained as the durable format | `editor` transactions, `document.Repository`, and application save/session workflows | Keep CRDT state out of widgets and filesystem adapters. Treat the current path identity and disk revision as today's storage contract, not as a permanent collaboration identity or CRDT version. A future stable document ID and change/revision contract must be introduced deliberately; materialized Markdown remains an adapter concern. |
| Headless operation | `application` use cases and domain models have no UI or MyGo dependency | Use cases must be callable without creating a window, UI event loop, or desktop singleton. Background work and lifetimes must be explicit and cancellable. |
| CLI | Separate command adapter over the same application APIs and ports | A CLI command should parse arguments, call application use cases, and format results. It must not invoke nativeui rendering or duplicate vault/search/document rules. Keep the desktop executable similarly thin. |
| Generic window and pane hosting | `nativeui/window` manager plus feature-owned surface factories | The manager exposes IDs, descriptors, placement, focus and lifecycle only. It must not leak concrete state such as image preview zoom/bitmap state; the owning surface keeps typed state private. Durable workspace snapshots store portable descriptors, not renderer objects. |

`app` assembles feature modules and platform capabilities; command packages select the runtime surface. The native desktop command connects that module assembly to `nativeui`. A future CLI command connects it to terminal input/output. If launching a headless workflow requires constructing a MyGo window or importing `nativeui`, the composition boundary has drifted and must be corrected before adding the CLI.

For plugin readiness, prefer several narrow registries owned by their feature (for example search fields/operators/scorers, document-type metadata, and editor block capabilities). Validate duplicate IDs, dependencies and lifecycle ordering during assembly. These contracts are extension points for Muse modules first; they do not imply a dynamic plugin loader or a universal event/hook framework.

For collaboration readiness, keep source materialization, document identity, and revision semantics separate in the model. The current implementation deliberately identifies notes by normalized path and uses a disk revision for conflict detection. Do not retrofit a path hash as a stable ID or reinterpret a file revision as a CRDT clock. When collaboration becomes active work, add its identity and transaction/sync contract at the document/application boundary while preserving the file adapter's Markdown compatibility.

Headless and CLI readiness also constrains initialization: feature packages must not perform I/O or start goroutines from package initializers. `app` owns construction and shutdown; a command owns its signal handling and output loop, and may omit UI-only services entirely.

## Extension and lifecycle contracts

- `search` supports explicit typed field/operator/scorer registrations. Query parsing produces a validated AST; planning compiles it before execution. Metadata source belongs to `document`/`vault`; query semantics and ranking belong to `search`.
- `editor` block capabilities are explicit and typed. Block operations produce editor transactions; table selection/view state is scoped to its editor session.
- Registration IDs are unique and validated at application assembly. Do not add runtime plugin loading or a universal contribution framework until an actual third-party extension requirement exists.
- Domain event types are owned by the module that changes state (`document.DocumentSaved{Path, Revision}`, `workspace.WorkspaceRestored`). The application dispatches them to independent consumers; add a vault event only when a real independent subscriber needs one. Never publish per keystroke.
- Direct calls remain the default for a single workflow. Events are for independent subscribers; hooks are only for deliberate synchronous customization (e.g. a scorer or query operator).
- Coalesce editor changes by transaction/revision. Never broadcast per-frame layout, pointer movement, or every keystroke to a global bus.
- `app` owns process start/stop. Modules take contexts, cancel background work, and release subscriptions/resources when a vault, document session, or workspace closes.

## Completion and acceptance

The refactor is not complete merely because the code compiles. Before closing the goal:

1. Every implementation and test resides under its owning target module; old package directories/import namespaces are gone.
2. Module APIs and dependency direction match this document. UI panels use application/module APIs; no feature reaches through another feature's internals.
3. `nativeui.App` no longer owns document, index, search, or workspace domain state/rules that belong to feature modules.
4. Search, table editing, document save/recovery, workspace restore and Markdown render lifecycles have explicit owners and typed contracts.
5. No migration aliases, duplicate implementations, temporary package shells, stale architecture reference tree, or TODO-based transition path remains.
6. Focused package tests, full native tests, build, and architecture/import-boundary checks pass after all moves and wiring are complete.

## Architecture decisions during implementation

- 2026-11-16: GoRex is adopted as a behavioral reference for tab/pane focus and split-layout interactions, not as a package architecture to copy. Muse separates durable layout (`workspace`), generic live hosting (`nativeui/window`), and toolkit layout presentation (`nativeui/workspacepresenter`). A pane's note/editor composition belongs to `nativeui/surfaces/noteeditor`; recent pane focus used for runtime close fallback is session policy and is not serialized as durable document content.
- 2026-11-16: Refactor execution is layout-first. First relocate implementation and test files to the target package tree without changing package declarations, imports, APIs, or behavior; then perform package/API wiring against that completed tree. The layout-only intermediate may not compile, so do not use build success as its acceptance criterion. The completed wiring phase must remove all migration shells and satisfy the full completion gate above.

Do not weaken these criteria to close the goal. If any criterion remains open, keep the goal active and continue the refactor.
