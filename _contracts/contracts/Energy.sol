pragma solidity ^0.4.18;
import "./Token.sol";
import './SafeMath.sol';
contract Energy is Token {
  using SafeMath for uint256;
  //假设每1VEN每100秒可额外执行一次交易
  uint public constant UNITTXFEE = 100;
  //每1VEN每秒增长ENG
  uint public constant UNITENGUP = 1;
  struct Balance {
    uint256 balance;
    uint256 timestamp;
    uint256 venamount;
    // uint256 shareamount;
    bool isSet;
  }
  mapping(address => Balance) balances;
  mapping(address => mapping (address => uint256)) allowed;
  event Transfer(address indexed _from, address indexed _to, uint256 _value);
  event Approval(address indexed _owner, address indexed _spender, uint256 _value);
  
  //Share 
  struct ShareEnergy {
    address from;
    uint256 amount;
    uint256 expire;
  }
  mapping (bytes32 => ShareEnergy[]) public shares;
  event ShareFrom(address indexed _from,address indexed _to,address indexed target,uint256 amount,uint256 expire);

  function name() public pure returns (string n) {
    n = "VET Energy";
  }
  function decimals() public pure returns (uint8 d) {
    d = 18;    
  }
  function symbol() public pure returns (string s) {
    s = "ENG";
  }

  //totalSupply
  function totalSupply() public constant returns (uint totalEnergy) {
    //TODO
    totalEnergy = 10000;
  }
  // Send back vet sent to me
  function() public {
    revert();
  }
  //share amount energy _from to _to within _expire
  function shareFrom(address _from,address _to,address target,uint256 _amount,uint256 _expire) public returns (uint256) {
    require(_from != _to);
    require(!isContract(_to));
    require(isContract(target));
    ShareFrom(_from,_to,target,_expire,_amount);
    bytes32 key = keccak256(_to,target);
    // require(shares[key].length <= 10);
    ShareEnergy[] storage ss = shares[key];
    for (uint256 i = 0; i < ss.length ; i++) {
      ShareEnergy storage s = ss[i];
      if (_from == s.from) {
        s.amount = _amount;
        s.expire = _expire;
        return;
      }
    }
    ss.push(ShareEnergy(_from,_amount,_expire));
  }

  //get ShareAmount by _to and _target(contract address)
  function getShareAmount(address _to,address _target) public constant returns (uint256) {
    bytes32 key = keccak256(_to,_target);
    uint256 sum = 0;
    ShareEnergy[] storage ss = shares[key];
    for (uint256 j = 0; j < ss.length; j++) {
      ShareEnergy storage s = ss[j];
      if (s.expire > now && s.amount > 0) {
        sum = sum.add(s.amount);
      }
    }
    return sum;
  }

  //first consume shared engergy
  function consumeShared(bytes32 key,uint256 _amount) internal returns(uint256) {
    //require(msg.sender == consume);
    ShareEnergy[] storage ss = shares[key];
    for (uint256 i = 0; i < ss.length; i++) {
      ShareEnergy storage s = ss[i];
      if (s.amount > 0 && s.expire > now) {
        if (s.amount >= _amount ) {
          s.amount = s.amount.sub(_amount);
          break;
        } else {
          _amount = _amount.sub(s.amount);
          s.amount = 0;
        }
      }
    }
    return _amount;
  }

  //if share engergy not enough, consume this Energy
  function consumeEnergy(address to,uint256 amount) internal returns (bool) {
    // require(msg.sender == consumer);
    uint256 b = balanceOf(to);
    balances[to].balance = b;
    if (b >= amount) {
      balances[to].balance = balances[to].balance.sub(amount);
    }
  }

  //consume energy
  function consume(address to,address target,uint256 amount) initialBalance reinitialBalance public {
    //require(msg.sender == admin);
    require(isContract(target));
    require(!isContract(to));
    bytes32 key = keccak256(to,target);
    uint256 shareRestAmount = getShareAmount(to,target);
    if (shareRestAmount >= amount) {
      consumeShared(key,amount);
      return;
    }
    uint256 engAmount = balanceOf(to);
    if (shareRestAmount.add(engAmount) >= amount) {
      uint256 rest = consumeShared(key,amount);
      consumeEnergy(to,rest);
      return;
    }
  }

  

  modifier initialBalance {
    if (!balances[msg.sender].isSet) {
      balances[msg.sender].isSet = true;
      balances[msg.sender].balance = 0;
      balances[msg.sender].venamount = msg.sender.balance;
      balances[msg.sender].timestamp = now;
    }
    _;
  }
  modifier reinitialBalance {
    _;
    balances[msg.sender].balance = balanceOf(msg.sender);
    balances[msg.sender].venamount = msg.sender.balance;
    balances[msg.sender].timestamp = now;
  }
  //balance of address
  function balanceOf(address _owner) public constant returns (uint256 balance) {
    uint256 amount = balances[_owner].balance;
    uint256 time = balances[_owner].timestamp;
    uint256 ven = balances[_owner].venamount;
    amount += UNITENGUP.mul(ven.mul(now.sub(time)))+(_owner.balance.sub(ven)).mul(UNITTXFEE);
    // balances[_owner].balance = amount;
    // balances[_owner].venamount = getVEN();
    return ven;
  }

  //transfer energy
  function transfer(address _to, uint256 _amount) initialBalance reinitialBalance public returns (bool success) {
    require(!isContract(_to));
    uint256 currentBalance = balanceOf(msg.sender);
    balances[msg.sender].balance = currentBalance;
    balances[msg.sender].venamount = msg.sender.balance;
    if (_amount > 0 && currentBalance >= _amount && balances[_to].balance.add(_amount) > balances[_to].balance) {
      balances[msg.sender].balance -= _amount;
      balances[_to].balance = balances[_to].balance.add(_amount);
      Transfer(msg.sender, _to, _amount);
      return true;
    } else {
      return false;
    }
  }

  function transferFrom(address _from,address _to,uint256 _amount) initialBalance reinitialBalance public returns (bool success) {
    require(!isContract(_to));
    uint256 currentBalance = balanceOf(_from);
    balances[_from].balance = currentBalance;
    balances[_from].venamount = _from.balance;
    if (_amount > 0 && currentBalance >= _amount && allowed[_from][msg.sender] >= _amount && balances[_to].balance.add(_amount) > balances[_to].balance) {
      balances[_from].balance = balances[_from].balance.sub(_amount);
      allowed[_from][msg.sender] = allowed[_from][msg.sender].sub(_amount);
      balances[_to].balance = balances[_to].balance.add(_amount);
      Transfer(_from, _to, _amount);
      return true;
    }
    return false;
    
  }

  function approve(address _spender, uint256 _amount) initialBalance reinitialBalance public returns (bool success) {
    allowed[msg.sender][_spender] = _amount;
    Approval(msg.sender, _spender, _amount);
    return true;
  }
  
  function allowance(address _owner, address _spender)  public constant returns (uint256 remaining) {
    return allowed[_owner][_spender];
  }
  //is a contranct
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
