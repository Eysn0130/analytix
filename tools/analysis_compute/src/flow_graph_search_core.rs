use serde_json::{json, Value};
use std::cmp::Ordering;
use std::collections::HashSet;

pub(crate) struct GraphSearchProjectionWeights {
    pub(crate) amount_abs: f64,
    pub(crate) count: i64,
}

#[derive(Debug)]
struct GraphSearchProjectionCandidate {
    priority: i64,
    amount_abs: f64,
    count: i64,
    node_id: String,
}

pub(crate) struct GraphSearchIndex<'a> {
    rows: Vec<GraphSearchIndexRow<'a>>,
}

impl<'a> GraphSearchIndex<'a> {
    pub(crate) fn from_nodes(nodes: &'a [Value]) -> Self {
        Self {
            rows: nodes
                .iter()
                .enumerate()
                .map(|(index, node)| GraphSearchIndexRow::from_node(index, node))
                .collect(),
        }
    }

    pub(crate) fn summary(&self) -> Value {
        let text_count = self.rows.iter().map(|row| row.text_count()).sum::<usize>();
        let indexed_row_count = self.rows.iter().filter(|row| row.text_count() > 0).count();
        let max_text_count = self
            .rows
            .iter()
            .map(|row| row.text_count())
            .max()
            .unwrap_or(0);
        json!({
            "rowCount": self.rows.len(),
            "indexedRowCount": indexed_row_count,
            "textCount": text_count,
            "maxTextCount": max_text_count,
        })
    }

    pub(crate) fn query_raw_node_ids(
        &self,
        query: &str,
        limit: Option<usize>,
        mut node_id_for: impl FnMut(&Value, usize) -> String,
    ) -> Vec<String> {
        let query = query.trim();
        if query.is_empty() {
            return Vec::new();
        }
        let limit = limit.unwrap_or(usize::MAX);
        if limit == 0 {
            return Vec::new();
        }

        let mut node_ids = Vec::new();
        for search_row in &self.rows {
            if search_row.matches_raw_query(query) {
                node_ids.push(node_id_for(search_row.node, search_row.index));
                if node_ids.len() >= limit {
                    break;
                }
            }
        }
        node_ids
    }

    pub(crate) fn query_projection_ranked_node_ids(
        &self,
        query: &str,
        limit: usize,
        mut node_id_for: impl FnMut(&Value) -> String,
        mut weights_for: impl FnMut(&Value) -> GraphSearchProjectionWeights,
    ) -> Vec<String> {
        let normalized_query = normalize_projection_search_text(query);
        if normalized_query.is_empty() {
            return Vec::new();
        }

        let mut ranked = Vec::new();
        let mut seen = HashSet::new();
        for search_row in &self.rows {
            let node_id = node_id_for(search_row.node);
            if node_id.is_empty() || !seen.insert(node_id.clone()) {
                continue;
            }
            let Some(priority) = search_row.projection_priority(&normalized_query) else {
                continue;
            };
            let weights = weights_for(search_row.node);
            ranked.push(GraphSearchProjectionCandidate {
                priority,
                amount_abs: weights.amount_abs,
                count: weights.count.max(0),
                node_id,
            });
        }
        ranked.sort_by(|left, right| {
            left.priority
                .cmp(&right.priority)
                .then_with(|| compare_f64_desc(left.amount_abs, right.amount_abs))
                .then_with(|| right.count.cmp(&left.count))
                .then_with(|| left.node_id.cmp(&right.node_id))
        });
        ranked
            .into_iter()
            .take(limit.max(1))
            .map(|item| item.node_id)
            .collect()
    }
}

struct GraphSearchIndexRow<'a> {
    index: usize,
    node: &'a Value,
    texts: Vec<String>,
    projection_texts: Vec<String>,
}

impl<'a> GraphSearchIndexRow<'a> {
    pub(crate) fn from_node(index: usize, node: &'a Value) -> Self {
        let texts = collect_graph_search_text(node);
        let projection_texts = texts
            .iter()
            .map(|text| normalize_projection_search_text(text))
            .filter(|text| !text.is_empty())
            .collect();
        Self {
            index,
            node,
            texts,
            projection_texts,
        }
    }

    fn matches_raw_query(&self, query: &str) -> bool {
        self.texts.iter().any(|text| text.contains(query))
    }

    fn text_count(&self) -> usize {
        self.texts.len()
    }

    fn projection_priority(&self, normalized_query: &str) -> Option<i64> {
        let mut priority: Option<i64> = None;
        for text in &self.projection_texts {
            if text == normalized_query {
                return Some(0);
            }
            if text.starts_with(normalized_query) {
                priority = Some(priority.unwrap_or(1).min(1));
            } else if text.contains(normalized_query) {
                priority = Some(priority.unwrap_or(2).min(2));
            }
        }
        priority
    }
}

