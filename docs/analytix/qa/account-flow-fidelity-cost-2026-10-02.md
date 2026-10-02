# Account-flow 保真与本地 page 成本

Status: candidate-local evidence；不是模型优势或安装／发布验收。
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
| ordinary lifecycle missing/disabled/unauthorized | 第一次因 schedule MCP entrypoint 缺失全部 SKIP；补齐 source node bundle 后仍因 MNT_REMOVABLE profile 全部 SKIP；隔离 host profile 定向结果待终态 |
| diff/type/schema | 无公开字段或 TS consumer 改动；`git diff --check` pass；affected-owner plan 为 unmapped，未据此跳 gate |

首次 fmt check 失败，测量尚未启动；仅格式修正后执行。sandbox 内分支/cache authority
查询受阻，正规 escalation 成功；没有更改权限。一次向父线程发状态的请求被 auto-review
拒绝，原因是委派上下文未被认可为可信终端用户跨线程通信授权；未绕过，实施不依赖该发送。
四项继承 dirty 原 SHA 保留，不进入任务提交。actual model/effort/full-access 都 unknown，
requested `gpt-6.1-sol/ultra` 不等于实际执行事实。

## Native 与 currentness 的结论边界

Go 独立 delivery CSV 使用原 fixture 的既有账户派生，SHA
`39e764977ee70279bd757d8d85947d5f98a3e6f16abdf1bf9975675e39bc4382`；
它与 Rust 的既有派生账户／CSV 表达不同，不能称为同字节输入。
真实 Rust importer/query 与 Go gold/typed consumers 分别通过，不拼成完整 native A/B PASS。
完整链仍须 exact-clean current-source package 与 fresh recovery 的一个候选回执。
旧完整 native codesign/recovery timeout 没有被本次 0.04s seam“修好”。
Thread B 仍为 fresh authorized native query；A 的 currentness metadata 不是数值 cache。
OwnerClose 只证明 SourceUnavailable；DSV2/grant/head drift 的 existing fail-closed tests
不等于持久 DSV2 revoke 的完整 public-chain PASS。模型只获 authorized projection；
alias 不是 capability，原文展示仍按 exact refs 回查。

## 待授权的窄同模型对照

只准备协议，不调用 Provider。建议用户指定同一 connected local Provider 的精确
model ID 和 reasoning；OpenAI 可作为候选 Provider，但当前可用 ID、价格与货币上限
仍待核实／授权。不使用本工具会话的 model 名冒充 Provider API ID。

- 4 个任务：精确账户收支；逐笔／重复多重集与 exact refs；A1→A2 currentness 与
  历史限定；B1 独立 entity／授权否定。复用原 CNY positive control 与 B1/A2 source-only
  holdouts；预先锁 source SHA，holdout expected-queries/gold 不进入模型上下文。
- 两臂各两次，共 **16 runs**；相同 model/reasoning、任务、seed/order、source-safe
  input、工具白名单与 effect grants、wall time、tool-call 和 token 上限。强 baseline
  能用同样的本地 exact CSV/DuckDB/source verification 工具；不能用无工具 baseline。
  唯一实验变量是既有 Funds instruction/context packing/currentness 使用；若冻结输入
  与工具后实际没有这一差分，不运行或声称优势。
- 每 run 最多 8000 input + 2000 output tokens、10 tool calls、60s；总上限
  **128000 input + 32000 output = 160000 tokens，160 tool calls**。价格 unknown；
  调用前必须得到 provider/model、合成字段、16 次及 token／货币预算的具体授权。
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
