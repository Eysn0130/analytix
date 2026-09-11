# windows-qa-operator-access Specification

## Purpose
Define repeatable secret-free access to the approved Windows QA host, including
SSH key placement, fallback credential storage and rotation, and redacted
verification.

## Requirements
### Requirement: Tracked Windows QA metadata contains no live secret

The Windows QA runbook SHALL omit every password, remote-control identifier,
access code, token, and private key. It MAY record the approved machine's LAN
address, hostname, account name, and service ports.

#### Scenario: Review the tracked runbook

- **WHEN** a reviewer inspects the Windows QA documentation
- **THEN** the machine and connection method are identifiable
- **AND** no live authentication value is present

### Requirement: SSH uses a stable local alias and public-key authentication

The runbook SHALL define a local SSH config alias and Ed25519 public-key setup
so routine access does not require a plaintext password in documentation,
scripts, shell history, or tool input.

#### Scenario: Connect after key enrollment

- **WHEN** an operator has enrolled the approved public key and configured the
  documented alias
- **THEN** the operator can connect with `ssh <alias>` without embedding a
  password in the command

#### Scenario: Rotate a key for an administrator account

- **WHEN** the approved account is captured by an OpenSSH administrators
  `Match` rule
- **THEN** the runbook uses the effective ProgramData administrators key file
- **AND** applies Administrators and SYSTEM ACLs by well-known SID
- **AND** does not instruct the operator to write an ignored profile-local file

### Requirement: Fallback credentials stay in approved local secret storage

Any RDP or remote-control fallback credential SHALL be stored in an approved
local secret manager and referenced by a non-secret locator or retrieval
procedure; it SHALL NOT be committed to Git.

#### Scenario: Use an RDP fallback

- **WHEN** SSH is unavailable and an operator follows the RDP procedure
- **THEN** the runbook provides the non-secret host/port and secret-manager
  retrieval steps
- **AND** the password is not copied from repository content

### Requirement: Exposed credentials are rotated before incident closure

The incident owner SHALL replace or disable every password, remote-control
access code, or authentication key that appeared in tracked history before
marking the incident resolved. A connection identifier SHALL remain absent
from source and SHALL be stored locally, but is not authentication by itself.

#### Scenario: History is clean but a password is unchanged

- **WHEN** Git history cleanup has completed but an exposed password remains
  valid
- **THEN** the runbook and audit status continue to report credential rotation
  as pending

### Requirement: Verification avoids secret disclosure

Automated checks SHALL verify secret absence using redacted fingerprints,
patterns, or exit status and SHALL NOT print a live secret into logs or reports.

#### Scenario: Scan documentation and retained refs

- **WHEN** the repository is scanned for leaked Windows access material
- **THEN** results report affected paths or redacted fingerprints only
- **AND** no matching live value is echoed
