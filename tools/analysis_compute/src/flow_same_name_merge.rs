use anyhow::{anyhow, bail, Context, Result};
use serde_json::{json, Map, Value};
use std::{
    collections::{HashMap, HashSet},
    fs,
    path::PathBuf,
};

const STYLE_FIELDS: &[&str] = &[
    "stroke",
    "lineWidth",
    "edgeDash",
    "textColor",
    "fontFamily",
    "fontSize",
    "fontBold",
    "fontItalic",
    "fontUnderline",
    "fontShadow",
    "gap",
    "holePad",
    "outPad",
];

pub(crate) struct SameNameMergeArgs {
    pub(crate) payload: Value,
}

pub(crate) fn parse_args(mut iter: impl Iterator<Item = String>) -> Result<SameNameMergeArgs> {
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
                    format!("read same-name merge input {}", input_path.display())
                })?;
                payload = Some(serde_json::from_str(&raw_payload).with_context(|| {
                    format!("parse same-name merge input {}", input_path.display())
                })?);
            }
            "--help" | "-h" => {
                bail!("Usage: analytix-analysis-compute merge-flow-same-name-graph <--payload-json <json>|--input-path <path>>");
            }
            other => bail!("unsupported same-name merge flag: {other}"),
        }
    }

    Ok(SameNameMergeArgs {
        payload: payload.ok_or_else(|| anyhow!("--payload-json or --input-path is required"))?,
    })
}

pub(crate) fn merge_same_name_graph(payload: &Value) -> Value {
    let source = payload.get("source").unwrap_or(payload);
    let nodes = object_rows(source.get("nodes").or_else(|| payload.get("nodes")));
    let edges = object_rows(source.get("edges").or_else(|| payload.get("edges")));
    let mode = object_text_value(payload.get("mode"));
    let net_mode = mode.eq_ignore_ascii_case("net");
    let merged = merge_same_name_rows(nodes, edges, net_mode);

    json!({
        "mode": if net_mode { "net" } else { "gross" },
        "hasMergeTarget": merged.has_merge_target,
        "nodes": merged.nodes.into_iter().map(Value::Object).collect::<Vec<_>>(),
        "edges": merged.edges.into_iter().map(Value::Object).collect::<Vec<_>>(),
    })
}

struct MergeResult {
    nodes: Vec<Map<String, Value>>,
    edges: Vec<Map<String, Value>>,
    has_merge_target: bool,
}

fn merge_same_name_rows(
    nodes: Vec<Map<String, Value>>,
    edges: Vec<Map<String, Value>>,
    net_mode: bool,
) -> MergeResult {
    let mut groups: Vec<(String, Vec<Map<String, Value>>)> = Vec::new();
    let mut group_index: HashMap<String, usize> = HashMap::new();
    let mut loose: Vec<Map<String, Value>> = Vec::new();

    for node in nodes {
        let key = node_name_key(&node);
        if key.is_empty() || should_skip_name_merge(&node) {
            loose.push(node);
            continue;
        }
        let index = if let Some(index) = group_index.get(&key).copied() {
            index
        } else {
            let index = groups.len();
            groups.push((key.clone(), Vec::new()));
            group_index.insert(key, index);
            index
        };
        groups[index].1.push(node);
    }

    let has_merge_target = groups.iter().any(|(_, list)| list.len() > 1);
    let mut id_map: HashMap<String, String> = HashMap::new();
    let mut merged_nodes = Vec::new();

    for (key, list) in groups {
        if list.len() == 1 {
            let only = list.into_iter().next().unwrap_or_default();
            if let Some(id) = object_text_id(&only, "id") {
                id_map.insert(id.clone(), id);
            }
            merged_nodes.push(only);
            continue;
        }
        if let Some(base_id) = object_text_id(&list[0], "id") {
            for node in &list {
                if let Some(id) = object_text_id(node, "id") {
                    id_map.insert(id, base_id.clone());
                }
            }
        }
        merged_nodes.push(merge_node_group(&key, &list));
    }

    for node in loose {
        if let Some(id) = object_text_id(&node, "id") {
            id_map.insert(id.clone(), id);
        }
        let mut copy = node;
        let allow_group = boolish(copy.get("mergedDisplay"));
        normalize_node_display(&mut copy, allow_group);
        merged_nodes.push(copy);
    }

    let merged_edges = if net_mode {
        merge_edges_net(&edges, &id_map)
    } else {
        merge_edges_gross(&edges, &id_map)
    };

    MergeResult {
        nodes: merged_nodes,
        edges: merged_edges,
        has_merge_target,
    }
}

