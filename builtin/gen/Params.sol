pragma solidity ^0.4.18;

contract Params {

    function n() internal view returns(ParamsNative) {
        return ParamsNative(this);
    }

    function executor() public view returns(address) {
        return n().nativeGetExecutor();
    }

    function set(bytes32 _key, uint256 _value) public {
        require(msg.sender == n().nativeGetExecutor());

        n().nativeSet(_key, _value);
        Set(_key, _value);
    }

    function get(bytes32 _key) public view returns(uint256) {
        return n().nativeGet(_key);
    }

    event Set(bytes32 indexed key, uint256 value);
}

contract ParamsNative {
    function nativeGetExecutor() public view returns(address);

    function nativeSet(bytes32 key, uint256 value) public;
    function nativeGet(bytes32 key) public view returns(uint256);
}