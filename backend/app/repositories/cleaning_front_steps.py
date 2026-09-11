from __future__ import annotations

from typing import Any, Dict, Tuple

from app.core.fc_import_norm_insert import ts_norm_expr as _ts_norm_expr
from app.repositories import cleaning_sql_counter, cleaning_sql_expr
from app.repositories.cleaning_step_context import CleaningStepContext


REVERSAL_KEYWORDS = ("冲正", "消费退货", "抹帐", "抹账")
REVERSAL_TEXT_COLUMNS = ("summary", "txn_type", "remark")


def _clean_numeric_text_expr(column: str) -> str:
    return f"replace(replace(COALESCE(TRIM(CAST({column} AS TEXT)), ''), '，', ','), ',', '')"


def _source_amount_negative_expr() -> str:
    orig_amount = f"TRY_CAST(NULLIF({_clean_numeric_text_expr('orig_amount')}, '') AS DOUBLE)"
    raw_amount = f"TRY_CAST(NULLIF({_clean_numeric_text_expr('amount')}, '') AS DOUBLE)"
    return f"COALESCE({orig_amount}, {raw_amount}) < 0"


def _reversal_detail_keyword_expr() -> str:
    clauses = [
        f"COALESCE({column},'') LIKE '%{keyword}%'"
        for column in REVERSAL_TEXT_COLUMNS
        for keyword in REVERSAL_KEYWORDS
    ]
    return "(" + " OR ".join(clauses) + ")"


