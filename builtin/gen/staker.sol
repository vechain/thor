//SPDX-License-Identifier: MIT
pragma solidity ^0.8.20;

contract Staker {
    mapping(address => uint256) private _stakes;
    uint256 private _totalStaked;

    event Staked(address indexed staker, uint256 amount);
    event Unstaked(address indexed staker, uint256 amount);

    function stake() public payable {
        StakerNative(address(this)).native_stake(msg.value, msg.sender);
    }

    function getStake(address staker) public view returns (uint256) {
        return StakerNative(address(this)).native_getStake(staker);
    }

    function unstake(uint256 amount) public {
        return StakerNative(address(this)).native_unstake(amount, msg.sender);
    }

    function totalStake() public view returns (uint256) {
        return StakerNative(address(this)).native_totalStake();
    }
}

// TODO: Implement the native integration
interface StakerNative {
    function native_totalStake() external pure returns(uint256);
    function native_stake(uint256 amount, address staker) external;
    function native_unstake(uint256 amount, address staker) external;
    function native_getStake(address staker) external pure returns(uint256);
}
