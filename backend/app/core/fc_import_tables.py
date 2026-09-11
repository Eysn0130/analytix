from __future__ import annotations

import logging
import time
from typing import Optional

from app.core import fc_import_schema
from app.core.db_engine import DuckDBEngine
from app.core.fc_import_duckdb import (
    duckdb_table_columns,
    duckdb_table_exists,
    ensure_columns,
    table_has_rows,
)
from app.core.fc_import_norm_insert import build_norm_insert_sql, row_hash_expr
from app.core.fc_import_privacy_delta import ensure_privacy_projection_delta_log
from app.core.fc_import_projection import raw_col
from app.core.safe_observability import log_closed_diagnostic


_IMPORT_FILE_LOG_COUNT_COLUMNS = (
    "rows_total",
    "rows_imported",
    "rows_imported_raw",
    "rows_imported_norm",
    "rows_dedup",
    "rows_error",
    "rows_skipped_non_data",
    "import_counts_version",
    "cleaned_rows_affected",
    "cleaning_counts_version",
)
_IMPORT_FILE_LOG_COUNT_COLUMN_TYPES = {
    column: "BIGINT" for column in _IMPORT_FILE_LOG_COUNT_COLUMNS
}


class ImportFileLogSchemaMigrationError(RuntimeError):
    def __init__(self) -> None:
        super().__init__("import_file_log_count_schema_migration_failed")


def build_raw_columns(schema: fc_import_schema.FcSchema) -> list[tuple[str, str]]:
    columns: list[tuple[str, str]] = [
        ("case_id", "TEXT"),
        ("file_id", "TEXT"),
        ("row_no", "BIGINT"),
        ("imported_at", "TEXT"),
        ("row_hash", "TEXT"),
    ]
    for header in schema.headers:
        columns.append((raw_col(schema.col_map[header]), "TEXT"))
    if schema.table in ("fc_transaction", "fc_account", "fc_sub_account"):
        columns.append(("extra_json", "TEXT"))
    if schema.store_raw_json:
        columns.append(("raw_json", "TEXT"))
    return columns


def build_norm_columns(schema: fc_import_schema.FcSchema) -> list[tuple[str, str]]:
    columns: list[tuple[str, str]] = [
        ("case_id", "TEXT"),
        ("file_id", "TEXT"),
        ("row_no", "BIGINT"),
        ("imported_at", "TEXT"),
        ("row_hash", "TEXT"),
    ]
    for header in schema.headers:
        columns.append((schema.col_map[header], "TEXT"))
    existing_col_names = {name for name, _ in columns}

    if schema.table == "fc_transaction":
        for name, type_sql in [
            ("txn_ts", "TIMESTAMP"),
            ("amount_val", "DOUBLE"),
            ("balance_val", "DOUBLE"),
            ("dc_norm", "TEXT"),
            ("card_no_norm", "TEXT"),
            ("acct_no_norm", "TEXT"),
            ("counterparty_acct_norm", "TEXT"),
            ("dc_final", "TEXT"),
            ("orig_amount", "TEXT"),
            ("orig_balance", "TEXT"),
            ("orig_dc_flag", "TEXT"),
            ("orig_card_no", "TEXT"),
            ("clean_amount", "TEXT"),
            ("clean_balance", "TEXT"),
            ("clean_dc_flag", "TEXT"),
            ("clean_card_no", "TEXT"),
            ("clean_acct_no", "TEXT"),
            ("clean_suffix_fixed", "INTEGER DEFAULT 0"),
            ("clean_invalid", "INTEGER DEFAULT 0"),
            ("clean_duplicate", "INTEGER DEFAULT 0"),
            ("clean_failed", "INTEGER DEFAULT 0"),
            ("clean_reversal", "INTEGER DEFAULT 0"),
            ("clean_dc_normalized", "INTEGER DEFAULT 0"),
            ("clean_dc_inferred", "INTEGER DEFAULT 0"),
            ("clean_card_filled", "INTEGER DEFAULT 0"),
            ("clean_amt_fixed", "INTEGER DEFAULT 0"),
            ("clean_amt_failed", "INTEGER DEFAULT 0"),
            ("clean_bal_fixed", "INTEGER DEFAULT 0"),
            ("clean_bal_failed", "INTEGER DEFAULT 0"),
            ("account_open_name", "TEXT"),
            ("opener_id_no", "TEXT"),
            ("clean_account_filled", "INTEGER DEFAULT 0"),
            ("cleaned_at", "TEXT"),
            ("extra_json", "TEXT"),
        ]:
            if name not in existing_col_names:
                columns.append((name, type_sql))
                existing_col_names.add(name)
    elif schema.table == "fc_account":
        columns.extend(
            [
                ("open_time_ts", "TIMESTAMP"),
                ("balance_val", "DOUBLE"),
                ("available_balance_val", "DOUBLE"),
                ("card_no_norm", "TEXT"),
                ("acct_no_norm", "TEXT"),
                ("clean_card_no", "TEXT"),
                ("clean_acct_no", "TEXT"),
                ("clean_suffix_fixed", "INTEGER DEFAULT 0"),
                ("clean_acct_invalid", "INTEGER DEFAULT 0"),
                ("cleaned_at", "TEXT"),
                ("extra_json", "TEXT"),
            ]
        )
    elif schema.table == "fc_sub_account":
        columns.extend([("extra_json", "TEXT")])
    return columns


