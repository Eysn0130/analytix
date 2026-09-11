from __future__ import annotations

import hashlib
import time
from dataclasses import dataclass
from pathlib import Path
from typing import Dict, List, Optional, Sequence

from app.core.db_engine import DuckDBEngine
from app.core.fc_import_bank_name import bank_name_from_file_name_sql, derived_bank_headers_for_kind
from app.core.fc_group_import_keys import GroupDedupKeysSql, build_group_dedup_keys_sql
from app.core.fc_import_duckdb import supports_json_object
from app.core.fc_import_missing_notes import transaction_missing_headers_for_warning
from app.core.fc_import_norm_insert import build_norm_insert_sql
from app.core.fc_import_privacy_delta import ensure_privacy_projection_delta_log
from app.core.fc_import_projection import (
    EXTRA_JSON_TABLES,
    build_import_projection,
    header_index,
    header_map,
    raw_col,
    sql_literal,
)
from app.core.fc_import_schema import (
    FC_SCHEMAS,
    HEADER_ALIASES_BY_TABLE,
    FcSchema,
    norm_table_name,
    raw_table_name,
)
from app.core.fc_import_timing import ImportTimingProfile, add_phase
from app.core.import_count_semantics import known_public_import_count
from app.core.storage import _now_iso


@dataclass(frozen=True)
class FcGroupImportSource:
    file_id: str
    display_name: str
    path: Path
    rows_total: int


@dataclass(frozen=True)
class FcGroupImportResult:
    file_id: str
    rows_total: int
    rows_seen: int
    rows_imported_raw: int
    rows_imported_norm: int
    rows_dedup: int
    rows_error: int
    note: str
    rows_skipped_non_data: int = 0


