use super::super::model::CashClass;
use super::super::normalization::{classify_rule_cash_txn, normalize_rule_direction};

#[test]
fn normalizes_rule_direction_from_common_tokens() {
    assert_eq!(normalize_rule_direction(" CREDIT "), "in");
    assert_eq!(normalize_rule_direction("客户入账"), "in");
    assert_eq!(normalize_rule_direction("debit"), "out");
    assert_eq!(normalize_rule_direction("转出支取"), "out");
    assert_eq!(normalize_rule_direction(""), "unknown");
    assert_eq!(normalize_rule_direction("balance"), "unknown");
}

#[test]
fn detects_cash_markers_without_misclassifying_non_cash_raw_value() {
    assert_eq!(classify_rule_cash_txn("1", &[]), CashClass::Cash);
    assert_eq!(
        classify_rule_cash_txn("", &["柜面现金存入"]),
        CashClass::Unknown
    );
    assert_eq!(
        classify_rule_cash_txn("0", &["柜面现金存入"]),
        CashClass::Conflict
    );
    assert_eq!(
        classify_rule_cash_txn("非现金", &["普通转账"]),
        CashClass::NonCash
    );
    assert_eq!(
        classify_rule_cash_txn("非现金", &["现金转账"]),
        CashClass::Conflict
    );
    assert_eq!(
        classify_rule_cash_txn("nocash", &["cash transfer"]),
        CashClass::Conflict
    );
    assert_eq!(
        classify_rule_cash_txn("0", &["明确非现金转账"]),
        CashClass::NonCash
    );
}
