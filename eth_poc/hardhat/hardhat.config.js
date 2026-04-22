require("@nomicfoundation/hardhat-toolbox");

// VeChain devnet genesis account #0 — pre-funded with 1B VET + VTHO.
// Only used for local solo testing; never use in production.
const DEV_PRIVATE_KEY =
  "99f0500549792796c14fed62011a51081dc5b5e68fe8bd8a13b86be829c4fd36";

/** @type import('hardhat/config').HardhatUserConfig */
module.exports = {
  solidity: {
    version: "0.8.24",
    settings: {
      optimizer: { enabled: true, runs: 200 },
    },
  },
  networks: {
    vechain_solo: {
      url: "http://localhost:8545",
      // Chain ID is derived from the devnet genesis block ID (last 2 bytes BE).
      // For the standard VeChain devnet genesis this is always 58712 (0xe558).
      chainId: 58712,
      accounts: [DEV_PRIVATE_KEY],
    },
  },
};