fn merge_node_group(key: &str, list: &[Map<String, Value>]) -> Map<String, Value> {
    let base = list.first().cloned().unwrap_or_default();
    let base_id = object_text_id(&base, "id").unwrap_or_default();
    let display_ids = collect_group_display_ids(list);
    let merged_display_id_raw = if display_ids.is_empty() {
        first_text(&base, &["displayIdRaw", "displayId", "display_id"])
    } else {
        display_ids.join(" / ")
    };

    let mut group_tags: Vec<Value> = Vec::new();
    let mut total_amount = 0.0;
    let mut total_count = 0.0;
    let mut max_r = 0.0;
    let mut has_seed = false;
    let mut sum_x = 0.0;
    let mut sum_y = 0.0;
    let mut pos_count = 0.0;

    for node in list {
        if let Some(tags) = node.get("groupTags").and_then(Value::as_array) {
            for tag in tags {
                if !group_tags.iter().any(|seen| seen == tag) {
                    group_tags.push(tag.clone());
                }
            }
        }
        total_amount += numeric_value(node.get("total_amount"));
        total_count += numeric_value(node.get("total_count"));
        let radius = numeric_value(node.get("r"));
        if radius > max_r {
            max_r = radius;
        }
        if node.get("ntype").and_then(Value::as_str) == Some("seed") {
            has_seed = true;
        }
        if let (Some(x), Some(y)) = (strict_number(node.get("x")), strict_number(node.get("y"))) {
            sum_x += x;
            sum_y += y;
            pos_count += 1.0;
        }
    }

    let mut merged = base.clone();
    if !base_id.is_empty() {
        merged.insert("id".to_string(), Value::String(base_id.clone()));
    }
    let name = first_non_empty_text(&[
        object_text(&base, "name"),
        object_text(&base, "title"),
        key.to_string(),
        base_id.clone(),
    ]);
    let title = first_non_empty_text(&[
        object_text(&base, "title"),
        object_text(&base, "name"),
        key.to_string(),
        base_id,
    ]);
    merged.insert("name".to_string(), Value::String(name));
    merged.insert("title".to_string(), Value::String(title));
    merged.insert(
        "displayId".to_string(),
        Value::String(format_display_id_label(
            &display_ids,
            &merged_display_id_raw,
            true,
        )),
    );
    merged.insert(
        "displayIdRaw".to_string(),
        Value::String(merged_display_id_raw),
    );
    merged.insert(
        "displayIds".to_string(),
        Value::Array(display_ids.into_iter().map(Value::String).collect()),
    );
    merged.insert("mergedDisplay".to_string(), Value::Bool(true));
    merged.insert(
        "total_amount".to_string(),
        if total_amount != 0.0 {
            json!(total_amount)
        } else {
            base.get("total_amount")
                .cloned()
                .unwrap_or_else(|| json!(0))
        },
    );
    merged.insert(
        "total_count".to_string(),
        if total_count != 0.0 {
            json!(total_count)
        } else {
            base.get("total_count").cloned().unwrap_or_else(|| json!(0))
        },
    );
    if has_seed {
        merged.insert("ntype".to_string(), Value::String("seed".to_string()));
    }
    merged.insert(
        "r".to_string(),
        if max_r != 0.0 {
            json!(max_r)
        } else {
            base.get("r").cloned().unwrap_or(Value::Null)
        },
    );
    if !group_tags.is_empty() {
        merged.insert("groupTags".to_string(), Value::Array(group_tags));
    }
    if pos_count > 0.0 {
        merged.insert("x".to_string(), json!(sum_x / pos_count));
        merged.insert("y".to_string(), json!(sum_y / pos_count));
    }
    merged
}

