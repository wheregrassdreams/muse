# Tantivy 搜索接入与资源基准

`native/search` 拥有 Tantivy 全文索引与查询契约；`native/platform/filesystem/vaultsearchstore` 负责为每个笔记库选择持久目录。上层提供已解析的磁盘快照，不让搜索后端遍历笔记库或重读文件。`ID` 用于更新、删除；当前 UI 由工作区相对路径生成 ID，重命名按旧 ID 删除、新 ID 插入处理，尚未实现跨重命名的文档身份。`Revision` 用于识别未变化快照；未保存编辑通过 `Search` / `Literal` 的 overlay 参数覆盖磁盘版本，不写入持久索引。

接口为 `Open(dir)`、`Reconcile(ctx, docs)`、`Apply(ctx, upserts, deletes)`、`Search(ctx, query, limit, overlays...)`、`Literal(ctx, query, limit, overlays...)` 和 `Close()`。`Reconcile` 使索引与传入快照一致；增量事件使用 `Apply`。普通搜索返回相关文档，字面量搜索保留子串语义及 UTF-8 字节区间。关闭索引后可从同一目录重开。

## Anytype 参考

- Go/Rust binding 固定参考：[anyproto/tantivy-go v1.0.6](https://github.com/anyproto/tantivy-go/tree/v1.0.6)。该 binding 需要 CGO 和对应平台的预编译 native library。
- 实际索引 schema、analyzer 注册、文档写入及搜索生命周期：[anytype-heart `ftsearch.go`](https://github.com/anyproto/anytype-heart/blob/develop/pkg/lib/localstore/ftsearch/ftsearch.go)。该实现将英文与中文文本分开索引，中文字段使用 jieba；用 raw ID 精确删除文档。Tantivy 会拒绝 schema 不兼容的旧索引，Anytype 会隔离旧目录再重建。

Muse 使用本地持久索引；首次初始化、文件变化及退出时机由 vault 生命周期管理。schema 变化需要明确重建策略，不应假设旧索引可直接打开。

当前字段按中英文 analyzer 分开，另有标题/路径前缀与中文单字、双字召回；源文只在 native document store 保存，Go 侧保留轻量路径/版本清单。普通结果按加权相关性排列，未保存覆盖层采用字段启发式评分，尚未与 Tantivy 的语料统计校准。字面量不依赖词项召回，按路径顺序逐批读取正文并返回每个不重叠命中的原始 UTF-8 字节区间。

缓存位于共享应用缓存根下的 `vaultsearch/<workspace-hash>/`，不在笔记库中。缓存根默认是 `$XDG_CACHE_HOME/muse`，未设置 `XDG_CACHE_HOME` 时为 `~/.cache/muse`；`MUSE_CACHE_DIR` 可覆盖完整根目录。该根也用于网络图片等其他可重建下载数据；测试必须把 `MUSE_CACHE_DIR` 指向独立临时目录。旧的搜索专用 `MUSE_SEARCH_CACHE` 不再生效。跨进程目录锁保护恢复与写入；写入覆盖整个操作的 dirty marker，最终成功才发布版本清单。每批最多 128 篇独立提交，整次批量不是单一原子事务；中断后关闭、重开并重建缓存，不会改写笔记。native 调用不能在执行中由 Go context 强行中止，取消在调用前后与批次间生效。

侧栏放大镜和 ⌘⇧F 打开笔记库浮层，⌘F 仍查找当前笔记；全文和字面量是明确区分的模式。空查询显示当前库最近笔记，方向键选择、Enter 打开、Esc 关闭。最多显示 500 条，点击结果前检查内容版本，过期位置重新查询；未保存 buffer 不写回磁盘索引。

## 独立资源基准

命令 `native/cmd/searchbench` 生成固定 2 KiB、UTF-8 中英混合的合成文档。默认 1,000 篇，`-n 10000` 生成 10,000 篇；`-n 0` 建立空索引，用作 native RSS 基线。命令依次测量首次 Open、全量 Reconcile、Close/Open、无变化 Reconcile、单篇更新、常见词/稀有词/无命中/中文单字/中文双字和 Literal 查询。更新检查会确认新 token 命中且旧 token 消失。

先构建一次，再按顺序运行各规模；不要并行启动大规模基准：

```sh
./scripts/go.sh -C native build -o /tmp/muse-searchbench ./cmd/searchbench
/usr/bin/time -l /tmp/muse-searchbench -n 0 -output /tmp/searchbench-empty.json -hold 10s
/usr/bin/time -l /tmp/muse-searchbench -n 1000 -output /tmp/searchbench-1k.json -hold 10s
/usr/bin/time -l /tmp/muse-searchbench -n 10000 -output /tmp/searchbench-10k.json -hold 10s
```

JSON 包含各阶段耗时、索引目录磁盘字节数、命中数及 Go heap/分配量。heap 指标取自 `runtime.MemStats`，每阶段计时前后在计时区间外执行 GC；它不包括 Tantivy/Rust native 分配。进程峰值 RSS 由 macOS `/usr/bin/time -l` 的 `maximum resident set size` 提供，包含该独立 benchmark 进程。`-hold` 在 JSON 输出后让已打开索引保持一段时间，便于外部观察；`-index DIR` 可指定持久目录，该目录会被索引操作修改。绑定的 `LibInit` 可能向 stdout 打印诊断行；指定 `-output FILE` 会将纯 JSON 写入该 artifact 文件，未指定时 JSON 行仍写 stdout，可从输出中过滤最后的 JSON 行。

构建固定 `tantivy-go v1.0.6` 对应平台 lib，并校验仓库内固定的 SHA256；通过 `scripts/go.sh` 自动下载并设置 CGO linker flags。macOS 使用系统 clang 与 SDK（可显式设置 CC/SDKROOT）；Linux artifact 是 musl 版本，需安装 musl 工具链并设置 `CC=musl-gcc`；Windows 使用 Git Bash 与对应 C 工具链。当前机器验收覆盖 macOS arm64，其他平台需 CI 验证，不宣称已经实机通过。

## 2026-11-10 独立进程实测

Apple M3 / macOS arm64，单次热合成语料，10,000 篇，每篇 2KiB，共约 19.5MiB 正文。原始结构化输出见 [测量记录](perf/tantivy-10000-2026-11-10.json)。

| 指标 | 本轮结果 |
|---|---:|
| 首次全量构建（128 篇一批） | 13.87s |
| 索引重开 | 4.14ms |
| 无变化 Reconcile | 11.89ms |
| 单篇更新 | 121.53ms |
| 稀疏命中 / 无命中 | 0.65ms / 1.08ms |
| 常见词前 500 篇 | 15.43ms |
| 中文单字 / 双字前 500 篇 | 15.37ms / 14.97ms |
| 字面量前 500 处 | 4.58ms |
| 字面量稀疏命中 / 无命中（全库扫描） | 343.24ms / 342.16ms |
| 进程峰值 RSS | 197.7MiB |
| 查询后保持索引打开时 RSS 单点采样 | 196.3MiB |
| 更新后索引目录 | 约 6.23MiB |

此前一次构造全部 10,000 个 native 文档时峰值约 378.6MiB；小批提交以首次构建时间换取更低峰值。上述 RSS 包含 benchmark 语料和 native 分配，**不是 Muse UI 进程总内存或索引净增量**。语料含大量重复文本，磁盘压缩率不能外推到真实笔记库；已有操作系统缓存，重开也不是冷磁盘启动。字面量扫描不依赖分词，保留精确子串召回；稀疏/无命中会扫描全库，本轮累计 Go 分配约 100MB（不是常驻 heap），仍是优化重点。本轮 RSS 包含这些额外扫描，与仅测常见词早停的旧记录不可直接比较。尚未给出真实库容量上限或查询并发时的输入延迟保证。

## 原生窗口采样

另启动隔离测试窗口，打开 1,000 篇、共 2,048,000 字节的真实 Markdown 文件并完成跨笔记查询。每 100ms 用 `ps` 采样：峰值样本 257,196,032 字节（约 257MB），结束前样本 236,175,360 字节（约 236MB）。这包含完整 UI 和 native 索引，尚未恢复到此前不到 200MB 的水平；采样峰值不等于精确最高值，短时结束样本也不是长期稳态。记录见 [窗口资源采样](perf/tantivy-ui-1000-2026-11-10.json)。
