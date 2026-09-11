from __future__ import annotations

import gzip
import logging
import time
from typing import Dict, Optional

from fastapi import APIRouter, Depends, Request
from fastapi.concurrency import run_in_threadpool
from fastapi.responses import JSONResponse, Response

from app.api.data_analysis_deps import get_case_service, get_stats_service
from app.api.v1.stats_direct_payloads import build_stats_rows_direct_payload, build_stats_txn_rows_direct_payload
from app.core.data_engine_client import data_engine_product_error
from app.core.safe_observability import log_closed_diagnostic
from app.domain.case_service import CaseDeletedError, CaseNotFoundError, CaseService
from app.domain.controlled_artifact_gate import CONTROLLED_ARTIFACT_PUBLICATION_REQUIRED
from app.domain.stats_service import StatsService
from app.middleware.request_logging import get_request_id
from app.schemas.stats import (
    StatsV2RowsPublicBoundaryDTO,
    StatsV2LogConfigDTO,
    StatsV2SetTableWidthsReq,
    StatsV2TableWidthsReq,
    StatsV2TxnRowsPublicBoundaryDTO,
)
from app.utils.time import utc_now

router = APIRouter(prefix="/analysis/stats", tags=["stats"])

_TIMED_RESPONSE_GZIP_MIN_BYTES = 64 * 1024
_TIMED_RESPONSE_GZIP_LEVEL = 1
_LOGGER = logging.getLogger("analytix.data_analysis.stats")


def _meta() -> Dict[str, str]:
    return {
        "request_id": get_request_id(),
        "timestamp": utc_now().isoformat(),
    }


def _error(
    *,
    status_code: int,
    code: str,
    message: str,
    retryable: bool = False,
    details: Optional[dict] = None,
) -> JSONResponse:
    return JSONResponse(
        status_code=status_code,
        content={
            **_meta(),
            "error": {
                "code": code,
                "message": message,
                "retryable": retryable,
                "details": details or {},
            },
        },
    )


def _ok(data: dict, *, status_code: int = 200) -> JSONResponse:
    return JSONResponse(status_code=status_code, content={**_meta(), "data": data})


def _exception_error(exc: Exception) -> JSONResponse:
    data_engine_error = data_engine_product_error(exc)
    if data_engine_error is not None:
        log_closed_diagnostic(
            _LOGGER,
            logging.WARNING,
            topic="stats_request",
            code="data_engine_failed",
            flags={"retryable": bool(data_engine_error.get("retryable", False))},
        )
        return _error(**data_engine_error)
    log_closed_diagnostic(
        _LOGGER,
        logging.ERROR,
        topic="stats_request",
        code="internal_error",
        flags={"retryable": True},
    )
    return _error(
        status_code=500,
        code="INTERNAL_ERROR",
        message="unexpected server error",
        retryable=True,
    )


class _EndpointTiming:
    def __init__(self, prefix: str) -> None:
        self._prefix = str(prefix or "endpoint").strip() or "endpoint"
        self._started_at = time.perf_counter()
        self._stages: list[tuple[str, float]] = []

    def mark(self) -> float:
        return time.perf_counter()

    def record_elapsed(self, stage: str, started_at: float) -> None:
        self._stages.append(
            (
                f"{self._prefix}.{stage}",
                _round_ms((time.perf_counter() - started_at) * 1000.0),
            )
        )

    def record_duration(self, name: str, duration_ms: object) -> None:
        try:
            duration = _round_ms(float(duration_ms))
        except (TypeError, ValueError):
            return
        self._stages.append((str(name or "").strip(), duration))

    def server_timing_header(self) -> str:
        total_ms = _round_ms((time.perf_counter() - self._started_at) * 1000.0)
        stages = [(name, duration) for name, duration in self._stages if name]
        stages.append((f"{self._prefix}.total", total_ms))
        return ", ".join(f"{name};dur={duration:g}" for name, duration in stages)


