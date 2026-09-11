## ADDED Requirements

### Requirement: Text-only primary models use Vision Bridge for user images
The runtime SHALL use the configured, ready Vision Bridge model to produce
textual observations before calling a selected text-only primary model when a
turn includes user image attachments.

#### Scenario: Composer image with text-only primary model
- **WHEN** a user sends an image attachment with primary model `deepseek-v4-pro` and Vision Bridge is ready with `qwen3-vl-plus`
- **THEN** the runtime calls the bridge model with the image and sends only textual observation content to the primary model

#### Scenario: Composer image with native vision primary model
- **WHEN** a user sends an image attachment with a primary model that supports image input
- **THEN** the runtime sends native image parts to the primary model and does not invoke Vision Bridge in auto mode

#### Scenario: Composer image with unavailable Vision Bridge
- **WHEN** a user sends an image attachment with a text-only primary model and Vision Bridge is disabled or unavailable
- **THEN** the runtime does not send raw image data to the primary model and records a clear fallback or unavailable reason

### Requirement: Runtime tool images share Vision Bridge observations
The runtime SHALL use the same configured, ready Vision Bridge capability for
image outputs returned by MCP or runtime tools when the selected primary model
does not support image input.

#### Scenario: Computer-use screenshot with text-only primary model
- **WHEN** `analytix-computer-use` returns screenshot image content and the primary model is text-only
- **THEN** the runtime injects a Vision Bridge observation into the tool result and redacts raw image bytes before model dispatch

#### Scenario: Multiple tool images respect bridge budget
- **WHEN** a tool result includes more images than the configured bridge image budget
- **THEN** the runtime observes only the allowed number of images and records the omitted image count

### Requirement: Renderer allows image upload when Vision Bridge is available
The renderer SHALL allow image attachment selection, paste, and drag/drop when
either the selected primary model supports image input or runtime Vision Bridge
is available.

#### Scenario: Text-only model with available bridge
- **WHEN** the selected model is text-only and runtime info reports Vision Bridge available
- **THEN** the composer accepts image attachments for upload

#### Scenario: Text-only model without available bridge
- **WHEN** the selected model is text-only and runtime info does not report Vision Bridge available
- **THEN** the composer prevents image upload and shows a clear unsupported-image message

### Requirement: Vision Bridge observations are safe model context
Vision Bridge observations SHALL be treated as untrusted image-derived context
and MUST NOT be inserted into the stable system prefix or treated as user/system
instructions.

#### Scenario: Observation injection
- **WHEN** the runtime injects a Vision Bridge observation into the primary model request
- **THEN** the injected text marks image content as untrusted observation context and contains no raw base64 image payload

#### Scenario: Primary request redaction
- **WHEN** the primary model does not support image input
- **THEN** the provider request and repaired model history contain no `image_url`, `input_image`, or image `data_base64` payload for those images

### Requirement: Credentialed live validation uses local secrets
Credentialed live validation SHALL load account tokens and official relay/proxy
configuration from local secret storage, ignored local environment files, or CI
secrets. It MUST NOT store plaintext account identifiers, passwords, API keys,
refresh tokens, or access tokens in repository-tracked specs, fixtures, `.env`
examples, snapshots, or logs.

#### Scenario: Live credentials configured
- **WHEN** `ANALYTIX_TEST_ACCOUNT_TOKEN` or equivalent local secret-backed authentication is available with `ANALYTIX_OFFICIAL_PROXY_BASE_URL`
- **THEN** the live test resolves model configuration through the official relay/proxy, verifies semantic probe support for `qwen3-vl-plus`, and runs a `deepseek-v4-pro` text-only primary plus `qwen3-vl-plus` Vision Bridge smoke test

#### Scenario: Token must be minted locally
- **WHEN** a live test must mint a relay/proxy token from account credentials
- **THEN** `ANALYTIX_TEST_ACCOUNT_EMAIL` and `ANALYTIX_TEST_ACCOUNT_PASSWORD` are read only from ignored local secret storage or CI secrets and are never emitted to repository files or logs

#### Scenario: Live credentials missing
- **WHEN** required local secrets or official relay/proxy configuration are missing
- **THEN** the live test reports an explicit missing-secret skip or block reason and does not fall back to plaintext repository credentials
