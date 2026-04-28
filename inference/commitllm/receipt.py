from dataclasses import dataclass, asdict
import json
import hashlib


@dataclass
class LayerTrace:
    x: list[float]  # input activations at last token position
    z: list[float]  # output activations at last token position


@dataclass
class Receipt:
    model_name: str
    model_commitment: str   # sha256 hex fingerprint of sampled model weights
    input_hash: str         # sha256 hex of prompt text
    output_hash: str        # sha256 hex of output text
    layer_traces: dict[str, LayerTrace]  # {layer_name: LayerTrace}

    def to_json(self) -> str:
        # Round activations to 6 significant figures so receipts fit in a
        # VeChain transaction (full float64 precision would be ~600KB per receipt;
        # 6 sig-figs reduces it to ~220KB while Freivalds tolerance remains <0.01%)
        d = asdict(self)
        for trace in d["layer_traces"].values():
            trace["x"] = [float(f"{v:.6g}") for v in trace["x"]]
            trace["z"] = [float(f"{v:.6g}") for v in trace["z"]]
        return json.dumps(d, separators=(",", ":"))

    @classmethod
    def from_json(cls, s: str) -> "Receipt":
        d = json.loads(s)
        d["layer_traces"] = {k: LayerTrace(**v) for k, v in d["layer_traces"].items()}
        return cls(**d)

    def to_bytes(self) -> bytes:
        return self.to_json().encode("utf-8")

    @classmethod
    def from_bytes(cls, b: bytes) -> "Receipt":
        return cls.from_json(b.decode("utf-8"))

    def commitment_hash(self) -> str:
        return hashlib.sha256(self.to_bytes()).hexdigest()
