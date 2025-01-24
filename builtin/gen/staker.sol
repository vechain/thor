//SPDX-License-Identifier: MIT
pragma solidity ^0.8.20;

contract Staker {
    mapping(address => uint256) private _stakes;
    uint256 private _totalStaked;

    event Staked(address indexed staker, uint256 amount);
    event Unstaked(address indexed staker, uint256 amount);

    modifier hasStake(uint256 amount) {
        require(_stakes[msg.sender] >= amount, "Staker: Insufficient balance");
        _;
    }

    function stake() external payable {
        require(msg.value > 0, "Staker: Stake amount must be greater than 0");
        _stakes[msg.sender] += msg.value;
        _totalStaked += msg.value;
        emit Staked(msg.sender, msg.value);
    }

    function getStake(address staker) external view returns (uint256) {
        return _stakes[staker];
    }

    function unstake(uint256 amount) external hasStake(amount) {
        _stakes[msg.sender] -= amount;
        _totalStaked -= amount;
        payable(msg.sender).transfer(amount);
        emit Unstaked(msg.sender, amount);
    }

    function totalStakes() external view returns (uint256) {
        return _totalStaked;
    }
}

// TODO: Implement the native integration
abstract contract StakerNative {
    function native_totalStake() public virtual view returns(uint256);
    function native_stake() public virtual payable;
    function native_unstake(uint256 amount) public virtual;
    function native_getStake(address staker) public virtual view returns(uint256);
}
