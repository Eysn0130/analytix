# Analytix 新 Owner 恢复与交付入口

Status: Operational checkpoint，2026-09-10；不是实时控制账本或产品验收
Applies to: 用户明确授权的新 Owner 替换与 canonical Analytix 施工
Source of truth: 当前 Git/代码、accepted specs、两项 active change 及候选绑定证据

## 后续施工检查点 — 2026-09-10 B1 同源链聚焦接受

Owner 已接受 B1 40 文件候选
`c32b732dfabfa489d6dcf8d3d7c052d2144d17f42ed91ec60ff263b98475c3f1`，
被测状态为 `5680a528e3a60d7bb8903ca254feca829ad44923` 加该候选。
Slice `01a08a92-c826-7ea0-bc8a-db858db66acc` 已匹配 revision 2
`COMPLETE_BRIEF` 安全释放；以下 R131 检查点的“下一包 B1”已由本节接续。

当前实现保留完整 witness selection 审计 digest，另签名绑定稳定内容；旧记录
不补写或自动升级。明确 Host 导入确认可在既有 enrolled witness 下激活真正
fresh-zero registry，启动观察不初始化。单次 registry 提交通过本次真实 CAS、
签名读回及 fresh post-witness 交接，原只读 `UseExact` 不变。新导入使用既有
local-display query profile，源定位符与原始行/字段 resolver 一致。
实际 account-flow 结算和受保护案件数据候选终态各自采用有限 60 秒预算，
其他路径保持 15 秒；原计时起点、效果锁、旧签名兼容和持久化检查保留。

`OWN2-B1-054` 在该候选上 PASS（native 测试 592.15s）：真实 native 基线
1250/0/count1，演进后 1250/225/count2；金额为 minor units。真实 preparation、
receipt、3 claims、Final Gate 与原始 full/masked slot 同源；一行证据的演进结果
保持 `PartialEvidenceAnswer`。第二次真实导入、durable handler 重开、历史源显示、
当前 preview、retained 缺失/损坏拒绝及损坏期间普通 Provider turn 均通过，
3 次 async completion / 0 failures。原始 CAS 损坏会同时阻断两种 local read，
不宣称故障域隔离；精确恢复原字节后两者恢复。Owner 独立 activation race 与
budget/legacy compatibility race 均 exit 0；独立 RD2/RD4 只读复核无确认 P1/P2。

证据入口为配置主机原 `own2-b1-public-chain-20260910` 目录的 054、055、
`review-seal-round1-generation1.json`、`brief-complete-ack-rev2.json` 与
`owner-review-*.json`，以及同一 instruction-maintenance 目录的
`own2-b1-public-chain-report-20260910.md`。全部历史失败保留，021 的 7 个失败
子例由 024/038 在 clean HEAD 精确复现；不把基线失败算作 PASS。049 的
`failure_record_error=unclassified` 不作追溯修复声明。

结论仅为 `FOCUSED_ACCEPTED`：当前 Go 组装使用固定历史 development native
输入与合成 Provider/data；不是当前源代码原生包、真实 runtime-server 进程、
Electron、Windows、性能或 Product/Formal RC 验收。下一依赖缺口是当前源代码
native generation 与真实 runtime-process 的同源证据，再接独立的 A0/B1 正式
产物/凭据/可见 UI 义务。旧 AB-R1 等一次性命令不复用；无自动化约束不变。

## 后续施工检查点 — 2026-09-10 R131 聚焦接受

新 Owner `01a08a3b-d294-7963-bf59-0c2c6fac293b` 已在同一控制记录完成接管。
R131 Slice `01a08a3d-9c9d-71e1-9fd1-c2c339189f04` 的 revision 3
`COMPLETE_BRIEF` 已匹配安全释放回执；旧 AI 终态不变。下文“F1 仍开放”和
“先派第一个施工包”是接管时的历史状态，由本节的当前检查点接续。

Owner 已接受 53 文件候选
`501d5ae3c9f2b97156d36d31d1498a960af3835c172f8a2e5070fc3c6c753826`，
被测基线为 `4b4392dc2d1ae4a0683ef46f71ecd93007acc9bc` 加该候选。
原 48 文件工作保留；GENERAL 启动在既有终态 outbox/usage 未闭合时禁止追加
下一 turn；CASE finalizer 在既有私有库存与不可变写入之间串行准入，禁止同一
turn 的不同 private preparation，保留原记录供既有恢复流程使用。