def ensure_fc_tables(engine: DuckDBEngine, *, include_secondary_indexes: bool = True) -> None:
    engine.execute(
        """CREATE TABLE IF NOT EXISTS import_file_log(
            file_id TEXT PRIMARY KEY,
            case_id TEXT,
            kind TEXT,
            filename TEXT,
            display_path TEXT,
            stored_path TEXT,
            file_type TEXT,
            size BIGINT,
            md5 TEXT,
            sha256 TEXT,
            rows_total BIGINT,
            rows_imported BIGINT,
            rows_imported_raw BIGINT,
            rows_imported_norm BIGINT,
            rows_dedup BIGINT,
            rows_error BIGINT,
            rows_skipped_non_data BIGINT,
            import_counts_version BIGINT,
            status TEXT,
            error TEXT,
            cleaned_status TEXT,
            cleaned_started_at TEXT,
            cleaned_finished_at TEXT,
            cleaned_error TEXT,
            cleaned_rows_affected BIGINT,
            cleaning_counts_version BIGINT,
            created_at TEXT,
            finished_at TEXT
        )"""
    )
    engine.execute("CREATE INDEX IF NOT EXISTS idx_import_case_created ON import_file_log(case_id, created_at)")

    try:
        existing_cols = duckdb_table_columns(engine, "import_file_log")
        alters = [
            f"ALTER TABLE import_file_log ADD COLUMN {column} {column_type}"
            for column, column_type in _IMPORT_FILE_LOG_COUNT_COLUMN_TYPES.items()
            if column not in existing_cols
        ]
        if "cleaned_status" not in existing_cols:
            alters.append("ALTER TABLE import_file_log ADD COLUMN cleaned_status TEXT")
        if "cleaned_started_at" not in existing_cols:
            alters.append("ALTER TABLE import_file_log ADD COLUMN cleaned_started_at TEXT")
        if "cleaned_finished_at" not in existing_cols:
            alters.append("ALTER TABLE import_file_log ADD COLUMN cleaned_finished_at TEXT")
        if "cleaned_error" not in existing_cols:
            alters.append("ALTER TABLE import_file_log ADD COLUMN cleaned_error TEXT")
        for statement in alters:
            engine.execute(statement)
        for column in _IMPORT_FILE_LOG_COUNT_COLUMNS:
            engine.execute(f"ALTER TABLE import_file_log ALTER COLUMN {column} DROP DEFAULT")

        count_schema = {
            str(column_name): (str(data_type).upper(), column_default)
            for column_name, data_type, column_default in engine.query(
                "SELECT column_name, data_type, column_default "
                "FROM information_schema.columns "
                "WHERE table_schema='main' AND table_name='import_file_log' "
                "AND column_name IN ("
                + ",".join("?" for _ in _IMPORT_FILE_LOG_COUNT_COLUMNS)
                + ")",
                _IMPORT_FILE_LOG_COUNT_COLUMNS,
            )
        }
        if set(count_schema) != set(_IMPORT_FILE_LOG_COUNT_COLUMNS):
            raise ImportFileLogSchemaMigrationError()
        if any(
            data_type != "BIGINT" or column_default is not None
            for data_type, column_default in count_schema.values()
        ):
            raise ImportFileLogSchemaMigrationError()
    except ImportFileLogSchemaMigrationError:
        raise
    except Exception as exc:
        raise ImportFileLogSchemaMigrationError() from exc

    engine.execute(
        """CREATE TABLE IF NOT EXISTS acct_state(
            case_id TEXT,
            acct_key TEXT,
            last_txn_ts TIMESTAMP,
            last_balance DOUBLE,
            last_dc TEXT,
            updated_at TEXT,
            PRIMARY KEY(case_id, acct_key)
        )"""
    )
    ensure_privacy_projection_delta_log(engine)

    for schema in fc_import_schema.FC_SCHEMAS.values():
        base_table = schema.table
        raw_table = fc_import_schema.raw_table_name(base_table)
        norm_table = fc_import_schema.norm_table_name(base_table)

        raw_seq = f"seq_{raw_table}"
        norm_seq = f"seq_{norm_table}"
        engine.execute(f"CREATE SEQUENCE IF NOT EXISTS {raw_seq}")
        engine.execute(f"CREATE SEQUENCE IF NOT EXISTS {norm_seq}")

        raw_cols = build_raw_columns(schema)
        norm_cols = build_norm_columns(schema)

        raw_defs = [f"id BIGINT PRIMARY KEY DEFAULT nextval('{raw_seq}')"]
        raw_defs.extend([f"{name} {col_type}" for name, col_type in raw_cols])
        norm_defs = [f"id BIGINT PRIMARY KEY DEFAULT nextval('{norm_seq}')"]
        norm_defs.extend([f"{name} {col_type}" for name, col_type in norm_cols])

        engine.execute(f"CREATE TABLE IF NOT EXISTS {raw_table}({', '.join(raw_defs)})")
        engine.execute(f"CREATE TABLE IF NOT EXISTS {norm_table}({', '.join(norm_defs)})")

        ensure_columns(engine, raw_table, raw_cols)
        ensure_columns(engine, norm_table, norm_cols)

        if base_table == "fc_transaction":
            compatibility_cols = [("receipt_no", "TEXT"), ("log_no", "TEXT")]
            ensure_columns(engine, raw_table, compatibility_cols)
            ensure_columns(engine, norm_table, compatibility_cols)

        try:
            engine.execute(
                f"CREATE UNIQUE INDEX IF NOT EXISTS uq_{raw_table}_case_hash ON {raw_table}(case_id, row_hash)"
            )
        except Exception:
            pass
        if include_secondary_indexes:
            ensure_fc_secondary_indexes(engine, schema=schema)

    _migrate_legacy_single_tables(engine)


