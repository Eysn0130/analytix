from __future__ import annotations

BASE_NAME_PICK_FILTERS: tuple[str, ...] = (
    "cp_key IS NOT NULL",
    "TRIM(cp_key) <> ''",
    "cp_name IS NOT NULL",
    "TRIM(cp_name) <> ''",
)
DETAIL_ACCOUNT_FILTERS: tuple[str, ...] = (
    "acct_key IS NOT NULL",
    "TRIM(acct_key) <> ''",
)


def name_pick_filters(*, require_account_key: bool = False) -> tuple[str, ...]:
    if require_account_key:
        return (*BASE_NAME_PICK_FILTERS, *DETAIL_ACCOUNT_FILTERS)
    return BASE_NAME_PICK_FILTERS


def build_name_pick_ctes_sql(source_name: str, *, require_account_key: bool = False) -> str:
    where_sql = "\n                AND ".join(name_pick_filters(require_account_key=require_account_key))
    return f"""
            name_stats AS (
              SELECT
                cp_key,
                cp_name,
                COUNT(1) AS name_cnt,
                MAX(txn_ts) AS name_last_ts
              FROM {source_name}
              WHERE {where_sql}
              GROUP BY cp_key, cp_name
            ), name_ranked AS (
              SELECT
                cp_key,
                cp_name,
                name_cnt,
                name_last_ts,
                ROW_NUMBER() OVER (
                  PARTITION BY cp_key
                  ORDER BY name_cnt DESC, name_last_ts DESC, cp_name DESC
                ) AS rn
              FROM name_stats
            ), name_pick AS (
              SELECT
                cp_key AS pick_key,
                cp_name AS pick_name,
                name_cnt AS pick_cnt
              FROM name_ranked
              WHERE rn = 1
            )
            """
