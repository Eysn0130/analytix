from __future__ import annotations

from contextlib import contextmanager
import os
from pathlib import Path
import re
import threading
from typing import Any, Iterable, Iterator, Optional
import weakref

import duckdb

from app.core.paths import get_app_data_dir, ensure_dir


_CASE_STORAGE_ID_RE = re.compile(r"^[A-Za-z0-9][A-Za-z0-9_-]{0,127}$")


def validate_case_storage_id(value: object) -> str:
    if type(value) is not str or value != value.strip() or _CASE_STORAGE_ID_RE.fullmatch(value) is None:
        raise ValueError("case_storage_id_invalid")
    return value


def get_case_db_path(case_id: str) -> Path:
    normalized_case_id = validate_case_storage_id(case_id)
    cases_root = (get_app_data_dir() / "cases").resolve()
    case_dir = cases_root / normalized_case_id
    if case_dir.is_symlink() or case_dir.resolve(strict=False).parent != cases_root:
        raise ValueError("case_storage_path_invalid")
    db_path = case_dir / "case.duckdb"
    if db_path.is_symlink():
        raise ValueError("case_storage_path_invalid")
    return db_path


class Engine:
    def execute(self, sql: str, params: Optional[Iterable[Any]] = None) -> None:
        raise NotImplementedError

    def executemany(self, sql: str, rows: Iterable[Iterable[Any]]) -> None:
        raise NotImplementedError

    def query(self, sql: str, params: Optional[Iterable[Any]] = None) -> list[tuple]:
        raise NotImplementedError

    def close(self) -> None:
        raise NotImplementedError


_DB_LOCKS_GUARD = threading.RLock()
_DB_OPEN_LOCKS: dict[Path, threading.RLock] = {}
_DB_OPERATION_LOCKS: dict[Path, threading.RLock] = {}
_DB_ACTIVE_ENGINES: dict[Path, weakref.WeakSet["DuckDBEngine"]] = {}


class DirectActiveDuckDBAccessDisabledError(RuntimeError):
    def __init__(self, db_path: Path | str, *, reason: str = "product active DB access") -> None:
        self.code = "direct_active_duckdb_access_disabled"
        # Paths and caller-provided reasons may contain case identifiers or PII.
        # Keep the exception text deterministic so ordinary API/log projection
        # cannot disclose either value.
        _ = db_path, reason
        super().__init__(
            "direct Python DuckDB access to the active case database is disabled; "
            "use the admitted Analytix data engine"
        )


def direct_python_duckdb_disabled() -> bool:
    """Active-case direct access is a production invariant, not configuration."""

    return True


def canonicalize_db_path(path: Path | str) -> Path:
    return _normalize_db_path(path)


def is_active_case_db_path(path: Path | str, case_id: str | None = None) -> bool:
    normalized = _normalize_db_path(path)
    if normalized.name.lower() != "case.duckdb":
        return False
    if case_id and normalized.parent.name == str(case_id).strip():
        return True
    try:
        cases_dir = (get_app_data_dir() / "cases").resolve()
        normalized.relative_to(cases_dir)
        return normalized.parent.parent == cases_dir
    except Exception:
        return normalized.parent.parent.name == "cases"


def assert_python_duckdb_allowed(
    db_path: Path | str,
    *,
    reason: str,
    legacy: bool = False,
    snapshot: bool = False,
    case_id: str | None = None,
) -> None:
    # An explicit maintenance classification never authorizes the live
    # cases/<case>/case.duckdb path. Snapshot and legacy maintenance databases
    # must remain separate paths selected by trusted host code.
    if is_active_case_db_path(db_path, case_id=case_id):
        raise DirectActiveDuckDBAccessDisabledError(db_path, reason=reason)
    if legacy or snapshot:
        return


def _db_lock(lock_map: dict[Path, threading.RLock], path: Path) -> threading.RLock:
    normalized_path = Path(path).resolve()
    with _DB_LOCKS_GUARD:
        lock = lock_map.get(normalized_path)
        if lock is None:
            lock = threading.RLock()
            lock_map[normalized_path] = lock
        return lock


