// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

pragma solidity 0.4.24;

import './extension-v2.sol';

/// @title ExtensionV3 extends EVM global functions.
contract ExtensionV3 is ExtensionV2 {

    /**
    * @dev Get the index of the current clause in the transaction.
    */
    function txClauseIndex() public view returns (uint32) {
        return ExtensionV3Native(this).native_txClauseIndex();
    }

    /**
    * @dev Get the total number of clauses in the transaction.
    */
    function txClauseCount() public view returns (uint32) {
        return ExtensionV3Native(this).native_txClauseCount();
    }
}

contract ExtensionV3Native is ExtensionV2Native {
    function native_txClauseCount() public view returns (uint32);

    function native_txClauseIndex()public view returns(uint32);
}
