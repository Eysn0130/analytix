# Analytix 文档整理与归并登记

Status: Operational / reference
Applies to: 项目内 Markdown 的分类、入口、历史保留和碎片治理
Current as of: 2026-10-02
Source of truth: `docs/analytix/README.md` 的证据治理规则和当前文件树
Supersedes: 不直接废弃任何现有文件；本登记为后续逐项归并入口

## 1. 为什么不直接大规模移动

2026-08-26 的 Git 跟踪树快照共有 335 个 Markdown/MDX，其中 `docs/analytix`
95 个、OpenSpec 71 个；这些是历史数量，不是当前分母。
插件、skills、包内文档另有大量契约和参考材料。若一次性移动：

- 现有 repo-relative 链接、脚本检查和外部引用会断裂；
- Git blame 与历史审计成本上升；
- historical evidence 可能被误写成当前规范；
- 脏工作区会进一步扩大，掩盖产品代码变更。

因此本轮采取“先建立权威入口和分类登记，再按独立 change 逐项迁移”的策略。

## 2. 目录职责

| 路径 | 分类 | 规则 |
| --- | --- | --- |
| `AGENTS.md` | Operational | 仓库施工规则；每轮必读。 |
| `docs/analytix/README.md` | Operational index | 文档与证据权威总入口。 |
| `docs/analytix/specs/01-11` | 按 registry 分类 | 以 `specs/README.md` 为准；05 是 Historical reference，其余按各自接受范围使用，不能统称现行规范。 |
| `openspec/specs/` | Normative scoped | 已接受 scoped requirements。 |
| `openspec/changes/*` | Target / work plan | active change 是目标和任务账，不是实现证明。 |
| `docs/analytix/handovers/` | Operational snapshot | 跨线程接续；后续交接 supersede 前一份 currentness。 |
| `docs/analytix/qa/` | Runbook + dated evidence | README 为入口；日期报告只对记录环境有效。 |
| `docs/analytix/benchmarks/` | Reference + dated evidence | benchmark 必须绑定 commit、输入、命令、结果和 skips。 |
| `docs/analytix/upstreams/` | Reference ledger | intake、审计、吸收和 benchmark 分开表达。 |
| `docs/analytix/runtime/` | Operational/runtime reference | 当前命令和合同需重新核验。 |
| `docs/legacy/` | Historical | 只作为迁移/来源历史，不作为当前实现。 |
| `重构升级方案.md` | Historical decision ledger | 只读顶部 currentness；长编号章节不能覆盖代码/spec。 |
| `plugins/*/references/` | Plugin reference | 不等于 production contract；重复副本需哈希核对。 |
| `build/` 中被跟踪的 entitlements、icons、NSIS 脚本 | Maintained build source | 是安装包输入，不能作为缓存删除。 |
| `build/` 中生成的 native/backend 资源、`dist*`、`output/`、`release/legacy/` | Generated/historical | 不是 maintained source；精确区分当前资源、可再生输出与需保留的历史证据。 |

## 3. 高风险大文档

这些文档体量大、历史层次多，最容易被全文搜索误判。现阶段保留原路径，不做
内容重写：

| 文件 | 约行数 | 风险 | 当前入口 |
| --- | ---: | --- | --- |
| `重构升级方案.md` | 14,742 | 多时期互相矛盾的阶段结论 | 只读顶部状态，并回到代码/spec |
| `upstreams/reasonix-sync.md` | 12,888 | 长期同步流水和历史 pin | `upstreams/README.md` + 最新 marker |
| `upstreams/conflict-decisions.md` | 12,214 | ADR 与历史决策混杂 | 按 decision id 查阅 |
| `qa/release-evidence-gate-2026-06-21.md` | 11,278 | 文件名像最终门，但仅为日期证据 | `qa/README.md` |
| `upstreams/go-runtime-conformance.md` | 8,814 | 含 Go cutover 前历史假设 | 当前 Go 代码/tests |
| `specs/08-upstream-absorption-and-go-runtime.md` | 8,552 | 规范长、含历史解释 | 规范目标，不是 current pass |
| `benchmarks/upstream-scorecard.md` | 8,223 | 多批次主观分数和旧结论 | 未来 CapabilityBenchmarkV1 |
| `upstreams/kun-sync.md` | 7,362 | 历史同步流水 | 当前 source marker |
| `upstreams/absorption-ledger.md` | 6,975 | 旧批次完成语气 | active OpenSpec tasks |
| `specs/09-agent-quality-product-benchmark.md` | 6,715 | 目标 benchmark | 可执行 benchmark artifact |

