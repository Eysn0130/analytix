use super::super::window_features::{
    compute_high_freq_small_out_features, compute_near_threshold_features,
    compute_repeated_amount_features, compute_threshold_split_features,
};
use super::{default_args, txn_row};

#[test]
fn computes_repeated_amount_pattern_first_qualified_window() {
    let mut args = default_args();
    args.repeated_amount_window_minutes = 30;

    let rows = vec![
        txn_row("txn-repeated-out-1", 0, 4_000.0, "out"),
        txn_row("txn-in-direction-noise", 5 * 60, 4_000.0, "in"),
        txn_row("txn-repeated-out-2", 10 * 60, 4_000.0, "out"),
        txn_row("txn-different-amount-noise", 15 * 60, 3_999.99, "out"),
        txn_row("txn-repeated-out-3", 20 * 60, 4_000.0, "out"),
        txn_row("txn-below-min-noise", 25 * 60, 900.0, "out"),
        txn_row("txn-late-repeated-noise", 40 * 60, 4_000.0, "out"),
    ];
    let row_refs = rows.iter().collect::<Vec<_>>();

    let features = compute_repeated_amount_features(&args, "A-009", &row_refs).unwrap();

    assert_eq!(features.len(), 1);
    let feature = &features[0];
    assert_eq!(feature.feature_code, "REPEATED_AMOUNT_PATTERN");
    assert_eq!(feature.account_key, "A-009");
    assert_eq!(feature.direction, "out");
    assert_eq!(feature.txn_count, 3);
    assert_eq!(feature.total_amount, 12_000.0);
    assert_eq!(feature.first_time, "2026-04-11 13:00:00");
    assert_eq!(feature.last_time, "2026-04-11 13:20:00");
    assert_eq!(feature.min_amount, 1_000.0);
    assert_eq!(feature.min_count, 3);
    assert_eq!(feature.min_total_amount, 10_000.0);

    let txn_ids = serde_json::from_str::<Vec<String>>(&feature.txn_ids_json).unwrap();
    assert_eq!(
        txn_ids,
        vec![
            "txn-repeated-out-1".to_string(),
            "txn-repeated-out-2".to_string(),
            "txn-repeated-out-3".to_string(),
        ]
    );
    let amounts = serde_json::from_str::<Vec<f64>>(&feature.amounts_json).unwrap();
    assert_eq!(amounts, vec![4_000.0]);
    let detail = serde_json::from_str::<serde_json::Value>(&feature.detail_json).unwrap();
    assert_eq!(detail, serde_json::json!({"amount": 4_000.0}));
}

#[test]
fn computes_near_threshold_structuring_once_per_qualified_window() {
    let mut args = default_args();
    args.near_threshold_window_minutes = 120;

    let rows = vec![
        txn_row("txn-near-threshold-1", 0, 49_000.0, "out"),
        txn_row("txn-near-threshold-2", 10 * 60, 48_000.0, "out"),
        txn_row("txn-over-threshold-noise", 20 * 60, 51_000.0, "out"),
        txn_row("txn-in-direction-noise", 30 * 60, 49_000.0, "in"),
        txn_row("txn-late-near-noise", 150 * 60, 49_000.0, "out"),
    ];
    let row_refs = rows.iter().collect::<Vec<_>>();

    let features = compute_near_threshold_features(&args, "A-008", &row_refs).unwrap();

    assert_eq!(features.len(), 1);
    let feature = &features[0];
    assert_eq!(feature.feature_code, "NEAR_THRESHOLD_STRUCTURING");
    assert_eq!(feature.account_key, "A-008");
    assert_eq!(feature.direction, "out");
    assert_eq!(feature.txn_count, 2);
    assert_eq!(feature.total_amount, 97_000.0);
    assert_eq!(feature.first_time, "2026-04-11 13:00:00");
    assert_eq!(feature.last_time, "2026-04-11 13:10:00");
    assert_eq!(feature.min_amount, 45_000.0);
    assert_eq!(feature.min_count, 2);
    assert_eq!(feature.min_total_amount, 90_000.0);

    let txn_ids = serde_json::from_str::<Vec<String>>(&feature.txn_ids_json).unwrap();
    assert_eq!(
        txn_ids,
        vec![
            "txn-near-threshold-1".to_string(),
            "txn-near-threshold-2".to_string(),
        ]
    );
    let amounts = serde_json::from_str::<Vec<f64>>(&feature.amounts_json).unwrap();
    assert_eq!(amounts, vec![49_000.0, 48_000.0]);
    let detail = serde_json::from_str::<serde_json::Value>(&feature.detail_json).unwrap();
    assert_eq!(
        detail,
        serde_json::json!({
            "threshold": 50_000.0,
            "lower_rate": 0.9,
            "lower_amount": 45_000.0,
            "amount_min": 48_000.0,
            "amount_max": 49_000.0,
        })
    );
}

