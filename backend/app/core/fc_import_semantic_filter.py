from __future__ import annotations

from typing import Dict, List, Protocol

from app.core.fc_import_norm_exprs import num_norm_expr, ts_norm_expr


class FcSemanticFilterSchema(Protocol):
    table: str
    headers: List[str]
    col_map: Dict[str, str]


def build_semantic_row_filter_expr(schema: FcSemanticFilterSchema, *, alias: str = "base") -> str:
    """Return an import-time business-validity filter for standard FC rows."""
    if schema.table == "fc_transaction":
        return _transaction_feedback_row_filter_expr(schema, alias=alias)
    if schema.table == "fc_person":
        id_expr = _raw_text_expr(schema, alias=alias, key="id_no")
        return _valid_business_value_expr(id_expr)
    if schema.table == "fc_account":
        card_expr = _raw_text_expr(schema, alias=alias, key="card_no")
        acct_expr = _raw_text_expr(schema, alias=alias, key="acct_no")
        valid_key_expr = f"({_valid_business_value_expr(card_expr)} OR {_valid_business_value_expr(acct_expr)})"
        meaningful_expr = " OR ".join(
            [
                _valid_business_value_expr(_raw_text_expr(schema, alias=alias, key=key))
                for key in (
                    "account_open_name",
                    "opener_id_no",
                    "open_time",
                    "available_balance",
                    "currency",
                    "branch_code",
                    "branch_name",
                    "acct_status",
                    "cashfx_flag_name",
                    "close_date",
                    "acct_type",
                    "remark",
                    "close_branch",
                )
            ]
        )
        balance_expr = _non_zero_amount_expr(_raw_text_expr(schema, alias=alias, key="balance"))
        return f"({valid_key_expr}) AND (({meaningful_expr}) OR {balance_expr})"
    return ""


def _transaction_feedback_row_filter_expr(schema: FcSemanticFilterSchema, *, alias: str) -> str:
    feedback_expr = _raw_text_expr(schema, alias=alias, key="query_feedback_reason")
    acct_expr = _raw_text_expr(schema, alias=alias, key="acct_no")
    card_expr = _raw_text_expr(schema, alias=alias, key="card_no")
    txn_time_expr = _raw_text_expr(schema, alias=alias, key="txn_time")
    amount_expr = _raw_text_expr(schema, alias=alias, key="amount")

    feedback_present = _non_empty_text_expr(feedback_expr)
    key_present = (
        f"COALESCE(({_valid_business_value_expr(acct_expr)} OR {_valid_business_value_expr(card_expr)}), FALSE)"
    )
    txn_time_present = f"{ts_norm_expr(txn_time_expr)} IS NOT NULL"
    normalized_amount = f"TRY_CAST({num_norm_expr(amount_expr)} AS DOUBLE)"
    amount_present = f"({normalized_amount} IS NOT NULL AND isfinite({normalized_amount}))"
    critical_missing = f"(NOT ({key_present}) OR NOT ({txn_time_present}) OR NOT ({amount_present}))"
    return f"NOT ({feedback_present} AND {critical_missing})"


def _raw_text_expr(schema: FcSemanticFilterSchema, *, alias: str, key: str) -> str:
    raw_column = _raw_column_name(schema, key)
    if not raw_column:
        return "NULL"
    return f"NULLIF(TRIM(COALESCE(CAST({alias}.{_quote_ident(raw_column)} AS VARCHAR), '')), '')"


def _raw_column_name(schema: FcSemanticFilterSchema, key: str) -> str:
    for header, column in schema.col_map.items():
        if column == key:
            return f"{column}_raw"
    return ""


def _valid_business_value_expr(expr: str) -> str:
    normalized = f"UPPER({expr})"
    return (
        f"{expr} IS NOT NULL "
        f"AND NOT regexp_matches({normalized}, "
        "'^(0+|NULL|N/A|NA|NONE|-+|—+|无|暂无|未知|不详|未找到相关数据|未查询到.*|未找到.*|没有.*(记录|数据)|无.*(记录|数据))$')"
    )


def _non_empty_text_expr(expr: str) -> str:
    return f"{expr} IS NOT NULL"


def _non_zero_amount_expr(expr: str) -> str:
    amount = f"try_cast(regexp_replace({expr}, '[,，\\s]', '', 'g') AS DOUBLE)"
    return f"({amount} IS NOT NULL AND isfinite({amount}) AND {amount} <> 0)"


def _quote_ident(name: str) -> str:
    return '"' + str(name or "").replace('"', '""') + '"'
