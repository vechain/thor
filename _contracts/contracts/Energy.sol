pragma solidity ^0.4.18;
import "./Token.sol";
import './SafeMath.sol';
/// @title Energy an token that represents fuel for VET.
contract Energy is Token {
    
    using SafeMath for uint256;
    
    //which represents an owner's balance
    struct Balance {
        uint256 balance;//the energy balance
        uint256 timestamp;//for calculate how much energy would be grown
        uint256 vetamount;//vet balance
    }
    //save all owner's balance
    mapping(address => Balance) balances;
    
    mapping(address => mapping (address => uint256)) allowed;

    event Transfer(address indexed _from, address indexed _to, uint256 _value);

    event Approval(address indexed _owner, address indexed _spender, uint256 _value);

    //owners of the contracts, which can only be used to transfer energy from contract
    mapping (address => address) contractOwners;

    //which represents the detail info for a shared energy credits
    struct SharedCredit {
        uint256 limit;             //max availableCredits

        uint256 availableCredits;  //credits can be consumed
        uint256 expire;            //expiration time

        uint256 creditGrowthRate;  //how mange credits grown in a second
        uint256 currentTimeStamp;  //current block number
    }

    //an array that stores all energy credits
    mapping (bytes32 => SharedCredit) public sharedCredits;

    event ShareFrom(address indexed from,address indexed to,uint256 _limit,uint256 creditGrowthRate,uint256 expire);


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
        return "THOR";
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
    ///@param _reciever who recieves the energy credits
    ///@param _limit max credits can be consumed
    ///@param _creditGrowthRate how mange credits grown in a second
    ///@param _expire a timestamp, if block time exceeded that, this shared energy credits would be useless
    function shareTo(address _reciever,uint256 _limit,uint256 _creditGrowthRate,uint256 _expire) public {
        //sharing credits can only be called by a contract
        require(isContract(msg.sender));
        //the contract caller should be a normal amount
        require(!isContract(_reciever));
        //shared credits should be greater than zero
        require(_limit >= 0);
        //the expiration time should be greater than block time
        require(_expire > now);
        
        address _from = msg.sender;
        ShareFrom(_from, _reciever, _limit, _creditGrowthRate, _expire);
        //hash the caller address and the contract address to ensure the key unique
        bytes32 key = keccak256(_from,_reciever);
        sharedCredits[key] = SharedCredit(_limit,_limit,_expire, _creditGrowthRate,now);

    }

    ///@param _reciever who recieved the credits
    ///@param _from which is a contract that shares the to _reciever
    ///
    ///@return how many credits is shared
    function getAvailableCredits(address _from,address _reciever) public view returns (uint256) {
        //hash the caller address and the contract address to ensure the key unique.
        bytes32 key = keccak256(_from,_reciever);
        SharedCredit storage s = sharedCredits[key];
        if ( s.availableCredits > 0 && s.expire > now ) {
            uint256 cbt = block.timestamp;
            if ( cbt > s.currentTimeStamp) {
                //credits has been grown,calculate the available credits within the limit.
                uint256 ac = s.limit.add((cbt.sub(s.currentTimeStamp)).mul(s.creditGrowthRate));
                if (ac >= s.limit) {
                    return s.limit;
                }
                return ac;
            }
            return s.availableCredits;
        } 
        return 0;
    }

    ///@notice consume `_amount` tokens of `_consumer`
    ///@param _consumer tokens of whom would be consumed 
    ///@param _amount   `_amount` tokens would be consumed
    ///@return _consumption `_consumption` tokens that has been consumed
    function consumeEnergy(address _consumer,uint256 _amount) public returns(uint256 _consumption) {
        uint256 b = calRestBalance(_consumer);
        if (b < _amount) {
            return 0;
        }
        balances[_consumer].balance = b.sub(_amount);
        balances[_consumer].timestamp = block.timestamp;
        return _amount;
    }

    ///@notice consume `_amount` tokens of `_consumer`
    ///@param _contract `_contract` who shared the credits to `_consumer`
    ///@param _consumer credits of whom would be consumed 
    ///@param _amount   `_amount` tokens would be consumed
    ///@return _consumption `_consumption` credits that has been consumed
    function consumeCredits(address _contract, address _consumer, uint256 _amount) public returns(uint256 _consumption) {

        require(isContract(_contract));
        require(!isContract(_consumer));

        uint256 ac = getAvailableCredits(_contract,_consumer);
        if (ac < _amount) {
            return 0;
        }
        bytes32 key = keccak256(_contract,_consumer);
        sharedCredits[key].currentTimeStamp = block.timestamp;
        sharedCredits[key].availableCredits = ac.sub(_amount);
        return _amount;
    }

    ///@param _owner who holds the energy and the vet
    ///
    ///@return how much energy the _owner holds
    function balanceOf(address _owner) public constant returns (uint256 balance) {
        uint256 amount = balances[_owner].balance;
        uint256 time = balances[_owner].timestamp;
        uint256 vetamount = balances[_owner].vetamount;
        //calculate the benefit with per vet per second
        uint256 total = (10**5)*(10**18);
        uint256 benefit = 42;
        uint256 timestamp = 3600*24;
        uint256 unitGrownStamp = benefit.div(timestamp).div(total);
        //calculate balance
        amount = amount.add(unitGrownStamp.mul(vetamount.mul(now.sub(time))));
        return amount;
    }

    ///@notice To initiate or calculate the energy balance of the owner
    ///@param _owner who holds the energy and the vet
    ///
    ///@return how much energy the _owner holds
    function calRestBalance(address _owner) internal returns(uint256) {
        if (balances[_owner].timestamp != 0) {
            balances[_owner].balance = balanceOf(_owner);
            balances[_owner].vetamount = _owner.balance;
            balances[_owner].timestamp = now;
        } else {
            balances[_owner].balance = 0;
            balances[_owner].vetamount = _owner.balance;
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

    /// @notice the contract owner approves `_contractAddr` to transfer `_amount` tokens to `_to`
    /// @param _contractAddr The address of the contract able to transfer the tokens
    /// @param _reciever who recieved the `_amount` tokens
    /// @param _amount The amount of wei to be approved for transfer
    /// @return Whether the approval was successful or not
    
    function ownerApprove(address _contractAddr,address _reciever, uint256 _amount) public returns (bool success) {
        //only approved to a contract
        require(isContract(_contractAddr));
        //only transfer to a normal contract
        require(!isContract(_reciever));
        if (tx.origin == contractOwners[_contractAddr]) {
            allowed[_contractAddr][_reciever] = _amount;
            Approval(_contractAddr, _reciever, _amount);
            return true;
        }
        return false;
    }

    /// @param _owner The address of the account owning tokens
    /// @param _spender The address of the account able to transfer the tokens
    /// @return Amount of remaining tokens allowed to spent
    function allowance(address _owner, address _spender)  public constant returns (uint256 remaining) {
        return allowed[_owner][_spender];
    }

    /// @notice Allow `_reciever` to withdraw from your account, multiple times, up to the `tokens` amount.
    /// If this function is called again it overwrites the current allowance with _value.
    /// @param _reciever who recieves `_amount` tokens from your account
    /// @param _amount The address of the account able to transfer the tokens
    /// @return whether approved successfully
    function approve(address _reciever, uint256 _amount) public returns (bool success) {
        allowed[msg.sender][_reciever] = _amount;
        Approval(msg.sender, _reciever, _amount);
        return true;
    }

    /// @notice set the contract owner
    /// @param _contractAddr a contract address
    function setOwnerForContract(address _contractAddr,address _owner) public {
        //require(msg.sender == god);
        //_contractAddr must be a contract address
        require(isContract(_contractAddr));
        //caller must be an normal account address
        require(!isContract(_owner));

        if (contractOwners[_contractAddr] == 0) {
            contractOwners[_contractAddr] = _owner;
        }
        
    }

    /// @notice set a new owner for a contract which has been set a owner
    /// @param _contractAddr  a contract address
    /// @param _newOwner  an account address ,which will be the owner of _contractAddr
    function setNewOwnerForContract(address _contractAddr,address _newOwner) public {
        //_contractAddr must be a contract address
        require(isContract(_contractAddr));
        //_newOwner must be an normal account address
        require(!isContract(_newOwner));

        //the contract must be approved to set a new owner,and the caller must be its owner
        if (contractOwners[_contractAddr] == tx.origin) {
            contractOwners[_contractAddr] = _newOwner;
        }

    }

    /// @notice transferFromContract only called by the owner of _contractAddr,
    /// which means the msg.sender must be the owner of _contractAddr
    ///
    /// @param _from who send `_amount` tokens to `_to`
    /// @param _to a normal account address who recieves the balance
    /// @param _amount  balance for transferring
    /// @return success  whether transferred successfully
    function transferFrom(address _from,address _to,uint256 _amount) public returns(bool success) {
        //only the contract owner can transfer the energy to `_to`
        if (isContract(_from)) {
            require(tx.origin == contractOwners[_from]);
        }
        require(!isContract(_to));
        uint256 contractBalance = calRestBalance(_from);
        uint256 recipientBalance = calRestBalance(_to);
        if (contractBalance >= _amount && recipientBalance.add(_amount) > recipientBalance && allowed[_from][_to] >= _amount) {
            balances[_from].balance = balances[_from].balance.sub(_amount);
            balances[_to].balance = balances[_to].balance.add(_amount);
            allowed[_from][_to] = allowed[_from][_to].sub(_amount);
            Transfer(_from, _to, _amount);
            return true;
        }
        return false;
    }

    /// @param _addr an address of a normal account or a contract
    /// 
    /// @return whether `_addr` is a contract or not
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
