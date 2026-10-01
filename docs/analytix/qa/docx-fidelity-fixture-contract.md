# DOCX 合成 fixture 与语义比较 oracle

Status: Operational，限定为第一方合成 fixture 和 oracle 准备。
Source of truth: [生成器与比较器](../../../scripts/docx-fidelity-oracle.py)、
[合成 ZIP 自测](../../../scripts/test_docx_fidelity_oracle.py)及其 fresh 执行结果。

此工具不启动 Office engine、GUI 或 Provider。自测通过不能证明 native DOCX
保存保真、排版保真、字体可用性或发布验收通过；本文件不是 native 验收报告。
实现仅使用 Python 3.9+ 标准库，无上游样本或第三方代码。

## 生成物与来源

在仓库根目录执行，使用全新任务目录：

```sh
source ./scripts/use-analytix-cache.sh
analytix_docx_fixture_dir=$(mktemp -d "$TMPDIR/docx-fidelity.XXXXXX")
python3 scripts/docx-fidelity-oracle.py generate --output-dir "$analytix_docx_fixture_dir"
```

生成器输出固定 ZIP 时间、排序部件和未压缩 ZIP 数据，不覆盖已有文件：

| 文件 | 明确含义 |
| --- | --- |
| `original.docx` | 第一方合成输入 |
| `expected-edit.docx` | 按指定范围构造的合成预期结构，不是 engine 输出 |
| `edit.json` | 单次预期文本替换的精确选择器 |
| `manifest.json` | 每个产物的 synthetic 来源、角色与 SHA-256；`native_engine_run: false` |

生成器不输出 `noop.docx` 或 `edited.docx`，不制造 native 保存证据。
同版本脚本的两次生成文件应字节一致；输出目录不进入 manifest。

合成 DOCX 的 `word/document.xml` 段落按文档顺序从 0 编号，包含表格段落：

| 段落 | 覆盖内容 |
| --- | --- |
| 0、1 | 两处相同的“重复文本”；中文、粗体、Calibri/宋体声明、颜色和字号 |
| 2、3 | 内部 bookmark 与指向该 bookmark 的 hyperlink，含下划线和颜色 |
| 4 | 简单 DATE field 和带 begin/separate/end 的 REF field，保留指令与显示值 |
| 5 | 跨三个相同属性 runs 的文本；预期只替换其中的“重复文本” |
| 6–9 | 2×2 表格，包含另一处相同文本、单元格文本、表宽与列宽 |

字体名称是 fixture 属性，未验证宿主实际安装了这些字体。

## 精确编辑契约

`edit.json` 只接受 `paragraph`、可选 `run`、`start`、`end`、`before`、`after`。
未知或重复字段、错误类型及越界选择器都失败。示例：

```json
{"paragraph":5,"start":3,"end":7,"before":"重复文本","after":"精确修改"}
```

- 索引均从 0 开始；范围 `[start,end)` 使用 Unicode code point，**不是 UTF-16
  code unit、Word selection offset 或字节偏移**。
- 无 `run` 时，偏移基于该段落所有 `w:t` 文本串联；有 `run` 时，`run` 是原始
  段落中 `w:r` 的文档顺序索引，偏移仅相对于该 run 的 `w:t` 文本。
  选择器始终先绑定 original，不按相同文本搜索其他位置。
- 只支持非空源范围的文本替换/删除；零宽插入拒绝，避免 run 边界的格式归属歧义。
  相同 `before`/`after` 保留原运行结构，不执行无意义的跨 run 重分配。
- 受选范围必须位于段落直接拥有的纯文本 runs；每个 run 只能有可选 `rPr` 和一个
  `w:t`。不能跨 bookmark、hyperlink、field、drawing、tab 或 break 等结构边界。
- 跨 run 替换把 `after` 放入第一个受选 `w:t`，从其余受选节点删除对应文字；
  保留所有 run、`rPr`、`w:t` 属性与未选中的前后缀。默认不会删除空 run。
  这是显式的预期分配规则，不推测 engine 应如何继承不同 runs 的格式。
- `before` 必须精确匹配。no-op 若已改变受选文字，会报告该基线漂移及无法应用
  原预期，不会重新定位或把目标移到第二处相同文本。

## 三文件比较与结论

比较器要求调用方明确提供三个输入，不推断任一文件由 engine 生成。
以下命令**仅是纯合成 oracle 自检**：显式用 original 充当无变化对照，用
expected-edit 充当合成 edited 输入，不产生 native 验收证据。

