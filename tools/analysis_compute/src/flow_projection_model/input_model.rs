use anyhow::{anyhow, bail, Context, Result};
use serde_json::{Map, Value};
use std::collections::{BTreeMap, HashSet};

pub(super) const MAX_SAFE_INTEGER: i64 = 9_007_199_254_740_991;

#[derive(Clone, Copy, Debug, Eq, PartialEq)]
pub(super) struct ProjectionNodeFacts {
    pub(super) total_amount_cents: i64,
    pub(super) total_count: i64,
}

#[derive(Clone, Debug, Eq, PartialEq)]
pub(super) struct ProjectionEdge {
    pub(super) id: String,
    pub(super) source: String,
    pub(super) target: String,
    pub(super) amount_cents: i64,
    pub(super) count: i64,
    pub(super) out_amount_cents: i64,
    pub(super) in_amount_cents: i64,
    pub(super) forward_amount_cents: i64,
    pub(super) reverse_amount_cents: i64,
    pub(super) first_time: Option<String>,
    pub(super) last_time: Option<String>,
}

#[derive(Debug)]
pub(super) struct ValidatedProjectionInput {
    pub(super) nodes: Vec<Value>,
    pub(super) node_map: BTreeMap<String, Value>,
    pub(super) node_facts: BTreeMap<String, ProjectionNodeFacts>,
    pub(super) edges: Vec<ProjectionEdge>,
    pub(super) request_context: Option<Map<String, Value>>,
    pub(super) base_projection: Option<Map<String, Value>>,
    pub(super) tile_size: usize,
}

#[derive(Clone, Copy, Debug, Eq, PartialEq)]
enum ProjectionViewMode {
    Relation,
    Net,
}

pub(super) fn validate_projection_input(payload: &Value) -> Result<ValidatedProjectionInput> {
    let root = payload
        .as_object()
        .ok_or_else(|| anyhow!("projection input must be an object"))?;
    let request_context = optional_object(root, "request_context")?;
    let base_projection = optional_object(root, "base_projection")?;
    let view_mode = projection_view_mode(request_context.as_ref())?;
    let tile_size = optional_positive_usize(root.get("tile_size"), "tile_size")?.unwrap_or(192);

    let raw_nodes = required_array(root, "nodes")?;
    let mut nodes = Vec::with_capacity(raw_nodes.len());
    let mut node_map = BTreeMap::new();
    let mut node_facts = BTreeMap::new();
    for (index, raw_node) in raw_nodes.iter().enumerate() {
        let path = format!("nodes[{index}]");
        let mut row = raw_node
            .as_object()
            .cloned()
            .ok_or_else(|| anyhow!("{path} must be an object"))?;
        let id = required_text(&row, "id", &path)?;
        let total_amount_cents = required_money_cents(&row, "total_amount", &path)?;
        let total_count = required_count(&row, "total_count", &path)?;
        validate_optional_node_text(&row, &path)?;
        if node_map.contains_key(&id) {
            bail!("{path}.id duplicates node id {id}");
        }
        row.insert("id".to_string(), Value::String(id.clone()));
        row.insert("total_amount".to_string(), money_value(total_amount_cents));
        row.insert("total_count".to_string(), Value::from(total_count));
        let row_value = Value::Object(row);
        nodes.push(row_value.clone());
        node_map.insert(id.clone(), row_value);
        node_facts.insert(
            id,
            ProjectionNodeFacts {
                total_amount_cents,
                total_count,
            },
        );
    }

    validate_base_projection(base_projection.as_ref(), &node_map)?;

    let raw_edges = required_array(root, "edges")?;
    let node_ids: HashSet<&str> = node_map.keys().map(String::as_str).collect();
    let mut edge_ids = HashSet::new();
    let mut edges = Vec::with_capacity(raw_edges.len());
    for (index, raw_edge) in raw_edges.iter().enumerate() {
        let path = format!("edges[{index}]");
        let row = raw_edge
            .as_object()
            .ok_or_else(|| anyhow!("{path} must be an object"))?;
        let edge = validate_edge(row, &path, view_mode)?;
        if !edge_ids.insert(edge.id.clone()) {
            bail!("{path}.id duplicates edge id {}", edge.id);
        }
        if !node_ids.contains(edge.source.as_str()) {
            bail!("{path}.source references unknown node {}", edge.source);
        }
        if !node_ids.contains(edge.target.as_str()) {
            bail!("{path}.target references unknown node {}", edge.target);
        }
        edges.push(edge);
    }

    Ok(ValidatedProjectionInput {
        nodes,
        node_map,
        node_facts,
        edges,
        request_context,
        base_projection,
        tile_size,
    })
}

