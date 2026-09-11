use serde_json::{json, Map, Value};

use super::value_helpers::{
    bool_field, first_truthy_text, has_non_null, number_field, raw_text, text_value,
};

pub(super) fn project_edge_render_updates(
    edges: &[Value],
    hints: &Value,
    preserve_labels: bool,
) -> Vec<Value> {
    edges
        .iter()
        .map(|edge| project_edge_render_update(edge, hints, preserve_labels))
        .collect()
}

fn project_edge_render_update(edge: &Value, hints: &Value, preserve_labels: bool) -> Value {
    let view_mode =
        text_value(hints.get("view_mode").or_else(|| hints.get("viewMode"))).to_ascii_lowercase();
    let project_relation_display = view_mode.is_empty() || view_mode == "relation";
    let hide_labels = matches!(hints.get("show_edge_labels"), Some(Value::Bool(false)));
    let restore_from_base =
        !preserve_labels && !hide_labels && bool_field(edge.get("__tierLabelSuppressed"));
    let current_labels = edge_label_fields(edge, false);
    let tier_base_labels = edge_label_fields(edge, true);
    let visible_labels = if restore_from_base {
        tier_base_labels.clone()
    } else {
        current_labels
    };
    let visible_display = project_edge_display(edge, &visible_labels, project_relation_display);
    let base_display = project_edge_display(edge, &tier_base_labels, project_relation_display);
    let base_label = json!(base_display.label);
    let base_label_top = json!(base_display.label_top);
    let base_label_bottom = json!(base_display.label_bottom);
    let base_detail_label = json!(base_display.detail_label);
    let mut object = Map::new();
    object.insert("id".to_string(), json!(text_value(edge.get("id"))));
    insert_edge_display_projection(&mut object, &visible_display);
    if hide_labels {
        object.insert("label".to_string(), json!(""));
        object.insert("labelTop".to_string(), json!(""));
        object.insert("labelBottom".to_string(), json!(""));
        object.insert("detailLabel".to_string(), json!(false));
        object.insert("edgeLabelHidden".to_string(), json!(true));
        object.insert("__tierBaseLabel".to_string(), base_label);
        object.insert("__tierBaseLabelTop".to_string(), base_label_top);
        object.insert("__tierBaseLabelBottom".to_string(), base_label_bottom);
        object.insert("__tierBaseDetailLabel".to_string(), base_detail_label);
        object.insert("__tierLabelSuppressed".to_string(), json!(true));
        return Value::Object(object);
    }

    if restore_from_base {
        object.insert("label".to_string(), json!(visible_display.label));
        object.insert("labelTop".to_string(), json!(visible_display.label_top));
        object.insert(
            "labelBottom".to_string(),
            json!(visible_display.label_bottom),
        );
        object.insert(
            "detailLabel".to_string(),
            json!(visible_display.detail_label),
        );
        object.insert("edgeLabelHidden".to_string(), json!(false));
        object.insert("__tierBaseLabel".to_string(), base_label);
        object.insert("__tierBaseLabelTop".to_string(), base_label_top);
        object.insert("__tierBaseLabelBottom".to_string(), base_label_bottom);
        object.insert("__tierBaseDetailLabel".to_string(), base_detail_label);
        object.insert("__tierLabelSuppressed".to_string(), json!(false));
        return Value::Object(object);
    }

    object.insert("label".to_string(), json!(visible_display.label));
    object.insert("labelTop".to_string(), json!(visible_display.label_top));
    object.insert(
        "labelBottom".to_string(),
        json!(visible_display.label_bottom),
    );
    object.insert(
        "detailLabel".to_string(),
        json!(visible_display.detail_label),
    );
    object.insert("edgeLabelHidden".to_string(), json!(false));
    object.insert("__tierBaseLabel".to_string(), base_label);
    object.insert("__tierBaseLabelTop".to_string(), base_label_top);
    object.insert("__tierBaseLabelBottom".to_string(), base_label_bottom);
    object.insert("__tierBaseDetailLabel".to_string(), base_detail_label);
    if let Some(value) = edge.get("__tierLabelSuppressed") {
        object.insert("__tierLabelSuppressed".to_string(), value.clone());
    }
    Value::Object(object)
}

