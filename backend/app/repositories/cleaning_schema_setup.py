from __future__ import annotations

from typing import Any

from app.repositories import cleaning_duckdb_meta


TRANSACTION_CLEAN_COLUMNS: tuple[tuple[str, str], ...] = (
    ("orig_amount", "TEXT"),
    ("orig_balance", "TEXT"),
    ("orig_dc_flag", "TEXT"),
    ("orig_card_no", "TEXT"),
    ("dc_final", "TEXT"),
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
)

ACCOUNT_CLEAN_COLUMNS: tuple[tuple[str, str], ...] = (
    ("clean_card_no", "TEXT"),
    ("clean_acct_no", "TEXT"),
    ("clean_suffix_fixed", "INTEGER DEFAULT 0"),
    ("clean_acct_invalid", "INTEGER DEFAULT 0"),
    ("cleaned_at", "TEXT"),
)

LOG_EXTENSION_COLUMNS: tuple[tuple[str, str], ...] = (
    ("file_id", "TEXT"),
    ("duration_ms", "BIGINT"),
    ("scope_rows", "BIGINT"),
    ("run_ref", "TEXT"),
)


def ensure_cleaning_schema(cur: Any) -> None:
    ensure_transaction_clean_columns(cur)
    ensure_account_clean_columns(cur)
    ensure_cleaning_log(cur)


def ensure_transaction_clean_columns(cur: Any) -> None:
    cols = _fetch_column_names(cur, "fc_transaction_norm")
    for name, definition in TRANSACTION_CLEAN_COLUMNS:
        if name not in cols:
            cur.execute(f"ALTER TABLE fc_transaction_norm ADD COLUMN {name} {definition}")


def ensure_account_clean_columns(cur: Any) -> None:
    if not cleaning_duckdb_meta.table_exists(cur, "fc_account_norm"):
        return
    cols = _fetch_column_names(cur, "fc_account_norm")
    for name, definition in ACCOUNT_CLEAN_COLUMNS:
        if name not in cols:
            cur.execute(f"ALTER TABLE fc_account_norm ADD COLUMN {name} {definition}")


def ensure_cleaning_log(cur: Any) -> None:
    cur.execute("CREATE SEQUENCE IF NOT EXISTS seq_cleaning_log")
    cur.execute(
        """CREATE TABLE IF NOT EXISTS cleaning_log(
               id BIGINT PRIMARY KEY DEFAULT nextval('seq_cleaning_log'),
               case_id TEXT,
               file_id TEXT,
               import_at TEXT,
               cleaned_at TEXT,
               duration_ms BIGINT,
               scope_rows BIGINT,
               summary TEXT
           )"""
    )
    cur.execute("CREATE INDEX IF NOT EXISTS idx_cleaning_log_case ON cleaning_log(case_id, cleaned_at)")
    cols = _fetch_column_names(cur, "cleaning_log")
    for name, definition in LOG_EXTENSION_COLUMNS:
        if name not in cols:
            cur.execute(f"ALTER TABLE cleaning_log ADD COLUMN {name} {definition}")


def _fetch_column_names(cur: Any, table: str) -> set[str]:
    cur.execute(
        "SELECT column_name FROM information_schema.columns "
        f"WHERE table_schema='main' AND table_name='{table}'"
    )
    return {r[0] for r in cur.fetchall()}


__all__ = [
    "ACCOUNT_CLEAN_COLUMNS",
    "LOG_EXTENSION_COLUMNS",
    "TRANSACTION_CLEAN_COLUMNS",
    "ensure_account_clean_columns",
    "ensure_cleaning_log",
    "ensure_cleaning_schema",
    "ensure_transaction_clean_columns",
]
