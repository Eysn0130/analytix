use serde_json::{json, Value};
use std::collections::{BTreeSet, HashMap, HashSet};

use super::render_hints::normalize_graph_tier;
use super::value_helpers::{
    compare_f64_desc, first_truthy_text, first_truthy_value, js_truthy, number_field, numeric_json,
    parse_unique_list, text_value,
};

#[derive(Clone, Debug, PartialEq)]
pub(super) struct RankingEntry {
    pub(super) id: String,
    pub(super) index: usize,
    degree: i64,
    amount: f64,
    count: f64,
    keep_rank: i64,
    is_focus: bool,
    is_seed: bool,
    is_core: bool,
}

pub(super) struct RenderProjection {
    pub(super) summary: Value,
    pub(super) node_render_modes: Vec<Value>,
    pub(super) node_mode_rows: Vec<NodeRenderModeProjection>,
}

#[derive(Clone, Debug)]
pub(super) struct NodeRenderModeProjection {
    pub(super) index: usize,
    pub(super) id: String,
    pub(super) mode: String,
}

#[derive(Clone, Debug)]
pub(super) struct RenderNodeFacts {
    pub(super) index: usize,
    pub(super) id: String,
    pub(super) title: String,
    pub(super) name: String,
    pub(super) display_id: String,
    pub(super) node_render_mode: String,
    pub(super) ntype: String,
    pub(super) role: String,
    pub(super) amount: f64,
    pub(super) count: f64,
    has_projection_visibility: bool,
}

pub(super) struct NodeVisibilityRanking {
    pub(super) entries: Vec<RankingEntry>,
    pub(super) focus_ids: BTreeSet<String>,
}

#[derive(Clone, Debug)]
pub(super) struct GraphTopologyIndex {
    degree_by_node: HashMap<String, i64>,
}

impl GraphTopologyIndex {
    pub(super) fn from_edges(edges: &[Value]) -> Self {
        let mut degree_by_node = HashMap::new();
        for edge in edges {
            let source = text_value(edge.get("source"));
            let target = text_value(edge.get("target"));
            if source.is_empty() || target.is_empty() {
                continue;
            }
            *degree_by_node.entry(source.clone()).or_insert(0) += 1;
            *degree_by_node.entry(target.clone()).or_insert(0) += 1;
        }
        Self { degree_by_node }
    }

    pub(super) fn degree(&self, node_id: &str) -> i64 {
        self.degree_by_node.get(node_id).copied().unwrap_or(0)
    }
}

pub(super) fn collect_render_node_facts(nodes: &[Value]) -> Vec<RenderNodeFacts> {
    nodes
        .iter()
        .enumerate()
        .map(|(index, node)| RenderNodeFacts {
            index,
            id: text_value(node.get("id")),
            title: first_truthy_text(&[node.get("title"), node.get("label")]),
            name: text_value(node.get("name")),
            display_id: first_truthy_text(&[node.get("displayId"), node.get("display_id")]),
            node_render_mode: text_value(node.get("nodeRenderMode")).to_ascii_lowercase(),
            ntype: text_value(node.get("ntype")).to_ascii_lowercase(),
            role: text_value(node.get("role")).to_ascii_lowercase(),
            amount: node_amount_hint(node),
            count: node_count_hint(node),
            has_projection_visibility: js_truthy(node.get("cluster_node"))
                || js_truthy(node.get("projection_visible")),
        })
        .collect()
}

pub(super) fn project_graph_tier_render_plan(
    node_count: usize,
    node_facts: &[RenderNodeFacts],
    ctx: &Value,
    hints: &Value,
    ranking: &NodeVisibilityRanking,
) -> RenderProjection {
    let tier = normalize_graph_tier(
        first_truthy_text(&[ctx.get("graphTier"), hints.get("tier")]),
        node_count,
    );
    let projection_mode = text_value(
        ctx.get("graphProjection")
            .and_then(|value| value.get("mode")),
    )
    .to_ascii_lowercase();
    let preserve_projection_modes = projection_mode == "skeleton"
        || node_facts.iter().any(|node| node.has_projection_visibility);
    let mut entity_ids = HashSet::new();
    if matches!(hints.get("default_node_mode"), Some(Value::String(mode)) if mode == "entity") {
        for node in node_facts {
            if !node.id.is_empty() {
                entity_ids.insert(node.id.clone());
            }
        }
    } else {
        let forced = ranking
            .entries
            .iter()
            .filter(|entry| entry.keep_rank > 0)
            .map(|entry| entry.id.clone())
            .collect::<Vec<_>>();
        let limit = forced
            .len()
            .max(number_field(hints.get("focus_entity_limit")).unwrap_or(0.0) as usize)
            .max(number_field(hints.get("entity_node_limit")).unwrap_or(0.0) as usize);
        for entry in ranking.entries.iter().take(limit) {
            entity_ids.insert(entry.id.clone());
        }
        for id in forced {
            entity_ids.insert(id);
        }
    }

    let mut entity_count = 0usize;
    let mut focus_count = 0usize;
    let mut node_render_modes = Vec::new();
    let mut node_mode_rows = Vec::new();
    for node in node_facts {
        let id = node.id.clone();
        let explicit_mode = if preserve_projection_modes && !ranking.focus_ids.contains(&id) {
            node.node_render_mode.clone()
        } else {
            String::new()
        };
        let mode = if ranking.focus_ids.contains(&id) {
            "focus".to_string()
        } else if matches!(explicit_mode.as_str(), "dot" | "entity" | "focus") {
            explicit_mode
        } else if entity_ids.contains(&id) {
            "entity".to_string()
        } else {
            "dot".to_string()
        };
        if mode == "focus" {
            focus_count += 1;
        }
        if mode != "dot" {
            entity_count += 1;
        }
        node_mode_rows.push(NodeRenderModeProjection {
            index: node.index,
            id: id.clone(),
            mode: mode.clone(),
        });
        node_render_modes.push(json!({
            "id": id,
            "nodeRenderMode": mode,
        }));
    }

    RenderProjection {
        summary: json!({
            "tier": tier,
            "entityCount": entity_count,
            "dotCount": node_count.saturating_sub(entity_count),
            "focusCount": focus_count,
        }),
        node_render_modes,
        node_mode_rows,
    }
}

