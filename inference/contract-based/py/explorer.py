"""
VeChain Inference Explorer (contract-based) — live dashboard at :8088

Reads InferenceRequested, InferenceResponded, VerdictSubmitted, NodeSlashed
events from InferenceMarketplace.sol and serves a web UI.

  GET /             HTML dashboard (auto-refreshes every 10 s)
  GET /api/events   JSON feed, newest-first
  GET /health       {"scanned_to": N, "requests": M}

Environment:
  THOR_URL   VeChain node URL (default: http://localhost:8669)
"""
from __future__ import annotations

import json
import os
import sys
import threading
import time
from contextlib import asynccontextmanager
from dataclasses import dataclass, asdict
from typing import Optional

sys.path.insert(0, os.path.dirname(__file__))

from fastapi import FastAPI
from fastapi.responses import HTMLResponse

from contract_client import ContractClient, _event_topic
from commitllm.merkle import decode_float32

THOR_URL      = os.getenv("THOR_URL", "http://localhost:8669")
SCAN_INTERVAL = 10


# ── Data model ────────────────────────────────────────────────────────────── #

@dataclass
class InferenceEntry:
    request_id:   str
    requester:    str
    model:        str
    input_hash:   str
    max_tokens:   int
    payment:      int
    req_block:    int
    req_tx:       str

    prover:       Optional[str]   = None
    output_hash:  Optional[str]   = None
    w_root:       Optional[str]   = None
    x_root:       Optional[str]   = None
    z_root:       Optional[str]   = None
    x_dim:        Optional[int]   = None
    resp_block:   Optional[int]   = None
    resp_tx:      Optional[str]   = None

    verdict:      Optional[bool]  = None
    verdict_reason: Optional[str] = None
    verdict_tx:   Optional[str]   = None
    slashed:      bool            = False

    @property
    def status(self) -> str:
        if self.slashed:
            return "SLASHED"
        if self.verdict is True:
            return "VALID"
        if self.verdict is False:
            return "FRAUD"
        if self.prover:
            return "RESPONDED"
        return "PENDING"

    def to_dict(self) -> dict:
        d = asdict(self)
        d["status"] = self.status
        return d


# ── Store ─────────────────────────────────────────────────────────────────── #

class InferenceStore:
    def __init__(self):
        self._lock    = threading.Lock()
        self.entries: dict[str, InferenceEntry] = {}
        self.scanned_to  = -1
        self.is_scanning = True

    def add_request(self, e: dict) -> None:
        rid = e["requestId"]
        with self._lock:
            if rid not in self.entries:
                self.entries[rid] = InferenceEntry(
                    request_id=rid, requester=e["requester"], model=e["model"],
                    input_hash=e["inputHash"], max_tokens=e["maxNewTokens"],
                    payment=e["payment"], req_block=e["blockNumber"], req_tx=e["txId"],
                )

    def add_response(self, e: dict) -> None:
        rid = e["requestId"]
        with self._lock:
            entry = self.entries.get(rid)
            if entry and entry.prover is None:
                x_dim = len(decode_float32(e["xEncoded"])) if e.get("xEncoded") else 0
                entry.prover      = e["prover"]
                entry.output_hash = e["outputHash"]
                entry.w_root      = e["modelCommitment"]
                entry.x_root      = e["xRoot"]
                entry.z_root      = e["zRoot"]
                entry.x_dim       = x_dim
                entry.resp_block  = e["blockNumber"]
                entry.resp_tx     = e["txId"]

    def add_verdict(self, e: dict) -> None:
        rid = e["requestId"]
        with self._lock:
            entry = self.entries.get(rid)
            if entry and entry.verdict is None:
                entry.verdict        = e["valid"]
                entry.verdict_reason = e["reason"]
                entry.verdict_tx     = e["txId"]

    def mark_slashed(self, request_id: str) -> None:
        with self._lock:
            entry = self.entries.get(request_id)
            if entry:
                entry.slashed = True

    def snapshot(self) -> tuple[bool, int, list[InferenceEntry]]:
        with self._lock:
            return (
                self.is_scanning, self.scanned_to,
                sorted(self.entries.values(), key=lambda e: e.req_block, reverse=True),
            )


# ── Scanner ───────────────────────────────────────────────────────────────── #

class EventScanner:
    def __init__(self, store: InferenceStore, thor_url: str):
        self.store  = store
        self.client = ContractClient(thor_url)

    def scan_range(self, from_block: int, to_block: int) -> None:
        req_sig  = "InferenceRequested(bytes32,address,string,bytes32,bytes32,uint32,uint256)"
        resp_sig = "InferenceResponded(bytes32,address,bytes32,bytes32,bytes32,bytes32,bytes,bytes)"
        verd_sig = "VerdictSubmitted(bytes32,address,bool,string)"

        for sig, parser, adder in [
            (req_sig,  self.client.parse_inference_requested, self.store.add_request),
            (resp_sig, self.client.parse_inference_responded, self.store.add_response),
            (verd_sig, self.client.parse_verdict_submitted,   self.store.add_verdict),
        ]:
            try:
                logs = self.client.get_events(sig, from_block, to_block)
                for log in logs:
                    try:
                        adder(parser(log))
                    except Exception:
                        pass
            except Exception:
                pass

        with self.store._lock:
            self.store.scanned_to = max(self.store.scanned_to, to_block)

    def run(self) -> None:
        try:
            best = int(self.client.thor.best_block()["number"])
            self.scan_range(0, best)
        except Exception:
            pass
        with self.store._lock:
            self.store.is_scanning = False

        while True:
            time.sleep(SCAN_INTERVAL)
            try:
                best    = int(self.client.thor.best_block()["number"])
                current = self.store.scanned_to
                if best > current:
                    self.scan_range(current + 1, best)
            except Exception:
                pass


