const Energy = artifacts.require("../contracts/Energy.sol");
const Authority = artifacts.require("../contracts/Authority.sol");
const { assertFail, assertEqual } = require('./utils.js')
contract("Energy", (accounts) => {

  it("share energy", async() => {
    let shareAmount = 10000;
    let expireTime = Date.now()/1000+10000;
    let auth = await Authority.deployed();
    let energy = await Energy.deployed();
    await energy.shareFrom(accounts[1],accounts[0],auth.address,shareAmount,expireTime);
    let sa = await energy.getShareAmount(accounts[0],auth.address);
    assertEqual(sa,shareAmount,'shared energy not equal to expected');
  });
  
  it("consume energy", async() => {
    let shareAmount = 10000;
    let expireTime = Date.now()/1000+10000;
    let consumeAmount = 3000;
    let auth = await Authority.deployed();
    let energy = await Energy.deployed();
    await energy.shareFrom(accounts[1],accounts[0],auth.address,shareAmount,expireTime);
    await energy.consume(accounts[0],auth.address,consumeAmount);
    let sa = await energy.getShareAmount(accounts[0],auth.address);
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