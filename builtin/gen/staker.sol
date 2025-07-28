//SPDX-License-Identifier: MIT
pragma solidity ^0.8.20;

contract Staker {
    event ValidatorQueued(
        address indexed endorsor,
        address indexed validationID,
        uint32 period,
        uint256 stake
    );
    event ValidatorWithdrawn(
        address indexed endorsor,
        address indexed validationID,
        uint256 stake
    );
    event ValidatorSignaledExit(
        address indexed endorsor,
        address indexed validationID
    );

    event StakeIncreased(
        address indexed endorsor,
        address indexed validationID,
        uint256 added
    );
    event StakeDecreased(
        address indexed endorsor,
        address indexed validationID,
        uint256 removed
    );

    event DelegationAdded(
        address indexed validationID,
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
     * @dev addValidator adds a validator to the queue.
     */
    function addValidator(address node, uint32 period) public payable checkStake(msg.value) {
        string memory error = StakerNative(address(this))
            .native_addValidator(msg.sender, node, period, msg.value);
        require(bytes(error).length == 0, error);
        emit ValidatorQueued(msg.sender, node,  period, msg.value);
    }

    /**
     * @dev increaseStake adds VET to the current stake of the queued/active validator.
     */
    function increaseStake(address validationID) public payable checkStake(msg.value) {
        string memory error = StakerNative(address(this)).native_increaseStake(
            msg.sender,
            validationID,
            msg.value
        );
        require(bytes(error).length == 0, error);
        emit StakeIncreased(msg.sender, validationID, msg.value);
    }

    /**
     * @dev decreaseStake removes VET from the current stake of an active validator
     */
    function decreaseStake(address validationID, uint256 amount) public checkStake(amount) {
        string memory error = StakerNative(address(this)).native_decreaseStake(
            msg.sender,
            validationID,
            amount
        );
        require(bytes(error).length == 0, error);
        emit StakeDecreased(msg.sender, validationID, amount);
    }

    /**
     * @dev allows the caller to withdraw a stake when their status is set to exited
     */
    function withdrawStake(address id) public {
        (uint256 stake, string memory error) = StakerNative(address(this))
            .native_withdrawStake(msg.sender, id);
        require(bytes(error).length == 0, error);

        (bool success, ) = msg.sender.call{value: stake}("");
        require(success, "Transfer failed");
        emit ValidatorWithdrawn(msg.sender, id, stake);
    }

    /**
     * @dev signalExit signals the intent to exit a validator position at the end of the staking period.
     */
    function signalExit(address id) public {
        string memory error = StakerNative(address(this)).native_signalExit(
            msg.sender,
            id
        );
        require(bytes(error).length == 0, error);
        emit ValidatorSignaledExit(msg.sender, id);
    }

    /**
     * @dev addDelegation creates a delegation position on a validator.
     */
    function addDelegation(
        address validationID,
        uint8 multiplier // (% of msg.value) 100 for x1, 200 for x2, etc. This enforces a maximum of 2.56x multiplier
    ) public payable onlyDelegatorContract checkStake(msg.value) returns (bytes32) {
        (bytes32 delegationID, string memory error) = StakerNative(
            address(this)
        ).native_addDelegation(validationID, msg.value, multiplier);
        require(bytes(error).length == 0, error);
        emit DelegationAdded(validationID, delegationID, msg.value, multiplier);
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
     * @dev getDelegation returns the validation ID, stake, start and end period, multiplier and isLocked status of a delegation.
     * @return (validationID, stake, startPeriod, endPeriod, multiplier, isLocked)
     */
    function getDelegation(
        bytes32 delegationID
    ) public view returns (address, uint256, uint32, uint32, uint8, bool) {
        (
            address validationID,
            uint256 stake,
            uint32 startPeriod,
            uint32 endPeriod,
            uint8 multiplier,
            bool isLocked,
            string memory error
        ) = StakerNative(address(this)).native_getDelegation(delegationID);
        require(bytes(error).length == 0, error);
        return (
            validationID,
            stake,
            startPeriod,
            endPeriod,
            multiplier,
            isLocked
        );
    }

    /**
     * @dev get returns the node. endorser, stake, weight, status, auto renew, online and staking period of a validator.
     * @return (node, endorser, stake, weight, status, online, stakingPeriod, startBlock, exitBlock)
     * - status (0: unknown, 1: queued, 2: active, 3: cooldown, 4: exited)
     */
    function get(
        address id
    )
        public
        view
        returns (
            address,
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
            address node,
            address endorser,
            uint256 stake,
            uint256 weight,
            uint8 status,
            bool online,
            uint32 period,
            uint32 startBlock,
            uint32 exitBlock,
            string memory error
        ) = StakerNative(address(this)).native_get(id);
        require(bytes(error).length == 0, error);
        return (
            node,
            endorser,
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
     * @dev lookupNode returns a validation ID if the node address exists in a queued or active validation.
     */
    function lookupNode(address node) public view returns (bytes32) {
        return StakerNative(address(this)).native_lookupNode(node);
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
     * @dev getDelegatorsRewards returns the delegators rewards for a given validation ID and staking period.
     */
    function getDelegatorsRewards(
        address validationID,
        uint32 stakingPeriod
    ) public view returns (uint256) {
        (uint256 reward, string memory error) = StakerNative(address(this))
            .native_getDelegatorsRewards(validationID, stakingPeriod);
        require(bytes(error).length == 0, error);
        return reward;
    }

    /**
     * @dev getCompletedPeriods returns the number of completed periods for validation
     */
    function getCompletedPeriods(
        address validationID
    ) public view returns (uint32) {
        (uint32 periods, string memory error) = StakerNative(address(this))
            .native_getCompletedPeriods(validationID);
        require(bytes(error).length == 0, error);
        return periods;
    }

    function getValidatorTotals(
        address validationID
    ) public view returns (uint256, uint256, uint256, uint256) {
        (
            uint256 lockedStake,
            uint256 lockedWeight,
            uint256 delegatorsStake,
            uint256 delegatorsWeight,
            string memory error
        ) = StakerNative(address(this)).native_getValidatorTotals(validationID);
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
    function native_addValidator(
        address endorsor,
        address node,
        uint32 period,
        uint256 stake
    ) external returns (string calldata);

    function native_increaseStake(
        address endorsor,
        address validationID,
        uint256 amount
    ) external returns (string calldata);

    function native_decreaseStake(
        address endorsor,
        address validationID,
        uint256 amount
    ) external returns (string calldata);

    function native_withdrawStake(
        address endorsor,
        address validationID
    ) external returns (uint256, string calldata);

    function native_signalExit(
        address endorsor,
        address validationID
    ) external returns (string calldata);

    function native_addDelegation(
        address validationID,
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
        address validationID
    )
        external
        view
        returns (
            address,
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

    function native_lookupNode(address node) external view returns (bytes32);

    function native_getWithdrawable(
        address validationID
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
        address validationID,
        uint32 stakingPeriod
    ) external view returns (uint256, string calldata);

    function native_getCompletedPeriods(
        address validationID
    ) external view returns (uint32, string calldata);

    function native_getValidatorTotals(
        address validationID
    )
        external
        view
        returns (uint256, uint256, uint256, uint256, string calldata);
}
