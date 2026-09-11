## ADDED Requirements

### Requirement: Bounded Task Briefs

Agent-assisted absorption work SHALL begin with a bounded task brief that names
the objective, capability ids, allowed evidence files, required output schema,
scope exclusions, and forbidden assumptions.

#### Scenario: Subagent is dispatched
- **WHEN** a subagent or focused reviewer is used for absorption work
- **THEN** the controller provides a task brief instead of full controller
  conversation history

#### Scenario: Brief lacks required bounds
- **WHEN** a subagent task lacks capability ids, evidence paths, output schema,
  or forbidden content rules
- **THEN** the task remains blocked until the brief is completed

### Requirement: File-Based Reports

Subagents SHALL write structured report artifacts or evidence packets and
return only terse status plus paths to the controller.

#### Scenario: Subagent completes work
- **WHEN** a subagent finishes its assigned audit or research lane
- **THEN** it records findings in a report artifact and returns the report path
  plus a short status summary

#### Scenario: Raw transcript is offered
- **WHEN** a subagent output depends on raw transcript replay or broad pasted
  context
- **THEN** the handoff is rejected and must be rewritten as a bounded report
  artifact

### Requirement: Review Packages

Reviewer work SHALL use a review package containing the task brief, report
artifact, relevant changed-file or evidence paths, gate evidence, and progress
ledger when applicable.

#### Scenario: Reviewer evaluates a packet
- **WHEN** a reviewer checks a subagent report or prepared OpenSpec packet
- **THEN** the reviewer records pass, blocked, or needs-revision verdicts with
  exact path-based findings

#### Scenario: Reviewer lacks evidence
- **WHEN** a review package omits the brief, report artifact, gate evidence, or
  relevant paths
- **THEN** the reviewer verdict remains blocked

### Requirement: Progress Ledger

Long-running or multi-agent absorption work SHALL maintain a progress ledger
outside runtime prompts.

#### Scenario: Multi-agent round is active
- **WHEN** multiple task briefs or review packages are active
- **THEN** the progress ledger records brief ids, report paths, review status,
  unresolved findings, blockers, and final synthesis path

#### Scenario: Work resumes after interruption
- **WHEN** a future thread resumes the absorption round
- **THEN** it can recover state from the progress ledger without replaying raw
  subagent transcripts into prompt context

### Requirement: Main Thread Synthesis

The main thread SHALL own matrix updates, conflict resolution, final OpenSpec
packet creation, and admission decisions.

#### Scenario: Subagent makes a recommendation
- **WHEN** a subagent recommends absorb, adapt, defer, reject, or forbidden
- **THEN** the main thread verifies evidence and records the final decision

### Requirement: Superpowers Reference Boundary

Superpowers methodology SHALL remain reference-only until a future plugin/skill
admission gate explicitly accepts installation and prompt/tool-schema impact.

#### Scenario: Superpowers method is used
- **WHEN** bounded briefs, report files, review packages, or progress ledgers
  follow Superpowers-inspired practice
- **THEN** the OpenSpec artifact remains the source of truth and Superpowers is
  not installed or invoked as a default workflow

#### Scenario: Superpowers installation is requested
- **WHEN** a future task proposes installing Superpowers
- **THEN** the task requires plugin version, manifest, hook, trust,
  prompt-token, prefix-hash, and tool-schema admission evidence before it can
  proceed

### Requirement: OpenSpec Source Of Truth

OpenSpec SHALL remain the authoritative source for absorption scope,
requirements, design, tasks, admission verdicts, and final acceptance even when
Superpowers-inspired methods are used.

#### Scenario: Proposal or design is being prepared
- **WHEN** a proposal, design, spec, or task list is needed for absorption work
- **THEN** the artifact is created under OpenSpec and not under
  `docs/superpowers/*`

#### Scenario: Superpowers assists approved work
- **WHEN** Superpowers-inspired methods assist implementation or review after
  OpenSpec artifacts are accepted
- **THEN** the methods operate from the OpenSpec task brief, accepted specs,
  report artifacts, and review packages

#### Scenario: Superpowers output conflicts with OpenSpec
- **WHEN** a Superpowers-derived plan, review, or report conflicts with an
  accepted OpenSpec requirement or admission verdict
- **THEN** the OpenSpec artifact controls and the conflicting output is treated
  as a finding to review, not as an override
