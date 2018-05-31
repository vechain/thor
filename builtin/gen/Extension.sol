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

    function blockTotalScore(uint32 num) public view returns(uint64) {
        return ExtensionNative(this).native_getTotalScoreByNum(num);
    }

    function blockTime(uint32 num) public view returns(uint64) {
        return ExtensionNative(this).native_getTimestampByNum(num);
    }

    function blockProposer(uint32 num) public view returns(address) {
        return ExtensionNative(this).native_getProposerByNum(num);
    }

    function totalSupply() public view returns(uint256) {
        return ExtensionNative(this).native_getTokenTotalSupply();
    }

    function txProvedWork() public view returns(uint256) {
        return ExtensionNative(this).native_getTransactionProvedWork();
    }
}

contract ExtensionNative {
    function native_blake2b256(bytes _value) public view returns(bytes32);
    function native_getBlockIDByNum(uint32 num) public view returns(bytes32);
    function native_getTotalScoreByNum(uint32 num) public view returns(uint64);
    function native_getTimestampByNum(uint32 num) public view returns(uint64);
    function native_getProposerByNum(uint32 num)public view returns(address);
    function native_getTokenTotalSupply()public view returns(uint256);
    function native_getTransactionProvedWork()public view returns(uint256);    
}