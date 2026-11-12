# 表格结构选择交接记录

更新日期：2026-11-11  
工作分支：`polish/table-structure-selection`  
当前基线提交：`ecc5b91a808f45d2a8b4663bd3bac81af421f604`（此分支 HEAD 与 `main` 当前指向同一提交；表格工作尚未提交）

2026-11-12 合并前更新：下方为最初交接时的历史记录。两条水平选择回归现已修复，文本端点与结构范围分离，导航用显式转换记录回放；跨段落选区按文本/表格结构分段，整行复制包含有效表头与对齐分隔行，结构边界可上下离开表格。鼠标松手保留结构边界端点，不再归一化到下一行。新增回归均通过；全包仍有 5 个既有代码/数学样式失败，留给后续样式任务处理。跨段落键盘扩选目前按整表跨越，逐行扩选是已提出的后续交互方向。

2026-11-12 表格 Vim Visual 行列模式：表格单元格内按 `V` 立即选择当前完整结构行；`j/k` 和计数逐个表格数据/表头行扩缩并可反向，Markdown 对齐分隔行不作为可视行。行结构活动时文本 inclusive/linewise flags 关闭，行范围和可视 focus 由结构状态派生。`h/l` 只切换结构行焦点的左右边缘，不降级为单元格文本；`w/b/e/^/$` 的源 motion 更新活动行，跨行时按原锚点扩缩行结构。离开表格边界会进入相邻文档行的 linewise 混合选区；反向会先恢复表格结构范围，再逐行收缩。行 `y` 使用含有效表头和分隔行的 table register；`d/c` 沿现有结构编辑与历史路径处理。

表格单元格内按 `Ctrl+V` 立即选择当前结构列，并以当前单元格所在行为活动 focus；`j/k` 在保持列范围时移动活动行。`h/l` 保留既有“结构选择、单元格内容 bridge、相邻列结构”的逐步可逆路径，bridge 使用当前活动行。越过列选择的上下表格边界时，结构选择收起、模式回到 Normal，并把焦点交给相邻可见文档叶片，不扩大为普通连续文字选区。列 `y/d/c/p` 继续使用既有 table register、删除、粘贴和多单元格输入路径；表外 `Ctrl+V` 仍由普通文字矩形选择处理。

这份文档交给继续处理 Muse 表格选择问题的工程师或 agent。目标是让接手者知道当前实现、用户确认过的交互语义、已知 bug、复现/验收缺口和安全的接手方式。当前工作树含有尚未提交的代码，接手时必须在这个工作区和分支上继续。

## 1. 当前结论

表格选择问题不是单个光标边界 bug，而是选择状态模型不完整。代码里已经有 `tableSelection` 和 `tableBridge`，但普通文本选择仍存放在 `Editor.anchor/caret`；结构选择建立和收缩时又会改写这两个文本端点。`returnTo` 快照只补偿局部转换，结构选择跨多行扩张、收缩时没有稳定保留整条路径。

用户最近确认：之前“从表格右下角出去跳回左上角”的跳转应已消失；目前仍有两类选择状态错误：

1. 第四行原本只部分选中的文本，经过第三行结构选择后再回到内容选择，第四行变成整行内容都被选中。
2. 从第三行开始向左，结构选择扩展到第三、第二、第一行；再向右返回，到第二行附近第三行的选区又重新开始，不能按原路径逆回去。

这些现象与代码中“结构范围覆盖文本 anchor/caret、桥接时再按整行重建文本范围”的机制吻合。**结构选择与文本选择应保存为分离的状态；目前还没有按这个方向完成重构。**

## 2. 用户已经对齐的交互语义