def step_amount_balance(context: CleaningStepContext, cur: Any, total_rows: int) -> Dict[str, int]:
    context.check_cancel()
    number_pattern = r"(?:\d+(?:,\d{3})*(?:\.\d+)?|\.\d+)(?:[eE][-+]?\d+)?"
    pattern = rf"[-+]?{number_pattern}"
    full_plain = rf"^[-+]?{number_pattern}$"
    full_paren = rf"^\({number_pattern}\)$"

    def _norm_expr(src: str) -> str:
        valid_plain = f"regexp_matches({src}, '{full_plain}')"
        valid_paren = f"regexp_matches({src}, '{full_paren}')"
        num = f"regexp_extract({src}, '{pattern}', 0)"
        num_clean = f"replace({num}, ',', '')"
        num_normalized = (
            "CASE "
            f"WHEN {num_clean} LIKE '.%' THEN '0' || {num_clean} "
            f"WHEN {num_clean} LIKE '-.%' THEN '-0' || substr({num_clean}, 2) "
            f"WHEN {num_clean} LIKE '+.%' THEN '+0' || substr({num_clean}, 2) "
            f"ELSE {num_clean} END"
        )
        num_abs = f"regexp_replace({num_normalized}, '^[-+]', '')"
        return (
            "CASE "
            f"WHEN {src} IS NULL OR {src}='' THEN NULL "
            f"WHEN NOT ({valid_plain} OR {valid_paren}) THEN NULL "
            f"WHEN {num} IS NULL OR {num}='' THEN NULL "
            f"ELSE {num_abs} END"
        )

    amt_src = "replace(replace(COALESCE(TRIM(CAST(amount AS TEXT)), ''), '，', ','), ' ', '')"
    bal_src = "replace(replace(COALESCE(TRIM(CAST(balance AS TEXT)), ''), '，', ','), ' ', '')"
    amt_norm = _norm_expr(amt_src)
    bal_norm = _norm_expr(bal_src)

    cur.execute("DROP TABLE IF EXISTS amount_balance_parsed")
    cur.execute(
        f"""CREATE TEMP TABLE amount_balance_parsed AS
            WITH normalized AS (
                SELECT id,
                       COALESCE(TRIM(CAST(amount AS TEXT)), '') AS amt_old,
                       COALESCE(TRIM(CAST(balance AS TEXT)), '') AS bal_old,
                       {amt_norm} AS amt_norm,
                       {bal_norm} AS bal_norm
                FROM fc_transaction_norm
                WHERE {context.scope_state.txn_where}
            )
            SELECT id,
                   amt_old,
                   bal_old,
                   amt_norm,
                   bal_norm,
                   TRY_CAST(amt_norm AS DOUBLE) AS amt_val,
                   TRY_CAST(bal_norm AS DOUBLE) AS bal_val
            FROM normalized""",
        tuple(context.scope_state.txn_params),
    )

    cur.execute("DROP TABLE IF EXISTS amount_balance_desired")
    cur.execute(
        """CREATE TEMP TABLE amount_balance_desired AS
           WITH joined AS (
               SELECT
                   t.id,
                   t.orig_amount,
                   t.orig_balance,
                   t.clean_amount,
                   t.clean_balance,
                   t.clean_amt_fixed,
                   t.clean_amt_failed,
                   t.clean_bal_fixed,
                   t.clean_bal_failed,
                   p.amt_old,
                   p.bal_old,
                   p.amt_norm,
                   p.bal_norm,
                   p.amt_val,
                   p.bal_val,
                   (
                       (p.amt_val IS NOT NULL AND p.amt_old<>'' AND p.amt_norm<>p.amt_old)
                       OR (p.amt_val IS NULL AND p.amt_old<>'')
                       OR (p.amt_val IS NOT NULL AND COALESCE(t.clean_amt_failed, 0)=1)
                       OR (p.bal_val IS NOT NULL AND p.bal_old<>'' AND p.bal_norm<>p.bal_old)
                       OR (p.bal_val IS NULL AND p.bal_old<>'')
                       OR (p.bal_val IS NOT NULL AND COALESCE(t.clean_bal_failed, 0)=1)
                       OR t.orig_amount IS NULL OR t.orig_amount=''
                       OR t.orig_balance IS NULL OR t.orig_balance=''
                   ) AS update_candidate
               FROM fc_transaction_norm AS t
               JOIN amount_balance_parsed p ON t.id=p.id
           ),
           desired_values AS (
               SELECT
                   id,
                   orig_amount,
                   orig_balance,
                   clean_amount,
                   clean_balance,
                   clean_amt_fixed,
                   clean_amt_failed,
                   clean_bal_fixed,
                   clean_bal_failed,
                   update_candidate,
                   CASE
                       WHEN update_candidate THEN COALESCE(NULLIF(orig_amount,''), NULLIF(amt_old,''))
                       ELSE orig_amount
                   END AS orig_amount_new,
                   CASE
                       WHEN update_candidate THEN COALESCE(NULLIF(orig_balance,''), NULLIF(bal_old,''))
                       ELSE orig_balance
                   END AS orig_balance_new,
                   CASE
                       WHEN NOT update_candidate THEN clean_amount
                       WHEN amt_old<>'' AND amt_val IS NULL THEN NULL
                       WHEN amt_val IS NOT NULL THEN amt_norm
                       ELSE clean_amount
                   END AS clean_amount_new,
                   CASE
                       WHEN NOT update_candidate THEN clean_balance
                       WHEN bal_old<>'' AND bal_val IS NULL THEN NULL
                       WHEN bal_val IS NOT NULL THEN bal_norm
                       ELSE clean_balance
                   END AS clean_balance_new,
                   CASE
                       WHEN update_candidate
                            AND amt_val IS NOT NULL AND amt_old<>'' AND amt_norm<>amt_old
                       THEN 1 ELSE clean_amt_fixed END AS clean_amt_fixed_new,
                   CASE
                       WHEN NOT update_candidate THEN clean_amt_failed
                       WHEN amt_val IS NULL AND amt_old<>'' THEN 1
                       WHEN amt_val IS NOT NULL THEN 0
                       ELSE clean_amt_failed
                   END AS clean_amt_failed_new,
                   CASE
                       WHEN update_candidate
                            AND bal_val IS NOT NULL AND bal_old<>'' AND bal_norm<>bal_old
                       THEN 1 ELSE clean_bal_fixed END AS clean_bal_fixed_new,
                   CASE
                       WHEN NOT update_candidate THEN clean_bal_failed
                       WHEN bal_val IS NULL AND bal_old<>'' THEN 1
                       WHEN bal_val IS NOT NULL THEN 0
                       ELSE clean_bal_failed
                   END AS clean_bal_failed_new,
                   CASE
                       WHEN update_candidate
                            AND amt_val IS NOT NULL AND amt_old<>'' AND amt_norm<>amt_old
                            AND (
                                clean_amount IS DISTINCT FROM amt_norm
                                OR clean_amt_fixed IS DISTINCT FROM 1
                                OR clean_amt_failed IS DISTINCT FROM 0
                            )
                       THEN 1 ELSE 0 END AS amount_updated_changed,
                   CASE
                       WHEN update_candidate
                            AND amt_val IS NULL AND amt_old<>''
                            AND (
                                clean_amount IS NOT NULL
                                OR clean_amt_failed IS DISTINCT FROM 1
                            )
                       THEN 1 ELSE 0 END AS amount_failed_changed,
                   CASE
                       WHEN update_candidate
                            AND bal_val IS NOT NULL AND bal_old<>'' AND bal_norm<>bal_old
                            AND (
                                clean_balance IS DISTINCT FROM bal_norm
                                OR clean_bal_fixed IS DISTINCT FROM 1
                                OR clean_bal_failed IS DISTINCT FROM 0
                            )
                       THEN 1 ELSE 0 END AS balance_updated_changed,
                   CASE
                       WHEN update_candidate
                            AND bal_val IS NULL AND bal_old<>''
                            AND (
                                clean_balance IS NOT NULL
                                OR clean_bal_failed IS DISTINCT FROM 1
                            )
                       THEN 1 ELSE 0 END AS balance_failed_changed
               FROM joined
           )
           SELECT
               id,
               orig_amount_new,
               orig_balance_new,
               clean_amount_new,
               clean_balance_new,
               clean_amt_fixed_new,
               clean_amt_failed_new,
               clean_bal_fixed_new,
               clean_bal_failed_new,
               amount_failed_changed,
               amount_updated_changed,
               balance_failed_changed,
               balance_updated_changed,
               CASE
                   WHEN update_candidate
                        AND (
                            orig_amount_new IS DISTINCT FROM orig_amount
                            OR orig_balance_new IS DISTINCT FROM orig_balance
                            OR clean_amount_new IS DISTINCT FROM clean_amount
                            OR clean_balance_new IS DISTINCT FROM clean_balance
                            OR clean_amt_fixed_new IS DISTINCT FROM clean_amt_fixed
                            OR clean_amt_failed_new IS DISTINCT FROM clean_amt_failed
                            OR clean_bal_fixed_new IS DISTINCT FROM clean_bal_fixed
                            OR clean_bal_failed_new IS DISTINCT FROM clean_bal_failed
                        )
                   THEN 1 ELSE 0 END AS should_update
           FROM desired_values"""
    )

    cur.execute(
        """SELECT
               SUM(amount_failed_changed) AS amt_failed,
               SUM(amount_updated_changed) AS amt_fixed,
               SUM(balance_failed_changed) AS bal_failed,
               SUM(balance_updated_changed) AS bal_fixed
           FROM amount_balance_desired"""
    )
    amt_failed, amt_updated, bal_failed, bal_updated = cleaning_sql_counter.require_count_values(
        cur.fetchone(),
        width=4,
        code="cleaning_amount_balance_counts_unavailable",
    )

    cur.execute(
        "SELECT COUNT(1) FROM amount_balance_desired WHERE should_update=1",
    )
    affected = cleaning_sql_counter.require_count_row(
        cur.fetchone(),
        code="cleaning_amount_balance_affected_count_unavailable",
    )

    cur.execute(
        """UPDATE fc_transaction_norm AS t
              SET orig_amount=d.orig_amount_new,
                  orig_balance=d.orig_balance_new,
                  clean_amount=d.clean_amount_new,
                  clean_balance=d.clean_balance_new,
                  clean_amt_fixed=d.clean_amt_fixed_new,
                  clean_amt_failed=d.clean_amt_failed_new,
                  clean_bal_fixed=d.clean_bal_fixed_new,
                  clean_bal_failed=d.clean_bal_failed_new
             FROM amount_balance_desired d
            WHERE t.id=d.id
              AND d.should_update=1""",
    )
    context.emit_progress(
        1,
        10,
        f"step1 amount/balance normalized ({total_rows}/{total_rows})",
        total_rows,
        total_rows,
    )

    cur.execute(
        """SELECT
               SUM(clean_amt_fixed=1),
               SUM(clean_amt_failed=1),
               SUM(clean_bal_fixed=1),
               SUM(clean_bal_failed=1)
           FROM fc_transaction_norm WHERE case_id=?""",
        (context.case_id,),
    )
    amount_fixed_total, amount_failed_total, balance_fixed_total, balance_failed_total = (
        cleaning_sql_counter.require_count_values(
            cur.fetchone(),
            width=4,
            code="cleaning_amount_balance_totals_unavailable",
        )
    )
    return {
        "amount_updated": amt_updated,
        "balance_updated": bal_updated,
        "amount_failed": amt_failed,
        "balance_failed": bal_failed,
        "amount_fixed_total": amount_fixed_total,
        "amount_failed_total": amount_failed_total,
        "balance_fixed_total": balance_fixed_total,
        "balance_failed_total": balance_failed_total,
        "rows_affected": affected,
    }


