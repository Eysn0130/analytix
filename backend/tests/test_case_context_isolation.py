from __future__ import annotations

from types import SimpleNamespace

import pytest
from fastapi import FastAPI
from fastapi.testclient import TestClient

from app.api.data_analysis_deps import get_case_service
from app.api.v1.cases import router as cases_router
from app.domain.case_service import (
    CaseEffectAuthorityUnavailableError,
    CaseEffectBindingRejectedError,
    CaseEffectBindingV1,
    CaseService,
    validate_unwired_case_effect_binding_v1,
)


class _CaseRepository:
    def __init__(self) -> None:
        self.cases = {
            "case_a": self._case("case_a", "Case A"),
            "case_b": self._case("case_b", "Case B"),
        }
        self.opened: list[str] = []
        self.requested: list[str] = []

    @staticmethod
    def _case(case_id: str, name: str):
        return SimpleNamespace(
            case_id=case_id,
            name=name,
            case_no=case_id,
            owner="",
            summary="",
            case_type="",
            tags=[],
            status="进行中",
            deleted_at="",
            created_at="2026-07-15 00:00:00",
            updated_at="2026-07-15 00:00:00",
        )

    def get_case(self, case_id: str):
        self.requested.append(case_id)
        return self.cases.get(case_id)

    def get_last_opened_case(self):
        raise AssertionError("last-opened case must never be factual authority")

    def record_case_opened(self, case_id: str) -> None:
        self.opened.append(case_id)

    @staticmethod
    def case_size(_case_id: str) -> int:
        return 1

    @staticmethod
    def get_case_import_overview(_case_id: str, *, limit: int = 5) -> dict:
        del limit
        return {"source_status": "available", "recent": []}


def _service() -> tuple[CaseService, _CaseRepository]:
    repository = _CaseRepository()
    return CaseService(repository), repository  # type: ignore[arg-type]


def _api_client(service: CaseService) -> TestClient:
    app = FastAPI()
    app.include_router(cases_router, prefix="/api/v1")
    app.dependency_overrides[get_case_service] = lambda: service
    return TestClient(app)


def _binding(
    *,
    case_id: str = "case_a",
    context_seed: str = "c",
    context_epoch: int = 7,
    snapshot_seed: str = "a",
    grant_seed: str = "b",
) -> CaseEffectBindingV1:
    return CaseEffectBindingV1(
        case_id=case_id,
        context_digest=context_seed * 64,
        context_epoch=context_epoch,
        dataset_snapshot_id=f"dsv1_{snapshot_seed * 64}",
        execution_grant_id=grant_seed * 64,
    )


def test_two_windows_cannot_change_each_others_explicit_case_lookup() -> None:
    service, repository = _service()

    service.activate_case("case_a")
    service.activate_case("case_b")

    assert service.get_explicit_case_selection("case_a")["case_id"] == "case_a"
    assert service.get_explicit_case_selection("case_b")["case_id"] == "case_b"
    assert repository.opened == ["case_a", "case_b"]
    assert not hasattr(service, "_active_case_id")


def test_active_compatibility_route_requires_an_explicit_case_id() -> None:
    service, repository = _service()
    client = _api_client(service)

    missing = client.get("/api/v1/cases/active")
    explicit_a = client.get("/api/v1/cases/active", params={"case_id": "case_a"})
    service.activate_case("case_b")
    explicit_a_after_other_window = client.get("/api/v1/cases/active", params={"case_id": "case_a"})

    assert missing.status_code == 422
    assert explicit_a.status_code == 200
    assert explicit_a.json()["data"]["case_id"] == "case_a"
    assert explicit_a_after_other_window.status_code == 200
    assert explicit_a_after_other_window.json()["data"]["case_id"] == "case_a"
    assert "" not in repository.requested


def test_unwired_binding_validator_rejects_stale_snapshot_and_grant() -> None:
    trusted = _binding()
    validate_unwired_case_effect_binding_v1(trusted_expected=trusted, observed=trusted)

    with pytest.raises(CaseEffectBindingRejectedError, match="case_effect_binding_mismatch"):
        validate_unwired_case_effect_binding_v1(
            trusted_expected=trusted,
            observed=_binding(snapshot_seed="d"),
        )
    with pytest.raises(CaseEffectBindingRejectedError, match="case_effect_binding_mismatch"):
        validate_unwired_case_effect_binding_v1(
            trusted_expected=trusted,
            observed=_binding(grant_seed="e"),
        )
    with pytest.raises(CaseEffectBindingRejectedError, match="case_effect_binding_mismatch"):
        validate_unwired_case_effect_binding_v1(
            trusted_expected=trusted,
            observed=_binding(context_seed="f"),
        )
    with pytest.raises(CaseEffectBindingRejectedError, match="case_effect_binding_mismatch"):
        validate_unwired_case_effect_binding_v1(
            trusted_expected=trusted,
            observed=_binding(context_epoch=6),
        )


def test_unwired_binding_validator_fails_closed_without_host_authority_or_observation() -> None:
    trusted = _binding()

    with pytest.raises(CaseEffectAuthorityUnavailableError, match="case_effect_authority_unavailable"):
        validate_unwired_case_effect_binding_v1(trusted_expected=None, observed=trusted)
    with pytest.raises(CaseEffectBindingRejectedError, match="case_effect_binding_incomplete"):
        validate_unwired_case_effect_binding_v1(trusted_expected=trusted, observed=None)
    with pytest.raises(CaseEffectBindingRejectedError, match="case_effect_binding_mismatch"):
        validate_unwired_case_effect_binding_v1(
            trusted_expected=trusted,
            observed=_binding(case_id="case_b"),
        )
