from __future__ import annotations

import atexit
import json
import math
import threading
from functools import lru_cache
from pathlib import Path
from typing import Any, NoReturn, Optional

from app.core.execution_authority_health import (
    ExecutionAuthorityHealth,
    execution_authority_health,
    unadmitted_execution_capability_health,
)


class DataEngineUnavailableError(RuntimeError):
    def __init__(
        self,
        message: str,
        *,
        code: str = "data_engine_unavailable",
        diagnostics: dict[str, Any] | None = None,
        raw_error: Any = None,
    ) -> None:
        self.message = str(message or "analytix data engine unavailable")
        self.code = str(code or "data_engine_unavailable")
        self.diagnostics = dict(diagnostics or {})
        self.raw_error = raw_error
        super().__init__(self.message)


_CLIENT_LOCK = threading.RLock()
_CLIENT: "_DataEngineClient | None" = None
_DATA_ENGINE_CAPABILITY_UNAVAILABLE = "managed_process_capability_admission_unavailable"
_MAX_REQUEST_LINE_BYTES = 64 * 1024 * 1024
_MAX_RESPONSE_LINE_BYTES = 64 * 1024 * 1024
_MAX_REQUEST_JSON_DEPTH = 64
_MAX_REQUEST_JSON_NODES = 2_000_000
_MAX_REQUEST_JSON_STRING_CHARS = 64 * 1024 * 1024
_MAX_RESPONSE_JSON_DEPTH = 64
_MAX_RESPONSE_JSON_NODES = 2_000_000
_MAX_RESPONSE_JSON_STRING_BYTES = 64 * 1024 * 1024
_MAX_REQUEST_JSON_INTEGER_BITS = 4096
_JSON_STRING_CHUNK_CHARS = 2048
_FIXED_ERROR_MESSAGES = {
    _DATA_ENGINE_CAPABILITY_UNAVAILABLE: _DATA_ENGINE_CAPABILITY_UNAVAILABLE,
    "data_engine_unavailable": "analytix data engine unavailable",
    "data_engine_invalid_timeout": "analytix data engine request timeout is invalid",
    "data_engine_request_invalid": "analytix data engine request is invalid",
    "data_engine_request_limit_exceeded": "analytix data engine request exceeded the byte limit",
    "data_engine_response_limit_exceeded": "analytix data engine response exceeded the byte limit",
    "data_engine_response_encoding_invalid": "analytix data engine response encoding is invalid",
    "data_engine_timeout": "analytix data engine request timed out",
    "data_engine_io_failed": "analytix data engine communication failed",
    "data_engine_launch_failed": "analytix data engine launch failed",
    "data_engine_empty_response": "analytix data engine returned an empty response",
    "data_engine_invalid_response_json": "analytix data engine returned invalid JSON",
    "data_engine_invalid_response": "analytix data engine returned an invalid response",
    "data_engine_response_id_mismatch": "analytix data engine response id mismatch",
    "data_engine_process_termination_failed": "analytix data engine process termination was not confirmed",
    "data_engine_io_worker_exit_failed": "analytix data engine I/O worker exit was not confirmed",
    "invalid_request_json": "analytix data engine rejected invalid request JSON",
    "data_engine_request_contract_invalid": "analytix data engine rejected the request contract",
    "data_engine_context_mismatch": "analytix data engine rejected a mismatched case context",
    "duckdb_read_only_violation": "analytix data engine rejected a read-only operation",
    "duckdb_query_timeout": "analytix data engine query timed out",
    "duckdb_result_limit_exceeded": "analytix data engine result exceeded the resource limit",
    "duckdb_result_contract_invalid": "analytix data engine result contract was invalid",
    "data_engine_owner_conflict": "analytix data engine ownership conflict",
    "external_process_holds_duckdb": "analytix data engine database ownership conflict",
    "data_engine_unsupported_command": "analytix data engine command is unsupported",
    "data_engine_error": "analytix data engine operation failed",
}
_ENGINE_RESPONSE_ERROR_CODES = frozenset(
    {
        "invalid_request_json",
        "data_engine_request_contract_invalid",
        "data_engine_context_mismatch",
        "data_engine_request_limit_exceeded",
        "data_engine_response_limit_exceeded",
        "duckdb_read_only_violation",
        "duckdb_query_timeout",
        "duckdb_result_limit_exceeded",
        "duckdb_result_contract_invalid",
        "data_engine_owner_conflict",
        "external_process_holds_duckdb",
        "data_engine_unsupported_command",
        "data_engine_error",
    }
)


