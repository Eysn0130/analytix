# production-config-truthfulness Specification

## Purpose
Keep desktop settings, runtime configuration, examples, prompts, and
capability copy aligned with production-consumed Go behavior while protecting
secret-bearing local files.

## Requirements
### Requirement: Runtime config contains only production-consumed settings

Electron SHALL construct the Go runtime config from an explicit allowlist of
production-consumed settings and SHALL exclude inactive storage, hooks,
token-economy, automatic-compaction, tool-storm, argument-repair, memory
enablement, and media-generation blocks.

#### Scenario: Synchronize runtime settings

- **WHEN** desktop settings are written to `<dataDir>/config.json`
- **THEN** only the explicit production config projection is serialized
- **AND** no image, speech, music, or video API credential is copied there

### Requirement: Shipped config example reflects production truth

The tracked runtime config example SHALL use the safe execution defaults and
SHALL show only production-consumed configuration families. It SHALL NOT
advertise compatibility-only storage, hook, quality, token-economy,
automatic-compaction, memory-enablement, or media-generation blocks.

#### Scenario: Validate the tracked example

- **WHEN** the shipped example is parsed and reviewed
- **THEN** it passes the public config schema
- **AND** its execution policy is `on-request` plus `workspace-write`
- **AND** compatibility-only field families are absent

### Requirement: Inactive settings do not restart the runtime

The desktop SHALL derive its runtime restart key from the same active startup
projection used by the production Go process rather than the complete legacy
settings object.

#### Scenario: Edit a compatibility-only setting

- **WHEN** a stored inactive compatibility field changes
- **THEN** the runtime settings key remains unchanged
- **AND** the Go runtime is not restarted for that edit

### Requirement: Settings advertise only implemented behavior

The Settings UI SHALL hide or mark unavailable controls without a production
Go consumer while retaining controls for provider profiles, step limits, MCP,
skills, subagents, web, Vision Bridge, Computer Use, Write image generation,
and speech-to-text in their implemented scopes.

#### Scenario: Open agent settings

- **WHEN** a user inspects agent/runtime settings
- **THEN** storage, hook, token-economy, automatic-compaction, tool-storm, and
  argument-repair controls are not presented as active Go runtime behavior

#### Scenario: Open media settings

- **WHEN** a user inspects media capabilities
- **THEN** Write image generation and speech-to-text are scoped to their real
  Electron features
- **AND** Go text-to-speech, music, and video generation are unavailable

### Requirement: Prompts do not advertise missing runtime tools

System prompts for Connect Phone and scheduled tasks SHALL NOT instruct a model
to call unavailable `generate_image`, `generate_speech`, `generate_music`, or
`generate_video` tools.

#### Scenario: Build a scheduled-task prompt

- **WHEN** the desktop constructs a prompt for an unattended Go runtime task
- **THEN** the prompt contains no instruction to call an unavailable
  `generate_*` media tool

### Requirement: Stream idle timeout has an end-to-end contract

The Go provider stream watchdog SHALL use a positive `streamIdleTimeoutMs` as a
millisecond timeout, SHALL disable the watchdog when the configured value is
zero, and SHALL retain the 120-second default when the value is absent.

#### Scenario: Disable stream idle watchdog

- **WHEN** runtime config explicitly sets `streamIdleTimeoutMs` to zero
- **THEN** a provider stream is not cancelled by the idle watchdog

#### Scenario: Omit stream idle timeout

- **WHEN** runtime config omits `streamIdleTimeoutMs`
- **THEN** the provider stream uses a 120-second idle timeout

### Requirement: Provider context-window metadata reaches runtime information

The desktop and Go runtime SHALL propagate a provider model profile's
`contextWindowTokens` into runtime model information without treating
automatic-compaction model lists as provider models.

#### Scenario: Select a profiled provider model

- **WHEN** the selected provider model has `contextWindowTokens`
- **THEN** runtime model information reports that context-window value
- **AND** unrelated compaction-only model entries are excluded

### Requirement: Memory is presented as a manual persistent registry

The memory surface SHALL support manual record management and privacy deletion
until automatic capture and model-context injection are implemented. It SHALL
NOT expose an enable toggle that cannot disable the store, and SHALL NOT claim
automatic learning or prompt injection.

#### Scenario: Open memory settings

- **WHEN** a user views memory settings
- **THEN** the UI describes manual persistent records and deletion
- **AND** diagnostics identify the capability as manual/store-only

### Requirement: Secret-bearing local config uses restrictive permissions

The desktop SHALL create and rewrite settings, runtime config, MCP config, and
atomic-write destinations that can contain credentials with owner-only mode
`0600` on POSIX systems.

#### Scenario: Tighten an existing settings file

- **WHEN** analytix rewrites an existing secret-bearing settings file whose
  mode is broader than `0600`
- **THEN** the resulting file mode is `0600`

#### Scenario: Write runtime config on Windows

- **WHEN** analytix writes runtime config on Windows
- **THEN** the write succeeds within the current user's profile boundary
- **AND** POSIX mode assertions are not applied