def ensure_fc_secondary_indexes(
    engine: DuckDBEngine,
    *,
    schema: Optional[fc_import_schema.FcSchema] = None,
) -> None:
    schemas = [schema] if schema is not None else list(fc_import_schema.FC_SCHEMAS.values())
    for current_schema in schemas:
        base_table = current_schema.table
        raw_table = fc_import_schema.raw_table_name(base_table)
        norm_table = fc_import_schema.norm_table_name(base_table)
        if not duckdb_table_exists(engine, raw_table) or not duckdb_table_exists(engine, norm_table):
            continue
        engine.execute(f"CREATE INDEX IF NOT EXISTS idx_{raw_table}_file ON {raw_table}(file_id)")
        engine.execute(f"CREATE INDEX IF NOT EXISTS idx_{norm_table}_file ON {norm_table}(file_id)")

        if base_table == "fc_transaction":
            engine.execute(
                f"CREATE INDEX IF NOT EXISTS idx_{norm_table}_card_norm ON {norm_table}(case_id, card_no_norm)"
            )
            engine.execute(
                f"CREATE INDEX IF NOT EXISTS idx_{norm_table}_acct_norm ON {norm_table}(case_id, acct_no_norm)"
            )
            engine.execute(
                f"CREATE INDEX IF NOT EXISTS idx_{norm_table}_txn_ts ON {norm_table}(case_id, txn_ts)"
            )
        if base_table == "fc_account":
            engine.execute(
                f"CREATE INDEX IF NOT EXISTS idx_{norm_table}_card_norm ON {norm_table}(case_id, card_no_norm)"
            )
            engine.execute(
                f"CREATE INDEX IF NOT EXISTS idx_{norm_table}_acct_norm ON {norm_table}(case_id, acct_no_norm)"
            )


