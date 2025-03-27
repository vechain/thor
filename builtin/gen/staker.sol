//SPDX-License-Identifier: MIT
pragma solidity ^0.8.20;

contract Staker {
    event ValidatorQueued(
        address indexed endorsor,
        address indexed master,
        uint32 period,
        uint256 stake,
        bool autoRenew
    );
    event ValidatorWithdrawn(
        address indexed endorsor,
        address indexed master,
        uint256 stake
    );
    event ValidatorUpdatedAutoRenew(
        address indexed endorsor,
        address indexed master,
        bool autoRenew
    );
    event StakeIncreased(
        address indexed endorsor,
        address indexed master,
        uint256 stake,
        uint256 added
    );
    event StakeDecreased(
        address indexed endorsor,
        address indexed master,
        uint256 stake,
        uint256 removed
    );

    /**
     * @dev totalStake returns all stakes by queued and active validators.
     */
    function totalStake() public view returns (uint256) {
        return StakerNative(address(this)).native_totalStake();
    }

    /**
     * @dev addValidator adds a validator to the queue.
     */
    function addValidator(
        address master,
        uint32 period,
        bool autoRenew
    ) public payable {
        StakerNative(address(this)).native_addValidator(
            msg.sender,
            master,
            period,
            msg.value,
            autoRenew
        );
        emit ValidatorQueued(msg.sender, master, period, msg.value, autoRenew);
    }

    /**
     * @dev increaseStake adds VET to the current stake of the queued/active validator.
     */
    function increaseStake(address master) public payable {
        require(msg.value > 0, "value is empty");
        uint256 stake = StakerNative(address(this)).native_increaseStake(
            msg.sender,
            master,
            msg.value
        );
        emit StakeIncreased(msg.sender, master, stake, msg.value);
    }

    /**
     * @dev decreaseStake removes VET from the current stake of an active validator
     */
    function decreaseStake(address master) public payable {
        require(msg.value > 0, "value is empty");
        uint256 stake = StakerNative(address(this)).native_decreaseStake(
            msg.sender,
            master,
            msg.value
        );
        emit StakeDecreased(msg.sender, master, stake, msg.value);
    }

    /**
     * @dev allows the caller to withdraw a stake when their status is set to exited
     */
    function withdraw(address master) public {
        uint256 stake = StakerNative(address(this)).native_withdraw(
            msg.sender,
            master
        );
        emit ValidatorWithdrawn(msg.sender, master, stake);

        (bool success, ) = msg.sender.call{value: stake}("");
        require(success, "Transfer failed");
    }

    /**
     * @dev get returns the stake, weight and status of a validator.
     */
    function get(
        address master
    ) public view returns (address, uint256, uint256, uint8) {
        return StakerNative(address(this)).native_get(master);
    }

    /**
     * @dev getWithdraw returns the Endorsor, Availability, amount of a validator's withdrawal.
     */
    function getWithdraw(
        address master
    ) public view returns (address, bool, uint256) {
        return StakerNative(address(this)).native_getWithdraw(master);
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

    /**
     * @dev updateAutoRenew updates the autoRenew flag of a validator.
     */
    function updateAutoRenew(address master, bool autoRenew) public {
        StakerNative(address(this)).native_updateAutoRenew(
            msg.sender,
            master,
            autoRenew
        );
        emit ValidatorUpdatedAutoRenew(msg.sender, master, autoRenew);
    }

    receive() external payable {
        revert("receive function not allowed");
    }

    fallback() external {
        revert("fallback function not allowed");
    }
}

interface StakerNative {
    // Write methods
    function native_addValidator(
        address endorsor,
        address master,
        uint32 period,
        uint256 stake,
        bool autoRenew
    ) external;

    function native_increaseStake(
        address endorsor,
        address master,
        uint256 stake
    ) external returns (uint256);

    function native_decreaseStake(
        address endorsor,
        address master,
        uint256 stake
    ) external returns (uint256);

    function native_withdraw(
        address endorsor,
        address master
    ) external returns (uint256);

    // Read methods
    function native_totalStake() external pure returns (uint256);

    function native_get(
        address master
    ) external view returns (address, uint256, uint256, uint8);

    function native_getWithdraw(
        address master
    ) external view returns (address, bool, uint256);

    function native_firstActive() external view returns (address);

    function native_firstQueued() external view returns (address);

    function native_next(address prev) external view returns (address);

    function native_updateAutoRenew(
        address endorsor,
        address master,
        bool autoRenew
    ) external;
}
