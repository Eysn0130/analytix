# PR28 e15 后续：协议、发布计时与连接复用

Status: Historical，2026-09-26 PDT 的源码候选记录；不替代安装验收。
当前事实以 Git、可重跑测试和准确制品为准。本记录在最终提交前填写结果，
后继接续时应重新读取 HEAD/CI，不回退本文起点。

## 范围与身份

- canonical：`/Users/sun/Projects/analytix`；原分支
  `codex/workbench-product-delivery-20260914`；[PR28](https://github.com/Eysn0130/analytix/pull/28)。
- START：`ee91dcfed15256329143f98e98b6a6653a48f9af`。
  本轮产品 SOURCE：`c8cec28cd4de61ae5c8fa23fa9d09d115fd58bc8`；
  tree：`cf5b7a152e002f3d7c9ec9684b9cdc9c97e07fa5`。29 个源码/测试文件，+779/-62。
- 直接读取远端 main：`ce96cf12581acfa0e19fae7c6aa9c709371012c8`；local main：
  `60839b721b273119ab30158c65109dde4db52441`，后者落后 5 个提交，未改动 main。
- 保留用户 `development-runbook.md` 修改、两份未跟踪 QA 草稿；不纳入本轮提交。
- 本轮无真实 Provider 调用、凭据迁移、profile 复制、依赖升级或新运行时。
  测试使用 loopback HTTP/SSE 和合成 key。安装版真实 Provider 授权依赖独立保留。

## 已落地改动

1. DeepSeek Chat：`low → low`，保留 `medium/high → high`、`max → max`、
   `off` 显式关闭和 `auto` 不覆盖服务商默认。产品既有规范值仍拒绝 `xhigh` 等别名。
2. Messages：保留正数 Host `MaxOutputTokens`，不再把 8192 等合法配置截成 4096；
   0 仍表示未指定并使用既有 4096 默认。compat 在构造请求前拒绝负数。
   不虚构跨模型统一最大 cap；现有 Host 输出预算继续覆盖 reasoning、正文和工具材料。
3. 同一 runtime client 内最多保留 8 个当前 transport slot。按 Provider/family、
   endpoint origin、显式 proxy/authority 区分；代理或 origin 改变退役旧连接。
   旧在途请求继续按原 context 完成，最后释放关闭空闲连接。生产 shutdown 先 drain 再关闭池。
   每请求认证头和每次发送前 currentness 检查保留，key 不进入池键或诊断。
   显式 off 不退回环境代理，重定向仍拒绝。
4. 复用现有 pipeline/thread-trace owner，增加安全计时；不开放私有正文、reasoning、
   请求体、连接对象或地址。Main 可选记录器抛错不能阻断已验证交付或 ACK。
5. 实测输出预算热路径后，用增量估算替换每 chunk 全量重扫和累计字符串副本。
   保留每个输入前缀的原估算值，包括 ASCII 舍入、中文、跨块 UTF-8 和非法字节。
6. 修正 SSE trace 副本的 `make(map, len(event)+1)` 容量表达式为 `len(event)`，
   避免加一溢出；map 的键值与终态封印规则不变。这对应 START 的 CodeQL 告警，
   本地修改不等于新候选 CodeQL 已关闭。

TLS 使用既有 netclient 默认信任配置；本轮没有新增运行时可变 mTLS/信任代际契约。
一个 HTTPProviderClient 属于一个 Core runtime/profile，不跨 runtime 共享连接池。
8 是当前可复用 slot 上限；退役且仍在途的连接保留到结束，不声称全局同时连接数上限为 8。

## 计时口径与尚未覆盖的时间

| 观察 | 本轮实现及边界 |
| --- | --- |
| Provider 连接 | `post_send` 增加首个成功 DNS/connect/TLS 完成、获取连接、写完请求、首响应字节的单调时间偏移；连接 reused/was-idle/idle-ms 独立记录。偏移均相对 transport 调用，不相加为分段耗时。未观察到的字段缺省，不填零。 |
| Provider 正文/推理 | Host 在首个非空 reasoning/text callback 分别计算 firstReasoningLatencyMs/firstRawTextLatencyMs。Chat parser 原始收包时间保留单调部分；无 parser 时间的适配器使用 callback 时间，不能称精确 socket 到达。 |
| Core 发布 | `OnTextChunk` 的私有候选边界原样保留。完整候选及 publication/currentness 校验通过后才可公开；没有提前展示 raw draft。 |
| Main | 已验证 terminal batch 记录 `thread.terminal.verified` 及单调 verificationMs；成功发送后记录 `thread.terminal.ipc_sent`，通过 lastSeq 关联。拒绝的 batch 不产生成功验证记录。 |
| Renderer | ordinary general 和 accepted-final 原子 store 提交后记录 `thread.terminal.renderer_committed`；随后活动 thread 的 rAF 回调记录 `thread.terminal.next_frame`。后者是下一帧采样，既不是 React commit 证明，也不是像素已经绘制。 |
| 存储/隐私 | Core 的 closed pipeline 数值和布尔字段、Main/Renderer 既有 thread trace 分别使用原 owner。桌面持久 trace 默认关闭，开启仍哈希 threadId，并由逐事件字段白名单过滤。普通公开 pipeline 投影不会把这些内部连接细节自动传到 UI。 |

仍缺完整的 Core 首次可交付时间、最后必要内容→公开提交的 publication_gap、
实际像素首显、跨进程校准和统一操作账本。现有 Date.now 墙钟用于定位事件，
不能直接将不同进程的单调时钟相减。未完成 trace off/on 的完整安装版观察器开销比较。
本轮不把首响应字节/首 reasoning/下一帧采样称为 first_visible_content。

## 本地 before/after 微基准

环境：本机 macOS arm64，Apple M4 Pro，Go 1.26.4；使用仓库缓存 helper。
同一 `BenchmarkProviderOutputBudgetChunking`、64 KiB ASCII 输出、相同 Host cap，
`-benchtime=250ms -count=3 -benchmem`。before 是本轮估算器修改前的原算法，
after 是增量算法；二者均为源码单函数基准，没有云端或 UI。

| chunk 数 / 每块字节 | before 三次范围 | after 三次范围 | 每操作分配次数 before → after |
| --- | --- | --- | --- |
| 1 / 65536 | 67.945–68.889 μs | 33.012–33.171 μs | 1 → 0 |
| 64 / 1024 | 2.062–2.075 ms | 33.860–34.235 μs | 12 → 0 |
| 1024 / 64 | 32.310–32.668 ms | 38.691–39.042 μs | 19 → 0 |

原算法在每块后扫描全部累计输出；新算法只扫描新增字节，最多保留 3 字节未完成 UTF-8。
基准结果支持该检查的改善，不证明原安装版 25–30 秒等待由它造成，也不证明
Analytix 整体快于 DSH。没有稳定 p95/SLA、真实 KV 命中率或费用下降结论。

连接证据使用真实本地分块 SSE：代理 A 连续两次使用同一 socket，切到 B 后再回 A
建立新 socket，httptrace 的 reused 标记与之相符。32 个并发 acquire 复用同一配置；
无效代理、LRU 容量、origin 变化、关闭禁止复活及在途流不被退役取消均有回归。

## 验证结果

最终冻结源码（即上述 SOURCE）检查：

- **pass**：Provider 八个适配器包、app/model、app/loop、inbound/sse、兼容 provider，12 包。
- **pass**：Provider client `-race`，含并发复用、回收、当前性、重连、重定向和取消回归。
  此次 race 在后续估算器修改前完成；连接池/trace 的字节未再变化，最终正常包测试再次通过。
- **pass**：`analytix_prod` 下三项 runtimeapp 装配/关闭测试，冻结候选重跑 exit 0。
- **pass**：web/node typecheck；四份桌面测试 **120/120**。这是测试用例数，不是新增长线程回合数。
- **pass**：估算器每前缀等价回归、原预算拒绝、thinking+text 共同预算；before/after 基准各三次。
- **pass**：`git diff --check`。未运行本轮完整全仓 CI、安装版或 Windows 原生验收。

远端 START 的 Development gate 为成功、CodeQL 检查仍失败；它们均不验证本轮 SOURCE。
新候选 CI 需要普通 push 后独立读取，不能将本地检查称为 GitHub 门禁通过。

可重跑命令（均先在 canonical 根目录 source 缓存 helper）：

```sh
source ./scripts/use-analytix-cache.sh
cd packages/runtime-go
go test ./internal/adapters/outbound/provider/... ./internal/app/model ./internal/app/loop ./internal/adapters/inbound/sse ./internal/provider
go test -race ./internal/adapters/outbound/provider/client
go test -tags analytix_prod ./internal/runtimeapp -run 'Test(RuntimeOwnedResourceClosesAfterInnerDrainAndRetriesSafely|NewRuntimeServerHandlerAssemblesRunnableHandler|RuntimeAppRequiresTokenUnlessInsecureIsExplicit)$' -count=1
go test ./internal/app/loop -run '^$' -bench '^BenchmarkProviderOutputBudgetChunking$' -benchtime=250ms -count=3 -benchmem
```

桌面：`npm run typecheck`，以及 `npx vitest run` 的 runtime-sse-ipc、
thread-trace-service、chat-store-runtime、thread-performance-trace 四份既有测试。
覆盖有效封印/伪造拒绝、ACK、诊断抛错、两种终态 store 提交与独立下一帧采样。
中途有一次生产验证与源码编辑重叠，vet 看到未就绪的新符号；该执行无效，
保留为失败并在冻结源码后重跑，不把它的子测试 `ok` 当作整条命令通过。

## A01—A09 与七项工作处置

| 项 | 处置 |
| --- | --- |
| A01 | 已证实、保留发布合同；新增两端计时，完整 publication_gap 仍待补测。 |
| A02 | 既有 16 ms/rAF/ACK 复用；没有新增 batching 框架。 |
| A03 | 已修 low wire 编码，off/auto/high/max 回归。 |
| A04 | 已修正数 cap；native in-history system 不属于这次 cap 修复。 |
| A05 | 已修受控路径逐次新 transport；本地 socket/race/生命周期证据。 |
| A06 | 当前结构摘要和固定 DynamicStateLeaked=false 不能证明前缀/泄漏检查通过；未改契约，真实 usage 与本地结构估计分开。 |
| A07 | 既有 typed continuation 保留；没有新增长程语义无损/质量通过声明。 |
| A08 | 本轮测量链使用原始 time.Now 做 elapsed，序列化时才转换墙钟；没有声称全仓所有时间都重构。 |
| A09 | 请求/Registry 解析/服务端观察模型仍不能混同；没有因返回缺字段就断言实际模型。 |

七项工作：测量 **partial**；确定缺陷修复 **pass（本记录范围）**；DeepSeek 原生扩展
**未实现/待能力合同**；历史与流式 **保留原发布并完成实测预算热点修复**；压缩/工具/附件
**既有能力保留、专项语义矩阵未执行**；缓存统计 **未新增云端观察**；三层验收
**本地源码通过范围见上，真实对照与新安装后继未验**。未做工作不归咎于安装密钥缺口。

### 精简协议与字段矩阵

| 路线/字段 | 当前可证明状态 |
| --- | --- |
| DeepSeek Chat low/medium/high/max/off/auto | 按现有显式 reasoningProtocol 编码；本轮合成 wire 回归。 |
| Messages max_tokens | 保留 Host 正数预算；默认 4096；模型特定最大值不由本轮猜测。 |
| Anthropic thinking/output_config | 仅既有 anthropic-thinking 合同，不能直接宣称 DeepSeek 所有 Messages 模型等价。 |
| DSH in-history system / addition-only tool | 固定 DSH 客户端声明与公开 API 承诺不同；当前 Analytix 重建 top-level system，不启用未经确认的 beta 扩展。 |
| DeepSeek Messages anthropic-beta / cache_control | 公开兼容文档列为忽略；发送这些字段不是缓存或增量工具已生效证明。 |
| user_id / metadata.user_id | 本轮不新增外发；需现有隐私域和 egress 合同先明确，再加固定作用域合成向量。 |
| KV usage / 本地摘要 / connection reused | 三种不同观测；本轮只有本地连接回归，真实 usage 覆盖率、费用和 DSH 公平对照不可用。 |

参考：[DeepSeek thinking](https://api-docs.deepseek.com/guides/thinking_mode/)、
[Messages compatibility](https://api-docs.deepseek.com/guides/anthropic_api/)、
[DSH 477b4f4 adapter README](https://github.com/deepseek-ai/deepseek-harness/blob/477b4f420553e8a52c2fbccc464d7561b239c443/packages/llm/llm-deepseek/README.md)。
只借鉴协议事实；没有复制 DSH 代码、提示词、默认日志外发或引入其运行时。

## e15、门禁与下一动作

[e15 C01—C18 原记录](pr28-e15-credential-authority-2026-09-26.md) 原样保留。
e15 SOURCE 为 `96294a3142ffe08f63ee9d367ce1e36c2b760473`，DMG SHA-256
`4a71c539aaeaec0fc419e662a1bba03a2b5f8e55f390821a5395dbe66f456aaf`。
合成可见 Save、Go/Main 重启、120+后续续答、e13→e15 保全/PDF 是该制品历史证据。
本轮不重包或重复这组 soak，也不把它们转移给新源码。

- 新源码 SourceReady 仅能依据最终 focused 检查与准确后继 CI 分别报告；旧 57/57 不转移。
- `PrivateCandidateReady=false`、`MergeReady=false`：原安装版真实 Provider 正常保存/重启
  仍缺获准安装 QA 句柄；独立于本轮协议/性能工作。不复制持久开发 authority。
- `PublicMacReleaseReady=false`；e15 仍为 `development_clean_non_publishable`。
  Windows 原生凭据后端/启动器历史 CI 不等于 Windows 安装产品验收，本轮没有新 Windows 原生结果。
- 当前可独立继续：完成 Core commit 与实际 renderer paint 的校准观测，建立明确 endpoint/model
  native 能力分支的合成兼容测试，再在授权入口验证。用新增计时定位历史/发布热点后再改。
- 新源码尚未封装安装后继；准确后继冻结后需做受影响的可见短回合、取消/恢复及重启检查。
  不把完整性能路线、所有 P01—P12 或“全面超过 DSH”加入 PR28 新合并门槛。
- Notion 仅镜像本记录的有界摘要，仓库证据和实际 Git 状态始终优先。