#[derive(Clone, Debug)]
struct EdgeLabelFields {
    label: String,
    label_top: String,
    label_bottom: String,
    detail_label: bool,
}

#[derive(Clone, Debug)]
struct EdgeDisplayProjection {
    label: String,
    label_top: String,
    label_bottom: String,
    detail_label: bool,
    mode: Option<String>,
    edge_arrow: Option<String>,
    show_arrow: Option<bool>,
}

fn edge_label_fields(edge: &Value, prefer_tier_base: bool) -> EdgeLabelFields {
    EdgeLabelFields {
        label: if prefer_tier_base && has_non_null(edge.get("__tierBaseLabel")) {
            raw_text(edge.get("__tierBaseLabel"))
        } else {
            raw_text(edge.get("label"))
        },
        label_top: if prefer_tier_base && has_non_null(edge.get("__tierBaseLabelTop")) {
            raw_text(edge.get("__tierBaseLabelTop"))
        } else {
            raw_text(edge.get("labelTop"))
        },
        label_bottom: if prefer_tier_base && has_non_null(edge.get("__tierBaseLabelBottom")) {
            raw_text(edge.get("__tierBaseLabelBottom"))
        } else {
            raw_text(edge.get("labelBottom"))
        },
        detail_label: if prefer_tier_base && has_non_null(edge.get("__tierBaseDetailLabel")) {
            bool_field(edge.get("__tierBaseDetailLabel"))
        } else {
            bool_field(edge.get("detailLabel"))
        },
    }
}

fn project_edge_display(
    edge: &Value,
    labels: &EdgeLabelFields,
    project_relation_display: bool,
) -> EdgeDisplayProjection {
    if project_relation_display {
        return project_relation_edge_display(edge, labels);
    }
    EdgeDisplayProjection {
        label: labels.label.clone(),
        label_top: labels.label_top.clone(),
        label_bottom: labels.label_bottom.clone(),
        detail_label: labels.detail_label,
        mode: None,
        edge_arrow: None,
        show_arrow: None,
    }
}

fn project_relation_edge_display(edge: &Value, labels: &EdgeLabelFields) -> EdgeDisplayProjection {
    let (forward, reverse) = edge_directional_amount_summary(edge, labels);
    let has_forward = forward > 0.0;
    let has_reverse = reverse > 0.0;
    if has_forward && has_reverse {
        return EdgeDisplayProjection {
            label: labels.label.clone(),
            label_top: format!("￥{}", format_money_amount(forward.abs())),
            label_bottom: format!("￥{}", format_money_amount(reverse.abs())),
            detail_label: labels.detail_label,
            mode: Some("double".to_string()),
            edge_arrow: Some("both".to_string()),
            show_arrow: Some(true),
        };
    }
    if has_forward || has_reverse {
        let amount = if has_forward { forward } else { reverse };
        return EdgeDisplayProjection {
            label: format!("￥{}", format_money_amount(amount.abs())),
            label_top: String::new(),
            label_bottom: String::new(),
            detail_label: labels.detail_label,
            mode: Some("single".to_string()),
            edge_arrow: Some(if has_forward { "end" } else { "start" }.to_string()),
            show_arrow: Some(true),
        };
    }

    let edge_arrow = normalize_edge_arrow_value(raw_text(edge.get("edgeArrow")), "end");
    EdgeDisplayProjection {
        label: labels.label.clone(),
        label_top: labels.label_top.clone(),
        label_bottom: labels.label_bottom.clone(),
        detail_label: labels.detail_label,
        mode: None,
        show_arrow: Some(edge_arrow != "none"),
        edge_arrow: Some(edge_arrow),
    }
}

