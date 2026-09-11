from __future__ import annotations

from dataclasses import dataclass
from typing import Optional, Sequence

from app.core.fc_import_norm_insert import ts_norm_expr as _ts_norm_expr


def _finite_double_or_null_expr(expr: str) -> str:
    cast_expr = f"TRY_CAST({expr} AS DOUBLE)"
    return (
        f"CASE WHEN {cast_expr} IS NOT NULL AND isfinite({cast_expr}) "
        f"THEN {cast_expr} ELSE NULL END"
    )


_CLEAN_AMOUNT_VALUE_EXPR = "COALESCE(NULLIF(clean_amount,''), amount)"
_CLEAN_AMOUNT_FINITE_EXPR = _finite_double_or_null_expr(_CLEAN_AMOUNT_VALUE_EXPR)
_T_CLEAN_AMOUNT_FINITE_EXPR = _finite_double_or_null_expr("NULLIF(t.clean_amount,'')")
_T_EFFECTIVE_AMOUNT_FINITE_EXPR = _finite_double_or_null_expr(
    "COALESCE(NULLIF(t.clean_amount,''), t.amount)"
)
_T_CLEAN_BALANCE_FINITE_EXPR = _finite_double_or_null_expr("NULLIF(t.clean_balance,'')")
_T_EFFECTIVE_BALANCE_FINITE_EXPR = _finite_double_or_null_expr(
    "COALESCE(NULLIF(t.clean_balance,''), t.balance)"
)


@dataclass(frozen=True)
class CleaningStepDefinition:
    step: int
    key: str
    title: str
    kind: str
    description: str
    headers: Sequence[str]
    sql: str
    count_sql: str
    empty_placeholder: Optional[str] = None


