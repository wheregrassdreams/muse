# 编辑器选择、导航与源码展开修整

日期：2026-11-12。表格基线：`2198f1a`，已合并本地 main。新工作保留为未提交改动。

## 共同约束

- Markdown 源码仍是唯一文档状态；文本字节范围、结构选区、布局坐标各司其职。
- 默认键位、Vim、Helix 复用原生导航、编辑、选区、复制和历史接口。模态层只负责按键解析、模式、计数和操作组合。
- 不破坏已合并的表格：结构染色与文本染色互斥，表格不因选中自动展开，边界可离开，鼠标松手不改变端点。
- 有意义的回归先复现再修复；所有新增源码测试不得依赖系统剪贴板或修改用户设置。
- 三名 worker_clean 并行，不切分支、不提交、不还原其他人的改动。公共文件按函数分工，修改前互相告知；主 agent 最后统一格式化和验收。

## A. 行内代码样式（worker: inline_style）

问题：普通段落跨越 inline code 的文本选区存在背景断裂，代码 pill 背景浮在 selection 之上；inline math 可作绘制顺序参照。

验收：

1. 正向、反向、部分和整体选择跨过 code 时，selection 连续覆盖内容及代码的横向留白，代码背景不得盖住选区。
2. 折叠、展开、换行、中英混排、深浅主题均验证；文字保持可读，不靠移除代码本身样式规避。
3. 解释既有 inline math/code 样式测试失败并修复真实实现；不得删除或弱化断言。
4. 新增截图与真实选区几何/像素断言，不能仅比 SelectedText。

所有权：`display.go` 的 code 样式/inline code builder 函数、`paint.go` 的 inline 背景/selection 绘制、`layout.go` 的 look/span code 样式部分，及新增 `inline_code_selection_test.go`。不改 list/container marker 的 builder 段和 reveal 策略；同文件局部 patch，不整文件重写。需要改其他模块先通知主 agent。

## B. 几何导航与 Vim（worker: vim_navigation）

问题：软换行长段落中的 Vim j/k 未正确追随屏幕几何列；Shift+V 当前只有模式标志，没有可靠的全行范围。需补常用能力并记录缺口。

验收：

1. 默认方向键及 Vim j/k 共用几何导航，持续保持目标 X；短行夹在长行之间也能恢复原目标列。水平移动、点击和编辑后目标列重置。选中/展开导致重排时使用实际布局，不沿用错误字符列。
2. Shift+V 立即显示完整源码逻辑行的原生选区（含适当换行），j/k、计数、反向收缩、Esc、y/d/c、p、undo/redo 一致；软换行显示行与源码行区别明确。
3. 先盘点已有键位，再实现至少 4 项常用缺口（优先 ^、I/A/O、x/X、D/C、Ctrl+r、g j/g k 等），选择能复用原生接口且语义完整的项目。不要为了数量做空壳或截断 Vim 行为。多字符 IME 继续不得解析为命令。
4. 新增原生导航命令层或复用现有方法，模态层不保有第二套源范围/几何/历史系统；Helix 既有能力不退化。
5. 输出已实现/未实现矩阵，测试新增能力、计数、操作组合、编辑撤销与输入隔离。

所有权：`modal.go`、`nav.go` 的几何导航函数（不改 revealFor/stopOK 的展开策略）、新增原生命令文件及 Vim/导航测试，`docs/vim-support.md`。需要 Editor 字段或 input.go 公共入口先通知主 agent，不覆盖其他模块。

## C. 列表标记与统一展开（worker: source_reveal）

问题：无序、有序、嵌套列表和 task 的光标进不了左侧源码符号。期望靠近标记时，checkbox / bullet / 数字等展开为可编辑 Markdown，例如 task 的 `- [ ] abc`。

验收：

1. 鼠标与键盘能从行首内容进入列表/task 前缀；靠近时展开，离开后收起。包含缩进、嵌套、多位数字、有/无勾选，保持 source 字节与布局 hit/caret 一致。
2. 所有可编辑语法元素在被非空选区覆盖时默认展开源码：行内粗斜体、删除线、链接/双链、code/math、列表/任务、标题/引用/代码块等按既有 parser 支持范围实现统一规则。明确列出特殊豁免，至少表格保留结构呈现；图片/图形等保持既有显式控制时需说明理由。
3. 正反选区、选区撤销/清除、跨段落选择和鼠标松手不跳行；不因为源码展开而重写源文档或扩大选择范围。
4. 在现有设置页增加“源码展开偏好”说明/占位，明确暂不可自定义。不增加可操作但无效的开关，不存储尚未生效的选项。
5. 新增列表前缀导航与 selection reveal 的集成测试和截图。

所有权：`nav.go` 的 revealFor 与必要 marker stop 策略、`layout.go` 的 selection/reveal 缓存和容器标记布局、`display.go` 的 revealed/selection 判定与 list/container marker builder（不改 inline code 样式函数），必要 `parsed.go`，`native/nativeui/editor_settings.go` 和设置占位/对应测试。使用新增 source_reveal 文件组织规则。公共 editor.go 字段修改先通知主 agent。

