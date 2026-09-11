from __future__ import annotations

import builtins
import io
import logging

from fastapi import FastAPI
from fastapi.testclient import TestClient

from app.api.v1 import stats as stats_api
from app.repositories import stats_repository
from app.repositories.stats_repository import StatsRepository


_CASE_ID = "case-debug-secret-71e0d4"
_ACCOUNT = "6222029912345678901"
_PATH = "/Users/private/cases/debug-secret.duckdb"
_URL = "https://debug-secret.invalid/query?account=6222029912345678901"
_ERROR = f"traceback {_CASE_ID} {_ACCOUNT} {_PATH} {_URL}"


def _client() -> TestClient:
    app = FastAPI()
    app.include_router(stats_api.router, prefix="/api/v1")
    return TestClient(app)


def _assert_no_untrusted_text(value: str) -> None:
    for secret in (_CASE_ID, _ACCOUNT, _PATH, "debug-secret.invalid", "traceback"):
        assert secret not in value


def test_stats_log_debug_endpoint_is_body_opaque_and_quarantined(caplog, capsys) -> None:
    caplog.set_level(logging.DEBUG)

    response = _client().post(
        "/api/v1/analysis/stats/v2/log-debug",
        json={
            "payload": {
                "level": "ERROR",
                "message": _ERROR,
                "data": {
                    "caseId": _CASE_ID,
                    "account": _ACCOUNT,
                    "path": _PATH,
                    "url": _URL,
                    "error": _ERROR,
                    "traceback": _ERROR,
                },
            }
        },
    )

    assert response.status_code == 410
    assert response.json()["error"] == {
        "code": "DEBUG_LOG_QUARANTINED",
        "message": "client debug logging is disabled",
        "retryable": False,
        "details": {},
    }
    _assert_no_untrusted_text(response.text)
    captured = capsys.readouterr()
    _assert_no_untrusted_text(f"{captured.out}\n{captured.err}\n{caplog.text}")


def test_stats_log_debug_endpoint_does_not_parse_malformed_body(caplog, capsys) -> None:
    caplog.set_level(logging.DEBUG)
    body = f'{{"payload":"{_ERROR}"'

    response = _client().post(
        "/api/v1/analysis/stats/v2/log-debug",
        content=body,
        headers={"content-type": "application/json"},
    )

    assert response.status_code == 410
    assert response.json()["error"]["code"] == "DEBUG_LOG_QUARANTINED"
    captured = capsys.readouterr()
    _assert_no_untrusted_text(f"{response.text}\n{captured.out}\n{captured.err}\n{caplog.text}")


def test_stats_log_debug_route_has_no_body_or_service_dependency() -> None:
    route = next(
        item
        for item in stats_api.router.routes
        if getattr(item, "path", "") == "/analysis/stats/v2/log-debug"
    )

    assert route.dependant.body_params == []
    assert route.dependant.dependencies == []


def test_stats_repository_debug_boundary_ignores_payload_and_never_prints(monkeypatch, capsys) -> None:
    stream = io.StringIO()
    handler = logging.StreamHandler(stream)
    logger = stats_repository._LOGGER
    previous_level = logger.level
    previous_propagate = logger.propagate
    logger.addHandler(handler)
    logger.setLevel(logging.DEBUG)
    logger.propagate = False

    printed: list[object] = []

    def _capture_print(*args, **_kwargs) -> None:
        printed.extend(args)

    monkeypatch.setattr(builtins, "print", _capture_print)
    repository = StatsRepository.__new__(StatsRepository)
    try:
        result = repository.log_v2_debug(
            payload={
                "level": "ERROR",
                "message": _ERROR,
                "data": {
                    "caseId": _CASE_ID,
                    "account": _ACCOUNT,
                    "path": _PATH,
                    "url": _URL,
                    "error": _ERROR,
                    "traceback": _ERROR,
                },
            }
        )
    finally:
        logger.removeHandler(handler)
        logger.setLevel(previous_level)
        logger.propagate = previous_propagate
        handler.close()

    assert result == {"ok": False, "code": "DEBUG_LOG_QUARANTINED"}
    assert printed == []
    assert stream.getvalue().strip() == "topic=stats_request code=failed retryable=false"
    captured = capsys.readouterr()
    _assert_no_untrusted_text(f"{stream.getvalue()}\n{captured.out}\n{captured.err}")


def test_stats_repository_debug_boundary_does_not_inspect_hostile_payload() -> None:
    class _HostilePayload:
        def __getattribute__(self, _name):
            raise RuntimeError(_ERROR)

        def __iter__(self):
            raise RuntimeError(_ERROR)

        def __str__(self):
            raise RuntimeError(_ERROR)

    repository = StatsRepository.__new__(StatsRepository)

    assert repository.log_v2_debug(payload=_HostilePayload()) == {
        "ok": False,
        "code": "DEBUG_LOG_QUARANTINED",
    }