```sh
python3 scripts/docx-fidelity-oracle.py compare \
  --original "$analytix_docx_fixture_dir/original.docx" \
  --noop "$analytix_docx_fixture_dir/original.docx" \
  --edited "$analytix_docx_fixture_dir/expected-edit.docx" \
  --edit-json "$analytix_docx_fixture_dir/edit.json"
```

将来验证产品 engine 时，另行取得真实“打开后不编辑并保存”的 no-op 文件，以及
执行指定选择区替换后的 edited 文件，再明确传入它们的路径：

```sh
python3 scripts/docx-fidelity-oracle.py compare \
  --original "$analytix_docx_fixture_dir/original.docx" \
  --noop /path/to/actual-engine-noop.docx \
  --edited /path/to/actual-engine-edited.docx \
  --edit-json "$analytix_docx_fixture_dir/edit.json"
```

该示例中的 engine 操作未由本工具执行或证明。实际证据还须记录 engine 版本、
平台、原文件与产物哈希、选择区和保存过程。不得将 `expected-edit.docx` 改名后
当作真实 engine 输出。

JSON 报告始终分开给出：

1. `noop_vs_original`：无编辑保存相对原文件的语义漂移。
2. `edited_vs_noop`：仅允许所声明的文本替换，其余语义相对 no-op 的漂移。
3. `inputs`：三份实际读取的 ZIP 字节数与 SHA-256，标为 `caller-supplied`，
   `native_provenance: not-established`；另给语义快照哈希和变化部件名，不回显正文。

总 `ok` 要求两次比较都通过。即使 edited 完整继承了 no-op 已损坏的样式、链接或
字段，`edited_vs_noop` 可通过，但总结果仍失败。省略 `--edit-json` 时，edited
也必须与 no-op 无语义漂移。退出码：`0` 匹配；`1` 发现漂移；`2` 输入不安全、
不支持或参数无效。

## 保守的语义边界

所有 XML 部件都比较 expanded 元素名、全部属性、文本和子节点顺序，保留注释与
processing instruction；二进制部件比较 SHA-256，部件增删也算漂移。XML 缩进、
属性排列及等价空元素写法不算漂移。`rPr`、`pPr`、link/field 结构、表格属性、
样式部件、未选区或元数据变化均不会因显示文本相同而被抹除。

默认保留 run 边界。只有明确传入 `--allow-equivalent-run-splits` 时，才对所选
段落允许相邻纯文本 runs 的 split/merge；它们必须具有完全相同的 run 属性、
完整 `rPr` 结构及 `w:t` 属性。段落必须仅含 `pPr` 和这些纯文本 runs，不接受
hyperlink、bookmark、field 等边界。其他段落仍严格比较，不做全局 run 合并。
不同字号/颜色/字体的 run 不会被合并；纯文本相同也不能掩盖属性丢失。

这是有限 fixture oracle，不是完整 OOXML schema validator、排版模型或通用
DOCX 修复器。未声明的 engine 重写（包括某些无害的元数据/结构重排）可能被保守地
报为漂移；应检查证据，不能临时放宽 oracle 来制造保真通过。

## 输入预算与自测

不解压到文件系统。拒绝外部或非规范 relationship target、缺失目标、ZIP 重复
条目（含大小写别名）、路径穿越/绝对路径/反斜杠/百分号别名、目录/特殊文件条目、
加密或未知压缩、CRC 错误、DTD/实体声明、非 UTF-8 XML、畸形 XML。
输入 ZIP ≤8 MiB、≤64 条目；单部件 ≤2 MiB、总展开量 ≤16 MiB、展开比 ≤1000；
每包 XML ≤50,000 节点、深度 ≤80。预期文本每字段 ≤65,536 code points，
edit JSON ≤512 KiB。

```sh
source ./scripts/use-analytix-cache.sh
analytix_docx_test_dir=$(mktemp -d "$TMPDIR/docx-oracle-tests.XXXXXX")
TMPDIR="$analytix_docx_test_dir" python3 scripts/test_docx_fidelity_oracle.py -v
```

测试使用真实第一方合成 ZIP 文件，覆盖确定性、精确重复文本/跨 run 编辑、两层
漂移判定、保守 split/merge，以及样式/段落/链接/字段/表格/未选区 corruption
probes。链接和简单 field 的损坏探针保留显示文字，仅移除结构，避免把“文本变了”
误当作结构保真验证。另覆盖输入拒绝与 CLI 的 `0/1/2` 退出码。
