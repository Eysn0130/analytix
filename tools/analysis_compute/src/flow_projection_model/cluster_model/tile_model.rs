use anyhow::Result;
use serde_json::{json, Value};
use sha1::{Digest, Sha1};
use std::collections::BTreeMap;

use super::super::{
    helpers::sum_projection_node_totals,
    input_model::{money_value, ProjectionNodeFacts},
    materialization_plan::rank_projection_materialize_node_ids,
};

const CLUSTER_NODE_PREFIX: &str = "__cluster__::";

pub(super) fn cluster_node_id(anchor_id: &str, member_ids: &[String]) -> String {
    let anchor = if anchor_id.trim().is_empty() {
        "anchor"
    } else {
        anchor_id.trim()
    };
    let mut members: Vec<String> = member_ids
        .iter()
        .map(|item| item.trim().to_string())
        .filter(|item| !item.is_empty())
        .collect();
    members.sort();
    let seed = format!("{anchor}|{}", members.join("|"));
    let digest = Sha1::digest(seed.as_bytes());
    let hex = format!("{digest:x}");
    format!("{CLUSTER_NODE_PREFIX}{anchor}::{}", &hex[..16])
}

pub(super) fn build_projection_cluster_tiles(
    cluster_id: &str,
    member_ids: &[String],
    node_map: &BTreeMap<String, Value>,
    node_facts: &BTreeMap<String, ProjectionNodeFacts>,
    degree_map: &BTreeMap<String, i64>,
    tile_size: usize,
) -> Result<(Vec<String>, Vec<Value>)> {
    let ranked_member_ids =
        rank_projection_materialize_node_ids(member_ids, node_map, node_facts, degree_map)?;
    let step = tile_size.max(1);
    let mut tiles = Vec::new();
    for chunk in ranked_member_ids.chunks(step) {
        let tile_index = tiles.len();
        let chunk_ids: Vec<String> = chunk.to_vec();
        let totals = sum_projection_node_totals(&chunk_ids, node_facts)?;
        tiles.push(json!({
            "tile_id": projection_cluster_tile_id(cluster_id, tile_index),
            "cluster_id": cluster_id.trim(),
            "member_ids": chunk_ids,
            "member_count": chunk.len(),
            "total_amount": money_value(totals.total_amount_cents),
            "total_count": totals.total_count,
        }));
    }
    Ok((ranked_member_ids, tiles))
}

fn projection_cluster_tile_id(cluster_id: &str, index: usize) -> String {
    format!("{}::tile::{index:04}", cluster_id.trim())
}
