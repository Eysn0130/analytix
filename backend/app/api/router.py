from __future__ import annotations

from fastapi import APIRouter

from app.api.v1 import (
    analysis,
    cases,
    cleaning_insights,
    cleaning_jobs,
    documents,
    export_jobs,
    flow,
    import_files,
    import_jobs,
    stats,
    system,
)

api_router = APIRouter()
api_router.include_router(cases.router)
api_router.include_router(analysis.router)
api_router.include_router(import_jobs.router)
api_router.include_router(import_files.router)
api_router.include_router(cleaning_jobs.router)
api_router.include_router(cleaning_insights.router)
api_router.include_router(export_jobs.router)
api_router.include_router(stats.router)
api_router.include_router(flow.router)
api_router.include_router(documents.router)
api_router.include_router(system.router)
