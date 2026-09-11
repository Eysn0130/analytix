# agent-handoff-review Specification

## Purpose
Define bounded agent handoff and review for Analytix absorption and commissioned
delivery, including Owner authority, Slice briefs and review, economical
monitoring, safe context transfer, evidence records, OpenSpec source of truth
and the Superpowers reference-only boundary.

## Requirements

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

#### Scenario: Current evidence is condensed
- **WHEN** the active recovery view is refreshed
- **THEN** each acceptance obligation retains its most recent still-valid evidence with pointers to complete external history, literal failure classifications, candidate/execution identity and invalidation boundaries
- **AND** remaining gaps and next action stay current without deleting evidence by byte threshold or creating a duplicate ledger

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

### Requirement: Owner Directed Slice Delivery

Commissioned delivery SHALL retain one verified Owner decision route and one active control authority for each milestone. The Controller SHALL issue coherent briefs, resolve ordinary in-scope implementation and corrections, consolidate review findings and report accepted outcomes without escalating settled decisions repeatedly. Explicit human command, target, permission and acceptance restrictions SHALL survive budget changes, follow-up and context transfer.

#### Scenario: Related corrections are discovered
- **WHEN** a Slice candidate needs related in-scope corrections under a settled accepted target
- **THEN** the Controller seals a safe candidate when necessary and sends a consolidated correlated revision
- **AND** independent optional improvements do not become required construction

#### Scenario: A user-imposed command limit exists
- **WHEN** a prior human instruction prohibits a rerun or restricts exact targets
- **THEN** changing the Controller's own budget, brief or epoch does not remove that restriction

#### Scenario: A brief ends before the authorized delivery
- **WHEN** a coherent brief is ready for review but the accepted authorized outcome has remaining work
- **THEN** the Controller records BRIEF_DONE separately from execution and lease state, completes required review and chooses the next dependency-valid gap through existing scope/spec references
- **AND** settled in-scope breakdown, corrections and ordinary verification remain with the Controller without an additional Owner approval layer
- **AND** a reserved or unauthorized next-package decision enters WAIT_OWNER; only acceptance of the entire current authorized outcome permits DELIVERY_COMPLETE

### Requirement: Safe Brief Completion And Reuse

A healthy Slice task SHALL be reusable after normal accepted brief completion only when both parties acknowledged that lifecycle before use, the old brief and lease are irrevocably closed, commands are proven inactive and the new work has a fresh brief, baseline and lease. Explicit permanent task retirement SHALL remain permanent, including for older protocol participants.

#### Scenario: A supported brief completes
- **WHEN** the candidate is accepted and a matching normal-completion acknowledgement proves the agreed safe state
- **THEN** the Controller may assign already-authorized next work to that non-retired task under a new brief and lease
- **AND** delayed old-brief packets cannot reactivate it

#### Scenario: Legacy terminal retirement is present
- **WHEN** a task was permanently retired or did not acknowledge the normal-completion extension
- **THEN** the Controller preserves its existing terminal lifecycle and does not retroactively reinterpret it as reusable

### Requirement: Economical Actionable Monitoring

Delegated delivery monitoring SHALL prefer actionable events and use bounded native waits for short joins. Authorized unattended monitoring SHALL have a verified wake route and at most one corresponding heartbeat per active Controller. It SHALL consume only necessary status and evidence, retain per-target cursors, deduplicate handled events and remain quiet on non-actionable state. It SHALL NOT treat monitoring as a second writer or infer success from completion notification, task idleness or artifact naming.

#### Scenario: A native completion is failed
- **WHEN** a native turn-completed notification reports failed or interrupted execution
- **THEN** monitoring records that state and requests only the evidence needed for recovery
- **AND** it does not claim candidate acceptance or absence of a writer from that notification

#### Scenario: A quiet heartbeat finds no change
- **WHEN** a scheduled check finds neither an unhandled event nor an executable authorized next action
- **THEN** it ends that check quietly without transcript replay, product commands or a waiting loop
- **AND** the product delivery remains pending under its existing authority

