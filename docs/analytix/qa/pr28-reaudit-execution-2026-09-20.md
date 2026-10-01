# PR28 完整提示词施工与验收记录

Status: Historical, commit-scoped local execution evidence; partial delivery

Scope: 用户明确采用 2026-09-20 接续包的 A—M。执行环境为获准的本机
Codex Desktop Mac、canonical checkout、合成 workspace 和隔离临时状态。
本记录不是 Product acceptance、固定原生引擎保真、安装或正式 RC 证明。

## 输入与基线

- ZIP SHA-256：`b801f335d5e38a48b0c830cf1fe70cc31c96b4c4b5a4411a802bc0a07717075c`。
  解压前检查路径、重复成员、类型、加密和尺寸预算；12 个普通文件通过，包内
  SHA256SUMS 全部匹配。完整读取 01 A—M、02—05 及两份 references 原材料。
  文档因本轮用户明确采用才构成任务要求；引用会话和其余材料不独立授权。
- START branch：`codex/workbench-product-delivery-20260914`，clean。
- START HEAD：`2ef3f9deb22714b7c05583603bf1e394cce1f06e`。
- START tree：`a85bba7ce9d712ee7c51ff2d2520ccf0acc72380`。
- 2026-09-21 06:46 UTC 附近再次只读读取 PR28：main/base
  `ce96cf12581acfa0e19fae7c6aa9c709371012c8`，remote head
  `dde784437dc8563e84066629dd57f4a11fd9acc9`，OPEN、Draft、BLOCKED、无 merge receipt。
  [PR28](https://github.com/Eysn0130/analytix/pull/28)。旧 head 的 Development gate
  成功、CodeQL 失败，不适用于本地新候选。
- 本轮沿原分支保留 START 及其全部后续历史；未 reset、clean、amend、rebase、
  force push、直接 push main、创建 tag/Release 或发布。未操作 Goal/定时续跑/独立任务。
- 未读取真实用户数据目录、个人 Keychain、历史 token/profile 或真实案件。
  本轮 Provider 证据均来自合成 transport/loopback HTTP，不是付费真实 Provider 验收。

## 全范围矩阵

| 范围 | 本轮已有进展 | 仍缺少的证据或实现 |
| --- | --- | --- |
| P0 / D1、H2 | Main sender/frame/导航生存期检查；Go 当前主会话授权的受控读取；目录/文件身份与前后复核；权限身份 cache key；实际关闭/切换/停用/退出接线；Core 投影和持久化 canary | 不等于全部路径 CodeQL 告警关闭；不声称阻塞系统调用可立即取消或 RSS 已实测 |
| P0 / D2 | 取得旧 SHA 的 SARIF，逐 trace 分流；精确整数解析和 duration 换算边界修复并通过回归 | 新 SHA CodeQL 未运行，未关闭任何 review/alert |
| P0 / D3 | 在当前分析工具树运行准确 Rust 单例并定位 host 子进程停在 dyld_start | 原 DuckDB INTERNAL 断言未复现，根因仍未关闭 |
| P1 | 拒绝原文不变的 native proposal；部分 native mutation 失败后禁止导出；rich DOCX 三输入语义 oracle 已通过 24 项测试 | `setString` 的实际固定引擎样式/链接/字段保真，真实 Diff→接受→保存→重开与视觉旅程未验收 |
| P2 | PNG/JPEG 单自然像素矩形+备注由 Core CAS 持久化、重开、同会话讨论引用；迟到/ABA/丢ACK/权限与源版本复核 | 多区域、旋转与真实 GUI 未完成；网页 page/frame/document revision 引用仍缺 Host/Core currentness 链；像素隐私 projector 缺失时保持拒绝 |
| P3 | Excelize 2.11 真正 pivot package；PPTX 有限形状 geometry/solid fill 的 native typed command、Core 整包校验、Diff/CAS/restart/undo，保留已有 text parts | 固定引擎刷新/汇总、导入 pivot 修改、chart 数据修改、实际保存重开与视觉结果未完成 |
| P4 / H1 | 原临时 thread create/turn/poll/delete 已迁移主会话只读辅助执行；物理取消、版本复核、审计收尾，短长补全串行 | 实际桌面/真实 Provider 未验收 |
| P4 / H3 | Skills 消费后权限复核；rootless installed catalog 合法；发现不完整与已确认为空分开，保留可用 sibling | 安装版中的安装/升级/停用/卸载全旅程未验收 |
| P5 | 定向本地提交，原分支历史保留 | 外发安全拒绝未解除，未 push、无新 SHA CI、未 merge、未验证新 main、未安装验收/正式 RC |

既有三条 Write 清退、R07、typed XLSX、Canvas、8/4 cache 限制均保留，未重新开发。
PDF 未取得旋转缺陷复现，未按猜测修改。资金分析、Rust 专业计算、恢复和
TypeScript contracts/launcher 保留。

## 已提交的源码检查点

以下短 SHA 均应从仓库解析完整身份；它们是本地提交，不是远端同步凭证。

| 提交 | 行为与限制 |
| --- | --- |
| `cbe9ac7c8` | 原生 XLSX pivot 生成；一个 row、最多一个 column/两个 filter、一个 value；sum/count/average/min/max。`saveData=false`、`refreshOnLoad=true`，没有伪造 cached aggregate。display header 与 raw header 不一致时拒绝 |
| `03a19cf32` | 原生文本 no-op 在 mutation 前拒绝；typed cell 的显示文本相等不被误判为 no-op |
| `84b2e1da4` | 图片 refresh sequence + navigation revision，拒绝旧成功/旧错误/ABA，保留尚未 settle 的操作占位 |
| `2e53a7064` | installed skill callback 后重新核对 principal、generation、activation、正文 digest；不承诺回滚 callback 已有副作用 |
| `e61d1a9e5` | 允许 configuredRootCount=0 的合法 installed skills，去掉旧 root-count 错误推断 |
| `a3ac15a95` | native mutation 抛错后 quarantine，后续编辑/导出失败关闭 |
| `a236c2d17` | Write retrieval 绑定当前 Core workspace authority；Main 不再直接扫描/重开文件，PDF 从受控 bytes 解析 |
| `a3896c7eb` | 补齐图片生命周期测试的类型化 rename/delete receipt |
| `dbf463a79` | installed skill discovery 错误贯穿 Go→closed public response→Main→renderer，旧能力撤回，UI 提示发现不完整 |
| `34760b8b3` | 精确预算整数/指数解析与溢出检查；非法请求和 profile 在实际执行前拒绝，保留 absent/null/alias 语义 |
| `92bb8cc4b` | 主会话辅助补全闭合链；无 create/turn/poll/delete，无用户消息污染；short/long 共用 single flight |
| `7e473b611` | 第一方 synthetic DOCX fixture 和三输入语义 oracle；不是固定引擎证据 |
| `b2b51e1fd` | 有限 PPT geometry/fill 的 native command、原件和整包校验、属性 Diff/CAS/restart/undo；保留旧文字 parts 路线 |
| `5c2f1b089` | 图片单区域/备注的 Core CAS、scope 引用、版本/权限复核、IME 导航与资源收尾；大载荷 schema 回归 |

## 新增图片与 PPT 源码边界

图片仅接受静态 PNG 和普通 JPEG（非1 EXIF方向、APNG、尺寸/字节预算超限拒绝区域标注，保留原预览能力）。
最多12 MiB、8192单边、16,777,216像素；一个 object/thread 当前保存一个矩形与备注，
坐标为原图自然像素。Core 从真实 thread 解析 workspace，不信任 Renderer 自报 workspace。
模型只获 geometry、投影 noteParts 和 `imageObservation: unavailable`，不能据此声称看见或涂黑图片。
Main 校验 local-display 图片字节hash；引用实际发送前再次核对 Core scope；被动引用不发送、不覆盖草稿。
私有 annotation 和 Office recovery 共用原 CAS helper，native wrapper 保留格式准入。
输入法组合输入期间阻止任务切换；await 期间开始组合输入会永久撤销该次迟到选择。
选择取消时保留原 SSE；已经完成的 worktree/thread/fork/delete 副作用先登记或释放。
安全 public-projection revocation 仍立即清除历史，不被输入 hold 延迟。跨任务保留的脏备注
显示返回原任务保存的提示，不自动绑定新任务。上述为 action/DOM 合成证据，未冒称真实中文 IME。

PPT 属性修改只支持无旋转/翻转/继承样式、可由原件严格匹配的顶层 rectangle/ellipse/text shape。
完整有序页最多64个形状，EMU 到 1/100毫米必须精确整除360；名字或索引单独不构成身份。
worker 比较同一 native object、version/sequence、完整页面快照；Core 再比对原件关系表和整个候选包。
只允许选中 shape 的 geometry 或 direct solid RGB fill 单类变化，其余页面、文本、notes/theme/media 变化拒绝。
已有 shape text 的 parts 提案仍保留；模型只接收原投影 parts，几何结构不含未投影 shape name/text。
失败的部分 native mutation 禁止导出并恢复 Core 原件；没有用 PptxGenJS 重建导入文稿。
固定引擎 exporter 的正常序列化差异可能仍触发保守拒绝，需真实 no-op/edited 对照后判定，不能放宽保真 gate。

Browser 研究确认 DevBrowserPanel 当前只是 local/LAN preview，没有可信 guest/document capture owner，
也没有 Go→Host 的实时 currentness 端口。Main renderer owner 不能代替 guest frame authority；
同URL、文本hash或 MutationObserver 计数也不能证明完整视觉区域版本。此链未实施，未冒充完成。

## 验证与证据上限

所有依赖/测试/构建命令均在同一 shell 先执行
`source ./scripts/use-analytix-cache.sh`。临时路径由该 helper 管理。
本节只记录这次实际运行，旧附件的 260/28 tests 不并入新计数。
环境：macOS 26.5.2 (25F84)、arm64；Node v22.22.1、Go 1.26.4、Python 3.11.15。
产品 Office manifest 仍为 build `efaf0670b4d055f838a2849becb10f08aa06a257`、
`executionMode: source-experiment`、`publishable: false`。本轮未更改资源 seal、
声明其可分发或验证磁盘资产；source build 不能解除安装资源资格缺口。

| 检查 | 结果 |
| --- | --- |
| D2 subagent 精确预算回归 | pass；fractional、超过 2^53 的 json.Number、host int/duration 换算溢出负例曾 RED→GREEN；Go subagent 包 5.257s，vet pass。未运行新 SHA CodeQL |
| DOCX oracle Python unittest | pass，24；仅合成 fixture/比较器，不是 native run |
| P2 Core image / annotation / recovery | root server+filestore定向 pass，21.770s /14.886s；worker另覆盖 ports/app/HTTP 和原 objectediting 回归 |
| P3 Core typed + package + recovery | root officeediting/presentationcodec/filestore/toolcatalog/server 定向 pass（0.695/0.251/1.562/0.509/11.682s）；worker 六包 vet pass；真实合成 OOXML，不是原生引擎 |
| 最终图片/IME导航联调 | root5文件114 pass；worker7文件190 pass，两者有交集不相加。覆盖 await取消、原SSE保留、worktree释放、registry收尾与安全撤权 |
| 图片最大载荷 schema / IPC | 新增 16 MiB canonical Base64 边界先复现 RangeError（1 fail/30定向未选），移除重复正则分组后原IPC文件31 pass；字节/hash验证保留 |
| P2/P3 TS关键联调 | root6文件111 tests pass；含组件、图片draft/reference、IPC、typed worker和Main controller；另7文件192 tests pass（有交集，不相加） |
| 全量 TypeScript + runtime/Main/preload/renderer build | pass；后续 preload 修正后再次pass。Office sandbox preload只依赖Electron，无拆分本地require |
| 修改文件 ESLint | pass，0 errors；NativeOfficePanel有3项已有hook依赖warnings，未借机重构 |
| Go officegeneration/workbookcodec/toolcatalog/workspaceread/httpapi 定向组 | pass；实际 OOXML pivot 元件、source 类型/重叠/预算与 header 漂移检查。未调用固定原生引擎 |
| `go -C packages/runtime-go test ./internal/app/officeediting` | pass；包括 no-op proposal；原生 worker/controller Vitest 51 项及 typed worker 5 项 pass |
| 图片 store/file-actions 定向 Vitest | 38 pass；核心 15 个迟到/ABA 回归曾确认 RED，再修 GREEN |
| packagehost 及 runtime-skills 根目录数量回归 | pass；消费后 revocation/body/generation 测试、rootless catalog 4 项定向通过 |
| Go server `^TestWorkspaceRead` | pass，21.983s；真实 files→Core 投影→合成 Provider，public history/SSE/durable/restart canary。普通 turn 测试是独立投影证据，不再冒称当前补全执行路线 |
| D1 Main 7 文件 Vitest | 187 pass；changed lint pass |
| `go -C packages/runtime-go test ./internal/app/pluginpackagehost ./internal/app/toolcatalog -run 'Test.*(Skill\|Discovery)' -count=1` | pass，0.448s/0.475s |
| H3 runtimeapp development package materialization、sibling error 与 Canvas revoke fixture | pass；这是 source materialization，不是 GUI 安装 |
| H1 renderer 5 文件定向 Vitest | 64 pass；Main/preload/local-display 四文件 112 pass；Main service 24 pass |
| 跨 H1/H3 的 7 文件 Vitest | 319 pass、1 个无关 fake-Go 启动缓存用例超过 5s。停止并发 build 后只重跑该原用例：1 pass，2.12s，无断言/timeout 修改。不得写成第一次全绿 |
| `npm run typecheck` | pass，两套 tsconfig；此前发现两处宽泛 Mock 类型后改为精确函数签名 |
| `npm run build` | pass，含 runtime clean/tsc 和 Electron main/preload/renderer Vite build；不是正式包装或安装证明 |
| `go -C packages/runtime-go test ./internal/runtimeapp -run '^TestNewRuntimeServerHandlerAssemblesRunnableHandler$' -count=1` | pass，30.990s，隔离 fixture 的实际组合启动/health |

Rust 命令保持附件指定原断言：

```sh
source ./scripts/use-analytix-cache.sh && \
cargo test --locked --manifest-path tools/analysis_compute/Cargo.toml \
  --test stats_query_cli \
  rows_aggregate_cases::query_stats_rows_cli_returns_sorted_aggregate_rows -- --exact
```

编译完成（16.78s），子 CLI 在本机约五分钟没有进入应用逻辑，采样
891 次中的 890 次在 `dyld_start`；仅终止本任务所属子进程后测试 exit 101。
这是 host 启动阻断，不是 DuckDB INTERNAL 已修复或原 SQL 断言失败的复现。
analysis_compute 工具树在 main/旧候选/本轮读取时相同：
`8338a25502e261b8b219a5b75f0bc508772e402c`。没有为造绿改 SQL/fixture/期望值。

## 精准清退与上游采用

| 旧入口 | 替代 owner / 保留 |
| --- | --- |
| Main 的 workspace 路径字符串 scan/read | Go workspaceread + live primary thread authority；保留人工检索和启用时 inline retrieval |
| 临时补全 thread create/turn/poll/delete | 同一 Core 的主会话辅助执行；不删除主会话，不产生用户聊天 turn/item；保留 parser/debug 的仍有消费者部分 |
| Skills discovery catch→成功空数组 | closed discovery diagnostics + 可用能力集合；历史展示不授予旧权限 |

- Excelize：沿用既有锁定 2.11.0，不新增依赖；生成 pivot 与导入修改分开验收。
- OpenCode：本轮读取
  [run-coordinator.ts @ d870e22](https://github.com/anomalyco/opencode/blob/d870e22c70f27103016dcd479edcfebf86136d93/packages/core/src/session/run-coordinator.ts)
  和该 SHA 的 MIT LICENSE。仅参考同 key 单 owner、取消等待清理的状态不变量，
  独立实现于既有 Go Controller；没有复制源码或引入 Effect/第二 Core。
- LibreOffice pinned `efaf0670b4d055f838a2849becb10f08aa06a257` 的 XChartDocument / XChartDataArray IDL 只作为 chart 接口研究；没有证明固定 WASM 支持 chart 数据往返，故 chart 修改仍不开放。没有复制 IDL/上游源码进入产品。
- Codex、DeepSeek、Claude 文档的附件判断保留为研究输入，未将其全部声称为本轮
  重新逐文件复核。未复制受限 anthropics Office skill、脚本或素材；未添加依赖。

## 外部阻断与独立裁定

[原 PR28 安全阻断记录](https://github.com/Eysn0130/analytix/pull/28#issuecomment-5709839864)
记录第三次 `create_tree` 源码同步被自动审核拒绝：
“This tool call was blocked by OpenAI because we couldn't determine the safety status of the request.”
未取得对应 payload 的清晰范围或解除证据。普通 DNS 错误和此前成功对象写入不构成解除；
没有换成 `git push`/其他 API 重做。Chromium 管理拒绝和 SecurityAgent 拒绝分别保留，
不换浏览器/profile/凭据或关闭安全设置。一般本机授权已具备，不再次索要。

| 裁定 | 当前状态 |
| --- | --- |
| 已合并 main | 否 |
| main 已验证 | 否；旧 main/PR CI 不能代表本地候选 |
| 完整产品验收 | 否；矩阵所列源码及实际旅程缺口仍在 |
| 正式 RC | 否 |
| 可公开发布 | 否；技术条件未满足，且本轮没有公开发布授权 |

## 最终本地源码检查点

- Branch：`codex/workbench-product-delivery-20260914`。
- SOURCE END：`5c2f1b089533b70e3a7623396e9690d2d355aab4`。
- SOURCE END tree：`2e74792c9ab31f9dc9d588fb88e131e177eb6acc`。
- START 是 SOURCE END 的祖先；本轮新增14个源码/测试检查点，未改写此前历史。
- 最终 typecheck、runtime/Main/preload/renderer build 与 root 114项关键联调对应该源码；
  当时仅本页、handover与产品矩阵文档尚未提交。后续文档提交不改变已验证源码。
- 精确文档交付 HEAD/tree 从包含本页的提交读取；不把不可自引用的本页 commit SHA 伪造为 SOURCE END。
- 最终只读远端检查仍为旧 dde 的56条检查：55成功、CodeQL失败；Development gate成功。
  这不是本地 SOURCE END 的CI，也不是新main验收。

最终新增检查的实际命令（均 exit 0；前置 helper 与命令同 shell）：

```sh
source ./scripts/use-analytix-cache.sh && npm run typecheck && npm run build
source ./scripts/use-analytix-cache.sh && npx vitest run \
  src/renderer/src/write/image-thread-navigation.test.ts \
  src/renderer/src/components/write/WriteImagePreview.test.tsx \
  src/renderer/src/write/image-region-session.test.ts \
  src/renderer/src/store/chat-store-thread-actions.test.ts \
  src/renderer/src/store/chat-store-claw-actions.test.ts --pool=threads
source ./scripts/use-analytix-cache.sh && npx vitest run src/main/ipc/object-editing-ipc.test.ts
source ./scripts/use-analytix-cache.sh && npx eslint \
  packages/runtime/src/contracts/object-editing.ts \
  src/main/ipc/object-editing-ipc.ts src/main/ipc/object-editing-ipc.test.ts
```

另外检查构建出的 `out/preload/office.cjs`，唯一require为Electron；未提交构建物。
文档检查为 `git diff --check` 与三份更新文档的相对链接存在性，均通过。
本轮无GUI截图、真实Provider或固定引擎no-op/edited导出文件，不提供虚构回执。

源码已提交不等于完整A—M完成。后续依赖已明确的源代码缺口包括多图片标注/旋转和浏览器
可信捕获链；依赖具体准入/资源的缺口包括固定引擎、安装、远端同步与新SHA验收。
不同缺口没有合并成一个笼统环境阻断。


## START → 本地交付净变更统计

按 Git numstat 相对 START 分类；测试优先，Markdown 归文档，其余归源码/契约/配置。
文件只计一次，数值不是测试通过数量。

| 类别 | 文件数 | 新增行 | 删除行 |
| --- | ---: | ---: | ---: |
| 源码/契约/配置 | 108 | 6213 | 796 |
| 测试 | 64 | 6624 | 87 |
| 文档 | 5 | 435 | 2 |