## 4. 已发现的碎片与重复

1. Funds 根 `references/` 为唯一维护源，skill 副本保留自包含相对路径。
   现有 `runtime-cache-contract.mjs --sync` 生成完整 25 文件镜像；
   `test:plugin-contracts`、doctor、Hub 包源准备与 runtime-cache 快照检查
   inventory、普通文件及逐字节一致。命令和清单由
   [插件 owner](../../plugins/analytix-fund-analysis/README.md) 管理；
   不把消费副本当无效代码删除，也不借此恢复 dormant MCP。
2. upstream、QA、benchmark 多份日期报告使用 `final`、`closure`、`passed`
   命名。保留原始证据，但引用必须经过目录 README 的 currentness 规则。
3. `docs/legacy/kun` 包含中英文历史材料。已增加 `docs/legacy/README.md`，
   不再让搜索结果看起来像当前 Analytix 规范。
4. `.agents`、`.claude`、`.cursor`、`.kunsdd` 内的 Markdown 属于工具/工作流
   配置，不应并入产品规范。
5. 包内 `node_modules` 的 README/LICENSE 是依赖材料，不纳入产品文档数量和
   整理范围。

## 5. 后续整理队列

### 2026-10-02 当前事实、历史路由与 reference 单源治理

基线 `d469a7406d897a97ca7d74f3dbbb29bf635a6ccf`；仅修四项已批准治理，
没有全仓 dead-code、adapter 重拆、产品/native/Provider/安装验收。现行事实从
launcher、实际 Go owner 和 required CI 核验；main 自己的 Development52/52、
CodeQL4/4 为源码证据，完整产品 Ready 与同模型优势仍未建立。

25 块原文（285271 bytes）迁入既有日期 review/PR37 QA，保失败、日期、SHA、
13 个旧 heading anchors；1360-byte 重复 Owner snapshot 单份保留。
case196/116、brand30/17 checkbox 原文及10.3a多行要求不变，accepted 与 active
delta 分开。当前剩余安装/import→Final→display→restart、平台和 paid 同模型证据
仍由 [product completion](product-completion.md) 路由，历史 blocked 不改写为 pass。

Funds 根资料是维护源；既有合同清单补第25项 `database-site-diagnostics.md`，
sync/check 与 doctor、包源准备、cache captured buffers 和既有 gate 共享。
25 对输出每套913199 bytes，排序 name+NUL+bytes+NUL SHA256
`b25f77bf031ad329094571a2feaa057a9823a7f672f3b28b6978ad487b949a36` 不变；
license/notices 未改。定向临时fixture证明遗漏文件漂移、缺失源/额外镜像/symlink
拒绝、缺镜像文件再生与幂等。既有 remount fixture 的两套旧路径标签不同，已让
producer 从同一根标签派生；事务 self-test pass（含八个crash恢复点），
skip-archive包源输出检查未通过：调用用了字面 `/tmp`，但cache helper将当前
临时目录置于缓存卷，脚本拒绝输出路径，未生成包源。没有换路径或改TMPDIR重试；
父随后明确允许按同一CLI原约束纠正一次：读取helper实际os.tmpdir、mkdtemp
专属容器并核realpath在边界内，新skip-archive包源检查pass，25对913199 bytes、
production MCP payload/golden isolation均通过，archive=null。未改TMPDIR/权限/
helper，未触碰旧拒绝路径或UI产物；旧失败仍保留。

固定三任务只做一次机械路由演练，无模型benchmark或源码fixture改写；每项固定
200000-byte完整文件读取预算，baseline在候选前冻结，读取清单/哈希为8/10/10文件。
下表是实际driver读取与路由stdout，不是模型返回全文或token。子验证器读取未计量，
错误定位和首次验证不是独立baseline耗时对照；源码fixture哈希相同才复用其验证。

