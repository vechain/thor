var Authority = artifacts.require("../contracts/Authority.sol");
var Energy = artifacts.require("../contracts/Energy.sol");

module.exports = function(deployer) {
  deployer.deploy(Authority);
  deployer.deploy(Energy);

};
