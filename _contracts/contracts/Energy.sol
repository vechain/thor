pragma solidity ^0.4.18;
import "./Token.sol";
import './SafeMath.sol';
/// @title Energy an ERC20 token.
contract Energy is Token {
    
    using SafeMath for uint256;
     
    //energy grown stamp for each VET
    uint public constant UNITGROWNSTAMP = 1;
    //which represents an owner's balance
    struct Balance {
        uint256 balance;//the energy balance
        uint256 timestamp;//for calculate how much energy would be grown
        uint256 venamount;//vet balance
        bool isSet;//whether the balances is initiated or not
    }
    //save all owner's balance
    mapping(address => Balance) balances;
    
    mapping(address => mapping (address => uint256)) allowed;

    event Transfer(address indexed _from, address indexed _to, uint256 _value);

    event Approval(address indexed _owner, address indexed _spender, uint256 _value);
    
    //which represents the detail info for a shared energy credits
    struct SharedCredit {
        address from;
        uint256 amount;
        uint256 expire;
    }
    //an array that stores all energy credits
    mapping (bytes32 => SharedCredit) public sharedCredits;

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
  
    ///@notice share `_amount` energy credits from `_from` to `_to` which can only be consumed,never transferred
    ///@param _from who shares the energy credits , it should be euqal to _target
    ///@param _to who recieves the energy credits
    ///@param _target which is a contract, if a msg is called in the contract, the energy credits would be consumed
    ///@param _amount how many energy credits would be shared
    ///@param _expire a timestamp, if block time exceeded that, this shared energy credits would be useless
    function shareFrom(address _from,address _to,address _target,uint256 _amount,uint256 _expire) public {
        //credits can only be applied to a contract
        require(isContract(_target));
        //the contract caller should be a normal amount
        require(!isContract(_to));
        //only the contract can share to its user
        require(_from == _target);
        //shared credits should be greater than zero
        require(_amount > 0);
        //the expiration time should be greater than block time
        require(_expire > now);

        ShareFrom(_from, _to, _target, _amount, _expire);
        //hash the caller address and the contract address to ensure the key unique
        bytes32 key = keccak256(_to,_target);
        sharedCredits[key] = SharedCredit(_from,_amount,_expire);

    }
    
    ///@param _to who recieves the energy credits
    ///@param _target which is a contract,if a msg is called in the contract,the credits would be consumed
    ///
    ///@return how many credits is shared
    function getSharedCredits(address _to,address _target) public constant returns (uint256) {
        //hash the caller address and the contract address to ensure the key unique
        bytes32 key = keccak256(_to,_target);
        SharedCredit storage s = sharedCredits[key];
        if ( s.amount > 0 && s.expire > now ) {
            return s.amount;
        } 
        return 0;
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
    ///@param _target which is a contract,if a msg is called in the contract,the energy credits would be consumed
    ///@param _amount total energy and shared energy credits would be consumed
    ///
    ///@return whether the energy and shared energy credits is consumed successfully or not
    function consume(address _to,address _target,uint256 _amount) public returns(bool success) {
        //require(msg.sender == admin);
        //credits can only be applied to a contract
        require(isContract(_target));
        //the contract caller should be a normal amount
        require(!isContract(_to));
        //shared credits should be greater than zero
        require(_amount > 0);

        bytes32 key = keccak256(_to,_target);
        uint256 sc = getSharedCredits(_to,_target);
        if (sc >= _amount) {
            sharedCredits[key].amount = sc.sub(_amount);
            return true;
        }
        uint256 engAmount = calRestBalance(_to);
        if (sc.add(engAmount) >= _amount) {
            sharedCredits[key].amount = 0;
            consumeEnergy(_to,_amount.sub(sc));
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

    ///@notice To initiate or calculate the energy balance of the owner
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
