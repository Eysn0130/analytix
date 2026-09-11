import crypto from "node:crypto";
import { spawn } from "node:child_process";
import fs from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";
import {
  arrayOf,
  hasOwn,
  intOrUndefined,
  objectOf,
  pruneEmpty,
  text
} from "./runtime-normalizers.mjs";
import {
  assertAnalytixArtifactRoot,
  DEFAULT_ANALYTIX_ARTIFACT_PARTS
} from "./artifact-path-policy.mjs";
import {
  assertExplicitCaseMatchesProject,
  explicitCaseIdFromArgs,
  resolveCaseDuckdbPath,
  resolveCaseProjectContext
} from "./case-project-context.mjs";
import { abortError, throwIfAborted } from "./abort-runtime.mjs";
import {
  analyzeDuckdbReadOnlySql,
  isAllowedDuckdbWorkbenchTable,
  stripDuckdbSqlComments
} from "./duckdb-sql-policy.mjs";
import {
  duckdbDiagnosticError,
  isDuckdbDiagnosticError,
  publicDuckdbDiagnostic,
  shouldRetryDuckdbRunner
} from "./duckdb-diagnostic.mjs";

export const DUCKDB_WORKBENCH_RUNTIME_VERSION = "0.16.16";
export const INTERNAL_DATASET_SNAPSHOT_PROBE_TOOL = "__analytix_internal_dataset_snapshot_probe";
export const INTERNAL_FUNDS_PRODUCER_CONTENT_PROBE_TOOL = "__analytix_internal_funds_producer_content_probe";

const FUNDS_PRODUCER_CONTENT_CONTRACT_V1 = "analytix.funds-producer-content-manifest/v1";

const FORBIDDEN_SQL_PATTERN = /\b(?:attach|detach|copy|create|alter|drop|insert|update|delete|merge|truncate|call|pragma|install|load|export|import|set|reset|vacuum|checkpoint|read_csv|read_parquet|read_json|sqlite_scan|postgres_scan|httpfs)\b/iu;
const SQL_IDENTIFIER_PATTERN = /^[A-Za-z_][A-Za-z0-9_]*$/u;
const LOCAL_DUCKDB_WORKBENCH_SKILLS = new Set([
  "rank_accounts",
  "rank_holders",
  "rank_counterparties",
  "trace_subject_top_outflows",
  "trace_fund_next_hop",
  "trace_fund",
  "get_scope_coverage",
  "inspect_case_schema",
  "profile_case_schema",
  "preview_case_rows",
  "explain_case_sql",
  "diagnose_case_sql",
  "count_case_rows",
  "inspect_workbench_history",
  "case_sql_recipes",
  "create_case_notebook",
  "run_case_sql"
]);
const LOCAL_WORKBENCH_HISTORY_LIMIT = 120;
const LOCAL_WORKBENCH_HISTORY = [];
const MAX_DUCKDB_STDOUT_BYTES = 8 * 1024 * 1024;
const MAX_DUCKDB_STDERR_BYTES = 256 * 1024;
const UNSCOPED_AMOUNT_FACT_TABLES = new Set([
  "analysis_key_node_features",
  "analysis_rule_hit",
  "analysis_txn_daily_agg",
  "analysis_txn_detail_idx",
  "analysis_txn_keyword_idx",
  "fc_transaction_norm"
]);

function topNContract({
  requestedLimit,
  resolvedLimit,
  returnedCount,
  filteredCount,
  compactTextRowCount,
  dataExhausted,
  resultComplete = false,
  truncationReason = ""
}) {
  const requested = positiveIntOrUndefined(requestedLimit) ?? positiveIntOrUndefined(resolvedLimit);
  const resolved = positiveIntOrUndefined(resolvedLimit) ?? requested;
  const returned = nonNegativeIntOrUndefined(returnedCount);
  if (requested === undefined || resolved === undefined || returned === undefined) {
    throw new Error("DuckDB Top-N result is missing a valid requested, resolved, or returned count.");
  }
  const compactRows = nonNegativeIntOrUndefined(compactTextRowCount) ?? returned;
  const filtered = nonNegativeIntOrUndefined(filteredCount);
  const explicitExhausted = typeof dataExhausted === "boolean" ? dataExhausted : undefined;
  const exhausted = resultComplete ? (explicitExhausted ?? returned < requested) : undefined;
  return {
    requested_limit: requested,
    resolved_limit: resolved,
    returned_count: returned,
    visible_count: returned,
    ...(filtered !== undefined ? { filtered_count: filtered } : {}),
    ...(exhausted !== undefined ? { data_exhausted: exhausted } : {}),
    truncation_reason: truncationReason || (
      exhausted === true ? "source_data_exhausted_before_requested_limit" : (
        exhausted === false ? "requested_limit_reached_with_more_source_rows" : "result_completeness_unresolved"
      )
    ),
    compact_text_row_count: compactRows,
    final_answer_expected_min_rows: Math.min(requested, returned),
    result_completeness: resultComplete ? "complete" : "partial"
  };
}

function nonNegativeIntOrUndefined(value) {
  const number = intOrUndefined(value);
  return number !== undefined && number >= 0 ? number : undefined;
}

function positiveIntOrUndefined(value) {
  const number = intOrUndefined(value);
  return number !== undefined && number > 0 ? number : undefined;
}

function strictNonNegativeInteger(value) {
	return typeof value === "number" && Number.isSafeInteger(value) && value >= 0 ? value : undefined;
}

function strictBoolean(value) {
	return typeof value === "boolean" ? value : undefined;
}

function consistentBoolean(...values) {
	const known = values.map(strictBoolean).filter((value) => value !== undefined);
	if (!known.length || known.some((value) => value !== known[0])) return undefined;
	return known[0];
}

function strictFiniteNumber(value) {
  return typeof value === "number" && Number.isFinite(value) ? value : undefined;
}

function isCanonicalIntegerValue(value) {
  return strictNonNegativeInteger(value) !== undefined
    || (typeof value === "string" && /^(?:0|[1-9][0-9]*)$/u.test(value));
}

function isCanonicalDecimalValue(value) {
  if (strictFiniteNumber(value) !== undefined) return true;
  return typeof value === "string"
    && /^-?(?:0|[1-9][0-9]*)(?:\.[0-9]+)?$/u.test(value)
    && !/^-0(?:\.0+)?$/u.test(value);
}

function requireStrictCount(record, field, context) {
  const source = objectOf(record);
  const value = strictNonNegativeInteger(source[field]);
  if (!hasOwn(source, field) || value === undefined) {
    throw new Error(`${context} returned an invalid ${field}; a non-negative integer is required.`);
  }
  return value;
}

function optionalStrictNumber(record, field, context) {
  const source = objectOf(record);
  if (!hasOwn(source, field) || source[field] === null) return undefined;
  const value = strictFiniteNumber(source[field]);
  if (value === undefined) {
    throw new Error(`${context} returned an invalid ${field}; a finite number or null is required.`);
  }
  return value;
}

function requireStrictPositiveInteger(record, field, context) {
  const value = requireStrictCount(record, field, context);
  if (value === 0) {
    throw new Error(`${context} returned zero ${field}; zero cannot support a trace fact.`);
  }
  return value;
}

function requireStrictPositiveNumber(record, field, context) {
  const source = objectOf(record);
  const value = strictFiniteNumber(source[field]);
  if (!hasOwn(source, field) || value === undefined || value <= 0) {
    throw new Error(`${context} returned an invalid ${field}; a positive finite number is required for a trace fact.`);
  }
  return value;
}

function requireStrictBinaryCount(record, field, context) {
  const value = requireStrictCount(record, field, context);
  if (value !== 0 && value !== 1) {
    throw new Error(`${context} returned an invalid ${field}; zero or one is required.`);
  }
  return value;
}

function validateDuckdbResult(value) {
  if (!value || typeof value !== "object" || Array.isArray(value)) {
    throw new Error("DuckDB runner returned a non-object result.");
  }
  const allowedRootFields = new Set([
    "columns",
    "column_types",
    "records",
    "row_count",
    "truncated",
    "snapshot_contract",
    "snapshot_factual_ready",
    "snapshot_blocker",
    "snapshot_manifest_schema_version",
    "observed_dataset_snapshot_id",
    "producer_content_contract",
    "producer_content_id",
    "producer_manifest_sha256",
    "producer_manifest_schema_version"
  ]);
  if (Object.keys(value).some((key) => !allowedRootFields.has(key))) {
    throw new Error("DuckDB runner result contains an undeclared root field.");
  }
  if (!Array.isArray(value.columns) || !Array.isArray(value.records)) {
    throw new Error("DuckDB runner result is missing columns or records arrays.");
  }
  const rowCount = strictNonNegativeInteger(value.row_count);
  if (rowCount === undefined || rowCount !== value.records.length) {
    throw new Error("DuckDB runner row_count is missing, invalid, or inconsistent with records.");
  }
  if (typeof value.truncated !== "boolean") {
    throw new Error("DuckDB runner result is missing a boolean truncated marker.");
  }
  if (
    typeof value.snapshot_contract !== "string"
    || typeof value.observed_dataset_snapshot_id !== "string"
    || typeof value.snapshot_factual_ready !== "boolean"
    || typeof value.snapshot_blocker !== "string"
    || strictNonNegativeInteger(value.snapshot_manifest_schema_version) === undefined
    || (value.producer_content_contract !== undefined && typeof value.producer_content_contract !== "string")
    || (value.producer_content_id !== undefined && typeof value.producer_content_id !== "string")
    || (value.producer_manifest_sha256 !== undefined && typeof value.producer_manifest_sha256 !== "string")
    || (value.producer_manifest_schema_version !== undefined
      && strictNonNegativeInteger(value.producer_manifest_schema_version) === undefined)
  ) {
    throw new Error("DuckDB runner result is missing closed snapshot or producer-content fields.");
  }
  const snapshotContract = text(value.snapshot_contract);
  const observedDatasetSnapshotId = text(value.observed_dataset_snapshot_id);
  const snapshotFactualReady = value.snapshot_factual_ready;
  const snapshotBlocker = text(value.snapshot_blocker);
  const snapshotManifestSchemaVersion = strictNonNegativeInteger(value.snapshot_manifest_schema_version);
  const producerContentContract = text(value.producer_content_contract);
  const producerContentId = text(value.producer_content_id);
  const producerManifestSha256 = text(value.producer_manifest_sha256);
  const producerManifestSchemaVersion = value.producer_manifest_schema_version === undefined
    ? 0
    : strictNonNegativeInteger(value.producer_manifest_schema_version);
  const canonicalSnapshotStrings = value.snapshot_contract === snapshotContract
    && value.observed_dataset_snapshot_id === observedDatasetSnapshotId
    && value.snapshot_blocker === snapshotBlocker
    && (value.producer_content_contract === undefined || value.producer_content_contract === producerContentContract)
    && (value.producer_content_id === undefined || value.producer_content_id === producerContentId)
    && (value.producer_manifest_sha256 === undefined || value.producer_manifest_sha256 === producerManifestSha256);
  const legacySnapshot = snapshotContract === "analytix_duckdb_dataset_snapshot_v1"
    && /^dsv1_[a-f0-9]{64}$/u.test(observedDatasetSnapshotId)
    && snapshotFactualReady === false
    && snapshotManifestSchemaVersion === 0
    && [
      "dataset_snapshot_manifest_v2_unavailable",
      "funds_producer_content_manifest_v1_unavailable",
      "legacy_exact_count_quarantine",
      "local_dataset_snapshot_v2_retired"
    ].includes(snapshotBlocker);
  const retiredLocalSnapshot = snapshotContract === "analytix_duckdb_dataset_snapshot_v2"
    && /^dsv2_[a-f0-9]{64}$/u.test(observedDatasetSnapshotId)
    && snapshotFactualReady === false
    && snapshotManifestSchemaVersion === 2
    && snapshotBlocker === "local_dataset_snapshot_v2_retired";
  const noSnapshot = !snapshotContract
    && !observedDatasetSnapshotId
    && snapshotFactualReady === false
    && !snapshotBlocker
    && snapshotManifestSchemaVersion === 0;
  const noProducerContent = !producerContentContract
    && !producerContentId
    && !producerManifestSha256
    && producerManifestSchemaVersion === 0;
  const verifiedProducerContent = producerContentContract === FUNDS_PRODUCER_CONTENT_CONTRACT_V1
    && /^fpc1_[a-f0-9]{64}$/u.test(producerContentId)
    && /^[a-f0-9]{64}$/u.test(producerManifestSha256)
    && producerManifestSchemaVersion === 1;
  if (
    !canonicalSnapshotStrings
    || Boolean(snapshotContract) !== Boolean(observedDatasetSnapshotId)
    || (snapshotContract && !legacySnapshot && !retiredLocalSnapshot)
    || (!snapshotContract && !noSnapshot)
    || (!noProducerContent && !verifiedProducerContent)
  ) {
    throw new Error("DuckDB runner returned invalid snapshot or producer-content authority.");
  }
  const columns = value.columns.map((column) => text(column));
  if (!columns.length || columns.some((column) => !column) || new Set(columns).size !== columns.length) {
    throw new Error("DuckDB runner returned invalid or duplicate column names.");
  }
  if (value.records.some((record) => !record || typeof record !== "object" || Array.isArray(record))) {
    throw new Error("DuckDB runner returned a malformed record.");
  }
  const columnTypes = value.column_types === undefined
    ? []
    : arrayOf(value.column_types).map((columnType) => text(columnType).toUpperCase());
  if (value.column_types !== undefined && (
    columnTypes.length !== columns.length || columnTypes.some((columnType) => !columnType)
  )) {
    throw new Error("DuckDB runner returned an invalid column type manifest.");
  }
  for (const record of value.records) {
    if (columns.some((column) => !hasOwn(record, column))) {
      throw new Error("DuckDB runner record is missing a declared column.");
    }
    if (Object.keys(record).some((key) => !columns.includes(key))) {
      throw new Error("DuckDB runner record contains an undeclared column.");
    }
  }
  return {
    ...value,
    columns,
    ...(columnTypes.length ? { column_types: columnTypes } : {}),
    records: value.records,
    row_count: rowCount,
    truncated: value.truncated,
    snapshot_contract: snapshotContract,
    snapshot_factual_ready: snapshotFactualReady,
    snapshot_blocker: snapshotBlocker,
    snapshot_manifest_schema_version: snapshotManifestSchemaVersion,
    observed_dataset_snapshot_id: observedDatasetSnapshotId,
    producer_content_contract: producerContentContract,
    producer_content_id: producerContentId,
    producer_manifest_sha256: producerManifestSha256,
    producer_manifest_schema_version: producerManifestSchemaVersion,
    result_integrity: {
      status: "validated",
      row_count_consistent: true,
      pagination_complete: value.truncated === false,
      column_types_bound: columnTypes.length === columns.length
    }
  };
}

