//SPDX-License-Identifier: MIT
pragma solidity ^0.8.20;

struct Node {
    address next;
    address prev;
}

enum ValidatorStatus {
    Unknown,
    Queued,
    Active,
    Exited
}

// TODO: Convert VET and weight amounts to uint32 (divide by 1e18). Will only consume 3 slots
// TODO: Must copy changes to validation.go
struct Validation {
    // ---- Slot 0 ----
    address endorser; // 20 bytes
    uint32 period; // 4 bytes
    uint32 completeIterations; // 4 bytes
    ValidatorStatus status; // 1 byte
    bool online; // 1 byte
    // ---- Slot 1 ----
    uint32 startBlock; // 4 bytes
    uint32 exitBlock; // 4 bytes
    // ---- Slots 2–7 ---- (each one uint256)
    uint256 lockedVET;
    uint256 pendingUnlockVET;
    uint256 queuedVET;
    uint256 cooldownVET;
    uint256 withdrawableVET;
    uint256 weight;
}

// TODO: Convert VET and weight amounts to uint32 (divide by 1e18). Will only consume 1 slot
// TODO: Must copy changes to aggregation.go
struct Aggregation {
    uint256 lockedVET; // VET locked this period (autoRenew == true)
    uint256 lockedWeight; // Weight including multipliers
    uint256 pendingVET; // VET that is pending to be locked in the next period (autoRenew == false)
    uint256 pendingWeight; // Weight including multipliers
    uint256 exitingVET; // VET that is exiting the next period
    uint256 exitingWeight; // Weight including multipliers
}

struct Delegation {
    // ---- Slot 0 ----
    address validator; // 20 bytes
    uint8 multiplier; // 1 byte
    uint32 firstIteration; // 4 bytes
    uint32 lastIteration; // 4 bytes
    // total: 29 bytes (fits in one slot)

    // ---- Slot 1 ----
    uint256 stake; // full 32 bytes
}

interface POAContract {
    function get(
        address _nodeMaster
    )
        external
        view
        returns (bool listed, address endorsor, bytes32 identity, bool active);
}

