from __future__ import annotations

import hashlib
import json
import re
from dataclasses import dataclass
from typing import Any

from app.repositories.txn_daily_materialization_plan_constants import (
    MATERIALIZATION_IDENTITY_PREFIX,
    MATERIALIZATION_IDENTITY_SCHEMA_VERSION,
)


_SHA256_PATTERN = re.compile(r"^[a-f0-9]{64}$")


class TxnDailyMaterializationIdentityError(RuntimeError):
    pass


def _required_nonnegative_manifest_count(value: Any) -> int:
    if isinstance(value, bool) or not isinstance(value, int) or value < 0:
        raise TxnDailyMaterializationIdentityError("materialization source manifest is incomplete")
    return value


@dataclass(frozen=True)
class TxnDailyMaterializationIdentity:
    value: str
    source_signature: str


def _normalized_source_sha256(con: Any, case_id: str) -> str:
    column_rows = con.query(
        """
        SELECT column_name
          FROM information_schema.columns
         WHERE table_schema='main' AND table_name='fc_transaction_norm'
         ORDER BY ordinal_position
        """
    )
    columns = [str(row[0]) for row in column_rows if row and row[0]]
    required = {"id", "case_id", "clean_invalid", "clean_failed", "clean_reversal"}
    if not columns or not required.issubset(columns):
        raise TxnDailyMaterializationIdentityError("materialization source lineage is unavailable")

    digest = hashlib.sha256()

    def update_framed(value: str) -> None:
        encoded = value.encode("utf-8")
        digest.update(len(encoded).to_bytes(8, "big"))
        digest.update(encoded)

    update_framed("analytix.txn-daily-normalized-source/v12")
    update_framed(case_id)
    for column in columns:
        update_framed(column)
    rows = con.query(
        """
        SELECT to_json(r)
          FROM fc_transaction_norm AS r
         WHERE r.case_id=?
         ORDER BY TRY_CAST(r.id AS BIGINT), to_json(r)
        """,
        (case_id,),
    )
    for row in rows:
        if len(row) != 1 or row[0] is None:
            raise TxnDailyMaterializationIdentityError("materialization source lineage is unavailable")
        update_framed(str(row[0]))
    return digest.hexdigest()