function requireCompleteAggregateRecord(result, context) {
  if (result.truncated !== false || result.row_count !== 1 || result.records.length !== 1) {
    throw new Error(`${context} requires one complete aggregate result row.`);
  }
  return objectOf(result.records[0]);
}
const PYTHON_SCRIPT = String.raw`
import datetime
import decimal
import hashlib
import json
import math
import re
import sys

import duckdb


def normalize(value):
    if isinstance(value, (datetime.datetime, datetime.date, datetime.time)):
        try:
            return value.isoformat(sep=" ")
        except TypeError:
            return value.isoformat()
    if isinstance(value, decimal.Decimal):
        return format(value, "f")
    if isinstance(value, bool):
        return value
    if isinstance(value, int):
        return str(value) if abs(value) > 9007199254740991 else value
    if isinstance(value, float):
        if not math.isfinite(value):
            raise RuntimeError("DuckDB result contains a non-finite numeric value")
        return value
    if isinstance(value, bytes):
        return value.decode("utf-8", errors="replace")
    if isinstance(value, list):
        return [normalize(item) for item in value]
    if isinstance(value, tuple):
        return [normalize(item) for item in value]
    if isinstance(value, dict):
        return {str(key): normalize(item) for key, item in value.items()}
    return value


SENSITIVE_IDENTIFIER_COLUMN = re.compile(
    r"^(?:(?:payer|payee|source|target|sender|receiver|counterparty|bank)_)?(?:account|acct|card)(?:_(?:no|number)(?:_norm)?|_(?:key|id|norm|raw|masked))?$",
    re.IGNORECASE,
)


def normalize_result_value(column_name, column_type, value):
    if value is None:
        return None
    if SENSITIVE_IDENTIFIER_COLUMN.search(str(column_name)):
        if not isinstance(value, str):
            raise RuntimeError("DuckDB identifier columns must use a textual source type")
        return value
    return normalize(value)


def require_table(con, table_name):
    row = con.execute(
        "SELECT COUNT(*) FROM information_schema.tables WHERE table_schema='main' AND table_name=?",
        [table_name],
    ).fetchone()
    if not row or int(row[0] or 0) != 1:
        raise RuntimeError("dataset snapshot source table is unavailable: " + table_name)


def table_exists(con, table_name):
    row = con.execute(
        "SELECT COUNT(*) FROM information_schema.tables WHERE table_schema='main' AND table_name=?",
        [table_name],
    ).fetchone()
    return bool(row and int(row[0]) == 1)


def require_base_table(con, table_name):
    rows = con.execute(
        "SELECT table_type FROM information_schema.tables WHERE table_schema='main' AND table_name=?",
        [table_name],
    ).fetchall()
    if len(rows) != 1 or str(rows[0][0]).upper() != "BASE TABLE":
        raise RuntimeError("dataset snapshot requires an immutable base table: " + table_name)


def scalar_count(con, sql, params=None):
    row = con.execute(sql, params or []).fetchone()
    value = int(row[0] or 0) if row else 0
    if value < 0:
        raise RuntimeError("dataset snapshot count is invalid")
    return value


def required_nonnegative_integer(value, label):
    if value is None or isinstance(value, bool) or not isinstance(value, int) or value < 0:
        raise RuntimeError("dataset snapshot " + label + " is missing or invalid")
    return value


MATERIALIZATION_VERSION = 12
MATERIALIZATION_IDENTITY_SCHEMA_VERSION = 2
MATERIALIZATION_IDENTITY_PREFIX = "txn_daily_snapshot:v12:"
DATASET_SNAPSHOT_MANIFEST_V2_TABLE = "analysis_dataset_snapshot_manifest_v2"
DATASET_SNAPSHOT_MANIFEST_V2_SCHEMA_VERSION = 2
FUNDS_PRODUCER_CONTENT_MANIFEST_V1_TABLE = "analysis_funds_content_manifest_v1"
FUNDS_PRODUCER_CONTENT_MANIFEST_V1_SCHEMA_VERSION = 1
FUNDS_PRODUCER_CONTENT_MANIFEST_V1_CONTRACT = "analytix.funds-producer-content-manifest/v1"
FUNDS_PRODUCER_CONTENT_ID_PREFIX = "fpc1_"
FUNDS_PRODUCER_CONTENT_DIGEST_DOMAIN = b"AnalytixFundsProducerContentManifestV1\x00"
DUCKDB_CONTENT_MANIFEST_ALGORITHM = "analytix.duckdb-content-manifest/v1"
REQUIRED_DUCKDB_MODULE_VERSION = "1.5.4"
REQUIRED_DUCKDB_ENGINE_VERSION = "v1.5.4"
PRODUCER_COMPONENT_ID = "analysis-compute"
PRODUCER_COMPONENT_VERSION = "0.1.0"
PRODUCER_OPERATION = "materialize-txn-daily"
PRODUCER_OPERATION_SCHEMA = (
    '{"additionalProperties":false,"properties":{'
    '"caseId":{"type":"string"},'
    '"sourceMaxId":{"minimum":0,"type":"integer"},'
    '"sourceMaxTxnTs":{"type":"string"},'
    '"sourceRevision":{"minimum":1,"type":"integer"},'
    '"sourceRowCount":{"minimum":0,"type":"integer"}},'
    '"required":["caseId","sourceMaxId","sourceMaxTxnTs",'
    '"sourceRevision","sourceRowCount"],"type":"object"}'
)
NORMALIZED_SOURCE_SIGNATURE_DOMAIN = "analytix.txn-daily-normalized-source/v12"
RESULT_SIGNATURE_DOMAIN = "analytix.txn-daily-result/v12"
RESULT_SIGNATURE_TABLES = (
    (
        "analysis_txn_daily_agg",
        "r.acct_key, r.txn_day, r.cp_key, r.dc_val",
        {"acct_key", "txn_day", "cp_key", "dc_val"},
    ),
    (
        "analysis_txn_detail_idx",
        "TRY_CAST(r.id AS BIGINT), r.id",
        {"id"},
    ),
    (
        "analysis_txn_keyword_idx",
        "TRY_CAST(r.txn_row_id AS BIGINT), r.txn_row_id, r.kind, r.token, r.token_order",
        {"txn_row_id", "kind", "token", "token_order"},
    ),
    (
        "analysis_account_dim",
        "r.account_key",
        {"account_key"},
    ),
)


def canonical_hash_value(value):
    if value is None:
        return {"type": "null"}
    if isinstance(value, bool):
        return {"type": "boolean", "value": value}
    if isinstance(value, int):
        return {"type": "integer", "value": str(value)}
    if isinstance(value, decimal.Decimal):
        return {"type": "decimal", "value": format(value, "f")}
    if isinstance(value, float):
        if not math.isfinite(value):
            raise RuntimeError("dataset snapshot contains a non-finite numeric value")
        normalized = 0.0 if value == 0.0 else value
        return {"type": "float", "value": normalized.hex()}
    if isinstance(value, (datetime.datetime, datetime.date, datetime.time)):
        try:
            rendered = value.isoformat(sep=" ")
        except TypeError:
            rendered = value.isoformat()
        return {"type": "temporal", "value": rendered}
    if isinstance(value, str):
        return {"type": "text", "value": value}
    if isinstance(value, bytes):
        return {"type": "bytes", "value": value.hex()}
    if isinstance(value, (list, tuple)):
        return {"type": "array", "value": [canonical_hash_value(item) for item in value]}
    if isinstance(value, dict):
        return {
            "type": "object",
            "value": {str(key): canonical_hash_value(item) for key, item in value.items()},
        }
    raise RuntimeError("dataset snapshot contains an unsupported content type")


def canonical_json(value):
    return json.dumps(value, ensure_ascii=False, separators=(",", ":"), sort_keys=True, allow_nan=False)


def sha256_canonical(value):
    return hashlib.sha256(canonical_json(value).encode("utf-8")).hexdigest()


def ordered_table_columns(con, table_name, required_columns=None):
    if not re.fullmatch(r"[a-z_][a-z0-9_]*", table_name):
        raise RuntimeError("dataset snapshot table identity is invalid")
    require_base_table(con, table_name)
    rows = con.execute(
        """
        SELECT column_name
          FROM information_schema.columns
         WHERE table_schema='main' AND table_name=?
         ORDER BY ordinal_position
        """,
        [table_name],
    ).fetchall()
    columns = [str(row[0]) for row in rows if row and row[0] is not None]
    if not columns or len(columns) != len(rows) or len(columns) != len(set(columns)):
        raise RuntimeError("dataset snapshot table schema is unavailable: " + table_name)
    if required_columns and not set(required_columns).issubset(columns):
        raise RuntimeError("dataset snapshot table schema is incomplete: " + table_name)
    return columns


def update_framed(digest, value):
    if not isinstance(value, str):
        raise RuntimeError("dataset snapshot framed value is invalid")
    encoded = value.encode("utf-8")
    digest.update(len(encoded).to_bytes(8, "big"))
    digest.update(encoded)


def update_framed_query_rows(con, digest, sql, params=None):
    cursor = con.execute(sql, params or [])
    while True:
        rows = cursor.fetchmany(2048)
        if not rows:
            break
        for row in rows:
            if len(row) != 1 or row[0] is None:
                raise RuntimeError("dataset snapshot content integrity is unavailable")
            update_framed(digest, str(row[0]))


def normalized_source_signature(con, case_id):
    columns = ordered_table_columns(
        con,
        "fc_transaction_norm",
        {"id", "case_id", "clean_invalid", "clean_failed", "clean_reversal"},
    )
    digest = hashlib.sha256()
    update_framed(digest, NORMALIZED_SOURCE_SIGNATURE_DOMAIN)
    update_framed(digest, case_id)
    for column in columns:
        update_framed(digest, column)
    update_framed_query_rows(
        con,
        digest,
        """
        SELECT to_json(r)
          FROM fc_transaction_norm AS r
         WHERE r.case_id=?
         ORDER BY TRY_CAST(r.id AS BIGINT), to_json(r)
        """,
        [case_id],
    )
    return digest.hexdigest()


def materialization_result_signature(con, case_id):
    digest = hashlib.sha256()
    update_framed(digest, RESULT_SIGNATURE_DOMAIN)
    update_framed(digest, case_id)
    for table_name, order_by, required_columns in RESULT_SIGNATURE_TABLES:
        columns = ordered_table_columns(con, table_name, required_columns)
        update_framed(digest, table_name)
        for column in columns:
            update_framed(digest, column)
        update_framed_query_rows(
            con,
            digest,
            "SELECT to_json(r) FROM \"" + table_name + "\" AS r ORDER BY " + order_by + ", to_json(r)",
        )
    return digest.hexdigest()


def supported_content_type(value):
    data_type = str(value).strip().upper()
    if data_type in {
        "BOOLEAN", "TINYINT", "SMALLINT", "INTEGER", "BIGINT", "HUGEINT",
        "UTINYINT", "USMALLINT", "UINTEGER", "UBIGINT", "FLOAT", "DOUBLE",
        "DATE", "TIME", "TIMESTAMP", "TIMESTAMP_S", "TIMESTAMP_MS", "VARCHAR", "BLOB",
    }:
        return True
    decimal_match = re.fullmatch(r"DECIMAL\(\s*([0-9]+)\s*,\s*([0-9]+)\s*\)", data_type)
    if not decimal_match:
        return False
    precision = int(decimal_match.group(1))
    scale = int(decimal_match.group(2))
    return precision <= 28 and scale <= 28 and scale <= precision


def table_content_manifest(con, table_name, where_sql="", params=None):
    if not re.fullmatch(r"[a-z_][a-z0-9_]*", table_name):
        raise RuntimeError("funds producer table identity is invalid")
    require_base_table(con, table_name)
    columns = con.execute(
        """
        SELECT column_name, data_type, collation_name
          FROM information_schema.columns
         WHERE table_schema='main' AND table_name=?
         ORDER BY ordinal_position
        """,
        [table_name],
    ).fetchall()
    if not columns:
        raise RuntimeError("funds producer table schema is unavailable: " + table_name)
    column_manifest = []
    for column_name, data_type, collation_name in columns:
        column_name = str(column_name)
        data_type = str(data_type)
        if (
            not column_name
            or column_name != column_name.strip()
            or (collation_name is not None and str(collation_name) != "")
            or not supported_content_type(data_type)
        ):
            raise RuntimeError("funds producer table schema is not admitted: " + table_name)
        column_manifest.append({"name": column_name, "type": data_type})
    quoted_columns = [
        '"' + column["name"].replace('"', '""') + '"'
        for column in column_manifest
    ]
    order_sql = ", ".join(column + " NULLS FIRST" for column in quoted_columns)
    sql = "SELECT " + ", ".join(quoted_columns) + " FROM \"" + table_name + "\" AS r"
    if where_sql:
        sql += " WHERE " + where_sql
    sql += " ORDER BY " + order_sql + ", to_json(r)"
    cursor = con.execute(sql, params or [])
    digest = hashlib.sha256()
    digest.update((canonical_json({
        "columns": column_manifest,
        "schemaVersion": 1,
        "table": table_name,
    }) + "\n").encode("utf-8"))
    row_count = 0
    while True:
        rows = cursor.fetchmany(2048)
        if not rows:
            break
        for row in rows:
            digest.update((canonical_json([canonical_hash_value(item) for item in row]) + "\n").encode("utf-8"))
            row_count += 1
    digest.update((canonical_json({"rowCount": row_count}) + "\n").encode("utf-8"))
    return {
        "columns": column_manifest,
        "contentSha256": digest.hexdigest(),
        "rowCount": row_count,
        "table": table_name,
    }


def require_unique_key(con, table_name, invalid_key_sql, group_by_sql):
    invalid = scalar_count(
        con,
        "SELECT COUNT(*) FROM \"" + table_name + "\" WHERE " + invalid_key_sql,
    )
    duplicates = scalar_count(
        con,
        "SELECT COUNT(*) FROM (SELECT 1 FROM \"" + table_name + "\" GROUP BY "
        + group_by_sql + " HAVING COUNT(*)<>1) AS duplicate_keys",
    )
    if invalid != 0 or duplicates != 0:
        raise RuntimeError("funds producer content ordering key is invalid: " + table_name)


def require_unique_content_keys(con, case_id, source_row_count):
    source_ids = con.execute(
        """
        SELECT COUNT(*), COUNT(TRY_CAST(id AS BIGINT)),
               COUNT(DISTINCT TRY_CAST(id AS BIGINT))
          FROM fc_transaction_norm WHERE case_id=?
        """,
        [case_id],
    ).fetchone()
    if not source_ids or tuple(int(value or 0) for value in source_ids) != (
        source_row_count,
        source_row_count,
        source_row_count,
    ):
        raise RuntimeError("funds producer normalized row key is ambiguous")
    require_unique_key(
        con,
        "analysis_txn_detail_idx",
        "id IS NULL OR TRY_CAST(id AS BIGINT) IS NULL",
        "TRY_CAST(id AS BIGINT)",
    )
    require_unique_key(
        con,
        "analysis_txn_daily_agg",
        "FALSE",
        "acct_key, txn_day, cp_key, dc_val",
    )
    require_unique_key(
        con,
        "analysis_txn_keyword_idx",
        "txn_row_id IS NULL OR TRY_CAST(txn_row_id AS BIGINT) IS NULL "
        "OR kind IS NULL OR token IS NULL OR token_order IS NULL",
        "TRY_CAST(txn_row_id AS BIGINT), kind, token, token_order",
    )
    require_unique_key(
        con,
        "analysis_account_dim",
        "account_key IS NULL",
        "account_key",
    )


def materialization_identity(case_id, revision, source_state, raw_sources, normalized_source_sha256):
    source_count, source_max_txn_ts, source_max_id, rejected_count = source_state
    manifest = {
        "acceptedRowCount": source_count - rejected_count,
        "caseId": case_id,
        "normalizedSourceSha256": normalized_source_sha256,
        "rawSources": raw_sources,
        "rejectedRowCount": rejected_count,
        "schemaVersion": MATERIALIZATION_IDENTITY_SCHEMA_VERSION,
        "sourceMaxId": source_max_id,
        "sourceMaxTxnTimestamp": source_max_txn_ts,
        "sourceRevision": revision,
        "sourceRowCount": source_count,
    }
    canonical = json.dumps(manifest, ensure_ascii=False, separators=(",", ":"), sort_keys=True)
    signature = hashlib.sha256(canonical.encode("utf-8")).hexdigest()
    return MATERIALIZATION_IDENTITY_PREFIX + signature, signature


def dataset_snapshot_v1(con, case_id):
    for table_name in (
        "analysis_revision_state",
        "analysis_materialization_meta",
        "analysis_txn_daily_agg",
        "analysis_txn_detail_idx",
        "analysis_txn_keyword_idx",
        "analysis_account_dim",
        "fc_transaction_norm",
        "import_file_log",
    ):
        require_table(con, table_name)
    revision_rows = con.execute(
        "SELECT revision FROM analysis_revision_state WHERE revision_key='stats_flow_source'"
    ).fetchall()
    if len(revision_rows) != 1:
        raise RuntimeError("dataset snapshot source revision is ambiguous")
    revision = required_nonnegative_integer(revision_rows[0][0], "source revision")
    if revision == 0:
        raise RuntimeError("dataset snapshot source revision is unavailable")
    raw_rows = con.execute(
        """
        SELECT file_id, sha256, rows_imported_norm, status, cleaned_status
          FROM import_file_log
         WHERE case_id=? AND kind='fc_transaction'
         ORDER BY file_id
        """,
        [case_id],
    ).fetchall()
    if not raw_rows:
        raise RuntimeError("dataset snapshot has no transaction source manifest")
    raw_sources = []
    seen_file_ids = set()
    for file_id, sha256, rows_imported_norm, status, cleaned_status in raw_rows:
        file_id = str(file_id or "").strip()
        sha256 = str(sha256 or "").strip().lower()
        status = str(status or "").strip()
        cleaned_status = str(cleaned_status or "").strip().lower()
        rows_imported_norm = required_nonnegative_integer(
            rows_imported_norm,
            "transaction source rows_imported_norm",
        )
        if (
            not file_id
            or file_id in seen_file_ids
            or not re.fullmatch(r"[a-f0-9]{64}", sha256)
            or status != "已完成"
            or cleaned_status != "done"
        ):
            raise RuntimeError("dataset snapshot transaction source manifest is incomplete")
        seen_file_ids.add(file_id)
        raw_sources.append({
            "cleanedStatus": cleaned_status,
            "fileId": file_id,
            "rowsImportedNorm": rows_imported_norm,
            "sha256": sha256,
            "status": status,
        })
    normalization_columns = {
        str(row[0])
        for row in con.execute(
            """
            SELECT column_name
              FROM information_schema.columns
             WHERE table_schema='main' AND table_name='fc_transaction_norm'
            """
        ).fetchall()
    }
    required_clean_flags = {"clean_invalid", "clean_failed", "clean_reversal"}
    if not required_clean_flags.issubset(normalization_columns):
        raise RuntimeError("dataset snapshot transaction cleaning state schema is unavailable")
    source_state_row = con.execute(
        """
        SELECT COUNT(*), COUNT(TRY_CAST(id AS BIGINT)),
               COUNT(DISTINCT TRY_CAST(id AS BIGINT)),
               COALESCE(CAST(MAX(txn_ts) AS VARCHAR), ''), COALESCE(MAX(id), 0),
               SUM(CASE WHEN clean_invalid IS NULL OR clean_invalid NOT IN (0, 1)
                              OR clean_failed IS NULL OR clean_failed NOT IN (0, 1)
                              OR clean_reversal IS NULL OR clean_reversal NOT IN (0, 1)
                        THEN 1 ELSE 0 END),
               SUM(CASE WHEN clean_invalid=1 OR clean_failed=1 OR clean_reversal=1
                        THEN 1 ELSE 0 END)
          FROM fc_transaction_norm
         WHERE case_id=?
        """,
        [case_id],
    ).fetchone()
    if not source_state_row:
        raise RuntimeError("dataset snapshot transaction source is unavailable")
    source_count = int(source_state_row[0] or 0)
    source_id_count = int(source_state_row[1] or 0)
    source_distinct_id_count = int(source_state_row[2] or 0)
    source_max_txn_ts = str(source_state_row[3] or "")
    source_max_id = int(source_state_row[4] or 0)
    invalid_flag_count = int(source_state_row[5] or 0)
    rejected_count = int(source_state_row[6] or 0)
    if source_count != source_id_count or source_count != source_distinct_id_count:
        raise RuntimeError("dataset snapshot transaction row lineage is ambiguous")
    if invalid_flag_count != 0:
        raise RuntimeError("dataset snapshot transaction rejection state is invalid")
    accepted_source_count = source_count - rejected_count
    detail_count = scalar_count(con, "SELECT COUNT(*) FROM analysis_txn_detail_idx")
    if accepted_source_count != detail_count:
        raise RuntimeError("dataset snapshot transaction index is not complete")
    normalized_source_sha256 = normalized_source_signature(con, case_id)
    expected_materialization_name, expected_source_signature = materialization_identity(
        case_id,
        revision,
        (source_count, source_max_txn_ts, source_max_id, rejected_count),
        raw_sources,
        normalized_source_sha256,
    )
    materialization_meta_columns = set(ordered_table_columns(con, "analysis_materialization_meta"))
    if "result_signature" not in materialization_meta_columns:
        raise RuntimeError("dataset snapshot materialization result signature is unavailable")
    materializations = con.execute(
        """
        SELECT agg_name, agg_version, case_id, identity_schema_version, source_revision,
               source_row_count, COALESCE(CAST(source_max_txn_ts AS VARCHAR), ''),
               source_max_id, source_signature, result_signature, row_count
          FROM analysis_materialization_meta
         WHERE starts_with(agg_name, 'txn_daily_')
         ORDER BY agg_name
        """
    ).fetchall()
    if not materializations:
        raise RuntimeError("dataset snapshot materialization authority is unavailable")
    if len(materializations) != 1:
        raise RuntimeError("dataset snapshot materialization authority is ambiguous")
    materialization = materializations[0]
    materialization_name = str(materialization[0] or "").strip()
    agg_version = int(materialization[1] or 0)
    materialized_case_id = str(materialization[2] or "").strip()
    identity_schema_version = int(materialization[3] or 0)
    materialized_revision = int(materialization[4] or 0)
    materialized_source_count = int(materialization[5] or 0)
    materialized_max_txn_ts = str(materialization[6] or "")
    materialized_max_id = int(materialization[7] or 0)
    materialized_source_signature = str(materialization[8] or "").strip().lower()
    materialized_result_signature = str(materialization[9] or "")
    materialized_row_count = required_nonnegative_integer(
        materialization[10],
        "materialized aggregate row_count",
    )
    if (
        agg_version != MATERIALIZATION_VERSION
        or identity_schema_version != MATERIALIZATION_IDENTITY_SCHEMA_VERSION
        or materialized_case_id != case_id
        or materialization_name != expected_materialization_name
        or materialized_source_signature != expected_source_signature
        or not re.fullmatch(r"[a-f0-9]{64}", materialized_result_signature)
        or not re.fullmatch(r"txn_daily_snapshot:v12:[a-f0-9]{64}", materialization_name)
        or materialized_revision != revision
        or materialized_source_count != source_count
        or materialized_max_txn_ts != source_max_txn_ts
        or materialized_max_id != source_max_id
    ):
        raise RuntimeError("dataset snapshot materialization is stale")
    result_signature = materialization_result_signature(con, case_id)
    if materialized_result_signature != result_signature:
        raise RuntimeError("dataset snapshot materialization result is stale")
    columns = con.execute(
        """
        SELECT column_name, data_type
          FROM information_schema.columns
         WHERE table_schema='main' AND table_name='analysis_txn_detail_idx'
         ORDER BY ordinal_position
        """
    ).fetchall()
    if not columns:
        raise RuntimeError("dataset snapshot transaction index schema is unavailable")
    column_names = {str(row[0]) for row in columns}
    if not {"amount", "amount_source_present", "amount_parse_failed", "dc_val"}.issubset(column_names):
        raise RuntimeError("dataset snapshot transaction index amount lineage is unavailable")
    aggregate_columns = {
        str(row[0])
        for row in con.execute(
            """
            SELECT column_name
              FROM information_schema.columns
             WHERE table_schema='main' AND table_name='analysis_txn_daily_agg'
            """
        ).fetchall()
    }
    required_aggregate_columns = {
        "txn_count",
        "amount_source_present_count",
        "amount_valid_count",
        "amount_missing_count",
        "amount_parse_failed_count",
        "amt_sum",
        "dc_val",
    }
    if not required_aggregate_columns.issubset(aggregate_columns):
        raise RuntimeError("dataset snapshot aggregate amount lineage is unavailable")
    detail_quality = con.execute(
        """
        SELECT
          COUNT(*),
          SUM(CASE WHEN amount_source_present=1 THEN 1 ELSE 0 END),
          SUM(CASE WHEN amount IS NOT NULL AND isfinite(amount)
                        AND amount_source_present=1 AND amount_parse_failed=0 THEN 1 ELSE 0 END),
          SUM(CASE WHEN amount IS NULL
                        AND amount_source_present=0 AND amount_parse_failed=0 THEN 1 ELSE 0 END),
          SUM(CASE WHEN amount IS NULL
                        AND amount_source_present=1 AND amount_parse_failed=1 THEN 1 ELSE 0 END),
          SUM(CASE WHEN dc_val IN ('进','出') THEN 1 ELSE 0 END)
        FROM analysis_txn_detail_idx
        """
    ).fetchone()
    aggregate_quality = con.execute(
        """
        SELECT
          COALESCE(SUM(txn_count), 0),
          COALESCE(SUM(amount_source_present_count), 0),
          COALESCE(SUM(amount_valid_count), 0),
          COALESCE(SUM(amount_missing_count), 0),
          COALESCE(SUM(amount_parse_failed_count), 0),
          COALESCE(SUM(CASE WHEN dc_val IN ('进','出') THEN txn_count ELSE 0 END), 0),
          SUM(CASE
                WHEN txn_count < 0 OR amount_source_present_count < 0 OR amount_valid_count < 0
                  OR amount_missing_count < 0 OR amount_parse_failed_count < 0
                  OR txn_count <> amount_valid_count + amount_missing_count + amount_parse_failed_count
                  OR amount_source_present_count <> amount_valid_count + amount_parse_failed_count
                  OR (amount_valid_count = txn_count AND (amt_sum IS NULL OR NOT isfinite(amt_sum)))
                  OR (amount_valid_count <> txn_count AND amt_sum IS NOT NULL)
                THEN 1 ELSE 0 END)
        FROM analysis_txn_daily_agg
        """
    ).fetchone()
    if not detail_quality or not aggregate_quality:
        raise RuntimeError("dataset snapshot amount coverage is unavailable")
    detail_quality = tuple(int(value or 0) for value in detail_quality)
    aggregate_quality = tuple(int(value or 0) for value in aggregate_quality)
    detail_total, detail_present, detail_valid, detail_missing, detail_failed, detail_direction = detail_quality
    aggregate_total, aggregate_present, aggregate_valid, aggregate_missing, aggregate_failed, aggregate_direction, aggregate_invalid = aggregate_quality
    if (
        detail_total != detail_count
        or detail_valid + detail_missing + detail_failed != detail_total
        or detail_present != detail_valid + detail_failed
        or aggregate_invalid != 0
        or aggregate_total != detail_total
        or aggregate_present != detail_present
        or aggregate_valid != detail_valid
        or aggregate_missing != detail_missing
        or aggregate_failed != detail_failed
        or aggregate_direction != detail_direction
    ):
        raise RuntimeError("dataset snapshot amount coverage is inconsistent")
    amount_coverage = {
        "amountMissingRows": detail_missing,
        "amountParseFailedRows": detail_failed,
        "amountSourcePresentRows": detail_present,
        "amountValidRows": detail_valid,
        "complete": detail_total > 0 and detail_valid == detail_total and detail_direction == detail_total,
        "directionCoveredRows": detail_direction,
        "totalRows": detail_total,
    }
    manifest = {
        "amountCoverage": amount_coverage,
        "caseId": case_id,
        "normalizedSourceSha256": normalized_source_sha256,
        "rawSources": raw_sources,
        "resultSignature": result_signature,
        "schemaVersion": 1,
        "sourceRevision": revision,
        "transactionIndex": {
            "columns": [{"name": str(row[0]), "type": str(row[1])} for row in columns],
            "rowCount": detail_count,
            "sourceRevision": materialized_revision,
            "table": "analysis_txn_detail_idx",
        },
        "transactionSource": {
            "acceptedRowCount": accepted_source_count,
            "materializationVersion": agg_version,
            "materializationName": materialization_name,
            "materializedRowCount": materialized_row_count,
            "maxId": materialized_max_id,
            "maxTxnTimestamp": materialized_max_txn_ts or None,
            "rejectedRowCount": rejected_count,
            "rowCount": source_count,
            "table": "fc_transaction_norm",
        },
    }
    canonical = json.dumps(manifest, ensure_ascii=False, separators=(",", ":"), sort_keys=True)
    digest = hashlib.sha256(canonical.encode("utf-8")).hexdigest()
    return {
        "acceptedSourceCount": accepted_source_count,
        "aggregateMetadataRowCount": materialized_row_count,
        "detailRowCount": detail_count,
        "legacyManifest": manifest,
        "normalizedSourceSha256": normalized_source_sha256,
        "rawSources": raw_sources,
        "rejectedRowCount": rejected_count,
        "resultSignature": result_signature,
        "snapshotContract": "analytix_duckdb_dataset_snapshot_v1",
        "snapshotId": "dsv1_" + digest,
        "sourceRevision": revision,
        "sourceRowCount": source_count,
    }


def exact_raw_sources(con, case_id):
    rows = con.execute(
        """
        SELECT file_id, sha256, rows_imported_norm, status, cleaned_status
          FROM import_file_log
         WHERE case_id=? AND kind='fc_transaction'
         ORDER BY file_id
        """,
        [case_id],
    ).fetchall()
    if not rows:
        raise RuntimeError("funds producer content has no transaction source manifest")
    raw_sources = []
    seen_file_ids = set()
    row_count = 0
    for file_id, sha256, rows_imported_norm, status, cleaned_status in rows:
        if (
            not isinstance(file_id, str)
            or not file_id
            or file_id != file_id.strip()
            or file_id in seen_file_ids
            or not isinstance(sha256, str)
            or not re.fullmatch(r"[a-f0-9]{64}", sha256)
            or rows_imported_norm is None
            or isinstance(rows_imported_norm, bool)
            or not isinstance(rows_imported_norm, int)
            or rows_imported_norm < 0
            or status != "已完成"
            or cleaned_status != "done"
        ):
            raise RuntimeError("funds producer transaction source manifest is incomplete")
        seen_file_ids.add(file_id)
        row_count += rows_imported_norm
        raw_sources.append({
            "cleanedStatus": cleaned_status,
            "fileId": file_id,
            "rowsImportedNorm": rows_imported_norm,
            "sha256": sha256,
            "status": status,
        })
    return raw_sources, row_count


def funds_producer_content_manifest_v1(con, case_id):
    if not case_id or case_id != case_id.strip():
        raise RuntimeError("funds producer content manifest v1 input is invalid")
    for table_name in (
        "fc_transaction_norm",
        "import_file_log",
        "analysis_txn_detail_idx",
        "analysis_txn_daily_agg",
        "analysis_txn_keyword_idx",
        "analysis_account_dim",
        FUNDS_PRODUCER_CONTENT_MANIFEST_V1_TABLE,
    ):
        require_base_table(con, table_name)
    module_version = str(getattr(duckdb, "__version__", ""))
    engine_row = con.execute("SELECT version()").fetchone()
    engine_version = str(engine_row[0]) if engine_row and engine_row[0] is not None else ""
    if module_version != REQUIRED_DUCKDB_MODULE_VERSION or engine_version != REQUIRED_DUCKDB_ENGINE_VERSION:
        raise RuntimeError("funds producer DuckDB version is not admitted")

    legacy = dataset_snapshot_v1(con, case_id)
    source_row_count = legacy["sourceRowCount"]
    source_scope = con.execute(
        """
        SELECT COUNT(*),
               SUM(CASE WHEN case_id=? THEN 1 ELSE 0 END),
               SUM(CASE WHEN case_id IS NULL OR TRIM(CAST(case_id AS VARCHAR))='' THEN 1 ELSE 0 END)
          FROM fc_transaction_norm
        """,
        [case_id],
    ).fetchone()
    if not source_scope or tuple(int(value or 0) for value in source_scope) != (
        source_row_count,
        source_row_count,
        0,
    ):
        raise RuntimeError("funds producer content contains cross-case transaction source rows")

    raw_sources, raw_source_row_count = exact_raw_sources(con, case_id)
    import_scope = con.execute(
        """
        SELECT COUNT(*), SUM(CASE WHEN case_id=? THEN 1 ELSE 0 END)
          FROM import_file_log WHERE kind='fc_transaction'
        """,
        [case_id],
    ).fetchone()
    if (
        not import_scope
        or int(import_scope[0] or 0) != len(raw_sources)
        or int(import_scope[1] or 0) != len(raw_sources)
    ):
        raise RuntimeError("funds producer content contains cross-case transaction source manifests")
    if raw_source_row_count != source_row_count:
        raise RuntimeError("funds producer transaction source manifest row count is unverified")
    raw_manifest_rows = con.execute(
        "SELECT raw_manifest_sha256 FROM \""
        + FUNDS_PRODUCER_CONTENT_MANIFEST_V1_TABLE
        + "\" WHERE case_id=? ORDER BY case_id",
        [case_id],
    ).fetchall()
    if (
        len(raw_manifest_rows) != 1
        or not isinstance(raw_manifest_rows[0][0], str)
        or not re.fullmatch(r"[a-f0-9]{64}", raw_manifest_rows[0][0])
        or raw_manifest_rows[0][0] == sha256_canonical(raw_sources)
    ):
        raise RuntimeError("typed raw artifact manifest binding is invalid")
    raw_artifact_manifest_sha256 = raw_manifest_rows[0][0]

    rejected_row_count = legacy["rejectedRowCount"]
    if rejected_row_count < 0 or rejected_row_count > source_row_count:
        raise RuntimeError("funds producer transaction rejection state is invalid")
    require_unique_content_keys(con, case_id, source_row_count)

    normalized = table_content_manifest(con, "fc_transaction_norm", '"case_id"=?', [case_id])
    detail = table_content_manifest(con, "analysis_txn_detail_idx")
    aggregate = table_content_manifest(con, "analysis_txn_daily_agg")
    keyword = table_content_manifest(con, "analysis_txn_keyword_idx")
    account = table_content_manifest(con, "analysis_account_dim")
    if (
        normalized["rowCount"] != source_row_count
        or detail["rowCount"] != source_row_count - rejected_row_count
    ):
        raise RuntimeError("funds producer content row coverage is inconsistent")

    payload = {
        "acceptedRowCount": detail["rowCount"],
        "accountContentSha256": account["contentSha256"],
        "accountRowCount": account["rowCount"],
        "aggregateContentSha256": aggregate["contentSha256"],
        "aggregateRowCount": aggregate["rowCount"],
        "canonicalEncoder": DUCKDB_CONTENT_MANIFEST_ALGORITHM,
        "caseId": case_id,
        "contract": FUNDS_PRODUCER_CONTENT_MANIFEST_V1_CONTRACT,
        "detailContentSha256": detail["contentSha256"],
        "detailRowCount": detail["rowCount"],
        "duckdbVersion": engine_version,
        "duplicateRowCount": 0,
        "keywordContentSha256": keyword["contentSha256"],
        "keywordRowCount": keyword["rowCount"],
        "normalizedContentSha256": normalized["contentSha256"],
        "normalizedRowCount": normalized["rowCount"],
        "producerComponentId": PRODUCER_COMPONENT_ID,
        "producerComponentVersion": PRODUCER_COMPONENT_VERSION,
        "producerOperation": PRODUCER_OPERATION,
        "producerOperationSchemaHash": hashlib.sha256(PRODUCER_OPERATION_SCHEMA.encode("utf-8")).hexdigest(),
        "rawManifestSha256": raw_artifact_manifest_sha256,
        "rejectedRowCount": rejected_row_count,
        "schemaVersion": FUNDS_PRODUCER_CONTENT_MANIFEST_V1_SCHEMA_VERSION,
        "sourceRevision": legacy["sourceRevision"],
    }
    canonical_payload = canonical_json(payload).encode("utf-8")
    manifest_sha256 = hashlib.sha256(canonical_payload).hexdigest()
    producer_content_digest = hashlib.sha256(
        FUNDS_PRODUCER_CONTENT_DIGEST_DOMAIN + canonical_payload
    ).hexdigest()
    producer_content_id = FUNDS_PRODUCER_CONTENT_ID_PREFIX + producer_content_digest

    persisted_columns = (
        "schema_version", "contract", "case_id", "source_revision",
        "producer_component_id", "producer_component_version", "producer_operation",
        "producer_operation_schema_hash", "duckdb_version", "canonical_encoder",
        "raw_manifest_sha256", "normalized_content_sha256", "detail_content_sha256",
        "aggregate_content_sha256", "keyword_content_sha256", "account_content_sha256",
        "normalized_row_count", "accepted_row_count", "rejected_row_count", "duplicate_row_count",
        "detail_row_count", "aggregate_row_count", "keyword_row_count", "account_row_count",
        "manifest_sha256", "producer_content_digest", "producer_content_id",
    )
    persisted_rows = con.execute(
        "SELECT " + ", ".join(persisted_columns)
        + " FROM \"" + FUNDS_PRODUCER_CONTENT_MANIFEST_V1_TABLE + "\" ORDER BY case_id"
    ).fetchall()
    if len(persisted_rows) != 1:
        raise RuntimeError("funds producer content manifest v1 is unavailable or ambiguous")
    persisted = dict(zip(persisted_columns, persisted_rows[0]))
    expected_persisted = {
        "schema_version": payload["schemaVersion"],
        "contract": payload["contract"],
        "case_id": payload["caseId"],
        "source_revision": payload["sourceRevision"],
        "producer_component_id": payload["producerComponentId"],
        "producer_component_version": payload["producerComponentVersion"],
        "producer_operation": payload["producerOperation"],
        "producer_operation_schema_hash": payload["producerOperationSchemaHash"],
        "duckdb_version": payload["duckdbVersion"],
        "canonical_encoder": payload["canonicalEncoder"],
        "raw_manifest_sha256": payload["rawManifestSha256"],
        "normalized_content_sha256": payload["normalizedContentSha256"],
        "detail_content_sha256": payload["detailContentSha256"],
        "aggregate_content_sha256": payload["aggregateContentSha256"],
        "keyword_content_sha256": payload["keywordContentSha256"],
        "account_content_sha256": payload["accountContentSha256"],
        "normalized_row_count": payload["normalizedRowCount"],
        "accepted_row_count": payload["acceptedRowCount"],
        "rejected_row_count": payload["rejectedRowCount"],
        "duplicate_row_count": payload["duplicateRowCount"],
        "detail_row_count": payload["detailRowCount"],
        "aggregate_row_count": payload["aggregateRowCount"],
        "keyword_row_count": payload["keywordRowCount"],
        "account_row_count": payload["accountRowCount"],
        "manifest_sha256": manifest_sha256,
        "producer_content_digest": producer_content_digest,
        "producer_content_id": producer_content_id,
    }
    if persisted != expected_persisted:
        raise RuntimeError("funds producer content manifest v1 is stale")
    if (
        payload["acceptedRowCount"] + payload["rejectedRowCount"] + payload["duplicateRowCount"]
        != payload["normalizedRowCount"]
        or payload["acceptedRowCount"] != payload["detailRowCount"]
    ):
        raise RuntimeError("funds producer content manifest v1 is invalid")
    return {
        "contract": FUNDS_PRODUCER_CONTENT_MANIFEST_V1_CONTRACT,
        "manifestSha256": manifest_sha256,
        "payload": payload,
        "producerContentId": producer_content_id,
    }


def dataset_snapshot_v2(con, case_id):
    require_base_table(con, DATASET_SNAPSHOT_MANIFEST_V2_TABLE)
    require_base_table(con, "analysis_txn_daily_agg")
    legacy = dataset_snapshot_v1(con, case_id)
    source_scope = con.execute(
        """
        SELECT COUNT(*),
               SUM(CASE WHEN case_id=? THEN 1 ELSE 0 END),
               SUM(CASE WHEN case_id IS NULL OR TRIM(CAST(case_id AS VARCHAR))='' THEN 1 ELSE 0 END)
          FROM fc_transaction_norm
        """,
        [case_id],
    ).fetchone()
    if (
        not source_scope
        or int(source_scope[0]) != legacy["sourceRowCount"]
        or int(source_scope[1] or 0) != legacy["sourceRowCount"]
        or int(source_scope[2] or 0) != 0
    ):
        raise RuntimeError("dataset snapshot contains cross-case transaction source rows")
    import_scope = con.execute(
        """
        SELECT COUNT(*), SUM(CASE WHEN case_id=? THEN 1 ELSE 0 END)
          FROM import_file_log
         WHERE kind='fc_transaction'
        """,
        [case_id],
    ).fetchone()
    if (
        not import_scope
        or int(import_scope[0]) != len(legacy["rawSources"])
        or int(import_scope[1] or 0) != len(legacy["rawSources"])
    ):
        raise RuntimeError("dataset snapshot contains cross-case transaction source manifests")
    if sum(item["rowsImportedNorm"] for item in legacy["rawSources"]) != legacy["sourceRowCount"]:
        raise RuntimeError("dataset snapshot transaction source manifest row count is unverified")

    normalized = table_content_manifest(
        con,
        "fc_transaction_norm",
        '"case_id"=?',
        [case_id],
    )
    detail = table_content_manifest(con, "analysis_txn_detail_idx")
    aggregate = table_content_manifest(con, "analysis_txn_daily_agg")
    if normalized["rowCount"] != legacy["sourceRowCount"]:
        raise RuntimeError("dataset snapshot normalized content row count is inconsistent")
    if detail["rowCount"] != legacy["detailRowCount"]:
        raise RuntimeError("dataset snapshot detail content row count is inconsistent")
    if aggregate["rowCount"] != legacy["aggregateMetadataRowCount"]:
        raise RuntimeError("dataset snapshot materialized aggregate row_count is unverified")

    manifest_rows = con.execute(
        """
        SELECT manifest_schema_version, case_id, source_revision,
               raw_manifest_sha256, normalized_content_sha256,
               detail_content_sha256, aggregate_content_sha256,
               normalized_row_count, detail_row_count, aggregate_row_count
          FROM analysis_dataset_snapshot_manifest_v2
         ORDER BY case_id
        """
    ).fetchall()
    if len(manifest_rows) != 1:
        raise RuntimeError("dataset snapshot manifest v2 authority is unavailable or ambiguous")
    persisted = manifest_rows[0]
    persisted_schema_version = required_nonnegative_integer(
        persisted[0],
        "manifest v2 schema version",
    )
    persisted_case_id = str(persisted[1] or "").strip()
    persisted_revision = required_nonnegative_integer(persisted[2], "manifest v2 source revision")
    persisted_roots = [str(value or "").strip().lower() for value in persisted[3:7]]
    persisted_counts = [
        required_nonnegative_integer(value, "manifest v2 row count")
        for value in persisted[7:10]
    ]
    raw_manifest_sha256 = sha256_canonical(legacy["rawSources"])
    expected_roots = [
        raw_manifest_sha256,
        normalized["contentSha256"],
        detail["contentSha256"],
        aggregate["contentSha256"],
    ]
    expected_counts = [
        normalized["rowCount"],
        detail["rowCount"],
        aggregate["rowCount"],
    ]
    if (
        persisted_schema_version != DATASET_SNAPSHOT_MANIFEST_V2_SCHEMA_VERSION
        or persisted_case_id != case_id
        or persisted_revision != legacy["sourceRevision"]
        or any(not re.fullmatch(r"[a-f0-9]{64}", value) for value in persisted_roots)
        or persisted_roots != expected_roots
        or persisted_counts != expected_counts
    ):
        raise RuntimeError("dataset snapshot manifest v2 content authority is stale")

    manifest = {
        "aggregate": aggregate,
        "caseId": case_id,
        "detail": detail,
        "legacyMaterializationSnapshotId": legacy["snapshotId"],
        "normalizedSource": normalized,
        "normalizedSourceSha256": legacy["normalizedSourceSha256"],
        "rawManifestSha256": raw_manifest_sha256,
        "rawSources": legacy["rawSources"],
        "resultSignature": legacy["resultSignature"],
        "schemaVersion": DATASET_SNAPSHOT_MANIFEST_V2_SCHEMA_VERSION,
        "sourceRevision": legacy["sourceRevision"],
    }
    return {
        "manifest": manifest,
        "snapshotContract": "analytix_duckdb_dataset_snapshot_v2",
        "snapshotId": "dsv2_" + sha256_canonical(manifest),
    }


payload = json.loads(sys.stdin.read())
policy_sql = str(payload.get("policy_sql") or "").strip()
if policy_sql:
    try:
        statements = duckdb.extract_statements(policy_sql)
    except Exception as exc:
        raise RuntimeError("DuckDB parser rejected policy SQL") from exc
    if len(statements) != 1 or str(statements[0].type) != "StatementType.SELECT":
        raise RuntimeError("DuckDB policy requires exactly one SELECT statement")

max_limit = 5000 if payload.get("metadata_mode") else 500
limit = max(1, min(int(payload.get("row_limit") or 100), max_limit))
con = duckdb.connect(payload["db_path"], read_only=True)
try:
    con.execute("SET autoinstall_known_extensions=false")
    con.execute("SET autoload_known_extensions=false")
    con.execute("SET enable_external_access=false")
    con.execute("SET TimeZone='UTC'")
    con.execute("BEGIN TRANSACTION")
    observed_snapshot_id = ""
    snapshot_contract = ""
    snapshot_factual_ready = False
    snapshot_blocker = ""
    snapshot_manifest_schema_version = 0
    producer_content_contract = ""
    producer_content_id = ""
    producer_manifest_sha256 = ""
    producer_manifest_schema_version = 0
    if payload.get("snapshot_case_id"):
        snapshot_case_id = str(payload["snapshot_case_id"]).strip()
        snapshot_mode = str(payload.get("snapshot_mode") or "legacy_exact_count").strip()
        if snapshot_mode == "require_v2":
            raise RuntimeError("local dataset snapshot v2 authority is retired; host authority is required")
        elif snapshot_mode == "probe_v2_with_legacy_fallback":
            if table_exists(con, FUNDS_PRODUCER_CONTENT_MANIFEST_V1_TABLE):
                producer = funds_producer_content_manifest_v1(con, snapshot_case_id)
                producer_content_contract = producer["contract"]
                producer_content_id = producer["producerContentId"]
                producer_manifest_sha256 = producer["manifestSha256"]
                producer_manifest_schema_version = FUNDS_PRODUCER_CONTENT_MANIFEST_V1_SCHEMA_VERSION
            else:
                snapshot = dataset_snapshot_v1(con, snapshot_case_id)
                observed_snapshot_id = snapshot["snapshotId"]
                snapshot_contract = snapshot["snapshotContract"]
                snapshot_blocker = (
                    "local_dataset_snapshot_v2_retired"
                    if table_exists(con, DATASET_SNAPSHOT_MANIFEST_V2_TABLE)
                    else "funds_producer_content_manifest_v1_unavailable"
                )
        elif snapshot_mode == "producer_content_v1":
            producer = funds_producer_content_manifest_v1(con, snapshot_case_id)
            producer_content_contract = producer["contract"]
            producer_content_id = producer["producerContentId"]
            producer_manifest_sha256 = producer["manifestSha256"]
            producer_manifest_schema_version = FUNDS_PRODUCER_CONTENT_MANIFEST_V1_SCHEMA_VERSION
        elif snapshot_mode == "legacy_exact_count":
            snapshot = dataset_snapshot_v1(con, snapshot_case_id)
            observed_snapshot_id = snapshot["snapshotId"]
            snapshot_contract = snapshot["snapshotContract"]
            snapshot_blocker = "legacy_exact_count_quarantine"
        else:
            raise RuntimeError("dataset snapshot mode is invalid")
    expected_snapshot_id = str(payload.get("expected_dataset_snapshot_id") or "").strip()
    if expected_snapshot_id and observed_snapshot_id != expected_snapshot_id:
        raise RuntimeError("dataset snapshot changed before SQL execution")
    expected_producer_content_id = str(payload.get("expected_producer_content_id") or "").strip()
    if expected_producer_content_id and producer_content_id != expected_producer_content_id:
        raise RuntimeError("funds producer content changed before SQL execution")
    if policy_sql:
        requested_tables = payload.get("policy_base_tables") or []
        requested_functions = payload.get("policy_functions") or []
        if (
            not isinstance(requested_tables, list)
            or not isinstance(requested_functions, list)
            or any(not isinstance(item, str) or not re.fullmatch(r"(?:analysis_[a-z0-9_]*|fc_[a-z0-9_]*_norm)", item) for item in requested_tables)
            or any(not isinstance(item, str) or not re.fullmatch(r"[a-z_][a-z0-9_]*", item) for item in requested_functions)
        ):
            raise RuntimeError("DuckDB policy manifest is invalid")
        available_tables = {
            str(row[0]).lower(): str(row[1]).upper()
            for row in con.execute(
                "SELECT table_name, table_type FROM information_schema.tables WHERE table_schema='main'"
            ).fetchall()
        }
        if any(table not in available_tables or available_tables[table] not in ("BASE TABLE", "VIEW") for table in requested_tables):
            raise RuntimeError("DuckDB policy relation manifest is stale or unavailable")
        if requested_functions:
            parser_intrinsics = {"coalesce", "if", "ifnull"}
            safe_internal_macros = {"nullif"}
            placeholders = ",".join("?" for _ in requested_functions)
            rows = con.execute(
                f"""
                SELECT function_name, function_type, internal, has_side_effects
                  FROM duckdb_functions()
                 WHERE function_name IN ({placeholders})
                """,
                requested_functions,
            ).fetchall()
            safe_functions = {name: True for name in requested_functions}
            seen_functions = set()
            for function_name, function_type, internal, has_side_effects in rows:
                function_name = str(function_name)
                seen_functions.add(function_name)
                allowed_type = str(function_type) in ("scalar", "aggregate") or (
                    function_name in safe_internal_macros and str(function_type) == "macro"
                )
                if not bool(internal) or bool(has_side_effects) or not allowed_type:
                    safe_functions[function_name] = False
            for function_name in requested_functions:
                if function_name not in seen_functions and function_name not in parser_intrinsics:
                    safe_functions[function_name] = False
            if any(safe_functions.get(name) is not True for name in requested_functions):
                raise RuntimeError("DuckDB policy function manifest is unsafe or unavailable")
    execution_sql = "EXPLAIN " + policy_sql.rstrip().rstrip(";") if payload.get("policy_explain_only") else payload["sql"]
    cursor = con.execute(execution_sql)
    columns = [desc[0] for desc in cursor.description or []]
    column_types = [str(desc[1]).upper() for desc in cursor.description or []]
    rows = cursor.fetchmany(limit + 1)
finally:
    try:
        con.execute("ROLLBACK")
    except Exception:
        pass
    con.close()

records = [
    {
        columns[index]: normalize_result_value(columns[index], column_types[index], value)
        for index, value in enumerate(row)
    }
    for row in rows[:limit]
]
print(json.dumps({
    "columns": columns,
    "column_types": column_types,
    "records": records,
    "row_count": len(records),
    "truncated": len(rows) > limit,
    "snapshot_contract": snapshot_contract,
    "snapshot_factual_ready": snapshot_factual_ready,
    "snapshot_blocker": snapshot_blocker,
    "snapshot_manifest_schema_version": snapshot_manifest_schema_version,
    "observed_dataset_snapshot_id": observed_snapshot_id,
    "producer_content_contract": producer_content_contract,
    "producer_content_id": producer_content_id,
    "producer_manifest_sha256": producer_manifest_sha256,
    "producer_manifest_schema_version": producer_manifest_schema_version,
}, ensure_ascii=False, allow_nan=False))
`;

