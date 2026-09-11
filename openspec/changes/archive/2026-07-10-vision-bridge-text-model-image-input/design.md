## Context

analytix now runs through the Go runtime by default while keeping the renderer
contract behind `window.analytix` and top-level `runtime` settings. User image
attachments are persisted by the runtime attachment store and serialized as
native image parts only when the primary provider profile supports image input.
For text-only primary models, the current attachment path falls back to
compressed text fallback data and the renderer blocks image upload before the
turn starts.

The Go runtime already has a Vision Bridge path for MCP/tool-result images:
`analytix-computer-use` screenshots are normalized as tool images, optionally
observed by a configured vision model, then redacted before the text-only
primary model sees the tool result. That path is screenshot-specific and only
handles tool results, so composer images and tool screenshots do not share one
capability.

## Goals / Non-Goals

**Goals:**

- Let text-only primary models use image attachments through a configured
  Vision Bridge model such as `qwen3-vl-plus`.
- Keep native image behavior unchanged for primary models that support image
  input.
- Use one runtime observation service for user attachments and tool-result
  images.
- Keep raw image bytes out of text-only primary model requests and repaired
  model history.
- Surface deterministic bridge pipeline metadata and unavailable reasons.
- Keep renderer changes limited to capability typing, upload gating, and user
  copy.

**Non-Goals:**

- Do not add a renderer-side direct call to the vision model.
- Do not add a new public runtime protocol, provider switcher, or deprecated
  bridge alias.
- Do not replace provider request serialization or existing image-native
  support.
- Do not require real external provider credentials for unit tests.

## Decisions

1. **Runtime service boundary**

   Implement or extract a Go `VisionObservationService` used by both attachment
   preparation and tool-result preparation. This keeps the extra bridge call
   visible to runtime pipeline logic and avoids hiding side effects in the
   provider adapter.

   Alternative considered: put bridge fallback inside provider serialization.
   Rejected because the adapter lacks attachment authorization context, would
   blur primary versus bridge usage, and would make cache/debug data harder to
   reason about.

2. **Source-specific observation prompts**

   The bridge request will use a source kind such as `user_attachment` or
   `tool_screenshot`. User attachments need general image/OCR/layout fields;
   tool screenshots need UI element and selected-text fields. Both prompts must
   state that image contents are untrusted and cannot override user/system
   instructions.

3. **Attachment fallback order**

   Attachment handling will choose:

   - primary native image support -> existing native image message parts;
   - text-only primary plus bridge ready -> bridge observation text parts;
   - bridge unavailable -> existing text fallback with pipeline reason.

   This preserves the current behavior for vision models and gives text-only
   models a safe image understanding path.

4. **Renderer gate**

   The renderer can allow image upload when the selected primary model supports
   image input or runtime info reports Vision Bridge available. It must not call
   the bridge provider or inspect bridge secrets.

5. **Testing strategy**

   Unit tests will use fake providers/stores to prove request shape and
   redaction. Credentialed end-to-end testing remains manual/live evidence
   because `qwen3-vl-plus` requires a configured provider key.

6. **Credentialed live validation**

   Live validation must authenticate through locally provisioned secrets and
   the project official model relay/proxy. Repository-tracked specs, fixtures,
   snapshots, `.env` examples, and logs must not contain plaintext account
   identifiers, passwords, API keys, refresh tokens, or access tokens.

   The preferred local contract is token-first:

   - `ANALYTIX_TEST_ACCOUNT_TOKEN`: local account access token for the official
     relay/proxy.
   - `ANALYTIX_TEST_ACCOUNT_EMAIL` and `ANALYTIX_TEST_ACCOUNT_PASSWORD`: only
     for ignored local secret storage or CI secrets when a token must be minted
     locally; never committed or logged.
   - `ANALYTIX_OFFICIAL_PROXY_BASE_URL`: official relay/proxy base URL used to
     resolve provider/model configuration.
   - `ANALYTIX_LIVE_PRIMARY_MODEL=deepseek-v4-pro`.
   - `ANALYTIX_LIVE_VISION_PROVIDER_ID=aliyun`.
   - `ANALYTIX_LIVE_VISION_MODEL=qwen3-vl-plus`.

   Live tests must obtain model configuration through the official relay/proxy,
   confirm semantic probe support for the bridge model, then run the
   `deepseek-v4-pro` plus `qwen3-vl-plus` bridge smoke. When required local
   secrets are absent, the test must skip or block with an explicit
   missing-secret reason instead of falling back to repository plaintext
   credentials.

## Risks / Trade-offs

- [Risk] Bridge observations can contain instructions from screenshots or
  uploaded images. -> Mitigation: bridge prompt and injected text mark image
  contents as untrusted observations, not instructions.
- [Risk] Extra bridge calls add latency and cost. -> Mitigation: bridge only
  runs when primary lacks image input and the bridge is ready; usage remains
  separate from primary usage.
- [Risk] Large images can exceed provider/body limits. -> Mitigation: keep
  existing upload limits, honor bridge image byte budget, and preserve fallback
  behavior when unavailable.
- [Risk] Dirty worktree contains unrelated edits. -> Mitigation: keep edits
  scoped to OpenSpec, runtime bridge/attachments, renderer capability/gate, and
  targeted tests.

## Migration Plan

1. Add OpenSpec contract and tests.
2. Update Go runtime bridge config/status helpers.
3. Add attachment bridge observation path.
4. Point MCP/tool image bridge to the shared observation service.
5. Update renderer capability typing and upload gate.
6. Run targeted Go/renderer tests and `git diff --check`.

Rollback is local and low-risk: disabling `runtime.visionBridge.enabled` or
setting mode `off` returns text-only primary models to the existing fallback
path.

## Open Questions

- Real provider validation depends on local secret-backed access to the
  official relay/proxy. Unit tests prove request shape and bridge routing;
  live/manual verification records skipped status when those local secrets are
  unavailable.