def _migrate_legacy_single_tables(engine: DuckDBEngine) -> None:
    for schema in fc_import_schema.FC_SCHEMAS.values():
        base_table = schema.table
        raw_table = fc_import_schema.raw_table_name(base_table)
        norm_table = fc_import_schema.norm_table_name(base_table)
        if not duckdb_table_exists(engine, base_table):
            continue
        if table_has_rows(engine, raw_table):
            continue
        started = time.perf_counter()
        existing_cols = duckdb_table_columns(engine, base_table)

        def _col_or_null(name: str) -> str:
            return f"t.{name}" if name in existing_cols else "NULL"

        base_cols = [schema.col_map[header] for header in schema.headers]
        raw_cols = [raw_col(column) for column in base_cols]
        row_hash_sql = "t.row_hash"
        if "row_hash" not in existing_cols:
            row_hash_sql = row_hash_expr([f"t.{column}" for column in base_cols])
        raw_json_expr = "t.raw_json" if schema.store_raw_json and "raw_json" in existing_cols else "NULL"

        insert_cols = ["case_id", "file_id", "row_no", "imported_at", "row_hash"]
        insert_cols.extend(raw_cols)
        if schema.store_raw_json:
            insert_cols.append("raw_json")

        select_exprs = [
            _col_or_null("case_id"),
            _col_or_null("file_id"),
            _col_or_null("row_no"),
            _col_or_null("imported_at"),
            f"{row_hash_sql} AS row_hash",
        ]
        select_exprs.extend([f"t.{column} AS {raw_col(column)}" for column in base_cols])
        if schema.store_raw_json:
            select_exprs.append(raw_json_expr)

        engine.execute(
            f"""INSERT INTO {raw_table}({', '.join(insert_cols)})
                SELECT {', '.join(select_exprs)}
                FROM {base_table} t
                WHERE t.case_id IS NOT NULL
                  AND NOT EXISTS (
                    SELECT 1 FROM {raw_table} r
                    WHERE r.case_id=t.case_id AND r.row_hash={row_hash_sql}
                  )"""
        )
        norm_sql, norm_params = build_norm_insert_sql(schema, raw_table, norm_table)
        engine.execute(norm_sql, norm_params)
        elapsed = time.perf_counter() - started
        log_closed_diagnostic(
            logging.getLogger("analytix.data_analysis.migration"),
            logging.INFO,
            topic="application",
            code="completed",
            numeric={"duration_ms": max(0, int(elapsed * 1000))},
        )
