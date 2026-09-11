from __future__ import annotations

from dataclasses import dataclass
from typing import List, Mapping, Sequence

from app.core.db_engine import DuckDBEngine
from app.core.fc_import_missing_notes import transaction_missing_headers_for_warning
from app.core.fc_import_projection import EXTRA_JSON_TABLES, ImportProjection
from app.core.fc_import_schema import FcSchema
from app.core.import_count_semantics import known_public_import_count


CLEAN_STATS_UNAVAILABLE_NOTE = "导入汇总：清洗统计不可用，未将缺失值解释为零。"


@dataclass(frozen=True)
class SingleFileImportNoteContext:
    schema: FcSchema
    norm_table: str
    case_id: str
    file_id: str
    total_rows: int
    inserted_rows: int
    norm_inserted: int
    dedup_rows: int
    dup_in_file: int
    dup_existing: int
    key_missing: Mapping[str, float]
    projection: ImportProjection
    alias_map: Mapping[str, Sequence[str]]


def build_single_file_import_note(engine: DuckDBEngine, context: SingleFileImportNoteContext) -> str:
    note_lines: List[str] = []
    _append_duplicate_note(note_lines, context)
    if context.schema.table == "fc_transaction":
        _append_transaction_projection_notes(note_lines, context)
        _append_key_missing_note(note_lines, context)
        _append_clean_stats_note(engine, note_lines, context)
    _append_extra_json_hints(engine, note_lines, context)
    return "\n".join(note_lines).strip()


def _append_duplicate_note(note_lines: List[str], context: SingleFileImportNoteContext) -> None:
    if not context.dedup_rows:
        return
    reasons: List[str] = []
    if context.dup_in_file:
        reasons.append(f"文件内重复 {context.dup_in_file} 行")
    if context.dup_existing:
        reasons.append(f"与历史导入重复 {context.dup_existing} 行")
    reason_text = "；".join(reasons) if reasons else "与历史数据或本文件重复（行哈希一致）"
    note_lines.append(f"重复导入：本次 {context.dedup_rows} 行未写入（{reason_text}）")


def _append_transaction_projection_notes(note_lines: List[str], context: SingleFileImportNoteContext) -> None:
    projection = context.projection
    if projection.mapping_alias_used:
        note_lines.append("自动映射：")
        note_lines.extend([f"- {item}" for item in projection.mapping_alias_used[:30]])
        if len(projection.mapping_alias_used) > 30:
            note_lines.append(f"- ...（共 {len(projection.mapping_alias_used)} 条）")
    if projection.mapping_derived:
        note_lines.append("自动处理：")
        for item in projection.mapping_derived:
            note_lines.append(f"- {item}")
    if projection.mapping_missing:
        missing = transaction_missing_headers_for_warning(projection.mapping_missing)
        if missing:
            note_lines.append("未识别固定字段（将置空，可能影响清洗/统计）：")
            note_lines.append("- " + "、".join(missing[:20]))
            if len(missing) > 20:
                note_lines.append(f"- ...（共 {len(missing)} 项）")


def _append_key_missing_note(note_lines: List[str], context: SingleFileImportNoteContext) -> None:
    if not context.key_missing:
        return
    warn_ratio = 0.3 if context.total_rows >= 20 else 0.8
    issues: List[str] = []
    for name, ratio in context.key_missing.items():
        if ratio >= warn_ratio:
            pct = int(round(ratio * 100))
            issues.append(f"{name}缺失 {pct}%")
    if issues:
        note_lines.append("表头不完整/格式异常（关键字段缺失率过高）：")
        note_lines.append("- " + "；".join(issues) + "（疑似未匹配到对应列）")


