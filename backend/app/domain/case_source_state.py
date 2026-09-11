from __future__ import annotations


class CaseSourceUnavailableError(RuntimeError):
    """A case-bound source cannot currently support an authoritative read."""


__all__ = ["CaseSourceUnavailableError"]
