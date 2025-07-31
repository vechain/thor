//SPDX-License-Identifier: MIT
pragma solidity ^0.8.20;

contract Staker {
    event ValidationQueued( 
        address indexed validator,
        address indexed endorsor,
        uint32 period, 
        uint256 stake
    );
    event ValidationWithdrawn(address indexed validator,uint256 stake);
    event ValidationSignaledExit(address indexed validator);
    event StakeIncreased(address indexed validator,uint256 added);
    event StakeDecreased(address indexed validator,uint256 removed);

    event DelegationAdded(
        address indexed validator,
        bytes32 indexed delegationID,
        uint256 stake,
        uint8 multiplier
    );
    event DelegationWithdrawn(bytes32 indexed delegationID, uint256 stake);
    event DelegationSignaledExit(bytes32 indexed delegationID);

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
    function addValidation(address validator, uint32 period) public payable checkStake(msg.value) {
        string memory error = StakerNative(address(this))
            .native_addValidation(validator, msg.sender, period, msg.value);
        require(bytes(error).length == 0, error);
        emit ValidationQueued(validator, msg.sender, period, msg.value);
    }

    /**
     * @dev increaseStake adds VET to the current stake of the queued/active validator.
     */
    function increaseStake(address validator) public payable checkStake(msg.value) {
        string memory error = StakerNative(address(this)).native_increaseStake(
            msg.sender,
            validator,
            msg.value
        );
        require(bytes(error).length == 0, error);
        emit StakeIncreased(validator, msg.value);
    }

    /**
     * @dev decreaseStake removes VET from the current stake of an active validator
     */
    function decreaseStake(address validator, uint256 amount) public checkStake(amount) {
        string memory error = StakerNative(address(this)).native_decreaseStake(
            msg.sender,
            validator,
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
            .native_withdrawStake(msg.sender, validator);
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
            msg.sender,
            validator
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
    ) public payable onlyDelegatorContract checkStake(msg.value) returns (bytes32) {
        (bytes32 delegationID, string memory error) = StakerNative(
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
        bytes32 delegationID
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
        bytes32 delegationID
    ) public onlyDelegatorContract {
        (uint256 stake, string memory error) = StakerNative(address(this))
            .native_withdrawDelegation(delegationID);
        require(bytes(error).length == 0, error);
        emit DelegationWithdrawn(delegationID, stake);
        (bool success, ) = msg.sender.call{value: stake}("");
        require(success, "Transfer failed");
    }

    /**
     * @dev getDelegation returns the validator, stake, start and end period, multiplier and isLocked status of a delegation.
     * @return (validator, stake, startPeriod, endPeriod, multiplier, isLocked)
     */
    function getDelegation(
        bytes32 delegationID
    ) public view returns (address, uint256, uint32, uint32, uint8, bool) {
        (
            address validator,
            uint256 stake,
            uint32 startPeriod,
            uint32 endPeriod,
            uint8 multiplier,
            bool isLocked,
            string memory error
        ) = StakerNative(address(this)).native_getDelegation(delegationID);
        require(bytes(error).length == 0, error);
        return (
            validator,
            stake,
            startPeriod,
            endPeriod,
            multiplier,
            isLocked
        );
    }

    /**
     * @dev get returns the validator. endorsor, stake, weight, status, auto renew, online and staking period of a validator.
     * @return (validator, endorsor, stake, weight, status, online, stakingPeriod, startBlock, exitBlock)
     * - status (0: unknown, 1: queued, 2: active, 3: cooldown, 4: exited)
     */
    function get(
        address validator
    )
        public
        view
        returns (
            address,
            uint256,
            uint256,
            uint8,
            bool,
            uint32,
            uint32,
            uint32
        )
    {
        (
            address endorsor,
            uint256 stake,
            uint256 weight,
            uint8 status,
            bool online,
            uint32 period,
            uint32 startBlock,
            uint32 exitBlock,
            string memory error
        ) = StakerNative(address(this)).native_get(validator);
        require(bytes(error).length == 0, error);
        return (
            endorsor,
            stake,
            weight,
            status,
            online,
            period,
            startBlock,
            exitBlock
        );
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

    /**
     * @dev getCompletedPeriods returns the number of completed periods for validation
     */
    function getCompletedPeriods(
        address validator
    ) public view returns (uint32) {
        (uint32 periods, string memory error) = StakerNative(address(this))
            .native_getCompletedPeriods(validator);
        require(bytes(error).length == 0, error);
        return periods;
    }

    function getValidationTotals(
        address validator
    ) public view returns (uint256, uint256, uint256, uint256) {
        (
            uint256 lockedStake,
            uint256 lockedWeight,
            uint256 delegatorsStake,
            uint256 delegatorsWeight,
            string memory error
        ) = StakerNative(address(this)).native_getValidationTotals(validator);
        require(bytes(error).length == 0, error);
        return (lockedStake, lockedWeight, delegatorsStake, delegatorsWeight);
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
        require(amount%1e18 == 0, "stake is not multiple of 1VET");
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
    ) external returns (string calldata);

    function native_increaseStake(
        address validator,
        address endorsor,
        uint256 amount
    ) external returns (string calldata);

    function native_decreaseStake(
        address validator,
        address endorsor,
        uint256 amount
    ) external returns (string calldata);

    function native_withdrawStake(
        address validator,
        address endorsor
    ) external returns (uint256, string calldata);

    function native_signalExit(
        address validator,
        address endorsor
    ) external returns (string calldata);

    function native_addDelegation(
        address validator,
        uint256 stake,
        uint8 multiplier
    ) external returns (bytes32, string calldata);

    function native_withdrawDelegation(
        bytes32 delegationID
    ) external returns (uint256, string calldata);

    function native_signalDelegationExit(
        bytes32 delegationID
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

    function native_getDelegation(
        bytes32 delegationID
    )
        external
        view
        returns (
            address,
            uint256,
            uint32,
            uint32,
            uint8,
            bool,
            string calldata
        );

    function native_get(
        address validator
    )
        external
        view
        returns (
            address,
            uint256,
            uint256,
            uint8,
            bool,
            uint32,
            uint32,
            uint32,
            string calldata
        );


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

    function native_getCompletedPeriods(
        address validator
    ) external view returns (uint32, string calldata);

    function native_getValidationTotals(
        address validator
    )
        external
        view
        returns (uint256, uint256, uint256, uint256, string calldata);
}
