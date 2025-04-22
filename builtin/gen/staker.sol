//SPDX-License-Identifier: MIT
pragma solidity ^0.8.20;

contract Staker {
    event ValidatorQueued(
        address indexed endorsor,
        address indexed master,
        bytes32 indexed validationID,
        uint32 period,
        uint256 stake,
        bool autoRenew
    );
    event ValidatorWithdrawn(address indexed endorsor, bytes32 indexed validationID,uint256 stake);
    event ValidatorUpdatedAutoRenew(address indexed endorsor, bytes32 indexed validationID, bool autoRenew);

    event StakeIncreased(address indexed endorsor, bytes32 indexed validationID,uint256 added);
    event StakeDecreased(address indexed endorsor, bytes32 indexed validationID,uint256 removed);

    event DelegationAdded(
        bytes32 indexed validationID,
        address indexed delegator,
        uint256 stake,
        bool autoRenew,
        uint8 multiplier
    );
    event DelegationWithdrawn(bytes32 indexed validationID, address indexed delegator, uint256 stake);
    event DelegationUpdatedAutoRenew(bytes32 indexed validationID, address indexed delegator, bool autoRenew);

    /**
     * @dev totalStake returns all stakes by queued and active validators.
     */
    function totalStake() public view returns (uint256) {
        return StakerNative(address(this)).native_totalStake();
    }

    /**
     * @dev queuedStake returns all stakes which are queued
     */
    function queuedStake() public view returns (uint256) {
        return StakerNative(address(this)).native_queuedStake();
    }

    /**
     * @dev addValidator adds a validator to the queue.
     */
    function addValidator(
        address master,
        uint32 period,
        bool autoRenew
    ) public payable {
        bytes32 id = StakerNative(address(this)).native_addValidator(
            msg.sender,
            master,
            period,
            msg.value,
            autoRenew
        );
        emit ValidatorQueued(msg.sender, master, id,period, msg.value, autoRenew);
    }

    /**
     * @dev increaseStake adds VET to the current stake of the queued/active validator.
     */
    function increaseStake(bytes32 validationID) public payable {
        require(msg.value > 0, "value is empty");
         StakerNative(address(this)).native_increaseStake(
            msg.sender,
            validationID,
            msg.value
        );
        emit StakeIncreased(msg.sender, validationID, msg.value);
    }

    /**
     * @dev decreaseStake removes VET from the current stake of an active validator
     */
    function decreaseStake(bytes32 id, uint256 amount) public {
        StakerNative(address(this)).native_decreaseStake(
            msg.sender,
            id,
            amount
        );
        emit StakeDecreased(msg.sender, id, amount);
    }

    /**
     * @dev allows the caller to withdraw a stake when their status is set to exited
     */
    function withdraw(bytes32 id) public {
        uint256 stake = StakerNative(address(this)).native_withdraw(
            msg.sender,
            id
        );
        emit ValidatorWithdrawn(msg.sender, id, stake);

        (bool success, ) = msg.sender.call{value: stake}("");
        require(success, "Transfer failed");
    }

    /**
     * @dev updateAutoRenew updates the autoRenew flag of a validator.
     */
    function updateAutoRenew(bytes32 id, bool autoRenew) public {
        StakerNative(address(this)).native_updateAutoRenew(
            msg.sender,
            id,
            autoRenew
        );
        emit ValidatorUpdatedAutoRenew(msg.sender, id, autoRenew);
    }

    /**
    * @dev addDelegation delegates VET to a validator, for the given delegator
    */
    function addDelegation(
        bytes32 validationID,
        address delegator,
        bool autoRenew,
        uint8 multiplier // (% of msg.value) 100 for x1, 200 for x2, etc. This enforces a maximum of 2.56x multiplier
    ) public payable onlyDelegatorContract()  {
        require(msg.value > 0, "value is empty");
        StakerNative(address(this)).native_addDelegation(
            validationID,
            delegator,
            msg.value,
            autoRenew,
            multiplier
        );
        emit DelegationAdded(validationID, delegator, msg.value, autoRenew, multiplier);
    }

    /**
     * @dev exitDelegation signals the intent for the delegator to exit at the end of the staking period.
     * Funds are available once the current staking period ends.
     */
    function updateDelegatorAutoRenew(bytes32 validationID, address delegator, bool active) public onlyDelegatorContract() {
        StakerNative(address(this)).native_updateDelegatorAutoRenew(validationID, delegator, active);
        emit DelegationUpdatedAutoRenew(validationID, delegator, active);
    }

    /**
     * @dev withdrawDelegation allows the delegator to withdraw all of the VET that is not currently locked.
     */
    function withdrawDelegation(bytes32 validationID, address delegator) public onlyDelegatorContract() {
        uint256 stake = StakerNative(address(this)).native_withdrawDelegation(validationID, delegator);
        emit DelegationWithdrawn(validationID, delegator, stake);
        (bool success, ) = msg.sender.call{value: stake}("");
        require(success, "Transfer failed");
    }

    /**
     * @dev fetDelegation returns the delegation id, amount and status of a validator.
     * @return (stake, multiplier, autoRenew)
     */
    function getDelegation(
        bytes32 validationID,
        address delegator
    ) public view returns (uint256, uint8, bool) {
        return StakerNative(address(this)).native_getDelegation(validationID, delegator);
    }

    /**
     * @dev get returns the stake, weight and status of a validator.
     * @return (master, endorser, stake, weight, status, autoRenew)
     */
    function get(
        bytes32 id
    ) public view returns (address, address, uint256, uint256, uint8, bool) {
        return StakerNative(address(this)).native_get(id);
    }

    /**
     * @dev getWithdraw returns the Endorsor, Availability, amount of a validator's withdrawal.
     */
    function getWithdraw(
        bytes32 id
    ) public view returns (uint256) {
        return StakerNative(address(this)).native_getWithdraw(id);
    }

    /**
     * @dev firstActive returns the head address of the active validators.
     */
    function firstActive() public view returns (bytes32) {
        return StakerNative(address(this)).native_firstActive();
    }

    /**
     * @dev firstQueued returns the head address of the queued validators.
     */
    function firstQueued() public view returns (bytes32) {
        return StakerNative(address(this)).native_firstQueued();
    }

    /**
     * @dev next returns the validator in a linked list
     */
    function next(bytes32 prev) public view returns (bytes32) {
        return StakerNative(address(this)).native_next(prev);
    }

    modifier onlyDelegatorContract {
        require(msg.sender == StakerNative(address(this)).native_getDelegatorContract(), "builtin: only delegator");
        _;
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
    ) external returns (bytes32);

    function native_increaseStake(
        address endorsor,
        bytes32 validationID,
        uint256 amount
    ) external;

    function native_decreaseStake(
        address endorsor,
        bytes32 validationID,
        uint256 amount
    ) external;

    function native_withdraw(
        address endorsor,
        bytes32 validationID
    ) external returns (uint256);

    function native_updateAutoRenew(
        address endorsor,
        bytes32 validationID,
        bool autoRenew
    ) external;

    function native_addDelegation(
        bytes32 validationID,
        address delegator,
        uint256 stake,
        bool autoRenew,
        uint8 multiplier
    ) external;

    function native_withdrawDelegation(
        bytes32 validationID,
        address delegator
    ) external returns (uint256);

    function native_updateDelegatorAutoRenew(
        bytes32 validationID,
        address delegator,
        bool autoRenew
    ) external;

    // Read methods
    function native_totalStake() external pure returns (uint256);

    function native_queuedStake() external pure returns (uint256);

    function native_getDelegation(
        bytes32 validationID,
        address delegator
    ) external view returns (uint256, uint8, bool);

    function native_get(
        bytes32 validationID
    ) external view returns (address, address, uint256, uint256, uint8, bool);

    function native_getWithdraw(
        bytes32 validationID
    ) external view returns (uint256);

    function native_firstActive() external view returns (bytes32);

    function native_firstQueued() external view returns (bytes32);

    function native_next(bytes32 prev) external view returns (bytes32);

    function native_getDelegatorContract() external view returns (address);
}
