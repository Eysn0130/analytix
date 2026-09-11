use anyhow::{anyhow, bail, Context, Result};
use serde_json::{json, Map, Value};
use std::{
    collections::{HashMap, HashSet},
    fs,
    path::PathBuf,
};

pub(crate) struct AnalysisGraphDataArgs {
    pub(crate) payload: Value,
}

pub(crate) fn parse_args(mut iter: impl Iterator<Item = String>) -> Result<AnalysisGraphDataArgs> {
    let mut payload: Option<Value> = None;

    while let Some(flag) = iter.next() {
        match flag.as_str() {
            "--payload-json" => {
                let raw_payload = crate::required_value(&mut iter, "--payload-json")?;
                payload = Some(
                    serde_json::from_str(&raw_payload)
                        .with_context(|| "parse --payload-json as JSON")?,
                );
            }
            "--input-path" => {
                let input_path = PathBuf::from(crate::required_value(&mut iter, "--input-path")?);
                let raw_payload = fs::read_to_string(&input_path).with_context(|| {
                    format!("read analysis graph data input {}", input_path.display())
                })?;
                payload = Some(serde_json::from_str(&raw_payload).with_context(|| {
                    format!("parse analysis graph data input {}", input_path.display())
                })?);
            }
            "--help" | "-h" => {
                bail!("Usage: analytix-analysis-compute project-analysis-graph-data <--payload-json <json>|--input-path <path>>");
            }
            other => bail!("unsupported analysis graph data flag: {other}"),
        }
    }

    Ok(AnalysisGraphDataArgs {
        payload: payload.ok_or_else(|| anyhow!("--payload-json or --input-path is required"))?,
    })
}

pub(crate) fn project_analysis_graph_data(payload: &Value) -> Result<Value> {
    let source = payload.get("source").unwrap_or(payload);
    let mut nodes = object_rows(source.get("nodes"));
    let mut edges = object_rows(source.get("edges"));
    let min = filter_bound(payload, "filterMin", "filter_min")?;
    let max = filter_bound(payload, "filterMax", "filter_max")?;
    if matches!((min, max), (Some(min_value), Some(max_value)) if min_value > max_value) {
        bail!("analysis graph filter range is invalid");
    }

    if min.is_some() || max.is_some() {
        let mut filtered_edges = Vec::with_capacity(edges.len());
        for edge in edges {
            let amount = edge_amount_total(&edge)
                .ok_or_else(|| anyhow!("analysis graph edge amount is unavailable"))?;
            if min.is_some_and(|min_value| amount < min_value)
                || max.is_some_and(|max_value| amount > max_value)
            {
                continue;
            }
            filtered_edges.push(edge);
        }
        edges = filtered_edges;

        let mut keep_node_ids = HashSet::new();
        for edge in &edges {
            if let Some(source_id) = object_text_id(edge, "source") {
                keep_node_ids.insert(source_id);
            }
            if let Some(target_id) = object_text_id(edge, "target") {
                keep_node_ids.insert(target_id);
            }
        }
        nodes.retain(|node| {
            object_text_id(node, "id")
                .map(|id| keep_node_ids.contains(&id))
                .unwrap_or(false)
        });
    }

    let core_ids = resolve_analysis_core_ids(&nodes, &edges);
    let child_map = build_analysis_core_child_map(&nodes, &edges, &core_ids);
    if bool_option(
        payload
            .get("collapseChildren")
            .or_else(|| payload.get("collapse_children")),
    ) {
        let hidden: HashSet<String> = child_map.keys().cloned().collect();
        nodes.retain(|node| {
            object_text_id(node, "id")
                .map(|id| !hidden.contains(&id))
                .unwrap_or(true)
        });
        edges.retain(|edge| {
            let source_hidden = object_text_id(edge, "source")
                .map(|id| hidden.contains(&id))
                .unwrap_or(false);
            let target_hidden = object_text_id(edge, "target")
                .map(|id| hidden.contains(&id))
                .unwrap_or(false);
            !source_hidden && !target_hidden
        });
    }

    apply_source_positions(&mut nodes, source.get("nodes"));

    Ok(json!({
        "nodes": nodes.into_iter().map(Value::Object).collect::<Vec<_>>(),
        "edges": edges.into_iter().map(Value::Object).collect::<Vec<_>>(),
        "coreIds": core_ids,
        "childMap": child_map,
    }))
}

fn object_rows(value: Option<&Value>) -> Vec<Map<String, Value>> {
    value
        .and_then(Value::as_array)
        .map(|rows| {
            rows.iter()
                .map(|row| row.as_object().cloned().unwrap_or_default())
                .collect()
        })
        .unwrap_or_default()
}

