# Account-flow 保真与本地 page 成本

Status: source slice verified；full native vector **failed**，Draft PR38 的合并验收仍 blocked。
不是模型优势或安装／发布验收。
Scope: 现有 CSV → DuckDB → source-row 的 page 选择与编码，及相关 Go 消费边界。
Base: PR37 main `28dbaface7cdab09adb896095ed138da0bea927e`，
branch `codex/account-flow-fidelity`。源码、native 冻结与 GitHub 终态分别记录。
Target: 用户本阶段有限合同及 accepted `case-evidence-kernel` 的 optional
general Agent、host receipt 和 exact-ref local display；未扩展财务产品或协议。

## As-built 与改动

旧 builder 对每个 prefix clone 全 inventory/snapshot/rows，生成完整摘要并序列化。
现在保留所有前置 source/schema/FPC1/currentness 检查和原顺序；每个 prefix
精确计 row JSON、逗号、真实 cursor、complete flag 与固定 64-byte digest 宽度，
遇到第一个超限仍停止。选中后才构造 canonical page digest 一次。不二分，不缓存权限。
原 canonical encoder、schema、43-field sentinel、Go golden 和预期值未修改。

独立 selection oracle 保留原线性算法，逐字节比较返回 wire、Go-compatible bytes、
cursor、digest、错误；覆盖 0/1/100/101 rows、unicode/HTML/JS separators/quote/backslash、
null/空串/精确 decimal、预算 −1/等值/+1、超过 1 MiB。既有源错误、provenance、
row/cell 上限及 valid-digest identity-swap 反例继续生效。

## 有界成本观察

配置 Mac arm64；Node 22.22.1、Go 1.26.4、Cargo 1.94.1、DuckDB crate
`1.10504.0`（Cargo.lock 不变），Rust debug test profile。每条 build/test 同 shell
source `scripts/use-analytix-cache.sh`；未清 compiler 或 OS cache。
“first open” 是真实 CSV import 后的首次 readonly 调用，**不是** OS-cache cold。
warm 是同进程后续七次。没有 paid Provider，token 与全部 SQL statement 数 **unknown**。
page invocation 不是 SQL 次数；无 SQL 次数下降声明。

输入复用 `core-funds-20260921` 原合成 vector 的既有 CNY 派生 importer：
9 rows CSV SHA `a62109cfe02505ae1c86b1cdfef7dc0e82785f363ce25b3acd2cb0a7ef68410f`；
100 rows 成本控制仅循环这些特征并给 observation 独立 ID，CSV SHA
`46904ca30499531a8d9ceacf136043e88cdbd2a83481e05eaf1b7cbe3e354e27`。
两次数据、producer ID、final digest 与 final wire 完全一致。

| 中位数，ms | 9 rows | 100 rows |
| --- | ---: | ---: |
| baseline prefix clone | 0.088 | 10.092 |
| baseline build + canonical digest | 13.906 | 1464.445 |
| baseline prefix serialization | 4.314 | 456.251 |
| baseline prefix total | 18.389 | 1936.545 |
| baseline readonly page return | 824.125 | 2798.338 |
| candidate readonly page return | 854.321 | 951.030 |
| candidate process paired legacy construction | 19.390 | 2001.718 |
| candidate selected construction + final serialization | 4.583 | 49.032 |

100-row readonly return 本次约 −66.0%；配对构造约 −97.6%。9-row readonly return
反而约 **+3.7%**，保留这个不利观察；只可说局部构造减少，不能称所有数据规模
端到端变快。样本各七次，无长期／正式包性能推论。selected replay 还包含一次
test-input clone；readonly return 不含下游 command 最终 serialization。
baseline 每轮 prefix wire serialization 为 172895 / 17305934 bytes；final wire
仍为 32613 / 340298 bytes。减少的是重复工作，不是返回信息。

## 实际工作账与验证

host 原始回执仅在 `/tmp/analytix-stage3-*`。baseline/candidate cost 各执行一次，
各含两次 CSV import、16 次 readonly page invocation、每规模七轮 legacy replay；
candidate 另有七轮 selected replay。额外 full-page digest probe、比较与日志是测量负担。
baseline 编译 149s、cost test 53.45s；candidate owner 重编译 61s、边界 owner
63.35s、vector 14.36s、golden 4.29s、cost test 37.19s。cache preflight、读取、
其他 Go 命令与装配成本不在这些内部数值里，不能从它们推导整阶段总成本下降。
新增测试与代码体积也不是免费的；没有总体维护成本下降声明。

