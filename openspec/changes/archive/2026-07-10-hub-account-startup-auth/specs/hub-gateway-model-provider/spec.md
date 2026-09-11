## ADDED Requirements

### Requirement: Gateway tokens remain outside renderer settings
The desktop app SHALL keep Hub gateway tokens out of renderer state, localStorage, and settings.

#### Scenario: Gateway is persisted
- **WHEN** main receives a gateway credential
- **THEN** it writes the token to a userData secrets file with restrictive permissions and stores only redacted metadata in settings

#### Scenario: Logout occurs
- **WHEN** the user logs out
- **THEN** main revokes the Hub desktop session when possible, deletes the gateway secret, clears in-memory credentials, and disables model access

### Requirement: Hub models are the source of truth
The desktop app SHALL build the managed `analytix-hub` provider from Hub `/v1/models` using the current user's gateway credential.

#### Scenario: Hub models are fetched after login
- **WHEN** a gateway credential is available
- **THEN** main requests `/v1/models` with Bearer authorization and updates the managed provider model list from the response

#### Scenario: Current model is no longer available
- **WHEN** the current runtime model is absent from the Hub model list
- **THEN** the app selects the first Hub-returned model and keeps runtime provider id set to `analytix-hub`

### Requirement: Runtime provider env injects user gateway at launch
The desktop runtime SHALL receive the current user's Hub gateway credential only through main-controlled runtime provider configuration.

#### Scenario: Runtime env is built
- **WHEN** runtime provider env is generated for `analytix-hub`
- **THEN** it includes the current user's gateway token and base URL without writing the token back to settings

#### Scenario: No gateway is configured
- **WHEN** runtime provider env is generated without a valid gateway
- **THEN** model access remains disabled and no stale gateway token is injected

### Requirement: Hub desktop endpoints support model readiness
Analytix Hub SHALL expose desktop Bearer APIs to refresh gateway credentials and provide account usage needed by desktop.

#### Scenario: Desktop refreshes gateway
- **WHEN** `/api/desktop/gateway/refresh` receives a valid desktopAuth for an accountReady user
- **THEN** it returns a gateway token and gateway baseUrl

#### Scenario: Desktop refreshes gateway for unverified account
- **WHEN** `/api/desktop/gateway/refresh` receives a valid desktopAuth for a user missing required verification
- **THEN** it returns a forbidden response with verification guidance and no gateway token
