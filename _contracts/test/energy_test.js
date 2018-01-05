const Energy = artifacts.require("../contracts/Energy.sol");
var Test = artifacts.require("../contracts/Test.sol");
const { assertFail, assertEqual } = require('./utils.js')
contract("Energy", (accounts) => {

  it("share energy", async() => {
    let shareAmount = 10000;
    let expireTime = Date.now()/1000+100000;
    let test = await Test.deployed();
    let energy = await Energy.deployed();
    let a = await test.shareWithEN(accounts[0],shareAmount,1,expireTime,energy.address);
    let sa = await energy.getAvailableCredits(test.address,accounts[0]);
    assertEqual(sa,shareAmount,'shared energy not equal to expected');
  });
  
  it("consume energy", async() => {
    let shareAmount = 10000;
    let expireTime = Date.now()/1000+10000;
    let consumeAmount = 3000;
    let test = await Test.deployed();
    let energy = await Energy.deployed();
    await test.shareWithEN(accounts[0],shareAmount,1,expireTime,energy.address);
    await energy.consume(test.address,accounts[0],consumeAmount);
    let sa = await energy.getAvailableCredits(test.address,accounts[0]);
    assertEqual(sa,shareAmount-consumeAmount,'consume energy from shared energy');
  });
  
  // it("consume not enough", async() => {
  //   let shareAmount = 10000;
  //   let expireTime = Date.now()/1000+10000;
  //   let consumeAmount = 30000;
  //   let auth = await Authority.deployed();
  //   let energy = await Energy.deployed();
  //   await energy.shareFrom(accounts[1],accounts[0],auth.address,shareAmount,expireTime);
  //   let sa = await energy.getShareAmount(accounts[0],auth.address);
  //   let b = await energy.balanceOf(accounts[0]);
  //   console.log(b);
  //   assertEqual(sa,shareAmount,'revert shared energy');
    
  // });
});