class _RequestEncodingLimit(Exception):
    pass


class _RequestEncodingInvalid(Exception):
    pass


class _ResponseJSONInvalid(Exception):
    pass


_PRODUCT_ERROR_MAP = {
    "data_engine_owner_conflict": {
        "status_code": 409,
        "code": "DATA_ENGINE_OWNER_CONFLICT",
        "message": "数据库正在被另一个 Analytix 数据引擎使用，请关闭重复打开的窗口后重试。",
        "retryable": True,
    },
    "external_process_holds_duckdb": {
        "status_code": 409,
        "code": "EXTERNAL_PROCESS_HOLDS_DUCKDB",
        "message": "数据库文件被外部进程占用，请关闭占用该文件的程序后重试。",
        "retryable": True,
    },
    "data_engine_busy": {
        "status_code": 409,
        "code": "DATA_ENGINE_BUSY",
        "message": "数据清洗正在更新数据库，分析查询将在完成后刷新。",
        "retryable": True,
    },
    "data_engine_queued": {
        "status_code": 409,
        "code": "DATA_ENGINE_QUEUED",
        "message": "数据清洗正在更新数据库，分析查询将在完成后刷新。",
        "retryable": True,
    },
    "direct_active_duckdb_access_disabled": {
        "status_code": 409,
        "code": "DIRECT_ACTIVE_DUCKDB_ACCESS_DISABLED",
        "message": "数据库访问已由 Analytix 数据引擎统一接管，请重试或重启应用。",
        "retryable": True,
    },
}


def direct_duckdb_helpers_disabled() -> bool:
    """Direct DuckDB helper fallback cannot be enabled by configuration."""

    return True


def find_data_engine_error(exc: BaseException) -> DataEngineUnavailableError | None:
    seen: set[int] = set()
    pending: list[BaseException] = [exc]
    while pending:
        current = pending.pop(0)
        if id(current) in seen:
            continue
        seen.add(id(current))
        if isinstance(current, DataEngineUnavailableError):
            return current
        if current.__cause__ is not None:
            pending.append(current.__cause__)
        if current.__context__ is not None:
            pending.append(current.__context__)
    return None


def data_engine_product_error(exc: BaseException) -> dict[str, Any] | None:
    engine_error = find_data_engine_error(exc)
    coded_error: BaseException | None = engine_error
    if coded_error is None:
        coded_error = _find_coded_product_error(exc)
    if coded_error is None:
        return None
    code = str(getattr(coded_error, "code", "") or "data_engine_error")
    template = _PRODUCT_ERROR_MAP.get(code)
    if template is None:
        return None
    return {
        "status_code": template["status_code"],
        "code": template["code"],
        "message": template["message"],
        "retryable": template["retryable"],
        "details": {"data_engine_code": code},
    }


def _find_coded_product_error(exc: BaseException) -> BaseException | None:
    seen: set[int] = set()
    pending: list[BaseException] = [exc]
    while pending:
        current = pending.pop(0)
        if id(current) in seen:
            continue
        seen.add(id(current))
        if str(getattr(current, "code", "") or "") in _PRODUCT_ERROR_MAP:
            return current
        if current.__cause__ is not None:
            pending.append(current.__cause__)
        if current.__context__ is not None:
            pending.append(current.__context__)
    return None


def data_engine_binary_health() -> dict[str, Any]:
    capability = _data_engine_capability_health()
    return {
        "data_engine_available": False,
        "data_engine_reason": capability.reason_code,
        "data_engine_bin": "",
        "data_engine_pid": None,
    }


@lru_cache(maxsize=1)
def data_engine_binary() -> Optional[Path]:
    """Compatibility projection; discovery requires a consumer admission."""

    return None


def _raise_data_engine_capability_unavailable() -> NoReturn:
    """Reject before case arguments, environment, paths, or process state.

    The shared managed-process adapter does not yet issue a versioned,
    data-engine-specific capability admission. Its global availability cannot
    authorize this consumer and no direct-process fallback remains.
    """

    _data_engine_capability_health()
    raise _fixed_error(_DATA_ENGINE_CAPABILITY_UNAVAILABLE)


def _data_engine_capability_health() -> ExecutionAuthorityHealth:
    return unadmitted_execution_capability_health(execution_authority_health())


def run_data_engine_command(
    command: str,
    *,
    case_id: str = "",
    db_path: Path | str | None = None,
    payload: dict[str, Any] | None = None,
    timeout: float = 1800,
) -> dict[str, Any]:
    _raise_data_engine_capability_unavailable()