- 表格行/列的内容选择与结构选择是两种不同状态，触发条件、光标位置和绘制样式都要区分。
- 结构选择要求光标/选区到达该行或列的最外边界。Visual 结构选择应将整行/列作为整体染色，单元格内部不要再叠加普通文本 selection 样式。
- 选择内容时可以跨过单元格和行；跨过边界后整体从内容选择切为结构选择，反向移动时也要整体退回内容选择，不能让同一时刻的结构染色和单元格文本选区互相叠加。
- 结构是选择操作中的语义单位。向上/下移动结构边缘光标应能进入相邻结构或离开表格；不能卡死在表格底边，也不能把表格后面的段落染成选中。
- 水平 h/l 不应把表格行列当作连续环形数组；跨越内容与结构时要走明确的内容桥接状态，不应从右下角绕到左上角。
- Visual 模式下按 Esc 应回 Normal 并清除 selection。
- 结构选择的破坏性编辑需要保持 Markdown 可恢复。如果操作会留下不完整表格（例如删除表头或 c/change 留空行），应回退成可见的 Markdown plain text；普通 d/dd 删除后可由后续行接续的行为需要与 c/change 区分。
- 列结构选择经 Ctrl+V / BlockVisual，从列的上下边界进入；行结构由行边界进入。触发方向、anchor/active 和可视光标位置要能逆向恢复。
- 行/列结构 selection 的背景应覆盖结构整体，边缘光标保持一致、细而可见；不能只在最后一行/列显示边界。

## 3. 现有实现地图

### 状态字段

- [`editor.go`](../native/mdedit/editor.go)：`Editor.anchor/caret` 是原本的文本选择端点；另有 `tableSelection`、`tableBridge`、`tableSelectDrag`。
- [`table_selection.go`](../native/mdedit/table_selection.go)：结构选择、内容桥接、行/列边界转换、表格来源范围和编辑行为的主要逻辑。
- [`input.go`](../native/mdedit/input.go)：方向键和 Shift+方向键入口；普通方向键调用 `tableHorizontalTransition`。
- [`modal.go`](../native/mdedit/modal.go)：Vim/Helix 的 h/j/k/l、Visual/BlockVisual 状态、Escape 和结构操作入口。
- [`nav.go`](../native/mdedit/nav.go)：结构边界命中、结构选择创建、光标/selection 导航等。
- [`paint.go`](../native/mdedit/paint.go)：文本 selection、整行/整列结构染色和边界光标绘制。
- [`table_selection_ui.go`](../native/mdedit/table_selection_ui.go)：结构选择相关 UI 状态/交互辅助。
- [`table_edges.go`](../native/mdedit/table_edges.go)：表格边缘控件。
- [`edit.go`](../native/mdedit/edit.go)、[`history.go`](../native/mdedit/history.go)：结构编辑与 undo/redo 状态维护。

### 当前主要结构

`tableSelection` 保存表格位置、axis、first/last、anchor/active、activeRow、边缘方向 `right`，以及局部快照 `returnTo`。

`tableContentBridge` 保存当前跨越的表格行/列、焦点单元格、方向、恢复模式、索引与 `selectionAnchor`，并可以再挂一个 `returnTo`。

`tableSelectionReturn` 保存模式、`anchor/caret`、bridge 光标偏移、可选 bridge/selection 指针和方向。它被用于将一次结构晋升或降级反向恢复。

这是一种“主选择端点 + 结构覆盖 + bridge + 嵌套 returnTo”的混合模型，而不是完全分离的文本选择和结构选择。

## 4. 为什么状态会错位

代码路径审查确认了这些机制：

1. [`selectTableAxis`](../native/mdedit/table_selection.go) 创建结构选择后，会通过 `tableSelectionSourceRange` 将整行/整列的 Markdown 源范围写回 `e.anchor/e.caret`。这两个字段原本承担文本选区 anchor/head，因此旧文本端点被结构范围覆盖。
2. [`structuralSelectionForRange`](../native/mdedit/table_selection.go) 从普通范围识别结构选择后，会把端点规范到行范围。结构状态因此反向改写普通选择端点。
3. [`tableSelectionContentRange`](../native/mdedit/table_selection.go) 从结构切到内容 bridge 时，按所选结构范围计算整行/整列内容跨度。它没有一个独立、持久的文本选区状态可恢复，因此内容范围可能包含原来未完整选中的第四行。
4. `returnTo` 记录局部转换快照；结构对象被重新创建或 active/方向改变时，前一段路径并未作为一个完整、明确的状态机历史保存。单次 h/l 反转可以通过，但连续跨多个结构再返回不保证是逐步逆过程。

