// deploy.js — deploys Counter using a direct ethers.Wallet (EIP-1559).
// Idempotent: skips if deployments/counter.json already exists.
const { ethers, artifacts } = require("hardhat");
const fs = require("fs");
const path = require("path");

const DEPLOYMENT_FILE = path.join(__dirname, "../deployments/counter.json");
const RPC_URL = "http://localhost:8545";
const DEV_PRIVATE_KEY =
  "0x99f0500549792796c14fed62011a51081dc5b5e68fe8bd8a13b86be829c4fd36";

async function main() {
  if (fs.existsSync(DEPLOYMENT_FILE)) {
    const d = JSON.parse(fs.readFileSync(DEPLOYMENT_FILE, "utf8"));
    console.log("Counter already deployed at:", d.address);
    console.log("Run 'make reset' then 'make deploy' to redeploy.");
    return;
  }

  // Use a direct JsonRpcProvider + Wallet so sendTransaction calls
  // eth_sendRawTransaction with a properly signed EIP-1559 tx instead of
  // eth_sendTransaction (which requires server-side signing).
  const provider = new ethers.JsonRpcProvider(RPC_URL);
  const wallet = new ethers.Wallet(DEV_PRIVATE_KEY, provider);
  console.log("Deploying Counter with account:", wallet.address);

  const artifact = await artifacts.readArtifact("Counter");
  const factory = new ethers.ContractFactory(artifact.abi, artifact.bytecode, wallet);
  const counter = await factory.deploy();
  await counter.waitForDeployment();

  const address = await counter.getAddress();

  const deployment = { address, abi: artifact.abi };
  fs.mkdirSync(path.dirname(DEPLOYMENT_FILE), { recursive: true });
  fs.writeFileSync(DEPLOYMENT_FILE, JSON.stringify(deployment, null, 2));

  console.log("Counter deployed to:", address);
  console.log("Deployment saved to:", DEPLOYMENT_FILE);
}

main().catch((e) => {
  console.error(e);
  process.exitCode = 1;
});
