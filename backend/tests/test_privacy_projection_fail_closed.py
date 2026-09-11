from __future__ import annotations

import json
import logging

import pytest

from app.api.v1.stats_direct_payloads import (
    build_stats_rows_direct_payload,
    build_stats_txn_rows_direct_payload,
)
from app.domain.analysis_service import AnalysisService
from app.repositories.privacy_projection_repository import (
    PrivacyProjectionLeakError,
    PrivacyProjectionRepository,
)
from app.repositories.analysis_repository import AnalysisRepository


_CASE_ID = "case-privacy-fail-closed"
_ACCOUNT = "6217000011112222333"
_ID_NO = "320101199001011234"
_PHONE = "13800138000"
_EMAIL = "case.owner@example.invalid"
_MAC = "AA:BB:CC:DD:EE:FF"


@pytest.mark.parametrize("status_mode", ["disabled", "stale", "unavailable"])
def test_ordinary_projection_never_returns_raw_payload_when_full_projection_is_not_ready(
    monkeypatch: pytest.MonkeyPatch,
    status_mode: str,
) -> None:
    repository = PrivacyProjectionRepository()
    if status_mode == "unavailable":

        def unavailable(_case_id: str) -> bool:
            raise RuntimeError("offline")

        monkeypatch.setattr(repository, "is_enabled", unavailable)
    else:
        monkeypatch.setattr(repository, "is_enabled", lambda _case_id: False)
    source = {
        "account_number": _ACCOUNT,
        "numeric_account": int(_ACCOUNT),
        "id_no": _ID_NO,
        "phone": _PHONE,
        "email": _EMAIL,
        "counterparty_name": "张三",
        "summary": f"账号：{_ACCOUNT} 身份证号：{_ID_NO} 电话：{_PHONE} 邮箱：{_EMAIL} 设备：{_MAC}",
        "amount": 120000000000,
    }

    projected = repository.project_model_payload(_CASE_ID, source)
    rendered = json.dumps(projected, ensure_ascii=False)

    for secret in (_ACCOUNT, _ID_NO, _PHONE, _EMAIL, _MAC, "张三"):
        assert secret not in rendered
    assert projected == {
        "privacy_projection": "privacy_projection_unavailable",
        "fact_answer_allowed": False,
        "raw_details_exposed": False,
    }
    assert source["account_number"] == _ACCOUNT
    assert source["id_no"] == _ID_NO
    assert source["phone"] == _PHONE


