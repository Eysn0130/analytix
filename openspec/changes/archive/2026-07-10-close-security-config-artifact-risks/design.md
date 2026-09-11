## Context

analytix currently has four related trust gaps. Fresh desktop settings use
`auto` approval with `danger-full-access`; the Go runtime also treats
`workspace-write` as sufficient for host shell execution. Several renderer
controls and synchronized config blocks describe TypeScript-era behavior that
the production Go runtime does not consume. Secret-bearing settings and MCP
configuration are written with ordinary file permissions, while unused media
credentials are copied to the Go config. Finally, generated packages/evidence
are tracked and linted as source, and a historical Windows QA document exposed
operator access material in Git history.

These are coupled by a single requirement: the implemented boundary, settings
surface, repository evidence, and operator documentation must tell the same
truth. A UI-only or documentation-only correction would leave an exploitable
or misleading alternate path.

## Goals / Non-Goals

### Goals

- Give new interactive sessions a conservative execution default and migrate
  only settings that still equal the untouched historical default.
- Make Go enforcement match the displayed sandbox policy, including shell,
  background shell, and filesystem alias resolution.
- Synchronize and restart the Go runtime only for production-consumed config;
  remove unused secret copies and misleading controls/prompts.
- Retain working product surfaces: provider profiles, step limits, MCP, skills,
  subagents, web, Vision Bridge, Computer Use, Write image generation, and
  speech-to-text dictation.
- Treat current memory as a manual persistent-record registry until automatic
  model injection/capture is actually implemented.
- Apply restrictive permissions to local files that can contain credentials.
- Separate generated packages and local evidence from maintained source and
  purge known credential-bearing or privacy-sensitive retired paths from
  retained Git history without making local evidence citations unresolvable.
- Provide a repeatable Windows QA runbook using non-secret connection metadata,
  local SSH aliases, key authentication, and approved secret storage.

### Non-Goals

- Implementing SQLite storage, hook execution, automatic compaction, tool-storm
  tuning, argument repair, automatic memory injection/capture, or new Go media
  generation tools.
- Providing an OS-level container or claiming that `workspace-write` is one.
- Removing compatibility parsing needed to import existing local settings.
- Rotating a remote credential without an authenticated operator session or
  putting the old secret into command output.
- Deleting local generated packages from disk when untracking them.

## Decisions

### 1. Version execution policy independently and migrate narrowly

Runtime settings gain an `executionPolicyVersion` marker. Version 2 fresh
defaults are `approvalPolicy: on-request` and `sandboxMode: workspace-write`.
When the marker is absent, normalization migrates only the exact legacy pair
`auto` plus `danger-full-access`; all other valid user combinations are
preserved. Version-2 users may still explicitly select the risky pair. Invalid
enum values fall back to the safe pair.

Durable thread policy records use the same marker. A legacy idle thread with
the exact old pair is migrated at the next safe policy-resolution boundary;
explicit request overrides win, and running turns or pending approval/input
gates are never rewritten in place.

This avoids a global settings-schema rename and keeps unrelated compatibility
code stable while still distinguishing an intentional opt-in from an untouched
default.

### 2. Separate interactive and unattended policy defaults

Interactive desktop sessions default to `on-request` plus `workspace-write`.
Unattended Connect Phone and scheduled-task entry points use `never` plus
`workspace-write`; an approval policy that waits for an absent operator would
hang those jobs. A later explicit product design may add controlled unattended
privilege escalation, but it is not implicit here.

### 3. Enforce workspace-write as an application path boundary

Foreground and background host shell execution require
`danger-full-access`; `workspace-write` rejects them before approval routing.
Filesystem tools continue to use real-path containment. External read aliases
must resolve the final target and re-check containment so a symlink under an
allowed alias cannot escape it.

Product copy will state that `workspace-write` is an analytix tool/path policy,
not an operating-system sandbox. This is deliberately conservative until a
real OS sandbox exists.

### 4. Synchronize an explicit production config projection

Electron builds `<dataDir>/config.json` from an allowlist of Go-consumed
settings rather than serializing the renderer settings tree. Inactive storage,
quality/hooks, token-economy, automatic compaction, tool-storm, argument-repair,
memory enablement, and media-generation nodes are neither synchronized nor
included in the runtime restart key.

Compatibility fields remain readable during normalization, but the Settings UI
does not advertise them as active. Prompts must not instruct models to call
missing `generate_image`, `generate_speech`, `generate_music`, or
`generate_video` runtime tools. Electron Write image generation and
speech-to-text dictation remain available in their actual scopes.

### 5. Wire small active contracts instead of hiding them

The Go provider stream watchdog consumes `streamIdleTimeoutMs`: a positive
value is a millisecond timeout, zero disables the watchdog, and absence retains
the current 120-second default. Provider model-profile
`contextWindowTokens` is propagated to runtime model information. Automatic
compaction thresholds and top-level compaction model lists remain unavailable.

### 6. Make memory claims match the implementation

The memory page remains a manual CRUD/privacy surface for persistent records.
It does not expose an enable switch or claim that records are injected into
model context or automatically learned. Runtime capability/diagnostic text
uses a store-only/manual status until those behaviors have an implementation
and contract tests.

### 7. Protect every secret-bearing local config file

Settings, runtime config, MCP JSON/TOML, and atomic-write destinations that can
hold provider or schedule secrets are created with owner-only permissions
(`0600` on POSIX). Rewrites also tighten an existing file's mode. Runtime config
contains no unused media credential copies. Windows ACL behavior is left to
the platform's per-user profile boundary; POSIX mode assertions are gated by
platform in tests.

