from __future__ import annotations

import ast
import json
import logging
import os
import subprocess
import sys
from pathlib import Path

import pytest
from fastapi import FastAPI, WebSocket
from fastapi.testclient import TestClient
from starlette.websockets import WebSocketDisconnect

from app.infra.config import AppSettings, get_settings
from app.core.process_environment import (
    build_sanitized_subprocess_env,
    clear_sensitive_data_analysis_environment,
)
from app.middleware.local_backend_auth import (
    DATA_ANALYSIS_AUTH_HEADER,
    DATA_ANALYSIS_LAUNCH_READY_PROTOCOL,
    LocalBackendAuthMiddleware,
    build_launch_ready_proof,
)
from app.middleware.request_logging import RequestLoggingMiddleware

AUTH_TOKEN = "A" * 43
WRONG_TOKEN = "B" * 43
LAUNCH_ID = "L" * 43
CHALLENGE = "C" * 43
PROTECTED_PATHS = (
    "/api/v1/cases",
    "/api/v1/analysis/stats/v2/log-config",
    "/api/v1/export/jobs/job_test",
)


def _assert_no_store(response) -> None:
    assert "no-store" in response.headers["cache-control"]
    assert response.headers["pragma"] == "no-cache"
    assert response.headers["expires"] == "0"


def _test_app(expected_token: str = AUTH_TOKEN) -> FastAPI:
    app = FastAPI()
    app.add_middleware(LocalBackendAuthMiddleware, expected_token=expected_token)
    app.add_middleware(RequestLoggingMiddleware)

    @app.get("/health")
    async def health() -> dict:
        return {"status": "ok"}

    @app.get("/api/v1/cases")
    @app.get("/api/v1/analysis/stats/v2/log-config")
    @app.get("/api/v1/export/jobs/{job_id}")
    async def protected_api(job_id: str = "") -> dict:
        return {"ok": True, "job_id": job_id}

    @app.websocket("/ws/events")
    async def ws_events(websocket: WebSocket) -> None:
        await websocket.accept()
        await websocket.send_text("ready")

    return app


@pytest.mark.parametrize("path", PROTECTED_PATHS)
@pytest.mark.parametrize("headers", [{}, {DATA_ANALYSIS_AUTH_HEADER: WRONG_TOKEN}])
def test_case_stats_and_export_reject_missing_or_wrong_token(path: str, headers: dict[str, str]) -> None:
    response = TestClient(_test_app()).get(path, headers=headers)

    assert response.status_code == 401
    assert response.json() == {"detail": "unauthorized"}
    _assert_no_store(response)


@pytest.mark.parametrize("path", PROTECTED_PATHS)
def test_case_stats_and_export_accept_correct_token(path: str) -> None:
    response = TestClient(_test_app()).get(path, headers={DATA_ANALYSIS_AUTH_HEADER: AUTH_TOKEN})

    assert response.status_code == 200
    assert response.json()["ok"] is True
    _assert_no_store(response)


def test_health_is_public_and_auth_configuration_fails_closed() -> None:
    client = TestClient(_test_app(expected_token=""))

    public_health = client.get("/health")
    assert public_health.status_code == 200
    _assert_no_store(public_health)
    response = client.get("/api/v1/cases")
    assert response.status_code == 401
    assert response.json() == {"detail": "unauthorized"}
    _assert_no_store(response)

    malformed = TestClient(_test_app(expected_token="not-a-launch-token"))
    malformed_response = malformed.get(
        "/api/v1/cases",
        headers={DATA_ANALYSIS_AUTH_HEADER: "not-a-launch-token"},
    )
    assert malformed_response.status_code == 401
    assert malformed_response.json() == {"detail": "unauthorized"}
    _assert_no_store(malformed_response)


@pytest.mark.parametrize("headers", [{}, {DATA_ANALYSIS_AUTH_HEADER: WRONG_TOKEN}])
def test_websocket_rejects_missing_or_wrong_token(headers: dict[str, str]) -> None:
    with pytest.raises(WebSocketDisconnect) as error:
        with TestClient(_test_app()).websocket_connect("/ws/events", headers=headers):
            pass

    assert error.value.code == 4401


def test_websocket_accepts_correct_token() -> None:
    with TestClient(_test_app()).websocket_connect(
        "/ws/events",
        headers={DATA_ANALYSIS_AUTH_HEADER: AUTH_TOKEN},
    ) as websocket:
        assert websocket.receive_text() == "ready"


