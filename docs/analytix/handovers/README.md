# Analytix 施工交接索引

Status: Operational。唯一恢复入口；原 Codex 保持唯一 writer，未交接。

Repository `Eysn0130/analytix`；canonical `/Users/sun/Projects/analytix`；
branch `codex/workbench-product-delivery-20260914`；[PR28](https://github.com/Eysn0130/analytix/pull/28)。

## 当前恢复点 — 2026-09-24 PDT

- 最新产品源码与完整私有 app/DMG/ZIP 绑定
  `14ab14cb9eb28611b1fd0c526865aee01bed5f95`；后继 `18d5f72f5` 只修正
  并发测试对新错误码的预期。原 `b9666a5af` 及全部有效祖先保留；恢复时先重新
  读取 HEAD/tree/dirty，不能用本行覆盖更晚提交。准确 `18d` PR CI 在本检查点
  仍运行；旧 `7b9cb2b4f` 56/56 成功不能继承。PR 仍为 Draft；真正 main
  最后直接核验仍为
  `ce96cf12581acfa0e19fae7c6aa9c709371012c8`，不是 PR 测试合并 SHA。
- ZIP 交接包的 soak 脚本修复已在原工程集成、测试、定向提交并普通 push；后继补上
  安装版定向诊断及审批门事件回放的真实 RED→GREEN 修复。28/3 等窄测和脚本
  通过均不构成 Mac 安装验收。
- 14ab 完整私有容器的 DMG 校验、安装副本严格签名、独立 Go Core 读包与
  准确许可审计通过；1,172 个实际依赖实例无强制工程阻断，Canvas 未随 Core
  分发，Lazy 替代已打包。该包仍为 `development_clean_non_publishable`。
- 历史 7b 独立 app 在合成隔离 Provider 下完成设置、重启、会话、工具、审批、
  用户输入、fork/resume 和 usage；完整 DMG 会话在审批允许时触及观察期限。
  后继 c25 安装版显式取消及审批后 fork 的定向会话通过。14ab 同一 DMG 安装
  副本在分开的 fresh 会话中通过工具/附件、审批、用户输入、显式取消、
  单次 fork 0→1、新 Main 历史恢复和第二轮完成 1→2；完整会话在四轮完成后
  触及原有 180 秒总观察期限，后段
  审批/派生/恢复未完成。更早的 65a 设置写入超时与最终读回 503 保留为历史
  失败；14ab 503 未复现，不能声称已证明根因。fork400 未取得准确响应；先核对
  派生前后状态，不重播写操作。详见
  [当前安装证据](../qa/pr28-installed-admission-followup-2026-09-24.md)、
  [先前接续](../qa/pr28-installed-continuation-2026-09-24.md)和
  [7b 历史证据](../qa/pr28-installed-closure-2026-09-24.md)。
- 当前四个出口：`SourceReady=pending`（18d 准确 CI 仍运行）；
  `PrivateCandidateReady=false`、`MergeReady=false`、`PublicMacReleaseReady=false`。
  同一最终候选仍需完整安装旅程、正常受保护 Provider、120-turn 实际 GUI、
  Core 升级/数据保全和安装版 PDF 预览等验收；不同会话与 SOURCE 的成功
  不能相加放行。
  Developer ID/公证/publication authority 只属于未来公开 macOS 分发；不索取
  原开发 Key/普通开发 Keychain 密码，不增加累计 US$5 预算。

## Historical handover entries

- 普通源码开发现使用持久 development Registry/Secret Store；API Key 已一次
  bootstrap，Core 及真实 GUI 的重启、新 profile 复用通过，不再索取同一 Key 或
  普通开发 Keychain 密码。专项 QA 用 `--isolated-keychain`；包装版拒绝共享开发
  authority。命令见 development-baseline，证据见本轮 admission 报告。
- `ff2981536` 四项 CodeQL 与 gate 通过；CI35809621946 的 analysis_compute
  输出文件测试出现 DuckDB 内部错误；后继已移除重复 count-page 查询，复用实际
  结果行数回执，本机95项 CLI 通过。仍须取得准确后继 Linux CI，不能继承旧绿。
- 长期执行顺序：[唯一执行方案](../delivery-execution-plan.md)。
- 唯一当前状态与证据路由：[产品矩阵](../product-completion.md)。
- 保留 SOURCE `0708620e21fc2bd706ba18a377a60824cdb78421`、
  DELIVERY `9c21c0ec911571ddba1c733ac4a828737654e02b` 及其后有效工作。
- 恢复先核对实际 HEAD/tree/dirty/祖先与适用 AGENTS；不能从远端旧 dde 重建。
- 本轮 Core 资格源码：`69e74fdb6`、`0ce2e92a0`；法律清单 owner 后继至
  `ac00f1602`。正式签名/安装/Provider/更新数据兼容验收仍未取得。
- 保留恢复 SOURCE `dc059977ae0ae3b4986698eda82c25bfe9a54275` 及 DELIVERY
  `e40693268b3bca6c8ae1d71253263dbe98efe1b1` 和全部有效后继。
- 保留制品 SOURCE `55838f047fbdccd2f8e073b157815d896c084d5d` 与 DELIVERY
  `d46d10b389285ce2b3e8a0a4d7ff68da7e75b60b` 及全部有效后继。
- 新源码 `7f6137a4d` 已正常 push：生产/QA credential 路径已审计，普通用户默认
  不需第二密码；新增初始化延后选中、Core 存储凭据可用性检查和 unavailable
  恢复。针对性检查通过；追加 checked integer 修复后的 `8e5142250` 已 push，
  其四种语言 CodeQL 与 gate 全通过。app-only 构建针对8e；旧 b2cf 不含凭据修复。
- 历史容器制品 SOURCE `b2cf1179e7b6e23bf565f4de94e8f1e4715a2c94`，tree
  `d6ca0b5dbd97a2f8eb8cd11aa1ea2beb336bd48c`；准确当前 HEAD/tree 在恢复时重读 Git。
- 本轮证据：[新制品与发行准入接续](../qa/pr28-core-release-admission-2026-09-22.md)。
  clean Core1.0.6 私有 app/DMG/ZIP 完成；全部8,639文件/链接与独立安装副本
  一致，严格签名及 production-tag 独立 Go 读包通过。实际1,174包实例审计中，
  原31+6行已有34通过、2阻断、1未分发；两个包级缺项是 Canvas 嵌入构建来源
  与 lazy-val 原始许可全文，原生/WASM/字体完整义务仍另列。
  新包仍为 development_clean_non_publishable，保留所有旧私有制品。
- 保留证据：[Core 与多快照闭环](../qa/pr28-release-closure-2026-09-22.md)。
  B1 完整原生链1776.55秒通过：A1/A2 public GET200及原始 final、历史/当前标识、
  缺失/篡改503整批拒绝、原字节复原、普通任务连续性、原件不变及新OS进程恢复。
  既有 accepted-final/compaction crash矩阵 production/race检查124.842秒通过。
- 下一动作：继续剩余许可与嵌入组件来源通知；Canvas 签前/签后可信转换已经
  通过真实夹具及新制品校验，不重复解决同一问题。Core profile、正式资格消费者
  和已通过 A1/A2 恢复不重做。
- 本机隔离 Mac 验收已获准，安装版已到正常 Provider onboarding。用户输入 Key
  后 Save 报 Registry503；原任务 Keychain 元数据确认为 locked，
  第二次正常解锁返回51、无信号、43ms，对应系统认证失败；用户已确认输入原密码，
  不重复归咎输入，不推断历史创建原因。独立合成新 Keychain 通过同一 helper 的
  创建/解锁，原配置与 Keychain 保留。新的 `core-credential-8e5142250` QA profile
  已通过正常受保护输入创建/解锁，GUI 保存、真实模型响应、重启恢复和锁定后
  保留配置/解锁重试均通过。后续模型交接与 Settings 错误提示修复通过107项
  针对性检查和 web typecheck，尚未进入安装候选。单独授权的官方
  DeepSeek `deepseek-flash` 探测 HTTP200、10tokens，证明 Key/模型有效，不能
  替代安装版保存、Agent/工具、重启历史及升级恢复验收。测试总上限 US$5。
- Developer ID/公证尚未配置；Apple 正常账户入口此前显示 Access Unavailable。
  production publication authority 尚未建立；现有 Keychain 安全存储探测的
  entitlement 缺项保留，不生成明文生产私钥，也不另建发布信任机制。
- 当前范围源码外发已通过原接口重审，正常 push 已到8e5142250；旧 NOT_CLEARED
  为历史状态。b2cf 的 Development CI 51任务全通过；8e 的该 CI 51任务全通过，CodeQL
  分析与 gate 已通过。精确 HEAD review 与其余适用门禁未关闭，PR28 仍 Draft/BLOCKED，真实 main
  仍 `ce96cf12581acfa0e19fae7c6aa9c709371012c8`。满足条件才 merge、验证 main
  并由合格来源发行；不换传输路径绕过拒绝。
- 不启用 Goal、自动续跑或并行 writer。私有制品不是正式发行。

## Historical entry anchors

The following links preserve prior entry URLs; their targets are dated evidence, not current next actions.

<a id="当前接续pr28-an-新反例与固定引擎验证--2026-09-21-pdt"></a>

- [当前接续：PR28 A–N 新反例与固定引擎验证 / 2026-09-21 PDT](2026-09-16-pr28-continuation.md#snapshot-d864-handover-12)

<a id="历史接续pr28-browser-与证据复核--2026-09-21-pdt"></a>

- [历史接续：PR28 Browser 与证据复核 / 2026-09-21 PDT](2026-09-16-pr28-continuation.md#snapshot-d864-handover-46)

<a id="历史接续pr28-独立审查落地--2026-09-21-pdt"></a>

- [历史接续：PR28 独立审查落地 / 2026-09-21 PDT](2026-09-16-pr28-continuation.md#snapshot-d864-handover-65)

<a id="历史接续pr28-双执行器--2026-09-21-pdt"></a>

- [历史接续：PR28 双执行器 / 2026-09-21 PDT](2026-09-16-pr28-continuation.md#snapshot-d864-handover-79)

<a id="历史接续pr28-am-本地源码推进--2026-09-20-pdt"></a>

- [历史接续：PR28 A—M 本地源码推进 / 2026-09-20 PDT](2026-09-16-pr28-continuation.md#snapshot-d864-handover-87)

<a id="历史接续完整性复核与缓存资源上限--2026-09-20-pdt"></a>

- [历史接续：完整性复核与缓存资源上限 / 2026-09-20 PDT](2026-09-16-pr28-continuation.md#snapshot-d864-handover-109)

<a id="上轮本机接收与开发路线授权"></a>

- [上轮本机接收与开发路线授权](2026-09-16-pr28-continuation.md#snapshot-d864-handover-133)

<a id="历史接续r07-合法后续刷新生命周期--2026-09-18-utc"></a>

- [历史接续：R07 合法后续刷新生命周期 / 2026-09-18 UTC](2026-09-16-pr28-continuation.md#snapshot-d864-handover-160)

<a id="历史接续2026-09-17-回执归属修复--2026-09-18-utc"></a>

- [历史接续：2026-09-17 回执归属修复 / 2026-09-18 UTC](2026-09-16-pr28-continuation.md#snapshot-d864-handover-197)

<a id="历史接续2026-09-17-实际发送链与案件授权组合"></a>

- [历史接续：2026-09-17 实际发送链与案件授权组合](2026-09-16-pr28-continuation.md#snapshot-d864-handover-233)

<a id="历史本地候选2026-09-17-canvas-引用持久审阅与消费者闭环"></a>

- [历史本地候选：2026-09-17 Canvas 引用、持久审阅与消费者闭环](2026-09-16-pr28-continuation.md#snapshot-d864-handover-268)

<a id="历史检查点2026-09-17-独立构建环境与回执授权边界"></a>

- [历史检查点：2026-09-17 独立构建环境与回执授权边界](2026-09-16-pr28-continuation.md#snapshot-d864-handover-307)

<a id="已落实的构建依赖"></a>

- [已落实的构建依赖](2026-09-16-pr28-continuation.md#snapshot-d864-handover-317)

<a id="本批真实产品修复与验证"></a>

- [本批真实产品修复与验证](2026-09-16-pr28-continuation.md#snapshot-d864-handover-335)

<a id="ci安全和-duckdb-的真实边界"></a>

- [CI、安全和 DuckDB 的真实边界](2026-09-16-pr28-continuation.md#snapshot-d864-handover-360)

<a id="下一依赖有效动作"></a>

- [下一依赖有效动作](2026-09-16-pr28-continuation.md#snapshot-d864-handover-380)

<a id="历史检查点2026-09-17-canvas-受控写入修复"></a>

- [历史检查点：2026-09-17 Canvas 受控写入修复](2026-09-16-pr28-continuation.md#snapshot-d864-handover-394)

<a id="最新保留快照"></a>

- [最新保留快照](2026-09-16-pr28-continuation.md#snapshot-d864-handover-433)

<a id="historical"></a>

- [Historical](2026-09-16-pr28-continuation.md#snapshot-d864-handover-460)

<a id="新线程恢复顺序"></a>

- [新线程恢复顺序](2026-09-16-pr28-continuation.md#snapshot-d864-handover-472)

<a id="交接最小合同"></a>

- [交接最小合同](2026-09-16-pr28-continuation.md#snapshot-d864-handover-497)

<a id="交接状态词"></a>

- [交接状态词](2026-09-16-pr28-continuation.md#snapshot-d864-handover-515)

<a id="单一事实链"></a>

- [单一事实链](2026-09-16-pr28-continuation.md#snapshot-d864-handover-531)
