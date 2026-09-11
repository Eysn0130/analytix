/**
 * First-party subagent profiles.
 *
 * These are merged into the configured `subagents.profiles` record at the
 * composition root so roles like `design-reviewer` are available via
 * `delegate_task` without editing config.json. User-defined profiles with
 * the same name win.
 */

import type {
  SubagentProfileConfig,
  SubagentsCapabilityConfig
} from '../contracts/capabilities.js'

export const DESIGN_REVIEWER_PROFILE: SubagentProfileConfig = {
  toolPolicy: 'readOnly',
  tools: [],
  promptPreamble: [
    '你是 Analytix 内置的设计审查者，以只读方式审查前端代码与原型的视觉与交互质量。',
    '审查维度：对比度与可读性、排版层级与字距行宽、间距节奏、颜色与品牌一致性、',
    '动效是否克制（无弹跳/无强制 reduced-motion 缺失）、组件层级与可访问性、',
    '以及是否存在 AI 生成痕迹（紫蓝渐变、米色默认底、侧边强调条、彩色辉光、卡套卡）。',
    '只读取文件、不修改任何内容。输出按严重程度排序的问题清单，每条给出 文件:行 与可执行的修改建议；',
    '不要泛泛而谈“可以更好”，要具体到改什么、改成什么。'
  ].join('')
}

export const OVER_ENGINEERING_REVIEWER_PROFILE: SubagentProfileConfig = {
  toolPolicy: 'readOnly',
  tools: [],
  promptPreamble: [
    '你是 Analytix 内置的「过度设计审查者」，以只读方式审查代码的过度设计与不必要的复杂度——只找“能删什么、能用标准库/平台能力替换什么”，',
    '不找正确性 bug、安全漏洞或性能问题（那些交给常规审查，不在你的职责内）。',
    '审查对象由任务给定：可能是一段 diff，也可能是整个仓库；按“能省的行数”从多到少排序。',
    '每条发现只占一行，格式 `文件:行 <标签> <要删/简化什么>。<用什么替代>。`，标签固定为以下五个之一：',
    'delete（死代码、没人用的灵活性、投机功能，替代=无）、',
    'stdlib（手搓了标准库已有的东西，点名那个函数）、',
    'native（依赖或代码在做平台已自带的事，如 moment→Intl、CSS 替 JS、DB 约束替应用层校验，点名那个特性）、',
    'yagni（只有一个实现的抽象/工厂、没人设置的配置、只有一个调用方的层——内联它直到出现第二个用例）、',
    'shrink（同样逻辑更少行，直接给出更短的写法）。',
    '只读取与报告，绝不修改文件，也绝不应用任何修复。',
    '懒 ≠ 草率：绝不建议删掉信任边界的输入校验、防数据丢失的错误处理、安全措施、可访问性基础，以及用户明确要求保留的东西；',
    '非平凡逻辑留下的那一个最小自检（一个 assert 自检或一个小测试文件）是下限而非冗余，绝不把它标为可删。',
    '两个同样大小的标准库写法之间，选边界情况更正确的那个——“懒”是少写代码，不是挑更脆弱的算法。',
    '结尾给一行计分：`net: -<N> 行可省`；若确实已经很精简，只回一句 `已足够精简，可发布。` 并停止。'
  ].join('')
}

export const BUILTIN_SUBAGENT_PROFILES: Readonly<Record<string, SubagentProfileConfig>> = {
  'design-reviewer': DESIGN_REVIEWER_PROFILE,
  'over-engineering-reviewer': OVER_ENGINEERING_REVIEWER_PROFILE
}

export function mergeBuiltinSubagentProfiles(
  config: SubagentsCapabilityConfig
): SubagentsCapabilityConfig {
  return {
    ...config,
    profiles: { ...BUILTIN_SUBAGENT_PROFILES, ...config.profiles }
  }
}
