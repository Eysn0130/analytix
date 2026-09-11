/**
 * PM-skill frameworks wired into the SDD "新建需求" (new requirement) workflow.
 *
 * Each entry distills one framework from the `pm-skills` marketplace
 * (https://github.com/phuryn/pm-skills) into prompt-injectable guidance plus
 * the i18n keys for its assistant-panel button. The registry is pure data so it
 * can be unit-tested without React and shared by both the assistant panel
 * (renders the discover/structure/risk buttons) and the SDD prompt builders
 * (assistant / plan / verify inject a framework's `guidance` into the model
 * prompt).
 *
 * Stage → where it plugs into the workflow:
 * - discover  : assistant button — clarify the "why" before writing the spec
 * - structure : assistant button — shape the draft into `### R-n` blocks + `- [ ]`
 * - risk      : assistant button — stress-test the requirement before planning
 * - plan      : guidance injected into buildSddDraftToPlanPrompt (no button)
 * - verify    : guidance injected into buildSddVerifyPrompt (no button)
 */
export type SddWorkflowStage = 'discover' | 'structure' | 'risk' | 'plan' | 'verify'

export type PmSkillFramework = {
  /** Stable id; referenced by prompt builders and the pending-framework ref. */
  id: string
  stage: SddWorkflowStage
  /** English name used as the header when guidance is injected into a prompt. */
  label: string
  /** Source pm-skill (`plugin/skill`) for attribution, or null for generic actions. */
  sourceSkill: string | null
  /** i18n key (common namespace) for the button title; absent for non-button frameworks. */
  titleKey?: string
  /** i18n key for the button subtitle. */
  subtitleKey?: string
  /** i18n key for the localized request injected into the composer when clicked. */
  promptKey?: string
  /** English framework guidance injected into the model prompt; absent for generic actions. */
  guidance?: string
}

