# vision-bridge-image-understanding Specification

## Purpose
Define privacy-safe image understanding for the single Analytix Agent,
including the trusted local projection required before any primary or Vision
Bridge provider effect and truthful renderer/live-validation behavior.
## Requirements
### Requirement: Text-only primary models use Vision Bridge for user images
The runtime SHALL use a configured, ready Vision Bridge for a user image only
after a host-owned trusted local component has inspected the exact owned image
and produced a deterministic privacy-safe image derivative or textual
observation. The source image bytes, source-exact URL, data URL, and other
uninspectable image references MUST NOT enter any primary or bridge provider
payload.

#### Scenario: Composer image with text-only primary model
- **WHEN** a user sends an image attachment with a text-only primary model, Vision Bridge is ready, and trusted local privacy projection succeeds
- **THEN** the runtime sends only the verified privacy-safe derivative or textual observation to the bridge and sends only privacy-safe textual observation content to the primary model

#### Scenario: Composer image with native vision primary model
- **WHEN** a user sends an image attachment with a primary model that supports image input and trusted local privacy projection succeeds
- **THEN** the runtime may send only the verified privacy-safe derivative to the primary model and does not send the original image or invoke Vision Bridge in auto mode

#### Scenario: Trusted local image projection is unavailable
- **WHEN** a user sends an image and no trusted local inspector/projector is available or its projection cannot be verified
- **THEN** the runtime makes zero primary or bridge image-provider calls, records a clear image-effect unavailable reason, and continues independent text, code, file, shell, Todo, subagent, and ordinary MCP work in the same Agent/request

#### Scenario: Case authority is unavailable
- **WHEN** a private case attachment would require a protected case-data effect but its current case/snapshot/epoch authority is absent or stale
- **THEN** the runtime rejects only that image effect before local inspection or provider dispatch and does not remove unrelated ordinary capabilities

### Requirement: Runtime tool images share Vision Bridge observations
The runtime SHALL apply exact tool-result ownership and the same trusted local
privacy projection before using Vision Bridge or a native-vision primary model
for image outputs returned by MCP or runtime tools. Raw tool image bytes and
uninspectable references MUST remain outside provider payloads, ordinary
history, SSE, compaction, logs, and UI.

#### Scenario: Computer-use screenshot with text-only primary model
- **WHEN** `analytix-computer-use` returns a screenshot and trusted local privacy projection succeeds
- **THEN** the runtime may inject only the privacy-safe Vision Bridge textual observation into the tool result and redacts the original image bytes before every model dispatch

#### Scenario: Tool image projection is unavailable
- **WHEN** a tool returns an image and trusted local privacy projection is unavailable or fails
- **THEN** the runtime records a typed image-observation unavailable result, makes zero provider image calls, and preserves the tool's independently authorized non-image result fields

#### Scenario: Multiple tool images respect bridge budget
- **WHEN** a tool result includes more images than the configured bridge image budget
- **THEN** the runtime considers only the allowed number for trusted local projection, records the omitted image count, and never sends omitted or unprojected originals to a provider

### Requirement: Renderer allows image upload when Vision Bridge is available
The renderer SHALL allow image attachment selection, paste, and drag/drop only
when runtime capability reports both a ready trusted local privacy projection
path and a compatible downstream native-vision or Vision Bridge path.

#### Scenario: Text-only model with available safe bridge path
- **WHEN** the selected model is text-only and runtime info reports trusted local privacy projection plus Vision Bridge available
- **THEN** the composer accepts image attachments for upload

#### Scenario: Text-only model without complete safe bridge path
- **WHEN** trusted local privacy projection or Vision Bridge is unavailable
- **THEN** the composer prevents image upload and shows a clear unsupported or privacy-projection-unavailable message without disabling text or other Agent capabilities

#### Scenario: Native vision model without trusted local projection
- **WHEN** the selected model supports native image input but trusted local privacy projection is unavailable
- **THEN** the composer does not treat native model support alone as authority to send the original image

### Requirement: Vision Bridge observations are safe model context
Vision Bridge observations SHALL be treated as untrusted image-derived context
and MUST NOT be inserted into the stable system prefix or treated as user/system
instructions. Every primary and bridge request SHALL pass the same host privacy
projection immediately before provider dispatch.

#### Scenario: Observation injection
- **WHEN** the runtime injects a Vision Bridge observation into the primary model request
- **THEN** the injected text marks image content as untrusted observation context and contains no raw base64 image payload, source-exact image reference, or complete protected identifier

#### Scenario: Primary and bridge request redaction
- **WHEN** an original user or tool image is considered for either a primary or bridge provider
- **THEN** provider request capture contains no original `image_url`, `input_image`, `data_base64`, binary `Data`, complete account/card/identity value, or equivalent uninspectable payload

#### Scenario: Privacy projection cannot inspect a part
- **WHEN** the final provider projection encounters an uninspectable image part or reference
- **THEN** it rejects that provider effect before network dispatch rather than treating prior bridge selection, native image support, or attachment ownership as privacy authorization

### Requirement: Credentialed live validation uses local secrets
Credentialed live validation SHALL load account tokens and official relay/proxy
configuration from local secret storage, ignored local environment files, or CI
secrets. It MUST NOT store plaintext account identifiers, passwords, API keys,
refresh tokens, access tokens, source images, or complete protected image
content in repository-tracked specs, fixtures, `.env` examples, snapshots, or
logs.

#### Scenario: Live credentials configured
- **WHEN** `ANALYTIX_TEST_ACCOUNT_TOKEN` or equivalent local secret-backed authentication is available with `ANALYTIX_OFFICIAL_PROXY_BASE_URL` and a trusted local privacy projector is available
- **THEN** the live test resolves model configuration through the official relay/proxy, verifies semantic probe support, runs the text-only primary plus Vision Bridge smoke test using only a privacy-safe derivative, and proves the original image sentinel is absent from every captured provider payload and ordinary surface

#### Scenario: Token must be minted locally
- **WHEN** a live test must mint a relay/proxy token from account credentials
- **THEN** `ANALYTIX_TEST_ACCOUNT_EMAIL` and `ANALYTIX_TEST_ACCOUNT_PASSWORD` are read only from ignored local secret storage or CI secrets and are never emitted to repository files or logs

#### Scenario: Live credentials or trusted local projection missing
- **WHEN** required local secrets, official relay/proxy configuration, or trusted local privacy projection is missing
- **THEN** the live test reports the exact missing-secret or missing-projection block reason and does not fall back to plaintext repository credentials, original image dispatch, or a mock PASS
