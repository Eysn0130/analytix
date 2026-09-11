from __future__ import annotations

import math
import sqlite3
import time
from dataclasses import dataclass
from pathlib import Path
from typing import Iterable

from app.core.paths import ensure_dir, get_app_data_dir

_USAGE_TABLE = "skill_usage"
_RECENCY_HALF_LIFE_DAYS = 7.0
_DENIED_HALF_LIFE_DAYS = 3.0
_MIN_RECENCY_FACTOR = 0.1
_SECONDS_PER_DAY = 24 * 60 * 60


@dataclass(frozen=True)
class SkillUsageSnapshot:
    skill_id: str
    usage_count: int = 0
    executed_count: int = 0
    approval_requested_count: int = 0
    denied_count: int = 0
    last_used_at: float = 0.0
    last_executed_at: float = 0.0
    last_approval_requested_at: float = 0.0
    last_denied_at: float = 0.0
    usage_score: float = 0.0

    def to_dict(self) -> dict[str, object]:
        return {
            "skill_id": self.skill_id,
            "usage_count": self.usage_count,
            "executed_count": self.executed_count,
            "approval_requested_count": self.approval_requested_count,
            "denied_count": self.denied_count,
            "last_used_at": self.last_used_at,
            "last_executed_at": self.last_executed_at,
            "last_approval_requested_at": self.last_approval_requested_at,
            "last_denied_at": self.last_denied_at,
            "usage_score": round(self.usage_score, 4),
        }


