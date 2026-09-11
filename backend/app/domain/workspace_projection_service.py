from __future__ import annotations

from datetime import date
from typing import Optional, Sequence

from app.domain.workspace_artifact_renderer import WorkspaceArtifactRenderer
from app.repositories.analysis_repository import AnalysisRepository


class WorkspaceProjectionService:
    """Explicit artifact/projection service.

    Workspace files remain human-readable artifacts only. This service owns
    the explicit render/list entrypoints so factual read paths can stay
    independent from projection generation.
    """

    def __init__(self, repository: AnalysisRepository) -> None:
        self._repository = repository
        self._artifact_renderer = WorkspaceArtifactRenderer()

    def ensure_case_projection(
        self,
        case_id: str,
        *,
        files: Optional[Sequence[str]] = None,
        sync_baseline: bool = True,
        render_mode: str = "manual",
    ) -> list[dict]:
        return self._repository.render_workspace_files(
            case_id,
            list(files or []),
            sync_baseline=sync_baseline,
            render_mode=render_mode,
        )

    def list_case_projection(self, case_id: str) -> list[dict]:
        return self._repository.list_workspace_files(case_id)

    def refresh_projection_files(
        self,
        case_id: str,
        *,
        files: Sequence[str],
        sync_baseline: bool = False,
        render_mode: str = "manual",
    ) -> list[dict]:
        return self.ensure_case_projection(
            case_id,
            files=files,
            sync_baseline=sync_baseline,
            render_mode=render_mode,
        )

    def refresh_workflow_note_projection(
        self,
        case_id: str,
        *,
        day_text: str = "",
        render_mode: str = "workflow_note",
    ) -> list[dict]:
        resolved_day = str(day_text or date.today().isoformat()).strip() or date.today().isoformat()
        return self.refresh_projection_files(
            case_id,
            files=[f"memory/{resolved_day}.md", "EVIDENCE_INDEX.md"],
            sync_baseline=False,
            render_mode=render_mode,
        )

    def refresh_evidence_index_projection(
        self,
        case_id: str,
        *,
        render_mode: str = "system_refresh",
    ) -> list[dict]:
        return self.refresh_projection_files(
            case_id,
            files=["EVIDENCE_INDEX.md"],
            sync_baseline=False,
            render_mode=render_mode,
        )

    def write_projection_file(
        self,
        case_id: str,
        *,
        file_name: str,
        content_md: str,
        render_mode: str,
    ) -> dict:
        return self._repository.write_workspace_projection_file(
            case_id,
            file_name=file_name,
            content_md=content_md,
            render_mode=render_mode,
        )

    def publish_report_projection(
        self,
        case_id: str,
        *,
        scope: str,
        status: str,
        version: int,
        content_md: str,
        render_mode: str = "report_publish",
    ) -> dict:
        file_name = self.build_report_projection_file_name(
            scope=scope,
            status=status,
            version=version,
        )
        return self.write_projection_file(
            case_id,
            file_name=file_name,
            content_md=content_md,
            render_mode=render_mode,
        )

    def build_report_projection_file_name(self, *, scope: str, status: str, version: int) -> str:
        return self._artifact_renderer.build_report_projection_file_name(
            scope=scope,
            status=status,
            version=version,
        )
