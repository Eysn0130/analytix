from __future__ import annotations


def sanitize_text_expr(expr: str) -> str:
    base = f"CAST({expr} AS TEXT)"
    stripped = f"regexp_replace({base}, '[\\x00-\\x1F\\x7F]', '', 'g')"
    stripped = f"regexp_replace({stripped}, '[\\t\\r\\n]', '', 'g')"
    stripped = f"replace({stripped}, chr(160), '')"
    trimmed = f"trim({stripped})"
    return f"NULLIF({trimmed}, '')"


def sanitize_precleaned_text_expr(expr: str) -> str:
    return f"NULLIF(trim(CAST({expr} AS TEXT)), '')"


def sanitize_text_keep_empty_expr(expr: str) -> str:
    base = f"CAST({expr} AS TEXT)"
    stripped = f"regexp_replace({base}, '[\\x00-\\x1F\\x7F]', '', 'g')"
    stripped = f"regexp_replace({stripped}, '[\\t\\r\\n]', '', 'g')"
    stripped = f"replace({stripped}, chr(160), '')"
    trimmed = f"trim({stripped})"
    return f"CASE WHEN {expr} IS NULL THEN NULL ELSE {trimmed} END"


def sanitize_precleaned_text_keep_empty_expr(expr: str) -> str:
    return f"CASE WHEN {expr} IS NULL THEN NULL ELSE trim(CAST({expr} AS TEXT)) END"


def hash_value_expr(expr: str) -> str:
    return sanitize_text_expr(expr)


def row_hash_expr(cols: list[str], *, precleaned_source: bool = False) -> str:
    clean_expr = sanitize_precleaned_text_expr if precleaned_source else hash_value_expr
    parts = [f"coalesce({clean_expr(column)}, '')" for column in cols]
    joined = " || chr(9247) || ".join(parts) if parts else "''"
    return f"lower(hex(sha256({joined})))"


def clean_text_expr(expr: str) -> str:
    return sanitize_text_expr(expr)


def num_norm_expr(expr: str) -> str:
    pattern = r"[-+]?(?:\d+(?:,\d{3})*(?:\.\d+)?|\.\d+)(?:[eE][-+]?\d+)?"
    num = f"regexp_extract({expr}, '{pattern}', 0)"
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
        f"WHEN {num} IS NULL OR {num}='' THEN NULL "
        f"WHEN {expr} LIKE '(%' AND {expr} LIKE '%)' THEN '-' || {num_abs} "
        f"ELSE {num_normalized} END"
    )


def ts_norm_expr(expr: str) -> str:
    return (
        "COALESCE("
        f"try_strptime({expr}, '%Y-%m-%d %H:%M:%S'),"
        f"try_strptime({expr}, '%Y-%m-%d %H:%M'),"
        f"try_strptime({expr}, '%Y-%m-%d'),"
        f"try_strptime({expr}, '%Y%m%d%H%M%S'),"
        f"try_strptime({expr}, '%Y%m%d%H%M'),"
        f"try_strptime({expr}, '%Y%m%d')"
        ")"
    )


def dc_norm_expr(expr: str) -> str:
    return (
        "CASE "
        f"WHEN {expr} IN ('进','出') THEN {expr} "
        f"WHEN {expr} LIKE '%进%' OR {expr} LIKE '%入%' OR {expr} LIKE '%贷%' OR {expr} LIKE '%收%' THEN '进' "
        f"WHEN {expr} LIKE '%出%' OR {expr} LIKE '%付%' OR {expr} LIKE '%借%' THEN '出' "
        "ELSE NULL END"
    )


def norm_key_expr(
    expr: str,
    *,
    precleaned_source: bool = False,
    input_values_cleaned: bool = False,
) -> str:
    cleaned = (
        expr
        if input_values_cleaned
        else (sanitize_precleaned_text_expr(expr) if precleaned_source else clean_text_expr(expr))
    )
    base = f"regexp_replace({cleaned}, '[[:space:]]+', '', 'g')"
    return (
        "CASE "
        f"WHEN {base} IS NULL OR {base}='' THEN NULL "
        f"WHEN instr({base}, '-') > 0 OR instr({base}, '_') > 0 THEN "
        f"  CASE WHEN (instr({base}, '_') > 0 AND (instr({base}, '-') = 0 OR instr({base}, '_') < instr({base}, '-'))) THEN "
        f"    CASE WHEN instr({base}, '_') > 1 THEN substr({base}, 1, instr({base}, '_') - 1) ELSE NULL END "
        f"  ELSE "
        f"    CASE WHEN instr({base}, '-') > 1 THEN substr({base}, 1, instr({base}, '-') - 1) ELSE NULL END "
        f"  END "
        f"ELSE {base} END"
    )
