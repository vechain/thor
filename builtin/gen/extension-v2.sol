// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

pragma solidity 0.4.24;
import './extension.sol';

/// @title Extension extends EVM global functions.
contract ExtensionV2 is Extension {
    function txGasPayer() public view returns(address) {
        return ExtensionV2Native(this).native_txGasPayer();
    }
}

contract ExtensionV2Native is ExtensionNative {    
    function native_txGasPayer()public view returns(address);
}
