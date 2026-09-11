from __future__ import annotations

import copy
from typing import Any


_RESERVED_CUE_FIELD = "provider_visible_semantic_cues"


def enrich_provider_visible_privacy_semantics(payload: Any) -> Any:
    """Return a safe copy without minting provider-visible case semantics.

    The Python analysis service does not own the host Evidence Registry and
    therefore cannot prove receipt membership, context binding, semantic
    success, or coverage.  Tool/model supplied flags and prose are untrusted,
    so this boundary may project privacy but must not derive case cues.  The
    reserved cue field is stripped recursively to prevent callers from
    smuggling a self-issued cue through the same channel.
    """

    if not isinstance(payload, (dict, list)):
        return payload
    return _copy_without_reserved_cues(payload)


def _copy_without_reserved_cues(value: Any) -> Any:
    if isinstance(value, dict):
        return {
            copy.deepcopy(key): _copy_without_reserved_cues(child)
            for key, child in value.items()
            if key != _RESERVED_CUE_FIELD
        }
    if isinstance(value, list):
        return [_copy_without_reserved_cues(child) for child in value]
    return copy.deepcopy(value)
