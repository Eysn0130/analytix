## MODIFIED Requirements

### Requirement: Bounded Task Briefs

Agent-assisted absorption work SHALL receive enough bounded context to establish the objective, authorized read/write territory, accepted requirement or capability, evidence needed, and relevant exclusions. The brief SHALL scale to the task and SHALL NOT require empty template fields or full conversation history.

#### Scenario: Subagent is dispatched
- **WHEN** a subagent or focused reviewer is used for absorption work
- **THEN** the controller provides a self-contained bounded brief with relevant source pointers instead of full controller history

#### Scenario: Brief lacks required bounds
- **WHEN** missing context makes the objective, ownership or required evidence materially ambiguous
- **THEN** the affected lane remains blocked until the parent resolves the ambiguity
- **AND** missing field names alone do not block a semantically complete brief

### Requirement: File-Based Reports

Absorption results SHALL be attributable, reviewable and recoverable at the level required by their use. Durable admission decisions, multi-session handoffs and evidence too large for a bounded return SHALL have a file-based report or evidence packet. A short read-only finding SHALL be allowed as a bounded path-based return when the controller records any durable synthesis.

#### Scenario: Subagent completes work
- **WHEN** a subagent finishes an audit or research lane
- **THEN** it returns observations, source paths, evidence limits and status in the brief's agreed form
- **AND** a required durable artifact has an explicitly authorized writer; a read-only role does not silently acquire write permission

#### Scenario: Raw transcript is offered
- **WHEN** a subagent output depends on raw transcript replay or broad pasted context
- **THEN** the handoff is rejected and rewritten as bounded evidence, with a report file when persistence is required

### Requirement: Review Packages

Review work SHALL have the accepted target, exact candidate or evidence identity, relevant paths and the checks necessary for the claimed result. A separate report or ledger SHALL be included when it contains required evidence; a redundant wrapper SHALL NOT be required for a self-contained review.

#### Scenario: Reviewer evaluates a packet
- **WHEN** a reviewer checks a candidate, subagent finding or prepared OpenSpec packet
- **THEN** it records pass, blocked or needs-revision findings with exact paths and evidence limits

#### Scenario: Reviewer lacks evidence
- **WHEN** missing evidence prevents assessment of a required claim
- **THEN** that claim remains blocked while independent claims may still be reviewed
- **AND** absence of an unnecessary report wrapper does not itself block review

### Requirement: Progress Ledger

Long-running or multi-agent absorption work SHALL retain a compact recovery record outside runtime prompts. An existing OpenSpec task record or referenced evidence index SHALL be reused when it already carries the necessary state rather than creating a second backlog.

#### Scenario: Multi-agent round is active
- **WHEN** multiple task briefs or review packages are active
- **THEN** the recovery record identifies active ownership, candidate/evidence paths, review state, unresolved required findings, blockers and final synthesis

#### Scenario: Work resumes after interruption
- **WHEN** a future task resumes the absorption round
- **THEN** it can reconstruct the relevant state from current repository facts and that record without replaying raw transcripts or treating historical evidence as current

## ADDED Requirements

### Requirement: Scope-Aware Delivery And Verification

The controller SHALL complete the authorized accepted outcome without mandatory model downgrades, per-file approval gates or unbounded construction. Verification SHALL match the actual candidate, environment, affected behavior and claim level. Valid evidence SHALL be reusable across roles; repeated or broader checks SHALL require changed inputs, unresolved risk, missing provenance or meaningful independent verification. Explicit permissions, writer exclusion and product/formal evidence gates SHALL remain intact.

#### Scenario: Settled work continues
- **WHEN** the user authorizes completion of a settled change and related tasks share a delivery outcome
- **THEN** the controller may group implementation and validation while preserving every requirement and necessary propagation
- **AND** it asks only for an unresolved material decision or missing reserved authority and continues independent authorized work

#### Scenario: Candidate evidence is reused
- **WHEN** a worker returns passing evidence bound to the same stable candidate and relevant environment
- **THEN** the controller independently reviews the diff and evidence and runs only additional checks needed for its claim
- **AND** a focused pass does not establish an unfinished product, package or formal RC obligation

#### Scenario: No-progress work is reassessed
- **WHEN** repeated investigation or verification produces no new evidence, hypothesis or substantive adjustment
- **THEN** the controller changes approach or reports the exact blocker instead of repeating the same work
- **AND** it does not waive a required finding or gate to claim completion
