use super::super::aggregate_features::{
    compute_night_activity_feature, compute_round_amount_features,
};
use super::{default_args, txn_row, txn_row_at};

#[test]
fn computes_round_amount_pattern_per_direction_with_sorted_unique_amounts() {
    let mut args = default_args();
    args.round_unit = 10_000.0;
    args.round_min_amount = 10_000.0;
    args.round_min_count = 3;
    args.round_min_total_amount = 50_000.0;

    let rows = vec![
        txn_row("txn-round-in-1", 0, 20_000.0, "in"),
        txn_row("txn-round-in-2", 10 * 60, 30_000.0, "in"),
        txn_row("txn-round-in-3", 20 * 60, 10_000.0, "in"),
        txn_row("txn-round-nonround-noise", 25 * 60, 12_500.0, "in"),
        txn_row("txn-round-below-min-noise", 30 * 60, 5_000.0, "in"),
        txn_row("txn-round-out-noise-1", 35 * 60, 20_000.0, "out"),
        txn_row("txn-round-out-noise-2", 40 * 60, 30_000.0, "out"),
    ];
    let row_refs = rows.iter().collect::<Vec<_>>();

    let features = compute_round_amount_features(&args, "A-003", &row_refs).unwrap();

    assert_eq!(features.len(), 1);
    let feature = &features[0];
    assert_eq!(feature.feature_code, "ROUND_AMOUNT_PATTERN");
    assert_eq!(feature.account_key, "A-003");
    assert_eq!(feature.direction, "in");
    assert_eq!(feature.txn_count, 3);
    assert_eq!(feature.total_amount, 60_000.0);
    assert_eq!(feature.first_time, "2026-04-11 13:00:00");
    assert_eq!(feature.last_time, "2026-04-11 13:20:00");
    assert_eq!(feature.round_unit, Some(10_000.0));
    assert_eq!(feature.min_amount, 10_000.0);
    assert_eq!(feature.min_count, 3);
    assert_eq!(feature.min_total_amount, 50_000.0);
    assert_eq!(feature.detail_json, "{}");

    let txn_ids = serde_json::from_str::<Vec<String>>(&feature.txn_ids_json).unwrap();
    assert_eq!(
        txn_ids,
        vec![
            "txn-round-in-1".to_string(),
            "txn-round-in-2".to_string(),
            "txn-round-in-3".to_string(),
        ]
    );
    let amounts = serde_json::from_str::<Vec<f64>>(&feature.amounts_json).unwrap();
    assert_eq!(amounts, vec![10_000.0, 20_000.0, 30_000.0]);
}

#[test]
fn computes_night_offhour_activity_across_midnight_window() {
    let mut args = default_args();
    args.night_min_count = 3;
    args.night_min_total_amount = 65_000.0;

    let rows = vec![
        txn_row("txn-daytime-noise", 60 * 60, 100_000.0, "in"),
        txn_row_at(
            "txn-night-1",
            "2026-04-11 23:00:00",
            10 * 60 * 60,
            23,
            20_000.0,
            "in",
        ),
        txn_row_at(
            "txn-night-2",
            "2026-04-11 23:20:00",
            10 * 60 * 60 + 20 * 60,
            23,
            40_000.0,
            "out",
        ),
        txn_row_at(
            "txn-night-next-morning",
            "2026-04-12 05:30:00",
            16 * 60 * 60 + 30 * 60,
            5,
            10_000.0,
            "out",
        ),
    ];
    let row_refs = rows.iter().collect::<Vec<_>>();

    let feature = compute_night_activity_feature(&args, "A-003", &row_refs)
        .unwrap()
        .expect("night offhour feature");

    assert_eq!(feature.feature_code, "NIGHT_OFFHOUR_ACTIVITY");
    assert_eq!(feature.account_key, "A-003");
    assert_eq!(feature.direction, "");
    assert_eq!(feature.txn_count, 3);
    assert_eq!(feature.total_amount, 70_000.0);
    assert_eq!(feature.first_time, "2026-04-11 23:00:00");
    assert_eq!(feature.last_time, "2026-04-12 05:30:00");
    assert_eq!(feature.round_unit, None);
    assert_eq!(feature.min_amount, 0.0);
    assert_eq!(feature.min_count, 3);
    assert_eq!(feature.min_total_amount, 65_000.0);
    assert_eq!(feature.start_hour, Some(22));
    assert_eq!(feature.end_hour, Some(6));
    assert_eq!(feature.amounts_json, "[]");
    assert_eq!(feature.detail_json, "{}");

    let txn_ids = serde_json::from_str::<Vec<String>>(&feature.txn_ids_json).unwrap();
    assert_eq!(
        txn_ids,
        vec![
            "txn-night-1".to_string(),
            "txn-night-2".to_string(),
            "txn-night-next-morning".to_string(),
        ]
    );
}
