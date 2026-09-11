# hub-startup-auth Specification

## Purpose
Define local Provider readiness for ordinary startup and keep Hub authentication behind explicit lazy compatibility.

## Requirements

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
