from __future__ import annotations

import hashlib
import logging
import math
import threading
import time
from typing import Any, Optional, Sequence, Tuple
from uuid import UUID

from app.core.analysis_compute import (
    AnalysisComputeUnavailableError,
    apply_layout_node_plan,
    apply_layout_worker_node_plan,
    clear_layout_node_meta,
    merge_flow_same_name_graph,
    project_analysis_graph_data,
    project_flow_graph_data_merge,
    project_flow_graph_render_plan,
    project_flow_graph_search,
    project_flow_layout_network_community_quality,
    project_flow_layout_network_plan,
    project_flow_layout_network_sector_placement,
    project_flow_projection_layout_seed,
    project_layout_cache_key,
    project_layout_node_plan_result,
    project_layout_role_graph,
)
from app.core.safe_observability import log_closed_diagnostic
from app.repositories.flow_repository import (
    FlowCaseLifecycleBindingV1,
    FlowRepository,
    FlowViewNotFoundError,
)
from app.tasks.failure_boundary import closed_task_failure_event_payload
from app.tasks.models import TaskRecord, TaskStatus, TaskType
from app.tasks.service import TaskNotFoundError, TaskService
from app.tasks.state_machine import InvalidTaskTransitionError
from app.utils.time import utc_now
from app.ws.events import build_event
from app.ws.manager import WebSocketManager


class FlowViewNotFound(KeyError):
    pass


class FlowJobNotFoundError(KeyError):
    pass


class FlowJobNotReadyError(RuntimeError):
    pass


class FlowJobNotCancellableError(RuntimeError):
    pass


class FlowCaseBindingMismatchError(PermissionError):
    pass


_FLOW_PUBLIC_REF_DOMAIN = b"AnalytixFlowPublicSnapshotRefV1\x00"
_FLOW_PUBLIC_EVENT_STAGES = frozenset({"queued", "running"})


def _required_nonnegative_min_amount(value: object) -> float:
    if isinstance(value, bool) or not isinstance(value, (int, float)):
        raise ValueError("flow_min_amount_invalid")
    normalized = float(value)
    if not math.isfinite(normalized) or normalized < 0:
        raise ValueError("flow_min_amount_invalid")
    return normalized


def _digest_flow_public_ref(*, case_id: str, snapshot_id: str) -> str:
    digest = hashlib.sha256()
    digest.update(_FLOW_PUBLIC_REF_DOMAIN)
    for value in (case_id, snapshot_id):
        encoded = value.encode("utf-8", errors="strict")
        digest.update(len(encoded).to_bytes(8, byteorder="big", signed=False))
        digest.update(encoded)
    return f"flowref_v1_{digest.hexdigest()}"


def project_flow_public_snapshot_ref(*, case_id: str, snapshot_ref: object) -> Optional[dict[str, Any]]:
    """Return a case-bound opaque reference without publishing graph facts."""

    normalized_case_id = str(case_id or "").strip()
    source = snapshot_ref if isinstance(snapshot_ref, dict) else {}
    snapshot_id = str(source.get("snapshot_id") or source.get("graph_hash") or "").strip()
    if not normalized_case_id or not snapshot_id:
        return None
    public_ref = _digest_flow_public_ref(case_id=normalized_case_id, snapshot_id=snapshot_id)
    return {
        "contract": "FlowPublicOpaqueSnapshotRefV1",
        "opaque_ref": public_ref,
        "content_access": "controlled_artifact_required",
    }


def project_flow_public_view(*, case_id: str, view: object) -> dict[str, Any]:
    """Project saved view metadata without graph/query/PII-bearing display content."""

    normalized_case_id = str(case_id or "").strip()
    source = view if isinstance(view, dict) else {}
    if not normalized_case_id or str(source.get("case_id") or "").strip() != normalized_case_id:
        raise FlowCaseBindingMismatchError("flow_case_binding_mismatch")
    view_id = str(source.get("view_id") or "").strip()
    try:
        if str(UUID(view_id)) != view_id:
            raise ValueError("non-canonical view id")
    except (AttributeError, TypeError, ValueError) as exc:
        raise FlowCaseBindingMismatchError("flow_public_projection_rejected") from exc
    graph_state: dict[str, Any] = {"nodes": [], "edges": []}
    view_state: dict[str, Any] = {
        "schema_version": 1,
        "runtime_view": {},
        "view_state_v2": {
            "schema_version": 2,
            "graph": dict(graph_state),
            "filters": {},
            "counts": {},
        },
        "graph": dict(graph_state),
        "filters": {},
        "counts": {},
    }
    return {
        "contract": "FlowPublicViewV1",
        "view_id": view_id,
        "case_id": normalized_case_id,
        "view_name": "Saved flow view",
        "graph_query": {},
        "view_state": view_state,
        "created_at": "",
        "updated_at": "",
        "content_access": "controlled_artifact_required",
    }


def project_flow_public_result(*, case_id: str, result: object) -> dict[str, Any]:
    """Close the ordinary HTTP boundary while retaining the private snapshot."""

    source = result if isinstance(result, dict) else {}
    source_stats = source.get("stats") if isinstance(source.get("stats"), dict) else {}
    safe_stats: dict[str, Any] = {}
    build_ms = source_stats.get("build_ms")
    if isinstance(build_ms, int) and not isinstance(build_ms, bool) and build_ms >= 0:
        safe_stats["build_ms"] = build_ms
    return {
        "contract": "FlowPublicResultBoundaryV1",
        "publication_status": "blocked",
        "fact_answer_allowed": False,
        "content_access": "controlled_artifact_required",
        "nodes": [],
        "edges": [],
        "stats": safe_stats,
        "runtime_graph": {"nodes": [], "edges": []},
        "result_snapshot_ref": project_flow_public_snapshot_ref(
            case_id=case_id,
            snapshot_ref=source.get("result_snapshot_ref"),
        ),
    }