fn normalize_projection_search_text(value: &str) -> String {
    let mut out = String::new();
    for ch in value.trim().chars().flat_map(char::to_lowercase) {
        if !ch.is_whitespace() {
            out.push(ch);
        }
    }
    out
}

fn collect_graph_search_text(node: &Value) -> Vec<String> {
    let display_id = text_value(first_truthy_value(&[
        node.get("displayId"),
        node.get("display_id"),
    ]));
    let display_id_raw = text_value(first_truthy_value(&[
        node.get("displayIdRaw"),
        node.get("display_id_raw"),
    ]));
    let display_id_source = first_truthy_value(&[
        node.get("displayIds"),
        node.get("display_ids"),
        node.get("displayIdRaw"),
        node.get("display_id_raw"),
        node.get("displayId"),
        node.get("display_id"),
    ]);
    let mut texts = Vec::new();
    push_text(node.get("id"), &mut texts);
    let title_or_label = text_value(first_truthy_value(&[node.get("title"), node.get("label")]));
    if !title_or_label.is_empty() {
        texts.push(title_or_label);
    }
    push_text(node.get("name"), &mut texts);
    if !display_id.is_empty() {
        texts.push(display_id);
    }
    if !display_id_raw.is_empty() {
        texts.push(display_id_raw);
    }
    for display_id in normalize_display_ids(display_id_source) {
        texts.push(display_id);
    }
    texts
}

fn first_truthy_value<'a>(values: &[Option<&'a Value>]) -> Option<&'a Value> {
    values
        .iter()
        .copied()
        .flatten()
        .find(|value| js_truthy(Some(*value)))
}

fn push_text(value: Option<&Value>, target: &mut Vec<String>) {
    let text = text_value(value);
    if !text.is_empty() {
        target.push(text);
    }
}

fn normalize_display_ids(value: Option<&Value>) -> Vec<String> {
    let raw_items = match value {
        Some(Value::Array(items)) => items
            .iter()
            .map(|item| strip_display_id_prefix(&text_value(Some(item))))
            .collect::<Vec<_>>(),
        Some(Value::String(text)) => parse_display_id_list(text),
        Some(Value::Number(_)) | Some(Value::Bool(_)) => parse_display_id_list(&text_value(value)),
        _ => Vec::new(),
    };
    let mut out = Vec::new();
    for item in raw_items {
        if item.is_empty() || is_unknown_account_label(&item) || out.contains(&item) {
            continue;
        }
        out.push(item);
    }
    out
}

fn parse_display_id_list(raw: &str) -> Vec<String> {
    let text = raw.trim();
    if text.is_empty() || text.contains("账户合集") || is_unknown_account_label(text) {
        return Vec::new();
    }
    strip_trailing_display_count(text)
        .split(['/', '／'])
        .map(strip_display_id_prefix)
        .filter(|item| !item.is_empty())
        .collect()
}

fn strip_trailing_display_count(value: &str) -> String {
    let text = value.trim();
    let Some(prefix) = text.strip_suffix('个') else {
        return text.to_string();
    };
    let mut chars = prefix.chars().rev();
    let mut digit_count = 0;
    for ch in &mut chars {
        if ch.is_ascii_digit() {
            digit_count += 1;
            continue;
        }
        if ch == '等' && digit_count > 0 {
            let keep_len = prefix.len() - '等'.len_utf8() - digit_count;
            return prefix[..keep_len].trim().to_string();
        }
        break;
    }
    text.to_string()
}

fn strip_display_id_prefix(value: &str) -> String {
    let mut text = value.trim().to_string();
    for prefix in ["卡号", "账号", "账户"] {
        if let Some(remainder) = text.strip_prefix(prefix) {
            text = remainder
                .trim_start_matches(|ch: char| ch == ':' || ch == '：' || ch.is_whitespace())
                .trim()
                .to_string();
            break;
        }
    }
    text
}

fn is_unknown_account_label(value: &str) -> bool {
    ["未知卡号", "账号未知", "未知账号"]
        .iter()
        .any(|label| value.contains(label))
}

fn text_value(value: Option<&Value>) -> String {
    match value {
        Some(Value::String(text)) => text.trim().to_string(),
        Some(Value::Number(number)) => number.to_string(),
        Some(Value::Bool(value)) => value.to_string(),
        _ => String::new(),
    }
}