def import_fc_csv_group_into_db(
    *,
    engine: DuckDBEngine,
    case_id: str,
    kind: str,
    sources: Sequence[FcGroupImportSource],
    headers: Sequence[str],
    field_mapping: Optional[Dict[str, str]] = None,
    profile_cb=None,
) -> List[FcGroupImportResult]:
    if len(sources) < 2:
        raise ValueError("group import requires at least two sources")
    if kind not in FC_SCHEMAS:
        raise ValueError(f"unsupported grouped import kind: {kind}")
    source_counts = _validate_sources(sources)
    expected_file_ids = list(source_counts)

    schema = FC_SCHEMAS[kind]
    projection_headers = [str(header or "") for header in headers]
    projection_header_map = header_map(projection_headers)
    derived_header_values = {
        header: bank_name_from_file_name_sql("r.__analytix_source_display_name")
        for header in derived_bank_headers_for_kind(schema.table)
    }
    projection = build_import_projection(
        schema=schema,
        raw_headers=projection_headers,
        header_map_value=projection_header_map,
        header_set=set(projection_header_map.keys()),
        header_index_value=header_index(projection_headers),
        use_positional=False,
        col_names=[],
        field_mapping=field_mapping,
        alias_map=HEADER_ALIASES_BY_TABLE.get(schema.table, {}),
        precleaned_source=True,
        supports_json_object=supports_json_object(engine),
        derived_header_values=derived_header_values,
    )
    raw_table = raw_table_name(schema.table)
    norm_table = norm_table_name(schema.table)
    imported_at = _now_iso()
    token = _safe_group_token(case_id, sources)
    staging_table = f"stg_{schema.table}_{token}"
    keys_table = f"keys_{schema.table}_{token}"
    timings: Dict[str, float] = {}
    profile_started = time.perf_counter()

    read_expr = _read_csv_group_expr(sources, projection.force_not_null_sql)
    try:
        started = time.perf_counter()
        engine.execute(
            f"""CREATE TEMP TABLE {staging_table} AS
                WITH source_rows AS (
                    {read_expr}
                ),
                base AS (
                    SELECT
                        {sql_literal(case_id)} AS case_id,
                        r.file_id AS file_id,
                        r.source_index AS source_index,
                        r.row_no AS row_no,
                        {sql_literal(imported_at)} AS imported_at,
                        {', '.join(projection.select_cols)}
                        {projection.extra_json_expr}
                        {projection.raw_json_expr}
                    FROM source_rows r
                )
                SELECT base.*, {projection.hash_expr} AS row_hash FROM base
                WHERE {projection.non_empty_filter_expr}
            """
        )
        add_phase(timings, "staging_s", time.perf_counter() - started)

        started = time.perf_counter()
        total_by_file = _count_by_file(
            engine,
            staging_table,
            expected_file_ids=expected_file_ids,
        )
        add_phase(timings, "count_by_file_s", time.perf_counter() - started)
        started = time.perf_counter()
        key_missing_by_file = _key_missing_by_file(
            engine,
            schema=schema,
            staging_table=staging_table,
            expected_counts=total_by_file,
        )
        add_phase(timings, "key_missing_s", time.perf_counter() - started)

        raw_insert_cols = ["case_id", "file_id", "row_no", "imported_at", "row_hash"]
        raw_insert_cols.extend(
            raw_col(schema.col_map[header])
            for header in schema.headers
            if header not in projection.mapping_missing
        )
        if schema.table in EXTRA_JSON_TABLES:
            raw_insert_cols.append("extra_json")
        if schema.store_raw_json:
            raw_insert_cols.append("raw_json")

        started = time.perf_counter()
        engine.execute(
            build_group_dedup_keys_sql(
                GroupDedupKeysSql(
                    keys_table=keys_table,
                    staging_table=staging_table,
                    raw_table=raw_table,
                    case_id=case_id,
                )
            )
        )
        add_phase(timings, "keys_build_s", time.perf_counter() - started)
        started = time.perf_counter()
        inserted_by_file = _count_by_file(
            engine,
            keys_table,
            expected_file_ids=expected_file_ids,
        )
        add_phase(timings, "keys_count_s", time.perf_counter() - started)

        started = time.perf_counter()
        engine.execute(
            f"""INSERT INTO {raw_table}({', '.join(raw_insert_cols)})
                SELECT {', '.join(f's.{column}' for column in raw_insert_cols)}
                FROM {staging_table} s
                JOIN {keys_table} k
                  ON k.case_id=s.case_id
                 AND k.file_id=s.file_id
                 AND k.row_no=s.row_no
                 AND k.row_hash=s.row_hash
            """
        )
        add_phase(timings, "raw_insert_s", time.perf_counter() - started)

        started = time.perf_counter()
        norm_sql, norm_params = build_norm_insert_sql(
            schema,
            staging_table,
            norm_table,
            case_id=case_id,
            delta_table=keys_table,
            precleaned_source=True,
            skip_existing_check=True,
            input_values_cleaned=True,
            missing_headers=set(projection.mapping_missing),
        )
        engine.execute(norm_sql, norm_params)
        add_phase(timings, "norm_insert_s", time.perf_counter() - started)

        if schema.table == "fc_transaction" and sum(inserted_by_file.values()) > 0:
            started = time.perf_counter()
            _append_privacy_delta_from_keys_table(
                engine,
                case_id=case_id,
                keys_table=keys_table,
            )
            add_phase(timings, "privacy_delta_s", time.perf_counter() - started)

        started = time.perf_counter()
        clean_stats = _clean_stats_by_file(
            engine,
            schema=schema,
            norm_table=norm_table,
            case_id=case_id,
            expected_counts=inserted_by_file,
        )
        add_phase(timings, "clean_stats_s", time.perf_counter() - started)
        started = time.perf_counter()
        extra_hints = _extra_json_hints_by_file(
            engine,
            schema=schema,
            norm_table=norm_table,
            case_id=case_id,
            sources=sources,
            mapping_missing=projection.mapping_missing,
        )
        add_phase(timings, "extra_hints_s", time.perf_counter() - started)

        results: List[FcGroupImportResult] = []
        for source in sources:
            total_rows = total_by_file[source.file_id]
            inserted_rows = inserted_by_file[source.file_id]
            rows_total = source_counts[source.file_id]
            if total_rows > rows_total:
                raise ValueError("group_staging_rows_exceed_source_total")
            if inserted_rows > total_rows:
                raise ValueError("group_inserted_rows_exceed_staging_total")
            rows_dedup = total_rows - inserted_rows
            rows_skipped_non_data = rows_total - total_rows
            note = _build_note(
                schema=schema,
                total_rows=total_rows,
                inserted_rows=inserted_rows,
                rows_dedup=rows_dedup,
                mapping_alias_used=projection.mapping_alias_used,
                mapping_derived=projection.mapping_derived,
                mapping_missing=projection.mapping_missing,
                key_missing=(
                    key_missing_by_file[source.file_id]
                    if schema.table == "fc_transaction"
                    else {}
                ),
                clean_stats=(
                    clean_stats[source.file_id]
                    if schema.table == "fc_transaction"
                    else {}
                ),
                extra_hints=(
                    extra_hints[source.file_id]
                    if schema.table in EXTRA_JSON_TABLES and projection.mapping_missing
                    else []
                ),
            )
            results.append(
                FcGroupImportResult(
                    file_id=source.file_id,
                    rows_total=rows_total,
                    rows_seen=total_rows,
                    rows_imported_raw=inserted_rows,
                    rows_imported_norm=inserted_rows,
                    rows_dedup=rows_dedup,
                    rows_error=0,
                    note=note,
                    rows_skipped_non_data=rows_skipped_non_data,
                )
            )
        if profile_cb:
            profile_cb(
                ImportTimingProfile(
                    phases=timings,
                    rows_seen=sum(total_by_file.values()),
                    rows_imported=sum(inserted_by_file.values()),
                    chunks=1,
                    files=len(sources),
                    wall_s=time.perf_counter() - profile_started,
                ).as_dict()
            )
        return results
    finally:
        for table in (staging_table, keys_table):
            try:
                engine.execute(f"DROP TABLE {table}")
            except Exception:
                pass