fn finite_option(value: Option<&Value>) -> Option<f64> {
    let value = value?;
    if value.is_null() {
        return None;
    }
    if let Some(text) = value.as_str() {
        let trimmed = text.trim();
        if trimmed.is_empty() {
            return None;
        }
        return trimmed
            .parse::<f64>()
            .ok()
            .filter(|number| number.is_finite());
    }
    value.as_f64().filter(|number| number.is_finite())
}

fn filter_bound(payload: &Value, primary: &str, alternate: &str) -> Result<Option<f64>> {
    let Some(value) = payload.get(primary).or_else(|| payload.get(alternate)) else {
        return Ok(None);
    };
    if value.is_null() || value.as_str().is_some_and(|text| text.trim().is_empty()) {
        return Ok(None);
    }
    finite_option(Some(value))
        .map(Some)
        .ok_or_else(|| anyhow!("analysis graph filter bound is invalid"))
}

fn bool_option(value: Option<&Value>) -> bool {
    match value {
        Some(Value::Bool(next)) => *next,
        Some(Value::String(text)) => matches!(
            text.trim().to_ascii_lowercase().as_str(),
            "1" | "true" | "yes" | "on"
        ),
        Some(Value::Number(number)) => number.as_i64().unwrap_or(0) != 0,
        _ => false,
    }
}

fn object_text_id(row: &Map<String, Value>, key: &str) -> Option<String> {
    let value = row.get(key)?;
    let text = match value {
        Value::String(text) => text.trim().to_string(),
        Value::Number(number) => number.to_string(),
        Value::Bool(value) => value.to_string(),
        _ => String::new(),
    };
    if text.is_empty() {
        None
    } else {
        Some(text)
    }
}

fn object_text(row: &Map<String, Value>, key: &str) -> String {
    row.get(key)
        .and_then(|value| match value {
            Value::String(text) => Some(text.trim().to_string()),
            Value::Number(number) => Some(number.to_string()),
            Value::Bool(value) => Some(value.to_string()),
            _ => None,
        })
        .unwrap_or_default()
}

fn finite_number(value: Option<&Value>) -> Option<f64> {
    match value {
        Some(Value::Number(number)) => number.as_f64().filter(|next| next.is_finite()),
        Some(Value::String(text)) => text
            .trim()
            .parse::<f64>()
            .ok()
            .filter(|next| next.is_finite()),
        _ => None,
    }
}

pub(crate) fn edge_amount_total(edge: &Map<String, Value>) -> Option<f64> {
    if edge.contains_key("forward_amount") || edge.contains_key("reverse_amount") {
        return complete_amount_pair(edge, "forward_amount", "reverse_amount");
    }

    if edge.contains_key("out_amount") || edge.contains_key("in_amount") {
        return complete_amount_pair(edge, "out_amount", "in_amount");
    }

    if edge.contains_key("amount") {
        return finite_number(edge.get("amount")).map(f64::abs);
    }

    if edge.contains_key("labelTop") || edge.contains_key("labelBottom") {
        let mut total = 0.0;
        for key in ["labelTop", "labelBottom"] {
            if edge.contains_key(key) {
                total += parse_amount_with_currency(edge.get(key))?;
            }
        }
        return Some(total);
    }

    if edge.contains_key("label") {
        return parse_amount_with_currency(edge.get("label"));
    }
    None
}

fn complete_amount_pair(edge: &Map<String, Value>, first: &str, second: &str) -> Option<f64> {
    Some(finite_number(edge.get(first))?.abs() + finite_number(edge.get(second))?.abs())
}

fn parse_amount_with_currency(value: Option<&Value>) -> Option<f64> {
    let text = value.and_then(Value::as_str)?;
    if text.is_empty() || !(text.contains('￥') || text.contains('元')) {
        return None;
    }
    parse_amount_from_label(text)
}

fn parse_amount_from_label(text: &str) -> Option<f64> {
    let chars: Vec<char> = text.chars().collect();
    let mut values = Vec::new();
    let mut index = 0;
    while index < chars.len() {
        let start = chars[index];
        let next_is_digit = chars
            .get(index + 1)
            .map(|c| c.is_ascii_digit())
            .unwrap_or(false);
        if !start.is_ascii_digit() && !(start == '-' && next_is_digit) {
            index += 1;
            continue;
        }

        let mut end = index + 1;
        while end < chars.len()
            && (chars[end].is_ascii_digit() || chars[end] == ',' || chars[end] == '.')
        {
            end += 1;
        }
        let token: String = chars[index..end].iter().filter(|c| **c != ',').collect();
        if let Ok(number) = token.parse::<f64>() {
            if number.is_finite() {
                values.push(number.abs());
            }
        }
        index = end;
    }
    values.into_iter().reduce(f64::max)
}

