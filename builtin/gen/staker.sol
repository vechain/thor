//SPDX-License-Identifier: MIT
pragma solidity ^0.8.20;

contract Staker {
    mapping(address => mapping(address => uint256)) private _stakes;
    address[] private _validators;

    event Staked(address indexed staker, uint256 amount, address indexed validator);
    event Unstaked(address indexed staker, uint256 amount, address indexed validator);
    event ValidatorAdded(address indexed validator);
    event ValidatorRemoved(address indexed validator);

    function stake(address validator) public payable {
        StakerNative(address(this)).native_stake(msg.value, msg.sender, validator);
    }

    function getStake(address staker, address validator) public view returns (uint256) {
        return StakerNative(address(this)).native_getStake(staker, validator);
    }

    function unstake(uint256 amount, address validator) public {
        return StakerNative(address(this)).native_unstake(amount, msg.sender, validator);
    }

    function totalStake() public view returns (uint256) {
        return StakerNative(address(this)).native_totalStake();
    }

    function addValidator() public payable {
        return StakerNative(address(this)).native_addValidator(msg.value, msg.sender);
    }

    function removeValidator() public {
        return StakerNative(address(this)).native_removeValidator(msg.sender);
    }

    function listValidators() public view returns (address[] memory) {
        return StakerNative(address(this)).native_listValidators();
    }
}

// TODO: Implement the native integration
interface StakerNative {
    function native_totalStake() external pure returns(uint256);
    function native_stake(uint256 amount, address staker, address validator) external;
    function native_unstake(uint256 amount, address staker, address validator) external;
    function native_getStake(address staker, address validator) external pure returns(uint256);
    function native_addValidator(uint256 amount, address validator) external;
    function native_removeValidator(address validator) external;
    function native_listValidators() external view returns (address[] memory);
}
