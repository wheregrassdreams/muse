# 交接：Muse 的现状（2026-11-09）

这份写给接手的人（或 agent）。读完应该能：不改坏东西地跑起来、知道每个决定背后的理由、
知道下一步该做什么、以及哪些坑已经踩过了。

---

## 1. 这是什么

**Muse 是一个原生 Markdown 笔记应用**：Go + MyGo `ui` 直接画窗口、侧栏、编辑器和面板，
**没有 WebView，没有前端构建**。目标不是"另一个 Markdown 编辑器"，而是**能安全打开并编辑
已有的 Obsidian vault**：不改坏文件、双链可用、编辑行为与原始 Markdown 一致（Obsidian 的
live preview 只是**显示**，不改变编辑语义）。

三条立身之本，任何取舍都先问这三条：

1. **原生**：不加载 WebView，不为一个功能引入浏览器运行时。
2. **兼容**：源码是唯一的真实状态；任何"美化"都不能改变文件的字节。
3. **安静**：能力靠 ⌘K 与 hover 召出，界面本身不抢戏（Anytype / Obsidian 味）。

---

## 2. 五分钟上手

```bash
cd native
go test ./...                       # 五个包，全绿是基线；改动前先跑一遍
```

跑起来（仓库根目录）：

```bash
sh scripts/dev.sh                   # 构建并运行；用稳定签名，macOS 权限不会每次重问
```

**快速验证流程（推荐）**：

```bash
sh scripts/play.sh                  # playground：全类型样例，改 .go 自动重新构建并重开
sh scripts/play.sh --build-only     # 只构建，把样例准备好
sh scripts/play.sh --once           # 打开一次，关窗即退
```

- 样例在 `native/testdata/playground.md`，每次启动复制到 `native/.mygo/play/playground.md`
  （**每次重置**，随便改）。它同时是渲染不变量测试的语料。
- 想直接截真窗口：`MUSE_CAPTURE=/tmp/x.png sh scripts/dev.sh`（窗口渲染一次即退出；
  可配 `MUSE_CAPTURE_DELAY_MS`）。

**性能闸门**（1MB 文档，一次按键的 CPU 时间 < 8.3ms）：

```bash
MUSE_PERF=1 go test ./mdedit -run TestTypingFrameBudget -v
```

它默认跳过，因为 `go test ./...` 会把各包并排跑，墙钟读数会被别的进程污染。

常用环境变量：`MUSE_THEME=dark|light`、`MUSE_WORKSPACE=<dir>`、`MUSE_OPEN=<file>`、
`MUSE_SETTINGS=1`、`MUSE_PANEL=outline|search|ai|closed`、`MUSE_CAPTURE=<png>`。

---

## 3. 仓库地图

```
native/
├── main.go           只调用 nativeui.Run
├── mdedit/           所见即所得编辑器（1.6 万行，37 个测试文件）
│   ├── editor.go     状态、sync（唯一的解析入口）、SetSource、公共 API
│   ├── parsed.go     parserSource：解析输入的单一所有者（set / edit / clear）
│   ├── doc.go        buildLeaves：块 + 空行 → 叶子；addBlankLeaves
│   ├── display.go    源码 → 显示：pieces、srcToDisp/dispToSrc、stops、对象展开
│   ├── layout.go     排版：每种块的 view、表格几何、view 缓存键
│   ├── paint.go      绘制：文字、对象、块标记、源码浮条
│   ├── nav.go        光标：step、normalize、edgeStop、hitTest、整块选区
│   ├── input.go      按键与鼠标：press/drag/hover、源码开关、任务勾选
│   ├── edit.go       编辑：Enter/Backspace/Delete、缩进、行首标记
│   ├── format.go     格式命令（粗体/表格/任务…）
│   ├── comment.go    %%注释%% 的屏蔽（注释是文档级区间，不是叶子）
│   ├── mask.go       前置元信息屏蔽
│   ├── history.go    撤销栈
│   └── ime.go        输入法（UTF-16 偏移）
├── mdoc/             Markdown 的模型与渲染（16 个测试文件）
│   ├── math_image.go 数学：go-tex 解析/布局 → 光栅 → 缓存
│   ├── texcanvas/    ★ vendored 的 go-tex canvasr（见 §6）
│   ├── sketch.go     mermaid：sketch 出 SVG，这里取形状与标签
│   ├── diagram.go    ParseDiagram / ParseDiagramThemed
│   ├── mermaid.go    旧的自己写的 mermaid 布局（**待删**，见 §9）
│   ├── class.go/gantt.go/pie.go/state.go/sequence.go   同一批旧布局（**待删**）
│   └── view.go       非编辑态的渲染（预览、标题、表格…）
├── nativeui/         窗口外壳（20 个测试文件）
│   ├── app.go        App 状态、View、侧栏、画布、编辑器头部、面板
│   ├── run.go        菜单、快捷键、窗口、单实例
│   ├── frame.go      窗口按钮与标题栏的房间
│   ├── header_fade.go  顶部模糊：一遍着色器 + 渐变罩色（见 §7.1）
│   └── *.go          AI 面板、文件 IO、设置、命令面板、对话框
├── services/         原生 AI HTTP 客户端
├── doc/              文件类型注册表
└── testdata/         all-syntax.md、playground.md、commonmark/
docs/
├── editor-model.md   ★ 编辑器模型与不变量（改编辑行为前必读）
├── rendering.md      ★ 数学与 mermaid 的管线、上游问题、可选路线
├── playground.md     快速验证流程
├── rfc-001-*.md      早期 Web 侧 PoC 与迁移记录（历史，不必读）
└── HANDOVER.md       本文
scripts/              dev.sh、play.sh、build.sh、build-macos-app.sh、signing.sh
```