def _read_csv_group_expr(sources: Sequence[FcGroupImportSource], force_not_null_sql: str) -> str:
    # row_no is a provenance field, so grouped imports must read each file in physical row order.
    opts = [
        "encoding='utf-8'",
        "delim=','",
        "quote='\"'",
        "escape='\"'",
        "header=true",
        "all_varchar=true",
        "parallel=false",
    ]
    if force_not_null_sql:
        opts.append(f"force_not_null={force_not_null_sql}")
    read_opts = ", ".join(opts)
    parts = []
    for index, source in enumerate(sources):
        source_path = sql_literal(str(source.path).replace("\\", "/"))
        parts.append(
            f"""SELECT
                    {sql_literal(source.file_id)} AS file_id,
                    {sql_literal(source.display_name)} AS __analytix_source_display_name,
                    {int(index)} AS source_index,
                    row_number() OVER () AS row_no,
                    r.*
                FROM read_csv({source_path}, {read_opts}) r"""
        )
    return "\nUNION ALL\n".join(parts)


def _validate_sources(sources: Sequence[FcGroupImportSource]) -> Dict[str, int]:
    counts: Dict[str, int] = {}
    for source in sources:
        file_id = source.file_id
        if type(file_id) is not str or not file_id.strip():
            raise ValueError("group_source_file_id_invalid")
        if file_id in counts:
            raise ValueError("group_source_file_id_duplicate")
        counts[file_id] = _require_count(
            source.rows_total,
            field="group_source_rows_total",
        )
    return counts


def _validate_expected_file_ids(file_ids: Sequence[str]) -> List[str]:
    expected: List[str] = []
    seen: set[str] = set()
    for file_id in file_ids:
        if type(file_id) is not str or not file_id.strip():
            raise ValueError("group_expected_file_id_invalid")
        if file_id in seen:
            raise ValueError("group_expected_file_id_duplicate")
        seen.add(file_id)
        expected.append(file_id)
    if not expected:
        raise ValueError("group_expected_file_ids_empty")
    return expected


def _require_count(value: object, *, field: str) -> int:
    normalized = known_public_import_count(value)
    if normalized is None:
        raise ValueError(f"{field}_count_invalid")
    return normalized


def _require_count_map(
    rows: object,
    *,
    expected_file_ids: Sequence[str],
    field: str,
) -> Dict[str, int]:
    if not isinstance(rows, list):
        raise ValueError(f"{field}_rows_invalid")
    expected = set(expected_file_ids)
    seen: set[str] = set()
    out: Dict[str, int] = {}
    for row in rows:
        file_id, values = _require_file_metric_row(
            row,
            expected_file_ids=expected,
            seen=seen,
            field=field,
            value_count=1,
        )
        out[file_id] = values[0]
    _require_complete_file_ids(seen, expected_file_ids, field=field)
    return out


