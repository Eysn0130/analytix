use anyhow::{Context, Result};
use std::collections::BTreeMap;

use super::super::input_model::{ProjectionEdge, MAX_SAFE_INTEGER};

#[derive(Clone)]
pub(super) struct ProjectedEdgeAgg {
    pub(super) a: String,
    pub(super) b: String,
    pub(super) amount_cents: i64,
    pub(super) count: i64,
    pub(super) out_amount_cents: i64,
    pub(super) in_amount_cents: i64,
    pub(super) first_time: Option<String>,
    pub(super) last_time: Option<String>,
}

impl ProjectedEdgeAgg {
    fn new(a: String, b: String) -> Self {
        Self {
            a,
            b,
            amount_cents: 0,
            count: 0,
            out_amount_cents: 0,
            in_amount_cents: 0,
            first_time: None,
            last_time: None,
        }
    }

    fn add_projected_edge(
        &mut self,
        edge: &ProjectionEdge,
        projection: &VisibleEdgeProjection,
    ) -> Result<()> {
        self.amount_cents = self
            .amount_cents
            .checked_add(edge.amount_cents)
            .context("projected edge amount overflow")?;
        ensure_safe_total(self.amount_cents, "projected edge amount")?;
        self.count = self
            .count
            .checked_add(edge.count)
            .context("projected edge count overflow")?;
        ensure_safe_total(self.count, "projected edge count")?;
        merge_edge_time(&mut self.first_time, edge.first_time.as_deref(), true);
        merge_edge_time(&mut self.last_time, edge.last_time.as_deref(), false);

        if projection.is_self_loop {
            self.out_amount_cents = self
                .out_amount_cents
                .checked_add(edge.forward_amount_cents)
                .context("projected self-loop forward amount overflow")?;
            ensure_safe_total(self.out_amount_cents, "projected self-loop forward amount")?;
            self.in_amount_cents = self
                .in_amount_cents
                .checked_add(edge.reverse_amount_cents)
                .context("projected self-loop reverse amount overflow")?;
            ensure_safe_total(self.in_amount_cents, "projected self-loop reverse amount")?;
            return self.validate_direction_coverage();
        }

        if projection.source_is_a {
            self.out_amount_cents = self
                .out_amount_cents
                .checked_add(edge.forward_amount_cents)
                .context("projected outgoing amount overflow")?;
            ensure_safe_total(self.out_amount_cents, "projected outgoing amount")?;
            self.in_amount_cents = self
                .in_amount_cents
                .checked_add(edge.reverse_amount_cents)
                .context("projected incoming amount overflow")?;
            ensure_safe_total(self.in_amount_cents, "projected incoming amount")?;
        } else {
            self.out_amount_cents = self
                .out_amount_cents
                .checked_add(edge.reverse_amount_cents)
                .context("projected outgoing amount overflow")?;
            ensure_safe_total(self.out_amount_cents, "projected outgoing amount")?;
            self.in_amount_cents = self
                .in_amount_cents
                .checked_add(edge.forward_amount_cents)
                .context("projected incoming amount overflow")?;
            ensure_safe_total(self.in_amount_cents, "projected incoming amount")?;
        }
        self.validate_direction_coverage()
    }

    pub(super) fn validate_direction_coverage(&self) -> Result<()> {
        let directional_total = self
            .out_amount_cents
            .checked_add(self.in_amount_cents)
            .context("projected directional amount overflow")?;
        if directional_total != self.amount_cents {
            anyhow::bail!("projected edge direction coverage is inconsistent");
        }
        Ok(())
    }
}

pub(super) struct ProjectedEdgeAggregation {
    pub(super) aggs: Vec<ProjectedEdgeAgg>,
    pub(super) total_amount_cents: i64,
    pub(super) total_count: i64,
}

pub(super) fn aggregate_projected_edges(
    edges: &[ProjectionEdge],
    collapsed_member_to_cluster: &BTreeMap<String, String>,
) -> Result<ProjectedEdgeAggregation> {
    let mut edge_aggs: BTreeMap<(String, String), ProjectedEdgeAgg> = BTreeMap::new();
    let mut total_amount_cents = 0_i64;
    let mut total_count = 0i64;

    for edge in edges {
        let source = &edge.source;
        let target = &edge.target;
        total_amount_cents = total_amount_cents
            .checked_add(edge.amount_cents)
            .context("projection total edge amount overflow")?;
        ensure_safe_total(total_amount_cents, "projection total edge amount")?;
        total_count = total_count
            .checked_add(edge.count)
            .context("projection total edge count overflow")?;
        ensure_safe_total(total_count, "projection total edge count")?;

        let projection = visible_edge_projection(&source, &target, collapsed_member_to_cluster);

        let agg = edge_aggs
            .entry((projection.a.clone(), projection.b.clone()))
            .or_insert_with_key(|(a, b)| ProjectedEdgeAgg::new(a.clone(), b.clone()));
        agg.add_projected_edge(edge, &projection)?;
    }

    Ok(ProjectedEdgeAggregation {
        aggs: edge_aggs.into_values().collect(),
        total_amount_cents,
        total_count,
    })
}

fn ensure_safe_total(value: i64, label: &str) -> Result<()> {
    if value > MAX_SAFE_INTEGER {
        anyhow::bail!("{label} exceeds safe range");
    }
    Ok(())
}