class DuckDBEngine(Engine):
    def __init__(self, path: Path, *, read_only: bool = False):
        self.path = Path(path)
        self._read_only = bool(read_only)
        self._closed = False
        self._operation_lock = threading.RLock()
        self._remote_session_id: str = ""
        self._remote = False
        self._remote_in_transaction = False
        if is_active_case_db_path(self.path):
            self._open_remote(read_only=read_only)
            if not read_only:
                configure_duckdb(self, case_dir=self.path.parent)
            return
        assert_python_duckdb_allowed(self.path, reason="DuckDBEngine direct open")
        with _db_lock(_DB_OPEN_LOCKS, self.path):
            self._con = duckdb.connect(str(self.path), read_only=read_only)
            if not read_only:
                configure_duckdb(self, case_dir=self.path.parent)
        _register_active_engine(self)

    @property
    def connection(self) -> duckdb.DuckDBPyConnection:
        if self._remote:
            raise DirectActiveDuckDBAccessDisabledError(self.path, reason="raw connection requested for remote DuckDBEngine")
        return self._con

    @contextmanager
    def connection_operation(self) -> Iterator[Any]:
        """Serialize direct operations on this DuckDB database.

        DuckDB can invalidate a database when multiple worker threads overlap
        execute/fetch/write work across connections. Most repository code should
        use execute/query; this guard is for the few places that need cursor
        metadata or streaming.
        """
        if self._remote:
            with self._operation_lock:
                yield _RemoteDuckDBCursor(self)
            return
        with _db_lock(_DB_OPERATION_LOCKS, self.path):
            with self._operation_lock:
                yield self._con

    def execute(self, sql: str, params: Optional[Iterable[Any]] = None) -> None:
        if self._remote:
            transaction_command = _transaction_command(sql)
            if transaction_command in {"commit", "rollback"} and not self._remote_in_transaction:
                return
            self._remote_request(
                "duckdb.execute",
                {
                    "session_id": self._remote_session_id,
                    "sql": str(sql),
                    "params": _json_ready_params(params),
                },
            )
            if transaction_command == "begin":
                self._remote_in_transaction = True
            elif transaction_command in {"commit", "rollback"}:
                self._remote_in_transaction = False
            return
        with _db_lock(_DB_OPERATION_LOCKS, self.path):
            with self._operation_lock:
                if params is None:
                    self._con.execute(sql)
                else:
                    self._con.execute(sql, params)

    def cursor(self) -> Any:
        if self._remote:
            return _RemoteDuckDBCursor(self)
        return self._con

    def executemany(self, sql: str, rows: Iterable[Iterable[Any]]) -> None:
        if self._remote:
            self._remote_request(
                "duckdb.executemany",
                {
                    "session_id": self._remote_session_id,
                    "sql": str(sql),
                    "rows": [_json_ready_params(row) for row in rows],
                },
            )
            return
        with _db_lock(_DB_OPERATION_LOCKS, self.path):
            with self._operation_lock:
                self._con.executemany(sql, rows)

    def query(self, sql: str, params: Optional[Iterable[Any]] = None) -> list[tuple]:
        if self._remote:
            payload = self._remote_request(
                "duckdb.query",
                {
                    "session_id": self._remote_session_id,
                    "sql": str(sql),
                    "params": _json_ready_params(params),
                },
            )
            raw_rows = payload.get("rows") if isinstance(payload, dict) else []
            if not isinstance(raw_rows, list):
                return []
            return [tuple(row if isinstance(row, list) else [row]) for row in raw_rows]
        with _db_lock(_DB_OPERATION_LOCKS, self.path):
            with self._operation_lock:
                if params is None:
                    return self._con.execute(sql).fetchall()
                return self._con.execute(sql, params).fetchall()

    def commit(self) -> None:
        try:
            if self._remote:
                self.execute("COMMIT")
                return
            with _db_lock(_DB_OPERATION_LOCKS, self.path):
                with self._operation_lock:
                    self._con.commit()
        except Exception:
            pass

    def close(self) -> None:
        with self._operation_lock:
            if self._closed:
                return
            self._closed = True
            if self._remote:
                try:
                    self._remote_request(
                        "duckdb.close_session",
                        {
                            "session_id": self._remote_session_id,
                        },
                    )
                except Exception:
                    pass
                return
            _unregister_active_engine(self)
            try:
                self._con.close()
            except Exception:
                pass

    def replace_with_compacted_generation(self) -> None:
        """Replace the current database with a fresh logical DuckDB generation.

        The caller must leave its durable migration journal in a non-readable
        phase before invoking this method. A successful return preserves the
        same engine object but points it at a freshly copied database file, so
        obsolete pages from the prior generation are no longer addressable by
        the case database path.
        """

        if self._read_only:
            raise RuntimeError("duckdb_generation_replace_read_only")
        if self._remote:
            payload = self._remote_request(
                "duckdb.compact_session",
                {"session_id": self._remote_session_id},
            )
            session_id = str(payload.get("session_id") or "").strip()
            if session_id != self._remote_session_id:
                raise RuntimeError("duckdb_generation_replace_failed")
            configure_duckdb(self, case_dir=self.path.parent)
            return

        source = _normalize_db_path(self.path)
        next_path = source.with_name(f".{source.name}.analytix-diagnostic-next")
        next_wal_path = Path(f"{next_path}.wal")
        source_wal_path = Path(f"{source}.wal")
        original_mode = source.stat().st_mode & 0o777
        with _db_lock(_DB_OPEN_LOCKS, source):
            with _db_lock(_DB_OPERATION_LOCKS, source):
                with self._operation_lock:
                    active = [
                        engine
                        for engine in _active_engines_for_path(source)
                        if not getattr(engine, "_closed", False)
                    ]
                    if len(active) != 1 or active[0] is not self:
                        raise RuntimeError("duckdb_generation_replace_owner_conflict")
                    _remove_generation_file(next_wal_path)
                    _remove_generation_file(next_path)
                    alias = f"analytix_compacted_{threading.get_ident()}"
                    catalog = _database_catalog_for_path(self._con, source)
                    if not catalog:
                        raise RuntimeError("duckdb_generation_replace_failed")
                    try:
                        self._con.execute("CHECKPOINT")
                        self._con.execute(
                            f"ATTACH {_sql_literal(str(next_path))} AS {_quote_identifier(alias)}"
                        )
                        try:
                            self._con.execute(
                                f"COPY FROM DATABASE {_quote_identifier(catalog)} "
                                f"TO {_quote_identifier(alias)}"
                            )
                        finally:
                            self._con.execute(f"DETACH {_quote_identifier(alias)}")
                        os.chmod(next_path, original_mode)
                        _fsync_file(next_path)
                        self._con.close()
                        _remove_generation_file(source_wal_path)
                        os.replace(next_path, source)
                        _fsync_directory(source.parent)
                        self._con = duckdb.connect(str(source), read_only=False)
                    except BaseException:
                        try:
                            self._con = duckdb.connect(str(source), read_only=False)
                        except Exception:
                            self._closed = True
                            _unregister_active_engine(self)
                        _remove_generation_file(next_wal_path)
                        _remove_generation_file(next_path)
                        raise RuntimeError("duckdb_generation_replace_failed") from None
        configure_duckdb(self, case_dir=self.path.parent)

    def _open_remote(self, *, read_only: bool) -> None:
        payload = self._remote_request(
            "duckdb.open_session",
            {
                "read_only": bool(read_only),
            },
        )
        session_id = str(payload.get("session_id") or "").strip()
        if not session_id:
            raise RuntimeError("analytix-data-engine did not return duckdb session_id")
        self._remote = True
        self._remote_session_id = session_id

    def _remote_request(self, command: str, payload: dict[str, Any]) -> dict[str, Any]:
        from app.core.data_engine_client import run_data_engine_command

        return run_data_engine_command(
            command,
            case_id=_case_id_from_db_path(self.path),
            db_path=self.path,
            payload=payload,
            timeout=1800,
        )


