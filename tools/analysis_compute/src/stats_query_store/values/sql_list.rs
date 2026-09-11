pub(in crate::stats_query_store) fn sql_literal_list(values: &[String]) -> String {
    values
        .iter()
        .map(|value| crate::sql_literal(value))
        .collect::<Vec<_>>()
        .join(",")
}
