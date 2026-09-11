from __future__ import annotations

from datetime import datetime, timedelta

from app.repositories.analysis_repository import (
    AnalysisRepository,
    _cash_rule_input_coverage,
    _cash_txn_state,
    _normalize_bool,
)


def _repository() -> AnalysisRepository:
    return object.__new__(AnalysisRepository)


def _txn(
    txn_id: str,
    minute: int,
    direction: str,
    amount: float,
    cash_flag: bool | None,
    *,
    summary: str = "转账",
) -> dict[str, object]:
    txn_time = datetime(2026, 7, 20, 9, 0) + timedelta(minutes=minute)
    return {
        "account_key": "acct-a",
        "account_entity_id": "account:acct-a",
        "txn_id": txn_id,
        "source_record_id": f"row:{txn_id}",
        "txn_time": txn_time.strftime("%Y-%m-%d %H:%M:%S"),
        "txn_ts": txn_time,
        "amount_val": amount,
        "direction_norm": direction,
        "cash_flag_norm": cash_flag,
        "summary": summary,
        "txn_type": "",
        "remark": "",
        "voucher_type": "",
        "counterparty_acct": "cp-a",
        "counterparty_name": "乙方",
    }


def test_cash_raw_classification_is_exact_and_missing_remains_unknown() -> None:
    assert _normalize_bool(None) is None
    assert _normalize_bool("") is None
    assert _normalize_bool("非现金") is False
    assert _normalize_bool("nocash") is False
    assert _normalize_bool("现金") is True
    assert _normalize_bool("现金转账") is None
    assert _normalize_bool("非现金转账") is None


def test_cash_text_is_only_a_cue_and_conflicts_fail_closed() -> None:
    assert _cash_txn_state(_txn("unknown", 0, "in", 1, None, summary="柜面现金存入")) == "unknown"
    assert _cash_txn_state(_txn("noncash", 0, "in", 1, False, summary="普通转账")) == "non_cash"
    assert _cash_txn_state(_txn("cash", 0, "in", 1, True, summary="柜面现金存入")) == "cash"
    assert _cash_txn_state(_txn("false-conflict", 0, "in", 1, False, summary="柜面现金存入")) == "conflict"
    assert _cash_txn_state(_txn("true-conflict", 0, "in", 1, True, summary="明确非现金转账")) == "conflict"
    assert _cash_txn_state(_txn("negative-cue", 0, "in", 1, False, summary="不含现金的转账")) == "non_cash"


def test_cash_coverage_distinguishes_unknown_conflict_and_real_non_cash() -> None:
    coverage = _cash_rule_input_coverage(
        [
            _txn("cash", 0, "in", 1, True),
            _txn("noncash", 1, "out", 1, False),
            _txn("unknown", 2, "in", 1, None),
            _txn("conflict", 3, "out", 1, False, summary="现金支取"),
        ]
    )

    assert coverage == {
        "status": "partial",
        "total_rows": 4,
        "classified_rows": 2,
        "cash_rows": 1,
        "non_cash_rows": 1,
        "unknown_rows": 1,
        "conflict_rows": 1,
    }


def test_unknown_cash_blocks_cash_dependent_rules_without_blocking_independent_rules() -> None:
    rows = [
        _txn("in-1", 0, "in", 4_000, False),
        _txn("out-1", 5, "out", 3_200, False),
        _txn("in-2", 10, "in", 4_000, False),
        _txn("out-2", 15, "out", 3_200, False),
    ]
    params = {
        "small_fast_in_out_min_event_count": 2,
        "small_fast_in_out_min_total_amount": 0,
        "small_fast_in_out_min_amount": 1_000,
        "small_fast_in_out_max_amount": 50_000,
        "small_fast_in_out_ratio": 0.75,
        "small_fast_in_out_window_minutes": 60,
    }

    complete_hits = _repository()._compute_rule_hits("case-a", rows, params=params)
    assert any(hit["rule_code"] == "SMALL_FAST_IN_OUT" for hit in complete_hits)
    assert all(
        hit["detail_json"]["feature_input_readiness"]["cash_classification"]["status"]
        in {"complete", "not_required"}
        for hit in complete_hits
    )

    rows[0] = {**rows[0], "cash_flag_norm": None}
    partial_hits = _repository()._compute_rule_hits("case-a", rows, params=params)
    assert all(hit["rule_code"] not in {
        "SMALL_FAST_IN_OUT",
        "SINGLE_FAST_IN_OUT_CANDIDATE",
        "CASH_QUICK_IN_OUT",
        "SINGLE_CASH_QUICK_IN_OUT_CANDIDATE",
    } for hit in partial_hits)
    assert all(
        hit["detail_json"]["feature_input_readiness"]["cash_classification"] == {"status": "not_required"}
        for hit in partial_hits
    )