fn merge_edges_gross(
    edges: &[Map<String, Value>],
    id_map: &HashMap<String, String>,
) -> Vec<Map<String, Value>> {
    let mut groups: Vec<GrossEdgeAgg> = Vec::new();
    let mut index_by_key: HashMap<String, usize> = HashMap::new();

    for edge in edges {
        let source_raw = object_text(edge, "source");
        let target_raw = object_text(edge, "target");
        let source = id_map.get(&source_raw).cloned().unwrap_or(source_raw);
        let target = id_map.get(&target_raw).cloned().unwrap_or(target_raw);
        if source.is_empty() || target.is_empty() {
            continue;
        }
        let key = unordered_pair_key(&source, &target, "::");
        let index = if let Some(index) = index_by_key.get(&key).copied() {
            index
        } else {
            let index = groups.len();
            groups.push(GrossEdgeAgg::new(edge, &source, &target));
            index_by_key.insert(key, index);
            index
        };
        groups[index].accumulate(edge, &source, &target);
    }

    groups.into_iter().map(GrossEdgeAgg::into_map).collect()
}

fn merge_edges_net(
    edges: &[Map<String, Value>],
    id_map: &HashMap<String, String>,
) -> Vec<Map<String, Value>> {
    let mut groups: Vec<NetEdgeAgg> = Vec::new();
    let mut index_by_key: HashMap<String, usize> = HashMap::new();

    for edge in edges {
        let source_raw = object_text(edge, "source");
        let target_raw = object_text(edge, "target");
        let source = id_map.get(&source_raw).cloned().unwrap_or(source_raw);
        let target = id_map.get(&target_raw).cloned().unwrap_or(target_raw);
        if source.is_empty() || target.is_empty() {
            continue;
        }
        let (a, b) = sorted_pair(&source, &target);
        let key = format!("{a}::{b}");
        let index = if let Some(index) = index_by_key.get(&key).copied() {
            index
        } else {
            let index = groups.len();
            groups.push(NetEdgeAgg::new(edge, &a, &b));
            index_by_key.insert(key, index);
            index
        };
        groups[index].accumulate(edge, &source, &target);
    }

    groups
        .into_iter()
        .filter_map(NetEdgeAgg::into_map)
        .collect()
}

struct GrossEdgeAgg {
    source: String,
    target: String,
    style: Map<String, Value>,
    user_created: bool,
    forward_amount: f64,
    reverse_amount: f64,
    amount: f64,
    count: f64,
    out_amount: f64,
    in_amount: f64,
    first_time: String,
    last_time: String,
}

impl GrossEdgeAgg {
    fn new(edge: &Map<String, Value>, source: &str, target: &str) -> Self {
        Self {
            source: source.to_string(),
            target: target.to_string(),
            style: copy_style_fields(edge),
            user_created: true,
            forward_amount: 0.0,
            reverse_amount: 0.0,
            amount: 0.0,
            count: 0.0,
            out_amount: 0.0,
            in_amount: 0.0,
            first_time: String::new(),
            last_time: String::new(),
        }
    }

    fn accumulate(&mut self, edge: &Map<String, Value>, source: &str, target: &str) {
        let (forward, reverse) = edge_directional_amounts(edge);
        if self.source == source && self.target == target {
            self.forward_amount += forward;
            self.reverse_amount += reverse;
        } else {
            self.forward_amount += reverse;
            self.reverse_amount += forward;
        }
        self.amount += numeric_value(edge.get("amount"));
        self.count += numeric_value(edge.get("count"));
        self.out_amount += numeric_value(edge.get("out_amount"));
        self.in_amount += numeric_value(edge.get("in_amount"));
        self.first_time = pick_time(&self.first_time, &object_text(edge, "first_time"), true);
        self.last_time = pick_time(&self.last_time, &object_text(edge, "last_time"), false);
        self.user_created = self.user_created && boolish(edge.get("userCreated"));
    }

