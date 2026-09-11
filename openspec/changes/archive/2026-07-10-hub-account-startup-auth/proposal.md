## Why

analytix currently gates first run on manual API key setup, which conflicts with the product direction that model access is supplied by the Analytix Hub account and gateway at `https://analytix.top`. The desktop app needs a complete Hub login/startup experience, account-scoped gateway token handling, and release-safe test automation that does not leak shared credentials into packaged builds.

## What Changes

- Replace the ordinary first-run API key requirement with a Hub account startup gate.
- Port the Analytix_new startup splash and login/register/password-reset challenge UI into the current Electron + React desktop architecture.
- Add desktop Hub account services and IPC that keep desktop auth and gateway tokens in the main process and expose only redacted account state to the renderer.
- Add Hub desktop Bearer endpoints for gateway refresh, console usage, and referral data.
- Generate and maintain an `analytix-hub` managed model provider from Hub `/v1/models`; use Hub models as the source of truth.
- Move sidebar settings into a CodexDesktop-Rebuild-inspired account dropdown that also exposes invite, remaining usage, account identity, and logout.
- Add dev/test-only Hub auth bootstrap from local/CI secrets so automated tests do not block on manual login.
- **BREAKING**: packaged builds must not allow model calls until the current user has completed Hub login and has a valid gateway credential.

## Capabilities

### New Capabilities
- `hub-startup-auth`: Hub-backed startup, login, account readiness, and renderer gate behavior.
- `hub-gateway-model-provider`: Hub gateway token persistence, model list synchronization, and runtime provider injection.
- `hub-account-menu`: sidebar account trigger, account dropdown, remaining usage, invite referral, settings, and logout behavior.
- `hub-auth-test-bootstrap`: dev/test-only automatic Hub auth bootstrap and packaged-build safety gates.

### Modified Capabilities
- None.

## Impact

- `analytix` renderer startup and shell: `src/renderer/src/App.tsx`, `AppShell.tsx`, `src/renderer/src/account/*`, sidebar components, and startup/login CSS.
- `analytix` shared contracts and IPC: `src/shared/*`, `src/preload/index.ts`, `src/main/ipc/*`.
- `analytix` main process account/model integration: settings normalization, token secret persistence, runtime provider env construction, runtime restart/logout handling.
- `analytix-hub` server API: `/api/desktop/gateway/refresh`, `/api/desktop/console`, `/api/desktop/referral`, and existing auth/gateway behavior.
- Tests and packaging checks: unit tests for startup gating, token secrecy, provider synchronization, dev/test bootstrap gating, and packaged-build refusal paths.