def _ok_timed(
    data: dict,
    timing: _EndpointTiming,
    *,
    status_code: int = 200,
    request: Request | None = None,
) -> Response:
    started = timing.mark()
    content = {**_meta(), "data": data}
    timing.record_elapsed("response_content", started)

    started = timing.mark()
    response = JSONResponse(status_code=status_code, content=content)
    timing.record_elapsed("response_encode", started)
    raw_body = response.body or b""
    json_bytes = len(raw_body)
    content_encoding = "identity"
    if json_bytes >= _TIMED_RESPONSE_GZIP_MIN_BYTES and _request_accepts_gzip(request):
        started = timing.mark()
        compressed_body = gzip.compress(raw_body, compresslevel=_TIMED_RESPONSE_GZIP_LEVEL, mtime=0)
        timing.record_elapsed("response_gzip", started)
        response = Response(content=compressed_body, status_code=status_code, media_type="application/json")
        response.headers["Content-Encoding"] = "gzip"
        response.headers["Vary"] = "Accept-Encoding"
        content_encoding = "gzip"

    response.headers["Server-Timing"] = timing.server_timing_header()
    response.headers["Timing-Allow-Origin"] = "*"
    response.headers["X-Analytix-Json-Bytes"] = str(json_bytes)
    response.headers["X-Analytix-Content-Encoding"] = content_encoding
    return response


def _round_ms(value: float) -> float:
    return round(float(value), 3)


def _request_accepts_gzip(request: Request | None) -> bool:
    if request is None:
        return False
    accept_encoding = str(request.headers.get("accept-encoding") or "").lower()
    for raw_item in accept_encoding.split(","):
        item = raw_item.strip()
        if not item:
            continue
        token, *params = [part.strip() for part in item.split(";")]
        if token not in {"gzip", "*"}:
            continue
        quality = 1.0
        for param in params:
            if not param.startswith("q="):
                continue
            try:
                quality = float(param[2:])
            except ValueError:
                quality = 0.0
        if quality > 0:
            return True
    return False


def _record_repository_server_timing(timing: _EndpointTiming, data: dict) -> None:
    diagnostics = data.get("diagnostics") if isinstance(data, dict) else None
    if not isinstance(diagnostics, dict):
        return
    repository = diagnostics.get("repository")
    if not isinstance(repository, dict):
        return
    for item in repository.get("stages") or []:
        if not isinstance(item, dict):
            continue
        stage = str(item.get("stage") or "").strip()
        if not stage:
            continue
        timing.record_duration(f"stats.{stage}", item.get("duration_ms"))


def _ensure_case_ready(case_service: CaseService, case_id: str) -> Optional[JSONResponse]:
    try:
        is_deleted = case_service.is_case_deleted(case_id)
    except CaseNotFoundError:
        return _error(
            status_code=404,
            code="CASE_NOT_FOUND",
            message="case not found",
        )
    except CaseDeletedError:
        return _error(
            status_code=409,
            code="CASE_DELETED",
            message="case is deleted",
        )

    if is_deleted:
        return _error(
            status_code=409,
            code="CASE_DELETED",
            message="case is deleted",
        )
    return None


def _stats_query_job_quarantine() -> JSONResponse:
    return _error(
        status_code=409,
        code="HOST_EVIDENCE_RECEIPT_REQUIRED",
        message="stats query jobs require Go host evidence authority",
    )


def _stats_fact_query_quarantine() -> JSONResponse:
    return _error(
        status_code=409,
        code="HOST_EVIDENCE_RECEIPT_REQUIRED",
        message="stats case facts require Go host evidence authority",
    )


def _stats_mutation_quarantine() -> JSONResponse:
    return _error(
        status_code=409,
        code="HOST_EXECUTION_GRANT_REQUIRED",
        message="stats mutation requires a current Go host execution grant",
    )


def _stats_export_quarantine() -> JSONResponse:
    return _error(
        status_code=409,
        code="CONTROLLED_ARTIFACT_PUBLICATION_REQUIRED",
        message=CONTROLLED_ARTIFACT_PUBLICATION_REQUIRED,
        details={"phase": "p0_quarantine", "replacement": "p2_staging_host_publish"},
    )


@router.post("/v2/meta")
async def query_stats_v2_meta():
    return _stats_fact_query_quarantine()


@router.post("/v2/case-overview")
async def query_stats_v2_case_overview():
    return _stats_fact_query_quarantine()


@router.post("/v2/chart-dashboard")
async def query_stats_v2_chart_dashboard():
    return _stats_fact_query_quarantine()


@router.post("/v2/chart-detail-rows")
async def query_stats_v2_chart_detail_rows():
    return _stats_fact_query_quarantine()


