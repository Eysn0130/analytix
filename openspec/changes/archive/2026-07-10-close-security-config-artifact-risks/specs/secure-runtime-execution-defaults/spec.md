## ADDED Requirements

### Requirement: Fresh interactive execution defaults are conservative

The desktop SHALL initialize a fresh interactive runtime with
`approvalPolicy: on-request`, `sandboxMode: workspace-write`, and execution
policy version 2.

#### Scenario: Create fresh settings

- **WHEN** no persisted analytix settings exist
- **THEN** normalized runtime settings use `on-request` and `workspace-write`
- **AND** the persisted execution policy marker is version 2

### Requirement: Legacy execution defaults migrate once

The settings normalizer SHALL migrate only an unversioned exact legacy pair of
`auto` and `danger-full-access` to the version-2 safe pair, preserve every
other valid explicit combination, and replace invalid policy enum values with
the safe pair.

#### Scenario: Migrate untouched historical defaults

- **WHEN** persisted runtime settings have no execution policy marker and equal
  `auto` plus `danger-full-access`
- **THEN** normalization returns `on-request` plus `workspace-write`
- **AND** writes execution policy version 2

#### Scenario: Preserve an explicit version-2 opt-in

- **WHEN** persisted runtime settings have execution policy version 2 and use
  `auto` plus `danger-full-access`
- **THEN** normalization preserves that combination

#### Scenario: Preserve a non-default legacy choice

- **WHEN** unversioned persisted runtime settings contain any other valid
  approval and sandbox combination
- **THEN** normalization preserves the combination and records version 2

### Requirement: Durable thread migration is safe and override-aware

The Go runtime SHALL migrate an idle unversioned thread only when its stored
policy is the exact historical default, SHALL prefer an explicit request
override, and SHALL NOT rewrite a running turn or pending approval/input gate.

#### Scenario: Resolve policy for an idle legacy thread

- **WHEN** an idle unversioned thread stores `auto` plus
  `danger-full-access` and the request has no override
- **THEN** the next resolved policy is `on-request` plus `workspace-write`
- **AND** the durable policy marker becomes version 2

#### Scenario: Request override wins migration

- **WHEN** a request supplies an explicit valid execution policy for a legacy
  thread
- **THEN** the runtime uses the request policy and records version 2

### Requirement: Unattended entry points cannot wait for approvals

Connect Phone and scheduled-task runtime entry points SHALL default to
`approvalPolicy: never` plus `sandboxMode: workspace-write` unless a future
explicit contract provides a different authorized policy.

#### Scenario: Start an unattended scheduled task

- **WHEN** a scheduled task starts without an explicit authorized policy
- **THEN** it uses `never` plus `workspace-write`
- **AND** it does not create an approval wait that no operator can answer

### Requirement: Workspace-write rejects host shell execution

The Go runtime SHALL require `danger-full-access` for foreground and background
host shell tools and SHALL reject those calls before approval routing when the
sandbox mode is `workspace-write` or `read-only`.

#### Scenario: Request shell under workspace-write

- **WHEN** a turn requests a foreground or background shell tool under
  `workspace-write`
- **THEN** the runtime returns a policy rejection
- **AND** no approval request or host process is created

### Requirement: External read aliases remain contained after symlink resolution

The filesystem adapter SHALL resolve the final real path of an external-read
alias target and verify that it remains under an authorized alias root.

#### Scenario: Alias symlink escapes its root

- **WHEN** an alias path beneath an authorized root resolves through a symlink
  to a path outside that root
- **THEN** the filesystem adapter rejects the read

### Requirement: Sandbox copy does not overstate isolation

Product settings and operator documentation SHALL describe
`workspace-write` as an analytix tool/path policy and SHALL NOT call it an
operating-system sandbox or containment boundary.

#### Scenario: Inspect permission settings copy

- **WHEN** a user views the workspace-write option
- **THEN** the copy states its application-level scope and shell restriction
- **AND** does not promise OS-level isolation
