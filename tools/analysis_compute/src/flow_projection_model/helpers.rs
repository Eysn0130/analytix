use anyhow::{anyhow, bail, Context, Result};
use serde_json::{Map, Value};
use std::collections::{BTreeMap, BTreeSet, HashSet};

use super::input_model::{ProjectionEdge, ProjectionNodeFacts, MAX_SAFE_INTEGER};

pub(super) fn value_bool(value: &Value) -> Option<bool> {
    if let Some(flag) = value.as_bool() {
        return Some(flag);
    }
    if let Some(number) = value_i64(value) {
        return Some(number != 0);
    }
    match value_text(value).to_lowercase().as_str() {
        "true" | "t" | "yes" | "y" | "on" => Some(true),
        "false" | "f" | "no" | "n" | "off" => Some(false),
        _ => None,
    }
}

pub(super) fn projection_use_point_layer(request_context: Option<&Map<String, Value>>) -> bool {
    request_context
        .and_then(|request| {
            request
                .get("use_point_layer")
                .or_else(|| request.get("usePointLayer"))
        })
        .and_then(value_bool)
        .unwrap_or(false)
}

pub(super) fn projection_has_viewport(request_context: Option<&Map<String, Value>>) -> bool {
    request_context
        .and_then(|request| request.get("viewport"))
        .and_then(Value::as_object)
        .map(|viewport| !viewport.is_empty())
        .unwrap_or(false)
}

pub(super) fn text_field(object: Option<&Map<String, Value>>, key: &str) -> String {
    object
        .and_then(|item| item.get(key))
        .map(value_text)
        .unwrap_or_default()
}

pub(super) fn value_text(value: &Value) -> String {
    match value {
        Value::String(text) => text.trim().to_string(),
        Value::Null => String::new(),
        Value::Bool(flag) => flag.to_string(),
        Value::Number(number) => number.to_string(),
        _ => String::new(),
    }
}

pub(super) fn value_i64(value: &Value) -> Option<i64> {
    value
        .as_i64()
        .or_else(|| value_text(value).parse::<i64>().ok())
}

pub(super) fn row_text(row: &Value, key: &str) -> String {
    row.as_object()
        .and_then(|object| object.get(key))
        .map(value_text)
        .unwrap_or_default()
}

pub(super) fn row_bool(row: &Value, key: &str) -> Option<bool> {
    row.as_object()
        .and_then(|object| object.get(key))
        .and_then(Value::as_bool)
}

pub(super) fn row_text_list(row: &Value, key: &str) -> Vec<String> {
    row.as_object()
        .and_then(|object| object.get(key))
        .map(|value| to_text_list(Some(value)))
        .unwrap_or_default()
}

#[derive(Clone, Copy, Debug, PartialEq)]
pub(super) struct ProjectionNodeTotals {
    pub(super) total_amount_cents: i64,
    pub(super) total_count: i64,
}

pub(super) fn sum_projection_node_totals(
    node_ids: &[String],
    node_facts: &BTreeMap<String, ProjectionNodeFacts>,
) -> Result<ProjectionNodeTotals> {
    let mut total_amount_cents = 0_i64;
    let mut total_count = 0i64;
    for node_id in node_ids {
        let facts = node_facts
            .get(node_id)
            .ok_or_else(|| anyhow!("projection node facts missing for {node_id}"))?;
        total_amount_cents = total_amount_cents
            .checked_add(facts.total_amount_cents)
            .context("projection node amount total overflow")?;
        if total_amount_cents > MAX_SAFE_INTEGER {
            bail!("projection node amount total exceeds safe range");
        }
        total_count = total_count
            .checked_add(facts.total_count)
            .context("projection node count total overflow")?;
        if total_count > MAX_SAFE_INTEGER {
            bail!("projection node count total exceeds safe range");
        }
    }
    Ok(ProjectionNodeTotals {
        total_amount_cents,
        total_count,
    })
}

pub(super) fn to_text_list(value: Option<&Value>) -> Vec<String> {
    let Some(items) = value.and_then(Value::as_array) else {
        return Vec::new();
    };
    let mut out = Vec::new();
    let mut seen = HashSet::new();
    for item in items {
        let text = value_text(item);
        if text.is_empty() || !seen.insert(text.clone()) {
            continue;
        }
        out.push(text);
    }
    out
}

pub(super) fn build_runtime_degree_map(
    node_map: &BTreeMap<String, Value>,
    edges: &[ProjectionEdge],
) -> Result<(BTreeMap<String, i64>, BTreeMap<String, BTreeSet<String>>)> {
    let mut degree_map: BTreeMap<String, i64> = BTreeMap::new();
    let mut adjacency: BTreeMap<String, BTreeSet<String>> = BTreeMap::new();
    for node_id in node_map.keys() {
        degree_map.insert(node_id.clone(), 0);
    }
    for edge in edges {
        let source = &edge.source;
        let target = &edge.target;
        if !degree_map.contains_key(source) || !degree_map.contains_key(target) {
            bail!("projection edge endpoint coverage changed after validation");
        }
        let source_delta = if source == target { 0 } else { 1 };
        let source_degree = degree_map
            .get_mut(source)
            .ok_or_else(|| anyhow!("projection source degree missing"))?;
        *source_degree = (*source_degree)
            .checked_add(source_delta)
            .context("projection source degree overflow")?;
        let target_degree = degree_map
            .get_mut(target)
            .ok_or_else(|| anyhow!("projection target degree missing"))?;
        *target_degree = (*target_degree)
            .checked_add(1)
            .context("projection target degree overflow")?;
        adjacency
            .entry(source.clone())
            .or_default()
            .insert(target.clone());
        adjacency
            .entry(target.clone())
            .or_default()
            .insert(source.clone());
    }
    Ok((degree_map, adjacency))
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn sum_projection_node_totals_preserves_real_zero_and_exact_cents() {
        let node_ids = vec!["a".to_string(), "b".to_string(), "c".to_string()];
        let node_facts = BTreeMap::from([
            (
                "a".to_string(),
                ProjectionNodeFacts {
                    total_amount_cents: 1_235,
                    total_count: 2,
                },
            ),
            (
                "b".to_string(),
                ProjectionNodeFacts {
                    total_amount_cents: 710,
                    total_count: 0,
                },
            ),
            (
                "c".to_string(),
                ProjectionNodeFacts {
                    total_amount_cents: 0,
                    total_count: 4,
                },
            ),
        ]);

        assert_eq!(
            sum_projection_node_totals(&node_ids, &node_facts).unwrap(),
            ProjectionNodeTotals {
                total_amount_cents: 1_945,
                total_count: 6,
            }
        );
    }

    #[test]
    fn sum_projection_node_totals_rejects_missing_fact_instead_of_zero() {
        assert!(sum_projection_node_totals(
            &["missing".to_string()],
            &BTreeMap::<String, ProjectionNodeFacts>::new(),
        )
        .is_err());
    }
}
