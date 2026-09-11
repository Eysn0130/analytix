## ADDED Requirements

### Requirement: Local Provider readiness gates ordinary startup
The desktop app SHALL use completion of local Provider onboarding and a usable committed Registry connection, not Hub account authorization, as the ordinary first-run and model-readiness gate.

#### Scenario: Fresh user starts without a local Provider
- **WHEN** startup completes for a profile with no usable committed Provider
- **THEN** the app opens the local initial setup flow without loading or displaying Hub login

#### Scenario: A usable local Provider exists
- **WHEN** startup completes for a profile whose selected Registry connection is usable
- **THEN** the app enters the main workspace and enables runtime startup without Hub account refresh

#### Scenario: A local Provider is configured but not usable
- **WHEN** the selected Registry connection lacks a committed credential or fails local readiness validation
- **THEN** the app keeps model calls disabled and presents local Provider recovery or Settings guidance

### Requirement: Startup splash prewarms local product resources
The desktop app SHALL display the existing startup splash and MAY prewarm the main workspace and local Provider setup resources while local readiness is checked, without prewarming Hub login, account, gateway, or network resources.

#### Scenario: Splash and local readiness complete
- **WHEN** splash completion and local Registry readiness both complete
- **THEN** the app transitions to either the main workspace or local initial setup according to the committed Provider state

### Requirement: Hub authentication is explicit lazy compatibility
Hub login and account interactions SHALL be unavailable from the ordinary startup gate and SHALL load only after a user explicitly enters a deprecated compatibility or migration surface.

#### Scenario: User does not request Hub compatibility
- **WHEN** the app starts and the user uses ordinary workspace or Provider Settings flows
- **THEN** Hub login code, account state, credentials, timers, and requests remain inactive

#### Scenario: User requests deprecated Hub compatibility
- **WHEN** the user explicitly opens the deprecated Hub surface
- **THEN** the app MAY lazy-load preserved login or migration interactions without changing ordinary local Provider authority

## REMOVED Requirements

### Requirement: Hub startup gate replaces ordinary API key setup
**Reason**: Ordinary startup is now gated by the local Provider Registry; Hub authorization is compatibility-only.
**Migration**: Existing users configure or migrate a local Provider through the protected onboarding or explicit compatibility flow.

### Requirement: Startup splash and prewarm run before auth handoff
**Reason**: Startup no longer hands off to Hub authentication or prewarms Hub login resources.
**Migration**: Preserve the splash while prewarming only local workspace and Provider setup resources.

### Requirement: Account readiness controls model access
**Reason**: Local committed Provider readiness replaces Hub account and gateway readiness.
**Migration**: Map supported legacy Provider state through the Registry migration and present local recovery guidance when unusable.

### Requirement: Migrated login features are preserved
**Reason**: Hub login is no longer an ordinary startup product surface.
**Migration**: Retain supported login and recovery interactions only inside the explicit lazy deprecated compatibility surface.
