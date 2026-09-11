use anyhow::{bail, Context, Result};
use serde_json::{json, Map, Value};

use super::super::helpers::text_field;
use super::super::input_model::money_value;
use super::aggregation::{ProjectedEdgeAgg, ProjectedEdgeAggregation};
use super::render_model::{render_projected_edge_row, ProjectedEdgeViewMode};

pub(super) fn projected_edge_view_mode(
    request_context: Option<&Map<String, Value>>,
) -> ProjectedEdgeViewMode {
    match text_field(request_context, "view_mode")
        .trim()
        .to_ascii_lowercase()
        .as_str()
    {
        "net" => ProjectedEdgeViewMode::Net,
        _ => ProjectedEdgeViewMode::Relation,
    }
}

pub(super) fn render_projected_edge_plan(
    mut aggregation: ProjectedEdgeAggregation,
    view_mode: ProjectedEdgeViewMode,
) -> Result<Value> {
    let (amount_total, count_total) =
        aggregation
            .aggs
            .iter()
            .try_fold((0_i64, 0_i64), |(amount_total, count_total), agg| {
                agg.validate_direction_coverage()?;
                Ok::<_, anyhow::Error>((
                    amount_total
                        .checked_add(agg.amount_cents)
                        .context("projected edge plan amount overflow")?,
                    count_total
                        .checked_add(agg.count)
                        .context("projected edge plan count overflow")?,
                ))
            })?;
    if amount_total != aggregation.total_amount_cents || count_total != aggregation.total_count {
        bail!("projected edge plan totals are inconsistent");
    }
    sort_projected_edge_aggs(&mut aggregation.aggs);
    let projected_edges = projected_edge_rows(&aggregation.aggs, view_mode)?;

    Ok(json!({
        "edges": projected_edges,
        "total_amount": money_value(aggregation.total_amount_cents),
        "total_count": aggregation.total_count,
    }))
}

fn sort_projected_edge_aggs(aggs: &mut [ProjectedEdgeAgg]) {
    aggs.sort_by(|left, right| {
        right
            .amount_cents
            .cmp(&left.amount_cents)
            .then_with(|| left.a.cmp(&right.a))
            .then_with(|| left.b.cmp(&right.b))
    });
}

fn projected_edge_rows(
    aggs: &[ProjectedEdgeAgg],
    view_mode: ProjectedEdgeViewMode,
) -> Result<Vec<Value>> {
    let mut rows = Vec::new();
    for agg in aggs {
        if let Some(row) = render_projected_edge_row(agg, view_mode)? {
            rows.push(row);
        }
    }
    Ok(rows)
}

#[cfg(test)]
mod tests {
    use super::*;
    use serde_json::json;

    fn edge_agg(
        a: &str,
        b: &str,
        amount_cents: i64,
        count: i64,
        out_amount_cents: i64,
        in_amount_cents: i64,
    ) -> ProjectedEdgeAgg {
        ProjectedEdgeAgg {
            a: a.to_string(),
            b: b.to_string(),
            amount_cents,
            count,
            out_amount_cents,
            in_amount_cents,
            first_time: Some("2026-03-01".to_string()),
            last_time: Some("2026-03-05".to_string()),
        }
    }

    #[test]
    fn projected_edge_view_mode_defaults_to_relation_and_accepts_net() {
        let mut request_context = Map::new();
        assert_eq!(
            projected_edge_view_mode(Some(&request_context)),
            ProjectedEdgeViewMode::Relation
        );

        request_context.insert("view_mode".to_string(), json!(" Net "));
        assert_eq!(
            projected_edge_view_mode(Some(&request_context)),
            ProjectedEdgeViewMode::Net
        );
    }

    #[test]
    fn render_projected_edge_plan_sorts_rows_and_preserves_totals() {
        let aggregation = ProjectedEdgeAggregation {
            aggs: vec![
                edge_agg("b", "c", 1_000, 2, 1_000, 0),
                edge_agg("a", "b", 1_100, 1, 1_100, 0),
                edge_agg("a", "c", 1_000, 3, 1_000, 0),
            ],
            total_amount_cents: 3_100,
            total_count: 6,
        };

        assert_eq!(
            render_projected_edge_plan(aggregation, ProjectedEdgeViewMode::Relation).unwrap(),
            json!({
                "edges": [
                    {
                        "id": "a==b",
                        "source": "a",
                        "target": "b",
                        "amount": 11.0,
                        "count": 1,
                        "out_amount": 11.0,
                        "in_amount": 0.0,
                        "forward_amount": 11.0,
                        "reverse_amount": 0.0,
                        "mode": "single",
                        "first_time": "2026-03-01",
                        "last_time": "2026-03-05",
                        "projection_edge": true,
                    },
                    {
                        "id": "a==c",
                        "source": "a",
                        "target": "c",
                        "amount": 10.0,
                        "count": 3,
                        "out_amount": 10.0,
                        "in_amount": 0.0,
                        "forward_amount": 10.0,
                        "reverse_amount": 0.0,
                        "mode": "single",
                        "first_time": "2026-03-01",
                        "last_time": "2026-03-05",
                        "projection_edge": true,
                    },
                    {
                        "id": "b==c",
                        "source": "b",
                        "target": "c",
                        "amount": 10.0,
                        "count": 2,
                        "out_amount": 10.0,
                        "in_amount": 0.0,
                        "forward_amount": 10.0,
                        "reverse_amount": 0.0,
                        "mode": "single",
                        "first_time": "2026-03-01",
                        "last_time": "2026-03-05",
                        "projection_edge": true,
                    },
                ],
                "total_amount": 31.0,
                "total_count": 6,
            })
        );
    }

    #[test]
    fn render_projected_edge_plan_filters_balanced_net_rows_without_changing_totals() {
        let aggregation = ProjectedEdgeAggregation {
            aggs: vec![
                edge_agg("a", "b", 1_000, 2, 500, 500),
                edge_agg("a", "c", 1_000, 3, 900, 100),
            ],
            total_amount_cents: 2_000,
            total_count: 5,
        };

        assert_eq!(
            render_projected_edge_plan(aggregation, ProjectedEdgeViewMode::Net).unwrap(),
            json!({
                "edges": [
                    {
                        "id": "a==c",
                        "source": "a",
                        "target": "c",
                        "amount": 8.0,
                        "count": 3,
                        "out_amount": 9.0,
                        "in_amount": 1.0,
                        "forward_amount": 8.0,
                        "reverse_amount": 0.0,
                        "mode": "single",
                        "label": "￥8.00",
                        "first_time": "2026-03-01",
                        "last_time": "2026-03-05",
                        "projection_edge": true,
                    },
                ],
                "total_amount": 20.0,
                "total_count": 5,
            })
        );
    }

    #[test]
    fn render_projected_edge_plan_rejects_total_drift() {
        let aggregation = ProjectedEdgeAggregation {
            aggs: vec![edge_agg("a", "b", 1_000, 2, 1_000, 0)],
            total_amount_cents: 999,
            total_count: 2,
        };
        assert!(render_projected_edge_plan(aggregation, ProjectedEdgeViewMode::Relation).is_err());
    }
}