@router.post("/v2/tree")
async def query_stats_v2_tree():
    return _stats_fact_query_quarantine()


@router.post("/v2/account-txn-rows")
async def query_stats_v2_account_txn_rows():
    return _stats_fact_query_quarantine()


@router.post("/v2/query/txn-rows/direct")
async def query_stats_v2_txn_rows_direct():
    out = StatsV2TxnRowsPublicBoundaryDTO(**build_stats_txn_rows_direct_payload({})).model_dump()
    return _ok(out)


@router.post("/v2/query/rows/direct")
async def query_stats_v2_rows_direct():
    out = StatsV2RowsPublicBoundaryDTO(**build_stats_rows_direct_payload({})).model_dump()
    return _ok(out)


@router.post("/v2/query/jobs")
async def create_stats_v2_query_job():
    return _stats_query_job_quarantine()


@router.get("/v2/query/jobs/{job_id}")
async def get_stats_v2_query_job(
    job_id: str,
):
    del job_id
    return _stats_query_job_quarantine()


@router.get("/v2/query/jobs/{job_id}/result")
async def get_stats_v2_query_job_result(
    job_id: str,
):
    del job_id
    return _stats_query_job_quarantine()


@router.post("/v2/query/jobs/{job_id}/cancel")
async def cancel_stats_v2_query_job(
    job_id: str,
):
    del job_id
    return _stats_query_job_quarantine()


@router.post("/v2/account-delete-info")
async def query_stats_v2_account_delete_info():
    return _stats_mutation_quarantine()


@router.post("/v2/update-account-info")
async def update_stats_v2_account_info():
    return _stats_mutation_quarantine()


@router.post("/v2/delete-accounts")
async def delete_stats_v2_accounts():
    return _stats_mutation_quarantine()


@router.post("/v2/doc-pending")
async def set_stats_v2_doc_pending():
    return _stats_mutation_quarantine()


@router.post("/v2/table-widths/get")
async def query_stats_v2_table_widths(
    payload: StatsV2TableWidthsReq,
    stats_service: StatsService = Depends(get_stats_service),
):
    try:
        data = await run_in_threadpool(
            stats_service.get_v2_table_widths,
            tab=payload.tab,
            mode=payload.mode,
        )
        return _ok(data if isinstance(data, dict) else {})
    except Exception as exc:
        return _exception_error(exc)


@router.post("/v2/table-widths/set")
async def set_stats_v2_table_widths(
    payload: StatsV2SetTableWidthsReq,
    stats_service: StatsService = Depends(get_stats_service),
):
    try:
        data = await run_in_threadpool(
            stats_service.set_v2_table_widths,
            tab=payload.tab,
            mode=payload.mode,
            version=payload.version,
            widths=payload.widths,
        )
        return _ok(data if isinstance(data, dict) else {"ok": True})
    except Exception as exc:
        return _exception_error(exc)


@router.get("/v2/log-config")
async def query_stats_v2_log_config(
    stats_service: StatsService = Depends(get_stats_service),
):
    try:
        config = stats_service.get_v2_log_config()
        return _ok({"logConfig": StatsV2LogConfigDTO(**config).model_dump()})
    except Exception as exc:
        return _exception_error(exc)


@router.post("/v2/log-debug")
async def write_stats_v2_log_debug():
    # Client-originated debug data may contain case identifiers, accounts,
    # paths, URLs, database values, exception text, or raw payloads.  This
    # retired endpoint deliberately does not parse the request body or resolve
    # a service dependency, so neither validation nor downstream diagnostics
    # can reflect that untrusted content into process logs.
    return _error(
        status_code=410,
        code="DEBUG_LOG_QUARANTINED",
        message="client debug logging is disabled",
    )


@router.post("/v2/export/default-path")
async def query_stats_v2_export_default_path():
    return _stats_export_quarantine()


@router.post("/v2/export/jobs")
async def create_stats_v2_export_job():
    return _stats_export_quarantine()


@router.get("/v2/export/jobs/{job_id}")
async def get_stats_v2_export_job(
    job_id: str,
):
    del job_id
    return _stats_export_quarantine()


@router.post("/v2/export/jobs/{job_id}/cancel")
async def cancel_stats_v2_export_job(
    job_id: str,
):
    del job_id
    return _stats_export_quarantine()