contract Staker {
    uint256 public constant MAX_STAKE = 600e6 * 1e18; // 600 million VET
    uint256 public constant MIN_STAKE = 25e6 * 1e18; // 25 million VET
    uint8 public constant VALIDATOR_MULTIPLIER = 200;

    // slot 0
    uint256 private _unused;

    // slot 1-4 (contract totals)
    uint256 private _lockedVET;
    uint256 private _lockedWeight;
    uint256 private _queuedVET;
    uint256 private _queuedWeight;

    // slot 5-8 (LinkedList)
    mapping(address => Node) private _queuedValidators;
    address private _firstQueuedValidator;
    address private _lastQueuedValidator;
    uint256 private _queuedValidatorsCount;

    // slot 9-12 (LinkedList)
    mapping(address => Node) private _activeValidators;
    address private _firstActiveValidator;
    address private _lastActiveValidator;
    uint256 private _activeValidatorsCount;

    // slot 13
    mapping(address => Validation) private _validators;
    // slot 14
    mapping(uint32 => address) private _exitBlocks;
    // slot 15
    mapping(address => Aggregation) private _aggregations;

    // slot 16
    mapping(uint256 => Delegation) private _delegations;
    // slot 17
    uint256 private _delegationCounter;
    // slot 18
    mapping(bytes32 => uint256) private _delegatorsRewards;

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

    event DelegationAdded(
        address indexed validator,
        uint256 indexed delegationID,
        uint256 stake,
        uint8 multiplier
    );
    event DelegationWithdrawn(uint256 indexed delegationID, uint256 stake);
    event DelegationSignaledExit(uint256 indexed delegationID);

    function isActive() public view returns (bool active) {
        return _activeValidatorsCount > 0;
    }

    function totalStake()
        public
        view
        returns (uint256 lockedVET, uint256 lockedWeight)
    {
        return (_lockedVET, _lockedWeight);
    }

    function queuedStake()
        public
        view
        returns (uint256 queuedVET, uint256 queuedWeight)
    {
        return (_queuedVET, _queuedWeight);
    }

    function addValidation(
        address validator,
        uint32 period
    )
        external
        payable
        checkStake(msg.value)
        checkAuthority(validator, msg.sender)
    {
        (uint32 lowPeriod, uint32 midPeriod, uint32 highPeriod) = StakerNative(
            address(this)
        ).native_stakingPeriods();
        require(
            period == lowPeriod || period == midPeriod || period == highPeriod,
            "invalid staking period"
        );
        require(
            _validators[validator].status == ValidatorStatus.Unknown,
            "validator already registered"
        );
        require(msg.value <= MAX_STAKE, "stake exceeds maximum allowed");
        require(msg.value >= MIN_STAKE, "stake is below minimum required");

        // store the validator
        Validation storage v = _validators[validator];
        v.endorser = msg.sender;
        v.period = period;
        v.status = ValidatorStatus.Queued;
        v.queuedVET = msg.value;
        v.exitBlock = type(uint32).max; // set to max, will be updated later

        // add to the queue
        _addToQueue(validator);
        // update the total queued stake
        _addQueuedStake(msg.value, VALIDATOR_MULTIPLIER);
        emit ValidationQueued(validator, msg.sender, period, msg.value);
    }

    function increaseStake(
        address validator
    )
        external
        payable
        checkStake(msg.value)
        checkMaxStake(validator, msg.value)
        onlyEndorser(validator)
        queuedOrActive(validator)
    {
        _validators[validator].queuedVET += msg.value;
        _addQueuedStake(msg.value, VALIDATOR_MULTIPLIER);
        emit StakeIncreased(validator, msg.value);
    }

    function decreaseStake(
        address validator,
        uint256 amount
    )
        external
        checkStake(amount)
        checkMinStake(validator, amount)
        onlyEndorser(validator)
        queuedOrActive(validator)
    {
        if (_validators[validator].status == ValidatorStatus.Queued) {
            _validators[validator].queuedVET -= amount;
            _validators[validator].withdrawableVET += amount;
            _removeQueuedStake(amount, VALIDATOR_MULTIPLIER);
        } else {
            _validators[validator].pendingUnlockVET += amount;
        }
        emit StakeDecreased(validator, amount);
    }

    function withdrawStake(address validator) external onlyEndorser(validator) {
        uint256 queued = _validators[validator].queuedVET;
        uint256 withdrawable = _validators[validator].withdrawableVET + queued;

        _removeQueuedStake(queued, VALIDATOR_MULTIPLIER);

        _validators[validator].queuedVET = 0;
        _validators[validator].withdrawableVET = 0;

        if (_validators[validator].status == ValidatorStatus.Queued) {
            // move the validator to exited status and remove from the queue
            _validators[validator].status = ValidatorStatus.Exited;
            _removeFromQueue(validator);
        } else if (_hasPassedCooldown(validator)) {
            withdrawable += _validators[validator].cooldownVET;
            _validators[validator].cooldownVET = 0;
        }

        // send the withdrawable amount
        (bool success, ) = msg.sender.call{value: withdrawable}("");
        require(success, "withdraw failed");
        emit ValidationWithdrawn(validator, withdrawable);
    }

    function signalExit(address validator) external onlyEndorser(validator) {
        require(
            _validators[validator].status == ValidatorStatus.Active,
            "validator not active"
        );
        require(
            _validators[validator].exitBlock != type(uint32).max,
            "exit already signaled or not active"
        );

        uint32 minExitBlock = _validators[validator].startBlock +
            _validators[validator].period *
            (_validators[validator].completeIterations + 1); // +1 for current period.

        _validators[validator].exitBlock = _setExitBlock(
            validator,
            minExitBlock
        );
        emit ValidationSignaledExit(validator);
    }

    function addDelegation(
        address validator,
        uint8 multiplier // (% of msg.value) 100 for x1, 200 for x2, etc. This enforces a maximum of 2.56x multiplier
    )
        external
        payable
        onlyDelegatorContract
        checkStake(msg.value)
        queuedOrActive(validator)
        checkMaxStake(validator, msg.value)
        returns (uint256 delegationID)
    {
        // validation checks
        require(multiplier != 0, "multiplier cannot be zero");
        require(multiplier <= 256, "multiplier cannot exceed 256 (2.56x)"); // 256 is the maximum multiplier allowed

        // create a new delegation
        _delegationCounter++;
        Delegation storage delegation = _delegations[_delegationCounter];
        delegation.validator = validator;
        delegation.multiplier = multiplier;
        delegation.stake = msg.value;
        delegation.lastIteration = type(uint32).max;
        delegation.firstIteration = _validatorNextPeriod(validator);

        // update the validator's aggregation
        _aggregations[validator].pendingVET += msg.value;
        _aggregations[validator].pendingWeight += _calcWeight(
            msg.value,
            multiplier
        );

        // update contract totals
        _addQueuedStake(msg.value, multiplier);

        emit DelegationAdded(
            validator,
            _delegationCounter,
            msg.value,
            multiplier
        );
        return _delegationCounter;
    }

    function signalDelegationExit(
        uint256 delegationID
    ) external onlyDelegatorContract {
        // validation checks
        require(_delegations[delegationID].stake > 0, "delegation not found");
        require(
            _delegations[delegationID].lastIteration == type(uint32).max,
            "delegation already exiting or exited"
        );
        address validator = _delegations[delegationID].validator;
        require(
            _validators[validator].status == ValidatorStatus.Active,
            "validator not active"
        );

        // update the delegation
        _delegations[delegationID].lastIteration = _validatorNextPeriod(
            validator
        );

        // update the validator's aggregation
        _aggregations[validator].exitingVET += _delegations[delegationID].stake;
        _aggregations[validator].exitingWeight += _calcWeight(
            _delegations[delegationID].stake,
            _delegations[delegationID].multiplier
        );

        emit DelegationSignaledExit(delegationID);
    }

    function withdrawDelegation(
        uint256 delegationID
    ) external onlyDelegatorContract {
        // validation checks
        require(_delegations[delegationID].stake > 0, "delegation not found");
        address validator = _delegations[delegationID].validator;
        bool started = _hasDelegationStarted(delegationID, validator);
        require(
            _isDelegationLocked(delegationID, validator) == false,
            "delegation is locked"
        );

        uint256 stake = _delegations[delegationID].stake;
        uint8 multiplier = _delegations[delegationID].multiplier;

        // update the aggregations if still queued
        if (!started) {
            _aggregations[validator].pendingVET -= _delegations[delegationID]
                .stake;
            _aggregations[validator].pendingWeight -= _calcWeight(
                stake,
                multiplier
            );
            _removeQueuedStake(stake, multiplier);
        }

        _delegations[delegationID].stake = 0;

        (bool success, ) = msg.sender.call{value: stake}("");
        require(success, "withdraw failed");
        emit DelegationWithdrawn(delegationID, stake);
    }

    function getDelegationStake(
        uint256 delegationID
    )
        external
        view
        returns (address validator, uint256 stakeVET, uint8 multiplier)
    {
        return (
            _delegations[delegationID].validator,
            _delegations[delegationID].stake,
            _delegations[delegationID].multiplier
        );
    }

    function getDelegationPeriodDetails(
        uint256 delegationID
    )
        external
        view
        returns (uint32 firstIteration, uint32 lastIteration, bool isLocked)
    {
        return (
            _delegations[delegationID].firstIteration,
            _delegations[delegationID].lastIteration,
            _isDelegationLocked(
                delegationID,
                _delegations[delegationID].validator
            )
        );
    }

    function getValidatorStake(
        address validator
    )
        external
        view
        returns (
            address endorsor,
            uint256 lockedVET, // validator only
            uint256 combinedLockedWeight, // validator + delegations
            uint256 queuedVET // validator only
        )
    {
        Validation storage v = _validators[validator];
        return (v.endorser, v.lockedVET, v.weight, v.queuedVET);
    }

    function getValidatorStatus(
        address validator
    ) public view returns (uint8 status, bool online) {
        Validation storage v = _validators[validator];
        return (uint8(v.status), v.online);
    }

    function getValidatorPeriodDetails(
        address validator
    )
        external
        view
        returns (
            uint32 stakingPeriodLength,
            uint32 startBlock,
            uint32 exitBlock,
            uint32 completeIterations
        )
    {
        Validation storage v = _validators[validator];
        return (v.period, v.startBlock, v.exitBlock, v.completeIterations);
    }

    function getWithdrawable(
        address id
    ) external view returns (uint256 withdrawableVET) {
        uint256 withdrawable = _validators[id].withdrawableVET;
        if (_validators[id].status == ValidatorStatus.Queued) {
            withdrawable += _validators[id].queuedVET;
        } else if (_hasPassedCooldown(id)) {
            withdrawable += _validators[id].cooldownVET;
        }
        return withdrawable;
    }

    function firstActive() external view returns (address) {
        return _firstActiveValidator;
    }

    function firstQueued() external view returns (address) {
        return _firstQueuedValidator;
    }

    function next(address prev) external view returns (address) {
        address activeNext = _activeValidators[prev].next;
        if (activeNext != address(0)) {
            return activeNext;
        }
        return _queuedValidators[prev].next;
    }

    function getDelegatorsRewards(
        address validator,
        uint32 stakingPeriod
    ) external view returns (uint256) {
        bytes32 key = keccak256(abi.encodePacked(validator, stakingPeriod));
        return _delegatorsRewards[key];
    }

    function getValidationTotals(
        address validator
    )
        external
        view
        returns (
            uint256 combinedLockedVET,
            uint256 combinedLockedWeight,
            uint256 combinedQueuedVET,
            uint256 combinedQueuedWeight,
            uint256 combinedExitingVET,
            uint256 combinedExitingWeight
        )
    {
        Validation memory v = _validators[validator];
        Aggregation memory agg = _aggregations[validator];

        uint256 exitingVET;
        uint256 exitingWeight;

        if (
            v.status == ValidatorStatus.Active &&
            v.exitBlock != type(uint32).max
        ) {
            exitingVET = v.lockedVET + agg.lockedVET;
            exitingWeight = v.weight;
        } else {
            exitingVET = v.pendingUnlockVET + agg.exitingVET;
            exitingWeight =
                _calcWeight(v.pendingUnlockVET, VALIDATOR_MULTIPLIER) +
                agg.exitingWeight;
        }

        uint256 queuedVET = v.queuedVET + agg.pendingVET;
        uint256 queuedWeight = _calcWeight(v.queuedVET, VALIDATOR_MULTIPLIER) +
            agg.pendingWeight;

        return (
            v.lockedVET + agg.lockedVET,
            v.weight,
            queuedVET,
            queuedWeight,
            exitingVET,
            exitingWeight
        );
    }

    function getValidatorsNum()
        external
        view
        returns (uint256 activeValidatorCount, uint256 queuedValidatorCount)
    {
        return (_activeValidatorsCount, _queuedValidatorsCount);
    }

    function issuance() external view returns (uint256 blockIssuanceVTHO) {
        (uint256 issuanceAmount, string memory error) = StakerNative(
            address(this)
        ).native_issuance();
        require(bytes(error).length == 0, error);
        return issuanceAmount;
    }

    function _addToQueue(address validator) internal {
        Node memory node = Node({next: address(0), prev: _lastQueuedValidator});
        if (_queuedValidatorsCount == 0) {
            _firstQueuedValidator = validator;
        } else {
            _queuedValidators[_lastQueuedValidator].next = validator;
        }
        _lastQueuedValidator = validator;
        _queuedValidators[validator] = node;
        _queuedValidatorsCount++;
    }

    function _removeFromQueue(address validator) internal {
        Node storage node = _queuedValidators[validator];
        if (node.prev != address(0)) {
            // link the previous node to the next
            _queuedValidators[node.prev].next = node.next;
        } else {
            // this is the first node in the queue
            _firstQueuedValidator = node.next;
        }
        if (node.next != address(0)) {
            // link the next node to the previous
            _queuedValidators[node.next].prev = node.prev;
        } else {
            // this is the last node in the queue
            _lastQueuedValidator = node.prev;
        }
        delete _queuedValidators[validator];
        _queuedValidatorsCount--;
    }

    function _setExitBlock(
        address validator,
        uint32 minBlock
    ) internal returns (uint32) {
        uint32 exitBlock = minBlock;
        uint32 epochLength;
        while (_exitBlocks[exitBlock] != address(0)) {
            if (epochLength == 0) {
                epochLength = StakerNative(address(this)).native_epochLength();
            }
            exitBlock = exitBlock + epochLength;
        }
        _exitBlocks[exitBlock] = validator;
        return exitBlock;
    }

    function _hasPassedCooldown(
        address validator
    ) internal view returns (bool) {
        uint32 cooldownPeriod = StakerNative(address(this))
            .native_cooldownPeriod();
        return
            _validators[validator].status == ValidatorStatus.Exited && // validator has exited
            block.number >= _validators[validator].exitBlock + cooldownPeriod; // cooldown period has passed
    }

    function _addQueuedStake(uint256 amount, uint8 multiplier) internal {
        _queuedVET += amount;
        _queuedWeight += _calcWeight(amount, multiplier);
    }

    function _removeQueuedStake(uint256 amount, uint8 multiplier) internal {
        _queuedVET -= amount;
        _queuedWeight -= _calcWeight(amount, multiplier);
    }

    function _validatorNextPeriod(
        address validator
    ) internal view returns (uint32) {
        if (_validators[validator].status == ValidatorStatus.Queued) {
            return 1;
        }

        // +2, 1 for the current period, 1 for the next
        return _validators[validator].completeIterations + 2;
    }

    function _calcWeight(
        uint256 amount,
        uint8 multiplier
    ) internal pure returns (uint256) {
        return (amount * multiplier) / 100; // Convert percentage to multiplier
    }

    function _isDelegationLocked(
        uint256 delegationID,
        address validator
    ) internal view returns (bool) {
        if (_validators[validator].status != ValidatorStatus.Active) {
            return false;
        }
        bool started = _hasDelegationStarted(delegationID, validator);
        bool ended = _hasDelegationEnded(delegationID, validator);
        return started && !ended;
    }

    function _hasDelegationStarted(
        uint256 delegationID,
        address validator
    ) internal view returns (bool) {
        if (_validators[validator].status != ValidatorStatus.Active) {
            return false;
        }
        uint32 first = _delegations[delegationID].firstIteration;
        uint32 completed = _validators[validator].completeIterations;
        return first >= completed;
    }

    function _hasDelegationEnded(
        uint256 delegationID,
        address validator
    ) internal view returns (bool) {
        return
            _delegations[delegationID].lastIteration <=
            _validators[validator].completeIterations;
    }

    modifier checkMaxStake(address validator, uint256 amount) {
        uint256 validatorNextPeriodTVL = _validators[validator].queuedVET +
            _validators[validator].lockedVET -
            _validators[validator].pendingUnlockVET;
        uint256 aggregationNextPeriodTVL = _aggregations[validator].pendingVET +
            _aggregations[validator].lockedVET -
            _aggregations[validator].exitingVET;
        uint256 totalNextPeriodTVL = validatorNextPeriodTVL +
            aggregationNextPeriodTVL +
            amount;
        require(
            totalNextPeriodTVL <= MAX_STAKE,
            "stake exceeds maximum allowed for validation"
        );
        _;
    }

    modifier onlyEndorser(address validator) {
        require(
            _validators[validator].endorser == msg.sender,
            "not endorser of the validator"
        );
        _;
    }

    modifier queuedOrActive(address validator) {
        require(
            _validators[validator].status == ValidatorStatus.Queued ||
                _validators[validator].status == ValidatorStatus.Active,
            "validator not queued or active"
        );
        _;
    }

    modifier onlyDelegatorContract() {
        require(
            msg.sender ==
                StakerNative(address(this)).native_delegatorContract(),
            "builtin: only delegator"
        );
        _;
    }

    modifier checkStake(uint256 amount) {
        require(amount > 0, "stake is empty");
        require(amount % 1e18 == 0, "stake is not multiple of 1VET");
        _;
    }

    modifier checkMinStake(address validator, uint256 amount) {
        ValidatorStatus status = _validators[validator].status;
        if (status == ValidatorStatus.Queued) {
            require(
                _validators[validator].queuedVET - amount >= MIN_STAKE,
                "not enough queued stake"
            );
        }

        if (status == ValidatorStatus.Active) {
            uint256 currentNextPeriod = _validators[validator].lockedVET -
                _validators[validator].pendingUnlockVET;
            require(
                currentNextPeriod - amount >= MIN_STAKE,
                "not enough locked stake"
            );
        }
        _;
    }

    modifier checkAuthority(address validator, address endorser) {
        if (!isActive()) {
            (
                bool listed,
                address currentEndorser,
                bytes32 identity,
                bool active
            ) = POAContract(0x0000000000000000000000417574686f72697479).get(
                    validator
                );
            require(listed, "validator not listed");
            require(currentEndorser == endorser, "endorser mismatch");
        }
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
    function native_issuance() external view returns (uint256, string calldata);
    function native_delegatorContract() external view returns (address);

    // config
    function native_stakingPeriods()
        external
        view
        returns (uint32, uint32, uint32);
    function native_epochLength() external view returns (uint32);
    function native_cooldownPeriod() external view returns (uint32);
}
