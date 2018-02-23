pragma solidity ^0.4.18;
import "./Token.sol";
import './ERC223Receiver.sol';

/// @title Energy an token that represents fuel for VET.
contract Energy is Token {
    mapping(address => mapping (address => uint256)) allowed;

    function n() internal view returns(EnergyNative) {
        return EnergyNative(this);
    }
  
    function executor() public view returns(address) {
        return n().nativeGetExecutor();
    }

    ///@return ERC20 token name
    function name() public pure returns (string) {
        return "Thor Power";
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
        return n().nativeGetTotalSupply();
    }

    function balanceOf(address _owner) public view returns (uint256 balance) {
        return n().nativeGetBalance(_owner);
    }

    function _transfer(address _from, address _to, uint256 _amount) internal returns(bool) {
        uint256 fromBalance = n().nativeGetBalance(_from);
        uint256 toBalance = n().nativeGetBalance(_to);        

        if (_amount > 0 && fromBalance >= _amount && toBalance + _amount > toBalance) {
            n().nativeSetBalance(_from, fromBalance - _amount);
            n().nativeSetBalance(_to, toBalance + _amount);
            if (isContract(_to)) {
                // Require proper transaction handling.
                ERC223Receiver(_to).tokenFallback(_from, _amount, new bytes(0));
            }
            Transfer(_from, _to, _amount);
            return true;
        }
        return false;        
    }

    function transfer(address _to, uint256 _amount) public returns (bool success) {
        return _transfer(msg.sender, _to, _amount);
    }

    function transferFrom(address _from,address _to,uint256 _amount) public returns(bool success) {
        if (_amount == 0 || allowed[_from][_to] < _amount)
            return false;

        if (_transfer(_from, _to, _amount)) {
            allowed[_from][_to] -= _amount;
            return true;
        }
        return false;
    }

    function allowance(address _owner, address _spender)  public view returns (uint256 remaining) {
        return allowed[_owner][_spender];
    }

    function approve(address _reciever, uint256 _amount) public returns (bool success) {
        allowed[msg.sender][_reciever] = _amount;
        Approval(msg.sender, _reciever, _amount);
        return true;
    }

    /// @notice the contract owner approves `_contractAddr` to transfer `_amount` tokens to `_to`
    /// @param _contractAddr The address of the contract able to transfer the tokens
    /// @param _to who receive the `_amount` tokens
    /// @param _amount The amount of wei to be approved for transfer
    /// @return Whether the approval was successful or not
    function transferFromContract(address _contractAddr, address _to, uint256 _amount) public returns (bool success) {
        require(msg.sender == n().nativeGetContractMaster(_contractAddr));        
        return _transfer(_contractAddr, _to, _amount);
    }
  
    ///@notice share `_credit` with `_to`. The shared credit can only be consumed, but never be transferred.
    ///@param _from who offers the credit
    ///@param _to who obtains the credit
    ///@param _credit max credit can be consumed
    ///@param _recoveryRate credit recovery rate in the unit of energy/second
    ///@param _expiration a timestamp, if block time exceeded that, the credit will be unusable.
    function share(address _from, address _to,uint256 _credit,uint256 _recoveryRate,uint64 _expiration) public {
        // the origin can be contract itself or master
        require(msg.sender == _from || msg.sender == n().nativeGetContractMaster(_from));

        //the expiration time should be greater than block time
        require(_expiration > now);

        //the _to should not be contract
        require(!isContract(_to));

        n().nativeSetSharing(_from, _to, _credit, _recoveryRate, _expiration);

        Share(_from, _to, _credit, _recoveryRate, _expiration);
    }

    ///@param _from who offered sharing
    ///@param _to who obtain the sharing
    ///@return remained sharing credit
    function getSharingRemained(address _from, address _to) public view returns (uint256) {
        return n().nativeGetSharingRemained(_from, _to);        
    }

    function getContractMaster(address _contractAddr) public view returns(address) {
        return n().nativeGetContractMaster(_contractAddr);
    }

    function setContractMaster(address _contractAddr, address _newMaster) public {
        address oldMaster = n().nativeGetContractMaster(_contractAddr);
        require(msg.sender == oldMaster);
        n().nativeSetContractMaster(_contractAddr, _newMaster);
        SetContractMaster(_contractAddr, oldMaster, _newMaster);
    }

    function adjustGrowthRate(uint256 rate) public {
        require(msg.sender == n().nativeGetExecutor());
        n().nativeAdjustGrowthRate(rate);

        AdjustGrowthRate(rate);
    }
    
    /// @param _addr an address of a normal account or a contract
    /// 
    /// @return whether `_addr` is a contract or not
    function isContract(address _addr) view internal returns(bool) {        
        if (_addr == 0) {
            return false;
        }
        uint size;
        assembly {
            size := extcodesize(_addr)
        }
        return size > 0;
    }
    
    
    event AdjustGrowthRate(uint256 rate);
    event Share(address indexed from, address indexed to, uint256 credit, uint256 recoveryRate, uint64 expiration);
    event SetContractMaster(address indexed contractAddr, address oldMaster, address newMaster);
}

contract EnergyNative {
    function nativeGetExecutor() public view returns(address);

    function nativeGetTotalSupply() public view returns(uint256);
    
    function nativeGetBalance(address addr) public view returns(uint256);
    function nativeSetBalance(address addr, uint256 balance) public;
    function nativeAdjustGrowthRate(uint256 rate) public;

    function nativeSetSharing(address from, address to, uint256 credit, uint256 recoveryRate, uint64 expiration) public;
    function nativeGetSharingRemained(address from, address to) public view returns(uint256);

    function nativeSetContractMaster(address contractAddr, address master) public;
    function nativeGetContractMaster(address contractAddr) public view returns(address);

}