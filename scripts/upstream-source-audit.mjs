#!/usr/bin/env node

import { execFileSync } from "node:child_process";
import { createHash } from "node:crypto";
import { existsSync, readFileSync, readdirSync } from "node:fs";
import { dirname, join, relative, resolve, sep } from "node:path";
import { fileURLToPath, pathToFileURL } from "node:url";

const REPO_ROOT = resolve(dirname(fileURLToPath(import.meta.url)), "..");
const MANIFEST_PATH = join(
  REPO_ROOT,
  "docs/analytix/upstreams/upstream-sources.json",
);
const LEDGER_ROOT = join(REPO_ROOT, "docs/analytix/upstreams");
const CODE_REUSE_LEDGER_PATH = join(LEDGER_ROOT, "code-reuse-provenance.md");
const UPSTREAM_ROOT = resolve(
  process.env.ANALYTIX_UPSTREAM_ROOT || "/Users/sun/Projects/_upstreams",
);
const strict = process.argv.includes("--strict");
const strictFreshness = process.argv.includes("--strict-freshness");
const jsonOutput = process.argv.includes("--json");
const HASH_RE = /^[0-9a-f]{40}$/i;
const SHA256_RE = /^[0-9a-f]{64}$/i;
const CLOSED_LICENSE_CONTENTS = new Map([
  [
    "dc024237821ac82056c37f8d82e3be919bd51e39a4529ec12a8ab3e2a346dc4c",
    { licenseId: "MIT", detectedClass: "permissive-with-notice" },
  ],
  [
    "62316704df7426e5a79d2827ff8aca36e9abb3a73b8e68557030749ebefec667",
    { licenseId: "MIT", detectedClass: "permissive-with-notice" },
  ],
  [
    "abd0e95093b7399ac682ec54f45883828ef305703fa1fb982f10a3d488cd085b",
    {
      licenseId: "PolyForm-Noncommercial-1.0.0",
      detectedClass: "restricted-source-license",
    },
  ],
]);
const ARTIFACT_DISPOSITIONS = new Set([
  "approved",
  "retired",
  "unverified",
  "blocked",
  "candidate",
  "rejected",
]);
const LEDGER_REVIEW_RE =
  /<!--\s*analytix-upstream-review-v1\s+(\{[^\r\n]+\})\s*-->/g;
const PARITY_STATES = new Set(["not-proven", "reached", "exceeded"]);
const LEDGER_REVIEW_FIELDS = new Set([
  "schemaVersion",
  "sourceId",
  "reviewedCommit",
  "parity",
  "capabilityBenchmarkV1",
]);
const BENCHMARK_FIELDS = new Set([
  "path",
  "sha256",
  "analytixCommit",
  "upstreamCommit",
  "testCommand",
  "testIds",
  "passed",
  "skipped",
]);
const LICENSE_CLASSES = new Set([
  "permissive-with-notice",
  "restricted-source-license",
  "reference-only-unlicensed",
  "reference-only-proprietary",
  "blocked-provenance",
]);

function singleOption(name) {
  const values = [];
  let malformed = false;
  for (let index = 0; index < process.argv.length; index += 1) {
    const argument = process.argv[index];
    if (argument === name) {
      const value = process.argv[index + 1];
      if (!value || value.startsWith("--")) malformed = true;
      else {
        values.push(value);
        index += 1;
      }
    } else if (argument.startsWith(`${name}=`)) {
      const value = argument.slice(name.length + 1);
      if (!value) malformed = true;
      else values.push(value);
    }
  }
  return {
    provided: values.length > 0,
    value: values[0] || "",
    valid: !malformed && values.length <= 1,
  };
}

const releaseHeadOption = singleOption("--release-head");
const releaseTreeOption = singleOption("--release-tree");

function canonicalJSONString(value) {
  if (value === undefined) return "null";
  if (value === null || typeof value !== "object") return JSON.stringify(value);
  if (Array.isArray(value)) {
    return `[${value.map((item) => canonicalJSONString(item)).join(",")}]`;
  }
  return `{${Object.keys(value)
    .filter((key) => value[key] !== undefined)
    .sort((left, right) => left.localeCompare(right))
    .map((key) => `${JSON.stringify(key)}:${canonicalJSONString(value[key])}`)
    .join(",")}}`;
}

function sha256(value) {
  return createHash("sha256").update(value).digest("hex");
}

export function closeGitEnvironment(environment) {
  for (const name of Object.keys(environment)) {
    if (name.startsWith("GIT_")) delete environment[name];
  }
  environment.GIT_NO_REPLACE_OBJECTS = "1";
  environment.GIT_TERMINAL_PROMPT = "0";
  return environment;
}

const GIT_ENVIRONMENT = Object.freeze(closeGitEnvironment({ ...process.env }));

export function classifyPinnedLicenseBytes(bytes) {
  const contentSha256 = sha256(bytes);
  const classification = CLOSED_LICENSE_CONTENTS.get(contentSha256);
  return classification
    ? {
        status: "recognized",
        licenseId: classification.licenseId,
        detectedClass: classification.detectedClass,
        contentSha256,
      }
    : {
        status: "unrecognized",
        licenseId: null,
        detectedClass: null,
        contentSha256,
      };
}

export function licenseClassConsistencyProblems(declaredClass, classification) {
  if (classification?.status !== "recognized") {
    return ["pinned license content is not recognized by the closed classifier"];
  }
  if (classification.detectedClass !== declaredClass) {
    return [
      `declared license class ${declaredClass || "<missing>"} does not match detected license class ${classification.detectedClass}`,
    ];
  }
  return [];
}

