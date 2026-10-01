# PR28 双执行器接续：SARIF 与数值转换修复

Status: Historical, exact-candidate local execution evidence; partial delivery.

用户采用 2026-09-21 双执行器包；报告和提示词不构成源码补丁。本轮保留原分支、
单一 Go Core 和完整 P0—P5 范围。下一步为 REVIEW_ONLY，不交出源码 writer。

## 候选与接收

- canonical：`/Users/sun/Projects/analytix`；branch：`codex/workbench-product-delivery-20260914`。
- START：`2edb7d4631c6411e4132d45575abd46afe2d0da3`，tree `bc025bc378e8ad0150ebfe3a53f334354667375f`，clean。
- SOURCE END：`148d188e4f7df71e1cf28bc25bfff988365fa1ea`，tree `3690f03eeb390a075b001c1c79fd1ae66d470f88`。
- `2ef3f9deb`、`5c2f1b089`、`2edb7d463` 均保持真实祖先；没有 reset/amend/rebase。
- 输入 ZIP SHA256：`ddd74faddee871142b9277c1751c7d0c97f475419a9e7e9be801b52af2900cd8`。
  118629 bytes，19 个普通文件，展开 228053 bytes；路径、类型、大小、大小写重复、
  加密与压缩比检查通过，18 个 SHA256SUMS 条目全部匹配；3 组 md/txt 字节相同。
  00、01 A—I、02、03 C1—C4、04—08、references 已完整读取或按字节一致复用先前完整阅读。
- 最终文档提交身份从包含本文件的 Git commit 取得，不伪造自引用 SHA。

## C2 / P0：实际缺陷与修复

旧 SARIF analysis `1792355564` 绑定远端 `dde784437dc8563e84066629dd57f4a11fd9acc9`，
created_at `2026-09-17T11:02:43Z`，category `/language:go`，67 个结果：path 15、allocation 9、
integer conversion 29、hash 14。文件 5460542 bytes，SHA256
`17dd5a46ebeec35cac260cf4eda5ffe8681df1d9c2ed2bc5d74a1beb7fdb3389`。
67 条是完整分析结果，PR check 的 37 high 是另一口径；SARIF 没有 baselineState，不能臆造对应关系。