fn validate_base_projection(
    base_projection: Option<&Map<String, Value>>,
    node_map: &BTreeMap<String, Value>,
) -> Result<()> {
    let Some(base_projection) = base_projection else {
        return Ok(());
    };
    let anchor_ids = optional_node_id_array(
        base_projection,
        &["anchor_node_ids", "anchorNodeIds"],
        "base_projection.anchor_node_ids",
        node_map,
    )?
    .unwrap_or_default();
    let anchor_set: HashSet<&str> = anchor_ids.iter().map(String::as_str).collect();
    let _ = optional_node_id_array(
        base_projection,
        &["expanded_node_ids", "expandedNodeIds"],
        "base_projection.expanded_node_ids",
        node_map,
    )?;

    let Some(raw_clusters) = base_projection.get("clusters") else {
        return Ok(());
    };
    let raw_clusters = raw_clusters
        .as_array()
        .ok_or_else(|| anyhow!("base_projection.clusters must be an array"))?;
    let mut cluster_ids = HashSet::new();
    let mut member_owners = BTreeMap::new();
    for (index, raw_cluster) in raw_clusters.iter().enumerate() {
        let path = format!("base_projection.clusters[{index}]");
        let row = raw_cluster
            .as_object()
            .ok_or_else(|| anyhow!("{path} must be an object"))?;
        let cluster_id = required_alias_text(row, &["cluster_id", "clusterId"], &path)?;
        if !cluster_ids.insert(cluster_id.clone()) {
            bail!("{path}.cluster_id duplicates cluster id {cluster_id}");
        }
        let anchor_id = required_alias_text(row, &["anchor_id", "anchorId"], &path)?;
        require_known_node(&anchor_id, &format!("{path}.anchor_id"), node_map)?;
        if !anchor_set.is_empty() && !anchor_set.contains(anchor_id.as_str()) {
            bail!("{path}.anchor_id is not covered by base_projection.anchor_node_ids");
        }
        let member_ids = required_node_id_array(
            row,
            &["member_ids", "memberIds"],
            &format!("{path}.member_ids"),
            node_map,
        )?;
        if member_ids.is_empty() {
            bail!("{path}.member_ids must not be empty");
        }
        for member_id in member_ids {
            if member_id == anchor_id {
                bail!("{path}.member_ids must not contain its anchor node");
            }
            if let Some(owner) = member_owners.insert(member_id.clone(), cluster_id.clone()) {
                bail!(
                    "{path}.member_ids assigns node {member_id} to both {owner} and {cluster_id}"
                );
            }
        }
    }
    Ok(())
}

fn optional_node_id_array(
    row: &Map<String, Value>,
    keys: &[&str],
    path: &str,
    node_map: &BTreeMap<String, Value>,
) -> Result<Option<Vec<String>>> {
    let Some(value) = alias_value(row, keys) else {
        return Ok(None);
    };
    parse_node_id_array(value, path, node_map).map(Some)
}

