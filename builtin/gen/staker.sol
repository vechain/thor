//SPDX-License-Identifier: MIT
pragma solidity ^0.8.20;

contract Staker {
    mapping(address => mapping(address => uint256)) private _stakes;
    address[] private _validators;

    event ValidatorQueued(address indexed validator, uint256 stake);
    event ValidatorWithdrawn(address indexed validator, uint256 stake);

    /**
    * @dev totalStake returns all stakes by queued and active validators.
    */
    function totalStake() public view returns (uint256) {
        return StakerNative(address(this)).native_totalStake();
    }
    /**
    * @dev activeStake returns all stakes by active validators.
    */
    function activeStake() public view returns (uint256) {
        return StakerNative(address(this)).native_activeStake();
    }

    /**
    * @dev addValidator adds a validator to the queue.
    */
    function addValidator() public payable {
        StakerNative(address(this)).native_addValidator(msg.sender, msg.value);
        emit ValidatorQueued(msg.sender, msg.value);
    }

    /**
    * @dev allows the caller to withdraw a stake when their status is set to exited
    */
    function withdraw() public {
        uint256 stake = StakerNative(address(this)).native_withdraw();
        emit ValidatorWithdrawn(msg.sender, stake);
        payable(msg.sender).transfer(stake);
    }

    /**
    * @dev get returns the stake, weight and status of a validator.
    */
    function get(address validator) public view returns (uint256, uint256, uint8) {
        return StakerNative(address(this)).native_get(validator);
    }

    /**
    * @dev firstActive returns the head address of the active validators.
    */
    function firstActive() public view returns (address) {
        return StakerNative(address(this)).native_firstActive();
    }

    /**
    * @dev firstQueued returns the head address of the queued validators.
    */
    function firstQueued() public view returns (address) {
        return StakerNative(address(this)).native_firstQueued();
    }

    /**
    * @dev next returns the validator in a linked list
    */
    function next(address prev) public view returns (address) {
        return StakerNative(address(this)).native_next(prev);
    }
}

interface StakerNative {
    // Write methods
    function native_addValidator(address validator, uint256 stake) external;
    function native_withdraw() external returns (uint256);

    // Read methods
    function native_totalStake() external pure returns(uint256);
    function native_activeStake() external view returns(uint256);
    function native_get(address validator) external view returns (uint256, uint256, uint8);
    function native_firstActive() external view returns (address);
    function native_firstQueued() external view returns (address);
    function native_next(address prev) external view returns (address);
}
