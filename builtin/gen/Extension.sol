pragma solidity ^0.4.18;

/// @title Evmlib implements evm lib functions.

contract Extension {
    function blake2b256(bytes _value) public view returns(bytes32) {
        return ExtensionNative(this).native_blake2b256(_value);
    }
}

contract ExtensionNative {
    function native_blake2b256(bytes _value) public view returns(bytes32);
}