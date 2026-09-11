from __future__ import annotations

import asyncio
import ast
import io
import json
import logging
import re
from pathlib import Path
from types import SimpleNamespace
from uuid import UUID

import pytest

from app.api.v1 import stats as stats_api
from app.core.safe_observability import configure_closed_process_logging, log_closed_diagnostic
from app.core.fc_import_single_file_result import (
    SingleFileImportSummary,
    format_single_file_import_log,
)
from app.domain.analysis_service import AnalysisPrivacyProjectionError, AnalysisService
from app.domain.cleaning_service import CleaningService
from app.middleware.request_logging import RequestLoggingMiddleware, get_request_id
from app.ws.manager import WebSocketManager


_CASE_ID = "case-secret-38dc50241996"
_ACCOUNT = "0000622202123456789"
_LOCAL_PATH = "/Users/private/cases/case.duckdb"
_RAW_ERROR = f"failed {_CASE_ID} {_ACCOUNT} {_LOCAL_PATH} https://secret.invalid/case"


def _captured_logger(name: str) -> tuple[logging.Logger, io.StringIO, logging.Handler]:
    stream = io.StringIO()
    handler = logging.StreamHandler(stream)
    logger = logging.getLogger(name)
    logger.setLevel(logging.DEBUG)
    logger.addHandler(handler)
    logger.propagate = False
    return logger, stream, handler


def _release_logger(logger: logging.Logger, handler: logging.Handler) -> None:
    logger.removeHandler(handler)
    handler.close()


def test_closed_diagnostic_never_reflects_rejected_text() -> None:
    logger, stream, handler = _captured_logger("test.closed.observability")
    try:
        log_closed_diagnostic(
            logger,
            logging.WARNING,
            topic="stats_request",
            code="data_engine_failed",
            numeric={"status_code": 409},
            flags={"retryable": True},
        )
        log_closed_diagnostic(
            logger,
            logging.WARNING,
            topic=_RAW_ERROR,
            code=_RAW_ERROR,
            numeric={_RAW_ERROR: 1},
        )
    finally:
        _release_logger(logger, handler)

    output = stream.getvalue()
    assert "topic=stats_request code=data_engine_failed" in output
    assert "status_code=409" in output
    assert "retryable=true" in output
    assert "topic=application code=internal_error" in output
    for secret in (_CASE_ID, _ACCOUNT, _LOCAL_PATH, "secret.invalid"):
        assert secret not in output


def test_closed_diagnostic_rejects_huge_or_account_shaped_numbers_without_raising() -> None:
    logger, stream, handler = _captured_logger("test.closed.numeric.observability")
    try:
        log_closed_diagnostic(
            logger,
            logging.INFO,
            topic="result_snapshot_gc",
            code="completed",
            numeric={"deleted_bytes": 10**100_000},
        )
        log_closed_diagnostic(
            logger,
            logging.INFO,
            topic="result_snapshot_gc",
            code="completed",
            numeric={"deleted_bytes": int(_ACCOUNT)},
        )
    finally:
        _release_logger(logger, handler)

    output = stream.getvalue()
    assert output.count("topic=application code=internal_error") == 2
    assert _ACCOUNT not in output
    assert str(int(_ACCOUNT)) not in output


def test_stats_internal_error_response_and_log_do_not_reflect_exception() -> None:
    logger = stats_api._LOGGER
    stream = io.StringIO()
    handler = logging.StreamHandler(stream)
    logger.addHandler(handler)
    logger.setLevel(logging.DEBUG)
    try:
        response = stats_api._exception_error(RuntimeError(_RAW_ERROR))
    finally:
        logger.removeHandler(handler)
        handler.close()

    body = json.loads(response.body.decode("utf-8"))
    assert response.status_code == 500
    assert body["error"] == {
        "code": "INTERNAL_ERROR",
        "message": "unexpected server error",
        "retryable": True,
        "details": {},
    }
    combined = f"{stream.getvalue()}\n{response.body.decode('utf-8')}"
    for secret in (_CASE_ID, _ACCOUNT, _LOCAL_PATH, "secret.invalid"):
        assert secret not in combined