第 1–3 点是源码直接确认的机制。它们与用户两种现象高度吻合。第二个现象的具体按键状态轨迹还没有用自动化回归逐步记录；下一步应先按用户路径精确构造测试，再实施状态模型调整。

## 5. 推荐的状态模型方向

不要只把 `tableSelection` 指针从 `Editor` 移到另一个字段，而保留 `anchor/caret` 继续兼作结构范围。应让选择状态有清晰的所有权：

```text
selectionState = TextSelection(anchor, head, mode)
              | TableStructure(axis, anchorIndex, activeIndex,
                               activeRow/cell, boundarySide, mode)
```

- TextSelection 保留精确的源字节 anchor/head；进入结构选择后作为 dormant/source 状态保存，不被整行范围覆盖。
- TableStructure 只拥有行/列索引、结构锚点/活动端、边界侧和可视焦点；其源范围由结构字段派生，用于绘制、复制、删除等操作，不写回文本选择端点。
- 内容 bridge 是一段有明确定义的导航状态：记录其进入前的结构端点、当前文本范围/焦点、方向，以及反向一步应恢复的状态。不要让 bridge 通过“当前结构范围内的全部文本”推测原选择。
- h/l 每一步应有确定的状态转换。对每个合法状态 `S`，做一步 `h` 再反向一步 `l`（或相反）应恢复完整状态快照；多步路径应满足逐步回放逆序，而不是只测一次 promotion。
- 渲染层读取唯一的 active selection variant：文本 selection 样式或结构整行/列染色二选一。结构范围是派生值，不作为第二套同时生效的 selection。

这回答了“结构选择和文本选择分别管各自状态行不行”：**可以，而且当前错误表明应该这样做。** 需要同时统一状态转换和渲染/编辑读取入口，否则只是加字段仍会留下两套互相覆盖的真相。

## 6. 自动化测试现状与固定复现

测试在 [`table_selection_test.go`](../native/mdedit/table_selection_test.go)。当前包含：

- `TestRowVisualSelectionReversesAcrossContentBridge`：从一行向右扩展到两行，再反向穿过 bridge 并恢复两行；覆盖部分行边界往返，但不覆盖三行扩展后完整逆行，也不以未完整选中的第四行作为文本起点。
- `TestVisualContentSelectionSurvivesStructuralPromotionRoundTrip`：普通部分文本 selection 晋升结构后，按相同步数反向，比较完整 snapshot；是单次方向路径。
- `TestHorizontalPromotionReversesExactlyOneStep`、`TestHorizontalColumnPromotionReversesExactlyOneStep`：检查单次 bridge/结构 promotion 的精确逆步。
- `TestRowStructureLeftFromHeaderAndFirstBodyRowRecordsFocusPath`：首行/表头附近的方向及状态快照。
- `TestRowRightBoundaryLeftMotionStaysOnCurrentRow`：防止一行结构向左跳至前一行。
- `TestVisualMotionOutwardFromBottomRightTableSelectionDoesNotWrap`：检查底右越界不回绕到左上角；当前 `l` 在结构边界会被消费并停住，`j` 可进入后续段落。
- 另有列选择、Esc、边界光标、行列删除与插入相关用例。

本次接手前验证记录：

```bash
cd native
go test ./mdedit -run '^(TestRowVisualSelectionReversesAcrossContentBridge|TestVisualContentSelectionSurvivesStructuralPromotionRoundTrip|TestHorizontalPromotionReversesExactlyOneStep|TestRowStructureLeftFromHeaderAndFirstBodyRowRecordsFocusPath)$' -count=1 -v
```

通过。另一个更宽的筛选组也通过：

```bash
go test ./mdedit -run 'Test.*Table|Test.*StructuralRow|TestHorizontalStructure|TestRowStructure|TestHorizontalPromotion|TestVisualContentSelection' -count=1
```

这些 PASS 只说明已有路径通过，**不覆盖 §1 两个最新 bug**。修复接手时先将它们变成失败测试，再改状态模型：

### 路径 A：不完整文本选区必须精确保留

