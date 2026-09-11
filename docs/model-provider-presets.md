# Model Provider Presets

> Status: active design and maintenance guidance. This document defines the
> mechanism; it intentionally does not copy the volatile provider/model
> catalog. Current catalog values come from source and tests.

## Product Paths

analytix has two legitimate provider paths:

1. **Analytix Hub (normal desktop startup).** The startup gate requires an
   authenticated, ready account and configured Hub gateway. Main then
   synchronizes the managed `analytix-hub` profile, selects it for the runtime,
   and obtains its model list from the gateway.
2. **User-managed provider profiles.** Settings lets a user create a custom
   profile or instantiate an editable profile from the built-in preset
   catalog.

These paths share `ModelProviderProfileV1` and the same runtime endpoint
contract. They do not imply multiple runtime implementations or a separate
provider subsystem.

The settings normalizer still creates the default DeepSeek profile for
compatibility with existing settings, migration, and manual provider
configuration. That compatibility default is not the normal fresh-desktop
startup gate: a successful Hub startup explicitly activates
`analytix-hub`.

## Hub-Managed Provider

Hub behavior is dynamic and must not be duplicated in the static preset
catalog:

- `src/shared/hub-account.ts` owns the `analytix-hub` identity and Hub
  contracts.
- `src/renderer/src/account/StartupAuthGate.tsx` owns the startup routing
  condition.
- `src/main/services/hub-account-service.ts` reads gateway models and
  synchronizes the managed profile.
- The persisted managed profile intentionally has an empty `apiKey`.
- `src/main/services/hub-gateway-runtime-secret.ts` makes the gateway token
  available only to the selected Hub runtime request path.

Never persist the Hub gateway token in `AppSettings`, place it in a provider
preset, expose it to the renderer, or copy it into documentation, logs, or
fixtures.

## Static Preset Catalog

The authoritative static catalog is:

```text
src/shared/model-provider-presets.ts
```

`MODEL_PROVIDER_PRESETS` owns the current preset ids, display names, category,
URLs, endpoint formats, models, model profiles, capability metadata,
documentation links, and optional token-plan variants.
`modelProviderPresetProfile()` converts a selected preset into an editable
`ModelProviderProfileV1`.

Do not maintain a second provider or model list in Markdown. Provider endpoints
and model ids change too often, and a hand-copied list can make an obsolete
model look supported. Review the current source and its tests when the exact
catalog matters.

Adding a preset is opt-in: it creates a normal provider profile in
`provider.providers[]`. Users may edit its name, API key, base URL, endpoint
format, models, model profiles, and supported media capabilities subject to the
existing settings schema. A blank custom provider remains supported.

## Contract Ownership

| Concern | Source of truth |
| --- | --- |
| Provider/profile schema | `src/shared/app-settings-types.ts` |
| Defaults, normalization, active profile | `src/shared/app-settings-provider.ts` |
| Static preset catalog and conversion | `src/shared/model-provider-presets.ts` |
| Hub identity and account contracts | `src/shared/hub-account.ts` |
| Hub-managed profile/token lifecycle | `src/main/services/hub-account-service.ts`, `src/main/services/hub-gateway-runtime-secret.ts` |
| Compatible URL construction | `src/shared/openai-compat-url.ts` |
| Settings UI behavior | `src/renderer/src/components/settings-section-providers.tsx` and focused tests |
| Production model execution | applicable consumers in Electron main and `packages/runtime-go` listed by the root `AGENTS.md` |

## Endpoint Rules

- Blank `baseUrl` uses the selected provider default.
- `chat_completions`, `responses`, and `messages` are endpoint families;
  each affects URL, headers, request body, streaming, usage, cache, error, and
  reasoning behavior.
- `custom_endpoint` represents an explicit full endpoint path. Do not append
  a guessed family path.
- Keep URL construction separate from request-body construction.
- A preset-level endpoint format may be overridden by model-profile metadata
  only where the existing schema and runtime support that override.
- Media and plan metadata may include image, speech-to-text, text-to-speech,
  music, video, and a separate token-plan profile. Do not assume every provider
  supports every capability or that plan credentials are interchangeable with
  pay-as-you-go credentials.

## Changing The Catalog

For a provider or model update:

1. Verify the upstream endpoint/protocol and the intended product path.
2. Update `src/shared/model-provider-presets.ts` rather than this document's
   prose.
3. Preserve an explicit full endpoint as `custom_endpoint`; do not turn it
   into an ambiguous base URL.
4. Add or update model profiles only for capabilities the runtime can
   represent and execute.
5. Cover profile conversion, URL behavior, Settings rendering, and affected Go
   provider behavior with focused tests.
6. Check every applicable provider consumer named by the root `AGENTS.md`,
   including write-inline, provider probe, model listing, scheduled-task
   detection, and the Go runtime.

Typical documentation-adjacent validation is:

```bash
npm run test -- src/shared/app-settings-provider.test.ts src/renderer/src/components/settings-section-agents.test.ts
npm run typecheck
git diff --check
```

Add focused Hub service tests when the managed provider path changes, and Go
provider tests when request or response behavior changes.
