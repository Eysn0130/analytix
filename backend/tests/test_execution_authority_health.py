from __future__ import annotations

import json
from types import SimpleNamespace

import pytest

from app.api.v1 import system as system_api
from app.core import (
    analysis_compute_runner,
    archive_extraction,
    data_engine_client,
    execution_authority_health as authority_health,
)
from app.main import _build_lifecycle_health
from app.utils.time import utc_now


SENSITIVE_VALUE = "6222020202020202020-/private/cases/case-a"
BLOCKED_CODE = "managed_process_execution_authority_unavailable"
UNADMITTED_CODE = "managed_process_capability_admission_unavailable"


class _PoisonRuntimeHealth:
    def get_runtime_health(self) -> dict:
        raise AssertionError("runtime discovery ran before execution authority admission")


class _PoisonQueryEngine:
    def get_codex_runtime_health(self) -> dict:
        raise AssertionError("query runtime discovery ran without capability admission")


def _request_state() -> SimpleNamespace:
    ready = object()
    return SimpleNamespace(
        settings=ready,
        started_at=utc_now(),
        task_service=ready,
        case_service=ready,
        analysis_service=ready,
        analysis_worker_service=ready,
        workspace_projection_service=ready,
        privacy_projection_service=_PoisonRuntimeHealth(),
        import_service=ready,
        document_service=ready,
        cleaning_service=_PoisonRuntimeHealth(),
        export_service=ready,
        stats_service=ready,
        stats_job_service=ready,
        flow_service=ready,
        ws_manager=ready,
        ws_sequence=ready,
        query_engine=_PoisonQueryEngine(),
    )


def _request() -> SimpleNamespace:
    return SimpleNamespace(app=SimpleNamespace(state=_request_state()))


def _settings() -> SimpleNamespace:
    return SimpleNamespace(
        app_name="analytix-data-analysis",
        app_version="test",
        app_env="test",
    )


def _unavailable_status() -> SimpleNamespace:
    return SimpleNamespace(available=False, reason_code=SENSITIVE_VALUE)


