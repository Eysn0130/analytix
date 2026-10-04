# DeepSeek Harness stage 1 dispositions

Status: Reference / candidate scope record. Base Analytix `09003e3f185bd0255cb227a9f96f1cc03b683fdd`; upstream pin `5badb15009ae1756c3afe0ae0cef1faafc290ccc`.

The 31 baseline functions and 36 refinements each have a disposition. Retained means the existing product owner stays authoritative; it does not claim all such flows were freshly exercised. Public-data gaps remain explicit in the [stage 2 plan](deepseek-harness-public-projection-plan.md).

| ID | Function | Stage 1 disposition | Actual owner/boundary |
| --- | --- | --- | --- |
| 1 | 用户消息 | existing product retained | message-timeline-bubbles / FloatingComposer |
| 2 | Assistant生命周期 | adapted renderer presentation | AssistantMarkdown / MessageTimeline |
| 3 | Streaming/final Markdown | adapted renderer presentation | DshAssistant / MarkdownText / finalization queue |
| 4 | 代码块 | adapted renderer presentation | CodeBlock / CodeSourceActions |
| 5 | 表格和数学 | adapted renderer presentation | parse / render / bounded katex |
| 6 | 链接/文件引用 | adapted renderer presentation | WorkspaceMarkdown / file-reference-validation |
| 7 | 图片/文件附件 | existing product retained | FloatingComposer attachments |
| 8 | Markdown媒体安全 | adapted renderer presentation | WorkspaceMarkdown / ImagePreviewLightbox / existing media |
| 9 | 工具显示注册 | public metadata retained; rich payload deferred | message-timeline-process + existing public tool projection |
| 10 | Shell chat card | public metadata retained; rich payload deferred | existing tool lifecycle; proposed shell_display_v1 |
| 11 | 交互式terminal | DSH engine/PTY excluded; existing product retained | existing Analytix terminal bridge |
| 12 | 工具专用cards | public metadata retained; rich payload deferred | existing public metadata cards; stage 2 closed payloads |
| 13 | Slash command行 | existing product retained | existing SlashCommandMenu |
| 14 | 问答与审批 | existing product retained | existing approval/user_input MessageBubble callbacks |
| 15 | 停止与发送 | existing product retained | FloatingComposer / Workbench |
| 16 | 终止错误与额度 | adapted renderer presentation | existing safe public errors / DisclosureRow |
| 17 | Retry/token cap | existing product retained | existing provider retry status and callbacks |
| 18 | Reasoning/公开过程 | adapted renderer presentation | DisclosureRow / existing public process projection |
| 19 | Process groups/display modes | adapted renderer presentation | ProcessSectionRow / ProcessStackRows / ProcessEntryRow |
| 20 | Whole Turn折叠 | existing product retained | existing turn/group collapse state |
| 21 | 工作时长 | adapted renderer presentation | MessageTimeline existing Go elapsed time |
| 22 | Usage/performance | existing product retained | existing usage projections/controls |
| 23 | Context/compaction | existing product retained | existing compaction projection/callbacks |
| 24 | Assistant actions | existing product retained | existing MessageBubble actions |
| 25 | Deliverables/diff | existing product retained | existing generated files / DiffView / receipts |
| 26 | Plans/workflow/subagent/feedback | existing product retained; DSH engine/private logs excluded | existing Go goal/plan/workflow/subagent consumers |
| 27 | 下载/导出 | existing product retained; DSH engine/private logs excluded | existing authorized code/file downloads |
| 28 | 滚动所有权 | existing product retained | existing virtualizer / measurement / use-timeline-scroll |
| 29 | 分页/导航 | existing product retained | existing Sidebar/thread pagination |
| 30 | 恢复/Session生命周期 | existing product retained | existing chat store/runtime rebind/recovery |
| 31 | 无障碍 | adapted renderer presentation | DisclosureRow / Tooltip / modal focus / existing IME |
| N01 | 语义草稿 编辑器 chip 与撤销边界 | retained | existing composer draft/undo owner |
| N02 | 异步插入和初始化 revision CAS | retained | existing async insertion revision guard |
| N03 | 统一 picker paste document drop intake | retained | existing picker/paste/drop |
| N04 | Desktop 引用与 Web 上传的不同路径 | retained | existing desktop attachment contract |
| N05 | 暂存上传 receipt retry remove abort 和提交门禁 | retained | existing attachment upload lifecycle |
| N06 | 附件 rail 的独立横向滚动 preview retry remove | adapted + retained | existing preview/download and mounted modal focus |
| N07 | Send Enter 反向 queue steer 和空草稿整队列手势 | retained | existing Go send/steer queue |
| N08 | 编辑器 keyboard IME focus 和 draft wheel 仲裁 | adapted + retained | DisclosureRow IME/target guards; composer keyboard owner |
| N09 | 普通发送并发恢复与命令冻结提交分离 | retained | existing send concurrency/recovery |
| N10 | 队列逐项编辑 删除 steer 和 sending echo | retained | existing Go queue projection |
| N11 | hero settling active 和 blocked removed 恢复姿态 | retained | existing hero/loading/blocked lifecycle |
| N12 | Slash candidate 与 command popup 的真实 occupants | retained | existing slash occupants |
| N13 | 引用与 skill candidate lexicon cache preview | retained | existing reference/skill serializer |
| N14 | 模型入口共享 directory 和 reconnect 选择状态 | retained | existing provider/model selection |
| N15 | 问答推荐默认 草稿 最小化与只读复查 | retained | existing public user_input question draft/gate |
| N16 | Produced declared changed 文件 provenance 与引用 | retained | existing artifact/source receipt authority |
| N17 | 交付 summary 与 diff 的不同 cache 恢复 | retained | existing diff cache/errors |
| N18 | Diff review 逐 tab 状态和 5000 行边界 | retained | existing DiffView cap/virtualizer/download |
| N19 | Native delivery actions 与 preview 独立能力 | retained | existing workspace native action capability |
| N20 | 共享 feedback form dialog command CAS 与 resync | retained | existing feedback command/resync |
| N21 | Plan 精确 invocation 恢复与无激活资源读取 | retained; DSH engine excluded | existing Go plan invocation |
| N22 | Workflow phase cycles focus 和 member 导航 | retained | existing workflow/member navigation |
| N23 | Subagent direct parent catalog timer 与嵌入会话 | retained | existing subagent/embedded-thread projections |
| N24 | Goal command-input node 与 GoalDock CAS | retained; DSH engine excluded | existing Go Goal input/dock |
| N25 | Schedule-created tail 和因果 freshness | retained; DSH scheduler excluded | existing schedule projection |
| N26 | 八份 global sheet body portal 与 theme font axis | adapted + retained | Analytix tokens/system fonts/body zoom + portal aliases |
| N27 | Modal menu focus composition top layer helper 闭包 | adapted + retained | existing modal focus; Tooltip/HoverCard/disclosure guard |
| N28 | cqw scrollbar composer gutter 多 scroll owner 几何 | retained | existing layout container/scroll owner/composer gutter |
| N29 | 保留 leaf 的额外 dependency build asset 许可闭包 | adapted | 35 pinned source leaves; 16 approved exact deps; existing MIT icons/OFL |
| N30 | 完整删除 hero running sidebar embedded 鲸鱼宠物并留计时 | pets/artwork excluded; timer retained | pet mounts/hooks/styles removed; Go elapsed timer retained |
| N31 | Mixed context some/every fallback 隐私边界 | private payload excluded; DTO deferred | existing safe metadata; stage 2 display publication plan |
| N32 | 文件拒绝后 Retry 保留完整 line request | adapted + retained | checked workspace paths; line/column + stale-validation guard |
| N33 | Synthetic display anchor 与 durable seq 分离 | retained | existing Core durable seq / renderer measurement anchors |
| N34 | 稳定 node group part identity 与 replacement store 重绑 | retained | existing stable block keys/rebind + incremental parser generations |
| N35 | 消息午夜 clock 与统计 dialog 的局部生命周期 | retained | existing time/usage owner; no DSH stats timer |
| N36 | 所有 fallback 诊断 inspect copy search 的 DTO sink 门禁 | private/unknown JSON excluded; DTO deferred | closed existing public projection and safe leaf sinks |