function currentModuleRoot() {
  return path.resolve(path.dirname(fileURLToPath(import.meta.url)), "..", "..", "..");
}

function pythonCandidates(env = process.env) {
  const candidates = [
    env.ANALYTIX_FUNDS_DUCKDB_PYTHON,
    env.ANALYTIX_BACKEND_PYTHON,
    env.PYTHON,
    path.join(currentModuleRoot(), ".venv", process.platform === "win32" ? "Scripts/python.exe" : "bin/python"),
    "python3",
    "python"
  ].map(text).filter(Boolean);
  return [...new Set(candidates)];
}

function resolveCommandFromPath(command, env = process.env) {
  if (!command || command.includes(path.sep)) return command;
  const extensions = process.platform === "win32"
    ? text(env.PATHEXT || ".EXE;.CMD;.BAT").split(";").filter(Boolean)
    : [""];
  for (const directory of text(env.PATH).split(path.delimiter).filter(Boolean)) {
    for (const extension of extensions) {
      const candidate = path.join(directory, command + extension);
      if (fs.existsSync(candidate)) return candidate;
    }
  }
  return command;
}

export function canonicalPythonCandidates(env = process.env) {
  const seen = new Set();
  const candidates = [];
  for (const configured of pythonCandidates(env)) {
    const resolved = resolveCommandFromPath(configured, env);
    let key = resolved;
    try {
      key = fs.realpathSync(resolved);
    } catch {
      key = path.resolve(resolved);
    }
    if (seen.has(key)) continue;
    seen.add(key);
    candidates.push(resolved);
  }
  return candidates;
}

const DUCKDB_PYTHON_CHILD_ENV_ALLOWLIST = Object.freeze([
  "PATH",
  "Path",
  "PATHEXT",
  "SystemRoot",
  "SYSTEMROOT",
  "WINDIR",
  "TEMP",
  "TMP",
  "TMPDIR",
  "LANG",
  "LC_ALL",
  "LC_CTYPE",
  "TZ"
]);

export function duckdbPythonChildEnv(env = process.env) {
  const childEnv = {};
  for (const key of DUCKDB_PYTHON_CHILD_ENV_ALLOWLIST) {
    if (env[key] === undefined || env[key] === null) continue;
    childEnv[key] = String(env[key]);
  }
  childEnv.PYTHONDONTWRITEBYTECODE = "1";
  childEnv.PYTHONIOENCODING = "utf-8";
  childEnv.PYTHONNOUSERSITE = "1";
  childEnv.PYTHONUTF8 = "1";
  return childEnv;
}

function caseDbPath(caseId, env = process.env) {
  return resolveCaseDuckdbPath(caseId, env);
}

function localCaseId(args = {}, env = process.env) {
  const projectContext = resolveCaseProjectContext(args, env);
  if (!projectContext) return "";
  assertExplicitCaseMatchesProject(args, projectContext);
  return projectContext.case_id || explicitCaseIdFromArgs(args);
}

function stripSqlComments(sql) {
  return stripDuckdbSqlComments(text(sql));
}

function stripTrailingSemicolon(sql) {
  return stripSqlComments(sql).replace(/;\s*$/u, "").trim();
}

function unquoteIdentifier(identifier) {
  return text(identifier).replace(/^"+|"+$/gu, "").replace(/""/gu, "\"");
}

function simpleTableName(value) {
  const table = unquoteIdentifier(value);
  return table.split(".").pop().toLowerCase();
}

function isAllowedWorkbenchTableName(tableName) {
  return isAllowedDuckdbWorkbenchTable(simpleTableName(tableName));
}

function requireSqlIdentifier(value, label = "identifier") {
  const identifier = unquoteIdentifier(value);
  if (!SQL_IDENTIFIER_PATTERN.test(identifier)) {
    throw new Error(`Local DuckDB workbench rejected unsafe ${label}.`);
  }
  return identifier;
}

function quoteIdentifier(value) {
  const identifier = requireSqlIdentifier(value);
  return `"${identifier.replace(/"/gu, "\"\"")}"`;
}

function sqlString(value) {
  return `'${text(value).replace(/'/gu, "''")}'`;
}

function clampInteger(value, fallback, min, max) {
  const number = Number(value);
  if (!Number.isFinite(number)) return fallback;
  return Math.max(min, Math.min(Math.trunc(number), max));
}

function firstPositiveInteger(...values) {
  for (const value of values) {
    const number = Number(value);
    if (Number.isFinite(number) && number > 0) return Math.trunc(number);
  }
  return 0;
}

function sqlStringList(values = []) {
  const list = Array.isArray(values) ? values : [values];
  return list.map(text).filter(Boolean).map(sqlString);
}

function addTextMatchFilter(clauses, column, value, matchMode = "exact") {
  const normalized = text(value);
  if (!normalized) return;
  if (text(matchMode) === "contains") {
    clauses.push(`${column} LIKE ${sqlString(`%${normalized}%`)}`);
    return;
  }
  clauses.push(`${column} = ${sqlString(normalized)}`);
}

const RANKING_CASH_TRUE_TOKENS = Object.freeze([
  "1", "true", "t", "yes", "y", "cash", "现金", "是"
]);
const RANKING_CASH_FALSE_TOKENS = Object.freeze([
  "0", "false", "f", "no", "n", "noncash", "no_cash", "nocash", "非现金", "否"
]);
const RANKING_CASH_CUE_COLUMNS = Object.freeze([
  "summary", "txn_type", "remark", "voucher_type"
]);
const RANKING_CASH_NEGATIVE_CUE_PATTERN = "(非现金|不含现金|无现金|非现钞|不含现钞|无现钞|noncash|no cash)";
const RANKING_CASH_POSITIVE_CUE_PATTERN = "(现金|现钞|存现|取现|cash)";
const RANKING_SUCCESS_TRUE_TOKENS = Object.freeze([
  "1", "true", "t", "yes", "y", "success", "succeeded", "ok", "是", "成功"
]);
const RANKING_SUCCESS_FALSE_TOKENS = Object.freeze([
  "0", "false", "f", "no", "n", "failure", "failed", "error", "rejected", "timeout", "否", "失败", "拒绝", "超时"
]);

function normalizedTokenSql(column) {
  return `LOWER(TRIM(CAST(${column} AS VARCHAR)))`;
}

function tokenPredicateSql(column, values) {
  return `${column} IS NOT NULL AND ${normalizedTokenSql(column)} IN (${values.map(sqlString).join(", ")})`;
}

function knownTokenPredicateSql(column, trueValues, falseValues) {
  return `(${tokenPredicateSql(column, trueValues)} OR ${tokenPredicateSql(column, falseValues)})`;
}

function rankingCashClassificationSql() {
  const cueText = `LOWER(CONCAT_WS(' ', ${RANKING_CASH_CUE_COLUMNS
    .map((column) => `COALESCE(CAST(${column} AS VARCHAR), '')`)
    .join(", ")}))`;
  const negativeCue = `REGEXP_MATCHES(${cueText}, ${sqlString(RANKING_CASH_NEGATIVE_CUE_PATTERN)})`;
  const positiveCue = `REGEXP_MATCHES(REGEXP_REPLACE(${cueText}, ${sqlString(RANKING_CASH_NEGATIVE_CUE_PATTERN)}, ' ', 'g'), ${sqlString(RANKING_CASH_POSITIVE_CUE_PATTERN)})`;
  const explicitCash = tokenPredicateSql("cash_flag", RANKING_CASH_TRUE_TOKENS);
  const explicitNonCash = tokenPredicateSql("cash_flag", RANKING_CASH_FALSE_TOKENS);
  const cash = `((${explicitCash}) AND NOT (${negativeCue}))`;
  const nonCash = `((${explicitNonCash}) AND NOT (${positiveCue}))`;
  return {
    cash,
    nonCash,
    covered: `((${cash}) OR (${nonCash}))`,
    conflict: `(((${explicitCash}) AND (${negativeCue})) OR ((${explicitNonCash}) AND (${positiveCue})))`
  };
}

function nonBlankSql(column) {
  return `${column} IS NOT NULL AND TRIM(CAST(${column} AS VARCHAR)) <> ''`;
}

function rankingScopeAliases(args = {}) {
  return sqlStringList([
    ...(Array.isArray(args.account_keys) ? args.account_keys : []),
    args.account_key,
    args.accountKey,
    args.account,
    args.card_no,
    args.cardNo,
    args.acct_no,
    args.acctNo
  ]);
}

function rankingCoverageRequirements(skillId, args = {}, counterpartyGroupMode = "name", includeSelfCounterparty = false) {
  const requirements = [
    ["amount_covered_rows", "amount"],
    ["direction_covered_rows", "direction"],
    ["timestamp_covered_rows", "timestamp"],
    ["grouping_covered_rows", "grouping"]
  ];
  if (text(args.cash_filter || args.cashFilter || "all") !== "all") {
    requirements.push(["cash_covered_rows", "cash"]);
  }
  if (text(args.success_filter || args.successFilter || "all") !== "all") {
    requirements.push(["success_covered_rows", "success"]);
  }
  if (text(args.date_start || args.dateStart) || text(args.date_end || args.dateEnd)) {
    requirements.push(["date_covered_rows", "date"]);
  }
  if (sqlStringList(args.source_file_ids || args.sourceFileIds).length) {
    requirements.push(["source_file_covered_rows", "source_file"]);
  }
  if (rankingScopeAliases(args).length) {
    requirements.push(["account_scope_covered_rows", "account_scope"]);
  }
  if (text(args.holder_name || args.holderName)) {
    requirements.push(["holder_scope_covered_rows", "holder_scope"]);
  }
  if (text(args.id_no || args.idNo)) {
    requirements.push(["id_scope_covered_rows", "id_scope"]);
  }
  if (skillId === "rank_counterparties" && counterpartyGroupMode === "name" && !includeSelfCounterparty) {
    requirements.push(["source_holder_covered_rows", "source_holder"]);
  }
  return requirements;
}

function rankingGroupingCoverageSql(skillId, counterpartyGroupMode) {
  const sourceAccountKnown = nonBlankSql("acct_key");
  if (skillId === "rank_accounts") {
    return `(${sourceAccountKnown} AND ${nonBlankSql("account_open_name")})`;
  }
  if (skillId === "rank_holders") {
    return nonBlankSql("account_open_name");
  }
  if (counterpartyGroupMode === "account") {
    return `(${sourceAccountKnown} AND (${nonBlankSql("cp_key")} OR ${nonBlankSql("counterparty_acct")} OR ${nonBlankSql("cp_raw")}))`;
  }
  return `(${sourceAccountKnown} AND (${nonBlankSql("counterparty_name")} OR ${nonBlankSql("cp_name_pick")} OR ${nonBlankSql("cp_name")} OR ${nonBlankSql("stats_name_key")}))`;
}

function rankingCoverageSql(skillId, counterpartyGroupMode) {
  const cashClass = rankingCashClassificationSql();
  const successKnown = knownTokenPredicateSql("is_success", RANKING_SUCCESS_TRUE_TOKENS, RANKING_SUCCESS_FALSE_TOKENS);
  return `
    SELECT
      COUNT(*) AS requested_rows,
      COUNT(CASE WHEN amount IS NOT NULL AND isfinite(amount) THEN 1 END) AS amount_covered_rows,
      COUNT(CASE WHEN dc_val IN ('进', '出') THEN 1 END) AS direction_covered_rows,
      COUNT(CASE WHEN ${nonBlankSql("txn_time")} THEN 1 END) AS timestamp_covered_rows,
      COUNT(CASE WHEN ${rankingGroupingCoverageSql(skillId, counterpartyGroupMode)} THEN 1 END) AS grouping_covered_rows,
      COUNT(CASE WHEN ${cashClass.covered} THEN 1 END) AS cash_covered_rows,
      COUNT(CASE WHEN ${cashClass.conflict} THEN 1 END) AS cash_conflict_rows,
      COUNT(CASE WHEN ${successKnown} THEN 1 END) AS success_covered_rows,
      COUNT(CASE WHEN ${nonBlankSql("txn_day")} THEN 1 END) AS date_covered_rows,
      COUNT(CASE WHEN ${nonBlankSql("file_id")} THEN 1 END) AS source_file_covered_rows,
      COUNT(CASE WHEN ${nonBlankSql("acct_key")} OR ${nonBlankSql("acct_no")} OR ${nonBlankSql("card_no")} THEN 1 END) AS account_scope_covered_rows,
      COUNT(CASE WHEN ${nonBlankSql("account_open_name")} THEN 1 END) AS holder_scope_covered_rows,
      COUNT(CASE WHEN ${nonBlankSql("opener_id_no")} THEN 1 END) AS id_scope_covered_rows,
      COUNT(CASE WHEN ${nonBlankSql("account_open_name")} THEN 1 END) AS source_holder_covered_rows
    FROM analysis_txn_detail_idx`;
}

function validateRankingCoverage(result, { skillId, args, counterpartyGroupMode, includeSelfCounterparty }) {
  if (result.records.length !== 1 || result.row_count !== 1 || result.truncated !== false) {
    throw new Error(`${skillId} required-field coverage returned an invalid aggregate shape.`);
  }
  const record = result.records[0];
  const requestedRows = requireStrictCount(record, "requested_rows", `${skillId} required-field coverage`);
  const coverage = { requested_rows: requestedRows };
  const incompleteFields = [];
  const countFields = [
    "amount_covered_rows",
    "direction_covered_rows",
    "timestamp_covered_rows",
    "grouping_covered_rows",
    "cash_covered_rows",
    "cash_conflict_rows",
    "success_covered_rows",
    "date_covered_rows",
    "source_file_covered_rows",
    "account_scope_covered_rows",
    "holder_scope_covered_rows",
    "id_scope_covered_rows",
    "source_holder_covered_rows"
  ];
  for (const field of countFields) {
    coverage[field] = requireStrictCount(record, field, `${skillId} required-field coverage`);
    if (coverage[field] > requestedRows) {
      throw new Error(`${skillId} required-field coverage count exceeds requested rows.`);
    }
  }
  if (coverage.cash_covered_rows + coverage.cash_conflict_rows > requestedRows) {
    throw new Error(`${skillId} cash classification counts exceed requested rows.`);
  }
  for (const [field, label] of rankingCoverageRequirements(
    skillId,
    args,
    counterpartyGroupMode,
    includeSelfCounterparty
  )) {
    if (coverage[field] !== requestedRows) incompleteFields.push(label);
  }
  return {
    ...coverage,
    status: incompleteFields.length ? "partial" : "complete",
    incomplete_fields: incompleteFields
  };
}

function rankingWhereClauses(args = {}, target = "holder") {
  const clauses = [
    "amount IS NOT NULL",
    "isfinite(amount)",
    "dc_val IN ('进', '出')"
  ];
  const directionMode = text(args.direction_mode || args.directionMode || "both");
  if (directionMode === "in") clauses.push("dc_val = '进'");
  if (directionMode === "out") clauses.push("dc_val = '出'");

  const dateStart = text(args.date_start || args.dateStart);
  const dateEnd = text(args.date_end || args.dateEnd);
  if (dateStart) clauses.push(`txn_day >= ${sqlString(dateStart)}`);
  if (dateEnd) clauses.push(`txn_day <= ${sqlString(dateEnd)}`);

  const sourceFileIds = sqlStringList(args.source_file_ids || args.sourceFileIds);
  if (sourceFileIds.length) clauses.push(`file_id IN (${sourceFileIds.join(", ")})`);

  const accountAliases = rankingScopeAliases(args);
  if (accountAliases.length) {
    clauses.push(`(acct_key IN (${accountAliases.join(", ")}) OR acct_no IN (${accountAliases.join(", ")}) OR card_no IN (${accountAliases.join(", ")}))`);
  }

  addTextMatchFilter(clauses, "account_open_name", args.holder_name || args.holderName, args.match_mode || args.matchMode);
  addTextMatchFilter(clauses, "opener_id_no", args.id_no || args.idNo, "exact");

  const successFilter = text(args.success_filter || args.successFilter || "all");
  if (successFilter === "success_only") {
    clauses.push(`(${tokenPredicateSql("is_success", RANKING_SUCCESS_TRUE_TOKENS)})`);
  }

  const cashFilter = text(args.cash_filter || args.cashFilter || "all");
  const cashClass = rankingCashClassificationSql();
  if (cashFilter === "cash_only") {
    clauses.push(`(${cashClass.cash})`);
  } else if (cashFilter === "non_cash_only") {
    clauses.push(`(${cashClass.nonCash})`);
  }

  if (target === "holder") {
    clauses.push("account_open_name IS NOT NULL");
    clauses.push("TRIM(account_open_name) <> ''");
  }

  return clauses;
}

function rankingOrderMetric(metric) {
  switch (text(metric || "turnover")) {
    case "inflow":
      return "inflow_total";
    case "outflow":
      return "outflow_total";
    case "txn_count":
      return "txn_count";
    case "max_single_amount":
      return "max_single_amount";
    case "turnover":
    default:
      return "turnover_total";
  }
}

function normalizedRankingMetric(metric) {
  const value = text(metric || "turnover");
  return ["inflow", "outflow", "turnover", "txn_count", "max_single_amount"].includes(value)
    ? value
    : "turnover";
}

function isNumericOrTemporalType(dataType) {
  return /\b(?:tinyint|smallint|integer|bigint|hugeint|utinyint|usmallint|uinteger|ubigint|float|double|decimal|numeric|real|date|time|timestamp)\b/iu.test(text(dataType));
}

function isSensitiveColumn(columnName) {
  return /(?:account|acct|card|bank|holder|name|id|cert|phone|mobile|tel|address|addr|ip|mac|device|email|contact|counterparty|payer|payee|person|证件|姓名|户名|账号|卡号|账户|电话|地址)/iu.test(text(columnName));
}

function maskSensitiveValue(value) {
  if (value === null || value === undefined) return value;
  const source = text(value);
  if (!source) return value;
  if (source.length <= 4) return "*".repeat(source.length);
  return `${"*".repeat(Math.max(0, source.length - 4))}${source.slice(-4)}`;
}

function stableJsonHash(value) {
  return crypto
    .createHash("sha256")
    .update(JSON.stringify(value || {}, (_key, item) => (typeof item === "bigint" ? String(item) : item)))
    .digest("hex");
}

function workbenchHistoryAuthority(args = {}, expectedCaseId = "") {
  const source = objectOf(args);
  const context = objectOf(source._analytix || source.__analytix || source.analytix_runtime_context);
  const authority = {
    workspace_real_path: text(context.workspaceRealPath),
    thread_id: text(context.threadId),
    turn_id: text(context.turnId),
    case_id: text(context.caseId),
    case_binding_hash: text(context.caseBindingHash),
    dataset_snapshot_id: text(context.datasetSnapshotId),
    context_epoch: context.contextEpoch,
    context_digest: text(context.contextDigest)
  };
  if (!authority.workspace_real_path || !authority.thread_id || !authority.turn_id
    || !authority.case_id || authority.case_id !== text(expectedCaseId)
    || !/^[a-f0-9]{64}$/u.test(authority.case_binding_hash)
    || !/^dsv[12]_[a-f0-9]{64}$/u.test(authority.dataset_snapshot_id)
    || !Number.isSafeInteger(authority.context_epoch) || authority.context_epoch < 1
    || !/^[a-f0-9]{64}$/u.test(authority.context_digest)) {
    return null;
  }
  return {
    key: stableJsonHash(authority),
    datasetSnapshotId: authority.dataset_snapshot_id,
    contextEpoch: authority.context_epoch
  };
}

function baseTablesForSql(sql) {
  try {
    return analyzeDuckdbReadOnlySql(sql).base_tables;
  } catch {
    return [];
  }
}

function validateReadOnlySql(sql) {
  return analyzeDuckdbReadOnlySql(sql);
}

function rejectUnscopedAmountFactSql(staticGuard, sql) {
  const baseTables = arrayOf(staticGuard?.base_tables).map((value) => text(value).toLowerCase());
  if (baseTables.some((tableName) => UNSCOPED_AMOUNT_FACT_TABLES.has(tableName))) {
    throw duckdbDiagnosticError({
      code: "amount_coverage_incomplete",
      stage: "result_validation",
      sql
    });
  }
}

function resultGrain({ resultMode, result }) {
  const mode = text(resultMode || "preview");
  const rowCount = strictNonNegativeInteger(result?.row_count);
  if (mode === "aggregate") return "bounded_aggregate";
  if (mode === "chart_ready") return "chart_ready_table";
  if (mode === "evidence_table") return "bounded_evidence_table";
  if (rowCount === undefined) return "unresolved_result";
  if (rowCount <= 1) return "single_row_result";
  return "bounded_preview";
}

function sqlShapeDiagnostics(sql) {
  const source = stripSqlComments(sql);
  const baseTables = baseTablesForSql(source);
  const joinCount = [...source.matchAll(/\bjoin\b/giu)].length;
  const aggregateLike = /\b(?:count|sum|avg|min|max|group\s+by|distinct)\b/iu.test(source);
  const hasLimit = /\blimit\s+\d+\b/iu.test(source);
  const usesSelectStar = /\bselect\s+\*/iu.test(source);
  return {
    base_tables: baseTables,
    base_table_count: baseTables.length,
    join_count: joinCount,
    aggregate_like: aggregateLike,
    has_limit: hasLimit,
    uses_select_star: usesSelectStar,
    possible_unbounded_detail_scan: usesSelectStar && !hasLimit && !aggregateLike
  };
}

function slowPathDiagnostics({ sql, planRows = [] }) {
  const sqlShape = sqlShapeDiagnostics(sql);
  const planText = text(JSON.stringify(planRows)).toLowerCase();
  const flags = [];
  if (sqlShape.possible_unbounded_detail_scan) {
    flags.push({
      code: "POSSIBLE_UNBOUNDED_DETAIL_SCAN",
      severity: "warning",
      message: "SQL 形态接近无界明细扫描；应改为显式列、聚合或受控 preview。"
    });
  }
  if (sqlShape.join_count >= 2) {
    flags.push({
      code: "MULTI_JOIN_QUERY",
      severity: "info",
      message: "SQL 含多处 JOIN；应先确认连接键、基表行数和结果粒度。"
    });
  }
  if (/\b(?:seq_scan|sequential scan|table_scan|scan)\b/iu.test(planText)) {
    flags.push({
      code: "PLAN_SCAN_REVIEW",
      severity: "info",
      message: "执行计划包含扫描节点；大范围明细题应先收窄时间、主体、方向或改用聚合索引。"
    });
  }
  return {
    sql_shape: sqlShape,
    flags,
    slow_path_likely: flags.some((flag) => text(flag.severity) === "warning"),
    next_actions: flags.length
      ? [
          "先用 count_case_rows 核验目标范围记录数。",
          "报告级金额优先改成聚合 SQL，避免从样本或明细 preview 外推。",
          "涉及连接时先核对 inspect_case_schema/profile_case_schema 返回的 SQL-safe 连接键。"
        ]
      : []
  };
}

function typedRunnerFailure(error, { sql = "", stage = "execute" } = {}) {
  if (isDuckdbDiagnosticError(error)) return error;
  const message = text(error?.message || error);
  const nativeCode = text(error?.code).toUpperCase();
  if (["ENOENT", "EACCES", "ENOEXEC"].includes(nativeCode)) {
    return duckdbDiagnosticError({ code: "runner_unavailable", stage: "runner_resolution", sql });
  }
  if (/No module named ['"]duckdb['"]|ModuleNotFoundError.*duckdb/iu.test(message)) {
    return duckdbDiagnosticError({ code: "runner_unavailable", stage: "runner_resolution", sql });
  }
  if (/dataset snapshot|snapshot source|snapshot authority/iu.test(message)) {
    return duckdbDiagnosticError({ code: "dataset_snapshot_mismatch", stage: "snapshot_validation", sql });
  }
  if (/Could not set lock|database is locked|Conflicting lock/iu.test(message)) {
    return duckdbDiagnosticError({ code: "database_locked", stage: "bind", sql });
  }
  if (/Parser Error|syntax error/iu.test(message)) {
    return duckdbDiagnosticError({ code: "sql_parse_rejected", stage: "parse", sql });
  }
  if (/function manifest is unsafe or unavailable/iu.test(message)) {
    return duckdbDiagnosticError({ code: "sql_policy_rejected", stage: "policy", sql });
  }
  if (/file system operations are disabled|external access/iu.test(message)) {
    return duckdbDiagnosticError({ code: "sql_bind_rejected", stage: "bind", sql });
  }
  if (/Binder Error|Catalog Error|does not exist|not found in FROM clause/iu.test(message)) {
    return duckdbDiagnosticError({ code: "sql_bind_rejected", stage: "bind", sql });
  }
  if (/identifier columns must use a textual source type|non-finite numeric value|malformed record|invalid column type/iu.test(message)) {
    return duckdbDiagnosticError({ code: "result_contract_invalid", stage: "result_validation", sql });
  }
  return duckdbDiagnosticError({ code: "sql_execution_failed", stage, sql });
}

async function validateCaseSqlPolicy({ caseId, sql, env = process.env, expectedDatasetSnapshotId = "", signal }) {
  throwIfAborted(signal);
  let staticGuard;
  try {
    staticGuard = validateReadOnlySql(sql);
  } catch {
    throw duckdbDiagnosticError({ code: "sql_policy_rejected", stage: "policy", sql });
  }
  // Arbitrary SQL/notebook cells cannot yet derive an exact amount scope from
  // a trusted AST. Dedicated host-owned executors must perform the v12
  // coverage check before filters, thresholds, ordering or LIMIT.
  rejectUnscopedAmountFactSql(staticGuard, sql);
  try {
    const parserProbe = await runLocalSql({
      caseId,
      env,
      rowLimit: 80,
      sql,
      metadataMode: true,
      bindDatasetSnapshot: Boolean(expectedDatasetSnapshotId),
      expectedDatasetSnapshotId,
      policyAnalysis: staticGuard,
      policyExplainOnly: true,
      signal
    });
    throwIfAborted(signal);
    return {
      status: "passed",
      policy_engine: "duckdb_extract_statement_and_explain_v2",
      parser: "duckdb",
      binder: "duckdb",
      db_connection_mode: "read_only",
      executed_user_sql: false,
      static_guard: staticGuard,
      base_tables: staticGuard.base_tables,
      plan_row_count: parserProbe.row_count,
      validation_query: "EXPLAIN <redacted-current-case-sql>"
    };
  } catch (error) {
    throwIfAborted(signal);
    const diagnostic = publicDuckdbDiagnostic(error, { stage: "bind", sql });
    const next = duckdbDiagnosticError({ code: diagnostic.code, stage: diagnostic.stage, sql });
    next.sql_policy = {
      status: "blocked",
      policy_engine: "duckdb_extract_statement_and_explain_v2",
      parser: "duckdb",
      binder: "duckdb",
      db_connection_mode: "read_only",
      executed_user_sql: false,
      static_guard: staticGuard,
      diagnostic,
      error_message: diagnostic.publicMessage
    };
    throw next;
  }
}

function extractCandidateBindings(message) {
  const raw = text(message);
  const matches = [...raw.matchAll(/(?:Candidate|线索)\s+bindings:\s*([^\n]+)/giu)];
  const output = [];
  for (const match of matches) {
    const segment = text(match[1]).replace(/\s+LINE\s+\d+:.*$/iu, "");
    for (const quoted of segment.matchAll(/"([^"]+)"/gu)) {
      output.push(text(quoted[1]));
    }
  }
  return [...new Set(output.filter(Boolean))];
}

function extractMissingIdentifier(message) {
  const raw = text(message);
  return text(
    raw.match(/Referenced\s+column\s+"([^"]+)"/iu)?.[1]
      || raw.match(/column\s+"([^"]+)"\s+(?:not\s+found|不存在)/iu)?.[1]
      || raw.match(/字段\s+"?([^"\s，,]+)"?\s*(?:不存在|未找到)/u)?.[1]
  );
}

function extractQuotedPredicateValue(sql, columns = []) {
  const source = text(sql);
  for (const column of columns) {
    const escaped = text(column).replace(/[.*+?^${}()|[\]\\]/gu, "\\$&");
    const match = source.match(new RegExp(`\\b${escaped}\\b\\s*=\\s*'([^']+)'`, "iu"));
    if (match) return text(match[1].replace(/''/gu, "'"));
  }
  return "";
}

function hasColumn(table, columnName) {
  const wanted = text(columnName).toLowerCase();
  return arrayOfColumns(table).some((column) => text(column.name || column.column_name || column.sql_name).toLowerCase() === wanted);
}

function arrayOfColumns(table) {
  const source = objectOf(table);
  return Array.isArray(source.columns) ? source.columns : [];
}

function preferredTxnDetailColumns(table) {
  if (text(table?.table_name) !== "analysis_txn_detail_idx") return [];
  return [
    ["account_open_name", "本方/主体户名过滤；过滤值必须来自当前请求中已绑定并核验的主体，不得使用固定案件示例。"],
    ["acct_key", "本方账户键过滤或分组。"],
    ["dc_val", "交易方向字段，出账使用 dc_val = '出'，入账使用 dc_val = '进'。"],
    ["amount", "交易金额字段，排行/汇总使用 SUM(amount)。"],
    ["counterparty_name", "优先对手方名称字段，可与 cp_name_pick/cp_name 共同兜底。"],
    ["cp_name_pick", "对手方名称兜底字段。"],
    ["cp_name", "对手方名称原始/兜底字段。"],
    ["counterparty_acct", "对手方账号字段，按账号粒度排行时使用。"],
    ["txn_time", "交易时间字段，输出首末笔时间。"],
    ["txn_ts", "交易时间戳字段，做时间窗过滤。"]
  ].filter(([column]) => hasColumn(table, column)).map(([column, usage]) => ({
    table_name: "analysis_txn_detail_idx",
    column_name: column,
    usage
  }));
}

function recommendedTopCounterpartySql(holderName, limit = 10) {
  const holder = text(holderName);
  if (!holder) return "";
  const safeLimit = clampInteger(limit, 10, 1, 50);
  return [
    "SELECT",
    "  COALESCE(NULLIF(TRIM(counterparty_name), ''), NULLIF(TRIM(cp_name_pick), ''), NULLIF(TRIM(cp_name), ''), '空户名') AS counterparty_name,",
    "  SUM(amount) AS outflow_total,",
    "  COUNT(*) AS txn_count,",
    "  MIN(txn_time) AS first_txn_at,",
    "  MAX(txn_time) AS last_txn_at",
    "FROM analysis_txn_detail_idx",
    `WHERE account_open_name = ${sqlString(holder)}`,
    "  AND dc_val = '出'",
    "GROUP BY counterparty_name",
    "ORDER BY outflow_total DESC, txn_count DESC",
    `LIMIT ${safeLimit}`
  ].join("\n");
}