def test_ordinary_text_projection_uses_deterministic_boundary_fallback(
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    repository = PrivacyProjectionRepository()
    monkeypatch.setattr(repository, "is_enabled", lambda _case_id: False)
    grouped_account = "6217 0000 1111 2222 333"
    text = (
        f"银行卡号：{_ACCOUNT}，备用卡 {grouped_account}，身份证号：{_ID_NO}，"
        f"电话：{_PHONE}，邮箱：{_EMAIL}，MAC：{_MAC}"
    )

    first = repository.project_model_text(_CASE_ID, text)
    second = repository.project_model_text(_CASE_ID, text)

    assert first == second
    assert first == "privacy_projection_unavailable"
    for secret in (_ACCOUNT, grouped_account, _ID_NO, _PHONE, _EMAIL, _MAC):
        assert secret not in first
    assert grouped_account not in repository._redact_text(f"备用卡 {grouped_account}")


def test_ordinary_artifact_projection_is_available_before_case_token_state(
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    repository = PrivacyProjectionRepository()
    monkeypatch.setattr(
        repository,
        "_ordinary_projection_context",
        lambda _case_id: (_ for _ in ()).throw(AssertionError("artifact projection opened case state")),
    )

    projected = repository.project_ordinary_artifact_text(
        _CASE_ID,
        f"银行卡号：{_ACCOUNT}，身份证号：{_ID_NO}，电话：{_PHONE}，MAC：{_MAC}",
    )

    for secret in (_ACCOUNT, _ID_NO, _PHONE, _MAC):
        assert secret not in projected
    repository.assert_provider_visible_payload_safe(_CASE_ID, projected)


def test_ordinary_artifact_projection_rejects_residual_sensitive_identifier(
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    repository = PrivacyProjectionRepository()
    monkeypatch.setattr(repository, "_redact_text", lambda _text: _ACCOUNT)

    with pytest.raises(PrivacyProjectionLeakError, match="^privacy_projection_unavailable$"):
        repository.project_ordinary_artifact_text(_CASE_ID, f"账号：{_ACCOUNT}")


def test_analysis_service_without_projection_dependency_still_redacts_provider_payload() -> None:
    service = AnalysisService.__new__(AnalysisService)
    service._privacy_projection_repository = None
    service._logger = logging.getLogger("test.privacy.fail-closed")

    projected = service.project_privacy_model_payload(
        _CASE_ID,
        {"account_number": _ACCOUNT, "summary": f"电话：{_PHONE}"},
    )
    text = service.project_privacy_model_text(_CASE_ID, f"账号：{_ACCOUNT} 电话：{_PHONE}")

    rendered = json.dumps(projected, ensure_ascii=False)
    assert projected["privacy_projection"] == "privacy_projection_unavailable"
    assert text == "privacy_projection_unavailable"
    assert _ACCOUNT not in rendered
    assert _PHONE not in rendered
    assert _ACCOUNT not in text
    assert _PHONE not in text


def test_provider_safety_assertion_remains_active_when_projection_is_disabled(
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    repository = PrivacyProjectionRepository()
    monkeypatch.setattr(repository, "is_enabled", lambda _case_id: False)

    with pytest.raises(PrivacyProjectionLeakError):
        repository.assert_provider_visible_payload_safe(
            _CASE_ID,
            {"account_number": _ACCOUNT, "device": _MAC},
        )

    projected = repository.project_model_payload(
        _CASE_ID,
        {"account_number": _ACCOUNT, "device": _MAC},
    )
    assert projected["privacy_projection"] == "privacy_projection_unavailable"
    repository.assert_provider_visible_payload_safe(_CASE_ID, projected)


def test_projection_rejects_corrupt_raw_alias_and_preserves_bank_account_as_token(
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    repository = PrivacyProjectionRepository()
    salt = "a" * 64
    token = repository._token("account", _ACCOUNT, salt=salt)
    monkeypatch.setattr(
        repository,
        "_ordinary_projection_context",
        lambda _case_id: (salt, {token: _ACCOUNT}, []),
    )

    projected = repository.project_model_payload(
        _CASE_ID,
        {"bank_account": _ACCOUNT},
    )

    assert projected == {"bank_account": token}
    assert _ACCOUNT not in json.dumps(projected, ensure_ascii=False)


def test_sensitive_key_error_is_fixed_and_never_echoes_key_or_value() -> None:
    repository = PrivacyProjectionRepository()
    payload = {_ACCOUNT: "张三"}

    with pytest.raises(PrivacyProjectionLeakError) as captured:
        repository.assert_provider_visible_payload_safe(_CASE_ID, payload)

    assert str(captured.value) == "privacy_projection_unavailable"
    assert _ACCOUNT not in str(captured.value)
    assert "张三" not in str(captured.value)


def test_unicode_separators_and_person_role_text_are_redacted() -> None:
    repository = PrivacyProjectionRepository()
    grouped_account = "6217－0000－1111－2222－333"

    projected = repository._redact_text(
        f"账号：{grouped_account}；经办人员张三与联系人：李四会面"
    )

    assert grouped_account not in projected
    assert _ACCOUNT not in projected
    assert "张三" not in projected
    assert "李四" not in projected
    assert "[PERSON]" in projected


def test_typed_amount_is_not_classified_as_bank_account_but_numeric_account_is() -> None:
    repository = PrivacyProjectionRepository()

    repository.assert_provider_visible_payload_safe(
        _CASE_ID,
        {"amount": 120000000000, "transaction_count": 120000000000},
    )
    with pytest.raises(PrivacyProjectionLeakError):
        repository.assert_provider_visible_payload_safe(
            _CASE_ID,
            {"account_number": 120000000000},
        )


def test_corrupt_fingerprint_state_is_unavailable_not_equal_empty(
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    repository = PrivacyProjectionRepository()

    class _Engine:
        def query(self, sql, _params=()):
            if "FROM privacy_runtime_state" in sql:
                return [
                    (
                        "privacy_projection_v1",
                        "completed",
                        True,
                        100,
                        "a" * 64,
                        "2026-07-16 12:00:00",
                    )
                ]
            raise AssertionError(sql)

        def close(self) -> None:
            return None

    monkeypatch.setattr(repository, "_open_case_engine", lambda _case_id: _Engine())
    monkeypatch.setattr(repository, "_table_exists", lambda _engine, _table: True)
    monkeypatch.setattr(
        repository,
        "_source_fingerprint",
        lambda _engine, _case_id: (_ for _ in ()).throw(RuntimeError("corrupt")),
    )

    status = repository.get_status(_CASE_ID)

    assert status["status"] == "stale"
    assert status["enabled"] is False
    assert status["requires_refresh"] is True
    assert status["source_rows"] == 0
    assert status["projected_rows"] == 0
    assert status["error"] == "privacy_projection_state_unavailable"


def test_short_or_malformed_salt_never_enables_projection(
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    repository = PrivacyProjectionRepository()
    monkeypatch.setattr(repository, "is_enabled", lambda _case_id: True)
    monkeypatch.setattr(repository, "_salt_for_projection", lambda _case_id: "x")

    projected = repository.project_model_payload(
        _CASE_ID,
        {"account_number": _ACCOUNT},
    )

    assert projected["privacy_projection"] == "privacy_projection_unavailable"
    assert _ACCOUNT not in json.dumps(projected, ensure_ascii=False)


def test_ordinary_stats_payloads_never_reuse_raw_rows_or_turn_missing_into_zero() -> None:
    source_rows = [{"account_number": _ACCOUNT, "counterparty_name": "张三"}]
    source = {
        "rows": source_rows,
        "row_fields": ["account_number", "counterparty_name"],
        "status": "completed",
        "total": 1,
        "row_summary": {"account_number": _ACCOUNT},
        "done": True,
        "nextCursor": {"account_number": _ACCOUNT},
    }

    rows_payload = build_stats_rows_direct_payload(source)
    txn_payload = build_stats_txn_rows_direct_payload(source)
    rendered = json.dumps([rows_payload, txn_payload], ensure_ascii=False)

    assert rows_payload["rows"] == []
    assert rows_payload["total"] is None
    assert txn_payload["rows"] == []
    assert txn_payload["done"] is False
    assert txn_payload["next_cursor"] is None
    assert rows_payload["fact_answer_allowed"] is False
    assert txn_payload["fact_answer_allowed"] is False
    assert rows_payload["rows"] is not source_rows
    assert txn_payload["rows"] is not source_rows
    assert _ACCOUNT not in rendered
    assert "张三" not in rendered


def test_report_projection_requires_publication_receipt_before_case_or_filesystem_access() -> None:
    repository = object.__new__(AnalysisRepository)
    repository.case_exists = lambda _case_id: (_ for _ in ()).throw(
        AssertionError("report quarantine inspected case state")
    )
    repository.open_case_engine = lambda _case_id: (_ for _ in ()).throw(
        AssertionError("report quarantine opened case database")
    )

    with pytest.raises(RuntimeError, match="^report_publication_receipt_required$"):
        repository.write_workspace_projection_file(
            _CASE_ID,
            file_name="reports/case-report.md",
            content_md=f"完整账号 {_ACCOUNT}",
            render_mode="report_publish",
        )


@pytest.mark.parametrize("file_name", ("reports/unreceipted.md", "reports\\unreceipted.md"))
def test_report_render_batch_requires_receipt_before_case_or_database_access(file_name: str) -> None:
    repository = object.__new__(AnalysisRepository)
    repository.case_exists = lambda _case_id: (_ for _ in ()).throw(
        AssertionError("report quarantine inspected case state")
    )
    repository.open_case_engine = lambda _case_id: (_ for _ in ()).throw(
        AssertionError("report quarantine opened case database")
    )

    with pytest.raises(RuntimeError, match="^report_publication_receipt_required$"):
        repository.render_workspace_files(_CASE_ID, [file_name])


def test_mixed_report_render_batch_is_rejected_atomically_before_baseline_sync() -> None:
    repository = object.__new__(AnalysisRepository)
    repository.case_exists = lambda _case_id: (_ for _ in ()).throw(
        AssertionError("report quarantine inspected case state")
    )
    repository.sync_case_baseline = lambda _case_id: (_ for _ in ()).throw(
        AssertionError("report quarantine synchronized the baseline")
    )

    with pytest.raises(RuntimeError, match="^report_publication_receipt_required$"):
        repository.render_workspace_files(
            _CASE_ID,
            ["CASE.md", "reports/unreceipted.md"],
            sync_baseline=True,
        )


def test_direct_report_renderer_requires_receipt_before_read_or_write() -> None:
    repository = object.__new__(AnalysisRepository)

    with pytest.raises(RuntimeError, match="^report_publication_receipt_required$"):
        repository._render_workspace_file(None, _CASE_ID, "reports/unreceipted.md")


def test_report_upsert_requires_receipt_before_privacy_or_database_access() -> None:
    repository = object.__new__(AnalysisRepository)

    with pytest.raises(RuntimeError, match="^report_publication_receipt_required$"):
        repository._upsert_workspace_file(
            None,
            _CASE_ID,
            "reports/unreceipted.md",
            f"完整账号 {_ACCOUNT}",
            render_mode="report_publish",
        )


def test_async_report_projection_requires_receipt_before_job_creation() -> None:
    repository = object.__new__(AnalysisRepository)
    service = AnalysisService.__new__(AnalysisService)
    service._repository = repository
    service._next_async_write_job_id = lambda _kind: (_ for _ in ()).throw(
        AssertionError("report quarantine created an async job")
    )

    with pytest.raises(RuntimeError, match="^report_publication_receipt_required$"):
        service.refresh_workspace_projection_async(
            _CASE_ID,
            files=["reports/unreceipted.md"],
        )
