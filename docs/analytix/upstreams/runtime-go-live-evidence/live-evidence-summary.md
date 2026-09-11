# Runtime Go Live Evidence Report

Current status: Go runtime default evidence is governed by `runtime:go:release-gate`; `ANALYTIX_RUNTIME_BACKEND=typescript` is a retired-backend diagnostic path, not a runtime rollback.

Generated at: 2026-07-03T12:44:29.110Z
Source commit: e0050d6b5e7791ae3bbed261c7367fc25e6e7fdf
Status: passed
Default backend ready: true
Go default runtime allowed: true
TypeScript default-path fallback retained: false
TypeScript override retired: true
Evidence digest algorithm: sha256:canonical-json-v1

## Final Gate

- Blocked: false
- Blockers: none

## Components

- Provider matrix: passed
- MCP execution: passed
- Packaged desktop QA: passed
- Packaged GUI bridge smoke: passed
- Packaged session soak: passed
- Operator gate: passed

## Provider Settings Coverage

- Settings source: default-app-settings
- Settings loaded: true
- Active provider id: unknown
- Selected provider id: deepseek
- Provider profile count: 2
- Usable provider ids: deepseek, openai-compatible
- Usable non-DeepSeek provider ids: openai-compatible
- Satisfies final provider coverage from settings: true

## Provider Completion Options

- Settings profile path: provider.providers[]
- Required non-DeepSeek settings fields: id or name identifying a non-DeepSeek provider, apiKey, endpointFormat or recognized preset/model profile default, baseUrl or recognized preset default, models[] or runtime.model or recognized preset default
- Recognized preset provider ids: aliyun, aliyun-token-plan, kimi-code, litellm, minimax, minimax-token-plan, moonshot-cn, moonshot-global, opencode-go, tencentcloud, tencentcloud-token-plan, volcengine-coding-plan, xiaomi, xiaomi-token-plan, zai-coding-plan, zhipu-coding-plan
- Accepted non-DeepSeek profile signals: openai-compatible: endpointFormat chat_completions or OpenAI-compatible id/name, anthropic-compatible: endpointFormat messages or Anthropic/Claude id/name, custom-endpoint: endpointFormat custom_endpoint or full endpoint URL
- Env completion groups: openai-compatible (ANALYTIX_RUNTIME_OPENAI_COMPAT_API_KEY, ANALYTIX_RUNTIME_OPENAI_COMPAT_BASE_URL, ANALYTIX_RUNTIME_OPENAI_COMPAT_MODEL); anthropic-compatible (ANALYTIX_RUNTIME_ANTHROPIC_COMPAT_API_KEY, ANALYTIX_RUNTIME_ANTHROPIC_COMPAT_BASE_URL, ANALYTIX_RUNTIME_ANTHROPIC_COMPAT_MODEL); custom-endpoint (ANALYTIX_RUNTIME_CUSTOM_ENDPOINT_API_KEY, ANALYTIX_RUNTIME_CUSTOM_ENDPOINT_URL, ANALYTIX_RUNTIME_CUSTOM_ENDPOINT_MODEL)

## Missing External Inputs

- none

## Next Evidence Commands

- `npm run runtime:go:approval-user-input-evidence -- --json`
- `npm run runtime:go:live-evidence -- --json`
- `npm run runtime:go:local-validation -- --json`
- `npm run runtime:go:packaged-soak -- --json --actual`
- `npm run runtime:go:packaged-gui-smoke -- --json`
- `npm run runtime:go:live-validation -- --json --live-deepseek-cache-from-settings --live-non-deepseek-provider-from-settings --no-gate`
- `npm run runtime:go:preflight`
- `ANALYTIX_RUNTIME_READY=1 ANALYTIX_RUNTIME_GO_OPERATOR_APPROVES_DEFAULT=1 npm run runtime:go:default-readiness-report -- --gate --json`

This report is evidence-only and does not switch the default runtime.