`R131-F1` / `R131-F1-CASE` 的当前不变量风险关闭。GENERAL 九种故障组合、
取消交错与六个生产恢复场景，以及 CASE candidate/fixed/longitudinal 三分支的
真实完成/fallback 双失败、HTTP/SSE、两次重启和普通续接形成分别绑定的证据。
Owner 独立 GENERAL race 检查 PASS（27.195s）、private preparation 恢复
race 检查 PASS（2.156s）、`tsc --noEmit -p tsconfig.web.json` exit 0。
CASE 三分支生产 race PASS（238.549s），`app/evidence` 整包 PASS（5.605s），
既有 CASE 仲裁/诊断 race PASS（3.279s）。所有执行均使用 macOS canonical
cache、隔离合成数据及 loopback Provider；没有 live Provider 或正式包验收。

历史 matrix36 的错误标记和原因 `UNKNOWN` 保留，不据当前修复倒推其原因。
longitudinal 领域语义故障的结论是已提交终态保留、可选能力降级、
`NOT_RECOVERED`，不是重放恢复；库存 `List` 的额外成本未作性能测量。
原 desktop 证据仍为 37 passed / 109 skipped。full9.8a、platform 2.4/4.3/4.4、
A0/B1、Product/Package/Formal RC 均未由本次聚焦接受关闭，两个 change 保持打开。

配置主机的审阅证据入口：同一 instruction-maintenance 目录下
`own2-r131-terminal-closure-report-20260910.md`、
`own2-r131-case-closure-r2-report-20260910.md` 和
`own2-r131-rev3-brief-complete-release-20260910.json`；R2 的
`review-round2-command-evidence-supplement.json` 补全 13 条命令枚举，原封存未改。
下一包核对 B1 精确 local-build generation，再闭合有价值 `analyze_account_flows`、
不可变 snapshot/Final Gate 与 typed-local full/masked 路径；不重复 AB-R1 原命令。
下文 ledger 自依赖风险仍待独立有限契约验证，不阻塞确定性 B1 施工。

## 已完成的安全交接

旧 `Analytix Owner Command｜最高项目指挥`
(`01a04667-5661-7f40-b2a3-0e883fbcaa49`) 已停止后续派工与集成；
`Analytix Controller Epoch AI`
(`01a07a71-90a1-7ca0-9ce8-d3858043156d`) 已按
`OWN-AI-R131-REV6-HANDOFF-FREEZE-20260910` 永久退役。
`AI-R131-W1/rev1` 与 rev5 legacy repository-exclusive reservation 均已撤销。
旧 AI 不能以新 brief 复活。新 Owner 必须使用新身份、fresh bootstrap 和新 lease。

本机唯一交接状态入口仍是
`/Users/sun/.codex/instruction-maintenance/2026-09-07-resume/controller-ai-state.json`。
它保存旧终态和恢复定位，不是新 Owner 的永久身份。新 Owner 接管后在同一
控制记录中绑定自己的真实 native thread id，保留封存的旧状态，不复制一套账本。
终态证明位于同级 `r131-rev6-terminal-freeze-AI/`：
`terminal-ack.json`、`safety-proof.json`、`candidate-manifest.json`、
`evidence-manifest.json` 和对应候选/证据副本。以上路径仅适用于配置的 macOS 主机。

交接时 HEAD 为 `cd1565cee10ddfcba42b1dfb2676130aae5e693c`，tree 为
`20542cbbe5c506a449c57ad1941f7b54fa06897c`。48 个 R131 文件仍在 canonical
工作区，指纹为 `04190178997e1f70b956847bb3c30ef2a28f936f176ea8701b3a9f3ee0fe3506`。
48 份候选副本和 135 份证据副本已按 manifest 保留；Owner 自身决策原件有已归属的
后续控制追加，不能拿它与旧封存副本的全文件 hash 比较后误判测试证据漂移。
这次治理提交会改变 HEAD，但不应改变那 48 个文件的 SHA 或权限。
重新绑定证据时须分别记录原被测 baseline 与当前治理提交后的 HEAD。

交接检查确认 index 空、无 index.lock、相关执行进程及 canonical 数值可写 FD 为零；
stash 保留 `3e04ee6801885b8d54a624e9519a197eac98ab67`。这些是时点证明，
新 Owner 写入前复核 delta；不得 reset、从缓存覆盖、混入提交或删除 R131 候选。

## 调度方式：没有自动化

用户本次明确选择：**不创建、恢复或重新绑定 heartbeat、cron、watcher 或其他
定时任务**。原 `analytix-ai` 已经原生暂停，继续保持 PAUSED。

