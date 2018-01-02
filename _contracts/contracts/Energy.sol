pragma solidity ^0.4.18;
import "./Token.sol";
import './SafeMath.sol';
contract Energy is Token {

    using SafeMath for uint256;

    //energy grown stamp for each VET
    uint public constant UNITGROWNSTAMP = 1;

    struct Balance {
        uint256 balance;
        uint256 timestamp;
        uint256 venamount;
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

    event ShareFrom(address indexed _from,address indexed _to,address indexed _target,uint256 amount,uint256 expire);
    ///@return ERC20 token name
    function name() public pure returns (string) {
        return "VET Energy";
    }
    ///@return ERC20 token decimals
    function decimals() public pure returns (uint8) {
        return 18;    
    }
    ///@return ERC20 token symbol
    function symbol() public pure returns (string) {
        return "ENG";
    }

    ///@return ERC20 token total supply
    function totalSupply() public constant returns (uint256) {
        return 0;
    }

    // Send back vet sent to me
    function() public payable {
        revert();
    }

    ///@notice share `_amount` energy credits from `_from` to `_to` which can only be consumed,never trasferred
    ///@param _from who shares the energy credits
    ///@param _to who recieves the energy credits
    ///@param _target which is a contract,if a msg is called in the contract,the energy credits would be consumed
    ///@param _amount how many energy credits would be shared
    ///@param _expire a timestamp ,when block time covered that, this shared energy credits would be useless
    function shareFrom(address _from,address _to,address _target,uint256 _amount,uint256 _expire) public {
        //never shared to self
        require(_from != _to);
        //never shared to a contract
        require(!isContract(_to));
        //energy credits can only be applied to a contract
        require(isContract(_target));
        
        ShareFrom(_from,_to,_target,_expire,_amount);
        bytes32 key = keccak256(_to,_target);
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
    
    ///@param _to who recieves the energy credits
    ///@param _target which is a contract,if a msg is called in the contract,the energy credits would be consumed
    ///
    ///@return how many energy credits is shared
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

    ///@param key the hash of _to and _target 
    ///@param _amount how many shared credits should be consumed
    ///
    ///@return how many shared energy credits are left
    function consumeShared(bytes32 key,uint256 _amount) internal returns(uint256) {
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

    ///@notice if shared energy credits are all consumed,the true energy would be consumed
    ///@param to who holds the energy
    ///@param amount how much energy should be consumed
    ///
    ///@return whether the energy is consumed successfully or not
    function consumeEnergy(address to,uint256 amount) internal returns (bool) {
        uint256 b = balances[to].balance;
        balances[to].balance = b;
        if (b >= amount) {
            balances[to].balance = balances[to].balance.sub(amount);
        }
    }

    ///@notice if shared energy credits are all consumed,the true energy would be consumed
    ///@param _to who holds the energy and the shared energy credits
    ///@param amount total energy and shared energy credits would be consumed
    ///
    ///@return whether the energy and shared energy credits is consumed successfully or not
    function consume(address _to,address target,uint256 amount) public returns(bool success) {
        //require(msg.sender == admin);
        require(isContract(target));
        require(!isContract(_to));
        bytes32 key = keccak256(_to,target);
        uint256 shareRestAmount = getShareAmount(_to,target);
        if (shareRestAmount >= amount) {
            consumeShared(key,amount);
            return true;
        }
        uint256 engAmount = calRestBalance(_to);
        if (shareRestAmount.add(engAmount) >= amount) {
            uint256 rest = consumeShared(key,amount);
            consumeEnergy(_to,rest);
            return true;
        }
        return false;
    }
    
    ///@param _owner who holds the energy and the vet
    ///
    ///@return how much energy the _owner holds
    function balanceOf(address _owner) public constant returns (uint256 balance) {
        uint256 amount = balances[_owner].balance;
        uint256 time = balances[_owner].timestamp;
        uint256 ven = balances[_owner].venamount;
        amount += UNITGROWNSTAMP.mul(ven.mul(now.sub(time)))+(_owner.balance.sub(ven)).mul(UNITGROWNSTAMP);
        return ven;
    }

    ///@notice To initiate the balances storage of the owner
    ///@param _owner who holds the energy and the vet
    ///
    ///@return how much energy the _owner holds
    function calRestBalance(address _owner) internal returns(uint256) {

        if (balances[_owner].isSet) {
            balances[_owner].balance = balanceOf(msg.sender);
            balances[_owner].venamount = msg.sender.balance;
            balances[_owner].timestamp = now;
        } else {
            balances[_owner].isSet = true;
            balances[_owner].balance = 0;
            balances[_owner].venamount = msg.sender.balance;
            balances[_owner].timestamp = now;
        }
        return balances[_owner].balance;

    }

    /// @notice send `_amount` token to `_to` from `msg.sender`
    /// @param _to The address of the recipient
    /// @param _amount The amount of token to be transferred
    /// @return Whether the transfer was successful or not
    function transfer(address _to, uint256 _amount) public returns (bool success) {
        require(!isContract(_to));
        require(_amount > 0);
        uint256 senderBalance = calRestBalance(msg.sender);
        uint256 recipientBalance = calRestBalance(_to);
        if (senderBalance >= _amount && recipientBalance.add(_amount) > recipientBalance) {
            balances[msg.sender].balance = balances[msg.sender].balance.sub(_amount);
            balances[_to].balance = balances[_to].balance.add(_amount);
            Transfer(msg.sender, _to, _amount);
            return true;
        } else {
            return false;
        }
    }

    /// @notice send `_amount` token to `_to` from `_from` on the condition it is approved by `_from`
    /// @param _from The address of the sender
    /// @param _to The address of the recipient
    /// @param _amount The amount of token to be transferred
    /// @return Whether the transfer was successful or not
    function transferFrom(address _from,address _to,uint256 _amount) public returns (bool success) {
        require(!isContract(_to));
        require(_amount > 0);
        uint256 senderBalance = calRestBalance(_from);
        uint256 recipientBalance = calRestBalance(_to);
        if (senderBalance >= _amount && allowed[_from][msg.sender] >= _amount && recipientBalance.add(_amount) > recipientBalance) {
            balances[_from].balance = balances[_from].balance.sub(_amount);
            allowed[_from][msg.sender] = allowed[_from][msg.sender].sub(_amount);
            balances[_to].balance = balances[_to].balance.add(_amount);
            Transfer(_from, _to, _amount);
            return true;
        }
        return false;  
    }

    /// @notice `msg.sender` approves `_addr` to spend `_value` tokens
    /// @param _spender The address of the account able to transfer the tokens
    /// @param _amount The amount of wei to be approved for transfer
    /// @return Whether the approval was successful or not
    function approve(address _spender, uint256 _amount) public returns (bool success) {
        allowed[msg.sender][_spender] = _amount;
        Approval(msg.sender, _spender, _amount);
        return true;
    }
    /// @param _owner The address of the account owning tokens
    /// @param _spender The address of the account able to transfer the tokens
    /// @return Amount of remaining tokens allowed to spent
    function allowance(address _owner, address _spender)  public constant returns (uint256 remaining) {
        return allowed[_owner][_spender];
    }

    /// @param _addr an address of a normal account or a contract
    /// 
    /// @return whether the account that the address `_addr` represents is a contract or not
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
