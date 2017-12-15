pragma solidity ^0.4.18;
library Util {
  function isContract(address _addr) constant internal returns(bool) {
    uint size;
    if (_addr == 0) {
      return false;
    }
    assembly {
      size := extcodesize(_addr)
    }
    return size > 0;
  }
}