def _append_clean_stats_note(
    engine: DuckDBEngine,
    note_lines: List[str],
    context: SingleFileImportNoteContext,
) -> None:
    try:
        rows = engine.query(
            "SELECT "
            "  SUM(CASE WHEN clean_amt_fixed=1 THEN 1 ELSE 0 END) AS amt_fixed, "
            "  SUM(CASE WHEN clean_amt_failed=1 THEN 1 ELSE 0 END) AS amt_failed, "
            "  SUM(CASE WHEN clean_bal_failed=1 THEN 1 ELSE 0 END) AS bal_failed, "
            "  SUM(CASE WHEN clean_dc_normalized=1 THEN 1 ELSE 0 END) AS dc_norm, "
            "  SUM(CASE WHEN clean_dc_inferred=1 THEN 1 ELSE 0 END) AS dc_infer, "
            "  SUM(CASE WHEN clean_suffix_fixed=1 THEN 1 ELSE 0 END) AS suffix_fixed "
            f"FROM {context.norm_table} WHERE case_id=? AND file_id=?",
            (context.case_id, context.file_id),
        )
        if len(rows) != 1 or not isinstance(rows[0], (list, tuple)) or len(rows[0]) != 6:
            raise ValueError("clean_stats_row_unavailable")
        counts = [known_public_import_count(value) for value in rows[0]]
        if any(value is None for value in counts):
            raise ValueError("clean_stats_count_unavailable")
        amt_fixed, amt_failed, bal_failed, _dc_norm, dc_infer, suffix_fixed = counts
        stat_parts: List[str] = []
        if amt_fixed:
            stat_parts.append(f"金额格式/符号标准化 {amt_fixed} 行")
        if amt_failed:
            stat_parts.append(f"金额格式异常 {amt_failed} 行{_sample_failed_values(engine, context, 'orig_amount', 'clean_amt_failed')}")
        if bal_failed:
            stat_parts.append(f"余额格式异常 {bal_failed} 行{_sample_failed_values(engine, context, 'orig_balance', 'clean_bal_failed')}")
        if dc_infer:
            stat_parts.append(f"收付标志推断 {dc_infer} 行")
        if suffix_fixed:
            stat_parts.append(f"卡号/账号后缀处理 {suffix_fixed} 行")
        if stat_parts:
            note_lines.append("导入汇总：")
            note_lines.append("- " + "；".join(stat_parts))
    except Exception:
        note_lines.append(CLEAN_STATS_UNAVAILABLE_NOTE)


def _sample_failed_values(
    engine: DuckDBEngine,
    context: SingleFileImportNoteContext,
    value_column: str,
    flag_column: str,
) -> str:
    try:
        rows = engine.query(
            f"SELECT DISTINCT {value_column} FROM {context.norm_table} "
            f"WHERE case_id=? AND file_id=? AND {flag_column}=1 "
            f"AND {value_column} IS NOT NULL AND TRIM({value_column})<>'' LIMIT 3",
            (context.case_id, context.file_id),
        )
        samples = [str(row[0]) for row in rows if row and row[0]]
        if samples:
            return "，例：" + "、".join(samples)
    except Exception:
        return ""
    return ""


def _append_extra_json_hints(
    engine: DuckDBEngine,
    note_lines: List[str],
    context: SingleFileImportNoteContext,
) -> None:
    if context.schema.table not in EXTRA_JSON_TABLES:
        return
    try:
        rows = engine.query(
            "SELECT k, COUNT(1) AS cnt FROM ("
            "  SELECT unnest(json_keys(extra_json)) AS k "
            f"  FROM {context.norm_table} "
            "  WHERE case_id=? AND file_id=? AND extra_json IS NOT NULL AND extra_json<>''"
            ") t GROUP BY k ORDER BY cnt DESC",
            (context.case_id, context.file_id),
        )
        known_rows = [
            known_public_import_count(context.norm_inserted),
            known_public_import_count(context.inserted_rows),
            known_public_import_count(context.total_rows),
        ]
        if any(value is None for value in known_rows):
            return
        base_rows = max(*known_rows, 1)
        extra_candidates: List[tuple[str, int]] = []
        for key, count in rows:
            known_count = known_public_import_count(count)
            if key and known_count is not None:
                extra_candidates.append((str(key), known_count))
        high_freq = [(key, count / base_rows) for key, count in extra_candidates if count / base_rows >= 0.6]
        if not high_freq:
            return
        note_lines.append("扩展字段高频候选（可考虑映射为固定字段）：")
        note_lines.append("- " + "；".join([f"{key}≈{int(ratio * 100)}%" for key, ratio in high_freq[:6]]))

        if context.projection.mapping_missing and context.alias_map:
            cand_keys = {key for key, _ in high_freq}
            suggestions: List[str] = []
            for std_header in context.projection.mapping_missing:
                for alias in context.alias_map.get(std_header, []):
                    if alias in cand_keys:
                        suggestions.append(f"{alias}→{std_header}")
                        break
            if suggestions:
                note_lines.append("扩展字段候选映射：")
                note_lines.append("- " + "；".join(suggestions[:10]))
    except Exception:
        return
