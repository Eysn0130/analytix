use serde_json::Value;

pub(in crate::stats_query_store) fn cursor_i64(cursor: &Value, key: &str) -> Option<i64> {
    let value = cursor.get(key)?;
    if let Some(item) = value.as_i64() {
        return Some(item);
    }
    value.as_str()?.trim().parse().ok()
}

pub(in crate::stats_query_store) fn cursor_f64(cursor: &Value, key: &str) -> Option<f64> {
    let value = cursor.get(key)?;
    if let Some(item) = value.as_f64() {
        return item.is_finite().then_some(item);
    }
    let item = value.as_str()?.trim().parse::<f64>().ok()?;
    item.is_finite().then_some(item)
}

pub(in crate::stats_query_store) fn cursor_string<'a>(cursor: &'a Value, key: &str) -> &'a str {
    cursor.get(key).and_then(Value::as_str).unwrap_or("")
}

pub(in crate::stats_query_store) fn cursor_bool(cursor: &Value, key: &str) -> bool {
    let Some(value) = cursor.get(key) else {
        return false;
    };
    if let Some(item) = value.as_bool() {
        return item;
    }
    if let Some(item) = value.as_i64() {
        return item != 0;
    }
    matches!(value.as_str().map(str::trim), Some("1" | "true" | "True"))
}