class _RemoteDuckDBCursor:
    def __init__(self, engine: DuckDBEngine) -> None:
        self._engine = engine
        self.description: list[tuple[str]] = []
        self._rows: list[tuple] = []
        self._index = 0

    def execute(self, sql: str, params: Optional[Iterable[Any]] = None) -> "_RemoteDuckDBCursor":
        if _sql_returns_rows(sql):
            payload = self._engine._remote_request(
                "duckdb.query",
                {
                    "session_id": self._engine._remote_session_id,
                    "sql": str(sql),
                    "params": _json_ready_params(params),
                },
            )
            columns = payload.get("columns") if isinstance(payload, dict) else []
            raw_rows = payload.get("rows") if isinstance(payload, dict) else []
            self.description = [(str(column or ""),) for column in columns] if isinstance(columns, list) else []
            self._rows = [tuple(row if isinstance(row, list) else [row]) for row in raw_rows] if isinstance(raw_rows, list) else []
        else:
            self._engine.execute(sql, params)
            self.description = []
            self._rows = []
        self._index = 0
        return self

    def fetchall(self) -> list[tuple]:
        rows = self._rows[self._index :]
        self._index = len(self._rows)
        return list(rows)

    def fetchmany(self, size: int | None = None) -> list[tuple]:
        limit = max(0, int(size or 1))
        start = self._index
        end = min(len(self._rows), start + limit)
        self._index = end
        return list(self._rows[start:end])

    def fetchone(self) -> Optional[tuple]:
        if self._index >= len(self._rows):
            return None
        row = self._rows[self._index]
        self._index += 1
        return row


