pragma solidity ^0.4.18;
import './Energy.sol';
contract Test {

  function shareWithEN(address _reciever,uint256 _limit,uint256 _creditGrowthRate,uint256 _expire,address en) public {
    Energy e = Energy(en);
    e.shareTo(_reciever,_limit,_creditGrowthRate,_expire);
  }

}