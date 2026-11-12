# 页面排版与表格控件（2026-11-12）

当前在 main 工作树继续，原有未提交修改全部保留；本轮不提交、不切分支。

## 页面

- 暂停独立 title 展示与专用编辑入口，正文 H1 承担页面标题。已有 YAML title 原样保留，可通过原始 YAML 访问，不自动改写正文或文件名。
- 默认 16px 正文下，H1–H6 为 40 / 27.2 / 21.6 / 18.4 / 16.8 / 16px；H1/H2 使用 700 字重、1.25 行高，其余标题 600 字重。编辑器和预览共用同一套 typography。
- 页面顶部只呈现 metadata。Tag 使用更厚的圆角胶囊（上下 6px、左右 10px），其他字段使用低对比度中性灰；Tag 保留现有稳定配色。
- 隐藏 frontmatter 时，结束分隔符自己的换行不再形成额外空行；真实正文空行仍保留。
- 标题后的首个章节有独立间距。折叠后的隐藏源码不能转成巨大空白间距。

## 属性栏

- 白色/深色页面背景，大圆角灰色卡片，淡灰键名、较深值。字段的常驻置顶/隐藏/排序/删除按钮改为值上的右键菜单；属性栏菜单提供添加、YAML、关闭。
- `Type` 默认显示 Note，“编辑类型”通过现有 frontmatter 操作编辑 type，可原生撤销。
- Backlinks 使用当前库索引和实际来源文档，按库、页面、链接 revision 缓存；点击跳转。Tag 与编辑区共享胶囊样式。
- 日期来自 YAML 或实际文件信息。macOS 支持文件创建时间；没有创建时间的系统不伪造。最近打开时间来自本次实际打开事件。
- 次要日期、文件信息和用户隐藏字段收进“隐藏”。不显示 title、Created by、Last modified by、Links；底层数据不删除。

## 表格外观

- 表头不填灰底；浅色网格 `#ededed`、深色网格 `#363638`。编辑器/预览一致。
- 空表格及短内容表格填满正文列宽，保留内容驱动的列宽差异；行内留白略增。
- 加行/列是外侧长条灰色圆角胶囊、深灰加号。仅胶囊本身接受点击，网格边界与胶囊之间保留原生边界光标通道。
- 结构选区只负责淡填充、1.5px 轮廓和原生边界光标。把手及 3px 短亮条不读取结构选中状态：键盘 Visual 全表选中不显示一排把手。
- hover 只投影单元格暴露的边：第一列触发左条，第一行触发上条，左上角同时触发两条，内部单元格都不触发。使用排版后 rowY/colX 的包含关系；每帧读取原生指针坐标，因此换行高行、滚动且鼠标不动都不会误指下一行。
- 单元格右侧的竖向省略号打开所在列的对齐菜单，明确标注“此列”。沿用 Markdown 列对齐，保留光标/结构状态并支持 Undo/Redo。

## 把手手势与重排

- 点击把手选中行/列，Shift 点击仍可扩展范围；按住已选范围内的把手可以拖动整组。
- 把手拖动使用独立 tableHandleDrag 状态。文本/边界拖动继续使用原生结构选区路径，不经过重排状态；把手与边界光标的命中区域分开。
- 按住时其他 hover 控件冻结，只画当前轴的表格长度/高度滑轨。Thumb 限制在表格范围，移动超出阈值后用 4px 亮条预览插入位置。
- 移动期间不写源码、不扩大选区，松手一次性移动整行/整列；列的 alignment 跟随列。投到自身范围不产生编辑；Esc 或系统 pointercancel 取消。
- 原生历史保存重排前后的结构范围/活动端点，Undo/Redo 精确恢复；内容中的转义管道、inline markup、公式、双链和表格外正文保留。引用/列表容器保持有效。
- Markdown 的第一行始终是 header：把其他行移到首位时改变的是 header 内容，delimiter 仍在第二个源码行，不生成非法表格。

## 验收入口

