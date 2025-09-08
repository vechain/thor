//SPDX-License-Identifier: MIT
pragma solidity ^0.8.20;

uint256 constant DELEGATOR_PAUSED_BIT = 1 << 0;
uint256 constant STAKER_PAUSED_BIT = 1 << 1;

contract Staker {
    event ValidationQueued(
        address indexed validator,
        address indexed endorser,
        uint32 period,
        uint256 stake
    );
    event ValidationWithdrawn(address indexed validator, uint256 stake);
    event ValidationSignaledExit(address indexed validator);
    event StakeIncreased(address indexed validator, uint256 added);
    event StakeDecreased(address indexed validator, uint256 removed);
    event BeneficiarySet(address indexed validator, address beneficiary);

    event DelegationAdded(
        address indexed validator,
        uint256 indexed delegationID,
        uint256 stake,
        uint8 multiplier
    );
    event DelegationWithdrawn(uint256 indexed delegationID, uint256 stake);
    event DelegationSignaledExit(uint256 indexed delegationID);

    /**
     * @dev totalStake returns all stakes and weight by active validators.
     */
    function totalStake() public view returns (uint256 totalVET, uint256 totalWeight) {
        return StakerNative(address(this)).native_totalStake();
    }

    /**
     * @dev queuedStake returns all stakes by queued validators.
     */
    function queuedStake() public view returns (uint256 queuedVET) {
        return StakerNative(address(this)).native_queuedStake();
    }

    /**
     * @dev addValidation creates a validation to the queue.
     */
    function addValidation(
        address validator,
        uint32 period
    ) public payable checkStake(msg.value) stakerNotPaused {
        StakerNative(address(this)).native_addValidation(validator, msg.sender, period, msg.value);
        emit ValidationQueued(validator, msg.sender, period, msg.value);
    }

    /**
     * @dev increaseStake adds VET to the current stake of the queued/active validator.
     */
    function increaseStake(address validator) public payable checkStake(msg.value) stakerNotPaused {
        StakerNative(address(this)).native_increaseStake(validator, msg.sender, msg.value);
        emit StakeIncreased(validator, msg.value);
    }

    /**
     * @dev setBeneficiary sets the beneficiary address for a validator.
     */
    function setBeneficiary(address validator, address beneficiary) public stakerNotPaused {
        StakerNative(address(this)).native_setBeneficiary(validator, msg.sender, beneficiary);

        emit BeneficiarySet(validator, beneficiary);
    }

    /**
     * @dev decreaseStake removes VET from the current stake of an active validator
     */
    function decreaseStake(
        address validator,
        uint256 amount
    ) public checkStake(amount) stakerNotPaused {
        StakerNative(address(this)).native_decreaseStake(validator, msg.sender, amount);
        emit StakeDecreased(validator, amount);
    }

    /**
     * @dev allows the caller to withdraw a stake when their status is set to exited
     */
    function withdrawStake(address validator) public stakerNotPaused {
        uint256 stake = StakerNative(address(this)).native_withdrawStake(validator, msg.sender);

        (bool success, ) = msg.sender.call{value: stake}("");
        require(success, "Transfer failed");
        emit ValidationWithdrawn(validator, stake);
    }

    /**
     * @dev signalExit signals the intent to exit a validator position at the end of the staking period.
     */
    function signalExit(address validator) public stakerNotPaused {
        StakerNative(address(this)).native_signalExit(validator, msg.sender);
        emit ValidationSignaledExit(validator);
    }

    /**
     * @dev addDelegation creates a delegation position on a validator.
     */
    function addDelegation(
        address validator,
        uint8 multiplier // (% of msg.value) 100 for x1, 200 for x2, etc. This enforces a maximum of 2.56x multiplier
    )
        public
        payable
        onlyDelegatorContract
        checkStake(msg.value)
        delegatorNotPaused
        returns (uint256 delegationID)
    {
        delegationID = StakerNative(address(this)).native_addDelegation(
            validator,
            msg.value,
            multiplier
        );
        emit DelegationAdded(validator, delegationID, msg.value, multiplier);
        return delegationID;
    }

    /**
     * @dev exitDelegation signals the intent to exit a delegation position at the end of the staking period.
     * Funds are available once the current staking period ends.
     */
    function signalDelegationExit(
        uint256 delegationID
    ) public onlyDelegatorContract delegatorNotPaused {
        StakerNative(address(this)).native_signalDelegationExit(delegationID);
        emit DelegationSignaledExit(delegationID);
    }

    /**
     * @dev withdrawDelegation withdraws the delegation position funds.
     */
    function withdrawDelegation(
        uint256 delegationID
    ) public onlyDelegatorContract delegatorNotPaused {
        uint256 stake = StakerNative(address(this)).native_withdrawDelegation(delegationID);

        emit DelegationWithdrawn(delegationID, stake);
        (bool success, ) = msg.sender.call{value: stake}("");
        require(success, "Transfer failed");
    }

    /**
     * @dev getDelegation returns the validator, stake, and multiplier of a delegation.
     */
    function getDelegation(
        uint256 delegationID
    ) public view returns (address validator, uint256 stake, uint8 multiplier, bool isLocked) {
        (validator, stake, multiplier, isLocked, , ) = StakerNative(address(this))
            .native_getDelegation(delegationID);
        return (validator, stake, multiplier, isLocked);
    }

    /**
     * @dev getDelegationPeriodDetails returns the start, end period and isLocked status of a delegation.
     */
    function getDelegationPeriodDetails(
        uint256 delegationID
    ) public view returns (uint32 startPeriod, uint32 endPeriod) {
        (, , , , startPeriod, endPeriod) = StakerNative(address(this)).native_getDelegation(
            delegationID
        );
        return (startPeriod, endPeriod);
    }

    /**
     * @dev getValidation returns the validator stake. endorser, stake, weight of a validator.
     */
    function getValidation(
        address validator
    )
        public
        view
        returns (
            address endorser,
            uint256 stake,
            uint256 weight,
            uint256 queuedVET,
            uint8 status,
            uint32 offlineBlock
        )
    {
        (endorser, stake, weight, queuedVET, status, offlineBlock, , , , ) = StakerNative(
            address(this)
        ).native_getValidation(validator);
        return (endorser, stake, weight, queuedVET, status, offlineBlock);
    }

    /**
     * @dev getValidationPeriodDetails returns the validator period details. period, startBlock, exitBlock and completed periods for a validator.
     */
    function getValidationPeriodDetails(
        address validator
    )
        public
        view
        returns (uint32 period, uint32 startBlock, uint32 exitBlock, uint32 completedPeriods)
    {
        (, , , , , , period, startBlock, exitBlock, completedPeriods) = StakerNative(address(this))
            .native_getValidation(validator);
        return (period, startBlock, exitBlock, completedPeriods);
    }

    /**
     * @dev getWithdrawable returns the amount of a validator's withdrawable VET.
     */
    function getWithdrawable(address id) public view returns (uint256 withdrawableVET) {
        return StakerNative(address(this)).native_getWithdrawable(id);
    }

    /**
     * @dev firstActive returns the head validatorId of the active validators.
     */
    function firstActive() public view returns (address first) {
        return StakerNative(address(this)).native_firstActive();
    }

    /**
     * @dev firstQueued returns the head validatorId of the queued validators.
     */
    function firstQueued() public view returns (address first) {
        return StakerNative(address(this)).native_firstQueued();
    }

    /**
     * @dev next returns the validator in a linked list
     */
    function next(address prev) public view returns (address nextValidation) {
        return StakerNative(address(this)).native_next(prev);
    }

    /**
     * @dev getDelegatorsRewards returns all delegators rewards for the given validator address and staking period.
     */
    function getDelegatorsRewards(
        address validator,
        uint32 stakingPeriod
    ) public view returns (uint256 rewards) {
        return StakerNative(address(this)).native_getDelegatorsRewards(validator, stakingPeriod);
    }

    /**
     * @dev getValidationTotals returns the total locked, total locked weight,
     * total queued, total queued weight, total exiting and total exiting weight for a validator.
     */
    function getValidationTotals(
        address validator
    )
        public
        view
        returns (
            uint256 lockedVET,
            uint256 lockedWeight,
            uint256 queuedVET,
            uint256 exitingVET,
            uint256 nextPeriodWeight
        )
    {
        return StakerNative(address(this)).native_getValidationTotals(validator);
    }

    /**
     * @dev getValidationsNum returns the number of active and queued validators.
     */
    function getValidationsNum() public view returns (uint64 activeCount, uint64 queuedCount) {
        return StakerNative(address(this)).native_getValidationsNum();
    }

    /**
     * @dev issuance returns the total amount of VTHO generated for the context of current block.
     */
    function issuance() public view returns (uint256 issued) {
        return StakerNative(address(this)).native_issuance();
    }

    modifier onlyDelegatorContract() {
        address expected = StakerNative(address(this)).native_getDelegatorContract();

        require(msg.sender == expected, "staker:only delegator");
        _;
    }

    modifier checkStake(uint256 amount) {
        require(amount > 0, "staker: stake is empty");
        require(amount % 1e18 == 0, "staker: stake is not multiple of 1VET");
        _;
    }

    modifier stakerNotPaused() {
        uint256 switches = StakerNative(address(this)).native_getControlSwitches();
        require((switches & STAKER_PAUSED_BIT) == 0, "staker: staker is paused");
        _;
    }

    modifier delegatorNotPaused() {
        uint256 switches = StakerNative(address(this)).native_getControlSwitches();
        require((switches & STAKER_PAUSED_BIT) == 0, "staker: staker is paused");
        require((switches & DELEGATOR_PAUSED_BIT) == 0, "staker: delegator is paused");
        _;
    }

    receive() external payable {
        revert("staker: receive function not allowed");
    }

    fallback() external {
        revert("staker: fallback function not allowed");
    }
}

