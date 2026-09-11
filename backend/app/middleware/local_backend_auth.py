from __future__ import annotations

import re
from hashlib import sha256
from hmac import new as new_hmac
from secrets import compare_digest

from starlette.datastructures import Headers, MutableHeaders
from starlette.responses import JSONResponse

DATA_ANALYSIS_AUTH_HEADER = "X-Analytix-Data-Analysis-Token"
DATA_ANALYSIS_AUTH_TOKEN_PATTERN = re.compile(r"^[A-Za-z0-9_-]{43}$")
DATA_ANALYSIS_LAUNCH_VALUE_PATTERN = re.compile(r"^[A-Za-z0-9_-]{43}$")
DATA_ANALYSIS_LAUNCH_READY_PROTOCOL = "analytix-data-analysis-launch-ready-v1"
SENSITIVE_RESPONSE_HEADERS = {
    "Cache-Control": "no-store, max-age=0",
    "Pragma": "no-cache",
    "Expires": "0",
}


def build_launch_ready_proof(
    *,
    token: str,
    launch_id: str,
    challenge: str,
    service: str,
    status: str,
    pid: int,
) -> str:
    if DATA_ANALYSIS_AUTH_TOKEN_PATTERN.fullmatch(str(token or "")) is None:
        raise ValueError("launch_auth_unavailable")
    if DATA_ANALYSIS_LAUNCH_VALUE_PATTERN.fullmatch(str(launch_id or "")) is None:
        raise ValueError("launch_id_unavailable")
    if DATA_ANALYSIS_LAUNCH_VALUE_PATTERN.fullmatch(str(challenge or "")) is None:
        raise ValueError("launch_challenge_invalid")
    if service != "analytix-data-analysis" or status != "ok" or not isinstance(pid, int) or pid <= 0:
        raise ValueError("launch_identity_invalid")
    payload = "\n".join(
        (
            DATA_ANALYSIS_LAUNCH_READY_PROTOCOL,
            launch_id,
            challenge,
            service,
            status,
            str(pid),
        )
    ).encode("utf-8")
    return new_hmac(token.encode("utf-8"), payload, sha256).hexdigest()


def _matches_path(path: str, prefix: str) -> bool:
    return path == prefix or path.startswith(f"{prefix}/")


class LocalBackendAuthMiddleware:
    def __init__(
        self,
        app,
        *,
        expected_token: str,
        protected_http_prefix: str = "/api/v1",
        protected_http_paths: tuple[str, ...] = ("/health/ready",),
        no_store_http_paths: tuple[str, ...] = ("/", "/health", "/health/live", "/health/launch-ready"),
        protected_websocket_path: str = "/ws/events",
    ) -> None:
        self.app = app
        self._expected_token = str(expected_token or "")
        self._configured = DATA_ANALYSIS_AUTH_TOKEN_PATTERN.fullmatch(self._expected_token) is not None
        self._protected_http_prefix = protected_http_prefix.rstrip("/") or "/"
        self._protected_http_paths = frozenset(protected_http_paths)
        self._no_store_http_paths = frozenset(no_store_http_paths)
        self._protected_websocket_path = protected_websocket_path

    def _authorized(self, scope) -> bool:
        if not self._configured:
            return False
        supplied = Headers(scope=scope).get(DATA_ANALYSIS_AUTH_HEADER, "")
        return len(supplied) == len(self._expected_token) and compare_digest(
            supplied,
            self._expected_token,
        )

    async def __call__(self, scope, receive, send) -> None:
        scope_type = scope.get("type")
        path = str(scope.get("path") or "")
        protected_http = scope_type == "http" and (
            _matches_path(path, self._protected_http_prefix) or path in self._protected_http_paths
        )
        no_store_http = protected_http or (
            scope_type == "http" and path in self._no_store_http_paths
        )
        protected_websocket = scope_type == "websocket" and path == self._protected_websocket_path

        async def send_no_store(message) -> None:
            if message.get("type") == "http.response.start":
                headers = MutableHeaders(raw=message.setdefault("headers", []))
                for name, value in SENSITIVE_RESPONSE_HEADERS.items():
                    headers[name] = value
            await send(message)

        if not protected_http and not protected_websocket:
            await self.app(scope, receive, send_no_store if no_store_http else send)
            return
        if self._authorized(scope):
            if protected_http:
                await self.app(scope, receive, send_no_store)
            else:
                await self.app(scope, receive, send)
            return
        if protected_websocket:
            await send({"type": "websocket.close", "code": 4401, "reason": "unauthorized"})
            return
        response = JSONResponse(
            status_code=401,
            content={"detail": "unauthorized"},
            headers=SENSITIVE_RESPONSE_HEADERS,
        )
        await response(scope, receive, send)