fn required_node_id_array(
    row: &Map<String, Value>,
    keys: &[&str],
    path: &str,
    node_map: &BTreeMap<String, Value>,
) -> Result<Vec<String>> {
    let value = alias_value(row, keys).ok_or_else(|| anyhow!("{path} is required"))?;
    parse_node_id_array(value, path, node_map)
}

fn parse_node_id_array(
    value: &Value,
    path: &str,
    node_map: &BTreeMap<String, Value>,
) -> Result<Vec<String>> {
    let values = value
        .as_array()
        .ok_or_else(|| anyhow!("{path} must be an array"))?;
    let mut node_ids = Vec::with_capacity(values.len());
    let mut seen = HashSet::new();
    for (index, value) in values.iter().enumerate() {
        let item_path = format!("{path}[{index}]");
        let node_id = value
            .as_str()
            .map(str::trim)
            .filter(|value| !value.is_empty())
            .ok_or_else(|| anyhow!("{item_path} must be a non-empty string"))?
            .to_string();
        require_known_node(&node_id, &item_path, node_map)?;
        if !seen.insert(node_id.clone()) {
            bail!("{item_path} duplicates node id {node_id}");
        }
        node_ids.push(node_id);
    }
    Ok(node_ids)
}

fn require_known_node(node_id: &str, path: &str, node_map: &BTreeMap<String, Value>) -> Result<()> {
    if !node_map.contains_key(node_id) {
        bail!("{path} references unknown node {node_id}");
    }
    Ok(())
}

fn required_alias_text(row: &Map<String, Value>, keys: &[&str], path: &str) -> Result<String> {
    let value = alias_value(row, keys)
        .and_then(Value::as_str)
        .map(str::trim)
        .filter(|value| !value.is_empty())
        .ok_or_else(|| anyhow!("{path}.{} must be a non-empty string", keys[0]))?;
    Ok(value.to_string())
}

fn alias_value<'a>(row: &'a Map<String, Value>, keys: &[&str]) -> Option<&'a Value> {
    keys.iter().find_map(|key| row.get(*key))
}

fn validate_edge(
    row: &Map<String, Value>,
    path: &str,
    view_mode: ProjectionViewMode,
) -> Result<ProjectionEdge> {
    let id = required_text(row, "id", path)?;
    let source = required_text(row, "source", path)?;
    let target = required_text(row, "target", path)?;
    let amount_cents = required_money_cents(row, "amount", path)?;
    let count = required_count(row, "count", path)?;
    let out_amount_cents = required_money_cents(row, "out_amount", path)?;
    let in_amount_cents = required_money_cents(row, "in_amount", path)?;
    let forward_amount_cents = required_money_cents(row, "forward_amount", path)?;
    let reverse_amount_cents = required_money_cents(row, "reverse_amount", path)?;
    let mode = required_text(row, "mode", path)?.to_ascii_lowercase();
    let first_time = optional_text(row, "first_time", path)?;
    let last_time = optional_text(row, "last_time", path)?;

    let directional_total = forward_amount_cents
        .checked_add(reverse_amount_cents)
        .context("flow projection directional amount overflow")?;
    if amount_cents != directional_total {
        bail!(
            "{path} direction coverage mismatch: amount must equal forward_amount + reverse_amount"
        );
    }
    let expected_mode = if forward_amount_cents > 0 && reverse_amount_cents > 0 {
        "double"
    } else {
        "single"
    };
    if mode != expected_mode {
        bail!("{path}.mode is inconsistent with directional coverage");
    }

    match view_mode {
        ProjectionViewMode::Relation => validate_relation_direction(
            path,
            &source,
            &target,
            amount_cents,
            out_amount_cents,
            in_amount_cents,
            forward_amount_cents,
            reverse_amount_cents,
        )?,
        ProjectionViewMode::Net => validate_net_direction(
            path,
            &source,
            &target,
            amount_cents,
            out_amount_cents,
            in_amount_cents,
            forward_amount_cents,
            reverse_amount_cents,
        )?,
    }

    Ok(ProjectionEdge {
        id,
        source,
        target,
        amount_cents,
        count,
        out_amount_cents,
        in_amount_cents,
        forward_amount_cents,
        reverse_amount_cents,
        first_time,
        last_time,
    })
}

