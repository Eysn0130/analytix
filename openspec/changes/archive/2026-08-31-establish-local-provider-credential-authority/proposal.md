## Why

Analytix's accepted Hub-first startup and model-provider requirements conflict with the required local Provider and credential authority, leaving implementation without one coherent source of truth. This change supersedes those ordinary-startup semantics while preserving Hub data only as an explicit compatibility and migration input.

## What Changes

- Add an Analytix-local Provider Registry backed by a protected Secret Store, with key-free registry/settings metadata and opaque credential references.
- Keep the existing Electron main/preload/renderer boundary and the single production Go runtime; introduce neither a second runtime nor another credential authority.
- **BREAKING** Replace ordinary Hub login, account-readiness, managed-model, gateway-token, and account-menu startup behavior with local Provider onboarding and settings. Ordinary startup performs no Hub import, instantiation, timer, token read, network request, or gateway fallback.
- Retain legacy Provider, plaintext credential, and Hub state only as lazy compatibility or migration input, with crash-safe migration, rollback, recovery, and no-credential-loss guarantees.
- Route custom Providers, model discovery and routing, OAuth, subscription, media models, CLI, quota, tray, MCP OAuth, extension accounts, and every model consumer through the same Registry and Secret Store authority.
- Define secret-free import/export and cross-device migration, plus observable development and fresh local-nonpublishable package acceptance using isolated profiles and normal protected credential entry.

## Capabilities

### New Capabilities

- `local-provider-credential-authority`: Defines the durable local Provider Registry, protected credential storage, migration and recovery guarantees, unified consumers, and isolated acceptance boundary.

### Modified Capabilities

- `hub-startup-auth`: Replaces Hub authorization as the ordinary startup gate with local Provider setup and makes Hub authentication lazy compatibility-only behavior.
- `hub-gateway-model-provider`: Removes Hub models and gateway credentials as ordinary provider authority or fallback while preserving explicit migration compatibility.
- `hub-account-menu`: Restores the sidebar settings entry as the ordinary footer and removes ordinary Hub-backed account refresh and actions.
- `hub-auth-test-bootstrap`: Replaces automatic Hub credential bootstrap with normal protected local Provider onboarding in isolated development profiles and user-entered credentials in packages.

## Impact

The implementation will span the Secret Store and Provider Registry, settings and migration, the main/preload/renderer local Provider API, onboarding and Provider Settings, runtime provider composition and all credential consumers, import/export, and Hub startup wiring. Public contracts must remain key-free, production execution must remain owned by `packages/runtime-go`, and no real credential value may enter source, artifacts, evidence, settings, renderer state, or public IPC.