| 命令／证据 | 实际状态与限制 |
| --- | --- |
| `cargo test --locked --manifest-path tools/analysis_compute/Cargo.toml --lib funds_transaction_source_row_page_v1` | 9 passed；含新 selection oracle 与原完整性错误 |
| 同上 `--lib delivery_vectors_ -- --nocapture` | 2 passed，手动 cost diagnostic 默认 ignored；真实 importer、DuckDB query/reopen、原 bytes/hash 保持 |
| 同上 `--lib source_row_projection_matches_go_canonical_row_golden` | 1 passed；原固定 golden 未改 |
| 同上 `--lib delivery_vectors_source_page_cost -- --ignored --nocapture` | baseline/candidate 各 1 passed；两种规模均 final bytes/digest 等价 |
| 临时 cursor budget +1 byte mutation / exact restoration | independent selection oracle RED exit 101；源码 SHA 逐字节恢复后关键 selection 1/1 passed |
| Go `domain/nativecomponent app/fundsquerysource app/datasetsnapshot app/turnsecurity app/executiongrant` | 5 packages、483 named tests/subtests passed，0 skipped；source refs、alias、DSV2/head/witness/material drift、epoch/grant fail-closed |
| tagged Go independent CSV/gold/recovery contracts | 9 导入行、7 target observations，CSV + big.Int 独立收入 **1301001**、支出 **120060**、净 **1180941** 分；原多重集/金额/币种/方向变异检测保留 |
| `TestExecuteDarwinCodeSignSystemToolUsesPinnedAuthority` | pass，0.04s；5s pinned codesign system-tool seam，不是 app seal／完整恢复 PASS |
| ordinary lifecycle missing/disabled/unauthorized | frozen production `5d7fde6` + failure-only test overlay、fresh host profiles、cache preflight PASS：missing 338.16s / 15 operations、disabled 167.02s / 10、单独 unauthorized 171.23s / 10，均完整 PASS；所有原 30s history / default 10m process 限制保留 |
| diff/type/schema | 无公开字段或 TS consumer 改动；`git diff --check` pass；affected-owner plan 为 unmapped，未据此跳 gate |

首次 fmt check 失败，测量尚未启动；仅格式修正后执行。sandbox 内分支/cache authority
查询受阻，正规 escalation 成功；没有更改权限。一次向父线程发状态的请求被 auto-review
拒绝，原因是委派上下文未被认可为可信终端用户跨线程通信授权；未绕过，实施不依赖该发送。
四项继承 dirty 原 SHA 保留，不进入任务提交。actual model/effort/full-access 都 unknown，
requested `gpt-6.1-sol/ultra` 不等于实际执行事实。

通用能力工作账保留：缺 schedule entrypoint 与 MNT_REMOVABLE profile 的两轮全 SKIP；
早期 canonical 三项合跑 600.596s 总超时，且含继承的 dirty recovery source，
不计 clean-source 验收；clean-source 单独 unauthorized 87.177s 失败于 Coding
history 的 30s 窗口。Guard 原来没有取消时间戳，不能判定取消是首因；本次只在现有
测试补 teardown 前有限计数／closed status，并让 `aborted` 立即失败，不增加公开诊断。
最终合规合跑 600.441s 中 missing/disabled 已通过、unauthorized 在 jobs 阶段耗尽
累计预算；仅补跑未完成 case，174.804s package wall。不能把这些负担从效率账删除。

