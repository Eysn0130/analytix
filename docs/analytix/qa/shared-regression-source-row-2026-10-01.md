# Shared regression 与 source-row 配对验证

Status: Historical / candidate-local evidence；当前候选 CI 与 main 交付另查 GitHub。
Scope: 两个 Vitest 文件的重复执行归属、34 raw 字段名称与引用配对。
Source: `codex/shared-regression-source-row`，base `e73348f891c0fa73111d3efeac02ba61e5c314b5`，
初次源码冻结及 A 观察 candidate 为 `3a7c7bba9938c0d17852cafb52a80d756f687eb4`；
最终源码为 `ca7a73a4e5330bc3127a14538e9ff2900d30b024`，补齐复核发现的取消边界。
配置 Mac、Node 22.22.1、Vitest 4.1.7；
所有编译/测试均在同一 shell source `scripts/use-analytix-cache.sh`，隔离 fixture 使用 `umask 077`。

## 行为与验证

- `p0-packaging-config-contract` 执行 packaging 文件，`packaged-go-boundary` 引用同一
  调用内的逐文件结果；provider/settings owner 执行 settings 文件，MCP UI gate 引用该结果。
  同 cwd/env/tag 下两个文件各执行一次，59 个 named gates 不变。共享 evidence 要求
  唯一文件、非空唯一实际 assertion IDs、全部 passed；失败、缺失、pending、取消不通过。
  两个 owner 专用 JsonReporter 子类追加公开回调的 run reason/module state；整个选择的
  文件集合须完整且结束。正常其他文件失败可以复用，interrupted 与异常退出拒绝。
  其他文件失败仍使原 owner gate 失败，不把该失败伪装为共享文件失败。没有跨调用缓存。
- `SourceValues::raw_fields` 直接配对 raw 名称与成员引用；移除名称表与取值数组的位置 zip。
  34 个原配对逐项相同，SQL key 顺序、`SOURCE_VALUES_JSON_SQL` 字节及其余生产逻辑保持。
  新手写 oracle 覆盖 34 raw + 9 norm，包含 null/空 text、原文金额与精确 decimal；
  account/name/id 三组主体与对手方交换可改变合法摘要，但不符合独立语义真值。

| 检查 | 实际结果 | 边界 |
| --- | --- | --- |
| 初次冻结的三个 Vitest owner 范围 | 67/67、193/193、28/28；288 个唯一 ID 与 baseline 精确相同 | 绑定 3a7；最终修正没有改变这些测试或选择 |
| 最终 product regression 窄行为组 | 19 passed、90 未选 | 14 共享场景加原进程行为组，保留 59 gates；四个新增反例修复前 RED |
| 最终真实 Vitest stdout | 23 个 settings IDs 解析为 passed，与 baseline 相同 | 专用 reporter 返回 passed reason/module state；npm 前缀、Windows slash fixture 保留 |
| 极小真实 interrupted Vitest | 一断言 passed、JSON success=true、exit 1/signal null；reason=interrupted，collector 拒绝 | 使用隔离合成测试与公开 cancelCurrentRun 回调；不是产品通过证据 |
| Rust source-row owner | 8/8 | 含真实 DuckDB、路径/权限边界、raw replay 限制、decimal/null 与独立 sentinel |
| 临时生产引用交换 | RED，exit 101；候选源码精确恢复后 8/8 | 三类身份引用被交换；不靠摘要自洽证明语义 |
| 原 CSV→DuckDB→source-row golden | 1/1，expected 未改 | 原固定摘要 `e092c1b936de8a85eb10747048f77c1343fe0ef8a8b9d22da2da298866c8b354`；原 Go 生成器 provenance 未定位 |
| Go 消费闭包 | 19 顶层与 42 子用例 passed | CSV admission、typed scalar/schema/digest、host-private source resolver；不是 Rust native 全链 |
| web/node typecheck、diff check | pass | 无新 runtime API/权限或 persistence 变更 |

复现命令（Vitest 三组文件参数以冻结脚本对应 owner 的完整列表为准）：