def test_request_logs_never_include_token(caplog: pytest.LogCaptureFixture) -> None:
    caplog.set_level(logging.INFO, logger="analytix.request")
    client = TestClient(_test_app())

    assert client.get(
        "/api/v1/cases",
        headers={DATA_ANALYSIS_AUTH_HEADER: AUTH_TOKEN},
    ).status_code == 200
    assert client.get(
        "/api/v1/cases",
        headers={DATA_ANALYSIS_AUTH_HEADER: WRONG_TOKEN},
    ).status_code == 401

    assert AUTH_TOKEN not in caplog.text
    assert WRONG_TOKEN not in caplog.text
    assert DATA_ANALYSIS_AUTH_HEADER not in caplog.text


def test_launch_ready_proof_is_token_launch_challenge_and_pid_bound() -> None:
    proof = build_launch_ready_proof(
        token=AUTH_TOKEN,
        launch_id=LAUNCH_ID,
        challenge=CHALLENGE,
        service="analytix-data-analysis",
        status="ok",
        pid=1234,
    )

    assert len(proof) == 64
    assert proof == build_launch_ready_proof(
        token=AUTH_TOKEN,
        launch_id=LAUNCH_ID,
        challenge=CHALLENGE,
        service="analytix-data-analysis",
        status="ok",
        pid=1234,
    )
    assert proof != build_launch_ready_proof(
        token=AUTH_TOKEN,
        launch_id=LAUNCH_ID,
        challenge="D" * 43,
        service="analytix-data-analysis",
        status="ok",
        pid=1234,
    )


def test_cors_defaults_and_env_are_restricted_to_desktop_loopback(monkeypatch: pytest.MonkeyPatch) -> None:
    monkeypatch.setenv(
        "ANALYTIX_CORS_ORIGINS",
        "*,https://attacker.example,http://127.0.0.1:5173/,http://localhost:5174/path,null,file://",
    )
    monkeypatch.setenv("ANALYTIX_DATA_ANALYSIS_AUTH_TOKEN", AUTH_TOKEN)
    monkeypatch.setenv("ANALYTIX_DATA_ANALYSIS_LAUNCH_ID", LAUNCH_ID)
    get_settings.cache_clear()
    try:
        settings = AppSettings.from_env()
    finally:
        get_settings.cache_clear()

    assert settings.cors_allow_origins == ["http://127.0.0.1:5173", "null", "file://"]
    assert "*" not in settings.cors_allow_origins
    assert "https://attacker.example" not in settings.cors_allow_origins
    assert settings.data_analysis_auth_token.get_secret_value() == AUTH_TOKEN
    assert settings.data_analysis_launch_id == LAUNCH_ID
    assert AUTH_TOKEN not in repr(settings)
    assert LAUNCH_ID not in repr(settings)