装配工作账保留：四个 native components 用 release 编译 profile、development
nonpublishable authority 构建，app build 已完成；最初缺本地
electron-builder 入口，改既有 pinned 26.15.3 后正常 legal gate 拒绝依赖 symlink。
失败 app 误放严格 evidence 目录，令后续 helper 拒绝；shell 没立即退出却执行了
55.959s diagnostic，**该 PASS 排除验收**，后续受影响命令已停止。失败 app 原字节／
权限保留到 task-owned builds 路径，helper 恢复 PASS，后续 source 必须 `|| exit 70`。
全部 SQL/token 数依旧 unknown；这次错误可能使用的存储环境未获合规确认，不称为
正常 cache-qualified 回执。随后隔离 `npm ci --offline` 通过（11m），app 重建
27.45s；pinned electron-builder 26.15.3 正常 legal／signature gate 通过，得到
clean nonpublishable directory app，未安装／发布。native source 冻结在
`5d7fde6e8ce137cffe2f6e1594900d63396095dd`；后续只有测试诊断和本报告更新。

首次 minimal Owner 命令缺正常 full-content/ad-hoc linker identity，5.25s 整项
失败为 `native_component_host_unavailable`；不是已定位的产品 codesign 缺陷。
按既有 after-pack/test 编译身份补齐后，package inspection 与 Owner admission
11.60s 通过；保留原 manifest/seal/source 检查。只追加一次完整 native public-chain，
保留原 child 11m／parent 12m 时限。该完整链终态另记，不能借旧片段拼 PASS。

## Native 与 currentness 的结论边界

Go 独立 delivery CSV 使用原 fixture 的既有账户派生，SHA
`39e764977ee70279bd757d8d85947d5f98a3e6f16abdf1bf9975675e39bc4382`；
它与 Rust 的既有派生账户／CSV 表达不同，不能称为同字节输入。
真实 Rust importer/query 与 Go gold/typed consumers 分别通过。唯一 current-source
native public-chain 已实测 A/B：9 行导入、7 target transactions、独立 gold
1301001/120060/1180941 分；2/2 native semantic requests、两个 signed preparations、
逐笔／多重集／exact refs／source locators／两段 transformation lineage 全部一致。
A current metadata 出现在 B；reopen 已恢复 2/2 candidates、零 held，并保留原文
exact-ref display。真实 OwnerClose 后重试零新增 Provider dispatch／preparation，
public hydration 23.212s、返回 SourceUnavailable。fresh-process recovery 终态另记；
唯一完整 native vector 最终 **FAIL 976.39s**（package wall 979.856s），不是 timeout。
fresh child 的 2/2 fact admission、normal Owner、assembly、thread1 readback/display
与 runtime/Owner 正常关闭均有 phase；thread2 `thread-get-end` 后没有第二次 display
或 `readback-pass`。因此没有完整 fresh-process recovery PASS；不把旧 timeout 称为已修复。

有界定位：失败位置限定到旧 B final 的 identity/history label 校验（JSON 来自已解析
HTTP，privacy sentinel 只记 t.Error 而不中止）。第三个 SourceUnavailable turn 改变了
B 的当前上下文；既有 `sameRetainedFactScopeV1` 包含同 snapshot 的更高 epoch，
而 vector 的 B expectation 仍为空 HistoryState。两个既有纯 Go projection tests
（stale epoch 隐藏与 exact retained admission）在 clean 5d7 上通过，package wall
0.220s；这支持 history-label 假设，但子进程故障文字被既有安全 journal 丢弃，
不能据此断言实际失败类别或排除伴随的 privacy t.Error。相关 producer/helper 与
PR37 base 的源码相同；本轮不改原 expected/golden，不再次跑整套 native，不宣布
产品恢复已修复或完整隐私验收。下一有界 seam 是仅保留 closed failure 分类的
第二线程 readback 最小复现；其通过前，当前 Draft 不 merge。

native 的四次合成 loopback dispatch 均完整捕获：request JSON 共 92646 bytes，
messages/tools JSON 共 92314 bytes；context A/B 为 296/2879 bytes。均不是 tokens；
没有整体 context 节省或模型效用声明。真实 SQL statement 次数仍 unknown。
Thread B 仍为 fresh authorized native query；A 的 currentness metadata 不是数值 cache。
OwnerClose 只证明 SourceUnavailable；DSV2/grant/head drift 的 existing fail-closed tests
不等于持久 DSV2 revoke 的完整 public-chain PASS。模型只获 authorized projection；
alias 不是 capability，原文展示仍按 exact refs 回查。

## 集成状态

