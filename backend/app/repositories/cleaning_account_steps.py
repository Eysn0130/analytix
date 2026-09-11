from __future__ import annotations

from typing import Any, Tuple

from app.repositories import cleaning_duckdb_meta, cleaning_sql_counter
from app.repositories.cleaning_step_context import CleaningStepContext


def step_fill_card(context: CleaningStepContext, cur: Any) -> Tuple[int, int]:
    context.check_cancel()
    delta = 0
    delta += cleaning_sql_counter.update_count(
        cur,
        f"""UPDATE fc_transaction_norm
           SET orig_card_no=COALESCE(NULLIF(orig_card_no,''), card_no),
               clean_card_no=COALESCE(NULLIF(clean_card_no,''), acct_no),
               clean_card_filled=1
           WHERE {context.scope_state.txn_where}
             AND COALESCE(TRIM(card_no),'')=''
             AND COALESCE(TRIM(clean_card_no),'')=''
             AND COALESCE(TRIM(acct_no),'')<>''""",
        tuple(context.scope_state.txn_params),
    )
    delta += cleaning_sql_counter.update_count(
        cur,
        f"""UPDATE fc_transaction_norm
           SET clean_acct_no=COALESCE(NULLIF(clean_acct_no,''), card_no),
               clean_card_filled=1
           WHERE {context.scope_state.txn_where}
             AND COALESCE(TRIM(acct_no),'')=''
             AND COALESCE(TRIM(clean_acct_no),'')=''
             AND COALESCE(TRIM(card_no),'')<>''""",
        tuple(context.scope_state.txn_params),
    )
    cur.execute("SELECT COUNT(1) FROM fc_transaction_norm WHERE case_id=? AND clean_card_filled=1", (context.case_id,))
    total = cleaning_sql_counter.require_count_row(
        cur.fetchone(),
        code="cleaning_fill_card_total_unavailable",
    )
    return delta, total


def step_suffix_transaction(context: CleaningStepContext, cur: Any) -> Tuple[int, int]:
    context.check_cancel()
    src_card = "COALESCE(NULLIF(clean_card_no,''), card_no)"
    src_acct = "COALESCE(NULLIF(clean_acct_no,''), acct_no)"
    card_base = (
        f"CASE WHEN instr({src_card}, '-') > 0 OR instr({src_card}, '_') > 0 THEN "
        f"  CASE WHEN (instr({src_card}, '_') > 0 AND (instr({src_card}, '-') = 0 OR instr({src_card}, '_') < instr({src_card}, '-'))) THEN "
        f"    CASE WHEN instr({src_card}, '_') > 1 THEN substr({src_card}, 1, instr({src_card}, '_') - 1) ELSE NULL END "
        f"  ELSE "
        f"    CASE WHEN instr({src_card}, '-') > 1 THEN substr({src_card}, 1, instr({src_card}, '-') - 1) ELSE NULL END "
        f"  END "
        f"ELSE NULL END"
    )
    acct_base = (
        f"CASE WHEN instr({src_acct}, '-') > 0 OR instr({src_acct}, '_') > 0 THEN "
        f"  CASE WHEN (instr({src_acct}, '_') > 0 AND (instr({src_acct}, '-') = 0 OR instr({src_acct}, '_') < instr({src_acct}, '-'))) THEN "
        f"    CASE WHEN instr({src_acct}, '_') > 1 THEN substr({src_acct}, 1, instr({src_acct}, '_') - 1) ELSE NULL END "
        f"  ELSE "
        f"    CASE WHEN instr({src_acct}, '-') > 1 THEN substr({src_acct}, 1, instr({src_acct}, '-') - 1) ELSE NULL END "
        f"  END "
        f"ELSE NULL END"
    )
    changed = cleaning_sql_counter.update_count(
        cur,
        f"""UPDATE fc_transaction_norm
               SET clean_card_no=COALESCE({card_base}, clean_card_no),
                   clean_acct_no=COALESCE({acct_base}, clean_acct_no),
                   clean_suffix_fixed=CASE
                     WHEN ({card_base}) IS NOT NULL OR ({acct_base}) IS NOT NULL THEN 1
                     ELSE clean_suffix_fixed
                   END
             WHERE {context.scope_state.txn_where}
               AND (
                 instr({src_card}, '-') > 1 OR instr({src_card}, '_') > 1
                 OR instr({src_acct}, '-') > 1 OR instr({src_acct}, '_') > 1
               )
               AND (
                 (({card_base}) IS NOT NULL
                   AND (clean_card_no IS DISTINCT FROM ({card_base}) OR clean_suffix_fixed IS DISTINCT FROM 1))
                 OR (({acct_base}) IS NOT NULL
                   AND (clean_acct_no IS DISTINCT FROM ({acct_base}) OR clean_suffix_fixed IS DISTINCT FROM 1))
               )""",
        tuple(context.scope_state.txn_params),
    )
    cur.execute("SELECT COUNT(1) FROM fc_transaction_norm WHERE case_id=? AND clean_suffix_fixed=1", (context.case_id,))
    return changed, cleaning_sql_counter.require_count_row(
        cur.fetchone(),
        code="cleaning_transaction_suffix_total_unavailable",
    )