fn insert_edge_display_projection(
    object: &mut Map<String, Value>,
    display: &EdgeDisplayProjection,
) {
    if let Some(mode) = &display.mode {
        object.insert("mode".to_string(), json!(mode));
    }
    if let Some(edge_arrow) = &display.edge_arrow {
        object.insert("edgeArrow".to_string(), json!(edge_arrow));
    }
    if let Some(show_arrow) = display.show_arrow {
        object.insert("showArrow".to_string(), json!(show_arrow));
    }
}

fn edge_directional_amount_summary(edge: &Value, labels: &EdgeLabelFields) -> (f64, f64) {
    let forward = number_field(edge.get("forward_amount"))
        .unwrap_or(0.0)
        .abs();
    let reverse = number_field(edge.get("reverse_amount"))
        .unwrap_or(0.0)
        .abs();
    if forward > 0.0 || reverse > 0.0 {
        return (forward, reverse);
    }
    let top = parse_amount_with_currency(&labels.label_top);
    let bottom = parse_amount_with_currency(&labels.label_bottom);
    if top > 0.0 || bottom > 0.0 {
        return (top.abs(), bottom.abs());
    }
    let amount = number_field(edge.get("amount")).unwrap_or(0.0).abs();
    if amount <= 0.0 {
        return (0.0, 0.0);
    }
    let arrow = first_truthy_text(&[edge.get("edgeArrow"), edge.get("arrow")]).to_ascii_lowercase();
    if arrow == "start" {
        (0.0, amount)
    } else {
        (amount, 0.0)
    }
}

fn normalize_edge_arrow_value(value: String, fallback: &str) -> String {
    match value.as_str() {
        "none" | "end" | "start" | "both" => value,
        _ => fallback.to_string(),
    }
}

fn parse_amount_with_currency(text: &str) -> f64 {
    if text.is_empty() || (!text.contains('￥') && !text.contains('元')) {
        return 0.0;
    }
    parse_amount_from_label(text)
}

fn parse_amount_from_label(text: &str) -> f64 {
    let chars = text.chars().collect::<Vec<_>>();
    let mut max_amount = 0.0;
    let mut index = 0;
    while index < chars.len() {
        let starts_negative_number = chars[index] == '-'
            && chars
                .get(index + 1)
                .is_some_and(|next| next.is_ascii_digit());
        if !chars[index].is_ascii_digit() && !starts_negative_number {
            index += 1;
            continue;
        }
        let mut end = index;
        let mut token = String::new();
        if starts_negative_number {
            token.push('-');
            end += 1;
        }
        while chars
            .get(end)
            .is_some_and(|next| next.is_ascii_digit() || *next == ',')
        {
            token.push(chars[end]);
            end += 1;
        }
        if chars.get(end).is_some_and(|next| *next == '.') {
            token.push('.');
            end += 1;
            while chars.get(end).is_some_and(|next| next.is_ascii_digit()) {
                token.push(chars[end]);
                end += 1;
            }
        }
        let normalized = token.replace(',', "");
        if let Ok(value) = normalized.parse::<f64>() {
            if value.is_finite() {
                let amount = value.abs();
                if amount > max_amount {
                    max_amount = amount;
                }
            }
        }
        index = end.max(index + 1);
    }
    max_amount
}

fn format_money_amount(value: f64) -> String {
    let rounded = format!("{:.2}", value.abs());
    let mut parts = rounded.split('.');
    let integer = parts.next().unwrap_or("0");
    let fraction = parts.next().unwrap_or("").trim_end_matches('0');
    let grouped = group_integer_digits(integer);
    if fraction.is_empty() {
        grouped
    } else {
        format!("{grouped}.{fraction}")
    }
}

fn group_integer_digits(integer: &str) -> String {
    let chars = integer.chars().rev().collect::<Vec<_>>();
    let mut out = String::new();
    for (index, ch) in chars.iter().enumerate() {
        if index > 0 && index % 3 == 0 {
            out.push(',');
        }
        out.push(*ch);
    }
    out.chars().rev().collect()
}