def _json_ready_params(params: Optional[Iterable[Any]]) -> list[Any]:
    if params is None:
        return []
    return [_json_ready_value(value) for value in params]


def _json_ready_value(value: Any) -> Any:
    if value is None or isinstance(value, (str, int, float, bool)):
        return value
    if isinstance(value, Path):
        return str(value)
    if isinstance(value, bytes):
        return list(value)
    if isinstance(value, (list, tuple)):
        return [_json_ready_value(item) for item in value]
    if isinstance(value, dict):
        return {str(key): _json_ready_value(item) for key, item in value.items()}
    return str(value)


def _case_id_from_db_path(path: Path) -> str:
    try:
        if path.name.lower() == "case.duckdb":
            return path.parent.name
    except Exception:
        pass
    return ""


def _sql_returns_rows(sql: str) -> bool:
    trimmed = str(sql or "").lstrip()
    if trimmed.upper().startswith("COPY FROM DATABASE"):
        return False
    first = (trimmed.split(None, 1)[0] if trimmed.strip() else "").strip("();").upper()
    return first in {"SELECT", "WITH", "PRAGMA", "SHOW", "DESCRIBE", "DESC", "EXPLAIN", "COPY"}


def _transaction_command(sql: str) -> str:
    normalized = " ".join(str(sql or "").strip().strip(";").split()).upper()
    if normalized in {"BEGIN", "BEGIN TRANSACTION", "START TRANSACTION"}:
        return "begin"
    if normalized == "COMMIT":
        return "commit"
    if normalized == "ROLLBACK":
        return "rollback"
    return ""


def copy_duckdb_database_snapshot_from_active_connection(source_path: Path, snapshot_path: Path) -> bool:
    """Copy a locked DuckDB database through an already-open in-process connection.

    On Windows, a live DuckDB connection can prevent plain file copy and a second
    process from opening the same `case.duckdb`. When that happens, the owning
    connection can still make a consistent database copy for read-only native
    analysis queries.
    """
    source = _normalize_db_path(source_path)
    engines = _active_engines_for_path(source)
    if not engines:
        return False

    destination = Path(snapshot_path)
    destination.parent.mkdir(parents=True, exist_ok=True)
    for engine in reversed(engines):
        if getattr(engine, "_closed", False):
            continue
        if _copy_database_from_engine(engine, source, destination):
            return True
    return False


def _copy_database_from_engine(engine: "DuckDBEngine", source: Path, destination: Path) -> bool:
    alias = f"analytix_snapshot_{threading.get_ident()}_{id(destination)}"
    try:
        with engine.connection_operation() as connection:
            catalog = _database_catalog_for_path(connection, source)
            if not catalog:
                return False
            try:
                destination.unlink()
            except FileNotFoundError:
                pass
            connection.execute(f"ATTACH {_sql_literal(str(destination))} AS {_quote_identifier(alias)}")
            try:
                connection.execute(
                    f"COPY FROM DATABASE {_quote_identifier(catalog)} TO {_quote_identifier(alias)}"
                )
            finally:
                try:
                    connection.execute(f"DETACH {_quote_identifier(alias)}")
                except Exception:
                    pass
        return destination.is_file() and destination.stat().st_size > 0
    except Exception:
        try:
            destination.unlink()
        except Exception:
            pass
        return False