def test_sensitive_launch_environment_is_cleared_after_settings_capture(
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    monkeypatch.setenv("ANALYTIX_DATA_ANALYSIS_AUTH_TOKEN", AUTH_TOKEN)
    monkeypatch.setenv("ANALYTIX_DATA_ANALYSIS_LAUNCH_ID", LAUNCH_ID)
    monkeypatch.setenv("ANALYTIX_DATA_ANALYSIS_FUTURE_SECRET", "must-not-survive")
    monkeypatch.setenv("ANALYTIX_DATA_ANALYSIS_DIR", "/safe/data-root")
    get_settings.cache_clear()
    try:
        settings = get_settings()
        clear_sensitive_data_analysis_environment()

        assert settings.data_analysis_auth_token.get_secret_value() == AUTH_TOKEN
        assert settings.data_analysis_launch_id == LAUNCH_ID
        assert "ANALYTIX_DATA_ANALYSIS_AUTH_TOKEN" not in os.environ
        assert "ANALYTIX_DATA_ANALYSIS_LAUNCH_ID" not in os.environ
        assert "ANALYTIX_DATA_ANALYSIS_FUTURE_SECRET" not in os.environ
        assert os.environ["ANALYTIX_DATA_ANALYSIS_DIR"] == "/safe/data-root"
    finally:
        get_settings.cache_clear()


def test_sanitized_subprocess_environment_inherits_only_the_closed_platform_baseline() -> None:
    source = {
        "PATH": os.environ.get("PATH", ""),
        "TMPDIR": "/tmp/analytix",
        "LANG": "C.UTF-8",
        "ANALYTIX_DATA_ANALYSIS_AUTH_TOKEN": AUTH_TOKEN,
        "ANALYTIX_DATA_ANALYSIS_LAUNCH_ID": LAUNCH_ID,
        "ANALYTIX_DATA_ANALYSIS_FUTURE_PROOF": "must-not-survive",
        "ANALYTIX_DATA_ANALYSIS_DIR": "/safe/data-root",
        "OPENAI_API_KEY": "must-not-survive",
        "AWS_SESSION_TOKEN": "must-not-survive",
        "GITHUB_TOKEN": "must-not-survive",
        "NPM_CONFIG_USERCONFIG": "/tmp/host-npmrc",
        "HTTPS_PROXY": "http://credential@proxy.invalid",
        "SSH_AUTH_SOCK": "/tmp/agent.sock",
        "DBUS_SESSION_BUS_ADDRESS": "unix:path=/tmp/dbus",
        "UNKNOWN_HOST_CAPABILITY": "must-not-survive",
    }
    sanitized = build_sanitized_subprocess_env(source)

    completed = subprocess.run(
        [
            sys.executable,
            "-c",
            (
                "import json, os; "
                "print(json.dumps({key: os.environ.get(key) for key in "
                "['ANALYTIX_DATA_ANALYSIS_AUTH_TOKEN', "
                "'ANALYTIX_DATA_ANALYSIS_LAUNCH_ID', "
                "'ANALYTIX_DATA_ANALYSIS_FUTURE_PROOF', "
                "'ANALYTIX_DATA_ANALYSIS_DIR', "
                "'OPENAI_API_KEY', 'AWS_SESSION_TOKEN', 'GITHUB_TOKEN', "
                "'NPM_CONFIG_USERCONFIG', 'HTTPS_PROXY', 'SSH_AUTH_SOCK', "
                "'DBUS_SESSION_BUS_ADDRESS', 'UNKNOWN_HOST_CAPABILITY', "
                "'TMPDIR', 'LANG']}))"
            ),
        ],
        env=sanitized,
        check=True,
        capture_output=True,
        text=True,
    )

    assert json.loads(completed.stdout) == {
        "ANALYTIX_DATA_ANALYSIS_AUTH_TOKEN": None,
        "ANALYTIX_DATA_ANALYSIS_LAUNCH_ID": None,
        "ANALYTIX_DATA_ANALYSIS_FUTURE_PROOF": None,
        "ANALYTIX_DATA_ANALYSIS_DIR": None,
        "OPENAI_API_KEY": None,
        "AWS_SESSION_TOKEN": None,
        "GITHUB_TOKEN": None,
        "NPM_CONFIG_USERCONFIG": None,
        "HTTPS_PROXY": None,
        "SSH_AUTH_SOCK": None,
        "DBUS_SESSION_BUS_ADDRESS": None,
        "UNKNOWN_HOST_CAPABILITY": None,
        "TMPDIR": None,
        "LANG": "C.UTF-8",
    }


def test_every_backend_subprocess_call_uses_the_sanitized_environment() -> None:
    app_root = Path(__file__).resolve().parents[1] / "app"
    missing: list[str] = []
    for source_path in sorted(app_root.rglob("*.py")):
        tree = ast.parse(source_path.read_text(encoding="utf-8"), filename=str(source_path))
        for node in ast.walk(tree):
            if not isinstance(node, ast.Call) or not isinstance(node.func, ast.Attribute):
                continue
            if (
                not isinstance(node.func.value, ast.Name)
                or node.func.value.id != "subprocess"
                or node.func.attr not in {"run", "Popen"}
            ):
                continue
            env_keyword = next((keyword for keyword in node.keywords if keyword.arg == "env"), None)
            valid_env = (
                env_keyword is not None
                and isinstance(env_keyword.value, ast.Call)
                and isinstance(env_keyword.value.func, ast.Name)
                and env_keyword.value.func.id == "build_sanitized_subprocess_env"
            )
            if not valid_env:
                missing.append(f"{source_path.relative_to(app_root)}:{node.lineno}")

    assert missing == []


def test_real_app_launch_ready_returns_exact_no_store_proof(monkeypatch: pytest.MonkeyPatch) -> None:
    from app.main import create_app

    monkeypatch.setenv("ANALYTIX_DATA_ANALYSIS_AUTH_TOKEN", AUTH_TOKEN)
    monkeypatch.setenv("ANALYTIX_DATA_ANALYSIS_LAUNCH_ID", LAUNCH_ID)
    get_settings.cache_clear()
    try:
        with TestClient(create_app()) as client:
            response = client.get(f"/health/launch-ready?challenge={CHALLENGE}")
            malformed = client.get("/health/launch-ready?challenge=short")

        assert "ANALYTIX_DATA_ANALYSIS_AUTH_TOKEN" not in os.environ
        assert "ANALYTIX_DATA_ANALYSIS_LAUNCH_ID" not in os.environ
        assert response.status_code == 200
        _assert_no_store(response)
        assert response.json() == {
            "protocol": DATA_ANALYSIS_LAUNCH_READY_PROTOCOL,
            "service": "analytix-data-analysis",
            "status": "ok",
            "launchId": LAUNCH_ID,
            "challenge": CHALLENGE,
            "pid": os.getpid(),
            "proof": build_launch_ready_proof(
                token=AUTH_TOKEN,
                launch_id=LAUNCH_ID,
                challenge=CHALLENGE,
                service="analytix-data-analysis",
                status="ok",
                pid=os.getpid(),
            ),
        }
        assert AUTH_TOKEN not in response.text
        assert malformed.status_code == 422
        _assert_no_store(malformed)
    finally:
        get_settings.cache_clear()


def test_real_app_wires_auth_before_case_stats_export_and_cors(monkeypatch: pytest.MonkeyPatch) -> None:
    from app.main import create_app

    allowed_origin = "http://127.0.0.1:5173"
    monkeypatch.setenv("ANALYTIX_CORS_ORIGINS", allowed_origin)
    monkeypatch.setenv("ANALYTIX_DATA_ANALYSIS_AUTH_TOKEN", AUTH_TOKEN)
    monkeypatch.setenv("ANALYTIX_DATA_ANALYSIS_LAUNCH_ID", LAUNCH_ID)
    get_settings.cache_clear()
    try:
        client = TestClient(create_app())
        for path in PROTECTED_PATHS:
            assert client.get(path).status_code == 401
            assert client.get(path, headers={DATA_ANALYSIS_AUTH_HEADER: WRONG_TOKEN}).status_code == 401
            authorized = client.get(path, headers={DATA_ANALYSIS_AUTH_HEADER: AUTH_TOKEN})
            assert authorized.status_code != 401
            _assert_no_store(authorized)

        preflight = client.options(
            "/api/v1/cases",
            headers={
                "Origin": allowed_origin,
                "Access-Control-Request-Method": "GET",
                "Access-Control-Request-Headers": f"content-type,{DATA_ANALYSIS_AUTH_HEADER}",
            },
        )
        assert preflight.status_code == 200
        assert preflight.headers["access-control-allow-origin"] == allowed_origin
        assert DATA_ANALYSIS_AUTH_HEADER.lower() in preflight.headers["access-control-allow-headers"].lower()

        blocked_origin = client.options(
            "/api/v1/cases",
            headers={
                "Origin": "https://attacker.example",
                "Access-Control-Request-Method": "GET",
            },
        )
        assert blocked_origin.status_code == 400
        assert "access-control-allow-origin" not in blocked_origin.headers
    finally:
        get_settings.cache_clear()


def test_real_app_public_health_is_minimal_and_detailed_readiness_requires_auth(
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    from app.main import create_app

    monkeypatch.setenv("ANALYTIX_DATA_ANALYSIS_AUTH_TOKEN", AUTH_TOKEN)
    monkeypatch.setenv("ANALYTIX_DATA_ANALYSIS_LAUNCH_ID", LAUNCH_ID)
    get_settings.cache_clear()
    try:
        with TestClient(create_app()) as client:
            public_health = client.get("/health")
            unauthenticated_readiness = client.get("/health/ready")
            authenticated_readiness = client.get(
                "/health/ready",
                headers={DATA_ANALYSIS_AUTH_HEADER: AUTH_TOKEN},
            )

        assert public_health.json() == {"status": "alive"}
        assert "pid" not in public_health.text.lower()
        assert "/" not in public_health.text
        _assert_no_store(public_health)
        assert unauthenticated_readiness.status_code == 401
        assert unauthenticated_readiness.json() == {"detail": "unauthorized"}
        _assert_no_store(unauthenticated_readiness)
        assert authenticated_readiness.status_code == 200
        assert authenticated_readiness.json()["service"] == "analytix-data-analysis"
        assert isinstance(authenticated_readiness.json()["checks"], dict)
        _assert_no_store(authenticated_readiness)
    finally:
        get_settings.cache_clear()
