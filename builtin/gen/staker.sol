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
    event ValidatorWithdrawn(
        address indexed endorsor,
        bytes32 indexed validationID,
        uint256 stake
    );
    event ValidatorUpdatedAutoRenew(
        address indexed endorsor,
        bytes32 indexed validationID,
        bool autoRenew
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
     * @dev totalStake returns all stakes by queued and active validators.
     */
    function totalStake() public view returns (uint256) {
        (uint256 stake, string memory error) = StakerNative(address(this))
            .native_totalStake();
        require(bytes(error).length == 0, error);
        return stake;
    }

    /**
     * @dev queuedStake returns all stakes which are queued
     */
    function queuedStake() public view returns (uint256) {
        (uint256 stake, string memory error) = StakerNative(address(this))
            .native_queuedStake();
        require(bytes(error).length == 0, error);
        return stake;
    }

    /**
     * @dev addValidator adds a validator to the queue.
     */
    function addValidator(
        address master,
        uint32 period,
        bool autoRenew
    ) public payable {
        (bytes32 id, string memory error) = StakerNative(address(this))
            .native_addValidator(
                msg.sender,
                master,
                period,
                msg.value,
                autoRenew
            );
        require(bytes(error).length == 0, error);
        emit ValidatorQueued(
            msg.sender,
            master,
            id,
            period,
            msg.value,
            autoRenew
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
    function withdraw(bytes32 id) public {
        (uint256 stake, string memory error) = StakerNative(address(this))
            .native_withdraw(msg.sender, id);
        require(bytes(error).length == 0, error);
        emit ValidatorWithdrawn(msg.sender, id, stake);

        (bool success, ) = msg.sender.call{value: stake}("");
        require(success, "Transfer failed");
    }

    /**
     * @dev updateAutoRenew updates the autoRenew flag of a validator.
     */
    function updateAutoRenew(bytes32 id, bool autoRenew) public {
        string memory error = StakerNative(address(this))
            .native_updateAutoRenew(msg.sender, id, autoRenew);
        require(bytes(error).length == 0, error);
        emit ValidatorUpdatedAutoRenew(msg.sender, id, autoRenew);
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
     * @return (stake, multiplier, autoRenew, isLocked)
     */
    function getDelegation(
        bytes32 delegationID
    ) public view returns (uint256, uint8, bool, bool) {
        (
            uint256 stake,
            uint8 multiplier,
            bool autoRenew,
            bool isLocked,
            string memory error
        ) = StakerNative(address(this)).native_getDelegation(delegationID);
        require(bytes(error).length == 0, error);
        return (stake, multiplier, autoRenew, isLocked);
    }

    /**
     * @dev get returns the master. endorser, stake, weight, status and auto renew of a validator.
     * @return (master, endorser, stake, weight, status, autoRenew)
     * - status (0: unknown, 1: queued, 2: active, 3: cooldown, 4: exited)
     */
    function get(
        bytes32 id
    ) public view returns (address, address, uint256, uint256, uint8, bool) {
        (
            address master,
            address endorser,
            uint256 stake,
            uint256 weight,
            uint8 status,
            bool autoRenew,
            string memory error
        ) = StakerNative(address(this)).native_get(id);
        require(bytes(error).length == 0, error);
        return (master, endorser, stake, weight, status, autoRenew);
    }

    /**
     * @dev getWithdraw returns the amount of a validator's withdrawal.
     */
    function getWithdraw(bytes32 id) public view returns (uint256) {
        (uint256 withdrawal, string memory error) = StakerNative(address(this))
            .native_getWithdraw(id);
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
    function getRewards(bytes32 validationID, uint32 stakingPeriod) public view returns (uint256) {
        (uint256 reward, string memory error) = StakerNative(address(this))
            .native_getRewards(validationID, stakingPeriod);
        require(bytes(error).length == 0, error);
        return reward;
    }

    /**
     * @dev getCompletedPeriods returns the number of completed periods for validation
     */
    function getCompletedPeriods(bytes32 validationID) public view returns (uint32) {
        (uint32 periods, string memory error) = StakerNative(address(this))
            .native_getCompletedPeriods(validationID);
        require(bytes(error).length == 0, error);
        return periods;
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
        address master,
        uint32 period,
        uint256 stake,
        bool autoRenew
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

    function native_withdraw(
        address endorsor,
        bytes32 validationID
    ) external returns (uint256, string calldata);

    function native_updateAutoRenew(
        address endorsor,
        bytes32 validationID,
        bool autoRenew
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
        returns (uint256, string calldata);

    function native_queuedStake()
        external
        pure
        returns (uint256, string calldata);

    function native_getDelegation(
        bytes32 delegationID
    ) external view returns (uint256, uint8, bool, bool, string calldata);

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
            string calldata
        );

    function native_getWithdraw(
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

    function native_getBlockProposerAndReward(uint32 blockNumber) external view returns (uint256, address, address, bytes32, string calldata);

    function native_getRewards(bytes32 validationID, uint32 stakingPeriod) external view returns (uint256, string calldata);

    function native_getCompletedPeriods(bytes32 validationID) external view returns (uint32, string calldata);
}