export const PM_SKILL_FRAMEWORKS: PmSkillFramework[] = [
  // ── Discover ──────────────────────────────────────────────────────────────
  {
    id: 'clarify',
    stage: 'discover',
    label: 'Clarify open questions',
    sourceSkill: null,
    titleKey: 'sddAssistantClarify',
    subtitleKey: 'sddAssistantClarifySub',
    promptKey: 'sddAssistantClarifyPrompt'
  },
  {
    id: 'research',
    stage: 'discover',
    label: 'Implementation research',
    sourceSkill: null,
    titleKey: 'sddAssistantResearch',
    subtitleKey: 'sddAssistantResearchSub',
    promptKey: 'sddAssistantResearchPrompt'
  },
  {
    id: 'brainstorm-ideas',
    stage: 'discover',
    label: 'Multi-perspective ideation',
    sourceSkill: 'pm-product-discovery/brainstorm-ideas-existing',
    titleKey: 'sddFwIdeasTitle',
    subtitleKey: 'sddFwIdeasSub',
    promptKey: 'sddFwIdeasPrompt',
    guidance:
      "Multi-perspective ideation (Teresa Torres' product trio). Generate ~5 options from each lens — " +
      'Product Manager (business value, strategic fit), Designer (UX, usability, delight), and Engineer ' +
      '(technical leverage, data, scalability) — then pick the top 5 across all lenses by strategic ' +
      'alignment, expected impact, feasibility, and differentiation. For each pick give a one-line ' +
      'description, why it was chosen, and the single riskiest assumption to validate. Explore options; ' +
      'do not commit to the first idea.'
  },
  {
    id: 'opportunity-tree',
    stage: 'discover',
    label: 'Opportunity Solution Tree',
    sourceSkill: 'pm-product-discovery/opportunity-solution-tree',
    titleKey: 'sddFwTreeTitle',
    subtitleKey: 'sddFwTreeSub',
    promptKey: 'sddFwTreePrompt',
    guidance:
      'Opportunity Solution Tree (Teresa Torres). Anchor on ONE measurable desired outcome at the top. ' +
      'From it, map 3-7 customer OPPORTUNITIES framed as problems/needs ("I struggle to…", "I wish I ' +
      'could…"), never as features; rank them with Opportunity Score = Importance × (1 − Satisfaction). ' +
      'For the top 2-3 opportunities generate 3+ candidate solutions each; for the most promising, propose ' +
      'a fast experiment (hypothesis, method, metric, success threshold). Prioritize the problem space — ' +
      'never let the solution come first.'
  },
  {
    id: 'triage-requests',
    stage: 'discover',
    label: 'Feature-request triage',
    sourceSkill: 'pm-product-discovery/analyze-feature-requests',
    titleKey: 'sddFwTriageTitle',
    subtitleKey: 'sddFwTriageSub',
    promptKey: 'sddFwTriagePrompt',
    guidance:
      'Feature-request triage. Group the requests into named themes and assess each theme’s strategic ' +
      'alignment with the stated goal. Pick the top 3 by Impact (customer value and number of users ' +
      'affected), Effort, ' +
      'Risk, and alignment. For each pick give the rationale, alternative solutions worth considering, the ' +
      'riskiest assumption, and the cheapest way to test it. Prioritize the underlying problem, not the ' +
      'literal feature that was asked for.'
  },

  // ── Structure ─────────────────────────────────────────────────────────────
  {
    id: 'structure',
    stage: 'structure',
    label: "User stories (3 C's + INVEST)",
    sourceSkill: 'pm-execution/user-stories',
    titleKey: 'sddAssistantStructure',
    subtitleKey: 'sddAssistantStructureSub',
    promptKey: 'sddAssistantStructurePrompt',
    guidance:
      "Shape each requirement as a user story (the 3 C's — Card, Conversation, Confirmation) honoring " +
      'INVEST (Independent, Negotiable, Valuable, Estimable, Small, Testable). Map onto the SDD block ' +
      'format: the heading "### R-n: <title>" is the Card; the description reads "As a <role>, I want ' +
      '<action>, so that <benefit>"; the "- [ ] " checklist is the Confirmation — 4-6 concrete, observable, ' +
      'testable acceptance criteria each, including edge cases. Keep blocks independent and sprint-sized.'
  },
  {
    id: 'wwa',
    stage: 'structure',
    label: 'Why-What-Acceptance backlog items',
    sourceSkill: 'pm-execution/wwas',
    titleKey: 'sddFwWwaTitle',
    subtitleKey: 'sddFwWwaSub',
    promptKey: 'sddFwWwaPrompt',
    guidance:
      'Why-What-Acceptance backlog format. Map onto the SDD block: "### R-n: <title>"; the description ' +
      'states the Why (1-2 sentences linking to user/business value) then the What (1-2 sentences — a ' +
      'reminder of intent, not a detailed spec); the "- [ ] " checklist is the Acceptance Criteria as ' +
      'high-level, observable, testable outcomes. Each block must be independent, valuable, and small ' +
      'enough to estimate. Preserve the user’s intent; do not invent requirements.'
  },
  {
    id: 'job-stories',
    stage: 'structure',
    label: 'Job stories (JTBD)',
    sourceSkill: 'pm-execution/job-stories',
    titleKey: 'sddFwJobsTitle',
    subtitleKey: 'sddFwJobsSub',
    promptKey: 'sddFwJobsPrompt',
    guidance:
      'Job-story (Jobs-to-be-Done) phrasing. Map onto the SDD block: "### R-n: <title>"; the description ' +
      'follows "When <situation>, I want to <motivation>, so I can <outcome>" — focus on the job and ' +
      'context, not a user role; the "- [ ] " checklist holds outcome-focused acceptance criteria that ' +
      'confirm the situation is recognized and the outcome achieved, including edge cases. Keep every ' +
      'criterion observable and measurable.'
  },
  {
    id: 'prd',
    stage: 'structure',
    label: 'PRD-grade completeness',
    sourceSkill: 'pm-execution/create-prd',
    titleKey: 'sddFwPrdTitle',
    subtitleKey: 'sddFwPrdSub',
    promptKey: 'sddFwPrdPrompt',
    guidance:
      'PRD-grade completeness (8-section template). Before the requirement blocks, capture a concise ' +
      'preamble: Background (why now), Objective (with SMART/OKR-style measurable key results), Market ' +
      'Segment(s) defined by jobs not demographics, Value Proposition(s) (jobs, gains, pains), and ' +
      'Assumptions (clearly flagged as unproven). Then express each concrete feature as an SDD requirement ' +
      'block "### R-n: <title>" + a "- [ ] " acceptance checklist. Be specific and data-driven; write in ' +
      'plain language.'
  },
  {
    id: 'polish',
    stage: 'structure',
    label: 'Copyedit (grammar / logic / flow)',
    sourceSkill: 'pm-toolkit/grammar-check',
    titleKey: 'sddFwPolishTitle',
    subtitleKey: 'sddFwPolishSub',
    promptKey: 'sddFwPolishPrompt',
    guidance:
      'Copyedit pass (grammar / logic / flow). Identify issues without rewriting wholesale: for each, give ' +
      'the Location, the Error (quote it), a targeted Fix, and a one-line Why. Cover grammar (spelling, ' +
      'agreement, tense, vague pronouns), logic (unsupported / contradictory / vague claims), and flow ' +
      '(weak transitions, choppy or passive sentences, redundancy). End with the 3-5 highest-impact fixes. ' +
      "Preserve the author's voice and intent; leave intentional style choices alone."
  },

  // ── Risk ──────────────────────────────────────────────────────────────────
  {
    id: 'assumptions',
    stage: 'risk',
    label: 'Risky-assumption mapping',
    sourceSkill: 'pm-product-discovery/identify-assumptions-existing',
    titleKey: 'sddFwAssumeTitle',
    subtitleKey: 'sddFwAssumeSub',
    promptKey: 'sddFwAssumePrompt',
    guidance:
      "Devil's-advocate assumption mapping. From three lenses — PM (viability, market fit, strategy), " +
      'Designer (usability, adoption), Engineer (feasibility, performance, integration) — surface the ' +
      'risky assumptions across the four risk areas: Value (does it solve a real problem?), Usability (can ' +
      'users figure it out?), Viability (can sales/finance/legal/marketing support it?), and Feasibility ' +
      '(can it be built on existing tech?). For each assumption: what could go wrong, your confidence ' +
      '(High/Medium/Low), and the cheapest way to test it. Be constructive — strengthen the requirement, ' +
      'do not kill it.'
  },
  {
    id: 'prioritize-assumptions',
    stage: 'risk',
    label: 'Assumption prioritization (Impact × Risk)',
    sourceSkill: 'pm-product-discovery/prioritize-assumptions',
    titleKey: 'sddFwAssumeRankTitle',
    subtitleKey: 'sddFwAssumeRankSub',
    promptKey: 'sddFwAssumeRankPrompt',
    guidance:
      'Assumption prioritization (Impact × Risk). For each assumption score Impact (value created × ' +
      'customers affected) and Risk = (1 − Confidence) × Effort, then place it on the matrix: ' +
      'High-Impact/Low-Risk → just build it; High-Impact/High-Risk → design an experiment to test first; ' +
      'Low-Impact/High-Risk → drop it; Low-Impact/Low-Risk → defer. For every assumption that needs ' +
      'testing, propose one experiment that maximizes learning per unit of effort, measures behavior (not ' +
      'opinion), and has a clear metric and success threshold. Present as a ranked matrix or table.'
  },
  {
    id: 'pre-mortem',
    stage: 'risk',
    label: 'Pre-mortem',
    sourceSkill: 'pm-execution/pre-mortem',
    titleKey: 'sddFwPremortemTitle',
    subtitleKey: 'sddFwPremortemSub',
    promptKey: 'sddFwPremortemPrompt',
    guidance:
      'Pre-mortem. Imagine this shipped and then FAILED, and work backward. Sort risks into Tigers (real ' +
      'problems you personally see — act on them), Paper Tigers (others worry, you do not — document to ' +
      'align stakeholders), and Elephants (unspoken, you are unsure — investigate before committing). ' +
      'Classify each Tiger as Launch-Blocking (fix before shipping), Fast-Follow (within ~30 days), or ' +
      'Track (monitor). For every Launch-Blocking Tiger give a concrete mitigation, an owner, and a ' +
      'decision date. Default to Tiger when unsure.'
  },
  {
    id: 'experiments',
    stage: 'risk',
    label: 'Validation experiments',
    sourceSkill: 'pm-product-discovery/brainstorm-experiments-existing',
    titleKey: 'sddFwExptTitle',
    subtitleKey: 'sddFwExptSub',
    promptKey: 'sddFwExptPrompt',
    guidance:
      'Lightweight validation design. For each risky assumption, pick the cheapest credible method — ' +
      'first-click / task test on a prototype, fake-door or feature stub, technical spike, guarded A/B, ' +
      'Wizard-of-Oz, or behavioral survey. Specify per experiment: the Assumption, the Experiment (exactly ' +
      'what you will do), the Metric, and the Success threshold. Measure real behavior over opinions, test ' +
      'responsibly (mitigate risk for any production test), and maximize validated learning per unit of ' +
      'effort.'
  },

  // ── Plan (injected into buildSddDraftToPlanPrompt; no button) ───────────────
  {
    id: 'prioritization-frameworks',
    stage: 'plan',
    label: 'Sequencing & prioritization',
    sourceSkill: 'pm-execution/prioritization-frameworks',
    guidance:
      'Sequencing & prioritization. Order the work by value-per-effort, not by what is easiest to build ' +
      'first. Use Opportunity Score = Importance × (1 − Satisfaction) to rank the underlying problems, then ' +
      'ICE (Impact × Confidence × Ease) or RICE = (Reach × Impact × Confidence) / Effort to order ' +
      'initiatives; reserve MoSCoW (Must / Should / Could / Won’t) for scoping the first release. Put the ' +
      'highest-impact, highest-confidence, lowest-effort steps first and call out what is explicitly deferred.'
  },

  // ── Verify (injected into buildSddVerifyPrompt; no button) ──────────────────
  {
    id: 'intended-vs-implemented',
    stage: 'verify',
    label: 'Intended-vs-implemented gap audit',
    sourceSkill: 'pm-ai-shipping/intended-vs-implemented',
    guidance:
      'Audit the gap between documented intent and the implementation — not just the code in a vacuum. ' +
      'Treat each requirement block and its acceptance criteria as the documented intent (the source of ' +
      'truth to verify). For every criterion, gather implementation evidence — a cited file:line where the ' +
      'behavior is, or provably is NOT, enforced; "probably handled elsewhere" is not evidence. Only mark ' +
      '"- [ ]" as "- [x]" when you can cite BOTH sides: the intent and the concrete code/behavior that ' +
      'satisfies it. A claim you cannot cite on both sides is a follow-up question, not a verified pass. ' +
      'Flag documented-but-unenforced criteria explicitly.'
  },
  {
    id: 'test-scenarios',
    stage: 'verify',
    label: 'Test scenarios per criterion',
    sourceSkill: 'pm-execution/test-scenarios',
    guidance:
      'Derive a concrete test scenario per acceptance criterion before judging it: Objective (what ' +
      'behavior it validates), Starting conditions (state / data / permissions), Role, ordered Steps ' +
      '(action → observable result), Expected outcomes, plus edge and invalid-input cases. Prefer running ' +
      'the test or inspecting the exact code path over assuming, and cite what you ran or read. Use these ' +
      'scenarios as the evidence behind each "- [ ]" → "- [x]" decision.'
  }
]

