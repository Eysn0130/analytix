# Account-flow 保真与本地 page 成本

Status: source slice verified；original full native **failed**，修正后唯一完整链外层超时；
CDP fixture 已确定化；留存 fixture 的 original witness 前置 seam 不可用，native
分阶段验收仍 blocked。旧 Rust incident 保留 UNKNOWN，停止无信号复现。
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
不能据此断言该次实际失败类别或排除伴随的 privacy t.Error。该次 freeze 的
Go recovery/history/turnstart/helper 与 PR37 base 相同；Rust production page-builder
确有本期改动，不能把整个失败归为历史问题。该次原完整 vector 的 expected/golden 未改。

本次续行最小诊断（canonical `1a8d40c69` 上的 test-only overlay，production 与
clean package 仍为 `5d7fde6e8`）：closed classifier／per-thread readback／expectation
bounds／private journal 4 top-level、8 subtests **PASS**，package wall 10.952s。
只运行一个 B 线程的原 9-row CSV：一次 fresh native numeric final → 实际 OwnerClose
→ SourceUnavailable retry → 同进程 reopen；**PASS 323.26s**（package 323.864s）。
严格 parser 验证原 final 与 digest 未变，旧 empty HistoryState 在 reopen 前后均得到
closed `final-history-label-mismatch`，实际为 `retained_snapshot`。私有 frozen contexts
确认 ThreadID／case binding／DSV2／manifest 相同、epoch 提高、risk policy digest 改变；
只记录封闭分类，不保存私有 record 或正文。两次 synthetic Provider dispatch、一份
native semantic；OwnerClose 后零新增 dispatch／preparation，所有 privacy 断言通过。
该诊断未独立比较完整 principal/workspace scope 或 reopen 前后全部 frozen contexts；
没有 fresh-process/display 第二次整链验收，不回填旧 FAIL 缺失的错误分类。
accepted target 要求原 epoch/snapshot 与授权的历史投影；same DSV2+higher epoch 的
保守 retained 语义由现 owner 实现。旧 vector 的阶段／状态期望存在具体 harness gap，
协调方在该实际诊断后明确准入最小 harness 修正：仅第三回合 OwnerClose 后的旧 B
要求 exact `retained_snapshot`；新增 numeric final 的 pre-current 与 A 的 current 状态、
原 digest、严格 parser、privacy、exact refs 和 fresh-process display 均保留。
新增 scope guard 检查完整 principal/workspace/case、原 DSV2/manifest、更高 epoch、
Host policy 与 case-evidence 权限；minimal reopen 比较两个完整 frozen contexts。
这些新增断言不回填上面的 323.26s 回执。修正后同一 closed-contract 4 top-level／
8 subtests **PASS 10.629s**。未改生产 epoch、信任或授权。

只读检查 clean cache 的 3594 个相关 tracked inputs：非 test 差异为零，无额外
Go/Rust source，只有三个当前 Go test overlay，均与 canonical 字节一致。
正确 `stage3-5d7fde6e8-clean-dir` app 的 raw runtime/analysis/authority SHA 未变。
可复用该包验证同一 5d7 production generation 的新 harness，不重复装配；这不是
新 test/report HEAD 的 exact package、安装或发布验收。canonical 的受保护 dirty
`recovery_plan.go` 仍不进入 clean production 或任务提交。

该修正后只追加一次完整链：同一 full/ad-hoc compiled identity、fresh 0700 Host profile、
clean 5d7 production + 三个 test overlays、原 child 11m／parent context 12m／outer 20m；
终态 **FAIL，package wall 1200.458s，outer timeout**。A/B 的两次 fresh native query、
9/7 行、独立 gold、多重集／exact refs／lineage、同进程 reopen 2/2／零 held、
原文 display 与 SourceUnavailable 均通过。新增完整 scope guard 实测 original epoch
**1 → 2**，principal/workspace/case/DSV2/manifest 保持；原 B 严格 parser/digest 与
exact retained 标签通过，OwnerClose 后零新增 dispatch/preparation。
fresh child 的 bounded journal 仅到 `assembly-begin`（input-ready、lease-begin/end），
没有 fact admission、正常 Owner、两个 display 或 readback-pass；不拼旧链片段为 PASS。
本次未命中任何新增 readback failure class，不能断言旧 976.39s 的实际类别已补齐。
只读检查后相关测试／runtime/native 进程为零；未延长时限、重启或再跑完整链。