## 主 agent 验收

- 审查公共文件合流及原生接口复用，独立复跑新增回归、完整 `mdedit` 和相关 `nativeui/mdoc` 测试。
- 统一 gofmt、diff 检查、`sh scripts/build.sh`；核对截图（不把静态测试当作用户运行中 app 的验收）。
- 按功能报告结果与尚未支持的 Vim 能力，注明构建签名及测试限制。

## D. 追加：标题 Fold

- Markdown 标题支持点击左侧 chevron 折叠/展开；展开箭头向下，折叠向右，折叠后标题末尾显示省略提示。
- 范围按标题层级：隐藏直到下一个同级或更高层级标题，包含下级标题及正文，保留标题本身。覆盖 ATX/Setext、嵌套、首尾、没有可折内容的标题。
- Fold 仅为视图状态，源码、复制范围、保存和 undo 文档内容不能被截断。折叠是显式操作，不因为标题被普通选中就立即自动打开；用户显式跳转/查找定位隐藏内容时必须展开其父折叠，防止不可见光标。
- 默认/Vim 导航均跳过折叠正文；鼠标命中、滚动高度、虚拟布局和折叠后的选择须与可见布局一致。更换文档不泄漏折叠状态，源码编辑后无效范围安全失效，不折错段落。
- 新增独立 folding.go 和测试组织状态/范围，尽量不把折叠解析塞进模态层。至少验证嵌套、边界、导航、跳转、source不变、hit几何和截图。

## E. 追加：链接视觉状态（worker: inline_style 后续）

- 外链文字带下划线与小型外链标识（参考用户截图），图标不写回源码，不污染 SelectedText、复制、caret字节映射；内链不显示外链标识。图标须可随周围文字排版，不孤悬。
- 有效双链使用正常链接样式；不存在目标的双链降低对比度并以不同下划线（虚线）区分，避免与正文或有效链接混淆。不要把未知 resolver 状态当作目标不存在。
- hover 时仅当前链接提亮，移出恢复，深浅主题可读；不改变文本几何或源码，保留既有 Cmd/Ctrl-click 打开交互。
- 链接有效性从 app/vault 现有解析器或索引通过 Editor Options 回调传入；不在渲染层另外做文件扫描。文件创建/删除/重命名后可更新样式，布局缓存不让过期状态永久滞留。
- 负责 display.go 的 link/wiki 绘制函数、新增 link_style 文件及测试、Options 的 resolver 回调（需告知主 agent）、nativeui editorOptions 现有 resolver 的绑定；与 source_reveal worker 协调同文件改动。原有 code 选择背景修复必须保留。

### E 实现约定

- `ResolveWikiLink` 使用 `Unknown / Valid / Invalid` 三态；没有 resolver 或 vault index 尚未就绪时返回 `Unknown`，按普通链接绘制。
- 样式查询只调用内存中的 `vaultindex.Index.Resolve`，不在布局或绘制中访问磁盘。索引刷新增加 `LinkRevision`；当前 vault 与文档路径进入 `LinkContext`，两者都参与视图缓存键。切换文档时重置 hover。
- hover 按可见链接几何与源码 range 定位，只改变命中的链接颜色；移动到另一链接或移出链接会刷新对应行视图。hover 查询恢复软换行 caret affinity，不改变 selection、caret 或源码。
- `http://` 和 `https://` 链接在折叠 label 视图显示紧凑 `↗` 装饰；相对 Markdown 路径、heading anchor、mailto 和 wiki link 不显示。装饰是零源码宽度的 display decoration；链接展开为 Markdown 源码时隐藏。

## 实施与独立验收结果

- A/B/C/D/E 均已实现，Visual 追加问题通过原生 `selectionInclusive / selectionLinewise` 范围语义修复，绘制、复制和删除共用有效选区，caret 保留实际焦点；Unicode grapheme 边界按源码 revision 缓存。默认输入与模态适配器共用 native commands。
- 主 agent 补充使用用户 playground 原段落的源码展开/几何导航、真实 Visual 表格往返、正反 CJK/emoji 选择复制删除、外链窄宽换行和虚拟大文档折叠验收。
- `go test ./mdedit -count=1` 与 `sh scripts/go.sh test -C native ./... -count=1` 全部通过，包括 nativeui 的 resolver/设置测试；此前 5 个样式失败也已解决。代码背景旧 span 断言迁移为共享背景盒与实际像素检查，保留等宽、颜色和覆盖范围验证。
- `sh scripts/build.sh` 成功，当前没有 Apple Development 签名身份，产物 unsigned。headless UI 截图已核对；未把构建或截图等同于运行中用户 app 的验收。
- 本轮新增改动保留为 main 工作树未提交内容；表格基线已合并为 `2198f1a`，未推送远端。
- 当前明确限制：标题 fold 是临时视图状态，源码编辑、Undo/Redo 和 SetSource 时清空，尚不持久化；展开偏好仅有说明占位；Vim 未实现项见 `vim-support.md`。
