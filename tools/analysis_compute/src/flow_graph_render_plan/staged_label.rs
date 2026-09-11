use serde_json::{json, Map, Value};
use std::collections::{HashMap, HashSet};

use super::node_visibility::{GraphTopologyIndex, NodeRenderModeProjection, RenderNodeFacts};
use super::value_helpers::{bool_field, compare_f64_desc, js_number_or, raw_text, text_value};

#[derive(Clone, Debug)]
struct StagedLabelEntry {
    id: String,
    index: usize,
    degree: i64,
    amount: f64,
    count: f64,
    keep_rank: i64,
    keep_subtitle: bool,
}

#[derive(Clone, Debug)]
struct StagedLabelBudget {
    t0_limit: usize,
    reveal_batch_size: usize,
    reveal_delay: usize,
    batch_gap_ms: usize,
    intro_duration: usize,
}

pub(super) fn project_staged_label_plan(
    contract: &Value,
    nodes: &[Value],
    node_facts: &[RenderNodeFacts],
    edges: &[Value],
    topology: &GraphTopologyIndex,
    ctx: &Value,
    hints: &Value,
    node_mode_rows: &[NodeRenderModeProjection],
    node_render_updates: &[Value],
    edge_render_updates: &[Value],
    staged_focus_ids: &[String],
) -> Value {
    if !staged_label_enabled(contract)
        || !should_use_staged_label_mode(nodes.len(), edges.len(), hints)
    {
        return Value::Null;
    }
    let budget = resolve_staged_label_budget(nodes.len(), hints);
    let mode_by_index = node_mode_rows
        .iter()
        .map(|row| (row.index, row.mode.clone()))
        .collect::<HashMap<_, _>>();
    let node_update_by_index = node_render_updates
        .iter()
        .enumerate()
        .map(|(index, update)| (index, update))
        .collect::<HashMap<_, _>>();
    let focus_ids = staged_focus_ids
        .iter()
        .filter(|id| !id.is_empty())
        .cloned()
        .collect::<HashSet<_>>();
    let focus_name = text_value(ctx.get("focusName"));
    let mut entries = node_facts
        .iter()
        .filter_map(|node| {
            if node.id.is_empty() {
                return None;
            }
            let render_mode = mode_by_index
                .get(&node.index)
                .cloned()
                .unwrap_or_else(|| node.node_render_mode.clone());
            if render_mode == "dot" {
                return None;
            }
            let degree = topology.degree(&node.id);
            let amount = node.amount;
            let count = node.count;
            let is_focus = focus_ids.contains(&node.id);
            let is_seed = node.ntype.as_str() == "seed";
            let is_core = node.role.as_str() == "core";
            let is_focus_name = !focus_name.is_empty()
                && (node.name.as_str() == focus_name.as_str()
                    || node.title.as_str() == focus_name.as_str());
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
            Some(StagedLabelEntry {
                id: node.id.clone(),
                index: node.index,
                degree,
                amount,
                count,
                keep_rank,
                keep_subtitle: is_focus || is_seed || is_core,
            })
        })
        .collect::<Vec<_>>();
    if entries.is_empty() {
        return Value::Null;
    }
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

    let forced_count = entries.iter().filter(|entry| entry.keep_rank > 0).count();
    let t0_count = entries.len().min(budget.t0_limit.max(forced_count));
    let t0_ids = entries
        .iter()
        .take(t0_count)
        .map(|entry| entry.id.clone())
        .collect::<HashSet<_>>();
    let keep_subtitle_ids = entries
        .iter()
        .filter(|entry| entry.keep_subtitle)
        .map(|entry| entry.id.clone())
        .collect::<HashSet<_>>();
    let entry_by_id = entries
        .iter()
        .map(|entry| (entry.id.clone(), entry.clone()))
        .collect::<HashMap<_, _>>();

    let mut deferred = Vec::new();
    let mut node_label_updates = Vec::new();
    let mut edge_label_updates = Vec::new();
    let mut hidden_node_count = 0usize;
    let mut suppressed_subtitle_count = 0usize;
    let mut hidden_edge_label_count = 0usize;
    let mut tier_hidden_node_count = 0usize;
    let mut initial_node_label_rows = 0usize;

    for node_fact in node_facts {
        if node_fact.id.is_empty() {
            continue;
        }
        let node = nodes.get(node_fact.index).unwrap_or(&Value::Null);
        let Some(entry) = entry_by_id.get(&node_fact.id) else {
            tier_hidden_node_count += 1;
            continue;
        };
        let visible = t0_ids.contains(&node_fact.id);
        let had_hidden = node_update_by_index
            .get(&node_fact.index)
            .map(|update| bool_field(update.get("nodeLabelHidden")))
            .unwrap_or_else(|| bool_field(node.get("nodeLabelHidden")));
        let next_hidden = !visible;
        let had_name = !node_fact.name.is_empty();
        let had_display_id = !node_fact.display_id.is_empty();
        let suppress_subtitle =
            visible && had_name && had_display_id && !keep_subtitle_ids.contains(&node_fact.id);
        if next_hidden {
            hidden_node_count += 1;
        } else if suppress_subtitle {
            suppressed_subtitle_count += 1;
        }
        if !next_hidden {
            if had_name {
                initial_node_label_rows += if suppress_subtitle {
                    1
                } else {
                    1 + usize::from(had_display_id)
                };
            } else if had_display_id || !node_fact.title.is_empty() || !node_fact.id.is_empty() {
                initial_node_label_rows += 1;
            }
        }
        if next_hidden != had_hidden || suppress_subtitle {
            let mut update = Map::new();
            update.insert("index".to_string(), json!(node_fact.index));
            update.insert("id".to_string(), json!(node_fact.id.clone()));
            update.insert("nodeLabelHidden".to_string(), json!(next_hidden));
            if suppress_subtitle {
                update.insert("displayId".to_string(), json!(""));
                update.insert("display_id".to_string(), json!(""));
            }
            node_label_updates.push(Value::Object(update));

            let mut restore = Map::new();
            restore.insert("id".to_string(), json!(entry.id.clone()));
            restore.insert("nodeLabelHidden".to_string(), json!(had_hidden));
            if let Some(value) = node.get("displayId") {
                restore.insert("displayId".to_string(), value.clone());
            }
            if let Some(value) = node.get("display_id") {
                restore.insert("display_id".to_string(), value.clone());
            }
            deferred.push(Value::Object(restore));
        }
    }

    if !matches!(hints.get("show_edge_labels"), Some(Value::Bool(false))) {
        for (index, edge) in edges.iter().enumerate() {
            let id = text_value(edge.get("id"));
            if id.is_empty() {
                continue;
            }
            let update = edge_render_updates.get(index).unwrap_or(edge);
            let base_label = raw_text(update.get("label"));
            let base_top = raw_text(update.get("labelTop"));
            let base_bottom = raw_text(update.get("labelBottom"));
            if base_label.is_empty() && base_top.is_empty() && base_bottom.is_empty() {
                continue;
            }
            deferred.push(json!({
                "kind": "edge",
                "id": id,
                "label": base_label,
                "labelTop": base_top,
                "labelBottom": base_bottom,
                "detailLabel": bool_field(update.get("detailLabel")),
            }));
            edge_label_updates.push(json!({
                "index": index,
                "id": text_value(edge.get("id")),
                "label": "",
                "labelTop": "",
                "labelBottom": "",
                "detailLabel": false,
            }));
            hidden_edge_label_count += 1;
        }
    }

    if hidden_node_count == 0 && suppressed_subtitle_count == 0 && hidden_edge_label_count == 0 {
        return Value::Null;
    }

    json!({
        "enabled": true,
        "nodeCount": nodes.len(),
        "edgeCount": edges.len(),
        "t0Count": entries.len().saturating_sub(hidden_node_count),
        "deferredCount": deferred.len(),
        "deferredNodeCount": node_label_updates.len(),
        "deferredEdgeCount": edge_label_updates.len(),
        "hiddenNodeCount": hidden_node_count,
        "tierHiddenNodeCount": tier_hidden_node_count,
        "suppressedSubtitleCount": suppressed_subtitle_count,
        "hiddenEdgeLabelCount": hidden_edge_label_count,
        "initialLabelRows": initial_node_label_rows,
        "initialNodeLabelRows": initial_node_label_rows,
        "initialEdgeLabelRows": 0,
        "budget": {
            "t0Limit": budget.t0_limit,
            "revealBatchSize": budget.reveal_batch_size,
            "revealDelay": budget.reveal_delay,
            "batchGapMs": budget.batch_gap_ms,
            "introDuration": budget.intro_duration,
        },
        "nodeLabelUpdates": node_label_updates,
        "edgeLabelUpdates": edge_label_updates,
        "deferred": deferred,
    })
}

