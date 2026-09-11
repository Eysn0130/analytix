## ADDED Requirements

### Requirement: Dev/test bootstrap avoids manual login
The desktop app SHALL provide an optional dev/test-only Hub auth bootstrap for automated tests.

#### Scenario: Test bootstrap is enabled in development
- **WHEN** the app is not packaged, `NODE_ENV` is not `production`, and `ANALYTIX_DESKTOP_AUTH_TEST_BOOTSTRAP=1`
- **THEN** main may use `ANALYTIX_HUB_TEST_EMAIL` and `ANALYTIX_HUB_TEST_PASSWORD` or pre-provided test tokens from environment secrets to acquire a test gateway

#### Scenario: Unit tests run without live Hub
- **WHEN** renderer unit tests exercise startup or menu behavior
- **THEN** they use mocked `window.analytix.account` APIs and do not require live Hub credentials

### Requirement: Packaged builds reject bootstrap
Packaged and production builds SHALL ignore all test bootstrap variables and require normal user login.

#### Scenario: Packaged app sees test env
- **WHEN** a packaged app starts with `ANALYTIX_DESKTOP_AUTH_TEST_BOOTSTRAP=1`
- **THEN** bootstrap is refused, no test login occurs, and missing user auth routes to the login page

#### Scenario: Release artifact is scanned
- **WHEN** release checks run
- **THEN** source and packaged artifacts contain no test email, password, desktop auth token, or gateway token literals

### Requirement: Test credentials are isolated
Dev/test bootstrap SHALL avoid cross-user token leakage.

#### Scenario: E2E test uses test account
- **WHEN** live E2E tests request Hub bootstrap
- **THEN** they use a temporary userData directory and clean token secrets after the run

#### Scenario: Bootstrap login fails
- **WHEN** test credentials are absent or rejected by Hub
- **THEN** the app reports a test bootstrap failure in dev/test logs without falling back to any production bypass
