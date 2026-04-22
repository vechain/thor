// SPDX-License-Identifier: MIT
pragma solidity ^0.8.24;

contract Counter {
    uint256 public count;

    event CounterIncreased(address indexed by, uint256 newCount);

    function increment() external {
        count += 1;
        emit CounterIncreased(msg.sender, count);
    }
}
