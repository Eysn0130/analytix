use anyhow::{anyhow, Result};
use serde_json::{Map, Value};
use std::collections::BTreeMap;

use super::super::{
    helpers::{row_text, text_field},
    input_model::ProjectionNodeFacts,
};
use super::request_model::map_number_field;
use crate::flow_graph_search_core::{GraphSearchIndex, GraphSearchProjectionWeights};

pub(in crate::flow_projection_model) fn build_search_match_node_ids(
    nodes: &[Value],
    request_context: Option<&Map<String, Value>>,
    node_facts: &BTreeMap<String, ProjectionNodeFacts>,
) -> Result<Vec<String>> {
    match_projection_search_nodes(
        nodes,
        text_field(request_context, "search_query"),
        map_number_field(request_context, "search_limit").unwrap_or(24),
        node_facts,
    )
}

fn match_projection_search_nodes(
    nodes: &[Value],
    query: String,
    limit: i64,
    node_facts: &BTreeMap<String, ProjectionNodeFacts>,
) -> Result<Vec<String>> {
    for row in nodes {
        let node_id = row_text(row, "id");
        if !node_facts.contains_key(&node_id) {
            return Err(anyhow!("projection search facts missing for {node_id}"));
        }
    }
    let search_index = GraphSearchIndex::from_nodes(nodes);
    Ok(search_index.query_projection_ranked_node_ids(
        &query,
        usize::try_from(limit.max(1)).unwrap_or(1),
        |row| row_text(row, "id"),
        |row| GraphSearchProjectionWeights {
            amount_abs: node_facts[&row_text(row, "id")].total_amount_cents as f64 / 100.0,
            count: node_facts[&row_text(row, "id")].total_count,
        },
    ))
}

#[cfg(test)]
mod tests {
    use super::*;
    use serde_json::json;

    #[test]
    fn projection_search_reuses_graph_search_fields() {
        let nodes = vec![
            json!({
                "id": "node:raw",
                "title": "Other",
                "displayIdRaw": "账号: 7000 / 8000",
                "total_amount": 10,
                "total_count": 1,
            }),
            json!({
                "id": "node:name",
                "name": "Alice Alias",
                "total_amount": 20,
                "total_count": 2,
            }),
            json!({
                "id": "node:display-list",
                "displayIds": ["账户: 9000"],
                "total_amount": 30,
                "total_count": 3,
            }),
        ];

        let facts = test_node_facts(&nodes);
        assert_eq!(
            match_projection_search_nodes(&nodes, "8000".to_string(), 8, &facts).unwrap(),
            vec!["node:raw".to_string()]
        );
        assert_eq!(
            match_projection_search_nodes(&nodes, "alicealias".to_string(), 8, &facts).unwrap(),
            vec!["node:name".to_string()]
        );
        assert_eq!(
            match_projection_search_nodes(&nodes, "9000".to_string(), 8, &facts).unwrap(),
            vec!["node:display-list".to_string()]
        );
    }

    #[test]
    fn projection_search_keeps_existing_rank_order() {
        let nodes = vec![
            json!({"id": "low", "title": "alice", "total_amount": 10, "total_count": 1}),
            json!({"id": "high", "title": "alice", "total_amount": 20, "total_count": 1}),
            json!({"id": "prefix", "title": "alice extra", "total_amount": 100, "total_count": 9}),
        ];

        let facts = test_node_facts(&nodes);
        assert_eq!(
            match_projection_search_nodes(&nodes, "alice".to_string(), 8, &facts).unwrap(),
            vec!["high".to_string(), "low".to_string(), "prefix".to_string()]
        );
    }

    fn test_node_facts(nodes: &[Value]) -> BTreeMap<String, ProjectionNodeFacts> {
        nodes
            .iter()
            .map(|row| {
                let id = row_text(row, "id");
                let amount = row["total_amount"].as_f64().unwrap();
                let count = row["total_count"].as_i64().unwrap();
                (
                    id,
                    ProjectionNodeFacts {
                        total_amount_cents: (amount * 100.0) as i64,
                        total_count: count,
                    },
                )
            })
            .collect()
    }
}
