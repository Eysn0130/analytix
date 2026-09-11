use serde_json::Value;

#[derive(Clone, Debug, PartialEq)]
pub(super) struct ProjectedNodeRow(Value);

impl ProjectedNodeRow {
    pub(super) fn from_object_value(value: Value) -> Self {
        assert!(
            value.is_object(),
            "projected node row must be a JSON object"
        );
        Self(value)
    }

    pub(super) fn into_value(self) -> Value {
        self.0
    }

    pub(super) fn into_values(rows: Vec<Self>) -> Vec<Value> {
        rows.into_iter().map(Self::into_value).collect()
    }
}

#[derive(Clone, Debug, PartialEq)]
pub(super) struct ClusterProjectionRow(Value);

impl ClusterProjectionRow {
    pub(super) fn from_object_value(value: Value) -> Self {
        assert!(
            value.is_object(),
            "cluster projection row must be a JSON object"
        );
        Self(value)
    }

    pub(super) fn into_value(self) -> Value {
        self.0
    }

    pub(super) fn into_values(rows: Vec<Self>) -> Vec<Value> {
        rows.into_iter().map(Self::into_value).collect()
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use serde_json::json;

    #[test]
    fn row_wrappers_keep_node_and_cluster_rows_distinct_until_rendering() {
        let node_rows = vec![ProjectedNodeRow::from_object_value(json!({"id": "node-a"}))];
        let cluster_rows = vec![ClusterProjectionRow::from_object_value(
            json!({"cluster_id": "cluster-a"}),
        )];

        assert_eq!(node_rows[0].clone().into_value(), json!({"id": "node-a"}));
        assert_eq!(
            ProjectedNodeRow::into_values(node_rows),
            vec![json!({"id": "node-a"})]
        );
        assert_eq!(
            ClusterProjectionRow::into_values(cluster_rows),
            vec![json!({"cluster_id": "cluster-a"})]
        );
    }

    #[test]
    #[should_panic(expected = "projected node row must be a JSON object")]
    fn projected_node_row_rejects_non_object_values() {
        let _ = ProjectedNodeRow::from_object_value(json!(["not", "an", "object"]));
    }

    #[test]
    #[should_panic(expected = "cluster projection row must be a JSON object")]
    fn cluster_projection_row_rejects_non_object_values() {
        let _ = ClusterProjectionRow::from_object_value(json!(null));
    }
}
