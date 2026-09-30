# Analytix 文档整理与归并登记

Status: Operational / reference
Applies to: 项目内 Markdown 的分类、入口、历史保留和碎片治理
Current as of: 2026-09-29
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

1. 资金插件 `references/` 与 owner skill 下的 `references/` 有 25 组同名
   文件；2026-08-26 在 `c31dc3afb7a4be1d5193dd4085a0e434074dfbf0`
   跟踪树上按同名文件逐组执行 `shasum -a 256`，结果为 25 组一致、
   0 组差异。这不是
   可直接删除的无效重复：根 `references/` 是发布校验和脚本 source，
   `skills/analytix-fund-analysis/references/` 是 skill 打包副本，README 与
   focused gate 明确要求它们字节一致。在生成/同步和消费者合同未改变前，
   保留两个位置并把“字节一致”作为可执行门禁。
2. upstream、QA、benchmark 多份日期报告使用 `final`、`closure`、`passed`
   命名。保留原始证据，但引用必须经过目录 README 的 currentness 规则。
3. `docs/legacy/kun` 包含中英文历史材料。已增加 `docs/legacy/README.md`，
   不再让搜索结果看起来像当前 Analytix 规范。
4. `.agents`、`.claude`、`.cursor`、`.kunsdd` 内的 Markdown 属于工具/工作流
   配置，不应并入产品规范。
5. 包内 `node_modules` 的 README/LICENSE 是依赖材料，不纳入产品文档数量和
   整理范围。

## 5. 后续整理队列

### 2026-09-30 有界资源与 quarantine 治理

基于 `fb210a84426f8bcce55e515c681422702f54232c` 的 canonical worktree；
四项继承 dirty 不属于本切片，哈希保持不变。此记录是局部源码验收，
不是系统治理、安装验收、同模型分析增益或 main/发布就绪结论。

- `runtimeapp/app.go` 的七处资源绑定失败出口收敛为一次有序绑定和一个出口；
  owner 仍在 `owned_runtime_resource.go`，原可选条件、defer、错误链与最终
  接管时点保留。反向顺序 fixture 得到行为 RED；多资源 drain/close 重试、
  已关闭前缀、未关闭后缀、必需 nil 与并发 shutdown 的定向 race 检查通过，
  真实 handler/maintenance 定向组装检查通过。
- quarantine 的唯一行为 owner 为 `backend-manager.test.ts`：synthetic overrides、
  冷导入、默认 GET/POST、并发 ensure、合法 renderer lease 与 stop 路径，
  覆盖进程、目录、监听、网络和清理写入。五项临时语义回归均得到行为 RED，
  原生产 blob 恢复哈希相同后才退役旧八项禁词及 packaging 的重复簇。
  规范化 module identity 检查保留执行 authority 的架构边界。
  与 native-runtime-paths/packaging 的聚合报告实际发现并通过 79/79，
  其他授权、泄露、平台与聚合门禁不变。
- 根指南初次只减 363 字节；在 `62aa16969` 冻结基线继续收敛自动适用链，
  root 15,467 → 13,945，src/go/docs 分别 -565/-73/-581。重复语义依赖
  已自动读取的 root，不新增全 imports，独立仓库安全守卫保留。
  global 9,094 不动，常驻 global+root 24,561 → 23,039；scope 按路径追加，
  不能把本次跨三 scope 的指南总量当作日常常驻。
- 旧三文件所选文本脚本计数 4,602 → 7,987：app -556、helper +557、test
  +3,384。app 缺少完整初始化/defer/接管上下文，旧 receipt 未绑定完整
  返回 payload；不能称为完整需求读取或 Agent 摄入。replay 各一调用，
  排除初始化/定位/实现/失败/轮询；这些成本并未消失，全任务总量未知。
  原 cache 失败、基线取消（130）及旧 null 保留。20.356s / 8.943s 的
  warm/编译缓存未受控，新增测试工作量不同，不推导速度、token 或费用收益。
- 本切片生产源码净 +1 字节，测试净 +7,086 字节；维护收益是失败处理
  修改点 7 → 1，不能以总 LOC/字节下降代替覆盖或收益。没有新增治理工具。

已启动的第二生产 owner 切片将 adapter 六条路由的 body/schema 注册合为一处，
summary 的 schema、identity 和精确投影选择由同一私有 resolver 负责；
严格 canonical equality、AcceptedFinal/currentMain pin 与两边 sanitation 未迁移。
同一 `runtimeRequestViaHost` 测试组 before/after 各 43 通过、73 未选，
最终候选 44 通过（包含两项实际 HTTP 回归）；组内其余项目混合直接 sanitizer
与 HTTP 测试，不称为全组真实 transport 证据。精确投影相关三文件 43/43、
web/node typecheck 通过；独立只读 review 无具体缺陷。该切片生产 +601 字节、
测试 +2,477 字节，不宣称整个 adapter 或系统技术债已收口。