#[allow(clippy::too_many_arguments)]
fn validate_relation_direction(
    path: &str,
    source: &str,
    target: &str,
    amount_cents: i64,
    out_amount_cents: i64,
    in_amount_cents: i64,
    forward_amount_cents: i64,
    reverse_amount_cents: i64,
) -> Result<()> {
    let canonical_total = out_amount_cents
        .checked_add(in_amount_cents)
        .context("flow projection relation amount overflow")?;
    if canonical_total != amount_cents {
        bail!("{path} relation coverage mismatch: amount must equal out_amount + in_amount");
    }
    let source_is_canonical_a = source <= target;
    let (expected_out, expected_in) = if source_is_canonical_a {
        (forward_amount_cents, reverse_amount_cents)
    } else {
        (reverse_amount_cents, forward_amount_cents)
    };
    if out_amount_cents != expected_out || in_amount_cents != expected_in {
        bail!("{path} relation direction is inconsistent with source and target");
    }
    Ok(())
}

#[allow(clippy::too_many_arguments)]
fn validate_net_direction(
    path: &str,
    source: &str,
    target: &str,
    amount_cents: i64,
    out_amount_cents: i64,
    in_amount_cents: i64,
    forward_amount_cents: i64,
    reverse_amount_cents: i64,
) -> Result<()> {
    if source == target && amount_cents != 0 {
        bail!("{path} net direction cannot publish a nonzero self-loop");
    }
    let net_cents = out_amount_cents
        .checked_sub(in_amount_cents)
        .context("flow projection net amount overflow")?;
    if net_cents.unsigned_abs() != amount_cents as u64
        || forward_amount_cents != amount_cents
        || reverse_amount_cents != 0
    {
        bail!("{path} net direction coverage is inconsistent with amount");
    }
    if amount_cents == 0 {
        if source != target && out_amount_cents != in_amount_cents {
            bail!("{path} zero net amount has inconsistent directional totals");
        }
        return Ok(());
    }
    let source_is_canonical_a = source <= target;
    if (net_cents > 0) != source_is_canonical_a {
        bail!("{path} net direction is inconsistent with source and target");
    }
    Ok(())
}

fn required_array<'a>(root: &'a Map<String, Value>, key: &str) -> Result<&'a Vec<Value>> {
    root.get(key)
        .and_then(Value::as_array)
        .ok_or_else(|| anyhow!("{key} must be a required array"))
}

fn optional_object(root: &Map<String, Value>, key: &str) -> Result<Option<Map<String, Value>>> {
    match root.get(key) {
        None | Some(Value::Null) => Ok(None),
        Some(Value::Object(value)) => Ok(Some(value.clone())),
        Some(_) => bail!("{key} must be an object when present"),
    }
}

fn required_text(row: &Map<String, Value>, key: &str, path: &str) -> Result<String> {
    let text = row
        .get(key)
        .and_then(Value::as_str)
        .map(str::trim)
        .filter(|value| !value.is_empty())
        .ok_or_else(|| anyhow!("{path}.{key} must be a non-empty string"))?;
    Ok(text.to_string())
}

fn optional_text(row: &Map<String, Value>, key: &str, path: &str) -> Result<Option<String>> {
    match row.get(key) {
        None | Some(Value::Null) => Ok(None),
        Some(Value::String(value)) => {
            let value = value.trim();
            if value.is_empty() {
                Ok(None)
            } else {
                Ok(Some(value.to_string()))
            }
        }
        Some(_) => bail!("{path}.{key} must be a string or null when present"),
    }
}

