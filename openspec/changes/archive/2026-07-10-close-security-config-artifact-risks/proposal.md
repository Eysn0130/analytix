## Why

The current desktop can start with an approval/sandbox combination that allows
mutating tools without approval, while several Settings controls claim Go
runtime behavior that is not actually wired and copy unused media credentials
into runtime config. Generated packages/evidence are also tracked and linted as
source, and a historical Windows QA document left live operator access material
in Git history. These gaps must be closed together so the product, repository,
and operator runbooks describe and enforce the same security boundary.

## What Changes

- **BREAKING** Change the fresh/default execution policy to
  `approvalPolicy: on-request` plus `sandboxMode: workspace-write`, migrate only
  untouched legacy defaults once, and make Go `workspace-write` reject host
  shell/background-shell execution rather than behaving like full access.
- Retain compatibility parsing for TypeScript-era settings, but stop presenting
  or synchronizing controls that have no current Go production consumer.
  Preserve active step-limit, MCP, skills, subagent, web, Vision Bridge,
  Computer Use, provider, Write image-generation, and speech-to-text surfaces.
- Remove unused image/speech/music/video credentials and inactive tuning blocks
  from `<dataDir>/config.json`, stop model prompts from advertising unavailable
  `generate_*` tools, and write remaining secret-bearing runtime/MCP config with
  restrictive local permissions.
- Reframe current memory UI as a manual persistent-record registry until model
  injection and automatic capture are implemented; do not expose a toggle or
  diagnostics copy that promises unavailable behavior.
- Exclude generated/tool/vendor inputs from ESLint, untrack and ignore root
  `output/`, `dist-standard-win/`, and runtime-local output trees while
  preserving the files on disk and retaining curated evidence under explicit
  source/evidence paths.
- Keep the shipped runtime config example on the production-consumed allowlist,
  preserve a safe old-to-new Analytix commit provenance map after history
  rewrites, and purge a discovered retired report containing personal financial
  identifiers from every retained local ref.
- Replace the credential-bearing Windows QA path with a secret-free operator
  runbook that uses a local SSH alias and approved secret storage. Live host
  passwords, remote-control identifiers, and access codes SHALL NOT be stored
  in Git even when an operator requests plaintext convenience.
- **BREAKING** Purge the compromised QA path and the discovered retired
  privacy-sensitive report from all local refs, then expire unreachable Git
  objects after safety verification. This rewrites commit/tag identities;
  preserve a non-secret mapping for locally cited Analytix commits, while old
  clones and backups still require owner-coordinated replacement.
- Archive completed Hub startup-auth and Vision Bridge OpenSpec changes only
  after strict validation and main-spec synchronization.

## Capabilities

### New Capabilities

- `secure-runtime-execution-defaults`: Safe fresh defaults, one-time migration,
  and matching Go approval/sandbox enforcement.
- `production-config-truthfulness`: Only production-consumed settings are
  advertised/synchronized, unused secrets are removed, and local runtime config
  has a restrictive storage boundary.
- `repository-evidence-hygiene`: Lint/source scope, generated artifact tracking,
  curated evidence rules, and compromised-history cleanup.
- `windows-qa-operator-access`: Repeatable Windows SSH/RDP/remote QA through
  local aliases and secret storage without repository credentials.

### Modified Capabilities

- None. The existing OpenSpec main specs do not yet define these repository and
  runtime-policy requirements.

## Impact

- Settings/default/migration contracts in `src/shared`, IPC schemas, renderer
  permission/agent/media/memory settings, and localized copy.
- Go thread/runtime defaults, approval policy, filesystem/shell enforcement,
  background command behavior, runtime info, and focused Go tests.
- Electron runtime-config synchronization and tests, including removal of
  unused secret-bearing capability nodes.
- `eslint.config.js`, `.gitignore`, tracked generated artifacts, configuration
  examples, documentation/provenance, and Git history across all local refs.
- Windows QA operating procedure and local operator setup. No live credential
  values are accepted into source, fixtures, command output, or change
  artifacts.
