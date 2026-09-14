# Office 只读预览精修验收（2026-09-14）

Status: Historical。本记录仅证明本次源代码开发候选的受控 GUI 与定向验证。
基线 `ab9a8f7b88a1e1ca60d6785ba78ba083e947fc5d`，任务增量见同目录外的主机证据清单 `candidate-source.json`；交付 commit 可由本文件 Git 历史定位。研究来源、版本、许可证和取舍见 [研究记录](../upstreams/office-preview-ui-research-2026-09-14.md)。

## 结果与边界

已完成文件身份层级、48px 文件栏、30px 图标热区、加载/失败/重试、底栏按下/禁用/焦点/适配状态、展开图标反馈与外观同步。Renderer 通过原有受校验 IPC 将封闭的 `theme` 枚举与 `reducedMotion` 布尔值传到原生外框；不接受任意样式。Electron 注入样式采用限定外框变量覆盖，以处理实际 GUI 发现的作者样式层叠顺序。

原生文档像素、编辑与持久化协议、单一中央主会话、原生视图坐标观察保留。没有 Office 编辑/公式编辑/保存/AI 修改能力新增，没有重新启用编辑工具栏。未新增动效依赖、字体、图标库或大幅 transform。只读输入继续拦截文本、F2 与保存快捷键，同时允许正常 Cmd+Q 退出。

## 操作步骤与健康状态

| 步骤 | 实际操作与观察 | 状态 / 截图 |
| --- | --- | --- |
| 1 文件打开 | 内置插件 → 正常系统文件选择器 → 右侧预览；引擎启动前显示真实加载状态，未加人为延迟 | pass；`after-docx-opening.png` |
| 2 DOCX | 两页中文文档；62% 窄栏、121% 展开；长文件名省略但完整标题可访问 | pass；`final-docx-dark-longname.png`、`final-docx-wide-focus-mode.png` |
| 3 XLSX | 概览 → 交付清单工作表切换；100% 浏览、选区与展开；F2/文本/Cmd+S 后内容保持 | pass；`final-xlsx-narrow.png`、`final-xlsx-sheet2.png`、`final-xlsx-wide.png` |
| 4 PPTX | 1/3 → 2/3 → 3/3，首/末页按钮正确禁用；39% 窄栏、76% 展开；点击下一页后 Tab 到缩小按钮 | pass；`final-pptx-narrow.png`、`final-pptx-last-page.png`、`final-pptx-wide.png` |
| 5 面板与外观 | 展开/恢复、折叠/恢复；焦点返回中央输入；浅/深色外框随应用切换；真实“减少动态”设置下原生和文件栏过渡均为 0s | pass；`final-docx-light-reduced.png`、`theme-final.jsonl`、`reduced-motion-final.jsonl` |
| 6 失败恢复 | 任务自有无效 DOCX 显示失败与重试；仅把此测试副本替换成已知合成 DOCX 后按“重新打开预览”，恢复实际渲染 | pass；`final-error-state.png`、`final-retry-recovered.png` |
| 7 退出与原件 | 原生画布焦点下 Cmd+Q 正常退出，exit 0；三份原始样本 SHA-256 与验证前相同 | pass；`files-after.json` |

工作台“专注模式”与设置中的“减少动态”分别操作，后者由 `reduced-motion-final.jsonl` 证明过渡为 0s。基线与候选的几何/longtask 采样没有显示长任务，但样本操作集合不同，不宣称性能提升、稳定 FPS 或全 IPC 数下降。

错误文案沿用通用预览不可用提示，尚不区分损坏文件与插件故障。此处仅确认失败不留白且可重试；不宣称所有异常原因均有专用诊断。原生 Office 内容及工作表标签本身仍由引擎绘制，主题同步只覆盖应用拥有的预览外框。

## 验证

所有依赖测试、类型检查和构建均在同一 shell 先执行 `source ./scripts/use-analytix-cache.sh`。

- `npx vitest run src/renderer/src/office/NativeOfficePanel.test.tsx src/main/office/native-office-surface.test.ts src/main/office/native-office-controller.test.ts src/main/office/surface-controls.test.ts src/main/office/surface-integration.test.ts src/renderer/src/components/Workbench.route-surface.test.ts src/renderer/src/write/write-workspace-file-actions.test.ts --reporter=dot`：最终相关 7 文件 **77/77 pass**，包含几何、状态、外观、隔离、协议、键盘与工作台路由边界。
- TypeScript web 检查 pass；node 首次发现类型 import 别名不受 node 配置支持，改成相对路径后 `npx tsc --noEmit -p tsconfig.node.json` pass。最终类型改动不产生运行代码。
- 定向 ESLint：0 errors、1 `react-hooks/exhaustive-deps` warning，指向沿用的按 target 打开 effect；未弱化 lint 或隐藏警告。
- 基于提交归档加任务源文件的 main/preload/renderer 冻结构建 pass；最终 main 重新构建包含样式层叠修复。原生 source surface 从同一冻结源码加载。`git diff --check` pass。

首次冷启动的 Go 缓存构建在既有 120s 界限内超时。公开诊断的 58 bytes 与对应 `exit null` 错误哈希一致。使用相同规定缓存环境独立预构建后通过，后续正常启动成功；没有延长启动限制、替换运行时解析机制或修改 Keychain 策略。

## 主机证据与交付

主机证据根为 `/Volumes/AnalytixCache/development-v3/tmp/office-preview-polish-20260914`，包括截图、`comparison.html`、源码 SHA-256 清单、构建和验证日志。此目录是证据/临时构建，不是产品源码交付；源码修改在 canonical repository。

GUI 使用既有受控 Mock 开发 profile 和合成文件，通过原控制器正常解锁后启动候选；未调用真实 Provider，未复用真实验收 profile，未复制凭据。浅色与原“开启”动效偏好在结束时恢复。控制器与证据保留，未删除 profile 或清理原件。

Branch：`codex/workbench-product-delivery-20260914`。本次仅 focused local commit；PR URL：本增量无；CI status：当前增量 mandatory CI 未执行；Can merge：否。本次不证明正式打包、Office 分发许可、真实 Provider、Product/Formal RC 或公开发布条件。
