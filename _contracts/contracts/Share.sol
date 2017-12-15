pragma solidity ^0.4.18;
import './Utils.sol';
contract Share {
  
  struct ShareEnergy {
    address from;
    uint256 amount;
    uint256 expire;
  }

  mapping (bytes32 => ShareEnergy[]) public shares;
  event ShareFrom(address indexed _from,address indexed _to,address indexed target,uint256 amount,uint256 expire);
  
  function shareFrom(address _from,address _to,address target,uint256 _amount,uint256 expire) public returns (uint256) {
    require(_from != _to);
    require(Util.isContract(target));
    ShareFrom(_from,_to,target,expire,_amount);
    bytes32 key = sha256(_to,target);
    require(shares[key].length <= 10);
    ShareEnergy[] storage ss = shares[key];
    for (uint i = 0; i < ss.length ; i++) {
      ShareEnergy storage s = ss[i];
      if (_from == s.from) {
        s.amount = _amount;
        s.expire = expire;
        return;
      }
    }
    ss.push(ShareEnergy(_from,_amount,expire));
  }

  function getShareAmount(address _to,address target) public constant returns (uint256) {
    require(Util.isContract(target));
    bytes32 key = sha256(_to,target);
    uint256 sum = 0;
    ShareEnergy[] storage ss = shares[key];
    for (uint j = 0; j < ss.length; j++) {
      ShareEnergy storage s = ss[j];
      if (s.expire > now && s.amount > 0) {
        sum += s.amount;
      }
    }
    return sum;
  }

  function consumeShared(bytes32 key,uint256 _amount) public returns(uint256) {
    ShareEnergy[] storage ss = shares[key];
    for (uint i = 0; i < ss.length; i++) {
      ShareEnergy storage s = ss[i];
      if (s.amount > 0 && s.expire > now) {
        if (s.amount >= _amount ) {
          s.amount -= _amount;
          break;
        } else {
          _amount -= s.amount;
          s.amount = 0;
        }
      }
    }
    return _amount;
  }
}