async function buildCaseSqlDiagnosis({ caseId, env, sql, diagnostics = [], errorMessage = "", signal }) {
  throwIfAborted(signal);
  const combinedMessage = [errorMessage, ...diagnostics].map(text).filter(Boolean).join("\n");
  const missingIdentifier = extractMissingIdentifier(combinedMessage);
  const candidateBindings = extractCandidateBindings(combinedMessage);
  const sqlShape = sql ? sqlShapeDiagnostics(sql) : {};
  let tables = [];
  try {
    const listed = await listAllowedTables({ caseId, env, tableLimit: 500, signal });
    tables = Array.isArray(listed.tables) ? listed.tables : [];
  } catch (error) {
    throwIfAborted(signal);
    tables = [];
  }
  const baseTables = new Set(arrayOf(sqlShape.base_tables).map(simpleTableName));
  const relevantTables = tables.filter((table) => !baseTables.size || baseTables.has(simpleTableName(table.table_name)));
  const candidateColumns = [];
  for (const binding of candidateBindings) {
    for (const table of relevantTables) {
      if (hasColumn(table, binding)) {
        candidateColumns.push({
          table_name: text(table.table_name),
          column_name: binding,
          usage: "DuckDB Binder 返回的当前表候选列；复用前先确认字段业务含义。"
        });
      }
    }
  }
  const txnDetail = tables.find((table) => text(table.table_name) === "analysis_txn_detail_idx");
  candidateColumns.push(...preferredTxnDetailColumns(txnDetail));
  const holderName = extractQuotedPredicateValue(sql, [
    "payer_name",
    "holder_name",
    "account_open_name",
    "付款方名称",
    "户名"
  ]);
  const requestedLimit = Number(text(sql).match(/\blimit\s+(\d+)/iu)?.[1]) || 10;
  const topCounterpartySql = /payer_name|cparty_name|txn_amt|counterparty|对手方|出账|给谁/u.test(text(sql) + text(errorMessage))
    ? recommendedTopCounterpartySql(holderName, requestedLimit)
    : "";
  const uniqueCandidateColumns = [];
  const seen = new Set();
  for (const column of candidateColumns) {
    const key = `${text(column.table_name)}.${text(column.column_name)}`;
    if (!text(column.table_name) || !text(column.column_name) || seen.has(key)) continue;
    seen.add(key);
    uniqueCandidateColumns.push(column);
  }
  const correctiveActions = [
    missingIdentifier ? `当前 SQL 引用了不存在字段 ${missingIdentifier}；不要继续猜同义列，先改用返回的 SQL-safe column_name。` : "",
    topCounterpartySql ? "主体出账 Top N/给谁最多问题优先使用 rank_counterparties；需要 SQL 复核时使用 analysis_txn_detail_idx 的 account_open_name/dc_val/amount/counterparty_name 口径。" : "",
    candidateBindings.length ? `DuckDB 给出的候选列包括：${candidateBindings.slice(0, 8).join("、")}。` : "",
    "如仍需现场 SQL，请用 inspect_case_schema({table_limit: 500, include_columns: true}) 或 profile_case_schema({tables:[目标表], column_limit:80}) 核对列名后再执行 run_case_sql。"
  ].filter(Boolean);
  return {
    likely_cause: missingIdentifier
      ? `SQL 字段名与当前案件 DuckDB schema 不匹配：${missingIdentifier}`
      : "SQL 需要结合当前案件 schema 继续核验。",
    missing_identifier: missingIdentifier,
    candidate_bindings: candidateBindings,
    candidate_columns: uniqueCandidateColumns.slice(0, 16),
    recommended_sql_patterns: [topCounterpartySql].filter(Boolean),
    corrective_actions: correctiveActions
  };
}

function workbenchSourceHash({ caseId, sql, sourceScope, result }) {
  return stableJsonHash({
    case_id: caseId,
    dataset_snapshot_id: text(result?.observed_dataset_snapshot_id),
    sql: stripTrailingSemicolon(sql),
    source_scope: sourceScope,
    columns: result?.columns || [],
    column_types: result?.column_types || [],
    records: result?.records || [],
    row_count: strictNonNegativeInteger(result?.row_count),
    truncated: typeof result?.truncated === "boolean" ? result.truncated : undefined
  });
}

function validateSqlFactRecordValues(result) {
  const countColumnPattern = /(?:^|_)(?:count|row_count|rows|rank|ordinal|position)$/iu;
  const numericColumnPattern = /(?:^|_)(?:amount|sum|total|inflow|outflow|turnover|balance|rate|ratio|share|score|value)$/iu;
  for (const record of result.records) {
    for (const column of result.columns) {
      if (!hasOwn(record, column) || record[column] === null) continue;
      if (countColumnPattern.test(column) && !isCanonicalIntegerValue(record[column])) {
        throw new Error(`DuckDB SQL result returned an invalid integer fact in ${column}.`);
      }
      if (numericColumnPattern.test(column) && !isCanonicalDecimalValue(record[column])) {
        throw new Error(`DuckDB SQL result returned an invalid numeric fact in ${column}.`);
      }
    }
  }
}

function zeroResultStatus(result, completeness) {
  if (completeness.status !== "complete") return "unresolved";
  if (result.records.length === 0) return "candidate_no_hit";
  const countColumns = result.columns.filter((column) => /(?:^|_)(?:count|row_count|rows)$/iu.test(column));
  if (
    result.records.length === 1
    && countColumns.length === result.columns.length
    && countColumns.length > 0
    && countColumns.every((column) => strictNonNegativeInteger(result.records[0]?.[column]) === 0)
  ) {
    return "candidate_no_hit";
  }
  return "not_zero";
}

function factualSnapshotReady(result) {
  void result;
  // A local DuckDB digest, including the retired local dsv2 form, is never a
  // host DatasetSnapshot authority. Fact readiness is admitted only after the
  // Go registry binds a verified producer-content manifest in a later layer.
  return false;
}

function resultCompleteness({ result, queryScope }) {
  const scopeComplete = Boolean(
    text(queryScope?.case_id)
    && Array.isArray(queryScope?.source_scope)
    && queryScope.source_scope.length > 0
    && text(queryScope?.query_hash)
  );
  const integrityValid = result.result_integrity?.status === "validated"
    && result.result_integrity?.row_count_consistent === true;
  const snapshotReady = factualSnapshotReady(result);
  return {
    status: result.truncated === false && scopeComplete && integrityValid && snapshotReady ? "complete" : "partial",
    query_scope_complete: scopeComplete,
    result_integrity_validated: integrityValid,
    factual_snapshot_ready: snapshotReady,
    pagination_complete: result.truncated === false,
    row_count_consistent: result.result_integrity?.row_count_consistent === true
  };
}

function inferRunCaseSqlTopNContract({ purpose = "", sql = "", result = {}, rowLimit = 0 }) {
  const source = stripSqlComments(sql).replace(/\s+/gu, " ");
  const textScope = `${purpose}\n${source}`;
  const explicitLimit = firstPositiveInteger(
    textScope.match(/\b(?:top|Top)\s*(\d{1,3})\b/u)?.[1],
    textScope.match(/前\s*(\d{1,3})\s*(?:位|名|个|户|条|笔)?/u)?.[1],
    textScope.match(/\brequested[_ ]?limit\s*[:=]\s*(\d{1,3})\b/iu)?.[1]
  );
  const sqlLimit = firstPositiveInteger(
    source.match(/\blimit\s+(\d{1,3})(?!\s*,)/iu)?.[1],
    source.match(/\b(?:rn|rank|row_num|row_number)\s*<=\s*(\d{1,3})\b/iu)?.[1],
    source.match(/\b(?:rn|rank|row_num|row_number)\s+between\s+1\s+and\s+(\d{1,3})\b/iu)?.[1]
  );
  const requested = firstPositiveInteger(explicitLimit, sqlLimit);
  if (!requested) return null;
  const returned = strictNonNegativeInteger(result?.row_count);
  if (returned === undefined) {
    throw new Error("DuckDB Top-N SQL result is missing a valid returned row count.");
  }
  const resultComplete = result?.truncated === false && factualSnapshotReady(result);
  return topNContract({
    requestedLimit: clampInteger(requested, requested, 1, 200),
    resolvedLimit: sqlLimit || requested,
    returnedCount: returned,
    compactTextRowCount: returned,
    dataExhausted: resultComplete ? returned < requested : undefined,
    resultComplete,
    truncationReason: result?.truncated || Number(rowLimit || 0) < requested
      ? "row_limit_or_result_truncation_before_requested_limit"
      : ""
  });
}

function buildRunCaseSqlEvidence({ caseId, purpose, sql, rowLimit, resultMode, result, queryId, sqlPolicy = null }) {
  validateSqlFactRecordValues(result);
  const sourceScope = baseTablesForSql(sql).filter(isAllowedWorkbenchTableName);
  const sourceHash = workbenchSourceHash({ caseId, sql, sourceScope, result });
  const truncated = result.truncated;
  const topN = inferRunCaseSqlTopNContract({ purpose, sql, result, rowLimit });
  const queryScope = {
    case_id: caseId,
    dataset_snapshot_id: text(result.observed_dataset_snapshot_id),
    source_scope: sourceScope,
    query_hash: stableJsonHash({ case_id: caseId, sql: stripTrailingSemicolon(sql) }),
    purpose,
    row_limit: rowLimit,
    result_mode: text(resultMode || "preview")
  };
  const completeness = resultCompleteness({ result, queryScope });
  const noHitStatus = zeroResultStatus(result, completeness);
  const metricScope = {
    purpose,
    source_scope: sourceScope,
    allowed_view_policy: "cleaned_and_analysis_only",
    result_mode: text(resultMode || "preview"),
    grain: resultGrain({ resultMode, result }),
    row_limit: rowLimit,
    row_count: result.row_count,
    truncated,
    raw_rows_exposed: false,
    query_scope: queryScope,
    result_completeness: completeness,
    zero_result_status: noHitStatus,
    ...(topN ? { top_n_contract: topN } : {})
  };
  const validationState = {
    status: completeness.status === "complete" ? "executed" : "partial",
    current_case_only: true,
    readonly: true,
    cleaned_analysis_scope_only: true,
    bounded_row_limit: true,
    raw_rows_exposed: false,
    sql_policy: sqlPolicy,
    report_grade_claim_requires_owner_review: true
  };
  const evidenceCard = {
    card_type: "case_workbench_evidence_card",
    fact_id: `workbench:${queryId.split(":").pop()}`,
    support_status: "unsupported",
    candidate_support_status: completeness.status === "complete" ? "candidate_complete" : "candidate_partial",
    fact_answer_allowed: false,
    verified_no_hit_allowed: false,
    requires_host_evidence_receipt: true,
    missing_fields: ["host_evidence_receipt"],
    support_query_name: "run_case_sql",
    source_level: "controlled_case_workbench",
    source_hash: sourceHash,
    source_refs: { query_ids: [queryId] },
    query_id: queryId,
    metric_scope: metricScope,
    query_scope: queryScope,
    result_completeness: completeness,
    zero_result_status: noHitStatus,
    validation_state: validationState,
    sql_policy: sqlPolicy,
    ...(topN ? { top_n_contract: topN } : {}),
    evidence_boundary: "专项资金核算结果只支持本次 SQL 明确限定的当前案件、清洗/分析范围；写入报告前仍需 focused owner 复核口径、表后研判和不能认定事项。",
    next_review_actions: [
      "将该结果与语义事实或既有核验结果做 source-of-truth selection。",
      "如用于报告级金额、链路或图表，补充异常特征、证明价值、暂不能认定事项和下一步调取材料。"
    ]
  };
  return { sourceScope, sourceHash, metricScope, validationState, evidenceCard, topN };
}

export async function runPythonDuckdb(payload, env = process.env, signal) {
  throwIfAborted(signal);
  const input = JSON.stringify(payload);
  const sql = text(payload?.sql || payload?.policy_sql);
  const executionStage = payload?.policy_explain_only === true ? "bind" : "execute";
  const childEnv = duckdbPythonChildEnv(env);
  let lastUnavailable = null;
  for (const python of canonicalPythonCandidates(env)) {
    throwIfAborted(signal);
    if (python.includes(path.sep) && !fs.existsSync(python)) continue;
    try {
      const result = await new Promise((resolve, reject) => {
        const child = spawn(python, ["-c", PYTHON_SCRIPT], {
          env: childEnv,
          stdio: ["pipe", "pipe", "pipe"],
          windowsHide: true
        });
        let stdout = "";
        let stderr = "";
        let stdoutBytes = 0;
        let stderrBytes = 0;
        let settled = false;
        let forceKillTimer;
        const cleanup = () => {
          clearTimeout(timer);
          signal?.removeEventListener("abort", onAbort);
        };
        const rejectOnce = (error, terminate = false) => {
          if (settled) return;
          settled = true;
          cleanup();
          if (terminate) {
            child.kill("SIGTERM");
            forceKillTimer = setTimeout(() => child.kill("SIGKILL"), 1_000);
            forceKillTimer.unref?.();
          }
          reject(error);
        };
        const onAbort = () => rejectOnce(abortError(signal), true);
        const timer = setTimeout(() => {
          rejectOnce(duckdbDiagnosticError({ code: "runner_timeout", stage: executionStage, sql }), true);
        }, 60_000);
        signal?.addEventListener("abort", onAbort, { once: true });
        if (signal?.aborted) {
          onAbort();
          return;
        }
        child.stdout.setEncoding("utf8");
        child.stderr.setEncoding("utf8");
        child.stdout.on("data", (chunk) => {
          if (settled) return;
          stdoutBytes += Buffer.byteLength(chunk, "utf8");
          if (stdoutBytes > MAX_DUCKDB_STDOUT_BYTES) {
            rejectOnce(duckdbDiagnosticError({ code: "resource_limit_exceeded", stage: "fetch", sql }), true);
            return;
          }
          stdout += chunk;
        });
        child.stderr.on("data", (chunk) => {
          if (settled) return;
          stderrBytes += Buffer.byteLength(chunk, "utf8");
          if (stderrBytes > MAX_DUCKDB_STDERR_BYTES) {
            rejectOnce(duckdbDiagnosticError({ code: "resource_limit_exceeded", stage: executionStage, sql }), true);
            return;
          }
          stderr += chunk;
        });
        child.on("error", (error) => {
          rejectOnce(typedRunnerFailure(error, { sql, stage: "runner_spawn" }));
        });
        child.on("close", (code) => {
          if (forceKillTimer) clearTimeout(forceKillTimer);
          if (settled) return;
          settled = true;
          cleanup();
          if (code === 0) {
            resolve({ stdout, stderr });
          } else {
            reject(typedRunnerFailure(new Error(stderr || `local DuckDB Python runner exited ${code}`), { sql, stage: executionStage }));
          }
        });
        try {
          child.stdin.end(input);
        } catch (error) {
          rejectOnce(duckdbDiagnosticError({ code: "runner_spawn_failed", stage: "runner_spawn", sql }), true);
        }
      });
      throwIfAborted(signal);
      let parsed;
      try {
        parsed = JSON.parse(result.stdout || "{}");
      } catch {
        throw duckdbDiagnosticError({ code: "runner_protocol_invalid", stage: "result_validation", sql });
      }
      try {
        return validateDuckdbResult(parsed);
      } catch {
        throw duckdbDiagnosticError({ code: "result_contract_invalid", stage: "result_validation", sql });
      }
    } catch (error) {
      throwIfAborted(signal);
      const typedError = typedRunnerFailure(error, { sql, stage: executionStage });
      if (!shouldRetryDuckdbRunner(typedError)) throw typedError;
      lastUnavailable = typedError;
    }
  }
  throw lastUnavailable || duckdbDiagnosticError({ code: "runner_unavailable", stage: "runner_resolution", sql });
}

export function supportsLocalDuckdbWorkbenchSkill(skillId) {
  return LOCAL_DUCKDB_WORKBENCH_SKILLS.has(text(skillId));
}

function localWorkbenchEnvelope({ skillId, caseId, data = {}, warnings = [], citations = {}, assertions = [], nextActions = [] }) {
  const queryIds = Array.isArray(citations.query_ids) ? citations.query_ids.map(text).filter(Boolean) : [];
  const queryId = queryIds[0] || queryIdFor({ caseId, skillId, extra: JSON.stringify(data).slice(0, 240) });
  return {
    skill_id: skillId,
    version: DUCKDB_WORKBENCH_RUNTIME_VERSION,
    status: "ok",
    snapshot_at: new Date().toISOString(),
    data: {
      workbench_contract: "local_duckdb_readonly_v1",
      execution_status: "executed",
      case_id: caseId,
      query_id: queryId,
      source_scope: "current_case_local_duckdb",
      allowed_view_policy: "cleaned_and_analysis_only",
      raw_rows_exposed: false,
      ...data
    },
    assertions,
    warnings: [
      {
        code: "LOCAL_DUCKDB_WORKBENCH_FALLBACK",
        severity: "info",
        message: "已按当前案件只读清洗/分析范围完成数据库现场勘查。"
      },
      ...warnings
    ],
    next_actions: nextActions,
    audit_ref: {
      query_id: queryId,
      workbench_contract: "local_duckdb_readonly_v1"
    },
    cursor: null,
    has_more: false,
    truncated: false,
    error: null,
    citations: {
      query_ids: queryIds.length ? queryIds : [queryId]
    }
  };
}

function queryIdFor({ caseId, skillId, sql = "", extra = "" }) {
  return `local-duckdb:${skillId}:${crypto
    .createHash("sha256")
    .update(JSON.stringify({ caseId, sql, extra }))
    .digest("hex")}`;
}

function compactValidationState(data = {}, status = "ok") {
	const validationState = objectOf(data.validation_state);
	const validationSummary = objectOf(data.validation_summary);
	const sqlPolicy = objectOf(data.sql_policy || validationState.sql_policy);
	const allowedTables = text(validationSummary.allowed_tables || data.allowed_view_policy);
	return pruneEmpty({
		execution_status: text(data.execution_status || validationState.status || status) || "ok",
		readonly: consistentBoolean(validationState.readonly, validationSummary.readonly),
		current_case_only: consistentBoolean(validationSummary.current_case_only, validationState.current_case_only),
		cleaned_analysis_scope_only: consistentBoolean(
			validationState.cleaned_analysis_scope_only,
			allowedTables.includes("cleaned") || allowedTables.includes("analysis_*") ? true : undefined
		),
		parser_binder_validated: consistentBoolean(
			validationSummary.parser_binder_validated,
			sqlPolicy.status === "passed" ? true : undefined
		),
		sql_policy_engine: text(sqlPolicy.policy_engine || validationSummary.sql_policy_engine),
		raw_rows_exposed: consistentBoolean(data.raw_rows_exposed, validationState.raw_rows_exposed)
	}) || {};
}

function historyBaseTables({ sql = "", data = {} }) {
  const fromSql = baseTablesForSql(sql).filter(isAllowedWorkbenchTableName);
  if (fromSql.length) return fromSql.slice(0, 20);
  const sourceScope = data.source_scope;
  if (Array.isArray(sourceScope)) {
    return sourceScope.map(text).filter(isAllowedWorkbenchTableName).slice(0, 20);
  }
  const tableName = text(data.table_name);
  return tableName && isAllowedWorkbenchTableName(tableName) ? [simpleTableName(tableName)] : [];
}

function historySlowPathLikely(data = {}) {
	const slowPath = objectOf(data.slow_path_diagnostics);
	const validationSummary = objectOf(data.validation_summary);
	return consistentBoolean(slowPath.slow_path_likely, validationSummary.slow_path_likely);
}

function recordLocalWorkbenchHistory({
  skillId,
  caseId,
  args = {},
  queryId = "",
  purpose = "",
  sql = "",
  data = {},
  startedAt = Date.now(),
  status = "ok",
  errorClass = "",
  evidenceId = "",
  signal
} = {}) {
  throwIfAborted(signal);
  const normalizedCaseId = text(caseId);
  const normalizedSkillId = text(skillId);
  const authority = workbenchHistoryAuthority(args, normalizedCaseId);
  if (!normalizedCaseId || !normalizedSkillId || normalizedSkillId === "inspect_workbench_history" || !authority) return;
  const durationMs = Math.max(0, Date.now() - Number(startedAt || Date.now()));
  const evidenceCard = objectOf(data.evidence_card);
  const queryDigest = sql
    ? stableJsonHash({
      case_id: normalizedCaseId,
      skill_id: normalizedSkillId,
      authority_key: authority.key,
      sql: stripTrailingSemicolon(sql)
      })
    : "";
  const rowCount = strictNonNegativeInteger(data.row_count);
  const entry = {
    recorded_at: new Date().toISOString(),
    authority_key: authority.key,
    case_id: normalizedCaseId,
    skill_id: normalizedSkillId,
    status: text(status || data.execution_status) || "ok",
    query_id: text(queryId || data.query_id),
    query_digest: queryDigest,
    purpose: text(purpose || data.purpose),
    base_tables: historyBaseTables({ sql, data }),
    duration_ms: Number.isFinite(durationMs) ? durationMs : 0,
    row_count: rowCount,
		truncated: strictBoolean(data.truncated),
    validation_state: compactValidationState(data, status),
    slow_path_likely: historySlowPathLikely(data),
    error_class: text(errorClass),
    evidence_id: text(evidenceId || evidenceCard.fact_id)
  };
  for (const [key, value] of Object.entries(entry)) {
    if (value === "" || value === undefined || (Array.isArray(value) && !value.length)) {
      delete entry[key];
    }
  }
  LOCAL_WORKBENCH_HISTORY.unshift(entry);
  if (LOCAL_WORKBENCH_HISTORY.length > LOCAL_WORKBENCH_HISTORY_LIMIT) {
    LOCAL_WORKBENCH_HISTORY.splice(LOCAL_WORKBENCH_HISTORY_LIMIT);
  }
}

function tableFilterSql() {
  return "(lower(table_name) LIKE 'analysis_%' OR regexp_matches(lower(table_name), '^fc_[a-z0-9_]*_norm$'))";
}

async function runLocalSql({
  caseId,
  sql,
  rowLimit = 100,
  env,
  metadataMode = false,
  bindDatasetSnapshot = false,
  expectedDatasetSnapshotId = "",
  expectedProducerContentId = "",
  snapshotMode = "",
  policyAnalysis,
  policyExplainOnly = false,
  signal
}) {
  throwIfAborted(signal);
  const dbPath = caseDbPath(caseId, env);
  if (!fs.existsSync(dbPath)) {
    throw duckdbDiagnosticError({ code: "database_unavailable", stage: "runner_resolution", sql });
  }
  const expectedSnapshot = text(expectedDatasetSnapshotId);
  if (expectedSnapshot && !/^dsv[12]_[a-f0-9]{64}$/u.test(expectedSnapshot)) {
    throw duckdbDiagnosticError({ code: "dataset_snapshot_mismatch", stage: "snapshot_validation", sql });
  }
  const expectedProducerContent = text(expectedProducerContentId);
  if (expectedProducerContent && !/^fpc1_[a-f0-9]{64}$/u.test(expectedProducerContent)) {
    throw duckdbDiagnosticError({ code: "dataset_snapshot_mismatch", stage: "snapshot_validation", sql });
  }
  const resolvedSnapshotMode = text(snapshotMode)
    || (expectedProducerContent ? "producer_content_v1" : (
      expectedSnapshot.startsWith("dsv2_") ? "require_v2" : "legacy_exact_count"
    ));
  const policy = policyAnalysis ? objectOf(policyAnalysis) : null;
  return runPythonDuckdb({
    db_path: dbPath,
    sql,
    row_limit: Math.max(1, Math.min(Number(rowLimit || 100) || 100, metadataMode ? 5000 : 500)),
    metadata_mode: metadataMode,
    ...(bindDatasetSnapshot || expectedSnapshot || expectedProducerContent ? { snapshot_case_id: caseId } : {}),
    ...(bindDatasetSnapshot || expectedSnapshot || expectedProducerContent ? {
      snapshot_mode: resolvedSnapshotMode
    } : {}),
    ...(expectedSnapshot ? { expected_dataset_snapshot_id: expectedSnapshot } : {}),
    ...(expectedProducerContent ? { expected_producer_content_id: expectedProducerContent } : {}),
    ...(policy ? {
      policy_sql: sql,
      policy_base_tables: arrayOf(policy.base_tables).map(text),
      policy_functions: arrayOf(policy.functions).map(text),
      policy_explain_only: policyExplainOnly === true
    } : {})
  }, env, signal);
}

export async function probeLocalFundsProducerContent(args = {}, options = {}) {
  const env = options.env || process.env;
  throwIfAborted(options.signal);
  const caseId = localCaseId(args, env);
  if (!caseId) throw new Error("Funds producer content probe requires a resolved case project.");
  const result = await runLocalSql({
    caseId,
    env,
    rowLimit: 1,
    sql: "SELECT 1 AS source_ready",
    bindDatasetSnapshot: true,
    snapshotMode: "producer_content_v1",
    signal: options.signal
  });
  throwIfAborted(options.signal);
  if (
    result.producer_content_contract !== FUNDS_PRODUCER_CONTENT_CONTRACT_V1
    || !/^fpc1_[a-f0-9]{64}$/u.test(text(result.producer_content_id))
    || !/^[a-f0-9]{64}$/u.test(text(result.producer_manifest_sha256))
    || result.producer_manifest_schema_version !== 1
    || result.snapshot_factual_ready !== false
  ) {
    throw duckdbDiagnosticError({ code: "result_contract_invalid", stage: "snapshot_validation", sql: "SELECT 1 AS source_ready" });
  }
  return {
    case_id: caseId,
    producer_content_contract: result.producer_content_contract,
    producer_content_id: result.producer_content_id,
    producer_manifest_sha256: result.producer_manifest_sha256,
    producer_manifest_schema_version: result.producer_manifest_schema_version,
    host_dataset_snapshot_authority: "unavailable",
    fact_ready: false,
    read_only: true
  };
}

export async function probeLocalDuckdbDatasetSnapshot(args = {}, options = {}) {
  const env = options.env || process.env;
  throwIfAborted(options.signal);
  const caseId = localCaseId(args, env);
  if (!caseId) throw new Error("Dataset snapshot probe requires a resolved case project.");
  const result = await runLocalSql({
    caseId,
    env,
    rowLimit: 1,
    sql: "SELECT 1 AS source_ready",
    bindDatasetSnapshot: true,
    snapshotMode: "probe_v2_with_legacy_fallback",
    signal: options.signal
  });
  throwIfAborted(options.signal);
  const producerReady = result.producer_content_contract === FUNDS_PRODUCER_CONTENT_CONTRACT_V1
    && /^fpc1_[a-f0-9]{64}$/u.test(text(result.producer_content_id));
  const legacyReady = result.snapshot_contract === "analytix_duckdb_dataset_snapshot_v1"
    && /^dsv1_[a-f0-9]{64}$/u.test(text(result.observed_dataset_snapshot_id));
  if (!producerReady && !legacyReady) {
    throw duckdbDiagnosticError({ code: "result_contract_invalid", stage: "snapshot_validation", sql: "SELECT 1 AS source_ready" });
  }
  return {
    case_id: caseId,
    observed_dataset_snapshot_id: legacyReady ? result.observed_dataset_snapshot_id : "",
    snapshot_contract: legacyReady ? result.snapshot_contract : "",
    snapshot_manifest_schema_version: legacyReady ? result.snapshot_manifest_schema_version : 0,
    producer_content_contract: producerReady ? result.producer_content_contract : "",
    producer_content_id: producerReady ? result.producer_content_id : "",
    producer_manifest_sha256: producerReady ? result.producer_manifest_sha256 : "",
    producer_manifest_schema_version: producerReady ? result.producer_manifest_schema_version : 0,
    ready: false,
    blocker: producerReady
      ? "host_dataset_snapshot_authority_unavailable"
      : text(result.snapshot_blocker || "funds_producer_content_manifest_v1_unavailable"),
    legacy_exact_count_only: legacyReady,
    read_only: true
  };
}

function semanticHintsForTable(tableName, columns = []) {
  const name = simpleTableName(tableName);
  const columnNames = new Set(columns.map((column) => text(column.name || column.column_name).toLowerCase()).filter(Boolean));
  const has = (...names) => names.every((column) => columnNames.has(column));
  if (name === "analysis_account_dim") {
    return {
      role: "账户维表/左侧账户树口径",
      preferred_for: ["户名名下账户", "账户归属", "银行/卡号展示", "账户 scope"],
      key_columns: ["account_key", "open_name", "id_no", "bank_name", "acct_display", "card_display"],
      join_hints: ["analysis_txn_daily_agg.acct_key = analysis_account_dim.account_key", "analysis_txn_detail_idx.acct_key = analysis_account_dim.account_key"]
    };
  }
  if (name === "analysis_txn_daily_agg") {
    return {
      role: "按日期/账户/对手方/方向聚合的交易索引",
      preferred_for: ["全案总览", "一人一档进出汇总", "Top 对手方", "趋势统计"],
      key_columns: ["txn_day", "acct_key", "cp_key", "cp_display", "cp_name", "dc_val", "txn_count", "amt_sum"],
      metric_columns: { amount: "amt_sum", count: "txn_count", direction: "dc_val", date: "txn_day" },
      join_hints: ["acct_key joins analysis_account_dim.account_key"]
    };
  }
  if (name === "analysis_txn_detail_idx") {
    return {
      role: "统计/可视分析默认交易明细表；用户问“交易明细表/明细表数据量”时使用",
      preferred_for: ["交易明细表数据量", "统计分析/可视分析交易明细口径", "逐笔证据表", "关键词线索", "同日快进快出", "金额挑战复核"],
      key_columns: ["id", "txn_ts", "txn_time", "acct_key", "cp_key", "dc_val", "amount", "balance", "summary", "remark", "txn_type"],
      metric_columns: { amount: "amount", direction: "dc_val", timestamp: "txn_ts" },
      evidence_boundary: "这是统计分析和可视分析默认交易明细索引；报告级金额仍应先聚合再表述。"
    };
  }
  if (name === "fc_transaction_norm") {
    return {
      role: "清洗后银行流水底表；不是统计/可视分析默认交易明细口径",
      preferred_for: ["清洗字段复核", "原始清洗口径核验", "analysis_txn_detail_idx 缺口回查"],
      key_columns: ["txn_ts", "txn_time", "acct_no_norm", "card_no_norm", "counterparty_acct_norm", "dc_final", "amount_val", "balance_val", "summary", "remark", "txn_type"],
      metric_columns: { amount: "amount_val", direction: "dc_final", timestamp: "txn_ts" },
      evidence_boundary: "默认交易明细数据量、统计分析和可视分析核验优先使用 analysis_txn_detail_idx；仅在需要核验清洗字段或索引差异时回到本表。"
    };
  }
  if (name === "fc_account_norm") {
    return {
      role: "清洗后的账户开户/余额信息",
      preferred_for: ["账户性质", "开户行", "余额/可用余额", "开户/销户状态"],
      key_columns: ["account_open_name", "opener_id_no", "card_no_norm", "acct_no_norm", "open_time_ts", "balance_val", "available_balance_val", "acct_status", "acct_type", "open_bank"]
    };
  }
  if (name === "fc_coercive_measure_norm") {
    return {
      role: "强制措施/冻结扣划等材料",
      preferred_for: ["司法冻结", "查控措施", "执行机关和文号线索"],
      key_columns: ["bank_name", "acct_no", "measure_type", "amount", "agency", "start_date", "end_date", "measure_seq_no"],
      evidence_boundary: "amount 多为文本字段，金额统计需 TRY_CAST 或逐条核验。"
    };
  }
  if (name === "analysis_key_node_features") {
    return {
      role: "重点节点特征索引",
      preferred_for: ["重点账户/对手方候选", "金额占比", "频次评分"],
      key_columns: ["node_key", "display_name", "txn_count", "total_amount", "in_amount", "out_amount", "amount_share", "key_score", "quality_label"],
      evidence_boundary: "评分是线索优先级，不是资金事实结论。"
    };
  }
  if (name === "analysis_relation_edge") {
    return {
      role: "实体关系/资金关系边索引",
      preferred_for: ["关系图谱候选", "节点间金额/频次概览"],
      key_columns: ["src_entity_id", "dst_entity_id", "relation_type", "txn_count", "amount_sum", "first_seen_at", "last_seen_at"],
      evidence_boundary: "关系边需回到交易明细或 trace path 支撑后才能画成确定资金链路。"
    };
  }
  if (name === "analysis_trace_path" || name === "analysis_trace_path_hop") {
    return {
      role: "资金追踪路径结果",
      preferred_for: ["资金穿透", "多跳路径", "下游去向"],
      key_columns: name === "analysis_trace_path"
        ? ["path_id", "trace_id", "hop_count", "path_score", "amount_match_rate", "sink_type", "summary"]
        : ["path_id", "hop_index", "txn_id", "txn_time", "amount", "direction", "attrs_json"],
      evidence_boundary: "路径结果属于分析产物，报告仍需列明关键逐笔交易或证据编号。"
    };
  }
  if (has("case_id", "row_hash")) {
    return {
      role: /^fc_/u.test(name) ? "清洗后案件材料表" : "分析索引表",
      preferred_for: /^fc_/u.test(name) ? ["字段覆盖核验", "专项事实补证"] : ["分析产物核验"],
      evidence_boundary: "先 profile 字段覆盖，再将样本/索引转为聚合或逐笔证据。"
    };
  }
  return {
    role: /^analysis_/u.test(name) ? "分析索引表" : "清洗表",
    evidence_boundary: "需结合字段覆盖和执行结果确认用途。"
  };
}

