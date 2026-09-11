from __future__ import annotations

AGG_TABLE = "analysis_txn_daily_agg"
AGG_STAGING_TABLE = "analysis_txn_daily_agg__staging"
DETAIL_TABLE = "analysis_txn_detail_idx"
DETAIL_STAGING_TABLE = "analysis_txn_detail_idx__staging"
KEYWORD_TABLE = "analysis_txn_keyword_idx"
ACCOUNT_DIM_TABLE = "analysis_account_dim"
ACCOUNT_DIM_STAGING_TABLE = "analysis_account_dim__staging"
META_TABLE = "analysis_materialization_meta"
AGG_VERSION = 12
MATERIALIZATION_IDENTITY_SCHEMA_VERSION = 2
MATERIALIZATION_IDENTITY_PREFIX = f"txn_daily_snapshot:v{AGG_VERSION}:"
LEGACY_MATERIALIZATION_STAGING_TABLES = (
    "analysis_txn_projected__staging",
    "analysis_txn_daily_name_pick__staging",
    "analysis_txn_detail_name_pick__staging",
)
NULL_TEXT = "CAST(NULL AS VARCHAR)"
NULL_DOUBLE = "CAST(NULL AS DOUBLE)"
NULL_TIMESTAMP = "CAST(NULL AS TIMESTAMP)"
ACCOUNT_KEY_CANDIDATES = (
    "clean_acct_no",
    "acct_no_norm",
    "acct_no",
    "clean_card_no",
    "card_no_norm",
    "card_no",
)
