pub(in crate::stats_query_store) fn placeholder_kind_from_token_value(
    value: &str,
) -> Option<&'static str> {
    let kind = value.trim().strip_prefix("__cp_placeholder__::")?;
    normalize_placeholder_kind(kind)
}

pub(in crate::stats_query_store) fn dedupe_placeholder_kinds(values: &[String]) -> Vec<String> {
    let mut out = Vec::new();
    for value in values {
        let Some(kind) = normalize_placeholder_kind(value) else {
            continue;
        };
        if !out.iter().any(|item| item == kind) {
            out.push(kind.to_string());
        }
    }
    out
}

pub(in crate::stats_query_store) fn normalize_placeholder_kind(
    value: &str,
) -> Option<&'static str> {
    match value.trim() {
        "db_null" => Some("db_null"),
        "empty" => Some("empty"),
        "slash_n" => Some("slash_n"),
        "dash" => Some("dash"),
        "emdash" => Some("emdash"),
        "fw_dash" => Some("fw_dash"),
        "literal_null" => Some("literal_null"),
        "literal_none" => Some("literal_none"),
        "literal_nan" => Some("literal_nan"),
        _ => None,
    }
}

pub(in crate::stats_query_store) fn placeholder_label_for_kind(value: &str) -> &'static str {
    match value.trim() {
        "db_null" => "NULL",
        "empty" => "空串",
        "slash_n" => "\\N",
        "dash" => "-",
        "emdash" => "—",
        "fw_dash" => "－",
        "literal_null" => "null",
        "literal_none" => "none",
        "literal_nan" => "nan",
        _ => "",
    }
}