- 页面、属性编辑、旧 title 数据保留：`native/nativeui/page_layout_test.go`、`properties_acceptance_test.go`。
- 表格 hover、换行、静止指针滚动、对齐菜单：`native/mdedit/table_chrome_test.go`。
- 行列原生重排、实际 Press/Move/Release、滑轨、取消、Visual 与 hover 解耦、边界文本选区通道、组移动及容器：`native/mdedit/table_reorder_test.go`。
- 原生渲染图：`/tmp/muse-page-layout/`、`/tmp/muse-page-tableshots/`。这些是 headless 原生 UI 证据，不等同于运行中桌面实例的手动验收。

最终检查：`sh scripts/go.sh test -C native ./... -count=1` 全包通过（mdedit 3.300s、nativeui 5.586s）；表格渲染/实际手势专项通过（1.034s）；`git diff --check` 与改动 Go 文件 gofmt 检查通过。`sh scripts/build.sh` 成功，`out/Muse` 91,485,058 bytes；缺少 Apple Development 身份，产物 unsigned。本轮全部工作仍未提交，原有 dirty 修改保留。

## 追加：块视口与横向滚动契约

- 中间列按住闪烁的根因是匿名 overlay 随其他控件隐藏而换位，原按压控件被框架移除、失去捕获。行列把手使用包含 Editor、table lineStart、axis、index 的稳定 key；三列和多表格持续按压均需覆盖。单元格菜单、加行/列控件也保持稳定身份。
- 块视口负责独立横移，源码、光标 byte offsets 与整页纵向滚动不作为横移状态。代码保留逻辑行，长公式保留可读大小；表格允许合理最小列宽形成溢出。普通段落保持现有换行。
- `table.x` 是稳定的 viewport origin，`geom.colX` 是内容坐标，`tableContentX = table.x - horizontal.x`。网格、文字、结构范围、菜单和拖动插入预览使用同一变换，再裁剪到 `blockViewport`；不能固定网格而单独移动文字。
- Table cell.x 已包含其表格横移，不能再次减去 parent offset。代码/公式等文本使用 `unitTextX` 统一绘制、caret、hit、矩形选区的坐标。键盘编辑可将活动 caret 横向滚入视口。
- 每个溢出块独立维护 offset 和 scrollui.Indicator。仅实际 offset 变化短暂显示滚动条；hover、初始绘制、到边缘的无效滚动不显示，停止滚动后自动隐藏。图表保留现有独立滚动实现及同一显示策略。
- 根 overlay 的表格按钮须向所属块/Editor 转交滚动。DX/Shift+DY 横移当前块，普通 DY 纵向滚正文；触控板同时给出 DX/DY 时不能吞掉纵向分量。滑块拖动不改源码、不破坏已有选区，不进入文本拖选或行列重排状态。

并行所有权：block_scroll_core 负责块状态、范围、共享几何与布局；table_scroll_view 负责绘制/裁剪和表格控件坐标；scroll_input_route 负责输入路由和滚动条指针手势。主 agent 负责接口契约、已有回归适配、独立验收与最终集成。此追加部分的最终检查另行记录，不能引用上轮检查冒充本轮已完成。

## 本轮收尾与独立验收

三个 worker 因使用额度限制中断后，主 agent 接管剩余集成、修复和验收。已有代码按上述模块边界保留；没有重新切分支、重置、提交或操作其他 worktree。

### 修复与状态轨迹

- 横移：`x=0 / bar hidden → 实际 DX 改变 x / bar visible → 停止 700ms / hidden`。hover 和到达边缘后的无效滚动不延长寿命。每块独立，表格网格、单元格、结构染色、caret/hit 和重排预览共用内容变换及视口裁剪。
- Overlay 滚动：单元格菜单、行列把手、加行列胶囊收到混合 `DX/DY` 后分别转交所属块和正文；5 类控件均通过真实 Tester.Scroll 的纵向与混合输入测试。在最右端的加列胶囊使用负 DX 验证向左滚动，避免将已到达边缘误报为吞事件。
- 中间列：`hover → 稳定 key 的 Press / 捕获原把手 → Move / 仅更新滑轨与插入位置 → Release / 一次源码重排`。覆盖所有三列、前面有另一张表格、横移后中间列的持续按压和重排。
- 编辑表格：保留原横移 → 重新测量 colX / 总宽度 → 依据新宽度钳制偏移 → 用同一偏移布局单元格。原 bug 在总宽度仍为零时先钳制，导致偏移丢失。纯代码文档编辑也保留横移；SetSource 打开/替换文档清零。
- Vim `j`：记录原几何 goalX 与目标显示行 → 命中下一行 → 落点展开语法并重排 → 固定目标显示行重新命中。候选落点若因语法再次折叠而退回上一行，恢复最后一个位于目标行的落点，防止逐次退回“这／是”。`k` 返回上一显示行，goalX 保留。此逻辑由普通方向键、Vim、Helix 共用的 nativeVertical 提供。