Branch `codex/account-flow-fidelity`；[Draft PR38](https://github.com/Eysn0130/analytix/pull/38)。
生产/native source freeze `5d7fde6e8ce137cffe2f6e1594900d63396095dd`：Development CI
`36947013605` 52/52 success；CodeQL `36947008655` 四项 Analyze 与 aggregate success。
两轮只读审查未发现 selector 或 failure-only generic diagnostics 的阻断；不是 GitHub approval。
最终 task-owned test/report commit 的当前 CI/CodeQL 以 PR exact HEAD 回执为准。
原四项 dirty SHA 未变、未提交；无 main 合并／postmerge PASS 声明。

## 待授权的窄同模型对照

只准备协议，不调用 Provider。建议用户指定同一 connected local Provider 的精确
model ID 和 reasoning；OpenAI 可作为候选 Provider，但当前可用 ID、价格与货币上限
仍待核实／授权。不使用本工具会话的 model 名冒充 Provider API ID。

- 4 个任务：精确账户收支；逐笔／重复多重集与 exact refs；A1→A2 currentness 与
  历史限定；B1 独立 entity／授权否定。复用原 CNY positive control 与 B1/A2 source-only
  holdouts；本次真实 importer 冻结的 B1 CSV SHA 为
  `1f74d5c01108d1aaf117f9643966ec391ab55689f4f70bc32dfac0793608e55b`，A2 为
  `e8bdf1d9b645c761c5b867cfd5bc5a68657141e02afdb56b410980fc16312afe`，各 1 row。
  原 facts SHA `6894ac103a21260de3260aba39a49c15749ee2ec1db52842f5764abc6fc3cc82`、
  expected-queries SHA `c70d888a3602725bfb0c22801989596d776e3a2db2f43fdf101738e27deb5cef`
  不变；expected-queries/gold 只给独立 scorer，不进入模型上下文。小 holdout 仅适合
  bounded pilot，不能据此泛化到真实案件或复杂财务任务。
  两臂使用 fresh model threads 与隔离的 input workspace；不继承实现对话、QA 或
  测试答案。只授予 source inputs 与已声明 query/verification 工具的既有 effect
  scopes，oracle 留在两臂可读／可执行范围之外。若现有权限机制不能证明这项隔离，
  不执行或声称 source-only holdout；不能仅靠提示模型“不要偷看”。
- 两臂各两次，共 **16 runs**；相同 model/reasoning、任务、seed/order、source-safe
  input、工具白名单与 effect grants、wall time、tool-call 和 token 上限。强 baseline
  能用同样的本地 exact CSV/DuckDB/source verification 工具；不能用无工具 baseline。
  唯一实验变量是既有 Funds instruction/context packing/currentness 使用；若冻结输入
  与工具后实际没有这一差分，不运行或声称优势。
- 每 run 最多 8000 input + 2000 output tokens、10 tool calls、60s；总上限
  **128000 input + 32000 output = 160000 tokens，160 tool calls**。价格 unknown；
  调用前必须得到 provider/model、合成字段、16 次及 token／货币预算的具体授权。
  输入上限含完整 system/tools/history/source；选定 model 后先做 tokenizer preflight。
  必需语义或工具 schema 放不进预算就阻断并重新申请具体预算，不能截断字段来凑数。
- 仅可发送授权的合成 safe字段：opaque entity/ref、整数 minor amounts、CNY、direction、
  timestamp/UTC含义、multiplicity、否定／限定语义与 source/version/currentness metadata；
  trusted-local synthetic name/account/memo 原文留在本地，exact-ref display 单独评分。
- 独立离线 scorer 复用 Go CSV + big.Int／transaction multiset，以及原 refs/lineage
  assertions；逐项评分金额精度、币种、行粒度、方向、时间、entity、否定／限定、授权。
  盲化 arm 标签后评分；failed/refused/missing/timeout 全保留，source mutation 应抓错，
  固定所有 work 包含重试／score／measurement，不按有利结果筛选。

Gap: 没有真实同模型执行、source-only holdout 答案或优势结果。合成 HTTP Provider
只验证 runtime 生命周期／投影，不证明模型更强。无付费调用、真实案件数据外发、
凭据复制、DMG/Windows 正式包或发布。
