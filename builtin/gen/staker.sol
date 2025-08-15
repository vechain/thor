//SPDX-License-Identifier: MIT
pragma solidity ^0.8.20;

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
    event BeneficiarySet(
        address indexed validator,
        address indexed endorser,
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
        (uint256 stake, uint256 weight, string memory error) = StakerNative(
            address(this)
        ).native_totalStake();
        require(bytes(error).length == 0, error);
        return (stake, weight);
    }

    /**
     * @dev queuedStake returns all stakes and weight by queued validators.
     */
    function queuedStake() public view returns (uint256, uint256) {
        (uint256 stake, uint256 weight, string memory error) = StakerNative(
            address(this)
        ).native_queuedStake();
        require(bytes(error).length == 0, error);
        return (stake, weight);
    }

    /**
     * @dev addValidation creates a validation to the queue.
     */
    function addValidation(
        address validator,
        uint32 period
    ) public payable checkStake(msg.value) {
        string memory error = StakerNative(address(this)).native_addValidation(
            validator,
            msg.sender,
            period,
            msg.value
        );
        require(bytes(error).length == 0, error);
        emit ValidationQueued(validator, msg.sender, period, msg.value);
    }

    /**
     * @dev increaseStake adds VET to the current stake of the queued/active validator.
     */
    function increaseStake(
        address validator
    ) public payable checkStake(msg.value) {
        string memory error = StakerNative(address(this)).native_increaseStake(
            validator,
            msg.sender,
            msg.value
        );
        require(bytes(error).length == 0, error);
        emit StakeIncreased(validator, msg.value);
    }

    /**
     * @dev setBeneficiary sets the beneficiary address for a validator.
     */
    function setBeneficiary(address validator, address beneficiary) public {
        string memory error = StakerNative(address(this)).native_setBeneficiary(
            validator,
            msg.sender,
            beneficiary
        );
        require(bytes(error).length == 0, error);
        emit BeneficiarySet(validator, msg.sender, beneficiary);
    }

    /**
     * @dev decreaseStake removes VET from the current stake of an active validator
     */
    function decreaseStake(
        address validator,
        uint256 amount
    ) public checkStake(amount) {
        string memory error = StakerNative(address(this)).native_decreaseStake(
            validator,
            msg.sender,
            amount
        );
        require(bytes(error).length == 0, error);
        emit StakeDecreased(validator, amount);
    }

    /**
     * @dev allows the caller to withdraw a stake when their status is set to exited
     */
    function withdrawStake(address validator) public {
        (uint256 stake, string memory error) = StakerNative(address(this))
            .native_withdrawStake(validator, msg.sender);
        require(bytes(error).length == 0, error);

        (bool success, ) = msg.sender.call{value: stake}("");
        require(success, "Transfer failed");
        emit ValidationWithdrawn(validator, stake);
    }

    /**
     * @dev signalExit signals the intent to exit a validator position at the end of the staking period.
     */
    function signalExit(address validator) public {
        string memory error = StakerNative(address(this)).native_signalExit(
            validator,
            msg.sender
        );
        require(bytes(error).length == 0, error);
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
        (uint256 delegationID, string memory error) = StakerNative(
            address(this)
        ).native_addDelegation(validator, msg.value, multiplier);
        require(bytes(error).length == 0, error);
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
        string memory error = StakerNative(address(this))
            .native_signalDelegationExit(delegationID);
        require(bytes(error).length == 0, error);
        emit DelegationSignaledExit(delegationID);
    }

    /**
     * @dev withdrawDelegation withdraws the delegation position funds.
     */
    function withdrawDelegation(
        uint256 delegationID
    ) public onlyDelegatorContract {
        (uint256 stake, string memory error) = StakerNative(address(this))
            .native_withdrawDelegation(delegationID);
        require(bytes(error).length == 0, error);
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
        (
            address validator,
            uint256 stake,
            uint8 multiplier,
            string memory error
        ) = StakerNative(address(this)).native_getDelegationStake(delegationID);
        require(bytes(error).length == 0, error);
        return (validator, stake, multiplier);
    }

    /**
     * @dev getDelegationPeriodDetails returns the start, end period and isLocked status of a delegation.
     * @return (startPeriod, endPeriod, isLocked)
     */
    function getDelegationPeriodDetails(
        uint256 delegationID
    ) public view returns (uint32, uint32, bool) {
        (
            uint32 startPeriod,
            uint32 endPeriod,
            bool isLocked,
            string memory error
        ) = StakerNative(address(this)).native_getDelegationPeriodDetails(
                delegationID
            );
        require(bytes(error).length == 0, error);
        return (startPeriod, endPeriod, isLocked);
    }

    /**
     * @dev get returns the validator stake. endorser, stake, weight of a validator.
     * @return (endorser, stake, weight)
     */
    function getValidatorStake(
        address validator
    ) public view returns (address, uint256, uint256, uint256) {
        (
            address endorser,
            uint256 stake,
            uint256 weight,
            uint256 queuedStakeAmount,
            string memory error
        ) = StakerNative(address(this)).native_getValidatorStake(validator);
        require(bytes(error).length == 0, error);
        return (endorser, stake, weight, queuedStakeAmount);
    }

    /**
     * @dev get returns the validator status. status and offline / online for a validator.
     * @return (status, online status)
     */
    function getValidatorStatus(
        address validator
    ) public view returns (uint8, bool) {
        (uint8 status, bool online, string memory error) = StakerNative(
            address(this)
        ).native_getValidatorStatus(validator);
        require(bytes(error).length == 0, error);
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
            uint32 completedPeriods,
            string memory error
        ) = StakerNative(address(this)).native_getValidatorPeriodDetails(
                validator
            );
        require(bytes(error).length == 0, error);
        return (period, startBlock, exitBlock, completedPeriods);
    }

    /**
     * @dev getWithdrawable returns the amount of a validator's withdrawable VET.
     */
    function getWithdrawable(address id) public view returns (uint256) {
        (uint256 withdrawal, string memory error) = StakerNative(address(this))
            .native_getWithdrawable(id);
        require(bytes(error).length == 0, error);
        return withdrawal;
    }

    /**
     * @dev firstActive returns the head validatorId of the active validators.
     */
    function firstActive() public view returns (address) {
        (address id, string memory error) = StakerNative(address(this))
            .native_firstActive();
        require(bytes(error).length == 0, error);
        return id;
    }

    /**
     * @dev firstQueued returns the head validatorId of the queued validators.
     */
    function firstQueued() public view returns (address) {
        (address id, string memory error) = StakerNative(address(this))
            .native_firstQueued();
        require(bytes(error).length == 0, error);
        return id;
    }

    /**
     * @dev next returns the validator in a linked list
     */
    function next(address prev) public view returns (address) {
        (address id, string memory error) = StakerNative(address(this))
            .native_next(prev);
        require(bytes(error).length == 0, error);
        return id;
    }

    /**
     * @dev getDelegatorsRewards returns the delegators rewards for a given validator address and staking period.
     */
    function getDelegatorsRewards(
        address validator,
        uint32 stakingPeriod
    ) public view returns (uint256) {
        (uint256 reward, string memory error) = StakerNative(address(this))
            .native_getDelegatorsRewards(validator, stakingPeriod);
        require(bytes(error).length == 0, error);
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
            uint256 exitingWeight,
            string memory error
        ) = StakerNative(address(this)).native_getValidationTotals(validator);
        require(bytes(error).length == 0, error);
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
        (
            uint256 leaderGroupSize,
            uint256 queuedValidators,
            string memory error
        ) = StakerNative(address(this)).native_getValidatorsNum();
        require(bytes(error).length == 0, error);
        return (leaderGroupSize, queuedValidators);
    }

    /**
     * @dev issuance returns the total amount of VTHO generated
     */
    function issuance() public view returns (uint256) {
        (uint256 issuanceAmount, string memory error) = StakerNative(
            address(this)
        ).native_issuance();
        require(bytes(error).length == 0, error);
        return issuanceAmount;
    }

    modifier onlyDelegatorContract() {
        (address expected, string memory error) = StakerNative(address(this))
            .native_getDelegatorContract();
        require(bytes(error).length == 0, error);
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
        address endorser,
        uint32 period,
        uint256 stake
    ) external returns (string calldata);

    function native_increaseStake(
        address validator,
        address endorser,
        uint256 amount
    ) external returns (string calldata);

    function native_setBeneficiary(
        address validator,
        address endorser,
        address beneficiary
    ) external returns (string calldata);

    function native_decreaseStake(
        address validator,
        address endorser,
        uint256 amount
    ) external returns (string calldata);

    function native_withdrawStake(
        address validator,
        address endorser
    ) external returns (uint256, string calldata);

    function native_signalExit(
        address validator,
        address endorser
    ) external returns (string calldata);

    function native_addDelegation(
        address validator,
        uint256 stake,
        uint8 multiplier
    ) external returns (uint256, string calldata);

    function native_withdrawDelegation(
        uint256 delegationID
    ) external returns (uint256, string calldata);

    function native_signalDelegationExit(
        uint256 delegationID
    ) external returns (string calldata);

    // Read methods
    function native_totalStake()
        external
        pure
        returns (uint256, uint256, string calldata);

    function native_queuedStake()
        external
        pure
        returns (uint256, uint256, string calldata);

    function native_getDelegationStake(
        uint256 delegationID
    ) external view returns (address, uint256, uint8, string calldata);

    function native_getDelegationPeriodDetails(
        uint256 delegationID
    ) external view returns (uint32, uint32, bool, string calldata);

    function native_getValidatorStake(
        address validator
    )
        external
        view
        returns (address, uint256, uint256, uint256, string calldata);

    function native_getValidatorStatus(
        address validator
    ) external view returns (uint8, bool, string calldata);

    function native_getValidatorPeriodDetails(
        address validator
    ) external view returns (uint32, uint32, uint32, uint32, string calldata);

    function native_getWithdrawable(
        address validator
    ) external view returns (uint256, string calldata);

    function native_firstActive()
        external
        view
        returns (address, string calldata);

    function native_firstQueued()
        external
        view
        returns (address, string calldata);

    function native_next(
        address prev
    ) external view returns (address, string calldata);

    function native_getDelegatorContract()
        external
        view
        returns (address, string calldata);

    function native_getDelegatorsRewards(
        address validator,
        uint32 stakingPeriod
    ) external view returns (uint256, string calldata);

    function native_getValidationTotals(
        address validator
    )
        external
        view
        returns (
            uint256,
            uint256,
            uint256,
            uint256,
            uint256,
            uint256,
            string calldata
        );

    function native_getValidatorsNum()
        external
        view
        returns (uint256, uint256, string calldata);

    function native_issuance() external view returns (uint256, string calldata);
}
