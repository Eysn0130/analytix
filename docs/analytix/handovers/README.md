# Analytix 施工交接索引

Status: Operational index
Applies to: 跨线程暂停、恢复与阶段性交接
Source of truth: 当前代码、当前适用 `openspec/changes/*/tasks.md`、当前 Git/GitHub、当前 PR/CI/review 和确有必要的新验证
Supersedes: 无；本目录不替代 accepted specs、OpenSpec 或代码

本目录只保存“某一时点如何安全接续施工”的操作性快照。交接文档可以说明当时
看到了什么、运行过什么、还缺什么，但不能把历史 PASS 自动继承给新的工作区。
**本页是跨线程恢复的恒定入口**：不要为每个新会话另建第二套总账。

## 当前接续：2026-09-17 实际发送链与案件授权组合

Status: **VERIFIED_SOURCE_BATCH / READ_FRESH_REMOTE_STATE**。本节与真实源码和测试一起提交，
不另造状态提交。继续原分支 `codex/workbench-product-delivery-20260914` / Draft PR28。
来源候选 `de709d9c322948e786f87b84fffede3d9f988296` 已实际收到，37 文件、完整父/tree/
blob/模式核验通过。旧未提交候选仍未收到，但用户已取消把它或旧停写回执作为本轮前提。
后来真实到达的改动仍须保护；不修改独立 UI Refresh，不 merge/rebase/force/main/release。

- 实际 Workbench/plan/send store/引用与草稿 owner 的 17 项初始回归发现 6 项失败，
  原 owner 修复后同 17 项全部通过；扩展为 23 项真实消费链测试。覆盖最后异步 I/O
  后 Core 复核、拒绝保留、busy 不排队临时授权、迟到回执/错误线程隔离、重复准备、
  只清理已提交快照及 Office 原路径。没有复制测试专用 handler 或新建 Canvas。
- 真实签名案件 Registry、Host risk-policy CAS、case binding、epoch、原 Host/projector/
  Registry/filestore 验证普通案件工作区文件的合法修改与失效拒绝；4 顶层、7 子场景。
  该组合明确不授予 DSV2 案件事实发布，不冒充 independent-witness 或完整案件 AI 验收。
- 本轮完整 UI 回归 43 实际文件 / 732 PASS；双配置类型检查、当前 lint、runtime TS 和
  Electron 源码构建通过。Go 当前 normal/prod/race 精确结果及命令见
  [同批 QA 记录](../qa/canvas-product-loop-2026-09-17.md)。新旧、重复、嵌套计数分别保留。
- 实际 Chromium 首次访问合成 loopback 验收页被 `ERR_BLOCKED_BY_ADMINISTRATOR` 拒绝。
  未绕过策略，没有浏览器 UI PASS；源码/组件证据不替代原生安装、GUI、IME、Provider。

**恢复顺序**：读取当前 Git/GitHub 原 ref/PR 和 comment `5709839864`，以及交付包实际
commit/tree/manifest，确认真实写入结果。终端 DNS 失败不等于连接器只读；完整出站检查后
使用受支持的普通 push 或真实单父 Git Data API + `force=false`，一次前移完整一致 tree。
若 API 生成等价 tree 的新 commit，准确记录新 SHA，不能声称原 de709 字节已原样推送。
没有 ref 前移就没有 push；在新 SHA 收口 CI，不借用 dde 的 51-job 历史成功。
只有实际动作证明必要通路/合规资源不可用，且不依赖该缺口的工作完成后，才把保存的
最终实际候选及准确剩余项交 Codex；下方旧“下一步 Codex”不覆盖这项接续要求。

37-high 完整 traces/复扫、历史 DuckDB inline 根因、原生共同对象旅程、授权真实模型/
媒体、安装、A0/B1/ProductRC/FormalRC、签名公证仍逐项核验。当前 PR comment 与外部
执行胶囊记录实际远端 SHA/CI/写入边界；本索引不把工具发现、setup 或旧 PASS 变成验收。

## 历史本地候选：2026-09-17 Canvas 引用、持久审阅与消费者闭环

Status: **LOCAL_SOURCE_CANDIDATE / NOT_PUSHED**。源代码、测试和本节在同一候选批次；
准确候选 SHA/tree 在随本轮交付的机器清单和当前 Git 中读取，不为自引用另造提交。
原分支 `codex/workbench-product-delivery-20260914`、PR28 Draft 保持不变；
最后远端核对仍为 `dde784437dc8563e84066629dd57f4a11fd9acc9`。

