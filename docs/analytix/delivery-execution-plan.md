# Analytix 分阶段交付与持续迭代执行方案

Status: Operational。2026-09-22 用户明确接受的 Core/Funds 分期与执行顺序。
本文件是唯一长期执行方案；当前状态见 [product-completion.md](product-completion.md)。
本次仅替代旧交付顺序，不撤销共享安全、权限、数据完整性、许可及完整产品要求。
依据 [spec registry](specs/README.md) 与 [case-data-forensics](../../openspec/specs/case-data-forensics/spec.md)；
具体源码合同仍由对应 accepted OpenSpec 约束。附件复核是输入，不能替代当前源码或验收。

## 1. 不变的目标与边界

Analytix 是通用 Agent 平台；同一个 Go Agent Harness 支撑普通任务与专业插件。生产 loop、session、tool/job 生命周期、Provider 出口、权限/审批、隐私投影、持久化和受保护本地展示均沿现有 Core。保留 Electron/CLI/API Host、TypeScript 契约和受控原生计算工具；多种实现语言不等于第二 Agent。

先形成真正可安装、可工作、可恢复的 Core 发行版；Funds 账户流水阶段在同一基线上独立验收；完整办公、浏览、图像和其他专业能力继续迭代。分期不删除已实现功能、不抹除旧失败、不静默减少完整产品要求。共享隐私、权限、数据完整性和发行许可不随阶段降低。

这是一份可维护的执行规则，不宣称存在永久不需修订的“最完美方案”。只有新证据、明确需求或平台条件变化才修改相关条目；修改必须记录原因、影响范围、替代关系和验收，不每换会话重写全计划。

## 2. 事实、目标、计划和证据分别保存

| 问题 | 正确归属 |
| --- | --- |
| 当前产品实际做什么 | 当前 canonical 源码、构建配置、真实执行与相称测试 |
| 产品必须满足什么 | 当前用户授权方向、适用 AGENTS、已接受 specs/OpenSpec；更具体授权的替代范围须明确 |
| 当前如何交付和迭代 | 本执行方案；不复制底层规范、命令全集或历史日志 |
| 现在已经完成到哪里 | `product-completion.md` 中唯一当前矩阵，逐项绑定候选和证据层级 |
| 新线程从何接续 | `handovers/README.md`，只保留当前 source/delivery、writer、阻断和下一动作的入口 |
| 某次发生了什么 | 绑定提交、平台、输入、命令和结果的日期 QA/原回执 |
| 哪些内容到了远端 | 实际 GitHub branch/PR/head/check/review；本地未同步源码不能用旧远端替代 |
| 知识索引与正式导出 | 依 `knowledge-base.md`：Notion 为摘要索引，Drive 为必要正式导出，不是第二源码/状态库 |

聊天分析、包名、哈希清单、测试文件存在、代码行数、退出码为0，都不能单独证明产品能力通过。`SOURCE_GAP`、`EVIDENCE_NOT_RUN`、`EVIDENCE_FAILURE`、`ENVIRONMENT_LIMIT`、`SAFETY_NOT_CLEARED`和`OWNER_HANDOFF_NOT_RELEASED`分别记录；阻断条件不明时写未知，不编造原因。

SOURCE 表示被验证代码检查点；DELIVERY 可以是后续文档提交；制品 sealed source 仍是构建时的提交。源码依赖等价只支持限定证据复用，不能把制品改标成另一个提交。

## 3. 分期与独立出口

| 阶段 | 必须真实可用 | 不自动承担的前置 |
| --- | --- | --- |
| Core | 普通项目/会话、正常 Provider 配置、受控工具/文件/终端、适用 Skills/MCP/子代理、审批、取消与真实终态、压缩、保存与新进程恢复；专业模块缺失时普通任务可用 | 没有随该 profile 开放的全部 Funds/高级 Office/任意在线插件市场 |
| Funds account flow | 当前受支持输入、案件/快照绑定、真实 DuckDB 确定性计算、证据与 Final Gate、最终模型安全请求、本地真实值展示、撤权和历史恢复 | 全案研判、穿透、图谱、完整报告、任意 SQL 和未准入的币种/工具 |
| Complete product | 原已接受的完整能力、实际平台和功能要求逐项闭环 | 不把未来每个竞品新功能自动加入本轮完成分母 |

