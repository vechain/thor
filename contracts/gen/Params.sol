pragma solidity ^0.4.18;

contract Params {
    // address of voting contract, which controls the params.
    address public voting;

    function _initialize(address _voting) public {
        require(msg.sender == address(this));
        voting = _voting;
    }

    function set(string _key, int256 _value) public {
        require(msg.sender == voting || msg.sender == address(this));

        this.nativeSet(_key, _value);        
        Set(_key, _key, _value);
    }

    function get(string _key) public view returns(int256) {
        return this.nativeGet(_key);
    }

    function nativeSet(string key, int256 value) public {}
    function nativeGet(string key) public view returns(int256) {}

    event Set(string indexed _indexedKey, string _key, int256 _value);
}