# ── FastAPI ───────────────────────────────────────────────────────────────── #

store = InferenceStore()


@asynccontextmanager
async def lifespan(app: FastAPI):
    t = threading.Thread(target=EventScanner(store, THOR_URL).run, daemon=True)
    t.start()
    yield


app = FastAPI(title="VeChain Inference Explorer (V1 Contract)", lifespan=lifespan)


@app.get("/", response_class=HTMLResponse)
def dashboard():
    return _HTML


@app.get("/api/events")
def api_events():
    scanning, scanned_to, entries = store.snapshot()
    return {
        "status":     "scanning" if scanning else "ready",
        "node":       THOR_URL,
        "scanned_to": max(scanned_to, 0),
        "entries":    [e.to_dict() for e in entries],
    }


@app.get("/health")
def health():
    return {
        "status":       "ok",
        "is_scanning":  store.is_scanning,
        "scanned_to":   store.scanned_to,
        "requests":     len(store.entries),
    }


# ── Minimal HTML ──────────────────────────────────────────────────────────── #

_HTML = """<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="utf-8">
<title>VeChain Inference Explorer — V1 Contract</title>
<style>
:root{--bg:#0d1117;--bg2:#161b22;--border:#30363d;--text:#c9d1d9;--muted:#6e7681;
      --blue:#388bfd;--green:#2ea043;--yellow:#d29922;--red:#f85149}
*{box-sizing:border-box;margin:0;padding:0}
body{background:var(--bg);color:var(--text);font-family:'Courier New',monospace;font-size:13px}
.hdr{background:var(--bg2);border-bottom:1px solid var(--border);padding:10px 20px;
     display:flex;align-items:center;justify-content:space-between}
.hdr-title{color:#58a6ff;font-size:15px;font-weight:bold}
.hdr-meta{color:var(--muted);font-size:11px}
table{width:100%;border-collapse:collapse}
th{background:var(--bg2);color:var(--muted);text-transform:uppercase;font-size:10px;
   padding:6px 10px;text-align:left;border-bottom:2px solid var(--border)}
td{padding:6px 10px;border-bottom:1px solid var(--border);vertical-align:top;
   overflow:hidden;text-overflow:ellipsis;white-space:nowrap}
.bdg{display:inline-block;padding:1px 7px;border-radius:3px;font-size:10px;font-weight:bold}
.bdg-pending  {background:#2d2208;color:var(--yellow)}
.bdg-responded{background:#0c2140;color:var(--blue)}
.bdg-valid    {background:#1e3a24;color:var(--green)}
.bdg-fraud    {background:#3d0c0c;color:var(--red)}
.bdg-slashed  {background:#3d0c0c;color:var(--red)}
.loading{padding:60px;text-align:center;color:var(--muted)}
</style>
</head>
<body>
<div class="hdr">
  <span class="hdr-title">&#9741; VeChain Inference Explorer — V1 Contract</span>
  <span id="meta" class="hdr-meta">connecting&hellip;</span>
</div>
<div id="main"><div class="loading">Connecting&hellip;</div></div>
<script>
const esc = s => s==null?'':String(s).replace(/&/g,'&amp;').replace(/</g,'&lt;');
const bdg = (cls,lbl) => `<span class="bdg bdg-${cls}">${lbl}</span>`;
const statusBadge = s => ({
  PENDING:   bdg('pending','pending'),
  RESPONDED: bdg('responded','responded'),
  VALID:     bdg('valid','valid ✓'),
  FRAUD:     bdg('fraud','fraud ✗'),
  SLASHED:   bdg('slashed','slashed'),
}[s] ?? bdg('pending',s));

async function load() {
  try {
    const d = await fetch('/api/events').then(r=>r.json());
    document.getElementById('meta').textContent =
      `${d.node} · ${d.status} · block #${d.scanned_to} · ${d.entries.length} requests`;

    if (!d.entries.length) {
      document.getElementById('main').innerHTML = '<div class="loading">No requests yet.</div>';
      return;
    }
    let html = `<table><thead><tr>
      <th>Block</th><th>Request ID</th><th>Model</th>
      <th>Status</th><th>W_root</th><th>Verdict reason</th>
    </tr></thead><tbody>`;
    for (const e of d.entries) {
      html += `<tr>
        <td>#${e.req_block}</td>
        <td>${esc(e.request_id.slice(0,12))}…</td>
        <td>${esc(e.model.split('/').pop())}</td>
        <td>${statusBadge(e.status)}</td>
        <td>${esc(e.w_root ? e.w_root.slice(0,14)+'…' : '—')}</td>
        <td>${esc(e.verdict_reason ?? '—')}</td>
      </tr>`;
    }
    html += '</tbody></table>';
    document.getElementById('main').innerHTML = html;
  } catch(e) {
    document.getElementById('meta').textContent = 'connection error';
  }
}
load(); setInterval(load, 10000);
</script>
</body>
</html>"""
