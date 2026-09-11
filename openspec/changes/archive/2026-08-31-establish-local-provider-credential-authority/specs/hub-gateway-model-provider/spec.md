## ADDED Requirements

### Requirement: Local Registry models are the source of truth
The desktop app SHALL build Provider and model state from the selected committed local Registry connection, including custom endpoints, discovered models, media models, proxies, and route pools.

#### Scenario: Local Provider models are refreshed
- **WHEN** a user requests discovery or probe for a Registry connection
- **THEN** the local Provider API uses that connection's protected credential and updates key-free model metadata after a successful transaction

#### Scenario: The selected model is unavailable
- **WHEN** the current model is absent from the committed Provider model set
- **THEN** the app requires a valid local selection or follows the committed local route policy without selecting a Hub-managed Provider

### Requirement: Runtime launch has no Hub gateway fallback
The desktop runtime SHALL receive only the selected local Registry Provider configuration and the just-in-time protected credential required for execution. Missing or invalid local credentials SHALL fail closed without reading or injecting a Hub gateway token.

#### Scenario: Runtime Provider configuration is built
- **WHEN** main builds Provider configuration for runtime execution
- **THEN** it resolves the committed Registry revision and credential without writing the secret to settings or public state

#### Scenario: No usable local credential exists
- **WHEN** runtime Provider configuration cannot resolve a valid committed local credential
- **THEN** model access remains disabled and no Hub token, endpoint, model, or fallback is used

### Requirement: Legacy Hub gateway state is lazy migration input
Legacy Hub desktop, gateway, model, and usage state SHALL remain outside renderer settings and SHALL be read only by an explicit deprecated compatibility or migration flow. The flow SHALL preserve recoverability until a replacement local Provider commits successfully.

#### Scenario: Ordinary startup encounters legacy Hub files
- **WHEN** legacy Hub state exists but the user has not opened the deprecated compatibility surface
- **THEN** startup does not import Hub code, read the state, refresh credentials, request models, or configure a gateway

#### Scenario: Explicit migration succeeds
- **WHEN** an authorized compatibility flow converts supported Hub-backed Provider state
- **THEN** the local Registry transaction commits and verifies the replacement before any eligible legacy cleanup occurs

#### Scenario: Explicit migration fails
- **WHEN** conversion, validation, commit, or verification fails
- **THEN** the legacy state remains recoverable and no partial local Provider becomes authoritative

## REMOVED Requirements

### Requirement: Gateway tokens remain outside renderer settings
**Reason**: Hub gateway tokens are no longer ordinary runtime credentials; all ordinary credentials belong to the local Secret Store.
**Migration**: Keep any legacy Hub token outside renderer/settings and inspect it only in an explicit protected compatibility or migration flow.

### Requirement: Hub models are the source of truth
**Reason**: The selected local Registry connection and its committed model metadata are now authoritative.
**Migration**: Convert supported Provider metadata through the local Registry and re-run local discovery or selection.

### Requirement: Runtime provider env injects user gateway at launch
**Reason**: Runtime launch resolves the local Registry credential and has no Hub gateway injection or fallback.
**Migration**: Configure a local Provider through onboarding, Settings, or explicit legacy migration before model execution.

### Requirement: Hub desktop endpoints support model readiness
**Reason**: Ordinary desktop model readiness no longer depends on Hub account or gateway endpoints.
**Migration**: Use local Registry readiness, probe, and recovery status; preserved Hub endpoints serve only an explicit deprecated compatibility lifecycle.
