import crypto from "node:crypto";
import fs from "node:fs";
import path from "node:path";

function sha256(value) {
  return crypto.createHash("sha256").update(value).digest("hex");
}

function goJsonString(value) {
  const encoded = JSON.stringify(value);
  if (encoded === undefined) return undefined;
  return encoded.replace(/[<>&\u2028\u2029]/gu, (character) => ({
    "<": "\\u003c",
    ">": "\\u003e",
    "&": "\\u0026",
    "\u2028": "\\u2028",
    "\u2029": "\\u2029"
  })[character]);
}

function canonicalJson(value) {
  if (value === null) return "null";
  if (Array.isArray(value)) return `[${value.map(canonicalJson).join(",")}]`;
  if (typeof value === "object") {
    return `{${Object.keys(value).sort().map((key) => `${goJsonString(key)}:${canonicalJson(value[key])}`).join(",")}}`;
  }
  return goJsonString(value);
}

function goContractHash(record) {
  return sha256(goJsonString(record));
}

export function writeCaseProjectBinding(workspaceRoot, caseId) {
  fs.mkdirSync(path.join(workspaceRoot, ".analytix"), { recursive: true });
  const workspaceRealPath = fs.realpathSync(workspaceRoot);
  const body = `${JSON.stringify({
    version: 1,
    workspaceRoot: workspaceRealPath,
    caseId,
    source: "analytix-data-analysis"
  }, null, 2)}\n`;
  const configPath = path.join(workspaceRealPath, ".analytix", "case-project.json");
  fs.writeFileSync(configPath, body, "utf8");
  const bindingSha256 = sha256(Buffer.from(body));
  const caseBindingHash = sha256(["case-binding-v1", workspaceRealPath, caseId, bindingSha256].join("\u0000"));
  return { workspaceRealPath, configPath, bindingSha256, caseBindingHash };
}

export function hostFactRuntimeContext({
  workspaceRealPath,
  caseId,
  caseBindingHash,
  toolName,
  args = {},
  serverId = "analytix_funds",
  serverName = "analytix_funds",
  serverVersion = "0.16.16",
  connectionEpoch = 7,
  overrides = {}
}) {
  const now = Date.now();
  const issuedAt = new Date(now - 10_000).toISOString();
  const grantIssuedAt = new Date(now - 5_000).toISOString();
  const expiresAt = new Date(now + 10 * 60_000).toISOString();
  const serverIdentity = [
    "mcpv2",
    Buffer.from(serverId).toString("base64url"),
    Buffer.from(serverName).toString("base64url"),
    Buffer.from(serverVersion).toString("base64url"),
    Buffer.from("2025-11-25").toString("base64url"),
    sha256(`connection:${serverId}:${connectionEpoch}`),
    String(connectionEpoch)
  ].join(":");
  const record = {
    version: 1,
    threadId: "thread_fixture",
    turnId: "turn_fixture",
    workspaceRealPath,
    tenantId: "local",
    userId: "local",
    caseId,
    caseBindingHash,
    datasetSnapshotId: `dsv1_${sha256(`snapshot:${caseId}`)}`,
    sourceManifestHash: sha256(`manifest:${caseId}`),
    contextEpoch: 3,
    issuedAt,
    contextDigest: "",
    grantId: "",
    provider: "fixture-provider",
    serverIdentity,
    toolName: `mcp__${serverId}__${toolName}`,
    toolCallId: `call_${toolName}`,
    connectionEpoch,
    argsHash: sha256(canonicalJson(args)),
    schemaHash: sha256(`schema:${toolName}`),
    scopeHash: sha256(`scope:${toolName}`),
    readOnly: true,
    approvalState: "not_required",
    grantIssuedAt,
    expiresAt,
    ...overrides
  };
  if (!Object.prototype.hasOwnProperty.call(overrides, "contextDigest")) {
    record.contextDigest = goContractHash({
      version: record.version,
      threadId: record.threadId,
      turnId: record.turnId,
      workspaceRealPath: record.workspaceRealPath,
      tenantId: record.tenantId,
      userId: record.userId,
      caseId: record.caseId,
      caseBindingHash: record.caseBindingHash,
      datasetSnapshotId: record.datasetSnapshotId,
      sourceManifestHash: record.sourceManifestHash,
      contextEpoch: record.contextEpoch,
      issuedAt: record.issuedAt,
      contextDigest: ""
    });
  }
  if (!Object.prototype.hasOwnProperty.call(overrides, "grantId")) {
    record.grantId = goContractHash({
      version: record.version,
      grantId: "",
      turnId: record.turnId,
      contextDigest: record.contextDigest,
      provider: record.provider,
      serverIdentity: record.serverIdentity,
      toolName: record.toolName,
      toolCallId: record.toolCallId,
      connectionEpoch: record.connectionEpoch,
      argsHash: record.argsHash,
      schemaHash: record.schemaHash,
      scopeHash: record.scopeHash,
      readOnly: record.readOnly,
      approvalState: record.approvalState,
      issuedAt: record.grantIssuedAt,
      expiresAt: record.expiresAt
    });
  }
  return record;
}

export function hostSourceProbeRuntimeContext(factContext) {
  return {
    version: factContext.version,
    workspaceRealPath: factContext.workspaceRealPath,
    threadId: factContext.threadId,
    turnId: factContext.turnId,
    caseId: factContext.caseId,
    caseBindingHash: factContext.caseBindingHash,
    datasetSnapshotId: factContext.datasetSnapshotId,
    contextEpoch: factContext.contextEpoch,
    contextDigest: factContext.contextDigest
  };
}