pub(super) fn build_node_visibility_ranking(
    node_facts: &[RenderNodeFacts],
    topology: &GraphTopologyIndex,
    ctx: &Value,
    staged_focus_ids: &[String],
) -> NodeVisibilityRanking {
    let focus_ids = collect_tier_focus_ids(ctx, staged_focus_ids);
    let focus_name = first_truthy_text(&[ctx.get("focusName"), ctx.get("focusLabel")]);
    let mut entries = node_facts
        .iter()
        .filter_map(|node| {
            if node.id.is_empty() {
                return None;
            }
            let degree = topology.degree(&node.id);
            let amount = node.amount.max(0.0);
            let count = node.count.max(0.0);
            let is_seed = node.ntype.as_str() == "seed";
            let is_core = node.role.as_str() == "core";
            let is_focus = focus_ids.contains(&node.id);
            let is_focus_name = !focus_name.is_empty()
                && (focus_name.as_str() == node.name.as_str()
                    || focus_name.as_str() == node.title.as_str()
                    || focus_name.as_str() == node.display_id.as_str());
            let keep_rank = if is_focus {
                6
            } else if is_seed {
                5
            } else if is_core {
                4
            } else if is_focus_name {
                3
            } else {
                0
            };
            Some(RankingEntry {
                id: node.id.clone(),
                index: node.index,
                degree,
                amount,
                count,
                keep_rank,
                is_focus,
                is_seed,
                is_core,
            })
        })
        .collect::<Vec<_>>();

    entries.sort_by(|left, right| {
        right
            .keep_rank
            .cmp(&left.keep_rank)
            .then_with(|| right.degree.cmp(&left.degree))
            .then_with(|| compare_f64_desc(left.amount, right.amount))
            .then_with(|| compare_f64_desc(left.count, right.count))
            .then_with(|| left.index.cmp(&right.index))
            .then_with(|| left.id.cmp(&right.id))
    });

    NodeVisibilityRanking { entries, focus_ids }
}

pub(super) fn render_ranking_entry(entry: &RankingEntry) -> Value {
    json!({
        "id": entry.id,
        "index": entry.index,
        "degree": entry.degree,
        "amount": numeric_json(entry.amount),
        "count": numeric_json(entry.count),
        "keepRank": entry.keep_rank,
        "isFocus": entry.is_focus,
        "isSeed": entry.is_seed,
        "isCore": entry.is_core,
    })
}

fn collect_tier_focus_ids(ctx: &Value, staged_focus_ids: &[String]) -> BTreeSet<String> {
    let mut out = BTreeSet::new();
    for id in staged_focus_ids {
        if !id.is_empty() {
            out.insert(id.clone());
        }
    }
    for key in ["focusIds", "leftSeeds", "selected"] {
        for id in parse_unique_list(ctx.get(key), true) {
            out.insert(id);
        }
    }
    let focus_id = text_value(ctx.get("focusId"));
    if !focus_id.is_empty() {
        out.insert(focus_id);
    }
    out
}

fn node_amount_hint(node: &Value) -> f64 {
    let mut out = 0.0;
    for key in [
        "total_amount",
        "totalAmount",
        "amount",
        "total_amt",
        "totalAmt",
        "weight",
    ] {
        let num = number_field(node.get(key)).unwrap_or(0.0).abs();
        if num > out {
            out = num;
        }
    }
    out
}

fn node_count_hint(node: &Value) -> f64 {
    number_field(first_truthy_value(&[
        node.get("total_count"),
        node.get("count"),
    ]))
    .unwrap_or(0.0)
    .abs()
}
