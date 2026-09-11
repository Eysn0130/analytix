from __future__ import annotations

from dataclasses import dataclass

from app.core.managed_subprocess import managed_execution_authority_status


MANAGED_EXECUTION_AUTHORITY_UNAVAILABLE = "managed_process_execution_authority_unavailable"
MANAGED_EXECUTION_CAPABILITY_ADMISSION_UNAVAILABLE = (
    "managed_process_capability_admission_unavailable"
)


@dataclass(frozen=True)
class ExecutionAuthorityHealth:
    available: bool
    reason_code: str


def execution_authority_health() -> ExecutionAuthorityHealth:
    """Return the closed, non-sensitive health projection for helper execution."""

    status = managed_execution_authority_status()
    if not status.available:
        return ExecutionAuthorityHealth(
            available=False,
            reason_code=MANAGED_EXECUTION_AUTHORITY_UNAVAILABLE,
        )
    return ExecutionAuthorityHealth(available=True, reason_code="")


def unadmitted_execution_capability_health(
    authority: ExecutionAuthorityHealth,
) -> ExecutionAuthorityHealth:
    """Keep an unbound helper blocked even after a global adapter is installed.

    A helper may become available only after its consumer accepts a versioned
    capability admission issued by the shared execution authority. No backend
    consumer has that receipt contract yet, so a global authority status alone
    cannot re-enable path, manifest, or direct-process probes.
    """

    if not authority.available:
        return ExecutionAuthorityHealth(
            available=False,
            reason_code=MANAGED_EXECUTION_AUTHORITY_UNAVAILABLE,
        )
    return ExecutionAuthorityHealth(
        available=False,
        reason_code=MANAGED_EXECUTION_CAPABILITY_ADMISSION_UNAVAILABLE,
    )
