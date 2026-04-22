// increment.js — calls Counter.increment() via a direct ethers.Wallet (EIP-1559).
const { ethers } = require("hardhat");
const fs = require("fs");
const path = require("path");

const DEPLOYMENT_FILE = path.join(__dirname, "../deployments/counter.json");
const RPC_URL = "http://localhost:8545";
const DEV_PRIVATE_KEY =
  "0x99f0500549792796c14fed62011a51081dc5b5e68fe8bd8a13b86be829c4fd36";

async function main() {
  if (!fs.existsSync(DEPLOYMENT_FILE)) {
    console.error("Counter not deployed. Run: make deploy");
    process.exitCode = 1;
    return;
  }

  const { address, abi } = JSON.parse(fs.readFileSync(DEPLOYMENT_FILE, "utf8"));

  const provider = new ethers.JsonRpcProvider(RPC_URL);
  const wallet = new ethers.Wallet(DEV_PRIVATE_KEY, provider);
  const counter = new ethers.Contract(address, abi, wallet);

  const before = await counter.count();
  console.log("Count before:", before.toString());

  console.log("Sending increment()...");
  const tx = await counter.increment();
  console.log("Tx hash:", tx.hash);

  const receipt = await tx.wait();
  console.log("Mined in block:", receipt.blockNumber);

  const after = await counter.count();
  console.log("Count after:", after.toString());

  for (const log of receipt.logs) {
    try {
      const parsed = counter.interface.parseLog(log);
      if (parsed?.name === "CounterIncreased") {
        console.log(
          `Event: CounterIncreased(by=${parsed.args.by}, newCount=${parsed.args.newCount})`
        );
      }
    } catch (_) {}
  }
}

main().catch((e) => {
  console.error(e);
  process.exitCode = 1;
});