def shutdown_data_engine() -> None:
    global _CLIENT
    with _CLIENT_LOCK:
        if _CLIENT is not None:
            _CLIENT.stop()
            _CLIENT = None


def _client() -> "_DataEngineClient":
    _raise_data_engine_capability_unavailable()


class _BoundedRequestJSONEncoder:
    def __init__(self) -> None:
        self._buffer = bytearray()
        self._nodes = 0

    def encode_line(self, value: Any) -> bytes:
        self._write_value(value, depth=0)
        self._append_bytes(b"\n")
        return bytes(self._buffer)

    def _append_bytes(self, value: bytes) -> None:
        if len(value) > _MAX_REQUEST_LINE_BYTES - len(self._buffer):
            raise _RequestEncodingLimit
        self._buffer.extend(value)

    def _take_node(self, depth: int) -> None:
        if depth > _MAX_REQUEST_JSON_DEPTH or self._nodes >= _MAX_REQUEST_JSON_NODES:
            raise _RequestEncodingLimit
        self._nodes += 1

    def _write_value(self, value: Any, *, depth: int) -> None:
        self._take_node(depth)
        value_type = type(value)
        if value is None:
            self._append_bytes(b"null")
            return
        if value_type is bool:
            self._append_bytes(b"true" if value else b"false")
            return
        if value_type is int:
            if value.bit_length() > _MAX_REQUEST_JSON_INTEGER_BITS:
                raise _RequestEncodingLimit
            self._append_bytes(str(value).encode("ascii"))
            return
        if value_type is float:
            if not math.isfinite(value):
                raise _RequestEncodingInvalid
            self._append_bytes(repr(value).encode("ascii"))
            return
        if value_type is str:
            self._write_string(value)
            return
        if value_type is list:
            self._write_list(value, depth=depth)
            return
        if value_type is dict:
            self._write_dict(value, depth=depth)
            return
        raise _RequestEncodingInvalid

    def _write_list(self, value: list[Any], *, depth: int) -> None:
        if len(value) > _MAX_REQUEST_JSON_NODES - self._nodes:
            raise _RequestEncodingLimit
        self._append_bytes(b"[")
        for index, item in enumerate(value):
            if index:
                self._append_bytes(b",")
            self._write_value(item, depth=depth + 1)
        self._append_bytes(b"]")

    def _write_dict(self, value: dict[Any, Any], *, depth: int) -> None:
        if len(value) > _MAX_REQUEST_JSON_NODES - self._nodes:
            raise _RequestEncodingLimit
        for key in value:
            if type(key) is not str:
                raise _RequestEncodingInvalid
            if len(key) > _MAX_REQUEST_JSON_STRING_CHARS:
                raise _RequestEncodingLimit
        keys = sorted(value)
        self._append_bytes(b"{")
        for index, key in enumerate(keys):
            if index:
                self._append_bytes(b",")
            self._write_string(key)
            self._append_bytes(b":")
            self._write_value(value[key], depth=depth + 1)
        self._append_bytes(b"}")

    def _write_string(self, value: str) -> None:
        if len(value) > _MAX_REQUEST_JSON_STRING_CHARS:
            raise _RequestEncodingLimit
        self._append_bytes(b'"')
        for offset in range(0, len(value), _JSON_STRING_CHUNK_CHARS):
            chunk = value[offset : offset + _JSON_STRING_CHUNK_CHARS]
            escaped = json.encoder.encode_basestring(chunk)[1:-1]
            try:
                encoded = escaped.encode("utf-8")
            except UnicodeEncodeError:
                raise _RequestEncodingInvalid from None
            self._append_bytes(encoded)
        self._append_bytes(b'"')


def _encode_request_line(request: dict[str, Any]) -> tuple[bytes | None, str | None]:
    try:
        return _BoundedRequestJSONEncoder().encode_line(request), None
    except (_RequestEncodingLimit, MemoryError):
        return None, "data_engine_request_limit_exceeded"
    except Exception:
        return None, "data_engine_request_invalid"


def _fixed_error(code: str) -> DataEngineUnavailableError:
    normalized = code if code in _FIXED_ERROR_MESSAGES else "data_engine_error"
    return DataEngineUnavailableError(_FIXED_ERROR_MESSAGES[normalized], code=normalized)


