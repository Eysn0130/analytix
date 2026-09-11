# postgres-mcp source and absorption ledger

<!-- analytix-upstream-review-v1 {"schemaVersion":1,"sourceId":"postgres-mcp","reviewedCommit":"07eb329c8c48e49640e0d1b5b35465d4d024c3ee","parity":"not-proven","capabilityBenchmarkV1":null} -->

Status: Current source intake; substantive capability absorption remains open.

## Current source

- Repository: `crystaldba/postgres-mcp`
- Local checkout: `/Users/sun/Projects/_upstreams/postgres-mcp`
- Branch: `main`
- Commit: `07eb329c8c48e49640e0d1b5b35465d4d024c3ee`
- License evidence: `LICENSE` blob
  `49eef5868972bb002014f178b491752cfd50409f` (MIT).
- Reuse boundary: file-level provenance and MIT notice remain required. The
  Goal-thread authorization does not remove notice, vendored-code, generated
  file, or dependency review.

## Role in this Goal

This source is a reference for execution-layer database safety: native SQL
parsing, AST statement/function policy, database-enforced read-only
transactions, parameter binding, connection-error redaction, and deterministic
negative tests. It is not a second Analytix runtime and is not a production MCP
dependency by default.

Current implementation anchors to review are:

- `src/postgres_mcp/sql/safe_sql.py`
- `src/postgres_mcp/sql/sql_driver.py`
- `src/postgres_mcp/sql/bind_params.py`
- `src/postgres_mcp/server.py`
- `tests/unit/sql/test_safe_sql.py`
- `tests/unit/sql/test_readonly_enforcement.py`
- `tests/unit/sql/test_sql_driver.py`

## Pinned white-box review

The restricted driver has two real enforcement layers. It parses SQL with
`pglast`, recursively rejects unknown AST node types and non-allowlisted
functions, blocks select locking clauses and `EXPLAIN ANALYZE`, and then forces
the underlying driver to run inside `BEGIN TRANSACTION READ ONLY`. Its focused
tests cover malformed and multi-statement SQL, transaction statements,
locking, functions, parameter rendering, timeout behavior, and the forced
read-only flag.

Those mechanisms are useful references, but the server is not safe as an
Analytix case-data front door without stronger host policy:

- command-line startup defaults to `unrestricted`; in that mode `execute_sql`
  advertises destructive arbitrary SQL and the base driver does not force a
  read-only transaction;
- the restricted AST allowlist intentionally includes administrative or
  session-mutating statements such as `CREATE EXTENSION`, `VACUUM`, prepared
  statements, and cursors, and permits `hypopg_*` functions. Tests explicitly
  treat `CREATE EXTENSION IF NOT EXISTS hypopg` and hypothetical-index mutation
  as allowed;
- the 30-second limit is an asyncio task timeout. The reviewed path does not
  prove a database `statement_timeout`, protocol cancel acknowledgement, or
  connection quarantine before the timed-out query can continue server-side;
- the driver calls `fetchall()` with no general row or byte ceiling, while only
  selected specialist tools have their own `limit` arguments;
- SQL text and raw database exceptions are logged, and tool failures are
  returned as ordinary `TextContent` beginning with `Error:` rather than an
  MCP `isError`/semantic-failure outcome;
- startup continues after its initial database connection fails. A running MCP
  transport therefore does not establish current source readiness or a
  case-bound dataset snapshot.

The current machine does not have the upstream `pglast` dependency and the
checkout has no prepared virtual environment, so upstream tests were
source-reviewed but not claimed as freshly executed. No `uv` sync, dependency
installation, database container, or upstream worktree mutation was performed.

## Intake decision

| Capability | Decision | Analytix boundary | Proof still required |
| --- | --- | --- | --- |
| Native parser plus AST allowlist | adapt | Keep the parse-tree-deny-by-default pattern, but use Analytix-owned DuckDB parser/binder catalogs and a much smaller SELECT-only statement/function/table policy. Do not port PostgreSQL administrative nodes. | Malicious SQL corpus, macro/table-function/extension escape tests, and deterministic diagnostics. |
| Database-enforced read-only transaction | adapt | Keep the host grant read-only bit authoritative and add engine-native read-only enforcement beneath syntax validation; fail closed when the engine cannot prove the mode. | Side-effect attempts execute zero writes under every entry path. |
| Parameter binding | adopt by contract | Preserve exact account, amount, and date strings without model interpolation or numeric coercion. | Exact-value and injection tests. |
| Connection secret redaction | adapt | Reuse the behavior, not unreviewed regex text, through the Analytix secret/PII projection boundary. | URL, DSN, query, error, log, and SSE redaction tests. |
| Query timeout and result bounds | adapt and strengthen | Enforce host deadline plus engine cancellation acknowledgement, quarantine ambiguous connections, and cap rows and serialized bytes before materialization. | Slow-query, cancellation, post-timeout side-effect, row flood, byte flood, and connection-reuse tests. |
| MCP error/readiness semantics | reject and replace | A live, identity-verified, case-bound health probe is required; transport success and semantic success remain separate typed fields. | Disconnected startup, stale catalog, HTTP success/semantic failure, and spoofed server tests. |
| Index tuning and write-capable administration | reject for the case-analysis front door | These capabilities exceed the required read-only evidence lane and need a separate approved product change. | No advertisement or execution under case grants. |

No row is marked absorbed or exceeded by this completed source audit. OpenSpec
tasks 6.11, 8.17, 8.19, and 8.20 remain the implementation and benchmark
authority.

## Unresolved risks

- PostgreSQL AST semantics differ from DuckDB binder and table-function
  semantics.
- A syntax allowlist alone does not prove read-only execution.
- Async task cancellation alone does not prove that database execution stopped;
  cancelled or ambiguous connections must not be returned to the pool.
- Unlimited `fetchall()` and raw SQL/error logging are incompatible with the
  funds plugin's memory, PII, and prompt-injection boundaries.
- Dependency and generated-file licenses have not yet been admitted for direct
  reuse.