def project_flow_public_event_payload(
    *,
    event: str,
    case_id: str,
    payload: object,
) -> dict[str, Any]:
    """Whitelist operational WS fields; graph facts never reach the socket."""

    source = payload if isinstance(payload, dict) else {}
    if event == "analysis.flow.build.progress":
        stage = str(source.get("stage") or "").strip().lower()
        progress_value = source.get("progress")
        progress = progress_value if isinstance(progress_value, int) and not isinstance(progress_value, bool) else 0
        projected: dict[str, Any] = {
            "progress": max(0, min(100, progress)),
            "stage": stage if stage in _FLOW_PUBLIC_EVENT_STAGES else "running",
        }
        return projected
    if event == "analysis.flow.build.completed":
        projected = {"status": "succeeded", "publication_status": "blocked"}
        build_ms = source.get("build_ms")
        if isinstance(build_ms, int) and not isinstance(build_ms, bool) and build_ms >= 0:
            projected["build_ms"] = build_ms
        snapshot_ref = project_flow_public_snapshot_ref(
            case_id=case_id,
            snapshot_ref=source.get("result_snapshot_ref"),
        )
        if snapshot_ref is not None:
            projected["result_snapshot_ref"] = snapshot_ref
        return projected
    if event == "analysis.flow.graph.patch":
        projected = {
            "patch_available": True,
            "publication_status": "blocked",
            "content_access": "controlled_artifact_required",
        }
        snapshot_ref = project_flow_public_snapshot_ref(
            case_id=case_id,
            snapshot_ref=source.get("result_snapshot_ref"),
        )
        if snapshot_ref is not None:
            projected["result_snapshot_ref"] = snapshot_ref
        return projected
    if event == "analysis.flow.views.patch":
        operation = str(source.get("operation") or "").strip().lower()
        if operation not in {"create", "update", "delete", "reorder"}:
            operation = "update"
        return {"operation": operation, "content_access": "controlled_artifact_required"}
    if event == "analysis.flow.build.failed":
        return {
            "code": "FLOW_JOB_FAILED",
            "message": "flow operation failed",
            "retryable": False,
            "details": {},
        }
    return {
        "publication_status": "blocked",
        "content_access": "controlled_artifact_required",
    }


