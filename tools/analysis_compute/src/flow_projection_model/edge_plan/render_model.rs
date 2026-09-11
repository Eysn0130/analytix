use anyhow::{Context, Result};
use serde_json::{json, Value};

use super::super::input_model::money_value;
use super::aggregation::ProjectedEdgeAgg;

#[derive(Clone, Copy, Debug, Eq, PartialEq)]
pub(super) enum ProjectedEdgeViewMode {
    Relation,
    Net,
}

pub(super) fn render_projected_edge_row(
    agg: &ProjectedEdgeAgg,
    view_mode: ProjectedEdgeViewMode,
) -> Result<Option<Value>> {
    agg.validate_direction_coverage()?;
    match view_mode {
        ProjectedEdgeViewMode::Net => render_net_projected_edge_row(agg),
        ProjectedEdgeViewMode::Relation => Ok(Some(render_relation_projected_edge_row(agg))),
    }
}

fn render_net_projected_edge_row(agg: &ProjectedEdgeAgg) -> Result<Option<Value>> {
    let net_cents = agg
        .out_amount_cents
        .checked_sub(agg.in_amount_cents)
        .context("projected net amount overflow")?;
    if net_cents == 0 {
        return Ok(None);
    }
    let net_amount_cents = i64::try_from(net_cents.unsigned_abs())
        .context("projected net amount exceeds signed range")?;
    let (source_id, target_id) = if net_cents > 0 {
        (agg.a.clone(), agg.b.clone())
    } else {
        (agg.b.clone(), agg.a.clone())
    };
    Ok(Some(json!({
        "id": format!("{}=={}", agg.a, agg.b),
        "source": source_id,
        "target": target_id,
        "amount": money_value(net_amount_cents),
        "count": agg.count,
        "out_amount": money_value(agg.out_amount_cents),
        "in_amount": money_value(agg.in_amount_cents),
        "forward_amount": money_value(net_amount_cents),
        "reverse_amount": 0.0,
        "mode": "single",
        "label": format!("￥{}.{:02}", net_amount_cents / 100, net_amount_cents % 100),
        "first_time": agg.first_time,
        "last_time": agg.last_time,
        "projection_edge": true,
    })))
}

fn render_relation_projected_edge_row(agg: &ProjectedEdgeAgg) -> Value {
    let direction = relation_projected_edge_direction(agg);
    json!({
        "id": format!("{}=={}", agg.a, agg.b),
        "source": direction.source_id,
        "target": direction.target_id,
        "amount": money_value(agg.amount_cents),
        "count": agg.count,
        "out_amount": money_value(agg.out_amount_cents),
        "in_amount": money_value(agg.in_amount_cents),
        "forward_amount": money_value(direction.forward_amount_cents),
        "reverse_amount": money_value(direction.reverse_amount_cents),
        "mode": relation_projected_edge_mode(agg),
        "first_time": agg.first_time,
        "last_time": agg.last_time,
        "projection_edge": true,
    })
}

#[derive(Debug, PartialEq)]
struct ProjectedEdgeDirection {
    source_id: String,
    target_id: String,
    forward_amount_cents: i64,
    reverse_amount_cents: i64,
}

fn relation_projected_edge_direction(agg: &ProjectedEdgeAgg) -> ProjectedEdgeDirection {
    let (source_id, target_id, forward_amount_cents, reverse_amount_cents) = if agg.a == agg.b {
        (
            agg.a.clone(),
            agg.b.clone(),
            agg.out_amount_cents,
            agg.in_amount_cents,
        )
    } else if agg.out_amount_cents > 0 && agg.in_amount_cents == 0 {
        (
            agg.a.clone(),
            agg.b.clone(),
            agg.out_amount_cents,
            agg.in_amount_cents,
        )
    } else if agg.in_amount_cents > 0 && agg.out_amount_cents == 0 {
        (
            agg.b.clone(),
            agg.a.clone(),
            agg.in_amount_cents,
            agg.out_amount_cents,
        )
    } else if agg.out_amount_cents >= agg.in_amount_cents {
        (
            agg.a.clone(),
            agg.b.clone(),
            agg.out_amount_cents,
            agg.in_amount_cents,
        )
    } else {
        (
            agg.b.clone(),
            agg.a.clone(),
            agg.in_amount_cents,
            agg.out_amount_cents,
        )
    };

    ProjectedEdgeDirection {
        source_id,
        target_id,
        forward_amount_cents,
        reverse_amount_cents,
    }
}

fn relation_projected_edge_mode(agg: &ProjectedEdgeAgg) -> &'static str {
    if agg.in_amount_cents > 0 && agg.out_amount_cents > 0 {
        "double"
    } else {
        "single"
    }
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
            first_time: Some("2026-02-01".to_string()),
            last_time: Some("2026-02-03".to_string()),
        }
    }

    #[test]
    fn relation_row_prefers_larger_visible_direction_without_losing_reverse_amount() {
        let row = render_projected_edge_row(
            &edge_agg("account-a", "account-b", 1_200, 3, 400, 800),
            ProjectedEdgeViewMode::Relation,
        )
        .unwrap();

        assert_eq!(
            row,
            Some(json!({
                "id": "account-a==account-b",
                "source": "account-b",
                "target": "account-a",
                "amount": 12.0,
                "count": 3,
                "out_amount": 4.0,
                "in_amount": 8.0,
                "forward_amount": 8.0,
                "reverse_amount": 4.0,
                "mode": "double",
                "first_time": "2026-02-01",
                "last_time": "2026-02-03",
                "projection_edge": true,
            }))
        );
    }

    #[test]
    fn net_row_uses_net_direction_and_filters_balanced_edges() {
        assert_eq!(
            render_projected_edge_row(
                &edge_agg("account-a", "account-b", 1_000, 3, 500, 500),
                ProjectedEdgeViewMode::Net,
            )
            .unwrap(),
            None
        );

        let row = render_projected_edge_row(
            &edge_agg("account-a", "account-b", 1_150, 3, 300, 850),
            ProjectedEdgeViewMode::Net,
        )
        .unwrap();

        assert_eq!(
            row,
            Some(json!({
                "id": "account-a==account-b",
                "source": "account-b",
                "target": "account-a",
                "amount": 5.5,
                "count": 3,
                "out_amount": 3.0,
                "in_amount": 8.5,
                "forward_amount": 5.5,
                "reverse_amount": 0.0,
                "mode": "single",
                "label": "￥5.50",
                "first_time": "2026-02-01",
                "last_time": "2026-02-03",
                "projection_edge": true,
            }))
        );
    }

    #[test]
    fn projected_edge_row_rejects_direction_coverage_drift() {
        assert!(render_projected_edge_row(
            &edge_agg("account-a", "account-b", 1_235, 3, 400, 800),
            ProjectedEdgeViewMode::Relation,
        )
        .is_err());
    }
}