    fn into_map(self) -> Map<String, Value> {
        let forward = self.forward_amount;
        let reverse = self.reverse_amount;
        let has_forward = forward > 0.0;
        let has_reverse = reverse > 0.0;
        let mut edge = self.style;
        edge.insert(
            "id".to_string(),
            Value::String(format!("{}=={}", self.source, self.target)),
        );
        edge.insert("source".to_string(), Value::String(self.source.clone()));
        edge.insert("target".to_string(), Value::String(self.target.clone()));
        edge.insert("showArrow".to_string(), Value::Bool(true));
        edge.insert("userCreated".to_string(), Value::Bool(self.user_created));
        edge.insert("forward_amount".to_string(), json!(forward));
        edge.insert("reverse_amount".to_string(), json!(reverse));
        edge.insert("count".to_string(), json!(self.count));
        edge.insert("first_time".to_string(), Value::String(self.first_time));
        edge.insert("last_time".to_string(), Value::String(self.last_time));
        if has_forward && has_reverse {
            edge.insert("mode".to_string(), Value::String("double".to_string()));
            edge.insert("edgeArrow".to_string(), Value::String("both".to_string()));
            edge.insert(
                "labelTop".to_string(),
                Value::String(format!("￥{}", format_money(forward))),
            );
            edge.insert(
                "labelBottom".to_string(),
                Value::String(format!("￥{}", format_money(reverse))),
            );
            edge.insert("label".to_string(), Value::String(String::new()));
        } else {
            let amount = if has_forward { forward } else { reverse };
            edge.insert("mode".to_string(), Value::String("single".to_string()));
            edge.insert(
                "edgeArrow".to_string(),
                Value::String(if has_forward { "end" } else { "start" }.to_string()),
            );
            edge.insert(
                "label".to_string(),
                Value::String(if amount != 0.0 {
                    format!("￥{}", format_money(amount))
                } else {
                    String::new()
                }),
            );
            edge.insert("labelTop".to_string(), Value::String(String::new()));
            edge.insert("labelBottom".to_string(), Value::String(String::new()));
        }
        let edge_arrow = object_text(&edge, "edgeArrow");
        edge.insert(
            "type".to_string(),
            Value::String(
                if self.source == self.target {
                    "self-loop"
                } else {
                    "center-link"
                }
                .to_string(),
            ),
        );
        edge.insert("showArrow".to_string(), Value::Bool(edge_arrow != "none"));
        edge.insert(
            "amount".to_string(),
            json!(if forward + reverse != 0.0 {
                forward + reverse
            } else {
                self.amount
            }),
        );
        edge.insert("out_amount".to_string(), json!(forward));
        edge.insert("in_amount".to_string(), json!(reverse));
        edge
    }
}

struct NetEdgeAgg {
    a: String,
    b: String,
    style: Map<String, Value>,
    user_created: bool,
    forward_amount: f64,
    reverse_amount: f64,
    count: f64,
    first_time: String,
    last_time: String,
}

impl NetEdgeAgg {
    fn new(edge: &Map<String, Value>, a: &str, b: &str) -> Self {
        Self {
            a: a.to_string(),
            b: b.to_string(),
            style: copy_style_fields(edge),
            user_created: true,
            forward_amount: 0.0,
            reverse_amount: 0.0,
            count: 0.0,
            first_time: String::new(),
            last_time: String::new(),
        }
    }

    fn accumulate(&mut self, edge: &Map<String, Value>, source: &str, target: &str) {
        let (forward, reverse) = edge_directional_amounts(edge);
        if source == self.a && target == self.b {
            self.forward_amount += forward;
            self.reverse_amount += reverse;
        } else {
            self.forward_amount += reverse;
            self.reverse_amount += forward;
        }
        self.count += numeric_value(edge.get("count"));
        self.first_time = pick_time(&self.first_time, &object_text(edge, "first_time"), true);
        self.last_time = pick_time(&self.last_time, &object_text(edge, "last_time"), false);
        self.user_created = self.user_created && boolish(edge.get("userCreated"));
    }

