// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

pragma solidity 0.4.24;

/// @title Params to manage governance parameters.
contract Params {
    function executor() public view returns(address) {
        return ParamsNative(this).native_executor();
    }

    function set(bytes32 _key, uint256 _value) public {
        require(msg.sender == executor(), "builtin: executor required");

        ParamsNative(this).native_set(_key, _value);
        emit Set(_key, _value);
    }

    function get(bytes32 _key) public view returns(uint256) {
        return ParamsNative(this).native_get(_key);
    }

    event Set(bytes32 indexed key, uint256 value);
}

contract ParamsNative {
    function native_executor() public view returns(address);

    function native_set(bytes32 key, uint256 value) public;
    function native_get(bytes32 key) public view returns(uint256);
}