def step_account_invalid(context: CleaningStepContext, cur: Any) -> Tuple[int, int]:
    if not cleaning_duckdb_meta.table_exists(cur, "fc_account_norm"):
        raise cleaning_sql_counter.CleaningCountUnavailableError(
            "cleaning_account_source_unavailable"
        )
    context.check_cancel()
    changed = cleaning_sql_counter.update_count(
        cur,
        f"""UPDATE fc_account_norm
           SET clean_acct_invalid=1
           WHERE {context.scope_state.acc_where}
             AND COALESCE(clean_acct_invalid, 0)=0
             AND (COALESCE(TRIM(card_no),'')='' OR TRIM(card_no) IN ('_', '-'))
             AND (COALESCE(TRIM(acct_no),'')='' OR TRIM(acct_no) IN ('_', '-'))""",
        tuple(context.scope_state.acc_params),
    )
    cur.execute("SELECT COUNT(1) FROM fc_account_norm WHERE case_id=? AND clean_acct_invalid=1", (context.case_id,))
    return changed, cleaning_sql_counter.require_count_row(
        cur.fetchone(),
        code="cleaning_account_invalid_total_unavailable",
    )


def step_suffix_account(context: CleaningStepContext, cur: Any) -> Tuple[int, int]:
    if not cleaning_duckdb_meta.table_exists(cur, "fc_account_norm"):
        raise cleaning_sql_counter.CleaningCountUnavailableError(
            "cleaning_account_source_unavailable"
        )
    context.check_cancel()
    src_card = "COALESCE(NULLIF(clean_card_no,''), card_no)"
    src_acct = "COALESCE(NULLIF(clean_acct_no,''), acct_no)"
    card_base = (
        f"CASE WHEN instr({src_card}, '-') > 0 OR instr({src_card}, '_') > 0 THEN "
        f"  CASE WHEN (instr({src_card}, '_') > 0 AND (instr({src_card}, '-') = 0 OR instr({src_card}, '_') < instr({src_card}, '-'))) THEN "
        f"    CASE WHEN instr({src_card}, '_') > 1 THEN substr({src_card}, 1, instr({src_card}, '_') - 1) ELSE NULL END "
        f"  ELSE "
        f"    CASE WHEN instr({src_card}, '-') > 1 THEN substr({src_card}, 1, instr({src_card}, '-') - 1) ELSE NULL END "
        f"  END "
        f"ELSE NULL END"
    )
    acct_base = (
        f"CASE WHEN instr({src_acct}, '-') > 0 OR instr({src_acct}, '_') > 0 THEN "
        f"  CASE WHEN (instr({src_acct}, '_') > 0 AND (instr({src_acct}, '-') = 0 OR instr({src_acct}, '_') < instr({src_acct}, '-'))) THEN "
        f"    CASE WHEN instr({src_acct}, '_') > 1 THEN substr({src_acct}, 1, instr({src_acct}, '_') - 1) ELSE NULL END "
        f"  ELSE "
        f"    CASE WHEN instr({src_acct}, '-') > 1 THEN substr({src_acct}, 1, instr({src_acct}, '-') - 1) ELSE NULL END "
        f"  END "
        f"ELSE NULL END"
    )
    changed = cleaning_sql_counter.update_count(
        cur,
        f"""UPDATE fc_account_norm
               SET clean_card_no=COALESCE({card_base}, clean_card_no),
                   clean_acct_no=COALESCE({acct_base}, clean_acct_no),
                   clean_suffix_fixed=CASE
                     WHEN ({card_base}) IS NOT NULL OR ({acct_base}) IS NOT NULL THEN 1
                     ELSE clean_suffix_fixed
                   END
             WHERE {context.scope_state.acc_where}
               AND (
                 instr({src_card}, '-') > 1 OR instr({src_card}, '_') > 1
                 OR instr({src_acct}, '-') > 1 OR instr({src_acct}, '_') > 1
               )
               AND (
                 (({card_base}) IS NOT NULL
                   AND (clean_card_no IS DISTINCT FROM ({card_base}) OR clean_suffix_fixed IS DISTINCT FROM 1))
                 OR (({acct_base}) IS NOT NULL
                   AND (clean_acct_no IS DISTINCT FROM ({acct_base}) OR clean_suffix_fixed IS DISTINCT FROM 1))
               )""",
        tuple(context.scope_state.acc_params),
    )
    cur.execute("SELECT COUNT(1) FROM fc_account_norm WHERE case_id=? AND clean_suffix_fixed=1", (context.case_id,))
    return changed, cleaning_sql_counter.require_count_row(
        cur.fetchone(),
        code="cleaning_account_suffix_total_unavailable",
    )


