## Context

The current desktop app is an Electron + React + TypeScript app whose renderer talks to main through `window.analytix`. Startup currently derives `needsInitialSetup` from missing model API credentials and opens `InitialSetupDialog`; this blocks the product direction where model access is owned by the Analytix Hub account and gateway.

Analytix Hub already issues `desktopAuth` on login and only issues `gateway` credentials when the user account is ready. Its gateway endpoints require Bearer tokens and `/v1/models` is not anonymously readable. The desktop app therefore needs a Hub account state machine before runtime model requests can safely start.

Analytix_new contains the desired splash/login experience, but it uses Vite/router/localStorage/desktop bridge assumptions that do not match this app. The visual and interaction model should be migrated, while credential storage and runtime wiring must follow the current desktop architecture.

## Goals / Non-Goals

**Goals:**
- Replace ordinary first-run API key setup with Hub account authorization.
- Keep gateway tokens in main-process controlled storage and never persist them in settings or renderer localStorage.
- Synchronize the managed model provider from Hub `/v1/models`.
- Preserve automated test velocity through a dev/test-only Hub bootstrap path.
- Move sidebar settings into a richer account menu with usage and referral actions.
- Add Hub desktop Bearer endpoints needed by the desktop menu and gateway refresh flow.

**Non-Goals:**
- Replacing the bundled runtime architecture.
- Removing advanced/manual provider settings that are still useful for development and diagnostics.
- Adding a production login bypass or embedding shared test credentials.
- Directly copying minified CodexDesktop-Rebuild bundle code.

## Decisions

1. **Main owns Hub credentials**
   - Store desktop auth and gateway token under `app.getPath("userData")/secrets` with restrictive permissions.
   - Renderer receives a redacted account snapshot and invokes IPC methods for login, refresh, logout, usage, referral, and models.
   - Rationale: this prevents token exposure through renderer state, screenshots, settings sync, or localStorage.
   - Alternative considered: port Analytix_new localStorage auth. Rejected because renderer persistence would expose the gateway token.

2. **Hub account state gates runtime startup**
   - Startup shows the splash immediately and preloads app modules while `account.refresh` checks desktopAuth/gateway.
   - Runtime probing and model calls are delayed until gateway is configured or an allowed dev/test bootstrap succeeds.
   - Rationale: packaged builds must not start model traffic before account authorization.
   - Alternative considered: start runtime and let requests fail. Rejected because it creates confusing errors and can leak unauthenticated provider state.

3. **`analytix-hub` is a managed provider**
   - Generate provider metadata from Hub `/v1/models` after login/refresh.
   - Inject the gateway token in main when building runtime provider env.
   - Rationale: Hub is the source of truth for qwen/deepseek/mimo availability and quota policy.
   - Alternative considered: hard-code local qwen/deepseek/mimo presets. Rejected because Hub can enable, disable, or rename models independently.

4. **Desktop Bearer APIs are added to Hub**
   - Add desktop endpoints for gateway refresh, console usage, and referral.
   - Rationale: existing console/referral endpoints require cookie sessions and cannot reliably serve Electron main-process Bearer requests.
   - Alternative considered: renderer cookie login against the website. Rejected because CORS/same-site behavior and token exposure are worse.

5. **Dev/test bootstrap is release-disabled**
   - Bootstrap requires `!app.isPackaged`, `NODE_ENV !== "production"`, and `ANALYTIX_DESKTOP_AUTH_TEST_BOOTSTRAP=1`.
   - Credentials are read only from environment/CI secrets and are not written to source, docs, snapshots, or build artifacts.
   - Rationale: tests can run unattended without creating a production backdoor.

6. **UI migration is visual/behavioral, not dependency migration**
   - Convert the startup splash away from `styled-components` into TSX + CSS.
   - Convert login routing into local component state and current store/IPC calls.
   - Rationale: avoids adding new app-wide dependencies and preserves current renderer architecture.

## Risks / Trade-offs

- Hub endpoint drift or unavailable `/v1/models` -> cache the last redacted model metadata but do not allow model calls without a valid gateway token.
- Test bootstrap leaking into release -> enforce main-process package guards and add packaged-mode tests plus source/dist secret scans.
- Existing provider tests expecting DeepSeek defaults -> update tests to distinguish advanced/manual provider behavior from ordinary Hub-managed startup.
- Analytix_new feature parity gaps -> use an explicit migration checklist for splash, login, register, password reset, challenge, remember login, draft persistence, errors, unverified account state, and logout.
- Cross-repo deployment ordering -> Hub endpoints should be backward-compatible; desktop should surface a clear "Hub desktop API unavailable" error if the server has not yet been deployed.

## Migration Plan

1. Add Hub desktop Bearer endpoints and tests in `/Users/sun/Projects/analytix-hub`.
2. Add desktop shared types, main service, token secret persistence, IPC, and preload account facade.
3. Add startup auth gate and migrated splash/login UI.
4. Wire managed `analytix-hub` provider generation and runtime env token injection.
5. Replace API-key first-run gate with Hub readiness gate.
6. Replace sidebar footer settings button with account menu and usage/referral actions.
7. Add dev/test bootstrap, packaged refusal tests, token secrecy tests, and startup gate tests.
8. Run targeted tests and typecheck. Rollback by disabling the startup account gate and preserving advanced manual provider settings.

## Open Questions

- Whether Hub should return richer model capability metadata beyond OpenAI-compatible `/v1/models`; if not, desktop will infer basic text profiles conservatively.
- Whether offline license import remains a product requirement in current analytix; if no current hook exists, leave it outside the default login path.
