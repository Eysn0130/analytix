import hashlib
import json
from collections.abc import Mapping, Sequence
from contextlib import contextmanager
from copy import deepcopy
from datetime import timedelta
from pathlib import Path
from threading import RLock
from time import monotonic
from typing import Dict, Iterator, List, Optional, Tuple
from uuid import uuid4

from app.core.diagnostic_file_transaction import (
    DiagnosticFileTransactionLease,
    DiagnosticFileTransactionError,
    diagnostic_file_transaction_lease,
    replace_diagnostic_file_transactionally,
)
from app.tasks.failure_boundary import (
    canonical_task_case_id,
    canonical_task_id,
    existing_failure_correlation_id,
    migrated_failure_correlation_id,
    public_task_metadata,
    stable_failure_code,
    stable_failure_phase,
)
from app.tasks.models import TaskRecord, TaskStatus, TaskType
from app.tasks.state_machine import TaskStateMachine
from app.utils.time import utc_now


class TaskNotFoundError(KeyError):
    pass


class TaskRuntimePayloadUnavailableError(RuntimeError):
    pass


class TaskCaseBindingMismatchError(PermissionError):
    pass


class TaskService:
    _TERMINAL_STATUSES = {TaskStatus.SUCCEEDED, TaskStatus.FAILED, TaskStatus.CANCELED}
    _QUARANTINED_TASK_TYPES = frozenset({TaskType.STATS_QUERY})
    _QUARANTINED_TASK_FAILURE_CODE = "task_runtime_payload_unavailable"
    # Queue-driven maintenance jobs must remain queryable after completion so
    # worker run-next endpoints and task polling can observe the terminal state.
    _EPHEMERAL_TASK_TYPES: set[TaskType] = set()
    _PERSIST_MAX_BYTES = 64 * 1024 * 1024

    def __init__(
        self,
        *,
        persist_path: Optional[Path] = None,
        max_records: int = 5000,
        terminal_ttl_days: int = 30,
        running_persist_interval_seconds: float = 0.75,
    ) -> None:
        self._tasks: Dict[str, TaskRecord] = {}
        # Runtime payloads may contain authorized paths, queries, account keys,
        # or result details. They are deliberately not TaskRecord fields and
        # are never serialized into the plaintext task store.
        self._runtime_payloads: Dict[str, tuple[Optional[str], dict]] = {}
        self._lock = RLock()
        self._persist_path = Path(persist_path) if persist_path else None
        self._max_records = max(0, int(max_records or 0))
        self._terminal_ttl_days = max(0, int(terminal_ttl_days or 0))
        self._running_persist_interval_seconds = max(0.0, float(running_persist_interval_seconds or 0.0))
        self._persist_mtime_ns: Optional[int] = None
        self._persist_size: int = -1
        self._persist_generation_sha256: Optional[str] = None
        self._persist_generation_loaded = False
        self._last_persist_monotonic: float = 0.0
        self._pending_persist: bool = False
        self._load_persisted()

    def _load_persisted(self) -> None:
        with self._lock:
            self._refresh_from_persisted_locked(mark_inflight_failed=True, force=True)

    @contextmanager
    def _task_store_lease_locked(self) -> Iterator[DiagnosticFileTransactionLease | None]:
        if self._persist_path is None:
            yield None
            return
        self._persist_path.parent.mkdir(parents=True, exist_ok=True)
        with diagnostic_file_transaction_lease(
            self._persist_path,
            surface="task_store",
            max_bytes=self._PERSIST_MAX_BYTES,
        ) as lease:
            yield lease

    @contextmanager
    def _mutating_store_locked(self) -> Iterator[DiagnosticFileTransactionLease | None]:
        with self._task_store_lease_locked() as lease:
            if lease is not None:
                self._refresh_from_persisted_locked(
                    mark_inflight_failed=False,
                    force=True,
                    lease=lease,
                )
            tasks_before = deepcopy(self._tasks)
            runtime_payloads_before = deepcopy(self._runtime_payloads)
            try:
                yield lease
            except BaseException:
                self._tasks = tasks_before
                self._runtime_payloads = runtime_payloads_before
                raise

    def _refresh_from_persisted_locked(
        self,
        *,
        mark_inflight_failed: bool,
        force: bool = False,
        lease: DiagnosticFileTransactionLease | None = None,
    ) -> None:
        if self._persist_path is None:
            return
        if lease is None:
            with self._task_store_lease_locked() as acquired:
                self._refresh_from_persisted_locked(
                    mark_inflight_failed=mark_inflight_failed,
                    force=force,
                    lease=acquired,
                )
            return
        persisted_bytes = lease.read()
        if persisted_bytes is None:
            self._persist_mtime_ns = None
            self._persist_size = -1
            self._persist_generation_sha256 = None
            self._persist_generation_loaded = True
            self._tasks = {}
            self._runtime_payloads = {}
            return
        current_digest = hashlib.sha256(persisted_bytes).hexdigest()
        if (
            not force
            and self._persist_generation_loaded
            and self._persist_generation_sha256 == current_digest
        ):
            return
        try:
            payload = json.loads(
                persisted_bytes.decode("utf-8"),
                object_pairs_hook=_strict_json_object,
            )
        except (UnicodeError, json.JSONDecodeError, ValueError):
            raise DiagnosticFileTransactionError("diagnostic_migration_input_invalid") from None
        if not isinstance(payload, dict):
            raise DiagnosticFileTransactionError("diagnostic_migration_input_invalid")
        rows = payload.get("tasks")
        if not isinstance(rows, list):
            raise DiagnosticFileTransactionError("diagnostic_migration_input_invalid")
        store_version = payload.get("version")
        if type(store_version) is not int or store_version not in {1, 2}:
            raise DiagnosticFileTransactionError("diagnostic_migration_input_invalid")
        legacy_payload = store_version == 1
        now = utc_now()
        loaded: Dict[str, TaskRecord] = {}
        for item in rows:
            try:
                task = self._load_public_task_record(item, legacy_payload=legacy_payload)
            except Exception:
                raise DiagnosticFileTransactionError("diagnostic_migration_input_invalid") from None
            if task.task_type in self._QUARANTINED_TASK_TYPES:
                self._quarantine_task_record(task, updated_at=now)
            elif mark_inflight_failed and task.status == TaskStatus.RUNNING:
                # Running work cannot resume without its process-local payload.
                task.status = TaskStatus.FAILED
                task.error = "task_interrupted"
                task.progress = max(int(task.progress or 0), 1)
                task.updated_at = now
                task.metadata = self._terminal_failure_metadata(
                    task,
                    code="task_interrupted",
                    phase="interrupted",
                    runtime_payload_discarded=True,
                )
            elif (
                task.status == TaskStatus.QUEUED
                and bool(task.metadata.get("runtime_payload_required"))
                and not self._has_runtime_payload(task)
            ):
                # Plaintext persistence intentionally excludes executable job
                # payloads. A different process or a restart must fail closed
                # instead of running a request without its same-case payload.
                task.status = TaskStatus.FAILED
                task.error = "task_runtime_payload_unavailable"
                task.progress = max(int(task.progress or 0), 1)
                task.updated_at = now
                task.metadata = self._terminal_failure_metadata(
                    task,
                    code="task_runtime_payload_unavailable",
                    phase="failed",
                    runtime_payload_discarded=True,
                )
            elif task.status in self._TERMINAL_STATUSES:
                # Process-private execution payloads are never terminal result
                # storage. Migrate every legacy terminal row to the same
                # tombstoned state before it can be observed or retained.
                self._mark_runtime_payload_discarded(task)
            if task.task_id in loaded:
                raise DiagnosticFileTransactionError("diagnostic_migration_input_invalid")
            loaded[task.task_id] = task
        self._tasks = loaded
        self._runtime_payloads = {
            task_id: binding
            for task_id, binding in self._runtime_payloads.items()
            if (
                task_id in loaded
                and binding[0] == loaded[task_id].case_id
                and loaded[task_id].status not in self._TERMINAL_STATUSES
                and not bool(loaded[task_id].metadata.get("runtime_payload_discarded"))
            )
        }
        self._persist_mtime_ns = None
        self._persist_size = len(persisted_bytes)
        self._persist_generation_sha256 = current_digest
        self._persist_generation_loaded = True
        self._prune_locked()
        # Startup may have changed terminal state, pruned records, or projected
        # a version-2 row that still carried unexpected metadata. Readiness is
        # unsafe until the exact closed generation is durably settled.
        self._persist_locked(force=True, required=True, lease=lease)

    def _prune_locked(self) -> None:
        if not self._tasks:
            return

        ephemeral_terminal_ids = [
            task.task_id
            for task in self._tasks.values()
            if task.task_type in self._EPHEMERAL_TASK_TYPES and task.status in self._TERMINAL_STATUSES
        ]
        for task_id in ephemeral_terminal_ids:
            self._tasks.pop(task_id, None)
            self._runtime_payloads.pop(task_id, None)

        if not self._tasks:
            return

        now = utc_now()
        cutoff = now - timedelta(days=self._terminal_ttl_days) if self._terminal_ttl_days > 0 else None

        if cutoff is not None:
            stale_ids = [
                task.task_id
                for task in self._tasks.values()
                if task.status in self._TERMINAL_STATUSES and task.updated_at < cutoff
            ]
            for task_id in stale_ids:
                self._tasks.pop(task_id, None)
                self._runtime_payloads.pop(task_id, None)

        if self._max_records <= 0 or len(self._tasks) <= self._max_records:
            return

        terminal_tasks = sorted(
            (task for task in self._tasks.values() if task.status in self._TERMINAL_STATUSES),
            key=lambda task: (task.updated_at, task.created_at, task.task_id),
        )
        remove_count = max(0, len(self._tasks) - self._max_records)
        for task in terminal_tasks[:remove_count]:
            self._tasks.pop(task.task_id, None)
            self._runtime_payloads.pop(task.task_id, None)

    def _persist_locked(
        self,
        *,
        force: bool = False,
        required: bool = False,
        lease: DiagnosticFileTransactionLease | None = None,
    ) -> None:
        if self._persist_path is None:
            return
        if not force and self._running_persist_interval_seconds > 0:
            now = monotonic()
            if (now - self._last_persist_monotonic) < self._running_persist_interval_seconds:
                self._pending_persist = True
                return
        try:
            self._persist_path.parent.mkdir(parents=True, exist_ok=True)
            payload = {
                "version": 2,
                "tasks": [task.model_dump(mode="json") for task in self._tasks.values()],
            }
            encoded = json.dumps(
                payload,
                ensure_ascii=False,
                separators=(",", ":"),
                sort_keys=True,
            ).encode("utf-8")
            if lease is None:
                replace_diagnostic_file_transactionally(
                    self._persist_path,
                    encoded,
                    surface="task_store",
                    max_bytes=self._PERSIST_MAX_BYTES,
                )
            else:
                lease.replace(encoded)
            self._persist_mtime_ns = None
            self._persist_size = len(encoded)
            self._persist_generation_sha256 = hashlib.sha256(encoded).hexdigest()
            self._persist_generation_loaded = True
            self._last_persist_monotonic = monotonic()
            self._pending_persist = False
        except Exception:
            if required:
                raise DiagnosticFileTransactionError() from None
            # Persistence failures should not break in-memory task state.
            return

    def create_task(
        self,
        task_type: TaskType,
        case_id: Optional[str] = None,
        metadata: Optional[dict] = None,
    ) -> TaskRecord:
        if task_type in self._QUARANTINED_TASK_TYPES:
            raise TaskRuntimePayloadUnavailableError(self._QUARANTINED_TASK_FAILURE_CODE)
        now = utc_now()
        normalized_case_id = canonical_task_case_id(case_id, allow_unbound=True)
        runtime_payload = deepcopy(metadata) if isinstance(metadata, dict) else {}
        self._validate_runtime_payload_case_binding(runtime_payload, normalized_case_id)
        task = TaskRecord(
            task_id=str(uuid4()),
            task_type=task_type,
            status=TaskStatus.QUEUED,
            progress=0,
            case_id=normalized_case_id,
            metadata=public_task_metadata(
                metadata,
                status=TaskStatus.QUEUED,
                runtime_payload_required=bool(runtime_payload),
            ),
            created_at=now,
            updated_at=now,
        )
        with self._lock:
            with self._mutating_store_locked() as lease:
                self._tasks[task.task_id] = task
                if runtime_payload:
                    self._runtime_payloads[task.task_id] = (normalized_case_id, runtime_payload)
                self._prune_locked()
                self._persist_locked(force=True, required=True, lease=lease)
                return self._public_copy(task)

    def get_task(self, task_id: str) -> TaskRecord:
        with self._lock:
            self._refresh_from_persisted_locked(mark_inflight_failed=False)
            task = self._tasks.get(task_id)
            if not task:
                raise TaskNotFoundError(task_id)
            return self._public_copy(task)

    def read_runtime_payload(self, task_id: str, *, case_id: Optional[str]) -> dict:
        """Read the non-public execution payload under an exact case binding.

        The explicit method keeps private job inputs/results out of TaskRecord,
        ordinary JSON persistence, and generic task endpoints. Callers must
        already hold the task's trusted case binding (including explicit None
        for intentionally unbound maintenance tasks).
        """

        expected_case_id = canonical_task_case_id(case_id, allow_unbound=True)
        with self._lock:
            self._refresh_from_persisted_locked(mark_inflight_failed=False)
            task = self._tasks.get(task_id)
            if task is None:
                raise TaskNotFoundError(task_id)
            if task.case_id != expected_case_id:
                raise TaskCaseBindingMismatchError("task_case_binding_mismatch")
            if task.status in self._TERMINAL_STATUSES:
                raise TaskRuntimePayloadUnavailableError("task_runtime_payload_unavailable")
            if bool(task.metadata.get("runtime_payload_discarded")):
                if bool(task.metadata.get("runtime_payload_required")):
                    raise TaskRuntimePayloadUnavailableError("task_runtime_payload_unavailable")
                return {}
            binding = self._runtime_payloads.get(task_id)
            if binding is None:
                if bool(task.metadata.get("runtime_payload_required")):
                    raise TaskRuntimePayloadUnavailableError("task_runtime_payload_unavailable")
                return {}
            if binding[0] != expected_case_id:
                raise TaskCaseBindingMismatchError("task_case_binding_mismatch")
            return deepcopy(binding[1])

    def list_tasks(
        self,
        page: int = 1,
        page_size: int = 50,
        status: Optional[TaskStatus] = None,
        task_type: Optional[TaskType] = None,
        case_id: Optional[str] = None,
        exclude_task_types: Optional[set[TaskType] | frozenset[TaskType]] = None,
    ) -> Tuple[List[TaskRecord], int]:
        with self._lock:
            self._refresh_from_persisted_locked(mark_inflight_failed=False)
            tasks = [self._public_copy(task) for task in self._tasks.values()]

        if exclude_task_types:
            tasks = [task for task in tasks if task.task_type not in exclude_task_types]
        if status is not None:
            tasks = [task for task in tasks if task.status == status]
        if task_type is not None:
            tasks = [task for task in tasks if task.task_type == task_type]
        if case_id is not None:
            normalized_case_id = canonical_task_case_id(case_id, allow_unbound=False)
            tasks = [task for task in tasks if task.case_id == normalized_case_id]

        tasks.sort(key=lambda item: item.created_at, reverse=True)
        total = len(tasks)
        start = (page - 1) * page_size
        end = start + page_size
        return tasks[start:end], total

    def transition_task(
        self,
        task_id: str,
        to_status: TaskStatus,
        progress: Optional[int] = None,
        error: Optional[str] = None,
        metadata: Optional[dict] = None,
    ) -> TaskRecord:
        with self._lock:
            with self._mutating_store_locked() as lease:
                task = self._tasks.get(task_id)
                if not task:
                    raise TaskNotFoundError(task_id)
                if task.task_type in self._QUARANTINED_TASK_TYPES and to_status != TaskStatus.FAILED:
                    raise TaskRuntimePayloadUnavailableError(self._QUARANTINED_TASK_FAILURE_CODE)
                if task.status in self._TERMINAL_STATUSES and task.status == to_status:
                    if (
                        task_id in self._runtime_payloads
                        or not bool(task.metadata.get("runtime_payload_discarded"))
                    ):
                        self._runtime_payloads.pop(task_id, None)
                        self._mark_runtime_payload_discarded(task)
                        self._tasks[task_id] = task
                        self._persist_locked(force=True, required=True, lease=lease)
                    return self._public_copy(task)

                TaskStateMachine.ensure_transition(task.status, to_status)
                if (
                    to_status in {TaskStatus.QUEUED, TaskStatus.RUNNING}
                    and bool(task.metadata.get("runtime_payload_required"))
                    and not self._has_runtime_payload(task)
                ):
                    raise TaskRuntimePayloadUnavailableError("task_runtime_payload_unavailable")
                task.status = to_status
                task.updated_at = utc_now()

                if progress is not None:
                    task.progress = progress
                elif to_status == TaskStatus.QUEUED:
                    task.progress = 0
                elif to_status == TaskStatus.SUCCEEDED:
                    task.progress = 100

                runtime_payload = self._runtime_payloads.get(task_id)
                runtime_metadata = deepcopy(runtime_payload[1]) if runtime_payload is not None else {}
                public_source = dict(task.metadata)
                if metadata:
                    public_source.update(metadata)
                terminal_transition = to_status in self._TERMINAL_STATUSES
                if terminal_transition:
                    self._runtime_payloads.pop(task_id, None)
                    runtime_metadata = {}

                if to_status in {TaskStatus.FAILED, TaskStatus.CANCELED}:
                    failure_code = stable_failure_code(to_status)
                    public_phase = public_task_metadata(public_source, status=to_status).get("stage")
                    failure_phase = public_phase or stable_failure_phase(to_status)
                    task.error = failure_code
                    task.metadata = self._terminal_failure_metadata(
                        task,
                        code=failure_code,
                        phase=failure_phase,
                        source=public_source,
                        runtime_payload_discarded=True,
                    )
                else:
                    if metadata and not terminal_transition:
                        self._validate_runtime_payload_case_binding(metadata, task.case_id)
                        runtime_metadata.update(deepcopy(metadata))
                    if runtime_metadata and not terminal_transition:
                        self._runtime_payloads[task_id] = (task.case_id, runtime_metadata)
                    task.error = None
                    task.metadata = public_task_metadata(
                        public_source,
                        status=to_status,
                        runtime_payload_discarded=terminal_transition,
                        runtime_payload_required=bool(runtime_metadata),
                    )

                self._tasks[task_id] = task
                self._prune_locked()
                self._persist_locked(force=True, required=True, lease=lease)
                return self._public_copy(task)

    def update_task_metadata(self, task_id: str, metadata: Optional[dict] = None) -> TaskRecord:
        with self._lock:
            with self._mutating_store_locked() as lease:
                task = self._tasks.get(task_id)
                if not task:
                    raise TaskNotFoundError(task_id)

                if task.status in self._TERMINAL_STATUSES:
                    # Terminal tasks are immutable public records. In particular,
                    # a late caller cannot append exception text after failure.
                    changed = task.task_id in self._runtime_payloads or not bool(
                        task.metadata.get("runtime_payload_discarded")
                    )
                    self._runtime_payloads.pop(task.task_id, None)
                    if changed:
                        self._mark_runtime_payload_discarded(task)
                        self._tasks[task.task_id] = task
                        self._persist_locked(force=True, required=True, lease=lease)
                    return self._public_copy(task)

                runtime_payload = self._runtime_payloads.get(task_id)
                runtime_metadata = deepcopy(runtime_payload[1]) if runtime_payload is not None else {}
                if metadata:
                    self._validate_runtime_payload_case_binding(metadata, task.case_id)
                    runtime_metadata.update(deepcopy(metadata))
                if runtime_metadata:
                    self._runtime_payloads[task_id] = (task.case_id, runtime_metadata)
                public_source = dict(task.metadata)
                if metadata:
                    public_source.update(metadata)
                task.metadata = public_task_metadata(
                    public_source,
                    status=task.status,
                    runtime_payload_required=bool(runtime_metadata),
                )
                task.updated_at = utc_now()
                self._tasks[task_id] = task
                self._prune_locked()
                self._persist_locked(force=True, required=True, lease=lease)
                return self._public_copy(task)

    def claim_next_task(
        self,
        *,
        task_types: Optional[List[TaskType]] = None,
        case_id: Optional[str] = None,
        metadata: Optional[dict] = None,
    ) -> Optional[TaskRecord]:
        normalized_case_id = (
            canonical_task_case_id(case_id, allow_unbound=False)
            if case_id is not None
            else None
        )
        with self._lock:
            with self._mutating_store_locked() as lease:
                candidates = [
                    task
                    for task in self._tasks.values()
                    if task.status == TaskStatus.QUEUED
                    and task.task_type not in self._QUARANTINED_TASK_TYPES
                    and (not task_types or task.task_type in task_types)
                    and (normalized_case_id is None or task.case_id == normalized_case_id)
                ]
                if not candidates:
                    return None
                candidates.sort(key=lambda item: (item.created_at, item.task_id))
                task = candidates[0]
                TaskStateMachine.ensure_transition(task.status, TaskStatus.RUNNING)
                task.status = TaskStatus.RUNNING
                task.progress = max(int(task.progress or 0), 1)
                task.updated_at = utc_now()
                if metadata:
                    self._validate_runtime_payload_case_binding(metadata, task.case_id)
                    runtime_payload = self._runtime_payloads.get(task.task_id)
                    runtime_metadata = deepcopy(runtime_payload[1]) if runtime_payload is not None else {}
                    runtime_metadata.update(deepcopy(metadata))
                    self._runtime_payloads[task.task_id] = (task.case_id, runtime_metadata)
                    public_source = dict(task.metadata)
                    public_source.update(metadata)
                    task.metadata = public_task_metadata(
                        public_source,
                        status=TaskStatus.RUNNING,
                        runtime_payload_required=True,
                    )
                self._tasks[task.task_id] = task
                self._prune_locked()
                self._persist_locked(force=True, required=True, lease=lease)
                return self._public_copy(task)

    def get_public_task(self, task_id: str, *, case_id: str) -> TaskRecord:
        expected_case_id = canonical_task_case_id(case_id, allow_unbound=False)
        task = self.get_task(task_id)
        if task.task_type in self._QUARANTINED_TASK_TYPES or task.case_id != expected_case_id:
            raise TaskNotFoundError(task_id)
        return task

    def list_public_tasks(
        self,
        *,
        case_id: str,
        page: int = 1,
        page_size: int = 50,
        status: Optional[TaskStatus] = None,
        task_type: Optional[TaskType] = None,
    ) -> Tuple[List[TaskRecord], int]:
        expected_case_id = canonical_task_case_id(case_id, allow_unbound=False)
        if task_type in self._QUARANTINED_TASK_TYPES:
            return [], 0
        return self.list_tasks(
            page=page,
            page_size=page_size,
            status=status,
            task_type=task_type,
            case_id=expected_case_id,
            exclude_task_types=self._QUARANTINED_TASK_TYPES,
        )

    @staticmethod
    def _public_copy(task: TaskRecord) -> TaskRecord:
        public = task.model_copy(deep=True)
        # The tombstone is durable host state used to reject stale callbacks
        # after terminal transitions and restarts. It is not part of the task
        # API contract and must not leak through polling, events, or exports.
        public.metadata.pop("runtime_payload_discarded", None)
        return public

    def _has_runtime_payload(self, task: TaskRecord) -> bool:
        if task.status in self._TERMINAL_STATUSES or bool(task.metadata.get("runtime_payload_discarded")):
            return False
        binding = self._runtime_payloads.get(task.task_id)
        return binding is not None and binding[0] == task.case_id

    @staticmethod
    def _validate_runtime_payload_case_binding(metadata: object, expected_case_id: Optional[str]) -> None:
        pending = [metadata]
        seen: set[int] = set()
        while pending:
            current = pending.pop()
            if isinstance(current, Mapping):
                object_id = id(current)
                if object_id in seen:
                    continue
                seen.add(object_id)
                for raw_key, value in current.items():
                    key = str(raw_key or "")
                    if key in {"case_id", "caseId"}:
                        observed = canonical_task_case_id(value, allow_unbound=True)
                        if observed != expected_case_id:
                            raise TaskCaseBindingMismatchError("task_case_binding_mismatch")
                    elif isinstance(value, (Mapping, list, tuple)):
                        pending.append(value)
            elif isinstance(current, Sequence) and not isinstance(current, (str, bytes, bytearray)):
                object_id = id(current)
                if object_id in seen:
                    continue
                seen.add(object_id)
                pending.extend(current)

    def _terminal_failure_metadata(
        self,
        task: TaskRecord,
        *,
        code: Optional[str],
        phase: Optional[str],
        source: Optional[dict] = None,
        runtime_payload_discarded: Optional[bool] = None,
    ) -> dict:
        correlation_id = existing_failure_correlation_id(task.metadata) or str(uuid4())
        return public_task_metadata(
            source if source is not None else task.metadata,
            status=task.status,
            failure_code=code,
            failure_phase=phase,
            failure_correlation_id=correlation_id,
            runtime_payload_discarded=(
                bool(task.metadata.get("runtime_payload_discarded"))
                if runtime_payload_discarded is None
                else bool(runtime_payload_discarded)
            ),
            runtime_payload_required=(
                self._has_runtime_payload(task)
                or bool(task.metadata.get("runtime_payload_required"))
            ),
        )

    def _quarantine_task_record(self, task: TaskRecord, *, updated_at) -> None:
        failure_code = self._QUARANTINED_TASK_FAILURE_CODE
        canonical_metadata = public_task_metadata(
            {},
            status=TaskStatus.FAILED,
            failure_code=failure_code,
            failure_phase="failed",
            failure_correlation_id=migrated_failure_correlation_id(
                task_id=task.task_id,
                failure_code=failure_code,
            ),
            runtime_payload_discarded=True,
            runtime_payload_required=True,
        )
        if (
            task.status == TaskStatus.FAILED
            and task.error == failure_code
            and task.metadata == canonical_metadata
        ):
            return
        task.status = TaskStatus.FAILED
        task.error = failure_code
        task.progress = max(int(task.progress or 0), 1)
        task.updated_at = updated_at
        task.metadata = canonical_metadata

    def _mark_runtime_payload_discarded(self, task: TaskRecord) -> None:
        source = dict(task.metadata)
        if task.status in {TaskStatus.FAILED, TaskStatus.CANCELED}:
            task.metadata = self._terminal_failure_metadata(
                task,
                code=stable_failure_code(task.status),
                phase=str(task.metadata.get("failure_phase") or stable_failure_phase(task.status) or ""),
                source=source,
                runtime_payload_discarded=True,
            )
            return
        task.metadata = public_task_metadata(
            source,
            status=task.status,
            runtime_payload_discarded=True,
            runtime_payload_required=False,
        )

    def _load_public_task_record(self, item: object, *, legacy_payload: bool) -> TaskRecord:
        if not isinstance(item, dict):
            raise ValueError("task_record_invalid")
        task_id = canonical_task_id(item.get("task_id"))
        status = TaskStatus(str(item.get("status") or ""))
        task_type = TaskType(str(item.get("task_type") or ""))
        case_id = canonical_task_case_id(item.get("case_id"), allow_unbound=True)
        raw_metadata = item.get("metadata") if isinstance(item.get("metadata"), dict) else {}
        runtime_payload_discarded = raw_metadata.get("runtime_payload_discarded") is True
        if "runtime_payload_discarded" in raw_metadata and raw_metadata.get("runtime_payload_discarded") is not True:
            raise ValueError("task_runtime_payload_discarded_invalid")
        if runtime_payload_discarded and status not in self._TERMINAL_STATUSES:
            raise ValueError("task_runtime_payload_discarded_invalid")
        runtime_payload_required = bool(raw_metadata.get("runtime_payload_required")) or (
            legacy_payload and bool(raw_metadata)
        )
        quarantined_failure = (
            task_type in self._QUARANTINED_TASK_TYPES
            and status == TaskStatus.FAILED
            and raw_metadata.get("failure_code") == self._QUARANTINED_TASK_FAILURE_CODE
            and raw_metadata.get("failure_phase") == "failed"
            and runtime_payload_discarded
            and runtime_payload_required
        )
        failure_code = (
            self._QUARANTINED_TASK_FAILURE_CODE
            if quarantined_failure
            else stable_failure_code(status)
        )
        failure_phase = "failed" if quarantined_failure else stable_failure_phase(status)
        failure_correlation_id = existing_failure_correlation_id(raw_metadata)
        if failure_code is not None and failure_correlation_id is None:
            failure_correlation_id = (
                migrated_failure_correlation_id(task_id=task_id, failure_code=failure_code)
                if legacy_payload
                else str(uuid4())
            )
        metadata = public_task_metadata(
            raw_metadata,
            status=status,
            failure_code=failure_code,
            failure_phase=failure_phase,
            failure_correlation_id=failure_correlation_id,
            runtime_payload_discarded=runtime_payload_discarded,
            runtime_payload_required=runtime_payload_required,
        )
        return TaskRecord.model_validate(
            {
                "task_id": task_id,
                "task_type": task_type,
                "status": status,
                "progress": item.get("progress", 0),
                "case_id": case_id,
                "metadata": metadata,
                "error": failure_code,
                "created_at": item.get("created_at"),
                "updated_at": item.get("updated_at"),
            }
        )


def _strict_json_object(pairs: list[tuple[str, object]]) -> dict[str, object]:
    result: dict[str, object] = {}
    for key, value in pairs:
        if key in result:
            raise ValueError("duplicate task-store key")
        result[key] = value
    return result