fn staged_label_enabled(contract: &Value) -> bool {
    let value = contract
        .get("stagedLabel")
        .or_else(|| contract.get("staged_label"));
    match value {
        Some(Value::Object(object)) => object
            .get("enabled")
            .or_else(|| object.get("enable"))
            .is_some_and(|value| bool_field(Some(value))),
        Some(_) => bool_field(value),
        None => false,
    }
}

fn should_use_staged_label_mode(node_count: usize, edge_count: usize, hints: &Value) -> bool {
    if node_count == 0 {
        return false;
    }
    if js_number_or(hints.get("t0_label_limit"), 0.0) == 0.0 || node_count > 100000 {
        return false;
    }
    let default_node_mode = text_value(hints.get("default_node_mode")).to_ascii_lowercase();
    if node_count < 180 && default_node_mode == "entity" {
        return false;
    }
    let tier = text_value(hints.get("tier")).to_ascii_lowercase();
    if edge_count > 32000 && tier != "large" {
        return false;
    }
    true
}

fn resolve_staged_label_budget(node_count: usize, hints: &Value) -> StagedLabelBudget {
    let count = node_count;
    let (default_t0, default_batch, default_delay, default_gap, default_intro): (
        usize,
        usize,
        usize,
        usize,
        usize,
    ) = if count <= 400 {
        (count.min(180), 84, 120, 16, 200)
    } else if count <= 1200 {
        (240, 120, 140, 18, 220)
    } else if count <= 3000 {
        (320, 160, 160, 20, 240)
    } else if count <= 6000 {
        (420, 220, 180, 22, 260)
    } else {
        (560, 260, 220, 24, 300)
    };
    let animation_mode = text_value(hints.get("animation_mode")).to_ascii_lowercase();
    let intro_duration = if animation_mode == "full" {
        default_intro.max(220)
    } else if animation_mode == "minimal" {
        140.max(default_intro.saturating_sub(60))
    } else {
        default_intro
    };
    StagedLabelBudget {
        t0_limit: count
            .min(js_number_or(hints.get("t0_label_limit"), default_t0 as f64).max(0.0) as usize),
        reveal_batch_size: (js_number_or(hints.get("t1_reveal_batch_size"), default_batch as f64)
            .max(24.0)) as usize,
        reveal_delay: (js_number_or(hints.get("t1_reveal_delay_ms"), default_delay as f64).max(0.0))
            as usize,
        batch_gap_ms: (js_number_or(hints.get("t1_batch_gap_ms"), default_gap as f64).max(8.0))
            as usize,
        intro_duration,
    }
}
