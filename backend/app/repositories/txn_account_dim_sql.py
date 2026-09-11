from __future__ import annotations

from dataclasses import dataclass
from typing import Optional

from app.repositories.txn_daily_materialization_plan_constants import (
    ACCOUNT_DIM_STAGING_TABLE,
    DETAIL_STAGING_TABLE,
    NULL_TEXT,
)
from app.repositories.txn_projection_expr_helpers import (
    account_key_expr,
    coalesce_expr,
    text_col_expr,
    text_col_expr_minlen,
)


@dataclass(frozen=True)
class AccountDimSelectItem:
    output_name: str
    expression: str

    @property
    def sql(self) -> str:
        return f"{self.expression} AS {self.output_name}"


def account_dim_select_items(
    *,
    include_account_table: bool,
    include_sub_account_parent: bool = False,
) -> tuple[AccountDimSelectItem, ...]:
    if include_account_table:
        sub_open_name_exprs = [", NULLIF(MAX(sp.parent_open_name), '')"] if include_sub_account_parent else []
        sub_id_no_exprs = [", NULLIF(MAX(sp.parent_id_no), '')"] if include_sub_account_parent else []
        sub_bank_exprs = [", NULLIF(MAX(sp.parent_bank_name), '')"] if include_sub_account_parent else []
        sub_branch_exprs = [", NULLIF(MAX(sp.parent_branch_name), '')"] if include_sub_account_parent else []
        sub_type_exprs = [", NULLIF(MAX(sp.parent_acct_type), '')"] if include_sub_account_parent else []
        return (
            AccountDimSelectItem("account_key", "b.account_key"),
            AccountDimSelectItem(
                "acct_display",
                "COALESCE(NULLIF(MAX(b.acct_display), ''), NULLIF(MAX(c.card_display), ''), b.account_key)",
            ),
            AccountDimSelectItem(
                "card_display",
                "COALESCE(NULLIF(MAX(c.card_display), ''), NULLIF(MAX(b.card_display), ''), NULLIF(MAX(b.acct_display), ''), b.account_key)",
            ),
            AccountDimSelectItem(
                "open_name",
                f"COALESCE(NULLIF(MAX(a.acct_open_name), ''), NULLIF(MAX(p.open_name), ''){''.join(sub_open_name_exprs)}, '')",
            ),
            AccountDimSelectItem(
                "id_no",
                f"COALESCE(NULLIF(MAX(a.acct_id_no), ''), NULLIF(MAX(p.id_no), ''){''.join(sub_id_no_exprs)}, '')",
            ),
            AccountDimSelectItem("bank_name", f"COALESCE(NULLIF(MAX(a.bank_name), ''){''.join(sub_bank_exprs)}, '')"),
            AccountDimSelectItem("branch_name", f"COALESCE(NULLIF(MAX(a.branch_name), ''){''.join(sub_branch_exprs)}, '')"),
            AccountDimSelectItem("acct_type", f"COALESCE(NULLIF(MAX(a.acct_type), ''){''.join(sub_type_exprs)}, '')"),
        )
    return (
        AccountDimSelectItem("account_key", "b.account_key"),
        AccountDimSelectItem(
            "acct_display",
            "COALESCE(NULLIF(MAX(b.acct_display), ''), NULLIF(MAX(c.card_display), ''), b.account_key)",
        ),
        AccountDimSelectItem(
            "card_display",
            "COALESCE(NULLIF(MAX(c.card_display), ''), NULLIF(MAX(b.card_display), ''), NULLIF(MAX(b.acct_display), ''), b.account_key)",
        ),
        AccountDimSelectItem("open_name", "COALESCE(NULLIF(MAX(p.open_name), ''), '')"),
        AccountDimSelectItem("id_no", "COALESCE(NULLIF(MAX(p.id_no), ''), '')"),
        AccountDimSelectItem("bank_name", "''"),
        AccountDimSelectItem("branch_name", "''"),
        AccountDimSelectItem("acct_type", "''"),
    )


