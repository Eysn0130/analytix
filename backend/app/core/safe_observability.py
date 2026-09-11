from __future__ import annotations

import logging
import math
from numbers import Real
from typing import Mapping


_ALLOWED_TOPICS = frozenset(
    {
        "analysis_async_write",
        "analysis_maintenance",
        "application",
        "cleaning_job",
        "http_request",
        "privacy_projection",
        "result_snapshot_gc",
        "stats_request",
        "websocket",
    }
)
_ALLOWED_CODES = frozenset(
    {
        "audit_failed",
        "completed",
        "data_engine_failed",
        "failed",
        "gc_failed",
        "internal_error",
        "janitor_failed",
        "projection_failed",
        "publish_failed",
        "retention_failed",
        "write_failed",
    }
)
_ALLOWED_NUMERIC_FIELDS = frozenset(
    {
        "affected_scope_count",
        "batch_size",
        "case_count",
        "deleted_bytes",
        "deleted_count",
        "duration_ms",
        "stale_count",
        "status_code",
    }
)
_ALLOWED_BOOLEAN_FIELDS = frozenset({"retryable"})
_ALLOWED_LEVELS = frozenset({logging.INFO, logging.WARNING, logging.ERROR})
_INVALID_DIAGNOSTIC = "topic=application code=internal_error"
_CLOSED_DIAGNOSTIC_MARKER = "_analytix_closed_diagnostic_v1"
_CLOSED_DIAGNOSTIC_SENTINEL = object()
_NUMERIC_BOUNDS: dict[str, tuple[Real, Real, bool]] = {
    "affected_scope_count": (0, 999_999_999, True),
    "batch_size": (0, 999_999_999, True),
    "case_count": (0, 999_999_999, True),
    "deleted_bytes": (0, 999_999_999, True),
    "deleted_count": (0, 999_999_999, True),
    "duration_ms": (0, 86_400_000, False),
    "stale_count": (0, 999_999_999, True),
    "status_code": (100, 599, True),
}


class _ClosedDiagnosticMessage(str):
    """Marker type carried through LogRecord creation without raw arguments."""


_UPSTREAM_RECORD_FACTORY = logging.getLogRecordFactory()


def _close_record(record: logging.LogRecord) -> None:
    if isinstance(record.msg, _ClosedDiagnosticMessage):
        record.msg = str(record.msg)
        setattr(record, _CLOSED_DIAGNOSTIC_MARKER, _CLOSED_DIAGNOSTIC_SENTINEL)
    elif getattr(record, _CLOSED_DIAGNOSTIC_MARKER, None) is not _CLOSED_DIAGNOSTIC_SENTINEL:
        record.name = "analytix.closed"
        record.msg = _INVALID_DIAGNOSTIC
        record.args = ()
    record.exc_info = None
    record.exc_text = None
    record.stack_info = None


def _closed_record_factory(*args, **kwargs) -> logging.LogRecord:
    record = _UPSTREAM_RECORD_FACTORY(*args, **kwargs)
    _close_record(record)
    return record


class _ClosedDiagnosticFilter(logging.Filter):
    """Replace every non-closed record before a process handler renders it."""

    def filter(self, record: logging.LogRecord) -> bool:
        _close_record(record)
        if not hasattr(record, "request_id"):
            record.request_id = "-"
        return True


_CLOSED_DIAGNOSTIC_FILTER = _ClosedDiagnosticFilter()


def _guard_handler(handler: logging.Handler) -> None:
    if not any(item is _CLOSED_DIAGNOSTIC_FILTER for item in handler.filters):
        handler.addFilter(_CLOSED_DIAGNOSTIC_FILTER)


def configure_closed_process_logging() -> None:
    """Fail closed for root/third-party handlers and disable access logging.

    The data-analysis service has no safe reason to persist request targets,
    query strings, exception text, tracebacks, or provider payloads.  Existing
    process handlers therefore accept only records issued by
    :func:`log_closed_diagnostic`; all other records collapse to a fixed code.
    """

    global _UPSTREAM_RECORD_FACTORY
    current_factory = logging.getLogRecordFactory()
    if current_factory is not _closed_record_factory:
        _UPSTREAM_RECORD_FACTORY = current_factory
        logging.setLogRecordFactory(_closed_record_factory)

    root_logger = logging.getLogger()
    for handler in tuple(root_logger.handlers):
        _guard_handler(handler)

    for value in tuple(logging.root.manager.loggerDict.values()):
        if not isinstance(value, logging.Logger):
            continue
        for handler in tuple(value.handlers):
            _guard_handler(handler)

    access_logger = logging.getLogger("uvicorn.access")
    access_logger.disabled = True
    access_logger.handlers = []
    access_logger.propagate = False


def _emit_closed(logger: logging.Logger, level: int, message: str) -> None:
    logger.log(level, _ClosedDiagnosticMessage(message))


def _render_bounded_numeric(name: str, value: Real) -> str | None:
    bounds = _NUMERIC_BOUNDS.get(name)
    if bounds is None or isinstance(value, bool) or not isinstance(value, Real):
        return None

    lower, upper, integer_only = bounds
    try:
        if isinstance(value, int):
            normalized: int | float = value
        else:
            normalized = float(value)
            if not math.isfinite(normalized):
                return None
        if normalized < lower or normalized > upper:
            return None
        if integer_only and normalized != int(normalized):
            return None
        if normalized == int(normalized):
            return str(int(normalized))
        return f"{float(normalized):.3f}"
    except Exception:
        # Numeric inputs cross an untrusted diagnostic boundary.  Custom Real
        # implementations and huge integers must never break request handling.
        return None


def log_closed_diagnostic(
    logger: logging.Logger,
    level: int,
    *,
    topic: str,
    code: str,
    numeric: Mapping[str, Real] | None = None,
    flags: Mapping[str, bool] | None = None,
) -> None:
    """Log only a closed topic/code plus bounded numeric or boolean fields.

    No caller-provided identifiers, paths, URLs, account values, exception
    strings, payloads, or traceback text are accepted by this boundary.
    Invalid diagnostics collapse to one fixed event rather than reflecting the
    rejected value into an ordinary log.
    """

    if level not in _ALLOWED_LEVELS or topic not in _ALLOWED_TOPICS or code not in _ALLOWED_CODES:
        _emit_closed(logger, logging.WARNING, _INVALID_DIAGNOSTIC)
        return

    fields = [f"topic={topic}", f"code={code}"]
    for name, value in sorted((numeric or {}).items()):
        if name not in _ALLOWED_NUMERIC_FIELDS:
            _emit_closed(logger, logging.WARNING, _INVALID_DIAGNOSTIC)
            return
        rendered = _render_bounded_numeric(name, value)
        if rendered is None:
            _emit_closed(logger, logging.WARNING, _INVALID_DIAGNOSTIC)
            return
        fields.append(f"{name}={rendered}")
    for name, value in sorted((flags or {}).items()):
        if name not in _ALLOWED_BOOLEAN_FIELDS or not isinstance(value, bool):
            _emit_closed(logger, logging.WARNING, _INVALID_DIAGNOSTIC)
            return
        fields.append(f"{name}={'true' if value else 'false'}")
    _emit_closed(logger, level, " ".join(fields))