async function listAllowedTables({ caseId, env, tableLimit = 50, signal }) {
  throwIfAborted(signal);
  const tablesResult = await runLocalSql({
    caseId,
    env,
    rowLimit: Math.max(1, Math.min(tableLimit, 500)),
    sql: `
      SELECT table_schema, table_name, table_type
      FROM information_schema.tables
      WHERE table_schema NOT IN ('information_schema', 'pg_catalog')
        AND ${tableFilterSql()}
      ORDER BY table_name
    `,
    signal
  });
  const tables = (tablesResult.records || [])
    .filter((row) => isAllowedWorkbenchTableName(row.table_name))
    .slice(0, tableLimit)
    .map((row) => ({
      table_schema: text(row.table_schema) || "main",
      table_name: text(row.table_name),
      table_type: text(row.table_type) || "BASE TABLE"
    }));
  if (!tables.length) {
    const catalogComplete = tablesResult.truncated === false;
    return {
      tables: [],
      coreTables: {},
      completeness: {
        status: catalogComplete ? "complete" : "partial",
        table_catalog_complete: catalogComplete,
        column_catalog_complete: true,
        table_counts_complete: true
      }
    };
  }

  const tableNamesSql = tables.map((row) => sqlString(row.table_name)).join(", ");
  const columnsResult = await runLocalSql({
    caseId,
    env,
    rowLimit: Math.max(100, Math.min(tables.length * 100, 5000)),
    metadataMode: true,
    sql: `
      SELECT table_name, column_name, data_type, is_nullable, ordinal_position
      FROM information_schema.columns
      WHERE table_name IN (${tableNamesSql})
      ORDER BY table_name, ordinal_position
    `,
    signal
  });
  const columnsByTable = new Map();
  for (const row of columnsResult.records || []) {
    const tableName = text(row.table_name);
    if (!columnsByTable.has(tableName)) columnsByTable.set(tableName, []);
    const columnName = text(row.column_name);
    columnsByTable.get(tableName).push({
      name: columnName,
      sql_name: columnName,
      sql_identifier: quoteIdentifier(columnName),
      display_name: columnName,
      data_type: text(row.data_type),
      type: text(row.data_type),
      nullable: /^yes$/iu.test(text(row.is_nullable)),
      ordinal_position: strictNonNegativeInteger(row.ordinal_position),
      ordinal: strictNonNegativeInteger(row.ordinal_position),
      usage: "Use sql_name or sql_identifier in run_case_sql; display_name is UI-only."
    });
  }

  const coreTables = {};
  const output = [];
  const tableCountFailures = [];
  for (const table of tables) {
    let rowCount;
    try {
      const countResult = await runLocalSql({
        caseId,
        env,
        rowLimit: 1,
        sql: `SELECT COUNT(*) AS row_count FROM ${quoteIdentifier(table.table_name)}`,
        signal
      });
      const countRecord = requireCompleteAggregateRecord(countResult, `table count for ${table.table_name}`);
      rowCount = requireStrictCount(countRecord, "row_count", `table count for ${table.table_name}`);
    } catch (error) {
      throwIfAborted(signal);
      tableCountFailures.push({
        table_name: table.table_name,
        reason: text(error?.message || error).slice(0, 160)
      });
    }
    const semanticHints = semanticHintsForTable(table.table_name, columnsByTable.get(table.table_name) || []);
    const tableRecord = {
      ...table,
      sql_name: table.table_name,
      sql_identifier: quoteIdentifier(table.table_name),
      display_name: table.table_name,
      role: semanticHints.role || (/^analysis_/iu.test(table.table_name) ? "分析索引表" : "清洗表"),
      analysis_scope: /^analysis_/iu.test(table.table_name) ? "analysis_index" : "cleaned_norm",
      row_count: rowCount,
      column_count: (columnsByTable.get(table.table_name) || []).length,
      columns: columnsByTable.get(table.table_name) || [],
      raw_rows_exposed: false,
      semantic_hints: semanticHints
    };
    if ([
      "analysis_account_dim",
      "analysis_txn_daily_agg",
      "analysis_txn_detail_idx",
      "analysis_relation_edge",
      "analysis_evidence_ref",
      "fc_transaction_norm",
      "fc_account_norm"
    ].includes(table.table_name)) {
      coreTables[table.table_name] = {
        sql_name: tableRecord.sql_name,
        row_count: tableRecord.row_count,
        role: tableRecord.role
      };
    }
    output.push(tableRecord);
  }
  const tableCatalogComplete = tablesResult.truncated === false;
  const columnCatalogComplete = columnsResult.truncated === false;
  const tableCountsComplete = tableCountFailures.length === 0;
  return {
    tables: output,
    coreTables,
    completeness: {
      status: tableCatalogComplete && columnCatalogComplete && tableCountsComplete ? "complete" : "partial",
      table_catalog_complete: tableCatalogComplete,
      column_catalog_complete: columnCatalogComplete,
      table_counts_complete: tableCountsComplete,
      table_count_failures: tableCountFailures
    }
  };
}

async function executeLocalDuckdbInspectCaseSchema(args = {}, options = {}) {
  const startedAt = Date.now();
  const env = options.env || process.env;
  throwIfAborted(options.signal);
  const caseId = localCaseId(args, env);
  if (!caseId) throw new Error("Local DuckDB workbench requires resolved case_id.");
  const tableLimit = Math.max(1, Math.min(Number(args.table_limit || args.tableLimit || 20) || 20, 500));
  const { tables, coreTables } = await listAllowedTables({ caseId, env, tableLimit, signal: options.signal });
  const queryId = queryIdFor({ caseId, skillId: "inspect_case_schema", extra: String(tableLimit) });
  const payload = localWorkbenchEnvelope({
    skillId: "inspect_case_schema",
    caseId,
    data: {
      execution_status: "inspected",
      schema_status: "available",
      table_count: tables.length,
      tables,
      core_tables: coreTables,
      sql_name_contract: "Use tables[].sql_name and tables[].columns[].sql_name/sql_identifier for run_case_sql. display_name is UI-only and must not be used as executable SQL.",
      validation_summary: {
        readonly: true,
        current_case_only: true,
        allowed_tables: "analysis_* and fc_*_norm only",
        local_duckdb_fallback: true
      }
    },
    citations: { query_ids: [queryId] }
  });
  recordLocalWorkbenchHistory({
    skillId: "inspect_case_schema",
    caseId,
    args,
    queryId,
    purpose: "inspect cleaned/analysis schema",
    data: payload.data,
    startedAt,
    signal: options.signal
  });
  return payload;
}

async function executeLocalDuckdbProfileCaseSchema(args = {}, options = {}) {
  const startedAt = Date.now();
  const env = options.env || process.env;
  throwIfAborted(options.signal);
  const caseId = localCaseId(args, env);
  if (!caseId) throw new Error("Local DuckDB workbench requires resolved case_id.");
  const tableLimit = Math.max(1, Math.min(Number(args.table_limit || args.tableLimit || 8) || 8, 20));
  const columnLimit = Math.max(1, Math.min(Number(args.column_limit || args.columnLimit || 24) || 24, 80));
  const requestedTables = Array.isArray(args.tables) ? args.tables.map(text).filter(Boolean) : [];
  for (const table of requestedTables) {
    if (!isAllowedWorkbenchTableName(table)) {
      throw new Error(`Local DuckDB workbench rejected table outside cleaned/analysis scope: ${table}`);
    }
  }
  const { tables: allTables } = await listAllowedTables({
    caseId,
    env,
    tableLimit: requestedTables.length ? 500 : Math.max(tableLimit, 20),
    signal: options.signal
  });
  const selectedTables = allTables
    .filter((table) => !requestedTables.length || requestedTables.map(simpleTableName).includes(simpleTableName(table.table_name)))
    .slice(0, tableLimit);
  const profiles = [];
  for (const table of selectedTables) {
    const columns = (table.columns || []).slice(0, columnLimit);
    const columnProfiles = [];
    for (const column of columns) {
      const numericOrTemporal = isNumericOrTemporalType(column.data_type);
      const valueClauses = [
        "COUNT(*) AS row_count",
        `SUM(CASE WHEN ${quoteIdentifier(column.name)} IS NULL THEN 1 ELSE 0 END) AS null_count`,
        `approx_count_distinct(${quoteIdentifier(column.name)}) AS distinct_count`
      ];
      if (numericOrTemporal) {
        valueClauses.push(`MIN(${quoteIdentifier(column.name)}) AS min_value`);
        valueClauses.push(`MAX(${quoteIdentifier(column.name)}) AS max_value`);
      }
      try {
        const result = await runLocalSql({
          caseId,
          env,
          rowLimit: 1,
          sql: `SELECT ${valueClauses.join(", ")} FROM ${quoteIdentifier(table.table_name)}`,
          signal: options.signal
        });
        const row = requireCompleteAggregateRecord(result, `profile ${table.table_name}.${column.name}`);
        columnProfiles.push({
          column_name: column.name,
          sql_name: column.sql_name,
          data_type: column.data_type,
          row_count: requireStrictCount(row, "row_count", `profile ${table.table_name}.${column.name}`),
          null_count: requireStrictCount(row, "null_count", `profile ${table.table_name}.${column.name}`),
          distinct_count: requireStrictCount(row, "distinct_count", `profile ${table.table_name}.${column.name}`),
          min_value: numericOrTemporal ? row.min_value : undefined,
          max_value: numericOrTemporal ? row.max_value : undefined,
          value_samples_exposed: false
        });
      } catch (error) {
        throwIfAborted(options.signal);
        columnProfiles.push({
          column_name: column.name,
          sql_name: column.sql_name,
          data_type: column.data_type,
          profile_status: "unavailable",
          diagnostic: text(error?.message || error).slice(0, 160),
          value_samples_exposed: false
        });
      }
    }
    profiles.push({
      table_name: table.table_name,
      sql_name: table.sql_name,
      analysis_scope: table.analysis_scope,
      row_count: table.row_count,
      columns: columnProfiles
    });
  }
  const queryId = queryIdFor({ caseId, skillId: "profile_case_schema", extra: JSON.stringify({ requestedTables, tableLimit, columnLimit }) });
  const payload = localWorkbenchEnvelope({
    skillId: "profile_case_schema",
    caseId,
    data: {
      execution_status: "profiled",
      table_count: profiles.length,
      profiles,
      validation_summary: {
        readonly: true,
        current_case_only: true,
        allowed_tables: "analysis_* and fc_*_norm only",
        value_samples_exposed: false,
        local_duckdb_fallback: true
      }
    },
    citations: { query_ids: [queryId] }
  });
  recordLocalWorkbenchHistory({
    skillId: "profile_case_schema",
    caseId,
    args,
    queryId,
    purpose: "profile cleaned/analysis schema",
    data: payload.data,
    startedAt,
    signal: options.signal
  });
  return payload;
}

function validatePreviewWhereClause(whereSql) {
  const clause = stripSqlComments(whereSql);
  if (!clause || /^(?:1\s*=\s*1|true)$/iu.test(clause.trim())) {
    throw new Error("preview_case_rows requires a narrow where_sql filter; unfiltered samples are not allowed.");
  }
  if (FORBIDDEN_SQL_PATTERN.test(clause) || /;\s*\S/u.test(clause) || /(?:\.\.\.|…)/u.test(clause)) {
    throw new Error("Local DuckDB workbench rejected unsafe preview where_sql.");
  }
}

function validateCountWhereClause(whereSql) {
  const clause = stripSqlComments(whereSql).trim();
  if (!clause) return "";
  if (
    FORBIDDEN_SQL_PATTERN.test(clause) ||
    /;\s*\S/u.test(clause) ||
    /(?:\.\.\.|…)/u.test(clause) ||
    /\b(?:select|with|from|join)\b/iu.test(clause)
  ) {
    throw new Error("Local DuckDB workbench rejected unsafe count where_sql.");
  }
  return clause;
}

function resolveCountCaseRowsTable({ tableName, whereSql }) {
  const sourceTableName = simpleTableName(tableName);
  if (["fc_transaction", "fc_transaction_norm"].includes(sourceTableName) && !text(whereSql)) {
    return {
      tableName: "analysis_txn_detail_idx",
      sourceTableName,
      normalization: {
        applied: true,
        reason: "transaction_detail_total_count_prefers_analysis_txn_detail_idx",
        original_table_name: sourceTableName
      }
    };
  }
  return {
    tableName: sourceTableName,
    sourceTableName,
    normalization: { applied: false }
  };
}

function maskPreviewRecords(records = []) {
  return records.map((record) => {
    const output = {};
    for (const [key, value] of Object.entries(record || {})) {
      output[key] = isSensitiveColumn(key) ? maskSensitiveValue(value) : value;
    }
    return output;
  });
}

async function executeLocalDuckdbCountCaseRows(args = {}, options = {}) {
  const startedAt = Date.now();
  const env = options.env || process.env;
  throwIfAborted(options.signal);
  const caseId = localCaseId(args, env);
  if (!caseId) throw new Error("Local DuckDB workbench requires resolved case_id.");
  const requestedTableName = requireSqlIdentifier(args.table_name || args.tableName, "table_name");
  const whereSql = validateCountWhereClause(args.where_sql || args.whereSql);
  const tableSelection = resolveCountCaseRowsTable({ tableName: requestedTableName, whereSql });
  const tableName = requireSqlIdentifier(tableSelection.tableName, "table_name");
  if (!isAllowedWorkbenchTableName(tableName)) {
    throw new Error(`Local DuckDB workbench rejected table outside cleaned/analysis scope: ${tableName}`);
  }
  const sql = `SELECT COUNT(*) AS row_count FROM ${quoteIdentifier(tableName)}${whereSql ? ` WHERE ${whereSql}` : ""}`;
  validateReadOnlySql(sql);
  const snapshotBound = requestedTableName === "analysis_txn_detail_idx" && !hasOwn(args, "where_sql") && !hasOwn(args, "whereSql");
  const result = await runLocalSql({
    caseId,
    env,
    rowLimit: 1,
    sql,
    bindDatasetSnapshot: snapshotBound,
    signal: options.signal
  });
  const countRecord = requireCompleteAggregateRecord(result, "count_case_rows");
  const rowCount = requireStrictCount(countRecord, "row_count", "count_case_rows");
  const queryId = queryIdFor({ caseId, skillId: "count_case_rows", sql });
  const queryScope = {
    case_id: caseId,
    source_scope: [tableName],
    table_name: tableName,
    requested_table_name: tableSelection.sourceTableName,
    where_applied: Boolean(whereSql),
    where_hash: whereSql ? stableJsonHash({ where_sql: whereSql }) : "none",
    query_hash: stableJsonHash({ case_id: caseId, sql })
  };
  const completeness = resultCompleteness({ result, queryScope });
  const noHitStatus = rowCount === 0
    ? (completeness.status === "complete" ? "candidate_no_hit" : "unresolved")
    : "not_zero";
  const payload = localWorkbenchEnvelope({
    skillId: "count_case_rows",
    caseId,
    data: {
      execution_status: "counted",
      table_name: tableName,
      business_table_label: tableName === "analysis_txn_detail_idx" ? "交易明细表" : tableName,
      requested_table_name: tableSelection.sourceTableName,
      table_name_normalization: tableSelection.normalization,
      where_applied: Boolean(whereSql),
      row_count: rowCount,
      result_kind: "row_count_only",
      query_scope: queryScope,
      result_completeness: completeness,
      ...(snapshotBound ? {
        observed_dataset_snapshot_id: text(result.observed_dataset_snapshot_id),
        snapshot_contract: text(result.snapshot_contract)
      } : {}),
      zero_result_status: noHitStatus,
      raw_rows_exposed: false,
      validation_summary: {
        readonly: true,
        current_case_only: true,
        allowed_tables: "analysis_* and fc_*_norm only",
        local_duckdb_fallback: true
      }
    },
    citations: { query_ids: [queryId] }
  });
  recordLocalWorkbenchHistory({
    skillId: "count_case_rows",
    caseId,
    args,
    queryId,
    purpose: "count case rows",
    sql,
    data: payload.data,
    startedAt,
    signal: options.signal
  });
  return payload;
}

async function executeLocalDuckdbGetScopeCoverage(args = {}, options = {}) {
  const startedAt = Date.now();
  const env = options.env || process.env;
  throwIfAborted(options.signal);
  const caseId = localCaseId(args, env);
  if (!caseId) throw new Error("Local DuckDB workbench requires resolved case_id.");

  const tableInventory = await listAllowedTables({ caseId, env, tableLimit: 500, signal: options.signal });
  const tables = arrayOf(tableInventory.tables);
  const tableCounts = {};
  for (const table of tables) {
    const rowCount = strictNonNegativeInteger(table.row_count);
    if (rowCount !== undefined) {
      tableCounts[table.table_name] = rowCount;
    }
  }

  const detailSql = `
    SELECT
      COUNT(*) AS total_rows,
      COUNT(*) FILTER (
        WHERE amount IS NOT NULL
          AND amount_source_present = 1
          AND amount_parse_failed = 0
      ) AS amount_present_rows,
      COUNT(*) FILTER (
        WHERE amount IS NULL
          AND amount_source_present = 0
          AND amount_parse_failed = 0
      ) AS amount_missing_rows,
      COUNT(*) FILTER (
        WHERE amount IS NULL
          AND amount_source_present = 1
          AND amount_parse_failed = 1
      ) AS amount_parse_failed_rows,
      COUNT(DISTINCT acct_key) AS account_count,
      COUNT(DISTINCT account_open_name) FILTER (WHERE account_open_name IS NOT NULL AND TRIM(account_open_name) <> '') AS holder_count,
      COUNT(DISTINCT cp_key) FILTER (WHERE cp_key IS NOT NULL AND TRIM(cp_key) <> '') AS counterparty_key_count,
      COUNT(DISTINCT counterparty_name) FILTER (WHERE counterparty_name IS NOT NULL AND TRIM(counterparty_name) <> '') AS counterparty_name_count,
      MIN(txn_time) AS first_txn_at,
      MAX(txn_time) AS last_txn_at
    FROM analysis_txn_detail_idx
  `;
  const detailResult = await runLocalSql({ caseId, env, rowLimit: 1, sql: detailSql, signal: options.signal });
  const detail = requireCompleteAggregateRecord(detailResult, "get_scope_coverage");
  const totalRows = requireStrictCount(detail, "total_rows", "get_scope_coverage");
  const amountPresentRows = requireStrictCount(detail, "amount_present_rows", "get_scope_coverage");
  const amountMissingRows = requireStrictCount(detail, "amount_missing_rows", "get_scope_coverage");
  const amountParseFailedRows = requireStrictCount(detail, "amount_parse_failed_rows", "get_scope_coverage");
  if (amountPresentRows + amountMissingRows + amountParseFailedRows !== totalRows) {
    throw new Error("Local DuckDB get_scope_coverage returned inconsistent amount quality counts.");
  }
  const amountCoverageComplete = totalRows > 0 && amountPresentRows === totalRows;
  const amountCoverageRate = totalRows > 0 ? amountPresentRows / totalRows : undefined;
  const transactionDetail = {
    row_count: totalRows,
    txn_count: totalRows,
    total_rows: totalRows,
    amount_present_rows: amountPresentRows,
    amount_missing_rows: amountMissingRows,
    amount_parse_failed_rows: amountParseFailedRows,
    ...(amountCoverageRate !== undefined ? { amount_coverage_rate: amountCoverageRate } : {}),
    amount_coverage_complete: amountCoverageComplete,
    account_count: requireStrictCount(detail, "account_count", "get_scope_coverage"),
    holder_count: requireStrictCount(detail, "holder_count", "get_scope_coverage"),
    counterparty_key_count: requireStrictCount(detail, "counterparty_key_count", "get_scope_coverage"),
    counterparty_name_count: requireStrictCount(detail, "counterparty_name_count", "get_scope_coverage"),
    first_txn_at: detail.first_txn_at,
    last_txn_at: detail.last_txn_at
  };
  const coreTableNames = [
    "analysis_txn_detail_idx",
    "analysis_txn_daily_agg",
    "analysis_account_dim",
    "analysis_relation_edge",
    "analysis_evidence_ref",
    "fc_transaction_norm",
    "fc_account_norm"
  ];
  const coreTables = coreTableNames.map((tableName) => ({
    table_name: tableName,
    ...(hasOwn(tableCounts, tableName) ? { row_count: tableCounts[tableName] } : {}),
    status: hasOwn(tableCounts, tableName)
      ? "available"
      : (tables.some((table) => table.table_name === tableName) ? "count_unavailable" : "missing")
  }));
  const queryId = queryIdFor({ caseId, skillId: "get_scope_coverage", sql: detailSql });
  const queryScope = {
    case_id: caseId,
    source_scope: ["analysis_txn_detail_idx", ...Object.keys(tableCounts).filter((name) => name !== "analysis_txn_detail_idx")],
    predicate: "all_transaction_detail_rows",
    query_hash: stableJsonHash({ case_id: caseId, sql: detailSql }),
    table_catalog_limit: 500
  };
  const detailCompleteness = resultCompleteness({ result: detailResult, queryScope });
  const inventoryCompleteness = objectOf(tableInventory.completeness);
  const coverageComplete = detailCompleteness.status === "complete"
    && inventoryCompleteness.status === "complete"
    && amountCoverageComplete;
  const completeness = {
    status: coverageComplete ? "complete" : "partial",
    detail_query_complete: detailCompleteness.status === "complete",
    table_catalog_complete: inventoryCompleteness.table_catalog_complete === true,
    column_catalog_complete: inventoryCompleteness.column_catalog_complete === true,
    table_counts_complete: inventoryCompleteness.table_counts_complete === true,
    amount_coverage_complete: amountCoverageComplete,
    amount_present_rows: amountPresentRows,
    amount_missing_rows: amountMissingRows,
    amount_parse_failed_rows: amountParseFailedRows,
    ...(amountCoverageRate !== undefined ? { amount_coverage_rate: amountCoverageRate } : {}),
    table_count_failures: arrayOf(inventoryCompleteness.table_count_failures)
  };
  const noHitStatus = transactionDetail.row_count === 0 && coverageComplete
    ? "candidate_no_hit"
    : (transactionDetail.row_count === 0 ? "unresolved" : "not_zero");
  const coverage = {
    coverage_status: coverageComplete ? "complete" : "partial",
    source: "current_case_local_duckdb",
    query_scope: queryScope,
    result_completeness: completeness,
    zero_result_status: noHitStatus,
    core_tables: coreTables,
    table_counts: tableCounts,
    transaction_detail: transactionDetail,
    required_fact_tools: [
      "rank_accounts",
      "rank_holders",
      "rank_counterparties",
      "count_case_rows"
    ],
    evidence_boundary: "覆盖统计来自当前案件 DuckDB 的清洗/分析表；金额、链路和报告结论仍需按具体问题进入语义事实工具或专项资金核算。"
  };
  const payload = localWorkbenchEnvelope({
    skillId: "get_scope_coverage",
    caseId,
    data: {
      execution_status: coverageComplete ? "covered" : "partial",
      coverage,
      coverage_status: coverage.coverage_status,
      table_counts: tableCounts,
      transaction_detail: coverage.transaction_detail,
      core_tables: coreTables,
      query_scope: queryScope,
      result_completeness: completeness,
      zero_result_status: noHitStatus,
      validation_summary: {
        readonly: true,
        current_case_only: true,
        allowed_tables: "analysis_* and fc_*_norm only",
        local_duckdb_fallback: true,
        source_table: "analysis_txn_detail_idx",
        coverage_complete: coverageComplete
      }
    },
    warnings: [
      {
        code: "LOCAL_DUCKDB_COVERAGE_FALLBACK",
        severity: "info",
        message: "后端覆盖服务不可用时，已按当前案件 DuckDB 只读分析索引完成覆盖统计。"
      },
      ...(!amountCoverageComplete ? [{
        code: "AMOUNT_COVERAGE_PARTIAL",
        severity: "warning",
        message: "金额字段并非全部可用；本工具未输出任何金额合计，需补齐数据并由宿主覆盖执行器复核。"
      }] : [])
    ],
    nextActions: [
      {
        action: "普通行数可继续使用 count_case_rows；金额核验需等待宿主覆盖执行器通过完整性门。",
        source_refs: { query_ids: [queryId] }
      }
    ],
    citations: { query_ids: [queryId] }
  });
  recordLocalWorkbenchHistory({
    skillId: "get_scope_coverage",
    caseId,
    args,
    queryId,
    purpose: "local current-case scope coverage",
    sql: detailSql,
    data: payload.data,
    startedAt,
    signal: options.signal
  });
  return payload;
}

export function compileLocalDuckdbRankingSql(skillId, args = {}) {
  const normalizedSkillId = text(skillId);
  if (!["rank_accounts", "rank_holders", "rank_counterparties"].includes(normalizedSkillId)) {
    throw new Error("Local DuckDB ranking requires a supported ranking skill.");
  }
  const limit = clampInteger(args.limit, 10, 1, 200);
  const metric = normalizedRankingMetric(args.metric);
  const orderMetric = rankingOrderMetric(metric);
  const directionMode = text(args.direction_mode || args.directionMode || "both") || "both";
  const successFilter = text(args.success_filter || args.successFilter || "all") || "all";
  const cashFilter = text(args.cash_filter || args.cashFilter || "all") || "all";
  if (!["both", "in", "out"].includes(directionMode)) {
    throw new Error("Local DuckDB ranking requires a valid direction_mode.");
  }
  if (!["all", "success_only"].includes(successFilter)) {
    throw new Error("Local DuckDB ranking requires a valid success_filter.");
  }
  if (!["all", "cash_only", "non_cash_only"].includes(cashFilter)) {
    throw new Error("Local DuckDB ranking requires a valid cash_filter.");
  }
  const rawCounterpartyGroupMode = text(args.counterparty_group_mode || args.counterpartyGroupMode || "name");
  const counterpartyGroupMode = rawCounterpartyGroupMode === "account" ? "account" : "name";
  const includeSelfCounterparty = args.include_self_counterparty === true || args.includeSelfCounterparty === true;
  const counterpartyNameKeyExpr = "COALESCE(NULLIF(TRIM(counterparty_name), ''), NULLIF(TRIM(cp_name_pick), ''), NULLIF(TRIM(cp_name), ''), NULLIF(TRIM(stats_name_key), ''), '__unknown__')";
  const counterpartyDisplayNameExpr = "COALESCE(NULLIF(TRIM(counterparty_name), ''), NULLIF(TRIM(cp_name_pick), ''), NULLIF(TRIM(cp_name), ''), NULLIF(TRIM(stats_name_key), ''), '空户名')";
  const sourceHolderNameExpr = "COALESCE(NULLIF(TRIM(account_open_name), ''), '__source__')";
  const counterpartyNameBoundarySql = normalizedSkillId === "rank_counterparties" && counterpartyGroupMode === "name" && !includeSelfCounterparty
    ? `AND ${counterpartyDisplayNameExpr} <> '空户名' AND ${counterpartyDisplayNameExpr} <> ${sourceHolderNameExpr}`
    : "";
  const target = normalizedSkillId === "rank_counterparties" ? "counterparty" : "holder";
  const whereSql = rankingWhereClauses(args, target).join(" AND ");
  const coverageSql = rankingCoverageSql(normalizedSkillId, counterpartyGroupMode);
  validateReadOnlySql(coverageSql);
  let groupedSelect = "";
  let summaryCountField = "rank_count";
  let rankingLabel = "rankings";

  if (normalizedSkillId === "rank_accounts") {
    summaryCountField = "account_count";
    groupedSelect = `
      SELECT
        acct_key AS account_key,
        COALESCE(NULLIF(TRIM(acct_no), ''), NULLIF(TRIM(card_no), ''), acct_key) AS account_no,
        COALESCE(NULLIF(TRIM(card_no), ''), NULLIF(TRIM(acct_no), ''), acct_key) AS card_no,
        account_open_name AS holder_name,
        opener_id_no AS id_no,
        COUNT(*) AS txn_count,
        COUNT(DISTINCT cp_key) AS counterparty_count,
        SUM(CASE WHEN dc_val = '进' THEN ABS(amount) ELSE NULL END) AS inflow_total,
        SUM(CASE WHEN dc_val = '出' THEN ABS(amount) ELSE NULL END) AS outflow_total,
        SUM(ABS(amount)) AS turnover_total,
        SUM(CASE WHEN dc_val = '进' THEN ABS(amount) WHEN dc_val = '出' THEN -ABS(amount) ELSE NULL END) AS net_flow,
        MAX(ABS(amount)) AS max_single_amount,
        MIN(txn_time) AS first_txn_at,
        MAX(txn_time) AS last_txn_at
      FROM analysis_txn_detail_idx
      WHERE ${whereSql}
      GROUP BY acct_key, acct_no, card_no, account_open_name, opener_id_no`;
  } else if (normalizedSkillId === "rank_holders") {
    summaryCountField = "holder_count";
    groupedSelect = `
      SELECT
        account_open_name AS holder_name,
        MIN(opener_id_no) AS id_no,
        COUNT(DISTINCT acct_key) AS account_count,
        COUNT(*) AS txn_count,
        COUNT(DISTINCT cp_key) AS counterparty_count,
        SUM(CASE WHEN dc_val = '进' THEN ABS(amount) ELSE NULL END) AS inflow_total,
        SUM(CASE WHEN dc_val = '出' THEN ABS(amount) ELSE NULL END) AS outflow_total,
        SUM(ABS(amount)) AS turnover_total,
        SUM(CASE WHEN dc_val = '进' THEN ABS(amount) WHEN dc_val = '出' THEN -ABS(amount) ELSE NULL END) AS net_flow,
        MAX(ABS(amount)) AS max_single_amount,
        MIN(txn_time) AS first_txn_at,
        MAX(txn_time) AS last_txn_at
      FROM analysis_txn_detail_idx
      WHERE ${whereSql}
      GROUP BY account_open_name`;
  } else if (counterpartyGroupMode === "name") {
    summaryCountField = "counterparty_count";
    rankingLabel = "counterparty_rankings";
    groupedSelect = `
      SELECT
        ${counterpartyNameKeyExpr} AS counterparty_key,
        ${counterpartyDisplayNameExpr} AS display_name,
        MIN(counterparty_acct) AS counterparty_account,
        'name' AS counterparty_group_mode,
        COUNT(DISTINCT acct_key) AS source_account_count,
        COUNT(*) AS txn_count,
        SUM(CASE WHEN dc_val = '进' THEN ABS(amount) ELSE NULL END) AS inflow_total,
        SUM(CASE WHEN dc_val = '出' THEN ABS(amount) ELSE NULL END) AS outflow_total,
        SUM(ABS(amount)) AS turnover_total,
        SUM(CASE WHEN dc_val = '进' THEN ABS(amount) WHEN dc_val = '出' THEN -ABS(amount) ELSE NULL END) AS net_flow,
        MAX(ABS(amount)) AS max_single_amount,
        MIN(txn_time) AS first_txn_at,
        MAX(txn_time) AS last_txn_at
      FROM analysis_txn_detail_idx
      WHERE ${whereSql}
        ${counterpartyNameBoundarySql}
      GROUP BY counterparty_key, display_name`;
  } else {
    summaryCountField = "counterparty_count";
    rankingLabel = "counterparty_rankings";
    groupedSelect = `
      SELECT
        COALESCE(NULLIF(TRIM(cp_key), ''), NULLIF(TRIM(counterparty_acct), ''), NULLIF(TRIM(cp_raw), ''), '__unknown__') AS counterparty_key,
        MIN(COALESCE(NULLIF(TRIM(counterparty_name), ''), NULLIF(TRIM(cp_name_pick), ''), NULLIF(TRIM(cp_name), ''), NULLIF(TRIM(stats_name_key), ''), '空户名')) AS display_name,
        COALESCE(NULLIF(TRIM(counterparty_acct), ''), NULLIF(TRIM(cp_raw), ''), COALESCE(NULLIF(TRIM(cp_key), ''), '__unknown__')) AS counterparty_account,
        'account' AS counterparty_group_mode,
        COUNT(DISTINCT acct_key) AS source_account_count,
        COUNT(*) AS txn_count,
        SUM(CASE WHEN dc_val = '进' THEN ABS(amount) ELSE NULL END) AS inflow_total,
        SUM(CASE WHEN dc_val = '出' THEN ABS(amount) ELSE NULL END) AS outflow_total,
        SUM(ABS(amount)) AS turnover_total,
        SUM(CASE WHEN dc_val = '进' THEN ABS(amount) WHEN dc_val = '出' THEN -ABS(amount) ELSE NULL END) AS net_flow,
        MAX(ABS(amount)) AS max_single_amount,
        MIN(txn_time) AS first_txn_at,
        MAX(txn_time) AS last_txn_at
      FROM analysis_txn_detail_idx
      WHERE ${whereSql}
      GROUP BY counterparty_key, counterparty_account`;
  }

  const sql = `
    WITH grouped AS (${groupedSelect}
    )
    SELECT
      ROW_NUMBER() OVER (ORDER BY ${orderMetric} DESC, txn_count DESC) AS rank,
      *,
      COUNT(*) OVER () AS ${summaryCountField}
    FROM grouped
    ORDER BY ${orderMetric} DESC, txn_count DESC
    LIMIT ${limit}`;
  validateReadOnlySql(sql);
  return Object.freeze({
    skillId: normalizedSkillId,
    limit,
    metric,
    orderMetric,
    directionMode,
    successFilter,
    cashFilter,
    counterpartyGroupMode,
    includeSelfCounterparty,
    whereSql,
    coverageSql,
    sql,
    summaryCountField,
    rankingLabel
  });
}