def build_account_dim_sql(account_columns: Optional[set[str]], sub_account_columns: Optional[set[str]] = None) -> str:
    if account_columns is not None:
        acct_key_expr_a = account_key_expr("a", account_columns)
        acct_open_name_expr = text_col_expr("a", account_columns, "account_open_name", clean_invalid=True) or NULL_TEXT
        acct_id_no_expr = text_col_expr("a", account_columns, "opener_id_no", clean_invalid=True) or NULL_TEXT
        acct_bank_expr = coalesce_expr(
            [text_col_expr_minlen("a", account_columns, column, 2) for column in ("open_bank", "branch_name")],
            NULL_TEXT,
        )
        acct_branch_expr = text_col_expr_minlen("a", account_columns, "branch_name", 2) or NULL_TEXT
        acct_type_expr = text_col_expr_minlen("a", account_columns, "acct_type", 2) or NULL_TEXT
        sub_parent_ctes = ""
        sub_parent_join = ""
        include_sub_parent = bool(
            sub_account_columns is not None
            and {"case_id", "parent_acct", "sub_acct"}.issubset(sub_account_columns)
        )
        if include_sub_parent:
            sub_acct_expr = text_col_expr("s", sub_account_columns or set(), "sub_acct", clean_invalid=True) or NULL_TEXT
            parent_acct_expr = text_col_expr("s", sub_account_columns or set(), "parent_acct", clean_invalid=True) or NULL_TEXT
            sub_bank_expr = text_col_expr_minlen("s", sub_account_columns or set(), "bank_name", 2) or NULL_TEXT
            sub_type_expr = text_col_expr_minlen("s", sub_account_columns or set(), "sub_type", 2) or NULL_TEXT
            sub_parent_ctes = f""",
                sub_parent_base AS (
                  SELECT
                    {sub_acct_expr} AS account_key,
                    {parent_acct_expr} AS parent_account_key,
                    {sub_bank_expr} AS sub_bank_name,
                    {sub_type_expr} AS sub_acct_type
                  FROM fc_sub_account_norm s
                  WHERE s.case_id=?
                ), sub_parent_joined AS (
                  SELECT
                    s.account_key,
                    a.acct_open_name,
                    a.acct_id_no,
                    COALESCE(NULLIF(s.sub_bank_name, ''), NULLIF(a.bank_name, '')) AS bank_name,
                    a.branch_name,
                    COALESCE(NULLIF(s.sub_acct_type, ''), NULLIF(a.acct_type, '')) AS acct_type
                  FROM sub_parent_base s
                  JOIN acct_dim a ON a.account_key = s.parent_account_key
                  WHERE s.account_key IS NOT NULL
                    AND TRIM(s.account_key) <> ''
                    AND s.parent_account_key IS NOT NULL
                    AND TRIM(s.parent_account_key) <> ''
                ), sub_parent_dim AS (
                  SELECT
                    account_key,
                    CASE
                      WHEN COUNT(DISTINCT NULLIF(acct_open_name, '')) = 1
                      THEN MAX(NULLIF(acct_open_name, ''))
                      ELSE NULL
                    END AS parent_open_name,
                    CASE
                      WHEN COUNT(DISTINCT NULLIF(acct_id_no, '')) = 1
                      THEN MAX(NULLIF(acct_id_no, ''))
                      ELSE NULL
                    END AS parent_id_no,
                    CASE
                      WHEN COUNT(DISTINCT NULLIF(bank_name, '')) = 1
                      THEN MAX(NULLIF(bank_name, ''))
                      ELSE NULL
                    END AS parent_bank_name,
                    CASE
                      WHEN COUNT(DISTINCT NULLIF(branch_name, '')) = 1
                      THEN MAX(NULLIF(branch_name, ''))
                      ELSE NULL
                    END AS parent_branch_name,
                    CASE
                      WHEN COUNT(DISTINCT NULLIF(acct_type, '')) = 1
                      THEN MAX(NULLIF(acct_type, ''))
                      ELSE NULL
                    END AS parent_acct_type
                  FROM sub_parent_joined
                  GROUP BY account_key
                )"""
            sub_parent_join = "LEFT JOIN sub_parent_dim sp ON sp.account_key = b.account_key"
        select_sql = _select_sql(
            account_dim_select_items(
                include_account_table=True,
                include_sub_account_parent=include_sub_parent,
            )
        )
        return f"""
                CREATE TABLE {ACCOUNT_DIM_STAGING_TABLE} AS
                WITH {_txn_name_pick_ctes()},
                acct_base AS (
                  SELECT
                    {acct_key_expr_a} AS account_key,
                    {acct_open_name_expr} AS acct_open_name,
                    {acct_id_no_expr} AS acct_id_no,
                    {acct_bank_expr} AS bank_name,
                    {acct_branch_expr} AS branch_name,
                    {acct_type_expr} AS acct_type
                  FROM fc_account_norm a
                  WHERE a.case_id=?
                ), acct_dim AS (
                  SELECT
                    account_key,
                    MAX(acct_open_name) AS acct_open_name,
                    MAX(acct_id_no) AS acct_id_no,
                    MAX(bank_name) AS bank_name,
                    MAX(branch_name) AS branch_name,
                    MAX(acct_type) AS acct_type
                  FROM acct_base
                  WHERE account_key IS NOT NULL
                    AND TRIM(account_key) <> ''
                  GROUP BY account_key
                ){sub_parent_ctes}
                SELECT
                  {select_sql}
                FROM txn_base b
                LEFT JOIN txn_card_pick c ON c.account_key = b.account_key
                LEFT JOIN txn_name_pick p ON p.account_key = b.account_key
                LEFT JOIN acct_dim a ON a.account_key = b.account_key
                {sub_parent_join}
                WHERE b.account_key IS NOT NULL
                  AND TRIM(b.account_key) <> ''
                GROUP BY b.account_key
                ORDER BY b.account_key
                """
    select_sql = _select_sql(account_dim_select_items(include_account_table=False))
    return f"""
                CREATE TABLE {ACCOUNT_DIM_STAGING_TABLE} AS
                WITH {_txn_name_pick_ctes()}
                SELECT
                  {select_sql}
                FROM txn_base b
                LEFT JOIN txn_card_pick c ON c.account_key = b.account_key
                LEFT JOIN txn_name_pick p ON p.account_key = b.account_key
                WHERE b.account_key IS NOT NULL
                  AND TRIM(b.account_key) <> ''
                GROUP BY b.account_key
                ORDER BY b.account_key
                """


