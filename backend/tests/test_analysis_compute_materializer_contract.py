from __future__ import annotations

from pathlib import Path

import pytest

import app.core.analysis_compute as analysis_compute
from app.core.analysis_compute_runner import AnalysisComputeUnavailableError


def _txn_result(*, row_count: int = 0) -> dict[str, object]:
    return {
        "ok": True,
        "case_id": "case-a",
        "row_count": row_count,
        "rebuilt": True,
        "agg_name": "rule_txn_index:v2",
        "agg_version": 2,
    }


def _pattern_result(
    *,
    total_rows: int = 3,
    cash_covered_rows: int | None = None,
    cash_unknown_rows: int = 0,
    cash_conflict_rows: int = 0,
) -> dict[str, object]:
    if cash_covered_rows is None:
        cash_covered_rows = total_rows - cash_unknown_rows - cash_conflict_rows
    cash_complete = cash_covered_rows == total_rows and cash_unknown_rows == 0 and cash_conflict_rows == 0
    return {
        "ok": True,
        "case_id": "case-a",
        "row_count": 0,
        "rebuilt": True,
        "input_coverage": {
            "total_rows": total_rows,
            "accepted_rows": total_rows,
            "rejected_rows": 0,
            "account_key_covered_rows": total_rows,
            "transaction_id_covered_rows": total_rows,
            "transaction_time_covered_rows": total_rows,
            "amount_covered_rows": total_rows,
            "direction_covered_rows": total_rows,
            "cash_covered_rows": cash_covered_rows,
            "cash_unknown_rows": cash_unknown_rows,
            "cash_conflict_rows": cash_conflict_rows,
        },
        "feature_readiness": {
            "cash_dependent_rules": {
                "status": "complete" if cash_complete else "partial",
                "requested_rows": total_rows,
                "eligible_rows": cash_covered_rows,
                "unknown_cash_rows": cash_unknown_rows,
                "conflict_cash_rows": cash_conflict_rows,
                "blocker": None if cash_complete else "cash_classification_coverage_incomplete",
            },
            "cash_independent_rules": {
                "status": "complete",
                "requested_rows": total_rows,
                "eligible_rows": total_rows,
                "blocker": None,
            },
        },
        "agg_name": "rule_pattern_index:v9",
        "agg_version": 9,
    }


def _pattern_kwargs(tmp_path: Path) -> dict[str, object]:
    return {
        "case_id": "case-a",
        "db_path": tmp_path / "case.duckdb",
        "param_signature": "rule-pattern-signature-a",
        "round_unit": 10_000.0,
        "round_min_amount": 10_000.0,
        "round_min_count": 3,
        "round_min_total_amount": 50_000.0,
        "small_fast_window_minutes": 60,
        "small_fast_ratio": 0.75,
        "small_fast_min_amount": 1_000.0,
        "small_fast_max_amount": 50_000.0,
        "cash_quick_window_minutes": 60,
        "cash_quick_min_amount": 5_000.0,
        "cash_candidate_window_minutes": 60,
        "cash_candidate_min_amount": 1_000.0,
        "cash_candidate_min_ratio": 0.9,
        "cash_candidate_max_ratio": 1.1,
        "near_threshold_amount": 50_000.0,
        "near_threshold_lower_rate": 0.9,
        "near_threshold_window_minutes": 14_400,
        "near_threshold_min_count": 2,
        "near_threshold_min_total_amount": 90_000.0,
        "repeated_amount_window_minutes": 1_440,
        "repeated_amount_min_amount": 1_000.0,
        "repeated_amount_min_count": 3,
        "repeated_amount_min_total_amount": 10_000.0,
        "threshold_split_window_minutes": 30,
        "threshold_split_amount": 50_000.0,
        "threshold_split_tolerance_rate": 0.05,
        "threshold_split_min_count": 3,
        "high_freq_small_amount_threshold": 5_000.0,
        "high_freq_window_minutes": 30,
        "high_freq_count_threshold": 8,
        "high_freq_min_total_amount": 20_000.0,
        "night_start_hour": 22,
        "night_end_hour": 6,
        "night_min_count": 3,
        "night_min_total_amount": 20_000.0,
        "force": False,
    }


def test_rule_txn_materializer_accepts_exact_same_case_zero_result(tmp_path, monkeypatch) -> None:
    expected = _txn_result()
    monkeypatch.setattr(analysis_compute, "run_analysis_compute", lambda _args: expected)

    assert analysis_compute.materialize_rule_txn_index(
        case_id="case-a", db_path=tmp_path / "case.duckdb"
    ) is expected