fn resolve_analysis_core_ids(
    nodes: &[Map<String, Value>],
    edges: &[Map<String, Value>],
) -> Vec<String> {
    if nodes.is_empty() {
        return Vec::new();
    }

    let role_core: Vec<String> = nodes
        .iter()
        .filter(|node| object_text(node, "role").to_ascii_lowercase() == "core")
        .filter_map(|node| object_text_id(node, "id"))
        .collect();
    if !role_core.is_empty() {
        return role_core;
    }

    let seed_rows: Vec<String> = nodes
        .iter()
        .filter(|node| object_text(node, "ntype").to_ascii_lowercase() == "seed")
        .filter_map(|node| object_text_id(node, "id"))
        .collect();
    if !seed_rows.is_empty() {
        return seed_rows;
    }

    let degree = build_degree_map(edges);
    let mut ids: Vec<String> = nodes
        .iter()
        .filter_map(|node| object_text_id(node, "id"))
        .collect();
    ids.sort_by(|a, b| {
        degree
            .get(b)
            .unwrap_or(&0)
            .cmp(degree.get(a).unwrap_or(&0))
            .then_with(|| a.cmp(b))
    });
    ids.into_iter().take(1).collect()
}

fn build_degree_map(edges: &[Map<String, Value>]) -> HashMap<String, usize> {
    let mut degree = HashMap::new();
    for edge in edges {
        if let Some(source_id) = object_text_id(edge, "source") {
            *degree.entry(source_id).or_insert(0) += 1;
        }
        if let Some(target_id) = object_text_id(edge, "target") {
            *degree.entry(target_id).or_insert(0) += 1;
        }
    }
    degree
}

fn build_analysis_core_child_map(
    nodes: &[Map<String, Value>],
    edges: &[Map<String, Value>],
    core_ids: &[String],
) -> Map<String, Value> {
    let mut out = Map::new();
    let core_set: HashSet<String> = core_ids.iter().cloned().collect();
    if nodes.is_empty() || core_set.is_empty() {
        return out;
    }

    let node_ids: HashSet<String> = nodes
        .iter()
        .filter_map(|node| object_text_id(node, "id"))
        .collect();
    let mut adjacency: HashMap<String, HashSet<String>> = node_ids
        .iter()
        .map(|id| (id.clone(), HashSet::new()))
        .collect();
    let mut edge_bucket_by_pair: HashMap<String, Vec<&Map<String, Value>>> = HashMap::new();

    for edge in edges {
        let Some(source_id) = object_text_id(edge, "source") else {
            continue;
        };
        let Some(target_id) = object_text_id(edge, "target") else {
            continue;
        };
        if !node_ids.contains(&source_id) || !node_ids.contains(&target_id) {
            continue;
        }
        if let Some(source_neighbors) = adjacency.get_mut(&source_id) {
            source_neighbors.insert(target_id.clone());
        }
        if let Some(target_neighbors) = adjacency.get_mut(&target_id) {
            target_neighbors.insert(source_id.clone());
        }
        if let Some(key) = pair_key(&source_id, &target_id) {
            edge_bucket_by_pair.entry(key).or_default().push(edge);
        }
    }

    let role_by_id: HashMap<String, String> = nodes
        .iter()
        .filter_map(|node| {
            Some((
                object_text_id(node, "id")?,
                object_text(node, "role").to_ascii_lowercase(),
            ))
        })
        .collect();

    for node in nodes {
        let Some(id) = object_text_id(node, "id") else {
            continue;
        };
        if core_set.contains(&id) {
            continue;
        }
        let role = object_text(node, "role").to_ascii_lowercase();
        if !role.is_empty() && role != "leaf" {
            continue;
        }

        let Some(neighbors) = adjacency.get(&id) else {
            continue;
        };
        if neighbors.len() != 1 {
            continue;
        }
        let Some(neighbor_id) = neighbors.iter().next().cloned() else {
            continue;
        };
        let Some(bucket_key) = pair_key(&id, &neighbor_id) else {
            continue;
        };
        let edge_bucket = edge_bucket_by_pair
            .get(&bucket_key)
            .cloned()
            .unwrap_or_default();
        if edge_bucket.is_empty()
            || !edge_bucket.iter().all(|edge| {
                let kind = edge_line_type(edge);
                kind == "single" || kind == "double"
            })
        {
            continue;
        }

        if core_set.contains(&neighbor_id) {
            out.insert(id, Value::String(neighbor_id));
            continue;
        }
        if role_by_id.get(&neighbor_id).map(String::as_str) != Some("adjacent") {
            continue;
        }
        let mut linked_core_ids: Vec<String> = adjacency
            .get(&neighbor_id)
            .map(|neighbors| {
                neighbors
                    .iter()
                    .filter(|neighbor| core_set.contains(*neighbor))
                    .cloned()
                    .collect()
            })
            .unwrap_or_default();
        linked_core_ids.sort();
        if linked_core_ids.len() == 1 {
            out.insert(id, Value::String(linked_core_ids.remove(0)));
        }
    }

    out
}

