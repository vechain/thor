// Copyright (c) 2018 The VeChainThor developers
 
// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

pragma solidity 0.4.24;

/// @title to measure basic gas usage for external call.
contract Measure {
    function outer() public view {
        this.inner();
    }
    function inner() public pure {}    
}