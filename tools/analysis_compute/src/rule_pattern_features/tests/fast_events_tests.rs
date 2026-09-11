use super::super::window_features::{
    compute_cash_quick_event_features, compute_small_fast_event_features,
};
use super::{cash_txn_row, default_args, txn_row};

#[test]
fn computes_small_fast_event_until_next_incoming_window() {
    let args = default_args();
    let mut rows = vec![
        txn_row("txn-small-fast-in-1", 0, 4000.0, "in"),
        txn_row("txn-small-fast-out-1", 300, 1500.0, "out"),
        txn_row("txn-small-fast-out-2", 600, 1700.0, "out"),
        cash_txn_row("txn-cash-out-noise", 660, 2000.0, "out"),
        txn_row("txn-next-in", 900, 3500.0, "in"),
        txn_row("txn-after-next-in-noise", 960, 2000.0, "out"),
    ];
    rows.sort_by_key(|row| row.txn_epoch);
    let row_refs = rows.iter().collect::<Vec<_>>();

    let features = compute_small_fast_event_features(&args, "A-004", &row_refs).unwrap();

    assert_eq!(features.len(), 1);
    let feature = &features[0];
    assert_eq!(feature.feature_code, "SMALL_FAST_EVENT");
    assert_eq!(feature.account_key, "A-004");
    assert_eq!(feature.txn_count, 3);
    assert_eq!(feature.total_amount, 4000.0);
    assert_eq!(feature.first_time, "2026-04-11 13:00:00");
    assert_eq!(feature.last_time, "2026-04-11 13:10:00");

    let txn_ids = serde_json::from_str::<Vec<String>>(&feature.txn_ids_json).unwrap();
    assert_eq!(
        txn_ids,
        vec![
            "txn-small-fast-in-1".to_string(),
            "txn-small-fast-out-1".to_string(),
            "txn-small-fast-out-2".to_string(),
        ]
    );
    let detail = serde_json::from_str::<serde_json::Value>(&feature.detail_json).unwrap();
    assert_eq!(
        detail,
        serde_json::json!({
            "in_txn_id": "txn-small-fast-in-1",
            "out_txn_ids": ["txn-small-fast-out-1", "txn-small-fast-out-2"],
            "in_amount": 4000.0,
            "out_amount": 3200.0,
            "out_ratio": 0.8,
            "duration_minutes": 10.0,
            "start_time": "2026-04-11 13:00:00",
            "end_time": "2026-04-11 13:10:00",
        })
    );
}

#[test]
fn computes_cash_quick_formal_and_candidate_events() {
    let args = default_args();
    let rows = vec![
        cash_txn_row("txn-cash-quick-in-1", 0, 8000.0, "in"),
        cash_txn_row("txn-cash-quick-out-1", 900, 7800.0, "out"),
        txn_row("txn-noncash-out-noise", 1200, 7600.0, "out"),
        cash_txn_row("txn-late-cash-out-noise", 7200, 7800.0, "out"),
    ];
    let row_refs = rows.iter().collect::<Vec<_>>();

    let features = compute_cash_quick_event_features(&args, "A-006", &row_refs).unwrap();
    let formal = features
        .iter()
        .find(|feature| feature.feature_code == "CASH_QUICK_EVENT")
        .expect("formal cash quick feature");
    let candidate = features
        .iter()
        .find(|feature| feature.feature_code == "CASH_QUICK_CANDIDATE_EVENT")
        .expect("candidate cash quick feature");

    assert_eq!(features.len(), 2);
    assert_eq!(formal.account_key, "A-006");
    assert_eq!(formal.txn_count, 2);
    assert_eq!(formal.total_amount, 8000.0);
    assert_eq!(formal.first_time, "2026-04-11 13:00:00");
    assert_eq!(formal.last_time, "2026-04-11 13:15:00");
    assert_eq!(formal.min_amount, 5000.0);
    assert_eq!(candidate.min_amount, 1000.0);

    let txn_ids = serde_json::from_str::<Vec<String>>(&formal.txn_ids_json).unwrap();
    assert_eq!(
        txn_ids,
        vec![
            "txn-cash-quick-in-1".to_string(),
            "txn-cash-quick-out-1".to_string(),
        ]
    );
    let detail = serde_json::from_str::<serde_json::Value>(&formal.detail_json).unwrap();
    assert_eq!(
        detail,
        serde_json::json!({
            "in_txn_id": "txn-cash-quick-in-1",
            "out_txn_ids": ["txn-cash-quick-out-1"],
            "in_amount": 8000.0,
            "out_amount": 7800.0,
            "out_ratio": 0.975,
            "duration_minutes": 15.0,
            "start_time": "2026-04-11 13:00:00",
            "end_time": "2026-04-11 13:15:00",
        })
    );
    assert_eq!(candidate.detail_json, formal.detail_json);
}
