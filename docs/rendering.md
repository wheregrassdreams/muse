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
| **B2. 借 go-mermaid 的布局** | [iokdigital/go-mermaid](https://pkg.go.dev/github.com/iokdigital/go-mermaid) 纯 Go、Unlicense、Sugiyama 布局；但它只给 *AST*（不解析 Mermaid 文本），且布局是 `svg` 包里的非导出函数 | 要抄一份它的布局（Unlicense 无需署名），画仍归我们 |
| C. 借 D2 的 dagre | [d2](https://pkg.go.dev/oss.terrastruct.com/d2) 的 dagre 布局是 goja 里跑 dagre.js | 依赖重（含 JS 引擎） |
| D. 外部渲染 | `mmdc`（mermaid-cli，Node + Chromium） | 把浏览器塞回来，与本项目方向相反 |

**实测 go-mermaid（2026-11，v0.4.0）**：它的 PNG 光栅化（oksvg + rasterx）**画不出中文**，
节点里的汉字全是豆腐块 □（拉丁字母正常）；只有浅色主题；没有导出的布局 API
（`svg/runLayout` 非导出），要复用坐标只能把那个文件抄过来。所以它不是"换个库就完事"，
而是"借它的分层坐标"这一条路可选。

常用笔记里的流程图（十几二十个节点、分支合流）A 已经够用；复杂图不要假装能画。

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

**方案 A（推荐）：用它的 parser，写自己的 visitor。**
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
