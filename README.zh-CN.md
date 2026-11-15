# Muse

**所写，即所见。** 全原生的桌面 Markdown 编辑器，内置会用工具的写作 Agent——整个应用由 Go 绘制在 GPU 上，没有 WebView，没有 Electron。

[![Release](https://img.shields.io/github/v/release/CarlOpenLab/muse?color=24292f)](https://github.com/CarlOpenLab/muse/releases/latest)
[![License](https://img.shields.io/badge/license-MIT-24292f)](LICENSE)
[![Go](https://img.shields.io/badge/Go-1.27+-24292f)](native/go.mod)

[English](README.md) | 简体中文

官网：**https://carlopenlab.github.io/muse/**

基于 [MyGo](https://mygo.egoist.dev/) 构建。窗口、图标栏、文件树、大纲、搜索、AI Agent 面板，以及正文编辑器本身，全部由 Go 用 MyGo 的 `ui` 包绘制（Metal / Direct3D 11 / OpenGL），文本经系统排版引擎（Core Text / DirectWrite / Pango）排版。一个 Go 二进制，运行时不需要浏览器内核，也不需要 Node。

## 编辑器

编辑器（`native/editor`）是一整面可以直接书写的文档，行为对齐 Typora：

- **所见即所得的整篇编辑** — 文档就是渲染结果：光标在段落、标题、列表、引用、表格、代码与公式之间连续移动，可以跨块选择、复制、删除。
- **语法只在光标处显露** — `#`、列表符号、`>`、代码围栏、表格竖线这些块级标记永远隐藏；`**粗体**`、`` `代码` ``、`[链接](url)`、`$公式$`、`![图片](src)` 的标记只在光标进入时以浅灰色出现，离开即渲染。
- **Markdown 即时生效** — 行首键入 `# `、`- `、`1. `、`- [ ] `、`> ` 立刻变成标题、列表、任务、引用；键入 ```` ``` ```` 生成带闭合围栏的代码块，`$$` 生成公式块，`| a | b |` 回车生成表格。
- **Typora 式按键** — 回车在列表里续项、在空项上退出列表、在引用里续段；`Shift+Enter` 软换行；行首退格把标题降为正文、把列表项移出列表；`Tab`/`Shift+Tab` 缩进列表、在表格单元格间移动；`⌘B/I/U/E/K`、`⌘0–6`、`⌘⇧Q/[/]/K/M`、`⌘⌥T` 等格式快捷键；完整撤销/重做（AI 的修改同样可以撤销）。
- **原生渲染** — 标题、任务列表、带语法高亮的代码（30+ 语言）、表格（对齐、增删行列）、图片（本地 / 远程 / 粘贴 / 拖入，自动存进 `assets/`）、go-latex 排版的块级与行内公式（与正文基线对齐）、脚注、YAML front matter、内联 HTML 格式标签。
- **Mermaid 原生支持** — 流程图、时序图、状态图、饼图、类图与甘特图子集，光标离开时绘制成图。
- **输入法** — 中文/日文输入法在光标处组字，候选窗跟随光标。
- **工作区面板** — 文件树、跟随光标高亮的大纲、带替换的全文搜索与 AI 助手共享一条侧栏，由图标栏切换；非 Markdown 文本文件（`.json`、`.go`……）以带高亮的代码打开。

## 缪斯 Agent

AI 面板是一位真正的写作 Agent，不是聊天框：

- **13 个文档工具** — 列出、读取、搜索、大纲、目录树、统计、打开、精确替换编辑、插入、写入、重命名、回收站删除、新建文件夹。
- **工具审批卡** — 任何写操作之前，Agent 先出示要做的修改：同意这一次、本会话总是允许，或拒绝；拒绝原因会回到对话里。
- **权限三档** — *每次询问* / *完全访问* / *只读*，输入框旁一键切换。
- **能力分组开关** — 浏览、编辑、文件管理三组工具可在设置里按组开关；关闭的工具不会发给模型。
- **过程全透明** — 思考可折叠回看，每次工具调用带状态、参数与结果，正文流式渲染；你的话靠右，Agent 的每一步靠左。
- **OpenAI 兼容** — 任意端点（OpenAI、DeepSeek、本地网关）；不支持 function calling 的端点自动降级为纯对话。

隐私：Agent 的工具被限制在工作区根目录之内；API Key 与对话记录只存在本机配置文件（0600）。

## 安装

从 [Releases](https://github.com/CarlOpenLab/muse/releases/latest) 下载（Windows / macOS / Linux），或源码构建：

```bash
sh scripts/dev.sh                       # 运行
sh scripts/build.sh                     # 构建二进制
sh scripts/go.sh -C native test ./...    # 测试，含原生搜索链接
```

需要 Go 1.27.3+、CGO 和 C 编译器。实验全文搜索使用 Anytype 的 Tantivy 绑定，脚本首次构建时自动下载并校验固定版本静态库。Linux 使用 musl 版本，需设置 `CC=musl-gcc`；Windows 在 Git Bash 中使用 `sh scripts/go.sh -C native run .`。详见[搜索构建与运行说明](docs/tantivy-search.md)。不需要 Node、pnpm、vite，也没有任何前端构建。

## 项目结构

```
native/
├── cmd/muse-native/   仅负责调用 app.Run 的原生入口
├── app/               进程组合根与平台适配器装配
├── application/       文档、笔记库和工作区用例
├── document/          笔记、版本、元数据与持久化端口
├── editor/            所见即所得编辑器、块子系统与文本排版
├── markdown/          Markdown 语法、渲染、图片、公式与图表
├── search/            查询语言、元数据过滤、排序与全文索引
├── vault/             文件目录、链接、索引与文件监听
├── workspace/         会话状态与存储契约
├── platform/          文件系统和操作系统适配器
├── nativeui/          原生窗口与 UI 组合
├── agent/             AI 客户端与工具流程
├── config/            应用设置与主题配置
├── events/            强类型、实例级应用事件
└── ui/                共享原生 UI 基础设施，例如滚动条
```

## 已知限制

- 公式由 go-latex 排版；较冷门的 TeX 命令可能回退为源码。
- Mermaid 覆盖流程图、时序图、状态图、饼图、类图与甘特图子集；高级特性可能渲染为代码。
- 对话记录最多保留最近 100 条。

## 许可

[MIT](LICENSE) © Carl Open Lab