/** Assistant-panel button groups, in display order. Verify/plan frameworks are excluded (no buttons). */
export const SDD_ASSISTANT_FRAMEWORK_GROUPS: Array<{
  stage: Extract<SddWorkflowStage, 'discover' | 'structure' | 'risk'>
  titleKey: string
}> = [
  { stage: 'discover', titleKey: 'sddFwGroupDiscover' },
  { stage: 'structure', titleKey: 'sddFwGroupStructure' },
  { stage: 'risk', titleKey: 'sddFwGroupRisk' }
]

const FRAMEWORKS_BY_ID = new Map(PM_SKILL_FRAMEWORKS.map((framework) => [framework.id, framework]))

export function frameworkById(id: string): PmSkillFramework | undefined {
  return FRAMEWORKS_BY_ID.get(id)
}

/** Button frameworks for a given assistant-panel group, in registry order. */
export function frameworksForStage(stage: SddWorkflowStage): PmSkillFramework[] {
  return PM_SKILL_FRAMEWORKS.filter((framework) => framework.stage === stage)
}

/**
 * Build a single prompt-injectable block from the named frameworks' guidance.
 * Unknown ids and frameworks without guidance are skipped; returns '' when none
 * resolve, so callers can spread it conditionally.
 */
export function composeFrameworkGuidance(ids: string[]): string {
  const blocks = ids
    .map((id) => frameworkById(id))
    .filter((framework): framework is PmSkillFramework => Boolean(framework?.guidance))
    .map((framework) => {
      const attribution = framework.sourceSkill
        ? ` (adapted from the ${framework.sourceSkill} PM skill)`
        : ''
      return `## ${framework.label}${attribution}\n${framework.guidance!.trim()}`
    })
  if (blocks.length === 0) return ''
  return ['Apply the following product-management framework(s):', '', blocks.join('\n\n')].join('\n')
}