class SkillUsageRepository:
    def __init__(self, *, db_path: Path | None = None) -> None:
        app_dir = ensure_dir(get_app_data_dir())
        self._db_path = db_path or (app_dir / "skill_usage.sqlite3")
        self._db_path.parent.mkdir(parents=True, exist_ok=True)
        self._ensure_schema()

    @property
    def db_path(self) -> Path:
        return self._db_path

    def _connect(self) -> sqlite3.Connection:
        connection = sqlite3.connect(self._db_path, timeout=5.0)
        connection.row_factory = sqlite3.Row
        return connection

    def _ensure_schema(self) -> None:
        with self._connect() as con:
            con.execute(
                f"""
                CREATE TABLE IF NOT EXISTS {_USAGE_TABLE}(
                    skill_id TEXT PRIMARY KEY,
                    usage_count INTEGER NOT NULL DEFAULT 0,
                    executed_count INTEGER NOT NULL DEFAULT 0,
                    approval_requested_count INTEGER NOT NULL DEFAULT 0,
                    denied_count INTEGER NOT NULL DEFAULT 0,
                    last_used_at REAL NOT NULL DEFAULT 0,
                    last_executed_at REAL NOT NULL DEFAULT 0,
                    last_approval_requested_at REAL NOT NULL DEFAULT 0,
                    last_denied_at REAL NOT NULL DEFAULT 0,
                    updated_at REAL NOT NULL DEFAULT 0
                )
                """
            )

    def record_turn(
        self,
        *,
        planned_skill_ids: Iterable[str] = (),
        executed_skill_ids: Iterable[str] = (),
        approval_requested_skill_ids: Iterable[str] = (),
        denied_skill_ids: Iterable[str] = (),
        now_ts: float | None = None,
    ) -> None:
        event_ts = float(now_ts if now_ts is not None else time.time())
        planned_ids = self._normalize_skill_ids(planned_skill_ids)
        executed_ids = self._normalize_skill_ids(executed_skill_ids)
        approval_ids = self._normalize_skill_ids(approval_requested_skill_ids)
        denied_ids = self._normalize_skill_ids(denied_skill_ids)
        if not any((planned_ids, executed_ids, approval_ids, denied_ids)):
            return
        with self._connect() as con:
            for skill_id in planned_ids:
                self._bump_counter(con, skill_id=skill_id, counter_field="usage_count", ts_field="last_used_at", event_ts=event_ts)
            for skill_id in executed_ids:
                self._bump_counter(con, skill_id=skill_id, counter_field="executed_count", ts_field="last_executed_at", event_ts=event_ts)
            for skill_id in approval_ids:
                self._bump_counter(
                    con,
                    skill_id=skill_id,
                    counter_field="approval_requested_count",
                    ts_field="last_approval_requested_at",
                    event_ts=event_ts,
                )
            for skill_id in denied_ids:
                self._bump_counter(con, skill_id=skill_id, counter_field="denied_count", ts_field="last_denied_at", event_ts=event_ts)
            con.commit()

    def _bump_counter(
        self,
        con: sqlite3.Connection,
        *,
        skill_id: str,
        counter_field: str,
        ts_field: str,
        event_ts: float,
    ) -> None:
        if counter_field not in {
            "usage_count",
            "executed_count",
            "approval_requested_count",
            "denied_count",
        }:
            raise ValueError(f"unsupported counter field: {counter_field}")
        if ts_field not in {
            "last_used_at",
            "last_executed_at",
            "last_approval_requested_at",
            "last_denied_at",
        }:
            raise ValueError(f"unsupported timestamp field: {ts_field}")
        con.execute(
            f"""
            INSERT INTO {_USAGE_TABLE}(skill_id, {counter_field}, {ts_field}, updated_at)
            VALUES (?, 1, ?, ?)
            ON CONFLICT(skill_id) DO UPDATE
            SET {counter_field}={_USAGE_TABLE}.{counter_field} + 1,
                {ts_field}=excluded.{ts_field},
                updated_at=excluded.updated_at
            """,
            (skill_id, event_ts, event_ts),
        )

    def get_usage_snapshot(self, skill_id: str, *, now_ts: float | None = None) -> SkillUsageSnapshot:
        normalized = self._normalize_skill_ids([skill_id])
        if not normalized:
            return SkillUsageSnapshot(skill_id="")
        snapshots = self.get_usage_snapshots(normalized, now_ts=now_ts)
        return snapshots.get(normalized[0], SkillUsageSnapshot(skill_id=normalized[0]))

    def get_usage_snapshots(
        self,
        skill_ids: Iterable[str],
        *,
        now_ts: float | None = None,
    ) -> dict[str, SkillUsageSnapshot]:
        normalized_ids = self._normalize_skill_ids(skill_ids)
        if not normalized_ids:
            return {}
        event_ts = float(now_ts if now_ts is not None else time.time())
        snapshots = {skill_id: SkillUsageSnapshot(skill_id=skill_id) for skill_id in normalized_ids}
        placeholders = ",".join("?" for _ in normalized_ids)
        with self._connect() as con:
            rows = con.execute(
                f"""
                SELECT
                    skill_id,
                    usage_count,
                    executed_count,
                    approval_requested_count,
                    denied_count,
                    last_used_at,
                    last_executed_at,
                    last_approval_requested_at,
                    last_denied_at
                FROM {_USAGE_TABLE}
                WHERE skill_id IN ({placeholders})
                """,
                tuple(normalized_ids),
            ).fetchall()
        for row in rows:
            skill_id = str(row["skill_id"] or "").strip()
            if not skill_id:
                continue
            usage_count = self._coerce_int(row["usage_count"])
            executed_count = self._coerce_int(row["executed_count"])
            approval_requested_count = self._coerce_int(row["approval_requested_count"])
            denied_count = self._coerce_int(row["denied_count"])
            last_used_at = self._coerce_float(row["last_used_at"])
            last_executed_at = self._coerce_float(row["last_executed_at"])
            last_approval_requested_at = self._coerce_float(row["last_approval_requested_at"])
            last_denied_at = self._coerce_float(row["last_denied_at"])
            usage_score = self._compute_usage_score(
                now_ts=event_ts,
                usage_count=usage_count,
                last_used_at=last_used_at,
                executed_count=executed_count,
                last_executed_at=last_executed_at,
                approval_requested_count=approval_requested_count,
                last_approval_requested_at=last_approval_requested_at,
                denied_count=denied_count,
                last_denied_at=last_denied_at,
            )
            snapshots[skill_id] = SkillUsageSnapshot(
                skill_id=skill_id,
                usage_count=usage_count,
                executed_count=executed_count,
                approval_requested_count=approval_requested_count,
                denied_count=denied_count,
                last_used_at=last_used_at,
                last_executed_at=last_executed_at,
                last_approval_requested_at=last_approval_requested_at,
                last_denied_at=last_denied_at,
                usage_score=usage_score,
            )
        return snapshots

    def _compute_usage_score(
        self,
        *,
        now_ts: float,
        usage_count: int,
        last_used_at: float,
        executed_count: int,
        last_executed_at: float,
        approval_requested_count: int,
        last_approval_requested_at: float,
        denied_count: int,
        last_denied_at: float,
    ) -> float:
        selected_score = self._decayed_count(
            count=usage_count,
            last_ts=last_used_at,
            now_ts=now_ts,
            half_life_days=_RECENCY_HALF_LIFE_DAYS,
            min_factor=_MIN_RECENCY_FACTOR,
        )
        executed_score = 0.35 * self._decayed_count(
            count=executed_count,
            last_ts=last_executed_at,
            now_ts=now_ts,
            half_life_days=_RECENCY_HALF_LIFE_DAYS,
            min_factor=_MIN_RECENCY_FACTOR,
        )
        approval_score = 0.15 * self._decayed_count(
            count=approval_requested_count,
            last_ts=last_approval_requested_at,
            now_ts=now_ts,
            half_life_days=_RECENCY_HALF_LIFE_DAYS,
            min_factor=_MIN_RECENCY_FACTOR,
        )
        denied_penalty = 0.25 * self._decayed_count(
            count=denied_count,
            last_ts=last_denied_at,
            now_ts=now_ts,
            half_life_days=_DENIED_HALF_LIFE_DAYS,
            min_factor=0.0,
        )
        return max(selected_score + executed_score + approval_score - denied_penalty, 0.0)

    def _decayed_count(
        self,
        *,
        count: int,
        last_ts: float,
        now_ts: float,
        half_life_days: float,
        min_factor: float,
    ) -> float:
        if count <= 0 or last_ts <= 0:
            return 0.0
        elapsed_days = max(now_ts - last_ts, 0.0) / _SECONDS_PER_DAY
        recency_factor = math.pow(0.5, elapsed_days / max(half_life_days, 1.0))
        if min_factor > 0:
            recency_factor = max(recency_factor, min_factor)
        return count * recency_factor

    def _normalize_skill_ids(self, skill_ids: Iterable[str]) -> list[str]:
        ordered: list[str] = []
        seen: set[str] = set()
        for raw_item in skill_ids:
            skill_id = str(raw_item or "").strip()
            if not skill_id or skill_id in seen:
                continue
            seen.add(skill_id)
            ordered.append(skill_id)
        return ordered

    def _coerce_int(self, value: object) -> int:
        try:
            return int(value or 0)
        except Exception:
            return 0

    def _coerce_float(self, value: object) -> float:
        try:
            return float(value or 0.0)
        except Exception:
            return 0.0