用户已经明确允许：不再以取得原未提交 Canvas 候选作为本轮独立源码施工前提，
从实时核实的已提交基线补齐功能，交付可回放源码候选，再由 Codex 审阅整合。
本轮没有取得、删除或冒充恢复那批旧候选，也没有声明控制原环境或原 writer。
这项授权不表示远端已经推送，或允许覆盖后来到达的代码与独立 UI Refresh。

- 从准确 `dde` 基线复用原 Go Core/Host/Registry/projector/filestore/Composer，
  完成 Canvas 选择引用、短期模型安全 scope、有限提案、真实差异、明确接受、CAS、
  原操作只读查询、持久审阅重开、撤销/恢复与有界两对象预览消费者；PNG 保持本地
  有限裁剪/旋转/标记，不冒充真实媒体生成。
- 已验证的源代码闭环、协议/隐私/存储边界、命令、失败及剩余门禁，见
  [本批 QA 记录](../qa/canvas-product-loop-2026-09-17.md)。
- 独立 Linux 非特权执行：受影响8包 normal 979顶层PASS、prod 992顶层PASS，
  各944嵌套子测试PASS、4既有SKIP、0FAIL；race 41顶层/91子测试PASS、0SKIP；
  前端/Main/相邻Office/契约/Composer 437PASS、30实际文件、0SKIP。
  不叠加不同模式、重复运行或顶层与子测试。不将其记为远端CI、GUI或安装验收。
- 双配置typecheck、16文件ESLint零警告、runtime TS与Electron源代码构建、Linux
  Go production runtime编译通过。未执行npm lifecycle/native安装或放宽完整构建门禁。
- 对相同旧基线的权限失败已经验证为测试环境umask0022使私有根不满足0700；
  改执行环境umask0077后通过，原断言未动。并发长上下文诊断超时保留；顺序完整
  server normal/prod均通过。细节和既有4跳过见QA记录。

**下一有效动作**：Codex先校验源码包manifest/patch/bundle与此基线，读取实时GitHub
和实际工作树，按真实diff保全所有不同来源的修改；仅整合本批，定向验证后普通非强制
push原分支，保持原PR28 Draft。若远端或工作树已变化，先审阅具体重叠，不整包覆盖，
不自动merge/rebase/cherry-pick另一任务，不复用旧测试替代变化部分的新证据。
本地源码候选并不依赖再次取回旧未提交候选；后来找到它时按真实代码去重和审阅。

原远端恢复锚点仍为comment `5709839864`；此次未更改评论或远端仓库。
新SHA的必需CI、安全告警37high的完整路径与复扫、DuckDB历史间歇性问题、独立原生
GUI/IME/安装、Provider/媒体、A0/B1/ProductRC/FormalRC等均未由本轮宣告闭合。
未访问用户Mac、Keychain、旧profile、凭据或真实案件；Notion/Drive没有替代仓库。

## 历史检查点：2026-09-17 独立构建环境与回执授权边界