def test_unhandled_http_exception_boundary_returns_only_fixed_fields() -> None:
    from app.main import create_app

    app = create_app()
    handler = app.exception_handlers[Exception]
    response = asyncio.run(handler(None, RuntimeError(_RAW_ERROR)))

    body = json.loads(response.body.decode("utf-8"))
    assert response.status_code == 500
    assert body["request_id"] == "-"
    assert body["error"] == {
        "code": "INTERNAL_ERROR",
        "message": "unexpected server error",
        "retryable": True,
        "details": {},
    }
    combined = response.body.decode("utf-8")
    for secret in (_CASE_ID, _ACCOUNT, _LOCAL_PATH, "secret.invalid", "Traceback"):
        assert secret not in combined


def test_unhandled_http_exception_boundary_survives_hostile_exception_metadata() -> None:
    from app.main import create_app

    class _HostileException(RuntimeError):
        @property
        def code(self):
            raise RuntimeError(_RAW_ERROR)

    app = create_app()
    handler = app.exception_handlers[Exception]
    response = asyncio.run(handler(None, _HostileException(_RAW_ERROR)))

    body = json.loads(response.body.decode("utf-8"))
    assert response.status_code == 500
    assert body["error"]["code"] == "INTERNAL_ERROR"
    assert body["error"]["message"] == "unexpected server error"
    assert _RAW_ERROR not in response.body.decode("utf-8")


def test_main_websocket_route_rejects_missing_case_before_accept() -> None:
    from app.main import create_app

    app = create_app()
    route = next(item for item in app.routes if getattr(item, "path", "") == "/ws/events")

    class _BoundarySocket:
        def __init__(self) -> None:
            self.app = SimpleNamespace(state=SimpleNamespace(settings=SimpleNamespace(ws_contract_version="v1")))
            self.query_params: dict[str, str] = {}
            self.accepted = False
            self.closed: tuple[int, str] | None = None

        async def accept(self) -> None:
            self.accepted = True

        async def close(self, *, code: int, reason: str) -> None:
            self.closed = (code, reason)

    websocket = _BoundarySocket()
    asyncio.run(route.endpoint(websocket))

    assert websocket.accepted is False
    assert websocket.closed == (4400, "case_id_required")


def test_cleaning_internal_error_payload_does_not_reflect_exception() -> None:
    payload = CleaningService._job_error_payload(RuntimeError(_RAW_ERROR))

    assert payload == {
        "status_code": 500,
        "code": "INTERNAL_ERROR",
        "message": "cleaning failed",
        "retryable": False,
        "details": {},
    }
    assert _RAW_ERROR not in json.dumps(payload, ensure_ascii=False)


class _FailingPrivacyProjectionRepository:
    def is_enabled(self, _case_id: str) -> bool:
        return True

    def project_model_payload(self, _case_id: str, _payload):
        raise RuntimeError(_RAW_ERROR)

    def project_model_text(self, _case_id: str, _text: str) -> str:
        raise RuntimeError(_RAW_ERROR)


def test_privacy_projection_failure_is_fixed_and_fail_closed() -> None:
    logger, stream, handler = _captured_logger("test.privacy.observability")
    service = AnalysisService.__new__(AnalysisService)
    service._privacy_projection_repository = _FailingPrivacyProjectionRepository()
    service._logger = logger
    try:
        with pytest.raises(AnalysisPrivacyProjectionError, match="^privacy_projection_failed$"):
            service.project_privacy_model_payload(_CASE_ID, {"account": _ACCOUNT})
        with pytest.raises(AnalysisPrivacyProjectionError, match="^privacy_projection_failed$"):
            service.project_privacy_model_text(_CASE_ID, _ACCOUNT)
    finally:
        _release_logger(logger, handler)

    output = stream.getvalue()
    assert output.count("topic=privacy_projection code=projection_failed") == 2
    for secret in (_CASE_ID, _ACCOUNT, _LOCAL_PATH, "secret.invalid"):
        assert secret not in output


