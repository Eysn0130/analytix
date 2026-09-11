from __future__ import annotations

import json

import pytest

from app.core import data_engine_client


def test_data_engine_request_encoder_is_deterministic_and_strict() -> None:
    first = {"z": "中\n\"", "a": [True, None, 7, 1.25]}
    second = {"a": [True, None, 7, 1.25], "z": "中\n\""}

    first_line, first_error = data_engine_client._encode_request_line(first)
    second_line, second_error = data_engine_client._encode_request_line(second)
    invalid_line, invalid_error = data_engine_client._encode_request_line(
        {"value": float("nan")}
    )

    assert first_error is None
    assert second_error is None
    assert first_line == second_line
    assert first_line is not None
    assert first_line.endswith(b"\n")
    assert json.loads(first_line) == first
    assert invalid_line is None
    assert invalid_error == "data_engine_request_invalid"


@pytest.mark.parametrize(
    ("limit_name", "limit_value", "payload"),
    (
        ("_MAX_REQUEST_JSON_DEPTH", 4, {"a": {"b": {"c": {"d": {"e": {}}}}}}),
        ("_MAX_REQUEST_JSON_STRING_CHARS", 32, {"secret": "6" * 128}),
        ("_MAX_REQUEST_JSON_NODES", 16, {"items": list(range(100))}),
        ("_MAX_REQUEST_LINE_BYTES", 32, {"items": ["x" * 128]}),
    ),
)
def test_data_engine_request_encoder_rejects_bounded_structure(
    monkeypatch: pytest.MonkeyPatch,
    limit_name: str,
    limit_value: int,
    payload: dict,
) -> None:
    monkeypatch.setattr(data_engine_client, limit_name, limit_value)

    encoded, error = data_engine_client._encode_request_line(payload)

    assert encoded is None
    assert error == "data_engine_request_limit_exceeded"


@pytest.mark.parametrize(
    "payload",
    (
        {1: "non-string key"},
        {"value": object()},
        {"value": float("inf")},
        {"value": "\ud800"},
    ),
)
def test_data_engine_request_encoder_rejects_non_json_values(payload: dict) -> None:
    encoded, error = data_engine_client._encode_request_line(payload)

    assert encoded is None
    assert error == "data_engine_request_invalid"


@pytest.mark.parametrize(
    ("limit_name", "limit_value", "response"),
    (
        ("_MAX_RESPONSE_JSON_DEPTH", 3, b"[[[[0]]]]"),
        ("_MAX_RESPONSE_JSON_STRING_BYTES", 3, b'{"value":"abcd"}'),
        ("_MAX_RESPONSE_JSON_NODES", 3, b"[0,0]"),
    ),
)
def test_data_engine_response_shape_is_bounded_before_json_loads(
    monkeypatch: pytest.MonkeyPatch,
    limit_name: str,
    limit_value: int,
    response: bytes,
) -> None:
    monkeypatch.setattr(data_engine_client, limit_name, limit_value)

    with pytest.raises(data_engine_client.DataEngineUnavailableError) as raised:
        data_engine_client._decode_response_frame(response + b"\n")

    assert raised.value.code == "data_engine_response_limit_exceeded"


def test_data_engine_response_shape_accepts_a_bounded_complete_envelope() -> None:
    response = b'{"request_id":"1","ok":true,"data":{}}'

    decoded = data_engine_client._decode_response_frame(response + b"\n")

    assert decoded == response.decode("utf-8")


@pytest.mark.parametrize("response", (b'{"value":[1}', b'{"value":"unterminated}'))
def test_data_engine_response_shape_distinguishes_malformed_json(response: bytes) -> None:
    with pytest.raises(data_engine_client.DataEngineUnavailableError) as raised:
        data_engine_client._decode_response_frame(response + b"\n")

    assert raised.value.code == "data_engine_invalid_response_json"


@pytest.mark.parametrize(
    "response",
    (
        '{"request_id":"1","request_id":"2","ok":true}',
        '{"request_id":"1","ok":true,"data":{"value":1,"value":2}}',
        '{"request_id":"1","ok":true,"data":{"value":NaN}}',
        '{"request_id":"1","ok":true,"data":{"value":Infinity}}',
    ),
)
def test_data_engine_response_json_rejects_duplicates_and_non_finite_numbers(response: str) -> None:
    with pytest.raises(data_engine_client._ResponseJSONInvalid):
        data_engine_client._strict_response_json_loads(response)