def _require_file_metric_row(
    row: object,
    *,
    expected_file_ids: set[str],
    seen: set[str],
    field: str,
    value_count: int,
) -> tuple[str, List[int]]:
    if not isinstance(row, (list, tuple)) or len(row) != value_count + 1:
        raise ValueError(f"{field}_row_invalid")
    file_id = row[0]
    if type(file_id) is not str or not file_id.strip():
        raise ValueError(f"{field}_file_id_invalid")
    if file_id not in expected_file_ids:
        raise ValueError(f"{field}_file_id_unexpected")
    if file_id in seen:
        raise ValueError(f"{field}_file_id_duplicate")
    seen.add(file_id)
    values = [
        _require_count(value, field=f"{field}_{index}")
        for index, value in enumerate(row[1:], start=1)
    ]
    return file_id, values


def _require_complete_file_ids(
    seen: set[str],
    expected_file_ids: Sequence[str],
    *,
    field: str,
) -> None:
    if seen != set(expected_file_ids):
        raise ValueError(f"{field}_file_id_missing")


def _count_by_file(
    engine: DuckDBEngine,
    table: str,
    *,
    expected_file_ids: Sequence[str],
) -> Dict[str, int]:
    expected_ids = _validate_expected_file_ids(expected_file_ids)
    expected_values = ", ".join("(?)" for _ in expected_ids)
    rows = engine.query(
        f"""WITH expected(file_id) AS (VALUES {expected_values}),
                   counts AS (
                       SELECT file_id, COUNT(1) AS row_count
                       FROM {table}
                       GROUP BY file_id
                   )
            SELECT e.file_id,
                   CASE WHEN c.file_id IS NULL THEN CAST(0 AS BIGINT) ELSE c.row_count END
            FROM expected e
            LEFT JOIN counts c ON c.file_id=e.file_id
            UNION ALL
            SELECT c.file_id, c.row_count
            FROM counts c
            LEFT JOIN expected e ON e.file_id=c.file_id
            WHERE e.file_id IS NULL""",
        expected_ids,
    )
    return _require_count_map(
        rows,
        expected_file_ids=expected_ids,
        field="group_file_rows",
    )


def _key_missing_by_file(
    engine: DuckDBEngine,
    *,
    schema: FcSchema,
    staging_table: str,
    expected_counts: Dict[str, int],
) -> Dict[str, Dict[str, float]]:
    if schema.table != "fc_transaction":
        return {}
    expected_ids = _validate_expected_file_ids(list(expected_counts))
    expected_values = ", ".join("(?)" for _ in expected_ids)
    raw_txn = raw_col("txn_time")
    raw_amt = raw_col("amount")
    raw_card = raw_col("card_no")
    raw_acct = raw_col("acct_no")
    rows = engine.query(
        f"""WITH expected(file_id) AS (VALUES {expected_values})
            SELECT e.file_id,
                   COUNT(s.file_id) AS total_rows,
                   COUNT(s.file_id) FILTER (
                       WHERE COALESCE(TRIM(s.{raw_txn}), '')=''
                   ) AS miss_time,
                   COUNT(s.file_id) FILTER (
                       WHERE COALESCE(TRIM(s.{raw_amt}), '')=''
                   ) AS miss_amt,
                   COUNT(s.file_id) FILTER (
                       WHERE COALESCE(TRIM(s.{raw_card}), '')=''
                         AND COALESCE(TRIM(s.{raw_acct}), '')=''
                   ) AS miss_acct
            FROM expected e
            LEFT JOIN {staging_table} s ON s.file_id=e.file_id
            GROUP BY e.file_id""",
        expected_ids,
    )
    out: Dict[str, Dict[str, float]] = {}
    seen: set[str] = set()
    if not isinstance(rows, list):
        raise ValueError("group_key_missing_rows_invalid")
    for row in rows:
        file_id, values = _require_file_metric_row(
            row,
            expected_file_ids=set(expected_ids),
            seen=seen,
            field="group_key_missing",
            value_count=4,
        )
        total, miss_time, miss_amt, miss_acct = values
        if total != expected_counts[file_id]:
            raise ValueError("group_key_missing_total_mismatch")
        if any(value > total for value in (miss_time, miss_amt, miss_acct)):
            raise ValueError("group_key_missing_count_exceeds_total")
        out[file_id] = {
            "交易时间": (miss_time / total) if total else 0.0,
            "交易金额": (miss_amt / total) if total else 0.0,
            "交易账号/卡号": (miss_acct / total) if total else 0.0,
        }
    _require_complete_file_ids(seen, expected_ids, field="group_key_missing")
    return out