def test_request_logging_omits_raw_method_path_and_exception() -> None:
    logger, stream, handler = _captured_logger("test.request.observability")

    async def successful_app(_scope, _receive, send) -> None:
        await send({"type": "http.response.start", "status": 204, "headers": []})
        await send({"type": "http.response.body", "body": b""})

    async def failing_app(_scope, _receive, _send) -> None:
        raise RuntimeError(_RAW_ERROR)

    scope = {
        "type": "http",
        "method": f"GET-{_ACCOUNT}",
        "path": f"/analysis/cases/{_CASE_ID}/open/{_LOCAL_PATH}",
        "headers": [],
    }

    async def receive():
        return {"type": "http.disconnect"}

    async def send(_message) -> None:
        return None

    try:
        asyncio.run(RequestLoggingMiddleware(successful_app, logger=logger)(scope, receive, send))
        with pytest.raises(RuntimeError, match=_CASE_ID):
            asyncio.run(RequestLoggingMiddleware(failing_app, logger=logger)(scope, receive, send))
    finally:
        _release_logger(logger, handler)

    output = stream.getvalue()
    assert "topic=http_request code=completed" in output
    assert "status_code=204" in output
    assert "topic=http_request code=failed" in output
    for secret in (_CASE_ID, _ACCOUNT, _LOCAL_PATH, "secret.invalid"):
        assert secret not in output


def test_request_id_is_host_issued_and_never_reflects_client_header() -> None:
    logger, stream, handler = _captured_logger("test.request.id.observability")
    response_start: dict = {}
    malicious_request_id = f"{_CASE_ID}\r\nX-Leak: {_ACCOUNT}"
    observed_request_ids: list[str] = []

    async def successful_app(_scope, _receive, send) -> None:
        observed_request_ids.append(get_request_id())
        await send({"type": "http.response.start", "status": 200, "headers": []})
        await send({"type": "http.response.body", "body": b""})

    scope = {
        "type": "http",
        "method": "GET",
        "path": "/health",
        "headers": [(b"x-request-id", malicious_request_id.encode("utf-8"))],
    }

    async def receive():
        return {"type": "http.disconnect"}

    async def send(message) -> None:
        if message["type"] == "http.response.start":
            response_start.update(message)

    try:
        asyncio.run(RequestLoggingMiddleware(successful_app, logger=logger)(scope, receive, send))
    finally:
        _release_logger(logger, handler)

    headers = {key.decode("latin-1").lower(): value.decode("latin-1") for key, value in response_start["headers"]}
    response_request_id = headers["x-request-id"]
    assert str(UUID(response_request_id)) == response_request_id
    assert observed_request_ids == [response_request_id]
    assert get_request_id() == "-"
    combined = f"{stream.getvalue()}\n{response_start!r}"
    assert malicious_request_id not in combined
    assert _CASE_ID not in combined
    assert _ACCOUNT not in combined


def test_process_logging_guard_closes_third_party_and_disables_uvicorn_access() -> None:
    logger, stream, handler = _captured_logger("third.party.case.payload")
    access_logger = logging.getLogger("uvicorn.access")
    previous_disabled = access_logger.disabled
    previous_handlers = list(access_logger.handlers)
    previous_propagate = access_logger.propagate
    try:
        configure_closed_process_logging()
        try:
            raise RuntimeError(_RAW_ERROR)
        except RuntimeError:
            logger.exception(_RAW_ERROR)
    finally:
        _release_logger(logger, handler)
        access_logger.disabled = previous_disabled
        access_logger.handlers = previous_handlers
        access_logger.propagate = previous_propagate

    output = stream.getvalue()
    assert output.strip() == "topic=application code=internal_error"
    for secret in (_CASE_ID, _ACCOUNT, _LOCAL_PATH, "secret.invalid", "Traceback"):
        assert secret not in output


def test_process_logging_guard_closes_handlers_added_after_configuration() -> None:
    configure_closed_process_logging()
    logger, stream, handler = _captured_logger("third.party.late.handler")
    try:
        try:
            raise RuntimeError(_RAW_ERROR)
        except RuntimeError:
            logger.exception(_RAW_ERROR)
    finally:
        _release_logger(logger, handler)

    output = stream.getvalue()
    assert output.strip() == "topic=application code=internal_error"
    for secret in (_CASE_ID, _ACCOUNT, _LOCAL_PATH, "secret.invalid", "Traceback"):
        assert secret not in output


class _RecordingWebSocket:
    def __init__(self) -> None:
        self.events: list[dict] = []

    async def send_json(self, event: dict) -> None:
        self.events.append(event)


