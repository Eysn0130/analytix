from __future__ import annotations

from app.api.router import api_router
from app.api.v1 import system


def test_api_router_exposes_system_task_lookup() -> None:
    system_paths = {getattr(route, "path", "") for route in system.router.routes}

    assert any(getattr(route, "original_router", None) is system.router for route in api_router.routes)
    assert "/system/tasks/{task_id}" in system_paths
