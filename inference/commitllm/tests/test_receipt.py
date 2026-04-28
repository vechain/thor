import json
import pytest

from commitllm.receipt import LayerTrace, Receipt


def _make_receipt(**overrides) -> Receipt:
    defaults = dict(
        model_name="test-model",
        model_commitment="a" * 64,
        input_hash="b" * 64,
        output_hash="c" * 64,
        layer_traces={
            "model.layers.0.q_proj": LayerTrace(x=[1.0, 2.0], z=[3.0, 4.0]),
        },
    )
    defaults.update(overrides)
    return Receipt(**defaults)


def test_json_roundtrip():
    r = _make_receipt()
    r2 = Receipt.from_json(r.to_json())
    assert r2.model_name == r.model_name
    assert r2.model_commitment == r.model_commitment
    assert r2.input_hash == r.input_hash
    assert r2.output_hash == r.output_hash
    assert list(r2.layer_traces.keys()) == list(r.layer_traces.keys())
    trace = r2.layer_traces["model.layers.0.q_proj"]
    assert trace.x == [1.0, 2.0]
    assert trace.z == [3.0, 4.0]


def test_bytes_roundtrip():
    r = _make_receipt()
    r2 = Receipt.from_bytes(r.to_bytes())
    assert r2 == r


def test_commitment_hash_stable():
    r = _make_receipt()
    assert r.commitment_hash() == r.commitment_hash()


def test_commitment_hash_changes_on_mutation():
    r1 = _make_receipt()
    r2 = _make_receipt(output_hash="d" * 64)
    assert r1.commitment_hash() != r2.commitment_hash()


def test_json_is_valid_json():
    r = _make_receipt()
    parsed = json.loads(r.to_json())
    assert "model_name" in parsed
    assert "layer_traces" in parsed


def test_empty_traces():
    r = _make_receipt(layer_traces={})
    r2 = Receipt.from_json(r.to_json())
    assert r2.layer_traces == {}


def test_multiple_traces_roundtrip():
    traces = {
        "model.layers.0.q_proj": LayerTrace(x=[1.0], z=[2.0]),
        "model.layers.12.q_proj": LayerTrace(x=[3.0, 4.0], z=[5.0, 6.0]),
        "model.layers.24.q_proj": LayerTrace(x=[7.0], z=[8.0]),
    }
    r = _make_receipt(layer_traces=traces)
    r2 = Receipt.from_bytes(r.to_bytes())
    assert len(r2.layer_traces) == 3
    assert r2.layer_traces["model.layers.12.q_proj"].x == [3.0, 4.0]