export function normalizeUpstreamArtifactProjection(admission) {
  const value =
    admission && typeof admission === "object" && !Array.isArray(admission)
      ? admission
      : {};
  const entriesArrayValid = Array.isArray(value.entries);
  const rawEntries = entriesArrayValid ? value.entries : [];
  const entryViews = rawEntries.map((entry, index) => {
    const recordValid =
      entry && typeof entry === "object" && !Array.isArray(entry);
    const record = recordValid ? entry : {};
    const problemsValid =
      Array.isArray(record.problems) &&
      record.problems.every(
        (problem) => typeof problem === "string" && problem.length > 0,
      );
    const problems = problemsValid
      ? record.problems
      : ["entry problems must be an array of non-empty strings"];
    const item =
      typeof record.item === "string" && record.item.length > 0
        ? record.item
        : `<entry-${index + 1}>`;
    const sourceId =
      typeof record.sourceId === "string" && record.sourceId.length > 0
        ? record.sourceId
        : "<none>";
    const disposition =
      typeof record.disposition === "string" && record.disposition.length > 0
        ? record.disposition
        : "<none>";
    const status = typeof record.status === "string" ? record.status : "invalid";
    return {
      raw: record,
      item,
      sourceId,
      disposition,
      status,
      problems,
      problemsValid,
      structureValid:
        recordValid &&
        item === record.item &&
        sourceId === record.sourceId &&
        disposition === record.disposition &&
        ["passed", "blocked"].includes(status) &&
        problemsValid,
    };
  });
  const projectionProblemsValid =
    Array.isArray(value.problems) &&
    value.problems.every(
      (problem) => typeof problem === "string" && problem.length > 0,
    );
  const projectionProblems = projectionProblemsValid
    ? value.problems
    : ["artifact admission problems must be an array of non-empty strings"];
  const blockers = entryViews
    .filter(
      (entry) =>
        !entry.structureValid ||
        entry.status !== "passed" ||
        entry.problems.length > 0,
    )
    .flatMap((entry) =>
      (entry.problems.length > 0
        ? entry.problems
        : ["entry projection is malformed"]
      ).map((problem) => ({
        item: entry.item,
        sourceId: entry.sourceId,
        disposition: entry.disposition,
        problem,
      })),
    );
  const projectedEntryProblems = new Set(
    entryViews.flatMap((entry) =>
      entry.problems.map((problem) => `${entry.item}: ${problem}`),
    ),
  );
  blockers.push(
    ...projectionProblems
      .filter((problem) => !projectedEntryProblems.has(problem))
      .map((problem) => ({
        item: "<projection>",
        sourceId: "<none>",
        disposition: "<none>",
        problem,
      })),
  );
  return {
    rawEntries,
    entryViews,
    entriesArrayValid,
    projectionProblems,
    projectionProblemsValid,
    blockers,
  };
}

function git(repoPath, args) {
  try {
    return {
      ok: true,
      value: execFileSync("git", ["-C", repoPath, ...args], {
        cwd: REPO_ROOT,
        env: GIT_ENVIRONMENT,
        encoding: "utf8",
        stdio: ["ignore", "pipe", "pipe"],
      }).trim(),
    };
  } catch (error) {
    return {
      ok: false,
      value: "",
      error: String(error?.stderr || error?.message || error).trim(),
    };
  }
}

function gitBytes(repoPath, args) {
  try {
    return {
      ok: true,
      value: execFileSync("git", ["-C", repoPath, ...args], {
        cwd: REPO_ROOT,
        env: GIT_ENVIRONMENT,
        stdio: ["ignore", "pipe", "pipe"],
        maxBuffer: 16 * 1024 * 1024,
      }),
    };
  } catch (error) {
    return {
      ok: false,
      value: Buffer.alloc(0),
      error: String(error?.stderr || error?.message || error).trim(),
    };
  }
}

export function readPinnedGitObject(repoPath, commit, path) {
  const evidence = git(repoPath, ["cat-file", "-e", `${commit}:${path}`]);
  const beforeBlob = git(repoPath, ["rev-parse", `${commit}:${path}`]);
  const firstRead = gitBytes(repoPath, ["show", `${commit}:${path}`]);
  const secondRead = gitBytes(repoPath, ["show", `${commit}:${path}`]);
  const afterBlob = git(repoPath, ["rev-parse", `${commit}:${path}`]);
  const firstHash = firstRead.ok ? sha256(firstRead.value) : "";
  const secondHash = secondRead.ok ? sha256(secondRead.value) : "";
  const readbackMatched =
    evidence.ok &&
    beforeBlob.ok &&
    afterBlob.ok &&
    HASH_RE.test(beforeBlob.value) &&
    beforeBlob.value === afterBlob.value &&
    firstRead.ok &&
    secondRead.ok &&
    firstHash === secondHash;
  return {
    ok: readbackMatched,
    commit: String(commit || ""),
    path: String(path || ""),
    blob: beforeBlob.value,
    bytes: firstRead.value,
    sha256: firstHash,
    readbackMatched,
  };
}

function normalizedRepoPath(repoRoot, path) {
  const value = String(path || "").trim().replaceAll("\\", "/");
  const resolvedPath = resolve(repoRoot, value);
  const normalized = relative(repoRoot, resolvedPath).replaceAll("\\", "/");
  if (
    !value ||
    value.startsWith("/") ||
    value.includes(":") ||
    normalized === ".." ||
    normalized.startsWith("../") ||
    normalized !== value
  ) {
    return "";
  }
  return normalized;
}

function repoFileBinding(record) {
  return {
    path: record?.path || "",
    sha256: record?.sha256 || "",
    gitBlob: record?.gitBlob || "",
    inputMode: record?.inputMode || "",
    readbackMatched: record?.readbackMatched === true,
  };
}

function createRepoInput({ repoRoot, releaseHead = "" }) {
  const releaseMode = Boolean(releaseHead);
  const records = new Map();
  const problems = [];

  function read(path) {
    const normalized = normalizedRepoPath(repoRoot, path);
    if (!normalized) {
      return {
        ok: false,
        path: String(path || ""),
        bytes: Buffer.alloc(0),
        sha256: "",
        gitBlob: "",
        inputMode: releaseMode ? "release-git-object" : "working-tree",
        readbackMatched: false,
        problem: `repository input path is invalid: ${String(path || "<missing>")}`,
      };
    }
    if (records.has(normalized)) return records.get(normalized);

    let record;
    if (releaseMode) {
      const beforeBlob = git(repoRoot, ["rev-parse", `${releaseHead}:${normalized}`]);
      const firstRead = gitBytes(repoRoot, ["show", `${releaseHead}:${normalized}`]);
      const secondRead = gitBytes(repoRoot, ["show", `${releaseHead}:${normalized}`]);
      const afterBlob = git(repoRoot, ["rev-parse", `${releaseHead}:${normalized}`]);
      const firstHash = firstRead.ok ? sha256(firstRead.value) : "";
      const secondHash = secondRead.ok ? sha256(secondRead.value) : "";
      const readbackMatched =
        beforeBlob.ok &&
        afterBlob.ok &&
        HASH_RE.test(beforeBlob.value) &&
        beforeBlob.value === afterBlob.value &&
        firstRead.ok &&
        secondRead.ok &&
        firstHash === secondHash;
      record = {
        ok: readbackMatched,
        path: normalized,
        bytes: firstRead.value,
        sha256: firstHash,
        gitBlob: beforeBlob.value,
        inputMode: "release-git-object",
        readbackMatched,
        problem: readbackMatched
          ? ""
          : `release Git-object input hash/readback mismatch: ${normalized}`,
      };
    } else {
      const absolutePath = resolve(repoRoot, normalized);
      try {
        const firstRead = readFileSync(absolutePath);
        const secondRead = readFileSync(absolutePath);
        const firstHash = sha256(firstRead);
        const secondHash = sha256(secondRead);
        const blob = git(repoRoot, ["rev-parse", `HEAD:${normalized}`]);
        record = {
          ok: firstHash === secondHash,
          path: normalized,
          bytes: firstRead,
          sha256: firstHash,
          gitBlob: blob.ok ? blob.value : "",
          inputMode: "working-tree",
          readbackMatched: firstHash === secondHash,
          problem:
            firstHash === secondHash
              ? ""
              : `working-tree input hash/readback mismatch: ${normalized}`,
        };
      } catch {
        record = {
          ok: false,
          path: normalized,
          bytes: Buffer.alloc(0),
          sha256: "",
          gitBlob: "",
          inputMode: "working-tree",
          readbackMatched: false,
          problem: `repository input is missing or unreadable: ${normalized}`,
        };
      }
    }
    records.set(normalized, record);
    if (!record.ok) problems.push(record.problem);
    return record;
  }

  function revalidate() {
    if (!releaseMode) return problems;
    for (const [path, record] of records.entries()) {
      const blob = git(repoRoot, ["rev-parse", `${releaseHead}:${path}`]);
      const readback = gitBytes(repoRoot, ["show", `${releaseHead}:${path}`]);
      const matched =
        record.ok &&
        blob.ok &&
        blob.value === record.gitBlob &&
        readback.ok &&
        sha256(readback.value) === record.sha256;
      if (!matched) {
        record.ok = false;
        record.readbackMatched = false;
        const problem = `release Git-object input changed during final readback: ${path}`;
        if (!problems.includes(problem)) problems.push(problem);
      }
    }
    return problems;
  }

  return { read, revalidate, problems, releaseMode };
}

