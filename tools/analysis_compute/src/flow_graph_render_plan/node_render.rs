use serde_json::{json, Map, Value};

use super::node_visibility::NodeRenderModeProjection;
use super::value_helpers::{
    clamp_number, has_non_null, js_number_or, js_truthy, number_field, numeric_json, raw_text,
    text_value,
};

#[derive(Clone, Debug)]
struct NodeGeometryState {
    radius: f64,
    line_width: f64,
}

#[derive(Clone, Debug)]
struct NodeGraphStyle {
    base_radius: f64,
    base_line_width: f64,
    restore_icon_size: Option<f64>,
    tier_base_icon_size: Option<f64>,
    node_shadow: bool,
}

impl NodeGraphStyle {
    fn from_value(graph_style: &Value) -> Self {
        let icon_size = graph_style.get("iconSize");
        Self {
            base_radius: js_number_or(graph_style.get("nodeSize"), 18.0),
            base_line_width: js_number_or(graph_style.get("nodeWidth"), 2.0),
            restore_icon_size: js_truthy(icon_size).then(|| js_number_or(icon_size, 16.0)),
            tier_base_icon_size: has_non_null(icon_size)
                .then(|| number_field(icon_size).unwrap_or(0.0)),
            node_shadow: as_bool(graph_style.get("nodeShadow"), false),
        }
    }
}

#[derive(Clone, Debug)]
struct NodeTierState {
    current_mode: String,
    next_mode: String,
    geometry: NodeGeometryState,
    tier_label_suppressed: bool,
}

impl NodeTierState {
    fn from_node(node: &Value, mode: &str, graph_style: &NodeGraphStyle) -> Self {
        Self {
            current_mode: normalize_node_render_mode(text_value(node.get("nodeRenderMode"))),
            next_mode: normalize_node_render_mode(mode.to_string()),
            geometry: resolve_node_geometry_state(node, graph_style),
            tier_label_suppressed: js_truthy(node.get("__tierLabelSuppressed")),
        }
    }

    fn current_is_dot(&self) -> bool {
        self.current_mode == "dot"
    }

    fn next_is_dot(&self) -> bool {
        self.next_mode == "dot"
    }

    fn next_is_focus(&self) -> bool {
        self.next_mode == "focus"
    }

    fn restore_from_tier_base(&self) -> bool {
        self.current_is_dot() || self.tier_label_suppressed
    }
}

#[derive(Clone, Debug)]
struct NodeRenderUpdate {
    r: f64,
    line_width: f64,
    node_shadow: bool,
    icon: String,
    icon_symbol: String,
    icon_size: f64,
    node_label_hidden: bool,
    node_render_mode: String,
}

pub(super) fn project_node_render_updates(
    nodes: &[Value],
    mode_rows: &[NodeRenderModeProjection],
    graph_style: &Value,
) -> Vec<Value> {
    let graph_style = NodeGraphStyle::from_value(graph_style);
    mode_rows
        .iter()
        .map(|mode_row| {
            let mut object = project_node_render_update_json(
                nodes.get(mode_row.index).unwrap_or(&Value::Null),
                &mode_row.mode,
                &graph_style,
            );
            object.insert("index".to_string(), json!(mode_row.index));
            object.insert("id".to_string(), json!(mode_row.id));
            Value::Object(object)
        })
        .collect()
}

fn project_node_render_update_json(
    node: &Value,
    mode: &str,
    graph_style: &NodeGraphStyle,
) -> Map<String, Value> {
    let tier_state = NodeTierState::from_node(node, mode, graph_style);
    let update = project_node_render_update(node, &tier_state, graph_style);
    let mut object = Map::new();
    object.insert("r".to_string(), numeric_json(update.r));
    object.insert("lineWidth".to_string(), numeric_json(update.line_width));
    object.insert("nodeShadow".to_string(), json!(update.node_shadow));
    object.insert("icon".to_string(), json!(update.icon));
    object.insert("iconSymbol".to_string(), json!(update.icon_symbol));
    object.insert("iconSize".to_string(), numeric_json(update.icon_size));
    object.insert(
        "nodeLabelHidden".to_string(),
        json!(update.node_label_hidden),
    );
    object.insert(
        "nodeRenderMode".to_string(),
        json!(update.node_render_mode.clone()),
    );
    add_node_tier_state_fields(&mut object, node, &tier_state, graph_style);
    object
}