async function executeLocalDuckdbRankingSkill(skillId, args = {}, options = {}) {
  const startedAt = Date.now();
  const env = options.env || process.env;
  throwIfAborted(options.signal);
  const caseId = localCaseId(args, env);
  if (!caseId) throw new Error("Local DuckDB workbench requires resolved case_id.");
  const authority = workbenchHistoryAuthority(args, caseId);
  if (!authority) {
    throw new Error("Local DuckDB ranking requires frozen same-turn dataset snapshot authority.");
  }
  const {
    limit,
    metric,
    orderMetric,
    directionMode,
    successFilter,
    cashFilter,
    counterpartyGroupMode,
    includeSelfCounterparty,
    whereSql,
    coverageSql,
    sql,
    summaryCountField,
    rankingLabel
  } = compileLocalDuckdbRankingSql(skillId, args);
  const coverageResult = await runLocalSql({
    caseId,
    env,
    rowLimit: 1,
    sql: coverageSql,
    bindDatasetSnapshot: true,
    expectedDatasetSnapshotId: authority.datasetSnapshotId,
    signal: options.signal
  });
  const fieldCoverage = validateRankingCoverage(coverageResult, {
    skillId,
    args,
    counterpartyGroupMode,
    includeSelfCounterparty
  });
  if (!factualSnapshotReady(coverageResult)) {
    throw duckdbDiagnosticError({
      code: "dataset_snapshot_mismatch",
      stage: "snapshot_validation",
      sql: coverageSql
    });
  }
  if (fieldCoverage.status !== "complete") {
    throw duckdbDiagnosticError({
      code: "amount_coverage_incomplete",
      stage: "result_validation",
      sql: coverageSql
    });
  }
  const result = await runLocalSql({
    caseId,
    env,
    rowLimit: limit,
    sql,
    bindDatasetSnapshot: true,
    expectedDatasetSnapshotId: authority.datasetSnapshotId,
    signal: options.signal
  });
  const queryId = queryIdFor({ caseId, skillId, sql });
  const rankings = result.records.map((record, index) => {
    const rank = requireStrictCount(record, "rank", `${skillId} row ${index + 1}`);
    if (rank < 1) {
      throw new Error(`${skillId} row ${index + 1} returned an invalid rank.`);
    }
    requireStrictCount(record, "txn_count", `${skillId} row ${index + 1}`);
    for (const countField of ["account_count", "counterparty_count", "source_account_count", summaryCountField]) {
      if (hasOwn(record, countField)) {
        requireStrictCount(record, countField, `${skillId} row ${index + 1}`);
      }
    }
    if (strictFiniteNumber(record?.[orderMetric]) === undefined) {
      throw new Error(`${skillId} row ${index + 1} returned an invalid ${orderMetric}.`);
    }
    return {
      ...record,
      rank,
      metric,
      metric_value: record[orderMetric]
    };
  });
  const resultComplete = result.truncated === false
    && factualSnapshotReady(result)
    && result.observed_dataset_snapshot_id === coverageResult.observed_dataset_snapshot_id
    && result.observed_dataset_snapshot_id === authority.datasetSnapshotId;
  const summaryCount = rankings.length
    ? requireStrictCount(rankings[0], summaryCountField, skillId)
    : (resultComplete ? 0 : undefined);
  if (summaryCount !== undefined && summaryCount < rankings.length) {
    throw new Error(`${skillId} returned a summary count smaller than its ranking rows.`);
  }
  const dataExhausted = resultComplete && summaryCount !== undefined ? summaryCount <= limit : undefined;
  const topContract = topNContract({
    requestedLimit: limit,
    resolvedLimit: limit,
    returnedCount: rankings.length,
    dataExhausted,
    resultComplete
  });
  const queryScope = {
    case_id: caseId,
    dataset_snapshot_id: authority.datasetSnapshotId,
    source_scope: ["analysis_txn_detail_idx"],
    query_hash: stableJsonHash({ case_id: caseId, dataset_snapshot_id: authority.datasetSnapshotId, coverage_sql: coverageSql, sql }),
    filter_hash: stableJsonHash({ where_sql: whereSql }),
    metric,
    direction_mode: directionMode,
    success_filter: successFilter,
    cash_filter: cashFilter,
    requested_limit: limit
  };
  const completeness = resultCompleteness({ result, queryScope });
  const noHitStatus = rankings.length === 0
    && dataExhausted === true
    && completeness.status === "complete"
    ? "candidate_no_hit"
    : (rankings.length === 0 ? "unresolved" : "not_zero");
  const summary = {
    metric,
    direction_mode: directionMode,
    success_filter: successFilter,
    cash_filter: cashFilter,
    limit,
    ...topContract,
    returned_count: rankings.length,
    ...(summaryCount === undefined ? {} : { [summaryCountField]: summaryCount }),
    query_scope: queryScope,
    result_completeness: completeness,
    field_coverage: fieldCoverage,
    zero_result_status: noHitStatus,
    local_duckdb_fallback: true
  };
  if (skillId === "rank_counterparties") {
    summary.counterparty_group_mode = counterpartyGroupMode;
    summary.include_self_counterparty = includeSelfCounterparty;
    summary.self_counterparty_filter = counterpartyGroupMode === "name" && !includeSelfCounterparty ? "excluded_empty_and_same_holder_name" : "not_applied";
  }
  const payload = localWorkbenchEnvelope({
    skillId,
    caseId,
    data: {
      execution_status: "ranked",
      rank_type: skillId,
      metric,
      order_metric: orderMetric,
      direction_mode: directionMode,
      success_filter: successFilter,
      cash_filter: cashFilter,
      ...(skillId === "rank_counterparties" ? {
        counterparty_group_mode: counterpartyGroupMode,
        include_self_counterparty: includeSelfCounterparty,
        self_counterparty_filter: counterpartyGroupMode === "name" && !includeSelfCounterparty ? "excluded_empty_and_same_holder_name" : "not_applied"
      } : {}),
      result_mode: "ranking",
      row_limit: limit,
      row_count: rankings.length,
      ...topContract,
      query_scope: queryScope,
      result_completeness: completeness,
      field_coverage: fieldCoverage,
      zero_result_status: noHitStatus,
      rankings,
      [rankingLabel]: rankings,
      records: rankings,
      top: rankings[0] || null,
      summary,
      validation_summary: {
        readonly: true,
        current_case_only: true,
        allowed_tables: "analysis_* and fc_*_norm only",
        local_duckdb_fallback: true,
        source_table: "analysis_txn_detail_idx"
      }
    },
    warnings: [
      {
        code: "LOCAL_DUCKDB_SEMANTIC_RANK_FALLBACK",
        severity: "info",
        message: "后端排行服务不可用时，已按当前案件 DuckDB 只读分析索引完成确定性排行。"
      }
    ],
    citations: { query_ids: [queryId] }
  });
  recordLocalWorkbenchHistory({
    skillId,
    caseId,
    args,
    queryId,
    purpose: `local semantic ranking ${skillId}`,
    sql,
    data: payload.data,
    startedAt,
    signal: options.signal
  });
  return payload;
}

function localTraceLimit(args = {}, fallback = 10, max = 100) {
  return clampInteger(
    args.top_n ?? args.topN ?? args.limit ?? args.row_limit ?? args.rowLimit,
    fallback,
    1,
    max
  );
}

function localTraceAmountTolerance(args = {}) {
  const raw = args.amount_tolerance ?? args.amount_tolerance_ratio;
  if (raw === undefined) return 0;
  const value = strictFiniteNumber(raw);
  if (value === undefined || value < 0 || value > 1) {
    throw new Error("Local trace_fund_next_hop requires amount_tolerance between zero and one.");
  }
  return value;
}

function terminalCategoryForRow(row, holderName = "") {
  const item = objectOf(row);
  const displayName = text(item.display_name || item.counterparty_name || item.counterparty_key);
  if (!displayName || displayName === "__unknown__" || /^(空户名|unknown|null|none)$/iu.test(displayName)) {
    return {
      terminal_category: "missing_counterparty",
      terminal_category_label: "对手方缺失",
      boundary: "对手方户名/账号缺失，只能作为未匹配终点或补证对象，不能写成最终流向。"
    };
  }
  if (holderName && displayName === holderName) {
    return {
      terminal_category: "self_counterparty_review",
      terminal_category_label: "同主体/自转待核",
      boundary: "对手方与本方户名相同，需核验换卡、补卡、同主体账户和重复事实，不能直接作为外部去向。"
    };
  }
  const targetAccountInCase = hasOwn(item, "target_account_in_case")
    ? requireStrictBinaryCount(item, "target_account_in_case", "trace terminal classification")
    : undefined;
  if (targetAccountInCase === 1) {
    return {
      terminal_category: "in_case_next_hop_candidate",
      terminal_category_label: "案内账户下一跳候选",
      boundary: "对手账号能在当前案件账户集合中匹配，可继续按该账户明细追下一跳；仍需逐笔余额承接或回单材料确认。"
    };
  }
  return {
    terminal_category: displayName.includes("公司") ? "direct_counterparty_company" : "direct_counterparty_person_or_other",
    terminal_category_label: displayName.includes("公司") ? "直接对手（单位）" : "直接对手（自然人/其他）",
    boundary: "当前只能说明直接出账对手；未取得收款侧完整流水、回单和用途材料前，不能认定最终去向或受益人。"
  };
}

function traceSeedTxnFromRow(row, context = "trace seed") {
  const item = objectOf(row);
  const amountField = hasOwn(item, "seed_amount") ? "seed_amount" : "amount";
  return {
    txn_id: text(item.seed_txn_id || item.txn_id),
    txn_time: text(item.seed_txn_time || item.txn_time),
    account_key: text(item.seed_account_key || item.acct_key),
    account_open_name: text(item.seed_holder_name || item.account_open_name),
    counterparty_key: text(item.counterparty_key || item.seed_counterparty_key || item.cp_key),
    counterparty_name: text(item.display_name || item.seed_counterparty_name || item.counterparty_name),
    amount: requireStrictPositiveNumber(item, amountField, context),
    direction: "out",
    summary: text(item.seed_summary || item.summary),
    txn_type: text(item.seed_txn_type || item.txn_type),
    file_id: text(item.seed_file_id || item.file_id)
  };
}

function followupReasonForTerminal(row) {
  const category = text(row.terminal_category);
  if (category === "missing_counterparty") {
    return "补调对手方账号/户名原始字段、银行回单和交易用途材料，先还原终点身份。";
  }
  if (category === "self_counterparty_review") {
    return "核验同主体账户、换卡/补卡、重复交易号和回单，防止把内部划转写成外部去向。";
  }
  if (category === "in_case_next_hop_candidate") {
    return "该对手账号可在当前案件账户集合中匹配，适合继续按收款账户追下一跳并核对余额承接。";
  }
  return "补收款账户完整流水、开户信息、银行回单、用途说明和账户归集材料，核验是否存在下一跳或资金断点。";
}

async function executeLocalDuckdbTraceSubjectTopOutflows(args = {}, options = {}) {
  const startedAt = Date.now();
  const env = options.env || process.env;
  throwIfAborted(options.signal);
  const caseId = localCaseId(args, env);
  if (!caseId) throw new Error("Local DuckDB workbench requires resolved case_id.");
  const holderName = text(args.holder_name || args.holderName);
  const limit = localTraceLimit(args, 10, 100);
  const traceArgs = {
    ...objectOf(args),
    direction_mode: "out"
  };
  const whereSql = rankingWhereClauses(traceArgs, "counterparty").join(" AND ");
  const scopeSql = `
    SELECT
      COUNT(*) AS txn_count,
      COUNT(DISTINCT acct_key) AS source_account_count,
      COUNT(DISTINCT COALESCE(NULLIF(TRIM(counterparty_name), ''), NULLIF(TRIM(cp_name_pick), ''), NULLIF(TRIM(cp_name), ''), NULLIF(TRIM(stats_name_key), ''), NULLIF(TRIM(counterparty_acct), ''), '__unknown__')) AS counterparty_count,
      SUM(amount) AS outflow_total,
      MIN(txn_time) AS first_txn_at,
      MAX(txn_time) AS last_txn_at,
      SUM(CASE WHEN counterparty_name IS NULL OR TRIM(counterparty_name) = '' THEN 1 ELSE 0 END) AS missing_counterparty_name_count,
      SUM(CASE WHEN counterparty_acct IS NULL OR TRIM(counterparty_acct) = '' THEN 1 ELSE 0 END) AS missing_counterparty_account_count
    FROM analysis_txn_detail_idx
    WHERE ${whereSql}`;
  const rankSql = `
    WITH scoped AS (
      SELECT
        id,
        txn_id,
        txn_time,
        txn_ts,
        acct_key,
        account_open_name,
        cp_key,
        counterparty_acct,
        COALESCE(NULLIF(TRIM(counterparty_name), ''), NULLIF(TRIM(cp_name_pick), ''), NULLIF(TRIM(cp_name), ''), NULLIF(TRIM(stats_name_key), ''), NULLIF(TRIM(counterparty_acct), ''), '__unknown__') AS counterparty_key,
        COALESCE(NULLIF(TRIM(counterparty_name), ''), NULLIF(TRIM(cp_name_pick), ''), NULLIF(TRIM(cp_name), ''), NULLIF(TRIM(stats_name_key), ''), '空户名') AS display_name,
        amount,
        summary,
        txn_type,
        file_id
      FROM analysis_txn_detail_idx
      WHERE ${whereSql}
    ),
    account_lookup AS (
      SELECT DISTINCT COALESCE(NULLIF(TRIM(acct_no), ''), NULLIF(TRIM(card_no), ''), acct_key) AS account_lookup
      FROM analysis_txn_detail_idx
      WHERE COALESCE(NULLIF(TRIM(acct_no), ''), NULLIF(TRIM(card_no), ''), acct_key) IS NOT NULL
    ),
    grouped AS (
      SELECT
        counterparty_key,
        display_name,
        MIN(counterparty_acct) AS counterparty_account,
        COUNT(*) AS txn_count,
        COUNT(DISTINCT acct_key) AS source_account_count,
        SUM(amount) AS outflow_total,
        MAX(amount) AS max_single_amount,
        MIN(txn_time) AS first_txn_at,
        MAX(txn_time) AS last_txn_at,
        MAX(CASE WHEN counterparty_acct IN (SELECT account_lookup FROM account_lookup) THEN 1 ELSE 0 END) AS target_account_in_case
      FROM scoped
      GROUP BY counterparty_key, display_name
    ),
    ranked AS (
      SELECT
        ROW_NUMBER() OVER (ORDER BY outflow_total DESC, txn_count DESC) AS rank,
        *,
        COUNT(*) OVER () AS total_counterparty_count
      FROM grouped
    ),
    seed_rows AS (
      SELECT *
      FROM (
        SELECT
          counterparty_key,
          display_name,
          txn_id AS seed_txn_id,
          txn_time AS seed_txn_time,
          acct_key AS seed_account_key,
          account_open_name AS seed_holder_name,
          cp_key AS seed_counterparty_key,
          counterparty_acct AS seed_counterparty_account,
          amount AS seed_amount,
          summary AS seed_summary,
          txn_type AS seed_txn_type,
          file_id AS seed_file_id,
          ROW_NUMBER() OVER (PARTITION BY counterparty_key, display_name ORDER BY amount DESC, txn_ts DESC, id DESC) AS seed_rank
        FROM scoped
      )
      WHERE seed_rank = 1
    )
    SELECT
      ranked.rank,
      ranked.counterparty_key,
      ranked.display_name,
      ranked.counterparty_account,
      ranked.txn_count,
      ranked.source_account_count,
      ranked.outflow_total,
      ranked.max_single_amount,
      ranked.first_txn_at,
      ranked.last_txn_at,
      ranked.target_account_in_case,
      ranked.total_counterparty_count,
      seed_rows.seed_txn_id,
      seed_rows.seed_txn_time,
      seed_rows.seed_account_key,
      seed_rows.seed_holder_name,
      seed_rows.seed_counterparty_key,
      seed_rows.seed_counterparty_account,
      seed_rows.seed_amount,
      seed_rows.seed_summary,
      seed_rows.seed_txn_type,
      seed_rows.seed_file_id
    FROM ranked
    LEFT JOIN seed_rows
      ON ranked.counterparty_key = seed_rows.counterparty_key
     AND ranked.display_name = seed_rows.display_name
    WHERE ranked.rank <= ${limit}
    ORDER BY ranked.rank`;
  validateReadOnlySql(scopeSql);
  validateReadOnlySql(rankSql);
  const [scopeResult, rankResult] = await Promise.all([
    runLocalSql({ caseId, env, rowLimit: 1, sql: scopeSql, signal: options.signal }),
    runLocalSql({ caseId, env, rowLimit: limit, sql: rankSql, signal: options.signal })
  ]);
  const scopeStats = requireCompleteAggregateRecord(scopeResult, "trace_subject_top_outflows scope");
  const scopeTxnCount = requireStrictCount(scopeStats, "txn_count", "trace_subject_top_outflows scope");
  const scopeSourceAccountCount = requireStrictCount(scopeStats, "source_account_count", "trace_subject_top_outflows scope");
  const scopeCounterpartyCount = requireStrictCount(scopeStats, "counterparty_count", "trace_subject_top_outflows scope");
  const scopeOutflowTotal = optionalStrictNumber(scopeStats, "outflow_total", "trace_subject_top_outflows scope");
  const missingCounterpartyNameCount = requireStrictCount(scopeStats, "missing_counterparty_name_count", "trace_subject_top_outflows scope");
  const missingCounterpartyAccountCount = requireStrictCount(scopeStats, "missing_counterparty_account_count", "trace_subject_top_outflows scope");
  const topOutflows = arrayOf(rankResult.records).map((record, index) => {
    const context = `trace_subject_top_outflows row ${index + 1}`;
    const rank = requireStrictPositiveInteger(record, "rank", context);
    const txnCount = requireStrictPositiveInteger(record, "txn_count", context);
    const sourceAccountCount = requireStrictPositiveInteger(record, "source_account_count", context);
    const outflowTotal = requireStrictPositiveNumber(record, "outflow_total", context);
    const maxSingleAmount = requireStrictPositiveNumber(record, "max_single_amount", context);
    const totalCounterpartyCount = requireStrictPositiveInteger(record, "total_counterparty_count", context);
    const targetAccountInCase = requireStrictBinaryCount(record, "target_account_in_case", context);
    if (rank > totalCounterpartyCount || totalCounterpartyCount !== scopeCounterpartyCount) {
      throw new Error(`${context} returned inconsistent rank or counterparty coverage.`);
    }
    const category = terminalCategoryForRow(record, holderName);
    return {
      rank,
      counterparty_key: text(record.counterparty_key),
      display_name: text(record.display_name),
      counterparty_name: text(record.display_name),
      counterparty_account: text(record.counterparty_account),
      txn_count: txnCount,
      source_account_count: sourceAccountCount,
      outflow_total: outflowTotal,
      amount_total: outflowTotal,
      max_single_amount: maxSingleAmount,
      first_txn_at: text(record.first_txn_at),
      last_txn_at: text(record.last_txn_at),
      ...(targetAccountInCase === 1
        ? { target_account_in_case: true, target_account_match_status: "matched" }
        : { target_account_match_status: "unverified" }),
      ...category,
      seed_txn: traceSeedTxnFromRow(record, `${context} seed`)
    };
  });
  if (scopeTxnCount === 0 && topOutflows.length > 0) {
    throw new Error("trace_subject_top_outflows returned rows outside its zero-row scope.");
  }
  if (scopeTxnCount > 0) {
    if (scopeSourceAccountCount === 0 || scopeCounterpartyCount === 0 || scopeOutflowTotal === undefined || scopeOutflowTotal <= 0 || topOutflows.length === 0) {
      throw new Error("trace_subject_top_outflows returned incomplete positive scope coverage.");
    }
  }
  const terminalSummaryMap = new Map();
  for (const row of topOutflows) {
    const key = text(row.terminal_category);
    const current = terminalSummaryMap.get(key) || {
      terminal_category: key,
      terminal_category_label: text(row.terminal_category_label),
      counterparty_count: 0,
      txn_count: 0,
      outflow_total: 0
    };
    current.counterparty_count += 1;
    current.txn_count += row.txn_count;
    current.outflow_total += row.outflow_total;
    terminalSummaryMap.set(key, current);
  }
  const queryId = queryIdFor({ caseId, skillId: "trace_subject_top_outflows", sql: rankSql });
  const zeroResultStatus = topOutflows.length > 0 ? "not_zero" : "unresolved";
  const traceSupportStatus = topOutflows.length > 0 ? "partial" : "unverified";
  const topContract = topNContract({
    requestedLimit: limit,
    resolvedLimit: limit,
    returnedCount: topOutflows.length,
    dataExhausted: topOutflows.length > 0 ? scopeCounterpartyCount <= limit : undefined,
    resultComplete: topOutflows.length > 0 && rankResult.truncated === false && factualSnapshotReady(rankResult)
  });
  const payload = localWorkbenchEnvelope({
    skillId: "trace_subject_top_outflows",
    caseId,
    data: {
      execution_status: "traced",
      result_mode: "top_outflows",
      holder_name: holderName,
      top_n: limit,
      ...topContract,
      row_limit: limit,
      row_count: topOutflows.length,
      trace_depth: 1,
      support_status: traceSupportStatus,
      coverage_status: "partial",
      zero_result_status: zeroResultStatus,
      scope_stats: {
        txn_count: scopeTxnCount,
        source_account_count: scopeSourceAccountCount,
        counterparty_count: scopeCounterpartyCount,
        ...(scopeOutflowTotal !== undefined ? { outflow_total: scopeOutflowTotal } : {}),
        first_txn_at: text(scopeStats.first_txn_at),
        last_txn_at: text(scopeStats.last_txn_at),
        missing_counterparty_name_count: missingCounterpartyNameCount,
        missing_counterparty_account_count: missingCounterpartyAccountCount,
        support_status: scopeTxnCount > 0 ? "bounded_observation" : "unverified"
      },
      top_outflows: topOutflows,
      terminal_summary: [...terminalSummaryMap.values()],
      followup_requests: topOutflows.slice(0, Math.min(limit, 20)).map((row) => ({
        target_name: row.display_name,
        target_account: row.counterparty_account,
        amount_total: row.outflow_total,
        txn_count: row.txn_count,
        reason: followupReasonForTerminal(row)
      })),
      validation_summary: {
        readonly: true,
        current_case_only: true,
        allowed_tables: "analysis_txn_detail_idx only",
        local_duckdb_fallback: true,
        source_table: "analysis_txn_detail_idx",
        trace_depth_supported: 1,
        graph_materialization_available: false,
        coverage_complete: false,
        fact_answer_allowed: false,
        zero_values_are_boundary_only: true
      }
    },
    warnings: [
      {
        code: "LOCAL_DUCKDB_TRACE_FALLBACK",
        severity: "info",
        message: topOutflows.length > 0
          ? "后端资金穿透语义服务不可用时，已按当前案件 DuckDB 明细索引返回一跳 Top 出账候选；正式案件事实仍需有效证据回执。"
          : "当前查询未返回可验证的一跳 Top 出账候选；显式零值只能作为覆盖边界，不能认定无出账或无关联。"
      },
      {
        code: "TRACE_DEPTH_BOUNDARY",
        severity: "warning",
        message: "本地 fallback 仅支持当前案件一跳直接出账和案内账号下一跳候选；最终受益人、余额承接和多跳穿透仍需收款侧流水/回单/开户材料或后端图谱服务。"
      }
    ],
    citations: { query_ids: [queryId] }
  });
  recordLocalWorkbenchHistory({
    skillId: "trace_subject_top_outflows",
    caseId,
    args,
    queryId,
    purpose: "local one-hop top outflow trace",
    sql: rankSql,
    data: payload.data,
    startedAt,
    signal: options.signal
  });
  return payload;
}

async function executeLocalDuckdbTraceFundNextHop(args = {}, options = {}) {
  const startedAt = Date.now();
  const env = options.env || process.env;
  throwIfAborted(options.signal);
  const caseId = localCaseId(args, env);
  if (!caseId) throw new Error("Local DuckDB workbench requires resolved case_id.");
  const seedTxnId = text(args.seed_txn_id || args.seedTxnId);
  const accountId = text(args.account_id || args.accountId || args.seed_account_id || args.seedAccountId);
  if (!seedTxnId && !accountId) {
    throw new Error("Local trace_fund_next_hop requires seed_txn_id or account_id.");
  }
  const limit = localTraceLimit(args, 20, 100);
  const tolerance = localTraceAmountTolerance(args);
  const windowMinutes = clampInteger(args.time_window_minutes ?? args.timeWindowMinutes, 43_200, 1, 525_600);
  const seedWhere = seedTxnId
    ? `txn_id = ${sqlString(seedTxnId)}`
    : `(acct_key = ${sqlString(accountId)} OR acct_no = ${sqlString(accountId)} OR card_no = ${sqlString(accountId)})`;
  const seedSql = `
    SELECT
      txn_id,
      txn_time,
      txn_ts,
      acct_key,
      account_open_name,
      counterparty_acct,
      counterparty_name,
      cp_key,
      amount,
      summary,
      txn_type,
      file_id
    FROM analysis_txn_detail_idx
    WHERE ${seedWhere}
      AND amount IS NOT NULL
    ORDER BY txn_ts DESC, amount DESC
    LIMIT 1`;
  validateReadOnlySql(seedSql);
  const seedResult = await runLocalSql({ caseId, env, rowLimit: 1, sql: seedSql, signal: options.signal });
  const seed = objectOf(seedResult.records?.[0]);
  if (!Object.keys(seed).length) {
    throw new Error("Local trace_fund_next_hop seed was not found in current case DuckDB.");
  }
  if (seedResult.truncated !== false || seedResult.row_count !== 1) {
    throw new Error("Local trace_fund_next_hop seed coverage is incomplete.");
  }
  const seedTxn = traceSeedTxnFromRow({
    seed_txn_id: seed.txn_id,
    seed_txn_time: seed.txn_time,
    seed_account_key: seed.acct_key,
    seed_holder_name: seed.account_open_name,
    seed_counterparty_key: seed.cp_key,
    display_name: seed.counterparty_name,
    seed_amount: seed.amount,
    seed_summary: seed.summary,
    seed_txn_type: seed.txn_type,
    seed_file_id: seed.file_id
  }, "trace_fund_next_hop seed");
  const receiverAccount = text(seed.counterparty_acct);
  const receiverName = text(seed.counterparty_name);
  const accountMatch = receiverAccount
    ? `(acct_key = ${sqlString(receiverAccount)} OR acct_no = ${sqlString(receiverAccount)} OR card_no = ${sqlString(receiverAccount)})`
    : receiverName
      ? `account_open_name = ${sqlString(receiverName)}`
      : "FALSE";
  const amountClause = tolerance > 0
    ? `AND amount BETWEEN ${seedTxn.amount * (1 - tolerance)} AND ${seedTxn.amount * (1 + tolerance)}`
    : "";
  const nextSql = `
    SELECT
      ROW_NUMBER() OVER (ORDER BY txn_ts ASC, amount DESC, id DESC) AS rank,
      txn_id,
      txn_time,
      txn_ts,
      acct_key,
      account_open_name,
      counterparty_acct,
      COALESCE(NULLIF(TRIM(counterparty_name), ''), NULLIF(TRIM(cp_name_pick), ''), NULLIF(TRIM(cp_name), ''), NULLIF(TRIM(stats_name_key), ''), '空户名') AS counterparty_name,
      cp_key,
      amount,
      summary,
      txn_type,
      file_id
    FROM analysis_txn_detail_idx
    WHERE ${accountMatch}
      AND dc_val = '出'
      AND txn_ts >= TIMESTAMP ${sqlString(text(seed.txn_ts))}
      AND txn_ts <= TIMESTAMP ${sqlString(text(seed.txn_ts))} + INTERVAL ${windowMinutes} MINUTE
      AND amount IS NOT NULL
      ${amountClause}
    ORDER BY txn_ts ASC, amount DESC, id DESC
    LIMIT ${limit}`;
  validateReadOnlySql(nextSql);
  const nextResult = await runLocalSql({ caseId, env, rowLimit: limit, sql: nextSql, signal: options.signal });
  const nextHops = arrayOf(nextResult.records).map((record, index) => {
    const context = `trace_fund_next_hop row ${index + 1}`;
    const rank = requireStrictPositiveInteger(record, "rank", context);
    const amount = requireStrictPositiveNumber(record, "amount", context);
    const category = terminalCategoryForRow({ ...record, display_name: record.counterparty_name }, receiverName);
    return {
      rank,
      txn_id: text(record.txn_id),
      txn_time: text(record.txn_time),
      from_account_key: text(record.acct_key),
      from_holder_name: text(record.account_open_name),
      counterparty_key: text(record.cp_key),
      counterparty_name: text(record.counterparty_name),
      counterparty_account: text(record.counterparty_acct),
      amount,
      summary: text(record.summary),
      txn_type: text(record.txn_type),
      file_id: text(record.file_id),
      ...category
    };
  });
  const queryId = queryIdFor({ caseId, skillId: "trace_fund_next_hop", sql: nextSql });
  const zeroResultStatus = nextHops.length > 0 ? "not_zero" : "unresolved";
  const traceSupportStatus = nextHops.length > 0 ? "candidate_next_hop" : "unverified";
  const topContract = topNContract({
    requestedLimit: limit,
    resolvedLimit: limit,
    returnedCount: nextHops.length,
    dataExhausted: nextHops.length > 0 && nextResult.truncated === false
      ? nextHops.length < limit
      : undefined,
    resultComplete: nextHops.length > 0 && nextResult.truncated === false && factualSnapshotReady(nextResult)
  });
  const payload = localWorkbenchEnvelope({
    skillId: "trace_fund_next_hop",
    caseId,
    data: {
      execution_status: "traced",
      result_mode: "next_hop",
      seed_txn: seedTxn,
      receiver_account: receiverAccount,
      receiver_name: receiverName,
      time_window_minutes: windowMinutes,
      amount_tolerance_ratio: tolerance,
      ...topContract,
      row_limit: limit,
      row_count: nextHops.length,
      support_status: traceSupportStatus,
      coverage_status: "partial",
      zero_result_status: zeroResultStatus,
      next_hops: nextHops,
      validation_summary: {
        readonly: true,
        current_case_only: true,
        allowed_tables: "analysis_txn_detail_idx only",
        local_duckdb_fallback: true,
        source_table: "analysis_txn_detail_idx",
        trace_depth_supported: 1,
        seed_found: true,
        ...(nextHops.length > 0 ? { receiver_account_matched_in_case: true } : {}),
        receiver_account_match_status: nextHops.length > 0 ? "candidate_rows_returned" : "unresolved",
        coverage_complete: false,
        fact_answer_allowed: false,
        zero_values_are_boundary_only: true
      }
    },
    warnings: [
      {
        code: "LOCAL_DUCKDB_NEXT_HOP_FALLBACK",
        severity: "info",
        message: "已按当前案件 DuckDB 明细索引从种子交易/账户追查一跳候选。"
      },
      ...(nextHops.length ? [] : [{
        code: "NEXT_HOP_NOT_VISIBLE_IN_CURRENT_CASE",
        severity: "warning",
        message: "当前案件内未匹配到收款账号/户名作为本方账户的后续出账；只能写为当前数据断点，不能认定最终去向。"
      }])
    ],
    citations: { query_ids: [queryId] }
  });
  recordLocalWorkbenchHistory({
    skillId: "trace_fund_next_hop",
    caseId,
    args,
    queryId,
    purpose: "local one-hop next-hop trace",
    sql: nextSql,
    data: payload.data,
    startedAt,
    signal: options.signal
  });
  return payload;
}

