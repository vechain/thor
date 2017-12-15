pragma solidity ^0.4.18;
import "./Token.sol";
import "./Owned.sol";
import "./Share.sol";
import './Utils.sol';
contract Energy is Token , Owned {
  //symbol
  string public constant symbol = "ENG";
  //token name
  string public constant name = "VET Energy";
  //decimals
  uint8 public constant decimals = 18;
  //假设每1VEN每100秒可额外执行一次交易
  uint8 public constant UNITTXFEE = 100;
  //每1VEN每秒增长ENG
  uint8 public constant UNITENGUP = 1;
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

  function totalSupply() public constant returns (uint256 totalEnergy) {
    //TODO
    totalEnergy = 10000;
  }
  //return current ven amout
  function venOf(address _owner) internal constant returns (uint256 ven) {
    return _owner.balance;
  }
  //initial the energy
  function () public {
    require(!Util.isContract(msg.sender));
    if (!balances[msg.sender].isSet) {
      balances[msg.sender].venamount = owner.balance;
      balances[msg.sender].timestamp = now;
      balances[msg.sender].balance = 0;
      balances[msg.sender].isSet = true;
    }
  }
  
  //balance of address
  function balanceOf(address _owner) public constant returns (uint256 balance) {
    uint256 amount = balances[_owner].balance;
    uint256 time = balances[_owner].timestamp;
    uint256 ven = balances[_owner].venamount;
    amount += UNITENGUP * (ven * (now - time))+(venOf(_owner)-ven)*UNITTXFEE;
    // balances[_owner].balance = amount;
    // balances[_owner].venamount = getVEN();
    return amount;
  }
  //transfer energy
  function transfer(address _to, uint256 _amount) public returns (bool success) {
    uint256 currentBalance = balanceOf(msg.sender);
    balances[msg.sender].balance = currentBalance;
    balances[msg.sender].venamount = venOf(msg.sender);
    if (_amount > 0 && currentBalance >= (_amount+UNITTXFEE) && balances[_to].balance + _amount > balances[_to].balance) {
      balances[msg.sender].balance -= (_amount+UNITTXFEE);
      balances[_to].balance += _amount;
      Transfer(msg.sender, _to, _amount);
      return true;
    } else {
      return false;
    }
  }

  function transferFrom(address _from,address _to,uint256 _amount) public returns (bool success) {
    uint256 currentBalance = balanceOf(_from);
    balances[_from].balance = currentBalance;
    balances[_from].venamount = venOf(_from);
    uint256 totalCost = _amount+UNITTXFEE;
    if (_amount > 0 && currentBalance >= totalCost && allowed[_from][msg.sender] >= totalCost && balances[_to].balance + _amount > balances[_to].balance) {
      balances[_from].balance -= totalCost;
      allowed[_from][msg.sender] -= totalCost;
      balances[_to].balance += _amount;
      Transfer(_from, _to, _amount);
      return true;
    } else {
      return false;
    }
  }

  function approve(address _spender, uint256 _amount) public returns (bool success) {
    allowed[msg.sender][_spender] = _amount;
    Approval(msg.sender, _spender, _amount);
    return true;
  }
  
  function allowance(address _owner, address _spender) public constant returns (uint256 remaining) {
    return allowed[_owner][_spender];
  }


}