fn add_node_tier_state_fields(
    object: &mut Map<String, Value>,
    node: &Value,
    tier_state: &NodeTierState,
    graph_style: &NodeGraphStyle,
) {
    if tier_state.next_is_dot() {
        if !tier_state.current_is_dot() {
            object.insert(
                "__tierBaseR".to_string(),
                numeric_json(tier_state.geometry.radius),
            );
            object.insert(
                "__tierBaseLineWidth".to_string(),
                numeric_json(tier_state.geometry.line_width),
            );
            object.insert(
                "__tierBaseNodeShadow".to_string(),
                json!(js_truthy(node.get("nodeShadow"))),
            );
            object.insert(
                "__tierBaseIcon".to_string(),
                json!(js_truthy_text(node.get("icon"))),
            );
            object.insert(
                "__tierBaseIconSymbol".to_string(),
                json!(js_truthy_text(node.get("iconSymbol"))),
            );
            object.insert(
                "__tierBaseIconSize".to_string(),
                numeric_json(node_tier_base_icon_size(node, graph_style)),
            );
            object.insert(
                "__tierBaseNodeLabelHidden".to_string(),
                json!(js_truthy(node.get("nodeLabelHidden"))),
            );
        } else {
            insert_or_compute_node_private_number(
                object,
                node,
                "__tierBaseR",
                js_number_or(node.get("__tierBaseR"), tier_state.geometry.radius),
            );
            insert_or_compute_node_private_number(
                object,
                node,
                "__tierBaseLineWidth",
                js_number_or(
                    node.get("__tierBaseLineWidth"),
                    tier_state.geometry.line_width,
                ),
            );
            insert_or_compute_node_private_bool(
                object,
                node,
                "__tierBaseNodeShadow",
                js_truthy(node.get("nodeShadow")),
            );
            insert_or_compute_node_private_text(
                object,
                node,
                "__tierBaseIcon",
                js_truthy_text(node.get("icon")),
            );
            insert_or_compute_node_private_text(
                object,
                node,
                "__tierBaseIconSymbol",
                js_truthy_text(node.get("iconSymbol")),
            );
            insert_or_compute_node_private_number(
                object,
                node,
                "__tierBaseIconSize",
                node_tier_base_icon_size(node, graph_style),
            );
            insert_or_compute_node_private_bool(
                object,
                node,
                "__tierBaseNodeLabelHidden",
                js_truthy(node.get("nodeLabelHidden")),
            );
        }
        object.insert("__tierLabelSuppressed".to_string(), json!(true));
        return;
    }

    for key in [
        "__tierBaseR",
        "__tierBaseLineWidth",
        "__tierBaseNodeShadow",
        "__tierBaseIcon",
        "__tierBaseIconSymbol",
        "__tierBaseIconSize",
        "__tierBaseNodeLabelHidden",
        "__tierLabelSuppressed",
    ] {
        if let Some(value) = node.get(key) {
            object.insert(key.to_string(), value.clone());
        }
    }
    if tier_state.tier_label_suppressed {
        object.insert("__tierLabelSuppressed".to_string(), json!(false));
    }
}

fn project_node_render_update(
    node: &Value,
    tier_state: &NodeTierState,
    graph_style: &NodeGraphStyle,
) -> NodeRenderUpdate {
    if tier_state.next_is_dot() {
        let entity_radius_base = if !tier_state.current_is_dot() {
            tier_state.geometry.radius
        } else {
            js_number_or(node.get("__tierBaseR"), tier_state.geometry.radius)
        };
        let entity_line_width_base = if !tier_state.current_is_dot() {
            tier_state.geometry.line_width
        } else {
            js_number_or(
                node.get("__tierBaseLineWidth"),
                tier_state.geometry.line_width,
            )
        };
        let dot_radius = clamp_number((entity_radius_base * 0.34).max(3.4).min(7.2), 2.8, 8.5);
        return NodeRenderUpdate {
            r: dot_radius,
            line_width: clamp_number(entity_line_width_base.min(1.5), 0.8, 1.8),
            node_shadow: false,
            icon: String::new(),
            icon_symbol: String::new(),
            icon_size: 0.0,
            node_label_hidden: true,
            node_render_mode: "dot".to_string(),
        };
    }

    let restore_from_tier_base = tier_state.restore_from_tier_base();
    let entity_radius_base = js_number_or(node.get("__tierBaseR"), tier_state.geometry.radius);
    let entity_line_width_base = js_number_or(
        node.get("__tierBaseLineWidth"),
        tier_state.geometry.line_width,
    );
    let r = entity_radius_base;
    let mut line_width = entity_line_width_base;
    let mut node_shadow = js_truthy(node.get("nodeShadow"));
    let mut icon = js_truthy_text(node.get("icon"));
    let mut icon_symbol = js_truthy_text(node.get("iconSymbol"));
    let mut icon_size = js_truthy_number_or(node.get("iconSize"), 0.0);
    let mut node_label_hidden = js_truthy(node.get("nodeLabelHidden"));

    if restore_from_tier_base {
        node_shadow = if has_non_null(node.get("__tierBaseNodeShadow")) {
            js_truthy(node.get("__tierBaseNodeShadow"))
        } else {
            graph_style.node_shadow
        };
        icon = js_truthy_text(node.get("__tierBaseIcon"));
        icon_symbol = js_truthy_text(node.get("__tierBaseIconSymbol"));
        icon_size = restore_icon_size(node, graph_style);
    }
    if tier_state.tier_label_suppressed {
        node_label_hidden = js_truthy(node.get("__tierBaseNodeLabelHidden"));
    }
    if tier_state.next_is_focus() {
        line_width = line_width.max(entity_line_width_base + 1.0);
        node_shadow = graph_style.node_shadow;
    }

    NodeRenderUpdate {
        r,
        line_width,
        node_shadow,
        icon,
        icon_symbol,
        icon_size,
        node_label_hidden,
        node_render_mode: if tier_state.next_is_focus() {
            "focus".to_string()
        } else {
            "entity".to_string()
        },
    }
}

