from __future__ import annotations

import json
import math
from typing import Any


class StrictJSONError(ValueError):
    """Raised when JSON is ambiguous, non-finite, or exceeds fixed bounds."""


def _reject_duplicate_keys(pairs: list[tuple[str, Any]]) -> dict[str, Any]:
    value: dict[str, Any] = {}
    for key, item in pairs:
        if key in value:
            raise StrictJSONError("strict_json_duplicate_key")
        value[key] = item
    return value


def _reject_nonstandard_constant(_value: str) -> None:
    raise StrictJSONError("strict_json_non_finite")


def _validate_json_string(value: str) -> None:
    if any(0xD800 <= ord(character) <= 0xDFFF for character in value):
        raise StrictJSONError("strict_json_invalid_unicode_scalar")


def _validate_json_tree(value: Any, *, max_depth: int, max_nodes: int) -> None:
    remaining = max_nodes
    stack: list[tuple[Any, int]] = [(value, 0)]
    while stack:
        item, depth = stack.pop()
        remaining -= 1
        if remaining < 0:
            raise StrictJSONError("strict_json_too_many_nodes")
        if depth > max_depth:
            raise StrictJSONError("strict_json_too_deep")
        if isinstance(item, float) and not math.isfinite(item):
            raise StrictJSONError("strict_json_non_finite")
        if isinstance(item, dict):
            for key in item:
                if not isinstance(key, str):
                    raise StrictJSONError("strict_json_invalid_key")
                _validate_json_string(key)
            stack.extend((child, depth + 1) for child in item.values())
        elif isinstance(item, list):
            stack.extend((child, depth + 1) for child in item)
        elif isinstance(item, str):
            _validate_json_string(item)
        elif item is not None and not isinstance(item, (str, int, float, bool)):
            raise StrictJSONError("strict_json_invalid_type")


def loads_strict_json(
    value: str | bytes,
    *,
    max_bytes: int,
    max_depth: int = 96,
    max_nodes: int = 1_000_000,
) -> Any:
    """Decode one bounded RFC 8259 value without duplicate-key ambiguity."""

    if (
        not isinstance(max_bytes, int)
        or isinstance(max_bytes, bool)
        or max_bytes <= 0
        or not isinstance(max_depth, int)
        or isinstance(max_depth, bool)
        or max_depth < 0
        or not isinstance(max_nodes, int)
        or isinstance(max_nodes, bool)
        or max_nodes <= 0
    ):
        raise StrictJSONError("strict_json_invalid_bounds")
    try:
        if isinstance(value, bytes):
            if len(value) > max_bytes:
                raise StrictJSONError("strict_json_too_large")
            text = value.decode("utf-8", errors="strict")
        elif isinstance(value, str):
            if len(value.encode("utf-8", errors="strict")) > max_bytes:
                raise StrictJSONError("strict_json_too_large")
            text = value
        else:
            raise StrictJSONError("strict_json_invalid_input")
        decoded = json.loads(
            text,
            object_pairs_hook=_reject_duplicate_keys,
            parse_constant=_reject_nonstandard_constant,
        )
        _validate_json_tree(decoded, max_depth=max_depth, max_nodes=max_nodes)
        return decoded
    except StrictJSONError:
        raise
    except (json.JSONDecodeError, MemoryError, RecursionError, UnicodeError, ValueError):
        raise StrictJSONError("strict_json_invalid") from None


def dumps_canonical_json(
    value: Any,
    *,
    max_depth: int = 96,
    max_nodes: int = 1_000_000,
    max_bytes: int | None = None,
) -> str:
    """Encode finite JSON with deterministic keys and no implementation extensions."""

    if max_bytes is not None and (
        not isinstance(max_bytes, int) or isinstance(max_bytes, bool) or max_bytes <= 0
    ):
        raise StrictJSONError("strict_json_invalid_bounds")
    _validate_json_tree(value, max_depth=max_depth, max_nodes=max_nodes)
    try:
        encoded = json.dumps(
            value,
            ensure_ascii=False,
            sort_keys=True,
            separators=(",", ":"),
            allow_nan=False,
        )
        encoded_bytes = encoded.encode("utf-8", errors="strict")
        if max_bytes is not None and len(encoded_bytes) > max_bytes:
            raise StrictJSONError("strict_json_too_large")
        return encoded
    except StrictJSONError:
        raise
    except (MemoryError, RecursionError, TypeError, UnicodeError, ValueError):
        raise StrictJSONError("strict_json_invalid") from None
