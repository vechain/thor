//SPDX-License-Identifier: MIT
pragma solidity ^0.8.20;

contract Staker {
    event ValidatorQueued(
        address indexed endorsor,
        address indexed node,
        bytes32 indexed validationID,
        uint32 period,
        uint256 stake
    );
    event ValidatorWithdrawn(
        address indexed endorsor,
        bytes32 indexed validationID,
        uint256 stake
    );
    event ValidatorDisabledAutoRenew(
        address indexed endorsor,
        bytes32 indexed validationID
    );

    event StakeIncreased(
        address indexed endorsor,
        bytes32 indexed validationID,
        uint256 added
    );
    event StakeDecreased(
        address indexed endorsor,
        bytes32 indexed validationID,
        uint256 removed
    );

    event DelegationAdded(
        bytes32 indexed validationID,
        bytes32 indexed delegationID,
        uint256 stake,
        bool autoRenew,
        uint8 multiplier
    );
    event DelegationWithdrawn(bytes32 indexed delegationID, uint256 stake);
    event DelegationUpdatedAutoRenew(
        bytes32 indexed delegationID,
        bool autoRenew
    );

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
    function addValidator(
        address node,
        uint32 period
    ) public payable {
        require(msg.value > 0, "value is empty");
        (bytes32 id, string memory error) = StakerNative(address(this))
            .native_addValidator(
                msg.sender,
                node,
                period,
                msg.value
            );
        require(bytes(error).length == 0, error);
        emit ValidatorQueued(
            msg.sender,
            node,
            id,
            period,
            msg.value
        );
    }

    /**
     * @dev increaseStake adds VET to the current stake of the queued/active validator.
     */
    function increaseStake(bytes32 validationID) public payable {
        require(msg.value > 0, "value is empty");
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
    function decreaseStake(bytes32 id, uint256 amount) public {
        require(amount > 0, "amount is empty");
        string memory error = StakerNative(address(this)).native_decreaseStake(
            msg.sender,
            id,
            amount
        );
        require(bytes(error).length == 0, error);
        emit StakeDecreased(msg.sender, id, amount);
    }

    /**
     * @dev allows the caller to withdraw a stake when their status is set to exited
     */
    function withdrawStake(bytes32 id) public {
        (uint256 stake, string memory error) = StakerNative(address(this))
            .native_withdrawStake(msg.sender, id);
        require(bytes(error).length == 0, error);

        (bool success, ) = msg.sender.call{value: stake}("");
        require(success, "Transfer failed");
        emit ValidatorWithdrawn(msg.sender, id, stake);
    }

    /**
     * @dev disableAutoRenew set the autoRenew to false for a validator.
     */
    function disableAutoRenew(bytes32 id) public {
        string memory error = StakerNative(address(this))
            .native_disableAutoRenew(msg.sender, id);
        require(bytes(error).length == 0, error);
        emit ValidatorDisabledAutoRenew(msg.sender, id);
    }

    /**
     * @dev addDelegation creates a delegation position on a validator.
     */
    function addDelegation(
        bytes32 validationID,
        bool autoRenew,
        uint8 multiplier // (% of msg.value) 100 for x1, 200 for x2, etc. This enforces a maximum of 2.56x multiplier
    ) public payable onlyDelegatorContract returns (bytes32) {
        require(msg.value > 0, "value is empty");
        (bytes32 delegationID, string memory error) = StakerNative(
            address(this)
        ).native_addDelegation(validationID, msg.value, autoRenew, multiplier);
        require(bytes(error).length == 0, error);
        emit DelegationAdded(
            validationID,
            delegationID,
            msg.value,
            autoRenew,
            multiplier
        );
        return delegationID;
    }

    /**
     * @dev exitDelegation signals the intent to exit a delegation position at the end of the staking period.
     * Funds are available once the current staking period ends.
     */
    function updateDelegationAutoRenew(
        bytes32 delegationID,
        bool active
    ) public onlyDelegatorContract {
        string memory error = StakerNative(address(this))
            .native_updateDelegationAutoRenew(delegationID, active);
        require(bytes(error).length == 0, error);
        emit DelegationUpdatedAutoRenew(delegationID, active);
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
     * @dev getDelegation returns the amount, multiplier and auto renew for delegator.
     * @return (validationID, stake, startPeriod, endPeriod, multiplier, autoRenew, isLocked)
     */
    function getDelegation(
        bytes32 delegationID
    )
        public
        view
        returns (bytes32, uint256, uint32, uint32, uint8, bool, bool)
    {
        (
            bytes32 validationID,
            uint256 stake,
            uint32 startPeriod,
            uint32 endPeriod,
            uint8 multiplier,
            bool autoRenew,
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
            autoRenew,
            isLocked
        );
    }

    /**
     * @dev get returns the node. endorser, stake, weight, status, auto renew, online and staking period of a validator.
     * @return (node, endorser, stake, weight, status, autoRenew, online, stakingPeriod, startBlock, exitBlock)
     * - status (0: unknown, 1: queued, 2: active, 3: cooldown, 4: exited)
     */
    function get(
        bytes32 id
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
            bool autoRenew,
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
            autoRenew,
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
    function getWithdrawable(bytes32 id) public view returns (uint256) {
        (uint256 withdrawal, string memory error) = StakerNative(address(this))
            .native_getWithdrawable(id);
        require(bytes(error).length == 0, error);
        return withdrawal;
    }

    /**
     * @dev firstActive returns the head validatorId of the active validators.
     */
    function firstActive() public view returns (bytes32) {
        (bytes32 id, string memory error) = StakerNative(address(this))
            .native_firstActive();
        require(bytes(error).length == 0, error);
        return id;
    }

    /**
     * @dev firstQueued returns the head validatorId of the queued validators.
     */
    function firstQueued() public view returns (bytes32) {
        (bytes32 id, string memory error) = StakerNative(address(this))
            .native_firstQueued();
        require(bytes(error).length == 0, error);
        return id;
    }

    /**
     * @dev next returns the validator in a linked list
     */
    function next(bytes32 prev) public view returns (bytes32) {
        (bytes32 id, string memory error) = StakerNative(address(this))
            .native_next(prev);
        require(bytes(error).length == 0, error);
        return id;
    }

    /**
     * @dev getRewards returns the rewards received for validation id and staking period (this function returns full reward for all delegations and validator)
     */
    function getRewards(
        bytes32 validationID,
        uint32 stakingPeriod
    ) public view returns (uint256) {
        (uint256 reward, string memory error) = StakerNative(address(this))
            .native_getRewards(validationID, stakingPeriod);
        require(bytes(error).length == 0, error);
        return reward;
    }

    /**
     * @dev getCompletedPeriods returns the number of completed periods for validation
     */
    function getCompletedPeriods(
        bytes32 validationID
    ) public view returns (uint32) {
        (uint32 periods, string memory error) = StakerNative(address(this))
            .native_getCompletedPeriods(validationID);
        require(bytes(error).length == 0, error);
        return periods;
    }

    function getValidatorTotals(bytes32 validationID) public view returns (uint256, uint256, uint256, uint256) {
        (uint256 lockedStake, uint256 lockedWeight, uint256 delegatorsStake, uint256 delegatorsWeight, string memory error) = StakerNative(address(this))
            .native_getValidatorTotals(validationID);
        require(bytes(error).length == 0, error);
        return (lockedStake, lockedWeight, delegatorsStake, delegatorsWeight);
    }

    modifier onlyDelegatorContract() {
        (address sender, string memory error) = StakerNative(address(this))
            .native_getDelegatorContract();
        require(bytes(error).length == 0, error);
        require(msg.sender == sender, "builtin: only delegator");
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
    ) external returns (bytes32, string calldata);

    function native_increaseStake(
        address endorsor,
        bytes32 validationID,
        uint256 amount
    ) external returns (string calldata);

    function native_decreaseStake(
        address endorsor,
        bytes32 validationID,
        uint256 amount
    ) external returns (string calldata);

    function native_withdrawStake(
        address endorsor,
        bytes32 validationID
    ) external returns (uint256, string calldata);

    function native_disableAutoRenew(
        address endorsor,
        bytes32 validationID
    ) external returns (string calldata);

    function native_addDelegation(
        bytes32 validationID,
        uint256 stake,
        bool autoRenew,
        uint8 multiplier
    ) external returns (bytes32, string calldata);

    function native_withdrawDelegation(
        bytes32 delegationID
    ) external returns (uint256, string calldata);

    function native_updateDelegationAutoRenew(
        bytes32 delegationID,
        bool autoRenew
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
            bytes32,
            uint256,
            uint32,
            uint32,
            uint8,
            bool,
            bool,
            string calldata
        );

    function native_get(
        bytes32 validationID
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
            bool,
            uint32,
            uint32,
            uint32,
            string calldata
        );

    function native_lookupNode(
        address node
    ) external view returns (bytes32);

    function native_getWithdrawable(
        bytes32 validationID
    ) external view returns (uint256, string calldata);

    function native_firstActive()
        external
        view
        returns (bytes32, string calldata);

    function native_firstQueued()
        external
        view
        returns (bytes32, string calldata);

    function native_next(
        bytes32 prev
    ) external view returns (bytes32, string calldata);

    function native_getDelegatorContract()
        external
        view
        returns (address, string calldata);

    function native_getRewards(
        bytes32 validationID,
        uint32 stakingPeriod
    ) external view returns (uint256, string calldata);

    function native_getCompletedPeriods(
        bytes32 validationID
    ) external view returns (uint32, string calldata);

    function native_getValidatorTotals(bytes32 validationID) external view returns (uint256, uint256, uint256, uint256, string calldata);
}
