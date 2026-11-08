# 渲染：Mermaid 与数学

两条渲染管线都是自己的代码（`native/mdoc`）。这一页写清它们现在是什么、
依赖的上限在哪、以及可选的路。

## Mermaid

**现在是自己的渲染器。** `mdoc/diagram.go` 按顺序尝试 gantt、class、pie、
state、sequence、flowchart 六个解析器，每个都自己算坐标、自己画（`ui.Painter`）。
没有依赖，也没有浏览器。

为什么不能直接用 mermaid.js：它是 JavaScript，需要 DOM 和 SVG，输出是 SVG 字符串。
本项目的立身之本是原生绘制、没有 WebView，所以"用官方渲染器"等于把浏览器
再塞回来。

**画不出来就显示源码，而不是画成别的样子。** `subgraph` 的框是这个图的一部分，
只画框里的节点等于换了张图——所以带 `subgraph` 的图落回代码块显示。
不认识的图类型（`sequenceDiagram` 之类没有解析器的）同样落回源码。
`TestASubgraphFallsBackToTheSource` 钉住这一条。

路线（按代价排）：

| 方案 | 做法 | 代价 |
|---|---|---|
| **A. 继续自己做**（现状） | 分层布局已有；补交叉最小化、正交连线（已做）、子图、更多图类型 | 每加一块几百行，质量可控 |
| **B. 只借坐标** | 用纯 Go 的 [go-graph-layout](https://socket.dev/go/package/github.com/gverger/go-graph-layout)（Sugiyama 分层）算坐标，画还是自己画 | 一个新依赖，接口要适配 |
| **B0. sketch（pele）出 SVG，我们重绘** | 见下：Obsidian 的渲染器，33 种图，带度量钩子与主题令牌 | 自己写一个 SVG→painter 的小渲染器（约 400 行） |
| **B2. 借 go-mermaid 的布局** | [iokdigital/go-mermaid](https://pkg.go.dev/github.com/iokdigital/go-mermaid) 纯 Go、Unlicense、Sugiyama 布局；但它只给 *AST*（不解析 Mermaid 文本），且布局是 `svg` 包里的非导出函数 | 要抄一份它的布局（Unlicense 无需署名），画仍归我们 |
| C. 借 D2 的 dagre | [d2](https://pkg.go.dev/oss.terrastruct.com/d2) 的 dagre 布局是 goja 里跑 dagre.js | 依赖重（含 JS 引擎） |
| D. 外部渲染 | `mmdc`（mermaid-cli，Node + Chromium） | 把浏览器塞回来，与本项目方向相反 |

**实测 sketch（`tuios.dev/sketch`，2026-11 v0.4.0）——推荐**：它是
[pele](https://github.com/obsidianmd/pele) 的 Go 移植，也就是 **Obsidian 自己的 Mermaid
渲染器**（MIT，带 pele 的版权声明）。33 种图类型，每种都和 pele 的 golden 逐字节对齐
（flowchart 114/114、sequence 38/38、class 38/38、er 25/25、gitGraph 35/35…），
`testdata/extra` 里专门有**中文/日文标签换行**的用例。

对我们最要紧的三点：

- `RenderOptions.Measurer` 是文本度量钩子：可以把我们的 CoreText 宽度量进去，
  布局按真实字宽算（这正是 markata 的对比报告说其它引擎做不到的）
- `RenderOptions.Theme`：pele 的 `--pele-*` 颜色令牌直接吃我们的配色
- `MaxWidth` + `AutoDirection`：图放不下时自己改成竖排（编辑器列宽会变）

但它的 PNG 光栅（`sketch/raster`）**只内嵌 Arimo（拉丁/希腊/数学）与 Go Mono**，
没有中日韩字形 ✗。所以路线是：**sketch 出 SVG，我们自己画**——pele 的 SVG 只用
`<g> <rect> <path> <text> <polygon>`（实测一张图 12 个 g、6 个 text、6 个 path、5 个 rect、
1 个 polygon），用 MyGo 的 painter + CoreText 排版重绘即可：矢量、中文、跟随主题，
不加新依赖（约 400 行）。

**实测 go-mermaid（2026-11，v0.4.0）**：它的 PNG 光栅化（oksvg + rasterx）**画不出中文**，
节点里的汉字全是豆腐块 □（拉丁字母正常）；只有浅色主题；没有导出的布局 API
（`svg/runLayout` 非导出），要复用坐标只能把那个文件抄过来。所以它不是"换个库就完事"，
而是"借它的分层坐标"这一条路可选。

**mmdg（`mermaid-go-renderer` v0.2.0）**：纯 Go、Mermaid→SVG、按系统字体度量。
markata 的对比报告（24 个 fixture）里它和 go-mermaid 都 24/24 通过，但 mmdg 在
class/ER 上有标签与边框重叠，且没有 sketch 那样的度量钩子/主题令牌/宽度自适应。
所以排在 sketch 之后。

**结论**：mermaid 换成 **sketch 出 SVG + 我们重绘**。自己那套分层布局退役后，
"交叉交叉"、"子图"、"序列图"这些问题都不再是自己的算法问题。

## 数学

**管线**：`normalizeTeX` → go-latex 的 `mtex.Parse`（TTF 后端，把字形和线条画到
`drawtex` 画布）→ 光栅化成位图（`mathDPI=192`，深色主题反相）→ 缓存。

**两条兜底**（问题就在这里）：

- `typesetScripts`：自己拼上下标，没有 TeX 的间距、数学轴、极限位置
- `parseMatrix`：自己画 `pmatrix`，所以括号只是一条竖线

**上游的真实状况**（2026-11 查 codeberg 上的 main）：`mtex/parser.go` 的 visitor 只处理
`Symbol` / `Word` / `Literal` / `MathExpr` / `Macro`，**没有 `*ast.Sup` / `*ast.Sub`**，
`\begin{...}` 仍然是 `unknown macro`。实测：

```
"a^2+b^2=c^2"                    panic: unknown ast node *ast.Sup
"\int_0^1 x^2 \, dx"             panic: unknown ast node *ast.Sub
"\begin{pmatrix} a & b \\ c & d \end{pmatrix}"   unknown macro \begin
"\frac{1}{2}"                    err=<nil>   ← 只有这种没有上下标的才走 TeX
```

所以"丑"不是配色问题，是**几乎所有真公式都没走 TeX**。

**第一刀已落地（display 样式）**：块级公式现在按 display 排——`\frac` 换成
`\dfrac`（满尺寸，不再缩小）、大算子（∫∑∏⋃⋂…）的上下限放到符号上方和
下方（行内仍然放在旁边，TeX 就是这样）、分片拼接处补回操作符前的间距。
`mdoc/display_math_test.go` 用位图尺寸断言这四条（display 比 inline 高、
更窄、分式更大、操作符吃掉了空间）。

**已换引擎（2026-11）**：数学改为 [go-tex](https://pkg.go.dev/github.com/luo-studio/go-tex)（RaTeX 的 Go 移植，
KaTeX 兼容）。它有自己的 lexer / 宏展开 / parser / **TeX 盒子布局（含 D/S/SS 数学样式）** /
KaTeX 字体度量与内嵌 KaTeX TTF，输出 PNG。于是：

- `a^2`、`\int_0^1`、`\sum_{i=1}^n`、`\left( \frac{a}{b} \right)^2`、`\begin{pmatrix}` 全部真排
- display / inline 由 `mathstyle` 决定（块级满尺寸分式、上下限在符号上下）
- **删掉了我们自己的三套补丁**：`script_layout.go`（拼接上下标）、`math.go` 里的矩阵渲染
  （`parseMatrix`）、`displayFracs` 伪装
- `mdoc/math_image.go` 只剩：解析 → 布局 → `render.PNGTo` → 解码 →（深色反相）→ 缓存
- 依赖代价：二进制 +6MB（内嵌 KaTeX 字体），MIT 许可，纯 Go 无 cgo
- **一个上游 bug 与我们的补丁**：`tex/canvasr` 里 `ptPerUnit = 1`，但 canvas 的单位是毫米、
  `Face` 收的是点，于是每个字形只有布局 em 的 72/25.4（0.35）——公式成了"角落里一个小点"。
  我们把 `canvasr` 那 250 行抄进 `native/mdoc/texcanvas/`（MIT，保留版权声明），
  只把那个常数改成 `72/25.4`，其余一行未动
- 已知待办：**按设备像素比光栅化**。现在固定 192 DPI，在 2× 屏上接近 1:1，在 1× 屏上
  细笔画会略淡（KaTeX 的笔画比原来的 Computer Modern 细）

**方案 A（历史，保留作参考）：用它的 parser，写自己的 visitor。**
它的 parser 是好的（`latex.ParseExpr` 产出的 AST 有 `Sup`/`Sub`/`Arg`/`OptArg`），
`tex` 包里也有盒子模型（`HList` / `VList` / `HRule` / `AutoHeightChar` / `SetShift`）。
缺的只是"把 AST 摆成盒子"这一步。分四刀：

1. 上下标（`a^2`、`x_i`、`\sum_{i=1}^n` 的极限位置）
2. 分数与数学轴（`\frac` 的基线、`=` 与分式的对齐）
3. 大算子与间距表（TeX 的 8 类原子与 3 种间距）
4. 矩阵与伸缩括号（替掉手画的 `parseMatrix`）

每刀都能单独出 golden 测试（宽度、基线、上下标相对位置），不必一次重写。

**方案 B**：等上游实现 Sup/Sub。不可控，且模块路径已换成
`codeberg.org/go-latex/latex`，将来若要跟新版需要加一条 `replace`。

**方案 C**：换渲染器。KaTeX / MathJax 都是 JS + DOM/SVG；纯 Go 里没有更完整的
TeX 排版引擎。