采用 `新 Owner → Slice → 主动回传 → Owner 审核/集成 → 下一 Slice`。
每份 brief 写明实际 Owner/Controller id 与 native `send_message_to_thread`
回传路线；review-ready、真实阻塞和终态事件触发协调者下一回合。发送成功、
控制已采纳、安全停止、验收完成分别记录。无需为普通 RED、文件扩展或例行
验证产生新的 Owner 批准；真正的保留决策才上报。

Owner 可以直接处理小包；不强制额外 Controller 层级或 Luna。保留用户配置的
GPT-6 Astra 等主模型及 reasoning effort；Skill 不改变模型能力、工具权限或
用户权限。更强模型也不能跳过隐私、持久化、Final Gate 或真实产物验收。

额度耗尽可能同时阻止施工和主动回报。无自动化路线不承诺自我唤醒；恢复额度后
由用户或可用的 native 后续消息恢复已有任务，先核对旧命令身份，不能新建重复
工作或重复消费命令。不要用第二个监控、自动 reset 或降级模型掩盖该限制。
平台定时能力与本项目的禁用选择是不同事实，参见
[官方 scheduled tasks 文档](https://learn.chatgpt.com/docs/automations?surface=app)。

## 当前可验收缺口

R129/R130 已集成的范围见
[case 任务的 2026-09-09 checkpoint](../../../openspec/changes/case-evidence-publication-gate/tasks.md)。
该 checkpoint 中“下一包 R131”是历史调度描述；现在已有未提交候选，不能重做。

| 义务 | as-built / 已有证据 | 当前 gap 与下一动作 |
| --- | --- | --- |
| R131 普通生命周期及 desktop consumer | 48 文件候选；原 matrix36 五格测试显示 PASS，但 stderr 出现固定异步终态失败标记；rev4 的闭合观测及 rev5 的 missing→disabled 同进程相邻格均未复现 | `R131-F1` 仍为 `UNVERIFIED-BLOCKED / MUST_FIX`。不能以通过次数、clean drain 或 detector 自测签收。先做有判别力的终态故障模型与确定性交错闭环。 |
| full9.8a 与平台 2.4/4.3/4.4 | 复用 R129 的范围内证据；R131 为共享候选 | 完整普通生命周期与适用插件故障类、desktop 消费者仍需稳定候选证据；不要两个 change 各做一遍。 |
| B1 确定性价值与本地显示 | 既有 native admission、Funds、snapshot、Final Gate 和 typed local-display 基础 | 核对 9.10a 当前实现，再关闭 `analyze_account_flows`、不可变 snapshot、Direct Preview/AcceptedSlotDisplay、full/masked 正负路径。`count_case_rows` 不是价值验收。 |
| 本地凭据与 A0/B1 | R130 focused harness 修正已集成；普通启动仅 Local Registry/Secret Store | 正常可见凭据入录的实际权限/生命周期、精确 artifact、固定工作流与分别报告的 A0/B1 尚未完成；Hub 旧方法不能恢复为默认。 |
| 全部产品/正式验收 | 当前并没有 Product/Package/Formal RC 完成结论 | 仍按适用 RC_REQUIRED、真实产物、支持平台、迁移/恢复、安全及 artifact admission 完成，不能以文档 strict validate 或 focused PASS 替代。 |

### 第一个施工包：R131 终态风险闭合

保留原失败；以现有 terminal producer、failure persistence、public projection、
restart/recovery 和 observer 为范围，不新增第二 authority/receipt/registry。
首要候选路径是 `packages/runtime-go/internal/server/turn_start.go`、
`turn_terminal.go` 及相应 `runtimeapp` 回归；必要 consumer 传播按实际证据确认。

先区分历史 matrix36 的未知根因、当前观测缺口与当前正确性风险。控制 completion
与 failure-persistence 的故障/取消/完成顺序，证明成功不能越过未持久化结果，
失败被明确观察，恢复不会丢失或伪造终态，且公共输出保持闭合无敏感值。现有
guard 自测只证明能检测，不能代替这些产品不变量。

每项试验预先说明结果怎样改变下一决定：发现违例则同包修复并补回归；现有实现
已满足则用相关故障类和恢复证据证明；仍无法区分则更换方法或明确缺少的证据，
保留受影响 MUST_FIX 并安全释放占用后推进独立义务。不能再原样重复整套矩阵。
历史根因可以仍 UNKNOWN，但只有当前必需风险全部被证据关闭且 accepted contract
不要求该历史精确归因时，Owner 才可关闭当前 finding。不是允许豁免未知风险。

完成一次集中 whole-diff 审核、失效检查和必要的风险复核后，Owner 才集成产品
候选。独立审阅有价值时使用，但不按角色数重复同一套测试，也不因内容相同的
本地 commit 强制重做全量验证。正式 artifact 的精确绑定规则不变。

## OpenSpec 与最终 gate 的闭合顺序

两项 change 的 proposal/design/spec/tasks 完整只表示 planning complete。
本次没有改变 accepted requirements、复写历史失败、勾选任何产品任务、迁移
POST_RC 或归档 change。第一阶段 A0/B1 交付和两个完整 change 归档是不同终点。
未来只有正式调整范围并保留后续责任/引用，才能将 deferred 义务迁到后续 change；
不能删除它们制造完成率。

另有必须在最终验收前明确关闭的 **ledger 自依赖风险**：
`scripts/runtime-go-release-gate.mjs` 要求 `openRcRequiredIds.length === 0`，
而 tasks 10.4 要求执行该 gate，10.8/10.9 要求最终报告/DoD，10.10 又把持续的
“不操作 Codex Goal”禁令表达为开放 RC_REQUIRED。若把这些行都解释为“先等同一
最终 gate PASS 才能勾选”，就会形成环。

不要据此跳过 ledger、预先勾选、把失败改成 PASS，或重复整套 gate 碰运气。
Owner 应在现有任务/receipt 契约中分清执行证据、独立验收条件与最终汇总签收，
核对 source/ledger hash 冻结时点，形成无自依赖的闭合顺序。`Run` 本身不等于
`PASS`；有未解决产品失败时不能完成 DoD。若确需改 parser/gate 或任务完成语义，
应作为一个有行为回归的有限契约包：零开放必需义务、精确证据绑定、失败保持
阻塞和期间漂移拒绝均不得放松。本次未执行/改造 release gate，也未宣布此风险
已经通过验证关闭。它不要求暂停独立的 R131/B1 实现。

## 上游吸收：服务于当前缺口

下列为 2026-09-10 研究的固定输入，不是新采纳版本或工程完成声明。先遵守
[上游准入](../upstreams/README.md)，只把能关闭已接受缺口的机制放进当前 Slice。
没有复制代码，也不把五个上游的全部功能变成新交付分母。

| 输入 | 本轮目的与边界 |
| --- | --- |
| [DeepSeek Harness persistence](https://github.com/deepseek-ai/deepseek-harness/blob/b2e3b2a0125854567a4a5fcba75782e42fe84901/docs/subsystems/persistence.md) | 对照 append/flush/durability 与 single-writer，挑战 R131；不把 Analytix Core/Privacy Layer 变为可卸载插件。 |
| [OpenAI Codex task lifecycle](https://github.com/openai/codex/blob/4f2449b4b21988d5015ce6edf755fbd6a37a4908/codex-rs/core/src/tasks/mod.rs) | 对照任务完成、取消、清理分工与恢复；不为复用 Rust 引入第二生产核心。 |
| [OpenCode processor](https://github.com/anomalyco/opencode/blob/f69beceaffca94bed05a7669af93602125c37248/packages/opencode/src/session/processor.ts) | 借鉴重复无进展识别；不复制敏感输入到日志，不把宽松默认权限当 Analytix 边界。 |
| [Claude Code](https://github.com/anthropics/claude-code/tree/347b38e4a733d95b2f00690a4ca58ac1544f8a1c) | 参考公开行为组织普通任务、失败修复与恢复基准；公开仓库不等于完整开源核心或任意复制许可。 |
| [Claw Code](https://github.com/ultraworkers/claw-code/tree/08106b0c3771ef5b4a5aa176acccd460e88b7325) | 参考 CLI/parity 与功能现状披露；权限检查必须连同调用方实际执行审阅，不能仅复制单个 enforcer。 |

真正复用时，只在既有 provenance owner 记录 source commit/path/blob、目的路径、
copy/port/reference、适用许可/NOTICE、accepted invariant、验证与 disposition。
研究输入的新旧不自动阻断整个工程；实际进入产物的必需第三方义务不能豁免。

## 本次修正的边界

本次交付是治理与恢复入口修正，保持原有模型自主性，补充故障处置与无自动化
事件回传规则，并保留 R131 全部工作。没有运行产品测试、正式打包、真实凭据或
case 操作，没有提交该产品候选，没有 push/tag/发布。新的 Owner 在完成本次
治理集成后创建；真实新身份和首个 Slice 的启动以 native task 结果为准，不由
这份文档臆造。新 Owner 接管后直接落实施工包，不再开启一轮无关的治理重写。
