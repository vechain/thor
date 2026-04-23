// SPDX-License-Identifier: MIT
pragma solidity ^0.8.24;

contract Counter {
    uint256 public count;
    string public lastMessage;

    event CounterIncreased(address indexed by, uint256 newCount, string message);

    function increment(string calldata message) external {
        count += 1;
        lastMessage = message;
        emit CounterIncreased(msg.sender, count, message);
    }
}