#[derive(Debug, PartialEq)]
struct VisibleEdgeProjection {
    a: String,
    b: String,
    source_is_a: bool,
    is_self_loop: bool,
}

fn visible_edge_projection(
    source: &str,
    target: &str,
    collapsed_member_to_cluster: &BTreeMap<String, String>,
) -> VisibleEdgeProjection {
    let source_visible = visible_node_id(source, collapsed_member_to_cluster);
    let target_visible = visible_node_id(target, collapsed_member_to_cluster);
    let source_is_a = source_visible <= target_visible;
    let (a, b) = if source_is_a {
        (source_visible, target_visible)
    } else {
        (target_visible, source_visible)
    };
    VisibleEdgeProjection {
        a: a.to_string(),
        b: b.to_string(),
        source_is_a,
        is_self_loop: source_visible == target_visible,
    }
}

fn visible_node_id<'a>(
    node_id: &'a str,
    collapsed_member_to_cluster: &'a BTreeMap<String, String>,
) -> &'a str {
    collapsed_member_to_cluster
        .get(node_id)
        .map(String::as_str)
        .unwrap_or(node_id)
}

fn merge_edge_time(current: &mut Option<String>, next: Option<&str>, first: bool) {
    let Some(next) = next else {
        return;
    };
    let replace = match current.as_deref() {
        None => true,
        Some(value) if first => next < value,
        Some(value) => next > value,
    };
    if replace {
        *current = Some(next.to_string());
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn visible_edge_projection_preserves_sorted_key_and_source_orientation() {
        let collapsed = BTreeMap::from([("leaf".to_string(), "cluster".to_string())]);

        assert_eq!(
            visible_edge_projection("seed", "leaf", &collapsed),
            VisibleEdgeProjection {
                a: "cluster".to_string(),
                b: "seed".to_string(),
                source_is_a: false,
                is_self_loop: false,
            }
        );
        assert_eq!(
            visible_edge_projection("leaf", "seed", &collapsed),
            VisibleEdgeProjection {
                a: "cluster".to_string(),
                b: "seed".to_string(),
                source_is_a: true,
                is_self_loop: false,
            }
        );
        assert_eq!(
            visible_edge_projection("leaf", "leaf", &collapsed),
            VisibleEdgeProjection {
                a: "cluster".to_string(),
                b: "cluster".to_string(),
                source_is_a: true,
                is_self_loop: true,
            }
        );
    }

    #[test]
    fn projected_edge_agg_adds_directional_amounts_from_visible_orientation() {
        let projection = VisibleEdgeProjection {
            a: "cluster".to_string(),
            b: "seed".to_string(),
            source_is_a: false,
            is_self_loop: false,
        };
        let mut agg = ProjectedEdgeAgg::new("cluster".to_string(), "seed".to_string());
        agg.add_projected_edge(
            &ProjectionEdge {
                id: "edge-1".to_string(),
                source: "seed".to_string(),
                target: "cluster".to_string(),
                amount_cents: 10_125,
                count: 2,
                out_amount_cents: 0,
                in_amount_cents: 10_125,
                forward_amount_cents: 10_125,
                reverse_amount_cents: 0,
                first_time: Some("2026-01-02".to_string()),
                last_time: Some("2026-01-03".to_string()),
            },
            &projection,
        )
        .unwrap();
        agg.add_projected_edge(
            &ProjectionEdge {
                id: "edge-2".to_string(),
                source: "cluster".to_string(),
                target: "seed".to_string(),
                amount_cents: 1_000,
                count: 1,
                out_amount_cents: 400,
                in_amount_cents: 600,
                forward_amount_cents: 400,
                reverse_amount_cents: 600,
                first_time: Some("2026-01-01".to_string()),
                last_time: Some("2026-01-04".to_string()),
            },
            &VisibleEdgeProjection {
                source_is_a: true,
                ..projection
            },
        )
        .unwrap();

        assert_eq!(agg.amount_cents, 11_125);
        assert_eq!(agg.count, 3);
        assert_eq!(agg.out_amount_cents, 400);
        assert_eq!(agg.in_amount_cents, 10_725);
        assert_eq!(agg.first_time.as_deref(), Some("2026-01-01"));
        assert_eq!(agg.last_time.as_deref(), Some("2026-01-04"));
    }

    #[test]
    fn projected_self_loop_preserves_directional_receipt_values() {
        let projection = VisibleEdgeProjection {
            a: "cluster".to_string(),
            b: "cluster".to_string(),
            source_is_a: true,
            is_self_loop: true,
        };
        let mut agg = ProjectedEdgeAgg::new("cluster".to_string(), "cluster".to_string());
        agg.add_projected_edge(
            &ProjectionEdge {
                id: "edge-1".to_string(),
                source: "member-a".to_string(),
                target: "member-b".to_string(),
                amount_cents: 1_000,
                count: 1,
                out_amount_cents: 400,
                in_amount_cents: 600,
                forward_amount_cents: 400,
                reverse_amount_cents: 600,
                first_time: None,
                last_time: None,
            },
            &projection,
        )
        .unwrap();

        assert_eq!(agg.amount_cents, 1_000);
        assert_eq!(agg.out_amount_cents, 400);
        assert_eq!(agg.in_amount_cents, 600);
    }
}
