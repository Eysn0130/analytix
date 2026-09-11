use serde_json::Value;
use std::collections::HashSet;

pub(super) fn js_number_or(value: Option<&Value>, fallback: f64) -> f64 {
    number_field(value)
        .filter(|next| *next != 0.0)
        .unwrap_or(fallback)
}

pub(super) fn has_non_null(value: Option<&Value>) -> bool {
    !matches!(value, None | Some(Value::Null))
}

pub(super) fn clamp_number(value: f64, min: f64, max: f64) -> f64 {
    if !value.is_finite() {
        return min;
    }
    min.max(value.min(max))
}

pub(super) fn render_plan_rows(
    payload: &Value,
    plural_key: &str,
    singular_key: &str,
) -> Vec<Value> {
    if let Some(rows) = payload.get(plural_key).and_then(Value::as_array) {
        return rows.to_vec();
    }
    payload
        .get(singular_key)
        .cloned()
        .map(|row| vec![row])
        .unwrap_or_else(|| vec![payload.clone()])
}

pub(super) fn parse_unique_list(value: Option<&Value>, allow_string: bool) -> Vec<String> {
    let raw_items = match value {
        Some(Value::Array(items)) => items.iter().map(|item| text_value(Some(item))).collect(),
        Some(Value::String(_)) if allow_string => vec![text_value(value)],
        Some(Value::Number(_)) | Some(Value::Bool(_)) if allow_string => vec![text_value(value)],
        _ => Vec::new(),
    };
    let mut out = Vec::new();
    let mut seen = HashSet::new();
    for item in raw_items {
        if item.is_empty() || seen.contains(&item) {
            continue;
        }
        seen.insert(item.clone());
        out.push(item);
    }
    out
}

pub(super) fn clone_array(value: Option<&Value>) -> Vec<Value> {
    value
        .and_then(Value::as_array)
        .map(|items| items.to_vec())
        .unwrap_or_default()
}

pub(super) fn text_value(value: Option<&Value>) -> String {
    match value {
        Some(Value::String(text)) => text.trim().to_string(),
        Some(Value::Number(number)) => number.to_string(),
        Some(Value::Bool(value)) => value.to_string(),
        _ => String::new(),
    }
}

pub(super) fn number_field(value: Option<&Value>) -> Option<f64> {
    match value {
        Some(Value::Number(number)) => number.as_f64().filter(|next| next.is_finite()),
        Some(Value::String(text)) => text
            .trim()
            .parse::<f64>()
            .ok()
            .filter(|next| next.is_finite()),
        Some(Value::Bool(true)) => Some(1.0),
        Some(Value::Bool(false)) => Some(0.0),
        _ => None,
    }
}

pub(super) fn bool_field(value: Option<&Value>) -> bool {
    match value {
        Some(Value::Bool(value)) => *value,
        Some(Value::Number(number)) => number.as_f64().is_some_and(|value| value != 0.0),
        Some(Value::String(text)) => !text.trim().is_empty(),
        Some(Value::Array(_)) | Some(Value::Object(_)) => true,
        _ => false,
    }
}

pub(super) fn raw_text(value: Option<&Value>) -> String {
    match value {
        Some(Value::String(text)) => text.to_string(),
        Some(Value::Number(number)) => number.to_string(),
        Some(Value::Bool(value)) => value.to_string(),
        _ => String::new(),
    }
}

pub(super) fn first_truthy_text(values: &[Option<&Value>]) -> String {
    values
        .iter()
        .copied()
        .flatten()
        .find(|value| js_truthy(Some(*value)))
        .map(|value| text_value(Some(value)))
        .unwrap_or_default()
}

pub(super) fn first_truthy_value<'a>(values: &[Option<&'a Value>]) -> Option<&'a Value> {
    values
        .iter()
        .copied()
        .flatten()
        .find(|value| js_truthy(Some(*value)))
}

pub(super) fn js_truthy(value: Option<&Value>) -> bool {
    match value {
        Some(Value::Bool(value)) => *value,
        Some(Value::Number(number)) => number.as_f64().is_some_and(|value| value != 0.0),
        Some(Value::String(text)) => !text.is_empty(),
        Some(Value::Array(_)) | Some(Value::Object(_)) => true,
        _ => false,
    }
}

pub(super) fn compare_f64_desc(left: f64, right: f64) -> std::cmp::Ordering {
    right
        .partial_cmp(&left)
        .unwrap_or(std::cmp::Ordering::Equal)
}

pub(super) fn numeric_json(value: f64) -> Value {
    serde_json::Number::from_f64(value)
        .map(Value::Number)
        .unwrap_or(Value::Null)
}