```sh
source ./scripts/use-analytix-cache.sh
umask 077
npm test -- src/main/runtime-go-packaged-contract-report.test.ts --run -t 'product regression|overlapping Vitest'
cargo test --locked --manifest-path tools/analysis_compute/Cargo.toml --lib funds_transaction_source_row_page_v1
cargo test --locked --manifest-path tools/analysis_compute/Cargo.toml --lib source_row_projection_matches_go_canonical_row_golden
go -C packages/runtime-go test -tags analytix_prod -count=1 -json ./internal/domain/nativecomponent ./internal/app/fundsquerysource ./internal/domain/evidence -run '^(TestTransactionSourceRowPageV1|TestControlledAccountFlowParsedRowMustMatchEveryClaimedTransactionField|TestAcceptedSlotSource|TestFundsCanonicalCSV|TestParsedPageV1)'
npm run typecheck
```

合并 12 文件的一次 baseline 与一次 candidate 检查均为 **287/288**：同一 packaging
架构绑定检查在约 5043/5781 ms 失败。原 JSON 只返回 `STACK_TRACE_ERROR`，先前的“5 秒
timeout”归因未由错误正文证实；失败回执仍保留。单项复现与原两文件 owner 67/67
通过没有抹去该失败，也不能豁免候选 CI。没有提高 timeout、删断言或降低 gate。
原“早期失败仍继续”进程 fixture 意外调用真实 Python，已补齐其既有隔离策略。
最终复核还发现原 JSON 丢失 interrupted 状态：非零退出仍可能有全 passed 文件。
新增完成状态后，已覆盖无 signal 取消、异常退出、缺失/未完 module、缺失 run state、
取消前已有其他失败，以及普通 assertion/hook 失败的逐文件归属；当前源码只读复核无新增缺陷。

## 维护观察与限制

固定题目、requested `gpt-6.1-sol/ultra`、10 分钟/20 实际嵌套调用；各会话自行定位，
不执行测试。actual 模型/档位、自动注入指南、隐藏 wrapper 与完整摄入量未知。
下表保留原会话报告的 UTF-8 计量，含重复读取、错误及可取得的截断文本。原 A candidate
把三次 structured clock 值计为 69 bytes，B candidate 则计 0；不跨口径归一化或比较。
纠偏配对统一只计 `exec_command.output` 与 MCP text content，结构元数据计 0、另列 unknown。

| 原会话 | 调用 | 返回文本 bytes | 定位结论 | 首次识别行为检查 UTC |
| --- | ---: | ---: | --- | --- |
| A baseline | 20 | 294872 | 静态 route complete；实际失败原因未知 | 19:06:11.149 |
| A candidate | 16 | 268284 | **无效**：提示沿用 baseline HEAD，读取旧脚本，未定位候选 owner | 20:28:12 |
| B baseline | 20 | 331791 | partial：mapper/已有 golden 定位，独立生成来源未闭合 | 19:06:22 |
| B candidate | 20 | 359581 | partial：识别候选原子配对与 sentinel，原 Go 生成来源仍未闭合 | 20:27:37.185 |

原四会话不作为维护收益成立的依据。A 输入错误由协调者造成，未覆盖或删除旧回执；
B 本次返回文本增加 27790 bytes。首次“识别”是建议行为检查的路由时刻，不是检查运行或通过。
父任务批准仅修复 A 的一次新配对：两个全新会话、各自 clean pinned checkout，首调用机械
验证 cwd/HEAD/目标脚本 SHA256；无已有答案或定位提示。两者首末状态均 clean，baseline
为 base commit，candidate 为初次冻结 3a7；目标脚本哈希各自匹配。