def step_invalid(context: CleaningStepContext, cur: Any) -> Tuple[int, int]:
    context.check_cancel()
    changed = cleaning_sql_counter.update_count(
        cur,
        f"""UPDATE fc_transaction_norm
           SET clean_invalid=1
           WHERE {context.scope_state.txn_where}
             AND (
               COALESCE(TRIM(txn_time),'')=''
               OR COALESCE(TRIM(amount),'')=''
               OR (
                 COALESCE(TRIM(COALESCE(NULLIF(clean_card_no,''), card_no)),'')=''
                 AND COALESCE(TRIM(COALESCE(NULLIF(clean_acct_no,''), acct_no)),'')=''
               )
             )
             AND COALESCE(clean_invalid,0)<>1""",
        tuple(context.scope_state.txn_params),
    )
    cur.execute("SELECT COUNT(1) FROM fc_transaction_norm WHERE case_id=? AND clean_invalid=1", (context.case_id,))
    total = cleaning_sql_counter.require_count_row(
        cur.fetchone(),
        code="cleaning_invalid_total_unavailable",
    )
    return changed, total


def step_duplicate(context: CleaningStepContext, cur: Any) -> Tuple[int, int]:
    context.check_cancel()
    changed = cleaning_sql_counter.update_count(
        cur,
        f"""UPDATE fc_transaction_norm
            SET clean_duplicate=0
            WHERE {context.scope_state.txn_where}
              AND COALESCE(clean_duplicate,0)=1""",
        tuple(context.scope_state.txn_params),
    )
    cur.execute("SELECT COUNT(1) FROM fc_transaction_norm WHERE case_id=? AND clean_duplicate=1", (context.case_id,))
    total = cleaning_sql_counter.require_count_row(
        cur.fetchone(),
        code="cleaning_duplicate_total_unavailable",
    )
    return changed, total