fn resolve_node_geometry_state(node: &Value, graph_style: &NodeGraphStyle) -> NodeGeometryState {
    let base_radius = resolve_node_base_radius(node, graph_style);
    let base_line_width = resolve_node_base_line_width(node, graph_style);
    let scaled_radius = number_field(node.get("__analysisScaledR"));
    let scaled_line_width = number_field(node.get("__analysisScaledLineWidth"));
    NodeGeometryState {
        radius: scaled_radius
            .filter(|value| *value > 0.0)
            .unwrap_or(base_radius),
        line_width: scaled_line_width
            .filter(|value| *value > 0.0)
            .unwrap_or(base_line_width),
    }
}

fn normalize_node_render_mode(mode: String) -> String {
    let normalized = mode.trim().to_ascii_lowercase();
    if normalized.is_empty() {
        "entity".to_string()
    } else {
        normalized
    }
}

fn resolve_node_base_radius(node: &Value, graph_style: &NodeGraphStyle) -> f64 {
    let style_base = graph_style.base_radius;
    if let Some(preferred) = number_field(node.get("__baseR")).filter(|value| *value > 0.0) {
        return preferred;
    }
    if let Some(fallback) = number_field(node.get("r")).filter(|value| *value > 0.0) {
        return fallback;
    }
    style_base
}

fn resolve_node_base_line_width(node: &Value, graph_style: &NodeGraphStyle) -> f64 {
    let style_base = graph_style.base_line_width;
    if let Some(preferred) = number_field(node.get("__baseLineWidth")).filter(|value| *value > 0.0)
    {
        return preferred;
    }
    if let Some(fallback) = number_field(node.get("lineWidth")).filter(|value| *value > 0.0) {
        return fallback;
    }
    style_base
}

fn restore_icon_size(node: &Value, graph_style: &NodeGraphStyle) -> f64 {
    if js_truthy(node.get("__tierBaseIconSize")) {
        return js_number_or(node.get("__tierBaseIconSize"), 16.0);
    }
    if let Some(icon_size) = graph_style.restore_icon_size {
        return icon_size;
    }
    16.0
}

fn node_tier_base_icon_size(node: &Value, graph_style: &NodeGraphStyle) -> f64 {
    let raw = if has_non_null(node.get("iconSize")) {
        number_field(node.get("iconSize")).unwrap_or(0.0)
    } else if let Some(icon_size) = graph_style.tier_base_icon_size {
        icon_size
    } else {
        16.0
    };
    if raw != 0.0 {
        raw
    } else {
        16.0
    }
}

fn insert_or_compute_node_private_number(
    object: &mut Map<String, Value>,
    node: &Value,
    key: &str,
    fallback: f64,
) {
    if has_non_null(node.get(key)) {
        object.insert(
            key.to_string(),
            node.get(key).cloned().unwrap_or(Value::Null),
        );
    } else {
        object.insert(key.to_string(), numeric_json(fallback));
    }
}

fn insert_or_compute_node_private_bool(
    object: &mut Map<String, Value>,
    node: &Value,
    key: &str,
    fallback: bool,
) {
    if has_non_null(node.get(key)) {
        object.insert(
            key.to_string(),
            node.get(key).cloned().unwrap_or(Value::Null),
        );
    } else {
        object.insert(key.to_string(), json!(fallback));
    }
}

fn insert_or_compute_node_private_text(
    object: &mut Map<String, Value>,
    node: &Value,
    key: &str,
    fallback: String,
) {
    if has_non_null(node.get(key)) {
        object.insert(
            key.to_string(),
            node.get(key).cloned().unwrap_or(Value::Null),
        );
    } else {
        object.insert(key.to_string(), json!(fallback));
    }
}

fn js_truthy_number_or(value: Option<&Value>, fallback: f64) -> f64 {
    if js_truthy(value) {
        return js_number_or(value, fallback);
    }
    fallback
}

fn js_truthy_text(value: Option<&Value>) -> String {
    if js_truthy(value) {
        raw_text(value)
    } else {
        String::new()
    }
}

fn as_bool(value: Option<&Value>, fallback: bool) -> bool {
    match value {
        None | Some(Value::Null) => fallback,
        Some(Value::Bool(value)) => *value,
        Some(Value::Number(number)) => number.as_f64().is_some_and(|value| value != 0.0),
        Some(Value::String(text)) => {
            let raw = text.trim().to_ascii_lowercase();
            if raw.is_empty() {
                fallback
            } else if matches!(raw.as_str(), "1" | "true" | "yes" | "on") {
                true
            } else if matches!(raw.as_str(), "0" | "false" | "no" | "off") {
                false
            } else {
                fallback
            }
        }
        Some(Value::Array(_)) | Some(Value::Object(_)) => fallback,
    }
}
