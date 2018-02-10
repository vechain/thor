pragma solidity ^0.4.18;

contract Params {

    function voting() public view returns(address) {
        return this.nativeGetVoting();
    }

    function set(bytes32 _key, uint256 _value) public {
        require(msg.sender == this.nativeGetVoting());

        this.nativeSet(_key, _value);
        Set(_key, _value);
    }

    function get(bytes32 _key) public view returns(uint256) {
        return this.nativeGet(_key);
    }

    function nativeGetVoting() public view returns(address) {}

    function nativeSet(bytes32 key, uint256 value) public {}
    function nativeGet(bytes32 key) public view returns(uint256) {}

    event Set(bytes32 indexed key, uint256 value);
}
