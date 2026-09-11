use anyhow::{bail, Context, Result};
use serde_json::Value;

#[derive(Debug, Clone)]
pub(crate) struct QueryChartFilterArg {
    pub(crate) dimension: String,
    pub(crate) value: String,
    pub(crate) payload: Value,
}

pub(crate) fn parse_chart_filter_args_json(raw: &str) -> Result<Vec<QueryChartFilterArg>> {
    let text = raw.trim();
    if text.is_empty() {
        return Ok(Vec::new());
    }
    let parsed: Value = serde_json::from_str(text).with_context(|| "parse --chart-filters-json")?;
    let Some(items) = parsed.as_array() else {
        bail!("--chart-filters-json must be a JSON array");
    };

    let mut filters = Vec::new();
    for item in items {
        let Some(object) = item.as_object() else {
            continue;
        };
        let dimension = json_text(object.get("dimension"));
        if dimension.is_empty() {
            continue;
        }
        filters.push(QueryChartFilterArg {
            dimension,
            value: json_text(object.get("value")),
            payload: object
                .get("payload")
                .filter(|value| value.is_object())
                .cloned()
                .unwrap_or(Value::Null),
        });
    }
    Ok(filters)
}

fn json_text(value: Option<&Value>) -> String {
    match value {
        Some(Value::String(text)) => text.trim().to_string(),
        Some(Value::Null) | None => String::new(),
        Some(other) => other.to_string().trim().to_string(),
    }
}