### 本轮文件边界

- 块状态与布局：`horizontal_scroll.go`、`editor.go`、`doc.go`、`layout.go`；共享 caret/hit 与矩形坐标：`nav.go`、`rectangle.go`。
- 绘制与表格控件：`paint.go`、`table_selection.go`、`table_selection_ui.go`、`table_edges.go`、`table_cell_menu.go`、`table_reorder.go`。
- 输入与自动滚入视口：`input.go`、`view.go`；共享纵向导航稳定：`native_commands.go`。
- 新增测试：`horizontal_scroll_core_test.go`、`control_scroll_test.go`、`table_scroll_view_test.go`、`horizontal_acceptance_test.go`、`vim_folded_vertical_test.go`；已有 hover/把手测试在 `table_chrome_test.go`、`table_reorder_test.go` 中扩充。

### 最终检查（本轮新增代码之后）

- `sh scripts/go.sh test -C native ./... -count=1` 全部通过：mdedit 3.534s、nativeui 5.119s，其他包通过。
- 主 agent 独立验收专项通过：整表网格/文字同移、clip 像素边界、caret/hit 往返、浅深色代码/公式视口、三列稳定捕获，以及 16 种宽度 × “这／是”两起点的 `j/k` 回归；原有 inline-code 矩形选区的 `co\nco` 也通过。
- 原生渲染图位于 `/tmp/muse-horizontal-acceptance/`，主 agent 已检查表格、拖动插入预览以及浅深色代码/公式图。证据来自 headless 原生 UI，未替换或重启用户正在运行的桌面实例。
- 改动 Go 文件 gofmt、`git diff --check` 通过；`sh scripts/build.sh` 成功生成 `out/Muse`。当前机器没有 Apple Development 签名身份，产物 unsigned。
- 测试日志：`/tmp/muse-final-native-tests.log`、`/tmp/muse-horizontal-root-acceptance.log`；构建日志：`/tmp/muse-final-build.log`。当前仍在 `main`，全部未提交修改保留。

## 追加：惯性滚动中按住鼠标触发页面持续滚动

- 根因：`autoScroll` 使用最后一次 PointerMove 缓存的文档坐标。鼠标不动时的滚动不会刷新该坐标；随后 Press 启用 dragging，旧坐标被误判在视口外，从而不断自动滚动、扩大选区。回归中向下滚至 1240 后按住不动，修复前可自行滚回 0；向上方向同样存在。
- 修复限定在输入和视口模块：`input.go` 的 Press 初始化当前指针；`view.go` 在拖选期间每帧从 Scroll 视口读取实际 PointerPosition，再依据当前 scroll.Y 转为文档坐标。保留真实拖出上下边界时的自动滚动，不吞掉后续触控板滚动。
- 新增 `drag_scroll_test.go`：上下方向的静止按压与残余滚动、原生 Wheel 尚在 glide 时按压、真正越界拖选及松手停止，共 6 个场景通过。
- 全包 `sh scripts/go.sh test -C native ./... -count=1` 通过（mdedit 3.998s、nativeui 5.597s），gofmt 与 diff 检查通过；重新构建 `out/Muse` 成功，仍为 unsigned。日志：`/tmp/muse-drag-scroll-before.log`、`/tmp/muse-drag-scroll-after.log`、`/tmp/muse-drag-scroll-native-tests.log`、`/tmp/muse-drag-scroll-build.log`。