def step_fill_account_info(context: CleaningStepContext, cur: Any) -> Tuple[int, int]:
    if not cleaning_duckdb_meta.table_exists(cur, "fc_account_norm"):
        raise cleaning_sql_counter.CleaningCountUnavailableError(
            "cleaning_account_source_unavailable"
        )
    context.check_cancel()
    t_card_col = "COALESCE(NULLIF(clean_card_no,''), card_no)"
    t_acct_col = "COALESCE(NULLIF(clean_acct_no,''), acct_no)"
    t_card = "COALESCE(NULLIF(t.clean_card_no,''), t.card_no)"
    t_acct = "COALESCE(NULLIF(t.clean_acct_no,''), t.acct_no)"
    a_card = "COALESCE(NULLIF(a.clean_card_no,''), a.card_no)"
    a_acct = "COALESCE(NULLIF(a.clean_acct_no,''), a.acct_no)"
    name_val = "NULLIF(TRIM(a.account_open_name),'')"
    id_val = "NULLIF(TRIM(a.opener_id_no),'')"
    t_name_val = "NULLIF(TRIM(account_open_name),'')"
    t_id_val = "NULLIF(TRIM(opener_id_no),'')"
    needs_fill = "COALESCE(TRIM(t.account_open_name),'')='' OR COALESCE(TRIM(t.opener_id_no),'')=''"

    def _valid(expr: str) -> str:
        return f"COALESCE(TRIM({expr}),'') NOT IN ('','-','—','_')"

    def _run(match_cond: str) -> int:
        sql = (
            "UPDATE fc_transaction_norm AS t SET "
            "account_open_name=CASE "
            f"  WHEN COALESCE(TRIM(t.account_open_name),'')='' AND {name_val} IS NOT NULL "
            "  THEN a.account_open_name ELSE t.account_open_name END, "
            "opener_id_no=CASE "
            f"  WHEN COALESCE(TRIM(t.opener_id_no),'')='' AND {id_val} IS NOT NULL "
            "  THEN a.opener_id_no ELSE t.opener_id_no END, "
            "clean_account_filled=CASE "
            f"  WHEN (COALESCE(TRIM(t.account_open_name),'')='' AND {name_val} IS NOT NULL) "
            f"    OR (COALESCE(TRIM(t.opener_id_no),'')='' AND {id_val} IS NOT NULL) "
            "  THEN 1 ELSE t.clean_account_filled END "
            "FROM fc_account_norm AS a "
            f"WHERE {context.scope_state.txn_where_t} AND {context.scope_state.acc_where_a} "
            f"  AND ({needs_fill}) AND ({match_cond}) "
            f"  AND ({name_val} IS NOT NULL OR {id_val} IS NOT NULL) "
            "  AND ("
            f"    (COALESCE(TRIM(t.account_open_name),'')='' AND {name_val} IS NOT NULL "
            "      AND t.account_open_name IS DISTINCT FROM a.account_open_name) "
            f"    OR (COALESCE(TRIM(t.opener_id_no),'')='' AND {id_val} IS NOT NULL "
            "      AND t.opener_id_no IS DISTINCT FROM a.opener_id_no) "
            "    OR (t.clean_account_filled IS DISTINCT FROM 1 AND ("
            f"      (COALESCE(TRIM(t.account_open_name),'')='' AND {name_val} IS NOT NULL) "
            f"      OR (COALESCE(TRIM(t.opener_id_no),'')='' AND {id_val} IS NOT NULL)"
            "    ))"
            "  )"
        )
        params = tuple([*context.scope_state.txn_params, *context.scope_state.acc_params])
        return cleaning_sql_counter.update_count(cur, sql, params)

    def _run_card_propagate() -> int:
        valid_card = _valid(t_card)
        valid_card_base = _valid(t_card_col)
        sql = f"""
            WITH card_map AS (
                SELECT {t_card_col} AS card_key,
                       MIN({t_name_val}) AS name_val,
                       MIN({t_id_val}) AS id_val,
                       COUNT(DISTINCT {t_name_val}) AS name_cnt,
                       COUNT(DISTINCT {t_id_val}) AS id_cnt
                FROM fc_transaction_norm
                WHERE {context.scope_state.txn_where}
                  AND {valid_card_base}
                  AND ({t_name_val} IS NOT NULL OR {t_id_val} IS NOT NULL)
                GROUP BY card_key
                HAVING COUNT(DISTINCT {t_name_val})=1 OR COUNT(DISTINCT {t_id_val})=1
            )
            UPDATE fc_transaction_norm AS t
            SET account_open_name=CASE
                    WHEN COALESCE(TRIM(t.account_open_name),'')='' AND m.name_cnt=1 AND m.name_val IS NOT NULL
                    THEN m.name_val ELSE t.account_open_name END,
                opener_id_no=CASE
                    WHEN COALESCE(TRIM(t.opener_id_no),'')='' AND m.id_cnt=1 AND m.id_val IS NOT NULL
                    THEN m.id_val ELSE t.opener_id_no END,
                clean_account_filled=CASE
                    WHEN (COALESCE(TRIM(t.account_open_name),'')='' AND m.name_cnt=1 AND m.name_val IS NOT NULL)
                      OR (COALESCE(TRIM(t.opener_id_no),'')='' AND m.id_cnt=1 AND m.id_val IS NOT NULL)
                    THEN 1 ELSE t.clean_account_filled END
            FROM card_map AS m
            WHERE {context.scope_state.txn_where_t}
              AND ({needs_fill})
              AND {valid_card}
              AND {t_card}=m.card_key
              AND (
                (COALESCE(TRIM(t.account_open_name),'')='' AND m.name_cnt=1 AND m.name_val IS NOT NULL
                  AND t.account_open_name IS DISTINCT FROM m.name_val)
                OR (COALESCE(TRIM(t.opener_id_no),'')='' AND m.id_cnt=1 AND m.id_val IS NOT NULL
                  AND t.opener_id_no IS DISTINCT FROM m.id_val)
                OR (t.clean_account_filled IS DISTINCT FROM 1 AND (
                  (COALESCE(TRIM(t.account_open_name),'')='' AND m.name_cnt=1 AND m.name_val IS NOT NULL)
                  OR (COALESCE(TRIM(t.opener_id_no),'')='' AND m.id_cnt=1 AND m.id_val IS NOT NULL)
                ))
              )
        """
        params = tuple([*context.scope_state.txn_params, *context.scope_state.txn_params])
        return cleaning_sql_counter.update_count(cur, sql, params)

    def _run_counterparty_name_fill() -> int:
        valid_card = _valid(t_card)
        cp_acct = "TRIM(COALESCE(NULLIF(counterparty_acct_norm,''), counterparty_acct))"
        cp_name = "NULLIF(TRIM(counterparty_name),'')"
        cp_name_valid = "COALESCE(TRIM(counterparty_name),'') NOT IN ('','-','—','_')"
        sql = f"""
            WITH name_stats AS (
                SELECT {cp_acct} AS acct_key, {cp_name} AS cp_name, COUNT(1) AS cnt
                FROM fc_transaction_norm
                WHERE {context.scope_state.txn_where}
                  AND COALESCE({cp_acct},'') NOT IN ('','-','—','_')
                  AND {cp_name} IS NOT NULL
                  AND {cp_name_valid}
                GROUP BY acct_key, cp_name
            ),
            ranked AS (
                SELECT acct_key, cp_name,
                       ROW_NUMBER() OVER (PARTITION BY acct_key ORDER BY cnt DESC, cp_name ASC) AS rn
                FROM name_stats
            )
            UPDATE fc_transaction_norm AS t
            SET account_open_name=CASE
                    WHEN COALESCE(TRIM(t.account_open_name),'')='' AND r.cp_name IS NOT NULL
                    THEN r.cp_name ELSE t.account_open_name END,
                clean_account_filled=CASE
                    WHEN COALESCE(TRIM(t.account_open_name),'')='' AND r.cp_name IS NOT NULL
                    THEN 1 ELSE t.clean_account_filled END
            FROM ranked AS r
            WHERE {context.scope_state.txn_where_t}
              AND ({needs_fill})
              AND {valid_card}
              AND {t_card}=r.acct_key
              AND r.rn=1
              AND COALESCE(TRIM(t.account_open_name),'')=''
              AND (
                t.account_open_name IS DISTINCT FROM r.cp_name
                OR t.clean_account_filled IS DISTINCT FROM 1
              )
        """
        params = tuple([*context.scope_state.txn_params, *context.scope_state.txn_params])
        return cleaning_sql_counter.update_count(cur, sql, params)

    def _run_card_prefix_match() -> int:
        t_card_digits = f"NULLIF(regexp_extract({t_card}, '^[0-9]+', 0), '')"
        has_alpha = f"regexp_matches(COALESCE({t_card}, ''), '.*[A-Za-z].*')"
        return _run(f"{has_alpha} AND {t_card_digits} IS NOT NULL AND {_valid(a_card)} AND {t_card_digits}={a_card}")

    def _run_acct_to_card_fill() -> int:
        valid_card = _valid(t_card)
        acct_key = f"TRIM({t_acct_col})"
        name_ok = "COALESCE(TRIM(account_open_name),'') NOT IN ('','-','—','_')"
        id_ok = "COALESCE(TRIM(opener_id_no),'') NOT IN ('','-','—','_')"
        sql = f"""
            WITH name_stats AS (
                SELECT {acct_key} AS acct_key,
                       NULLIF(TRIM(account_open_name),'') AS name_val,
                       COUNT(1) AS cnt
                FROM fc_transaction_norm
                WHERE {context.scope_state.txn_where}
                  AND {acct_key} NOT IN ('','-','—','_')
                  AND {name_ok}
                GROUP BY acct_key, name_val
            ),
            name_ranked AS (
                SELECT acct_key, name_val,
                       ROW_NUMBER() OVER (PARTITION BY acct_key ORDER BY cnt DESC, name_val ASC) AS rn
                FROM name_stats
            ),
            id_stats AS (
                SELECT {acct_key} AS acct_key,
                       NULLIF(TRIM(opener_id_no),'') AS id_val,
                       COUNT(1) AS cnt
                FROM fc_transaction_norm
                WHERE {context.scope_state.txn_where}
                  AND {acct_key} NOT IN ('','-','—','_')
                  AND {id_ok}
                GROUP BY acct_key, id_val
            ),
            id_ranked AS (
                SELECT acct_key, id_val,
                       ROW_NUMBER() OVER (PARTITION BY acct_key ORDER BY cnt DESC, id_val ASC) AS rn
                FROM id_stats
            ),
            keys AS (
                SELECT acct_key FROM name_ranked WHERE rn=1
                UNION
                SELECT acct_key FROM id_ranked WHERE rn=1
            ),
            top_vals AS (
                SELECT k.acct_key, n.name_val, i.id_val
                FROM keys k
                LEFT JOIN name_ranked n ON n.acct_key=k.acct_key AND n.rn=1
                LEFT JOIN id_ranked i ON i.acct_key=k.acct_key AND i.rn=1
            )
            UPDATE fc_transaction_norm AS t
            SET account_open_name=CASE
                    WHEN COALESCE(TRIM(t.account_open_name),'')='' AND v.name_val IS NOT NULL
                    THEN v.name_val ELSE t.account_open_name END,
                opener_id_no=CASE
                    WHEN COALESCE(TRIM(t.opener_id_no),'')='' AND v.id_val IS NOT NULL
                    THEN v.id_val ELSE t.opener_id_no END,
                clean_account_filled=CASE
                    WHEN (COALESCE(TRIM(t.account_open_name),'')='' AND v.name_val IS NOT NULL)
                      OR (COALESCE(TRIM(t.opener_id_no),'')='' AND v.id_val IS NOT NULL)
                    THEN 1 ELSE t.clean_account_filled END
            FROM top_vals AS v
            WHERE {context.scope_state.txn_where_t}
              AND ({needs_fill})
              AND {valid_card}
              AND TRIM({t_card})=v.acct_key
              AND (v.name_val IS NOT NULL OR v.id_val IS NOT NULL)
              AND (
                (COALESCE(TRIM(t.account_open_name),'')='' AND v.name_val IS NOT NULL
                  AND t.account_open_name IS DISTINCT FROM v.name_val)
                OR (COALESCE(TRIM(t.opener_id_no),'')='' AND v.id_val IS NOT NULL
                  AND t.opener_id_no IS DISTINCT FROM v.id_val)
                OR (t.clean_account_filled IS DISTINCT FROM 1 AND (
                  (COALESCE(TRIM(t.account_open_name),'')='' AND v.name_val IS NOT NULL)
                  OR (COALESCE(TRIM(t.opener_id_no),'')='' AND v.id_val IS NOT NULL)
                ))
              )
        """
        params = tuple([*context.scope_state.txn_params, *context.scope_state.txn_params, *context.scope_state.txn_params])
        return cleaning_sql_counter.update_count(cur, sql, params)

    updated = 0
    updated += _run(f"{_valid(t_card)} AND {_valid(a_card)} AND {t_card}={a_card}")
    updated += _run(f"{_valid(t_acct)} AND {_valid(a_acct)} AND {t_acct}={a_acct}")
    updated += _run(f"{_valid(t_card)} AND {_valid(a_acct)} AND {t_card}={a_acct}")
    updated += _run_card_propagate()
    updated += _run_counterparty_name_fill()
    updated += _run_card_prefix_match()
    updated += _run_acct_to_card_fill()

    cur.execute("SELECT COUNT(1) FROM fc_transaction_norm WHERE case_id=? AND clean_account_filled=1", (context.case_id,))
    total = cleaning_sql_counter.require_count_row(
        cur.fetchone(),
        code="cleaning_account_fill_total_unavailable",
    )
    return updated, total


__all__ = [
    "step_account_invalid",
    "step_fill_account_info",
    "step_fill_card",
    "step_suffix_account",
    "step_suffix_transaction",
]
