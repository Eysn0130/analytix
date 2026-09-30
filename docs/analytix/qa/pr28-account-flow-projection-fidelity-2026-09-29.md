# Account-flow projection fidelity source checkpoint

Status: Historical verification checkpoint, 2026-09-29 PDT, macOS arm64,
Go 1.26.4. Base HEAD `39250cafa`; the tested candidate is identified by the
three source hashes below. This is a synthetic source seam, not installed,
live-Provider, DuckDB-computation or end-to-end analysis-gain evidence.

## Change and independent truth

The production path is `app/nativecomponent` ->
`ProjectAnalyzeAccountFlowsResultV1` -> provider semantic output. The projection
previously accepted both a distinct counterparty using the subject alias and
the subject reference using another alias. It now refuses either inconsistency
before returning semantic data or a host evidence carrier. The valid self-transfer
and distinct-counterparty controls remain accepted. The existing private
caseentity store remains the identity authority; no wire shape or persistence
format changes.

Three synthetic vectors compare the actual projection and canonical provider
consumer against independent `math/big.Rat` decimal truth: repeated equal-value
independent transactions, recorded equal inflow/outflow with the same name on
distinct accounts, and amounts beyond floating-point exact-integer range.
The test checks exact amounts/counts, identity aliases, distinct evidence refs,
time/currency preservation, and no factual-answer authority. The supplied raw
name, bank, source locator and account canaries do not appear in that semantic JSON.

Truth is populated into the native result before projection, so these vectors
prove validation/projection preservation only. They do not independently prove
DuckDB aggregation, real entity matching, reversal-link recognition, window
boundaries, all output channels, or model correctness/efficiency.

## Candidate and validation

| File under `packages/runtime-go/internal/domain/nativecomponent/` | SHA-256 |
| --- | --- |
| `account_flow_projection_v1.go` | `d4cbe37f98deb27e05df58776c10a2d10f85f5e695ef2a0379065a21a2ad539b` |
| `account_flow_projection_v1_test.go` | `4bd68d55bd948bcf65c4458314202149babb86445c5f60d9d60652de52a33a9a` |
| `account_flow_projection_oracle_v1_test.go` | `dea27e17bf8f9b70c6be9751684c99cf2c9a63ba3a063860da2a10055cef54eb` |

The ordered `path + NUL + sha256 + LF` scope fingerprint is
`274306d8edb79751c4cc2897a2a9d763637f7181055e4a0fcdf1e061edb26094`.
Every command below sourced `./scripts/use-analytix-cache.sh` in the same shell.

| Command from `packages/runtime-go` | Result |
| --- | --- |
| `go test ./internal/domain/nativecomponent -run '^TestAccountFlowProjection(MatchesIndependentDecimalAndIdentityOracle\|BindsSubjectAndCounterpartyAliasIdentity)$' -count=1 -timeout=90s`, before the fix | exit 1: both inconsistent-identity negative cases failed; independent vectors and valid controls passed |
| `go test ./internal/domain/nativecomponent -count=1 -timeout=120s` | pass |
| `go test -tags analytix_prod ./internal/domain/nativecomponent -count=1 -timeout=120s` | pass |
| `go test -race ./internal/domain/nativecomponent -run '^TestAccountFlowProjection(MatchesIndependentDecimalAndIdentityOracle\|BindsSubjectAndCounterpartyAliasIdentity)$' -count=1 -timeout=90s` | pass |
| `go test ./internal/app/nativecomponent -run 'AccountFlow' -count=1 -timeout=120s` | pass |

The initial working-tree tests included inherited, uncommitted B1 files outside
this scope. A fresh `git archive` of committed source
`7ad8351340f4dce08c0756093faf169afba8b7ef` then passed the full domain package
in ordinary/production tags and the app `AccountFlow` selection above. The
immutable extraction is `analytix-projection-7ad835134.NIO7HX` under the configured
cache's temporary storage. This proves those source seams without the dirty B1
dependency; it does not establish current remote CI, main, release or installed
readiness. Cleaning predecessor, Final/recovery and installed acceptance
remain separately routed through the [canonical handover](../handovers/README.md).
