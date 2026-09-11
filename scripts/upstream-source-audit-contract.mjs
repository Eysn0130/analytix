#!/usr/bin/env node

import assert from "node:assert/strict";
import { execFileSync, spawnSync } from "node:child_process";
import { createHash } from "node:crypto";
import { mkdtempSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";

import {
  artifactAdmissionEntries,
  buildArtifactAdmissionProjection,
  classifyPinnedLicenseBytes,
  closeGitEnvironment,
  isPinnedRootLicensePath,
  licenseClassConsistencyProblems,
  normalizeUpstreamArtifactProjection,
  readPinnedGitObject,
  validateManifestReleaseBinding,
  validateLedgerReview,
} from "./upstream-source-audit.mjs";

const SOURCE_COMMIT = "a".repeat(40);
const ANALYTIX_COMMIT = "b".repeat(40);
const SOURCE = { id: "fixture-source", commit: SOURCE_COMMIT };

function marker(review) {
  return `<!-- analytix-upstream-review-v1 ${JSON.stringify(review)} -->`;
}

function intakeReview(overrides = {}) {
  return {
    schemaVersion: 1,
    sourceId: SOURCE.id,
    reviewedCommit: SOURCE.commit,
    parity: "not-proven",
    capabilityBenchmarkV1: null,
    ...overrides,
  };
}

function validate(root, ledgerText) {
  const ledgerPath = join(root, "fixture-source-sync.md");
  writeFileSync(ledgerPath, ledgerText);
  const problems = [];
  validateLedgerReview(SOURCE, ledgerPath, problems, root, ANALYTIX_COMMIT);
  return problems;
}

function expectProblem(problems, fragment) {
  assert.ok(
    problems.some((problem) => problem.includes(fragment)),
    `expected problem containing ${JSON.stringify(fragment)}, got ${JSON.stringify(problems)}`,
  );
}

function repoBinding(path, hash = "d") {
  return {
    path,
    sha256: hash.repeat(64),
    gitBlob: hash.repeat(40),
    readbackMatched: true,
  };
}

function sourceEvidence(id, licenseClass, hash = "e") {
  return {
    id,
    admissionProblems: [],
    license: { class: licenseClass },
    evidenceBindings: {
      manifestEntrySha256: hash.repeat(64),
      ledger: repoBinding(`docs/analytix/upstreams/${id}-sync.md`, hash),
      licenseObject: {
        commit: hash.repeat(40),
        path: licenseClass === "reference-only-unlicensed" ? null : "LICENSE",
        blob: licenseClass === "reference-only-unlicensed" ? null : hash.repeat(40),
        sha256: licenseClass === "reference-only-unlicensed" ? null : hash.repeat(64),
        readbackMatched: true,
        classification: licenseClass === "reference-only-unlicensed"
          ? null
          : {
              status: "recognized",
              licenseId: licenseClass === "permissive-with-notice"
                ? "MIT"
                : "PolyForm-Noncommercial-1.0.0",
              detectedClass: licenseClass,
              contentSha256: hash.repeat(64),
            },
      },
    },
  };
}

function admissionProjection(ledgerText, {
  sources = [sourceEvidence("deepseek-reasonix", "permissive-with-notice")],
  manifestSources = sources.map((source) => ({
    id: source.id,
    license: { class: source.license.class },
  })),
  manifestBinding = repoBinding("docs/analytix/upstreams/upstream-sources.json", "a"),
  provenanceLedgerBinding = repoBinding(
    "docs/analytix/upstreams/code-reuse-provenance.md",
    "b",
  ),
} = {}) {
  return buildArtifactAdmissionProjection({
    manifest: {
      analytixCommit: ANALYTIX_COMMIT,
      sources: manifestSources,
    },
    manifestBinding,
    provenanceLedgerBinding,
    ledgerText,
    sources,
    releaseSource: {
      head: "c".repeat(40),
      tree: "f".repeat(40),
      verified: true,
    },
  });
}

function admissionTable(rows) {
  return `
## Current Review Queue

| Item | Admission scope | Source id | Evidence | Current disposition | Required closure |
| --- | --- | --- | --- | --- | --- |
${rows.join("\n")}

## Review Rule
`;
}

function git(repo, args, options = {}) {
  return execFileSync("git", ["-C", repo, ...args], {
    encoding: "utf8",
    ...options,
  }).trim();
}

const root = mkdtempSync(join(tmpdir(), "analytix-upstream-audit-"));

try {
  const parsedAdmissionTableText = `
## Current Review Queue

| Item | Admission scope | Source id | Evidence | Current disposition | Required closure |
| --- | --- | --- | --- | --- | --- |
| Adopted file | \`artifact-entering\` | \`fixture-source\` | exact destination | \`unverified\` | close lineage |
| Research checkout | \`research-only\` | \`other-source\` | no destination | \`candidate\` | review later |

## Review Rule
`;
  assert.deepEqual(artifactAdmissionEntries(parsedAdmissionTableText), {
    entries: [{
      item: "Adopted file",
      scope: "artifact-entering",
      sourceId: "fixture-source",
      evidence: "exact destination",
      disposition: "unverified",
      requiredClosure: "close lineage",
    }],
    problems: [],
  });
  assert.ok(
    artifactAdmissionEntries("# missing table\n").problems.some((problem) =>
      problem.includes("exactly one Current Review Queue, got 0"),
    ),
  );
  const duplicateQueue = `${admissionTable([
    "| Reasonix port | `artifact-entering` | `deepseek-reasonix` | exact object | `approved` | preserve notice |",
  ])}\n${admissionTable([
    "| Kun-derived product baseline | `artifact-entering` | `kun` | exact baseline | `blocked` | material-specific authorization required |",
  ])}`;
  const duplicateQueueResult = artifactAdmissionEntries(duplicateQueue);
  assert.deepEqual(duplicateQueueResult.entries, []);
  expectProblem(duplicateQueueResult.problems, "exactly one Current Review Queue, got 2");

  const codeSpanItem = artifactAdmissionEntries(admissionTable([
    "| Reasonix-derived `packages/runtime-go/internal/netclient/netclient.go` | `artifact-entering` | `deepseek-reasonix` | exact object | `approved` | preserve notice |",
  ]));
  assert.equal(
    codeSpanItem.entries[0]?.item,
    "Reasonix-derived `packages/runtime-go/internal/netclient/netclient.go`",
  );

  const blockedKun = admissionProjection(
    admissionTable([
      "| Kun-derived product baseline | `artifact-entering` | `kun` | exact baseline | `blocked` | material-specific authorization required |",
    ]),
    {
      sources: [sourceEvidence("kun", "restricted-source-license")],
    },
  );
  assert.equal(blockedKun.ok, false);
  assert.deepEqual(
    blockedKun.entries.map(({ item, sourceId, disposition, status }) => ({
      item,
      sourceId,
      disposition,
      status,
    })),
    [{
      item: "Kun-derived product baseline",
      sourceId: "kun",
      disposition: "blocked",
      status: "blocked",
    }],
  );
  assert.match(blockedKun.projectionSha256, /^[0-9a-f]{64}$/);

  const restrictedApproved = admissionProjection(
    admissionTable([
      "| Kun-derived product baseline | `artifact-entering` | `kun` | exact baseline | `approved` | material-specific authorization required |",
    ]),
    {
      sources: [sourceEvidence("kun", "restricted-source-license")],
    },
  );
  expectProblem(
    restrictedApproved.problems,
    "restricted source approved without an accepted material-specific authorization contract",
  );
  const mislabeledKun = sourceEvidence("kun", "permissive-with-notice");
  const mislabeledClassification = {
    status: "recognized",
    licenseId: "PolyForm-Noncommercial-1.0.0",
    detectedClass: "restricted-source-license",
    contentSha256: mislabeledKun.evidenceBindings.licenseObject.sha256,
  };
  mislabeledKun.admissionProblems = licenseClassConsistencyProblems(
    "permissive-with-notice",
    mislabeledClassification,
  );
  mislabeledKun.evidenceBindings.licenseObject.classification =
    mislabeledClassification;
  const mislabeledApproved = admissionProjection(
    admissionTable([
      "| Kun-derived product baseline | `artifact-entering` | `kun` | exact baseline | `approved` | material-specific authorization required |",
    ]),
    { sources: [mislabeledKun] },
  );
  assert.equal(mislabeledApproved.ok, false);
  expectProblem(
    mislabeledApproved.problems,
    "does not match detected license class restricted-source-license",
  );
  const restrictedRetired = admissionProjection(
    admissionTable([
      "| Kun-derived product baseline | `artifact-entering` | `kun` | replacement claimed without exact contract | `retired` | preserve replacement proof |",
    ]),
    {
      sources: [sourceEvidence("kun", "restricted-source-license")],
    },
  );
  expectProblem(
    restrictedRetired.problems,
    "restricted source retired without an accepted exact replacement evidence contract",
  );

  const permissiveAndRetired = admissionProjection(
    admissionTable([
      "| Reasonix port | `artifact-entering` | `deepseek-reasonix` | exact object | `approved` | preserve MIT notice |",
      "| OpenClaw shim | `artifact-entering` | `openclaw` | exact tag and objects | `approved` | preserve nested MIT notice |",
      "| Retired reference | `artifact-entering` | `retired-source` | exact replacement evidence | `retired` | keep replacement proof |",
      "| Stale research checkout | `research-only` | `unknown-research` | diagnostic only | `candidate` | refresh later |",
    ]),
    {
      sources: [
        sourceEvidence("deepseek-reasonix", "permissive-with-notice", "1"),
        sourceEvidence("openclaw", "permissive-with-notice", "2"),
        sourceEvidence("retired-source", "reference-only-unlicensed", "3"),
      ],
    },
  );
  assert.equal(permissiveAndRetired.ok, true);
  assert.deepEqual(
    permissiveAndRetired.entries.map(({ disposition, status }) => ({ disposition, status })),
    [
      { disposition: "approved", status: "passed" },
      { disposition: "approved", status: "passed" },
      { disposition: "retired", status: "passed" },
    ],
  );

  for (const [name, projection, fragment] of [
    [
      "malformed table",
      admissionProjection("# missing table\n"),
      "exactly one Current Review Queue, got 0",
    ],
    [
      "unknown source",
      admissionProjection(admissionTable([
        "| Unknown source | `artifact-entering` | `unknown` | exact object | `approved` | register source |",
      ])),
      "source is not registered",
    ],
    [
      "unknown disposition",
      admissionProjection(admissionTable([
        "| Unknown disposition | `artifact-entering` | `deepseek-reasonix` | exact object | `waived` | choose a closed disposition |",
      ])),
      "unrecognized disposition",
    ],
    [
      "duplicate entry",
      admissionProjection(admissionTable([
        "| Duplicate | `artifact-entering` | `deepseek-reasonix` | exact object | `approved` | preserve notice |",
        "| Duplicate | `artifact-entering` | `deepseek-reasonix` | exact object | `approved` | preserve notice |",
      ])),
      "duplicate artifact entry",
    ],
    [
      "input readback mismatch",
      admissionProjection(
        admissionTable([
          "| Reasonix port | `artifact-entering` | `deepseek-reasonix` | exact object | `approved` | preserve notice |",
        ]),
        {
          manifestBinding: {
            ...repoBinding("docs/analytix/upstreams/upstream-sources.json", "a"),
            readbackMatched: false,
          },
        },
      ),
      "input hash/readback mismatch",
    ],
    [
      "missing source evidence",
      admissionProjection(
        admissionTable([
          "| Reasonix port | `artifact-entering` | `deepseek-reasonix` | exact object | `approved` | preserve notice |",
        ]),
        {
          sources: [{
            ...sourceEvidence("deepseek-reasonix", "permissive-with-notice"),
            evidenceBindings: null,
          }],
        },
      ),
      "source evidence binding is missing",
    ],
  ]) {
    assert.equal(projection.ok, false, name);
    expectProblem(projection.problems, fragment);
  }

  const releaseRepo = mkdtempSync(join(tmpdir(), "analytix-upstream-release-binding-"));
  try {
    git(releaseRepo, ["init", "-q"]);
    git(releaseRepo, ["config", "user.name", "Analytix Test"]);
    git(releaseRepo, ["config", "user.email", "analytix-test@example.invalid"]);
    writeFileSync(join(releaseRepo, "tracked.txt"), "base\n");
    git(releaseRepo, ["add", "tracked.txt"]);
    git(releaseRepo, ["commit", "-q", "-m", "base"]);
    const manifestCommit = git(releaseRepo, ["rev-parse", "HEAD"]);
    const sideCommit = git(
      releaseRepo,
      ["commit-tree", git(releaseRepo, ["rev-parse", "HEAD^{tree}"])],
      { input: "unreachable\n" },
    );
    writeFileSync(join(releaseRepo, "tracked.txt"), "release\n");
    git(releaseRepo, ["add", "tracked.txt"]);
    git(releaseRepo, ["commit", "-q", "-m", "release"]);
    const releaseHead = git(releaseRepo, ["rev-parse", "HEAD"]);
    const releaseTree = git(releaseRepo, ["rev-parse", "HEAD^{tree}"]);

    assert.equal(validateManifestReleaseBinding({
      repoRoot: releaseRepo,
      declaredAnalytixCommit: manifestCommit,
      releaseHead,
      releaseTree,
    }).ok, true);
    expectProblem(validateManifestReleaseBinding({
      repoRoot: releaseRepo,
      declaredAnalytixCommit: "not-a-commit",
      releaseHead,
      releaseTree,
    }).problems, "must be a full 40-character commit");
    expectProblem(validateManifestReleaseBinding({
      repoRoot: releaseRepo,
      declaredAnalytixCommit: "f".repeat(40),
      releaseHead,
      releaseTree,
    }).problems, "object is unavailable");
    expectProblem(validateManifestReleaseBinding({
      repoRoot: releaseRepo,
      declaredAnalytixCommit: sideCommit,
      releaseHead,
      releaseTree,
    }).problems, "is not an ancestor of release HEAD");
  } finally {
    rmSync(releaseRepo, { recursive: true, force: true });
  }

  const replaceRepo = mkdtempSync(join(tmpdir(), "analytix-upstream-replace-ref-"));
  try {
    git(replaceRepo, ["init", "-q"]);
    git(replaceRepo, ["config", "user.name", "Analytix Test"]);
    git(replaceRepo, ["config", "user.email", "analytix-test@example.invalid"]);
    writeFileSync(join(replaceRepo, "LICENSE"), "original pinned license\n");
    writeFileSync(join(replaceRepo, "ordinary.txt"), "not a license\n");
    git(replaceRepo, ["add", "LICENSE", "ordinary.txt"]);
    git(replaceRepo, ["commit", "-q", "-m", "original"]);
    const originalCommit = git(replaceRepo, ["rev-parse", "HEAD"]);
    const originalSha256 = createHash("sha256")
      .update("original pinned license\n")
      .digest("hex");
    writeFileSync(join(replaceRepo, "LICENSE"), "replacement license\n");
    git(replaceRepo, ["add", "LICENSE"]);
    git(replaceRepo, ["commit", "-q", "-m", "replacement"]);
    const replacementCommit = git(replaceRepo, ["rev-parse", "HEAD"]);
    git(replaceRepo, ["replace", originalCommit, replacementCommit]);
    assert.equal(
      git(replaceRepo, ["show", `${originalCommit}:LICENSE`]),
      "replacement license",
    );
    const priorNoReplace = process.env.GIT_NO_REPLACE_OBJECTS;
    process.env.GIT_NO_REPLACE_OBJECTS = "0";
    try {
      const pinned = readPinnedGitObject(replaceRepo, originalCommit, "LICENSE");
      assert.equal(pinned.ok, true);
      assert.equal(pinned.sha256, originalSha256);
      assert.equal(isPinnedRootLicensePath(replaceRepo, originalCommit, "LICENSE"), true);
      assert.equal(isPinnedRootLicensePath(replaceRepo, originalCommit, "ordinary.txt"), false);
    } finally {
      if (priorNoReplace === undefined) delete process.env.GIT_NO_REPLACE_OBJECTS;
      else process.env.GIT_NO_REPLACE_OBJECTS = priorNoReplace;
    }
  } finally {
    rmSync(replaceRepo, { recursive: true, force: true });
  }

  const knownLicenses = [
    {
      repo: "/Users/sun/Projects/_upstreams/DeepSeek-Reasonix",
      commit: "9eb9511f8b2049a47ee2fef7597256151ac824cb",
      expectedClass: "permissive-with-notice",
      expectedId: "MIT",
    },
    {
      repo: "/Users/sun/Projects/_upstreams/openclaw",
      commit: "50a2481652b6a62d573ece3cead60400dc77020d",
      expectedClass: "permissive-with-notice",
      expectedId: "MIT",
    },
    {
      repo: "/Users/sun/Projects/_upstreams/Kun",
      commit: "f65ac05c0f62d060b7a0cf208ee2941b6ecf41f7",
      expectedClass: "restricted-source-license",
      expectedId: "PolyForm-Noncommercial-1.0.0",
    },
  ];
  for (const license of knownLicenses) {
    const pinned = readPinnedGitObject(license.repo, license.commit, "LICENSE");
    assert.equal(pinned.ok, true);
    const classification = classifyPinnedLicenseBytes(pinned.bytes);
    assert.equal(classification.status, "recognized");
    assert.equal(classification.detectedClass, license.expectedClass);
    assert.equal(classification.licenseId, license.expectedId);
  }
  const kunLicense = readPinnedGitObject(
    "/Users/sun/Projects/_upstreams/Kun",
    "f65ac05c0f62d060b7a0cf208ee2941b6ecf41f7",
    "LICENSE",
  );
  expectProblem(
    licenseClassConsistencyProblems(
      "permissive-with-notice",
      classifyPinnedLicenseBytes(kunLicense.bytes),
    ),
    "does not match detected license class restricted-source-license",
  );
  expectProblem(
    licenseClassConsistencyProblems(
      "permissive-with-notice",
      classifyPinnedLicenseBytes(Buffer.from("unknown license text\n")),
    ),
    "not recognized by the closed classifier",
  );

  assert.deepEqual(validate(root, marker(intakeReview())), []);

  expectProblem(validate(root, "# missing marker\n"), "exactly one");
  expectProblem(
    validate(root, `${marker(intakeReview())}\n${marker(intakeReview())}\n`),
    "got 2",
  );
  expectProblem(
    validate(root, marker(intakeReview({ reviewedCommit: "c".repeat(40) }))),
    "reviewedCommit mismatch",
  );
  expectProblem(
    validate(root, marker(intakeReview({ unexpectedAuthority: true }))),
    "unknown fields",
  );
  expectProblem(
    validate(
      root,
      marker(intakeReview({ parity: "exceeded", capabilityBenchmarkV1: null })),
    ),
    "requires CapabilityBenchmarkV1 evidence",
  );

  const evidencePath = join(root, "capability-benchmark-v1.json");
  const evidenceBytes = '{"schemaVersion":1,"passed":true,"skipped":0}\n';
  writeFileSync(evidencePath, evidenceBytes);
  const evidenceHash = createHash("sha256").update(evidenceBytes).digest("hex");
  const validBenchmark = {
    path: "capability-benchmark-v1.json",
    sha256: evidenceHash,
    analytixCommit: ANALYTIX_COMMIT,
    upstreamCommit: SOURCE_COMMIT,
    testCommand: "go test ./... -run UpstreamCapabilityBenchmarkV1",
    testIds: ["UpstreamCapabilityBenchmarkV1/fixture-source"],
    passed: true,
    skipped: 0,
  };
  assert.deepEqual(
    validate(
      root,
      marker(
        intakeReview({
          parity: "exceeded",
          capabilityBenchmarkV1: validBenchmark,
        }),
      ),
    ),
    [],
  );

  expectProblem(
    validate(
      root,
      marker(
        intakeReview({
          parity: "exceeded",
          capabilityBenchmarkV1: { ...validBenchmark, path: "../outside.json" },
        }),
      ),
    ),
    "path escapes",
  );
  expectProblem(
    validate(
      root,
      marker(
        intakeReview({
          parity: "reached",
          capabilityBenchmarkV1: { ...validBenchmark, skipped: 1 },
        }),
      ),
    ),
    "skipped must be 0",
  );

  const currentAudit = spawnSync(
    process.execPath,
    ["scripts/upstream-source-audit.mjs", "--json"],
    { cwd: process.cwd(), encoding: "utf8", stdio: "pipe" },
  );
  assert.equal(currentAudit.status, 0);
  assert.equal(currentAudit.stderr, "");
  const currentReport = JSON.parse(currentAudit.stdout);
  assert.equal(currentReport.artifactAdmission.ok, false);
  assert.equal(currentReport.researchFreshness.releaseBlocking, false);
  assert.ok(currentReport.artifactAdmission.entries.some((entry) =>
    entry.item === "Kun-derived product baseline" &&
    entry.sourceId === "kun" &&
    entry.disposition === "blocked" &&
    entry.status === "blocked"
  ));
  const exactCurrentItems = currentReport.artifactAdmission.entries.map((entry) => entry.item);
  for (const item of [
    "Reasonix-derived `packages/runtime-go/internal/netclient/netclient.go`",
    "Reasonix-derived `packages/runtime-go/internal/diff/diff.go`",
    "Reasonix-derived `packages/runtime-go/internal/provider/provider.go`",
    "OpenClaw-derived `vendor/openclaw-shim`",
  ]) {
    assert.ok(exactCurrentItems.includes(item), `missing exact artifact item ${item}`);
  }

  const hostileGitEnvironment = {
    PATH: process.env.PATH,
    GIT_NO_REPLACE_OBJECTS: "0",
    GIT_REPLACE_REF_BASE: "refs/replace-bypass",
  };
  closeGitEnvironment(hostileGitEnvironment);
  assert.deepEqual(hostileGitEnvironment, {
    PATH: process.env.PATH,
    GIT_NO_REPLACE_OBJECTS: "1",
    GIT_TERMINAL_PROMPT: "0",
  });

  const malformedGateCases = [null, {}, "invalid"].map((problems) => {
    const state = normalizeUpstreamArtifactProjection({
      entries: [{
        item: "fixture",
        sourceId: "fixture-source",
        disposition: "approved",
        status: "passed",
        problems,
      }],
      problems: [],
    });
    const projectionValid = state.entriesArrayValid &&
      state.entryViews.length === 1 &&
      state.entryViews.every((entry) => entry.structureValid) &&
      state.projectionProblemsValid;
    const gatePassed = projectionValid &&
      state.entryViews.every((entry) =>
        entry.status === "passed" && entry.problems.length === 0
      ) &&
      state.projectionProblems.length === 0;
    return {
      problemShape: problems === null ? "null" : typeof problems,
      projectionValid,
      gatePassed,
      blockers: state.blockers,
    };
  });
  assert.deepEqual(
    malformedGateCases.map((entry) => entry.problemShape),
    ["null", "object", "string"],
  );
  assert.ok(malformedGateCases.every((entry) =>
    entry.projectionValid === false &&
    entry.gatePassed === false &&
    entry.blockers.some((blocker) =>
      blocker.problem === "entry problems must be an array of non-empty strings"
    )
  ));

  console.log("PASS upstream source audit contract");
} finally {
  rmSync(root, { recursive: true, force: true });
}
