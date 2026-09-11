## ADDED Requirements

### Requirement: Owner Directed Slice Delivery

Commissioned delivery SHALL retain one verified Owner decision route and one active control authority for each milestone. The Controller SHALL issue coherent briefs, resolve ordinary in-scope implementation and corrections, consolidate review findings and report accepted outcomes without escalating settled decisions repeatedly. Explicit human command, target, permission and acceptance restrictions SHALL survive budget changes, follow-up and context transfer.

#### Scenario: Related corrections are discovered
- **WHEN** a Slice candidate needs related in-scope corrections under a settled accepted target
- **THEN** the Controller seals a safe candidate when necessary and sends a consolidated correlated revision
- **AND** independent optional improvements do not become required construction

#### Scenario: A user-imposed command limit exists
- **WHEN** a prior human instruction prohibits a rerun or restricts exact targets
- **THEN** changing the Controller's own budget, brief or epoch does not remove that restriction

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
- **WHEN** a scheduled check finds no actionable delta on the known active targets
- **THEN** it ends that check quietly without transcript replay, product commands or a waiting loop
- **AND** the product delivery remains pending under its existing authority

#### Scenario: Only one wait target is returned
- **WHEN** a multi-target wait returns only one completed target
- **THEN** only returned cursors advance and already handled terminal targets are excluded from subsequent waits
- **AND** the monitor does not assume other targets were examined

#### Scenario: A task is only auditing instructions
- **WHEN** the current request reviews or edits the skill without commissioning live delivery monitoring
- **THEN** a prepared heartbeat prompt does not install an automation or authorize restarting interrupted product tasks

### Requirement: Safe Control Context Transfer

Slice or Controller context transfer SHALL preserve accepted scope, source/evidence identity, explicit restrictions, pending decisions and the verified Owner route in a compact recovery record. Transfer SHALL require inactive mutation, revoked writer leases and paused outgoing monitoring before successor bootstrap and rebinding. Replacing the canonical Owner SHALL require explicit authority for that route change.

#### Scenario: A paused writer is unresolved
- **WHEN** the old writer still has an unrevoked lease or its mutation activity cannot be disproved
- **THEN** another writer is not granted ownership and a fresh context does not bypass the blocker

#### Scenario: Execution context grows unreliable
- **WHEN** repeated reconstruction or drift makes a fresh Controller more effective
- **THEN** the Controller transfers at a verified safe boundary with concise evidence pointers
- **AND** neither a fixed compaction count nor elapsed time alone mandates rotation