export function validateManifestReleaseBinding({
  repoRoot,
  declaredAnalytixCommit,
  releaseHead,
  releaseTree,
}) {
  const problems = [];
  const releaseCommit = git(repoRoot, ["cat-file", "-e", `${releaseHead}^{commit}`]);
  const actualReleaseTree = git(repoRoot, ["rev-parse", `${releaseHead}^{tree}`]);
  if (!HASH_RE.test(String(releaseHead || "")) || !releaseCommit.ok) {
    problems.push("release HEAD must be an existing full 40-character commit");
  }
  if (
    !HASH_RE.test(String(releaseTree || "")) ||
    !actualReleaseTree.ok ||
    actualReleaseTree.value !== releaseTree
  ) {
    problems.push("release tree must match the exact release HEAD tree");
  }
  if (!HASH_RE.test(String(declaredAnalytixCommit || ""))) {
    problems.push("manifest analytixCommit must be a full 40-character commit");
  } else {
    const declaredCommit = git(repoRoot, [
      "cat-file",
      "-e",
      `${declaredAnalytixCommit}^{commit}`,
    ]);
    if (!declaredCommit.ok) {
      problems.push("manifest analytixCommit object is unavailable");
    } else if (HASH_RE.test(String(releaseHead || ""))) {
      const ancestor = git(repoRoot, [
        "merge-base",
        "--is-ancestor",
        declaredAnalytixCommit,
        releaseHead,
      ]);
      if (!ancestor.ok) {
        problems.push("manifest analytixCommit is not an ancestor of release HEAD");
      }
    }
  }
  return {
    ok: problems.length === 0,
    declaredAnalytixCommit: String(declaredAnalytixCommit || ""),
    releaseHead: String(releaseHead || ""),
    releaseTree: String(releaseTree || ""),
    problems,
  };
}

function normalizeRemote(value) {
  return String(value || "")
    .trim()
    .replace(/\.git$/i, "")
    .replace(/\/$/, "");
}

function discoverRepositories(upstreamRoot = UPSTREAM_ROOT) {
  if (!existsSync(upstreamRoot)) return [];
  return readdirSync(upstreamRoot, { withFileTypes: true })
    .filter(
      (entry) =>
        entry.isDirectory() &&
        existsSync(join(upstreamRoot, entry.name, ".git")),
    )
    .map((entry) => entry.name)
    .sort();
}

function rootLicensePaths(repoPath, commit) {
  const listing = git(repoPath, ["ls-tree", "--name-only", commit]);
  if (!listing.ok) return [];
  return listing.value
    .split(/\r?\n/)
    .filter((name) => /^(?:LICENSE|COPYING|NOTICE)(?:\.[^/]+)?$/i.test(name))
    .sort();
}

export function isPinnedRootLicensePath(repoPath, commit, path) {
  return rootLicensePaths(repoPath, commit).includes(path);
}

function unknownFields(value, allowed) {
  if (!value || typeof value !== "object" || Array.isArray(value)) return [];
  return Object.keys(value)
    .filter((field) => !allowed.has(field))
    .sort();
}

function sha256File(path) {
  return createHash("sha256").update(readFileSync(path)).digest("hex");
}

