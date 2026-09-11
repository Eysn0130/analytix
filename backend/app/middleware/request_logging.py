import logging
from contextvars import ContextVar, Token
from time import perf_counter
from typing import Optional
from uuid import uuid4

from starlette.datastructures import Headers, MutableHeaders

from app.core.safe_observability import log_closed_diagnostic

_REQUEST_ID_CTX: ContextVar[str] = ContextVar("request_id", default="-")
REQUEST_DURATION_HEADER = "X-Analytix-Request-Duration-Ms"
REQUEST_SERVER_TIMING_METRIC = "analytix.request"


class RequestContextFilter(logging.Filter):
    def filter(self, record: logging.LogRecord) -> bool:
        record.request_id = _REQUEST_ID_CTX.get("-")
        return True


def get_request_id() -> str:
    return _REQUEST_ID_CTX.get("-")


def bind_request_id(request_id: str) -> Token:
    return _REQUEST_ID_CTX.set(request_id)


def reset_request_id(token: Token) -> None:
    _REQUEST_ID_CTX.reset(token)


def _round_duration_ms(started_at: float) -> float:
    return round((perf_counter() - started_at) * 1000, 3)


def _issue_request_id() -> str:
    """Issue correlation authority locally; client headers are untrusted data."""

    return str(uuid4())


def _append_server_timing(headers: MutableHeaders, metric: str, duration_ms: float) -> None:
    item = f"{metric};dur={duration_ms:g}"
    existing = headers.get("Server-Timing")
    headers["Server-Timing"] = f"{existing}, {item}" if existing else item


class RequestLoggingMiddleware:
    def __init__(self, app, logger: Optional[logging.Logger] = None) -> None:
        self.app = app
        self._logger = logger or logging.getLogger("analytix.request")

    async def __call__(self, scope, receive, send) -> None:
        if scope["type"] != "http":
            await self.app(scope, receive, send)
            return

        # Parse headers so malformed ASGI input still fails at the boundary,
        # but never echo or persist a caller-controlled correlation value.
        _ = Headers(scope=scope).get("X-Request-Id")
        request_id = _issue_request_id()
        token = bind_request_id(request_id)
        started_at = perf_counter()
        status_code = 500

        async def send_with_timing(message) -> None:
            nonlocal status_code
            if message["type"] == "http.response.start":
                status_code = int(message.get("status") or 0)
                duration_ms = _round_duration_ms(started_at)
                headers = MutableHeaders(scope=message)
                headers["X-Request-Id"] = request_id
                headers[REQUEST_DURATION_HEADER] = f"{duration_ms:g}"
                _append_server_timing(headers, REQUEST_SERVER_TIMING_METRIC, duration_ms)
            await send(message)

        try:
            try:
                await self.app(scope, receive, send_with_timing)
            except Exception:
                duration_ms = _round_duration_ms(started_at)
                log_closed_diagnostic(
                    self._logger,
                    logging.ERROR,
                    topic="http_request",
                    code="failed",
                    numeric={"duration_ms": duration_ms},
                )
                raise

            duration_ms = _round_duration_ms(started_at)
            log_closed_diagnostic(
                self._logger,
                logging.INFO,
                topic="http_request",
                code="completed",
                numeric={"duration_ms": duration_ms, "status_code": status_code},
            )
        finally:
            reset_request_id(token)
