pub(super) fn build_paginated_data_sql(
    agg_sql: &str,
    search_where: &str,
    sort_expr: &str,
    sort_order: &str,
    limit: i64,
    row_offset: i64,
) -> String {
    if is_unfiltered_search(search_where) {
        return format!(
            "WITH agg AS ({agg_sql}) \
             SELECT key_value, counterparty_account, counterparty_name, bank, location, \
                    placeholder_kind, key_label, in_amount, out_amount, in_count, out_count, \
                    first_time, last_time, doc, CAST(COUNT(1) OVER() AS BIGINT) AS total_count \
               FROM agg \
              ORDER BY {sort_expr} {sort_order}, CAST(key_value AS VARCHAR) ASC \
              LIMIT {limit} OFFSET {row_offset}"
        );
    }
    format!(
        "WITH agg AS ({agg_sql}), filtered AS (\
           SELECT key_value, counterparty_account, counterparty_name, bank, location, \
                  placeholder_kind, key_label, in_amount, out_amount, in_count, out_count, \
                  first_time, last_time, doc, CAST(COUNT(1) OVER() AS BIGINT) AS total_count \
             FROM agg \
            WHERE {search_where}\
         ) \
         SELECT key_value, counterparty_account, counterparty_name, bank, location, \
                placeholder_kind, key_label, in_amount, out_amount, in_count, out_count, \
                CAST(first_time AS VARCHAR) AS first_time, CAST(last_time AS VARCHAR) AS last_time, \
                doc, total_count \
           FROM filtered \
          ORDER BY {sort_expr} {sort_order}, CAST(key_value AS VARCHAR) ASC \
          LIMIT {limit} OFFSET {row_offset}"
    )
}

pub(super) fn build_paginated_count_sql(agg_sql: &str, search_where: &str) -> String {
    if is_unfiltered_search(search_where) {
        return format!("WITH agg AS ({agg_sql}) SELECT COUNT(1) FROM agg");
    }
    format!("WITH agg AS ({agg_sql}) SELECT COUNT(1) FROM agg WHERE {search_where}")
}

pub(super) fn build_unpaginated_data_sql(
    agg_sql: &str,
    search_where: &str,
    sort_expr: &str,
    sort_order: &str,
) -> String {
    if is_unfiltered_search(search_where) {
        return format!(
            "{agg_sql} ORDER BY {sort_expr} {sort_order}, CAST(grouped.key_value AS VARCHAR) ASC"
        );
    }
    format!(
        "WITH agg AS ({agg_sql}) \
         SELECT key_value, counterparty_account, counterparty_name, bank, location, \
                placeholder_kind, key_label, in_amount, out_amount, in_count, out_count, \
                CAST(first_time AS VARCHAR) AS first_time, CAST(last_time AS VARCHAR) AS last_time, \
                doc \
           FROM agg \
          WHERE {search_where} \
          ORDER BY {sort_expr} {sort_order}, CAST(key_value AS VARCHAR) ASC"
    )
}

fn is_unfiltered_search(search_where: &str) -> bool {
    search_where.trim() == "1=1"
}

#[cfg(test)]
mod tests {
    use super::*;

    const AGG_SQL: &str = "WITH grouped AS (SELECT 1 AS key_value) SELECT * FROM grouped";

    #[test]
    fn unpaginated_data_sql_skips_outer_agg_when_search_is_empty() {
        let sql = build_unpaginated_data_sql(AGG_SQL, "1=1", "key_value", "ASC");

        assert!(sql.starts_with(AGG_SQL));
        assert!(!sql.contains("WITH agg AS"));
        assert!(!sql.contains("WHERE 1=1"));
        assert!(sql.contains("ORDER BY key_value ASC, CAST(grouped.key_value AS VARCHAR) ASC"));
    }

    #[test]
    fn paginated_data_sql_skips_filtered_cte_when_search_is_empty() {
        let sql = build_paginated_data_sql(AGG_SQL, "1=1", "key_value", "ASC", 10, 0);

        assert!(sql.contains("WITH agg AS"));
        assert!(!sql.contains("filtered AS"));
        assert!(!sql.contains("WHERE 1=1"));
        assert!(sql.contains("CAST(COUNT(1) OVER() AS BIGINT) AS total_count"));
    }

    #[test]
    fn paginated_count_sql_skips_noop_where_when_search_is_empty() {
        let sql = build_paginated_count_sql(AGG_SQL, "1=1");

        assert_eq!(
            sql,
            format!("WITH agg AS ({AGG_SQL}) SELECT COUNT(1) FROM agg")
        );
    }

    #[test]
    fn search_filtered_data_sql_keeps_search_where_layer() {
        let sql = build_unpaginated_data_sql(
            AGG_SQL,
            "LOWER(counterparty_name) LIKE '%a%'",
            "key_value",
            "ASC",
        );

        assert!(sql.contains("WITH agg AS"));
        assert!(sql.contains("WHERE LOWER(counterparty_name) LIKE '%a%'"));
    }
}