export function validateLedgerReview(
  source,
  ledgerPath,
  problems,
  repoRoot = REPO_ROOT,
  analytixCommit = "",
  options = {},
) {
  let ledger;
  if (typeof options.ledgerText === "string") {
    ledger = options.ledgerText;
  } else {
    if (!existsSync(ledgerPath)) return null;
    ledger = readFileSync(ledgerPath, "utf8");
  }
  const markers = [...ledger.matchAll(LEDGER_REVIEW_RE)];
  if (markers.length !== 1) {
    problems.push(
      `ledger must contain exactly one analytix-upstream-review-v1 marker, got ${markers.length}`,
    );
    return null;
  }

  let review;
  try {
    review = JSON.parse(markers[0][1]);
  } catch {
    problems.push("ledger review marker is not valid JSON");
    return null;
  }

  const extraReviewFields = unknownFields(review, LEDGER_REVIEW_FIELDS);
  if (extraReviewFields.length > 0) {
    problems.push(
      `ledger review has unknown fields: ${extraReviewFields.join(", ")}`,
    );
  }

  if (review.schemaVersion !== 1)
    problems.push("ledger review schemaVersion must be 1");
  if (review.sourceId !== source.id) {
    problems.push(
      `ledger review sourceId mismatch: expected ${source.id}, got ${review.sourceId || "<missing>"}`,
    );
  }
  if (!HASH_RE.test(review.reviewedCommit || "")) {
    problems.push("ledger reviewedCommit is not a full hash");
  } else if (review.reviewedCommit !== source.commit) {
    problems.push(
      `ledger reviewedCommit mismatch: expected ${source.commit}, got ${review.reviewedCommit}`,
    );
  }
  if (!PARITY_STATES.has(review.parity)) {
    problems.push(`ledger parity is invalid: ${review.parity || "<missing>"}`);
  }

  const benchmark = review.capabilityBenchmarkV1;
  if (review.parity === "not-proven") {
    if (benchmark !== null) {
      problems.push(
        "not-proven ledger parity must set capabilityBenchmarkV1 to null",
      );
    }
  } else {
    if (
      !benchmark ||
      typeof benchmark !== "object" ||
      Array.isArray(benchmark)
    ) {
      problems.push(
        `${review.parity} ledger parity requires CapabilityBenchmarkV1 evidence`,
      );
    } else {
      const extraBenchmarkFields = unknownFields(benchmark, BENCHMARK_FIELDS);
      if (extraBenchmarkFields.length > 0) {
        problems.push(
          `CapabilityBenchmarkV1 has unknown fields: ${extraBenchmarkFields.join(", ")}`,
        );
      }
      const benchmarkPath = String(benchmark.path || "").trim();
      const resolvedBenchmarkPath = resolve(repoRoot, benchmarkPath);
      if (!resolvedBenchmarkPath.startsWith(`${resolve(repoRoot)}${sep}`)) {
        problems.push(
          "CapabilityBenchmarkV1 evidence path escapes the repository",
        );
      } else if (!benchmarkPath) {
        problems.push(
          `CapabilityBenchmarkV1 evidence is missing: ${benchmarkPath || "<missing>"}`,
        );
      } else if (!SHA256_RE.test(String(benchmark.sha256 || ""))) {
        problems.push("CapabilityBenchmarkV1 sha256 must be a full hash");
      } else if (typeof options.readRepoFile === "function") {
        const evidence = options.readRepoFile(benchmarkPath);
        if (!evidence?.ok) {
          problems.push(`CapabilityBenchmarkV1 evidence is missing: ${benchmarkPath}`);
        } else if (evidence.sha256 !== benchmark.sha256) {
          problems.push(
            "CapabilityBenchmarkV1 sha256 does not match the evidence file",
          );
        }
      } else {
        if (!existsSync(resolvedBenchmarkPath)) {
          problems.push(`CapabilityBenchmarkV1 evidence is missing: ${benchmarkPath}`);
        } else if (sha256File(resolvedBenchmarkPath) !== benchmark.sha256) {
          problems.push(
            "CapabilityBenchmarkV1 sha256 does not match the evidence file",
          );
        }
      }
      if (!HASH_RE.test(String(benchmark.analytixCommit || ""))) {
        problems.push(
          "CapabilityBenchmarkV1 analytixCommit must be a full hash",
        );
      } else if (
        analytixCommit &&
        benchmark.analytixCommit !== analytixCommit
      ) {
        problems.push(
          "CapabilityBenchmarkV1 analytixCommit must match the source manifest",
        );
      }
      if (benchmark.upstreamCommit !== source.commit) {
        problems.push(
          "CapabilityBenchmarkV1 upstreamCommit must match the reviewed source commit",
        );
      }
      if (!String(benchmark.testCommand || "").trim()) {
        problems.push("CapabilityBenchmarkV1 testCommand is missing");
      }
      if (
        !Array.isArray(benchmark.testIds) ||
        benchmark.testIds.length === 0 ||
        benchmark.testIds.some((testId) => !String(testId || "").trim())
      ) {
        problems.push("CapabilityBenchmarkV1 testIds are missing");
      }
      if (benchmark.passed !== true)
        problems.push("CapabilityBenchmarkV1 passed must be true");
      if (benchmark.skipped !== 0)
        problems.push("CapabilityBenchmarkV1 skipped must be 0");
    }
  }

  return review;
}

