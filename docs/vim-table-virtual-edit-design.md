# Vim 表格虚拟编辑模式设计

## 目标

Vim 模式下，对表格行、列或整表执行结构选择后，`d` 应把选中结构存入结构寄存器；随后 `p/P` 按表格结构语义插入该结构。操作以表格的行列坐标为依据，不能要求光标偏移恰好落在 cell 文本上。

当前故障路径是 `Vj d p`：删除后光标落在表格行首的 Markdown 分隔符上，`unitAt` 没有对应 cell；既有粘贴逻辑把 nil 传给 `tableOf`，随后又把光标位置当成表格外，导致 panic 或生成独立表格。

## 状态模型

Markdown 源码仍是唯一持久状态。表格模式是从解析结果派生的短生命周期虚拟状态，不保存第二份可独立修改的表格正文。

```text
source string
  └─ parser leaf: table
       └─ virtual table session
            ├─ table identity: source anchor
            ├─ cursor: row, column, edge
            ├─ structural selection: row/column/whole
            └─ optional post-delete cursor: row, column, edge
```

实现约束：

- 结构光标使用 `TablePosition{Row, Column, Edge}`，不以 cell 文本中的字节偏移表示结构位置。
- 结构寄存器保留轴、二维 cell 内容、列对齐和是否整表；空 cell 是有效内容，空结构和未设置寄存器状态分开表示。
- 表格命令接收虚拟位置和结构寄存器，解析、应用一个结构变换，再通过编辑器现有 transaction 写回源码。解析器仍是编辑后的结构权威。
- 普通单元格文字编辑继续使用编辑器的源码光标；虚拟表格模式只拥有结构导航、结构选择和结构粘贴语义。
- 结构命令不得从任意源码 offset 猜测行列；偏移只用于解析器生成的表格身份锚点及最终源码替换。

## `d` 后的虚拟光标

- 删除行范围后，虚拟行光标落在删除区起始位置对应的下一条现存数据行；若后面没有行，则落在前一条数据行；若表格没有数据行，则落在 header 行之后的插入边界。
- 删除列范围后，虚拟列光标落在删除区起始位置对应的下一条现存列；若后面没有列，则落在前一列。
- 删除整表后不保留表格 session；整表寄存器之后按文档级表格粘贴语义生成独立表格。
- 删除产生的虚拟光标在 transaction 后仍有效，直到 `p/P`、离开表格、源变化使锚点失效或用户执行其他会改变焦点的导航命令。单纯 `y` 不冻结目标；后续 `p/P` 按当前 caret 的解析位置执行。

## `p/P` 规则

- 行寄存器：`p` 插在虚拟光标行之后，`P` 插在之前；对仅含 header/separator 的表格，插入在 header 之后。
- 列寄存器：`p` 插在虚拟光标列之后，`P` 插在之前；保留寄存器的列顺序、空单元格和对齐信息。
- 表格内没有可恢复的虚拟焦点时，从当前 cell 解析成 `TablePosition`；若当前位置在分隔符或行首，则映射到该源码行对应的表格行。
- 整表寄存器或目标表格失效时，使用文档级粘贴路径；不得因为 `unitAt` 返回 nil 而 panic。
- 插入完成后将虚拟焦点移至新结构，并清除一次性删除目标。

## 操作边界

- Vim 按键是消费者：`d/y/c/p/P` 请求表格结构操作；结构模型不读取按键。
- `EditorOperation` 或表格专用语义接口暴露 `DeleteSelection`、`YankSelection`、`PutRegister{Before}` 等动作。
- 表格结构写回是一笔 undo transaction；Undo/Redo 恢复源码并重建表格投影，同时清除基于旧源码版本的临时虚拟焦点。后续操作从恢复后的真实选择重新解析，不能复用旧删除目标。
- 不允许单独修改 parser leaf、table rows 或 cell 值作为持久状态。

## 验收用例

- `Vj d p`：删除两行后在当前虚拟行之后插回两行；结果仍是一张表，不 panic。
- `Vj d P`：在当前虚拟行之前插回两行。
- `V d p`：仅一条 body row 的表格可安全删除并插回。
- 表格最后几行被删除后，`p/P` 仍插入 header/separator 之后。
- `Ctrl+v d p/P`：列结构插回同一张表，保持每行单元格数和对齐。
- `V y p/P` 与 `Ctrl+v y p/P`：不先删除时也从当前虚拟 cell/row/column 正确定位。
- 整表删除后粘贴仍生成一张独立表格，不污染相邻段落。
- Undo/Redo 后源码、表格数、行列数、寄存器形状和虚拟焦点均一致。
- `unitAt` 在表格分隔符、行首、表格边缘返回 nil 时，所有操作均安全。

## 当前实现说明

现有 `tableSelection` 和 `tableRegister` 已表达结构选择与寄存器内容；它们会保留。此工程补足结构编辑 session 和操作接口，逐步把删除后的定位与粘贴从源码 caret 推断迁移到虚拟 row/column 状态，不重做普通 Markdown 文本模型。
