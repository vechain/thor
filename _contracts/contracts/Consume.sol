pragma solidity ^0.4.18;
import './Utils.sol';
import './Share.sol';
import './Energy.sol';
contract Consume {
  function consume(address to,address target,uint256 amount,address sAddr,address engAddr) public returns(bool) {
    require(Util.isContract(target));
    require(Util.isContract(sAddr));
    require(Util.isContract(engAddr));
    Energy eng = Energy(engAddr);
    Share share = Share(sAddr);
    uint256 shareRestAmount = share.getShareAmount(to,target);
    bytes32 key = keccak256(to,target);
    if (shareRestAmount >= amount) {
      share.consumeShared(key,amount);
      return true;
    }
    uint256 engAmount = eng.balanceOf(to);
    if (shareRestAmount + engAmount >= amount) {
      // TODO
      // uint256 rest = share.consumeShared(key,amount);

      return true;
    }
    return false;
  }
}