function validateSource(
  source,
  analytixCommit,
  {
    repoRoot = REPO_ROOT,
    upstreamRoot = UPSTREAM_ROOT,
    repoInput = createRepoInput({ repoRoot }),
  } = {},
) {
  const repoPath = join(upstreamRoot, String(source?.directory || ""));
  const ledgerPath = resolve(repoRoot, String(source?.ledger || ""));
  const problems = [];
  const warnings = [];
  const ledgerRecord = repoInput.read(String(source?.ledger || ""));
  const evidenceBindings = {
    manifestEntrySha256: sha256(canonicalJSONString(source)),
    ledger: repoFileBinding(ledgerRecord),
    licenseObject: {
      commit: String(source?.commit || ""),
      path: source?.license?.path ?? null,
      blob: source?.license?.blob ?? null,
      sha256: null,
      readbackMatched: false,
      classification: null,
    },
  };
  if (!ledgerRecord.ok) {
    problems.push(
      `ledger input is missing or unstable: ${source?.ledger || "<missing>"}`,
    );
  }
  if (!existsSync(join(repoPath, ".git"))) {
    return {
      id: source?.id,
      directory: source?.directory,
      repoPath,
      ok: false,
      problems: [...problems, "repository is unavailable"],
      admissionProblems: [
        ...problems,
        "pinned provenance object is unavailable",
      ],
      freshnessProblems: ["repository is unavailable"],
      evidenceBindings,
      warnings,
    };
  }

  const head = git(repoPath, ["rev-parse", "HEAD"]);
  const branch = git(repoPath, ["branch", "--show-current"]);
  const remote = git(repoPath, ["remote", "get-url", "origin"]);
  const trackingHead = git(repoPath, ["rev-parse", "@{upstream}"]);
  const aheadBehind = git(repoPath, [
    "rev-list",
    "--left-right",
    "--count",
    "HEAD...@{upstream}",
  ]);
  const status = git(repoPath, ["status", "--porcelain"]);
  const dirtyEntries =
    status.ok && status.value
      ? status.value.split(/\r?\n/).filter(Boolean)
      : [];

  if (!HASH_RE.test(source.commit))
    problems.push("manifest commit is not a full hash");
  const pinnedCommit = git(repoPath, ["cat-file", "-e", `${source.commit}^{commit}`]);
  if (!pinnedCommit.ok) {
    problems.push("pinned commit object is unavailable");
  }
  if (!head.ok || head.value !== source.commit) {
    problems.push(
      `HEAD mismatch: expected ${source.commit}, got ${head.value || "<unavailable>"}`,
    );
  }
  if (!branch.ok || branch.value !== source.branch) {
    problems.push(
      `branch mismatch: expected ${source.branch}, got ${branch.value || "<unavailable>"}`,
    );
  }
  if (
    !remote.ok ||
    normalizeRemote(remote.value) !== normalizeRemote(source.remote)
  ) {
    problems.push(
      `remote mismatch: expected ${source.remote}, got ${remote.value || "<unavailable>"}`,
    );
  }
  if (!trackingHead.ok || trackingHead.value !== source.commit) {
    problems.push(
      `tracking HEAD mismatch: expected ${source.commit}, got ${trackingHead.value || "<unavailable>"}`,
    );
  }
  if (!aheadBehind.ok || !/^0\s+0$/.test(aheadBehind.value)) {
    problems.push(
      `checkout is not synchronized with its tracking branch: ${aheadBehind.value || "<unavailable>"}`,
    );
  }
  const ledgerRoot = join(repoRoot, "docs/analytix/upstreams");
  const ledgerPathAllowed =
    ledgerPath.startsWith(`${ledgerRoot}${sep}`) && ledgerPath.endsWith("-sync.md");
  if (!ledgerPathAllowed) {
    problems.push(
      `ledger path is outside the admitted sync-ledger boundary: ${source.ledger}`,
    );
  } else if (!ledgerRecord.ok) {
    problems.push(`ledger is missing: ${source.ledger}`);
  }
  const ledgerReview = ledgerPathAllowed
    ? validateLedgerReview(
        source,
        ledgerPath,
        problems,
        repoRoot,
        analytixCommit,
        {
          ledgerText: ledgerRecord.ok ? ledgerRecord.bytes.toString("utf8") : "",
          readRepoFile: repoInput.read,
        },
      )
    : null;
  if (!LICENSE_CLASSES.has(source.license?.class)) {
    problems.push(
      `unsupported license class: ${source.license?.class || "<missing>"}`,
    );
  }
  if (!String(source.license?.reusePolicy || "").trim()) {
    problems.push("reuse policy is missing");
  }

  const licensePath = source.license?.path;
  const trackedRootLicenses = rootLicensePaths(repoPath, source.commit);
  let licenseClassification = null;
  if (licensePath) {
    if (!isPinnedRootLicensePath(repoPath, source.commit, licensePath)) {
      problems.push(
        `license path is not a root LICENSE/COPYING/NOTICE object: ${licensePath}`,
      );
    }
    const pinnedLicense = readPinnedGitObject(repoPath, source.commit, licensePath);
    if (!pinnedLicense.ok)
      problems.push(`license path is not tracked at commit: ${licensePath}`);
    if (!pinnedLicense.ok || pinnedLicense.blob !== source.license.blob) {
      problems.push(
        `license blob mismatch: expected ${source.license.blob || "<missing>"}, got ${pinnedLicense.blob || "<unavailable>"}`,
      );
    }
    licenseClassification = classifyPinnedLicenseBytes(pinnedLicense.bytes);
    problems.push(
      ...licenseClassConsistencyProblems(
        source.license?.class,
        licenseClassification,
      ),
    );
    const licenseReadbackMatched =
      pinnedLicense.ok && pinnedLicense.blob === source.license.blob;
    evidenceBindings.licenseObject = {
      commit: source.commit,
      path: licensePath,
      blob: source.license?.blob ?? null,
      sha256: pinnedLicense.sha256,
      readbackMatched: licenseReadbackMatched,
      classification: licenseClassification,
    };
    if (!licenseReadbackMatched) {
      problems.push("license Git object hash/readback mismatch");
    }
  } else {
    if (
      new Set(["permissive-with-notice", "restricted-source-license"])
        .has(source.license?.class)
    ) {
      problems.push(
        `declared license class ${source.license.class} requires pinned root license content`,
      );
    }
    if (source.license?.blob !== null)
      problems.push("license blob must be null when path is null");
    if (trackedRootLicenses.length > 0) {
      problems.push(
        `manifest omits tracked root license evidence: ${trackedRootLicenses.join(", ")}`,
      );
    }
    evidenceBindings.licenseObject = {
      commit: source.commit,
      path: null,
      blob: null,
      sha256: null,
      readbackMatched:
        HASH_RE.test(String(source.commit || "")) &&
        pinnedCommit.ok &&
        trackedRootLicenses.length === 0,
      classification: null,
    };
  }

  if (dirtyEntries.length > 0) {
    warnings.push(
      `worktree has ${dirtyEntries.length} local entries; tracked evidence was read from Git objects`,
    );
  }

  return {
    id: source.id,
    directory: source.directory,
    repoPath,
    branch: branch.value,
    head: head.value,
    remote: remote.value,
    trackingHead: trackingHead.value,
    aheadBehind: aheadBehind.value,
    ledger: source.ledger,
    ledgerReview,
    license: {
      path: licensePath,
      blob: source.license?.blob ?? null,
      name: source.license?.name,
      class: source.license?.class,
      detectedClass: licenseClassification?.detectedClass ?? null,
      detectedLicenseId: licenseClassification?.licenseId ?? null,
      reusePolicy: source.license?.reusePolicy,
      trackedRootLicenses,
    },
    worktreeDirtyEntries: dirtyEntries.length,
    worktreePreview: dirtyEntries.slice(0, 8),
    evidenceBindings,
    ok: problems.length === 0,
    problems,
    admissionProblems: problems.filter(
      (problem) => !/^(HEAD mismatch|branch mismatch|remote mismatch|tracking HEAD mismatch|checkout is not synchronized)/.test(problem),
    ),
    freshnessProblems: problems.filter((problem) =>
      /^(HEAD mismatch|branch mismatch|remote mismatch|tracking HEAD mismatch|checkout is not synchronized)/.test(problem),
    ),
    warnings,
  };
}

function revalidatePinnedLicenseObject(source) {
  const binding = source?.evidenceBindings?.licenseObject;
  if (!binding || binding.readbackMatched !== true) return;
  let matched = false;
  if (binding.path === null) {
    const commit = git(source.repoPath, ["cat-file", "-e", `${binding.commit}^{commit}`]);
    matched =
      commit.ok &&
      binding.blob === null &&
      binding.sha256 === null &&
      rootLicensePaths(source.repoPath, binding.commit).length === 0;
  } else {
    const readback = readPinnedGitObject(
      source.repoPath,
      binding.commit,
      binding.path,
    );
    matched =
      readback.ok &&
      readback.blob === binding.blob &&
      readback.sha256 === binding.sha256;
  }
  if (!matched) {
    binding.readbackMatched = false;
    const problem = "license Git object changed during final readback";
    source.ok = false;
    if (!source.problems.includes(problem)) source.problems.push(problem);
    if (!source.admissionProblems.includes(problem)) {
      source.admissionProblems.push(problem);
    }
  }
}