def _select_sql(items: tuple[AccountDimSelectItem, ...]) -> str:
    return ",\n                  ".join(item.sql for item in items)


def _txn_name_pick_ctes() -> str:
    return f"""
                txn_base AS (
                  SELECT
                    acct_key AS account_key,
                    COALESCE(NULLIF(acct_no, ''), acct_key) AS acct_display,
                    COALESCE(NULLIF(card_no, ''), NULLIF(acct_no, ''), acct_key) AS card_display,
                    account_open_name AS open_name,
                    opener_id_no AS id_no,
                    txn_ts AS txn_ts
                  FROM {DETAIL_STAGING_TABLE}
                ), txn_card_stats AS (
                  SELECT
                    account_key,
                    card_display,
                    COUNT(1) AS hit_count,
                    MAX(txn_ts) AS last_ts
                  FROM txn_base
                  WHERE account_key IS NOT NULL
                    AND TRIM(account_key) <> ''
                    AND card_display IS NOT NULL
                    AND TRIM(card_display) <> ''
                  GROUP BY account_key, card_display
                ), txn_card_pick AS (
                  SELECT account_key, card_display
                  FROM (
                    SELECT
                      account_key,
                      card_display,
                      hit_count,
                      last_ts,
                      ROW_NUMBER() OVER (
                        PARTITION BY account_key
                        ORDER BY
                          (last_ts IS NOT NULL) DESC,
                          last_ts DESC,
                          hit_count DESC,
                          card_display DESC
                      ) AS rn
                    FROM txn_card_stats
                  ) ranked
                  WHERE rn = 1
                ), txn_name_stats AS (
                  SELECT
                    account_key,
                    open_name,
                    id_no,
                    COUNT(1) AS hit_count,
                    MAX(txn_ts) AS last_ts
                  FROM txn_base
                  WHERE account_key IS NOT NULL
                    AND TRIM(account_key) <> ''
                    AND (COALESCE(open_name, '') <> '' OR COALESCE(id_no, '') <> '')
                  GROUP BY account_key, open_name, id_no
                ), txn_name_pick AS (
                  SELECT account_key, open_name, id_no
                  FROM (
                    SELECT
                      account_key,
                      open_name,
                      id_no,
                      hit_count,
                      last_ts,
                      ROW_NUMBER() OVER (
                        PARTITION BY account_key
                        ORDER BY
                          CASE WHEN COALESCE(open_name, '') <> '' THEN 1 ELSE 0 END DESC,
                          hit_count DESC,
                          (last_ts IS NOT NULL) DESC,
                          last_ts DESC,
                          open_name DESC,
                          id_no DESC
                      ) AS rn
                    FROM txn_name_stats
                  ) ranked
                  WHERE rn = 1
                )"""
