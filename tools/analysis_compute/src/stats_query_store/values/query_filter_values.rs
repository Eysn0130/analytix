pub(in crate::stats_query_store) fn normalize_key_type(value: &str) -> &'static str {
    if value.trim() == "name" {
        "name"
    } else {
        "account"
    }
}

pub(in crate::stats_query_store) fn direction_value(value: &str) -> Option<&'static str> {
    match value {
        "in" => Some("进"),
        "out" => Some("出"),
        _ => None,
    }
}
