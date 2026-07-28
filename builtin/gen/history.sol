//SPDX-License-Identifier: LGPL-3.0
// Copyright (c) 2026 The VeChainThor developers
 
// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

pragma solidity 0.8.20;

/// @title History serves historical block hashes via the EIP-2935 calling convention.
///
/// Calldata is exactly 32 bytes — a uint256 block number with no function
/// selector. Dispatch therefore goes through the fallback function. All
/// invalid inputs revert with empty returndata (matching the EIP-2935
/// reference bytecode's `revert(0, 0)`) so dApps written for ETH mainnet
/// observe identical failure semantics on Thor.
contract History {
    uint256 constant HISTORY_SERVE_WINDOW = 8191;

    // builtin.Extension.Address == thor.BytesToAddress([]byte("Extension"))
    address constant EXTENSION = 0x0000000000000000000000457874656E73696F6e;

    fallback(bytes calldata input) external returns (bytes memory) {
        if (input.length != 32) revert();
        uint256 num = abi.decode(input, (uint256));
        if (num >= block.number || block.number - num > HISTORY_SERVE_WINDOW) revert();

        bytes32 result = _Extension(EXTENSION).blockID(num);

        assembly {
            mstore(0, result)
            return(0, 32)
        }
    }
}

/// @dev Minimal interface against the 0.4.24-compiled Extension builtin
/// (see builtin/gen/extension.sol). Underscore prefix follows the repo
/// convention (see _Token, _proto_helper) — the `_*` cleanup step in
/// gen.go drops the unused artifact from compiled/.
interface _Extension {
    function blockID(uint256 num) external view returns (bytes32);
}
