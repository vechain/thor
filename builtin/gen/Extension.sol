// Copyright (c) 2018 The VeChainThor developers
 
// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

pragma solidity ^0.4.18;

/// @title Evmlib implements evm lib functions.

contract Extension {
    function blake2b256(bytes _value) public view returns(bytes32) {
        return ExtensionNative(this).native_blake2b256(_value);
    }

    function blockID(uint32 num) public view returns(bytes32) {
        return ExtensionNative(this).native_getBlockIDByNum(num);
    }
}

contract ExtensionNative {
    function native_blake2b256(bytes _value) public view returns(bytes32);
    function native_getBlockIDByNum(uint32 num) public view returns(bytes32);
}