def step_failed_reversal(context: CleaningStepContext, cur: Any) -> Tuple[int, int, int, int]:
    context.check_cancel()
    reversal_detail_expr = _reversal_detail_keyword_expr()
    source_amount_negative_expr = _source_amount_negative_expr()
    failed_changed = cleaning_sql_counter.update_count(
        cur,
        f"""UPDATE fc_transaction_norm
           SET clean_failed=1
           WHERE {context.scope_state.txn_where}
             AND (
               COALESCE(query_feedback_reason,'') LIKE '%失败%'
               OR COALESCE(query_feedback_reason,'') LIKE '%无%'
               OR COALESCE(query_feedback_reason,'') LIKE '%不%'
               OR COALESCE(query_feedback_reason,'') LIKE '%没有%'
               OR COALESCE(query_feedback_reason,'') LIKE '%未%'
               OR COALESCE(query_feedback_reason,'') LIKE '%查询无明细%'
               OR COALESCE(query_feedback_reason,'') LIKE '%找不到客户%'
               OR COALESCE(query_feedback_reason,'') LIKE '%核心查无记录%'
             )
             AND COALESCE(clean_failed,0)<>1""",
        tuple(context.scope_state.txn_params),
    )
    reversal_changed = cleaning_sql_counter.update_count(
        cur,
        f"""UPDATE fc_transaction_norm
           SET clean_reversal=1
           WHERE {context.scope_state.txn_where}
             AND (
               COALESCE(query_feedback_reason,'') LIKE '%冲正%'
               OR (
                 {source_amount_negative_expr}
                 AND {reversal_detail_expr}
               )
             )
             AND COALESCE(clean_reversal,0)<>1""",
        tuple(context.scope_state.txn_params),
    )
    cur.execute("SELECT COUNT(1) FROM fc_transaction_norm WHERE case_id=? AND clean_failed=1", (context.case_id,))
    failed_total = cleaning_sql_counter.require_count_row(
        cur.fetchone(),
        code="cleaning_failed_total_unavailable",
    )
    cur.execute("SELECT COUNT(1) FROM fc_transaction_norm WHERE case_id=? AND clean_reversal=1", (context.case_id,))
    reversal_total = cleaning_sql_counter.require_count_row(
        cur.fetchone(),
        code="cleaning_reversal_total_unavailable",
    )
    return (
        failed_changed,
        reversal_changed,
        failed_total,
        reversal_total,
    )


