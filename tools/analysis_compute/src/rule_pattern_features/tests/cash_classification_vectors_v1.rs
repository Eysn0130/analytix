use std::collections::{BTreeSet, HashSet};
use std::fs;
use std::path::PathBuf;

use serde::Deserialize;
use serde_json::Value;

use super::super::model::CashClass;
use super::super::normalization::classify_rule_cash_txn;

#[derive(Deserialize)]
#[serde(deny_unknown_fields)]
struct Fixture {
    contract: String,
    version: u64,
    vectors: Vec<Vector>,
    empty_dataset_expected_counts: EmptyDatasetExpectedCounts,
}

#[derive(Deserialize)]
#[serde(deny_unknown_fields)]
struct Vector {
    id: String,
    explicit: Value,
    wire_token: Option<String>,
    fields: Fields,
    expected: String,
}

#[derive(Default, Deserialize)]
#[serde(default, deny_unknown_fields)]
struct Fields {
    summary: String,
    txn_type: String,
    remark: String,
    voucher_type: String,
}

#[derive(Deserialize)]
#[serde(deny_unknown_fields)]
struct EmptyDatasetExpectedCounts {
    requested_rows: u64,
    cash_covered_rows: u64,
    cash_conflict_rows: u64,
}

fn fixture() -> Fixture {
    let path = PathBuf::from(env!("CARGO_MANIFEST_DIR"))
        .join("../../testdata/cash-classification-vectors-v1.json");
    serde_json::from_str(&fs::read_to_string(path).expect("read CashClassificationVectorsV1"))
        .expect("parse CashClassificationVectorsV1")
}

fn class_name(value: CashClass) -> &'static str {
    match value {
        CashClass::Cash => "cash",
        CashClass::NonCash => "non_cash",
        CashClass::Unknown => "unknown",
        CashClass::Conflict => "conflict",
    }
}

#[test]
fn shared_cash_classification_vectors_v1_match_rust_classifier() {
    let fixture = fixture();
    assert_eq!(fixture.contract, "CashClassificationVectorsV1");
    assert_eq!(fixture.version, 1);
    assert_eq!(
        (
            fixture.empty_dataset_expected_counts.requested_rows,
            fixture.empty_dataset_expected_counts.cash_covered_rows,
            fixture.empty_dataset_expected_counts.cash_conflict_rows,
        ),
        (0, 0, 0)
    );

    let mut ids = HashSet::new();
    let mut expected_states = BTreeSet::new();
    for vector in fixture.vectors {
        assert!(
            ids.insert(vector.id.clone()),
            "duplicate vector id: {}",
            vector.id
        );
        assert!(
            vector.explicit.is_null()
                || vector.explicit.is_boolean()
                || vector.explicit.is_string(),
            "invalid canonical explicit type: {}",
            vector.id
        );
        expected_states.insert(vector.expected.clone());
        let raw = vector.wire_token.as_deref().unwrap_or("");
        let fields = [
            vector.fields.summary.as_str(),
            vector.fields.txn_type.as_str(),
            vector.fields.remark.as_str(),
            vector.fields.voucher_type.as_str(),
        ];
        assert_eq!(
            class_name(classify_rule_cash_txn(raw, &fields)),
            vector.expected,
            "{}",
            vector.id
        );
    }
    assert_eq!(
        expected_states,
        BTreeSet::from([
            "cash".to_string(),
            "conflict".to_string(),
            "non_cash".to_string(),
            "unknown".to_string(),
        ])
    );
}
