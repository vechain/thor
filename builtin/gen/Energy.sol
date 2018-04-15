pragma solidity ^0.4.18;
import "./Token.sol";
import "./ERC223Receiver.sol";

/// @title Energy an token that represents fuel for VET.
contract Energy is Token {
    mapping(address => mapping (address => uint256)) allowed;

    ///@return ERC20 token name
    function name() public pure returns (string) {
        return "VeThor";
    }

    ///@return ERC20 token decimals
    function decimals() public pure returns (uint8) {
        return 18;    
    }

    ///@return ERC20 token symbol
    function symbol() public pure returns (string) {
        return "VTHO";
    }

    ///@return ERC20 token total supply
    function totalSupply() public constant returns (uint256) {
        return EnergyNative(this).native_getTotalSupply();
    }

    function totalBurned() public constant returns(uint256) {
        return EnergyNative(this).native_getTotalBurned();
    }

    function balanceOf(address _owner) public view returns (uint256 balance) {
        return EnergyNative(this).native_getBalance(_owner);
    }

    // promise that it will not modify state when if returns false.
    function _transfer(address _from, address _to, uint256 _amount) internal returns (bool) {
        if (_amount > 0) {
            if (!EnergyNative(this).native_subBalance(_from, _amount)) {
                return false;
            }
            // believed that will never overflow
            EnergyNative(this).native_addBalance(_to, _amount);
        }
    
        if (isContract(_to)) {
            // Require proper transaction handling.
            ERC223Receiver(_to).tokenFallback(_from, _amount, new bytes(0));
        }
        Transfer(_from, _to, _amount);
        return true;
    }

    function transfer(address _to, uint256 _amount) public returns (bool success) {
        return _transfer(msg.sender, _to, _amount);
    }

    function transferFrom(address _from, address _to, uint256 _amount) public returns(bool success) {
        if (!_transfer(_from, _to, _amount)) {
            return false;
        }
        require(allowed[_from][_to] >= _amount);
        allowed[_from][_to] -= _amount;
        return true;
    }

    function allowance(address _owner, address _spender)  public view returns (uint256 remaining) {
        return allowed[_owner][_spender];
    }

    function approve(address _reciever, uint256 _amount) public returns (bool success) {
        allowed[msg.sender][_reciever] = _amount;
        Approval(msg.sender, _reciever, _amount);
        return true;
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
}

contract EnergyNative {
    function native_getTotalSupply() public view returns(uint256);
    function native_getTotalBurned() public view returns(uint256);
    
    function native_getBalance(address addr) public view returns(uint256);
    function native_addBalance(address addr, uint256 amount) public;
    function native_subBalance(address addr, uint256 amount) public returns(bool);
}