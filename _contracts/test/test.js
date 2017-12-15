const Energy = artifacts.require("../contracts/Energy.sol");
const Share = artifacts.require("../contracts/Share.sol");
const { assertFail, assertEqual } = require('./utils.js')
contract("Energy", (accounts) => {

  it("share", async() => {
    let shareAmount = 10000;
    let expireTime = Date.now()/1000+10000;
    let share = await Share.deployed();
    let energy = await Energy.deployed();
    await share.shareFrom(accounts[1],accounts[0],energy.address,shareAmount,expireTime);
    let sa = await share.getShareAmount(accounts[0],energy.address);
    assertEqual(sa,shareAmount,'shared amount not equal to expected');
  });
  
  
  

});