def _database_catalog_for_path(connection: duckdb.DuckDBPyConnection, source: Path) -> str:
    try:
        rows = connection.execute("PRAGMA database_list").fetchall()
    except Exception:
        return ""
    for row in rows:
        if len(row) < 3:
            continue
        name = str(row[1] or "")
        db_path = str(row[2] or "")
        if name and db_path and _same_db_path(db_path, source):
            return name
    return ""


def _register_active_engine(engine: "DuckDBEngine") -> None:
    normalized_path = _normalize_db_path(engine.path)
    with _DB_LOCKS_GUARD:
        engines = _DB_ACTIVE_ENGINES.get(normalized_path)
        if engines is None:
            engines = weakref.WeakSet()
            _DB_ACTIVE_ENGINES[normalized_path] = engines
        engines.add(engine)


def _unregister_active_engine(engine: "DuckDBEngine") -> None:
    normalized_path = _normalize_db_path(engine.path)
    with _DB_LOCKS_GUARD:
        engines = _DB_ACTIVE_ENGINES.get(normalized_path)
        if engines is not None:
            engines.discard(engine)
            if not list(engines):
                _DB_ACTIVE_ENGINES.pop(normalized_path, None)


def _active_engines_for_path(path: Path) -> list["DuckDBEngine"]:
    normalized_path = _normalize_db_path(path)
    with _DB_LOCKS_GUARD:
        engines = _DB_ACTIVE_ENGINES.get(normalized_path)
        return list(engines or ())


def _normalize_db_path(path: Path | str) -> Path:
    try:
        return Path(path).resolve()
    except Exception:
        return Path(path).absolute()


def _same_db_path(left: Path | str, right: Path | str) -> bool:
    try:
        return Path(left).resolve() == Path(right).resolve()
    except Exception:
        return os.path.normcase(os.path.abspath(str(left))) == os.path.normcase(os.path.abspath(str(right)))


def _remove_generation_file(path: Path) -> None:
    try:
        path.unlink()
    except FileNotFoundError:
        return


def _fsync_file(path: Path) -> None:
    descriptor = os.open(path, os.O_RDONLY)
    try:
        os.fsync(descriptor)
    finally:
        os.close(descriptor)


def _fsync_directory(path: Path) -> None:
    if os.name == "nt":
        return
    flags = os.O_RDONLY | getattr(os, "O_DIRECTORY", 0)
    descriptor = os.open(path, flags)
    try:
        os.fsync(descriptor)
    finally:
        os.close(descriptor)


def _quote_identifier(value: str) -> str:
    return '"' + str(value or "").replace('"', '""') + '"'


def _sql_literal(value: str) -> str:
    return "'" + (value or "").replace("'", "''") + "'"


def configure_duckdb(
    engine: DuckDBEngine,
    *,
    case_dir: Optional[Path] = None,
    threads: Optional[int] = None,
    memory_limit: Optional[str] = None,
) -> None:
    cpu = os.cpu_count() or 4
    threads_value = max(1, threads or cpu)
    configured_memory_limit = memory_limit
    if configured_memory_limit is None:
        configured_memory_limit = os.environ.get("ANALYTIX_DUCKDB_MEMORY_LIMIT", "").strip() or None
    temp_dir = None
    if case_dir:
        temp_dir = ensure_dir(Path(case_dir) / "duckdb_tmp")
    try:
        if temp_dir:
            engine.execute(f"PRAGMA temp_directory={_sql_literal(str(temp_dir))}")
    except Exception:
        pass
    try:
        engine.execute(f"PRAGMA threads={threads_value}")
    except Exception:
        pass
    try:
        if configured_memory_limit:
            engine.execute(f"PRAGMA memory_limit={_sql_literal(configured_memory_limit)}")
    except Exception:
        pass
    for sql in (
        "SET preserve_insertion_order=false",
        "SET wal_autocheckpoint='1GB'",
        "SET checkpoint_threshold='1GB'",
        "SET auto_checkpoint_skip_wal_threshold=1073741824",
    ):
        try:
            engine.execute(sql)
        except Exception:
            pass