fn compare_f64_desc(left: f64, right: f64) -> Ordering {
    right.partial_cmp(&left).unwrap_or(Ordering::Equal)
}

fn js_truthy(value: Option<&Value>) -> bool {
    match value {
        Some(Value::Bool(value)) => *value,
        Some(Value::Number(number)) => number.as_f64().map(|value| value != 0.0).unwrap_or(false),
        Some(Value::String(text)) => !text.is_empty(),
        Some(Value::Array(_)) | Some(Value::Object(_)) => true,
        _ => false,
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use serde_json::json;

    #[test]
    fn search_index_row_matches_graph_display_fields() {
        let node = json!({
            "id": "holder:a",
            "title": "Alice",
            "name": "Alice Alias",
            "displayIdRaw": "账号: 9555 / 9666",
            "displayIds": ["账户: 7000"],
        });
        let row = GraphSearchIndexRow::from_node(0, &node);

        assert!(row.matches_raw_query("Alice Alias"));
        assert!(row.matches_raw_query("9555"));
        assert!(row.projection_texts.iter().any(|text| text == "alicealias"));
        assert_eq!(row.projection_priority("7000"), Some(0));
    }

    #[test]
    fn search_index_builds_rows_once_for_nodes() {
        let nodes = vec![
            json!({"id": "a", "title": "Alice"}),
            json!({"id": "b", "displayIdRaw": "账号: 9555 / 9666"}),
        ];
        let index = GraphSearchIndex::from_nodes(&nodes);

        assert_eq!(
            index.summary(),
            json!({
                "rowCount": 2,
                "indexedRowCount": 2,
                "textCount": 6,
                "maxTextCount": 4,
            })
        );
        assert_eq!(
            index.query_raw_node_ids("9555", None, |node, _index| text_value(node.get("id"))),
            vec!["b".to_string()]
        );
    }

    #[test]
    fn raw_query_preserves_graph_order_and_limit() {
        let nodes = vec![
            json!({"id": "a", "title": "no match"}),
            json!({"id": "b", "displayIdRaw": "账号: 9555 / 9666"}),
            json!({"id": "c", "displayIds": ["9555"]}),
            json!({"id": "d", "name": "9555 holder"}),
        ];
        let index = GraphSearchIndex::from_nodes(&nodes);

        assert_eq!(
            index.query_raw_node_ids("9555", Some(2), |node, index| {
                format!("{}:{index}", text_value(node.get("id")))
            }),
            vec!["b:1".to_string(), "c:2".to_string()]
        );
    }

    #[test]
    fn projection_priority_preserves_exact_prefix_contains_order() {
        let exact_node = json!({"id": "node", "title": "alice"});
        let prefix_node = json!({"id": "node", "title": "alice extra"});
        let contains_node = json!({"id": "node", "title": "xx alice xx"});
        let exact = GraphSearchIndexRow::from_node(0, &exact_node);
        let prefix = GraphSearchIndexRow::from_node(1, &prefix_node);
        let contains = GraphSearchIndexRow::from_node(2, &contains_node);

        assert_eq!(exact.projection_priority("alice"), Some(0));
        assert_eq!(prefix.projection_priority("alice"), Some(1));
        assert_eq!(contains.projection_priority("alice"), Some(2));
    }

    #[test]
    fn projection_priority_uses_best_matching_field() {
        let node = json!({
            "id": "alice-node",
            "title": "alice",
        });
        let row = GraphSearchIndexRow::from_node(0, &node);

        assert_eq!(row.projection_priority("alice"), Some(0));
    }

    #[test]
    fn projection_query_ranks_by_priority_amount_count_and_id() {
        let nodes = vec![
            json!({"id": "low", "title": "alice", "total_amount": 10, "total_count": 1}),
            json!({"id": "high", "title": "alice", "total_amount": 20, "total_count": 1}),
            json!({"id": "prefix", "title": "alice extra", "total_amount": 100, "total_count": 9}),
            json!({"id": "count", "title": "alice", "total_amount": 20, "total_count": 4}),
        ];
        let index = GraphSearchIndex::from_nodes(&nodes);

        assert_eq!(
            index.query_projection_ranked_node_ids(
                "alice",
                8,
                |node| text_value(node.get("id")),
                |node| GraphSearchProjectionWeights {
                    amount_abs: text_value(node.get("total_amount"))
                        .parse::<f64>()
                        .unwrap_or(0.0),
                    count: text_value(node.get("total_count"))
                        .parse::<i64>()
                        .unwrap_or(0),
                },
            ),
            vec![
                "count".to_string(),
                "high".to_string(),
                "low".to_string(),
                "prefix".to_string(),
            ]
        );
    }
}
