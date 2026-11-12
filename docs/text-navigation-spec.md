# 导航与块选择：本轮实现契约

日期：2026-11-12。沿用 main 工作树全部未提交改动，不 reset、切分支、提交或推送。

## 1. Fold 控件

- 可折标题仅在鼠标位于该标题区域（包括左侧 chevron 通道）时显示控件，移出隐藏；折叠态也遵守此规则，省略提示保留。
- 图标以实际标题首条显示行的垂直中心对齐，不能沿用正文大小算固定 Y；覆盖 H1-H6、Setext、展开源码和换行标题。
- 显隐不影响排版、光标、选区和滚动。标题到控件的路径连续可达，不因移向控件就消失；隐藏标题不得有残留命中区域。

## 2. 原生矩形选择

- Ctrl+V 真正建立可视矩形选择，h/j/k/l 与计数扩张、反向收缩、Esc 清除。采用原生显示行与几何列，支持 CJK/英文比例字体、软换行和短行，不能用单一连续源范围伪装矩形。
- 在编辑器核心提供统一的多范围查询，范围使用半开 UTF-8 byte offsets；绘制、SelectedText、Copy/Cut/Delete/Change 读取同一组范围。Vim 层只解析按键与设置模式。
- y 按矩形各行复制；d 不删除行间换行；c、I/A 支持逐行输入与撤销；矩形粘贴可回到相应行列，不把多行块拼成一条普通文本替换。剪贴板/寄存器块元数据不能污染实际文本。
- 编辑通过现有 begin/replace/commit，以降序范围操作并构成原生历史步骤；离开模式/新文档/显式普通选择不会残留块状态。
- 保留表格结构行列选择与其 Ctrl+V 行为；保留字符 Visual 双端包含、linewise V 实际焦点、fold 跳过隐藏内容、IME guards 和已有普通文本编辑。
- 新增有意义回归：实际 Ctrl+V 事件、双向矩形/空短行/CJK/软换行、逐行背景、copy/d/c/I/A/p、history、退出与表格兼容。不依赖系统剪贴板。
- 块的行单位采用原生显示行；软换行会成为独立块行，Copy 用换行符分隔显示行。短行与空行保留零宽范围，参与 Copy 行数和 I/A/c 多点输入；Delete 跳过零宽范围，绝不吞行间换行。块 clipboard 在普通 Text/Markdown/HTML 表示外附加 `application/x-muse-rectangle+json` 形状数据；纯文本 Paste 忽略形状。表格作为原生结构边界，Ctrl+V 在表格 cell 中进入结构列选择，普通块编辑不修改表格 cell。
- 用户已确认表格 Ctrl+V 使用整列结构，沿用此前约定。V 从当前完整结构行开始，j/k 按渲染行增减，反向逐行还原；h/l 只改变活动边界侧，词/行首尾移动仍保持完整行。越出表格后继续文档 linewise 选区，反向回表格恢复结构。Ctrl+V 的 h/l 保留既有“结构 → 内容过渡 → 相邻列结构”的逐步可逆路径，j/k 只移动 activeRow；越过表格上下边界时结束选区并以 Normal 模式进入相邻可见正文。
- 主 agent 独立验收必须覆盖分开输入的多次 j/k 穿过短行并往返，不能只用同一事件的 3j 代替；还要检查 inline code 展开后的几何列、选区首行顶部像素和部分 code 选区未选中字符的像素。

## 3. 通用文本分词与目标查询