def test_websocket_requires_case_binding_and_closes_raw_diagnostics() -> None:
    manager = WebSocketManager()
    unbound = _RecordingWebSocket()
    case_a = _RecordingWebSocket()
    case_b = _RecordingWebSocket()

    async def exercise() -> None:
        with pytest.raises(ValueError, match="^case_id_required$"):
            await manager.connect(unbound, None)
        await manager.connect(case_a, "case_alpha")
        await manager.connect(case_b, "case_beta")
        await manager.publish(
            {
                "event": "job.failed",
                "type": "error",
                "case_id": "case_alpha",
                "payload": {"message": _RAW_ERROR, "traceback": _RAW_ERROR},
            }
        )
        await manager.publish(
            {
                "event": "job.completed",
                "type": "info",
                "case_id": None,
                "payload": {"message": _RAW_ERROR},
            }
        )
        await manager.publish(
            {
                "event": "job.progress",
                "type": "info",
                "case_id": "case_alpha",
                "payload": {
                    "status": "running",
                    "progress": 50,
                    "counters": {"processed": 3},
                    "account_number": _ACCOUNT,
                    "rows": [{"account_number": _ACCOUNT}],
                },
            }
        )

    asyncio.run(exercise())

    assert unbound.events == []
    assert case_b.events == []
    assert case_a.events == [
        {
            "event": "system.diagnostic_blocked",
            "type": "error",
            "case_id": "case_alpha",
            "payload": {
                "code": "INTERNAL_ERROR",
                "message": "operation failed",
                "retryable": True,
                "details": {},
            },
        },
        {
            "event": "job.progress",
            "type": "info",
            "case_id": "case_alpha",
            "payload": {
                "contract": "OperationalWebSocketEventV1",
                "status": "running",
                "progress": 50,
                "counters": {"processed": 3},
                "fact_answer_allowed": False,
                "raw_details_exposed": False,
            },
        },
    ]
    assert _ACCOUNT not in json.dumps(case_a.events, ensure_ascii=False)
    assert _RAW_ERROR not in json.dumps(case_a.events, ensure_ascii=False)


def test_backend_ordinary_logging_uses_only_closed_diagnostic_boundary() -> None:
    app_root = Path(__file__).resolve().parents[1] / "app"
    direct_log_call = re.compile(
        r"(?:\b(?:logger|_LOGGER|self\._logger)\.(?:debug|info|warning|error|exception|critical)\s*\("
        r"|\blogging\.(?:debug|info|warning|error|exception|critical)\s*\("
        r"|\blogging\.getLogger\([^\n]*\)\.(?:debug|info|warning|error|exception|critical)\s*\()",
        re.MULTILINE,
    )
    violations = []
    for source_path in sorted(app_root.rglob("*.py")):
        if source_path.name == "safe_observability.py":
            continue
        source = source_path.read_text(encoding="utf-8")
        if direct_log_call.search(source):
            violations.append(str(source_path.relative_to(app_root)))

    assert violations == []


def test_backend_has_no_stdout_print_bypass_and_import_detail_is_identifier_free() -> None:
    app_root = Path(__file__).resolve().parents[1] / "app"
    violations: list[str] = []
    for source_path in sorted(app_root.rglob("*.py")):
        tree = ast.parse(source_path.read_text(encoding="utf-8"), filename=str(source_path))
        if any(
            isinstance(node, ast.Call)
            and isinstance(node.func, ast.Name)
            and node.func.id == "print"
            for node in ast.walk(tree)
        ):
            violations.append(str(source_path.relative_to(app_root)))

    summary = SingleFileImportSummary(
        display_name=f"case-{_CASE_ID}-{_ACCOUNT}-{_LOCAL_PATH}",
        total_rows=5,
        inserted_rows=4,
        norm_inserted=4,
        dedup_rows=1,
        chunk_count=1,
        staging_elapsed_s=0.5,
        row_write_elapsed_s=0.25,
    )
    rendered = format_single_file_import_log(summary)

    assert violations == []
    assert "topic=application code=completed" in rendered
    for secret in (_CASE_ID, _ACCOUNT, _LOCAL_PATH):
        assert secret not in rendered