| 纠偏 A 会话 | 调用 | 返回文本 bytes | 定位结论 | UTC 开始 → 结束 | 首次识别行为检查 UTC |
| --- | ---: | ---: | --- | --- | --- |
| baseline | 13 | 287590 | route complete；定位旧故障/取消 fixture，实际失败原因未知 | 20:47:10.565 → 20:53:06 | 20:49:59.945 |
| candidate | 9 | 186861 | route complete；区分执行 owner 与 consumer，定位七场景共享行为 fixture | 20:47:21.320 → 20:51:12.333 | 20:49:47.680 |

这是各一次观测，返回文本差值为 -100729 bytes；含重复读取、错误与已返回的截断告知。
两者未运行行为检查，没有真实失败 assertion，也没有预登记完整真实 assertion 清单；
不能据此证明总体维护收益。candidate 开始时刻在首工具返回后采样，时间仅作审计。
此配对没有覆盖后续 reporter/取消边界修正，不替最终源码建立维护收益结论。
不增加第三轮、不依据正负结果选择性保留，不推导耗时、cache、token、费用或模型分析收益。

初次三项 source/test corpus 为 348376 → 363927 bytes（+15551）；最终含新增 reporter 的
四项为 367217 bytes，比 base 增加 18841。新增测试与结果文档是维护成本，
不能把结构收敛表述为总 LOC/读取字节下降。实际结构结论是两个文件的重复执行归属退役，
以及 raw semantic 名称/引用不再需要两个数组同步；全 Agent 维护成本和长期收益尚未证明。

最终源码 SHA256：script `494ddecc39954c21c59356bd5ecf8ac54301f1b0ade53be58a2302cfb16ff71c`；
report test `a134fe2db9eae33eb75ec969cdd16b06c66bf36e6ee2a9e3f2262c4da8e726f4`；
mapper `cd29cffeccd5e1e41823f4ac3635ba025aadf9a343d8f3fbfe400284e5628a02`；
reporter `74cb69648c0f6248d3ca59a416cbd823b7185d7e56f38c3ab4b49dd948ad9afd`。
小 host receipts 在 `/tmp/analytix-stage2-*`；原始观察对象只保存在会话 store，未完整导出。
四项继承 dirty 保持原哈希。`validation-burden --plan` 对本范围为 unmapped，仍走 affected
owner 与原 CI matrix。未重跑全 native 长测、9 行/7 笔完整产品向量、安装/恢复或正式包；
未调用 paid Provider。OwnerClose 不等于持久撤权，fresh query 不等于数值缓存复用；没有模型 pilot。

<a id="snapshot-d469-consolidation-81"></a>

### consolidation: original lines 81–95

Source: `docs/analytix/document-consolidation-register.md` at `d469a7406d897a97ca7d74f3dbbb29bf635a6ccf`; original block SHA256 `c33132580a29daa0602d08b87f99d967ddfa56028a8bc7a4767432df50f845ca`.

### 2026-10-01 共享测试执行与 source-row 原子配对

初次观察冻结于 `3a7c7bba9938c0d17852cafb52a80d756f687eb4`；最终源码
`ca7a73a4e5330bc3127a14538e9ff2900d30b024` 补齐真实 Vitest interrupted 完成状态。
均基于 PR28 合并 main。
两个重复 Vitest 文件各保留一次执行 owner，59 named gates、实际 288 唯一 ID 与原断言保留；
34 raw 名称与成员引用由同一处原子配对，SQL/协议/金额/null/权限与原固定 golden 未改。
定向验证、实际身份交换 RED→恢复 GREEN、独立观察与 baseline 失败均记录于
[紧凑结果](shared-regression-source-row-2026-10-01.md)。原四会话中 A candidate 读错
baseline HEAD，B candidate 读取反升且 partial；不宣称总体维护摄入或模型收益。唯一一次
clean pinned A 配对纠偏返回文本为 287590/186861 bytes，均定位正确；单样本不证明长期收益。
初次三 source/test 文件 corpus 增加 15551 bytes；最终含专用 reporter 四项增加 18841 bytes，
配对观察未覆盖后续修正；
结构退役重复执行不等于总量下降。四项继承 dirty 保留；当前候选 CI/main 状态查 GitHub，
本记录不授权安装包或正式发布。
