use icu_collator::{options::CollatorOptions, Collator};
use icu_locale_core::locale;
use serde_json::{json, Value};
use std::collections::BTreeMap;

use super::value_helpers::{numeric_json, raw_text, text_value};

#[derive(Clone, Debug)]
struct EdgeOffsetMember {
    index: usize,
    id: String,
}

pub(super) fn project_edge_offset_updates(edges: &[Value]) -> Vec<Value> {
    if edges.len() < 2 {
        return Vec::new();
    }
    let mut updates: Vec<(usize, String, f64)> = Vec::new();
    let mut groups: BTreeMap<String, Vec<EdgeOffsetMember>> = BTreeMap::new();
    for (index, edge) in edges.iter().enumerate() {
        let source = text_value(edge.get("source"));
        let target = text_value(edge.get("target"));
        if source.is_empty() || target.is_empty() {
            continue;
        }
        let id = raw_text(edge.get("id"));
        if source == target {
            updates.push((index, id, 0.0));
            continue;
        }
        let key = if source < target {
            format!("{source}::{target}")
        } else {
            format!("{target}::{source}")
        };
        groups
            .entry(key)
            .or_default()
            .push(EdgeOffsetMember { index, id });
    }

    let collator = Collator::try_new(locale!("zh-CN").into(), CollatorOptions::default()).ok();
    for (_, mut members) in groups {
        if members.len() == 1 {
            let member = members.remove(0);
            updates.push((member.index, member.id, 0.0));
            continue;
        }
        members.sort_by(|left, right| match &collator {
            Some(collator) => collator
                .compare(left.id.as_str(), right.id.as_str())
                .then_with(|| left.index.cmp(&right.index)),
            None => left
                .id
                .cmp(&right.id)
                .then_with(|| left.index.cmp(&right.index)),
        });
        let gap = 12.0;
        let mid = (members.len() as f64 - 1.0) / 2.0;
        for (order_index, member) in members.into_iter().enumerate() {
            updates.push((member.index, member.id, (order_index as f64 - mid) * gap));
        }
    }

    updates.sort_by(|left, right| left.0.cmp(&right.0));
    updates
        .into_iter()
        .map(|(index, id, edge_offset)| {
            json!({
                "index": index,
                "id": id,
                "edgeOffset": numeric_json(edge_offset),
            })
        })
        .collect()
}