def _clean_stats_by_file(
    engine: DuckDBEngine,
    *,
    schema: FcSchema,
    norm_table: str,
    case_id: str,
    expected_counts: Dict[str, int],
) -> Dict[str, Dict[str, int]]:
    if schema.table != "fc_transaction":
        return {}
    file_ids = _validate_expected_file_ids(list(expected_counts))
    expected_values = ", ".join("(?)" for _ in file_ids)
    rows = engine.query(
        f"""WITH expected(file_id) AS (VALUES {expected_values})
            SELECT e.file_id,
                   COUNT(n.file_id) AS total_rows,
                   COUNT(n.file_id) FILTER (WHERE n.clean_amt_fixed=1) AS amt_fixed,
                   COUNT(n.file_id) FILTER (WHERE n.clean_amt_failed=1) AS amt_failed,
                   COUNT(n.file_id) FILTER (WHERE n.clean_bal_failed=1) AS bal_failed,
                   COUNT(n.file_id) FILTER (WHERE n.clean_dc_normalized=1) AS dc_norm,
                   COUNT(n.file_id) FILTER (WHERE n.clean_dc_inferred=1) AS dc_infer,
                   COUNT(n.file_id) FILTER (WHERE n.clean_suffix_fixed=1) AS suffix_fixed
            FROM expected e
            LEFT JOIN {norm_table} n
              ON n.case_id=? AND n.file_id=e.file_id
            GROUP BY e.file_id""",
        [*file_ids, case_id],
    )
    out: Dict[str, Dict[str, int]] = {}
    seen: set[str] = set()
    if not isinstance(rows, list):
        raise ValueError("group_clean_stats_rows_invalid")
    for row in rows:
        file_id, values = _require_file_metric_row(
            row,
            expected_file_ids=set(file_ids),
            seen=seen,
            field="group_clean_stats",
            value_count=7,
        )
        total, amt_fixed, amt_failed, bal_failed, dc_norm, dc_infer, suffix_fixed = values
        if total != expected_counts[file_id]:
            raise ValueError("group_clean_stats_total_mismatch")
        if any(
            value > total
            for value in (amt_fixed, amt_failed, bal_failed, dc_norm, dc_infer, suffix_fixed)
        ):
            raise ValueError("group_clean_stats_count_exceeds_total")
        out[file_id] = {
            "amt_fixed": amt_fixed,
            "amt_failed": amt_failed,
            "bal_failed": bal_failed,
            "dc_norm": dc_norm,
            "dc_infer": dc_infer,
            "suffix_fixed": suffix_fixed,
        }
    _require_complete_file_ids(seen, file_ids, field="group_clean_stats")
    return out


def _extra_json_hints_by_file(
    engine: DuckDBEngine,
    *,
    schema: FcSchema,
    norm_table: str,
    case_id: str,
    sources: Sequence[FcGroupImportSource],
    mapping_missing: Sequence[str],
) -> Dict[str, List[str]]:
    if schema.table not in EXTRA_JSON_TABLES or not mapping_missing:
        return {}
    out: Dict[str, List[str]] = {}
    for source in sources:
        rows = engine.query(
            "SELECT k, COUNT(1) AS cnt FROM ("
            "  SELECT unnest(json_keys(extra_json)) AS k "
            f"  FROM {norm_table} "
            "  WHERE case_id=? AND file_id=? AND extra_json IS NOT NULL AND extra_json<>''"
            ") t GROUP BY k ORDER BY cnt DESC",
            (case_id, source.file_id),
        )
        if not isinstance(rows, list):
            raise ValueError("group_extra_hints_rows_invalid")
        base_rows = _require_count(source.rows_total, field="group_source_rows_total")
        high_freq: List[str] = []
        seen_keys: set[str] = set()
        for row in rows:
            if not isinstance(row, (list, tuple)) or len(row) != 2:
                raise ValueError("group_extra_hints_row_invalid")
            key, raw_count = row
            if type(key) is not str or not key or key in seen_keys:
                raise ValueError("group_extra_hints_key_invalid")
            seen_keys.add(key)
            count = _require_count(raw_count, field="group_extra_hint_rows")
            if count > base_rows:
                raise ValueError("group_extra_hint_rows_exceed_source_total")
            ratio = count / base_rows if base_rows else 0.0
            if ratio >= 0.6:
                high_freq.append(f"{key}≈{int(ratio * 100)}%")
        out[source.file_id] = high_freq[:6]
    return out