一秒采样与 40-frame addr2line 是诊断负担：出现 private-authority extended-security
检查与 runtime waits，不是 codesign failure、CPU profile 或单项 SQL 时间的证据。
outer-timeout stack 处于父进程 `b1AssertFreshProcessRecovery`／exec wait，另有
private CAS inventory 工作；child assembly 的具体等待／错误原因仍 **UNKNOWN**。
一次结束后 lsof 返回 exit 1，后续 inventory 证实进程已退出，未当作权限拒绝。
新 full receipt 72045 bytes、SHA
`13955fa13ee4b0667784be7c971b71721fa98c97b2704ed98656be8bef0afb14`；
closed child journal 293 bytes、SHA
`d250d8eceef0d27f3286ed68ef0b4221b697e220a96783b586e8aa23926fd6e0`。
新增原始回执另存私有 `stage3-account-flow-1a8d40c69-r2`，原归档未改。
缺口是 child assembly 的可区分证据及完整新进程验收；当前 Draft 仍不 merge。

native 的四次合成 loopback dispatch 均完整捕获：request JSON 共 92646 bytes，
messages/tools JSON 共 92314 bytes；context A/B 为 296/2879 bytes。均不是 tokens；
没有整体 context 节省或模型效用声明。真实 SQL statement 次数仍 unknown。
Thread B 仍为 fresh authorized native query；A 的 currentness metadata 不是数值 cache。
OwnerClose 只证明 SourceUnavailable；DSV2/grant/head drift 的 existing fail-closed tests
不等于持久 DSV2 revoke 的完整 public-chain PASS。模型只获 authorized projection；
alias 不是 capability，原文展示仍按 exact refs 回查。

## 本轮有界修复与留存阶段入口

候选 `3042b4d31` 的 Source baseline job `110708817191`：133 PASS／1 FAIL，
CDP startup 最后阶段为 `cdp_handshake_timeout`，预期 `runtime_bridge_missing`；
302.316ms 内失败。原日志没有首个观察／catch 时钟，具体触发时序仍 unknown。
仅现有 VM fixture 注入受控 `Date.now`，明确一次 bridge reply 后 stalled handshake
到原 deadline；保留 300ms caller／700ms watchdog、两个 exact expected 和只读 GET。
默认 fixture 仍用真实 Date，owner SHA
`ff370d6b1825166f14e384d2e740442975cb5dd43fa7830b0b15369f843bf61c` 不变。
VM 内删除阶段保留分支的 mutation **RED 1／304.812ms**；真实 owner 的窄 case
**GREEN 1／304.902ms**。最终 `npm run test:baseline` **134 PASS／0 skip，32.300s**。
该 mutation 证明断言抓错，不把真实 owner 宣称为已修复的生产回归。

原 R2 留存 input 0600／3006 bytes、SHA
`7360e402d00b0b5879367f2bdb354d5ffc50c4d63def497fd845ab763c827754`，两个独立
thread/turn 的持久化 digest 匹配，A current／B retained_snapshot，`Config.APIKey` 为空。
复用生产 freeze `5d7fde6e8` 与原 native SHA `d0c9ff29…512b50`；新增内容仅为
canonical Go test overlays。原完整命令在进入 fresh child 前已耗约 17.5m，外层约
剩 151s，不能据此认定 child 自身超过原 11m／12m 阶段限制。
现有 `TestFundsRecoveryFreshProcessHelper` 已可通过
`ANALYTIX_FUNDS_RECOVERY_PROCESS_INPUT` 独立调用，保留 `-test.timeout=11m`。
现于 lease／assembly 前用 OpenExisting 原 key → anchored LoadCurrent → 签名随机
nonce → 原 mTLS `Observe` 做 10s 有界只读 prerequisite；不创建身份或 Advance。
live／unavailable／missing key／另一安装 key 四向量 **PASS 1 top／4 sub，0.736s**，
缺失目录与原 protected stores 不变；原四项严格 readback/journal tests 仍 PASS。
留存 input 的实际独立调用 **FAIL 0.01s（package 0.040s）**，closed journal 为
input-ready → witness-begin → **witness-unavailable**；未执行 assembly、native、
admission、display 或 Provider，不能称阶段 PASS。其原 544 files／2020066 bytes
前后 tree SHA 同为 `efb1811204bcb97ffedb9b19b2dc482bd50cb17d2518c34771ac146544cd57f7`。

原 `formalauthority` fixture 仅内存持有随机 witness/server/CA 私钥和 checkpoint；
落盘 client 凭据不能恢复签名者。原两个 loopback endpoints 均 connection refused，
没有现有 restore seam；不能新建 genesis／替换 manifest 或重放旧 observation。
本次 unavailable 只证明当前 prerequisite 缺失，不能回填旧 976s／1200s 的因果。
原 fixture 已无法满足 fresh 2/2 admission/readback/display；恢复 live 原 authority
或另行授权新的前序 fixture 前，native 验收与 merge 保持 blocked。
执行负担：baseline 1、窄 case 2（含 mutation）、Go 编译 2、原合同组 1、补强后的
preflight 组 1、留存 stage 1；无第三次完整链或本轮 Rust stress。完整编译 wall unknown；
原合同组 package 16.729s，实际退出和不利结果均保留于独立 private R3 回执。

