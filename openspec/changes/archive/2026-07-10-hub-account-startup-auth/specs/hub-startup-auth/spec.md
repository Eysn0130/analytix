## ADDED Requirements

### Requirement: Hub startup gate replaces ordinary API key setup
The desktop app SHALL use Hub account authorization, not missing API key settings, as the ordinary first-run gate.

#### Scenario: Fresh packaged user starts the app
- **WHEN** the app starts with no stored Hub desktop authorization
- **THEN** the app shows the startup splash and then the Hub login page without opening the API key setup dialog

#### Scenario: Existing API key is absent but Hub auth is valid
- **WHEN** settings have no manual API key and the current user has valid desktopAuth and gateway credentials
- **THEN** the app enters the main workspace and allows runtime startup

### Requirement: Startup splash and prewarm run before auth handoff
The desktop app SHALL display the migrated Analytix_new startup splash immediately and prewarm main workspace and login resources while authorization is checked.

#### Scenario: Boot completes with authorized account
- **WHEN** splash completion and account refresh both complete successfully
- **THEN** the app transitions to the main workspace without showing the login form

#### Scenario: Boot completes without authorized account
- **WHEN** splash completion finishes but no valid Hub authorization exists
- **THEN** the app transitions to the migrated Hub login page

### Requirement: Account readiness controls model access
The desktop app SHALL distinguish logged-in-but-not-ready accounts from model-ready accounts.

#### Scenario: Hub login returns no gateway
- **WHEN** a login response includes desktopAuth but lacks gateway because phone or real-name verification is incomplete
- **THEN** the app stores the desktop account state, displays the missing verification guidance, and keeps model calls disabled

#### Scenario: Hub refresh reports invalid desktopAuth
- **WHEN** desktopAuth refresh returns unauthorized or expired
- **THEN** the app clears local auth state and returns to the Hub login page

### Requirement: Migrated login features are preserved
The Hub login UI SHALL preserve the Analytix_new login, registration, verification code, password reset, challenge, remember-login, input draft, error recovery, unverified-account, and logout interactions adapted to the current desktop architecture.

#### Scenario: User completes Hub login
- **WHEN** the user submits valid Hub credentials and any required auth challenge
- **THEN** desktopAuth is persisted by main, gateway is configured if returned, and the app transitions according to account readiness

#### Scenario: User resets password
- **WHEN** the user requests and confirms a password reset code
- **THEN** the UI surfaces success and returns the user to login without losing the remembered email draft