CLEANING_STEP_DEFINITIONS: tuple[CleaningStepDefinition, ...] = (
    CleaningStepDefinition(
        step=1,
        key="step1",
        title="金额/余额数值化",
        kind="处理",
        description="识别并修正交易金额、余额字段中的非标准数值。",
        headers=(
            "交易卡号",
            "交易账号",
            "交易时间",
            "交易金额（原）",
            "交易金额（改）",
            "交易余额（原）",
            "交易余额（改）",
            "收付标志",
            "交易对手账号",
            "对手户名",
        ),
        sql=(
            "SELECT t.card_no, t.acct_no, t.txn_time, "
            "COALESCE(NULLIF(r.amount_raw,''), COALESCE(NULLIF(t.orig_amount,''), t.amount)), "
            "CASE "
            "  WHEN (t.clean_amt_failed=1 OR t.clean_amt_fixed=1) "
            f"    AND {_T_EFFECTIVE_AMOUNT_FINITE_EXPR} IS NULL "
            "  THEN '__RED__转换失败' "
            "  WHEN t.clean_amt_fixed=1 "
            f"    AND {_T_CLEAN_AMOUNT_FINITE_EXPR} IS NOT NULL "
            "  THEN '__GREEN__' || COALESCE(NULLIF(t.clean_amount,''), t.amount) "
            "  ELSE COALESCE(NULLIF(t.clean_amount,''), t.amount) END, "
            "COALESCE(NULLIF(r.balance_raw,''), COALESCE(NULLIF(t.orig_balance,''), t.balance)), "
            "CASE "
            "  WHEN (t.clean_bal_failed=1 OR t.clean_bal_fixed=1) "
            f"    AND {_T_EFFECTIVE_BALANCE_FINITE_EXPR} IS NULL "
            "  THEN '__RED__转换失败' "
            "  WHEN t.clean_bal_fixed=1 "
            f"    AND {_T_CLEAN_BALANCE_FINITE_EXPR} IS NOT NULL "
            "  THEN '__GREEN__' || COALESCE(NULLIF(t.clean_balance,''), t.balance) "
            "  ELSE COALESCE(NULLIF(t.clean_balance,''), t.balance) END, "
            "COALESCE(NULLIF(t.clean_dc_flag,''), t.dc_flag), "
            "t.counterparty_acct, t.counterparty_name "
            "FROM fc_transaction_norm t "
            "LEFT JOIN fc_transaction_raw r "
            "  ON r.case_id=t.case_id AND r.file_id=t.file_id AND r.row_no=t.row_no "
            "WHERE t.case_id=? AND (t.clean_amt_fixed=1 OR t.clean_amt_failed=1 OR "
            "t.clean_bal_fixed=1 OR t.clean_bal_failed=1) "
            "ORDER BY t.id LIMIT ? OFFSET ?"
        ),
        count_sql=(
            "SELECT COUNT(1) FROM fc_transaction_norm WHERE case_id=? AND "
            "(clean_amt_fixed=1 OR clean_amt_failed=1 OR clean_bal_fixed=1 OR clean_bal_failed=1)"
        ),
    ),
    CleaningStepDefinition(
        step=2,
        key="step2",
        title="无效数据标记",
        kind="标记",
        description="标记时间、金额或关键识别字段异常的交易记录。",
        headers=(
            "交易卡号",
            "交易账号",
            "交易时间",
            "交易金额",
            "交易对手账号",
            "对手户名",
            "查询反馈结果原因",
            "无效数据",
        ),
        sql=(
            "SELECT card_no, acct_no, txn_time, "
            "COALESCE(NULLIF(clean_amount,''), amount), "
            "counterparty_acct, counterparty_name, query_feedback_reason, "
            "CASE WHEN clean_invalid=1 THEN '__RED__无效数据' ELSE '' END "
            "FROM fc_transaction_norm WHERE case_id=? AND clean_invalid=1 "
            "ORDER BY id LIMIT ? OFFSET ?"
        ),
        count_sql="SELECT COUNT(1) FROM fc_transaction_norm WHERE case_id=? AND clean_invalid=1",
        empty_placeholder="[空]",
    ),
    CleaningStepDefinition(
        step=3,
        key="step3",
        title="重复数据标记",
        kind="停用",
        description="保留旧版重复标记语义，当前用于审计重复候选记录。",
        headers=(
            "交易卡号",
            "交易账号",
            "交易时间",
            "交易金额",
            "收付标志",
            "交易对手账号",
            "对手户名",
            "重复数据",
        ),
        sql=(
            "SELECT card_no, acct_no, txn_time, "
            "COALESCE(NULLIF(clean_amount,''), amount), "
            "COALESCE(NULLIF(clean_dc_flag,''), dc_flag), "
            "counterparty_acct, counterparty_name, '重复数据' "
            "FROM fc_transaction_norm WHERE case_id=? AND clean_duplicate=1 "
            "ORDER BY id LIMIT ? OFFSET ?"
        ),
        count_sql="SELECT COUNT(1) FROM fc_transaction_norm WHERE case_id=? AND clean_duplicate=1",
    ),
    CleaningStepDefinition(
        step=4,
        key="step4",
        title="交易失败/冲正",
        kind="标记",
        description="标记失败交易与冲正流水，供后续审计与过滤。",
        headers=(
            "交易卡号",
            "交易账号",
            "交易时间",
            "交易金额",
            "收付标志",
            "交易对手账号",
            "对手户名",
            "查询反馈结果原因",
            "失败标记",
            "冲正标记",
        ),
        sql=(
            "SELECT card_no, acct_no, txn_time, "
            "COALESCE(NULLIF(clean_amount,''), amount), "
            "COALESCE(NULLIF(clean_dc_flag,''), dc_flag), "
            "counterparty_acct, counterparty_name, query_feedback_reason, "
            "CASE WHEN clean_failed=1 THEN '__RED__失败' ELSE '' END, "
            "CASE WHEN clean_reversal=1 THEN '__RED__冲正' ELSE '' END "
            "FROM fc_transaction_norm WHERE case_id=? AND (clean_failed=1 OR clean_reversal=1) "
            "ORDER BY id LIMIT ? OFFSET ?"
        ),
        count_sql=(
            "SELECT COUNT(1) FROM fc_transaction_norm WHERE case_id=? AND (clean_failed=1 OR clean_reversal=1)"
        ),
    ),
    CleaningStepDefinition(
        step=5,
        key="step5",
        title="收付标志修正",
        kind="修正",
        description="规范收付方向字段，并尝试补全缺失的进出标志。",
        headers=(
            "交易卡号",
            "交易账号",
            "交易时间",
            "交易金额",
            "收付标志（原）",
            "收付标志（改）",
            "交易对手账号",
            "对手户名",
        ),
        sql=(
            "SELECT card_no, acct_no, txn_time, "
            "COALESCE(NULLIF(clean_amount,''), amount), "
            "COALESCE(NULLIF(orig_dc_flag,''), dc_flag), "
            "CASE "
            "  WHEN COALESCE(NULLIF(clean_dc_flag,''), '') IN ('进','出') THEN "
            "    '__GREEN__' || COALESCE(NULLIF(clean_dc_flag,''), '') "
            "  ELSE '__RED__匹配失败' END, "
            "counterparty_acct, counterparty_name "
            "FROM fc_transaction_norm WHERE case_id=? "
            "AND (COALESCE(TRIM(COALESCE(NULLIF(orig_dc_flag,''), dc_flag)),'')='' "
            "     OR COALESCE(NULLIF(orig_dc_flag,''), dc_flag) NOT IN ('进','出')) "
            "AND (clean_dc_normalized=1 OR clean_dc_inferred=1 "
            "     OR COALESCE(TRIM(COALESCE(NULLIF(orig_dc_flag,''), dc_flag)),'')<>'') "
            "ORDER BY id LIMIT ? OFFSET ?"
        ),
        count_sql=(
            "SELECT COUNT(1) FROM fc_transaction_norm WHERE case_id=? "
            "AND (COALESCE(TRIM(COALESCE(NULLIF(orig_dc_flag,''), dc_flag)),'')='' "
            "     OR COALESCE(NULLIF(orig_dc_flag,''), dc_flag) NOT IN ('进','出')) "
            "AND (clean_dc_normalized=1 OR clean_dc_inferred=1 "
            "     OR COALESCE(TRIM(COALESCE(NULLIF(orig_dc_flag,''), dc_flag)),'')<>'')"
        ),
    ),
    CleaningStepDefinition(
        step=6,
        key="step6",
        title="交易卡号补全",
        kind="补全",
        description="基于同源账号与交易上下文补全缺失的交易卡号。",
        headers=(
            "交易卡号（原）",
            "交易卡号（补）",
            "交易账号",
            "交易时间",
            "交易金额",
            "收付标志",
            "交易对手账号",
            "对手户名",
        ),
        sql=(
            "SELECT COALESCE(NULLIF(orig_card_no,''), card_no), "
            "COALESCE(NULLIF(clean_card_no,''), card_no), "
            "acct_no, txn_time, "
            "COALESCE(NULLIF(clean_amount,''), amount), "
            "COALESCE(NULLIF(clean_dc_flag,''), dc_flag), "
            "counterparty_acct, counterparty_name "
            "FROM fc_transaction_norm WHERE case_id=? AND clean_card_filled=1 "
            "ORDER BY id LIMIT ? OFFSET ?"
        ),
        count_sql="SELECT COUNT(1) FROM fc_transaction_norm WHERE case_id=? AND clean_card_filled=1",
    ),
    CleaningStepDefinition(
        step=7,
        key="step7",
        title="交易明细后缀处理",
        kind="处理",
        description="清理交易明细中的卡号、账号后缀噪声，统一标识格式。",
        headers=(
            "交易卡号（原）",
            "交易卡号（改）",
            "交易账号（原）",
            "交易账号（改）",
            "交易时间",
            "交易金额",
            "收付标志",
            "交易对手账号",
            "对手户名",
        ),
        sql=(
            "SELECT card_no, COALESCE(NULLIF(clean_card_no,''), card_no), "
            "acct_no, COALESCE(NULLIF(clean_acct_no,''), acct_no), "
            "txn_time, COALESCE(NULLIF(clean_amount,''), amount), "
            "COALESCE(NULLIF(clean_dc_flag,''), dc_flag), "
            "counterparty_acct, counterparty_name "
            "FROM fc_transaction_norm WHERE case_id=? AND ("
            "  clean_suffix_fixed=1 OR "
            "  instr(card_no, '-') > 1 OR instr(card_no, '_') > 1 OR "
            "  instr(acct_no, '-') > 1 OR instr(acct_no, '_') > 1 OR "
            "  COALESCE(NULLIF(clean_card_no,''), card_no)<>card_no OR "
            "  COALESCE(NULLIF(clean_acct_no,''), acct_no)<>acct_no"
            ") "
            "ORDER BY id LIMIT ? OFFSET ?"
        ),
        count_sql=(
            "SELECT COUNT(1) FROM fc_transaction_norm WHERE case_id=? AND ("
            "  clean_suffix_fixed=1 OR "
            "  instr(card_no, '-') > 1 OR instr(card_no, '_') > 1 OR "
            "  instr(acct_no, '-') > 1 OR instr(acct_no, '_') > 1 OR "
            "  COALESCE(NULLIF(clean_card_no,''), card_no)<>card_no OR "
            "  COALESCE(NULLIF(clean_acct_no,''), acct_no)<>acct_no"
            ")"
        ),
    ),
    CleaningStepDefinition(
        step=8,
        key="step8",
        title="账户信息表清洗",
        kind="处理",
        description="清洗账户信息表中的无效记录与格式异常字段。",
        headers=(
            "账户开户名称",
            "交易卡号（原）",
            "交易卡号（改）",
            "交易账号（原）",
            "交易账号（改）",
            "开户人证件号码",
            "账户开户时间",
            "账户余额",
            "可用余额",
            "账户类型",
            "账户开户银行",
            "无效数据",
        ),
        sql=(
            "SELECT account_open_name, card_no, COALESCE(NULLIF(clean_card_no,''), card_no), "
            "acct_no, COALESCE(NULLIF(clean_acct_no,''), acct_no), opener_id_no, "
            "open_time, balance, available_balance, acct_type, open_bank, "
            "CASE WHEN clean_acct_invalid=1 THEN '__RED__无效数据' ELSE '' END "
            "FROM fc_account_norm WHERE case_id=? AND ("
            "  clean_acct_invalid=1 OR "
            "  clean_suffix_fixed=1 OR "
            "  COALESCE(NULLIF(clean_card_no,''), card_no)<>card_no OR "
            "  COALESCE(NULLIF(clean_acct_no,''), acct_no)<>acct_no"
            ") "
            "ORDER BY id LIMIT ? OFFSET ?"
        ),
        count_sql=(
            "SELECT COUNT(1) FROM fc_account_norm WHERE case_id=? AND ("
            "  clean_acct_invalid=1 OR "
            "  clean_suffix_fixed=1 OR "
            "  COALESCE(NULLIF(clean_card_no,''), card_no)<>card_no OR "
            "  COALESCE(NULLIF(clean_acct_no,''), acct_no)<>acct_no"
            ")"
        ),
        empty_placeholder="[空]",
    ),
    CleaningStepDefinition(
        step=9,
        key="step9",
        title="账户信息后缀处理",
        kind="处理",
        description="统一账户信息表中的卡号、账号后缀与主键格式。",
        headers=(
            "账户开户名称",
            "开户人证件号码",
            "交易卡号（原）",
            "交易卡号（改）",
            "交易账号（原）",
            "交易账号（改）",
            "账户开户时间",
            "账户余额",
            "可用余额",
            "账户类型",
            "账户开户银行",
        ),
        sql=(
            "SELECT account_open_name, opener_id_no, "
            "card_no, COALESCE(NULLIF(clean_card_no,''), card_no), "
            "acct_no, COALESCE(NULLIF(clean_acct_no,''), acct_no), "
            "open_time, balance, available_balance, acct_type, open_bank "
            "FROM fc_account_norm WHERE case_id=? AND ("
            "  clean_suffix_fixed=1 OR "
            "  COALESCE(NULLIF(clean_card_no,''), card_no)<>card_no OR "
            "  COALESCE(NULLIF(clean_acct_no,''), acct_no)<>acct_no"
            ") "
            "ORDER BY id LIMIT ? OFFSET ?"
        ),
        count_sql=(
            "SELECT COUNT(1) FROM fc_account_norm WHERE case_id=? AND ("
            "  clean_suffix_fixed=1 OR "
            "  COALESCE(NULLIF(clean_card_no,''), card_no)<>card_no OR "
            "  COALESCE(NULLIF(clean_acct_no,''), acct_no)<>acct_no"
            ")"
        ),
    ),
    CleaningStepDefinition(
        step=10,
        key="step10",
        title="账户开户名称及证件号补全",
        kind="补全",
        description="基于交易聚合信息补全账户开户名称与证件号码。",
        headers=(
            "交易卡号",
            "交易账号",
            "账户开户名称",
            "开户人证件号码",
            "进账金额",
            "进账次数",
            "出账金额",
            "出账次数",
            "最早交易时间",
            "最晚交易时间",
        ),
        sql=(
            "WITH base AS ("
            "  SELECT "
            "    COALESCE(NULLIF(clean_card_no,''), card_no) AS card_key, "
            "    COALESCE(NULLIF(clean_acct_no,''), acct_no) AS acct_key, "
            "    COALESCE(NULLIF(account_open_name,''), '') AS acct_name, "
            "    COALESCE(NULLIF(opener_id_no,''), '') AS id_no, "
            "    COALESCE(NULLIF(clean_dc_flag,''), dc_flag) AS dc_val, "
            f"    {_CLEAN_AMOUNT_FINITE_EXPR} AS amt_val, "
            f"    COALESCE(txn_ts, {_ts_norm_expr('txn_time')}) AS txn_ts "
            "  FROM fc_transaction_norm "
            "  WHERE case_id=? "
            "    AND clean_invalid=0 "
            "    AND clean_failed=0 "
            "    AND clean_reversal=0 "
            "    AND (COALESCE(TRIM(account_open_name),'')='' OR COALESCE(TRIM(opener_id_no),'')='')"
            "    AND (COALESCE(TRIM(COALESCE(NULLIF(clean_card_no,''), card_no)),'')<>'' "
            "         OR COALESCE(TRIM(COALESCE(NULLIF(clean_acct_no,''), acct_no)),'')<>'')"
            "), agg AS ("
            "  SELECT "
            "    card_key, acct_key, acct_name, id_no, "
            "    CASE WHEN SUM(CASE WHEN dc_val='进' AND amt_val IS NULL THEN 1 ELSE 0 END)>0 "
            "      THEN NULL ELSE ROUND(SUM(CASE WHEN dc_val='进' THEN ABS(amt_val) ELSE NULL END), 2) END AS in_amount, "
            "    NULLIF(SUM(CASE WHEN dc_val='进' THEN 1 ELSE 0 END), 0) AS in_count, "
            "    CASE WHEN SUM(CASE WHEN dc_val='出' AND amt_val IS NULL THEN 1 ELSE 0 END)>0 "
            "      THEN NULL ELSE ROUND(SUM(CASE WHEN dc_val='出' THEN ABS(amt_val) ELSE NULL END), 2) END AS out_amount, "
            "    NULLIF(SUM(CASE WHEN dc_val='出' THEN 1 ELSE 0 END), 0) AS out_count, "
            "    MIN(txn_ts) AS first_txn, "
            "    MAX(txn_ts) AS last_txn "
            "  FROM base "
            "  GROUP BY card_key, acct_key, acct_name, id_no"
            ") "
            "SELECT card_key, acct_key, acct_name, id_no, "
            "in_amount, in_count, out_amount, out_count, first_txn, last_txn "
            "FROM agg "
            "ORDER BY last_txn DESC NULLS LAST LIMIT ? OFFSET ?"
        ),
        count_sql=(
            "WITH base AS ("
            "  SELECT "
            "    COALESCE(NULLIF(clean_card_no,''), card_no) AS card_key, "
            "    COALESCE(NULLIF(clean_acct_no,''), acct_no) AS acct_key, "
            "    COALESCE(NULLIF(account_open_name,''), '') AS acct_name, "
            "    COALESCE(NULLIF(opener_id_no,''), '') AS id_no "
            "  FROM fc_transaction_norm "
            "  WHERE case_id=? "
            "    AND clean_invalid=0 "
            "    AND clean_failed=0 "
            "    AND clean_reversal=0 "
            "    AND (COALESCE(TRIM(account_open_name),'')='' OR COALESCE(TRIM(opener_id_no),'')='')"
            "    AND (COALESCE(TRIM(COALESCE(NULLIF(clean_card_no,''), card_no)),'')<>'' "
            "         OR COALESCE(TRIM(COALESCE(NULLIF(clean_acct_no,''), acct_no)),'')<>'')"
            ") "
            "SELECT COUNT(1) FROM ("
            "  SELECT card_key, acct_key, acct_name, id_no "
            "  FROM base GROUP BY card_key, acct_key, acct_name, id_no"
            ") t"
        ),
    ),
)


def get_cleaning_step_definition(step: int) -> CleaningStepDefinition:
    for definition in CLEANING_STEP_DEFINITIONS:
        if definition.step == step:
            return definition
    raise KeyError(step)