interface StakerNative {
    // Write methods
    function native_addValidation(
        address validator,
        address endorser,
        uint32 period,
        uint256 stake
    ) external;

    function native_increaseStake(address validator, address endorser, uint256 amount) external;

    function native_setBeneficiary(
        address validator,
        address endorser,
        address beneficiary
    ) external;

    function native_decreaseStake(address validator, address endorser, uint256 amount) external;

    function native_withdrawStake(address validator, address endorser) external returns (uint256);

    function native_signalExit(address validator, address endorser) external;

    function native_addDelegation(
        address validator,
        uint256 stake,
        uint8 multiplier
    ) external returns (uint256);

    function native_withdrawDelegation(uint256 delegationID) external returns (uint256);

    function native_signalDelegationExit(uint256 delegationID) external;

    // Read methods
    function native_totalStake() external pure returns (uint256, uint256);

    function native_queuedStake() external pure returns (uint256);

    function native_getDelegation(
        uint256 delegationID
    ) external view returns (address, uint256, uint8, bool, uint32, uint32);

    function native_getValidation(
        address validator
    )
        external
        view
        returns (address, uint256, uint256, uint256, uint8, uint32, uint32, uint32, uint32, uint32);

    function native_getWithdrawable(address validator) external view returns (uint256);

    function native_firstActive() external view returns (address);

    function native_firstQueued() external view returns (address);

    function native_next(address prev) external view returns (address);

    function native_getDelegatorContract() external view returns (address);

    function native_getDelegatorsRewards(
        address validator,
        uint32 stakingPeriod
    ) external view returns (uint256);

    function native_getValidationTotals(
        address validator
    ) external view returns (uint256, uint256, uint256, uint256, uint256);

    function native_getValidationsNum() external view returns (uint64, uint64);

    function native_issuance() external view returns (uint256);

    function native_getControlSwitches() external view returns (uint256);
}