### 8. Define maintained-source and evidence boundaries

ESLint ignores generated packages, local outputs/evidence, managed browser
payloads, and agent tooling metadata while continuing to lint product source.
Root `output/`, `dist-standard-win*`, `validation-evidence/`, and
`packages/runtime-go/output/` are removed from the Git index and ignored, but
preserved on disk. Required packaging inputs such as `vendor/` and
`managed-chrome/` remain tracked; managed payloads are only excluded from lint.

Durable conclusions belong in accepted specs or curated documentation with
provenance, not in bulk local snapshots.

The tracked runtime config example is part of this truthfulness boundary. It
contains only production-consumed field families and uses the safe execution
default; compatibility-only storage, hook, quality, token-economy, compaction,
memory-enablement, and media blocks remain documented as inactive rather than
advertised in the example.

### 9. Use a secret-free Windows operator runbook

Tracked documentation may contain the LAN host address, hostname, account name,
and service ports needed to identify the approved QA machine. It must not
contain a password, RustDesk/remote-control identifier, access code, token, or
private key. Operators use a stable local SSH alias, Ed25519 public-key
authentication, and a local secret manager for any fallback credential. The
runbook includes RDP and remote-control fallback procedures without their live
secret values.

The historical compromised path is removed from all retained local refs with a
history-rewrite tool, verified by path and pattern scans, then unreachable
objects are expired. This cannot revoke already copied authenticators:
password/access-code rotation and replacement of old clones/backups remain an
owner action and a hard incident-closure gate. A remote-control identifier is
kept out of source and stored locally, but is not treated as authentication on
its own.

The effective Windows OpenSSH `Match` rules decide the public-key file. The
approved administrator account uses the ProgramData administrators key file
with Administrators and SYSTEM ACLs identified by well-known SIDs; a
profile-local key file is documented only for a confirmed non-administrator.

### 10. Preserve evidence provenance while removing retired sensitive history

History rewrite changes Analytix commit and tag identities. Current manifests
must use the post-rewrite commit, while dated evidence may retain its original
local source commit only when a tracked, non-secret provenance map resolves it
to the final Analytix commit. The map applies only to explicitly classified
local Analytix history; external upstream commit/tag/blob identifiers are not
mechanically remapped even when their hex value appears in filter tooling.

A separately discovered, already-deleted financial analysis report contains
personal and full account identifiers in retained history. It is not required
product evidence and is purged by path from every retained local ref. The
verification records only path reachability and counts, never the report
contents or identifiers.

## Risks / Trade-offs

- Existing automation that relied on the unsafe default can stop or request
  approval. Narrow migration and explicit unattended defaults make that change
  visible rather than silently retaining privilege.
- Blocking shell in `workspace-write` is stricter than the setting's old
  behavior. Users who genuinely require host shell can explicitly select
  `danger-full-access` with clear copy.
- Hiding inactive controls reduces apparent configurability but removes false
  promises. Stored legacy values are preserved for import compatibility.
- Owner-only POSIX modes do not replace Windows ACL review and cannot protect a
  compromised user account.
- Untracking large artifacts changes repository checkout contents but keeps
  local files. History purification rewrites commit identities and requires
  coordinated replacement of clones; it must be performed only after a clean
  backup/ref inventory and verification plan.
- Dated evidence that stores a pre-rewrite local commit becomes hard to resolve
  unless the rewrite publishes a scoped provenance map. A broad mechanical SHA
  replacement would corrupt external upstream references, so mapping is
  restricted to classified Analytix-local evidence identifiers.
- Publishing non-secret LAN metadata still reveals topology to anyone with
  repository access. It is accepted because the user explicitly requires a
  reusable operator method; credentials remain separated.

## Migration Plan

1. Land contracts, version marker, normalization, and focused migration tests.
2. Land Go policy enforcement, unattended defaults, and filesystem containment
   tests before exposing the new defaults.
3. Replace runtime-config serialization with the explicit allowlist, wire the
   stream/model contracts, tighten file modes, and remove unused credential
   copies.
4. Align Settings, diagnostics, memory/media copy, and model prompts with the
   production feature set.
5. Establish lint/ignore boundaries and remove generated trees from the index
   without deleting local files.
6. Replace the Windows QA document with the secret-free runbook. The owner then
   rotates fallback credentials and installs the approved SSH public key.
7. In isolated maintenance steps, rewrite retained refs to remove the old QA
   path and the retired privacy-sensitive report, verify both rewrites, replace
   old clones/backups, and expire unreachable objects.
8. Publish a safe Analytix-local old-to-new commit map and update current
   manifests without rewriting external upstream identifiers.
9. Run focused TypeScript and Go tests, typecheck, lint, strict OpenSpec
   validation, link/path scans, secret scans, and `git diff --check`.

Rollback of code/config changes is a normal commit revert, but it must not
restore unsafe defaults or secret copies. Git history rewrite is not rolled
back by force-publishing old refs; recovery uses the pre-rewrite offline backup
only for incident response.

## Resolved And External Boundary

- The authenticated Windows operator session completed credential rotation,
  SSH key verification, and the administrator key-file/ACL review.
- Both sensitive historical paths are absent from retained local refs and
  objects; temporary recovery bundles were removed after verification.
- This checkout has no configured remote. Any independently held pre-rewrite
  clone, bundle, backup, or release archive remains outside the local closure
  boundary and must not reintroduce old objects.
