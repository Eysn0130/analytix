from __future__ import annotations

import re
from typing import Iterable

_ACCOUNT_MENTION_RE = re.compile(r"^@(\d{6,})$")
_ACCOUNT_MENTION_TOKEN_RE = re.compile(r"(?<![A-Za-z0-9_])@(\d{6,})(?![A-Za-z0-9_])")


def normalize_account_key_token(value: object) -> str:
    text = str(value or "").strip()
    if not text:
        return ""
    match = _ACCOUNT_MENTION_RE.fullmatch(text)
    return str(match.group(1) or "").strip() if match else text


def normalize_account_keys(values: Iterable[object]) -> list[str]:
    seen: set[str] = set()
    normalized: list[str] = []
    for value in values:
        account_key = normalize_account_key_token(value)
        if not account_key or account_key in seen:
            continue
        seen.add(account_key)
        normalized.append(account_key)
    return normalized


def extract_account_key_mentions(value: object) -> list[str]:
    text = str(value or "")
    if not text:
        return []
    return normalize_account_keys(match.group(1) for match in _ACCOUNT_MENTION_TOKEN_RE.finditer(text))
