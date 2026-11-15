# 通用文本分词与词导航架构

本文件记录 `docs/text-navigation-spec.md` §3 的当前实现，不代表 Flash/EasyMotion UI 已实现。

## 边界与 API

`native/editor/navigation` 是不依赖 UI、Editor 或 Vim 的纯文本子系统。`navigation.New(text)` 生成不可变 `Index`；`Tokens()` 返回副本，`TokenCount()`、`WordAt()`、`NextStart()`、`PrevStart()`、`WordEnd()`、`NextEnd()` 和 `Targets(region, direction, kinds...)` 查询 token 与候选位置。`Token`、`Range`、`Span`、`Target` 的起止均是原始 UTF-8 字节半开区间。索引不改写源字符串。

tokenizer 先以 `go-text/typesetting/segmenter` 的 `GraphemeIterator` 划分扩展 grapheme，再产生 whitespace、punctuation、symbol、word、number、identifier token。`Provider.Segment(run)` 接收连续字母/数字/下划线 run，可被另一语言或用户词典实现替换；返回 nil 时使用通用 Unicode run token。默认 gse provider 对不含 Han 的 run 返回 nil，所以纯英文/数字不触发中文词典初始化。所有 provider span 都必须落在 grapheme 边界，否则拒绝该 span 并将缺失源片段作为 token 保留。空白、标点和 symbol 也完整保留在 token 序列里。

`Index.Targets` 只处理纯文本位置，按区域、方向、kind 过滤并排序；方向必须为 `Forward` 或 `Backward`，0 返回 nil。未来 Flash/EasyMotion 消费者可以请求 word start/end 候选，而 UI label 与目标绘制由调用方负责。Markdown display adapter 额外检查映射后的 source span；`pDeco` 装饰没有源字节，不会成为 Editor 词移动/双击目标。

## 中文 provider 与失败策略

默认 provider 固定 `github.com/go-ego/gse v1.3.1`，使用包内嵌的简体中文 `zh_s` 词典，`SkipSubSeg: true`（只需要普通分词，不构建搜索模式子分词表）。本实现调用该版本的 `Segment` 词典最短路径接口（未调用 HMM 分词接口）；歧义结果由固定词典频次与算法确定，不另维护应用内手写测试词表。示例 `今天我们学习中文分词` 得到 `今天 / 我们 / 学习 / 中文 / 分词`。词典外的未知汉字在 gse 输出中逐字退化（例如 `龘龘龘龘` 得到四个单字 token）；若嵌入词典初始化失败，默认 provider 保留整个连续 linguistic run 为一个 token，并通过 `navigation.DefaultStatus() (ready, err)` 暴露初始化错误，不伪称分词成功。自定义 provider 可通过 `NewWithProvider` 注入，不影响默认 provider 状态。

词典通过 gse 的 `//go:embed` 资源随二进制打包，运行时不读取工作目录、Go module cache 或用户文件。版本源码随模块缓存在本地，许可文件为 gse v1.3.1 的 [Apache License 2.0](https://github.com/go-ego/gse/blob/v1.3.1/LICENSE)；直接依赖和校验值固定在 `native/go.mod`、`native/go.sum`。gse 的间接数据结构依赖包括 `github.com/vcaesar/cedar v0.50.1`。

## Editor 字节与 UTF-16 adapter

Editor 的源码 caret 是 UTF-8 byte offset，display 和 `ui` 布局 offset 是 UTF-16 code unit。`displayWordIndex` 在两种坐标间统一适配，只保存 rune boundary 的有序 byte/u16 offset 数组，以二分查找做转换；传入扩展 grapheme 的合法边界可精确往返。补充字符内部的 surrogate / UTF-8 编码字节若被传入，start/end 参数分别向前/向后吸附到相邻 rune 边界。所有 `w`、`b`、`e`、双击 word range 和 native Option/Ctrl word move/delete 都先经此 adapter，不把 textnav byte offset 直接交给 UTF-16 API。

Editor 按精确 display string 缓存 immutable `Index` 和映射数组，8 项 LRU、估算总成本上限 256 KiB；超过预算的单个大 display string 作为唯一 entry 保留，普通短串出现后会淘汰它。缓存命中时，段落内连续查询和跨段往返不会重复分词；超长的当前段落也不会每次按键重建。被淘汰的段落在再次访问时重建索引。

`SetSource` 打开含 Han 的文档、或首次插入 Han 时，Editor 调用 `WarmUpIfNeeded` 触发全进程唯一后台 warmup；英文编辑不启动中文词典。索引若在 warmup 完成前马上遇到 Han，会等待同一个 `sync.Once` 完成后使用确定结果。调用方也可以显式同步调用 `navigation.WarmUp()`；无界并发加载不会发生。

## w/b/e 和第二消费者

`w` 取下一个非空白 token start，`b` 取前一个非空白 token start，`e` 取当前/后续 token 的 exclusive end；Vim 普通 `e` 落到该 grapheme 的起点来表示 inclusive caret，operator-pending `e` 保留 exclusive endpoint。重复计数的 e 从上一个 token 接着走。Punctuation 和 symbol 仍是独立 token，因此 Vim 类命令可跨过标点停靠；具体按键语义不写入 `textnav`。

`de`、`ce`、`dw`、计数、双击 word range 和原生 Option/Ctrl 左右移动/删除复用 Editor 的同一分词索引与 byte/UTF-16 adapter。纯文本查询本身不了解 Markdown；Editor 负责把 display token 映射回显示内容对应的源字节。

## 实测代价与基准

环境：macOS arm64，Apple M3，Go 1.27.3，gse v1.3.1，`zh_s` + `SkipSubSeg:true`。用单独最小 Go probe 对比无 gse 的空程序与实际 provider 配置：空程序 1.8 MB，gse/内嵌字典程序 35 MB，增加约 33.2 MB。字典冷加载多次实测约 126–168 ms（首次 probe 274 ms）；执行 `runtime.GC()` 且 `runtime.KeepAlive(segmenter)` 后 live HeapAlloc 约 58.2 MB，HeapSys 约 83 MB，`/usr/bin/time -l` 峰值 RSS 约 80.7 MB。未启用 `SkipSubSeg` 的 probe live allocation 约 112 MB、峰值 RSS 约 120 MB；此应用只需普通模式，因此启用该选项。不同平台链接器、Go 版本和运行环境会改变具体数字。

`cd native && go test ./editor/navigation -run '^$' -bench BenchmarkNewChineseIndex -benchmem -count=3` 在 Apple M3 上的 warm segmentation 结果记录为约 12.2–12.9 µs/op、13,784 B/op、41 allocs/op（代表性混合中文/英文/标识符/数字短段）。该基准在计时前初始化一次默认词典；冷启动单独按上述 probe 计量。

## 验证范围

`native/editor/navigation/navigation_test.go` 覆盖真实中文词、UTF-8 byte ranges、source 字节保真、英语数字混排、emoji ZWJ、组合音标、full-width 字符、OOV 单字、provider grapheme 边界拒绝、region/kind/direction 查询、ASCII lazy path 和并发只读索引。`native/editor/textnav_adapter_test.go` 覆盖 UTF-16↔byte 映射、跨段缓存、有界缓存和外链零源装饰过滤；`native/editor/navigation_acceptance_test.go` 覆盖 `w/b/e/2e/de/ce/dw/2dw` 与同一分词边界。