def _response_json_shape_status(content: bytes) -> str:
    containers: list[int] = []
    nodes = 0
    string_bytes = 0
    in_string = False
    escaped = False
    for byte in content:
        if in_string:
            if escaped:
                escaped = False
                string_bytes += 1
            elif byte == 0x5C:
                escaped = True
                string_bytes += 1
            elif byte == 0x22:
                in_string = False
            else:
                string_bytes += 1
            if string_bytes > _MAX_RESPONSE_JSON_STRING_BYTES:
                return "limit"
            continue
        if byte == 0x22:
            in_string = True
            escaped = False
            string_bytes = 0
            nodes += 1
        elif byte == 0x7B:
            containers.append(0x7D)
            nodes += 1
            if len(containers) > _MAX_RESPONSE_JSON_DEPTH:
                return "limit"
        elif byte == 0x5B:
            containers.append(0x5D)
            nodes += 1
            if len(containers) > _MAX_RESPONSE_JSON_DEPTH:
                return "limit"
        elif byte in (0x7D, 0x5D):
            if not containers or containers.pop() != byte:
                return "invalid"
        elif byte in (0x2C, 0x3A):
            nodes += 1
        elif byte not in b" \t\r\n":
            nodes += 1
        if nodes > _MAX_RESPONSE_JSON_NODES:
            return "limit"
    if in_string or escaped or containers:
        return "invalid"
    return "ok"


def _strict_response_json_loads(value: str) -> Any:
    def object_from_pairs(pairs: list[tuple[str, Any]]) -> dict[str, Any]:
        result: dict[str, Any] = {}
        for key, item in pairs:
            if key in result:
                raise _ResponseJSONInvalid
            result[key] = item
        return result

    def reject_constant(_value: str) -> NoReturn:
        raise _ResponseJSONInvalid

    try:
        return json.loads(
            value,
            object_pairs_hook=object_from_pairs,
            parse_constant=reject_constant,
        )
    except (json.JSONDecodeError, _ResponseJSONInvalid, RecursionError, MemoryError):
        raise _ResponseJSONInvalid from None


def _decode_response_frame(raw_line: bytes) -> str:
    if not isinstance(raw_line, bytes):
        raise _fixed_error("data_engine_io_failed")
    content = raw_line
    if content.endswith(b"\n"):
        content = content[:-1]
        if content.endswith(b"\r"):
            content = content[:-1]
    if len(content) > _MAX_RESPONSE_LINE_BYTES:
        raise _fixed_error("data_engine_response_limit_exceeded")
    shape_status = _response_json_shape_status(content)
    if shape_status == "limit":
        raise _fixed_error("data_engine_response_limit_exceeded")
    if shape_status != "ok":
        raise _fixed_error("data_engine_invalid_response_json")
    try:
        return content.decode("utf-8")
    except UnicodeDecodeError:
        raise _fixed_error("data_engine_response_encoding_invalid") from None


def _parse_data_engine_response(response_line: str, request_id: str) -> dict[str, Any]:
    if not response_line:
        raise _fixed_error("data_engine_empty_response")
    response_invalid = False
    try:
        response = _strict_response_json_loads(response_line)
    except _ResponseJSONInvalid:
        response_invalid = True
        response = None
    if response_invalid:
        # Raise outside the exception handler so neither __cause__ nor the
        # rejected parser exception remains reachable through __context__.
        raise _fixed_error("data_engine_invalid_response_json")
    if not isinstance(response, dict):
        raise _fixed_error("data_engine_invalid_response")
    if response.get("request_id") != request_id:
        raise _fixed_error("data_engine_response_id_mismatch")
    if response.get("ok") is not True:
        error = response.get("error")
        code = error.get("code") if isinstance(error, dict) else None
        if not isinstance(code, str) or code not in _ENGINE_RESPONSE_ERROR_CODES:
            code = "data_engine_error"
        raise _fixed_error(code)
    data = response.get("data")
    return data if isinstance(data, dict) else {}


class _DataEngineClient:
    def __init__(self, binary: Path) -> None:
        self.binary = binary
        self._lock = threading.RLock()
        self._process: Any = None

    @property
    def pid(self) -> Optional[int]:
        return None

    def request(
        self,
        *,
        command: str,
        case_id: str,
        db_path: str,
        payload: dict[str, Any],
        timeout: float,
    ) -> dict[str, Any]:
        _raise_data_engine_capability_unavailable()

    def _ensure_process(self) -> NoReturn:
        _raise_data_engine_capability_unavailable()

    def stop(self) -> None:
        with self._lock:
            self._process = None


atexit.register(shutdown_data_engine)