当前 macOS arm64 / Go 1.26.4 实测 `float64(2^63)` 转成 int/int64 后为 MaxInt64，
旧 roundtrip 检查仍相等。loop/model 还会接受截断小数或非有限浮点值。
[Go 数值转换规范](https://go.dev/ref/spec#Conversions_between_numeric_types)说明超出目标范围的
浮点转换结果依赖实现；不能用转换后的结果来替代转换前范围验证。

修复9个源码文件、8个测试文件，production +64/-19，tests +210/-0，无新依赖/上游代码复制：

| owner | 实际改变及正反例 |
| --- | --- |
| `contracts/records.go` NumericSeq | 转换前限定 host-int；拒绝2^63/NaN/Inf/小数，保留合法上界和原负零规则 |
| `app/loop/step_limits.go` numericAny | 无效配置/线程限制使用原 fallback，不把小数或溢出恢复成有效步数 |
| `app/model/pending_tool.go` numericAny | 无效 primary 不覆盖合法 fallback，整数与 json.Number 原兼容范围保留 |
| `app/control/interrupt.go` | 非负中断计数在转换前校验，不把2^63当最大合法计数 |
| `app/turn/case_compaction_public.go` | host-int版本/epoch与int64 replacedTokens分别按实际返回宽度校验 |
| `app/turn/general_interrupt_metadata.go` | 中断元数据拒绝越界输入；保留非负值规则 |
| `app/subagent/steer.go`、`domain/event/general_compaction.go`、`domain/steering/entry.go` | 相同转换不变量的必要传播：拒绝越界版本表示。此前还有固定版本等值检查，未声称已证实权限绕过 |

旧57/58由上一轮 `34760b8b3` 的 `numeric_budget.go` 修复，本次没有重做。
旧74/75已有 `ParseUint(..., IntSize-1)` 边界。其余 numeric guard 更新只构成修复候选，
不是新 SHA CodeQL 关闭证据。9条 allocation 尚无现实可构造溢出复现；14条 hash
需要结合完整数据流和用途独立审查，不能因使用SHA256就批量判误报或改成密码KDF。

alert145/146：旧 `filestore/object_editing.go:121/:170` 文件与 START 完全相同，
每条4条 trace，源为 HTTP object-editing body 或 plugin-package-host body。
当前链已有 runtime认证、typed local-display lane、principal复核、受保护路径、
Rel containment、symlink/hardlink限制以及 descriptor-based实际读取。
4项 filestore 回归通过；这不能代替对“手工本地对象打开的workspace”与“会话workspace授权”
契约的审查。没有证实可利用漏洞，也没有关闭这两条review。

## 本轮验证

所有测试/编译/hook命令同shell先 `source ./scripts/use-analytix-cache.sh`。
源文件和回归固定后运行，环境 Go1.26.4 / darwin arm64；不声称已运行32位测试。

- RED：5个新增顶层回归在修复前失败，实测2^63及无效模型数值接受。
- 中间定向运行与补充上界测试编辑重叠，loop编译报告 `could not import strconv`。
  该运行失败且不计验收；随后停止编辑，在稳定文件上完成以下验证，没有删测试或放宽断言。
- GREEN：`go test ./internal/contracts ./internal/app/control ./internal/app/loop ./internal/app/model ./internal/app/turn ./internal/app/subagent ./internal/domain/event ./internal/domain/steering ./internal/adapters/inbound/sse -count=1 -json`，exit0。
  9包、995个顶层测试；含子测试1783条PASS，0 fail/skip。不是1783个独立断言。
- 生产标签：同组 `-tags analytix_prod -run 'Test(NumericSeqBounds|NumericHostUpperBoundary|NumericSeqRejectsNegativeZero|StepLimitsRejectInvalidNumericInput|OptionalIntFieldRejectsInvalidNumericInput|InterruptCountRejectsOutOfRangeFloat|TurnMetadataRejectsOutOfRangeNumbers|VersionNumericBounds)$' -count=1 -json`，exit0，12个测试PASS。
  SSE包本次pattern无匹配，不把该空匹配计为生产SSE行为验收；普通包运行实际42项通过。
- path：`go test ./internal/adapters/outbound/filestore -run '^TestObjectEditing(RejectsUnsafePathsAndProtectedMetadata|CommitRejectsHardLinkCreatedAfterInspection|LastMomentExternalDriftIsNotOverwritten|FilesystemAliasesShareReceiptBinding)$' -count=1 -v`，exit0；4顶层+1子测试PASS，无skip。
- 独立子代理只读复核当前diff和兼容语义；主线程复核并执行上述验证。`git diff --check`通过。
- 先前TypeScript/build/native合成成绩仅按上一份[执行记录](pr28-reaudit-execution-2026-09-20.md)继承，未冒称本轮重跑。

## C1/C3：具体拒绝与远端

- create_tree：原PR评论记录第三次源码同步被自动审核阻断；缺原始payload及解除证据，
  `PAYLOAD_SCOPE_UNKNOWN / NOT_CLEARED`。未换API、普通push、ZIP或执行器重做源码外发。
- Chromium：d465批次Linux首次loopback导航报告 `ERR_BLOCKED_BY_ADMINISTRATOR`；
  原URL/profile/策略范围未取得。不是本机全部GUI被禁的证据，也不是已经解除。
- SecurityAgent：找到2026-09-14T22:16:45Z原调用与原错误，Computer Use访问
  `com.apple.SecurityAgent`被安全策略禁止。另一份旧隔离安装sample有
  `SecItemAdd → defaultKeychainUI → AuthorizationCopyRights`等待；不能推断凭据内容或用户拒绝。
  旧安装source为8365b16，不是本轮candidate。没有新的解除或当前候选正常启动回执。
- ChatGPT annotations入口400是不支持路由；没有把它写成GitHub403。本轮用实际SARIF。
- fresh 2026-09-21 08:01 UTC：PR28 open/draft/unmerged；head dde、base/main ce96。
  56条latest checks分页完整，55success/1CodeQL failure；Development gate成功，CodeQL37high。
  Development run35213288194/pull_request/attempt1；CodeQL run35213284499/dynamic/attempt1。
  Analyze任务success不等于CodeQL告警check成功。六条review、两条未解决，pageInfo无下一页。
  规则22946189仍strict Development gate/app15368、review resolved、普通approval0、无bypass。
  新本地SHA没有远端CI；actual checkout SHA未逐日志核实，UNKNOWN，不能用test-merge冒充main。

## C4 / 剩余范围与交接

[原完整产品矩阵](../product-completion.md)仍有效。固定引擎DOCX保真、真实IME、安装/Provider、
图片多区域/旋转、Browser可信currentness、导入pivot/chart、安装Skills等未因本次数值修复消失。
本轮不重复Canvas、R07、typed XLSX、Write清退或8/4限制。无新原生/GUI/Provider验收。
Rust dyld_start宿主阻断与原DuckDB INTERNAL分开；工具树仍8338a255，相同输入无新假设不重跑。

新诊断和原始公开SARIF已构成有价值REVIEW_ONLY交接点。交接包只含报告、合法取得的旧远端
SARIF及诊断元数据，不含未获准外发的本地源码、patch或Git bundle。Codex保留writer；
clean不作为WRITER_RELEASED。ChatGPT可据公开dde和真实trace继续独立审查，其对新源码结论
必须注明NO_SOURCE限制；以后确有源码写入需要再真实停写、核准候选和交接。

main merged：否；main verified：否；full product：否；Formal RC：否；public release：否。
未push/Ready/merge、未修改规则、未关闭review、未创建Goal/自动续跑/独立任务/tag/Release。
