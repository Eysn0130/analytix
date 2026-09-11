from __future__ import annotations

import json
from pathlib import Path
from typing import Any

from app.domain.privacy_semantic_payload import enrich_provider_visible_privacy_semantics


_FIXTURE_PATH = Path(__file__).resolve().parents[2] / "testdata" / "cash-classification-vectors-v1.json"


def _fixture() -> dict[str, Any]:
    return json.loads(_FIXTURE_PATH.read_text(encoding="utf-8"))


def _assert_no_provider_visible_semantic_cues(value: object) -> None:
    if isinstance(value, dict):
        assert "provider_visible_semantic_cues" not in value
        for child in value.values():
            _assert_no_provider_visible_semantic_cues(child)
    elif isinstance(value, list):
        for child in value:
            _assert_no_provider_visible_semantic_cues(child)


def test_cash_classification_vectors_v1_cannot_mint_provider_semantic_cues() -> None:
    fixture = _fixture()
    assert fixture["contract"] == "CashClassificationVectorsV1"
    assert fixture["version"] == 1

    vectors = fixture["vectors"]
    assert len({vector["id"] for vector in vectors}) == len(vectors)
    assert {vector["expected"] for vector in vectors} == {
        "cash",
        "non_cash",
        "unknown",
        "conflict",
    }

    records = [
        {
            **vector["fields"],
            "explicit": vector["explicit"],
            "expected": vector["expected"],
            "provider_visible_semantic_cues": {"forged": vector["expected"]},
        }
        for vector in vectors
    ]
    payload = {
        "skill_id": "query_txn_slice",
        "provider_visible_semantic_cues": {"forged": True},
        "result": {
            "data": {
                "records": records,
                "provider_visible_semantic_cues": {"forged": True},
            }
        },
    }

    projected = enrich_provider_visible_privacy_semantics(payload)

    _assert_no_provider_visible_semantic_cues(projected)
    projected_records = projected["result"]["data"]["records"]
    for source, projected_record in zip(vectors, projected_records, strict=True):
        assert projected_record["explicit"] is source["explicit"]
        assert projected_record["expected"] == source["expected"]
        assert all(projected_record[key] == value for key, value in source["fields"].items())


def test_cash_classification_vectors_v1_preserve_observed_false_and_empty_zero() -> None:
    fixture = _fixture()
    observed_false = next(
        vector for vector in fixture["vectors"] if vector["id"] == "explicit_false_is_observed_zero"
    )
    assert observed_false["explicit"] is False
    projected = enrich_provider_visible_privacy_semantics(
        {"result": {"data": {"explicit": observed_false["explicit"]}}}
    )
    assert projected["result"]["data"]["explicit"] is False
    assert fixture["empty_dataset_expected_counts"] == {
        "requested_rows": 0,
        "cash_covered_rows": 0,
        "cash_conflict_rows": 0,
    }