fn pair_key(a: &str, b: &str) -> Option<String> {
    if a.is_empty() || b.is_empty() {
        return None;
    }
    if a < b {
        Some(format!("{a}|{b}"))
    } else {
        Some(format!("{b}|{a}"))
    }
}

fn edge_line_type(edge: &Map<String, Value>) -> String {
    let direct = object_text(edge, "lineType").to_ascii_lowercase();
    if direct == "single" || direct == "double" {
        return direct;
    }
    let mode = object_text(edge, "mode").to_ascii_lowercase();
    if mode == "single" || mode == "double" {
        return mode;
    }
    let arrow = {
        let edge_arrow = object_text(edge, "edgeArrow");
        if edge_arrow.is_empty() {
            object_text(edge, "arrow")
        } else {
            edge_arrow
        }
    }
    .to_ascii_lowercase();
    match arrow.as_str() {
        "both" => "double".to_string(),
        "start" | "end" | "none" => "single".to_string(),
        _ => String::new(),
    }
}

fn apply_source_positions(nodes: &mut [Map<String, Value>], source_nodes: Option<&Value>) {
    let mut positions: HashMap<String, (f64, f64)> = HashMap::new();
    for source_node in source_nodes.and_then(Value::as_array).into_iter().flatten() {
        let Some(row) = source_node.as_object() else {
            continue;
        };
        let Some(id) = object_text_id(row, "id") else {
            continue;
        };
        let Some(x) = finite_number(row.get("x")) else {
            continue;
        };
        let Some(y) = finite_number(row.get("y")) else {
            continue;
        };
        positions.insert(id, (x, y));
    }
    if positions.is_empty() {
        return;
    }
    for node in nodes {
        let Some(id) = object_text_id(node, "id") else {
            continue;
        };
        let Some((x, y)) = positions.get(&id).copied() else {
            continue;
        };
        if finite_number(node.get("x")).is_none() {
            node.insert("x".to_string(), json!(x));
        }
        if finite_number(node.get("y")).is_none() {
            node.insert("y".to_string(), json!(y));
        }
    }
}

#[cfg(test)]
mod tests {
    use super::project_analysis_graph_data;
    use serde_json::{json, Value};

    fn graph(payload: Value) -> Value {
        project_analysis_graph_data(&payload).expect("project analysis graph data")
    }

    #[test]
    fn projects_core_child_map_without_filter() {
        assert_eq!(
            graph(json!({
                "source": {
                    "nodes": [
                        {"id": "core", "role": "core", "x": 0, "y": 0},
                        {"id": "adj", "role": "adjacent", "x": 50, "y": 0},
                        {"id": "leaf", "role": "leaf", "x": 100, "y": 0},
                        {"id": "isolated", "x": 200, "y": 0}
                    ],
                    "edges": [
                        {"id": "core-adj", "source": "core", "target": "adj", "amount": 100, "mode": "double"},
                        {"id": "adj-leaf", "source": "adj", "target": "leaf", "amount": 40, "mode": "single"}
                    ]
                }
            })),
            json!({
                "nodes": [
                    {"id": "core", "role": "core", "x": 0, "y": 0},
                    {"id": "adj", "role": "adjacent", "x": 50, "y": 0},
                    {"id": "leaf", "role": "leaf", "x": 100, "y": 0},
                    {"id": "isolated", "x": 200, "y": 0}
                ],
                "edges": [
                    {"id": "core-adj", "source": "core", "target": "adj", "amount": 100, "mode": "double"},
                    {"id": "adj-leaf", "source": "adj", "target": "leaf", "amount": 40, "mode": "single"}
                ],
                "coreIds": ["core"],
                "childMap": {"leaf": "core"}
            })
        );
    }