本机小 receipt 在 `/private/tmp/analytix-debt-resource-matched-{before,after}-20260930.json`
及 `analytix-quarantine-{red,green}-20260930.json`；前者是明示范围的读取 proxy，
后者保留五项 RED 与执行式发现结果，不复制大型 inventory。

整改固定 17 个完整必要文件（含新增 helper/测试、适用指南及既有 fixture），
清单和哈希在 `analytix-debt-cost-frozen-{before,after}-20260930.json`；物理
corpus 与实际完整指南返回 payload 分开计数，register 增量是 corpus 内的
治理成本分解，小 receipt 和验证证据另计。测试数据表减少 529 字节，
关闭/错误身份/重试/并发及冷导入、caught 副作用、两 packaged 态、lease
覆盖保留；反向关闭及五类副作用在新测试上再次 RED，生产 blob 恢复一致。

按实际影响验证整理涉及的链接和消费者。相关文档可合并修订；本队列不是产品交付的前置门禁，也不要求每项另建 change、切片或全量验证。

当前交接与矩阵的 2026-09-27 叙述已迁入既有
[日期 QA 的 Historical appendix](qa/pr28-next-execution-2026-09-26.md#historical-entry-snapshots-relocated-from-c2cd--2026-09-29-pdt)，
保留旧 entry anchors。入口现在区分 c2cd HEAD、继承的未提交 host-local B1、
7dc 准确安装证据与当前未知远端状态。只读
`node scripts/validation-burden.mjs --json` 使用
[小基线 manifest](validation-burden-baseline.json) 比较声明读取字节并盘点静态测试范围，
接入既有 `verify:baseline`。静态入口重叠不等于 CI 重跑，未知依赖仍走原全量门禁；
报告不是低价值测试删除列表，也没有实际 token、正确率或首次有效验证时间基线。

| 优先级 | 动作 | 最小完成证据 |
| --- | --- | --- |
| P0-doc | 为会被当作当前入口的旧 `final/closure` 报告加 historical banner | 无正文改写；链接检查通过 |
| P1-doc | 把 `重构升级方案.md` 的 current-state 摘要拆为短索引，原文件保留 decision ledger | 所有旧 anchor 可追踪 |
| P1-doc | 为 upstream 大 ledger 生成按 source/date/decision-id 的机器可读索引 | marker 与 commit 校验 |
| P1-doc | 将插件 reference 镜像收敛为明确的单源生成/同步流程，不直接删除消费者要求的副本 | 生成器或同步命令、运行时路径测试、全量哈希 |
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


## 2026-09-09 本轮处置

本轮 [审阅记录](documentation-delivery-review-2026-09-09.md) 修正当前入口的
Hub-first 冲突、插件身份归属与规范分类。文档地图中的 2026-07-29 外部指南比较
迁至 [Historical reference](upstreams/agent-guidance-review-2026-07-29.md)，
原入口保留链接。历史报告、归档 change 和打包要求的 reference 镜像保留；
不会因为文件长、旧或重复就删除仍被消费或用于证据追溯的内容。

## 2026-09-21 PR28 current-entry consolidation

Scope: the two actual current entry files and their existing dated evidence owner,
not a whole-repository documentation approval. START d8640957c; product SOURCE2f25c22f7.

| Original at d8640957c | Exact original SHA256 | Preserved owner / action |
| --- | --- | --- |
| `handovers/README.md` | `57d367da2f4971651f1627f9ccdd71c674c83700716d2cf566b93379ea4714c2` | Complete snapshot moved to `handovers/2026-09-16-pr28-continuation.md`, explicit historical heading IDs; README now current identity/gaps/recovery with compatibility links |
| `product-completion.md` | `c2d7429c6fc7a389587535ef0a606e322bed94704ac7461b4339144491aa881e` | Complete snapshot in the same dated owner; product-relative links rebased, current file becomes P0–P5 as-built/gap/evidence/gate matrix |

Original text remains recoverable at the exact Git commit; migration changes heading
currentness labels and link locations, not recorded outcomes. Private migration
receipts bind original/relocated hashes and each heading's old/new anchor. The current
QA adds errata for the wrong LibreOffice anchor, browser-red executed-versus-discovered
counts, and old65ab totals. It does not rewrite immutable command receipts.
The document map and knowledge governance now preserve a newer unsynchronized local
writer/checkpoint before consulting an older remote. No license, historical failure,
compatibility reader or product feature was removed. Other documents are UNREVIEWED
unless a specific reading/validation record says otherwise.