def test_rule_txn_materializer_rejects_malformed_or_spoofed_envelopes(tmp_path, monkeypatch) -> None:
    invalid_payloads: list[object] = [None, [], "ok"]
    for field, value in (
        ("ok", False),
        ("case_id", "case-b"),
        ("row_count", True),
        ("row_count", -1),
        ("rebuilt", 1),
        ("agg_name", "rule_txn_index:v1"),
        ("agg_version", 1),
    ):
        payload = _txn_result()
        payload[field] = value
        invalid_payloads.append(payload)
    missing = _txn_result()
    missing.pop("row_count")
    invalid_payloads.append(missing)
    extra = _txn_result()
    extra["unexpected"] = True
    invalid_payloads.append(extra)

    for payload in invalid_payloads:
        monkeypatch.setattr(analysis_compute, "run_analysis_compute", lambda _args, payload=payload: payload)
        with pytest.raises(AnalysisComputeUnavailableError, match="invalid envelope"):
            analysis_compute.materialize_rule_txn_index(
                case_id="case-a", db_path=tmp_path / "case.duckdb"
            )
        assert analysis_compute.try_materialize_rule_txn_index(
            case_id="case-a", db_path=tmp_path / "case.duckdb"
        ) is None


@pytest.mark.parametrize(
    "payload",
    (
        _pattern_result(total_rows=0),
        _pattern_result(total_rows=3),
        _pattern_result(total_rows=3, cash_covered_rows=2, cash_unknown_rows=1),
        _pattern_result(total_rows=3, cash_covered_rows=2, cash_conflict_rows=1),
    ),
)
def test_rule_pattern_materializer_accepts_exact_complete_partial_and_empty_envelopes(
    tmp_path, monkeypatch, payload
) -> None:
    monkeypatch.setattr(analysis_compute, "run_analysis_compute", lambda _args: payload)

    assert analysis_compute.materialize_rule_pattern_index(
        **_pattern_kwargs(tmp_path)
    ) is payload


def test_rule_pattern_materializer_rejects_malformed_or_inconsistent_envelopes(
    tmp_path, monkeypatch
) -> None:
    invalid_payloads: list[object] = [None, [], "ok"]
    mutations = (
        ("result", "case_id", "case-b"),
        ("result", "row_count", True),
        ("result", "rebuilt", 1),
        ("result", "agg_name", "rule_pattern_index:v8"),
        ("result", "agg_version", 8),
        ("coverage", "total_rows", True),
        ("coverage", "accepted_rows", 2),
        ("coverage", "rejected_rows", 1),
        ("coverage", "amount_covered_rows", 2),
        ("coverage", "cash_covered_rows", 2),
        ("cash_dependent", "status", "partial"),
        ("cash_dependent", "eligible_rows", 2),
        ("cash_dependent", "unknown_cash_rows", 1),
        ("cash_dependent", "blocker", "cash_classification_coverage_incomplete"),
        ("cash_independent", "eligible_rows", 2),
        ("cash_independent", "blocker", "unexpected"),
    )
    for location, field, value in mutations:
        payload = _pattern_result()
        coverage = payload["input_coverage"]
        readiness = payload["feature_readiness"]
        assert isinstance(coverage, dict)
        assert isinstance(readiness, dict)
        target = {
            "result": payload,
            "coverage": coverage,
            "cash_dependent": readiness["cash_dependent_rules"],
            "cash_independent": readiness["cash_independent_rules"],
        }[location]
        assert isinstance(target, dict)
        target[field] = value
        invalid_payloads.append(payload)
    for location in ("result", "coverage", "readiness", "cash_dependent", "cash_independent"):
        payload = _pattern_result()
        coverage = payload["input_coverage"]
        readiness = payload["feature_readiness"]
        assert isinstance(coverage, dict)
        assert isinstance(readiness, dict)
        target = {
            "result": payload,
            "coverage": coverage,
            "readiness": readiness,
            "cash_dependent": readiness["cash_dependent_rules"],
            "cash_independent": readiness["cash_independent_rules"],
        }[location]
        assert isinstance(target, dict)
        target["unexpected"] = True
        invalid_payloads.append(payload)
    missing = _pattern_result()
    missing.pop("feature_readiness")
    invalid_payloads.append(missing)

    for payload in invalid_payloads:
        monkeypatch.setattr(analysis_compute, "run_analysis_compute", lambda _args, payload=payload: payload)
        with pytest.raises(AnalysisComputeUnavailableError):
            analysis_compute.materialize_rule_pattern_index(**_pattern_kwargs(tmp_path))
        assert analysis_compute.try_materialize_rule_pattern_index(
            **_pattern_kwargs(tmp_path)
        ) is None
