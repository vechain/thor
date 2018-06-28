// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

pragma solidity 0.4.24;

/// @title Extension extends EVM global functions.
contract Extension {
    function blake2b256(bytes data) public view returns(bytes32) {
        return ExtensionNative(this).native_blake2b256(data);
    }

    function blockID(uint num) public view returns(bytes32) {
        return ExtensionNative(this).native_blockID(uint32(num));
    }

    function blockTotalScore(uint num) public view returns(uint64) {
        return ExtensionNative(this).native_blockTotalScore(uint32(num));
    }

    function blockTime(uint num) public view returns(uint) {
        return ExtensionNative(this).native_blockTime(uint32(num));
    }

    function blockSigner(uint num) public view returns(address) {
        return ExtensionNative(this).native_blockSigner(uint32(num));
    }

    function totalSupply() public view returns(uint256) {
        return ExtensionNative(this).native_totalSupply();
    }

    function txProvedWork() public view returns(uint256) {
        return ExtensionNative(this).native_txProvedWork();
    }

    function txID() public view returns(bytes32) {
        return ExtensionNative(this).native_txID();
    }

    function txBlockRef() public view returns(bytes8) {
        return ExtensionNative(this).native_txBlockRef();
    }

    function txExpiration() public view returns(uint) {
        return ExtensionNative(this).native_txExpiration();
    }
}

contract ExtensionNative {
    function native_blake2b256(bytes data) public view returns(bytes32);
    function native_blockID(uint32 num) public view returns(bytes32);
    function native_blockTotalScore(uint32 num) public view returns(uint64);
    function native_blockTime(uint32 num) public view returns(uint64);
    function native_blockSigner(uint32 num)public view returns(address);
    function native_totalSupply()public view returns(uint256);
    function native_txProvedWork()public view returns(uint256);
    function native_txID()public view returns(bytes32);
    function native_txBlockRef()public view returns(bytes8);
    function native_txExpiration()public view returns(uint32);
}