async function executeLocalDuckdbTraceFund(args = {}, options = {}) {
  const seedTxnId = text(args.seed_txn_id || args.seedTxnId);
  if (seedTxnId || text(args.account_id || args.accountId)) {
    const payload = await executeLocalDuckdbTraceFundNextHop(args, options);
    return {
      ...payload,
      skill_id: "trace_fund",
      data: {
        ...payload.data,
        execution_status: "traced",
        result_mode: "trace_fund_next_hop_fallback",
        trace_paths: arrayOf(payload.data.next_hops).map((hop) => ({
          hop_count: 1,
          seed_txn: payload.data.seed_txn,
          terminal_txn: hop,
          support_status: "candidate_next_hop"
        }))
      }
    };
  }
  return executeLocalDuckdbTraceSubjectTopOutflows(args, options);
}

async function executeLocalDuckdbPreviewCaseRows(args = {}, options = {}) {
  const startedAt = Date.now();
  const env = options.env || process.env;
  throwIfAborted(options.signal);
  const caseId = localCaseId(args, env);
  if (!caseId) throw new Error("Local DuckDB workbench requires resolved case_id.");
  const purpose = text(args.purpose);
  if (!purpose) throw new Error("Local DuckDB workbench requires purpose.");
  const tableName = requireSqlIdentifier(args.table_name || args.tableName, "table_name");
  if (!isAllowedWorkbenchTableName(tableName)) {
    throw new Error(`Local DuckDB workbench rejected table outside cleaned/analysis scope: ${tableName}`);
  }
  const whereSql = text(args.where_sql || args.whereSql);
  validatePreviewWhereClause(whereSql);
  const rowLimit = Math.max(1, Math.min(Number(args.row_limit || args.rowLimit || 20) || 20, 20));
  const columns = Array.isArray(args.columns) && args.columns.length
    ? args.columns.map((column) => quoteIdentifier(column)).join(", ")
    : "*";
  const sql = `SELECT ${columns} FROM ${quoteIdentifier(tableName)} WHERE ${whereSql} LIMIT ${rowLimit + 1}`;
  const sqlPolicy = await validateCaseSqlPolicy({ caseId, sql, env, signal: options.signal });
  const result = await runLocalSql({ caseId, env, rowLimit: rowLimit + 1, sql, signal: options.signal });
  const records = maskPreviewRecords((result.records || []).slice(0, rowLimit));
  const queryId = queryIdFor({ caseId, skillId: "preview_case_rows", sql });
  const payload = localWorkbenchEnvelope({
    skillId: "preview_case_rows",
    caseId,
    data: {
      execution_status: "previewed",
      purpose,
      table_name: tableName,
      columns: result.columns || [],
      records,
      row_count: records.length,
      truncated: Boolean((result.records || []).length > rowLimit || result.truncated),
      sample_only: true,
      sample_cannot_support_totals: true,
      sql_policy: sqlPolicy,
      validation_summary: {
        readonly: true,
        current_case_only: true,
        allowed_tables: "analysis_* and fc_*_norm only",
        privacy_projected: true,
        local_duckdb_fallback: true,
        sql_policy_engine: sqlPolicy.policy_engine,
        parser_binder_validated: true
      }
    },
    warnings: [
      {
        code: "PREVIEW_SAMPLE_NOT_AGGREGATE",
        severity: "warning",
        message: "样本只用于核验记录形态，不得外推金额、笔数、排名或资金链路。"
      }
    ],
    citations: { query_ids: [queryId] }
  });
  recordLocalWorkbenchHistory({
    skillId: "preview_case_rows",
    caseId,
    args,
    queryId,
    purpose,
    sql,
    data: payload.data,
    startedAt,
    signal: options.signal
  });
  return payload;
}

async function executeLocalDuckdbExplainCaseSql(args = {}, options = {}) {
  const startedAt = Date.now();
  const env = options.env || process.env;
  throwIfAborted(options.signal);
  const caseId = localCaseId(args, env);
  const purpose = text(args.purpose || args.query_request || args.queryRequest);
  const sql = stripTrailingSemicolon(args.sql);
  if (!caseId) throw new Error("Local DuckDB workbench requires resolved case_id.");
  if (!purpose) throw new Error("Local DuckDB workbench requires purpose.");
  if (!sql) throw new Error("Local DuckDB workbench requires sql.");
  const sqlPolicy = await validateCaseSqlPolicy({ caseId, sql, env, signal: options.signal });
  const includeAnalyze = Boolean(args.include_analyze || args.includeAnalyze);
  const rowLimit = Math.max(1, Math.min(Number(args.row_limit || args.rowLimit || 100) || 100, 500));
  const explainSql = `${includeAnalyze ? "EXPLAIN ANALYZE" : "EXPLAIN"} ${sql}`;
  const result = await runLocalSql({ caseId, env, rowLimit, sql: explainSql, signal: options.signal });
  const slowPath = slowPathDiagnostics({ sql, planRows: result.records || [] });
  const queryId = queryIdFor({ caseId, skillId: "explain_case_sql", sql: explainSql });
  const payload = localWorkbenchEnvelope({
    skillId: "explain_case_sql",
    caseId,
    data: {
      execution_status: "explained",
      purpose,
      include_analyze: includeAnalyze,
      plan_rows: result.records || [],
      row_count: result.row_count,
      row_limit: rowLimit,
      sql_shape: slowPath.sql_shape,
      slow_path_diagnostics: slowPath,
      sql_policy: sqlPolicy,
      validation_summary: {
        readonly: true,
        current_case_only: true,
        allowed_tables: "analysis_* and fc_*_norm only",
        returns_detail_rows: false,
        slow_path_likely: slowPath.slow_path_likely,
        local_duckdb_fallback: true,
        sql_policy_engine: sqlPolicy.policy_engine,
        parser_binder_validated: true
      }
    },
    citations: { query_ids: [queryId] }
  });
  recordLocalWorkbenchHistory({
    skillId: "explain_case_sql",
    caseId,
    args,
    queryId,
    purpose,
    sql,
    data: payload.data,
    startedAt,
    signal: options.signal
  });
  return payload;
}

async function executeLocalDuckdbDiagnoseCaseSql(args = {}, options = {}) {
  const startedAt = Date.now();
  const env = options.env || process.env;
  throwIfAborted(options.signal);
  const caseId = localCaseId(args, env);
  const purpose = text(args.purpose || args.query_request || args.queryRequest) || "查询问题诊断";
  const sql = stripTrailingSemicolon(args.sql);
  const errorMessage = text(args.error_message || args.errorMessage);
  if (!caseId) throw new Error("Local DuckDB workbench requires resolved case_id.");
  const diagnostics = [];
  let slowPath = null;
  let validationState = "diagnosed";
  let sqlPolicy = null;
  if (errorMessage) {
    if (/column|binder|not found|不存在|字段/iu.test(errorMessage)) diagnostics.push("疑似字段名或表名不匹配，先回到 inspect_case_schema/profile_case_schema 复核 SQL-safe 名称。");
    if (/syntax|parser|语法/iu.test(errorMessage)) diagnostics.push("疑似 SQL 语法不完整或含占位符，应改为完整单条 SELECT/WITH。");
    if (/permission|denied|readonly|read-only/iu.test(errorMessage)) diagnostics.push("疑似越过当前案件只读边界，必须限定清洗表或 analysis 索引。");
  }
  if (sql) {
    try {
      sqlPolicy = await validateCaseSqlPolicy({ caseId, sql, env, signal: options.signal });
      const probeSql = `SELECT * FROM (${sql}) AS local_probe LIMIT 1`;
      const probe = await runLocalSql({ caseId, env, rowLimit: 2, sql: probeSql, signal: options.signal });
      slowPath = slowPathDiagnostics({ sql, planRows: [] });
      diagnostics.push(probe.row_count > 0
        ? "查询可返回记录；若目标是报告级金额，应继续执行聚合型 run_case_sql 而非样本外推。"
        : "查询可执行但当前条件未命中记录；应复核时间、方向、账号、户名和清洗字段覆盖。");
    } catch (error) {
      throwIfAborted(options.signal);
      validationState = "blocked";
      sqlPolicy = error?.sql_policy || sqlPolicy;
      diagnostics.push(text(error?.message || error));
    }
  }
  if (!diagnostics.length) {
    diagnostics.push("未提供可诊断 SQL 或错误信息；先读取 inspect_case_schema，再对具体 SQL 做 explain/diagnose。");
  }
  const queryId = queryIdFor({ caseId, skillId: "diagnose_case_sql", sql, extra: errorMessage });
  const diagnosticMode = text(args.diagnostic_mode || args.diagnosticMode) || (errorMessage ? "error" : "validate");
  const diagnoseSlowPath = slowPath || (sql ? slowPathDiagnostics({ sql, planRows: [] }) : {});
  const diagnosis = await buildCaseSqlDiagnosis({
    caseId,
    env,
    sql,
    diagnostics,
    errorMessage,
    signal: options.signal
  });
  const nextActions = [
    ...arrayOf(diagnosis.corrective_actions),
    "字段或表名问题先回 inspect_case_schema/profile_case_schema。",
    "性能或全表扫描风险先用 explain_case_sql。",
    "空结果只能说明当前条件未命中，不能写成不存在资金事实。"
  ].filter(Boolean);
  const payload = localWorkbenchEnvelope({
    skillId: "diagnose_case_sql",
    caseId,
    data: {
      execution_status: validationState,
      purpose,
      diagnostic_mode: diagnosticMode,
      diagnostics,
      diagnosis,
      sql_policy: sqlPolicy,
      sql_shape: slowPath?.sql_shape || (sql ? sqlShapeDiagnostics(sql) : {}),
      slow_path_diagnostics: diagnoseSlowPath,
      audit_events: [
        pruneEmpty({
          event: `case_sql_diagnose:${validationState}`,
          tool: "diagnose_case_sql",
          diagnostic_mode: diagnosticMode,
          diagnostic_count: diagnostics.length,
          slow_path_likely: strictBoolean(diagnoseSlowPath.slow_path_likely),
          fact_policy: "diagnose_results_are_not_case_amount_or_flow_facts"
        })
      ],
      next_actions: nextActions,
      validation_summary: {
        readonly: true,
        current_case_only: true,
        allowed_tables: "analysis_* and fc_*_norm only",
        returns_detail_rows: false,
        local_duckdb_fallback: true
      }
    },
    citations: { query_ids: [queryId] }
  });
  recordLocalWorkbenchHistory({
    skillId: "diagnose_case_sql",
    caseId,
    args,
    queryId,
    purpose,
    sql,
    data: payload.data,
    startedAt,
    status: validationState,
    signal: options.signal
  });
  return payload;
}

function localCaseSqlRecipes({ caseId, category = "", limit = 50 }) {
  const recipes = [
    {
      recipe_id: "case_overview_from_daily_agg",
      category: "case_overview",
      purpose: "全案交易体量、进出金额和时间范围总览",
      parameters: [],
      preferred_tables: ["analysis_txn_daily_agg"],
      sql_template: [
        "SELECT",
        "  SUM(txn_count) AS txn_count,",
        "  SUM(CASE WHEN dc_val = '进' THEN amt_sum ELSE 0 END) AS in_amount,",
        "  SUM(CASE WHEN dc_val = '出' THEN amt_sum ELSE 0 END) AS out_amount,",
        "  MIN(first_ts) AS first_txn_ts,",
        "  MAX(last_ts) AS last_txn_ts,",
        "  COUNT(DISTINCT acct_key) AS account_count,",
        "  COUNT(DISTINCT cp_key) AS counterparty_count",
        "FROM analysis_txn_daily_agg"
      ].join("\n"),
      required_preflight: ["inspect_case_schema", "profile_case_schema for analysis_txn_daily_agg"],
      validation_gate: "run_case_sql aggregate; missing direction rows must be stated as coverage boundary"
    },
    {
      recipe_id: "holder_account_scope_overview",
      category: "subject_dossier",
      purpose: "按户名/证件号核验名下账户、进出金额、笔数和期间",
      parameters: ["holder_name optional", "id_no optional"],
      preferred_tables: ["analysis_account_dim", "analysis_txn_daily_agg"],
      sql_template: [
        "WITH holder_scope AS (",
        "  SELECT DISTINCT account_key, open_name, id_no, bank_name, acct_type",
        "  FROM analysis_account_dim",
        "  WHERE (:holder_name IS NULL OR open_name = :holder_name)",
        "    AND (:id_no IS NULL OR id_no = :id_no)",
        ")",
        "SELECT",
        "  COALESCE(open_name, '') AS holder_name,",
        "  COALESCE(id_no, '') AS id_no,",
        "  COUNT(DISTINCT holder_scope.account_key) AS account_count,",
        "  SUM(txn_count) AS txn_count,",
        "  SUM(CASE WHEN dc_val = '进' THEN amt_sum ELSE 0 END) AS in_amount,",
        "  SUM(CASE WHEN dc_val = '出' THEN amt_sum ELSE 0 END) AS out_amount,",
        "  MIN(first_ts) AS first_txn_ts,",
        "  MAX(last_ts) AS last_txn_ts",
        "FROM holder_scope",
        "LEFT JOIN analysis_txn_daily_agg ON analysis_txn_daily_agg.acct_key = holder_scope.account_key",
        "GROUP BY 1, 2",
        "ORDER BY (COALESCE(in_amount, 0) + COALESCE(out_amount, 0)) DESC, holder_name, id_no"
      ].join("\n"),
      required_preflight: ["inspect_case_schema", "profile_case_schema for analysis_account_dim/analysis_txn_daily_agg"],
      validation_gate: "holder/account scope must be stated before account-level conclusion"
    },
    {
      recipe_id: "holder_top_counterparties",
      category: "counterparty",
      purpose: "按户名/证件号统计进账或出账 Top 对手方",
      parameters: ["holder_name optional", "id_no optional", "direction one of 进/出", "limit"],
      preferred_tables: ["analysis_account_dim", "analysis_txn_daily_agg"],
      sql_template: [
        "WITH holder_scope AS (",
        "  SELECT DISTINCT account_key",
        "  FROM analysis_account_dim",
        "  WHERE (:holder_name IS NULL OR open_name = :holder_name)",
        "    AND (:id_no IS NULL OR id_no = :id_no)",
        ")",
        "SELECT",
        "  cp_display,",
        "  cp_name,",
        "  counterparty_bank,",
        "  SUM(txn_count) AS txn_count,",
        "  SUM(amt_sum) AS amount",
        "FROM analysis_txn_daily_agg",
        "WHERE acct_key IN (SELECT account_key FROM holder_scope)",
        "  AND dc_val = :direction",
        "GROUP BY cp_display, cp_name, counterparty_bank",
        "ORDER BY amount DESC",
        "LIMIT :limit"
      ].join("\n"),
      required_preflight: ["rank_counterparties first when semantic tool covers it", "inspect_case_schema only for custom scope"],
      validation_gate: "ranking is a clue package; pair amount conclusion needs bounded amount review"
    },
    {
      recipe_id: "one_hop_amount_by_parties",
      category: "pair_amount",
      purpose: "两方一跳金额核验",
      parameters: ["payer_name optional", "payer_account optional", "payee_name optional", "payee_account optional", "date_start optional", "date_end optional"],
      preferred_tables: ["analysis_txn_detail_idx"],
      sql_template: [
        "SELECT",
        "  COUNT(*) AS txn_count,",
        "  SUM(amount) AS amount,",
        "  MIN(txn_ts) AS first_txn_ts,",
        "  MAX(txn_ts) AS last_txn_ts",
        "FROM analysis_txn_detail_idx",
        "WHERE dc_val = '出'",
        "  AND (:payer_name IS NULL OR account_open_name = :payer_name)",
        "  AND (:payer_account IS NULL OR acct_key = :payer_account OR card_no = :payer_account OR acct_no = :payer_account)",
        "  AND (:payee_name IS NULL OR cp_name = :payee_name OR counterparty_name = :payee_name)",
        "  AND (:payee_account IS NULL OR cp_key = :payee_account OR counterparty_acct = :payee_account)",
        "  AND (:date_start IS NULL OR txn_ts >= CAST(:date_start AS TIMESTAMP))",
        "  AND (:date_end IS NULL OR txn_ts < CAST(:date_end AS TIMESTAMP))"
      ].join("\n"),
      required_preflight: ["inspect_case_schema", "profile_case_schema", "explain_case_sql"],
      validation_gate: "run_case_sql aggregate; do not use rank or preview as final amount"
    },
    {
      recipe_id: "keyword_evidence_hits",
      category: "suspicious_feature",
      purpose: "按摘要/备注/交易类型检索购车购房、理财、工程、现金等资金线索",
      parameters: ["keyword", "holder_name optional", "id_no optional", "limit"],
      preferred_tables: ["analysis_account_dim", "analysis_txn_detail_idx"],
      sql_template: [
        "WITH holder_scope AS (",
        "  SELECT DISTINCT account_key",
        "  FROM analysis_account_dim",
        "  WHERE (:holder_name IS NULL OR open_name = :holder_name)",
        "    AND (:id_no IS NULL OR id_no = :id_no)",
        ")",
        "SELECT",
        "  txn_ts, account_open_name, dc_val, amount, cp_name, counterparty_name,",
        "  summary, remark, txn_type",
        "FROM analysis_txn_detail_idx",
        "WHERE (:holder_name IS NULL OR acct_key IN (SELECT account_key FROM holder_scope))",
        "  AND (summary LIKE '%' || :keyword || '%'",
        "    OR remark LIKE '%' || :keyword || '%'",
        "    OR txn_type LIKE '%' || :keyword || '%'",
        "    OR merchant_name LIKE '%' || :keyword || '%')",
        "ORDER BY amount DESC NULLS LAST, txn_ts",
        "LIMIT :limit"
      ].join("\n"),
      required_preflight: ["profile_case_schema for text-field coverage", "preview_case_rows only for narrow sample checks"],
      validation_gate: "keyword hits are clues; final wording must state evidence boundary and next proof action"
    },
    {
      recipe_id: "coercive_measure_by_account",
      category: "coercive_measure",
      purpose: "按账号核验冻结、扣划、查控等强制措施",
      parameters: ["account optional", "holder_name optional", "id_no optional"],
      preferred_tables: ["analysis_account_dim", "fc_coercive_measure_norm"],
      sql_template: [
        "WITH holder_scope AS (",
        "  SELECT DISTINCT account_key, acct_display, card_display",
        "  FROM analysis_account_dim",
        "  WHERE (:holder_name IS NULL OR open_name = :holder_name)",
        "    AND (:id_no IS NULL OR id_no = :id_no)",
        ")",
        "SELECT",
        "  measure_type, agency, start_date, end_date, measure_seq_no,",
        "  acct_no, TRY_CAST(amount AS DOUBLE) AS amount_num, amount AS amount_text, remark",
        "FROM fc_coercive_measure_norm",
        "WHERE (:account IS NULL OR acct_no = :account)",
        "   OR acct_no IN (SELECT acct_display FROM holder_scope)",
        "   OR acct_no IN (SELECT card_display FROM holder_scope)",
        "ORDER BY start_date, amount_num DESC NULLS LAST"
      ].join("\n"),
      required_preflight: ["inspect_case_schema", "profile_case_schema for fc_coercive_measure_norm"],
      validation_gate: "amount is text in source; use TRY_CAST and keep original amount_text for review"
    },
    {
      recipe_id: "downstream_destinations",
      category: "fund_tracing",
      purpose: "收款后下游去向复核",
      parameters: ["receiver_name optional", "receiver_account optional", "date_start optional", "date_end optional", "limit"],
      preferred_tables: ["analysis_txn_detail_idx"],
      sql_template: [
        "SELECT",
        "  cp_name, counterparty_name, counterparty_acct, counterparty_bank,",
        "  COUNT(*) AS txn_count,",
        "  SUM(amount) AS amount,",
        "  MIN(txn_ts) AS first_txn_ts,",
        "  MAX(txn_ts) AS last_txn_ts",
        "FROM analysis_txn_detail_idx",
        "WHERE dc_val = '出'",
        "  AND (:receiver_name IS NULL OR account_open_name = :receiver_name)",
        "  AND (:receiver_account IS NULL OR acct_key = :receiver_account OR card_no = :receiver_account OR acct_no = :receiver_account)",
        "  AND (:date_start IS NULL OR txn_ts >= CAST(:date_start AS TIMESTAMP))",
        "  AND (:date_end IS NULL OR txn_ts < CAST(:date_end AS TIMESTAMP))",
        "GROUP BY cp_name, counterparty_name, counterparty_acct, counterparty_bank",
        "ORDER BY amount DESC",
        "LIMIT :limit"
      ].join("\n"),
      required_preflight: ["trace_subject_top_outflows", "profile_case_schema for date/account coverage"],
      validation_gate: "draw only supported in-case edges; unresolved endpoints become proof gaps"
    },
    {
      recipe_id: "account_transaction_overview",
      category: "account_dossier",
      purpose: "单账户交易概况、收支结构、对手方数量和交易期间",
      parameters: ["account_key optional", "account optional", "date_start optional", "date_end optional", "limit"],
      preferred_tables: ["analysis_txn_daily_agg", "analysis_account_dim"],
      sql_template: [
        "SELECT",
        "  agg.acct_key,",
        "  MAX(dim.open_name) AS holder_name,",
        "  MAX(dim.bank_name) AS bank_name,",
        "  SUM(agg.txn_count) AS txn_count,",
        "  SUM(CASE WHEN agg.dc_val = '进' THEN agg.amt_sum ELSE 0 END) AS in_amount,",
        "  SUM(CASE WHEN agg.dc_val = '出' THEN agg.amt_sum ELSE 0 END) AS out_amount,",
        "  COUNT(DISTINCT agg.cp_key) AS counterparty_count,",
        "  MIN(agg.first_ts) AS first_txn_ts,",
        "  MAX(agg.last_ts) AS last_txn_ts",
        "FROM analysis_txn_daily_agg agg",
        "LEFT JOIN analysis_account_dim dim ON dim.account_key = agg.acct_key",
        "WHERE (:account_key IS NULL OR agg.acct_key = :account_key)",
        "  AND (:account IS NULL OR dim.acct_display = :account OR dim.card_display = :account)",
        "  AND (:date_start IS NULL OR agg.txn_day >= CAST(:date_start AS DATE))",
        "  AND (:date_end IS NULL OR agg.txn_day < CAST(:date_end AS DATE))",
        "GROUP BY agg.acct_key",
        "ORDER BY (COALESCE(in_amount, 0) + COALESCE(out_amount, 0)) DESC, agg.acct_key",
        "LIMIT :limit"
      ].join("\n"),
      required_preflight: ["resolve_account_scope or inspect_case_schema", "profile_case_schema for analysis_txn_daily_agg/analysis_account_dim"],
      validation_gate: "run_case_sql aggregate; state account scope before account-role or abnormal-pattern conclusion"
    },
    {
      recipe_id: "large_amount_cash_review",
      category: "large_cash",
      purpose: "大额收支、大额存取现、现金断点和大额消费线索汇总",
      parameters: ["amount_threshold", "holder_name optional", "account_key optional", "date_start optional", "date_end optional"],
      preferred_tables: ["analysis_account_dim", "analysis_txn_detail_idx"],
      sql_template: [
        "WITH holder_scope AS (",
        "  SELECT DISTINCT account_key",
        "  FROM analysis_account_dim",
        "  WHERE (:holder_name IS NULL OR open_name = :holder_name)",
        ")",
        "SELECT",
        "  COUNT(*) AS txn_count,",
        "  SUM(amount) AS amount_sum,",
        "  SUM(CASE WHEN amount >= :amount_threshold THEN 1 ELSE 0 END) AS large_txn_count,",
        "  SUM(CASE WHEN amount >= :amount_threshold THEN amount ELSE 0 END) AS large_amount_sum,",
        "  SUM(CASE WHEN cash_flag IN ('01', '1') THEN 1 ELSE 0 END) AS cash_txn_count,",
        "  SUM(CASE WHEN cash_flag IN ('01', '1') THEN amount ELSE 0 END) AS cash_amount_sum",
        "FROM analysis_txn_detail_idx",
        "WHERE (:holder_name IS NULL OR acct_key IN (SELECT account_key FROM holder_scope))",
        "  AND (:account_key IS NULL OR acct_key = :account_key)",
        "  AND (:date_start IS NULL OR txn_ts >= CAST(:date_start AS TIMESTAMP))",
        "  AND (:date_end IS NULL OR txn_ts < CAST(:date_end AS TIMESTAMP))"
      ].join("\n"),
      required_preflight: ["detect_cash_breakpoints when cash-specific tool covers it", "profile_case_schema for cash_flag/amount coverage"],
      validation_gate: "cash_flag is a transaction clue; do not identify physical cash source/use without voucher or counter evidence"
    },
    {
      recipe_id: "fast_in_out_pattern_review",
      category: "flow_pattern",
      purpose: "短期集中入账后快速转出、快进快出、分散转出和归集转入候选统计",
      parameters: ["holder_name optional", "date_start optional", "date_end optional", "out_to_in_ratio default 0.8", "limit"],
      preferred_tables: ["analysis_account_dim", "analysis_txn_daily_agg", "analysis_rule_hit"],
      sql_template: [
        "WITH holder_scope AS (",
        "  SELECT DISTINCT account_key",
        "  FROM analysis_account_dim",
        "  WHERE (:holder_name IS NULL OR open_name = :holder_name)",
        "), daily AS (",
        "  SELECT acct_key, txn_day,",
        "    SUM(CASE WHEN dc_val = '进' THEN amt_sum ELSE 0 END) AS in_amount,",
        "    SUM(CASE WHEN dc_val = '出' THEN amt_sum ELSE 0 END) AS out_amount,",
        "    SUM(txn_count) AS txn_count,",
        "    COUNT(DISTINCT cp_key) AS counterparty_count",
        "  FROM analysis_txn_daily_agg",
        "  WHERE (:holder_name IS NULL OR acct_key IN (SELECT account_key FROM holder_scope))",
        "    AND (:date_start IS NULL OR txn_day >= CAST(:date_start AS DATE))",
        "    AND (:date_end IS NULL OR txn_day < CAST(:date_end AS DATE))",
        "  GROUP BY acct_key, txn_day",
        ")",
        "SELECT acct_key, txn_day, in_amount, out_amount, txn_count, counterparty_count,",
        "  CASE WHEN in_amount > 0 THEN out_amount / in_amount ELSE NULL END AS out_to_in_ratio",
        "FROM daily",
        "WHERE in_amount > 0 AND out_amount >= in_amount * :out_to_in_ratio",
        "ORDER BY out_amount DESC",
        "LIMIT :limit"
      ].join("\n"),
      required_preflight: ["get_rule_hits for FAST_IN_FAST_OUT/FAN_IN_FAN_OUT", "run_case_sql aggregate before writing pattern strength"],
      validation_gate: "pattern rows are abnormal clues; report must state time window, account scope, and proof boundary"
    },
    {
      recipe_id: "shared_device_contact_address_review",
      category: "association_clue",
      purpose: "同 IP、同 MAC、同终端、同联系方式、同地址等关联线索覆盖和候选",
      parameters: ["min_account_count default 2", "limit"],
      preferred_tables: ["analysis_txn_detail_idx", "fc_person_contact_norm", "fc_person_address_norm"],
      sql_template: [
        "WITH ip_links AS (",
        "  SELECT 'ip_addr' AS link_type, ip_addr AS link_value, COUNT(DISTINCT acct_key) AS account_count, COUNT(*) AS row_count",
        "  FROM analysis_txn_detail_idx",
        "  WHERE ip_addr IS NOT NULL AND ip_addr <> ''",
        "  GROUP BY ip_addr",
        "  HAVING COUNT(DISTINCT acct_key) >= :min_account_count",
        "), mac_links AS (",
        "  SELECT 'mac_addr' AS link_type, mac_addr AS link_value, COUNT(DISTINCT acct_key) AS account_count, COUNT(*) AS row_count",
        "  FROM analysis_txn_detail_idx",
        "  WHERE mac_addr IS NOT NULL AND mac_addr <> ''",
        "  GROUP BY mac_addr",
        "  HAVING COUNT(DISTINCT acct_key) >= :min_account_count",
        "), contact_links AS (",
        "  SELECT 'contact_phone' AS link_type, contact_phone AS link_value, COUNT(DISTINCT id_no) AS account_count, COUNT(*) AS row_count",
        "  FROM fc_person_contact_norm",
        "  WHERE contact_phone IS NOT NULL AND contact_phone <> ''",
        "  GROUP BY contact_phone",
        "  HAVING COUNT(DISTINCT id_no) >= :min_account_count",
        "), address_links AS (",
        "  SELECT 'home_addr' AS link_type, home_addr AS link_value, COUNT(DISTINCT id_no) AS account_count, COUNT(*) AS row_count",
        "  FROM fc_person_address_norm",
        "  WHERE home_addr IS NOT NULL AND home_addr <> ''",
        "  GROUP BY home_addr",
        "  HAVING COUNT(DISTINCT id_no) >= :min_account_count",
        ")",
        "SELECT * FROM ip_links",
        "UNION ALL SELECT * FROM mac_links",
        "UNION ALL SELECT * FROM contact_links",
        "UNION ALL SELECT * FROM address_links",
        "ORDER BY account_count DESC, row_count DESC, link_type, link_value",
        "LIMIT :limit"
      ].join("\n"),
      required_preflight: ["profile_case_schema for field coverage", "get_rule_hits for SHARED_IP_MULTI_ACCOUNT/SHARED_MAC_MULTI_ACCOUNT"],
      validation_gate: "shared device/contact/address is an association clue, not direct control or ownership proof"
    },
    {
      recipe_id: "abnormal_time_high_frequency_review",
      category: "time_pattern",
      purpose: "夜间、周末、短时间高频交易和异常交易时段复核；法定节假日需外部日历补证",
      parameters: ["holder_name optional", "date_start optional", "date_end optional", "hour_txn_threshold default 20", "limit"],
      preferred_tables: ["analysis_account_dim", "analysis_txn_detail_idx", "analysis_rule_hit"],
      sql_template: [
        "WITH holder_scope AS (",
        "  SELECT DISTINCT account_key",
        "  FROM analysis_account_dim",
        "  WHERE (:holder_name IS NULL OR open_name = :holder_name)",
        "), hourly AS (",
        "  SELECT acct_key, DATE_TRUNC('hour', txn_ts) AS hour_bucket, COUNT(*) AS txn_count, SUM(amount) AS amount_sum",
        "  FROM analysis_txn_detail_idx",
        "  WHERE txn_ts IS NOT NULL",
        "    AND (:holder_name IS NULL OR acct_key IN (SELECT account_key FROM holder_scope))",
        "    AND (:date_start IS NULL OR txn_ts >= CAST(:date_start AS TIMESTAMP))",
        "    AND (:date_end IS NULL OR txn_ts < CAST(:date_end AS TIMESTAMP))",
        "  GROUP BY acct_key, DATE_TRUNC('hour', txn_ts)",
        ")",
        "SELECT acct_key, hour_bucket, txn_count, amount_sum,",
        "  EXTRACT(hour FROM hour_bucket) AS hour_of_day,",
        "  CASE WHEN strftime(hour_bucket, '%w') IN ('0', '6') THEN TRUE ELSE FALSE END AS is_weekend",
        "FROM hourly",
        "WHERE txn_count >= :hour_txn_threshold",
        "   OR EXTRACT(hour FROM hour_bucket) BETWEEN 0 AND 5",
        "   OR strftime(hour_bucket, '%w') IN ('0', '6')",
        "ORDER BY txn_count DESC, amount_sum DESC, acct_key, hour_bucket",
        "LIMIT :limit"
      ].join("\n"),
      required_preflight: ["get_rule_hits for NIGHT_OFFHOUR_ACTIVITY/HIGH_FREQ_SMALL_OUT", "profile_case_schema for txn_ts coverage"],
      validation_gate: "time pattern supports abnormal clue language only; do not infer purpose or actor intent; statutory-holiday conclusion requires external calendar evidence"
    },
    {
      recipe_id: "fund_usage_keyword_classifier",
      category: "fund_usage",
      purpose: "交易摘要、备注、类型和商户文本的逐字关键词命中检查；只返回待复核文本线索，不归类资金用途或项目身份",
      parameters: ["holder_name optional", "date_start optional", "date_end optional", "limit"],
      preferred_tables: ["analysis_account_dim", "analysis_txn_detail_idx", "analysis_txn_keyword_idx"],
      sql_template: [
        "WITH holder_scope AS (",
        "  SELECT DISTINCT account_key",
        "  FROM analysis_account_dim",
        "  WHERE (:holder_name IS NULL OR open_name = :holder_name)",
        "), tagged AS (",
        "  SELECT",
        "    CASE WHEN regexp_matches(",
        "      COALESCE(summary, '') || ' ' || COALESCE(remark, '') || ' ' || COALESCE(txn_type, '') || ' ' || COALESCE(merchant_name, ''),",
        "      '车|汽车|购车|车辆|停车|房|购房|物业|装修|不动产|理财|基金|证券|股票|保险|投资|贷款|还款|借款|利息|工程|项目|货款|材料|现金|ATM|取现|存现'",
        "    ) OR cash_flag IN ('01', '1') THEN 'transaction_text_keyword_hit' ELSE 'no_keyword_hit' END AS lead_status,",
        "    amount",
        "  FROM analysis_txn_detail_idx",
        "  WHERE (:holder_name IS NULL OR acct_key IN (SELECT account_key FROM holder_scope))",
        "    AND (:date_start IS NULL OR txn_ts >= CAST(:date_start AS TIMESTAMP))",
        "    AND (:date_end IS NULL OR txn_ts < CAST(:date_end AS TIMESTAMP))",
        ")",
        "SELECT lead_status, COUNT(*) AS txn_count, SUM(amount) AS amount_sum",
        "FROM tagged",
        "WHERE lead_status = 'transaction_text_keyword_hit'",
        "GROUP BY lead_status",
        "ORDER BY amount_sum DESC, lead_status",
        "LIMIT :limit"
      ].join("\n"),
      required_preflight: ["profile_case_schema for exact text-field coverage", "independent authoritative source receipt for any purpose, asset, project, or destination claim"],
      validation_gate: "transaction_text_keyword_hit records only literal source text; it cannot establish purpose, asset ownership, project identity, final destination, collusion, bribery, or benefit transfer"
    },
    {
      recipe_id: "account_role_candidate_classification",
      category: "account_role",
      purpose: "重点账户、中转账户、沉淀账户、终端账户、疑似控制账户等账户角色候选",
      parameters: ["limit"],
      preferred_tables: ["analysis_key_node_features", "analysis_txn_daily_agg", "analysis_rule_hit"],
      sql_template: [
        "SELECT",
        "  node_key, display_name, txn_count, total_amount, in_amount, out_amount, amount_share,",
        "  CASE",
        "    WHEN in_amount > 0 AND out_amount / in_amount >= 0.8 AND txn_count >= 20 THEN 'transfer_hub_candidate'",
        "    WHEN in_amount > out_amount * 3 THEN 'accumulation_candidate'",
        "    WHEN out_amount > in_amount * 3 THEN 'terminal_outflow_candidate'",
        "    WHEN amount_share >= 0.1 THEN 'key_account_candidate'",
        "    ELSE 'review_candidate'",
        "  END AS role_candidate,",
        "  quality_label, reasons_json",
        "FROM analysis_key_node_features",
        "ORDER BY key_score DESC, total_amount DESC, node_key",
        "LIMIT :limit"
      ].join("\n"),
      required_preflight: ["rank_accounts/rank_holders first when enough", "get_rule_hits for supporting abnormal features"],
      validation_gate: "role_candidate is a scoring clue; actual control/terminal status requires path and external materials"
    },
    {
      recipe_id: "fund_path_evidence_support_check",
      category: "visual_evidence",
      purpose: "资金流向图、路径边和证据引用支撑完整性核验",
      parameters: [],
      preferred_tables: ["analysis_relation_edge", "analysis_entity_node", "analysis_trace_path", "analysis_trace_path_hop", "analysis_txn_detail_idx", "analysis_evidence_ref"],
      sql_template: [
        "SELECT",
        "  (SELECT COUNT(*) FROM analysis_relation_edge) AS edge_count,",
        "  (SELECT COUNT(*) FROM analysis_relation_edge edge",
        "    LEFT JOIN analysis_entity_node src ON src.entity_id = edge.src_entity_id",
        "    LEFT JOIN analysis_entity_node dst ON dst.entity_id = edge.dst_entity_id",
        "    WHERE src.entity_id IS NULL OR dst.entity_id IS NULL) AS missing_endpoint_edge_count,",
        "  (SELECT COUNT(*) FROM analysis_trace_path_hop) AS trace_hop_count,",
        "  (SELECT COUNT(*) FROM analysis_trace_path_hop hop",
        "    LEFT JOIN analysis_trace_path path ON path.path_id = hop.path_id",
        "    LEFT JOIN analysis_txn_detail_idx txn ON CAST(txn.id AS VARCHAR) = CAST(hop.txn_id AS VARCHAR)",
        "      OR CAST(txn.txn_id AS VARCHAR) = CAST(hop.txn_id AS VARCHAR)",
        "    WHERE path.path_id IS NULL OR txn.id IS NULL) AS missing_trace_support_count,",
        "  (SELECT COUNT(*) FROM analysis_evidence_ref",
        "    WHERE ref_table IS NULL OR ref_pk IS NULL OR ref_table = '') AS malformed_evidence_ref_count"
      ].join("\n"),
      required_preflight: ["build_fund_flow_graph or trace_fund first for user-visible graph", "run_case_sql support check before report graph"],
      validation_gate: "visual graph/table output must bind each supported edge/path to transaction or evidence refs; candidate edges stay bounded"
    },
    {
      recipe_id: "same_fact_duplicate_review",
      category: "data_quality",
      purpose: "同事实/换卡重复候选复核",
      parameters: ["holder_name optional", "id_no optional", "amount optional", "date_start optional", "date_end optional"],
      preferred_tables: ["analysis_account_dim", "analysis_txn_detail_idx", "fc_transaction_norm"],
      sql_template: [
        "WITH holder_scope AS (",
        "  SELECT DISTINCT account_key",
        "  FROM analysis_account_dim",
        "  WHERE (:holder_name IS NULL OR open_name = :holder_name)",
        "    AND (:id_no IS NULL OR id_no = :id_no)",
        ")",
        "SELECT",
        "  txn_ts, account_open_name, acct_key, dc_val, amount, balance,",
        "  cp_name, counterparty_name, counterparty_acct, summary, txn_id",
        "FROM analysis_txn_detail_idx",
        "WHERE (:holder_name IS NULL OR acct_key IN (SELECT account_key FROM holder_scope))",
        "  AND (:amount IS NULL OR amount = :amount)",
        "  AND (:date_start IS NULL OR txn_ts >= CAST(:date_start AS TIMESTAMP))",
        "  AND (:date_end IS NULL OR txn_ts < CAST(:date_end AS TIMESTAMP))",
        "ORDER BY txn_ts, amount DESC"
      ].join("\n"),
      required_preflight: ["resolve_duplicate_families", "audit_case_data_quality"],
      validation_gate: "candidate duplicate amount is not deducted until confirmed"
    }
  ];
  const filtered = category
    ? recipes.filter((recipe) => recipe.category === category || recipe.recipe_id === category)
    : recipes;
  const projected = filtered.slice(0, limit).map((recipe) => {
    const amountFactRecipe = arrayOf(recipe.preferred_tables)
      .some((tableName) => UNSCOPED_AMOUNT_FACT_TABLES.has(text(tableName).toLowerCase()));
    return amountFactRecipe
      ? {
          ...recipe,
          execution_status: "blocked",
          blocker: "host_amount_coverage_executor_required",
          validation_gate: "A dedicated host-owned executor must prove complete v12 amount and cleaning-state coverage before filters, thresholds, ordering or LIMIT."
        }
      : recipe;
  });
  return {
    skill_id: "case_sql_recipes",
    status: "ok",
    data: {
      workbench_contract: "local_duckdb_readonly_v1",
      execution_status: "listed",
      case_id: caseId,
      source_scope: "reviewed_local_templates",
      recipe_count: Math.min(filtered.length, limit),
      recipes: projected,
      recipe_contract: "Recipes are reviewed starting points only. Amount-bearing templates are blocked until a dedicated host-owned coverage executor is available; arbitrary run_case_sql/notebook execution cannot bypass this gate."
    },
    warnings: [
      {
        code: "RECIPES_ARE_NOT_FACTS",
        severity: "info",
        message: "专项模板不是案件事实；金额模板当前固定阻断，不能通过任意 SQL 或 notebook 生成部分金额。"
      }
    ],
    citations: { query_ids: [] }
  };
}