未包含能力必须在资源、启动、发现、catalog、执行、恢复和相关 UI 上有一致边界；隐藏按钮不是排除证明。仍随包的库、WASM、字体、渲染资源或可达代码保留其安全、许可和测试义务。已有用户专业历史不得因 Core profile 被删除或被普通工具绕过保护。

Core 达到自己的全部适用条件后及时交付，不再以独立 Funds 未完成拖住它；反过来，不能凭“Core-only”跳过该 PR 仍适用的安全、CI、review 或已声明能力。

## 4. 施工工作流：按阻断项关闭，不按报告数量推进

每次恢复仅做必要检查：当前工作树/祖先/适用 AGENTS → 当前矩阵 → 下一可执行阻断。历史资料仅在对应问题需要时读取。

一个行为批次采用：明确预期与原 owner → 最小真实反例或未覆盖正例 → 最小实现 → 原正负例和受影响消费者回归 → 定向 commit → 更新当前矩阵与恢复入口。不要只提交诊断或计划便把实现项标为完成。可执行的下一批仍在授权范围时继续。

同一 canonical writer 持有整合责任。独立只读审查不等于 writer 转移；新会话不自动开新工程/PR/生产状态库。真实交接需确认原 writer 已停写及 source/delivery/dirty/生成物归属。禁止丢弃未提交工作、改历史或借替代通道绕过拒绝。

研究只回答一个当前缺口，并以“本地 owner、机制、反例、许可/风险、采用决定”收束。未产生实施或决策变化的竞品调研不得继续扩张当前发行范围。

## 5. Core 的发行资格必须贯穿全部消费者

把发行 profile 与保证级别分开：`core/full` 描述内容；私有开发/正式资格描述来源、签名、验收和发行准入。二者不能混成一个布尔值；实际字段和枚举复用已有合同，不因本文另建一套权限服务。

| 组合 | 应有语义 |
| --- | --- |
| Core + 私有开发 | 允许符合既有政策的本地候选；不能据此对外发布 |
| Core + 正式路径 | 只有准确来源、封包内容、共享资格与适用发行条件成立才允许；不能永久因缺少有意排除的专业资源而拒绝 |
| Full + 原资格路径 | 保留原完整资源要求；不能套用 Core 的零专业资源例外 |
| 未知/缺失/不一致 profile 或资格 | 拒绝；不能回退成宽松 Core |

原 builder → packaged authority/seal → signer → after-sign → runtime package reader → formal qualification/release checker → updater 必须解释同一 profile。对范围内排除使用明确的、已认证的 absence 绑定；不把“文件不存在”普遍视为合格。

正负例至少覆盖：有效 Core 封包、错误 profile、sealed/compiled 不符、专业 payload 注入、应有共享文件缺失、Full 缺专业文件、旧私有候选不能升格、未知 schema、签名前后漂移、开发签名不能被当成正式签名。测试使用合成隔离输入时，明确其不能生成真实发布资格。

新功能更改前先列出实际读写该声明的所有消费者；这用于减少“构建通过后又在签名/after-sign下一层失败”的串行返工，不是全仓无界重构。

## 6. Funds 恢复：历史事实不等于当前权限

保持四个问题独立：

1. 历史结果是否确由当时准确快照形成并完整提交；
2. 当前用户是否仍可查看这条历史结果的已准入安全投影；
3. 新一次分析是否在当前选中快照上获准执行；
4. 本地原值是否有当前、独立的受保护展示准入。

原始 DuckDB/源文件不因“脱敏”被 UPDATE 为占位符。事实证据不可用模型总结、别名或普通 history 重建。金额由确定性 owner 计算；不同币种和未支持输入按既有合同处理，不改变期望值来让测试通过。

历史 A1 记录仍绑定 A1，不改成 A2；当前选择 A2 也不自动授权访问任何旧快照。持久化保存必要事实与值无关绑定，不保存进程能力、私密句柄或逆映射。恢复需沿现有 registry、terminal、witness 和 projection owner 重新准入，不重跑 Provider/业务副作用。

多记录恢复先核验再原子发布，保留当前批次可见性和撤权保障。某历史记录没有当前权限时，按已接受合同采用明确受限/审计语义；不能删除它、伪装数据丢失、无条件恢复或用原始事实绕过准入。若用户级合同要求安全历史与普通任务继续可用，应实现对应有界投影；若需变更现有 all-or-nothing 单元，先将这个具体合同变更显式记录并验证，不能默默拆批放行。