def step_dc_flag(context: CleaningStepContext, cur: Any) -> Tuple[int, int, int, int]:
    context.check_cancel()
    normalized = cleaning_sql_counter.update_count(
        cur,
        f"""UPDATE fc_transaction_norm
           SET orig_dc_flag=COALESCE(NULLIF(orig_dc_flag,''), dc_flag),
               clean_dc_flag=COALESCE(NULLIF(clean_dc_flag,''), dc_norm),
               clean_dc_normalized=CASE
                 WHEN dc_norm IS NOT NULL THEN 1
                 ELSE clean_dc_normalized END
           WHERE {context.scope_state.txn_where}
             AND COALESCE(TRIM(clean_dc_flag),'')=''
             AND dc_norm IS NOT NULL""",
        tuple(context.scope_state.txn_params),
    )

    acct_key_expr = cleaning_sql_expr.account_key_expr()
    amt_val = "COALESCE(TRY_CAST(clean_amount AS DOUBLE), amount_val, TRY_CAST(amount AS DOUBLE))"
    bal_val = "COALESCE(balance_val, TRY_CAST(clean_balance AS DOUBLE), TRY_CAST(balance AS DOUBLE))"
    txn_ts = f"COALESCE(txn_ts, {_ts_norm_expr('txn_time')})"
    order_by = f"CASE WHEN txn_ts IS NULL THEN 1 ELSE 0 END, txn_ts, row_no, txn_time, id"

    inferred = cleaning_sql_counter.update_count(
        cur,
        f"""WITH scope AS (
               SELECT id,
                      {txn_ts} AS txn_ts,
                      txn_time,
                      {amt_val} AS amt_val,
                      {bal_val} AS bal_val,
                      {acct_key_expr} AS acct_key,
                      row_no
               FROM fc_transaction_norm
               WHERE {context.scope_state.txn_where}
                 AND COALESCE(TRIM(clean_dc_flag),'')=''
           ),
           affected AS (
               SELECT DISTINCT acct_key
               FROM scope
               WHERE acct_key IS NOT NULL AND acct_key<>''
           ),
           seed AS (
               SELECT acct_key,
                      last_txn_ts AS txn_ts,
                      last_balance AS bal_val,
                      NULL::DOUBLE AS amt_val,
                      -1 AS row_no,
                      NULL AS txn_time
               FROM acct_state
               WHERE case_id=? AND acct_key IN (SELECT acct_key FROM affected)
           ),
           base AS (
               SELECT id, acct_key, txn_ts, txn_time, amt_val, bal_val, row_no
               FROM scope
               WHERE acct_key IS NOT NULL AND acct_key<>''
           ),
           merged AS (
               SELECT NULL AS id, acct_key, txn_ts, txn_time, amt_val, bal_val, row_no FROM seed
               UNION ALL
               SELECT id, acct_key, txn_ts, txn_time, amt_val, bal_val, row_no FROM base
           ),
           ordered AS (
               SELECT *,
                      LAG(bal_val) OVER (PARTITION BY acct_key ORDER BY {order_by}) AS prev_bal
               FROM merged
           ),
           calc AS (
               SELECT id,
                      CASE
                         WHEN prev_bal IS NULL THEN
                           CASE WHEN amt_val IS NOT NULL AND amt_val < 0 THEN '出' ELSE NULL END
                         WHEN bal_val IS NULL THEN NULL
                         ELSE
                           CASE
                             WHEN amt_val IS NOT NULL THEN
                               CASE
                                 WHEN abs((bal_val - prev_bal) - amt_val)
                                      <= greatest(0.01, abs(amt_val) * 0.01) THEN '进'
                                 WHEN abs((bal_val - prev_bal) + amt_val)
                                      <= greatest(0.01, abs(amt_val) * 0.01) THEN '出'
                                 WHEN (bal_val - prev_bal) > 0 THEN '进'
                                 WHEN (bal_val - prev_bal) < 0 THEN '出'
                                 ELSE NULL
                               END
                             ELSE
                               CASE
                                 WHEN (bal_val - prev_bal) > 0 THEN '进'
                                 WHEN (bal_val - prev_bal) < 0 THEN '出'
                                 ELSE NULL
                               END
                           END
                      END AS inferred_flag
               FROM ordered
               WHERE id IS NOT NULL
           )
           UPDATE fc_transaction_norm AS t
              SET clean_dc_flag=calc.inferred_flag,
                  clean_dc_inferred=1,
                  orig_dc_flag=COALESCE(NULLIF(t.orig_dc_flag,''), t.dc_flag),
                  dc_final=COALESCE(calc.inferred_flag, t.dc_final)
             FROM calc
            WHERE t.id=calc.id AND calc.inferred_flag IS NOT NULL""",
        (context.case_id,),
    )

    try:
        cur.execute(
            f"UPDATE fc_transaction_norm SET dc_final=COALESCE(NULLIF(clean_dc_flag,''), dc_norm) "
            f"WHERE {context.scope_state.txn_where}",
            tuple(context.scope_state.txn_params),
        )
    except Exception:
        pass

    cur.execute(
        "SELECT COUNT(1) FROM fc_transaction_norm WHERE case_id=? AND clean_dc_normalized=1",
        (context.case_id,),
    )
    normalized_total = cleaning_sql_counter.require_count_row(
        cur.fetchone(),
        code="cleaning_dc_normalized_total_unavailable",
    )
    cur.execute(
        "SELECT COUNT(1) FROM fc_transaction_norm WHERE case_id=? AND clean_dc_inferred=1",
        (context.case_id,),
    )
    inferred_total = cleaning_sql_counter.require_count_row(
        cur.fetchone(),
        code="cleaning_dc_inferred_total_unavailable",
    )
    return (
        normalized,
        inferred,
        normalized_total,
        inferred_total,
    )


__all__ = [
    "step_amount_balance",
    "step_dc_flag",
    "step_duplicate",
    "step_failed_reversal",
    "step_invalid",
]
