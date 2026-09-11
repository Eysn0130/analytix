## MODIFIED Requirements

### Requirement: Unattended entry points cannot wait for approvals
Connect Phone and scheduled-task runtime entry points SHALL default to `approvalPolicy: never` plus `sandboxMode: workspace-write` unless a future explicit contract provides a different authorized policy. `never` SHALL mean that approval-requiring actions fail closed; it SHALL NOT authorize report/file publication, mutation, or another side effect that would otherwise require approval.

#### Scenario: Start an unattended scheduled task
- **WHEN** a scheduled task starts without an explicit authorized policy
- **THEN** it uses `never` plus `workspace-write`
- **AND** it does not create an approval wait that no operator can answer

#### Scenario: Unattended turn requests a report write
- **WHEN** a provider calls a write-capable report or artifact tool under `approvalPolicy: never` without an explicit pre-authorized artifact contract
- **THEN** the runtime rejects the call before execution
- **AND** no file is created and the final answer reports the publication boundary

#### Scenario: Read-only unattended analysis is permitted
- **WHEN** a current granted read-only tool requires no approval and satisfies workspace, case, epoch, source, and evidence policy
- **THEN** `never` does not block that read merely because no approver is present