    #[test]
    fn amount_filter_removes_unlinked_nodes() {
        assert_eq!(
            graph(json!({
                "source": {
                    "nodes": [
                        {"id": "core", "role": "core", "x": 0, "y": 0},
                        {"id": "adj", "role": "adjacent", "x": 50, "y": 0},
                        {"id": "leaf", "role": "leaf", "x": 100, "y": 0}
                    ],
                    "edges": [
                        {"id": "core-adj", "source": "core", "target": "adj", "amount": 100, "mode": "double"},
                        {"id": "adj-leaf", "source": "adj", "target": "leaf", "amount": 40, "mode": "single"}
                    ]
                },
                "filterMin": 50
            })),
            json!({
                "nodes": [
                    {"id": "core", "role": "core", "x": 0, "y": 0},
                    {"id": "adj", "role": "adjacent", "x": 50, "y": 0}
                ],
                "edges": [
                    {"id": "core-adj", "source": "core", "target": "adj", "amount": 100, "mode": "double"}
                ],
                "coreIds": ["core"],
                "childMap": {}
            })
        );
    }

    #[test]
    fn collapse_children_removes_child_edges() {
        assert_eq!(
            graph(json!({
                "source": {
                    "nodes": [
                        {"id": "core", "role": "core", "x": 0, "y": 0},
                        {"id": "adj", "role": "adjacent", "x": 50, "y": 0},
                        {"id": "leaf", "role": "leaf", "x": 100, "y": 0}
                    ],
                    "edges": [
                        {"id": "core-adj", "source": "core", "target": "adj", "amount": 100, "mode": "double"},
                        {"id": "adj-leaf", "source": "adj", "target": "leaf", "amount": 40, "mode": "single"}
                    ]
                },
                "collapseChildren": true
            })),
            json!({
                "nodes": [
                    {"id": "core", "role": "core", "x": 0, "y": 0},
                    {"id": "adj", "role": "adjacent", "x": 50, "y": 0}
                ],
                "edges": [
                    {"id": "core-adj", "source": "core", "target": "adj", "amount": 100, "mode": "double"}
                ],
                "coreIds": ["core"],
                "childMap": {"leaf": "core"}
            })
        );
    }

    #[test]
    fn parses_currency_labels_for_filtering() {
        assert_eq!(
            graph(json!({
                "source": {
                    "nodes": [{"id": "a"}, {"id": "b"}, {"id": "c"}],
                    "edges": [
                        {"id": "keep", "source": "a", "target": "b", "labelTop": "流出 ￥1,200.50"},
                        {"id": "drop", "source": "b", "target": "c", "label": "金额 80 元"}
                    ]
                },
                "filterMin": 1000
            }))
            .get("edges")
            .cloned()
            .unwrap_or(Value::Null),
            json!([{"id": "keep", "source": "a", "target": "b", "labelTop": "流出 ￥1,200.50"}])
        );
    }

    #[test]
    fn amount_filter_rejects_missing_partial_or_invalid_amounts() {
        for edge in [
            json!({"source": "a", "target": "b"}),
            json!({"source": "a", "target": "b", "amount": null, "label": "金额 20 元"}),
            json!({"source": "a", "target": "b", "forward_amount": 20}),
            json!({"source": "a", "target": "b", "forward_amount": 20, "reverse_amount": "bad"}),
        ] {
            let error = project_analysis_graph_data(&json!({
                "source": {"nodes": [{"id": "a"}, {"id": "b"}], "edges": [edge]},
                "filterMin": 0,
            }))
            .expect_err("unknown amount must block filtering");
            assert_eq!(
                error.to_string(),
                "analysis graph edge amount is unavailable"
            );
        }
    }

    #[test]
    fn amount_filter_preserves_explicit_zero_without_label_fallback() {
        let result = graph(json!({
            "source": {
                "nodes": [{"id": "a"}, {"id": "b"}],
                "edges": [{
                    "id": "zero",
                    "source": "a",
                    "target": "b",
                    "amount": 0,
                    "label": "金额 999 元"
                }]
            },
            "filterMin": 0,
            "filterMax": 0,
        }));
        assert_eq!(result["edges"].as_array().map(Vec::len), Some(1));
    }

    #[test]
    fn amount_filter_rejects_invalid_bound() {
        let error = project_analysis_graph_data(&json!({
            "source": {"nodes": [], "edges": []},
            "filterMin": "not-a-number",
        }))
        .expect_err("invalid bound must not disable filtering");
        assert_eq!(error.to_string(), "analysis graph filter bound is invalid");
    }
}
