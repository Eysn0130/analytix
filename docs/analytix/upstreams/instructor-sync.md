# Instructor source and absorption ledger

<!-- analytix-upstream-review-v1 {"schemaVersion":1,"sourceId":"instructor","reviewedCommit":"47fdb2ca07119d389a3c0e8bc28b9930b814f294","parity":"not-proven","capabilityBenchmarkV1":null} -->

Status: Current source intake; substantive capability absorption remains open.

## Current source

- Repository: `567-labs/instructor`
- Local checkout: `/Users/sun/Projects/_upstreams/instructor`
- Branch: `main`
- Commit: `47fdb2ca07119d389a3c0e8bc28b9930b814f294`
- License evidence: `LICENSE` blob
  `f3325f8da4271c8e711369623d138881737177cf` (MIT).
- Reuse boundary: file-level provenance and MIT notice remain required. The
  Goal-thread authorization does not remove notice, generated-code, provider
  SDK, or dependency review.

## Role in this Goal

This source is a reference for schema-first structured responses, typed local
validation, provider adapters, hooks, streaming parsers, and bounded structural
retry. It is not evidence authority: a validated object cannot issue a case
citation, upgrade support, invent a claim, or bypass the Final Evidence Gate.

Current implementation anchors to review are:

- `instructor/v2/core/response_model.py`
- `instructor/v2/core/response.py`
- `instructor/v2/core/retry.py`
- `instructor/v2/core/hooks.py`
- `instructor/v2/providers/openai/schema.py`
- `instructor/v2/providers/openai/handlers.py`
- `instructor/v2/providers/`
- `tests/v2/test_retry_runtime.py`
- `tests/v2/test_response_schema_compat.py`
- `tests/core/test_schema.py`

## Pinned white-box review

The current docs accurately describe Instructor as a schema-first extraction
library, not an agent or evidence authority. The implementation converts
response-model inputs to Pydantic models, dispatches provider-specific request
and response handlers, and validates parsed values locally. Sync and async
retry loops share the same retryable parse-error set and attach attempt
metadata to completion/parse/last-attempt hooks.

The useful mechanisms are concrete, but narrower than the Analytix target:

- integer `max_retries` means that many retries after the initial call, so the
  default `1` permits two provider attempts; a numeric request timeout is also
  used as a Tenacity stop-after-delay bound;
- validation and JSON parsing failures are passed to a provider-specific
  `reask_handler`, which appends the failed assistant/tool output and asks the
  model to correct it;
- hooks can observe complete request kwargs, raw provider responses, parse
  errors, API errors, and last-attempt metadata; handler exceptions are
  isolated as warnings rather than changing the completion result;
- OpenAI structured-output modes can force strict schemas, while the generic
  schema helper preserves the Pydantic schema as supplied and does not itself
  recursively close every object with `additionalProperties:false`;
- successful parsed models retain the raw provider response, and debug paths
  log raw responses plus request kwargs. Credential-name redaction exists, but
  messages, case facts, PII, tool arguments, and provider reasoning are not a
  closed safe-observability projection.

The tests verify retry counts, sync/async behavior, reask invocation, hook
attempt metadata, usage accumulation, provider schema delegation, and broad
Pydantic type compatibility. They do not assert evidence-registry membership,
same-case/turn/epoch/snapshot binding, fact-hash non-increase, reasoning/PII
non-observability, or an atomic Final Evidence Gate. The current machine lacks
the upstream Python dependency set (`tenacity` is unavailable), so these tests
were source-reviewed but not claimed as freshly executed; no dependency or
virtual-environment mutation was made in the upstream checkout.

## Intake decision

| Capability | Decision | Analytix boundary | Proof still required |
| --- | --- | --- | --- |
| Schema-first typed parsing | adapt | Provider output becomes an untrusted proposal parsed into the closed Analytix final-answer union. | Unknown/mixed variants and properties fail closed for every provider family. |
| Validation hooks | adapt | Keep the typed lifecycle and handler isolation, but expose only fixed codes, attempt numbers, counts, booleans, and safe hashes. Hooks receive neither request/response bodies nor receipt-signing/publication authority. | Hook failure cannot publish, persist reasoning/PII, or mutate evidence state. |
| Bounded structure retry | adapt with stricter invariant | At most one repair after local parse failure; compare canonical before/after fact, evidence-id, support, scope, and payload sets. Only structural normalization or removal is allowed. | Fact/support/scope non-increase, no-new-value, and boundary-only fallback tests for every provider family. |
| Raw response attachment and diagnostic logging | reject for high-risk paths | Provider bytes remain attempt-local; only numeric usage/cache fields and closed structural diagnostics may cross the adapter. | No prompt, case fact, full PII, raw response, or reasoning bytes in logs, hooks, history, errors, or exports. |
| Streaming partial objects | reject for high-risk facts | Case facts are not streamed before host acceptance. Ordinary progress may still stream through the existing contract. | Zero pre-acceptance case-fact bytes across SSE/UI/history. |
| LLM-backed validators | reject as authority | They may be advisory only and cannot replace deterministic receipt/claim validation. | Capability matrix records no authority path. |

No row is marked absorbed or exceeded by this completed source audit. OpenSpec
tasks 8.18, 8.19, 8.20, 9.1, 9.2, and 9.3 remain the implementation and
benchmark authority.

## Unresolved risks

- Provider-specific retry messages can accidentally carry untrusted content or
  increase the factual surface.
- Retry exceptions retain failed completions, mutated messages, and create
  kwargs; they cannot be persisted or surfaced directly in Analytix.
- Strict structured output constrains shape, not claim truth, citation
  membership, field equality, source capability, or publication eligibility.
- Streaming parsers and partial models conflict with atomic high-risk
  publication unless explicitly constrained.
- Generated/provider-specific code and dependencies require file-level
  provenance before direct reuse.
- Current Analytix production wiring and deterministic superiority benchmark
  have not yet been demonstrated.
