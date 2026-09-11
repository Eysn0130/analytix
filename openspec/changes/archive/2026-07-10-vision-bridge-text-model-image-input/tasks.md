## 1. Runtime Vision Bridge Core

- [x] 1.1 Extend Go Vision Bridge config/status helpers so fallback policy and image budgets are honored.
- [x] 1.2 Add a shared Go observation service for user attachment and tool screenshot source kinds.
- [x] 1.3 Add source-specific strict JSON prompts that mark image contents as untrusted observations.

## 2. User Attachment Bridge Path

- [x] 2.1 Add runtime attachment handling for text-only primary models to call Vision Bridge when ready.
- [x] 2.2 Inject bridge observations as text-only message parts and keep raw images out of primary provider requests.
- [x] 2.3 Record attachment pipeline metadata for native image, bridge observation, and fallback paths.

## 3. Tool Image Bridge Path

- [x] 3.1 Route MCP/tool-result images through the shared observation service.
- [x] 3.2 Support multiple tool images up to the configured bridge budget and record omitted images.
- [x] 3.3 Preserve native image follow-up behavior for primary vision models.

## 4. Renderer Capability and Upload Gate

- [x] 4.1 Add Vision Bridge capability typing to the renderer runtime contract.
- [x] 4.2 Allow composer image upload when selected model supports image input or Vision Bridge is available.
- [x] 4.3 Keep unsupported-image copy clear when neither native image input nor bridge is available.

## 5. Tests and Validation

- [x] 5.1 Add Go tests for attachment bridge, native image bypass, unavailable fallback, multi-image budget, and redaction.
- [x] 5.2 Add/adjust renderer tests for text-only upload gating with and without Vision Bridge.
- [x] 5.3 Run targeted Go and renderer validation plus `git diff --check`; document live credentialed test status.

## 6. Credentialed Live Validation Contract

- [x] 6.1 Document that credentialed live tests must use local secret-backed tokens and official relay/proxy model configuration.
- [x] 6.2 Document that plaintext account identifiers, passwords, API keys, refresh tokens, and access tokens must never be committed or logged.
- [x] 6.3 Specify the local env/secret contract for `deepseek-v4-pro` plus `qwen3-vl-plus` bridge smoke validation.
