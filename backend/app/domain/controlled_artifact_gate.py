from __future__ import annotations


CONTROLLED_ARTIFACT_PUBLICATION_REQUIRED = "controlled_artifact_publication_required"
CONTROLLED_SOURCE_INGESTION_REQUIRED = "controlled_source_ingestion_required"


class ControlledArtifactPublicationRequiredError(RuntimeError):
    def __init__(self) -> None:
        super().__init__(CONTROLLED_ARTIFACT_PUBLICATION_REQUIRED)


class ControlledSourceIngestionRequiredError(RuntimeError):
    def __init__(self) -> None:
        super().__init__(CONTROLLED_SOURCE_INGESTION_REQUIRED)


def require_controlled_artifact_publication() -> None:
    raise ControlledArtifactPublicationRequiredError()


def require_controlled_source_ingestion() -> None:
    raise ControlledSourceIngestionRequiredError()
