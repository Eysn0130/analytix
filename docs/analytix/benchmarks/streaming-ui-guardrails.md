# Streaming UI guardrails

This note records the smooth-streaming boundaries that must stay true as
analytix gains more content types, longer answers, and more provider paths.

## Boundary

The goal is not to copy Kun back into analytix. The durable boundary is:

```text
provider/runtime chunks -> runtime SSE -> renderer active stream store
-> row-level streaming render -> measured row cache / virtualizer
-> rAF scroll anchoring
```

Streaming deltas must not rewrite whole conversation stores, whole side-panel
state, or the root message timeline on every token. Long reasoning and long
final answers must be observed through small row-level subscriptions and
height-bucketed estimates.

## Upstream lessons absorbed

| Source | Lesson | Analytix-owned landing |
| --- | --- | --- |
| Kun | Streaming text is paced and scroll work is rAF-throttled, so bursty chunks do not force one layout pass per token. | `ActiveStreamStore`, row-level live rendering, rAF side/main scroll. |
| DeepSeek-Reasonix | SSE streams open immediately with a comment frame, heartbeat through idle proxy windows, and avoid back-pressure from slow subscribers. | Runtime SSE connected comment, heartbeat, replay cursor, and no per-token Zustand hot path. |
| CodexDesktop-Rebuild | Mature desktop feel comes from preserving bottom distance, avoiding forced history jumps, and separating live output from settled transcript work. | Thread virtualizer measured-row cache, active-stream estimate metrics, and send-before-replay subscription discipline. |

## Required local UI gate

Run the fake-provider browser gate whenever streaming hot paths, markdown
rendering, virtualizer measurement, renderer store shape, runtime SSE, or model
provider parsing changes:

```bash
node scripts/streaming-ui-benchmark.mjs --gate --include-raw --timeout-ms 120000 --out .bench/streaming/<run-id>
```

The default matrix covers:

```text
short answer
reasoning + final answer
long reasoning + long final answer
tool call + final answer
markdown final answer
```

The report records both UI paint cadence and fake provider write cadence. A
passing UI gate means analytix can render a known-good incremental provider
smoothly; it does not prove a real upstream or API proxy is flushing smoothly.

## Proxy/API buffering diagnosis

When users report "it appears in chunks" or "it waits and then prints a lot",
separate provider cadence from renderer cadence before changing UI code.

1. Run the fake-provider UI gate above. If it fails, fix the renderer/runtime
   hot path first.
2. Run the proxy-burst simulation:

```bash
node scripts/streaming-ui-benchmark.mjs --include-proxy-burst --include-raw-provider --timeout-ms 120000 --out .bench/streaming/<run-id>
```

`proxy-burst` intentionally batches many SSE frames into fewer raw writes. It
is diagnostic, not a default release gate. It proves whether the UI remains
responsive after receiving a burst, and it demonstrates the metrics shape of a
buffering proxy.

3. Probe the real provider or intermediary outside Electron/React:

```bash
ANALYTIX_STREAM_PROBE_BASE_URL="https://example.com" \
ANALYTIX_STREAM_PROBE_API_KEY="$API_KEY" \
ANALYTIX_STREAM_PROBE_MODEL="model-name" \
node scripts/model-stream-cadence-probe.mjs --endpoint-format chat_completions --include-raw --out .bench/streaming/provider-cadence.json
```

Use `--url` instead of `--base-url` for a custom full endpoint path. The probe
does not print the API key. It reports sanitized endpoint metadata, raw HTTP
read cadence, SSE frame cadence, first text timing, chunk sizes, and a cadence
classification.

## How to read the result

| Evidence | Likely owner |
| --- | --- |
| Fake-provider UI gate passes, real probe has `provider_or_proxy_batched`. | Provider or intermediary buffers SSE; analytix cannot paint before bytes arrive. |
| Real probe is incremental, but UI paint p95 or max chars per paint fails. | Analytix renderer/runtime regression. |
| Time to headers is high. | Network, auth, routing, or provider queue before stream starts. |
| Time to first raw read/text delta is high while headers are fast. | Model/provider first-token latency or proxy buffering after headers. |
| Max SSE frames per raw read is high. | Intermediary or provider is coalescing frames. |

## Expansion rule

When adding content surfaces, add at least one streaming scenario before
claiming smoothness. Good candidates:

```text
large markdown tables
deep nested lists
long code blocks
mixed tool/reasoning/final answer
image/file references in final text
side conversation inheritance
history prepend while streaming
away-from-bottom streaming
resume/reconnect after partial stream
```

Every new scenario should state the expected hot-path boundary: what may update
per token, what may update per frame, and what must wait until the turn settles.
