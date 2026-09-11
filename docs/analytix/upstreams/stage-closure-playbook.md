# analytix stage-closure playbook

Status: Historical execution playbook; superseded for current construction
Applies to: preserved pre-Go-cutover upstream stage procedure
Current authority: repository/scoped `AGENTS.md`, current Git and code,
accepted specs, active authorized OpenSpec, and fresh validation
Superseded by: the controller routing in `../README.md` and the current
Go-only runtime/product architecture

This file preserves the procedure used by older upstream-absorption stages. It
is not a current instruction to create a Codex `/goal`, restore a TypeScript
runtime oracle, or start Go as a shadow. Codex Goal tracking is used only when
the user explicitly requests it. Current work uses one highest scheduling and
acceptance authority, finite dependency-valid Slices, one product writer, and
fresh evidence; historical priorities below do not authorize work or override
the active OpenSpec change.

The historical playbook defined how an upstream absorption thread ran when the
user asked for a complete stage rather than a single small patch.

Authoritative sources remain:

```text
重构升级方案.md
docs/analytix/specs/01-09
docs/analytix/upstreams/*
docs/analytix/benchmarks/*
```

These were the authoritative inputs at the time. The preserved body below does
not override current specs, code, `AGENTS.md`, or OpenSpec.

## 1. Stage Closure Rule

A `/goal` should close a stage. It should not stop after each small item to ask
for a new prompt.

Continue within the same goal until one of these happens:

- the stage has code/doc changes, validation, ledger updates, and a commit or
  explicit no-commit decision;
- a user product decision is required;
- external credentials, a Windows machine, signing/notarization access, or
  release permissions are required;
- validation fails in a way that cannot be safely attributed;
- the proposed change would violate analytix identity, bridge, settings, UI, or
  runtime contract ownership.

## 2. Required Stage Checklist

Every stage starts with:

```text
git status --short --branch
upstream currentness recheck
dirty-worktree review
```

Every stage must classify the relevant upstream changes:

```text
must absorb
should absorb
optional
reject
needs redesign
defer pending evidence
record-only
```

Every stage that touches product UI must prove:

- target Kun version;
- target Kun product position;
- analytix entry level;
- trigger path;
- renderer route or command tests;
- conflict decision when the level changes.

Every stage that touches Reasonix engine/runtime must prove:

- affected runtime contract;
- code-level reuse mode;
- tests or fixtures;
- benchmark or scorecard effect if "stronger" is claimed;
- no Reasonix public protocol or identity leak.

Every stage ends with:

```text
focused tests
required typechecks
git diff --check
identity/protocol scan
Go/Rust scaffold scan
docs/spec/ledger/scorecard updates
commit or explicit no-commit reason
next-stage gate decision
```

## 3. Recommended Sub-Agent Split

Use sub-agents when the stage is broad enough to benefit from parallel review:

| Agent | Scope |
| --- | --- |
| Kun product reviewer | Target-version product entry, route, settings, and workflow parity. |
| Reasonix engine reviewer | Cache/provider/tool/runtime deltas and code-level reuse candidates. |
| Verification reviewer | Tests, benchmark evidence, identity scans, Go/Rust scaffold scans. |
| Go/Rust reviewer | G0-G6 readiness, shadow runtime constraints, helper-only Rust boundary. |

Sub-agents should provide findings and evidence. The main agent remains
responsible for final edits, validation, and commit decisions.

## 4. Superpowers Boundary

Superpowers is optional. It may help structure a plan, TDD checklist, or review,
but it must not become a second source of truth.

Do not block a stage because Superpowers is unavailable. Use Codex `/goal`,
sub-agents, local tests, and the authoritative specs instead.

## 5. Historical Priority Order

Use this order unless a user explicitly changes the priority:

1. Preserve and prove Kun `0.2.13` -> `0.2.14` product baseline.
2. Correct product entry mistakes such as top-level Workflow navigation.
3. Absorb Kun stable/current fixes through analytix product positions.
4. Prove Reasonix cache/provider/tool/runtime improvements through benchmarks.
5. Start Go only as shadow/conformance after TypeScript oracle fixtures are
   stable.
6. Use Rust only as a profiling-backed helper, not as a UI/runtime rewrite.