---

## 4. 编辑器模型：先读这一节，再动 `mdedit`

完整版在 **`docs/editor-model.md`**（含"参考系"：每条设计的先例与自研前提）。这里只留
接手时最容易踩的四条：

1. **源码是唯一状态。** 没有富文本树。显示、光标、选区、撤销都从 `src` 派生；
   一次编辑就是一次字符串替换。
2. **光标是源码字节偏移**，不是节点引用。任何"块内位置"都能还原成偏移。
3. **每一行都能被寻址**，包括空行（`kBlank` 叶子）。这是"按 Enter 就是加一行"的底座。
4. **显示是投影**：`pieces` 说明哪段源码显示成什么；光标落在边界上时标记展开成源码，
   这只是显示变化，不改编辑行为。

**解析输入只有一个所有者**（`mdedit/parsed.go` 的 `parserSource`）：`doc`、`src`、`mask`、
`comments`、`fm` 只有 `set`/`edit`/`clear` 能写，`Editor.sync` 是唯一决定"整篇重算还是增量推进"的
地方。新增派生字段时只需填这两个地方；`TestTheParserSourceIsWrittenInOnePlace` 会读本包
源码阻止别处写入，随机按键测试会在每一步校验"派生状态 == 重算"。

**原子对象（P1，已做一半）**：行内图片/公式/嵌入是原子单元——光标只停在两侧、方向键一次
跨过、Backspace/Delete 整段删除、点击选中整个对象（选区覆盖才展开源码）。
`TestAnObjectIsOneCharacter` 守着。表格与"独占一段的图片"是**整块一块**：三击/整块选区取
源码整行（`TestABlockIsSelectedWhole`）。

**区块上的控件（已做）**：任务框、代码块语言标签、图表源码、表格/图片的 `</>` 标签
——**都不动光标、不滚动视图，按两次回原样**。`TestBlockControlsLeaveTheCaretAndTheViewAlone`
守着五个控件。这是被用户连报三轮的同一类 bug，别再引入新的"借光标表达状态"的控件。

**已知近似**（`docs/editor-model.md` §5）：注释不是叶子（是文档级区间）；空行归属由行首标记
决定；引用里的空行按 Enter 会退出引用。

---

## 5. 渲染栈

| | 引擎 | 位置 |
|---|---|---|
| 数学 | **go-tex**（RaTeX 的 Go 移植，KaTeX 兼容：解析 + 盒子布局 + KaTeX 字体 + 光栅） | `mdoc/math_image.go` |
| Mermaid | **sketch**（pele 的 Go 移植 = Obsidian 自己的渲染器）出 SVG，我们取形状（tdewolff/canvas 光栅）与标签（MyGo 文字） | `mdoc/sketch.go` |
| 代码高亮 | mygo 自带 tokenizer | `mdedit/highlight.go` |
| 图片 | 原生解码 + 缓存 | `mdoc/inline_runs.go`、`mdoc/text.go` |

数学：块级用 **display 样式**（满尺寸分式、大算子上下限），行内用 text 样式；
`mdoc/display_math_test.go` 用位图尺寸断言。mermaid：`ParseDiagramThemed` 先试 sketch
（33 种图类型），它拒绝的类型才落到 `ParseDiagram` 里的旧解析器。