#### Scenario: A local test result changes
- **WHEN** a new local RED/GREEN result or test number does not change a reserved decision, real blocker, accepted target/architecture, delivery path, stable review/completion or urgent safety state
- **THEN** ordinary evidence remains local and does not trigger an Owner notification
- **AND** actionable events still use the valid route without ACK-of-ACK exchanges

#### Scenario: A direct Controller step is ready without a new event
- **WHEN** there are no active Slices or new events but a current authorized next action is ready and unexecuted
- **THEN** the Controller reconciles its step/session and writer authority and performs that action
- **AND** event silence alone does not cause early return, duplicate execution or artificial Slice creation

#### Scenario: Only one wait target is returned
- **WHEN** a multi-target wait returns only one completed target
- **THEN** only returned cursors advance and already handled terminal targets are excluded from subsequent waits
- **AND** the monitor does not assume other targets were examined

#### Scenario: A task is only auditing instructions
- **WHEN** the current request reviews or edits the skill without commissioning live delivery monitoring
- **THEN** a prepared heartbeat prompt does not install an automation or authorize restarting interrupted product tasks

#### Scenario: A side-effecting launch loses its completion receipt
- **WHEN** an interruption leaves the execution of an authorized step uncertain
- **THEN** recovery uses its brief-scoped step/session identity and available effect or completion evidence before starting another command
- **AND** unresolved identity blocks only dependent work, without bypassing writer leases or human command limits
- **AND** ordinary read-only probes do not require an additional command journal

#### Scenario: A delivered Owner event has no decision yet
- **WHEN** the message tool confirms delivery of a decision request but no matching decision has arrived
- **THEN** the same compact record retains its event identity, brief/revision, delivery and decision-ACK states and next useful check
- **AND** at most one overdue confirmation for unchanged state is followed by backoff or a recorded pause with a verified wake route, without a second Owner watcher or repeated reminders
- **AND** delivery and elapsed time do not grant the missing authority

#### Scenario: Quota prevents the scheduled model from starting
- **WHEN** a persistent platform failure prevents model execution or the required pause tool
- **THEN** monitoring does not claim that a prompt has paused the automation or that an unverified scheduler circuit breaker will do so
- **AND** the available coordinator or user pause route and exact unobserved limitation are retained without spawning another watcher

#### Scenario: Only active scheduling configuration is observed
- **WHEN** an automation is ACTIVE but no natural scheduled run receipt has been observed
- **THEN** the claim is limited to configured scheduling, not verified unattended execution or automatic failure handling
- **AND** validation does not manufacture repeated runs or waits just to obtain a scheduled success

### Requirement: Safe Control Context Transfer

Slice or Controller context transfer SHALL preserve accepted scope, source/evidence identity, explicit restrictions, pending decisions and the verified Owner route in a compact recovery record. Transfer SHALL require inactive mutation, revoked writer leases and paused outgoing monitoring before successor bootstrap and rebinding. Replacing the canonical Owner SHALL require explicit authority for that route change.

#### Scenario: A paused writer is unresolved
- **WHEN** the old writer still has an unrevoked lease or its mutation activity cannot be disproved
- **THEN** another writer is not granted ownership and a fresh context does not bypass the blocker

#### Scenario: Execution context grows unreliable
- **WHEN** repeated reconstruction or drift makes a fresh Controller more effective
- **THEN** the Controller transfers at a verified safe boundary with concise evidence pointers
- **AND** neither a fixed compaction count nor elapsed time alone mandates rotation

#### Scenario: A helper lane is selected or continued
- **WHEN** a bounded defect/owner lane benefits from a helper
- **THEN** a suitable existing helper may be reused, while an independent new owner, stale-hypothesis contamination or stable final review may justify fresh limited context
- **AND** active high-risk review finishes naturally unless actual safety or invalidated work requires intervention; no round count mandates replacement
- **AND** useful low-risk constructor, field or call-site work may use a fixed named Luna Max role, with direct work allowed when cheaper and model/ownership boundaries preserved

### Requirement: Message Continuity During Commissioned Work

