var Share = artifacts.require("../contracts/Share.sol");
var Energy = artifacts.require("../contracts/Energy.sol");

module.exports = function(deployer) {
  deployer.deploy(Share);
  deployer.deploy(Energy);

};
