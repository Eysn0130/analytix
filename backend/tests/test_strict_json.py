from __future__ import annotations

import pytest

from app.utils.strict_json import StrictJSONError, dumps_canonical_json, loads_strict_json


def test_strict_json_rejects_duplicate_keys_and_nonfinite_values() -> None:
    with pytest.raises(StrictJSONError, match="strict_json_duplicate_key"):
        loads_strict_json('{"case_id":"case-a","case_id":"case-b"}', max_bytes=1024)
    for payload in ('{"value":NaN}', '{"value":Infinity}', '{"value":1e309}'):
        with pytest.raises(StrictJSONError, match="strict_json_non_finite"):
            loads_strict_json(payload, max_bytes=1024)


def test_strict_json_rejects_non_string_keys_and_unpaired_surrogates() -> None:
    with pytest.raises(StrictJSONError, match="strict_json_invalid_key"):
        dumps_canonical_json({1: "must-not-be-coerced"})
    with pytest.raises(StrictJSONError, match="strict_json_invalid_unicode_scalar"):
        loads_strict_json('{"value":"\\ud800"}', max_bytes=1024)
    with pytest.raises(StrictJSONError, match="strict_json_invalid_unicode_scalar"):
        dumps_canonical_json({"value": "\udfff"})


def test_canonical_encoder_enforces_utf8_byte_limit() -> None:
    assert dumps_canonical_json({"value": "案"}, max_bytes=15) == '{"value":"案"}'
    with pytest.raises(StrictJSONError, match="strict_json_too_large"):
        dumps_canonical_json({"value": "案"}, max_bytes=14)