def build_txn_daily_materialization_identity(
    con: Any,
    *,
    case_id: str,
    source_revision: int,
    source_snapshot: tuple[int, str, int],
) -> TxnDailyMaterializationIdentity:
    case_key = str(case_id or "").strip()
    if not case_key:
        raise TxnDailyMaterializationIdentityError("materialization case identity is unavailable")

    revision_rows = con.query(
        """
        SELECT COUNT(1), COALESCE(MIN(revision), 0), COALESCE(MAX(revision), 0)
          FROM analysis_revision_state
         WHERE revision_key='stats_flow_source'
        """
    )
    if not revision_rows or int(revision_rows[0][0] or 0) != 1:
        raise TxnDailyMaterializationIdentityError("materialization source revision is ambiguous")
    observed_revision = int(revision_rows[0][1] or 0)
    if observed_revision <= 0 or observed_revision != int(source_revision):
        raise TxnDailyMaterializationIdentityError("materialization source revision changed")
    if observed_revision != int(revision_rows[0][2] or 0):
        raise TxnDailyMaterializationIdentityError("materialization source revision is inconsistent")

    source_rows = con.query(
        """
        SELECT COUNT(1), COUNT(TRY_CAST(id AS BIGINT)),
               COUNT(DISTINCT TRY_CAST(id AS BIGINT)),
               COALESCE(CAST(MAX(txn_ts) AS VARCHAR), ''), COALESCE(MAX(id), 0),
               SUM(CASE WHEN clean_invalid IS NULL OR clean_invalid NOT IN (0, 1)
                             OR clean_failed IS NULL OR clean_failed NOT IN (0, 1)
                             OR clean_reversal IS NULL OR clean_reversal NOT IN (0, 1)
                        THEN 1 ELSE 0 END),
               SUM(CASE WHEN clean_invalid=1 OR clean_failed=1 OR clean_reversal=1 THEN 1 ELSE 0 END)
          FROM fc_transaction_norm
         WHERE case_id=?
        """,
        (case_key,),
    )
    if not source_rows:
        raise TxnDailyMaterializationIdentityError("materialization source content is unavailable")
    source_row_count = int(source_rows[0][0] or 0)
    source_id_count = int(source_rows[0][1] or 0)
    source_distinct_id_count = int(source_rows[0][2] or 0)
    source_max_txn_ts = str(source_rows[0][3] or "")
    source_max_id = int(source_rows[0][4] or 0)
    invalid_rejection_state_count = int(source_rows[0][5] or 0)
    rejected_row_count = int(source_rows[0][6] or 0)
    expected_row_count, expected_max_txn_ts, expected_max_id = source_snapshot
    if invalid_rejection_state_count != 0:
        raise TxnDailyMaterializationIdentityError("materialization rejection state is invalid")
    if source_id_count != source_row_count or source_distinct_id_count != source_row_count:
        raise TxnDailyMaterializationIdentityError("materialization source row lineage is ambiguous")
    if (
        source_row_count != int(expected_row_count)
        or source_max_txn_ts != str(expected_max_txn_ts or "")
        or source_max_id != int(expected_max_id)
    ):
        raise TxnDailyMaterializationIdentityError("materialization source content changed")

    manifest_rows = con.query(
        """
        SELECT file_id, sha256, rows_imported_norm, status, cleaned_status
          FROM import_file_log
         WHERE case_id=? AND kind='fc_transaction'
         ORDER BY file_id
        """,
        (case_key,),
    )
    if not manifest_rows:
        raise TxnDailyMaterializationIdentityError("materialization source manifest is unavailable")

    raw_sources: list[dict[str, Any]] = []
    seen_file_ids: set[str] = set()
    for row in manifest_rows:
        file_id = str(row[0] or "").strip()
        sha256 = str(row[1] or "").strip().lower()
        rows_imported_norm = _required_nonnegative_manifest_count(row[2])
        status = str(row[3] or "").strip()
        cleaned_status = str(row[4] or "").strip().lower()
        if (
            not file_id
            or file_id in seen_file_ids
            or _SHA256_PATTERN.fullmatch(sha256) is None
            or status != "已完成"
            or cleaned_status != "done"
        ):
            raise TxnDailyMaterializationIdentityError("materialization source manifest is incomplete")
        seen_file_ids.add(file_id)
        raw_sources.append(
            {
                "cleanedStatus": cleaned_status,
                "fileId": file_id,
                "rowsImportedNorm": rows_imported_norm,
                "sha256": sha256,
                "status": status,
            }
        )

    normalized_source_sha256 = _normalized_source_sha256(con, case_key)
    canonical = json.dumps(
        {
            "acceptedRowCount": source_row_count - rejected_row_count,
            "caseId": case_key,
            "normalizedSourceSha256": normalized_source_sha256,
            "rawSources": raw_sources,
            "rejectedRowCount": rejected_row_count,
            "schemaVersion": MATERIALIZATION_IDENTITY_SCHEMA_VERSION,
            "sourceMaxId": source_max_id,
            "sourceMaxTxnTimestamp": source_max_txn_ts,
            "sourceRevision": observed_revision,
            "sourceRowCount": source_row_count,
        },
        ensure_ascii=False,
        separators=(",", ":"),
        sort_keys=True,
    )
    source_signature = hashlib.sha256(canonical.encode("utf-8")).hexdigest()
    return TxnDailyMaterializationIdentity(
        value=f"{MATERIALIZATION_IDENTITY_PREFIX}{source_signature}",
        source_signature=source_signature,
    )
