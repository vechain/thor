//SPDX-License-Identifier: MIT
pragma solidity ^0.8.20;

contract Staker {
    event ValidationQueued(
        address indexed validator,
        address indexed endorsor,
        uint32 period,
        uint256 stake
    );
    event ValidationWithdrawn(address indexed validator, uint256 stake);
    event ValidationSignaledExit(address indexed validator);
    event StakeIncreased(address indexed validator, uint256 added);
    event StakeDecreased(address indexed validator, uint256 removed);
    event BeneficiarySet(
        address indexed validator,
        address indexed endorsor,
        address beneficiary
    );

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
    function totalStake() public view returns (uint256, uint256) {
        (uint256 stake, uint256 weight) = StakerNative(address(this))
            .native_totalStake();

        return (stake, weight);
    }

    /**
     * @dev queuedStake returns all stakes and weight by queued validators.
     */
    function queuedStake() public view returns (uint256, uint256) {
        (uint256 stake, uint256 weight) = StakerNative(address(this))
            .native_queuedStake();

        return (stake, weight);
    }

    /**
     * @dev addValidation creates a validation to the queue.
     */
    function addValidation(
        address validator,
        uint32 period
    ) public payable checkStake(msg.value) {
        StakerNative(address(this)).native_addValidation(
            validator,
            msg.sender,
            period,
            msg.value
        );

        emit ValidationQueued(validator, msg.sender, period, msg.value);
    }

    /**
     * @dev increaseStake adds VET to the current stake of the queued/active validator.
     */
    function increaseStake(
        address validator
    ) public payable checkStake(msg.value) {
        StakerNative(address(this)).native_increaseStake(
            validator,
            msg.sender,
            msg.value
        );

        emit StakeIncreased(validator, msg.value);
    }

    /**
     * @dev setBeneficiary sets the beneficiary address for a validator.
     */
    function setBeneficiary(address validator, address beneficiary) public {
        StakerNative(address(this)).native_setBeneficiary(
            validator,
            msg.sender,
            beneficiary
        );

        emit BeneficiarySet(validator, msg.sender, beneficiary);
    }

    /**
     * @dev decreaseStake removes VET from the current stake of an active validator
     */
    function decreaseStake(
        address validator,
        uint256 amount
    ) public checkStake(amount) {
        StakerNative(address(this)).native_decreaseStake(
            validator,
            msg.sender,
            amount
        );

        emit StakeDecreased(validator, amount);
    }

    /**
     * @dev allows the caller to withdraw a stake when their status is set to exited
     */
    function withdrawStake(address validator) public {
        (uint256 stake) = StakerNative(address(this)).native_withdrawStake(
            validator,
            msg.sender
        );

        (bool success, ) = msg.sender.call{value: stake}("");
        require(success, "Transfer failed");
        emit ValidationWithdrawn(validator, stake);
    }

    /**
     * @dev signalExit signals the intent to exit a validator position at the end of the staking period.
     */
    function signalExit(address validator) public {
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
        returns (uint256)
    {
        (uint256 delegationID) = StakerNative(address(this))
            .native_addDelegation(validator, msg.value, multiplier);

        emit DelegationAdded(validator, delegationID, msg.value, multiplier);
        return delegationID;
    }

    /**
     * @dev exitDelegation signals the intent to exit a delegation position at the end of the staking period.
     * Funds are available once the current staking period ends.
     */
    function signalDelegationExit(
        uint256 delegationID
    ) public onlyDelegatorContract {
        StakerNative(address(this)).native_signalDelegationExit(delegationID);

        emit DelegationSignaledExit(delegationID);
    }

    /**
     * @dev withdrawDelegation withdraws the delegation position funds.
     */
    function withdrawDelegation(
        uint256 delegationID
    ) public onlyDelegatorContract {
        (uint256 stake) = StakerNative(address(this)).native_withdrawDelegation(
            delegationID
        );

        emit DelegationWithdrawn(delegationID, stake);
        (bool success, ) = msg.sender.call{value: stake}("");
        require(success, "Transfer failed");
    }

    /**
     * @dev getDelegationStake returns the validator, stake, and multiplier of a delegation.
     * @return (validator, stake, multiplier)
     */
    function getDelegationStake(
        uint256 delegationID
    ) public view returns (address, uint256, uint8) {
        (address validator, uint256 stake, uint8 multiplier) = StakerNative(
            address(this)
        ).native_getDelegationStake(delegationID);

        return (validator, stake, multiplier);
    }

    /**
     * @dev getDelegationPeriodDetails returns the start, end period and isLocked status of a delegation.
     * @return (startPeriod, endPeriod, isLocked)
     */
    function getDelegationPeriodDetails(
        uint256 delegationID
    ) public view returns (uint32, uint32, bool) {
        (uint32 startPeriod, uint32 endPeriod, bool isLocked) = StakerNative(
            address(this)
        ).native_getDelegationPeriodDetails(delegationID);

        return (startPeriod, endPeriod, isLocked);
    }

    /**
     * @dev get returns the validator stake. endorsor, stake, weight of a validator.
     * @return (endorsor, stake, weight)
     */
    function getValidatorStake(
        address validator
    ) public view returns (address, uint256, uint256, uint256) {
        (
            address endorsor,
            uint256 stake,
            uint256 weight,
            uint256 queuedStakeAmount
        ) = StakerNative(address(this)).native_getValidatorStake(validator);

        return (endorsor, stake, weight, queuedStakeAmount);
    }

    /**
     * @dev get returns the validator status. status and offline / online for a validator.
     * @return (status, online status)
     */
    function getValidatorStatus(
        address validator
    ) public view returns (uint8, bool) {
        (uint8 status, bool online) = StakerNative(address(this))
            .native_getValidatorStatus(validator);

        return (status, online);
    }

    /**
     * @dev get returns the validator period details. period, startBlock, exitBlock and completed periods for a validator.
     * @return (period, startBlock, exitBlock)
     */
    function getValidatorPeriodDetails(
        address validator
    ) public view returns (uint32, uint32, uint32, uint32) {
        (
            uint32 period,
            uint32 startBlock,
            uint32 exitBlock,
            uint32 completedPeriods
        ) = StakerNative(address(this)).native_getValidatorPeriodDetails(
                validator
            );

        return (period, startBlock, exitBlock, completedPeriods);
    }

    /**
     * @dev getWithdrawable returns the amount of a validator's withdrawable VET.
     */
    function getWithdrawable(address id) public view returns (uint256) {
        (uint256 withdrawal) = StakerNative(address(this))
            .native_getWithdrawable(id);

        return withdrawal;
    }

    /**
     * @dev firstActive returns the head validatorId of the active validators.
     */
    function firstActive() public view returns (address) {
        (address id) = StakerNative(address(this)).native_firstActive();

        return id;
    }

    /**
     * @dev firstQueued returns the head validatorId of the queued validators.
     */
    function firstQueued() public view returns (address) {
        (address id) = StakerNative(address(this)).native_firstQueued();

        return id;
    }

    /**
     * @dev next returns the validator in a linked list
     */
    function next(address prev) public view returns (address) {
        (address id) = StakerNative(address(this)).native_next(prev);

        return id;
    }

    /**
     * @dev getDelegatorsRewards returns the delegators rewards for a given validator address and staking period.
     */
    function getDelegatorsRewards(
        address validator,
        uint32 stakingPeriod
    ) public view returns (uint256) {
        (uint256 reward) = StakerNative(address(this))
            .native_getDelegatorsRewards(validator, stakingPeriod);

        return reward;
    }

    function getValidationTotals(
        address validator
    )
        public
        view
        returns (uint256, uint256, uint256, uint256, uint256, uint256)
    {
        (
            uint256 lockedStake,
            uint256 lockedWeight,
            uint256 queuedStake,
            uint256 queuedWeight,
            uint256 exitingStake,
            uint256 exitingWeight
        ) = StakerNative(address(this)).native_getValidationTotals(validator);

        return (
            lockedStake,
            lockedWeight,
            queuedStake,
            queuedWeight,
            exitingStake,
            exitingWeight
        );
    }

    function getValidatorsNum() public view returns (uint256, uint256) {
        (uint256 leaderGroupSize, uint256 queuedValidators) = StakerNative(
            address(this)
        ).native_getValidatorsNum();

        return (leaderGroupSize, queuedValidators);
    }

    /**
     * @dev issuance returns the total amount of VTHO generated
     */
    function issuance() public view returns (uint256) {
        (uint256 issuanceAmount) = StakerNative(address(this))
            .native_issuance();

        return issuanceAmount;
    }

    modifier onlyDelegatorContract() {
        (address expected) = StakerNative(address(this))
            .native_getDelegatorContract();

        require(msg.sender == expected, "builtin: only delegator");
        _;
    }

    modifier checkStake(uint256 amount) {
        require(amount > 0, "stake is empty");
        require(amount % 1e18 == 0, "stake is not multiple of 1VET");
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
    function native_addValidation(
        address validator,
        address endorsor,
        uint32 period,
        uint256 stake
    ) external;

    function native_increaseStake(
        address validator,
        address endorsor,
        uint256 amount
    ) external;

    function native_setBeneficiary(
        address validator,
        address endorsor,
        address beneficiary
    ) external;

    function native_decreaseStake(
        address validator,
        address endorsor,
        uint256 amount
    ) external;

    function native_withdrawStake(
        address validator,
        address endorsor
    ) external returns (uint256);

    function native_signalExit(address validator, address endorsor) external;

    function native_addDelegation(
        address validator,
        uint256 stake,
        uint8 multiplier
    ) external returns (uint256);

    function native_withdrawDelegation(
        uint256 delegationID
    ) external returns (uint256);

    function native_signalDelegationExit(uint256 delegationID) external;

    // Read methods
    function native_totalStake() external pure returns (uint256, uint256);

    function native_queuedStake() external pure returns (uint256, uint256);

    function native_getDelegationStake(
        uint256 delegationID
    ) external view returns (address, uint256, uint8);

    function native_getDelegationPeriodDetails(
        uint256 delegationID
    ) external view returns (uint32, uint32, bool);

    function native_getValidatorStake(
        address validator
    ) external view returns (address, uint256, uint256, uint256);

    function native_getValidatorStatus(
        address validator
    ) external view returns (uint8, bool);

    function native_getValidatorPeriodDetails(
        address validator
    ) external view returns (uint32, uint32, uint32, uint32);

    function native_getWithdrawable(
        address validator
    ) external view returns (uint256);

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
    )
        external
        view
        returns (uint256, uint256, uint256, uint256, uint256, uint256);

    function native_getValidatorsNum() external view returns (uint256, uint256);

    function native_issuance() external view returns (uint256);
}