def _build_note(
    *,
    schema: FcSchema,
    total_rows: int,
    inserted_rows: int,
    rows_dedup: int,
    mapping_alias_used: Sequence[str],
    mapping_derived: Sequence[str],
    mapping_missing: Sequence[str],
    key_missing: Dict[str, float],
    clean_stats: Dict[str, int],
    extra_hints: Sequence[str],
) -> str:
    note_lines: List[str] = []
    if rows_dedup:
        note_lines.append(f"重复导入：本次 {rows_dedup} 行未写入（与历史数据或本批次较早文件重复，行哈希一致）")
    if schema.table == "fc_transaction":
        if mapping_alias_used:
            note_lines.append("自动映射：")
            note_lines.extend([f"- {item}" for item in list(mapping_alias_used)[:30]])
            if len(mapping_alias_used) > 30:
                note_lines.append(f"- ...（共 {len(mapping_alias_used)} 条）")
        if mapping_derived:
            note_lines.append("自动处理：")
            note_lines.extend([f"- {item}" for item in mapping_derived])
        if mapping_missing:
            missing = transaction_missing_headers_for_warning(mapping_missing)
            if missing:
                note_lines.append("未识别固定字段（将置空，可能影响清洗/统计）：")
                note_lines.append("- " + "、".join(missing[:20]))
                if len(missing) > 20:
                    note_lines.append(f"- ...（共 {len(missing)} 项）")

        if key_missing:
            warn_ratio = 0.3 if total_rows >= 20 else 0.8
            issues = []
            for name, ratio in key_missing.items():
                if ratio >= warn_ratio:
                    issues.append(f"{name}缺失 {int(round(ratio * 100))}%")
            if issues:
                note_lines.append("表头不完整/格式异常（关键字段缺失率过高）：")
                note_lines.append("- " + "；".join(issues) + "（疑似未匹配到对应列）")

        stat_parts = []
        if clean_stats.get("amt_fixed"):
            stat_parts.append(f"金额格式/符号标准化 {clean_stats['amt_fixed']} 行")
        if clean_stats.get("amt_failed"):
            stat_parts.append(f"金额格式异常 {clean_stats['amt_failed']} 行")
        if clean_stats.get("bal_failed"):
            stat_parts.append(f"余额格式异常 {clean_stats['bal_failed']} 行")
        if clean_stats.get("dc_infer"):
            stat_parts.append(f"收付标志推断 {clean_stats['dc_infer']} 行")
        if clean_stats.get("suffix_fixed"):
            stat_parts.append(f"卡号/账号后缀处理 {clean_stats['suffix_fixed']} 行")
        if stat_parts:
            note_lines.append("导入汇总：")
            note_lines.append("- " + "；".join(stat_parts))

    if extra_hints:
        note_lines.append("扩展字段高频候选（可考虑映射为固定字段）：")
        note_lines.append("- " + "；".join(extra_hints))
    return "\n".join(note_lines).strip()


def _append_privacy_delta_from_keys_table(
    engine: DuckDBEngine,
    *,
    case_id: str,
    keys_table: str,
) -> None:
    ensure_privacy_projection_delta_log(engine)
    engine.execute(
        f"""INSERT INTO privacy_projection_delta_log(case_id, op, file_id, row_hash, source, created_at)
            SELECT case_id, 'insert', COALESCE(file_id, ''), row_hash, 'import', ?
            FROM {keys_table}
            WHERE case_id=?""",
        (_now_iso(), case_id),
    )


def _safe_group_token(case_id: str, sources: Sequence[FcGroupImportSource]) -> str:
    seed = "|".join([case_id, *[source.file_id for source in sources], str(time.time())])
    return hashlib.md5(seed.encode("utf-8")).hexdigest()[:16]
