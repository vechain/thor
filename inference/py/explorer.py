"""
VeChain Inference Explorer — live dashboard at :8088

Scans all VeChain blocks from genesis, decodes INFR/INRS/INCH messages,
groups them by request_id, and serves a web UI + JSON API.

  GET /             HTML dashboard (auto-refreshes every 10 s)
  GET /api/events   JSON feed, newest-first
  GET /health       {"scanned_to": N, "conversations": M}

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

from protocol import decode_message, InferenceRequest, InferenceResponse, InferenceChallenge
from vechain_client import ThorClient

THOR_URL      = os.getenv("THOR_URL", "http://localhost:8669")
SCAN_INTERVAL = 10   # seconds between incremental scans
_MAGICS       = {b"INFR", b"INRS", b"INCH"}


# ── Data model ───────────────────────────────────────────────────────────── #

@dataclass
class ProofDetails:
    model_commitment: str
    input_hash: str
    output_hash: str
    layer_name: str
    layer_x_dim: int
    layer_z_dim: int
    receipt_size_bytes: int


@dataclass
class ConversationEntry:
    request_id: str
    infr_block: int
    infr_timestamp: int
    infr_tx: str
    source: str                       # "USER" | "INTERNAL"
    model: str
    prompt: str
    max_new_tokens: int
    inrs_block: Optional[int]         = None
    inrs_tx: Optional[str]            = None
    output: Optional[str]             = None
    proof: Optional[ProofDetails]     = None
    inch_tx: Optional[str]            = None
    verdict: Optional[bool]           = None
    verdict_reason: Optional[str]     = None

    @property
    def status(self) -> str:
        if self.verdict is not None:
            return "VALID" if self.verdict else "FRAUD"
        return "RESPONDED" if self.inrs_tx else "PENDING"

    def to_dict(self) -> dict:
        return {
            "request_id":     self.request_id,
            "infr_block":     self.infr_block,
            "infr_timestamp": self.infr_timestamp,
            "infr_tx":        self.infr_tx,
            "source":         self.source,
            "model":          self.model,
            "prompt":         self.prompt,
            "max_new_tokens": self.max_new_tokens,
            "status":         self.status,
            "inrs_block":     self.inrs_block,
            "inrs_tx":        self.inrs_tx,
            "output":         self.output,
            "proof":          asdict(self.proof) if self.proof else None,
            "inch_tx":        self.inch_tx,
            "verdict":        self.verdict,
            "verdict_reason": self.verdict_reason,
        }


# ── In-memory store ──────────────────────────────────────────────────────── #

class InferenceStore:
    def __init__(self):
        self._lock           = threading.Lock()
        self.conversations   : dict[str, ConversationEntry] = {}
        self._inrs_tx_to_rid : dict[str, str] = {}   # inrs_tx_id → request_id
        self._pending_inrs   : list = []              # INRS seen before INFR
        self._pending_inch   : list = []              # INCH seen before INRS
        self.scanned_to      : int  = -1
        self.is_scanning     : bool = True

    def process_messages(self, block_num: int, block_ts: int,
                         messages: list[tuple[str, object]]) -> None:
        with self._lock:
            for tx_id, msg in messages:
                if isinstance(msg, InferenceRequest):
                    self._add_infr(block_num, block_ts, tx_id, msg)
                elif isinstance(msg, InferenceResponse):
                    self._add_inrs(block_num, block_ts, tx_id, msg)
                elif isinstance(msg, InferenceChallenge):
                    self._add_inch(tx_id, msg)
            self._flush_pending()

    def mark_scanned(self, block_num: int) -> None:
        with self._lock:
            self.scanned_to = max(self.scanned_to, block_num)

    def _add_infr(self, block_num, block_ts, tx_id, msg: InferenceRequest) -> None:
        if msg.request_id in self.conversations:
            return
        src = "INTERNAL" if msg.prompt.startswith("### Task:") else "USER"
        self.conversations[msg.request_id] = ConversationEntry(
            request_id    = msg.request_id,
            infr_block    = block_num,
            infr_timestamp= block_ts,
            infr_tx       = tx_id,
            source        = src,
            model         = msg.model,
            prompt        = msg.prompt,
            max_new_tokens= msg.max_new_tokens,
        )

    def _add_inrs(self, block_num, block_ts, tx_id, msg: InferenceResponse) -> None:
        entry = self.conversations.get(msg.request_id)
        if entry is None:
            self._pending_inrs.append((block_num, block_ts, tx_id, msg))
            return
        if entry.inrs_tx is not None:
            return  # duplicate — first wins
        entry.inrs_block = block_num
        entry.inrs_tx    = tx_id
        entry.output     = msg.output
        entry.proof      = _parse_proof(msg.receipt_json)
        self._inrs_tx_to_rid[tx_id] = msg.request_id

    def _add_inch(self, tx_id, msg: InferenceChallenge) -> None:
        rid = self._inrs_tx_to_rid.get(msg.response_tx)
        if rid is None:
            self._pending_inch.append((tx_id, msg))
            return
        entry = self.conversations.get(rid)
        if entry and entry.inch_tx is None:
            entry.inch_tx        = tx_id
            entry.verdict        = msg.valid
            entry.verdict_reason = msg.reason

    def _flush_pending(self) -> None:
        still = []
        for n, ts, tx, msg in self._pending_inrs:
            if msg.request_id in self.conversations:
                self._add_inrs(n, ts, tx, msg)
            else:
                still.append((n, ts, tx, msg))
        self._pending_inrs = still

        still = []
        for tx_id, msg in self._pending_inch:
            rid = self._inrs_tx_to_rid.get(msg.response_tx)
            if rid:
                entry = self.conversations.get(rid)
                if entry and entry.inch_tx is None:
                    entry.inch_tx        = tx_id
                    entry.verdict        = msg.valid
                    entry.verdict_reason = msg.reason
            else:
                still.append((tx_id, msg))
        self._pending_inch = still

    def snapshot(self) -> tuple[bool, int, list[ConversationEntry]]:
        with self._lock:
            return (
                self.is_scanning,
                self.scanned_to,
                sorted(self.conversations.values(),
                       key=lambda e: e.infr_block, reverse=True),
            )


def _parse_proof(receipt_json: str) -> Optional[ProofDetails]:
    try:
        r      = json.loads(receipt_json)
        traces = r.get("layer_traces", {})
        name, xd, zd = "", 0, 0
        if traces:
            name = next(iter(traces))
            xd   = len(traces[name].get("x", []))
            zd   = len(traces[name].get("z", []))
        return ProofDetails(
            model_commitment   = r.get("model_commitment", ""),
            input_hash         = r.get("input_hash", ""),
            output_hash        = r.get("output_hash", ""),
            layer_name         = name,
            layer_x_dim        = xd,
            layer_z_dim        = zd,
            receipt_size_bytes = len(receipt_json.encode()),
        )
    except Exception:
        return None


# ── Block scanner ────────────────────────────────────────────────────────── #

class BlockScanner:
    def __init__(self, store: InferenceStore, thor_url: str):
        self.store  = store
        self.client = ThorClient(thor_url)

    def _scan_one(self, block_num: int) -> None:
        # Cheap header check — skip blocks with no transactions
        header = self.client.get_block_header(str(block_num))
        if header.get("gasUsed", 0) == 0:
            return

        block    = self.client.get_block(str(block_num))
        block_ts = block.get("timestamp", 0)
        messages : list[tuple[str, object]] = []

        for tx in (block.get("transactions") or []):
            tx_id = tx.get("id", "")
            for clause in (tx.get("clauses") or []):
                raw_hex = clause.get("data", "0x")
                if len(raw_hex) < 10:
                    continue
                raw = bytes.fromhex(raw_hex[2:])
                if raw[:4] not in _MAGICS:
                    continue
                msg = decode_message(raw)
                if msg is not None:
                    messages.append((tx_id, msg))

        if messages:
            self.store.process_messages(block_num, block_ts, messages)

    def scan_range(self, from_block: int, to_block: int) -> None:
        for n in range(from_block, to_block + 1):
            try:
                self._scan_one(n)
            except Exception:
                pass
            if n % 50 == 0:
                self.store.mark_scanned(n)
        self.store.mark_scanned(to_block)

    def run(self) -> None:
        try:
            best = int(self.client.best_block()["number"])
            self.scan_range(0, best)
        except Exception:
            pass
        with self.store._lock:
            self.store.is_scanning = False

        while True:
            time.sleep(SCAN_INTERVAL)
            try:
                best    = int(self.client.best_block()["number"])
                current = self.store.scanned_to
                if best > current:
                    self.scan_range(current + 1, best)
            except Exception:
                pass


# ── FastAPI ──────────────────────────────────────────────────────────────── #

store = InferenceStore()


@asynccontextmanager
async def lifespan(app: FastAPI):
    t = threading.Thread(target=BlockScanner(store, THOR_URL).run, daemon=True)
    t.start()
    yield


app = FastAPI(title="VeChain Inference Explorer", lifespan=lifespan)


@app.get("/", response_class=HTMLResponse)
def dashboard():
    return _HTML


@app.get("/api/events")
def api_events():
    scanning, scanned_to, entries = store.snapshot()
    return {
        "status":        "scanning" if scanning else "ready",
        "node":          THOR_URL,
        "scanned_to":    max(scanned_to, 0),
        "updated_at":    int(time.time()),
        "conversations": [e.to_dict() for e in entries],
    }


@app.get("/health")
def health():
    return {
        "status":        "ok",
        "is_scanning":   store.is_scanning,
        "scanned_to":    store.scanned_to,
        "conversations": len(store.conversations),
    }


# ── HTML dashboard ───────────────────────────────────────────────────────── #

_HTML = """<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="utf-8">
<title>VeChain Inference Explorer</title>
<style>
:root{
  --bg:#0d1117;--bg2:#161b22;--bg3:#1c2128;--border:#30363d;--text:#c9d1d9;
  --muted:#6e7681;--blue:#388bfd;--green:#2ea043;--yellow:#d29922;--red:#f85149;
  --bg-green:#1e3a24;--bg-yellow:#2d2208;--bg-blue:#0c2140;--bg-red:#3d0c0c;--bg-gray:#1f2229;
}
*{box-sizing:border-box;margin:0;padding:0}
body{background:var(--bg);color:var(--text);font-family:'Courier New',monospace;font-size:13px}

