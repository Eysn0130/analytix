# Optional Fleet Routing

This reference does not change the main model or effort. The global AGENTS.md
Sol xhigh/max routing applies only in that actual configuration. Astra and other
valid configurations retain native scheduling unless the user explicitly selects
Luna. Named Luna roles retain their configured model/effort; never substitute
Luna for the main agent or force delegation because these roles exist.

## Route for marginal value

The coordinating agent remains the decision owner. Luna is useful when it isolates noisy search or
logs, performs a genuinely independent lane, or produces a narrowly bounded
candidate faster than the coordinating agent can without ownership overlap. If the coordinating agent already has a
clear direct path, use it.

For bounded low-risk constructor, field or call-site changes, use a configured
named Luna Max role when the saved work or context has a clear benefit; direct
work remains appropriate when cheaper. This does not change the main model,
role configuration, ownership rules or required evidence.

The two delegation layers differ:

- A **peer Slice** is a user-visible task with its own implementer and may own the
  canonical writer lease under current peer-task authority.
- A **Luna subagent** is an internal helper of the current coordinating agent. It receives only
  its bounded contract and never becomes Controller or acceptance authority.

Do not allocate agents merely to fill concurrency or satisfy a fixed role
count. Before delegation, state what decision, latency, or context cost the lane
can improve and the evidence that ends it. Stop unused lanes once enough
evidence fixes the next action.

## Controller and Slice routing

The Controller may use:

- `luna_explorer` for one falsifiable repository or upstream question needed to
  choose a denominator, write a brief, or review a risk;
- `luna_verifier` on a `REVIEW_SEALED` candidate when fresh-context challenge
  can change acceptance confidence;
- `luna_worker` when no conflicting unrevoked writer reservation exists and an
  exact-file candidate contract is safer or cheaper than direct Controller work.
  Existing repository-exclusive leases conflict with every other canonical
  writer; only explicitly adopted new scoped briefs allow independent writers.

The Slice implementer may use the same roles within its brief. It integrates every Luna
result, confirms any Luna writer and its processes are inactive, reviews the
diff, and validates evidence binding and runs only necessary additional checks before returning a candidate.

While a Luna Worker owns exact files, the coordinating agent and other agents
cannot write those files or conflicting resources. Under legacy repository-wide
exclusion they remain read-only across the whole checkout; new mutually adopted
scoped leases follow the control protocol's isolation and integration rules.
Internal delegation records the effective writer within its parent's envelope,
not a second overlapping write lease. The parent retains responsibility and
pauses its own writes to those files. Ownership returns only after the Worker
and its task-owned mutation commands are proven inactive. Exact-file and actual
runtime-permission requirements do not change.

Use a verifier before `TERMINAL_FREEZE`, not after it, whenever verifier findings
may require correction. Do not require a verifier for every low-risk candidate;
use one for meaningful independence, risk, uncertainty, or a claim that the coordinating agent
cannot adequately falsify from its current context.

## Role contracts

### `luna_explorer`

Provide the decision context, accepted invariant, one falsifiable question,
bounded read territory, output evidence form, and research stop condition. Let
the Explorer choose the most informative reads and probes inside that contract.
Do not prescribe a transcript or ask it to analyze all of Analytix.

For external research, require original URLs, exact commit/version when
applicable, license/provenance, relevant paths, observed facts, unknowns, and
the no-copy boundary. Search summaries and unpinned default branches are
discovery evidence only.

### `luna_worker`

Provide one candidate outcome, the exact writable files required by repository
rules, accepted invariants, forbidden semantic changes, objective acceptance
evidence, and the return contract. State that the Worker is not alone in the
repository and must preserve other edits.

Let the Worker choose implementation details and useful checks inside those
files. If another file is required, it returns `BLOCKED_SCOPE_EXPANSION`; it
does not stage, commit, publish, authenticate, change authority, or declare
overall completion.

### `luna_verifier`

Provide the sealed candidate diff/paths, claims and invariants to falsify,
relevant public/runtime seams, and required evidence level. Do not provide a
desired verdict or suspected answer. Let the Verifier choose adversarial reads
and checks inside the read-only contract.

It echoes the control identity, review round, `review_barrier_generation`,
`review_barrier_id`, and candidate fingerprint; mismatched or lower-generation
results are ignored. It distinguishes observation, inference, failure, and
unverified coverage. Output remains `CANDIDATE_RESULT`; the coordinating agent classifies findings
and owns acceptance of the evidence under SKILL.md.

## Spawn and runtime rules

- Select the corresponding named Luna agent type and use no history or only the
  minimal recent context. Never use a full-history fork for a named role.
- Do not override the model or reasoning effort fixed by the role.
- A role TOML sandbox is a default; inherited runtime permissions may differ.
  Claim only the behavior or enforcement actually observed.
- On first use, after upgrades, or when behavior appears inconsistent, inspect
  available evidence for role/model/effort identity. Reuse a verified identity
  within an unchanged epoch instead of rechecking every call.
- Prefer native steer/interrupt/stop controls. Still prove commands inactive
  before writer transfer.
- If a named role is unavailable, follow repository fallback rules and report
  the fallback without widening scope or permission.
- Reuse a suitable existing Luna within the same bounded defect/owner lane.
  An independent new owner, contamination by stale hypotheses or a stable final
  review may justify fresh limited context. Let an active high-risk review lane
  finish naturally unless actual safety or invalidated work requires intervention;
  do not replace every reviewer or require a new agent by round count.

## Context and result discipline

Send the bounded question, relevant paths, and accepted invariants, not the
Controller transcript or RC backlog. Keep raw searches and logs in the owning
task; return terse findings with path/command/source pointers, unknowns, and the
next decision they affect.

Parallelize independent read lanes and explicitly adopted nonconflicting scoped
writers. Serialize conflicting files/resources and every writer excluded by a
legacy repository-wide lease; keep shared Git integration serialized. End delegation
when integration and review cost exceeds the context or latency it saves.

When an accepted artifact contract requires the delegated agent itself to write
a formal report, ledger, or review package, a read-only Luna result cannot
substitute for that artifact. Use an authorized writable peer/role and name the
artifact writer in the brief.

## Non-delegable decisions

The coordinating agent retains permissions, authentication and credentials, external writes,
destructive actions, conflict resolution, Git delivery, accepted target or
architecture changes, package/release verdicts, and final completion claims.