---

## 6. ★ `native/mdoc/texcanvas/`：vendored 的上游，带两个补丁

`go-tex` 的 `tex/canvasr` 有两个 bug，我们把那个文件抄进来（MIT，保留版权）+ 改了两处：

1. `ptPerUnit` 从 `1.0` 改成 `72/25.4`。canvas 的单位是毫米、`Face` 收的是点，
   原样让每个字形只有布局 em 的 0.35 倍——公式会是"角落里一个小点"。
2. `drawRect` 补上 `FontSize` 与 `Padding`（和 `drawLine`/`drawPath` 一致）。
   原样时 `\boxed`/`\colorbox`/`\rule`/数组分隔线都画在原点、亚像素。

两处都已报上游：[#6](https://github.com/Luo-Studio/go-tex/issues/6)、
[#7](https://github.com/Luo-Studio/go-tex/issues/7)。**上游修好后**：删掉整个
`native/mdoc/texcanvas/`，`math_image.go` 改回 `render.PNGTo`，`docs/rendering.md` 里那段
说明也删掉。

---

## 7. UI 现状（外壳）

- **默认扁平**：编辑器不是悬浮卡片（无描边/投影/圆角，与窗口同色）。菜单"扁平风格"
  （⌘⇧F）可切回卡片样式。`App.flat`。
- **没有左侧工具栏**（56px 图标列已删除）。它的内容分到侧栏：
  - 第一行：空间磁贴（首字母）+ 工作区名 + ⌄（菜单：打开文件/文件夹 + 四个视图）、
    搜索、撰写药丸（✎ ⌄ → 新建文档/新建文件夹）
  - 第二行：文件 / 大纲 / 搜索 / AI 助手（安静小图标，当前视图抬起）
  - 底部：主题、设置
- **侧栏开关只有一个**，固定在窗口左上（`x = 6`，有交通灯时从 `TitleBar` 之后算）。
  面板打开时压在侧栏头部左上；面板收起时它**不动**，编辑卡片向左扩，编辑器头部固定
  留 30px 给它——`TestTheSidebarToggleKeepsItsSeat` 守着"只有一个、x 不变"。
- **首行与红绿灯对齐**：扁平时 `headerH = max(44, titleBar.Height)`，这一行的中心就是
  红绿灯的中心（`App.headerH`/`App.topLine`）。测试器不画标题栏，所以只能靠取法保证。
- **面板边缘的把手**：3px 竖条，平时不可见，指针在边缘时出现（40px），拖动时 56px 且提亮，
  越过折叠阈值变强调色；触区 12px；双击复位。`panelResizeHandle`/`panelGrip`。
- **编辑器头部**：左 = 后退/前进（不可用时淡色、无动作，历史在 `navHist/navAt`）；
  中 = **文件路径**（`docPath()`，截断**保留尾部**，用真实文本度量 `tailPath`）；
  右 = 信息 ⓘ（路径/字数行数/修改时间/在访达中显示）与 更多 ⋯（保存/复制路径/设为工作区）。
- **顶部模糊（可选，默认开）**：扁平时头部**浮在文档上**，正文滚到它下面时渐进模糊、
  淡出，而不是在头部边缘被切断。实现在 `nativeui/header_fade.go`，设置 → 外观 →
  "顶部模糊"（视图菜单也有），`App.headerFade`。细则见 §7.1。

### 7.1 顶部模糊：一遍着色器，不重画文档

- **几何**：编辑器让出 `Options.TopInset = headerH`（`mdedit` 里进 `padTop`，
  `scrollIntoView` 的 `topMargin` 也抬到它之上），头部与一条
  `headerH + headerFadeReach(20)` 高的 pass-through 条带**绝对定位**在内容列里，
  于是正文能真正滚到头部下面——这是 macOS scroll edge 的前提。
- **macOS 的 scroll edge：正文从头部底下过，模糊逐级加深，加一层很浅的罩**
  （`header_fade.go`）。坡长 20 DIP，t 从头部下边界（0）到坡尾（1），三级级联，每级比
  下一级模糊 2.7–3 倍；下表由数字生成，别手画（手画过一次、和参数不符，被用户抓到）：

  ```
  0 ───── 0.10 ───── 0.35 ───────── 0.50 ───────── 0.65 ─ 1
  σ8  ████▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓░░░░░░░░░░░░░░░░░░░░
  σ3  ██████████████▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓░░░░░░░░░
  σ1  ██████████████████████████▓▓▓▓▓▓▓▓▓▓▓▓▓▓
  罩   ▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓░░░░░░
  ```
  （█ 满不透明　▓ 淡出中　░ 不画；每级都是一次 backdrop 读回）

  - **最细一级 1 DIP（2× 屏 2px）**——"模糊开始不像模糊"的门槛（glass 插件的
    `finestLevel = 2` 说的是**像素**，不是 DIP；按 DIP 做就是 4px，跟原图的交界看得出来）。
  - **淡出是指数**（`headerFadeDecay`）：变化集中在最上面（最糊、最不惹眼），后面拖长尾；
    S 曲线会把变化放在坡中间，正是眼睛最容易抓到的位置。
  - **罩 = 0.2**：只为可读性——顶部那团糊如果是纯灰的，头部的路径文字读起来会脏。
    0.2 提亮一点点，彩色内容只被洗掉两成。**别超过 0.5**：超过就是白板 + 白板的边，
    `TestHeaderFadeIsTransparent` 会红。罩 0.85 之前收完，坡尾不带颜色。
  - **级数**：四级（8/4/2/1）观感最接近"一张图逐渐变糊"，但三级（8/3/1）配 0.2 的罩
    在 20 DIP 内已经看不出台阶，且少一次读回。要更讲究就回到四级。
- **为什么不能更多级**：每级是一次 backdrop 读回（Metal 上 `endEncoding` 重开编码器）。
  两次读回实测落在噪声里，3 级要重新量；而且带宽只有 22 DIP，级数越多每段越短。
- **罩色 0.3**：页面色的浅罩。0.8 时它在 callout 的蓝、表格线、代码块上是白板；0 时糊掉
  的字太"实"，跟下面的锐字对比太强。`TestHeaderFadeIsTransparent` 卡在 0.3（中位蓝 12）
  与 0.8（4）之间，**0.3 是上限**。
- **静止时零成本**：`cover = ScrollY()/48`，文档在顶部时 `cover = 0`，条带不画、也不读
  backdrop —— 首行在静止时是锐的（`TestHeaderFadeStartsAtTheTopOfTheDocument`）。
- **成本**：每次背景读回在 Metal 上是一次 `endEncoding` + 重开编码器，是这笔账的全部。
  同一次运行里对照（每帧整屏滚动的最坏情况，`MYGO_FRAME_STATS=all`，**四级读回**）：
  开 med 0.6 / p90 1.5，关 med 1.3 / p90 2.5——落在噪声里（甚至更低，因为开的时候
  重画区域更小）。**这种绝对值必须同一次运行里对照**，
  负载不同能差一倍（早期在负载高时量到过 3 层 p90 7.4ms，据此得出的"加一级很贵"是错的）。
- **对照过 mygo-music**（`egoist/mygo-music` 的 `wallPane`）：它用框架自带的
  `plugins/glass.Blur{Radius, Mask}`——按 mask 渐变的**渐进**模糊（内部把半径拆成逐级
  减半的 blur level，每级只画在该半径适用的带里），再在上面重刷缓存背景图，让接缝处
  "与上面一模一样，于是没有边"。**没照抄的三点**：藏边靠重刷背景图（我们的页面没有可重刷
  的东西）；渐进模糊正是这里要避开的可读性缝（它背后是色块、带宽 72–112 才不明显）；
  它 Radius 16 要拆 5 次读回。值得借的只有"最细一级要细到看不出是模糊"这一条。
- 拍带滚动的截图：`MUSE_OPEN=… MUSE_SCROLL=<字节偏移> MUSE_CAPTURE=/tmp/a.png`。

## 8. 守护测试（改动前先看它们守什么）

| 测试 | 守什么 |
|---|---|
| `mdedit/document_state_test.go` | 20 种子 × 120 随机按键：光标合法性、`src==""⟹phantom`、派生状态 == 重算、叶子顺序/UTF-8 |
| `mdedit/render_invariants_test.go` | 语料 + 注释摆放：每片显示都是合法 UTF-8、有对象必有盒子、不许出现替换符 |
| `mdedit/atomic_test.go` | 行内对象是原子单元 |
| `mdedit/controls_test.go` | 五个区块控件：不动光标、不滚视图、两次回原样 |
| `mdedit/source_control_test.go` | 源码浮条：hover 前不画、点开变 raw 叶、cell 不再是 unit、整块选中含行尾 |
| `mdedit/budget_test.go` | 1MB 按键 < 8.3ms（`MUSE_PERF=1`，默认跳过） |
| `mdedit/parser_source_test.go` | 读本包源码：只有 `parsed.go` 能写解析输入 |
| `mdoc/display_math_test.go` | display/inline 的尺寸关系、分式尺寸、操作符间距 |
| `nativeui/panel_test.go` | 侧栏开关只有一个且位置不变、顶栏同一行 |
| `nativeui/header_fade_test.go` | 顶部模糊：滚动后文字变糊、条带外与原样逐像素相同、静止时不画、关掉是不透明条、开关不动文档、深色、**彩色块不被洗白**、降低透明度 |
| `nativeui/app_test.go` | 侧栏模块行、模块切换、主题跟随 |

---

## 9. 正在做 / 下一步

**进行中：侧栏部件卡（用户选的 B 方案：部件卡 + 下面接文件树）**

- 上层：space 头像、Inbox 卡（含一张预览卡）、可折叠分组"最近编辑/类型"、底部回收站
- 数据先接现有的：`a.recents`、`a.fileList`、workspace 目录，不新增存储
- 视觉：卡片 = 填充色 + 10 圆角 + 无描边；分组头 = 小字 muted + chevron
- 计划：先出"卡片骨架 + Inbox 一张真卡"给用户看味道，再铺其余

**待办（按优先级）**

1. **清死代码**：`nativeui/app.go` 的 `rail()`/`railButton`/`railInset`/`railButtonSize`/`railGapY`
   已随"去掉工具栏"失效；`mdoc/mermaid.go` 与 `class.go`/`gantt.go`/`state`/`sequence`/`pie`
   几套旧布局在 sketch 覆盖后可以删（删前确认 `ParseDiagram` 的兜底还需要它们）。
2. **mermaid 接 `Measurer`**：把本页字体度量传给 sketch，长中文标签才不会溢出
   （`mdoc.SketchOptions.Measure` 已留好接口，`mdedit/assets.go:sketchOptions` 还没填）。
3. **深色主题核对**：mermaid 的 pele 令牌已经吃我们的 palette，`\boxed`/`\frac` 的
   颜色是黑的（深色下反相），都需要一张深色截图确认。
4. **数学按设备像素比光栅化**：现在固定 `mdoc.MathDPI = 192`，1× 屏细笔画略淡，
   2× 屏接近 1:1。改法：把窗口的 backing scale 传进 `RenderMath*` 与光栅密度。
5. **文档头**（参考图里的 `Note · 1 反向链接 · Tag` + 面包屑）：需要新的数据（反向链接数）。
6. 表格"从外面整体选中"的交互（CM6 语义）：目前是网格按单元格编辑 + 三击整块选中，
   用户还没确认要不要"点一下先选中整表"。

---

## 10. 协作约定（这个项目踩出来的）

- **先找根因，别改症状。** 这一路修过三个"同一个原因"的 bug（控件借光标表达状态、
  注释屏蔽规则有两份、派生状态只在增量路径更新），每次的修法都是**结构**改动 + 不变量测试，
  而不是加一个特判。
- **能让测试守住的，就不要靠注释。** 行为断言优先用"位图尺寸/光标偏移/叶子顺序"这类
  可计算的东西，而不是肉眼看截图。
- **不半修。** 做不到的（比如 mermaid 的 `subgraph`、数学的矩阵）就**显示源码**，
  不要画成别的样子。
- **提交信息**写"为什么"和"代价"（依赖、体积、近似），不只写"做了什么"。
- **不引入 WebView / 浏览器运行时**。这是产品的立身之本。
- 用户会亲自跑、亲自看截图；汇报时给**位置**（文件:行）、**命令**、**数字**，
  别只说"已修复"。

---

## 11. 接手第一小时

1. `cd native && go test ./...` —— 确认五个包全绿（当前基线）。
2. `sh scripts/play.sh` —— 打开 playground，翻一遍：中文 mermaid、块级公式、
   `%%注释%%`、表格 `</>`、空笔记。
3. 读 `docs/editor-model.md` 与 `docs/rendering.md`（这两份是这个项目的"为什么"）。
4. 读本文 §7、§8、§9。
5. 挑一件 §9 里的事做，动手前先写/找对应的不变量测试。

有问题先看 `git log --oneline -30`：最近三十条提交的信息里写着每一次取舍的理由。