function cleanLedgerCell(value) {
  const trimmed = String(value || "").trim();
  return /^`[^`]*`$/.test(trimmed) ? trimmed.slice(1, -1) : trimmed;
}

export function artifactAdmissionEntries(ledgerText) {
  const text = String(ledgerText || "");
  const queueHeadings = [...text.matchAll(/^## Current Review Queue[ \t]*\r?$/gm)];
  if (queueHeadings.length !== 1) {
    return {
      entries: [],
      problems: [
        `code-reuse provenance must contain exactly one Current Review Queue, got ${queueHeadings.length}`,
      ],
    };
  }
  const queueStart = queueHeadings[0].index;
  const queueRemainder = text.slice(queueStart);
  const nextHeading = queueRemainder.slice(1).search(/^## /m);
  const queueSection = nextHeading < 0
    ? queueRemainder
    : queueRemainder.slice(0, nextHeading + 1);
  const section = queueSection.match(
    /^## Current Review Queue[ \t]*\r?\n[\s\S]*?^\| Item \| Admission scope \| Source id \| Evidence \| Current disposition \| Required closure \|\r?\n^\|[^\r\n]+\|\r?\n([\s\S]*)$/m,
  );
  if (!section) {
    return {
      entries: [],
      problems: ["artifact admission table is missing or malformed"],
    };
  }
  const entries = [];
  const problems = [];
  for (const line of section[1]
    .split(/\r?\n/)
    .filter((value) => value.startsWith("|"))) {
    const cells = line.split("|").slice(1, -1).map((value) => value.trim());
    if (cells.length !== 6) {
      problems.push("artifact admission row has an unexpected column count");
      continue;
    }
    const scope = cleanLedgerCell(cells[1]);
    if (scope !== "artifact-entering") continue;
    const dispositionMatch = cells[4].match(/`([^`]+)`/);
    const disposition = cleanLedgerCell(dispositionMatch?.[1] || "unknown");
    const item = cleanLedgerCell(cells[0]);
    entries.push({
      item,
      scope,
      sourceId: cleanLedgerCell(cells[2]),
      evidence: cells[3],
      disposition,
      requiredClosure: cells[5],
    });
    if (!dispositionMatch || !ARTIFACT_DISPOSITIONS.has(disposition)) {
      problems.push(`${item || "<missing item>"} has an unrecognized disposition: ${disposition}`);
    }
  }
  if (entries.length === 0) problems.push("artifact admission inventory is empty");
  const entryCounts = new Map();
  for (const entry of entries) {
    entryCounts.set(entry.item, (entryCounts.get(entry.item) || 0) + 1);
  }
  for (const [item, count] of entryCounts.entries()) {
    if (!item) problems.push("artifact admission item is missing");
    if (count > 1) problems.push(`duplicate artifact entry: ${item}`);
  }
  return { entries, problems };
}

function inputBindingProblems(label, binding) {
  const problems = [];
  if (!binding || typeof binding !== "object" || Array.isArray(binding)) {
    return [`${label}: input binding is missing`];
  }
  if (!String(binding.path || "").trim()) {
    problems.push(`${label}: input path is missing`);
  }
  if (!SHA256_RE.test(String(binding.sha256 || ""))) {
    problems.push(`${label}: input SHA-256 is missing or invalid`);
  }
  if (!HASH_RE.test(String(binding.gitBlob || ""))) {
    problems.push(`${label}: tracked Git blob identity is missing or invalid`);
  }
  if (binding.readbackMatched !== true) {
    problems.push(`${label}: input hash/readback mismatch`);
  }
  return problems;
}

function sourceEvidenceProblems(source) {
  const problems = [];
  const bindings = source?.evidenceBindings;
  if (!bindings || typeof bindings !== "object" || Array.isArray(bindings)) {
    return ["source evidence binding is missing"];
  }
  if (!SHA256_RE.test(String(bindings.manifestEntrySha256 || ""))) {
    problems.push("source manifest entry SHA-256 is missing or invalid");
  }
  problems.push(...inputBindingProblems("source provenance ledger", bindings.ledger));
  const licenseObject = bindings.licenseObject;
  if (!licenseObject || typeof licenseObject !== "object" || Array.isArray(licenseObject)) {
    problems.push("source license object identity is missing");
  } else {
    if (!HASH_RE.test(String(licenseObject.commit || ""))) {
      problems.push("source license commit identity is missing or invalid");
    }
    if (licenseObject.path === null) {
      if (licenseObject.blob !== null || licenseObject.sha256 !== null) {
        problems.push("license-less source must bind null license blob and SHA-256");
      }
    } else {
      if (!String(licenseObject.path || "").trim()) {
        problems.push("source license path is missing");
      }
      if (!HASH_RE.test(String(licenseObject.blob || ""))) {
        problems.push("source license Git blob identity is missing or invalid");
      }
      if (!SHA256_RE.test(String(licenseObject.sha256 || ""))) {
        problems.push("source license SHA-256 is missing or invalid");
      }
      const classification = licenseObject.classification;
      if (
        !classification ||
        !new Set(["recognized", "unrecognized"]).has(classification.status) ||
        classification.contentSha256 !== licenseObject.sha256 ||
        (
          classification.status === "recognized" &&
          (
            !String(classification.licenseId || "").trim() ||
            !String(classification.detectedClass || "").trim()
          )
        ) ||
        (
          classification.status === "unrecognized" &&
          (classification.licenseId !== null || classification.detectedClass !== null)
        )
      ) {
        problems.push("source license content classification is missing or malformed");
      }
    }
    if (licenseObject.readbackMatched !== true) {
      problems.push("source license object hash/readback mismatch");
    }
  }
  return problems;
}

function closedSourceEvidence(source) {
  return {
    sourceId: source.id,
    licenseClass: source.license?.class || "unknown",
    manifestEntrySha256: source.evidenceBindings?.manifestEntrySha256 || "",
    provenanceLedger: source.evidenceBindings?.ledger || null,
    licenseObject: source.evidenceBindings?.licenseObject || null,
  };
}