    fn into_map(self) -> Option<Map<String, Value>> {
        let net = self.forward_amount - self.reverse_amount;
        if net.abs() < 1e-9 {
            return None;
        }
        let (source, target) = if net >= 0.0 {
            (self.a.clone(), self.b.clone())
        } else {
            (self.b.clone(), self.a.clone())
        };
        let amount = net.abs();
        let mut edge = self.style;
        edge.insert(
            "id".to_string(),
            Value::String(format!("{source}=={target}")),
        );
        edge.insert("source".to_string(), Value::String(source.clone()));
        edge.insert("target".to_string(), Value::String(target.clone()));
        edge.insert("showArrow".to_string(), Value::Bool(true));
        edge.insert("userCreated".to_string(), Value::Bool(self.user_created));
        edge.insert("forward_amount".to_string(), json!(amount));
        edge.insert("reverse_amount".to_string(), json!(0));
        edge.insert("amount".to_string(), json!(amount));
        edge.insert("count".to_string(), json!(self.count));
        edge.insert("out_amount".to_string(), json!(amount));
        edge.insert("in_amount".to_string(), json!(0));
        edge.insert("first_time".to_string(), Value::String(self.first_time));
        edge.insert("last_time".to_string(), Value::String(self.last_time));
        edge.insert("mode".to_string(), Value::String("single".to_string()));
        edge.insert("edgeArrow".to_string(), Value::String("end".to_string()));
        edge.insert(
            "label".to_string(),
            Value::String(format!("￥{}", format_money(amount))),
        );
        edge.insert("labelTop".to_string(), Value::String(String::new()));
        edge.insert("labelBottom".to_string(), Value::String(String::new()));
        edge.insert(
            "type".to_string(),
            Value::String(
                if source == target {
                    "self-loop"
                } else {
                    "center-link"
                }
                .to_string(),
            ),
        );
        Some(edge)
    }
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

fn object_text_id(row: &Map<String, Value>, key: &str) -> Option<String> {
    let text = object_text(row, key);
    if text.is_empty() {
        None
    } else {
        Some(text)
    }
}

fn object_text(row: &Map<String, Value>, key: &str) -> String {
    object_text_value(row.get(key))
}

fn object_text_value(value: Option<&Value>) -> String {
    value
        .and_then(|value| match value {
            Value::String(text) => Some(text.trim().to_string()),
            Value::Number(number) => Some(number.to_string()),
            Value::Bool(value) => Some(value.to_string()),
            _ => None,
        })
        .unwrap_or_default()
}

fn numeric_value(value: Option<&Value>) -> f64 {
    match value {
        Some(Value::Number(number)) => number.as_f64().unwrap_or(0.0),
        Some(Value::String(text)) => text.trim().parse::<f64>().unwrap_or(0.0),
        _ => 0.0,
    }
}

fn strict_number(value: Option<&Value>) -> Option<f64> {
    value
        .and_then(Value::as_f64)
        .filter(|number| number.is_finite())
}

fn boolish(value: Option<&Value>) -> bool {
    match value {
        Some(Value::Bool(value)) => *value,
        Some(Value::Number(number)) => number.as_f64().unwrap_or(0.0) != 0.0,
        Some(Value::String(text)) => !text.is_empty(),
        Some(Value::Null) | None => false,
        Some(_) => true,
    }
}

fn node_name_key(node: &Map<String, Value>) -> String {
    let name = object_text(node, "name");
    if !name.is_empty() {
        return name;
    }
    object_text(node, "title")
}

fn should_skip_name_merge(node: &Map<String, Value>) -> bool {
    node_name_key(node) == "未知户名" && !collect_node_display_ids(node).is_empty()
}

fn collect_group_display_ids(list: &[Map<String, Value>]) -> Vec<String> {
    let mut display_ids = Vec::new();
    let mut seen = HashSet::new();
    for node in list {
        for id in collect_node_display_ids(node) {
            if seen.insert(id.clone()) {
                display_ids.push(id);
            }
        }
    }
    display_ids
}

fn collect_node_display_ids(node: &Map<String, Value>) -> Vec<String> {
    let mut out = Vec::new();
    let mut seen = HashSet::new();
    for key in [
        "displayIds",
        "display_ids",
        "displayIdRaw",
        "display_id_raw",
        "display_id",
        "displayId",
    ] {
        let Some(value) = node.get(key) else {
            continue;
        };
        for id in normalize_display_ids(value) {
            if seen.insert(id.clone()) {
                out.push(id);
            }
        }
    }
    out
}

fn normalize_node_display(node: &mut Map<String, Value>, allow_group: bool) {
    let ids = collect_node_display_ids(node);
    if ids.is_empty() {
        return;
    }
    let raw = ids.join(" / ");
    node.insert(
        "displayIds".to_string(),
        Value::Array(ids.iter().cloned().map(Value::String).collect()),
    );
    node.insert("displayIdRaw".to_string(), Value::String(raw.clone()));
    node.insert(
        "displayId".to_string(),
        Value::String(format_display_id_label(&ids, &raw, allow_group)),
    );
}

fn normalize_display_ids(value: &Value) -> Vec<String> {
    let raw_items: Vec<String> = match value {
        Value::Array(items) => items
            .iter()
            .map(|item| object_text_value(Some(item)))
            .collect(),
        _ => parse_display_id_list(&object_text_value(Some(value))),
    };
    let mut out = Vec::new();
    let mut seen = HashSet::new();
    for item in raw_items {
        let stripped = strip_display_id_prefix(&item);
        if stripped.is_empty() || is_unknown_account_label(&stripped) {
            continue;
        }
        if seen.insert(stripped.clone()) {
            out.push(stripped);
        }
    }
    out
}

fn parse_display_id_list(raw: &str) -> Vec<String> {
    let text = raw.trim();
    if text.is_empty() || text.contains("账户合集") || is_unknown_account_label(text) {
        return Vec::new();
    }
    let cleaned = strip_trailing_group_suffix(text);
    cleaned
        .split(|ch| ch == '/' || ch == '／')
        .map(strip_display_id_prefix)
        .filter(|item| !item.is_empty())
        .collect()
}

fn strip_trailing_group_suffix(text: &str) -> String {
    if !text.ends_with('个') {
        return text.to_string();
    }
    let Some(index) = text.rfind('等') else {
        return text.to_string();
    };
    let digits = &text[index + '等'.len_utf8()..text.len() - '个'.len_utf8()];
    if !digits.is_empty() && digits.chars().all(|ch| ch.is_ascii_digit()) {
        text[..index].trim().to_string()
    } else {
        text.to_string()
    }
}

fn strip_display_id_prefix(value: &str) -> String {
    let trimmed = value.trim();
    for prefix in ["卡号", "账号", "账户"] {
        if let Some(rest) = trimmed.strip_prefix(prefix) {
            let rest = rest.trim_start();
            if let Some(rest) = rest.strip_prefix(':').or_else(|| rest.strip_prefix('：')) {
                return rest.trim().to_string();
            }
        }
    }
    trimmed.to_string()
}

fn is_unknown_account_label(text: &str) -> bool {
    ["未知卡号", "账号未知", "未知账号"]
        .iter()
        .any(|label| text.contains(label))
}

fn format_display_id_label(list: &[String], fallback: &str, allow_group: bool) -> String {
    let fallback_text = fallback.trim();
    if !list.is_empty() {
        if allow_group && list.len() > 1 {
            return format!("账户合集（{}）", list.len());
        }
        if list.len() == 1 {
            return list[0].clone();
        }
        if !fallback_text.is_empty() && !fallback_text.contains("账户合集") {
            return fallback_text.to_string();
        }
        return list.join(" / ");
    }
    if !allow_group && fallback_text.contains("账户合集") {
        return String::new();
    }
    fallback_text.to_string()
}

fn edge_directional_amounts(edge: &Map<String, Value>) -> (f64, f64) {
    let forward = numeric_value(edge.get("forward_amount")).abs();
    let reverse = numeric_value(edge.get("reverse_amount")).abs();
    if forward != 0.0 || reverse != 0.0 {
        return (forward, reverse);
    }
    let top = parse_amount_from_label(&object_text(edge, "labelTop"));
    let bottom = parse_amount_from_label(&object_text(edge, "labelBottom"));
    if top != 0.0 || bottom != 0.0 {
        return (top, bottom);
    }
    let arrow = first_non_empty_text(&[
        object_text(edge, "edgeArrow"),
        object_text(edge, "arrow"),
        if boolish(edge.get("showArrow")) {
            "end".to_string()
        } else {
            "none".to_string()
        },
    ]);
    let flip = arrow == "start";
    let amount = numeric_value(edge.get("amount")).abs();
    if amount != 0.0 {
        return if flip { (0.0, amount) } else { (amount, 0.0) };
    }
    let out_amount = numeric_value(edge.get("out_amount")).abs();
    let in_amount = numeric_value(edge.get("in_amount")).abs();
    if out_amount != 0.0 || in_amount != 0.0 {
        return if flip {
            (in_amount, out_amount)
        } else {
            (out_amount, in_amount)
        };
    }
    let label_amount = parse_amount_from_label(&object_text(edge, "label"));
    if label_amount != 0.0 {
        return if flip {
            (0.0, label_amount)
        } else {
            (label_amount, 0.0)
        };
    }
    let count = numeric_value(edge.get("count")).abs();
    if flip {
        (0.0, count)
    } else {
        (count, 0.0)
    }
}

fn parse_amount_from_label(text: &str) -> f64 {
    let chars: Vec<char> = text.chars().collect();
    let mut values = Vec::new();
    let mut index = 0;
    while index < chars.len() {
        let start = chars[index];
        let next_is_digit = chars
            .get(index + 1)
            .map(|ch| ch.is_ascii_digit())
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
        let token: String = chars[index..end].iter().filter(|ch| **ch != ',').collect();
        if let Ok(number) = token.parse::<f64>() {
            if number.is_finite() {
                values.push(number.abs());
            }
        }
        index = end;
    }
    values.into_iter().fold(0.0, f64::max)
}

fn format_money(value: f64) -> String {
    let rounded = (value * 100.0).round() / 100.0;
    let raw = format!("{rounded:.2}");
    let mut parts = raw.split('.').collect::<Vec<_>>();
    let integer = parts.remove(0);
    let mut grouped = String::new();
    for (index, ch) in integer.chars().rev().enumerate() {
        if index > 0 && index % 3 == 0 {
            grouped.push(',');
        }
        grouped.push(ch);
    }
    let mut out = grouped.chars().rev().collect::<String>();
    if let Some(fraction) = parts.first() {
        let trimmed = fraction.trim_end_matches('0');
        if !trimmed.is_empty() {
            out.push('.');
            out.push_str(trimmed);
        }
    }
    out
}

fn pick_time(current: &str, next: &str, pick_min: bool) -> String {
    let a = current.trim();
    let b = next.trim();
    if a.is_empty() {
        return b.to_string();
    }
    if b.is_empty() {
        return a.to_string();
    }
    if (pick_min && a <= b) || (!pick_min && a >= b) {
        a.to_string()
    } else {
        b.to_string()
    }
}

fn copy_style_fields(edge: &Map<String, Value>) -> Map<String, Value> {
    let mut out = Map::new();
    for key in STYLE_FIELDS {
        if let Some(value) = edge.get(*key) {
            out.insert((*key).to_string(), value.clone());
        }
    }
    out
}

fn unordered_pair_key(a: &str, b: &str, sep: &str) -> String {
    let (left, right) = sorted_pair(a, b);
    format!("{left}{sep}{right}")
}

fn sorted_pair(a: &str, b: &str) -> (String, String) {
    if a <= b {
        (a.to_string(), b.to_string())
    } else {
        (b.to_string(), a.to_string())
    }
}

fn first_text(row: &Map<String, Value>, keys: &[&str]) -> String {
    for key in keys {
        let text = object_text(row, key);
        if !text.is_empty() {
            return text;
        }
    }
    String::new()
}

fn first_non_empty_text(values: &[String]) -> String {
    values
        .iter()
        .map(|value| value.trim())
        .find(|value| !value.is_empty())
        .unwrap_or("")
        .to_string()
}

#[cfg(test)]
mod tests {
    use super::merge_same_name_graph;
    use serde_json::{json, Value};

