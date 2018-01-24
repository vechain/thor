pragma solidity ^0.4.18;
import "./Token.sol";
import './SafeMath.sol';
import './Voting.sol';
import './ERC223Receiver.sol';
/// @title Energy an token that represents fuel for VET.
contract Energy is Token {
    event Transfer(address indexed _from, address indexed _to, uint256 _value);
    event Approval(address indexed _owner, address indexed _spender, uint256 _value);
    event SetBalanceBirth(address indexed executor,uint256 time,uint256 birth);
    event ShareFrom(address indexed from,address indexed to,uint256 _limit,uint256 creditGrowthRate,uint256 expire);
    
    using SafeMath for uint256;
    
    uint64 public constant UNIT = 10**18;
    //which represents an owner's balance
    struct Balance {
        uint256 balance;    //the energy balance
        uint64 timestamp;   //for calculate how much energy would be grown
        uint128 vetamount;  //vet balance
        uint64 version;     //index of `snapGRs`
    }

    //which represents the detail info for a shared energy credits
    struct SharedCredit {
        uint256 limit;            //max availableCredits        
        uint64 expire;            //expiration time
        uint64 creditGrowthRate;  //how many credits grown in a second

        uint256 creditsUsed;      //credits can be consumed
        uint64 snapTime;          //current block number
    }
    
    //balance growth rate at `timestamp`
    struct BalanceBirth {
        uint256 timestamp;  // `timestamp` changes if birth updated.
        uint256 birth;      // how many tokens grown by per vet per second
    }
    
    BalanceBirth[] birthRevisions;

    //save all owner's balance
    mapping(address => Balance) balances;
    mapping(address => mapping (address => uint256)) allowed;
    //owners of the contracts, which can only be used to transfer energy from contract
    mapping (address => address) contractOwners;
    //an array that stores all energy credits
    mapping (bytes32 => SharedCredit) public sharedCredits;
    
    address public voting; 
    
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

  
    ///@notice share `_amount` energy credits from `_from` to `_to` which can only be consumed,never transferred
    ///@param _reciever who recieves the energy credits
    ///@param _limit max credits can be consumed
    ///@param _creditGrowthRate how mange credits grown in a second
    ///@param _expire a timestamp, if block time exceeded that, this shared energy credits would be useless
    function shareTo(address _reciever,uint256 _limit,uint64 _creditGrowthRate,uint64 _expire) public {
        //sharing credits can only be called by a contract
        require(isContract(msg.sender));
        //the contract caller should be a normal amount
        require(!isContract(_reciever));
        //the expiration time should be greater than block time
        require(_expire > now);
        
        address _from = msg.sender;
        ShareFrom(_from, _reciever, _limit, _creditGrowthRate, _expire);
        //hash the caller address and the contract address to ensure the key unique
        bytes32 key = keccak256(_from,_reciever);
        sharedCredits[key] = SharedCredit(_limit, _expire, _creditGrowthRate, 0, uint64(now));

    }

    ///@param _reciever who recieved the credits
    ///@param _from which is a contract that shares the to _reciever
    ///
    ///@return how many credits is shared
    function getAvailableCredits(address _from,address _reciever) public view returns (uint256) {
        //hash the caller address and the contract address to ensure the key unique.
        bytes32 key = keccak256(_from,_reciever);
        SharedCredit storage s = sharedCredits[key];
        
        if (now > s.expire) {
            return 0;
        }

        if ( s.creditsUsed >= s.limit ) {
            return 0;
        }

        uint256 growth = now.sub(s.snapTime).mul(s.creditGrowthRate);
        if (growth >= s.creditsUsed) {
            return s.limit;
        } 
        return s.limit.sub(s.creditsUsed.sub(growth));
        
    }

    ///@notice consume `_amount` tokens or credits of `_caller`
    ///@param _callee `_callee` who shared the credits to `_callee`
    ///@param _caller credits of `_caller` would be consumed 
    ///@param _amount   `_amount` tokens would be consumed
    ///@return _consumer credits of `_consumer` would be consumed
    function consume(address _caller, address _callee, uint256 _amount) public returns(address _consumer) {
        //only called by `thor`
        require(msg.sender == address(this));

        uint256 ac = getAvailableCredits(_callee,_caller);
        if (ac >= _amount) {
            bytes32 key = keccak256(_callee,_caller);
            sharedCredits[key].snapTime = uint64(now);
            sharedCredits[key].creditsUsed = sharedCredits[key].creditsUsed.add(_amount);
            return _callee;
        }
        
        uint256 b = updateBalance(_caller); 
        if (b < _amount) {
            revert();
        }
        balances[_caller].balance = b.sub(_amount);
        return _caller;

    }

    ///@notice charge `_amount` tokens to `_reciever`
    ///@param _reciever `_reciever` recieves `_amount` tokens
    ///@param _amount `_amount` send to `_reciever`
    function charge(address _reciever, uint256 _amount) public {
        require(msg.sender == address(this));
        
        uint256 b = updateBalance(_reciever); 
        balances[_reciever].balance = b.add(_amount);
    }

    ///@param _owner who holds the energy and the vet
    ///
    ///@return how much energy the _owner holds
    function balanceOf(address _owner) public constant returns (uint256 balance) {
        uint256 time = balances[_owner].timestamp;
        if ( time == 0 ) {
            return 0;
        }

        uint256 revisionLen = lengthOfRevisions();
        uint256 amount = balances[_owner].balance;
        if ( revisionLen == 0 ) {
            return amount;   
        }

        uint256 vetamount = balances[_owner].vetamount;
        uint256 version = balances[_owner].version;
        if ( timeWithVer(revisionLen-1) <= time || version == revisionLen - 1 ) {
            return amount.add(birthWithVer(revisionLen-1).mul(vetamount.mul(now.sub(time))).div(UNIT));
        }

        //`_owner` has not operated his account for a long time
        for ( uint256 i = version; i < revisionLen; i++ ) {
            uint256 currentBirth = birthWithVer(i);
            uint256 currentTime = timeWithVer(i);
            if ( i == version ) {
                uint256 nextTime = timeWithVer(i+1);
                amount = amount.add(currentBirth.mul(vetamount.mul(nextTime.sub(time))).div(UNIT));
                continue;
            }
            
            if ( i == revisionLen - 1 ) {
                amount = amount.add(currentBirth.mul(vetamount.mul(now.sub(currentTime))).div(UNIT));
                return amount;
            }

            uint256 nTime = timeWithVer(i+1);
            amount = amount.add(currentBirth.mul(vetamount.mul(nTime.sub(currentTime))).div(UNIT));
        }
    }

    ///@notice To initiate or calculate the energy balance of the owner
    ///@param _owner an account
    ///
    ///@return how much energy `_owner` holds
    function updateBalance(address _owner) public returns(uint256) {
        if (balances[_owner].timestamp == 0) {
            balances[_owner].timestamp = uint64(now);
        }

        balances[_owner].balance = balanceOf(_owner);
        balances[_owner].vetamount = _owner.balance.toUINT128();
        balances[_owner].timestamp = uint64(now);
        uint256 revisionLen = lengthOfRevisions();
        if ( revisionLen > 0 ) {
            balances[_owner].version = uint64(revisionLen - 1);
        }
        return balances[_owner].balance;
    }

    /// @notice send `_amount` token to `_to` from `msg.sender`
    /// @param _to The address of the recipient
    /// @param _amount The amount of token to be transferred
    /// @return Whether the transfer was successful or not
    function transfer(address _to, uint256 _amount) public returns (bool success) {
        uint256 senderBalance = updateBalance(msg.sender);
        uint256 recipientBalance = updateBalance(_to);
        
        if (_amount > 0 && senderBalance >= _amount && recipientBalance.add(_amount) > recipientBalance) {
            balances[msg.sender].balance = balances[msg.sender].balance.sub(_amount);
            balances[_to].balance = balances[_to].balance.add(_amount);
            if (isContract(_to)) {
                // Require proper transaction handling.
                ERC223Receiver receiver = ERC223Receiver(_to);
                receiver.tokenFallback(msg.sender, _amount, msg.data);
            }
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
    /// If this function is called again it overwrites the current allowance with _amount.
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
        require(msg.sender == address(this));
        //_contractAddr must be a contract address
        require(isContract(_contractAddr));
        //_owner must be a normal account address
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

        uint256 contractBalance = updateBalance(_from);
        uint256 recipientBalance = updateBalance(_to);
        if (contractBalance >= _amount && recipientBalance.add(_amount) > recipientBalance && allowed[_from][_to] >= _amount) {
            balances[_from].balance = balances[_from].balance.sub(_amount);
            balances[_to].balance = balances[_to].balance.add(_amount);
            allowed[_from][_to] = allowed[_from][_to].sub(_amount);
            if (isContract(_to)) {
                // Require proper transaction handling.
                ERC223Receiver receiver = ERC223Receiver(_to);
                receiver.tokenFallback(msg.sender, _amount, msg.data);
            }
            Transfer(_from, _to, _amount);
            return true;
        }
        return false;
    }

    function initialize(address _voting) public {
        require(msg.sender == address(this));

        voting = _voting;        
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
    
    ///@notice adjust balance growth rate to `_birth`
    ///@param _birth how much energy grows by per vet per second
    function setBalanceBirth(uint256 _birth) public {
        require(msg.sender == voting);
        require(_birth > 0);
        uint256 len = lengthOfRevisions();
        if (len > 0) {
            if (now == timeWithVer(len-1)) {
                birthRevisions[len-1].birth = _birth;
                SetBalanceBirth(msg.sender,now,_birth);
                return;
            }
        }
        birthRevisions.push(BalanceBirth(now,_birth));
        SetBalanceBirth(msg.sender,now,_birth);
    }

    function birthWithVer(uint256 version) public view returns(uint256) {
        require(version <= lengthOfRevisions()-1);
        if (birthRevisions.length == 0) {
            return 0;
        }
        return birthRevisions[version].birth;
    }

    function timeWithVer(uint256 version) public view returns(uint256) {
        require(version <= lengthOfRevisions()-1);
        if (birthRevisions.length == 0) {
            return 0;
        }
        return birthRevisions[version].timestamp;
    }

    function lengthOfRevisions() public view returns(uint256) {
        return birthRevisions.length;
    }
}