class FlowService:
    """Flow domain service for graph build/view CRUD and async flow build jobs."""

    def __init__(
        self,
        *,
        repository: FlowRepository,
        task_service: Optional[TaskService] = None,
        ws_manager: Optional[WebSocketManager] = None,
        ws_sequence=None,
        ws_version: str = "v1",
    ) -> None:
        self._repository = repository
        self._task_service = task_service
        self._ws_manager = ws_manager
        self._ws_sequence = ws_sequence
        self._ws_version = ws_version

        self._lock = threading.RLock()
        self._cancel_flags: dict[str, threading.Event] = {}
        self._threads: dict[str, threading.Thread] = {}
        self._job_event_seq: dict[str, int] = {}
        self._event_dedup: dict[str, set[str]] = {}
        self._maintenance_stop = threading.Event()
        self._maintenance_thread: Optional[threading.Thread] = None
        self._maintenance_interval_s = 0
        self._maintenance_startup_delay_s = 0
        self._logger = logging.getLogger("analytix.flow.maintenance")

    def build_graph(
        self,
        *,
        case_id: str,
        seeds: Sequence[str],
        depth: int,
        direction: str,
        min_amount: float,
        request_context: Optional[dict] = None,
        case_lifecycle_binding: Optional[FlowCaseLifecycleBindingV1] = None,
    ) -> dict:
        normalized_direction = direction if direction in {"in", "out", "both"} else "both"
        normalized_depth = max(1, int(depth))
        normalized_min_amount = _required_nonnegative_min_amount(min_amount)
        normalized_context = request_context or {}
        return self._repository.build_graph(
            case_id=case_id,
            seeds=seeds,
            depth=normalized_depth,
            direction=normalized_direction,
            min_amount=normalized_min_amount,
            request_context=normalized_context,
            case_lifecycle_binding=case_lifecycle_binding,
        )

    def create_build_job(
        self,
        *,
        case_id: str,
        seeds: Sequence[str],
        depth: int,
        direction: str,
        min_amount: float,
        request_context: Optional[dict] = None,
    ) -> TaskRecord:
        task_service = self._require_task_service()
        case_lifecycle_binding = self._repository.freeze_case_lifecycle_binding(case_id)

        context_payload: dict[str, Any] = {}
        if isinstance(request_context, dict):
            for raw_key, raw_value in request_context.items():
                key = str(raw_key or "").strip()
                if not key:
                    continue
                if key in {"case_id", "seeds", "depth", "direction", "min_amount"}:
                    continue
                if raw_value is None:
                    continue
                context_payload[key] = raw_value

        request_payload = {
            "case_id": case_id,
            "case_lifecycle_binding": case_lifecycle_binding.to_dict(),
            "seeds": [str(item or "").strip() for item in seeds if str(item or "").strip()],
            "depth": max(1, int(depth)),
            "direction": direction if direction in {"in", "out", "both"} else "both",
            "min_amount": _required_nonnegative_min_amount(min_amount),
        }
        if context_payload:
            request_payload["context"] = context_payload

        metadata = {
            "request": request_payload,
            "summary": {},
            "result": None,
        }
        task = task_service.create_task(
            task_type=TaskType.FLOW_BUILD,
            case_id=case_id,
            metadata=metadata,
        )

        with self._lock:
            self._cancel_flags[task.task_id] = threading.Event()

        self._emit_event(
            job_id=task.task_id,
            case_id=case_id,
            event="analysis.flow.build.progress",
            event_type="progress",
            dedupe_key="progress:queued",
            payload={
                "progress": 0,
                "stage": "queued",
                "message": "flow build job accepted",
            },
        )

        thread = threading.Thread(
            target=self._run_build_job,
            args=(task.task_id,),
            daemon=True,
            name=f"flow-build-job-{task.task_id[:8]}",
        )
        with self._lock:
            self._threads[task.task_id] = thread
        thread.start()
        return task

    def _get_job_for_worker(self, job_id: str) -> TaskRecord:
        task_service = self._require_task_service()
        try:
            task = task_service.get_task(job_id)
        except TaskNotFoundError as exc:
            raise FlowJobNotFoundError(job_id) from exc
        if task.task_type != TaskType.FLOW_BUILD:
            raise FlowJobNotFoundError(job_id)
        return task

    def get_job(self, job_id: str, *, case_id: str) -> TaskRecord:
        task_service = self._require_task_service()
        try:
            task = task_service.get_public_task(job_id, case_id=case_id)
        except (TaskNotFoundError, ValueError) as exc:
            raise FlowJobNotFoundError(job_id) from exc
        if task.task_type != TaskType.FLOW_BUILD:
            raise FlowJobNotFoundError(job_id)
        return task

    def cancel_job(self, job_id: str, *, case_id: str) -> TaskRecord:
        task = self.get_job(job_id, case_id=case_id)
        if task.status in {TaskStatus.SUCCEEDED, TaskStatus.FAILED, TaskStatus.CANCELED}:
            raise FlowJobNotCancellableError(job_id)

        with self._lock:
            self._cancel_flags.setdefault(job_id, threading.Event()).set()

        if task.status == TaskStatus.QUEUED:
            updated = self._safe_transition(
                job_id,
                to_status=TaskStatus.CANCELED,
                progress=task.progress,
                error="canceled by user",
            )
            if updated is not None:
                self._emit_event(
                    job_id=job_id,
                    case_id=updated.case_id,
                    event="analysis.flow.build.failed",
                    event_type="error",
                    dedupe_key="failed:canceled",
                    payload={
                        "code": "JOB_CANCELED",
                        "message": "flow build job canceled before execution",
                        "retryable": False,
                        "details": {},
                    },
                )
                self._cleanup_runtime(job_id)
                return updated

        return self.get_job(job_id, case_id=case_id)

    def get_job_result(self, job_id: str, *, case_id: str) -> dict:
        task = self.get_job(job_id, case_id=case_id)
        if task.status != TaskStatus.SUCCEEDED:
            raise FlowJobNotReadyError(job_id)
        manifest = self._repository.get_job_result_manifest(
            case_id=task.case_id or "",
            job_id=job_id,
        )
        if not isinstance(manifest, dict):
            raise FlowJobNotReadyError(job_id)
        snapshot_ref = manifest.get("result_snapshot_ref")
        if not isinstance(snapshot_ref, dict):
            raise FlowJobNotReadyError(job_id)
        hydrated = self._repository.get_result_snapshot(
            case_id=task.case_id or "",
            snapshot_ref=snapshot_ref,
        )
        runtime_graph = hydrated.get("runtime_graph") if isinstance(hydrated.get("runtime_graph"), dict) else {}
        if not runtime_graph.get("nodes") and not runtime_graph.get("edges"):
            raise FlowJobNotReadyError(job_id)
        return project_flow_public_result(case_id=case_id, result=hydrated)

    def job_result_available(self, job_id: str, *, case_id: str) -> bool:
        task = self.get_job(job_id, case_id=case_id)
        return task.status == TaskStatus.SUCCEEDED and self._repository.has_job_result(
            case_id=task.case_id or "",
            job_id=job_id,
        )

    def create_view(self, *, case_id: str) -> dict:
        item = self._repository.create_view(case_id=case_id)
        self._emit_views_patch(
            case_id=case_id,
            operation="create",
            payload={
                "view": item,
            },
            dedupe_key=f"views:create:{item.get('view_id')}",
        )
        return item

    def list_views(self, *, case_id: str, page: int, page_size: int) -> Tuple[list[dict], int]:
        return self._repository.list_views(case_id=case_id, page=page, page_size=page_size)

    def get_view(self, view_id: str, *, case_id: str) -> dict:
        try:
            return self._repository.get_view(case_id=case_id, view_id=view_id)
        except FlowViewNotFoundError as exc:
            raise FlowViewNotFound(view_id) from exc

    def update_view(self, *, case_id: str, view_id: str) -> dict:
        try:
            item = self._repository.update_view(case_id=case_id, view_id=view_id)
            self._emit_views_patch(
                case_id=item.get("case_id"),
                operation="update",
                payload={
                    "view": item,
                },
                dedupe_key=f"views:update:{item.get('view_id')}",
            )
            return item
        except FlowViewNotFoundError as exc:
            raise FlowViewNotFound(view_id) from exc

    def delete_view(self, view_id: str, *, case_id: str) -> None:
        try:
            item = self._repository.get_view(case_id=case_id, view_id=view_id)
            self._repository.delete_view(case_id=case_id, view_id=view_id)
            self._emit_views_patch(
                case_id=item.get("case_id"),
                operation="delete",
                payload={
                    "view_id": item.get("view_id") or view_id,
                },
                dedupe_key=f"views:delete:{item.get('view_id') or view_id}",
            )
        except FlowViewNotFoundError as exc:
            raise FlowViewNotFound(view_id) from exc

    def reorder_views(self, *, case_id: str, order: Sequence[str]) -> dict:
        result = self._repository.reorder_views(case_id=case_id, order=order)
        self._emit_views_patch(
            case_id=case_id,
            operation="reorder",
            payload={
                "order": [str(item or "").strip() for item in order if str(item or "").strip()],
                "count": result.get("count") if isinstance(result, dict) else 0,
            },
            dedupe_key=f"views:reorder:{case_id}:{','.join(str(item or '').strip() for item in order if str(item or '').strip())}",
        )
        return result

    def get_result_snapshot(self, *, case_id: str, snapshot_ref: dict[str, Any]) -> dict:
        return self._repository.get_result_snapshot(case_id=case_id, snapshot_ref=snapshot_ref)

    def persist_result_snapshot(
        self,
        *,
        case_id: str,
        result: dict[str, Any],
        base_snapshot_ref: Optional[dict[str, Any]] = None,
    ) -> Optional[dict[str, Any]]:
        snapshot_ref = self._repository.persist_result_snapshot(
            case_id=case_id,
            result=result,
            base_snapshot_ref=base_snapshot_ref,
        )
        return snapshot_ref

    def get_result_snapshot_patch(
        self,
        *,
        case_id: str,
        base_snapshot_ref: dict[str, Any],
        target_snapshot_ref: dict[str, Any],
    ) -> dict:
        return self._repository.get_result_snapshot_patch(
            case_id=case_id,
            base_snapshot_ref=base_snapshot_ref,
            target_snapshot_ref=target_snapshot_ref,
        )

    def expand_result_snapshot(
        self,
        *,
        case_id: str,
        base_snapshot_ref: dict[str, Any],
        expand_request: Optional[dict[str, Any]] = None,
    ) -> dict:
        return self._repository.expand_result_snapshot(
            case_id=case_id,
            base_snapshot_ref=base_snapshot_ref,
            expand_request=expand_request or {},
        )

    def update_result_snapshot_projection_layout(
        self,
        *,
        case_id: str,
        snapshot_ref: dict[str, Any],
        layout_index: Optional[dict[str, Any]] = None,
    ) -> dict:
        return self._repository.update_result_snapshot_projection_layout(
            case_id=case_id,
            snapshot_ref=snapshot_ref,
            layout_index=layout_index or {},
        )

    def _resolve_network_layout_graph_source(
        self,
        *,
        case_id: str,
        nodes: Sequence[dict[str, Any]],
        edges: Optional[Sequence[dict[str, Any]]] = None,
        result_snapshot_ref: Optional[dict[str, Any]] = None,
    ) -> Tuple[list[dict[str, Any]], list[dict[str, Any]], str, dict[str, Any]]:
        node_rows = list(nodes or [])
        edge_rows = list(edges or [])
        source = "inline"
        result_ref = (
            result_snapshot_ref
            if isinstance(result_snapshot_ref, dict) and result_snapshot_ref
            else None
        )
        source_snapshot_ref: dict[str, Any] = {}

        def _snapshot_runtime_graph(snapshot: dict[str, Any]) -> tuple[list[dict[str, Any]], list[dict[str, Any]]]:
            runtime_graph = snapshot.get("runtime_graph") if isinstance(snapshot.get("runtime_graph"), dict) else {}
            graph_nodes = runtime_graph.get("nodes") if isinstance(runtime_graph, dict) else None
            graph_edges = runtime_graph.get("edges") if isinstance(runtime_graph, dict) else None
            if isinstance(graph_nodes, list) and isinstance(graph_edges, list) and graph_nodes:
                return list(graph_nodes), list(graph_edges)
            snapshot_nodes = snapshot.get("nodes")
            snapshot_edges = snapshot.get("edges")
            if isinstance(snapshot_nodes, list) and isinstance(snapshot_edges, list):
                return list(snapshot_nodes), list(snapshot_edges)
            return [], []

        def _snapshot_ref(value: Any) -> dict[str, Any]:
            if not isinstance(value, dict):
                return {}
            snapshot_id = str(
                value.get("snapshot_id")
                or value.get("snapshotId")
                or value.get("graph_hash")
                or value.get("graphHash")
                or ""
            ).strip()
            if not snapshot_id:
                return {}
            return {
                "snapshot_id": snapshot_id,
                "graph_hash": str(value.get("graph_hash") or value.get("graphHash") or snapshot_id).strip() or snapshot_id,
            }

        if (not node_rows or not edge_rows) and result_ref:
            snapshot = self._repository.get_result_snapshot(
                case_id=str(case_id or "").strip(),
                snapshot_ref=result_ref,
            )
            graph_nodes, graph_edges = _snapshot_runtime_graph(snapshot)
            projection = snapshot.get("projection") if isinstance(snapshot.get("projection"), dict) else {}
            source_ref = _snapshot_ref(projection.get("source_result_snapshot_ref") or projection.get("sourceResultSnapshotRef"))
            if source_ref:
                source_snapshot = self._repository.get_result_snapshot(
                    case_id=str(case_id or "").strip(),
                    snapshot_ref=source_ref,
                )
                source_nodes, source_edges = _snapshot_runtime_graph(source_snapshot)
                if source_nodes and len(source_nodes) > len(graph_nodes):
                    graph_nodes = source_nodes
                    graph_edges = source_edges
                    source = "snapshot_source"
                    source_snapshot_ref = source_ref
            if graph_nodes:
                node_rows = graph_nodes
                edge_rows = graph_edges
                if source != "snapshot_source":
                    source = "snapshot"
                    source_snapshot_ref = result_ref
        return node_rows, edge_rows, source, source_snapshot_ref

    def compute_layout_node_plan(
        self,
        *,
        case_id: str = "",
        operation: str,
        nodes: Sequence[dict[str, Any]],
        edges: Optional[Sequence[dict[str, Any]]] = None,
        nodes_by_id: Optional[dict[str, Any]] = None,
        worker_nodes: Optional[Sequence[dict[str, Any]]] = None,
        meta_keys: Optional[Sequence[str]] = None,
        algo_version: str = "",
        mode: str = "",
        focus_id: str = "",
        graph_mutation_seq: int = 0,
        layout_direction: Optional[dict[str, Any]] = None,
        seed_context: Optional[dict[str, Any]] = None,
        semantic: Optional[dict[str, Any]] = None,
        result_snapshot_ref: Optional[dict[str, Any]] = None,
    ) -> dict[str, Any]:
        normalized_operation = str(operation or "").strip().lower()
        if normalized_operation == "project":
            result = project_layout_node_plan_result(nodes=nodes, meta_keys=meta_keys)
            return {
                "operation": "project",
                "node_plan": result["node_plan"],
                "updates": result["updates"],
            }
        if normalized_operation == "cache_key":
            return {
                "operation": "cache_key",
                "cacheKey": project_layout_cache_key(
                    nodes=nodes,
                    edges=edges or [],
                    algo_version=algo_version,
                    mode=mode,
                    focus_id=focus_id,
                    graph_mutation_seq=graph_mutation_seq,
                    layout_direction=layout_direction,
                    seed_context=seed_context,
                ),
            }
        if normalized_operation == "apply":
            result = apply_layout_node_plan(
                nodes=nodes,
                nodes_by_id=nodes_by_id if isinstance(nodes_by_id, dict) else {},
                meta_keys=meta_keys,
            )
            return {
                "operation": "apply",
                "applied": bool(result.get("applied")),
                "nodes": result.get("nodes") if isinstance(result.get("nodes"), list) else [],
            }
        if normalized_operation == "apply_worker":
            result = apply_layout_worker_node_plan(
                nodes=nodes,
                worker_nodes=worker_nodes or [],
                meta_keys=meta_keys,
            )
            return {
                "operation": "apply_worker",
                "applied": bool(result.get("applied")),
                "source": str(result.get("source") or ""),
                "matched": int(result.get("matched") or 0),
                "gridFallbackLikely": bool(result.get("gridFallbackLikely")),
                "updates": result.get("updates") if isinstance(result.get("updates"), list) else [],
                "nodes": result.get("nodes") if isinstance(result.get("nodes"), list) else [],
            }
        if normalized_operation == "clear":
            return {
                "operation": "clear",
                "nodes": clear_layout_node_meta(nodes=nodes, meta_keys=meta_keys),
            }
        if normalized_operation in {"network_plan", "network_community_quality"}:
            result_ref = (
                result_snapshot_ref
                if isinstance(result_snapshot_ref, dict) and result_snapshot_ref
                else None
            )
            has_inline_graph = bool(nodes) and bool(edges)
            if result_ref and not has_inline_graph:
                source_ref = self._repository.get_result_snapshot_source_ref(
                    case_id=case_id,
                    snapshot_ref=result_ref,
                )
                source_path = (
                    self._repository.get_result_snapshot_compute_path(
                        case_id=case_id,
                        snapshot_ref=source_ref,
                    )
                    if source_ref
                    else ""
                )
                if source_path:
                    source_snapshot_id = str(
                        source_ref.get("snapshot_id") or source_ref.get("graph_hash") or ""
                    ).strip()
                    result_snapshot_id = str(
                        result_ref.get("snapshot_id") or result_ref.get("graph_hash") or ""
                    ).strip()
                    source = (
                        "snapshot_source"
                        if source_snapshot_id and result_snapshot_id and source_snapshot_id != result_snapshot_id
                        else "snapshot"
                    )
                    network_payload = {
                        "nodes": [],
                        "edges": [],
                        "mode": "network",
                        "focusId": focus_id,
                        "layoutDirection": layout_direction or {},
                        "seedContext": seed_context or {},
                        "semantic": semantic if isinstance(semantic, dict) else {},
                        "source": {
                            "kind": source,
                            "resultSnapshotRef": source_ref or result_ref,
                            "resultSnapshotPath": source_path,
                        },
                    }
                    if normalized_operation == "network_plan":
                        plan = project_flow_layout_network_plan(payload=network_payload)
                        report = plan.get("report") if isinstance(plan.get("report"), dict) else {}
                        if int(report.get("nodeCount") or 0) > 0:
                            return {
                                "operation": "network_plan",
                                "source": source,
                                "networkPlan": plan,
                            }
                    else:
                        quality = project_flow_layout_network_community_quality(payload=network_payload)
                        report = quality.get("report") if isinstance(quality.get("report"), dict) else {}
                        if int(report.get("nodeCount") or 0) > 0:
                            return {
                                "operation": "network_community_quality",
                                "source": source,
                                "mode": str(quality.get("mode") or ""),
                                "quality": quality.get("quality") if isinstance(quality.get("quality"), dict) else None,
                                "communityQuality": (
                                    quality.get("communityQuality")
                                    if isinstance(quality.get("communityQuality"), list)
                                    else []
                                ),
                                "report": report,
                            }
        if normalized_operation in {"network_plan", "network_community_quality"}:
            node_rows, edge_rows, source, source_snapshot_ref = self._resolve_network_layout_graph_source(
                case_id=case_id,
                nodes=nodes,
                edges=edges,
                result_snapshot_ref=result_snapshot_ref,
            )
            network_payload = {
                "nodes": node_rows,
                "edges": edge_rows,
                "mode": "network",
                "focusId": focus_id,
                "layoutDirection": layout_direction or {},
                "seedContext": seed_context or {},
                "semantic": semantic if isinstance(semantic, dict) else {},
                "source": {
                    "kind": source,
                    "resultSnapshotRef": source_snapshot_ref,
                },
            }
            if normalized_operation == "network_community_quality":
                quality = project_flow_layout_network_community_quality(payload=network_payload)
                return {
                    "operation": "network_community_quality",
                    "source": source,
                    "mode": str(quality.get("mode") or ""),
                    "quality": quality.get("quality") if isinstance(quality.get("quality"), dict) else None,
                    "communityQuality": (
                        quality.get("communityQuality")
                        if isinstance(quality.get("communityQuality"), list)
                        else []
                    ),
                    "report": quality.get("report") if isinstance(quality.get("report"), dict) else {},
                }
            return {
                "operation": "network_plan",
                "source": source,
                "networkPlan": project_flow_layout_network_plan(payload=network_payload),
            }
        raise ValueError("unsupported layout node plan operation")

    def project_graph_render_plan(
        self,
        *,
        render_plans: Optional[Sequence[dict[str, Any]]] = None,
        render_plan: Optional[dict[str, Any]] = None,
        viewport_expand_targets: Optional[dict[str, Any]] = None,
    ) -> dict[str, Any]:
        result = project_flow_graph_render_plan(
            render_plans=render_plans,
            render_plan=render_plan,
            viewport_expand_targets=viewport_expand_targets,
        )
        response = {
            "renderResults": result.get("renderResults")
            if isinstance(result.get("renderResults"), list)
            else []
        }
        if isinstance(result.get("viewportExpandTargets"), dict):
            response["viewportExpandTargets"] = result["viewportExpandTargets"]
        return response

    def project_network_sector_placement(
        self,
        *,
        payload: dict[str, Any],
    ) -> dict[str, Any]:
        try:
            sector_placement = project_flow_layout_network_sector_placement(payload=payload)
        except AnalysisComputeUnavailableError:
            return {
                "sectorPlacement": {
                    "optionalAdapterMissing": True,
                    "report": {
                        "sectorFastPath": "network-general-sector-placement-js-fallback",
                        "generalPlacementFallback": True,
                        "optionalAdapterMissing": True,
                        "error": "analysis_compute_unavailable",
                    },
                }
            }
        return {"sectorPlacement": sector_placement}

    def project_layout_role_graph(
        self,
        *,
        nodes: Sequence[dict[str, Any]],
        edges: Sequence[dict[str, Any]],
        traversal: Optional[dict[str, Any]] = None,
        demotion: Optional[dict[str, Any]] = None,
        promotion: Optional[dict[str, Any]] = None,
        role_resolution: Optional[dict[str, Any]] = None,
        cluster_resolution: Optional[dict[str, Any]] = None,
        semantic_pipeline: Optional[dict[str, Any]] = None,
    ) -> dict[str, Any]:
        return project_layout_role_graph(
            nodes=nodes,
            edges=edges,
            traversal=traversal if isinstance(traversal, dict) else None,
            demotion=demotion if isinstance(demotion, dict) else None,
            promotion=promotion if isinstance(promotion, dict) else None,
            role_resolution=role_resolution if isinstance(role_resolution, dict) else None,
            cluster_resolution=cluster_resolution if isinstance(cluster_resolution, dict) else None,
            semantic_pipeline=semantic_pipeline if isinstance(semantic_pipeline, dict) else None,
        )

    def project_analysis_graph_data(
        self,
        *,
        source: dict[str, Any],
        filter_min: Optional[float] = None,
        filter_max: Optional[float] = None,
        collapse_children: bool = False,
    ) -> dict[str, Any]:
        return project_analysis_graph_data(
            source=source if isinstance(source, dict) else {},
            filter_min=filter_min,
            filter_max=filter_max,
            collapse_children=collapse_children,
        )

    def merge_flow_same_name_graph(
        self,
        *,
        nodes: list[dict[str, Any]],
        edges: list[dict[str, Any]],
        mode: str = "gross",
    ) -> dict[str, Any]:
        return merge_flow_same_name_graph(
            nodes=nodes or [],
            edges=edges or [],
            mode=mode,
        )

    def project_flow_graph_search(
        self,
        *,
        nodes: Sequence[dict[str, Any]],
        query: str,
        limit: Optional[int] = 1,
    ) -> dict[str, Any]:
        return project_flow_graph_search(
            nodes=list(nodes or []),
            query=query,
            limit=limit,
        )

    def project_flow_projection_layout_seed(
        self,
        *,
        nodes: Sequence[dict[str, Any]],
        base_nodes: Sequence[dict[str, Any]],
    ) -> dict[str, Any]:
        return project_flow_projection_layout_seed(
            nodes=list(nodes or []),
            base_nodes=list(base_nodes or []),
        )

    def project_flow_graph_data_merge(
        self,
        *,
        base_graph_data: Optional[dict[str, Any]] = None,
        patch_payload: Optional[dict[str, Any]] = None,
        use_base_graph: bool = True,
    ) -> dict[str, Any]:
        return project_flow_graph_data_merge(
            base_graph_data=base_graph_data if isinstance(base_graph_data, dict) else {},
            patch_payload=patch_payload if isinstance(patch_payload, dict) else {},
            use_base_graph=use_base_graph,
        )

    def get_result_snapshot_metrics(self, *, case_id: Optional[str] = None) -> dict:
        return self._repository.collect_result_snapshot_metrics(case_id=case_id)

    def run_result_snapshot_gc(self, *, case_id: Optional[str] = None) -> dict:
        return self._repository.gc_result_snapshots(case_id=case_id)

    def start_background_maintenance(self, *, interval_s: int = 1800, startup_delay_s: int = 180) -> None:
        normalized_interval = max(0, int(interval_s or 0))
        if normalized_interval <= 0:
            return
        with self._lock:
            thread = self._maintenance_thread
            if thread is not None and thread.is_alive():
                return
            self._maintenance_stop.clear()
            self._maintenance_interval_s = normalized_interval
            self._maintenance_startup_delay_s = max(0, int(startup_delay_s or 0))
            self._maintenance_thread = threading.Thread(
                target=self._run_background_maintenance,
                name="analytix-flow-maintenance",
                daemon=True,
            )
            self._maintenance_thread.start()

    def shutdown(self) -> None:
        self._maintenance_stop.set()
        with self._lock:
            thread = self._maintenance_thread
            self._maintenance_thread = None
        if thread is not None and thread.is_alive():
            thread.join(timeout=2.5)

    def _run_background_maintenance(self) -> None:
        if self._maintenance_startup_delay_s > 0 and self._maintenance_stop.wait(self._maintenance_startup_delay_s):
            return
        while not self._maintenance_stop.is_set():
            try:
                summary = self._repository.gc_result_snapshots()
                deleted_count = int(summary.get("deleted_count") or 0) if isinstance(summary, dict) else 0
                if deleted_count > 0:
                    log_closed_diagnostic(
                        self._logger,
                        logging.INFO,
                        topic="result_snapshot_gc",
                        code="completed",
                        numeric={
                            "deleted_count": deleted_count,
                            "deleted_bytes": int(summary.get("deleted_bytes") or 0),
                            "case_count": int(summary.get("case_count") or 0),
                        },
                    )
            except Exception:
                log_closed_diagnostic(
                    self._logger,
                    logging.WARNING,
                    topic="result_snapshot_gc",
                    code="gc_failed",
                )
            if self._maintenance_stop.wait(max(60, int(self._maintenance_interval_s or 1800))):
                return

    def _run_build_job(self, job_id: str) -> None:
        task = self._get_job_for_worker(job_id)
        case_id = task.case_id or ""

        if self._is_cancel_requested(job_id):
            self._finalize_canceled(task)
            return

        started = self._safe_transition(
            job_id,
            to_status=TaskStatus.RUNNING,
            progress=1,
            metadata={"started_at": utc_now().isoformat()},
        )
        if started is None:
            return
        runtime_payload = self._require_task_service().read_runtime_payload(
            job_id,
            case_id=started.case_id,
        )
        request = runtime_payload.get("request") if isinstance(runtime_payload, dict) else {}
        request_obj = request if isinstance(request, dict) else {}
        self._emit_event(
            job_id=job_id,
            case_id=case_id,
            event="analysis.flow.build.progress",
            event_type="progress",
            dedupe_key="progress:running",
            payload={
                "progress": 1,
                "stage": "running",
                "message": "flow graph building",
            },
        )

        begin = time.perf_counter()
        try:
            case_lifecycle_binding = self._repository.require_case_lifecycle_binding_current(
                request_obj.get("case_lifecycle_binding"),
                case_id=case_id,
            )
            try:
                from app.core.analysis_compute_worker import shutdown_stats_query_worker

                shutdown_stats_query_worker()
            except Exception:
                pass
            graph = self.build_graph(
                case_id=case_id,
                seeds=request_obj.get("seeds") or [],
                depth=int(request_obj.get("depth") or 1),
                direction=str(request_obj.get("direction") or "both"),
                min_amount=_required_nonnegative_min_amount(request_obj.get("min_amount")),
                request_context=request_obj.get("context") if isinstance(request_obj.get("context"), dict) else {},
                case_lifecycle_binding=case_lifecycle_binding,
            )

            if self._is_cancel_requested(job_id):
                self._finalize_canceled(self._get_job_for_worker(job_id))
                return

            nodes = list(graph.get("nodes") or [])
            edges = list(graph.get("edges") or [])
            stats = dict(graph.get("stats") or {})
            runtime_graph = graph.get("runtime_graph") if isinstance(graph.get("runtime_graph"), dict) else {"nodes": [], "edges": []}
            build_ms = int(round((time.perf_counter() - begin) * 1000))
            summary = {
                "node_count": len(nodes),
                "edge_count": len(edges),
                "build_ms": build_ms,
            }

            result_payload = {
                "nodes": nodes,
                "edges": edges,
                "stats": stats,
                "runtime_graph": runtime_graph,
                "publication_status": graph.get("publication_status"),
                "fact_answer_allowed": graph.get("fact_answer_allowed"),
                "graph_tier": graph.get("graph_tier"),
                "render_hints": graph.get("render_hints"),
                "projection": graph.get("projection"),
            }
            result_snapshot_ref = self._repository.persist_result_snapshot(
                case_id=case_id,
                result=result_payload,
                case_lifecycle_binding=case_lifecycle_binding,
            )
            if not isinstance(result_snapshot_ref, dict):
                raise FlowJobNotReadyError("flow_result_snapshot_unavailable")
            self._repository.persist_job_result_manifest(
                case_id=case_id,
                job_id=job_id,
                snapshot_ref=result_snapshot_ref,
                case_lifecycle_binding=case_lifecycle_binding,
            )

            self._repository.require_case_lifecycle_binding_current(
                case_lifecycle_binding,
                case_id=case_id,
            )

            with self._repository.case_lifecycle_publication_lease(
                case_lifecycle_binding,
                case_id=case_id,
            ):
                updated = self._safe_transition(
                    job_id,
                    to_status=TaskStatus.SUCCEEDED,
                    progress=100,
                    metadata={
                        "summary": summary,
                        "finished_at": utc_now().isoformat(),
                    },
                )
                if updated is not None:
                    self._emit_event(
                        job_id=job_id,
                        case_id=case_id,
                        event="analysis.flow.build.completed",
                        event_type="success",
                        dedupe_key="completed",
                        payload={
                            "node_count": len(nodes),
                            "edge_count": len(edges),
                            "build_ms": build_ms,
                            "result_snapshot_ref": result_snapshot_ref,
                        },
                    )
        except Exception:
            updated = self._safe_transition(
                job_id,
                to_status=TaskStatus.FAILED,
                progress=100,
                error="flow_build_failed",
            )
            if updated is not None:
                self._emit_event(
                    job_id=job_id,
                    case_id=case_id,
                    event="analysis.flow.build.failed",
                    event_type="error",
                    dedupe_key="failed",
                    payload=closed_task_failure_event_payload(
                        service="flow",
                        code="INTERNAL_ERROR",
                        retryable=False,
                        correlation_id=updated.metadata.get("failure_correlation_id"),
                    ),
                )
        finally:
            self._cleanup_runtime(job_id)

    def _finalize_canceled(self, task: TaskRecord) -> None:
        updated = self._safe_transition(
            task.task_id,
            to_status=TaskStatus.CANCELED,
            progress=max(task.progress, 1),
            error="flow_build_canceled",
        )
        if updated is None:
            return
        self._emit_event(
            job_id=task.task_id,
            case_id=task.case_id,
            event="analysis.flow.build.failed",
            event_type="error",
            dedupe_key="failed:canceled",
            payload=closed_task_failure_event_payload(
                service="flow",
                code="JOB_CANCELED",
                retryable=False,
                correlation_id=updated.metadata.get("failure_correlation_id"),
            ),
        )

    def _safe_transition(
        self,
        job_id: str,
        *,
        to_status: TaskStatus,
        progress: Optional[int] = None,
        error: Optional[str] = None,
        metadata: Optional[dict] = None,
    ) -> Optional[TaskRecord]:
        task_service = self._task_service
        if task_service is None:
            return None
        try:
            return task_service.transition_task(
                task_id=job_id,
                to_status=to_status,
                progress=progress,
                error=error,
                metadata=metadata,
            )
        except (TaskNotFoundError, InvalidTaskTransitionError):
            return None

    def _is_cancel_requested(self, job_id: str) -> bool:
        with self._lock:
            flag = self._cancel_flags.get(job_id)
        return bool(flag and flag.is_set())

    def _cleanup_runtime(self, job_id: str) -> None:
        with self._lock:
            self._threads.pop(job_id, None)
            self._cancel_flags.pop(job_id, None)
            self._job_event_seq.pop(job_id, None)
            self._event_dedup.pop(job_id, None)

    def _emit_event(
        self,
        *,
        job_id: str,
        case_id: Optional[str],
        event: str,
        event_type: str,
        payload: dict[str, Any],
        dedupe_key: Optional[str],
    ) -> None:
        ws_manager = self._ws_manager
        sequence = self._ws_sequence
        if ws_manager is None or sequence is None:
            return

        with self._lock:
            if dedupe_key:
                emitted = self._event_dedup.setdefault(job_id, set())
                if dedupe_key in emitted:
                    return
                emitted.add(dedupe_key)
            local_seq = self._job_event_seq.get(job_id, 0) + 1
            self._job_event_seq[job_id] = local_seq

        payload_obj = project_flow_public_event_payload(
            event=event,
            case_id=str(case_id or "").strip(),
            payload=payload,
        )
        payload_obj.setdefault("event_id", f"{job_id}:{local_seq}")

        envelope = build_event(
            event=event,
            event_type=event_type,
            channel="analysis",
            sequence=next(sequence),
            version=self._ws_version,
            job_id=job_id,
            case_id=case_id,
            payload=payload_obj,
        )
        ws_manager.publish_threadsafe(envelope)

    def _emit_views_patch(
        self,
        *,
        case_id: Optional[str],
        operation: str,
        payload: dict[str, Any],
        dedupe_key: Optional[str],
    ) -> None:
        normalized_case_id = str(case_id or "").strip()
        job_id = f"flow-views:{normalized_case_id or 'global'}"
        self._emit_event(
            job_id=job_id,
            case_id=normalized_case_id or None,
            event="analysis.flow.views.patch",
            event_type="info",
            dedupe_key=dedupe_key,
            payload={
                "operation": str(operation or "").strip() or "update",
                **dict(payload or {}),
            },
        )

    def _require_task_service(self) -> TaskService:
        if self._task_service is None:
            raise RuntimeError("flow task service not configured")
        return self._task_service