export function buildArtifactAdmissionProjection({
  manifest,
  manifestBinding,
  provenanceLedgerBinding,
  ledgerText,
  sources,
  releaseSource,
  inputProblems = [],
}) {
  const parsed = artifactAdmissionEntries(ledgerText);
  const manifestSources = Array.isArray(manifest?.sources) ? manifest.sources : [];
  const validatedSources = Array.isArray(sources) ? sources : [];
  const registeredSourceCounts = new Map();
  for (const source of manifestSources) {
    const sourceId = String(source?.id || "");
    registeredSourceCounts.set(sourceId, (registeredSourceCounts.get(sourceId) || 0) + 1);
  }
  const validatedSourceCounts = new Map();
  for (const source of validatedSources) {
    const sourceId = String(source?.id || "");
    if (!validatedSourceCounts.has(sourceId)) validatedSourceCounts.set(sourceId, []);
    validatedSourceCounts.get(sourceId).push(source);
  }
  const duplicateItems = new Set(
    parsed.entries
      .map((entry) => entry.item)
      .filter((item, index, items) => items.indexOf(item) !== index),
  );
  const entries = parsed.entries.map((entry) => {
    const matchingSources = validatedSourceCounts.get(entry.sourceId) || [];
    const source = matchingSources.length === 1 ? matchingSources[0] : null;
    const problems = [];
    const registeredCount = registeredSourceCounts.get(entry.sourceId) || 0;
    if (registeredCount === 0) problems.push("source is not registered");
    if (registeredCount > 1) problems.push("source registration is duplicated or ambiguous");
    if (matchingSources.length !== 1) {
      problems.push("registered source evidence is unavailable or ambiguous");
    }
    problems.push(...(source?.admissionProblems || []));
    problems.push(...(source ? sourceEvidenceProblems(source) : []));
    if (!ARTIFACT_DISPOSITIONS.has(entry.disposition)) {
      problems.push(`unrecognized disposition: ${entry.disposition}`);
    } else if (!new Set(["approved", "retired"]).has(entry.disposition)) {
      problems.push(`disposition is ${entry.disposition}`);
    }
    if (entry.disposition === "approved") {
      if (source?.license?.class === "restricted-source-license") {
        problems.push(
          "restricted source approved without an accepted material-specific authorization contract",
        );
      } else if (source?.license?.class !== "permissive-with-notice") {
        problems.push(
          `license class ${source?.license?.class || "unknown"} is not admissible for an approved artifact entry`,
        );
      }
    } else if (
      entry.disposition === "retired" &&
      source?.license?.class === "restricted-source-license"
    ) {
      problems.push(
        "restricted source retired without an accepted exact replacement evidence contract",
      );
    }
    if (!String(entry.evidence || "").trim()) problems.push("evidence is missing");
    if (!String(entry.requiredClosure || "").trim()) {
      problems.push("required closure is missing");
    }
    if (duplicateItems.has(entry.item)) problems.push("duplicate artifact entry");
    return { ...entry, status: problems.length === 0 ? "passed" : "blocked", problems };
  });
  const relevantSourceIds = [...new Set(parsed.entries.map((entry) => entry.sourceId))].sort();
  const sourceEvidence = relevantSourceIds.flatMap((sourceId) => {
    const matchingSources = validatedSourceCounts.get(sourceId) || [];
    return matchingSources.length === 1 ? [closedSourceEvidence(matchingSources[0])] : [];
  });
  const projectionProblems = [
    ...inputProblems,
    ...inputBindingProblems("upstream manifest", manifestBinding),
    ...inputBindingProblems("code-reuse provenance ledger", provenanceLedgerBinding),
    ...parsed.problems,
    ...entries.flatMap((entry) => entry.problems.map((problem) => `${entry.item}: ${problem}`)),
  ];
  const projection = {
    schemaVersion: 1,
    id: "analytix.upstream-artifact-admission/v1",
    status: projectionProblems.length === 0 ? "passed" : "blocked",
    ok: projectionProblems.length === 0,
    engineeringAdmission: projectionProblems.length === 0,
    releaseAuthorization: false,
    releaseSource,
    inputs: {
      manifest: {
        ...manifestBinding,
        declaredAnalytixCommit: String(manifest?.analytixCommit || ""),
      },
      provenanceLedger: provenanceLedgerBinding,
      sourceEvidence,
    },
    entries,
    problems: projectionProblems,
  };
  return {
    ...projection,
    projectionSha256: sha256(canonicalJSONString(projection)),
  };
}

function currentSourceIdentity(repoRoot) {
  return {
    head: git(repoRoot, ["rev-parse", "HEAD"]).value,
    tree: git(repoRoot, ["rev-parse", "HEAD^{tree}"]).value,
  };
}

function parseManifest(record) {
  const problems = [];
  let manifest = { analytixCommit: "", sources: [] };
  if (!record?.ok) {
    problems.push("upstream manifest is missing or unreadable");
    return { manifest, problems };
  }
  try {
    const parsed = JSON.parse(record.bytes.toString("utf8"));
    if (!parsed || typeof parsed !== "object" || Array.isArray(parsed)) {
      problems.push("upstream manifest root must be an object");
    } else {
      manifest = parsed;
    }
  } catch {
    problems.push("upstream manifest is not valid JSON");
  }
  if (!Array.isArray(manifest.sources)) {
    problems.push("upstream manifest sources must be an array");
    manifest = { ...manifest, sources: [] };
  }
  for (const [index, source] of manifest.sources.entries()) {
    if (!source || typeof source !== "object" || Array.isArray(source)) {
      problems.push(`upstream manifest source ${index + 1} must be an object`);
    }
  }
  return { manifest, problems };
}