Control SHALL distinguish message receipt, applied revision, safe command/candidate state and accepted completion. Actual sender and current authority/brief/revision SHALL be matched before applying peer instructions. Ordinary messages SHALL preserve the current goal and in-flight command identity; direct user instructions retain their actual priority. Native delivery SHALL NOT be represented as a guaranteed after-turn queue or process cancellation.

#### Scenario: A same-brief supplement arrives during a command
- **WHEN** a valid higher CONTINUE revision adds a small same-brief clarification while a command is running
- **THEN** the receiver preserves the original command identity/evidence and applies the revision at the next safe command boundary before a new step
- **AND** one matching application event, when needed, closes the change without restarting the command or acknowledging acknowledgements

#### Scenario: A future brief arrives early
- **WHEN** the current brief remains active and a future independent intent is known
- **THEN** the sender retains a short intent/dependency until an authorized natural boundary instead of pushing repeated full prompts
- **AND** an early recipient does not replace the current goal, launch the future brief or infer missing authority

#### Scenario: A short maintenance window uses a recoverable checkpoint
- **WHEN** the Owner selects a natural recoverable checkpoint before a long brief finishes
- **THEN** a short authorized maintenance window may proceed after preserving the candidate/evidence, proving relevant commands inactive and explicitly releasing conflicting reservations
- **AND** ordinary future intent or PAUSED status alone does not authorize preemption or transfer; unfinished work retains its identity and resumes only under a valid fresh lease

#### Scenario: A revision invalidates the candidate
- **WHEN** a material authorized change makes the current candidate or in-flight work invalid
- **THEN** the Controller obtains a bounded REVIEW_SEAL and real safe-state evidence before consolidated REVISE
- **AND** message delivery or turn completion alone does not establish stopped child processes

#### Scenario: An urgent authorized safety stop arrives
- **WHEN** the valid control route or direct user requests an urgent safety stop or cancellation
- **THEN** it is handled promptly without waiting behind ordinary future intents
- **AND** stopped state is claimed only after the relevant command/process evidence, with unrelated work and permission boundaries preserved

#### Scenario: A stale revision or duplicate acknowledgement arrives
- **WHEN** a message belongs to an obsolete control identity/revision or repeats an already handled application ACK
- **THEN** it cannot restart work, advance authority or generate an ACK-of-ACK loop
- **AND** a finite audit/specialist routes necessary changes through the responsible coordinator and exits daily dispatch after its own handoff

### Requirement: Explicit Scoped Parallel Writer Adoption

New briefs MAY adopt SCOPED_PARALLEL_WRITERS_V1 after Controller/writer acknowledgement before writing. Each write scope and conflicting resource SHALL have at most one unrevoked writer. Legacy or unacknowledged modes SHALL remain repository-exclusive. One coordinator SHALL serialize shared Git integration and accept stable combined candidates. Scope records SHALL NOT imply OS isolation or additional authority.

#### Scenario: Two independent Slices can improve delivery
- **WHEN** new briefs explicitly adopt scoped concurrency and shared contracts, disjoint write scopes and relevant resource isolation are established
- **THEN** both Slices may implement concurrently without a mandatory minimum Slice count
- **AND** required propagation and shared generated outputs, fixtures, ports and Git integration are considered rather than assuming different directories prove independence

#### Scenario: An added path or resource conflicts
- **WHEN** otherwise necessary propagation intersects another ACTIVE or PAUSED reservation
- **THEN** the writer coordinates before writing and affected conflicting work is isolated or serialized
- **AND** unknown independence falls back to one writer with parallel read-only work, without commandeering files or weakening exact-file restrictions

#### Scenario: Existing exclusive work is active
- **WHEN** a current or historical repository-exclusive lease is ACTIVE or PAUSED
- **THEN** adding scoped capability does not narrow, reopen or convert it
- **AND** another conflicting writer waits for actual command safety and legitimate revocation; new scoped briefs require their own explicit adoption

#### Scenario: Parallel candidates are integrated
- **WHEN** the scoped Slices submit safely sealed contributing candidates
- **THEN** one coordinating agent serializes shared Git/index/commit integration and reviews combined behavior on a stable candidate
- **AND** individual Slice passes do not replace necessary combined acceptance or authorize unsafe shared-resource commands