def test_execution_authority_health_uses_only_fixed_non_sensitive_blocker(
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    monkeypatch.setattr(
        authority_health,
        "managed_execution_authority_status",
        _unavailable_status,
    )

    health = authority_health.execution_authority_health()

    assert health.available is False
    assert health.reason_code == BLOCKED_CODE
    assert SENSITIVE_VALUE not in repr(health)
    unadmitted = authority_health.unadmitted_execution_capability_health(
        authority_health.ExecutionAuthorityHealth(
            available=False,
            reason_code=SENSITIVE_VALUE,
        )
    )
    assert unadmitted.reason_code == BLOCKED_CODE
    assert SENSITIVE_VALUE not in repr(unadmitted)


def test_lifecycle_health_blocks_every_external_probe_before_manifest_or_path_discovery(
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    monkeypatch.setattr(
        authority_health,
        "managed_execution_authority_status",
        _unavailable_status,
    )

    def must_not_probe() -> dict:
        raise AssertionError("external runtime probe ran without execution authority")

    monkeypatch.setattr(archive_extraction, "archive_extractor_health", must_not_probe)
    monkeypatch.setattr(analysis_compute_runner, "analysis_compute_binary_health", must_not_probe)
    monkeypatch.setattr(data_engine_client, "data_engine_binary_health", must_not_probe)

    payload = _build_lifecycle_health(_request(), _settings())

    for key in (
        "managed_execution_available",
        "archive_extraction_available",
        "analysis_compute_available",
        "data_engine_available",
        "document_conversion_available",
        "cleaning_native_available",
        "privacy_projection_native_available",
    ):
        assert payload[key] is False
    for key in (
        "managed_execution_reason",
        "archive_extraction_reason",
        "analysis_compute_reason",
        "data_engine_reason",
        "document_conversion_reason",
        "cleaning_native_reason",
        "privacy_projection_native_reason",
    ):
        assert payload[key] == BLOCKED_CODE
    assert payload["archive_extraction_supported_exts"] == []
    assert payload["analysis_compute_required_commands"] == []
    assert payload["data_engine_pid"] is None
    assert all(
        payload[key] == ""
        for key in (
            "archive_extraction_bin",
            "archive_extraction_bin_source",
            "analysis_compute_bin",
            "data_engine_bin",
            "cleaning_native_bin",
            "cleaning_native_bin_source",
            "privacy_projection_native_bin",
            "privacy_projection_native_bin_source",
        )
    )
    assert SENSITIVE_VALUE not in json.dumps(payload, ensure_ascii=False)


def test_system_health_cannot_upgrade_archive_manifest_without_execution_authority(
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    monkeypatch.setattr(
        authority_health,
        "managed_execution_authority_status",
        _unavailable_status,
    )

    def must_not_probe() -> dict:
        raise AssertionError("archive manifest was inspected without execution authority")

    monkeypatch.setattr(archive_extraction, "archive_extractor_health", must_not_probe)

    payload = system_api._build_health_payload(_request(), _settings()).model_dump()

    assert payload["managed_execution_available"] is False
    assert payload["managed_execution_reason"] == BLOCKED_CODE
    assert payload["codex_runtime_ready"] is False
    assert payload["codex_runtime_reason"] == BLOCKED_CODE
    assert payload["document_conversion_available"] is False
    assert payload["document_conversion_reason"] == BLOCKED_CODE
    assert payload["archive_extraction_available"] is False
    assert payload["archive_extraction_reason"] == BLOCKED_CODE
    assert payload["archive_extraction_bin"] == ""
    assert payload["archive_extraction_bin_source"] == ""
    assert payload["archive_extraction_supported_exts"] == []
    assert SENSITIVE_VALUE not in json.dumps(payload, ensure_ascii=False)


def test_global_authority_does_not_admit_unmigrated_helper_health(
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    monkeypatch.setattr(
        authority_health,
        "managed_execution_authority_status",
        lambda: SimpleNamespace(available=True, reason_code=""),
    )

    def must_not_probe() -> dict:
        raise AssertionError("global authority is not a per-capability admission")

    monkeypatch.setattr(archive_extraction, "archive_extractor_health", must_not_probe)
    monkeypatch.setattr(analysis_compute_runner, "analysis_compute_binary_health", must_not_probe)
    monkeypatch.setattr(data_engine_client, "data_engine_binary_health", must_not_probe)

    payload = _build_lifecycle_health(_request(), _settings())
    system_payload = system_api._build_health_payload(_request(), _settings()).model_dump()

    assert payload["managed_execution_available"] is True
    for key in (
        "archive_extraction_available",
        "analysis_compute_available",
        "data_engine_available",
        "cleaning_native_available",
        "privacy_projection_native_available",
        "document_conversion_available",
    ):
        assert payload[key] is False
    for key in (
        "archive_extraction_reason",
        "analysis_compute_reason",
        "data_engine_reason",
        "cleaning_native_reason",
        "privacy_projection_native_reason",
        "document_conversion_reason",
    ):
        assert payload[key] == UNADMITTED_CODE
    assert payload["analysis_compute_required_commands"] == []
    assert payload["data_engine_pid"] is None
    assert system_payload["managed_execution_available"] is True
    assert system_payload["codex_runtime_ready"] is False
    assert system_payload["codex_runtime_reason"] == UNADMITTED_CODE
    assert system_payload["archive_extraction_available"] is False
    assert system_payload["archive_extraction_reason"] == UNADMITTED_CODE
    assert system_payload["document_conversion_available"] is False
    assert system_payload["document_conversion_reason"] == UNADMITTED_CODE
    assert SENSITIVE_VALUE not in json.dumps(payload, ensure_ascii=False)
    assert SENSITIVE_VALUE not in json.dumps(system_payload, ensure_ascii=False)