#[test]
fn computes_threshold_split_and_skips_matched_window() {
    let args = default_args();
    let rows = vec![
        txn_row("txn-threshold-split-1", 0, 50_000.0, "in"),
        txn_row("txn-threshold-split-2", 5 * 60, 49_500.0, "in"),
        txn_row("txn-threshold-split-3", 10 * 60, 50_500.0, "in"),
        txn_row("txn-over-tolerance-noise", 15 * 60, 53_000.0, "in"),
        txn_row("txn-out-direction-noise", 20 * 60, 50_000.0, "out"),
        txn_row("txn-late-split-noise", 60 * 60, 50_000.0, "in"),
    ];
    let row_refs = rows.iter().collect::<Vec<_>>();

    let features = compute_threshold_split_features(&args, "A-010", &row_refs).unwrap();

    assert_eq!(features.len(), 1);
    let feature = &features[0];
    assert_eq!(feature.feature_code, "THRESHOLD_SPLIT");
    assert_eq!(feature.account_key, "A-010");
    assert_eq!(feature.direction, "in");
    assert_eq!(feature.txn_count, 3);
    assert_eq!(feature.total_amount, 150_000.0);
    assert_eq!(feature.first_time, "2026-04-11 13:00:00");
    assert_eq!(feature.last_time, "2026-04-11 13:10:00");
    assert_eq!(feature.min_amount, 47_500.0);
    assert_eq!(feature.min_count, 3);

    let txn_ids = serde_json::from_str::<Vec<String>>(&feature.txn_ids_json).unwrap();
    assert_eq!(
        txn_ids,
        vec![
            "txn-threshold-split-1".to_string(),
            "txn-threshold-split-2".to_string(),
            "txn-threshold-split-3".to_string(),
        ]
    );
    let amounts = serde_json::from_str::<Vec<f64>>(&feature.amounts_json).unwrap();
    assert_eq!(amounts, vec![50_000.0, 49_500.0, 50_500.0]);
    let detail = serde_json::from_str::<serde_json::Value>(&feature.detail_json).unwrap();
    assert_eq!(
        detail,
        serde_json::json!({
            "threshold": 50_000.0,
            "tolerance_rate": 0.05,
            "amount_min": 49_500.0,
            "amount_max": 50_500.0,
        })
    );
}

#[test]
fn computes_high_freq_small_out_qualified_window_only() {
    let args = default_args();
    let mut rows = vec![
        txn_row("txn-in-noise", -120, 3000.0, "in"),
        txn_row("txn-large-out-noise", -60, 6000.0, "out"),
    ];
    for index in 0..8 {
        rows.push(txn_row(
            &format!("txn-highfreq-out-{}", index + 1),
            index * 180,
            3000.0,
            "out",
        ));
    }
    rows.push(txn_row("txn-late-out-noise", 3600, 3000.0, "out"));
    let row_refs = rows.iter().collect::<Vec<_>>();

    let features = compute_high_freq_small_out_features(&args, "A-011", &row_refs).unwrap();

    assert_eq!(features.len(), 1);
    let feature = &features[0];
    assert_eq!(feature.feature_code, "HIGH_FREQ_SMALL_OUT");
    assert_eq!(feature.account_key, "A-011");
    assert_eq!(feature.direction, "out");
    assert_eq!(feature.txn_count, 8);
    assert_eq!(feature.total_amount, 24000.0);
    assert_eq!(feature.first_time, "2026-04-11 13:00:00");
    assert_eq!(feature.last_time, "2026-04-11 13:21:00");
    assert_eq!(feature.min_count, 8);
    assert_eq!(feature.min_total_amount, 20000.0);

    let txn_ids = serde_json::from_str::<Vec<String>>(&feature.txn_ids_json).unwrap();
    assert_eq!(
        txn_ids,
        (1..=8)
            .map(|index| format!("txn-highfreq-out-{index}"))
            .collect::<Vec<_>>()
    );
    let detail = serde_json::from_str::<serde_json::Value>(&feature.detail_json).unwrap();
    assert_eq!(
        detail,
        serde_json::json!({
            "small_amount_threshold": 5000.0,
            "min_total_amount": 20000.0,
            "total_out_amount": 24000.0,
        })
    );
}