function localWorkbenchHistory({ caseId, args = {}, limit = 20 }) {
  const authority = workbenchHistoryAuthority(args, caseId);
  const entries = LOCAL_WORKBENCH_HISTORY
    .filter((entry) => authority && text(entry.case_id) === text(caseId) && entry.authority_key === authority.key)
    .slice(0, Math.max(1, Math.min(Number(limit || 20) || 20, 50)))
    .map(({ authority_key: _authorityKey, ...entry }) => entry);
  return {
    skill_id: "inspect_workbench_history",
    status: "ok",
    data: {
      workbench_contract: "local_duckdb_readonly_v1",
      execution_status: "listed",
      case_id: caseId,
      source_scope: "current_case_local_duckdb",
      history_available: Boolean(authority && entries.length > 0),
      limit,
      entries,
      diagnostic: authority && entries.length
        ? "已返回当前 MCP 进程内最近工作台调用摘要；仅含 digest 和核验元数据，不含 SQL 文本、明细行、敏感路径或 debug payload。"
        : authority
          ? "本地 DuckDB 兜底未发现当前同 thread、turn、case binding、epoch 和 dataset snapshot 的工作台历史；不读取其他上下文、历史线程、旧 evidence 或本地输出目录。"
          : "缺少完整冻结宿主上下文，不能读取进程内工作台历史；不按 active 或仅按 case_id 回退。"
    },
    warnings: [
      {
        code: authority && entries.length ? "LOCAL_HISTORY_IN_PROCESS_ONLY" : "LOCAL_HISTORY_UNAVAILABLE",
        severity: "info",
        message: authority && entries.length
          ? "本地历史仅覆盖当前 MCP 进程内调用；持久化可回放历史仍应使用后端审计记录。"
          : "当前同一冻结上下文未返回本地历史核算摘要；如需可回放历史，应使用后端审计记录。"
      }
    ],
    citations: { query_ids: [] }
  };
}

function safeFileSegment(value, fallback = "case-notebook") {
  return text(value || fallback)
    .replace(/[\\/:*?"<>|#%{}^[\]`]/gu, "-")
    .replace(/\s+/gu, "-")
    .replace(/-+/gu, "-")
    .replace(/^-+|-+$/gu, "")
    .slice(0, 80) || fallback;
}

function notebookArtifactRoot({ caseId, args = {}, env = process.env, signal } = {}) {
  throwIfAborted(signal);
  const configured = text(env.ANALYTIX_FUNDS_ARTIFACT_DIR);
  const projectContext = resolveCaseProjectContext(args, env);
  const projectRoot = text(projectContext?.workspace_root);
  const home = text(env.HOME) || ".";
  const root = assertAnalytixArtifactRoot(
    configured || (projectRoot
      ? path.join(projectRoot, ...DEFAULT_ANALYTIX_ARTIFACT_PARTS)
      : path.join(home, ...DEFAULT_ANALYTIX_ARTIFACT_PARTS)),
    { env }
  );
  throwIfAborted(signal);
  const caseRoot = path.join(root, "cases", safeFileSegment(caseId, "case"), "notebooks");
  fs.mkdirSync(caseRoot, { recursive: true });
  return caseRoot;
}

function notebookSourceLines(value) {
  const source = text(value);
  if (!source) return [];
  return source.split(/\r?\n/u).map((line, index, lines) => `${line}${index < lines.length - 1 ? "\n" : ""}`);
}

function markdownCell(source, metadata = {}) {
  return {
    cell_type: "markdown",
    metadata,
    source: notebookSourceLines(source)
  };
}

function codeCell(source, metadata = {}) {
  return {
    cell_type: "code",
    execution_count: null,
    metadata,
    outputs: [],
    source: notebookSourceLines(source)
  };
}

function notebookCellsFromArgs(source = {}) {
  const cells = Array.isArray(source.cells) ? source.cells.map(objectOf) : [];
  const directSql = text(source.sql);
  if (directSql) {
    return [
      {
        type: "query",
        purpose: text(source.purpose || source.analysis_goal || source.analysisGoal || source.title) || "专项资金核算",
        sql: directSql
      },
      ...cells
    ];
  }
  return cells;
}

function visibleNotebookResultRecords({ result, sql }) {
  const records = Array.isArray(result?.records) ? result.records : [];
  const shape = sqlShapeDiagnostics(sql);
  if (!shape.aggregate_like || records.length > 20) {
    return [];
  }
  return maskPreviewRecords(records);
}

function notebookResultPolicy({ result, sql }) {
  const shape = sqlShapeDiagnostics(sql);
  const records = Array.isArray(result?.records) ? result.records : [];
  if (shape.aggregate_like && records.length <= 20) {
    return "aggregate_records_masked_in_artifact";
  }
  return "detail_or_large_rows_omitted_from_artifact; rerun safe SQL in current-case workbench for review";
}

function notebookJsonCodeForSql(sql) {
  return [
    "import duckdb, os",
    "db_path = os.environ.get('ANALYTIX_CASE_DUCKDB_PATH')",
    "if not db_path:",
    "    raise RuntimeError('Set ANALYTIX_CASE_DUCKDB_PATH to the current analytix case DuckDB before executing this notebook.')",
    "con = duckdb.connect(db_path, read_only=True)",
    "sql = r'''",
    stripTrailingSemicolon(sql),
    "'''",
    "con.execute(sql).fetchdf()"
  ].join("\n");
}

async function executeNotebookSqlCell({ cell, cellIndex, caseId, env, maxRows, signal }) {
  throwIfAborted(signal);
  const source = objectOf(cell);
  const purpose = text(source.purpose || source.query_request || source.queryRequest || `notebook cell ${cellIndex + 1}`);
  const sql = stripTrailingSemicolon(source.sql);
  if (!sql) {
    return {
      cell_index: cellIndex,
      cell_type: text(source.type || "markdown") || "markdown",
      purpose,
      execution_status: "documented",
      notes: text(source.notes || source.query_request || source.queryRequest),
      query_id: ""
    };
  }
  const startedAt = Date.now();
  try {
    const sqlPolicy = await validateCaseSqlPolicy({ caseId, sql, env, signal });
    const result = await runLocalSql({ caseId, env, rowLimit: maxRows, sql, signal });
    throwIfAborted(signal);
    const queryId = queryIdFor({ caseId, skillId: "create_case_notebook", sql, extra: String(cellIndex) });
    const shape = sqlShapeDiagnostics(sql);
    const resultMode = shape.aggregate_like ? "aggregate" : "evidence_table";
    const evidence = buildRunCaseSqlEvidence({
      caseId,
      purpose,
      sql,
      rowLimit: maxRows,
      resultMode,
      result,
      queryId,
      sqlPolicy
    });
    return {
      cell_index: cellIndex,
      cell_type: text(source.type || "query") || "query",
      purpose,
      execution_status: "executed",
      sql_shape: shape,
      query_id: queryId,
      source_hash: evidence.sourceHash,
      source_scope: evidence.sourceScope,
      metric_scope: evidence.metricScope,
      validation_state: evidence.validationState,
      sql_policy: sqlPolicy,
      evidence_card: evidence.evidenceCard,
      columns: result.columns || [],
      row_count: result.row_count,
      truncated: result.truncated,
      result_records: visibleNotebookResultRecords({ result, sql }),
      result_record_policy: notebookResultPolicy({ result, sql }),
      duration_ms: Math.max(0, Date.now() - startedAt),
      sql
    };
  } catch (error) {
    throwIfAborted(signal);
    return {
      cell_index: cellIndex,
      cell_type: text(source.type || "query") || "query",
      purpose,
      execution_status: "blocked",
      error_class: "notebook_cell_sql_blocked",
      error_message: text(error?.message || error).slice(0, 500),
      query_id: queryIdFor({ caseId, skillId: "create_case_notebook", sql, extra: `blocked:${cellIndex}` }),
      sql_shape: sql ? sqlShapeDiagnostics(sql) : {},
      sql_policy: error?.sql_policy || null,
      sql
    };
  }
}

function buildNotebookIpynb({ title, analysisGoal, caseId, maxRows, cellResults, notebookHash }) {
  const ipynbCells = [
    markdownCell(`# ${title}\n\n当前案件：${caseId}\n\n分析目标：${analysisGoal}`),
    markdownCell([
      "## 核验边界",
      "",
      "- 当前案件只读 DuckDB。",
      "- 仅允许 `analysis_*` 和 `fc_*_norm` 清洗/分析范围。",
      `- 单个 SQL cell 最大返回 ${maxRows} 行。`,
      "- 明细或大结果不写入 notebook 结果区；需要复核时重新在当前案件 Workbench 执行。",
      `- notebook_hash: ${notebookHash}`
    ].join("\n"))
  ];
  for (const result of cellResults) {
    const purpose = text(result.purpose) || `cell ${Number(result.cell_index || 0) + 1}`;
    if (text(result.sql)) {
      ipynbCells.push(markdownCell([
        `## ${purpose}`,
        "",
        `执行状态：${text(result.execution_status)}`,
        `source_hash：${text(result.source_hash) || "not_available"}`,
        `row_count：${strictNonNegativeInteger(result.row_count) ?? "unknown"}`,
        `truncated：${notebookTruncatedLabel(result.truncated)}`,
        `result_policy：${text(result.result_record_policy) || "not_applicable"}`,
        text(result.error_message) ? `error：${text(result.error_message)}` : ""
      ].filter(Boolean).join("\n")));
      ipynbCells.push(codeCell(notebookJsonCodeForSql(result.sql), {
        analytix_query_id: text(result.query_id),
        source_hash: text(result.source_hash)
      }));
      if (Array.isArray(result.result_records) && result.result_records.length) {
        ipynbCells.push(markdownCell([
          "### 已执行聚合结果预览",
          "",
          "```json",
          JSON.stringify(result.result_records, null, 2),
          "```"
        ].join("\n")));
      }
    } else {
      ipynbCells.push(markdownCell([
        `## ${purpose}`,
        "",
        text(result.notes) || "记录范围、口径或复核说明。"
      ].join("\n")));
    }
  }
  return {
    cells: ipynbCells,
    metadata: {
      analytix: {
        case_id: caseId,
        workbench_contract: "local_duckdb_readonly_v1",
        allowed_view_policy: "cleaned_and_analysis_only",
        raw_rows_exposed: false,
        notebook_hash: notebookHash
      },
      kernelspec: {
        display_name: "Python 3",
        language: "python",
        name: "python3"
      },
      language_info: {
        name: "python",
        pygments_lexer: "ipython3"
      }
    },
    nbformat: 4,
    nbformat_minor: 5
  };
}

export function notebookTruncatedLabel(value) {
  if (value === true) return "true";
  if (value === false) return "false";
  return "unknown";
}

export async function executeLocalDuckdbCreateCaseNotebook(args = {}, options = {}) {
  const startedAt = Date.now();
  const source = objectOf(args);
  const env = options.env || process.env;
  throwIfAborted(options.signal);
  const caseId = localCaseId(source, env);
  if (!caseId) throw new Error("Local DuckDB workbench requires resolved case_id.");
  if (text(source.allowed_view_policy || "cleaned_and_analysis_only") !== "cleaned_and_analysis_only") {
    throw new Error("Local DuckDB workbench allows only cleaned_and_analysis_only.");
  }
  const analysisGoal = text(source.analysis_goal || source.analysisGoal);
  if (!analysisGoal) throw new Error("Local DuckDB notebook requires analysis_goal.");
  const maxRows = Math.max(1, Math.min(Number(source.max_rows_per_query || source.maxRowsPerQuery || 100) || 100, 500));
  const title = text(source.title) || "当前案件专项资金核算记录";
  const cells = notebookCellsFromArgs(source);
  const cellResults = [];
  for (const [cellIndex, cell] of cells.entries()) {
    cellResults.push(await executeNotebookSqlCell({ cell, cellIndex, caseId, env, maxRows, signal: options.signal }));
  }
  throwIfAborted(options.signal);
  const queryIds = cellResults.map((item) => text(item.query_id)).filter(Boolean);
  const executedCount = cellResults.filter((item) => item.execution_status === "executed").length;
  const blockedCount = cellResults.filter((item) => item.execution_status === "blocked").length;
  const executionStatus = blockedCount > 0 && executedCount === 0
    ? "blocked"
    : blockedCount > 0
      ? "partial"
      : executedCount > 0
        ? "executed"
        : "planned";
  const sourceHash = stableJsonHash({
    case_id: caseId,
    analysis_goal: analysisGoal,
    max_rows_per_query: maxRows,
    cells: cellResults.map((item) => ({
      cell_index: item.cell_index,
      execution_status: item.execution_status,
      query_id: item.query_id,
      source_hash: item.source_hash,
      row_count: item.row_count,
      truncated: item.truncated,
      sql_shape: item.sql_shape
    }))
  });
  const artifactRoot = notebookArtifactRoot({ caseId, args: source, env, signal: options.signal });
  const artifactId = `case-notebook-${sourceHash}`;
  const baseName = `${safeFileSegment(title)}-${sourceHash}`;
  const notebookPath = path.join(artifactRoot, `${baseName}.ipynb`);
  const manifestPath = path.join(artifactRoot, `${baseName}.json`);
  const ipynb = buildNotebookIpynb({
    title,
    analysisGoal,
    caseId,
    maxRows,
    cellResults,
    notebookHash: sourceHash
  });
  const manifest = {
    artifact_id: artifactId,
    artifact_type: "case_workbench_notebook",
    created_at: new Date().toISOString(),
    case_id: caseId,
    title,
    analysis_goal: analysisGoal,
    workbench_contract: "local_duckdb_readonly_v1",
    allowed_view_policy: "cleaned_and_analysis_only",
    execution_status: executionStatus,
    source_hash: sourceHash,
    max_rows_per_query: maxRows,
    cell_count: cellResults.length,
    executed_cell_count: executedCount,
    blocked_cell_count: blockedCount,
    query_ids: queryIds,
    cell_results: cellResults.map((item) => ({
      ...item,
      sql: undefined
    })),
    raw_rows_exposed: false,
    notebook_path: notebookPath,
    manifest_path: manifestPath,
    validation_summary: {
      readonly: true,
      current_case_only: true,
      allowed_tables: "cleaned_and_analysis_only",
      local_duckdb_fallback: true,
      raw_rows_exposed: false,
      notebook_cells_executed_against_current_case: executedCount
    }
  };
  throwIfAborted(options.signal);
  fs.writeFileSync(notebookPath, `${JSON.stringify(ipynb, null, 2)}\n`, "utf8");
  fs.writeFileSync(manifestPath, `${JSON.stringify(manifest, null, 2)}\n`, "utf8");
  const queryId = queryIdFor({ caseId, skillId: "create_case_notebook", extra: sourceHash });
  const payload = localWorkbenchEnvelope({
    skillId: "create_case_notebook",
    caseId,
    data: {
      execution_status: executionStatus,
      artifact_id: artifactId,
      artifact_type: "case_workbench_notebook",
      delivery_mode: text(source.delivery_mode || source.deliveryMode || "notebook_artifact"),
      title,
      analysis_goal: analysisGoal,
      notebook_path: notebookPath,
      manifest_path: manifestPath,
      source_hash: sourceHash,
      max_rows_per_query: maxRows,
      cell_count: cellResults.length,
      executed_cell_count: executedCount,
      blocked_cell_count: blockedCount,
      query_ids: queryIds,
      cell_summaries: manifest.cell_results,
      validation_state: {
        status: executionStatus,
        current_case_only: true,
        readonly: true,
        cleaned_analysis_scope_only: true,
        bounded_row_limit: true,
        raw_rows_exposed: false,
        artifact_written: true
      },
      validation_summary: manifest.validation_summary
    },
    assertions: [
      {
        assertion: "current_case_readonly_notebook_artifact",
        status: blockedCount > 0 ? "partial" : "passed",
        source_refs: { query_ids: queryIds.length ? queryIds : [queryId] }
      }
    ],
    warnings: [
      {
        code: blockedCount > 0 ? "NOTEBOOK_PARTIAL" : "NOTEBOOK_ARTIFACT_WRITTEN",
        severity: blockedCount > 0 ? "warning" : "info",
        message: blockedCount > 0
          ? "notebook 中存在未执行 SQL cell；不得把阻断 cell 写成案件事实。"
          : "已生成当前案件只读专项资金核算 notebook artifact。"
      }
    ],
    nextActions: [
      {
        action: "如用于报告、图表或用户结论，先用 validate_report_claims 或 focused owner 复核每个 source_hash。",
        source_refs: { query_ids: queryIds.length ? queryIds : [queryId] }
      }
    ],
    citations: { query_ids: queryIds.length ? [queryId, ...queryIds] : [queryId] }
  });
  recordLocalWorkbenchHistory({
    skillId: "create_case_notebook",
    caseId,
    args,
    queryId,
    purpose: analysisGoal,
    sql: cellResults.map((item) => text(item.sql)).filter(Boolean).join("\n\n"),
    data: payload.data,
    startedAt,
    status: executionStatus,
    evidenceId: artifactId,
    signal: options.signal
  });
  return payload;
}

export async function executeLocalDuckdbRunCaseSql(args = {}, options = {}) {
  const startedAt = Date.now();
  const source = objectOf(args);
  const env = options.env || process.env;
  throwIfAborted(options.signal);
  const caseId = localCaseId(source, env);
  const purpose = text(source.purpose || source.analysis_goal || source.question);
  const sql = text(source.sql);
  if (!caseId) throw new Error("Local DuckDB workbench requires resolved case_id.");
  const authority = workbenchHistoryAuthority(source, caseId);
  if (!authority) throw new Error("Local DuckDB workbench requires frozen same-turn dataset snapshot authority.");
  if (!purpose) throw new Error("Local DuckDB workbench requires purpose.");
  if (text(source.allowed_view_policy || "cleaned_and_analysis_only") !== "cleaned_and_analysis_only") {
    throw new Error("Local DuckDB workbench allows only cleaned_and_analysis_only.");
  }
  if (!sql) throw new Error("Local DuckDB workbench requires sql.");
  const dbPath = caseDbPath(caseId, env);
  if (!fs.existsSync(dbPath)) {
    throw duckdbDiagnosticError({ code: "database_unavailable", stage: "runner_resolution", sql });
  }
  const sqlPolicy = await validateCaseSqlPolicy({
    caseId,
    sql,
    env,
    expectedDatasetSnapshotId: authority.datasetSnapshotId,
    signal: options.signal
  });
  const rowLimit = Math.max(1, Math.min(Number(source.row_limit || source.rowLimit || 100) || 100, 500));
  const queryId = `local-duckdb:${crypto.createHash("sha256").update(JSON.stringify({ caseId, datasetSnapshotId: authority.datasetSnapshotId, sql })).digest("hex")}`;
  const result = await runLocalSql({
    caseId,
    env,
    sql,
    rowLimit,
    bindDatasetSnapshot: true,
    expectedDatasetSnapshotId: authority.datasetSnapshotId,
    policyAnalysis: sqlPolicy.static_guard,
    signal: options.signal
  });
  throwIfAborted(options.signal);
  const resultMode = text(source.result_mode || source.resultMode || "preview");
  const {
    sourceScope,
    sourceHash,
    metricScope,
    validationState,
    evidenceCard,
    topN
  } = buildRunCaseSqlEvidence({
    caseId,
    purpose,
    sql,
    rowLimit,
    resultMode,
    result,
    queryId,
    sqlPolicy
  });
  const payload = {
    skill_id: "run_case_sql",
    version: DUCKDB_WORKBENCH_RUNTIME_VERSION,
    status: "ok",
    snapshot_at: new Date().toISOString(),
    data: {
      workbench_contract: "local_duckdb_readonly_v1",
      execution_status: "executed",
      case_id: caseId,
      purpose,
      allowed_view_policy: "cleaned_and_analysis_only",
      source_scope: sourceScope,
      result_mode: resultMode,
      query_id: queryId,
      source_hash: sourceHash,
      metric_scope: metricScope,
      validation_state: validationState,
      sql_policy: sqlPolicy,
      evidence_card: evidenceCard,
      snapshot_contract: text(result.snapshot_contract),
      observed_dataset_snapshot_id: text(result.observed_dataset_snapshot_id),
      ...(topN ? { top_n_contract: topN } : {}),
      columns: result.columns,
      column_types: result.column_types,
      records: result.records,
      row_count: result.row_count,
      row_limit: rowLimit,
      truncated: result.truncated,
      raw_rows_exposed: false,
      evidence_status: evidenceCard.support_status,
      validation_summary: {
        readonly: true,
        current_case_only: true,
        allowed_tables: "cleaned_and_analysis_only",
        local_duckdb_fallback: true,
        sql_policy_engine: sqlPolicy.policy_engine,
        parser_binder_validated: true,
        source_hash: sourceHash,
        query_id: queryId
      }
    },
    evidence_card: evidenceCard,
    assertions: [
      {
        assertion: "current_case_readonly_cleaned_analysis_scope",
        status: "passed",
        source_refs: { query_ids: [queryId] }
      }
    ],
    warnings: [
      {
        code: "LOCAL_DUCKDB_WORKBENCH_FALLBACK",
        severity: "info",
        message: "已按当前案件只读明细完成专项资金核算。"
      }
    ],
    next_actions: evidenceCard.next_review_actions.map((message) => ({
      action: message,
      source_refs: { query_ids: [queryId] }
    })),
    audit_ref: {
      query_id: queryId,
      source_hash: sourceHash,
      workbench_contract: "local_duckdb_readonly_v1"
    },
    cursor: null,
    has_more: result.truncated,
    truncated: result.truncated,
    error: null,
    citations: {
      query_ids: [queryId]
    }
  };
  recordLocalWorkbenchHistory({
    skillId: "run_case_sql",
    caseId,
    args,
    queryId,
    purpose,
    sql,
    data: payload.data,
    startedAt,
    evidenceId: evidenceCard.fact_id,
    signal: options.signal
  });
  return payload;
}

export async function executeLocalDuckdbWorkbenchSkill(skillId, args = {}, options = {}) {
  throwIfAborted(options.signal);
  const normalizedSkillId = text(skillId);
  if (normalizedSkillId === "rank_accounts" || normalizedSkillId === "rank_holders" || normalizedSkillId === "rank_counterparties") {
    return executeLocalDuckdbRankingSkill(normalizedSkillId, args, options);
  }
  if (normalizedSkillId === "trace_subject_top_outflows") {
    return executeLocalDuckdbTraceSubjectTopOutflows(args, options);
  }
  if (normalizedSkillId === "trace_fund_next_hop") {
    return executeLocalDuckdbTraceFundNextHop(args, options);
  }
  if (normalizedSkillId === "trace_fund") {
    return executeLocalDuckdbTraceFund(args, options);
  }
  if (normalizedSkillId === "inspect_case_schema") {
    return executeLocalDuckdbInspectCaseSchema(args, options);
  }
  if (normalizedSkillId === "profile_case_schema") {
    return executeLocalDuckdbProfileCaseSchema(args, options);
  }
  if (normalizedSkillId === "preview_case_rows") {
    return executeLocalDuckdbPreviewCaseRows(args, options);
  }
  if (normalizedSkillId === "explain_case_sql") {
    return executeLocalDuckdbExplainCaseSql(args, options);
  }
  if (normalizedSkillId === "diagnose_case_sql") {
    return executeLocalDuckdbDiagnoseCaseSql(args, options);
  }
  if (normalizedSkillId === "count_case_rows") {
    return executeLocalDuckdbCountCaseRows(args, options);
  }
  if (normalizedSkillId === "get_scope_coverage") {
    return executeLocalDuckdbGetScopeCoverage(args, options);
  }
  if (normalizedSkillId === "case_sql_recipes") {
    const caseId = localCaseId(args, options.env || process.env);
    if (!caseId) throw new Error("Local DuckDB workbench requires resolved case_id.");
    const startedAt = Date.now();
    const category = text(args.category || args.recipe_id || args.recipeId);
    const payload = localCaseSqlRecipes({
      caseId,
      category,
      limit: Math.max(1, Math.min(Number(args.limit || 50) || 50, 100))
    });
    recordLocalWorkbenchHistory({
      skillId: "case_sql_recipes",
      caseId,
      args,
      queryId: queryIdFor({ caseId, skillId: "case_sql_recipes", extra: category }),
      purpose: "case SQL recipe catalog",
      data: payload.data,
      startedAt,
      signal: options.signal
    });
    return payload;
  }
  if (normalizedSkillId === "inspect_workbench_history") {
    const caseId = localCaseId(args, options.env || process.env);
    if (!caseId) throw new Error("Local DuckDB workbench requires resolved case_id.");
    return localWorkbenchHistory({
      caseId,
      args,
      limit: Math.max(1, Math.min(Number(args.limit || 20) || 20, 50))
    });
  }
  if (normalizedSkillId === "create_case_notebook") {
    return executeLocalDuckdbCreateCaseNotebook(args, options);
  }
  if (normalizedSkillId === "run_case_sql") {
    return executeLocalDuckdbRunCaseSql(args, options);
  }
  throw new Error(`Local DuckDB workbench does not support skill: ${normalizedSkillId}`);
}
