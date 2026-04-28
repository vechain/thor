import { HardhatUserConfig } from "hardhat/config";
import "@nomicfoundation/hardhat-toolbox";

// Hardhat is used ONLY for Solidity compilation.
// Deployment is handled by scripts/deploy.py (Python + VeChain signer)
// because VeChain's chain ID is a 256-bit genesis block ID that Hardhat
// cannot sign with.

const config: HardhatUserConfig = {
  solidity: {
    version: "0.8.20",
    settings: {
      optimizer: {
        enabled: true,
        runs: 200,
      },
      viaIR: true,
    },
  },
  // No networks configured — we never deploy via Hardhat
};

export default config;
