from __future__ import annotations

import pytest

from app.repositories import cleaning_native_results


CASE_ID = "case-alpha"


def _payload(keys: tuple[str, ...]) -> dict[str, object]:
    return {"ok": True, "case_id": CASE_ID, **{key: 0 for key in keys}}


@pytest.mark.parametrize(
    "mutation",
    [
        lambda payload: payload.pop("amount_updated"),
        lambda payload: payload.__setitem__("amount_updated", None),
        lambda payload: payload.__setitem__("amount_updated", False),
        lambda payload: payload.__setitem__("amount_updated", -1),
        lambda payload: payload.__setitem__("amount_updated", 1.5),
        lambda payload: payload.__setitem__("ok", False),
        lambda payload: payload.__setitem__("case_id", "case-bravo"),
        lambda payload: payload.__setitem__("unknown_field", 0),
    ],
)
def test_amount_balance_unknown_or_mismatched_values_never_become_zero(mutation) -> None:
    payload = _payload(cleaning_native_results.AMOUNT_BALANCE_KEYS)
    mutation(payload)

    with pytest.raises(
        cleaning_native_results.CleaningNativeResultError,
        match="cleaning_native_result_invalid",
    ):
        cleaning_native_results.amount_balance_result(payload, expected_case_id=CASE_ID)


def test_observed_zero_is_preserved_only_when_every_required_field_is_present() -> None:
    payload = _payload(cleaning_native_results.AMOUNT_BALANCE_KEYS)

    assert cleaning_native_results.amount_balance_result(
        payload,
        expected_case_id=CASE_ID,
    ) == {key: 0 for key in cleaning_native_results.AMOUNT_BALANCE_KEYS}


def test_clean_all_requires_factual_counters_and_native_completion() -> None:
    payload = _payload(cleaning_native_results.CLEAN_ALL_FACT_KEYS)
    payload["native_completion_applied"] = 1

    result = cleaning_native_results.clean_all_result(payload, expected_case_id=CASE_ID)
    assert result["native_completion_applied"] == 1
    assert not any(key in result for key in cleaning_native_results.CLEAN_ALL_TELEMETRY_KEYS)

    missing_fact = dict(payload)
    missing_fact.pop("invalid_total")
    with pytest.raises(cleaning_native_results.CleaningNativeResultError):
        cleaning_native_results.clean_all_result(missing_fact, expected_case_id=CASE_ID)

    incomplete = dict(payload)
    incomplete["native_completion_applied"] = 0
    with pytest.raises(
        cleaning_native_results.CleaningNativeResultError,
        match="cleaning_native_result_incomplete",
    ):
        cleaning_native_results.clean_all_result(incomplete, expected_case_id=CASE_ID)


def test_active_is_never_accepted_as_a_case_binding() -> None:
    payload = _payload(cleaning_native_results.AMOUNT_BALANCE_KEYS)
    payload["case_id"] = "active"

    with pytest.raises(cleaning_native_results.CleaningNativeResultError):
        cleaning_native_results.amount_balance_result(payload, expected_case_id="active")