1. 用至少四个 body row 的表格，第四行含有长文本。
2. 在第四行内容中间建立一个不覆盖整行的 Visual 文本选区，保存完整状态 snapshot。
3. 按固定方向跨入第三行，并将选区推进到第三行结构边界，使其触发结构选择。
4. 按相反方向退回内容选择。
5. 断言原始第四行文本端点/内容没有扩大；分清预期的当前文本范围与 dormant 的原始文本选区。

### 路径 B：三行结构选择逐步可逆

1. 从第三行选择的已知起始状态开始。
2. 按 `h` 直到结构范围覆盖第三、第二、第一行，记录每一步的 mode、文本端点、结构 first/last、anchor/active、activeRow、边缘、bridge 行列和选中内容。
3. 原样按相反方向 `l` 回放同样步数。
4. 在每一步断言应逆到前一个完整状态；尤其检查到第二行时第三行不应被重置/重新开始选择。
5. 对称测试：从第三行向右扩张再向左回放；首行、末行、表头、列结构及底右边界也要覆盖。

测试不要只比较 `SelectedText()` 或 `first/last`；比较完整语义状态及 caret 所在行/列，并断言文本 selection 与结构 selection 不会同时绘制。

## 7. 工作区和保护事项

当前 `git status`：

```text
## polish/table-structure-selection
 M native/mdedit/edit.go
 M native/mdedit/editor.go
 M native/mdedit/history.go
 M native/mdedit/input.go
 M native/mdedit/modal.go
 M native/mdedit/nav.go
 M native/mdedit/paint.go
 M native/mdedit/table_edges.go
?? native/mdedit/table_selection.go
?? native/mdedit/table_selection_test.go
?? native/mdedit/table_selection_ui.go
```

上述所有源码更改都是当前表格结构选择工作的一部分，未提交。不要 `git reset`、切分支、清理未跟踪文件、从 `main` 重建工作区或覆盖这些文件。仓库还登记了另一个 detached worktree `/private/tmp/main-check`，不要操作或清理它。

仓库中没有发现适用的 `AGENTS.md`。当前工作树已有修改；开始接手时重新检查 `pwd`、`git status --short --branch`、`git worktree list --porcelain`，以现场结果为准。

## 8. 建议的接手顺序

1. 先读本文件 §2、§4、§5 和 `native/mdedit/table_selection.go`、`editor.go`、`input.go`、`modal.go`、`nav.go`、`paint.go`。
2. 检查工作区，不重置已有改动。
3. 按 §6 两条路径补最小化失败回归测试，输出逐步 snapshot；如果用户提供真实按键序列，固定成表驱动测试。
4. 设计并实现独立 text/structure selection state；避免 `e.anchor/caret` 同时作为两种状态的事实来源。
5. 把结构 selection 的范围查询、绘制、Copy/Delete/Change、Esc、undo/redo 一起迁移到新模型，避免只修 h/l。
6. 运行新增回归用例、既有表格选择测试，再运行 `go test ./mdedit` 和 `sh scripts/build.sh`；截图核对结构染色、内容高亮和边界光标。构建签名身份缺失时产物会 unsigned，需要如实记录。
7. 协调者独立复跑固定路径；确认通过前不要宣称修复完成或提交。

## 9. 如何让另一个 agent 接手

在同一 Muse 工作区启动新 agent，并把本文件作为首个阅读目标。可直接把下面这段交给它：

> 请先阅读 `docs/table-selection-handover.md`。在当前共享工作区和 `polish/table-structure-selection` 分支继续，保留所有已有 dirty changes，不要 reset、切分支、清理文件或提交。先复现并新增两个回归测试：第四行部分文本选区经过第三行结构选择再返回不得扩大；第三行向左扩成三行结构后向右逐步回放必须逆回原状态。然后根据文档 §5 重构文本/结构选择的状态所有权，不要用局部 h/l 特判掩盖问题。完成后报告逐步状态轨迹、变更文件、测试命令和结果；由协调者独立验收。

如果用 Codex 的 agent/worker 功能，在当前线程让主 agent 负责复现和最终验收，把上面这段作为 worker 任务说明。若新 agent 不共享此工作区，先确认它拿到的是包含这些未提交改动的工作树；不要只把当前 HEAD 当成完整任务基线，因为表格实现尚未提交。
