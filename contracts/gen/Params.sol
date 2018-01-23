pragma solidity ^0.4.18;
import './Owned.sol';

contract Params {
    // address of voting contract, which controls the params.
    address public voting;

    mapping(string=>uint256) values;

    function sysInitialize(address _voting) public {
        require(msg.sender == address(this));
        voting = _voting;        
    }

    function sysPreset(string _key, uint256 _value) public {
        require(msg.sender == address(this));
        values[_key] = _value;
    }

    function get(string _key) public view returns(uint256) {
        return values[_key];
    }

    function set(string _key, uint256 _value) public {
        require(msg.sender == voting);
        values[_key] = _value;
        
        Set(_key, _key, _value);
    }

    event Set(string indexed _indexedKey, string _key, uint256 _value);
}
