# 三层知识治理验收与 Round2 归档接续

Status: Operational supplement / dated snapshot; not product acceptance
Record ID: ANALYTIX-KNOW-20260917
Applies to: PR #28 documentation governance, historical artifact intake and cross-thread recovery
Observed source HEAD: `2c96809b16278d15927f50ae312eb41281bfd288`
Date: 2026-09-17 UTC / 2026-09-16 America/Los_Angeles
Source of truth: current GitHub, accepted requirements and commit-scoped evidence

本记录补充 [PR #28 产品检查点](2026-09-16-pr28-continuation.md)，不替代产品范围、
验收门禁或 [知识治理规则](../knowledge-base.md)。用户授权本轮核验并落实
GitHub + Notion + Google Drive 的记录和接续机制；本轮不应用产品补丁、不合并、不发布。

## 核对到的三个不同版本

| 含义 | SHA / 状态 |
| --- | --- |
| PR base / main 观察点 | `ce96cf12581acfa0e19fae7c6aa9c709371012c8` |
| Round2 补丁的远端基线 | `5a2db4e67c739b0fab41ed48cde6fe4d16f13de7` |
| 本轮治理施工起点 | `2c96809b16278d15927f50ae312eb41281bfd288` |
| Round2 独立本地候选 | `6678364c16c0e5142df652753761e31e9ac6d58f`；NOT INTEGRATED |
| 施工分支 | `codex/workbench-product-delivery-20260914` |
| PR | #28，Open / Draft / not merged；需 fresh 查询 |

GitHub compare 证实 `5a2db4e...` 到 `2c96809...` 只有一份提交，改动仅在三个
治理文档：`handovers/2026-09-16-pr28-continuation.md`、`handovers/README.md`
和 `knowledge-base.md`。因此这次远端前移不是 Round2 产品补丁集成。

本治理提交继续只改文档。不要把独立重建的本地候选当作远端祖先、直接 merge 其
历史或假设该 SHA 能从远端 cherry-pick。旧报告中“连接器未暴露写入动作”仅是
当时环境事实，不是账号永久没有 push 权限的结论。

本轮观察 `2c96809...` 的 Development CI run `35171520165` 为 queued、无最终
结论。新治理 HEAD 也必须读取自己的 checks；本记录没有声明产品 CI、CodeQL、
真实 GUI、安装或 release readiness 通过。旧检查点的 Keychain 环境阻塞没有在本轮解除。

## 已恢复的原件

四份相关 Round2 原件已从用户 Library 完整恢复，保持原名和原字节，并以用户请求的
历史交付归档存入私有 Drive `Analytix/05 Archive/2026-09-17-Round2/`。
这只是归档例外，不把日常源码或可变计划批量镜像到 Drive。

| 原件 | 字节 | SHA-256 |
| --- | ---: | --- |
| `Analytix-Round2-Implementation-2026-09-17.md` | 17707 | `2ef93ccb6c6a56cc19ee47b3b35df493a968043cd1f3aa7e8ad164932fe754ba` |
| `Analytix-Round2-Next-Plan-2026-09-17.md` | 7235 | `2ab92260cefeab5e8f48f20d561250f6d37bb6f618706597d2aa6b0ce19d7ae6` |
| `Analytix-Round2-2026-09-17.patch` | 33255 | `06af7fbaf6f889853203525288fc3cd720e5f04f2bbecbaf6bab4a15855a5bd1` |
| `Analytix-Round2-Evidence-2026-09-17.zip` | 41804 | `04bb288377d7cad9a07dc394f49aac39aa9e6a6997b687854dbe0a712afe6452` |

ZIP 的 18 个条目 CRC 校验通过；内部 SHA256SUMS 的 17 项校验通过；外部三份
文本/补丁与 ZIP 内同名文件一致。四个原件和新建归档 manifest 的 Drive 字节级
下载回读一致。模式筛查没有发现所检查的凭据格式；这不等于完备的敏感数据证明。
私有文件链接仅存 Notion 和私有交付清单，不放入公开仓库。

原报告的 48/48 focused tests、8/8 CI helper 以及依赖安装失败均为历史结果，本轮
没有重新运行这些产品测试。原 Next Plan 仍是计划，不代表已执行。完整安装依赖后的
插件套件、CodeQL 逐项闭合、Rust 回归和 GUI 旅程不能由这些有限测试替代。

用户提供的旧会话分享链接在本轮无法读取；相关文件是通过 Library 恢复的，不能
宣称已读取分享全文或验证了“最后附件”的精确关联。这个来源关联缺口独立保留，
不妨碍对已取得原件归档和继续核验仓库。另有更晚文件时追加关联，不覆盖原件。

## 本轮治理修正与验收边界

- 文档地图移除重复维护的过期“最新交接”指针，转向 [恒定入口](README.md)。
- 文档地图以现行 GitHub / Notion / Drive 分工替代旧 canonical Obsidian 描述；
  旧 vault 和历史文档保留，不迁移、不删除、不自动同步。
- Notion 首页改为仓库实际路径；原 Workflow Skill、Construction Ledger、ADR、
  Research 数据源继续复用，不新建第二套数据库或重复 PR 施工条目。
- 单独记录本治理项，产品项仍维持未完成；治理完成不传递为产品完成。
- 验证文档时使用从 GitHub 读取并通过 blob SHA 核对的文本工作副本，运行
  `git diff --check`，检查变更链接及文档差异；不是在用户 macOS canonical
  worktree 上运行产品验证。本地临时 Git 历史不会合入产品仓库。
- 本轮未变更 Notion/Drive 分享设置；Drive 目标归档父目录返回 `shared=false`。
  不据此推断所有 Notion 页面或整个工作区的公开权限均已审计。

## 以后每个线程的最小记录

只在目标、改动、证据、阻塞或下一步发生实质变化时更新拥有该工作的 checkpoint。
计划放既有 OpenSpec / docs 归属位置；不要为了模板另建目录。摘要应区分：

```text
Record ID / repository / branch / PR
Current user-authorized scope and accepted decisions
Proposals not approved / explicitly out of scope
Observed base / start HEAD / local candidate / tested SHA / delivered remote HEAD
Actual changes and changed files
Checks completed: command, environment, scope, result, evidence locator
Blockers and exact next dependency-valid action
Artifact manifest: name, bytes, SHA-256, source version, destination, readback
Notion mirror status / Drive export status / unresolved source association
```

checkpoint 自身的 Git commit SHA 不能可靠地写进该提交自己的内容；正文记录观察点，
最终 delivered HEAD 在提交后的 PR、Notion 台账和交付报告中回填。不要为修正自己的
SHA 无限追加提交。测试 SHA 与最终包含文档的 delivered SHA 要分开记录。

会话只提取用户决定、真实结果、原因和下一步，不整段复制聊天、隐藏推理或原始
工具 transcript。引用分享链接是辅助定位，不是永久可读保证。文件名里的 `final`、
云盘修改时间或较新的聊天日期不能决定工程权威。

## 新线程实际接续顺序

1. fresh 读取 PR #28。仍开放时使用它的当前 head 分支；已经合并或被替代时，先
   识别真实后继再继续。不要盲目从默认 main 读取未合并分支的最新交接。
2. 在该 ref 读取适用 `AGENTS.md`、[文档地图](../README.md)、[交接入口](README.md)、
   产品 checkpoint、本补充以及 [知识治理规则](../knowledge-base.md)。
3. 获取 fresh HEAD / base / CI / review；有真实本地 worktree 时再读取工作树状态，
   无主机访问时明确局限，不假装检查过用户电脑。
4. 读取 Notion 的既有 Workflow Skill 和原 PR #28 Ledger。仅缺 Notion 时，
   仓库接续仍可进行；需要历史补丁却拿不到原件时，只阻塞依赖该补丁的步骤。
5. 本次新发现的首个集成缺口是 Round2 本地候选：由获授权的唯一集成写入者校验
   原件 hash，按 fresh HEAD 审阅每个差异，再执行真实锁定依赖下的正/负例测试。
   不能机械按旧 5a2db4e 基线应用，也不能 merge 独立重建历史。
6. 根据最新 CI 的真实失败再定位 Rust / CodeQL；不降规则、不弱化断言。GUI 仅在
   正常凭据与环境权限具备后执行。未闭合门禁继续保留 Draft。
7. 结束时先记录仓库事实，再更新原 Notion 条目；只有明确正式交付/归档需求时才
   保存 Drive 原件与 manifest，并回读核验。不因换线程新建 PR 或产品总账。

安装连接器不等于自动归档或自动跨线程加载。本机制是每次任务内的显式读取和
有界写入，不是后台同步服务；本轮没有建立定时任务、双向同步或产品运行时依赖。