fn validate_optional_node_text(row: &Map<String, Value>, path: &str) -> Result<()> {
    for key in ["title", "name", "display_id", "ntype"] {
        let _ = optional_text(row, key, path)?;
    }
    Ok(())
}

fn required_count(row: &Map<String, Value>, key: &str, path: &str) -> Result<i64> {
    let value = row
        .get(key)
        .and_then(Value::as_u64)
        .ok_or_else(|| anyhow!("{path}.{key} must be a non-negative JSON integer"))?;
    if value > MAX_SAFE_INTEGER as u64 {
        bail!("{path}.{key} exceeds the safe integer range");
    }
    Ok(value as i64)
}

fn required_money_cents(row: &Map<String, Value>, key: &str, path: &str) -> Result<i64> {
    let value = row
        .get(key)
        .ok_or_else(|| anyhow!("{path}.{key} is required"))?;
    money_cents(value).with_context(|| format!("{path}.{key} must be finite canonical money"))
}

fn money_cents(value: &Value) -> Result<i64> {
    let number = value
        .as_number()
        .ok_or_else(|| anyhow!("money must be a JSON number"))?;
    decimal_text_to_cents(&number.to_string())
}

fn decimal_text_to_cents(text: &str) -> Result<i64> {
    if text.starts_with('-') {
        bail!("money must be non-negative");
    }
    let (mantissa, exponent) = split_exponent(text)?;
    let mut parts = mantissa.split('.');
    let whole = parts.next().unwrap_or_default();
    let fraction = parts.next().unwrap_or_default();
    if parts.next().is_some()
        || whole.is_empty()
        || !whole.bytes().all(|byte| byte.is_ascii_digit())
        || !fraction.bytes().all(|byte| byte.is_ascii_digit())
    {
        bail!("money has invalid decimal syntax");
    }

    let mut digits = format!("{whole}{fraction}");
    let scale = i64::try_from(fraction.len()).context("money scale overflow")? - exponent;
    if scale < 2 {
        let append = usize::try_from(2 - scale).context("money scale overflow")?;
        if append > 32 {
            bail!("money exceeds the safe range");
        }
        digits.extend(std::iter::repeat('0').take(append));
    } else if scale > 2 {
        let remove = usize::try_from(scale - 2).context("money scale overflow")?;
        if remove >= digits.len() {
            if digits.bytes().any(|byte| byte != b'0') {
                bail!("money has precision below one cent");
            }
            digits.clear();
        } else {
            let split_at = digits.len() - remove;
            if digits[split_at..].bytes().any(|byte| byte != b'0') {
                bail!("money has more than two decimal places");
            }
            digits.truncate(split_at);
        }
    }

    let cents_text = digits.trim_start_matches('0');
    if cents_text.is_empty() {
        return Ok(0);
    }
    let mut cents = 0_i64;
    for byte in cents_text.bytes() {
        cents = cents
            .checked_mul(10)
            .and_then(|value| value.checked_add(i64::from(byte - b'0')))
            .ok_or_else(|| anyhow!("money exceeds the safe range"))?;
        if cents > MAX_SAFE_INTEGER {
            bail!("money exceeds the safe range");
        }
    }
    Ok(cents)
}

fn split_exponent(text: &str) -> Result<(&str, i64)> {
    let Some(index) = text.find(['e', 'E']) else {
        return Ok((text, 0));
    };
    if text[index + 1..].contains(['e', 'E']) {
        bail!("money has invalid exponent syntax");
    }
    let exponent = text[index + 1..]
        .parse::<i64>()
        .context("money has invalid exponent")?;
    if !(-32..=32).contains(&exponent) {
        bail!("money exponent exceeds the safe range");
    }
    Ok((&text[..index], exponent))
}

fn optional_positive_usize(value: Option<&Value>, path: &str) -> Result<Option<usize>> {
    let Some(value) = value else {
        return Ok(None);
    };
    let value = value
        .as_u64()
        .filter(|value| *value > 0)
        .ok_or_else(|| anyhow!("{path} must be a positive JSON integer"))?;
    usize::try_from(value)
        .map(Some)
        .with_context(|| format!("{path} exceeds platform range"))
}

