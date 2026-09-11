use serde_json::{Map, Value};

use super::super::helpers::{to_text_list, value_bool, value_i64};

pub(super) fn request_text_list(object: Option<&Map<String, Value>>, keys: &[&str]) -> Vec<String> {
    let Some(object) = object else {
        return Vec::new();
    };
    for key in keys {
        if let Some(value) = object.get(*key) {
            return to_text_list(Some(value));
        }
    }
    Vec::new()
}

pub(super) fn projection_materialize_limit(request_context: Option<&Map<String, Value>>) -> usize {
    request_context
        .and_then(|request| {
            request
                .get("materialize_limit")
                .or_else(|| request.get("materializeLimit"))
        })
        .and_then(value_i64)
        .unwrap_or(4_000)
        .clamp(1, 50_000) as usize
}

pub(super) fn projection_neighbor_depth(request_context: Option<&Map<String, Value>>) -> usize {
    request_context
        .and_then(|request| {
            request
                .get("neighbor_depth")
                .or_else(|| request.get("neighborDepth"))
        })
        .and_then(value_i64)
        .unwrap_or(1)
        .clamp(0, 3) as usize
}

pub(super) fn projection_include_neighbors(request_context: Option<&Map<String, Value>>) -> bool {
    request_context
        .and_then(|request| {
            request
                .get("include_neighbors")
                .or_else(|| request.get("includeNeighbors"))
        })
        .and_then(value_bool)
        .unwrap_or(true)
}

pub(super) fn map_number_field(object: Option<&Map<String, Value>>, key: &str) -> Option<i64> {
    object.and_then(|item| item.get(key)).and_then(value_i64)
}
