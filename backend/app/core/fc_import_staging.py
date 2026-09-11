from __future__ import annotations

from dataclasses import dataclass
from typing import Sequence


@dataclass(frozen=True)
class ImportStagingSql:
    staging_table: str
    case_id_sql: str
    file_id_sql: str
    imported_at_sql: str
    select_cols: Sequence[str]
    extra_json_expr: str
    raw_json_expr: str
    hash_expr: str
    read_expr: str
    non_empty_filter_expr: str = ""


def build_import_staging_sql(config: ImportStagingSql) -> str:
    if not config.select_cols:
        raise ValueError("import staging requires projected columns")
    where_sql = f"\n                            WHERE {config.non_empty_filter_expr}" if config.non_empty_filter_expr else ""
    return f"""CREATE TEMP TABLE {config.staging_table} AS
                            WITH base AS (
                                SELECT
                                    {config.case_id_sql} AS case_id,
                                    {config.file_id_sql} AS file_id,
                                    row_number() OVER () AS row_no,
                                    {config.imported_at_sql} AS imported_at,
                                    {', '.join(config.select_cols)}
                                    {config.extra_json_expr}
                                    {config.raw_json_expr}
                                FROM {config.read_expr}
                            )
                            SELECT base.*, {config.hash_expr} AS row_hash FROM base{where_sql}
                        """