仍使用 [PR28 comment 5709839864](https://github.com/Eysn0130/analytix/pull/28#issuecomment-5709839864)
作为唯一活动恢复锚点。恢复时 fresh 查询原分支、PR、该评论和准确候选的 checks/jobs；
本节不将历史 PASS 自动转记给新 HEAD，不授权合并或发布。

本轮实际起点为 `d57b5282fccad025b5f4812a17056527980662c9`。
已推送配置提交 `0bc900e98d9f7152c902481776eaac49777273fb`，真实父提交为 d57b。
本节与回执修复和测试原子提交；包含本节的实际代码 SHA 从 GitHub 读取，不为自引用再提交。

### 已落实的构建依赖

- 新增 `.github/workflows/pr28-independent-environment.yml`，配置 1 文件 +141/-0。
  使用现有标准 Ubuntu runner、固定 action SHA、只读权限、无持久 checkout 凭据；
  限原分支、20 分钟、1 天产物保留，只导出准确公开 HEAD 的祖先对象及锁定工具输入。
  原 Development CI、CodeQL、路径过滤、依赖锁和门禁不变，无新付费设施或 Provider 调用。
- `35209039144` attempt1 在 0bc 上 SUCCESS。源代码 artifact `10490972138`，
  Go `10491192006`，Node `10491477101`，Rust `10490689192`；摘要及恢复步骤在原评论。
  全部外层/内部 SHA256、ZIP CRC 和依赖输入摘要校验通过。产物过期时仅按实际需要
  重运行既有取源作业；不得以重试替代测试根因修复，也不得回退当前分支。
- 独立 Linux 的完整 tracked checkout 已建立：0bc tree
  `0dfcaa5bae4caea214d1804c57cc5d8e8dd020bf`，6,363 个 blob/文件模式、157 个真实历史
  提交、唯一公开根和 `git fsck --full --strict` 均通过。不是四文件索引或重造父历史。
  Go1.26.4、Node22.22.1/npm10.9.4、Rust1.94.1/cargo1.94.1 实际运行且输入与锁一致。
  npm lifecycle 脚本未在取源中执行；完整源码不等于原生安装、合法资源和 GUI 已验收。
- 配置静态/四段 shell 语法检查、7 项合成 Git 导出正常/拒绝测试通过；这些是环境验证，
  不是产品功能或模型测试。终端 DNS 仍不可用，连接器读写可用，两者不能混为权限问题。

### 本批真实产品修复与验证

- `canvasediting.Service.Apply` 的 pending/applied 重复点击分支，在原 operation 的
  Status 返回后、暴露回执或改变 proposal 状态之前，再校验 principal、scope 和取消状态。
  撤权/取消只返回原 operation ID、UNKNOWN 和 ErrUnavailable，不返回磁盘 revision 或
  committed 状态，不再次写盘；正常重新授权仍能查询该原操作。保留已有 current.id、CAS、
  文件检查与唯一 Core，不改变跨层 API，不声称关闭 CodeQL 告警。
- 生产 `packages/runtime-go/internal/app/canvasediting/service.go` +6/-0；新增测试
  `packages/runtime-go/internal/adapters/outbound/filestore/canvas_receipt_test.go` +137/-0。
  一个顶层测试含 8 个子场景：applied/pending × 合法/撤 scope/撤 identity/取消。
  使用真实 object service、filestore、文件 CAS 与 Registry；包装器仅模拟丢失回执和
  Status 返回后的授权变化，授权 projector/identity 为可控测试边界，不冒充完整隐私链。
- 原实现 RED：8 子场景 2 PASS / 6 FAIL，exit1；修复 GREEN：8 PASS，exit0。
  非特权 uid1000、独立 HOME/TMPDIR、Go1.26.4、GOPROXY=off 下，完整受影响两包
  normal/prod 各 268 顶层 PASS、4 既有 SKIP、0 FAIL，exit0。两种模式不重复计数。
  `go test -race -count=1 -p 1 -parallel 2 -json ./internal/app/canvasediting
  ./internal/adapters/outbound/filestore -run 'TestCanvas'`：9 顶层 PASS、35 子场景 PASS、
  0 SKIP，exit0；子场景内含于顶层，不相加。均在 `packages/runtime-go` 执行。
  完整包命令去掉 race/run 过滤，production 模式增加 `-tags analytix_prod`。
- 初次 root 包测试 267 PASS / 4 SKIP / 1 FAIL：root 可读 chmod000，导致
  `TestCaseBindingObserverClassifiesUnreadableFile` 失败。原失败保留，改用非特权测试身份，
  未改用例或断言。4 既有跳过为 BundledCodecPackageContract、FilesystemAliasesShareReceiptBinding、
  OfficePackageSavedSyntheticFixtures、CreatePlanAutoSelectionDoesNotOverwriteFilesystemAliases；
  当前 Canvas/capture/receipt 测试无跳过。不得据此声称零跳过或完整安装 Office 验收。

### CI、安全和 DuckDB 的真实边界

- d57b 的 Development `35203579095` attempt1 已完整分页：51/51 作业 SUCCESS，
  包括 gate `105160409258`；0bc push 未取消这些已结束作业。两个 Go 日志
  `105144536417` / `105144536403` 确认实际 checkout
  `26a2e0ee820bbd743a57779f8aa93b523040f826`，并确实选择 Canvas/filestore 测试。
  303/297 是 package 分区数量，不是用例数。此证据复用，不重复全量执行旧测试。
- d57b CodeQL check `105144046012` 为 FAILURE、37 high/37 annotations；旧“缺少三个
  配置/NEUTRAL”只保留为历史中间观察。完整 source-to-sink 尚未取得，未关闭或 dismiss。
  0bc Development `35209046530`、dynamic CodeQL `35209040822` 及本批新候选必须 fresh
  核验；环境取源 SUCCESS 和旧 d57b 开发测试通过不能替代它们或安全准入。
- 在准确锁定 Rust/DuckDB 依赖、未改 Rust 源码的 0bc 独立环境，原失败定向复现：
  `cargo test --offline --locked --manifest-path tools/analysis_compute/Cargo.toml
  --test stats_query_cli output_file_cases::query_stats_rows_cli_writes_result_payload_to_output_json
  -- --exact --nocapture`，exit101，同样 INTERNAL index0/vector-size0，1 FAIL。
  堆栈明确在 `tests/stats_query_cli/output_file_cases.rs:16` 的第一次 inline 调用；第二次
  output-json 调用未执行。路径为 `query_rows.rs:124` → `session.rs:319` 的范围非空
  COUNT 检查 → `duckdb_utils.rs:174`。这不是输出文件写盘失败，也未完成最小复现或根因修复。
  不加 SQL 猜修、重试、串行、假计数、skip 或弱化 coverage 条件。原失败日志仍保留。

### 下一依赖有效动作

继续原 CanvasHost/Adapter、Core scope/projector/capture、native selection 工具和原 Composer
引用机制。当前 Canvas 仍缺 model-selection-read/propose/capture；现有原生工具仅由 Office
实现对应分派，schema 的 parts/workbook 不等于 Canvas ops。预览切换/隐藏会关闭 Core
会话，未接受 proposal 的内存生命周期需要与引用、折叠、第三对象回收一起闭合。
不得只加引用按钮或拼原始 Scene 到模型；引用不自动发送，草稿/附件保全，未知结果查原操作。
先补有界选择、真实投影和跨线程/epoch/撤权正反例，再原子贯通接受/CAS/重开/撤销的消费者。

DuckDB 下一步针对 inline 的实际 SQL/计划做最小复现；CodeQL 需完整路径和流证据后按共同
根因修复并复扫。完整 Canvas 主会话链、Office/图片共同旅程、独立 macOS ARM64 安装、
原生 GUI/中文 IME、合法资源、当前受保护 Provider 授权/费用和 Product/Formal RC 均仍未验收。
不依赖用户旧 Mac，不访问旧 Keychain/profile/案件/凭据；Notion 非权威镜像不阻塞 GitHub。

## 历史检查点：2026-09-17 Canvas 受控写入修复

当前活动恢复入口为 [PR28 comment 5709839864](https://github.com/Eysn0130/analytix/pull/28#issuecomment-5709839864)。
先 fresh 读取该评论、PR refs/checks，再读下方历史快照；旧评论和旧 PASS 不覆盖它。
分支仍是 `codex/workbench-product-delivery-20260914`，PR28 Open / Draft，未合并。

本轮代码基线 `57d1add9960f2f72ab926dd34695ac2f5b778d8a`；已非强制推送
`0f4c7af4c4e0d0f2b4057069ff81d6bf9f7fd520`。本页为后续文档提交，包含本页的
提交与最新 HEAD 从 GitHub 获取，不把代码候选检查自动转记到新 HEAD。

- 修复 `canvasediting.Service` 的 Apply / RecoverOperation：managed capture 的
  session-ID 参数使用 Core 的 `current.id`，不再误传绝对 workspace 路径。
  原路径、重新授权、CAS、回执与释放检查不变；不是 CodeQL 告警关闭证明。
- 三个代码文件 +187/-17：生产 1 文件 +2/-2；测试 2 文件 +185/-15。
  真实 Registry 的 18 个成对场景覆盖合法 apply/undo/resume、跨线程、旧修订、
  撤权、硬链接和持久化失败。既有 Canvas/PNG 文件 CAS/重启/撤销测试改用真实
  Registry，去掉忽略 session 参数的宽松替身；未新增依赖或放宽门禁。
- 本地 Go1.23.2、独立 Linux、23 包标准库源码闭包，不是完整当前 checkout。
  新场景对原实现 12 PASS / 6 FAIL，修复后 18 PASS；相关 5 包共 23 个顶层测试
  PASS、0 SKIP，race 同样通过，重复运行不重复计数。持久化 spy 不冒充真实写盘。
  module-mode 调用因仓库要求 Go >=1.25 而 BLOCKED；完整 filestore 与当前 CI
  的真实结果仍须查询。历史快照 6,341 blob 校验不代表完整新 HEAD 可构建。
- 基线 `35197821626` attempt1 的 51 作业已完整分页：46 SUCCESS、2 FAILURE、
  3 CANCELLED。实际 checkout `e29cef4f857dd334fc446ff016343d0fc293fe60`。
  data_engine `105125131513` SUCCESS；analysis_compute `105125131323` FAILURE：
  `output_file_cases::query_stats_rows_cli_writes_result_payload_to_output_json`
  遇 DuckDB index0/size0 INTERNAL 错误。没有最小复现，不猜测 SQL 修法；原失败保留。
- 代码候选 Development `35202453830`、CodeQL check `105140340461` 必须 fresh
  核验；后者初次返回缺少三个配置的中间状态，不是通过。此前 37 high 未关闭。
  当前连接器拒绝完整 code-scanning alerts 端点，不能据此认定无告警。
- 下一步先核对准确 HEAD 的 Go/filestore 和完整 CI，再复用现有 CanvasHost、Core
  objectediting scope/projector 与 native selection 工具补齐安全引用到同一主会话。
  Selector 必须重新捕获，引用不自动发送，未接受不写盘；不另建权限/提交体系。
  同项目重新授权正例、旧线程句柄拒绝、草稿保全、审阅/CAS/恢复都仍在验收范围内。
- 完整 Canvas 对话链、Office/图片共同旅程、原生 GUI/中文 IME、独立 macOS ARM64
  安装、真实 Provider/媒体与 Product/Formal RC 仍未验收。终端 DNS、工具链和独立
  原生设施/授权缺口分别记录；未访问用户 Mac、Keychain、真实 profile 或案件。
  Notion mirror: PENDING；GitHub 可独立恢复。无 main/force/merge/tag/release 或新付费设施。

## 最新保留快照

- [`2026-09-17-round2-integration.md`](2026-09-17-round2-integration.md)：
  Round2 代码已按两批集成至 `d7f542ab…`，保留 Round3 隐私修复；含本轮计数、
  独立复验、CI/CodeQL 证据边界及当时恢复锚点；按需读取，不覆盖上方新检查点。

- [`2026-09-17-round3-privacy-projection.md`](2026-09-17-round3-privacy-projection.md)：
  新增 Go 隐私投影修复、精确金额与受信任协议边界回归、受限环境验证及下一步。
  当时 Round2 未集成的状态已由上方集成检查点更新；其余证据保持原有范围。

- [`2026-09-17-knowledge-acceptance.md`](2026-09-17-knowledge-acceptance.md)：
  三层知识治理验收、Round2 本地候选原件归档、SHA/交付清单与线程接续补充。
  与下面的 PR #28 产品检查点一起读取；归档完成不表示补丁已集成或产品通过。

- [`2026-09-16-pr28-continuation.md`](2026-09-16-pr28-continuation.md)：
  当前 Draft PR #28、`codex/workbench-product-delivery-20260914`、精确候选 CI、
  CodeQL/review、GUI 外部阻塞、Notion/Drive 知识治理和新线程恢复协议。恢复当前
  workbench / native annotation 施工优先读它；其中 SHA 与 CI 仍须 fresh 核对。
- [`2026-09-10-owner-replacement.md`](2026-09-10-owner-replacement.md)：
  新 Owner 替换、R131/B1 同源链聚焦接受、无自动化 Slice 路线与当时 A0/B1
  缺口。自 2026-09-16 起它不再是最新施工入口；仍保留当时证据边界和历史恢复背景。
- [`2026-08-05-damaged-cache-retirement.md`](2026-08-05-damaged-cache-retirement.md)：
  当时 clean canonical 基线、损坏缓存退役、v3 保全与容量状态。仅保留该日期的
  缓存和恢复历史，不是当前 HEAD、CI、writer 或施工顺序的依据。
- [`../document-consolidation-register.md`](../document-consolidation-register.md)：
  项目文档分类、碎片归并和历史文档治理登记。

## Historical

- [`2026-08-04-controlled-thread-checkpoint.md`](2026-08-04-controlled-thread-checkpoint.md)：
  当时的 canonical 收敛、缓存容量与 Milestone A/B gap；其中旧镜像保留和容量
  blocker 状态已由 2026-08-05 checkpoint 取代。
- [`2026-07-25-thread-close-route-audit.md`](2026-07-25-thread-close-route-audit.md)：
  当时的产品路线复核、Codex/Claude Code/OpenCode 对照、Obsidian 审核和
  上游 currentness；不替代当前知识治理或 2026-09-16 checkpoint。
- [`2026-07-25-first-stage-pause.md`](2026-07-25-first-stage-pause.md)：
  第一阶段施工暂停时的工作区、Goal、P0-P4、资金插件、上游吸收、验证和风险
  快照；仅作 Historical 背景。

## 新线程恢复顺序

1. 先读取当前目标 PR 元数据，并在其 fresh head 分支读取根 `AGENTS.md`；
   文档工作再读 `docs/AGENTS.md`。读取 `docs/analytix/README.md`、本页、
   当前产品检查点及其治理/归档补充和
   [`../knowledge-base.md`](../knowledge-base.md)。
2. fresh 获取当前 `main`、目标分支、PR、Base SHA、HEAD、Draft/merge 状态和
   current checks；有本地 worktree 时再执行 `git status --short --branch`、
   `git rev-parse HEAD` 和 `git diff --stat`。
3. 比对交接记录与当前 Git/GitHub；任何差异都按新的事实处理，不覆盖用户修改，
   不把旧候选 SHA 的 PASS 自动继承给新 HEAD。
4. 动态查看 `openspec/changes/`，读取当前用户授权范围涉及的实际 `tasks.md`，
   重新计算任务分母和完成数。active change 存在本身不扩大当前施工范围。
5. 检查当前精确 HEAD 的 CI、CodeQL/review 和必要的外部环境 seam。对“上次命令
   仍在运行”“结果丢失”“未复跑”“外部环境缺失”分别标记，不改写成 PASS。
6. 如果 Notion connector 可用，读取 **Analytix Engineering Workflow & Templates**
   Skill，并检索 **Construction Ledger** 中 `Repository = Eysn0130/analytix` 的
   非 `Complete` / 非 `Superseded` 项。Notion 是索引；与 GitHub 冲突时先保留
   差异，再按仓库、accepted target 和 fresh evidence 修正镜像。
7. 只为当前批准的施工切片建立验证计划；从依赖已满足、可验证的下一个 gap 继续。
   交接中的 backlog、旧 PR 或历史 OpenSpec 行不自动进入范围。
8. 一个线程产生 durable 改动、验证结论、阻塞或 next-action 变化时，先更新仓库
   事实/证据，再同步 Notion。Google Drive 仅用于正式报告、表格、PPT、release /
   evidence export，不承担当前施工状态。

## 交接最小合同

每个会话只有在状态发生有意义变化时才需要新 checkpoint；不要为无变化的读取建立
大量日志。新 checkpoint 至少能回答：

| 字段 | 必须回答的问题 |
| --- | --- |
| Repository / Branch / PR | 正在改哪里，是否还是同一条施工线？ |
| Base SHA / Start HEAD / End HEAD | 本轮从哪里开始、实际交付到哪里？ |
| Authorized scope | 用户本轮实际允许做什么？ |
| Changes actually made | 真正修改了什么，不是计划做什么？ |
| Checks actually completed | 哪些命令/CI 最终有结果？ |
| CI / CodeQL / review | 当前候选还有哪些外部门禁？ |
| PASS / FAIL / BLOCKED / UNVERIFIED | 每个关键 seam 的真实状态是什么？ |
| Next dependency-valid action | 新线程第一项应该做什么？ |
| Out of scope | 哪些明确没有做，避免新线程误补？ |
| Notion mirror | Construction Ledger 是否已同步或明确 pending？ |

## 交接状态词

| 状态 | 含义 |
| --- | --- |
| `PASS` | 在记录的工作区、SHA 和环境中实际执行并通过。 |
| `FAIL` | 实际执行且失败，结果仍有效。 |
| `ABORTED` | 人工终止或因范围冲突终止，不是产品失败证明。 |
| `RUNNING` | 检查仍在执行；不得推断最终结果。 |
| `RESULT LOST` | 命令曾运行，但无法取得最终退出状态或完整结果。 |
| `UNVERIFIED` | 没有满足真实环境、凭据、主机、数据或重新执行条件。 |
| `NOT IMPLEMENTED` | 代码路径或生产组合尚未存在。 |
| `BLOCKED` | 明确前置条件未满足，无法进行该项验证。 |

不得用文件存在、fixture 通过、进程启动、历史报告标题含 `final`、旧 SHA 的绿色
检查或 active OpenSpec artifact 已生成来替代产品级验收。

## 单一事实链

跨线程连续性采用以下单向关系，不建立自动双向同步：

```text
GitHub current code / specs / PR / CI / evidence
                    ↓
        handovers（恢复索引与时点快照）
                    ↓
Notion Construction Ledger / ADR / Research（非权威结构化镜像）
                    ↓
Google Drive formal deliverables（需要时才输出）
```

聊天记录可以帮助解释，但不是必需依赖。即使更换会话、Notion 临时不可用或旧线程
无法访问，只要 GitHub 可读，新线程仍应能从本页和最新 checkpoint 恢复到可施工状态。