fn projection_view_mode(
    request_context: Option<&Map<String, Value>>,
) -> Result<ProjectionViewMode> {
    let Some(value) = request_context.and_then(|request| request.get("view_mode")) else {
        return Ok(ProjectionViewMode::Relation);
    };
    let value = value
        .as_str()
        .ok_or_else(|| anyhow!("request_context.view_mode must be a string"))?
        .trim()
        .to_ascii_lowercase();
    match value.as_str() {
        "" | "relation" => Ok(ProjectionViewMode::Relation),
        "net" => Ok(ProjectionViewMode::Net),
        _ => bail!("request_context.view_mode must be relation or net"),
    }
}

pub(super) fn money_value(cents: i64) -> Value {
    Value::from(cents as f64 / 100.0)
}

#[cfg(test)]
mod tests {
    use super::*;
    use serde_json::json;

    fn minimal_payload(edge: Value) -> Value {
        json!({
            "nodes": [
                {"id": "a", "total_amount": 0, "total_count": 0},
                {"id": "b", "total_amount": 0, "total_count": 0}
            ],
            "edges": [edge]
        })
    }

    #[test]
    fn canonical_money_uses_exact_cents_without_epsilon() {
        assert_eq!(money_cents(&json!(0)).unwrap(), 0);
        assert_eq!(money_cents(&json!(12.34)).unwrap(), 1_234);
        assert_eq!(money_cents(&json!(12.340)).unwrap(), 1_234);
        assert!(money_cents(&json!(12.345)).is_err());
        assert!(money_cents(&json!("NaN")).is_err());
        assert!(money_cents(&json!(-0.01)).is_err());
    }

    #[test]
    fn relation_direction_uses_exact_cents_without_amount_inference() {
        let payload = minimal_payload(json!({
            "id": "a==b",
            "source": "a",
            "target": "b",
            "amount": 0.3,
            "count": 1,
            "out_amount": 0.2,
            "in_amount": 0.1,
            "forward_amount": 0.2,
            "reverse_amount": 0.1,
            "mode": "double"
        }));

        let input = validate_projection_input(&payload).unwrap();
        assert_eq!(input.edges[0].amount_cents, 30);
        assert_eq!(input.edges[0].forward_amount_cents, 20);
        assert_eq!(input.edges[0].reverse_amount_cents, 10);

        let mut missing_direction = payload;
        missing_direction["edges"][0]
            .as_object_mut()
            .unwrap()
            .remove("forward_amount");
        assert!(validate_projection_input(&missing_direction)
            .unwrap_err()
            .to_string()
            .contains("edges[0].forward_amount"));
    }

    #[test]
    fn net_direction_requires_source_consistent_exact_coverage() {
        let mut payload = minimal_payload(json!({
            "id": "a==b",
            "source": "a",
            "target": "b",
            "amount": 6,
            "count": 1,
            "out_amount": 10,
            "in_amount": 4,
            "forward_amount": 6,
            "reverse_amount": 0,
            "mode": "single"
        }));
        payload["request_context"] = json!({"view_mode": "net"});

        let input = validate_projection_input(&payload).unwrap();
        assert_eq!(input.edges[0].amount_cents, 600);

        let mut self_loop = payload.clone();
        self_loop["edges"][0]["target"] = json!("a");
        assert!(validate_projection_input(&self_loop)
            .unwrap_err()
            .to_string()
            .contains("cannot publish a nonzero self-loop"));

        payload["edges"][0]["source"] = json!("b");
        payload["edges"][0]["target"] = json!("a");
        assert!(validate_projection_input(&payload)
            .unwrap_err()
            .to_string()
            .contains("net direction is inconsistent"));
    }
}