    fn graph(payload: Value) -> Value {
        merge_same_name_graph(&payload)
    }

    #[test]
    fn merges_same_name_nodes_and_preserves_bidirectional_amounts() {
        let result = graph(json!({
            "mode": "gross",
            "nodes": [
                {"id": "a1", "name": "张三", "display_id": "账号: 1001", "total_amount": 10, "total_count": 1, "r": 3, "x": 0, "y": 0, "groupTags": ["g1"]},
                {"id": "a2", "name": "张三", "displayId": "1002", "total_amount": 20, "total_count": 2, "r": 5, "x": 10, "y": 20, "ntype": "seed", "groupTags": ["g1", "g2"]},
                {"id": "b", "name": "李四", "displayId": "2001"},
                {"id": "unknown", "name": "未知户名", "displayId": "3001"}
            ],
            "edges": [
                {"source": "a1", "target": "b", "amount": 100, "first_time": "2024-02", "last_time": "2024-02", "userCreated": true},
                {"source": "b", "target": "a2", "amount": 40, "first_time": "2024-01", "last_time": "2024-03", "userCreated": true}
            ]
        }));

        assert_eq!(result["hasMergeTarget"], true);
        assert_eq!(result["nodes"][0]["id"], "a1");
        assert_eq!(result["nodes"][0]["displayId"], "账户合集（2）");
        assert_eq!(result["nodes"][0]["displayIds"], json!(["1001", "1002"]));
        assert_eq!(result["nodes"][0]["total_amount"], json!(30.0));
        assert_eq!(result["nodes"][0]["total_count"], json!(3.0));
        assert_eq!(result["nodes"][0]["ntype"], "seed");
        assert_eq!(result["nodes"][0]["r"], json!(5.0));
        assert_eq!(result["nodes"][0]["x"], json!(5.0));
        assert_eq!(result["nodes"][0]["y"], json!(10.0));
        assert_eq!(result["nodes"][0]["groupTags"], json!(["g1", "g2"]));
        assert_eq!(result["nodes"][2]["id"], "unknown");
        assert_eq!(result["edges"][0]["source"], "a1");
        assert_eq!(result["edges"][0]["target"], "b");
        assert_eq!(result["edges"][0]["mode"], "double");
        assert_eq!(result["edges"][0]["edgeArrow"], "both");
        assert_eq!(result["edges"][0]["labelTop"], "￥100");
        assert_eq!(result["edges"][0]["labelBottom"], "￥40");
        assert_eq!(result["edges"][0]["first_time"], "2024-01");
        assert_eq!(result["edges"][0]["last_time"], "2024-03");
    }

    #[test]
    fn nets_same_name_edges_and_drops_zero_balance() {
        let result = graph(json!({
            "mode": "net",
            "nodes": [
                {"id": "a1", "name": "张三"},
                {"id": "a2", "name": "张三"},
                {"id": "b", "name": "李四"},
                {"id": "c", "name": "王五"}
            ],
            "edges": [
                {"source": "a1", "target": "b", "amount": 100, "count": 1},
                {"source": "b", "target": "a2", "amount": 40, "count": 2},
                {"source": "a1", "target": "c", "amount": 50},
                {"source": "c", "target": "a2", "amount": 50}
            ]
        }));

        assert_eq!(result["mode"], "net");
        assert_eq!(result["edges"].as_array().unwrap().len(), 1);
        assert_eq!(result["edges"][0]["source"], "a1");
        assert_eq!(result["edges"][0]["target"], "b");
        assert_eq!(result["edges"][0]["amount"], json!(60.0));
        assert_eq!(result["edges"][0]["label"], "￥60");
        assert_eq!(result["edges"][0]["mode"], "single");
    }
}
