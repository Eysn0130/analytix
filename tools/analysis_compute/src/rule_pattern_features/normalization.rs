pub(crate) fn normalize_rule_direction(value: &str) -> String {
    let raw = value.trim().to_lowercase();
    if raw.is_empty() {
        return "unknown".to_string();
    }
    if matches!(raw.as_str(), "in" | "credit" | "incoming") {
        return "in".to_string();
    }
    if matches!(raw.as_str(), "out" | "debit" | "outgoing") {
        return "out".to_string();
    }
    if ["进", "入", "贷", "收"]
        .iter()
        .any(|token| raw.contains(token))
    {
        return "in".to_string();
    }
    if ["出", "付", "借", "支"]
        .iter()
        .any(|token| raw.contains(token))
    {
        return "out".to_string();
    }
    "unknown".to_string()
}

use super::model::CashClass;

pub(crate) fn classify_rule_cash_txn(cash_raw: &str, text_fields: &[&str]) -> CashClass {
    let raw = cash_raw.trim().to_lowercase();
    let explicit = if matches!(
        raw.as_str(),
        "1" | "true" | "t" | "yes" | "y" | "cash" | "现金" | "是"
    ) {
        Some(CashClass::Cash)
    } else if matches!(
        raw.as_str(),
        "0" | "false" | "f" | "no" | "n" | "noncash" | "no_cash" | "nocash" | "非现金" | "否"
    ) {
        Some(CashClass::NonCash)
    } else {
        None
    };
    let mut text = text_fields
        .iter()
        .map(|value| value.trim())
        .collect::<Vec<_>>()
        .join(" ")
        .to_lowercase();
    let negative_markers = [
        "非现金",
        "不含现金",
        "无现金",
        "非现钞",
        "不含现钞",
        "无现钞",
        "noncash",
        "no cash",
    ];
    let negative_cue = negative_markers.iter().any(|marker| text.contains(marker));
    for marker in negative_markers {
        text = text.replace(marker, " ");
    }
    let positive_cue = ["现金", "现钞", "存现", "取现", "cash"]
        .iter()
        .any(|marker| text.contains(marker));
    match explicit {
        Some(CashClass::Cash) if negative_cue => CashClass::Conflict,
        Some(CashClass::NonCash) if positive_cue => CashClass::Conflict,
        Some(value) => value,
        None => CashClass::Unknown,
    }
}

pub(super) fn is_night_or_offhour(hour: i64, start_hour: i64, end_hour: i64) -> bool {
    if start_hour > end_hour {
        hour >= start_hour || hour < end_hour
    } else {
        hour >= start_hour && hour < end_hour
    }
}

pub(super) fn round4(value: f64) -> f64 {
    (value * 10000.0).round() / 10000.0
}
