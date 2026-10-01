# Office 只读预览：界面与动效研究取舍

Status: Reference。范围为 Documents、Spreadsheets、Presentations 的工作台预览。
基线为 `ab9a8f7b88a1e1ca60d6785ba78ba083e947fc5d`；实现与当前验收见 [本次 QA 记录](../qa/office-preview-polish-2026-09-14.md)。

## 设计判断

右侧工作面围绕当前文件组织，中央保留一条主会话。信息层级依次为文件身份、原始内容、导航与缩放；控件不能暗示可编辑或已保存。高级感来自内容与边界的稳定性、可读标签、统一热区、恰当状态与及时反馈。原始 Office 文件的字体、列宽、分页、幻灯片构图均不用于界面美化。

当前 GUI 基线显示：打开 DOCX 后标题立即出现，但原生引擎就绪前整个预览区域留白；文件栏缺少加载状态。底栏使用 24px 控件，缺少按下反馈与当前适配状态；点击视图控件后代码主动把焦点送回画布。折叠后焦点可留在隐藏按钮上。展开与恢复的坐标观察已存在，不应机械替换为 CSS transform。

## Taste Skills 固定版本与选择

来源为 [Leonxlnx/taste-skill](https://github.com/Leonxlnx/taste-skill/tree/ccbc15639c97057cbfcf32ecebc38ef716e4bb37)，本次远端 HEAD 复核仍为该提交。
[MIT LICENSE](https://github.com/Leonxlnx/taste-skill/blob/ccbc15639c97057cbfcf32ecebc38ef716e4bb37/LICENSE) 版权人为 Leonxlnx，2026；LICENSE blob 为 `48a2f6640b81ff8eca9ce7f6a96337692713ef5b`。

| Skill | 使用决定 | 本次落地/排除理由 |
| --- | --- | --- |
| [redesign-existing-projects](https://github.com/Leonxlnx/taste-skill/blob/ccbc15639c97057cbfcf32ecebc38ef716e4bb37/skills/redesign-skill/SKILL.md) | 加载使用 | 先扫描现有实现、诊断真实界面，再做小范围修复；应用状态完整性、数字等宽、语义色与光学对齐。 |
| [minimalist-ui](https://github.com/Leonxlnx/taste-skill/blob/ccbc15639c97057cbfcf32ecebc38ef716e4bb37/skills/minimalist-skill/SKILL.md) | 加载使用 | 采用低噪音表面、弱分隔线、克制的小面积类型色。现有系统字体与 Lucide 保留，避免另引图标/字体系统。 |
| [design-taste-frontend](https://github.com/Leonxlnx/taste-skill/blob/ccbc15639c97057cbfcf32ecebc38ef716e4bb37/skills/taste-skill/SKILL.md) | 阅读适用范围后不加载 | 当前 v2 明确以落地页、作品集为主，并排除数据表、多步产品 UI；不应用默认视觉/动效强度。 |
| [high-end-visual-design](https://github.com/Leonxlnx/taste-skill/blob/ccbc15639c97057cbfcf32ecebc38ef716e4bb37/skills/soft-skill/SKILL.md) | 阅读后不加载 | soft-skill 强制嵌套外框、较大留白、弹簧运动与 800ms 以上入场；不适合高频办公预览，也会替换现有系统字体/图标。 |

两份选用 Skill 在任务工具目录 `taste-ccbc156/skills/` 按原路径保存，根 LICENSE 同时保留；无脚本执行、无全量安装、无全局配置更改。精确 blob：redesign `c8304f0c9301385e3641b052d6b25b8af11cb6c1`，minimalist `44ead27ef04ffe79ade0c6df7fd696dbcf7b246b`。工具副本不进入产品运行包。

上游建议存在冲突：redesign 建议纹理、玻璃、强字重等视觉升级，而 minimalist 又排除较重阴影与渐变；两者均包含适合营销页面的默认值。本次以已接受的产品边界和现有设计系统选择适用方法，不把这些默认值视为产品要求。

## transitions.dev：版本、实现和许可

固定源码为 [598d3d6ad89dabb4bdf742fd2e887ca53914a888](https://github.com/Jakubantalik/transitions.dev/tree/598d3d6ad89dabb4bdf742fd2e887ca53914a888)。阅读范围包含主 Skill、独立 polish、refine 规则及公开 dropdown、panel、icon、skeleton、tooltip 实现。

[网站说明](https://transitions.dev/skill.html) 称 review/refine/polish 输出审查建议。固定源码的 [主 Skill](https://github.com/Jakubantalik/transitions.dev/blob/598d3d6ad89dabb4bdf742fd2e887ca53914a888/skills/transitions-dev/SKILL.md) 中 review/refine 仍为只读，但 [独立 polish](https://github.com/Jakubantalik/transitions.dev/blob/598d3d6ad89dabb4bdf742fd2e887ca53914a888/skills/transitions-polish/SKILL.md) 已描述确认后写入。故只读命令清单不能代表所有当前实现；本次已有持续实现授权，研究后由 Analytix 自主实现。

[固定 Terms](https://github.com/Jakubantalik/transitions.dev/blob/598d3d6ad89dabb4bdf742fd2e887ca53914a888/terms.html) 区分：Refine/CLI 工具适用 MIT；可访问的 transition 片段可以用于产品，但不能将库本身重新分发为竞争库、模板包或组件库。根目录未找到通用 LICENSE，不能宣称整个仓库均为 MIT。Skill 提示文本的独立再分发授权未明确，因此本次仅在线加载研究其公开内容，不安装/复制其 Skill、CSS 或 JS 到仓库；无 Pro 登录、购买或付费内容访问。

核心方法为按用途选择节奏，而非按数值接近程度套 token。[公开 Refine 实现](https://github.com/Jakubantalik/transitions.dev/blob/598d3d6ad89dabb4bdf742fd2e887ca53914a888/refine/server/motion-tokens.mjs) 同时存在 usage-aware 与 nearest-value fallback；后者不作为本次自动修改依据。

| 表面 | 采用方法 | 具体取舍 |
| --- | --- | --- |
| hover / press / focus | 快速视觉确认 | 沿用设计系统；底栏颜色反馈 150ms，按下立即响应，焦点环独立可见，禁用按钮无 hover 假反馈。 |
| 展开图标 | 同位置可逆切换 | 150ms 透明度变化，不加模糊、弹簧或大幅缩放；按钮可立即再次操作。 |
| 加载 → 就绪 | 稳定位置、真实状态驱动 | 原生内容未就绪时展示明确状态；内容就绪即显示，不等待骨架动画计时，不伪造进度。 |
| 面板展开/折叠 | 保持原生几何同步 | 不采用公开 panel 的 100px 位移、2px blur、400ms 入场配方。WebContentsView 独立合成，不受父 DOM transform/opacity 正确约束。 |
| 文件菜单与提示 | 不阻塞操作、焦点可理解 | 保持原生工具提示与明确 aria-label；不引入浮动穿越原生子视图的 DOM 提示层。 |
| reduced-motion | 去除非必要变化 | 新增颜色/图标过渡响应系统减少动态效果；最终将应用解析后的主题与减少动态偏好同步到原生外框，真实 GUI 验证见 QA。 |

## 测量与验收方法

在真实隔离 Electron 中先操作后观察。基线记录 1280×840 renderer、DPR 2；窄原生视图 524×804。展开恢复、折叠恢复的观察记录包含几何时间戳、DOM 变更批次、PerformanceObserver longtask 与 worker 回复类型计数；不采集文档正文、输入框内容或 Provider 数据。一次折叠约 217ms、一次恢复约 216ms，均约 27 次几何变化；该采样窗口内未出现 longtask，不能据此宣称稳定 FPS 或全量性能改善。原生 worker 在展开恢复测量中回报 2 次 local-view；这不是全 IPC 次数。

最终证据必须覆盖正常文件打开、三格式实际渲染、窄/宽布局、反复折叠恢复、缩放/分页/工作表切换、键盘焦点、禁写快捷键、原件哈希以及适用主题状态。单元测试验证状态与权限边界，截图验证视觉；两类证据不能替代。完整 mandatory CI、Office 分发许可、正式打包与 Product/Formal RC 均独立于本次精修。