| 固定任务 | baseline / candidate driver bytes | candidate route stdout bytes | 首个有效验证与修改触点 |
| --- | ---: | ---: | --- |
| docs-only | 160294 / 112153 | 569 | diff/link pass，52.527ms；修README开发隔离真值，指南/基线提供owner路线 |
| local-typescript | 160157 / 111773 | 820 | typecheck pass，23524.532ms；fixture/consumer未改，明确映射边界 |
| go-authority | 186719 / 138262 | 1900 | 三grant tests普通/tag各pass，首次113990.101ms；修包README错误owner，fixture未改 |

固定读取范围含完整development-baseline，因此迁移历史会减少driver bytes；
实际模型、tokens、模型time与长期效益均unknown，共享warm/编译缓存未受控，
不推导速度/费用/智能收益。baseline未读候选答案，candidate仍基于当时HEAD并绑定
文件SHA，不误称旧SHA是候选提交。现有mapper只覆盖docs+两个固定fixture；
PR37同调用shared test owner、普通/tag/platform语义和所有CI阈值保留。

验证：plugin合同gate pass，router/CI回归15/15、14 Markdown路径、两active change
strict validation、四脚本syntax、原文/anchors/requirements机械核验pass；三固定
维护任务pass。进程盘点 `ps` 被系统权限拒绝，未另路重试；自身command sessions
按原工具追踪。四项继承dirty SHA不变，未纳提交；Git交付按父统一排序；父已授权本候选的focused commit及正常Draft PR准备。

四个operational owner总量由409044降至134464 bytes；
全部19文件含历史迁移、工具、指南与本条的净变化为+23429 bytes。
本轮不承诺全仓净减或全部技术债
已清。当前候选与main/CI状态查Git，日期证据不转为新的安装或发布授权。

<a id="2026-10-01-共享测试执行与-source-row-原子配对"></a>
Historical evidence: [consolidation 81–95](qa/shared-regression-source-row-2026-10-01.md#snapshot-d469-consolidation-81).


<a id="2026-09-30-有界资源与-quarantine-治理"></a>
Historical evidence: [consolidation 97–166](documentation-delivery-review-2026-09-09.md#snapshot-d469-consolidation-97).


按实际影响验证整理涉及的链接和消费者。相关文档可合并修订；本队列不是产品交付的前置门禁，也不要求每项另建 change、切片或全量验证。

Historical evidence: [consolidation 170–177](documentation-delivery-review-2026-09-09.md#snapshot-d469-consolidation-170).


| 优先级 | 动作 | 最小完成证据 |
| --- | --- | --- |
| P0-doc | 为会被当作当前入口的旧 `final/closure` 报告加 historical banner | 无正文改写；链接检查通过 |
| P1-doc | 把 `重构升级方案.md` 的 current-state 摘要拆为短索引，原文件保留 decision ledger | 所有旧 anchor 可追踪 |
| P1-doc | 为 upstream 大 ledger 生成按 source/date/decision-id 的机器可读索引 | marker 与 commit 校验 |
| 已完成本轮 | Funds references 单源生成/完整漂移检查 | 现有合同模块与 gate；完整 25 对字节保持，证据见本轮条目 |
| P2-doc | 将 QA 证据元数据统一为 commit/platform/command/result/skips | checker 验证 |
| P2-doc | 从 scorecard 迁移到 `CapabilityBenchmarkV1` 原始结果索引 | 可执行测试绑定，禁止主观 exceeded |
| P2-doc | 清理过期双语文档的 currentness 漂移 | 中英文同步、链接检查 |

## 6. 新文档写入标准

关键文档按需要说明以下信息；已由目录明确分类的历史材料无需逐文件增加重复字段：

```text
Status:
Applies to:
Current as of:
Source of truth:
Supersedes / Superseded by:
```

每个 present-tense PASS 必须包含精确命令、commit/worktree、环境、退出结果和
skip 状态。仅规划、只读审计、fixture、历史包或外部条件缺失必须显式标记。

本机绝对路径只允许作为来源/交接 provenance；不得写凭据、真实案件数据、
完整银行账号、完整 PII、reasoning 或原始外部错误正文。


<a id="2026-09-09-本轮处置"></a>
<a id="2026-09-21-pr28-current-entry-consolidation"></a>
Historical evidence: [consolidation 208–234](documentation-delivery-review-2026-09-09.md#snapshot-d469-consolidation-208).