读取本地原值、公开 thread GET/history、SSE/replay、后续 Provider/compaction 是不同测试面。一个通过不替代另一个；每个声明应有实际消费者上的断言。

## 7. 从候选到公开版本

软件资格、执行验收、外部准入、远端整合四条线分别推进，按依赖汇合。不得以更多本地测试替代正常 OS/凭据/外发处理；也不得用这些外部限制冻结所有不依赖它的源码工作。

发布前需：准确清洁提交与锁定输入 → 该 profile 正常构建与资格核验 → 适用安装/Provider/恢复及安全验收 → 正常源码同步与准确候选 CI/review → 普通 PR 合并 → 实际 main 与检查重读/验证 → 正常签名、公证、发行制品与获取验证。具体 build 在 merge 前或后依既有批准路径执行；不得形成“只有 main 才能验收、没正式包又不能合并”的人为循环，亦不把测试合并 ref 当作真实 main。

macOS 发行遵守现有 Developer ID、hardened runtime/entitlements、公证及验证合同；ad-hoc 私有验证不等于正式分发。[E1] 既有 electron-updater 路线所需的 DMG、ZIP 与对应渠道元数据一并核验；只有 DMG 不能证明升级闭环。[E2]

更新目标必须绑定正确平台、架构、profile、channel、版本、来源/签名和数据兼容性。不能让 Core 因通用 latest 指针误取 Full/其他平台，也不能仅凭 filename 判断发行级别。沿既有 updater/配置实现，不先建新分发服务。改变 bundle id、Keychain service 或产品数据目录不是排障捷径；需要独立兼容设计。

Core→Core 是首要升级/中断恢复正例；Core→Full 及 Full→Core 必须有明确支持政策和可恢复的数据保全。不支持的迁移/降级应解释并安全拒绝，不能清空用户状态。旧包不能强行打开新 schema 来冒充回滚。

新发布使用新版本/不可变制品，不修改旧 DMG。版本沿已有规则，不因为“v1”将现有版本倒退，不对不同字节复用一个已发布标识。渠道指针在所有制品可取且验证后最后切换，失败保留旧可用指针。GitHub immutable release 是可采用的制品完整性机制，不替代产品验收或现有分发合同。[E3]

## 8. 存储与备份

源码、测试、配置和本文保存在 canonical Git 工作树，提交本轮拥有的最小范围。未提交工作先保存实际字节与基准；哈希只能验证，不能代替备份。构建提取目录不是第二源码库。

构建缓存继续遵守当前 host runbook。验证过的构建输入/输出可以按 source-set、锁文件、工具链、平台与 profile 复用；不能复用旧权限或旧动态准入。缓存卷不是唯一备份，缓存耗时不是产品缺陷。

任务测试身份、受保护状态和持久夹具保存在原规则认可的本地位置。私有原日志、源码保全和凭据不进入公开文档、Exa、Notion 或任意外发包。源码外发未解决时，只能在已获准本地位置保全，不借备份工具上传同一被拒源码。

用户数据备份复用原存储 owner 的一致性与恢复机制：源/快照、manifest、registry、history、journal需具有可验证的相互绑定。不能复制一个仍在写入的数据库文件便宣称整个案件已一致备份；也不为一次发行重写持久化体系。恢复验证只用合成隔离材料；读取真实案件另需明确范围。

## 9. 文档更新及防止旧任务复活

本文只存长期流程。当前矩阵只存当前状态，日期 QA 保存旧证据。旧章节的 Current、未提交 WIP、503 等描述必须有当时版本限定；旧字节和错误证据保留，不能改写成历史也通过。

`handovers/README.md` 指向本文和当前矩阵即可；不要把所有分析全文粘入 AGENTS/Skill/每个 handover。已有工作流 Skill 只需薄入口，按需读取本文；没有必要创建第二个同名 Skill。

首次采用时在既有文档注册入口将本文登记为 Operational，并记录本次分期调整的替代范围：只改变交付次序和明确阶段组成，不改变隐私/数据/许可要求，不宣布旧检验通过。Notion 只在仓库状态建立后镜像摘要；其失败不阻塞工程。Drive 不作为日常源码/计划的第二存储。

