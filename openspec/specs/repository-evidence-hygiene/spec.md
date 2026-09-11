# repository-evidence-hygiene Specification

## Purpose
Define maintained-source and evidence boundaries, generated-artifact tracking,
sensitive-history cleanup, and durable local commit provenance after history
rewrites.

## Requirements
### Requirement: Lint scope distinguishes source from generated inputs

ESLint SHALL continue to check maintained product source while excluding
generated packages, local output/evidence trees, managed browser payloads, and
agent-tool metadata.

#### Scenario: Run repository lint

- **WHEN** `npm run lint` runs with generated Windows packages present locally
- **THEN** generated payloads do not produce lint errors
- **AND** maintained source remains in lint scope

### Requirement: Generated artifacts are not tracked as maintained source

Git SHALL ignore and stop tracking root `output/`, `dist-standard-win*`,
`validation-evidence/`, and `packages/runtime-go/output/` while preserving
local files during the index migration.

#### Scenario: Apply artifact tracking migration

- **WHEN** the generated-artifact cleanup is applied
- **THEN** those paths are absent from the tracked-file set
- **AND** existing local files remain available to the operator

### Requirement: Required packaging inputs remain tracked

Repository cleanup SHALL retain required packaging dependencies such as
`vendor/` and `managed-chrome/`; managed browser payloads MAY be lint-excluded
without being untracked.

#### Scenario: Validate packaging inputs after cleanup

- **WHEN** tracked paths are inspected after artifact migration
- **THEN** required vendor and managed browser inputs are still tracked

### Requirement: Durable evidence has explicit provenance

Present-tense conclusions SHALL live in accepted specs or curated
documentation that identifies relevant commit/environment provenance, and bulk
local evidence snapshots SHALL NOT act as current source-of-truth documents.

#### Scenario: Cite a validation conclusion

- **WHEN** documentation makes a present-tense readiness claim
- **THEN** the claim is backed by a current rerun or identifies the recorded
  commit and environment of a snapshot

### Requirement: Compromised QA history is purged and verified

The credential-bearing historical QA path SHALL be removed from every retained
local branch and tag, the rewrite SHALL be verified by path and secret-pattern
scans, and unreachable objects SHALL be expired only after a recovery backup
and ref inventory exist.

#### Scenario: Verify rewritten refs

- **WHEN** local history rewrite completes
- **THEN** no retained branch or tag contains the compromised path
- **AND** secret-pattern scans do not find the removed operator credentials

### Requirement: History cleanup does not claim credential revocation

Repository documentation SHALL treat credential rotation and replacement of
old clones/backups as separate incident-closure gates that history rewrite
cannot satisfy.

#### Scenario: Repository is sanitized before rotation

- **WHEN** the compromised path is absent from retained Git refs but the remote
  credential has not been rotated
- **THEN** the incident remains documented as open rather than resolved

### Requirement: History rewrite preserves classified local provenance

A history rewrite SHALL leave a tracked, non-secret mapping for pre-rewrite
Analytix commit identifiers that remain cited by accepted specs, tests, or
curated evidence. Current machine-readable manifests SHALL use the final
post-rewrite commit. External upstream identifiers SHALL NOT be changed merely
because the same hex value appears in the local rewrite map.

#### Scenario: Resolve dated local evidence after a rewrite

- **WHEN** a dated evidence file cites a pre-rewrite Analytix commit
- **THEN** the provenance map resolves that local identifier to a retained
  post-rewrite commit
- **AND** a separately classified upstream commit remains unchanged

### Requirement: Retired privacy-sensitive evidence is purged

An already-deleted report containing personal financial identifiers SHALL be
removed from every retained local ref and from unreachable local objects. The
verification SHALL report only path reachability or counts, never the report's
contents or identifiers.

#### Scenario: Verify the retired report cleanup

- **WHEN** local history cleanup completes
- **THEN** no retained branch, tag, stash, or tree-valued snapshot ref exposes
  the retired report path
- **AND** the temporary recovery bundle is removed after verification