/* ── Header ── */
.hdr{background:var(--bg2);border-bottom:1px solid var(--border);
     padding:10px 20px;display:flex;align-items:center;justify-content:space-between}
.hdr-title{color:#58a6ff;font-size:15px;font-weight:bold}
.hdr-meta{color:var(--muted);font-size:11px;text-align:right}

/* ── Stats ── */
.stats{display:flex;background:var(--bg2);border-bottom:1px solid var(--border)}
.stat{flex:1;text-align:center;padding:8px 4px;border-right:1px solid var(--border)}
.stat:last-child{border-right:none}
.stat-n{font-size:20px;font-weight:bold}
.stat-l{font-size:10px;color:var(--muted);text-transform:uppercase;letter-spacing:.5px}

/* ── Scan note ── */
.scan-note{padding:6px 20px;background:var(--bg-yellow);color:var(--yellow);
           font-size:11px;border-bottom:1px solid var(--border)}

/* ── Table ── */
table{width:100%;border-collapse:collapse;table-layout:fixed}
col.c-blk{width:62px} col.c-time{width:112px} col.c-src{width:80px}
col.c-model{width:110px} col.c-prompt{width:22%} col.c-status{width:90px}
col.c-resp{width:22%} col.c-tog{width:36px}
th{background:var(--bg2);color:var(--muted);text-transform:uppercase;font-size:10px;
   padding:6px 10px;text-align:left;border-bottom:2px solid var(--border);letter-spacing:.4px}
td{padding:6px 10px;border-bottom:none;vertical-align:top;
   overflow:hidden;text-overflow:ellipsis;white-space:nowrap}
tr.data-row td{border-bottom:none;cursor:pointer}
tr.data-row:hover td{background:rgba(255,255,255,.03)}
tr.sep-row td{padding:0;border-bottom:1px solid var(--border);height:1px}
tr.detail-row td{background:var(--bg3);border-bottom:1px solid var(--border);
                 padding:14px 20px 16px;white-space:normal;vertical-align:top}

/* ── Badges ── */
.bdg{display:inline-block;padding:1px 7px;border-radius:3px;font-size:10px;
     font-weight:bold;text-transform:uppercase;letter-spacing:.3px;white-space:nowrap}
.bdg-user     {background:var(--bg-green);color:var(--green)}
.bdg-internal {background:var(--bg-gray);color:var(--muted)}
.bdg-pending  {background:var(--bg-yellow);color:var(--yellow)}
.bdg-responded{background:var(--bg-blue);color:var(--blue)}
.bdg-valid    {background:var(--bg-green);color:var(--green)}
.bdg-fraud    {background:var(--bg-red);color:var(--red)}

/* ── Detail panel ── */
.dp-cols{display:grid;grid-template-columns:1fr 1fr;gap:16px;margin-bottom:12px}
.dp-section{color:var(--muted);text-transform:uppercase;font-size:10px;
            letter-spacing:.5px;margin:12px 0 6px}
.dp-section:first-child{margin-top:0}
.dp-grid{display:grid;grid-template-columns:170px 1fr;row-gap:4px;column-gap:14px}
.dp-key{color:var(--muted);white-space:nowrap}
.dp-val{color:var(--text);word-break:break-all}
.dp-text{white-space:pre-wrap;background:var(--bg2);border:1px solid var(--border);
         border-radius:4px;padding:8px;max-height:180px;overflow-y:auto;
         word-break:break-word;margin-top:4px}
.tog{text-align:center;color:var(--muted);font-size:13px;user-select:none}
.loading{padding:60px;text-align:center;color:var(--muted)}
</style>
</head>
<body>

<div class="hdr">
  <span class="hdr-title">&#9741; VeChain Inference Explorer</span>
  <span id="hdr-meta" class="hdr-meta">connecting&hellip;</span>
</div>

<div id="stats" class="stats">
  <div class="stat"><div class="stat-n">&mdash;</div><div class="stat-l">requests</div></div>
  <div class="stat"><div class="stat-n">&mdash;</div><div class="stat-l">responded</div></div>
  <div class="stat"><div class="stat-n">&mdash;</div><div class="stat-l">pending</div></div>
  <div class="stat"><div class="stat-n" style="color:var(--green)">&mdash;</div><div class="stat-l">verified</div></div>
  <div class="stat"><div class="stat-n" style="color:var(--red)">&mdash;</div><div class="stat-l">fraud</div></div>
</div>

<div id="scan-note" class="scan-note" style="display:none">
  Scanning chain history &mdash; data will appear as blocks are processed&hellip;
</div>

<div id="main"><div class="loading">Connecting to node&hellip;</div></div>

<script>
const expanded = new Set();

const esc = s => s == null ? '' : String(s)
  .replace(/&/g,'&amp;').replace(/</g,'&lt;').replace(/>/g,'&gt;').replace(/"/g,'&quot;');

const ts  = t => new Date(t*1000).toISOString().replace('T',' ').slice(0,16)+' UTC';
const bdg = (cls,lbl) => `<span class="bdg bdg-${cls}">${lbl}</span>`;
const srcBadge = s => s==='USER' ? bdg('user','user') : bdg('internal','internal');
const statusBadge = s => ({
  PENDING:   bdg('pending','pending'),
  RESPONDED: bdg('responded','responded'),
  VALID:     bdg('valid','valid ✓'),
  FRAUD:     bdg('fraud','fraud ✗'),
}[s] ?? bdg('pending', s));

function toggle(rid) {
  const d = document.getElementById('d-'+rid);
  const t = document.getElementById('t-'+rid);
  if (!d) return;
  if (expanded.has(rid)) {
    expanded.delete(rid); d.style.display='none'; if(t) t.textContent='▶';
  } else {
    expanded.add(rid); d.style.display=''; if(t) t.textContent='▼';
  }
}

function proofHtml(p) {
  if (!p) return '';
  return `
    <div class="dp-section">CommitLLM Proof</div>
    <div class="dp-grid">
      <span class="dp-key">Model commitment</span><span class="dp-val">${esc(p.model_commitment)}</span>
      <span class="dp-key">Input hash</span><span class="dp-val">${esc(p.input_hash)}</span>
      <span class="dp-key">Output hash</span><span class="dp-val">${esc(p.output_hash)}</span>
      <span class="dp-key">Layer</span><span class="dp-val">${esc(p.layer_name)}</span>
      <span class="dp-key">Dimensions</span><span class="dp-val">${p.layer_x_dim} &times; ${p.layer_z_dim}</span>
      <span class="dp-key">Receipt size</span><span class="dp-val">${(p.receipt_size_bytes/1024).toFixed(1)} KB</span>
    </div>`;
}

function verifyHtml(c) {
  if (c.inch_tx) return `
    <div class="dp-section">On-chain Verification</div>
    <div class="dp-grid">
      <span class="dp-key">Verdict</span><span class="dp-val">${statusBadge(c.status)}</span>
      <span class="dp-key">Reason</span><span class="dp-val">${esc(c.verdict_reason||'&mdash;')}</span>
      <span class="dp-key">INCH tx</span><span class="dp-val">${esc(c.inch_tx)}</span>
    </div>`;
  return `<div class="dp-section">Verification</div>
          <span style="color:var(--muted)">No INCH tx &mdash; unverified</span>`;
}

function detailHtml(c) {
  const resp = c.output != null
    ? `<div><div class="dp-section">Response</div><div class="dp-text">${esc(c.output)}</div></div>`
    : '';
  return `
    <div class="dp-section">Transaction IDs</div>
    <div class="dp-grid">
      <span class="dp-key">Request ID</span><span class="dp-val">${esc(c.request_id)}</span>
      <span class="dp-key">INFR tx</span><span class="dp-val">${esc(c.infr_tx)}</span>
      ${c.inrs_tx ? `<span class="dp-key">INRS tx</span><span class="dp-val">${esc(c.inrs_tx)}</span>` : ''}
      <span class="dp-key">Model</span><span class="dp-val">${esc(c.model)}</span>
      <span class="dp-key">max_tokens</span><span class="dp-val">${c.max_new_tokens}</span>
    </div>
    <div class="dp-cols">
      <div><div class="dp-section">Prompt</div><div class="dp-text">${esc(c.prompt)}</div></div>
      ${resp}
    </div>
    ${proofHtml(c.proof)}
    ${verifyHtml(c)}`;
}

function renderTable(data) {
  if (data.conversations.length === 0) {
    const msg = data.status === 'scanning'
      ? 'Scanning chain history&hellip;'
      : 'No inference transactions found on this chain.';
    return `<div class="loading">${msg}</div>`;
  }

  const head = `
    <table>
    <colgroup>
      <col class="c-blk"><col class="c-time"><col class="c-src"><col class="c-model">
      <col class="c-prompt"><col class="c-status"><col class="c-resp"><col class="c-tog">
    </colgroup>
    <thead><tr>
      <th>Block</th><th>Time (UTC)</th><th>Source</th><th>Model</th>
      <th>Prompt</th><th>Status</th><th>Response</th><th></th>
    </tr></thead><tbody>`;

  const rows = data.conversations.map(c => {
    const rid      = c.request_id;
    const open     = expanded.has(rid);
    const model    = c.model.split('/').pop();
    const promptPr = esc(c.prompt.length > 55 ? c.prompt.slice(0,55)+'…' : c.prompt);
    const respPr   = c.output
      ? esc(c.output.length > 55 ? c.output.slice(0,55)+'…' : c.output)
      : '<span style="color:var(--muted)">&mdash;</span>';

    return `
      <tr class="data-row" onclick="toggle('${rid}')">
        <td>#${c.infr_block}</td>
        <td>${ts(c.infr_timestamp).slice(5)}</td>
        <td>${srcBadge(c.source)}</td>
        <td>${esc(model)}</td>
        <td>${promptPr}</td>
        <td>${statusBadge(c.status)}</td>
        <td>${respPr}</td>
        <td class="tog" id="t-${rid}">${open ? '▼' : '▶'}</td>
      </tr>
      <tr id="d-${rid}" class="detail-row"${open ? '' : ' style="display:none"'}>
        <td colspan="8">${detailHtml(c)}</td>
      </tr>
      <tr class="sep-row"><td colspan="8"></td></tr>`;
  }).join('');

  return head + rows + '</tbody></table>';
}

async function loadData() {
  try {
    const data = await fetch('/api/events').then(r => r.json());

    // Header
    const info = data.status === 'scanning'
      ? `<span style="color:var(--yellow)">scanning — block #${data.scanned_to}&hellip;</span>`
      : `<span style="color:var(--green)">✓ ready</span> &nbsp;·&nbsp; block #${data.scanned_to}`;
    document.getElementById('hdr-meta').innerHTML = `${esc(data.node)} &nbsp;&middot;&nbsp; ${info}`;

    // Scan note
    document.getElementById('scan-note').style.display = data.status === 'scanning' ? '' : 'none';

    // Stats
    const cv = data.conversations;
    const responded = cv.filter(c => c.status !== 'PENDING').length;
    const pending   = cv.filter(c => c.status === 'PENDING').length;
    const verified  = cv.filter(c => c.status === 'VALID').length;
    const fraud     = cv.filter(c => c.status === 'FRAUD').length;
    document.getElementById('stats').innerHTML = `
      <div class="stat"><div class="stat-n">${cv.length}</div><div class="stat-l">requests</div></div>
      <div class="stat"><div class="stat-n">${responded}</div><div class="stat-l">responded</div></div>
      <div class="stat"><div class="stat-n">${pending}</div><div class="stat-l">pending</div></div>
      <div class="stat"><div class="stat-n" style="color:var(--green)">${verified}</div><div class="stat-l">verified</div></div>
      <div class="stat"><div class="stat-n" style="color:var(--red)">${fraud}</div><div class="stat-l">fraud</div></div>`;

    // Table
    document.getElementById('main').innerHTML = renderTable(data);
  } catch(e) {
    document.getElementById('hdr-meta').textContent = 'connection error';
  }
}

loadData();
setInterval(loadData, 10000);
</script>
</body>
</html>"""