export function buildAuditReport({
  repoRoot = REPO_ROOT,
  upstreamRoot = UPSTREAM_ROOT,
  releaseHead = "",
  releaseTree = "",
  releaseOptionsValid = true,
} = {}) {
  const currentSource = currentSourceIdentity(repoRoot);
  const explicitReleaseSource = Boolean(releaseHead || releaseTree);
  const selectedHead = releaseHead || currentSource.head;
  const selectedTree = releaseTree || currentSource.tree;
  const releaseSourceProblems = [];
  if (!releaseOptionsValid || explicitReleaseSource !== Boolean(releaseHead && releaseTree)) {
    releaseSourceProblems.push(
      "release HEAD and tree must be supplied together exactly once",
    );
  }
  if (explicitReleaseSource) {
    if (currentSource.head !== selectedHead || currentSource.tree !== selectedTree) {
      releaseSourceProblems.push(
        "provided release HEAD/tree do not match the current release source",
      );
    }
  }
  const releaseObject = validateManifestReleaseBinding({
    repoRoot,
    declaredAnalytixCommit: selectedHead,
    releaseHead: selectedHead,
    releaseTree: selectedTree,
  });
  releaseSourceProblems.push(
    ...releaseObject.problems.filter((problem) =>
      problem.startsWith("release HEAD") || problem.startsWith("release tree"),
    ),
  );

  const repoInput = createRepoInput({
    repoRoot,
    releaseHead: explicitReleaseSource ? selectedHead : "",
  });
  const manifestRelativePath = normalizedRepoPath(
    repoRoot,
    relative(repoRoot, MANIFEST_PATH).replaceAll("\\", "/"),
  );
  const provenanceRelativePath = normalizedRepoPath(
    repoRoot,
    relative(repoRoot, CODE_REUSE_LEDGER_PATH).replaceAll("\\", "/"),
  );
  const manifestRecord = repoInput.read(manifestRelativePath);
  const provenanceRecord = repoInput.read(provenanceRelativePath);
  const { manifest, problems: manifestProblems } = parseManifest(manifestRecord);
  const manifestReleaseBinding = validateManifestReleaseBinding({
    repoRoot,
    declaredAnalytixCommit: manifest.analytixCommit,
    releaseHead: selectedHead,
    releaseTree: selectedTree,
  });
  const safeManifestSources = manifest.sources.filter(
    (source) => source && typeof source === "object" && !Array.isArray(source),
  );
  const discovered = discoverRepositories(upstreamRoot);
  const configured = safeManifestSources
    .map((source) => String(source.directory || ""))
    .sort();
  const configuredSet = new Set(configured);
  const discoveredSet = new Set(discovered);
  const unregistered = discovered.filter((name) => !configuredSet.has(name));
  const unavailable = configured.filter((name) => !discoveredSet.has(name));
  const duplicateDirectories = configured.filter(
    (name, index) => configured.indexOf(name) !== index,
  );
  const duplicateIds = safeManifestSources
    .map((source) => source.id)
    .filter((id, index, ids) => ids.indexOf(id) !== index);
  const duplicateLedgers = safeManifestSources
    .map((source) => source.ledger)
    .filter((ledger, index, ledgers) => ledgers.indexOf(ledger) !== index);
  const sources = safeManifestSources.map((source) =>
    validateSource(source, manifest.analytixCommit, {
      repoRoot,
      upstreamRoot,
      repoInput,
    }),
  );
  const structuralProblems = [
    ...unregistered.map((name) => `unregistered upstream: ${name}`),
    ...unavailable.map((name) => `configured upstream unavailable: ${name}`),
    ...duplicateDirectories.map((name) => `duplicate directory: ${name}`),
    ...duplicateIds.map((id) => `duplicate source id: ${id}`),
    ...duplicateLedgers.map((ledger) => `duplicate ledger: ${ledger}`),
  ];
  repoInput.revalidate();
  for (const source of sources) revalidatePinnedLicenseObject(source);
  const finalSource = currentSourceIdentity(repoRoot);
  if (finalSource.head !== selectedHead || finalSource.tree !== selectedTree) {
    releaseSourceProblems.push("release HEAD/tree changed during upstream audit");
  }
  const releaseSource = {
    head: selectedHead,
    tree: selectedTree,
    inputMode: explicitReleaseSource ? "release-git-object" : "working-tree",
    verified:
      releaseSourceProblems.length === 0 &&
      HASH_RE.test(selectedHead) &&
      HASH_RE.test(selectedTree),
    problems: [...new Set(releaseSourceProblems)],
  };
  const artifactAdmission = buildArtifactAdmissionProjection({
    manifest,
    manifestBinding: repoFileBinding(manifestRecord),
    provenanceLedgerBinding: repoFileBinding(provenanceRecord),
    ledgerText: provenanceRecord.ok
      ? provenanceRecord.bytes.toString("utf8")
      : "",
    sources,
    releaseSource,
    inputProblems: [
      ...manifestProblems,
      ...manifestReleaseBinding.problems,
      ...releaseSource.problems,
      ...repoInput.problems,
    ],
  });
  const researchProblems = [
    ...structuralProblems,
    ...sources.flatMap((source) =>
      (source.freshnessProblems || []).map((problem) => `${source.id}: ${problem}`),
    ),
  ];
  const researchFreshness = {
    status: researchProblems.length === 0
      ? "passed"
      : unavailable.length > 0 || unregistered.length > 0
        ? "unverified"
        : "stale",
    releaseBlocking: false,
    problems: researchProblems,
    discovered,
    configured,
  };
  const ok = artifactAdmission.ok;
  const report = {
    schemaVersion: 2,
    id: "upstream-source-audit",
    releaseSource,
    manifestPath: manifestRelativePath,
    manifestSha256: manifestRecord.sha256,
    manifestGitBlob: manifestRecord.gitBlob,
    upstreamRoot,
    reviewedAt: manifest.reviewedAt,
    analytixCommit: manifest.analytixCommit,
    strict,
    strictFreshness,
    ok,
    artifactAdmission,
    projectionSha256: artifactAdmission.projectionSha256,
    researchFreshness,
    sourceCount: sources.length,
    discovered,
    configured,
    structuralProblems,
    sources,
  };

  return report;
}

function main() {
  let report;
  try {
    report = buildAuditReport({
      releaseHead: releaseHeadOption.value,
      releaseTree: releaseTreeOption.value,
      releaseOptionsValid: releaseHeadOption.valid && releaseTreeOption.valid,
    });
  } catch (error) {
    const releaseSource = {
      head: releaseHeadOption.value,
      tree: releaseTreeOption.value,
      inputMode:
        releaseHeadOption.provided || releaseTreeOption.provided
          ? "release-git-object"
          : "working-tree",
      verified: false,
      problems: ["upstream audit failed before completing its closed projection"],
    };
    const artifactAdmission = buildArtifactAdmissionProjection({
      manifest: { analytixCommit: "", sources: [] },
      manifestBinding: null,
      provenanceLedgerBinding: null,
      ledgerText: "",
      sources: [],
      releaseSource,
      inputProblems: [String(error?.message || error || "unknown audit failure")],
    });
    report = {
      schemaVersion: 2,
      id: "upstream-source-audit",
      releaseSource,
      strict,
      strictFreshness,
      ok: false,
      artifactAdmission,
      projectionSha256: artifactAdmission.projectionSha256,
      researchFreshness: {
        status: "unverified",
        releaseBlocking: false,
        problems: ["audit did not complete"],
        discovered: [],
        configured: [],
      },
      sourceCount: 0,
      discovered: [],
      configured: [],
      structuralProblems: [],
      sources: [],
    };
  }

  if (jsonOutput) {
    process.stdout.write(`${JSON.stringify(report, null, 2)}\n`);
  } else {
    console.log(
      `${report.ok ? "PASS" : "BLOCKED"} upstream artifact admission (${report.artifactAdmission.entries.length} entries)`,
    );
    console.log(`manifest: ${report.manifestPath || "<unavailable>"}`);
    console.log(`upstream root: ${report.upstreamRoot || UPSTREAM_ROOT}`);
    for (const problem of report.structuralProblems || []) console.log(`FAIL ${problem}`);
    console.log(`${report.researchFreshness.status.toUpperCase()} research checkout freshness`);
    for (const source of report.sources || []) {
      console.log(
        `${source.ok ? "PASS" : "FAIL"} ${source.directory} ${source.branch || "<no-branch>"} ${source.head || "<no-head>"} ${source.license?.class || "<no-license-class>"}`,
      );
      for (const problem of source.problems || [])
        console.log(`  FAIL ${problem}`);
      for (const warning of source.warnings || [])
        console.log(`  WARN ${warning}`);
    }
  }

  if (!report.ok && strict) process.exitCode = 1;
  if (strictFreshness && report.researchFreshness.status !== "passed") {
    process.exitCode = 1;
  }
}

if (
  process.argv[1] &&
  pathToFileURL(resolve(process.argv[1])).href === import.meta.url
) {
  main();
}