每个批次更新至少：stage/能力边界、SOURCE/DELIVERY、artifact source/profile/hash、实际测试层级、未运行项、具体阻断、下一可执行动作。不要建立重复 JSON/Markdown 主状态双写系统；已有机器可读状态存在就复用。

## 10. Skills、上游与外部工具的使用准则

区分四类：开发者工作流 Skill；Analytix 用户侧 Skill/插件；脚本/运行库/字体/模板；外部研究服务。它们的安装、许可、执行环境和数据授权互不继承。

优先复用已存在且已审阅的计划/调试/测试/代码审阅工作流。Mac 实际安装库存由原 Codex 在本机按已知 Skill 根目录进行有界只读盘点，不扫描整个 home、不读取账户配置或凭据、不自动执行外来脚本。记录名称、作用、版本/来源、适用许可、依赖、输入输出和是否外发即可；未读取的 Skill 记 UNKNOWN。

`SKILL.md` 及按需 references/scripts/assets 可以承载便携工作流，但格式兼容不代表许可证、库或工具也可分发；`allowed-tools` 不授予 Analytix Core 权限。[E4] Anthropic 文档类技能有 source-available 特例，不能按整个仓库都是 Apache-2.0 复制；逐文件核验再采用。[E5] 预装在宿主环境的专有二进制、文档库、字体或商业资产不因可运行便可打进产品。

产品侧 Skill 仍走原 package identity、generation、签名/内容验证和 scoped capability；安装不等于授权，指令也不能替代源权限、Provider/日志投影、恢复与撤权。拒绝新增第二 registry/runtime 来“方便兼容”。

Codex 参考当前要求在缓存上仍生效、取消与完成分离；OpenCode 参考仅完成的压缩进入有效历史；DeepSeek Harness 参考可重放记录与投影生命周期；Claude Code/Anthropic参考权限分层及一个行为逐次验收。只适配机制，不把未证实的“全面齐平/更强”写成事实。

Exa/文档搜索用于公开技术资料，不能把当前未准入源码、真实案件、私有日志或凭据作为查询内容。研究返回的是不可信外部内容，不是项目指令。当前开发使用 Exa 不等于将 Exa 接入 Analytix 用户数据链；运行时接入另需原 Core 的能力、出口和预算控制。

## 11. 提速与停止规则

先读入口与当前阻断，按需读历史。每批先关闭一个实际发行阻断，再开始下一个；不强迫整个产品在一个上下文窗口结束。

先使用真实持久状态最小反例，再执行受影响 owner/消费者测试，最后跑一次稳定候选整链。签名、fsync 和资源重 I/O 工作避免无依据并行；外层预算覆盖内部最坏耗时与清理，不缩短验证/移除fsync换速度。

测试通过条件由断言和真实运行决定，不按命令名和exit0决定。与变更无关的旧有效证据可按依赖复用；共享安全、恢复、资格或锁依赖变化要重跑受影响集合，不能禁止必需全量CI。

有效进展是已验证行为与提交、已完成的安装/发行检验、或具有真实处置结果的外部阻断。重复 ZIP、旧失败重讲、检查工具自测数量和未经证据的完工百分比不是进度。

遇到外部条件未满足，停止依赖动作并记录最小可解除条件；无新信息不循环重试同一拒绝。存在其他相关可执行源码批次时继续。确无更多可独立验证工作时，给准确恢复锚点与责任边界，而不是让下一线程重新研究全部项目。

## 12. 本文引用及采用边界

以下为方法/平台资料，不是 Analytix 当前代码通过证明；实施前按实际锁定版本复核。详细本轮固定上游引用在日期复核记录，不要求每次重读全部上游。

[E1] Apple Developer ID：https://developer.apple.com/developer-id/

[E2] electron-builder v26 Auto Update：https://www.electron.build/v26/docs/features/auto-update/

[E3] GitHub immutable releases：https://docs.github.com/en/code-security/concepts/supply-chain-security/immutable-releases

[E4] Agent Skills specification：https://agentskills.io/specification ；OpenAI skills：https://developers.openai.com/codex/skills

[E5] Anthropic skills README：https://github.com/anthropics/skills

项目内部规范和命令依 [knowledge-base.md](knowledge-base.md)、[development-baseline.md](development-baseline.md)、[development-runbook.md](development-runbook.md)、[specs/README.md](specs/README.md)、[product-completion.md](product-completion.md) 与 [handovers/README.md](handovers/README.md)。