- 新建不依赖 UI/Editor/Vim 的 `native/textnav` 包。输入纯 UTF-8 文本，输出有序、不重叠、源字符串字节坐标的 tokens/boundaries；区分空白、标点、词/数字，尊重组合字符与 ZWJ grapheme。
- 中文采用真实词典/统计分词，不能把每个汉字当词，也不能仅维护若干测试词的硬编码词表。默认离线工作，词典打包可恢复；可替换 segmenter provider，为未来用户词典/其他语言保留清晰入口。优先评估纯 Go、内嵌词典的 gse，固定版本并记录运行代价与许可来源。
- 提供不含按键语义的词首/词尾/前后边界查询。Editor adapter 统一处理 UTF-8 byte 与显示层 UTF-16 的映射，不把两种 offset 混用。
- `w/b/e` 和 operator 组合/计数消费该 API；至少第二个真实消费者（默认 Option/Ctrl word movement/delete 或鼠标双击选词）也复用。源文本与选区不能因分词被重写。
- 通过区域、方向和目标类型过滤的候选 target 查询，为 easymotion/flash 消费词首/词尾等候选保留入口；本轮不做 label overlay、Flash 模式或虚假可用开关。
- 按文本 revision 或显示字符串缓存索引，不能每帧/每次 selection 查询重新载词典或全量分词；默认词典只初始化一次。说明歧义策略、失败回退、启动/内存/二进制代价，做代表性测试/benchmark。
- 回归覆盖真实中文词、中英混排、标点/空白、数字/标识符、emoji/组合字符、Markdown 显示到源码映射、`w b e` 往返/计数/操作范围、第二消费者、候选查询区域过滤与缓存失效。

## 并行所有权

- fold worker：folding.go 的 chevron 区域/hover 状态，paint.go 的 paintFoldChevron，input.go 的 hover fold 局部，folding UI tests；不改正文 range 绘制。
- rectangle worker：原生多范围选择/多点编辑新文件，Editor 块字段与核心 Copy/Delete/Change/历史/显示入口，modal.go Ctrl+V 和块操作分支（不改 w/b/e 分支），paintUnit 的多范围绘制与缓存，块测试。
- tokenizer worker：textnav 包及 provider/dictionary 依赖、go.mod/go.sum、nav.go 的 wordStepIn/wordRangeAt 等词导航、modal.go 仅 w/b/e 分支与原生命令词导航新方法、分词测试和架构文档。Editor 的词索引缓存字段按局部增量协调。
- 所有 worker 都不独占工作树；不得回滚他人改动。共享文件用函数级 patch，保持工作树尽可能可编译，统一格式化与最终全包验收由主 agent 负责。

## 主 agent 独立验收（2026-11-12）

- 当前工作树为 `main`，HEAD `2198f1a`，ahead origin 3；本轮全部修改仍未提交，保留原有 dirty 内容和 `/private/tmp/main-check`。
- 表格 V：body row 1 → rows 1–2 → rows 1–3 → rows 1–2 → row 1，随后再次扩张；每步保持结构态。Ctrl+V：col 1、activeRow 1 → j 到 activeRow 2 → 经内容 bridge 扩张到 cols 1–2 → h 按相同步数精确回放每个完整 snapshot，最终恢复 col 1/activeRow 2；Copy HTML 为 table，d/Undo 恢复源码、范围、模式。
- 文字块：`abcdef/x/abcdef` 从 c 开始，分开输入 l、j、j 得到 `cd/空/cd`，k、k 恢复 `cd`；code 的 `co/co` 复制范围和第二行 o 上的实际 caret 一致。像素回归比较原生选区背景与未选中 code 基准，保留原透明度。
- 中文 `w/b/e/2e/de/ce/dw/2dw`、原生选词、UTF-16/UTF-8、词缓存及零源外链装饰已验收；跳过装饰后紧邻的真实标点仍是合法词目标。
- `sh scripts/go.sh test -C native ./... -count=1`：全 native 包 PASS；mdedit 3.175s、nativeui 6.568s。
- `sh scripts/go.sh test -C native -race ./textnav -count=2 -shuffle=on`：PASS，3.241s。原生渲染专项 `TestNavigationAcceptance`、Fold hover 和矩形截图回归：PASS。
- `sh scripts/build.sh`：PASS；`out/Muse` 为 91,433,746 bytes，较本轮构建前 58,218,594 bytes 增加约 33.2 MB。缺少 Apple Development 身份，产物 unsigned。
- `git diff --check`：PASS。独立渲染图在 `/tmp/muse-navigation-review/`；主 agent 已检查 Fold hover 显隐、inline code 矩形和空短行矩形。证据来自原生 headless UI harness，未将其表述为运行中桌面实例的手动验收。
- 核心文件：`native/textnav/`、`native/mdedit/{native_commands,rectangle,multirange,table_visual_modes,folding,nav,modal,paint}.go`；主验收测试为 `navigation_acceptance_test.go`。分词机制和代价见 [text-navigation-architecture.md](text-navigation-architecture.md)，按键支持及缺口见 [vim-support.md](vim-support.md)。
