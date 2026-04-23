// spam.js — fires increment(message) every 2s via both ETH RPC and VeChain native API.
//
// ETH path   → ethers EIP-1559 wallet → eth_sendRawTransaction on localhost:8545
// VeChain path → hand-built legacy VeChain tx → POST /transactions on localhost:8669
//
// Run: node scripts/spam.js  (from the hardhat/ directory)
const { ethers } = require("ethers");
const { blake2b } = require("@noble/hashes/blake2b");
const fs = require("fs");
const path = require("path");

const DEPLOYMENT_FILE = path.join(__dirname, "../deployments/counter.json");
const ETH_RPC_URL    = "http://localhost:8545";
const VECHAIN_URL    = "http://localhost:8669";
const DEV_PRIVATE_KEY =
  "0x99f0500549792796c14fed62011a51081dc5b5e68fe8bd8a13b86be829c4fd36";
const INTERVAL_MS = 2000;

// ─── RLP helpers ────────────────────────────────────────────────────────────

// Minimal big-endian hex for a non-negative integer (strips leading zeros).
// 0 encodes as "0x" so that RLP emits 0x80 (empty byte string = zero integer).
function rlpUint(n) {
  const b = BigInt(n);
  if (b === 0n) return "0x";
  let hex = b.toString(16);
  if (hex.length % 2) hex = "0" + hex;
  return "0x" + hex;
}

// ─── Blake2b-256 ────────────────────────────────────────────────────────────

function blake2b256(bytes) {
  return blake2b(bytes, { dkLen: 32 });
}

// ─── VeChain native transaction builder ─────────────────────────────────────
//
// VeChain legacy tx wire format (RLP list):
//   [chainTag, blockRef, expiration, clauses, gasPriceCoef,
//    gas, dependsOn, nonce, reserved, signature]
//
// Signing hash = Blake2b256(RLP([...same without signature...]))
//
function buildSignedVeChainTx({ chainTag, blockRef, expiration, to, calldata, gas, nonce }) {
  // Each clause: [to(20B), value(0=empty), data(bytes)]
  const clauses = [[to, "0x", calldata]];

  const signingFields = [
    rlpUint(chainTag),    // uint8  ChainTag
    rlpUint(blockRef),    // uint64 BlockRef (minimal BE int)
    rlpUint(expiration),  // uint32 Expiration
    clauses,              // Clauses list
    rlpUint(0),           // uint8  GasPriceCoef = 0
    rlpUint(gas),         // uint64 Gas
    "0x",                 // *Bytes32 nil → empty bytes (0x80)
    rlpUint(nonce),       // uint64 Nonce
    [],                   // reserved (empty list → 0xc0)
  ];

  const signingHash = blake2b256(ethers.getBytes(ethers.encodeRlp(signingFields)));

  const signingKey = new ethers.SigningKey(DEV_PRIVATE_KEY);
  const sig = signingKey.sign(signingHash);

  // VeChain signature: r (32B) ‖ s (32B) ‖ yParity (1B as 0x00 or 0x01)
  const sigBytes = ethers.getBytes(
    ethers.concat([sig.r, sig.s, sig.yParity === 0 ? "0x00" : "0x01"])
  );

  const wireFields = [...signingFields, sigBytes];
  return ethers.encodeRlp(wireFields);
}

// ─── Main ────────────────────────────────────────────────────────────────────

async function main() {
  if (!fs.existsSync(DEPLOYMENT_FILE)) {
    console.error("Counter not deployed. Run: make deploy");
    process.exit(1);
  }
  const { address, abi } = JSON.parse(fs.readFileSync(DEPLOYMENT_FILE, "utf8"));
  const iface = new ethers.Interface(abi);

  // Fetch chain tag once (last byte of genesis block ID)
  const genesisRes = await fetch(`${VECHAIN_URL}/blocks/0`);
  const genesis = await genesisRes.json();
  const chainTag = parseInt(genesis.id.slice(-2), 16);
  console.log(`Contract : ${address}`);
  console.log(`Chain tag: 0x${chainTag.toString(16)}`);
  console.log(`Interval : ${INTERVAL_MS}ms\n`);

  // ETH path
  const ethProvider = new ethers.JsonRpcProvider(ETH_RPC_URL);
  const wallet      = new ethers.Wallet(DEV_PRIVATE_KEY, ethProvider);
  const counter     = new ethers.Contract(address, abi, wallet);

  let seq = 0;

  async function sendEth(msg) {
    const n = seq;
    try {
      const tx = await counter.increment(msg);
      console.log(`[ETH #${n}] sent  tx=${tx.hash.slice(0, 20)}…`);
    } catch (e) {
      console.error(`[ETH #${n}] error: ${e.shortMessage || e.message}`);
    }
  }

  async function sendVeChain(msg) {
    const n = seq;
    try {
      // Fetch best block for blockRef (first 8 bytes of ID as uint64)
      const bestRes = await fetch(`${VECHAIN_URL}/blocks/best`);
      const best    = await bestRes.json();
      const blockRef = BigInt("0x" + best.id.slice(2, 18));
      const nonce    = BigInt(Math.floor(Math.random() * 0xFFFFFFFF));

      const calldata = iface.encodeFunctionData("increment", [msg]);

      const rawHex = buildSignedVeChainTx({
        chainTag,
        blockRef,
        expiration: 32,
        to: address,
        calldata,
        gas: 100000,
        nonce,
      });

      const res = await fetch(`${VECHAIN_URL}/transactions`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ raw: rawHex }),
      });
      const result = await res.json();
      if (result.id) {
        console.log(`[VEC #${n}] sent  tx=${result.id.slice(0, 20)}…`);
      } else {
        console.error(`[VEC #${n}] error: ${JSON.stringify(result)}`);
      }
    } catch (e) {
      console.error(`[VEC #${n}] error: ${e.message}`);
    }
  }

  console.log("Press Ctrl+C to stop.\n");

  setInterval(async () => {
    seq++;
    const ts  = new Date().toISOString();
    const eth = `eth#${seq} ${ts}`;
    const vec = `vec#${seq} ${ts}`;
    await Promise.all([sendEth(eth), sendVeChain(vec)]);
  }, INTERVAL_MS);
}

main().catch((e) => {
  console.error(e);
  process.exit(1);
});
