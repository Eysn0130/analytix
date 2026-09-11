## Why

Text-only primary models such as `deepseek-v4-pro` cannot currently use images
from the composer, while `analytix-computer-use` screenshots only work through a
tool-result-specific bridge path. analytix already has provider profiles,
attachment persistence, and a Go vision bridge for tool screenshots, so the
missing piece is a shared runtime bridge that lets text-only models receive safe
image observations without switching the user's primary model.

## What Changes

- Add a runtime-level Vision Bridge image understanding path for user image
  attachments when the selected primary model does not support image input.
- Reuse the same bridge observation service for MCP/tool-result images,
  including `analytix-computer-use` screenshots.
- Keep native image input unchanged for primary models that already support
  images.
- Allow renderer image upload when either the primary model supports image input
  or runtime Vision Bridge is available.
- Record bridge pipeline metadata and unavailable reasons without sending raw
  image payloads to text-only primary models.
- Preserve top-level `runtime` settings and the existing `window.analytix`
  facade; no renderer direct call to the vision model is added.

## Capabilities

### New Capabilities

- `vision-bridge-image-understanding`: Text-only primary models can use
  configured Vision Bridge observations for composer attachments and runtime
  tool images.

### Modified Capabilities

- None.

## Impact

- Renderer: attachment gating and runtime capability typing in
  `src/renderer/src`.
- Main/settings: existing `runtime.visionBridge` resolution and runtime config
  serialization.
- Go runtime: attachment resolution, model-bound message preparation, MCP/tool
  image bridge, provider request construction, redaction, and tests.
- Tests: Go runtime bridge coverage plus renderer upload availability coverage.
