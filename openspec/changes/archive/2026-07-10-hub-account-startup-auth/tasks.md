## 1. Hub Desktop API

- [x] 1.1 Add desktop Bearer gateway refresh, console, and referral endpoints in `/Users/sun/Projects/analytix-hub/server/index.mjs`.
- [x] 1.2 Add or update Hub tests/manual probes for accountReady, unverified, and unauthorized desktop endpoint behavior.

## 2. Desktop Account Contracts And IPC

- [x] 2.1 Add shared Hub account, auth, gateway, usage, referral, and model response types.
- [x] 2.2 Implement main-process Hub account service with login/register/password reset/challenge/refresh/logout, secret-file persistence, and token redaction.
- [x] 2.3 Register IPC handlers and preload `window.analytix.account` facade with renderer-safe payloads.
- [x] 2.4 Add dev/test bootstrap guarded by `!app.isPackaged`, non-production `NODE_ENV`, and `ANALYTIX_DESKTOP_AUTH_TEST_BOOTSTRAP=1`.

## 3. Managed Hub Model Provider

- [x] 3.1 Build `analytix-hub` provider metadata from gateway-authorized `/v1/models`.
- [x] 3.2 Inject the current user's gateway token into runtime provider env without writing it to settings.
- [x] 3.3 Replace ordinary startup API key gating with Hub account readiness gating.

## 4. Startup And Login UI Migration

- [x] 4.1 Port Analytix_new startup splash to current React/CSS without `styled-components`.
- [x] 4.2 Port Analytix_new Hub login/register/password reset/challenge UI to current renderer architecture.
- [x] 4.3 Add startup auth gate that coordinates splash completion, module prewarm, account refresh, login, unverified state, and main app handoff.

## 5. Sidebar Account Menu

- [x] 5.1 Replace the lower-left footer settings action with a taller account trigger preserving Connect Phone.
- [x] 5.2 Implement account dropdown entries for email, personal account, settings, invite friend, remaining usage, and logout.
- [x] 5.3 Implement usage/referral panels backed by desktop Bearer APIs.

## 6. Validation And Safety

- [x] 6.1 Add unit tests for startup gate, provider sync, token secrecy, logout cleanup, and bootstrap package guards.
- [x] 6.2 Run targeted tests, `npm run typecheck`, and `git diff --check` in `analytix`.
- [x] 6.3 Run relevant Hub validation for the new desktop endpoints.
- [x] 6.4 Document final packaging checks ensuring release artifacts contain no test credentials or gateway tokens.
