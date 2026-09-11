from __future__ import annotations

from dataclasses import dataclass

from app.core.fc_import_projection import sql_literal


@dataclass(frozen=True)
class GroupDedupKeysSql:
    keys_table: str
    staging_table: str
    raw_table: str
    case_id: str


def build_group_dedup_keys_sql(config: GroupDedupKeysSql) -> str:
    return f"""CREATE TEMP TABLE {config.keys_table} AS
                WITH first_sources AS (
                    SELECT s.row_hash, MIN(s.source_index) AS source_index
                    FROM {config.staging_table} s
                    GROUP BY s.row_hash
                ),
                first_rows AS (
                    SELECT s.row_hash, s.source_index, MIN(s.row_no) AS row_no
                    FROM {config.staging_table} s
                    JOIN first_sources f
                      ON f.row_hash=s.row_hash AND f.source_index=s.source_index
                    GROUP BY s.row_hash, s.source_index
                )
                SELECT s.case_id, s.file_id, s.row_no, s.row_hash
                FROM {config.staging_table} s
                JOIN first_rows f
                  ON f.row_hash=s.row_hash
                 AND f.source_index=s.source_index
                 AND f.row_no=s.row_no
                WHERE NOT EXISTS (
                    SELECT 1 FROM {config.raw_table} t
                    WHERE t.case_id={sql_literal(config.case_id)} AND t.row_hash=s.row_hash
                )
            """
