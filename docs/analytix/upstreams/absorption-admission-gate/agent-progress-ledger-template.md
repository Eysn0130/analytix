# Agent Progress Ledger Template

Use this template for long-running research rounds or multi-agent audit work.
The ledger is durable state outside runtime prompts.

## Round Metadata

- round_id:
- objective:
- started_at:
- status: active / blocked / complete
- controller:
- final_synthesis_path:

## Scope

- included capability_ids:
- excluded capability_ids:
- source files:
- stop conditions:

## Work Items

| Brief | Owner | Report | Review | Status | Notes |
| --- | --- | --- | --- | --- | --- |
|  |  |  |  |  |  |

## Findings Ledger

| Finding id | Capability | Evidence path | Decision | Follow-up |
| --- | --- | --- | --- | --- |
|  |  |  |  |  |

## Review Ledger

| Review id | Artifact | Verdict | Blocking findings | Resolution |
| --- | --- | --- | --- | --- |
|  |  |  |  |  |

## Blockers

| Blocker | Affected rows | Required unblock evidence | Owner |
| --- | --- | --- | --- |
|  |  |  |  |

## Final Synthesis Checklist

- [ ] All reports have review packages.
- [ ] Blocking findings are resolved or carried as explicit blockers.
- [ ] Matrix updates are owned by the main thread.
- [ ] OpenSpec packet paths are recorded.
- [ ] Full transcripts and broad upstream docs remain out of prompt context.
- [ ] Superpowers installation remains blocked unless plugin admission exists.

