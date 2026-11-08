# Playground：一份样例，改完就能看

`native/testdata/playground.md` 是一份把编辑器所有内容类型都写了一遍的笔记：
front matter、各级标题、行内标记、双链、标签、行内公式、行内/多行注释、列表与任务、
引用与 callout、表格、代码块（含 mermaid）、公式块、图片（内联/独占/HTML）、HTML 块、
分隔线、脚注，以及一批边界情况（转义、代码里的百分号、跨行注释、超长段落、只有表头的表）。

`scripts/play.sh` 把它跑起来：

```sh
sh scripts/play.sh              # 构建、打开，之后改文件自动重新构建并打开
sh scripts/play.sh --build-only # 只构建，把样例准备好
sh scripts/play.sh --once       # 构建、打开一次，关窗即退出
MUSE_THEME=dark sh scripts/play.sh   # 环境变量照样透传
```

## 每次都从同一份样例开始

样例在启动时被复制到 `native/.mygo/play/playground.md`（在 `.gitignore` 里），
**每次启动都重新复制**。所以在里面乱改不会积累：关掉重开就是干净的那一份，
想验证"打开一个陌生 vault"也不会因为上次的编辑而变样。

想改样例的内容，改 `native/testdata/playground.md` 本身——它也是"改完自动重启"
的一项输入。

## 关于"热重载"

Go 编出来的是新二进制，所以这里的热重载是**改文件 → 重新构建 → 关窗再开**，
不是同一个进程里换代码。窗口位置存在 `window-state.json` 里，重开时窗口回到原处，
看起来像是原地刷新。

脚本用 `find -newer <stamp>` 轮询（每 0.4 秒），不依赖 `fswatch` 之类的外部工具：
改 `native/mdedit`、`native/nativeui`、`native/mdoc`、`native/services`、`native/doc`、
`native/cmd` 下的 `.go`，或者改样例本身，都会触发。构建失败时窗口保持关闭，
再存一次文件就重试。

## 为什么它也在测试里

`TestRenderedTextHasNoStrayGlyph` 把 `native/testdata/playground.md` 放进渲染语料：
它在多个光标位置渲染，断言"每片显示文本都是合法 UTF-8、有对象必须有盒子、
没有对象不许出现替换符"。这份样例写出来第一次跑就抓到一个真 bug——
多行 `%%注释%%` 的中间行被当成缩进代码块，把汉字切成两半。
所以 playground 不只是给人看的，它是一份会自己找 bug 的输入。