## 集成状态

Branch `codex/account-flow-fidelity`；[Draft PR38](https://github.com/Eysn0130/analytix/pull/38)。
生产/native source freeze `5d7fde6e8ce137cffe2f6e1594900d63396095dd`：Development CI
`36947013605` 52/52 success；CodeQL `36947008655` 四项 Analyze 与 aggregate success。
两轮只读审查未发现 selector 或 failure-only generic diagnostics 的阻断；不是 GitHub approval。
最终 task-owned test/report commit 的当前 CI/CodeQL 以 PR exact HEAD 回执为准。
原四项 dirty SHA 未变、未提交；无 main 合并／postmerge PASS 声明。
当前 `1a8d40c69` 的 CodeQL `36956724184` 四项 Analyze success。
Development `36956726587` 的 fresh API 终态为 **50 success／2 failure**，共 52 jobs，
无未完成项；失败为 Rust 与 aggregate Development gate。Rust job `110681212846` 的原
`query_stats_rows_cli_search_pages_keep_total_and_stable_tie_order` 在
`stats_rows.page_values` 触发 DuckDB INTERNAL index 0/vector size 0，exit 101；
95 integration PASS／1 FAIL，lib 294 PASS／1 ignored。原 SQL/fixture/Cargo lock
没有本期改动；仍不能据此称历史或 flaky。原日志 105611 bytes、SHA
`31cd09fc7c649cc328d711cf1cbfe428e8333140c6f9b1fa4d44580fe24ab8e4`。
该 test 仅新增 offset 的失败 context，原 SQL、排序、gold、预期不变。
本地 exact CLI **PASS 1／209.27s**，编译 1m54s；一次 1s CLI sample 只有
`_dyld_start`／112KB，不能算 DuckDB SQL 等待或 codesign 根因。
既有 plan-probe owner 加真实 search/tie/offset 0/1 对照：原 window/default 8 threads、
test-only 1 thread、仅 total 列替换为同 session 实际 count，生产 SQL 未改。
初次 raw-key probe 的独立 expected 错用了 CLI id 前缀，**3 FAIL／2.68s**，保留；
纠正 fixture 读取契约后 **3 PASS／6.64s**。完整 `query_stats_row_values` consumer
及 JSON row/summary 的 6 pages **3 PASS／5.16s**，DuckDB `v1.5.4`；最后 wrapper
resume transport 失败，shell exit **unknown**，完整 test 回执 PASS、fresh 相关进程为零。
这组成功不能证明 Linux 内部错误已经消失。

主源 [DuckDB issue 25713](https://github.com/duckdb/duckdb/issues/25713) 与
[修复 PR25844](https://github.com/duckdb/duckdb/pull/25844/files) 提供相同 index 0/vector 0
窗口并行故障的候选机制。固定依赖的 bundled archive SHA
`53398a1a9ac6b8c0bd9314a07db61d4b0473cc2ad3a567e323f820cc18fe4ba8` 与实际 debug
编译源确认仍使用 SINK `sunk == count`、按行累加的旧逻辑；不是已纳入修复的 engine。
但本 fixture 的未匹配 LEFT JOIN 不等于最终输入为空，CI 无符号栈或 offset，不能
把该源级故障路径当作本案因果结论。未复制／改编上游代码、未升级依赖或修改生产 SQL。
一次固定上限的同 fixture scheduling diagnostic **PASS 1／29.32s**，重编译 22.82s：
同 verified transaction 每 offset window 256、constant 32、single-thread 32，
共 **640 个实际 page SQL**；EXPLAIN 确认原路径有 WINDOW、constant 控制没有。
另有 EXPLAIN／metadata／aggregate／session 校验开销，全部 SQL 次数仍 unknown。
没有错误或区分信号，停止该本地 lane，保留 incident cause **UNKNOWN**；不改生产
SQL、不盲重跑旧 CI，也不把重复 GREEN 当成当前 finding 的关闭依据。
后续 `3042b4d31` analysis_compute job `110708817028` PASS；父已明确将旧事件
按 UNKNOWN 保留，不再无信号重跑或改生产 SQL。新失败的诊断入口是现有
`search_page_window_exact_ci_shape`／single_thread／count_literal controls：同一
verified session，COUNT OVER search/tie、threads 8/1、DuckDB v1.5.4、offset 0/1；
CLI 原失败也附 offset context。只在新失败给出区分信号时继续，不重跑 ignored stress。

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
